"use client";

import { useMemo } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";
import { localForageStorage, persistStorage } from "@/lib/localforage-storage";

import { apiGet } from "@/services/api/request";
import type { AdminPublicSettings } from "@/services/api/admin";
import { getPurchaseConfig } from "@/services/api/license";
import { isVideoModelName, isImageModelName, isAudioModelName, isTextModelName, type ModelCapability } from "@/lib/model-category";
import { useUserStore } from "@/stores/use-user-store";
import { useVendorStore } from "./use-vendor-store";

export type LocalModelChannel = {
    id: string;
    protocol: "openai" | "kie" | "mimo";
    name: string;
    baseUrl: string;
    apiKey: string;
    models: string[];
    modelLabels?: Record<string, string>;
};

export type VideoMultiPromptItem = { prompt: string; duration: string };
export type VideoElementReference = { id: string; kind: "image" | "video" | "audio"; name: string; type: string; dataUrl?: string; url?: string; storageKey?: string; bytes?: number; width?: number; height?: number; durationMs?: number };
export type VideoElementItem = { name: string; description: string; references: VideoElementReference[] };

export type AiConfig = {
    channelMode: "remote" | "local";
    baseUrl: string;
    apiKey: string;
    model: string;
    imageModel: string;
    videoModel: string;
    textModel: string;
    audioModel: string;
    audioVoice: string;
    audioFormat: string;
    audioSpeed: string;
    audioInstructions: string;
    mimoTtsVoice: string;
    mimoTtsFormat: string;
    mimoVoiceDesignPrompt: string;
    videoSeconds: string;
    videoMode: string;
    videoNegativePrompt: string;
    videoMultiShot: string;
    videoShotType: string;
    videoMultiPrompt: VideoMultiPromptItem[];
    videoElementList: VideoElementItem[];
    vquality: string;
    videoGenerateAudio: string;
    videoWatermark: string;
    videoCharacterOrientation: string;
    systemPrompt: string;
    models: string[];
    imageModels: string[];
    videoModels: string[];
    textModels: string[];
    audioModels: string[];
    quality: string;
    size: string;
    videoSize: string;
    count: string;
    canvasImageCount: string;
    timeout: string;
    apiMode: string;
    streamImages: string;
    streamPartialImages: string;
    responseFormatB64Json: string;
    codexCli: string;
    systemPrompts: {
        image: string;
        video: string;
        text: string;
        workflow: string;
        workflowAgent: string;
        storyboardScript: string;
        storyboardVideo: string;
        storyboardImage: string;
    };
    localChannels: LocalModelChannel[];
    publicChannels: Array<{ id?: string; name?: string; baseUrl?: string; models?: string[]; modelLabels?: Record<string, string>; weight?: number; timeout?: number; enabled?: boolean; remark?: string }>;
    // 公开配置里“系统可用模型”表设置的模型别名（模型ID → 别名）；展示优先级高于渠道 modelLabels。空=显示真实模型名。
    modelCostLabels?: Record<string, string>;
    // 供应商模式下的模型别名（模型ID → 显示名），由 buildVendorEffectiveConfig 从模型快照填充。
    vendorModelLabels?: Record<string, string>;
    syncStorageConfig: boolean;
    syncWebDAVStorageConfig: boolean;
    activeChannelId: string;
    imageChannelId: string;
    videoChannelId: string;
    textChannelId: string;
    audioChannelId: string;
    // ================= P0 新增：当前激活的云端供应商 =================
    // - "official" = 走现有官方路径（下面 resolveEffectiveConfig 原逻辑完全不动）：
    //     管理员云端渠道 / 本地直连二选一，allowCustomChannel 生效
    // - 其他 = 切到对应供应商云端；P1 再填模型，P0 这里先把值存起来 UI 用
    activeVendorType: "official" | "updream" | "libtv" | "newwow";
};

export const CONFIG_STORE_KEY = "freedom:ai_config_store";
export type { ModelCapability };

