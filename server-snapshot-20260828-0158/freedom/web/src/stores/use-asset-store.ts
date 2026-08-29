"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import { cleanupUnusedImages, resolveImageUrl, uploadImage } from "@/services/image-storage";
import { cleanupUnusedMedia, resolveMediaUrl } from "@/services/file-storage";
import { fetchUserAssetData, syncUserAssetData } from "@/services/api/user-config";

export type AssetKind = "text" | "image" | "video" | "audio";
export type TextAsset = AssetBase<"text"> & { data: { content: string } };
export type ImageAsset = AssetBase<"image"> & { data: { dataUrl: string; storageKey?: string; width: number; height: number; bytes: number; mimeType: string } };
export type VideoAsset = AssetBase<"video"> & { data: { url: string; storageKey?: string; width: number; height: number; bytes: number; mimeType: string } };
export type AudioAsset = AssetBase<"audio"> & { data: { url: string; storageKey?: string; bytes?: number; mimeType: string; durationMs?: number } };
export type Asset = TextAsset | ImageAsset | VideoAsset | AudioAsset;

type AssetBase<T extends AssetKind> = {
    id: string;
    kind: T;
    title: string;
    coverUrl: string;
    tags: string[];
    source?: string;
    note?: string;
    createdAt: string;
    updatedAt: string;
    metadata?: Record<string, unknown>;
};

type AssetStore = {
    assets: Asset[];
    addAsset: (asset: Omit<Asset, "id" | "createdAt" | "updatedAt">) => string;
    updateAsset: (id: string, patch: Partial<Omit<Asset, "id" | "createdAt">>) => void;
    removeAsset: (id: string) => void;
    hydrateAccountAssets: (token: string, syncEnabled?: boolean) => Promise<void>;
    syncAccountAssets: (token: string) => Promise<void>;
    stopAccountAssetSync: () => void;
    cleanupImages: (extra?: unknown) => void;
};

const ASSET_STORE_KEY = "freedom:asset_store";
let activeAssetSyncToken = "";
let accountAssetSyncEnabled = false;
let isHydratingAccountAssets = false;
let syncTimer: number | null = null;
// 资产 URL 解析飞行锁与缓存：避免路由切换时重复解析
// ✅ 优化：冷却提升到 5 分钟；onRehydrateStorage 不再直接触发（进入页面后 store 订阅多次会触发多次 rehydrate）
let assetUrlsResolvedInFlight = false;
let lastAssetUrlsResolvedAt = 0;
const ASSET_URL_RESOLVE_COOLDOWN_MS = 5 * 60_000; // 5 分钟内不再重复解析
let lastRehydrateTriggeredAt = 0;
const REHYDRATE_RESOLVE_DEBOUNCE_MS = 3_000; // 水合后 3 秒内才真正调度解析，抑制路由切换时连续 rehydrate

type AssetSnapshot = { assets: Asset[] };

/**
 * 【异步非阻塞】后台批量解析资产的 URL（storageKey → 可访问 URL / blob URL）
 * 和 base64 → 正式上传迁移。这些操作在水合阶段不阻塞页面渲染，
 * 全部处理完成后再一次性 setState 合并更新。
 */
async function resolveAssetUrlsInBackground(initialAssets: Asset[]) {
    if (assetUrlsResolvedInFlight) return;
    const now = Date.now();
    // 冷却时间内已经解析过，直接跳过
    if (now - lastAssetUrlsResolvedAt < ASSET_URL_RESOLVE_COOLDOWN_MS) return;
    if (!initialAssets.length) {
        lastAssetUrlsResolvedAt = now;
        return;
    }
    assetUrlsResolvedInFlight = true;
    try {
        const resolved = await Promise.all(
            initialAssets.map(async (asset) => {
                try {
                    if (asset.kind === "video" && asset.data.storageKey) {
                        const url = await resolveMediaUrl(asset.data.storageKey, asset.data.url);
                        if (url !== asset.data.url) {
                            return { ...asset, data: { ...asset.data, url } };
                        }
                        return asset;
                    }
                    if (asset.kind === "audio" && asset.data.storageKey) {
                        const url = await resolveMediaUrl(asset.data.storageKey, asset.data.url);
                        if (url !== asset.data.url) {
                            return { ...asset, data: { ...asset.data, url } };
                        }
                        return asset;
                    }
                    if (asset.kind !== "image") return asset;
                    if (asset.data.storageKey) {
                        const [coverUrl, dataUrl] = await Promise.all([
                            asset.coverUrl.startsWith("blob:")
                                ? resolveImageUrl(asset.data.storageKey, asset.coverUrl)
                                : Promise.resolve(asset.coverUrl),
                            resolveImageUrl(asset.data.storageKey, asset.data.dataUrl),
                        ]);
                        if (coverUrl !== asset.coverUrl || dataUrl !== asset.data.dataUrl) {
                            return {
                                ...asset,
                                coverUrl,
                                data: { ...asset.data, dataUrl },
                            };
                        }
                        return asset;
                    }
                    // 旧数据：图片直接保存了 data:image/xxx;base64，懒迁移到正式存储
                    if (!asset.data.dataUrl.startsWith("data:image/")) return asset;
                    const image = await uploadImage(asset.data.dataUrl);
                    const coverUrl = asset.coverUrl.startsWith("data:image/") ? image.url : asset.coverUrl;
                    return {
                        ...asset,
                        coverUrl,
                        data: {
                            ...asset.data,
                            dataUrl: image.url,
                            storageKey: image.storageKey,
                            bytes: image.bytes,
                            mimeType: image.mimeType,
                        },
                    };
                } catch (e) {
                    // 单个资产解析失败不影响其他资产，静默保留原值
                    console.error("Failed to resolve asset url for", asset.id, e);
                    return asset;
                }
            }),
        );
        // 比较是否有实际变更，无变更不触发 re-render
        const hasDiff = resolved.some((a, i) => a !== initialAssets[i]);
        if (hasDiff) {
            useAssetStore.setState({ assets: resolved });
        }
        lastAssetUrlsResolvedAt = Date.now();
    } catch (e) {
        console.error("resolveAssetUrlsInBackground failed", e);
    } finally {
        assetUrlsResolvedInFlight = false;
    }
}

