import { apiGet, apiPost } from "@/services/api/request";

export const AUTH_TOKEN_KEY = "freedom-auth-token-v1";

export type UserRole = "guest" | "user" | "admin";

export type AuthUser = {
    id: string;
    username: string;
    displayName: string;
    avatarUrl: string;
    role: UserRole;
    balanceCents: number;
    groupId: string; // Sprint 3：当前用户组
    createdAt: string;
    updatedAt: string;
};

export type AuthSession = {
    token: string;
    user: AuthUser;
};

export type AuthPayload = {
    username: string;
    password: string;
    inviterCode?: string;
};

export type UpdateProfilePayload = {
    displayName?: string;
    avatarUrl?: string;
};

export async function login(payload: AuthPayload) {
    return apiPost<AuthSession>("/api/auth/login", payload);
}

export async function register(payload: AuthPayload) {
    return apiPost<AuthSession>("/api/auth/register", payload);
}

// token 参数保留向后兼容，但 httpOnly cookie 会自动携带认证信息。
export async function fetchCurrentUser(token?: string) {
    return apiGet<AuthUser>("/api/auth/me", undefined, token);
}

export async function updateMyProfile(payload: UpdateProfilePayload, token?: string) {
    return apiPost<AuthUser>("/api/v1/user/profile", payload, token);
}