export const defaultConfig: AiConfig = {
    channelMode: "local",
    baseUrl: "https://api.openai.com",
    apiKey: "",
    model: "gpt-image-2",
    imageModel: "gpt-image-2",
    videoModel: "grok-imagine-video",
    textModel: "gpt-5.5",
    audioModel: "gpt-4o-mini-tts",
    audioVoice: "alloy",
    audioFormat: "mp3",
    audioSpeed: "1",
    audioInstructions: "",
    mimoTtsVoice: "冰糖",
    mimoTtsFormat: "wav",
    mimoVoiceDesignPrompt: "",
    videoSeconds: "15",
    videoMode: "std",
    videoNegativePrompt: "",
    videoMultiShot: "false",
    videoShotType: "intelligence",
    videoMultiPrompt: [{ prompt: "", duration: "1" }],
    videoElementList: [{ name: "", description: "", references: [] }],
    vquality: "720",
    videoGenerateAudio: "false",
    videoWatermark: "false",
    videoCharacterOrientation: "video",
    systemPrompt: "",
    models: [],
    imageModels: [],
    videoModels: [],
    textModels: [],
    audioModels: [],
    quality: "auto",
    size: "1:1",
    videoSize: "1280x720",
    count: "1",
    canvasImageCount: "1",
    timeout: "600",
    apiMode: "responses",
    streamImages: "",
    streamPartialImages: "1",
    responseFormatB64Json: "",
    codexCli: "",
    systemPrompts: {
        image: "",
        video: "",
        text: "",
        workflow: "",
        workflowAgent: "",
        storyboardScript: "",
        storyboardVideo: "",
        storyboardImage: "",
    },
    localChannels: [],
    publicChannels: [],
    syncStorageConfig: false,
    syncWebDAVStorageConfig: false,
    activeChannelId: "",
    imageChannelId: "",
    videoChannelId: "",
    textChannelId: "",
    audioChannelId: "",
    // P0 新增：默认走官方云端模式，保持原有行为不变
    activeVendorType: "official",
};

type ConfigStore = {
    config: AiConfig;
    publicSettings: AdminPublicSettings | null;
    purchaseURL: string;
    isPublicSettingsLoading: boolean;
    isConfigOpen: boolean;
    shouldPromptContinue: boolean;
    updateConfig: <K extends keyof AiConfig>(key: K, value: AiConfig[K]) => void;
    loadPublicSettings: () => Promise<void>;
    isAiConfigReady: (config: AiConfig, model: string) => boolean;
    openConfigDialog: (shouldPromptContinue?: boolean) => void;
    setConfigDialogOpen: (isOpen: boolean) => void;
    clearPromptContinue: () => void;
};

// ========== P1：供应商模型快照 → 有效配置 ==========

type VendorModelSnapshotItem = {
    id?: string;
    name?: string;
    capability?: string;
    defaultFor?: string;
    constraints?: Record<string, unknown>;
    extra?: Record<string, unknown>;
};

type VendorModelsSnapshot = {
    imageModels?: VendorModelSnapshotItem[];
    videoModels?: VendorModelSnapshotItem[];
    textModels?: VendorModelSnapshotItem[];
    audioModels?: VendorModelSnapshotItem[];
    modelLabels?: Record<string, string>;
    fetchedAt?: string;
};

// parseVendorModelsSnapshot 解析后端 account.availableModelsJson（结构与文档 §3.3 对齐）。
export function parseVendorModelsSnapshot(json?: string): VendorModelsSnapshot | null {
    const text = (json || "").trim();
    if (!text) return null;
    try {
        return JSON.parse(text) as VendorModelsSnapshot;
    } catch {
        return null;
    }
}

// Bug #3 前端兜底：vendor 模式下，供应商快照可能仍含 rolldek/apimart 上游拒收的 gemini image preview
// 模型（落库过滤前已配比的脏快照、或后端未部署 A 补丁时）。前端在折算 imageModels 与取默认值时都跳过，
// 双保险避免「下拉隐藏但仍被默认选中 / 实际请求 gemini 被后端 4xx 拦截」。
const VENDOR_UNSUPPORTED_IMAGE_PREVIEW_MODELS = new Set<string>([
    "gemini-3-pro-image-preview",
    "gemini-3.1-flash-image-preview",
]);

