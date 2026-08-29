"use client";

import localforage from "localforage";

import { nanoid } from "nanoid";
import { readImageMeta } from "@/lib/image-utils";
import { apiGet } from "@/services/api/request";
import { useUserStore } from "@/stores/use-user-store";

export type UploadedImage = {
    url: string;
    storageKey: string;
    width: number;
    height: number;
    bytes: number;
    mimeType: string;
};

type UserStorageProviderBase = {
    enabled: boolean;
    name: string;
    endpoint: string;
};

export type UserS3StorageProvider = UserStorageProviderBase & {
    type: "s3";
    region: string;
    bucket: string;
    accessKeyId: string;
    secretAccessKey: string;
    publicBaseUrl: string;
    pathPrefix: string;
};

export type UserWebDAVStorageProvider = UserStorageProviderBase & {
    type: "webdav";
    pathPrefix: string;
    username: string;
    password: string;
};

export type UserStorageProvider = UserS3StorageProvider | UserWebDAVStorageProvider;

type UploadImageOptions = {
    localOnly?: boolean;
};

export type StorageConfig = {
    mode: string;
    allowUserProvider: boolean;
    allowUserGlobalProvider: boolean;
};

const store = localforage.createInstance({ name: "freedom", storeName: "image_files" });
const objectUrls = new Map<string, string>();
const serverUrls = new Map<string, string>();
export const USER_STORAGE_PROVIDER_KEY = "freedom:user_storage_provider";
export const USER_WEBDAV_STORAGE_PROVIDER_KEY = "freedom:user_webdav_storage_provider";
let storageConfigPromise: Promise<StorageConfig> | null = null;

export function canUseGlobalStorage(config: StorageConfig) {
    const user = useUserStore.getState().user;
    return config.mode === "server_sqlite_s3" && Boolean(user && user.role !== "guest" && (user.role === "admin" || config.allowUserGlobalProvider));
}

function isLocalNetworkHost(hostname: string) {
    const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
    if (
        host === "localhost" ||
        host.endsWith(".localhost") ||
        host.endsWith(".local") ||
        host === "host.docker.internal" ||
        host === "::1"
    ) {
        return true;
    }
    if (
        host.includes(":") &&
        (host.startsWith("fc") ||
            host.startsWith("fd") ||
            /^fe[89ab]/.test(host))
    ) {
        return true;
    }
    const parts = host.split(".").map(Number);
    if (
        parts.length !== 4 ||
        parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)
    ) {
        return false;
    }
    const [a, b] = parts;
    return (
        a === 10 ||
        a === 127 ||
        (a === 172 && b >= 16 && b <= 31) ||
        (a === 192 && b === 168) ||
        (a === 169 && b === 254)
    );
}

export function getProxyUrl(url: string): string {
    if (!url.startsWith("http://") && !url.startsWith("https://")) {
        return url;
    }
    try {
        const parsed = new URL(url);
        if (
            isLocalNetworkHost(parsed.hostname) ||
            (typeof window !== "undefined" &&
                parsed.host === window.location.host)
        ) {
            return url;
        }
    } catch {
        return url;
    }
    return `/api/proxy-image?url=${encodeURIComponent(url)}`;
}

// getMediaProxyUrl 与 getProxyUrl 类似，但指向 /api/proxy-media，可代理图片与视频。
// 供 downloadRemoteMedia 等媒体下载/上传场景使用（图片代理仅放行 image/*，会拒绝视频导致 415）。
export function getMediaProxyUrl(url: string): string {
    if (!url.startsWith("http://") && !url.startsWith("https://")) {
        return url;
    }
    try {
        const parsed = new URL(url);
        if (
            isLocalNetworkHost(parsed.hostname) ||
            (typeof window !== "undefined" &&
                parsed.host === window.location.host)
        ) {
            return url;
        }
    } catch {
        return url;
    }
    return `/api/proxy-media?url=${encodeURIComponent(url)}`;
}

