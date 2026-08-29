"use client";

import { Alert, App, Avatar, Badge, Button, Card, Divider, Form, Image, Input, Modal, Popconfirm, Segmented, Select, Space, Steps, Switch } from "antd";
import { useEffect, useState } from "react";

import { ModelPicker } from "@/components/model-picker";
import { fetchImageModels } from "@/services/api/image";
import { fetchUserConfig, measureUserStorageProvider, syncUserModelConfig, syncUserStorageProvider } from "@/services/api/user-config";
import { clearStorageConfigCache as clearFileStorageCache } from "@/services/file-storage";
import { clearStorageConfigCache as clearImageStorageCache, defaultUserStorageProvider, defaultUserWebDAVStorageProvider, loadStorageConfig, loadUserS3StorageProvider, loadUserWebDAVStorageProvider, saveUserStorageProvider, saveUserWebDAVStorageProvider, type UserStorageProvider } from "@/services/image-storage";
import { audioFormatOptions, audioVoiceOptions, normalizeAudioSpeedValue } from "@/lib/audio-generation";
import { isMimoPresetTtsModel, isMimoTtsModel, isMimoVoiceCloneModel, isMimoVoiceDesignModel, mimoTtsFormatOptions, mimoTtsVoiceOptions } from "@/lib/mimo-tts";
import { filterModelsByCapability, normalizeLocalChannels, useConfigStore, useEffectiveConfig, type AiConfig, type LocalModelChannel, type ModelCapability } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
// P0 新增：供应商 store + 类型
import { useVendorStore, type VendorType as AppConfigVendorType } from "@/stores/use-vendor-store";
import { bindVendorByCookie, type BindVendorByCookieRequest, type BoundAccount, type PublicVendorInfo, type VendorType } from "@/services/api/vendor";

type ModelGroup = {
    capability: ModelCapability;
    modelKey: "imageModel" | "videoModel" | "textModel";
    channelKey: "imageChannelId" | "videoChannelId" | "textChannelId";
    modelsKey: "imageModels" | "videoModels" | "textModels";
    defaultLabel: string;
    optionsLabel: string;
};

const modelGroups: ModelGroup[] = [
    { capability: "image", modelKey: "imageModel", channelKey: "imageChannelId", modelsKey: "imageModels", defaultLabel: "默认生图模型", optionsLabel: "生图模型可选项" },
    { capability: "video", modelKey: "videoModel", channelKey: "videoChannelId", modelsKey: "videoModels", defaultLabel: "默认视频模型", optionsLabel: "视频模型可选项" },
    { capability: "text", modelKey: "textModel", channelKey: "textChannelId", modelsKey: "textModels", defaultLabel: "默认文本模型", optionsLabel: "文本模型可选项" },
];