// buildVendorEffectiveConfig 把激活供应商账户的模型快照折算成前端可直接消费的 AiConfig：
// - 强制 channelMode=remote（图片请求打后端代理，被 vendor dispatch 接管）
// - 用快照里的模型填充 publicChannels / models / imageModels 等，ModelPicker 直接可用
// - 同步刷新 vendorCapabilityIndex，让 modelMatchesCapability 能识别供应商模型 ID
function buildVendorEffectiveConfig(config: AiConfig): AiConfig {
    const vendorType = config.activeVendorType;
    const accounts = useVendorStore.getState().accounts;
    const activeAccount = accounts.find((a) => a.isActive && a.vendorType === vendorType);
    const snapshot = parseVendorModelsSnapshot(activeAccount?.availableModelsJson);

    const index: Record<string, ModelCapability> = {};
    const labels: Record<string, string> = {};
    const imageModels: string[] = [];
    const videoModels: string[] = [];
    const textModels: string[] = [];
    const audioModels: string[] = [];

    const collect = (list: VendorModelSnapshotItem[] | undefined, capability: ModelCapability, target: string[]) => {
        (list || []).forEach((m) => {
            const rawId = (m.id || "").trim();
            if (!rawId) return;
            // 修复：对 vendor 模型 ID 也应用 migrateModelName，确保新旧名统一
            const id = migrateModelName(rawId);
            index[id] = capability;
            target.push(id);
            const name = (m.name || "").trim();
            if (name) labels[id] = name;
        });
    };
    if (snapshot) {
        collect(snapshot.imageModels, "image", imageModels);
        collect(snapshot.videoModels, "video", videoModels);
        collect(snapshot.textModels, "text", textModels);
        collect(snapshot.audioModels, "audio", audioModels);
        Object.assign(labels, snapshot.modelLabels || {});
    }
    // Bug #3 前端兜底：剔除 vendor 快照里上游拒收的 gemini image preview 模型，避免默认选中必败模型
    const filteredImageModels = imageModels.filter((m) => !VENDOR_UNSUPPORTED_IMAGE_PREVIEW_MODELS.has(m));
    // 刷新全局能力索引（modelMatchesCapability 兜底用）
    vendorCapabilityIndex = index;
    // 把"用户最近一次跑这个模型消耗的 power"叠到 labels 上，显示"模型名 · N 积分"
    const powerHistory = activeAccount?.powerHistory || {};
    for (const [modelKey, rec] of Object.entries(powerHistory)) {
        if (!modelKey || !rec?.power) continue;
        const existing = labels[modelKey] || modelKey;
        labels[modelKey] = `${existing} · ${rec.power} 积分`;
    }

    const defaultImageModel =
        (snapshot?.imageModels || []).find((m) => m.defaultFor === "imageModel" && !VENDOR_UNSUPPORTED_IMAGE_PREVIEW_MODELS.has(m.id ?? ""))?.id || filteredImageModels[0] || defaultConfig.imageModel;
    const defaultTextModel =
        (snapshot?.textModels || []).find((m) => m.defaultFor === "textModel")?.id || textModels[0] || defaultConfig.textModel;
    const defaultVideoModel =
        (snapshot?.videoModels || []).find((m) => m.defaultFor === "videoModel")?.id || videoModels[0] || defaultConfig.videoModel;
    const defaultAudioModel =
        (snapshot?.audioModels || []).find((m) => m.defaultFor === "audioModel")?.id || audioModels[0] || defaultConfig.audioModel;
    const allModels = [...filteredImageModels, ...videoModels, ...textModels, ...audioModels];

    return {
        ...config,
        // 供应商模式固定强制走后端代理；前端不直连第三方
        channelMode: "remote" as const,
        localChannels: [],
        // 合成一个虚拟渠道承载供应商模型，ModelPicker 直接消费（展示名走 modelLabels）
        publicChannels: [
            {
                id: `vendor:${vendorType}`,
                name: activeAccount?.displayName || (vendorType || "供应商"),
                baseUrl: "",
                models: allModels,
                modelLabels: labels,
                enabled: true,
            },
        ],
        models: allModels,
        imageModels: filteredImageModels,
        videoModels,
        textModels,
        audioModels,
        // 同步各 capability 的"当前选中模型"，避免激活 vendor 后仍用旧 channel 下的 model ID 导致后端 dispatch 报"模型不存在"
        model: defaultImageModel || defaultConfig.model,
        imageModel: defaultImageModel || defaultConfig.imageModel,
        textModel: defaultTextModel || defaultConfig.textModel,
        videoModel: defaultVideoModel || defaultConfig.videoModel,
        audioModel: defaultAudioModel || defaultConfig.audioModel,
        vendorModelLabels: labels,
        systemPrompts: config.systemPrompts || defaultConfig.systemPrompts,
    };
}