export async function uploadImage(input: string | Blob, options: UploadImageOptions = {}): Promise<UploadedImage> {
    const url = typeof input === "string" ? getProxyUrl(input) : input;
    let blob: Blob;
    if (typeof url === "string") {
        const response = await fetch(url);
        if (!response.ok) {
            const payload = await response.json().catch(() => null) as { msg?: string } | null;
            throw new Error(payload?.msg || `代理图片拉取失败：${response.status}`);
        }
        const contentType = response.headers.get("content-type") || "";
        if (contentType.includes("application/json")) {
            const payload = await response.json().catch(() => null) as { msg?: string } | null;
            throw new Error(payload?.msg || "代理图片下载失败");
        }
        blob = await response.blob();
    } else {
        blob = url;
    }
    if (!options.localOnly) {
        const serverUpload = await maybeUploadImageToServer(blob);
        if (serverUpload) return serverUpload;
    }
    const storageKey = `image:${nanoid()}`;
    await store.setItem(storageKey, blob);
    const urlObj = URL.createObjectURL(blob);
    objectUrls.set(storageKey, urlObj);
    const meta = await readImageMeta(urlObj);
    return { url: urlObj, storageKey, width: meta.width, height: meta.height, bytes: blob.size, mimeType: blob.type || meta.mimeType };
}

export async function uploadRemoteImageToServer(url: string, filename: string): Promise<UploadedImage> {
    const response = await fetch(getProxyUrl(url));
    if (!response.ok) {
        const payload = await response.json().catch(() => null) as { msg?: string } | null;
        throw new Error(payload?.msg || "代理图片拉取失败：" + response.status);
    }
    const blob = await response.blob();
    const config = await loadStorageConfig();
    const userProvider = config.allowUserProvider ? loadUserStorageProvider() : null;
    if (!canUseGlobalStorage(config) && !userProvider) throw new Error("服务端对象存储未启用");
    const token = useUserStore.getState().token;
    if (!token) throw new Error("服务端存储需要先登录");
    const formData = new FormData();
    formData.append("file", blob, filename || "image-" + nanoid() + "." + imageExtension(blob.type));
    if (userProvider) formData.append("provider", JSON.stringify(toProviderPayload(userProvider)));
    const uploadResponse = await fetch("/api/v1/files", { method: "POST", headers: { Authorization: "Bearer " + token }, body: formData });
    const payload = (await uploadResponse.json().catch(() => null)) as { code?: number; msg?: string; data?: UploadedImage } | null;
    if (!uploadResponse.ok || payload?.code !== 0 || !payload.data) throw new Error(payload?.msg || "服务端图片上传失败");
    // 用本地 blob 读取宽高，避免用服务端返回 URL 再把图拉一遍
    const localMetaUrl = URL.createObjectURL(blob);
    const meta = await readImageMeta(localMetaUrl);
    URL.revokeObjectURL(localMetaUrl);
    if (payload.data.storageKey?.startsWith("server:")) serverUrls.set(payload.data.storageKey.slice("server:".length), payload.data.url);
    return { ...payload.data, width: payload.data.width || meta.width, height: payload.data.height || meta.height, mimeType: payload.data.mimeType || blob.type || "image/png", bytes: payload.data.bytes || blob.size };
}

/**
 * 前端图片压缩：轻度压缩，保留足够细节供模型参考
 * - 小于 1MB 的图片不压缩，直接返回
 * - 大图缩放到最大 2560px（保留更多细节）
 * - JPEG 质量 92%（高质量，减少细节丢失）
 */
