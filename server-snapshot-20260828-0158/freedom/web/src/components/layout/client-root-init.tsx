"use client";

import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import { usePathname } from "next/navigation";
import { App } from "antd";

import { fetchUserConfig } from "@/services/api/user-config";
import { defaultUserStorageProvider, defaultUserWebDAVStorageProvider, saveUserStorageProvider, saveUserWebDAVStorageProvider } from "@/services/image-storage";
import { useConfigStore, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";

// 模块级冷却：loadPublicSettings 调用冷却 5 秒，避免路由切换间重复请求/重入
// 缩短冷却：管理员在后台修改设置后，用户切换页面能较快看到更新
let lastLoadPublicSettingsAt = 0;
const LOAD_PUBLIC_SETTINGS_COOLDOWN_MS = 5_000;

export function ClientRootInit({ children }: { children: ReactNode }) {
    const { message } = App.useApp();
    const handledConfigParams = useRef(false);
    const modelAdminConfigHintShownRef = useRef(false);
    // 已处理过的 user.id：同一个用户只做一次 fetchUserConfig 配置同步，余额变动不再重跑
    const lastAppliedUserIdRef = useRef<string | null>(null);
    const pathname = usePathname();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const loadPublicSettings = useConfigStore((state) => state.loadPublicSettings);
    const publicSettings = useConfigStore((state) => state.publicSettings);
    const channelMode = useConfigStore((state) => state.config.channelMode);
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const isLoginPage = pathname === "/login" || pathname === "/admin/login";
    const adminRemoteTokenRef = useRef("");

    useEffect(() => {
        // ✅ 优化：公共设置接口加调用冷却，路由切换间不重复发请求
        const now = Date.now();
        if (now - lastLoadPublicSettingsAt < LOAD_PUBLIC_SETTINGS_COOLDOWN_MS) return;
        lastLoadPublicSettingsAt = now;
        void loadPublicSettings();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [loadPublicSettings]);

    useEffect(() => {
        if (!isLoginPage) void hydrateUser();
    }, [hydrateUser, isLoginPage]);

    // 商用版本：已登录用户加载完公共设置后，检查管理员是否配置了模型，没配就明确提示
    useEffect(() => {
        if (modelAdminConfigHintShownRef.current) return;
        if (!token || !user) return;
        if (isLoginPage) return;
        const modelChannel = publicSettings?.modelChannel;
        if (!modelChannel) return;
        modelAdminConfigHintShownRef.current = true;
        const available = modelChannel.availableModels || [];
        if (available.length === 0) {
            message.error("管理员尚未配置可用模型，请联系管理员在后台「系统设置」中添加模型渠道并勾选可用模型");
            return;
        }
    }, [isLoginPage, message, publicSettings, token, user]);

    useEffect(() => {
        if (!token || user?.role !== "admin" || adminRemoteTokenRef.current === token) return;
        adminRemoteTokenRef.current = token;
        if (channelMode !== "remote") updateConfig("channelMode", "remote");
    }, [channelMode, token, updateConfig, user?.role]);

    useEffect(() => {
        if (!token || !user?.id) return;
        // ✅ 优化：同一个用户 id 只做一次配置同步，避免重复调用（余额更新等场景不再触发）
        if (lastAppliedUserIdRef.current === user.id) return;
        lastAppliedUserIdRef.current = user.id;
        // fetchUserConfig 服务层自带 10 秒共享缓存，与 UserLayout 不会重复发请求
        void fetchUserConfig(token)
            .then((payload) => {
                const syncS3 = payload.modelConfig?.syncStorageConfig === true;
                const syncWebDAV = payload.modelConfig?.syncWebDAVStorageConfig === true;
                if (payload.modelConfig) {
                    Object.entries(payload.modelConfig)
                        .forEach(([key, value]) => updateConfig(key as keyof AiConfig, value as never));
                }
                updateConfig("syncStorageConfig", syncS3);
                updateConfig("syncWebDAVStorageConfig", syncWebDAV);
                if (syncS3 && payload.storageProvider?.s3) {
                    saveUserStorageProvider({
                        ...defaultUserStorageProvider(),
                        ...payload.storageProvider.s3,
                        type: "s3",
                    });
                }
                if (syncWebDAV && payload.storageProvider?.webdav) {
                    saveUserWebDAVStorageProvider({
                        ...defaultUserWebDAVStorageProvider(),
                        ...payload.storageProvider.webdav,
                        type: "webdav",
                    });
                }
            })
            .catch(() => {});
    }, [token, updateConfig, user?.id]);

    useEffect(() => {
        if (handledConfigParams.current) return;
        const searchParams = new URLSearchParams(window.location.search);
        const baseUrl = searchParams.get("baseUrl") || searchParams.get("baseurl");
        const apiKey = searchParams.get("apiKey") || searchParams.get("apikey");
        if (!baseUrl && !apiKey) return;
        if (!publicSettings) return;
        handledConfigParams.current = true;
        searchParams.delete("baseUrl");
        searchParams.delete("baseurl");
        searchParams.delete("apiKey");
        searchParams.delete("apikey");
        window.history.replaceState(null, "", `${window.location.pathname}${searchParams.size ? `?${searchParams}` : ""}${window.location.hash}`);
        if (!publicSettings.modelChannel.allowCustomChannel) {
            openConfigDialog(false);
            message.error("后台未允许用户自定义渠道，请联系管理员进行配置");
            return;
        }
        updateConfig("channelMode", "local");
        // 安全限制：只允许从 URL 预填 baseUrl（方便分享渠道地址），不接受 apiKey（防止钓鱼链接窃取密钥）。
        // apiKey 必须由用户手动在配置弹窗里输入。
        if (baseUrl) updateConfig("baseUrl", baseUrl);
        openConfigDialog(false);
    }, [message, openConfigDialog, publicSettings, updateConfig]);

    return <>{children}</>;
}