function resolveEffectiveConfig(config: AiConfig, modelChannel: AdminPublicSettings["modelChannel"] | null, canUseRemoteChannel: boolean) {
    // ========== P1：供应商分发 ==========
    // 非 official 时，从激活账户的模型快照构建真实可用模型列表，强制走后端代理（vendor dispatch 接管图片生成）。
    if (config.activeVendorType && config.activeVendorType !== "official") {
        return buildVendorEffectiveConfig(config);
    }
    // ========== 原有 official 模式逻辑（100% 不动） ==========
    // ✅ 商用版本：只要后台已配置 modelChannel（有渠道+可用模型列表），
    // 即使未登录的游客也能通过 remote 模式看到管理员配置的模型下拉框
    const adminHasConfiguredRemote = Boolean(modelChannel && (modelChannel.channels?.length ?? 0) > 0 && (modelChannel.availableModels?.length ?? 0) > 0);
    // 只有"明确允许自定义渠道 + 用户选择了 local 模式"时才走本地直连
    const preferLocal = modelChannel?.allowCustomChannel === true && config.channelMode === "local";
    const channelMode: AiConfig["channelMode"] = adminHasConfiguredRemote && !preferLocal ? "remote" : (canUseRemoteChannel ? (modelChannel?.allowCustomChannel ? config.channelMode : "remote") : "local");
    if (channelMode === "local" || !modelChannel) {
        const localChannels = normalizeLocalChannels(config);
        // 本地直连模式也合并管理员配置的 systemPrompts（空值回退到本地 config）
        const mergedPrompts = modelChannel?.systemPrompts
            ? {
                  image: modelChannel.systemPrompts.image || config.systemPrompts?.image || "",
                  video: modelChannel.systemPrompts.video || config.systemPrompts?.video || "",
                  text: modelChannel.systemPrompts.text || config.systemPrompts?.text || "",
                  workflow: modelChannel.systemPrompts.workflow || config.systemPrompts?.workflow || "",
                  workflowAgent: modelChannel.systemPrompts.workflowAgent || config.systemPrompts?.workflowAgent || "",
                  storyboardScript: modelChannel.systemPrompts.storyboardScript || config.systemPrompts?.storyboardScript || "",
                  storyboardVideo: modelChannel.systemPrompts.storyboardVideo || config.systemPrompts?.storyboardVideo || "",
                  storyboardImage: modelChannel.systemPrompts.storyboardImage || config.systemPrompts?.storyboardImage || "",
              }
            : config.systemPrompts || defaultConfig.systemPrompts;
        // 修复：对 availableModels 也应用 migrateModelName
        const migratedCloudModels = (modelChannel?.availableModels || []).map((m) => migrateModelName(m));
        return {
            ...config,
            channelMode,
            localChannels,
            models: normalizeModelList([...migratedCloudModels, ...localChannels.flatMap((channel) => channel.models)]),
            publicChannels: (modelChannel?.channels || []).map((channel) => ({
                ...channel,
                models: (channel.models || []).map((m) => migrateModelName(m))
            })),
            systemPrompts: mergedPrompts,
        };
    }
    // 修复：对云端模型也应用 migrateModelName，确保新旧名（official-* -> agnes-*）统一，防止重复
    const cloudModels = (modelChannel.availableModels || []).map((m) => migrateModelName(m));
    const localChannels = normalizeLocalChannels(config);
    const localModels = normalizeModelList(localChannels.flatMap((channel) => channel.models));
    // 官方云端下：云端模型 + 本地直连模型合并进 flat models，避免用户选中的本地模型被云端列表校验清空。
    // 实际请求路由仍由 activeChannelKind(按选中模型) 决定，这里只保证下拉/选中态不被破坏。
    const models = normalizeModelList([...cloudModels, ...localModels]);
    const textModels = filterModelsByCapability(models, "text");
    // Bug #3 官方模式前端兜底：与 vendor 模式 buildVendorEffectiveConfig 对齐，把"上游拒收的
    // gemini-3-pro-image-preview / gemini-3.1-flash-image-preview"从 imageModels 列表里剔除。
    // 否则 line 376 的 `imageModels.includes(config.imageModel) ? config.imageModel : imageModels[0]`
    // 会因为 admin 配的 availableModels 仍含这俩 gemini 而保留脏值 effectiveConfig.imageModel，
    // 后续 novel/page.tsx 资产生图拿这个值去调后端，命中 handler/canvas_task.go:50 的 4xx 拦截。
    const imageModels = filterModelsByCapability(models, "image").filter(
        (m) => !VENDOR_UNSUPPORTED_IMAGE_PREVIEW_MODELS.has(m)
    );
    const videoModels = filterModelsByCapability(models, "video");
    const audioModels = filterModelsByCapability(models, "audio");
    // 公开配置"系统可用模型"表里设置的别名（model → label），仅收集非空别名
    const modelCostLabels: Record<string, string> = {};
    for (const cost of modelChannel.modelCosts || []) {
        const label = (cost.label || "").trim();
        if (cost.model && label) modelCostLabels[cost.model] = label;
    }
    return {
        ...config,
        channelMode,
        models,
        imageModels,
        videoModels,
        textModels,
        audioModels,
        // 关键修复（Bug #2 2026-08-24）：当持久化的 imageModel / videoModel / textModel
        // 不在当前可用模型列表时，绝不能静默清零——否则 novel 生图会 fallback 到文本模型
        // （例如 gemini-3-pro-image-preview）走 /images/generations，被后端拦截报"不被上游接受"。
        // 改为回退到同能力列表里第一个真实可用的模型，保证请求永远发出去的是正确类别的模型。
        // 进一步修复：如果同能力列表为空，回退到默认配置中的对应模型，而不是空字符串。
        model: textModels.includes(config.model) ? config.model : textModels[0] || defaultConfig.model,
        imageModel: imageModels.includes(config.imageModel) ? config.imageModel : imageModels[0] || defaultConfig.imageModel,
        videoModel: videoModels.includes(config.videoModel) ? config.videoModel : videoModels[0] || defaultConfig.videoModel,
        textModel: textModels.includes(config.textModel) ? config.textModel : textModels[0] || defaultConfig.textModel,
        audioModel: audioModels.includes(config.audioModel) ? config.audioModel : audioModels[0] || defaultConfig.audioModel,
        systemPrompt: modelChannel.systemPrompt,
        // 把后台配置的 systemPrompts（含分镜剧本/视频/图片三套）合并到生效配置中，
        // 这样 novel/page.tsx 可以直接从 effectiveConfig.systemPrompts 读取管理员自定义值，
        // 而不是用代码里写死的常量。空值保留用户本地 config 中的原值。
        systemPrompts: {
            image: modelChannel.systemPrompts?.image ?? config.systemPrompts?.image ?? "",
            video: modelChannel.systemPrompts?.video ?? config.systemPrompts?.video ?? "",
            text: modelChannel.systemPrompts?.text ?? config.systemPrompts?.text ?? "",
            workflow: modelChannel.systemPrompts?.workflow ?? config.systemPrompts?.workflow ?? "",
            workflowAgent: modelChannel.systemPrompts?.workflowAgent ?? config.systemPrompts?.workflowAgent ?? "",
            storyboardScript: modelChannel.systemPrompts?.storyboardScript ?? config.systemPrompts?.storyboardScript ?? "",
            storyboardVideo: modelChannel.systemPrompts?.storyboardVideo ?? config.systemPrompts?.storyboardVideo ?? "",
            storyboardImage: modelChannel.systemPrompts?.storyboardImage ?? config.systemPrompts?.storyboardImage ?? "",
        },
        publicChannels: modelChannel.channels || [],
        modelCostLabels,
    };
}