async function compressImage(blob: Blob): Promise<Blob> {
    // 小于 1MB 的图片不压缩，直接返回（保留原图质量）
    if (blob.size < 1024 * 1024) return blob;
    try {
        const img = new Image();
        const objectUrl = URL.createObjectURL(blob);
        await new Promise<void>((resolve, reject) => {
            img.onload = () => resolve();
            img.onerror = () => reject(new Error("图片加载失败"));
            img.src = objectUrl;
        });
        URL.revokeObjectURL(objectUrl);
        // 最大边 2560px，等比缩放（保留更多细节供模型参考）
        const maxDim = 2560;
        let { width, height } = img;
        if (width > maxDim || height > maxDim) {
            const ratio = Math.min(maxDim / width, maxDim / height);
            width = Math.round(width * ratio);
            height = Math.round(height * ratio);
        }
        const canvas = document.createElement("canvas");
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext("2d");
        if (!ctx) return blob;
        ctx.drawImage(img, 0, 0, width, height);
        // 转换为 JPEG，质量 92%（高质量，保留细节）
        const compressed = await new Promise<Blob>((resolve, reject) => {
            canvas.toBlob(
                (b) => (b ? resolve(b) : reject(new Error("压缩失败"))),
                "image/jpeg",
                0.92,
            );
        });
        // 只有压缩后更小时才用压缩结果
        return compressed.size < blob.size ? compressed : blob;
    } catch {
        // 压缩失败则返回原图
        return blob;
    }
}

export function clearStorageConfigCache() {
    storageConfigPromise = null;
}

export async function resolveImageUrl(storageKey?: string, fallback = "") {
    if (!storageKey) return fallback;
    if (storageKey.startsWith("server:")) {
        const id = storageKey.slice("server:".length);
        const cached = objectUrls.get(storageKey);
        if (cached) return cached;
        const blob = await store.getItem<Blob>(storageKey).catch(() => null);
        if (blob) {
            const url = URL.createObjectURL(blob);
            objectUrls.set(storageKey, url);
            return url;
        }
        const cachedUrl = serverUrls.get(id);
        if (cachedUrl) return cachedUrl;
        // 旧的破损 URL（例如 Nginx 旧的 /s3/ 路径、无法访问的旧域名）不能直接当 fallback 先用，
        // 一定要先查 storage_objects.publicUrl，接口拿不到才最后用 fallback。
        const info = await apiGet<{ publicUrl?: string }>(`/api/files/${encodeURIComponent(id)}`).catch(() => null);
        if (!info) return fallback;
        const url = info?.publicUrl || `/api/files/${encodeURIComponent(id)}/content`;
        serverUrls.set(id, url);
        return url;
    }
    const cached = objectUrls.get(storageKey);
    if (cached) return cached;
    const blob = await store.getItem<Blob>(storageKey);
    if (!blob) return fallback;
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    return url;
}

async function maybeUploadImageToServer(blob: Blob): Promise<UploadedImage | null> {
    const config = await loadStorageConfig().catch(() => null);
    const userProvider = config?.allowUserProvider ? loadUserStorageProvider() : null;
    const canUseGlobalProvider = config ? canUseGlobalStorage(config) : false;
    const useServerStorage = canUseGlobalProvider || Boolean(userProvider);
    if (!config || !useServerStorage) return null;
    const token = useUserStore.getState().token;
    if (!token) {
        if (canUseGlobalProvider) throw new Error("服务端存储需要先登录");
        return null;
    }
    // 上传前压缩图片（大图缩放 + JPEG 85% 质量）
    const compressedBlob = await compressImage(blob);
    const formData = new FormData();
    formData.append("file", compressedBlob, `image-${nanoid()}.jpg`);
    if (userProvider) formData.append("provider", JSON.stringify(toProviderPayload(userProvider)));
    const response = await fetch("/api/v1/files", { method: "POST", headers: { Authorization: `Bearer ${token}` }, body: formData });
    const payload = (await response.json().catch(() => null)) as { code?: number; msg?: string; data?: UploadedImage } | null;
    if (!response.ok || payload?.code !== 0 || !payload.data) {
        if (!canUseGlobalProvider) return null;
        throw new Error(payload?.msg || "服务端图片上传失败");
    }
    // 用本地 blob 读取宽高，避免用服务端返回 URL 再把图拉一遍
    const localMetaUrl = URL.createObjectURL(blob);
    const meta = await readImageMeta(localMetaUrl);
    URL.revokeObjectURL(localMetaUrl);
    if (payload.data.storageKey?.startsWith("server:")) serverUrls.set(payload.data.storageKey.slice("server:".length), payload.data.url);
    return { ...payload.data, width: payload.data.width || meta.width, height: payload.data.height || meta.height, mimeType: payload.data.mimeType || blob.type || "image/png", bytes: payload.data.bytes || blob.size };
}

