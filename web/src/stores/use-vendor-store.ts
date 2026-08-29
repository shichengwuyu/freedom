// use-vendor-store.ts：供应商状态管理（配套文档 §6.3）
// P0 阶段做空壳骨架：
//   - vendors：GET /api/vendors 拉（或后端 DB 空时 service 里返回内置 4 家兜底）
//   - accounts：GET /api/v1/vendor/accounts 拉（未登录或 DB 空=[]）
//   - activateVendor：切 activeVendorType，P0 不校验绑定状态直接走 state；
//                     同步会 POST /api/v1/vendor/activate，失败了只 Toast 不阻塞 UI（UI 优先）。

"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import { message } from "antd";
import { localForageStorage, persistStorage } from "@/lib/localforage-storage";
import {
    activateVendor as apiActivateVendor,
    getVendorAccounts as apiGetVendorAccounts,
    getVendors as apiGetVendors,
    refreshVendorBalance as apiRefreshVendorBalance,
    refreshVendorModels as apiRefreshVendorModels,
    unbindVendorAccount as apiUnbindVendorAccount,
    type BoundAccount,
    type PublicVendorInfo,
    type VendorType,
} from "@/services/api/vendor";
import { useConfigStore } from "./use-config-store";

export type { BoundAccount, PublicVendorInfo, VendorType };

type VendorState = {
    // ========== 可持久化的 state（localStorage）==========
    vendors: PublicVendorInfo[];      // 供应商列表（UI 下拉用）
    accounts: BoundAccount[];         // 当前用户绑定账户
    fetchedAtVendors: number;         // vendors 最后拉取时间（ms）
    fetchedAtAccounts: number;        // accounts 最后拉取时间（ms）

    // ========== 临时状态 ==========
    isLoading: boolean;
    errorMsg: string;

    // ========== 对外 Actions ==========
    // loadVendors：刷新供应商列表（force=true 时即使刚拉过也强制刷新；默认 5min 内不重复拉）
    loadVendors: (force?: boolean) => Promise<void>;
    // loadAccounts：刷新当前用户绑定账户（未登录=空数组，不报错）
    loadAccounts: (force?: boolean) => Promise<void>;
    // activateVendor：切换激活供应商（type="official" 时切回官方，其他走后端 activate API）
    activateVendor: (type: VendorType) => Promise<void>;
    // unbindVendor：P1 再实现，P0 先占位抛"尚未绑定，无法解绑"
    unbindVendor: (type: VendorType) => Promise<void>;
    // refreshVendorModels：刷新某供应商账户的模型快照（拉后端 /vendor/refresh-models）
    refreshVendorModels: (type: VendorType) => Promise<void>;
    // refreshBalance：刷新某供应商账户的余额/套餐快照（拉后端 /vendor/refresh-balance）
    refreshBalance: (type: VendorType) => Promise<void>;
    // clearLoadingError：UI 关闭弹窗时可重置
    clearLoadingError: () => void;
};

// 5 分钟缓存，避免每个组件挂载都打一次
const VENDORS_STALE_MS = 5 * 60 * 1000;
const ACCOUNTS_STALE_MS = 30 * 1000; // 账户状态 30 秒内不重复拉（刷新绑定状态更实时）