function preferredModel(models: string[], predicate: (model: string) => boolean) {
    return models.find(predicate) || "";
}

// 启发式分类统一从 @/lib/model-category 引入；本文件保留 vendorCapabilityIndex
// 作为模型 ID 优先判定的覆盖层（LibTV 模板 UUID 等不走名称启发式）。

// 供应商模型能力索引：buildVendorEffectiveConfig 填充，modelMatchesCapability 兜底查询。
// 供应商模型 ID（如 LibTV 模板 UUID）不走名称启发式，直接用快照里的 capability 判定。
let vendorCapabilityIndex: Record<string, ModelCapability> = {};

export function modelMatchesCapability(model: string, capability?: ModelCapability) {
    if (!capability) return true;
    const vendorCap = vendorCapabilityIndex[model];
    if (vendorCap) return vendorCap === capability;
    if (capability === "image") return isImageModelName(model);
    if (capability === "video") return isVideoModelName(model);
    if (capability === "audio") return isAudioModelName(model);
    return isTextModelName(model);
}

export function filterModelsByCapability(models: string[], capability?: ModelCapability) {
    return capability ? models.filter((model) => modelMatchesCapability(model, capability)) : models;
}

export function selectableModelsByCapability(config: AiConfig, capability?: ModelCapability) {
    return filterModelsByCapability(config.models, capability);
}

function isAiConfigReady(config: AiConfig, model: string) {
    const scoped = { ...config, model };
    const channel = localChannelForActiveModel(scoped);
    const kind = activeChannelKind(scoped);
    return Boolean(model.trim()) && (kind === "remote" || Boolean(channel?.baseUrl.trim() && channel?.apiKey.trim()));
}

