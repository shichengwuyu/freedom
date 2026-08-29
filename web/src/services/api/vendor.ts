// services/api/vendor.ts
// 供应商相关 API 的前端封装（对应 P0 新增的 3 个后端路由）
// 后续 P1 会继续加 OAuth URL / Callback 回跳 / 解绑 / 刷新模型等 API。

import { apiGet, apiPost } from "./request";
import { useUserStore } from "@/stores/use-user-store";

// 供应商类型（和后端 model.VendorType 一字不差）
export type VendorType = "official" | "updream" | "libtv" | "newwow";

// PublicVendorInfo：后端 GET /api/vendors 返回的元素（脱敏供应商元信息）
export type PublicVendorInfo = {
    type: VendorType;
    name: string;
    logoUrl: string;
    enabled: boolean;
    sort: number;
    hasOAuth: boolean;
    apiRootHint?: string;
    authMode?: "cookie" | "custom_header" | "openapi_signature"; // 鉴权模式（前端 placeholder 区分用）
    authHeaderName?: string; // 仅 custom_header 模式：提示用户复制哪个 header
};

// BoundAccount：后端 GET /api/v1/vendor/accounts 返回的元素（脱敏绑定账户）
export type BoundAccount = {
    vendorType: VendorType;
    isActive: boolean;
    displayName: string;
    avatarUrl?: string;
    balanceText?: string;
    hasModels: boolean;
    availableModelsJson?: string; // 模型快照原文（buildVendorEffectiveConfig 直接消费）
    powerHistory?: Record<string, { power: number; updatedAt: string }>; // 模型消耗算力历史
    boundAt: string; // ISO 时间字符串
    lastUsedAt: string;
};

// POST /api/v1/vendor/activate 请求体 & 返回体
export type ActivateVendorRequest = { vendorType: VendorType };
export type ActivateVendorResponse = { activated: VendorType };

// POST /api/v1/vendor/bind-cookie 请求体 & 返回体
export type BindVendorByCookieRequest = {
    vendorType: VendorType;
    cookieString?: string;
    displayName?: string;
    avatarUrl?: string;
    vendorUserId?: string;
    expiresAt?: string; // ISO 字符串
    accessKey?: string;
    accessSecret?: string;
    appKey?: string;
    // custom_header 鉴权模式（UpDream Authorization Bearer / NewWow accesstoken）：用户复制 Request Headers 的对应字段值
    authHeaderName?: string;
    authHeaderValue?: string;
    // 叠加鉴权（cookie 模式附加 header，旧版 UpDream 双链路兼容）：用户粘完整的 header value（含 "Bearer " 前缀）
    authExtraHeaderName?: string;
    authExtraHeaderValue?: string;
};
export type BindVendorByCookieResponse = {
    vendorType: VendorType;
    account: BoundAccount;
};

// ========== 实际请求（带 Authorization Header，token 从 useUserStore 取）==========

// GET /api/vendors（不需要登录；游客也能看到供应商列表用于引导）
export async function getVendors(): Promise<PublicVendorInfo[]> {
    const data = await apiGet<PublicVendorInfo[]>("/api/vendors");
    return data ?? [];
}

// GET /api/v1/vendor/accounts（需要登录；游客返回空数组）
export async function getVendorAccounts(): Promise<BoundAccount[]> {
    const token = useUserStore.getState().token;
    if (!token) {
        return [];
    }
    const data = await apiGet<BoundAccount[]>("/api/v1/vendor/accounts", undefined, token);
    return data ?? [];
}

// POST /api/v1/vendor/activate（需要登录；body { vendorType }）
export async function activateVendor(vendorType: VendorType): Promise<ActivateVendorResponse> {
    const token = useUserStore.getState().token;
    if (!token) {
        throw new Error("请先登录再切换云端供应商");
    }
    const data = await apiPost<ActivateVendorResponse>("/api/v1/vendor/activate", { vendorType } as ActivateVendorRequest, token);
    return data ?? { activated: "official" };
}

// POST /api/v1/vendor/bind-cookie（需要登录）：把 AccessToken / Cookie / AccessKey 绑定到用户账户
// 绑定成功后后端会自动激活该供应商并拉取模型快照，前端再刷新 vendor accounts + 调 useConfigStore.updateConfig("activeVendorType", vendorType)。
export async function bindVendorByCookie(payload: BindVendorByCookieRequest): Promise<BindVendorByCookieResponse> {
    const token = useUserStore.getState().token;
    if (!token) {
        throw new Error("请先登录再绑定云端供应商");
    }
    const data = await apiPost<BindVendorByCookieResponse>("/api/v1/vendor/bind-cookie", payload, token);
    if (!data) {
        throw new Error("绑定失败：后端无响应");
    }
    return data;
}