export async function loadStorageConfig() {
    storageConfigPromise ||= apiGet<StorageConfig>("/api/storage/config");
    return storageConfigPromise;
}

function imageExtension(mimeType: string) {
    if (mimeType === "image/jpeg") return "jpg";
    if (mimeType === "image/webp") return "webp";
    return "png";
}

export async function getImageBlob(storageKey: string) {
    return store.getItem<Blob>(storageKey);
}

export async function setImageBlob(storageKey: string, blob: Blob) {
    await store.setItem(storageKey, blob);
    const url = URL.createObjectURL(blob);
    objectUrls.set(storageKey, url);
    return url;
}

// 参考图 dataUrl 内存缓存：同一张参考图多次生图时只 fetch+base64 编码一次，避免重复网络拉取与编码开销。
// key 为 storageKey 或原始 url（临时 blob: URL 不缓存，因其随时可能被 revoke）。
const referenceDataUrlCache = new Map<string, string>();

// 参考图拉取超时：厂商 CDN / 外部图片代理可能长时间无响应，避免参考图转换无限等待（线上曾因厂商 CDN 挂起导致 fetch 超时后整批生图失败）。
const REFERENCE_FETCH_TIMEOUT_MS = 30_000;

async function fetchReferenceUrl(url: string) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), REFERENCE_FETCH_TIMEOUT_MS);
    try {
        return await fetch(url, { signal: controller.signal });
    } finally {
        clearTimeout(timer);
    }
}

function referenceHost(url: string) {
    try {
        return new URL(url).host;
    } catch {
        return url;
    }
}

function referenceFetchError(error: unknown, url: string) {
    const host = referenceHost(url);
    if (error instanceof DOMException && error.name === "AbortError") return `参考图加载超时（${host}）`;
    if (error instanceof TypeError) return `参考图加载失败（${host} 不可访问）`;
    return error instanceof Error ? error.message : "参考图加载失败";
}

export async function imageToDataUrl(image: { url?: string; dataUrl?: string; storageKey?: string }) {
    // 缓存命中：同一张参考图多次生图时直接复用，避免重复 fetch+base64 编码
    const cacheKey = image.storageKey || image.url || image.dataUrl || "";
    if (cacheKey && !cacheKey.startsWith("blob:") && referenceDataUrlCache.has(cacheKey)) {
        return referenceDataUrlCache.get(cacheKey)!;
    }
    const serverObjectId = image.storageKey?.startsWith("server:") ? image.storageKey.slice("server:".length) : "";
    const urls = [
        image.dataUrl && !image.dataUrl.startsWith("blob:") ? image.dataUrl : "",
        image.url && !image.url.startsWith("blob:") ? image.url : "",
        serverObjectId ? `/api/files/${encodeURIComponent(serverObjectId)}/content` : "",
        !serverObjectId ? await resolveImageUrl(image.storageKey, image.url || image.dataUrl || "") : "",
    ].filter((url, index, list): url is string => Boolean(url) && list.indexOf(url) === index);
    if (!urls.length) return "";
    let lastError = "";
    for (const url of urls) {
        if (url.startsWith("data:")) return url;
        try {
            const proxyUrl = getProxyUrl(url);
            const response = await fetchReferenceUrl(proxyUrl);
            if (!response.ok) {
                lastError = `参考图加载失败：HTTP ${response.status}`;
                continue;
            }
            const dataUrl = await blobToDataUrl(await response.blob());
            // 写入缓存：同一张参考图后续生图直接复用（临时 blob: URL 不缓存）
            if (cacheKey && !cacheKey.startsWith("blob:")) referenceDataUrlCache.set(cacheKey, dataUrl);
            return dataUrl;
        } catch (error) {
            lastError = referenceFetchError(error, url);
        }
    }
    throw new Error(lastError || "参考图加载失败");
}