export const useConfigStore = create<ConfigStore>()(
    persist(
        (set, get) => ({
            config: defaultConfig,
            publicSettings: null,
            purchaseURL: "",
            isPublicSettingsLoading: false,
            isConfigOpen: false,
            shouldPromptContinue: false,
            updateConfig: (key, value) =>
                set((state) => ({
                    config: {
                        ...state.config,
                        [key]: value,
                    },
                })),
            loadPublicSettings: async () => {
                if (get().isPublicSettingsLoading) return;
                set({ isPublicSettingsLoading: true });
                try {
                    set({ publicSettings: await apiGet<AdminPublicSettings>("/api/settings") });
                } finally {
                    set({ isPublicSettingsLoading: false });
                }
                void getPurchaseConfig()
                    .then((cfg) => set({ purchaseURL: cfg.purchaseURL || "" }))
                    .catch(() => {});
            },
            isAiConfigReady: (config, model) => isAiConfigReady(config, model),
            openConfigDialog: (shouldPromptContinue = false) => set({ isConfigOpen: true, shouldPromptContinue }),
            setConfigDialogOpen: (isConfigOpen) => set({ isConfigOpen }),
            clearPromptContinue: () => set({ shouldPromptContinue: false }),
        }),
        {
            name: CONFIG_STORE_KEY,
            storage: persistStorage,
            partialize: (state) => ({ config: state.config }),
            merge: (persisted, current) => {
                const persistedState = (persisted || {}) as Partial<ConfigStore>;
                const persistedConfig = (persistedState.config || {}) as Partial<AiConfig>;
                // 迁移旧版模型名（official-* → agnes-*），避免脏值残留到下拉/请求
                const migratedConfig: Partial<AiConfig> = persistedConfig
                    ? {
                          ...persistedConfig,
                          model: migrateModelName(persistedConfig.model as string),
                          imageModel: migrateModelName(persistedConfig.imageModel as string),
                          videoModel: migrateModelName(persistedConfig.videoModel as string),
                          textModel: migrateModelName(persistedConfig.textModel as string),
                          audioModel: migrateModelName(persistedConfig.audioModel as string),
                      }
                    : persistedConfig;
                const config = { ...defaultConfig, ...migratedConfig };
                const localChannels = normalizeLocalChannels(config);
                const localModels = normalizeModelList(localChannels.flatMap((channel) => channel.models));
                return {
                    ...current,
                    config: {
                        ...config,
                        localChannels,
                        models: localModels,
                        baseUrl: localChannels[0]?.baseUrl || config.baseUrl,
                        apiKey: localChannels[0]?.apiKey || config.apiKey,
                        imageChannelId: config.imageChannelId || localChannels[0]?.id || "",
                        videoChannelId: config.videoChannelId || localChannels[0]?.id || "",
                        textChannelId: config.textChannelId || localChannels[0]?.id || "",
                        audioChannelId: config.audioChannelId || localChannels[0]?.id || "",
                        activeChannelId: config.activeChannelId || "",
                        syncStorageConfig: config.syncStorageConfig === true,
                        syncWebDAVStorageConfig: config.syncWebDAVStorageConfig === true,
                        channelMode: config.channelMode || "remote",
                        imageModel: config.imageModel || defaultConfig.imageModel,
                        videoModel: config.videoModel || "grok-imagine-video",
                        textModel: config.textModel || defaultConfig.textModel,
                        audioModel: config.audioModel || defaultConfig.audioModel,
                        audioVoice: config.audioVoice || defaultConfig.audioVoice,
                        audioFormat: config.audioFormat || defaultConfig.audioFormat,
                        audioSpeed: config.audioSpeed || defaultConfig.audioSpeed,
                        // ✅ 商用版本：apiMode 强制统一 responses，兼容性更好且无需用户理解技术差异
                        apiMode: "responses",
                        systemPrompts: (config.systemPrompts?.image || config.systemPrompts?.storyboardScript) ? config.systemPrompts : defaultConfig.systemPrompts,
                        audioInstructions: config.audioInstructions || "",
                        videoSeconds: config.videoSeconds || "6",
                        videoMode: config.videoMode || "std",
                        videoNegativePrompt: config.videoNegativePrompt || "",
                        videoMultiShot: config.videoMultiShot || "false",
                        videoShotType: config.videoShotType || "intelligence",
                        videoMultiPrompt: Array.isArray(config.videoMultiPrompt) && config.videoMultiPrompt.length ? config.videoMultiPrompt : defaultConfig.videoMultiPrompt,
                        videoElementList: Array.isArray(config.videoElementList) && config.videoElementList.length ? config.videoElementList : defaultConfig.videoElementList,
                        vquality: config.vquality || "720",
                        videoGenerateAudio: config.videoGenerateAudio || "false",
                        videoWatermark: config.videoWatermark || "false",
                        videoCharacterOrientation: config.videoCharacterOrientation === "image" ? "image" : "video",
                        // ✅ 商用版本：图片生成数量强制合法范围 1-10，默认 1（避免旧持久化里 7 等异常值）
                        count: (() => {
                            const n = Number(config.count);
                            if (!Number.isFinite(n) || n < 1 || n > 10) return "1";
                            return String(Math.floor(n));
                        })(),
                        canvasImageCount: (() => {
                            const n = Number(config.canvasImageCount);
                            if (!Number.isFinite(n) || n < 1 || n > 10) return "1";
                            return String(Math.floor(n));
                        })(),
                        imageModels: filterModelsByCapability(localModels, "image"),
                        videoModels: filterModelsByCapability(localModels, "video"),
                        textModels: filterModelsByCapability(localModels, "text"),
                        audioModels: filterModelsByCapability(localModels, "audio"),
                    },
                };
            },
        },
    ),
);

function normalizeModelList(models: string[]) {
    return Array.from(new Set((models || []).map((model) => model.trim()).filter(Boolean)));
}

