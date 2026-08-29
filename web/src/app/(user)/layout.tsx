"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";

import { AppTopNav } from "@/components/layout/app-top-nav";
import { fetchUserConfig } from "@/services/api/user-config";
import { useUserStore } from "@/stores/use-user-store";

const protectedPrefixes = ["/asset-library", "/wallet"];

export default function UserLayout({ children }: { children: ReactNode }) {
    const pathname = usePathname();
    const router = useRouter();
    const user = useUserStore((state) => state.user);
    const isReady = useUserStore((state) => state.isReady);
    const wasLoggedOutRef = useRef(false);
    // 记录上次已处理过的 userId，避免同一个用户多次（如 余额变化）触发同步链路
    const lastSyncedUserIdRef = useRef<string | null>(null);
    const isProtectedPage = protectedPrefixes.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));

    useEffect(() => {
        if (!isReady || !isProtectedPage || user) return;
        router.replace(`/login?redirect=${encodeURIComponent(pathname)}`);
    }, [isProtectedPage, isReady, pathname, router, user]);

    useEffect(() => {
        if (!isReady) return;
        if (!user) {
            wasLoggedOutRef.current = true;
            lastSyncedUserIdRef.current = null; // 登出清空，下次登录重新走同步
            return;
        }
        // ✅ 优化：同一个 userId 短时间内只触发一次完整同步（余额更新等场景不再重跑）
        if (lastSyncedUserIdRef.current === user.id) return;
        const syncCanvasAfterLogin = wasLoggedOutRef.current;
        const token = useUserStore.getState().token;
        if (!token) return;
        wasLoggedOutRef.current = false;
        lastSyncedUserIdRef.current = user.id;
        // fetchUserConfig 服务层自带 10 秒共享缓存，这里直接调不会和 ClientRootInit 重复发请求
        fetchUserConfig(token).then(async (config) => {
            const syncEnabled = config.syncCapabilities?.userData === true;
            // ✅ 优化：两个大 store 改为并行动态 import，不再串行等一个再下一个
            const [{ useCanvasStore }, { useAssetStore }] = await Promise.all([
                import("@/app/(user)/canvas/stores/use-canvas-store"),
                import("@/stores/use-asset-store"),
            ]);
            const canvasStore = useCanvasStore.getState();
            canvasStore.setSyncEnabled(syncEnabled);
            if (syncCanvasAfterLogin && syncEnabled && canvasStore.hydrated) {
                void canvasStore.syncWithRemote(token, true);
            }
            void useAssetStore.getState().hydrateAccountAssets(token, syncEnabled);
        }).catch(() => { });
    }, [isReady, user]);

    return (
        <div className="flex h-dvh flex-col overflow-hidden bg-background text-foreground">
            <AppTopNav />
            <div className="min-h-0 flex-1 overflow-hidden">{isProtectedPage && (!isReady || !user) ? null : children}</div>
        </div>
    );
}