export const useVendorStore = create<VendorState>()(
    persist(
        (set, get) => ({
            vendors: [],
            accounts: [],
            fetchedAtVendors: 0,
            fetchedAtAccounts: 0,
            isLoading: false,
            errorMsg: "",

            loadVendors: async (force) => {
                const now = Date.now();
                const { vendors, fetchedAtVendors, isLoading } = get();
                if (isLoading) return;
                if (!force && vendors.length > 0 && now - fetchedAtVendors < VENDORS_STALE_MS) return;
                set({ isLoading: true, errorMsg: "" });
                try {
                    const list = await apiGetVendors();
                    set({ vendors: list, fetchedAtVendors: now, isLoading: false });
                } catch (e) {
                    const msg = e instanceof Error ? e.message : "拉取供应商列表失败";
                    set({ isLoading: false, errorMsg: msg });
                    message.error(msg);
                }
            },

            loadAccounts: async (force) => {
                const now = Date.now();
                const { accounts, fetchedAtAccounts, isLoading } = get();
                if (isLoading) return;
                if (!force && accounts.length > 0 && now - fetchedAtAccounts < ACCOUNTS_STALE_MS) return;
                set({ isLoading: true, errorMsg: "" });
                try {
                    const list = await apiGetVendorAccounts();
                    set({ accounts: list, fetchedAtAccounts: now, isLoading: false });
                } catch (e) {
                    const msg = e instanceof Error ? e.message : "拉取绑定账户失败";
                    set({ isLoading: false, errorMsg: msg });
                    message.error(msg);
                }
            },

            activateVendor: async (type) => {
                // 先乐观更新本地 config.activeVendorType → 即使后端激活 API 失败，UI 也先反馈
                const { updateConfig } = useConfigStore.getState();
                updateConfig("activeVendorType", type);
                set({ errorMsg: "" });

                // 无论 official 还是非 official，都要通知后端：
                // official 时后端会把该用户所有 vendor account 的 is_active 置为 false，
                // 这样画布生图等后端逻辑（handler/canvas_task.go 按 DB is_active 判断供应商模式）
                // 才能正确回落到官方路径，而不是继续走之前激活的 libtv/updream/newwow。
                try {
                    await apiActivateVendor(type);
                    // 激活成功后刷一次账户状态（会更新 isActive 标志，UI 上能看到"当前"Badge）
                    await get().loadAccounts(true);
                } catch (e) {
                    const msg = e instanceof Error ? e.message : "切换失败，请稍后重试";
                    set({ errorMsg: msg });
                    message.warning(msg + "（当前仅切换了 UI 状态）");
                }
            },

            unbindVendor: async (type) => {
                try {
                    await apiUnbindVendorAccount(type);
                } catch (e) {
                    const msg = e instanceof Error ? e.message : "解绑失败";
                    message.error(msg);
                    throw e;
                }
                // 刷新账户列表（解绑后该账户消失）
                await get().loadAccounts(true);
                // 被解绑的供应商若正是当前激活的，回落官方
                const { config, updateConfig } = useConfigStore.getState();
                if (config.activeVendorType === type) {
                    updateConfig("activeVendorType", "official");
                }
                message.success("已解绑该供应商账户");
            },

refreshVendorModels: async (type) => {
        let account: BoundAccount | null = null;
        try {
            account = await apiRefreshVendorModels(type);
        } catch (e) {
            const msg = e instanceof Error ? e.message : "刷新模型失败";
            message.error(msg);
            throw e;
        }
        if (!account) return;
        const updated = account;
        // 就地更新对应账户的快照，避免整列重新拉取
        set((s) => ({
            accounts: s.accounts.map((a) =>
                a.vendorType === type
                    ? { ...a, ...updated, availableModelsJson: updated.availableModelsJson, hasModels: Boolean(updated.availableModelsJson) }
                    : a,
            ),
            fetchedAtAccounts: Date.now(),
        }));
        message.success("模型列表已刷新");
    },

    refreshBalance: async (type) => {
        let account: BoundAccount | null = null;
        try {
            account = await apiRefreshVendorBalance(type);
        } catch (e) {
            const msg = e instanceof Error ? e.message : "刷新余额失败";
            message.error(msg);
            throw e;
        }
        if (!account) return;
        const updated = account;
        // 就地更新对应账户的 balanceText
        set((s) => ({
            accounts: s.accounts.map((a) =>
                a.vendorType === type ? { ...a, ...updated } : a,
            ),
            fetchedAtAccounts: Date.now(),
        }));
        message.success("余额已刷新");
    },

            clearLoadingError: () => set({ errorMsg: "" }),
        }),
        {
            name: "freedom:vendor_store",
            storage: persistStorage,
            // 不持久化 accounts / fetchedAtAccounts：isActive / balanceText / avatarUrl / hasModels 都是服务端权威，
            // 本地缓存会让"当前激活"出现幽灵态（跨设备 / 多 tab / 重连后不一致）。
            // 每次启动由 loadAccounts(false) 在 accounts.length===0 时自动从后端拉一遍；首次会有几十 ms 的空状态，可接受。
            // 仍缓存 vendors（5min）和 fetchedAtVendors，减少首屏重复请求。
            partialize: (s) => ({
                vendors: s.vendors,
                fetchedAtVendors: s.fetchedAtVendors,
            }),
        }
    )
);