export function AppConfigModal() {
    const { message } = App.useApp();
    const [loadingModels, setLoadingModels] = useState(false);
    const [savingConfig, setSavingConfig] = useState(false);
    const [remoteStorageSyncEnabled, setRemoteStorageSyncEnabled] = useState(false);
    const [remoteWebDAVStorageSyncEnabled, setRemoteWebDAVStorageSyncEnabled] = useState(false);
    const [allowUserStorageProvider, setAllowUserStorageProvider] = useState(false);
    const [userStorage, setUserStorage] = useState(() => defaultUserStorageProvider());
    const [userWebDAVStorage, setUserWebDAVStorage] = useState(() => defaultUserWebDAVStorageProvider());
    const [measuringStorageType, setMeasuringStorageType] = useState<"s3" | "webdav" | null>(null);
    const [storageUsageText, setStorageUsageText] = useState("");
    const [webDAVStorageUsageText, setWebDAVStorageUsageText] = useState("");
    // ========== P1 新增：绑定账户弹窗 state ==========
    const [bindModalOpen, setBindModalOpen] = useState(false);
    const [bindTarget, setBindTarget] = useState<VendorType>("updream"); // 绑定弹窗当前在绑哪家
    const [bindForm, setBindForm] = useState<BindVendorByCookieRequest>({ vendorType: "updream" });
    const [bindSubmitting, setBindSubmitting] = useState(false);
    const config = useConfigStore((state) => state.config);
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const isConfigOpen = useConfigStore((state) => state.isConfigOpen);
    const shouldPromptContinue = useConfigStore((state) => state.shouldPromptContinue);
    const setConfigDialogOpen = useConfigStore((state) => state.setConfigDialogOpen);
    const clearPromptContinue = useConfigStore((state) => state.clearPromptContinue);
    const publicSettings = useConfigStore((state) => state.publicSettings);
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const effectiveConfig = useEffectiveConfig();
    const modelChannel = publicSettings?.modelChannel;
    const isLoggedIn = Boolean(token && user);
    const isAdmin = user?.role === "admin";
    // ========== P0 新增：供应商相关 state ==========
    const vendors = useVendorStore((s) => s.vendors);
    const accounts = useVendorStore((s) => s.accounts);
    const vendorLoading = useVendorStore((s) => s.isLoading);
    const loadVendors = useVendorStore((s) => s.loadVendors);
    const loadAccounts = useVendorStore((s) => s.loadAccounts);
    const activateVendor = useVendorStore((s) => s.activateVendor);
    const unbindVendor = useVendorStore((s) => s.unbindVendor);
    const refreshBalance = useVendorStore((s) => s.refreshBalance);
    const refreshVendorModels = useVendorStore((s) => s.refreshVendorModels);
    // 商用版本：普通用户 + 游客（只要后台已配置云端渠道）默认走 remote
    const canUseRemoteChannel = Boolean(modelChannel && modelChannel.channels?.length) || isLoggedIn;
    // ✅ 关键：允许自定义本地渠道 = 后台开关开启 AND 已登录（游客不允许，避免 apiKey 明文直连第三方）
    const allowCustomChannel = isLoggedIn && modelChannel?.allowCustomChannel === true;
    // 允许自定义时用户可自由切换 local / remote；否则只要存在后台云端渠道就强制 remote
    const effectiveMode: AiConfig["channelMode"] = (() => {
        if (allowCustomChannel) return config.channelMode;
        return canUseRemoteChannel ? "remote" : "local";
    })();
    const localModelConfig: AiConfig = effectiveMode === "local" && config.channelMode !== "local" ? { ...config, channelMode: "local" } : config;
    const modelConfig = effectiveMode === "remote" ? effectiveConfig : localModelConfig;
    const canUseUserStorageProvider = isLoggedIn && allowUserStorageProvider;

    useEffect(() => {
        setUserStorage(loadUserS3StorageProvider() || defaultUserStorageProvider());
        setUserWebDAVStorage(loadUserWebDAVStorageProvider() || defaultUserWebDAVStorageProvider());
        if (!isConfigOpen || !token) return;
        let canceled = false;
        void fetchUserConfig(token)
            .then((payload) => {
                if (canceled) return;
                const remoteConfig = payload.modelConfig;
                const syncS3 = remoteConfig?.syncStorageConfig === true;
                const syncWebDAV = remoteConfig?.syncWebDAVStorageConfig === true;
                setRemoteStorageSyncEnabled(syncS3);
                setRemoteWebDAVStorageSyncEnabled(syncWebDAV);
                if (remoteConfig) {
                    Object.entries(remoteConfig)
                        .forEach(([key, value]) => updateConfig(key as keyof AiConfig, value as never));
                }
                updateConfig("syncStorageConfig", syncS3);
                updateConfig("syncWebDAVStorageConfig", syncWebDAV);
                if (syncS3 && payload.storageProvider?.s3) {
                    const next = { ...defaultUserStorageProvider(), ...payload.storageProvider.s3, type: "s3" as const };
                    setUserStorage(next);
                    saveUserStorageProvider(next);
                }
                if (syncWebDAV && payload.storageProvider?.webdav) {
                    const next = { ...defaultUserWebDAVStorageProvider(), ...payload.storageProvider.webdav, type: "webdav" as const };
                    setUserWebDAVStorage(next);
                    saveUserWebDAVStorageProvider(next);
                }
            })
            .catch(() => { });
        return () => {
            canceled = true;
        };
    }, [isConfigOpen, token, updateConfig]);

    useEffect(() => {
        if (!isConfigOpen) return;
        let canceled = false;
        void loadStorageConfig()
            .then((storage) => {
                if (!canceled) setAllowUserStorageProvider(storage.allowUserProvider === true);
            })
            .catch(() => {
                if (!canceled) setAllowUserStorageProvider(false);
            });
        return () => {
            canceled = true;
        };
    }, [isConfigOpen]);

    // ========== 辅助：打开绑定弹窗（针对某供应商） ==========
    function openBindModal(vendorType: VendorType) {
        setBindTarget(vendorType);
        // 预填 authHeaderName：从 vendors 列表取，取不到用前端硬编码兜底
        const v = vendors.find((x) => x.type === vendorType);
        const headerName = v?.authHeaderName
            || (vendorType === "updream" ? "Authorization"
                : vendorType === "newwow" ? "accesstoken"
                : vendorType === "libtv" ? "Token"
                : "");
        setBindForm({ vendorType, authHeaderName: headerName });
        setBindModalOpen(true);
    }
    // ========== 辅助：提交 bind-cookie（AccessToken / Cookie / 鉴权 Header 模式）==========
    async function submitBind() {
        if (!bindForm.vendorType) {
            message.warning("请选择供应商");
            return;
        }
        const hasCookie = (bindForm.cookieString || "").trim().length > 0;
        const hasHeader = (bindForm.authHeaderValue || "").trim().length > 0;
        if (!hasCookie && !hasHeader) {
            message.warning("请粘贴 Authorization Bearer token");
            return;
        }
        setBindSubmitting(true);
        try {
            const res = await bindVendorByCookie({ ...bindForm, vendorType: bindTarget });
            message.success(`已成功绑定 ${vendorDisplayName(res.vendorType)}：${res.account.displayName}`);
            setBindModalOpen(false);
            // 绑定成功后：后端已经把该账户设为 active，前端同步 state + 刷 accounts
            updateConfig("activeVendorType", res.vendorType);
            await loadAccounts(true);
            await activateVendor(res.vendorType);
        } catch (e: any) {
            message.error(e?.message || "绑定失败");
        } finally {
            setBindSubmitting(false);
        }
    }
    function vendorDisplayName(t: VendorType | AppConfigVendorType) {
        switch (t) {
            case "official": return "官方云端";
            case "updream":  return "UpDream 云端";
            case "libtv":    return "LibTV 云端";
            case "newwow":   return "NewWow 云端";
            default:         return t;
        }
    }
    useEffect(() => {
        if (!isConfigOpen) return;
        void loadVendors(false);
        void loadAccounts(false);
    }, [isConfigOpen, loadVendors, loadAccounts]);

    const finishConfig = async () => {
        const localIncomplete = effectiveMode === "local" && normalizeLocalChannels(config).some((channel) => !channel.baseUrl.trim() || !channel.apiKey.trim());
        const modelIncomplete = !modelConfig.imageModel.trim() || !modelConfig.videoModel.trim() || !modelConfig.textModel.trim();
        if (userStorage.enabled && userWebDAVStorage.enabled) {
            message.error("S3/R2 与 WebDAV 不能同时启用");
            return;
        }
        if (!canUseRemoteChannel && config.channelMode !== "local") updateConfig("channelMode", "local");
        else if (canUseRemoteChannel && !allowCustomChannel && config.channelMode !== "remote") updateConfig("channelMode", "remote");
        if (canUseUserStorageProvider) {
            saveUserStorageProvider(userStorage);
            saveUserWebDAVStorageProvider(userWebDAVStorage);
        }
        setSavingConfig(true);
        try {
            if (token) {
                const configToSave = effectiveMode === "local" && config.channelMode !== "local" ? { ...config, channelMode: "local" as const } : config;
                await syncUserModelConfig(token, configToSave);
            }
            const providers = {
                ...(config.syncStorageConfig || remoteStorageSyncEnabled ? { s3: config.syncStorageConfig ? userStorage : { ...userStorage, enabled: false, endpoint: "", bucket: "", accessKeyId: "", secretAccessKey: "" } } : {}),
                ...(config.syncWebDAVStorageConfig || remoteWebDAVStorageSyncEnabled ? { webdav: config.syncWebDAVStorageConfig ? userWebDAVStorage : { ...userWebDAVStorage, enabled: false, endpoint: "", username: "", password: "" } } : {}),
            };
            if (token && canUseUserStorageProvider && Object.keys(providers).length) {
                await syncUserStorageProvider(token, providers);
                setRemoteStorageSyncEnabled(config.syncStorageConfig);
                setRemoteWebDAVStorageSyncEnabled(config.syncWebDAVStorageConfig);
            }
            clearImageStorageCache();
            clearFileStorageCache();
            setConfigDialogOpen(false);
            if ((config.syncStorageConfig || config.syncWebDAVStorageConfig) && !token) message.warning("请登录后再同步配置");
            else if (localIncomplete || modelIncomplete) message.warning("部分模型或本地渠道密钥尚未配置完整，配置已保存");
            else message.success(shouldPromptContinue ? "配置已保存，请继续刚才的请求" : "配置已保存");
            clearPromptContinue();
        } catch (error) {
            message.error(error instanceof Error ? "同步配置失败：" + error.message : "同步配置失败");
        } finally {
            setSavingConfig(false);
        }
    };

    const refreshModels = async () => {
        const channels = normalizeLocalChannels(config);
        if (!channels.length) {
            message.warning("请先添加本地模型渠道");
            return;
        }
        if (channels.some((channel) => !channel.baseUrl.trim() || !channel.apiKey.trim())) {
            message.error("请先填写所有本地渠道的 Base URL 和 API Key");
            return;
        }
        setLoadingModels(true);
        try {
            const nextChannels = await Promise.all(channels.map(async (channel) => ({ ...channel, models: await fetchImageModels(configForLocalChannel(config, channel)) })));
            updateLocalChannels(nextChannels);
            message.success("模型列表已更新");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取模型失败");
        } finally {
            setLoadingModels(false);
        }
    };

    const updateLocalChannels = (channels: LocalModelChannel[]) => {
        const normalized = channels.length ? channels : normalizeLocalChannels({ baseUrl: config.baseUrl, apiKey: config.apiKey, models: config.models });
        const models = uniqueModels(normalized.flatMap((channel) => channel.models));
        const nextImageModels = filterModelsByCapability(models, "image");
        const nextVideoModels = filterModelsByCapability(models, "video");
        const nextTextModels = filterModelsByCapability(models, "text");
        const imageModel = nextImageModels.includes(config.imageModel) ? config.imageModel : nextImageModels[0] || "";
        const videoModel = nextVideoModels.includes(config.videoModel) ? config.videoModel : nextVideoModels[0] || "";
        const textModel = nextTextModels.includes(config.textModel) ? config.textModel : nextTextModels[0] || "";
        updateConfig("localChannels", normalized);
        updateConfig("models", models);
        updateConfig("imageModels", nextImageModels);
        updateConfig("videoModels", nextVideoModels);
        updateConfig("textModels", nextTextModels);
        updateConfig("imageModel", imageModel);
        updateConfig("videoModel", videoModel);
        updateConfig("textModel", textModel);
        updateConfig("imageChannelId", channelIdForLocalModel(normalized, imageModel, config.imageChannelId));
        updateConfig("videoChannelId", channelIdForLocalModel(normalized, videoModel, config.videoChannelId));
        updateConfig("textChannelId", channelIdForLocalModel(normalized, textModel, config.textChannelId));
        updateConfig("baseUrl", normalized[0]?.baseUrl || config.baseUrl);
        updateConfig("apiKey", normalized[0]?.apiKey || config.apiKey);
    };

    const patchLocalChannel = (id: string, patch: Partial<LocalModelChannel>) => {
        updateLocalChannels(normalizeLocalChannels(config).map((channel) => (channel.id === id ? { ...channel, ...patch } : channel)));
    };

    const addLocalChannel = () => {
        updateLocalChannels([...normalizeLocalChannels(config), { id: "local-" + Date.now(), protocol: "openai", name: "新渠道", baseUrl: "", apiKey: "", models: [] }]);
    };

    const removeLocalChannel = (id: string) => {
        updateLocalChannels(normalizeLocalChannels(config).filter((channel) => channel.id !== id));
    };

    const refreshLocalChannelModels = async (channel: LocalModelChannel) => {
        if (!channel.baseUrl.trim() || !channel.apiKey.trim()) {
            message.error("请先填写该渠道的 Base URL 和 API Key");
            return;
        }
        setLoadingModels(true);
        try {
            patchLocalChannel(channel.id, { models: await fetchImageModels(configForLocalChannel(config, channel)) });
            message.success("模型列表已更新");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取模型失败");
        } finally {
            setLoadingModels(false);
        }
    };


    const measureStorage = async (provider: UserStorageProvider) => {
        if (!token) {
            message.warning("请先登录后再统计容量");
            return;
        }
        setMeasuringStorageType(provider.type);
        try {
            const result = await measureUserStorageProvider(token, provider);
            const usageText = formatBytes(result.bytes) + " / " + formatBytes(result.limitBytes) + (result.overLimit ? "，已达到上限" : "");
            if (provider.type === "webdav") {
                setWebDAVStorageUsageText(usageText);
                if (result.overLimit) {
                    const next = { ...userWebDAVStorage, enabled: false };
                    setUserWebDAVStorage(next);
                    saveUserWebDAVStorageProvider(next);
                }
            } else {
                setStorageUsageText(usageText);
                if (result.overLimit) {
                    const next = { ...userStorage, enabled: false };
                    setUserStorage(next);
                    saveUserStorageProvider(next);
                }
            }
            message.success("容量统计完成");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "容量统计失败");
        } finally {
            setMeasuringStorageType(null);
        }
    };

    return (
        <Modal
            title={
                <div>
                    <div className="text-lg font-semibold">配置与用户偏好</div>
                    <div className="mt-1 text-xs font-normal text-stone-500">云端供应商 · 模型 · 渠道 · 画布默认行为</div>
                </div>
            }
            open={isConfigOpen}
            width={960}
            centered
            onCancel={() => setConfigDialogOpen(false)}
            styles={{ body: { maxHeight: "72vh", overflowY: "auto", paddingRight: 18 } }}
            footer={
                <Button type="primary" loading={savingConfig} onClick={() => void finishConfig()}>
                    完成
                </Button>
            }
        >
            <div className="pt-1">
                {/* ============ 绑定供应商子弹窗：粘贴 AccessToken / Cookie 绑定 ============ */}
                <Modal
                    open={bindModalOpen}
                    title={`绑定 ${vendorDisplayName(bindTarget)} 账户`}
                    onCancel={() => setBindModalOpen(false)}
                    width={1200}
                    okButtonProps={{ style: { display: "none" } }}
                    cancelButtonProps={{ style: { display: "none" } }}
                    footer={
                        <Space>
                            <Button onClick={() => setBindModalOpen(false)}>取消</Button>
                            <Button type="primary" loading={bindSubmitting} onClick={() => void submitBind()}>
                                确认绑定
                            </Button>
                        </Space>
                    }
                >
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                        <div className="space-y-4">
                            {(() => {
                                // UpDream：只需 Bearer token 鉴权
                                if (bindTarget === "updream") {
                                    return (
                                        <div className="rounded-xl border border-stone-200 dark:border-stone-700 bg-white dark:bg-stone-900/40 overflow-hidden">
                                            <div className="px-4 pt-3 pb-2 flex items-center gap-2 border-b border-stone-100 dark:border-stone-800">
                                                <span className="text-sm font-semibold text-stone-800 dark:text-stone-100">UpDream Bearer Token 鉴权</span>
                                                <span className="text-[11px] leading-none px-1.5 py-0.5 rounded bg-green-100 dark:bg-green-900/50 text-green-700 dark:text-green-300">仅需 Authorization</span>
                                            </div>
                                            <div className="p-4">
                                                <Steps
                                                    direction="vertical"
                                                    size="small"
                                                    current={-1}
                                                    items={[
                                                        { title: "登录官网", description: <span>打开 <a href="https://www.updream.cn/" target="_blank" rel="noreferrer" className="underline">www.updream.cn</a> 登录你的账号</span> },
                                                        { title: "打开 DevTools", description: "按 F12 → 切到 Network 选项卡 → 任意点一下页面让列表里有请求" },
                                                        { title: "找到 Request Headers", description: "点开任意 www.updream.cn 请求 → 右侧 Headers → 滚到 Request Headers" },
                                                        { title: "复制 Authorization", description: "复制 Authorization 字段 value（含 Bearer 前缀的 JWT），粘到右栏输入框" },
                                                    ]}
                                                />
                                                <div className="mt-3 rounded-lg border border-stone-200 dark:border-stone-700 overflow-hidden">
                                                    <Image
                                                        src="/bind-cookie-updream.png"
                                                        alt="UpDream 在 DevTools Request Headers 复制 Authorization 字段示例"
                                                        style={{ width: "100%", maxHeight: 260, objectFit: "contain", display: "block" }}
                                                        preview={{ mask: "点击查看完整截图" }}
                                                    />
                                                </div>
                                                <Alert
                                                    className="mt-3"
                                                    type="info"
                                                    showIcon
                                                    message="只需 Bearer token"
                                                    description={
                                                        <span className="text-xs">
                                                            从 DevTools → Network → Request Headers → 找到 <b>Authorization</b> 字段 → 复制完整 value（含 <code className="bg-stone-100 dark:bg-stone-800 px-1 rounded">Bearer </code> 前缀）粘贴到右栏即可。不需要 Cookie。
                                                        </span>
                                                    }
                                                />
                                            </div>
                                        </div>
                                    );
                                }
                                // NewWow：custom_header 鉴权，复制 Request Headers 的 accesstoken 字段
                                if (bindTarget === "newwow") {
                                    return (
                                        <div className="text-xs leading-relaxed bg-stone-50 dark:bg-stone-900/40 rounded p-3 border border-stone-200 dark:border-stone-700">
                                            <div className="font-medium text-stone-700 dark:text-stone-200 mb-2">NewWow 用 <code className="bg-stone-200 dark:bg-stone-800 px-1 rounded">accesstoken</code> HTTP Header 鉴权（不是 Cookie）</div>
                                            <ol className="list-decimal pl-5 space-y-1 text-stone-600 dark:text-stone-300">
                                                <li>打开 <a href="https://neowow.cn/" target="_blank" rel="noreferrer" className="underline">neowow.cn</a> 登录你的账号</li>
                                                <li>按 F12 打开 DevTools → 切到 <b>Network</b> 选项卡 → 任意点页面让列表里有请求</li>
                                                <li>点开任意一个 <code className="bg-stone-200 dark:bg-stone-800 px-1 rounded">neowow.cn</code> 的请求 → 右侧 <b>Headers</b> → 滚到 <b>Request Headers</b></li>
                                                <li>找到 <b>Accesstoken</b> 字段（不是 Cookie！），复制它整段 value（eyJ 开头的 JWT）粘到右栏输入框</li>
                                            </ol>
                                            <div className="mt-3 flex justify-center">
                                                <Image
                                                    src="/bind-header-newwow.png"
                                                    alt="NewWow 在 DevTools Request Headers 复制 Accesstoken 字段示例"
                                                    className="rounded border border-stone-200 dark:border-stone-700"
                                                    style={{ maxWidth: "100%" }}
                                                    preview={{ mask: "点击查看完整截图" }}
                                                />
                                            </div>
                                            <div className="mt-2 text-stone-400 text-center">↑ 在 DevTools Request Headers 找到 <b>Accesstoken</b> 字段，复制它整段 value 即可</div>
                                            <div className="mt-2 text-stone-400">accesstoken 是登录后由 SPA 缓存的会话 JWT，<b>不在 Cookie 里</b>，必须从 Request Headers 单独复制。整段复制、前后换行也无所谓，下方会做清洗。</div>
                                        </div>
                                    );
                                }
                                // LibTV：custom_header 鉴权，复制 Request Headers 的 Token 字段
                                if (bindTarget === "libtv") {
                                    return (
                                        <div className="text-xs leading-relaxed bg-stone-50 dark:bg-stone-900/40 rounded p-3 border border-stone-200 dark:border-stone-700">
                                            <div className="font-medium text-stone-700 dark:text-stone-200 mb-2">LibTV 用 <code className="bg-stone-200 dark:bg-stone-800 px-1 rounded">Token</code> HTTP Header 鉴权（不是 Cookie）</div>
                                            <ol className="list-decimal pl-5 space-y-1 text-stone-600 dark:text-stone-300">
                                                <li>打开 <a href="https://www.liblib.tv/" target="_blank" rel="noreferrer" className="underline">www.liblib.tv</a> 登录你的账号</li>
                                                <li>按 F12 打开 DevTools → 切到 <b>Network</b> 选项卡 → 任意点页面让列表里有请求</li>
                                                <li>点开任意一个 <code className="bg-stone-200 dark:bg-stone-800 px-1 rounded">api2.liblib.art</code> 的请求 → 右侧 <b>Headers</b> → 滚到 <b>Request Headers</b></li>
                                                <li>找到 <b>Token</b> 字段，复制它整段 value（48 位十六进制字符串）粘到右栏输入框</li>
                                            </ol>
                                            <div className="mt-3 flex justify-center">
                                                <Image
                                                    src="/bind-token-libtv.png"
                                                    alt="LibTV 在 DevTools Request Headers 复制 Token 字段示例"
                                                    className="rounded border border-stone-200 dark:border-stone-700"
                                                    style={{ maxWidth: "100%" }}
                                                    preview={{ mask: "点击查看完整截图" }}
                                                />
                                            </div>
                                            <div className="mt-2 text-stone-400 text-center">↑ 在 DevTools Request Headers 找到 <b>Token</b> 字段，复制它整段 value 即可</div>
                                            <div className="mt-2 text-stone-400">Token 是登录后 liblib.tv 前端缓存的会话 token（不是 Cookie），<b>必须从 Request Headers 单独复制</b>，整段保留、不要漏字符。</div>
                                        </div>
                                    );
                                }
                                return (
                                    <div className="text-xs text-stone-500 leading-relaxed">
                                        用浏览器插件采集 Cookie 后粘贴到右栏输入框即可。
                                        插件操作：打开插件 → 点「去登录」打开官网 → 登录后点「立即采集」→ 点「复制 Cookie」→ 回来粘贴。
                                    </div>
                                );
                            })()}
                        </div>
                        <div className="space-y-4">
                            {(() => {
                                // 当前绑定目标的 vendor 配置（用来决定鉴权模式提示）
                                const target = vendors.find((v) => v.type === bindTarget);
                                // 前端兜底：vendors 列表未加载或 localStorage 缓存过时时，
                                // 按 vendorType 硬编码默认 authMode，避免 UpDream 误走 cookie 输入框
                                const fallbackAuthMode = bindTarget === "updream" || bindTarget === "newwow" || bindTarget === "libtv"
                                    ? "custom_header"
                                    : "cookie";
                                const fallbackHeaderName = bindTarget === "updream" ? "Authorization"
                                    : bindTarget === "newwow" ? "accesstoken"
                                    : bindTarget === "libtv" ? "Token"
                                    : "";
                                const authMode = target?.authMode || fallbackAuthMode;
                                const headerName = target?.authHeaderName || fallbackHeaderName;
                                if (authMode === "custom_header") {
                                    // custom_header 鉴权：让用户粘 HTTP header value（UpDream 的 Authorization Bearer JWT / NewWow 的 accesstoken / LibTV 的 Token）
                                    const isBearer = headerName === "Authorization";
                                    return (
                                        <Form.Item
                                            label={`${vendorDisplayName(bindTarget)} ${headerName || "Header"} 值`}
                                            extra={isBearer
                                                ? `从 DevTools → Network → 任意 XHR 请求 → Request Headers → 找到 "Authorization" 字段 → 复制完整 value（含 Bearer 前缀）粘贴到下方`
                                                : `从 DevTools → Network → 任意 XHR 请求 → Request Headers → 找到 "${headerName || "accesstoken"}" 字段 → 复制它的 value（整段 JWT 字符串）粘贴到下方`}
                                        >
                                            <Input.TextArea
                                                rows={3}
                                                placeholder={isBearer
                                                    ? "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
                                                    : `粘贴 ${headerName || "accesstoken"} 字段的 value（eyJ 开头的 JWT 字符串）`}
                                                value={bindForm.authHeaderValue || ""}
                                                onChange={(e) => setBindForm((s) => ({ ...s, authHeaderValue: e.target.value, authHeaderName: headerName }))}
                                            />
                                        </Form.Item>
                                    );
                                }
                                // 默认 cookie 模式：右栏输入框
                                return (
                                    <>
                                        <Form.Item label={`${vendorDisplayName(bindTarget)} Cookie 字符串`} extra="格式：key1=val1; key2=val2; …（从浏览器 Request Headers 的 cookie 字段复制整段值）">
                                            <Input.TextArea
                                                rows={4}
                                                placeholder="粘贴从浏览器 Request Headers 的 cookie 字段复制的整段值"
                                                value={bindForm.cookieString || ""}
                                                onChange={(e) => setBindForm((s) => ({ ...s, cookieString: e.target.value }))}
                                            />
                                        </Form.Item>
                                    </>
                                );
                            })()}
                        </div>
                    </div>
                </Modal>
                <Form layout="vertical" requiredMark={false}>
                    {/* ============================================================ */}
                    {/* ===== P0 新增：云端供应商切换（放在最顶部，用户最容易看到）===== */}
                    {/* ============================================================ */}
                    <Divider titlePlacement="left" plain>
                        <span className="text-sm font-medium text-stone-700 dark:text-stone-300">云端供应商</span>
                    </Divider>
                    <Form.Item
                        label="选择云端平台"
                        className="mb-4"
                        extra={
                            <div className="text-xs text-stone-500 leading-relaxed">
                                · <b>官方云端</b>：沿用系统后台管理员配置的模型渠道，支持自定义本地模型渠道（allowCustomChannel 开启时）。
                                <br />
                                · <b>其他平台</b>：切换后模型、接口、素材库全部走对应平台官方；未绑定会引导授权登录（P1 开放）。
                            </div>
                        }
                    >
                        <Segmented
                            block
                            size="large"
                            value={config.activeVendorType}
                            onChange={(value) => {
                                const t = value as AppConfigVendorType;
                                // 选中即切换（选择哪个供应商就用哪个）；绑定是独立动作，可在选中后再绑定。
                                // 未绑定的非官方供应商：只切 UI 状态 + 由 onClick 打开绑定弹窗，不调后端激活
                                // （避免未绑定账户调后端 activate 失败弹出"切换失败"误导提示），也不弹 toast。
                                if (t !== "official" && !accounts.some((a) => a.vendorType === t)) {
                                    const { updateConfig } = useConfigStore.getState();
                                    updateConfig("activeVendorType", t);
                                    return;
                                }
                                void activateVendor(t);
                            }}
                            options={
                                (vendors.length ? vendors : [
                                    // 兜底：后端没返回时至少显示 4 家内置，不影响 UI 切换
                                    { type: "official", name: "官方云端（管理员配置）", logoUrl: "", enabled: true, sort: 0, hasOAuth: false },
                                    { type: "updream", name: "UpDream 云端", logoUrl: "", enabled: true, sort: 10, hasOAuth: true },
                                    { type: "libtv", name: "LibTV 云端", logoUrl: "", enabled: true, sort: 20, hasOAuth: true },
                                    { type: "newwow", name: "NewWow 云端", logoUrl: "", enabled: true, sort: 30, hasOAuth: true },
                                ] as PublicVendorInfo[])
                                    .filter((v) => v.enabled)
                                    .map((v) => {
                                        // 如果选了非官方且没绑定，label 上显示"未绑定"标识；切的时候弹 OAuth 引导
                                        const bound = accounts.find((a) => a.vendorType === v.type);
                                        const currentActive = config.activeVendorType === v.type || bound?.isActive;
                                        // 下拉选项 label：去掉 vendor logo / emoji，只保留名称 + 状态 Badge
                                        const label = (
                                            <div className="flex items-center gap-2 px-1 py-0.5 min-w-[140px] justify-center">
                                                <span className="text-sm">{v.name}</span>
                                                {currentActive ? (
                                                    <Badge color="green" status="processing" text={<span className="text-xs">当前</span>} />
                                                ) : bound ? (
                                                    <Badge color="blue" text={<span className="text-[10px] px-1">已绑定</span>} />
                                                ) : v.hasOAuth ? (
                                                    <Badge color="default" text={<span className="text-[10px] px-1">未绑定</span>} />
                                                ) : null}
                                            </div>
                                        );
                                        return {
                                            label,
                                            value: v.type,
                                            disabled: !v.enabled,
                                            // 非 official 且未绑定时：点击 Segmented 打开绑定弹窗
                                            onClick: (_e: any) => {
                                                if (v.type === "official") return; // official 正常走 onChange activate
                                                if (bound) return; // 已绑定正常走 activate
                                                // 未绑定 → 打开绑定弹窗
                                                openBindModal(v.type as VendorType);
                                            },
                                        };
                                    })
                            }
                        />
                    </Form.Item>

                    {/* 如果当前激活的是非官方供应商 & 有绑定账户 → 显示账户卡片 */}
                    {(() => {
                        const activeNonOfficial =
                            config.activeVendorType !== "official" &&
                            accounts.find((a) => a.vendorType === config.activeVendorType);
                        if (!activeNonOfficial) return null;
                        return (
                            <Card size="small" className="mb-5">
                                <div className="flex items-center justify-between gap-3 flex-wrap">
                                    <div className="flex items-center gap-3 min-w-0">
                                        <Avatar
                                            src={activeNonOfficial.avatarUrl}
                                            size={40}
                                        >
                                            {activeNonOfficial.displayName?.slice(0, 1) || "?"}
                                        </Avatar>
                                        <div className="min-w-0">
                                            <div className="font-medium truncate">{activeNonOfficial.displayName}</div>
                                            <div className="text-xs text-stone-500 truncate">
                                                绑定于 {new Date(activeNonOfficial.boundAt).toLocaleString()}
                                                {activeNonOfficial.balanceText ? (
                                                    <span className="ml-2 text-stone-700 dark:text-stone-300">· {activeNonOfficial.balanceText}</span>
                                                ) : null}
                                            </div>
                                        </div>
                                    </div>
                                    <Space wrap>
                                        <Button
                                            size="small"
                                            onClick={() => void refreshVendorModels(activeNonOfficial.vendorType)}
                                            disabled={vendorLoading}
                                        >
                                            刷新模型
                                        </Button>
                                        <Button
                                            size="small"
                                            onClick={() => void refreshBalance(activeNonOfficial.vendorType)}
                                            disabled={vendorLoading}
                                        >
                                            刷新余额
                                        </Button>
                                        <Popconfirm
                                            title="确定解绑该供应商账户？"
                                            description="解绑后需要重新授权才能继续使用其云端能力。"
                                            onConfirm={() => void unbindVendor(activeNonOfficial.vendorType)}
                                            okButtonProps={{ danger: true }}
                                        >
                                            <Button size="small" danger>
                                                解绑账户
                                            </Button>
                                        </Popconfirm>
                                    </Space>
                                </div>
                                {activeNonOfficial.hasModels ? (
                                    <div className="mt-2 text-xs text-emerald-600 dark:text-emerald-500">
                                        ✓ 已同步该账户可用模型快照
                                    </div>
                                ) : (
                                    <div className="mt-2 text-xs text-amber-600 dark:text-amber-500">
                                        尚未拉取该账户可用模型，点击「刷新」初始化
                                    </div>
                                )}
                            </Card>
                        );
                    })()}

                    {/* 切非官方供应商但尚未绑定 → 给个引导提示 */}
                    {config.activeVendorType !== "official" &&
                        !accounts.find((a) => a.vendorType === config.activeVendorType) && (
                            <div className="mb-5 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800/40 dark:bg-amber-900/10 dark:text-amber-300">
                                <div className="font-medium">该平台尚未绑定</div>
                                <div className="mt-1 leading-relaxed">
                                    请完成授权登录后再使用其云端模型；未绑定前切换仅标记状态，生图/视频等请求仍无法真正下发。
                                </div>
                            </div>
                        )}

                    {/* ============================================================ */}
                    {/* 原有「渠道模式」等 UI 仅在 activeVendorType=official 时显示 */}
                    {/* ============================================================ */}
                    {config.activeVendorType === "official" ? (
                        <>
                            {/* 云端渠道卡片：系统后台配置的模型，登录后走后端代理，始终可用 */}
                            <div className="mb-5 rounded-lg border border-stone-200 p-3 text-sm text-stone-500 dark:border-stone-800">
                                <div className="font-medium text-stone-900 dark:text-stone-100">云端渠道</div>
                                {modelChannel && modelChannel.availableModels?.length ? (
                                    <div className="mt-1">由系统后台渠道转发请求，当前可用 {modelChannel.availableModels.length} 个模型。</div>
                                ) : isLoggedIn ? (
                                    <div className="mt-1 text-amber-600 dark:text-amber-500">管理员尚未配置可用模型，请稍后再试或联系客服。</div>
                                ) : (
                                    <div className="mt-1 text-amber-600 dark:text-amber-500">请先登录以使用系统配置的云端渠道模型。</div>
                                )}
                            </div>
                            {allowCustomChannel ? (
                                <>
                                    <div className="mb-5 space-y-3 rounded-lg border border-stone-200 p-3 dark:border-stone-800">
                                        <div className="flex items-center justify-between gap-3">
                                            <div>
                                                <div className="text-sm font-medium">本地模型渠道</div>
                                                <div className="mt-1 text-xs text-stone-500">可为生图、视频、文本、音频分别选择不同渠道的模型。</div>
                                            </div>
                                            <Button size="small" onClick={addLocalChannel}>
                                                新增渠道
                                            </Button>
                                        </div>
                                        {normalizeLocalChannels(config).map((channel, index) => (
                                            <div key={channel.id} className="space-y-2 rounded-md bg-stone-50 p-2 dark:bg-stone-900">
                                                <div className="grid gap-2 md:grid-cols-[130px_150px_minmax(0,1fr)_minmax(0,1fr)_auto]">
                                                    <Input value={channel.name} placeholder="渠道名称" onChange={(event) => patchLocalChannel(channel.id, { name: event.target.value })} />
                                                    <Select
                                                        value={channel.protocol}
                                                        options={[
                                                            { label: "OpenAI", value: "openai" },
                                                            { label: "KIE", value: "kie" },
                                                            { label: "MiMo", value: "mimo" },
                                                        ]}
                                                        onChange={(protocol) => patchLocalChannel(channel.id, { protocol: protocol as LocalModelChannel["protocol"] })}
                                                    />
                                                    <Input value={channel.baseUrl} placeholder="Base URL" onChange={(event) => patchLocalChannel(channel.id, { baseUrl: event.target.value })} />
                                                    <Input.Password value={channel.apiKey} placeholder="API Key" onChange={(event) => patchLocalChannel(channel.id, { apiKey: event.target.value })} />
                                                    <div className="flex gap-2">
                                                        <Button size="small" loading={loadingModels} onClick={() => void refreshLocalChannelModels(channel)}>
                                                            拉取
                                                        </Button>
                                                        <Button size="small" danger disabled={index === 0 && normalizeLocalChannels(config).length === 1} onClick={() => removeLocalChannel(channel.id)}>
                                                            删除
                                                        </Button>
                                                    </div>
                                                </div>
                                                <div className="text-xs text-stone-500">已保存 {channel.models.length} 个模型</div>
                                            </div>
                                        ))}
                                    </div>
                                    <div className="mb-5 flex items-center justify-between gap-3 rounded-lg border border-stone-200 px-3 py-2 dark:border-stone-800">
                                        <div className="min-w-0">
                                            <div className="text-sm font-medium">模型列表</div>
                                            <div className="mt-1 text-xs text-stone-500">当前已保存 {config.models.length} 个模型</div>
                                        </div>
                                        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                                            <Button size="small" loading={loadingModels} onClick={() => void refreshModels()}>
                                                拉取全部渠道
                                            </Button>
                                        </div>
                                    </div>
                                </>
                            ) : null}
                        </>
                    ) : (
                        // 切到第三方供应商 → 简要展示当前平台的模型状态（P0 暂空，P1 会填 availableModelsJSON）
                        <div className="mb-5 rounded-lg border border-stone-200 p-3 text-sm text-stone-500 dark:border-stone-800">
                            <div className="font-medium text-stone-900 dark:text-stone-100">
                                {config.activeVendorType === "updream" ? "UpDream 云端模型" :
                                 config.activeVendorType === "libtv"   ? "LibTV 云端模型"   :
                                 config.activeVendorType === "newwow"  ? "NewWow 云端模型"  : "第三方云端模型"}
                            </div>
                            {(() => {
                                const bound = accounts.find((a) => a.vendorType === config.activeVendorType);
                                if (!bound) {
                                    return (
                                        <div className="mt-1 text-amber-600 dark:text-amber-500 flex items-center gap-1">
                                            尚未绑定账户，
                                            <Button type="link" size="small" onClick={() => openBindModal(config.activeVendorType as VendorType)}>
                                                前往绑定（粘贴 AccessToken）
                                            </Button>
                                        </div>
                                    );
                                }
                                if (bound.hasModels) {
                                    return <div className="mt-1 text-emerald-700 dark:text-emerald-400">账户可用模型快照已同步，可在各创作页直接选择。</div>;
                                }
                                return <div className="mt-1 text-stone-500">模型列表待刷新，绑定完成后点击「刷新模型」获取。</div>;
                            })()}
                        </div>
                    )}
                    {/* 默认模型选择已移除：各板块（生图/视频/文本/音频）在各自页面内独立选择模型 */}
                    <div className="grid gap-4 md:grid-cols-4">
                        <Form.Item label="画布默认生图张数" extra="新建画布生图和配置节点默认使用，单个节点仍可单独覆盖。" className="mb-4">
                            <Input
                                type="number"
                                min={1}
                                max={15}
                                value={config.canvasImageCount}
                                onChange={(event) => updateConfig("canvasImageCount", event.target.value)}
                                onBlur={(event) => updateConfig("canvasImageCount", normalizeImageCount(event.target.value))}
                            />
                        </Form.Item>
                        {/* ✅ 普通用户隐藏专业音频参数（保留给管理员），使用系统后台配置的默认值即可 */}
                        {isAdmin ? (
                            isMimoPresetTtsModel(config.audioModel) ? (
                                <Form.Item label="默认 MiMo 音色" className="mb-4">
                                    <Select value={config.mimoTtsVoice} options={[...mimoTtsVoiceOptions]} onChange={(value) => updateConfig("mimoTtsVoice", value)} />
                                </Form.Item>
                            ) : isMimoVoiceDesignModel(config.audioModel) ? (
                                <Form.Item label="默认音色描述" className="mb-4">
                                    <Input value={config.mimoVoiceDesignPrompt} placeholder="例如：年轻女性，声音清亮自然，有亲和力。" onChange={(event) => updateConfig("mimoVoiceDesignPrompt", event.target.value)} />
                                </Form.Item>
                            ) : isMimoTtsModel(config.audioModel) ? null : (
                                <Form.Item label="默认音频声音" className="mb-4">
                                    <Select value={config.audioVoice} options={audioVoiceOptions} onChange={(value) => updateConfig("audioVoice", value)} />
                                </Form.Item>
                            )
                        ) : null}
                        {isAdmin ? (
                            <Form.Item label="默认音频格式" className="mb-4">
                                <Select value={isMimoTtsModel(config.audioModel) ? config.mimoTtsFormat : config.audioFormat} options={isMimoTtsModel(config.audioModel) ? [...mimoTtsFormatOptions] : audioFormatOptions} onChange={(value) => isMimoTtsModel(config.audioModel) ? updateConfig("mimoTtsFormat", value) : updateConfig("audioFormat", value)} />
                            </Form.Item>
                        ) : null}
                        {isAdmin && !isMimoTtsModel(config.audioModel) ? (
                            <Form.Item label="默认音频语速" className="mb-4">
                                <Input
                                    type="number"
                                    min={0.25}
                                    max={4}
                                    step={0.05}
                                    value={config.audioSpeed}
                                    onChange={(event) => updateConfig("audioSpeed", event.target.value)}
                                    onBlur={(event) => updateConfig("audioSpeed", normalizeAudioSpeedValue(event.target.value))}
                                />
                            </Form.Item>
                        ) : null}
                    </div>
                    {/* ✅ 普通用户隐藏 3 个技术开关 + S3/WebDAV 存储，仅管理员可调这些高级参数 */}
                    {isAdmin ? (
                        <div className="mb-4 grid gap-3 md:grid-cols-3">
                            <FeatureSwitch title="流式传输" description="开启后请求中追加 stream，支持读取中间图片事件并避免长时间无数据。" checked={Boolean(config.streamImages)} onChange={(checked) => updateConfig("streamImages", checked ? "1" : "")} />
                            <FeatureSwitch title="返回 Base64 图片数据" description="开启后 Image API 请求会追加 response_format: b64_json。" checked={Boolean(config.responseFormatB64Json)} onChange={(checked) => updateConfig("responseFormatB64Json", checked ? "1" : "")} />
                            <FeatureSwitch title="Codex CLI 兼容模式" description="开启后减少不兼容参数，并追加防提示词改写前缀。" checked={Boolean(config.codexCli)} onChange={(checked) => updateConfig("codexCli", checked ? "1" : "")} />
                        </div>
                    ) : null}
                    {/* ✅ S3/R2 + WebDAV 用户自定义存储：管理员永远可见；普通用户仅当管理员在后台开启了"允许用户配置存储"后才显示 */}
                    {canUseUserStorageProvider ? (
                        <>
                            <section className="mb-5 mt-4 rounded-xl border border-stone-200 bg-stone-50/70 p-3 dark:border-stone-800 dark:bg-stone-900/50">
                                <div className="flex items-center justify-between gap-3">
                                    <div>
                                        <div className="text-sm font-medium">用户 S3/R2 存储</div>
                                        <div className="mt-1 text-xs text-stone-500">
                                            开启后，新生成图片和媒体文件会优先保存到你的 S3 兼容对象存储。
                                            {storageUsageText ? <>当前容量：{storageUsageText}</> : null}
                                        </div>
                                    </div>
                                    <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                                        <Button size="small" loading={measuringStorageType === "s3"} onClick={() => void measureStorage(userStorage)}>
                                            统计容量
                                        </Button>
                                        <span className="text-xs text-stone-500">自动同步</span>
                                        <Switch size="small" checked={config.syncStorageConfig} onChange={(checked) => updateConfig("syncStorageConfig", checked)} />
                                        <Switch checked={userStorage.enabled} disabled={userWebDAVStorage.enabled} onChange={(enabled) => setUserStorage((value) => ({ ...value, enabled }))} />
                                    </div>
                                </div>
                                {userStorage.enabled ? (
                                    <div className="mt-3 grid gap-3 md:grid-cols-2">
                                        <Input value={userStorage.name} placeholder="配置名称" onChange={(event) => setUserStorage((value) => ({ ...value, name: event.target.value }))} />
                                        <Input value={userStorage.endpoint} placeholder="Endpoint，例如 https://<account>.r2.cloudflarestorage.com" onChange={(event) => setUserStorage((value) => ({ ...value, endpoint: event.target.value }))} />
                                        <Input value={userStorage.region} placeholder="Region，R2 通常为 auto" onChange={(event) => setUserStorage((value) => ({ ...value, region: event.target.value }))} />
                                        <Input value={userStorage.bucket} placeholder="Bucket 名称" onChange={(event) => setUserStorage((value) => ({ ...value, bucket: event.target.value }))} />
                                        <Input value={userStorage.accessKeyId} placeholder="Access Key ID" onChange={(event) => setUserStorage((value) => ({ ...value, accessKeyId: event.target.value }))} />
                                        <Input.Password value={userStorage.secretAccessKey} placeholder="Secret Access Key" onChange={(event) => setUserStorage((value) => ({ ...value, secretAccessKey: event.target.value }))} />
                                        <Input value={userStorage.publicBaseUrl} placeholder="公开访问地址，例如 https://pub-xxx.r2.dev" onChange={(event) => setUserStorage((value) => ({ ...value, publicBaseUrl: event.target.value }))} />
                                        <Input value={userStorage.pathPrefix} placeholder="保存路径前缀，例如 images" onChange={(event) => setUserStorage((value) => ({ ...value, pathPrefix: event.target.value }))} />
                                    </div>
                                ) : null}
                            </section>
                            <section className="mb-5 mt-4 rounded-xl border border-stone-200 bg-stone-50/70 p-3 dark:border-stone-800 dark:bg-stone-900/50">
                                <div className="flex items-center justify-between gap-3">
                                    <div>
                                        <div className="text-sm font-medium">WebDAV 存储</div>
                                        <div className="mt-1 text-xs text-stone-500">
                                            开启后，新生成图片和媒体文件会优先保存到你的 WebDAV。
                                            {webDAVStorageUsageText ? <>当前容量：{webDAVStorageUsageText}</> : null}
                                        </div>
                                    </div>
                                    <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                                        <Button size="small" loading={measuringStorageType === "webdav"} onClick={() => void measureStorage(userWebDAVStorage)}>
                                            统计容量
                                        </Button>
                                        <span className="text-xs text-stone-500">自动同步</span>
                                        <Switch size="small" checked={config.syncWebDAVStorageConfig} onChange={(checked) => updateConfig("syncWebDAVStorageConfig", checked)} />
                                        <Switch checked={userWebDAVStorage.enabled} disabled={userStorage.enabled} onChange={(enabled) => setUserWebDAVStorage((value) => ({ ...value, enabled }))} />
                                    </div>
                                </div>
                                {userWebDAVStorage.enabled ? (
                                    <div className="mt-3 grid gap-3 md:grid-cols-2">
                                        <Input value={userWebDAVStorage.name} placeholder="配置名称" onChange={(event) => setUserWebDAVStorage((value) => ({ ...value, name: event.target.value }))} />
                                        <Input value={userWebDAVStorage.endpoint} placeholder="WebDAV 地址" onChange={(event) => setUserWebDAVStorage((value) => ({ ...value, endpoint: event.target.value }))} />
                                        <Input value={userWebDAVStorage.pathPrefix} placeholder="远程目录" onChange={(event) => setUserWebDAVStorage((value) => ({ ...value, pathPrefix: event.target.value }))} />
                                        <Input value={userWebDAVStorage.username} placeholder="用户名" onChange={(event) => setUserWebDAVStorage((value) => ({ ...value, username: event.target.value }))} />
                                        <Input.Password value={userWebDAVStorage.password} placeholder="密码 / 应用密码" onChange={(event) => setUserWebDAVStorage((value) => ({ ...value, password: event.target.value }))} />
                                    </div>
                                ) : null}
                            </section>
                        </>
                    ) : null}
                    {/* ✅ 默认音频指令：所有人都可见，普通用户可能想设置自己喜欢的音色偏好（温暖/自然等） */}
                    {!isMimoTtsModel(config.audioModel) || isMimoPresetTtsModel(config.audioModel) || isMimoVoiceCloneModel(config.audioModel) ? (
                        <Form.Item label="默认音频指令" className="mb-4">
                            <Input.TextArea rows={2} value={config.audioInstructions} placeholder="例如：自然、温暖、适合旁白。" onChange={(event) => updateConfig("audioInstructions", event.target.value)} />
                        </Form.Item>
                    ) : null}
                    {isAdmin && effectiveMode === "local" ? (
                        <Form.Item label="系统提示词" className="mb-0">
                            <Input.TextArea rows={3} value={config.systemPrompt} placeholder="例如：你是一位擅长电影感写实摄影的视觉导演。" onChange={(event) => updateConfig("systemPrompt", event.target.value)} />
                        </Form.Item>
                    ) : null}
                </Form>
            </div>
        </Modal>
    );
}

