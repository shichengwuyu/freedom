import { apiDelete, apiGet, apiPost } from "@/services/api/request";

export type UserTokenStatus = "active" | "disabled" | "exhausted" | "expired";

// 单条 user_token 记录（与后端 model/user_token.go 对齐）。
// KeyHash 字段 GORM tag json:"-" 屏蔽，永远不会出现在前端。
export type UserToken = {
    id: string;
    userId: string;
    name: string;
    // 后端已脱敏："sk-fk-...1234" 格式
    keyPrefix: string;
    status: UserTokenStatus;
    // null = 永不过期
    expiredAt: string | null;
    // 0 = 用用户全局余额
    balanceCapCents: number;
    usedCents: number;
    unlimitedBalance: boolean;
    // "" = 不限
    modelLimits: string;
    allowIps: string;
    lastUsedIp: string;
    lastUsedAt: string | null;
    createdAt: string;
    updatedAt: string;
};

export type UserTokenCreateRequest = {
    name: string;
    expiredAt?: string;
    balanceCapCents?: number;
    unlimitedBalance?: boolean;
    // modelLimits / allowIps 后端已支持，但 Sprint 1.5 UI 不暴露配置入口
    modelLimits?: string[];
    allowIps?: string[];
};

export type UserTokenCreateResponse = {
    token: UserToken;
    // 明文 sk-fk-...：仅在创建时一次性返回，关闭弹窗后无法再看到
    raw: string;
};

export type UserTokenListResponse = {
    items: UserToken[];
    total: number;
};

export async function listUserTokens(token: string) {
    return apiGet<UserTokenListResponse>("/api/v1/user-tokens", undefined, token);
}

export async function createUserToken(token: string, req: UserTokenCreateRequest) {
    return apiPost<UserTokenCreateResponse>("/api/v1/user-tokens", req, token);
}

export async function deleteUserToken(token: string, id: string) {
    return apiDelete<{ code: number; data: { ok: boolean }; msg: string }>(
        `/api/v1/user-tokens/${id}`,
        token,
    );
}

export async function disableUserToken(token: string, id: string) {
    return apiPost<{ code: number; data: { ok: boolean }; msg: string }>(
        `/api/v1/user-tokens/${id}/disable`,
        {},
        token,
    );
}

export async function enableUserToken(token: string, id: string) {
    return apiPost<{ code: number; data: { ok: boolean }; msg: string }>(
        `/api/v1/user-tokens/${id}/enable`,
        {},
        token,
    );
}