// POST /api/v1/vendor/refresh-models（需要登录）：手动刷新某家供应商的可用模型快照，返回更新后的账户
export async function refreshVendorModels(vendorType: VendorType): Promise<BoundAccount> {
    const token = useUserStore.getState().token;
    if (!token) {
        throw new Error("请先登录");
    }
    const data = await apiPost<BoundAccount>("/api/v1/vendor/refresh-models", { vendorType }, token);
    if (!data) {
        throw new Error("刷新模型失败：后端无响应");
    }
    return data;
}

// POST /api/v1/vendor/refresh-balance（需要登录）：手动刷新某家供应商的余额/套餐快照，返回更新后的账户
export async function refreshVendorBalance(vendorType: VendorType): Promise<BoundAccount> {
    const token = useUserStore.getState().token;
    if (!token) {
        throw new Error("请先登录");
    }
    const data = await apiPost<BoundAccount>("/api/v1/vendor/refresh-balance", { vendorType }, token);
    if (!data) {
        throw new Error("刷新余额失败：后端无响应");
    }
    return data;
}

// POST /api/v1/vendor/unbind（需要登录）：解绑某家供应商账户（后端会自动回落官方模式）
export async function unbindVendorAccount(vendorType: VendorType): Promise<{ vendorType: VendorType; unbound: boolean }> {
    const token = useUserStore.getState().token;
    if (!token) {
        throw new Error("请先登录");
    }
    const data = await apiPost<{ vendorType: VendorType; unbound: boolean }>("/api/v1/vendor/unbind", { vendorType }, token);
    if (!data) {
        throw new Error("解绑失败：后端无响应");
    }
    return data;
}

// POST /api/v1/vendor/estimate-cost（需要登录）：实时估算当前参数组合的供应商扣费额度
export type EstimateVendorCostRequest = {
    vendorType: VendorType;
    capability: "image" | "video" | "audio" | "text";
    model: string;
    quality?: string;
    size?: string;
    count?: number;
    refImageCount?: number;
    refVideoCount?: number;
    hasSound?: boolean;
};
export type EstimateVendorCostResponse = { credits: number; source: "estimate" | "fallback"; error?: string };

export async function estimateVendorCost(payload: EstimateVendorCostRequest): Promise<EstimateVendorCostResponse> {
    const token = useUserStore.getState().token;
    if (!token) {
        throw new Error("请先登录");
    }
    const data = await apiPost<EstimateVendorCostResponse>("/api/v1/vendor/estimate-cost", payload, token);
    return data ?? { credits: 0, source: "fallback" };
}

// 便捷：尝试从 window 里读到浏览器插件「content.js / background.js」临时暴露的 capture 结果（用于和插件做同域通信，实际用的场景不多，先留 API 占位）
export type VendorExtensionCapture = {
    vendorType: VendorType;
    cookieString?: string;
    authHeaderValue?: string; // UpDream Authorization Bearer / NewWow accesstoken 等 custom_header 凭证
    authHeaderName?: string;
    displayName?: string;
    avatarUrl?: string;
    vendorUserId?: string;
    expiresAt?: string;
};
export function readVendorCaptureFromWindow(vendorType: VendorType): VendorExtensionCapture | null {
    if (typeof window === "undefined") return null;
    try {
        const w = window as unknown as Record<string, unknown>;
        const key = `__FREEDOM_VENDOR_CAPTURE__${vendorType}`;
        const raw = w[key];
        if (!raw || typeof raw !== "object") return null;
        // 仅提取已知字段，忽略任何额外属性，防止注入不可信数据
        const obj = raw as Record<string, unknown>;
        const result: VendorExtensionCapture = { vendorType };
        if (typeof obj.cookieString === "string") result.cookieString = obj.cookieString;
        if (typeof obj.authHeaderValue === "string") result.authHeaderValue = obj.authHeaderValue;
        if (typeof obj.authHeaderName === "string") result.authHeaderName = obj.authHeaderName;
        if (typeof obj.displayName === "string") result.displayName = obj.displayName;
        if (typeof obj.avatarUrl === "string") result.avatarUrl = obj.avatarUrl;
        if (typeof obj.vendorUserId === "string") result.vendorUserId = obj.vendorUserId;
        if (typeof obj.expiresAt === "string") result.expiresAt = obj.expiresAt;
        return result;
    } catch {
        return null;
    }
}