function FeatureSwitch({ title, description, checked, onChange }: { title: string; description: string; checked: boolean; onChange: (checked: boolean) => void }) {
    return (
        <div className="rounded-lg border border-stone-200 px-3 py-2 dark:border-stone-800">
            <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-medium">{title}</div>
                <Switch checked={checked} onChange={onChange} />
            </div>
            <div className="mt-1 text-xs leading-5 text-stone-500">{description}</div>
        </div>
    );
}

function configForLocalChannel(config: AiConfig, channel: LocalModelChannel): AiConfig {
    return {
        ...config,
        channelMode: "local",
        baseUrl: channel.baseUrl,
        apiKey: channel.apiKey,
        localChannels: [{ ...channel }],
        imageChannelId: channel.id,
        videoChannelId: channel.id,
        textChannelId: channel.id,
        audioChannelId: channel.id,
        model: channel.models[0] || config.model,
    };
}

function channelIdForLocalModel(channels: LocalModelChannel[], model: string, currentId: string) {
    if (!channels.length) return "";
    if (channels.some((channel) => channel.id === currentId && (!model || channel.models.includes(model)))) return currentId;
    return channels.find((channel) => model && channel.models.includes(model))?.id || channels[0].id;
}

function normalizeImageCount(value: string) {
    return String(Math.max(1, Math.min(15, Math.floor(Math.abs(Number(value)) || 3))));
}


function uniqueModels(models: string[]) {
    return Array.from(new Set(models.map((model) => model.trim()).filter(Boolean)));
}

function formatBytes(bytes: number) {
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
}