// 模型名迁移（Bug #2 后续 2026-08-24）：旧版云端模型列表用 `official-*` 前缀
// （official-image-2.0-flash / official-video-v2.0 / official-2.0-flash 等），
// 现已全部改名为 `agnes-*` 前缀。用户本地持久化的旧名不会出现在当前 availableModels
// 里，导致「下拉显示 official-xxx 但实际请求落到别的模型/被拦截」的困惑。
// 这里在配置 hydrated 时一次性把旧名映射到新名并写回 localStorage，根治脏值。
const MODEL_NAME_MIGRATIONS: Record<string, string> = {
    "official-image-2.0-flash": "agnes-image-2.0-flash",
    "official-image-2.1-flash": "agnes-image-2.1-flash",
    "official-video-v2.0": "agnes-video-v2.0",
    "official-2.0-flash": "agnes-2.0-flash",
    "official-2.5-flash": "agnes-2.5-flash",
    "official-2.5-pro": "agnes-2.5-pro",
};
function migrateModelName(model: string | undefined): string {
    if (!model) return "";
    return MODEL_NAME_MIGRATIONS[model] || model;
}

export function useEffectiveConfig() {
    const config = useConfigStore((state) => state.config);
    const modelChannel = useConfigStore((state) => state.publicSettings?.modelChannel ?? null);
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    // 商用版本：所有已登录用户都必须走"公司后台统一配置的云端渠道 /api/v1/* 代理"，
    // 这样才能扣 Credits 并严格限制可用模型为管理员勾选的 availableModels。
    const canUseRemoteChannel = Boolean(token && user);
    return useMemo(() => resolveEffectiveConfig(config, modelChannel, canUseRemoteChannel), [canUseRemoteChannel, config, modelChannel]);
}