const assetStorage: PersistStorage<AssetStore> = {
    getItem: async (name) => {
        // ✅ 优化：只做纯 JSON 读取并立刻返回，不阻塞在 URL 解析/上传上
        // URL 解析和旧数据迁移放到 onRehydrateStorage 后的后台异步任务
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        return JSON.parse(value) as StorageValue<AssetStore>;
    },
    setItem: (name, value) => localForageStorage.setItem(name, JSON.stringify(value)),
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useAssetStore = create<AssetStore>()(
    persist(
        (set, get) => ({
            assets: [],
            addAsset: (asset) => {
                const now = new Date().toISOString();
                const id = nanoid();
                set((state) => ({ assets: [{ ...asset, id, createdAt: now, updatedAt: now } as Asset, ...state.assets] }));
                scheduleAssetSync(get);
                return id;
            },
            updateAsset: (id, patch) =>
                set((state) => {
                    const assets = state.assets.map((asset) => (asset.id === id ? ({ ...asset, ...patch, updatedAt: new Date().toISOString() } as Asset) : asset));
                    window.setTimeout(() => scheduleAssetSync(get), 0);
                    return { assets };
                }),
            removeAsset: (id) =>
                set((state) => {
                    const deletedAsset = state.assets.find((asset) => asset.id === id);
                    const assets = state.assets.filter((asset) => asset.id !== id);

                    if (deletedAsset && deletedAsset.kind !== "text" && deletedAsset.data.storageKey) {
                        const key = deletedAsset.data.storageKey;
                        window.setTimeout(async () => {
                            const { useCanvasStore } = await import("@/app/(user)/canvas/stores/use-canvas-store");
                            const usedKeys = new Set<string>();
                            // 收集其余资产的 storageKey
                            assets.forEach((a) => {
                                if (a.kind !== "text" && a.data.storageKey) usedKeys.add(a.data.storageKey);
                            });
                            // 收集画布中引用的 storageKey
                            const projects = useCanvasStore.getState().projects;
                            const { collectImageStorageKeys } = await import("@/services/image-storage");
                            const { collectMediaStorageKeys } = await import("@/services/file-storage");
                            collectImageStorageKeys(projects, usedKeys);
                            collectMediaStorageKeys(projects, usedKeys);

                            // 收集本地/云端生图历史与视频历史中的 storageKey，避免生成结果卡片失效
                            try {
                                const localforage = (await import("localforage")).default;
                                const imageLogStore = localforage.createInstance({ name: "freedom", storeName: "image_generation_logs" });
                                await imageLogStore.iterate((log: any) => {
                                    if (log) {
                                        if (Array.isArray(log.images)) {
                                            log.images.forEach((img: any) => {
                                                if (img && img.storageKey) usedKeys.add(img.storageKey);
                                            });
                                        }
                                        if (Array.isArray(log.references)) {
                                            log.references.forEach((ref: any) => {
                                                if (ref && ref.storageKey) usedKeys.add(ref.storageKey);
                                            });
                                        }
                                    }
                                });
                            } catch (e) {
                                console.error("Error iterating image_generation_logs", e);
                            }

                            try {
                                const localforage = (await import("localforage")).default;
                                const videoLogStore = localforage.createInstance({ name: "freedom", storeName: "video_generation_logs" });
                                await videoLogStore.iterate((log: any) => {
                                    if (log) {
                                        if (log.video && log.video.storageKey) {
                                            usedKeys.add(log.video.storageKey);
                                        }
                                        if (Array.isArray(log.references)) {
                                            log.references.forEach((ref: any) => {
                                                if (ref && ref.storageKey) usedKeys.add(ref.storageKey);
                                            });
                                        }
                                    }
                                });
                            } catch (e) {
                                console.error("Error iterating video_generation_logs", e);
                            }

                            // 若全站没有其他地方再引用此 storageKey，则执行真正的物理删除
                            if (!usedKeys.has(key)) {
                                if (key.startsWith("image:") || key.startsWith("server:")) {
                                    const { deleteStoredImages } = await import("@/services/image-storage");
                                    await deleteStoredImages([key]);
                                }
                                if (key.startsWith("file:") || key.startsWith("video:") || key.startsWith("server:")) {
                                    const { deleteStoredMedia } = await import("@/services/file-storage");
                                    await deleteStoredMedia([key]);
                                }
                            }
                        }, 0);
                    }

                    window.setTimeout(() => scheduleAssetSync(get), 0);
                    return { assets };
                }),
            hydrateAccountAssets: async (token, syncEnabled = false) => {
                if (!token) return;
                activeAssetSyncToken = token;
                accountAssetSyncEnabled = syncEnabled;
                isHydratingAccountAssets = true;
                try {
                    const remote = await fetchUserAssetData<AssetSnapshot>(token);
                    const remoteAssets = Array.isArray(remote?.assets) ? remote.assets : [];
                    if (syncEnabled) {
                        set({ assets: remoteAssets });
                    } else {
                        const localHasAssets = get().assets.length > 0;
                        if (!localHasAssets && remoteAssets.length) {
                            set({ assets: remoteAssets });
                        }
                    }
                } finally {
                    isHydratingAccountAssets = false;
                }
            },
            syncAccountAssets: async (token) => {
                if (!token || !accountAssetSyncEnabled) return;
                await syncUserAssetData(token, { assets: get().assets });
            },
            stopAccountAssetSync: () => {
                activeAssetSyncToken = "";
                if (syncTimer) window.clearTimeout(syncTimer);
                syncTimer = null;
            },
            cleanupImages: (extra) => {
                window.setTimeout(async () => {
                    const { useCanvasStore } = await import("@/app/(user)/canvas/stores/use-canvas-store");
                    const logKeys: string[] = [];
                    try {
                        const localforage = (await import("localforage")).default;
                        const imageLogStore = localforage.createInstance({ name: "freedom", storeName: "image_generation_logs" });
                        await imageLogStore.iterate((log: any) => {
                            if (log) {
                                if (Array.isArray(log.images)) {
                                    log.images.forEach((img: any) => {
                                        if (img && img.storageKey) logKeys.push(img.storageKey);
                                    });
                                }
                                if (Array.isArray(log.references)) {
                                    log.references.forEach((ref: any) => {
                                        if (ref && ref.storageKey) logKeys.push(ref.storageKey);
                                    });
                                }
                            }
                        });
                        const videoLogStore = localforage.createInstance({ name: "freedom", storeName: "video_generation_logs" });
                        await videoLogStore.iterate((log: any) => {
                            if (log) {
                                if (log.video && log.video.storageKey) {
                                    logKeys.push(log.video.storageKey);
                                }
                                if (Array.isArray(log.references)) {
                                    log.references.forEach((ref: any) => {
                                        if (ref && ref.storageKey) logKeys.push(ref.storageKey);
                                    });
                                }
                            }
                        });
                    } catch (e) {
                        console.error("Error gathering log keys in cleanupImages", e);
                    }

                    await cleanupUnusedImages({ assets: get().assets, projects: useCanvasStore.getState().projects, extra, logKeys });
                    await cleanupUnusedMedia({ assets: get().assets, projects: useCanvasStore.getState().projects, extra, logKeys });
                }, 0);
            },
        }),
        {
            name: ASSET_STORE_KEY,
            storage: assetStorage,
            partialize: (state) => ({ assets: state.assets }) as StorageValue<AssetStore>["state"],
            onRehydrateStorage: () => (state) => {
                // 本地水合结束后，在后台异步解析资产 URL，不阻塞页面渲染
                // ✅ 优化：加 debounce，路由切换频繁触发 rehydrate 时不会重复调度解析任务
                if (state?.assets) {
                    lastRehydrateTriggeredAt = Date.now();
                    const rehydrateTs = lastRehydrateTriggeredAt;
                    setTimeout(() => {
                        if (rehydrateTs !== lastRehydrateTriggeredAt) return; // 期间又有新的水合，跳过本次
                        void resolveAssetUrlsInBackground(state.assets);
                    }, REHYDRATE_RESOLVE_DEBOUNCE_MS);
                }
            },
        },
    ),
);

function scheduleAssetSync(get: () => AssetStore) {
    if (isHydratingAccountAssets || !activeAssetSyncToken || !accountAssetSyncEnabled || typeof window === "undefined") return;
    if (syncTimer) window.clearTimeout(syncTimer);
    syncTimer = window.setTimeout(() => {
        void get().syncAccountAssets(activeAssetSyncToken).catch(() => {});
    }, 600);
}

export function mergeAssets(remoteAssets: Asset[], localAssets: Asset[]) {
    const records = new Map<string, Asset>();
    [...localAssets, ...remoteAssets].forEach((asset) => {
        const previous = records.get(asset.id);
        if (!previous || Date.parse(asset.updatedAt || "") >= Date.parse(previous.updatedAt || "")) {
            records.set(asset.id, asset);
        }
    });
    return Array.from(records.values()).sort((a, b) => Date.parse(b.updatedAt || "") - Date.parse(a.updatedAt || ""));
}