// 批量解析参考图：单张失败跳过（不 abort 整批，避免一张坏图导致整次生成失败），全部失败才抛错。
export async function resolveReferenceDataUrls(references: Array<{ url?: string; dataUrl?: string; storageKey?: string }>) {
    if (!references.length) return [];
    const results = await Promise.allSettled(references.map((image) => imageToDataUrl(image)));
    const rejected = results.filter((result): result is PromiseRejectedResult => result.status === "rejected");
    if (rejected.length === results.length) {
        throw rejected[0].reason instanceof Error ? rejected[0].reason : new Error("参考图加载失败");
    }
    if (rejected.length) {
        console.warn(`[image-storage] ${rejected.length}/${results.length} 张参考图无法加载，已跳过`, rejected.map((result) => result.reason));
    }
    return results.filter((result): result is PromiseFulfilledResult<string> => result.status === "fulfilled").map((result) => result.value);
}

export async function deleteStoredImages(keys: Iterable<string>) {
    const { useAssetStore } = await import("@/stores/use-asset-store");
    const assetKeys = new Set(
        useAssetStore.getState().assets
            .map((a) => (a.kind !== "text" ? a.data.storageKey : null))
            .filter((k): k is string => Boolean(k))
    );
    await Promise.all(
        Array.from(new Set(keys)).map(async (key) => {
            if (assetKeys.has(key)) return;
            if (key.startsWith("server:")) {
                await deleteServerImage(key);
                return;
            }
            const url = objectUrls.get(key);
            if (url) URL.revokeObjectURL(url);
            objectUrls.delete(key);
            await store.removeItem(key);
        }),
    );
}

export async function cleanupUnusedImages(usedData: unknown) {
    const usedKeys = collectImageStorageKeys(usedData);
    const unused: string[] = [];
    await store.iterate((_value, key) => {
        if (!usedKeys.has(key)) unused.push(key);
    });
    await deleteStoredImages(unused);
}

export function collectImageStorageKeys(value: unknown, keys = new Set<string>()) {
    if (typeof value === "string") {
        if (value.startsWith("image:") || value.startsWith("server:")) keys.add(value);
        return keys;
    }
    if (!value || typeof value !== "object") return keys;
    if ("storageKey" in value && typeof value.storageKey === "string" && (value.storageKey.startsWith("image:") || value.storageKey.startsWith("server:"))) keys.add(value.storageKey);
    Object.values(value).forEach((item) => (Array.isArray(item) ? item.forEach((child) => collectImageStorageKeys(child, keys)) : collectImageStorageKeys(item, keys)));
    return keys;
}

export function defaultUserStorageProvider(): UserS3StorageProvider {
    return {
        enabled: false,
        name: "我的 R2",
        type: "s3",
        endpoint: "",
        region: "auto",
        bucket: "",
        accessKeyId: "",
        secretAccessKey: "",
        publicBaseUrl: "",
        pathPrefix: "canvas",
    };
}

export function defaultUserWebDAVStorageProvider(): UserWebDAVStorageProvider {
    return {
        enabled: false,
        name: "我的 WebDAV",
        type: "webdav",
        endpoint: "",
        pathPrefix: "canvas",
        username: "",
        password: "",
    };
}

export function loadUserS3StorageProvider() {
    if (typeof window === "undefined") return null;
    try {
        const parsed = JSON.parse(window.localStorage.getItem(USER_STORAGE_PROVIDER_KEY) || "null") as UserS3StorageProvider | null;
        return parsed ? { ...defaultUserStorageProvider(), ...parsed, type: "s3" as const } : null;
    } catch {
        return null;
    }
}