export function buildApiUrl(baseUrl: string, path: string) {
    let normalizedBaseUrl = baseUrl.trim().replace(/\/+$/, "");
    normalizedBaseUrl = normalizeArkPlanBaseUrl(normalizedBaseUrl);
    const lowerBaseUrl = normalizedBaseUrl.toLowerCase();
    const apiBaseUrl = lowerBaseUrl.endsWith("/v1") || lowerBaseUrl.endsWith("/api/v3") || lowerBaseUrl.endsWith("/api/plan/v3") ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`;
    return `${apiBaseUrl}${path}`;
}

function normalizeArkPlanBaseUrl(baseUrl: string) {
    try {
        const url = new URL(baseUrl);
        const path = url.pathname.replace(/\/+$/, "");
        const lowerPath = path.toLowerCase();
        const arkPlanIndex = lowerPath.indexOf("/api/plan/v3");
        if (arkPlanIndex < 0) return baseUrl;
        const end = arkPlanIndex + "/api/plan/v3".length;
        if (lowerPath.length !== end && lowerPath[end] !== "/") return baseUrl;
        url.pathname = path.slice(0, end);
        url.search = "";
        url.hash = "";
        return url.toString().replace(/\/+$/, "");
    } catch {
        return baseUrl;
    }
}

export function normalizeLocalChannels(config: Partial<AiConfig>, allowFallback = true): LocalModelChannel[] {
    const channels = Array.isArray(config.localChannels) ? config.localChannels : [];
    const normalized: LocalModelChannel[] = channels.map((channel, index) => {
        const rawModels = Array.isArray(channel.models) ? channel.models.filter(Boolean) : [];
        // Bug（2026-08-26 旧名残留）：migrateModelName 只迁移了顶层 model/imageModel 等字段，
        // 没迁移 localChannels[].models。用户「自定义渠道」里残留 official-* 旧名时，会和云端已改名的
        // agnes-* 作为「同模型两条」一起出现在下拉（dedupByModel 按原始 id 去重拦不住）。
        // 这里在去重前先把每个 model 走一遍 migrateModelName，从根上统一成新名再参与展示/去重。
        const seen = new Set<string>();
        const dedupedModels: string[] = [];
        for (const raw of rawModels) {
            const m = migrateModelName(raw);
            if (!seen.has(m)) {
                seen.add(m);
                dedupedModels.push(m);
            }
        }
        return {
            id: channel.id || `local-${index + 1}`,
            protocol: channel.protocol === "kie" || channel.protocol === "mimo" ? channel.protocol : "openai",
            name: typeof channel.name === "string" ? channel.name : `本地渠道 ${index + 1}`,
            baseUrl: channel.baseUrl || "",
            apiKey: channel.apiKey || "",
            models: dedupedModels,
        };
    });
    if (!normalized.length) {
        if (!allowFallback) return normalized;
        const rawFallbackModels = Array.isArray(config.models) ? config.models.filter(Boolean) : [];
        const seenF = new Set<string>();
        const fallbackModels: string[] = [];
        for (const raw of rawFallbackModels) {
            const m = migrateModelName(raw);
            if (!seenF.has(m)) { seenF.add(m); fallbackModels.push(m); }
        }
        normalized.push({ id: "local-default", protocol: "openai", name: "自定义渠道", baseUrl: config.baseUrl || defaultConfig.baseUrl, apiKey: config.apiKey || "", models: fallbackModels });
    }
    return normalized;
}

// 当前应当参与"模型展示与渠道归属解析"的渠道集合（全局单一事实来源）。
// 规范：
// - 供应商模式（activeVendorType !== "official"）：只返回供应商虚拟渠道，隐藏全部本地/自定义渠道；
//   选了 UpDream / NewWow / LibTV 等就只看到该供应商模型，不会混入"自定义渠道"。
// - 官方模式：云端渠道(publicChannels) + 用户真实添加的本地渠道；
//   本地直连模式(channelMode==="local") 保留兜底默认渠道以兼容旧本地配置，
//   云端模式下不兜底（用户未添加本地渠道即不显示"自定义渠道"分组）。
export function selectableModelChannels(config: AiConfig): Array<{ id: string; name: string; baseUrl?: string; models: string[]; modelLabels?: Record<string, string> }> {
    const isVendor = Boolean(config.activeVendorType) && config.activeVendorType !== "official";
    const publicChannels = (config.publicChannels || [])
        .filter((channel) => (channel.models || []).length > 0)
        .map((channel) => ({
            id: channel.id || "",
            name: channel.name || "云端渠道",
            baseUrl: channel.baseUrl,
            // 修复：对云端渠道的 models 也应用 migrateModelName，确保新旧名统一
            models: (channel.models || []).map((m) => migrateModelName(m)),
            modelLabels: channel.modelLabels
        }));
    if (isVendor) return publicChannels;
    const allowFallback = config.channelMode === "local";
    const localChannels = normalizeLocalChannels(config, allowFallback)
        .filter((channel) => channel.models.length > 0)
        .map((channel) => ({ id: channel.id, name: channel.name || "自定义渠道", baseUrl: channel.baseUrl, models: channel.models, modelLabels: channel.modelLabels }));
    return [...publicChannels, ...localChannels];
}

export function channelIdForActiveModel(config: AiConfig) {
    if (modelMatchesCapability(config.model, "image") && config.imageChannelId) return config.imageChannelId;
    if (modelMatchesCapability(config.model, "video") && config.videoChannelId) return config.videoChannelId;
    if (modelMatchesCapability(config.model, "audio") && config.audioChannelId) return config.audioChannelId;
    if (modelMatchesCapability(config.model, "text") && config.textChannelId) return config.textChannelId;
    if (config.activeChannelId) return config.activeChannelId;
    if (config.model === config.videoModel) return config.videoChannelId;
    if (config.model === config.textModel) return config.textChannelId;
    if (config.model === config.audioModel) return config.audioChannelId;
    return config.imageChannelId;
}

export function localChannelForActiveModel(config: AiConfig) {
    const channels = normalizeLocalChannels(config);
    const preferredId = channelIdForActiveModel(config);
    return channels.find((channel) => channel.id === preferredId && channel.models.includes(config.model)) || channels.find((channel) => channel.models.includes(config.model)) || channels.find((channel) => channel.id === preferredId) || channels[0];
}

// 当前选中模型所属渠道类型：云端(publicChannels) 还是 本地(localChannels)。
// 取代 config.channelMode === "remote" 的硬编码判断，支持「云端模型 + 本地直连模型并存、选哪个用哪个」。
export function activeChannelKind(config: AiConfig): "remote" | "local" {
    const model = config.model;
    const inRemote = (config.publicChannels || []).some((c) => (c.models || []).includes(model));
    if (inRemote) return "remote";
    if (localChannelForActiveModel(config)) return "local";
    // 兜底：当前模型不在任何渠道列表时，沿用原 channelMode 决定（保持旧行为）
    return config.channelMode === "remote" ? "remote" : "local";
}

export type DirectAIProvider = "kie" | "apimart";

const directAIProviderCache = new Map<string, DirectAIProvider | null>();

export function directAIProviderForConfig(config: AiConfig): DirectAIProvider | null {
    const channel = localChannelForActiveModel(config);
    if (!channel) return null;
    const protocol = channel.protocol.toLowerCase();
    const baseUrl = channel.baseUrl.trim().toLowerCase();
    const model = (config.model || "").trim().toLowerCase();
    const key = `${protocol}\n${baseUrl}\n${model}`;
    if (directAIProviderCache.has(key)) return directAIProviderCache.get(key) || null;
    const provider = protocol === "kie" || baseUrl.includes("kie.ai") || model.includes("kie/")
        ? "kie"
        : baseUrl.includes("apimart.ai") || model.includes("apimart")
            ? "apimart"
            : null;
    directAIProviderCache.set(key, provider);
    return provider;
}