export function loadUserWebDAVStorageProvider() {
    if (typeof window === "undefined") return null;
    try {
        const parsed = JSON.parse(window.localStorage.getItem(USER_WEBDAV_STORAGE_PROVIDER_KEY) || "null") as UserWebDAVStorageProvider | null;
        return parsed ? { ...defaultUserWebDAVStorageProvider(), ...parsed, type: "webdav" as const } : null;
    } catch {
        return null;
    }
}

export function loadUserStorageProvider(): UserStorageProvider | null {
    const s3 = loadUserS3StorageProvider();
    const webdav = loadUserWebDAVStorageProvider();
    if (s3?.enabled && webdav?.enabled) return null;
    if (s3?.enabled && validS3Provider(s3)) return s3;
    if (webdav?.enabled && validWebDAVProvider(webdav)) return webdav;
    return null;
}

export function saveUserStorageProvider(provider: UserS3StorageProvider) {
    // 安全：不将 secretAccessKey 持久化到 localStorage，仅存非敏感字段。
    // secretAccessKey 只通过后端 API 保存（SaveUserStorageProvider → DB）。
    const { secretAccessKey, ...safeFields } = { ...defaultUserStorageProvider(), ...provider, type: "s3" as const };
    window.localStorage.setItem(USER_STORAGE_PROVIDER_KEY, JSON.stringify(safeFields));
}

export function saveUserWebDAVStorageProvider(provider: UserWebDAVStorageProvider) {
    // 安全：不将 password 持久化到 localStorage，仅存非敏感字段。
    const { password, ...safeFields } = { ...defaultUserWebDAVStorageProvider(), ...provider, type: "webdav" as const };
    window.localStorage.setItem(USER_WEBDAV_STORAGE_PROVIDER_KEY, JSON.stringify(safeFields));
}

function validS3Provider(provider: UserS3StorageProvider) {
    // secretAccessKey 可能不在 localStorage 中（安全策略：不持久化密钥），
    // 只要 endpoint + bucket + accessKeyId 在，就认为配置有效（密钥在后端 DB 中）。
    return Boolean(provider.endpoint && provider.bucket && provider.accessKeyId);
}

function validWebDAVProvider(provider: UserWebDAVStorageProvider) {
    // password 可能不在 localStorage 中（安全策略：不持久化密钥）。
    return Boolean(provider.endpoint && provider.username);
}

export function toProviderPayload(provider: UserStorageProvider) {
    if (provider.type === "webdav") {
        return {
            enabled: provider.enabled,
            name: provider.name,
            type: "webdav" as const,
            endpoint: provider.endpoint,
            pathPrefix: provider.pathPrefix,
            username: provider.username,
            password: provider.password,
        };
    }
    return {
        enabled: provider.enabled,
        name: provider.name,
        type: "s3" as const,
        endpoint: provider.endpoint,
        region: provider.region || "auto",
        bucket: provider.bucket,
        accessKeyId: provider.accessKeyId,
        secretAccessKey: provider.secretAccessKey,
        publicBaseUrl: provider.publicBaseUrl,
        pathPrefix: provider.pathPrefix,
    };
}

async function deleteServerImage(storageKey: string) {
    const id = storageKey.slice("server:".length);
    if (!id) return;
    const token = useUserStore.getState().token;
    serverUrls.delete(id);
    if (!token) return;
    const provider = loadUserStorageProvider();
    const response = await fetch(`/api/v1/files/${encodeURIComponent(id)}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify(provider ? { provider: toProviderPayload(provider) } : {}),
    });
    const payload = (await response.json().catch(() => null)) as { code?: number; msg?: string } | null;
    if (!response.ok || payload?.code !== 0) throw new Error(payload?.msg || "删除服务端图片失败");
}

function blobToDataUrl(blob: Blob) {
    return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ""));
        reader.onerror = () => reject(new Error("读取图片失败"));
        reader.readAsDataURL(blob);
    });
}
