import { apiDelete, apiGet, apiPost, compactApiParams } from "@/services/api/request";
import type { Prompt, PromptListResponse } from "@/services/api/prompts";

export type AdminPromptCategory = {
    category: string;
    name: string;
    description: string;
    file: string;
    githubUrl: string;
    remote: boolean;
};

export type AdminUser = {
    id: string;
    username: string;
    email: string;
    displayName: string;
    avatarUrl: string;
    role: "user" | "admin";
    balanceCents: number;
    affCode: string;
    affCount: number;
    inviterId: string;
    linuxDoId: string;
    status: "active" | "ban";
    lastLoginAt: string;
    createdAt: string;
    updatedAt: string;
};

export type AdminUserListResponse = {
    items: AdminUser[];
    total: number;
};

export type AdminBalanceLog = {
    id: string;
    userId: string;
    userDisplayName: string;
    type: string;
    amount: number;   // 分
    balance: number;  // 分
    relatedId: string;
    remark: string;
    extra: string;
    createdAt: string;
};

export type AdminBalanceLogListResponse = {
    items: AdminBalanceLog[];
    total: number;
};

export type AdminUserQuery = {
    keyword?: string;
    page?: number;
    pageSize?: number;
    startDate?: string;
    endDate?: string;
};

export async function fetchAdminUsers(token: string, query: AdminUserQuery = {}) {
    return apiGet<AdminUserListResponse>("/api/admin/users", compactApiParams(query), token);
}

export async function saveAdminUser(token: string, user: Partial<AdminUser> & { password?: string }) {
    return apiPost<AdminUser>("/api/admin/users", user, token);
}

export async function adjustAdminUserBalance(token: string, id: string, balanceCents: number) {
    return apiPost<AdminUser>(`/api/admin/users/${encodeURIComponent(id)}/balance`, { costCents: balanceCents }, token);
}

export async function deleteAdminUser(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/users/${encodeURIComponent(id)}`, token);
}

export async function fetchAdminBalanceLogs(token: string, query: AdminUserQuery = {}) {
    return apiGet<AdminBalanceLogListResponse>("/api/admin/balance-logs", compactApiParams(query), token);
}

export async function saveAdminBalanceLog(token: string, log: { userId: string; balance: number }) {
    return apiPost<AdminBalanceLog>("/api/admin/balance-logs", log, token);
}

export async function fetchAdminPromptCategories(token: string) {
    return apiGet<AdminPromptCategory[]>("/api/admin/prompt-categories", undefined, token);
}

export async function syncAdminPromptCategory(token: string, category: string) {
    return apiPost<AdminPromptCategory[]>("/api/admin/prompt-categories/sync", { category }, token);
}

export async function syncAdminPromptCategoriesAll(token: string) {
    return apiPost<AdminPromptCategory[]>("/api/admin/prompt-categories/sync-all", {}, token);
}

export type AdminPromptQuery = {
    keyword?: string;
    category?: string;
    tag?: string[];
    page?: number;
    pageSize?: number;
};

export type AdminAsset = {
    id: string;
    title: string;
    type: "text" | "image" | "video" | "audio";
    coverUrl: string;
    tags: string[];
    category: string;
    description: string;
    content: string;
    url: string;
    createdAt: string;
    updatedAt: string;
};

export type AdminAssetListResponse = {
    items: AdminAsset[];
    tags: string[];
    total: number;
};

export async function fetchAdminPrompts(token: string, query: AdminPromptQuery = {}) {
    return apiGet<PromptListResponse>("/api/admin/prompts", compactApiParams(query), token);
}

export async function saveAdminPrompt(token: string, prompt: Partial<Prompt>) {
    return apiPost<Prompt>("/api/admin/prompts", prompt, token);
}

export async function deleteAdminPrompt(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/prompts/${encodeURIComponent(id)}`, token);
}

export async function deleteAdminPrompts(token: string, ids: string[]) {
    return apiPost<boolean>("/api/admin/prompts/batch-delete", { ids }, token);
}

export type PendingPromptListResponse = {
    items: import("./prompts").Prompt[];
    total: number;
};

export type RejectedPromptListResponse = {
    items: import("./prompts").Prompt[];
    total: number;
};

export async function fetchAdminPendingPrompts(token: string, page = 1, pageSize = 20) {
    return apiGet<PendingPromptListResponse>(`/api/admin/prompts/pending?page=${page}&pageSize=${pageSize}`, undefined, token);
}

export async function fetchAdminRejectedPrompts(token: string, page = 1, pageSize = 20) {
    return apiGet<RejectedPromptListResponse>(`/api/admin/prompts/rejected?page=${page}&pageSize=${pageSize}`, undefined, token);
}

export async function approveAdminPrompt(token: string, id: string) {
    return apiPost<boolean>(`/api/admin/prompts/${encodeURIComponent(id)}/approve`, {}, token);
}

export async function rejectAdminPrompt(token: string, id: string) {
    return apiPost<boolean>(`/api/admin/prompts/${encodeURIComponent(id)}/reject`, {}, token);
}

export type AdminAssetQuery = {
    keyword?: string;
    type?: string;
    tag?: string[];
    page?: number;
    pageSize?: number;
};

export async function fetchAdminAssets(token: string, query: AdminAssetQuery = {}) {
    return apiGet<AdminAssetListResponse>("/api/admin/assets", compactApiParams(query), token);
}

export async function saveAdminAsset(token: string, asset: Partial<AdminAsset>) {
    return apiPost<AdminAsset>("/api/admin/assets", asset, token);
}

export async function deleteAdminAsset(token: string, id: string) {
    return apiDelete<boolean>(`/api/admin/assets/${encodeURIComponent(id)}`, token);
}

export type AdminModelChannel = {
    id: string;
    protocol: "openai" | "kie" | "mimo";
    name: string;
    baseUrl: string;
    apiKey: string;
    models: string[];
    modelLabels?: Record<string, string>;
    weight: number;
    timeout: number;
    enabled: boolean;
    remark: string;

    // Sprint 2.5 新增：与后端 model/setting.go::ModelChannel 字段一一对应
    priority?: number; // 0=默认；数字小=优先
    statusCodeMapping?: string; // "429,500,502,503"；空=默认 429/5xx
    cooldownSeconds?: number; // 0=默认 60s
    keys?: string[]; // 多 key 列表；空=回退 apiKey
    group?: string; // Sprint 3 启用，先留空
    capability?: "" | "text" | "image" | "video" | "audio"; // 空=通用
};

export type AdminPublicModelChannelSettings = {
    availableModels: string[];
    modelCosts: AdminModelCost[];
    channels: AdminPublicModelChannelInfo[];
    systemPrompt: string;
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
    allowCustomChannel: boolean;
    allowUserRemoteChannel: boolean;
};

export type AdminModelCostUnit = "per_call" | "per_second";

export type AdminModelCost = {
    model: string;
    label?: string;
    costCents: number;             // 单次扣费，整数存储避免浮点误差
    unit?: AdminModelCostUnit;
    costCentsPerSecond?: number;   // 按秒时每秒扣费（分）
    refVideo?: boolean;
    refAudio?: boolean;
    genAudio?: boolean;
};

export type AdminPublicModelChannelInfo = {
    id: string;
    name: string;
    baseUrl: string;
    models: string[];
    modelLabels?: Record<string, string>;
    weight: number;
    timeout: number;
    enabled: boolean;
    remark: string;

    // Sprint 2.5 新增：后端 PublicModelChannelInfo 已有，前端 type 同步
    priority?: number;
    statusCodeMapping?: string;
    cooldownSeconds?: number;
    keyCount?: number;
    group?: string;
    capability?: string;
};

export type AdminPublicSettings = {
    modelChannel: AdminPublicModelChannelSettings;
    auth: {
        allowRegister: boolean;
        linuxDo: {
            enabled: boolean;
        };
    };
    storage: {
        mode: string;
        allowUserProvider: boolean;
    };
    siteNotice: {
        enabled: boolean;
        title: string;
        contents: string[];
    };
    contactSupport: {
        enabled: boolean;
        wechat: string;
        qq: string;
        wechatQr: string;
        qqGroup: string;
        qqGroupQr: string;
        remark: string;
    };
};

export type AdminStorageProvider = {
    id: string;
    name: string;
    type: "s3" | "webdav" | "local";
    endpoint: string;
    region: string;
    bucket: string;
    accessKeyId: string;
    secretAccessKey: string;
    publicBaseUrl: string;
    pathPrefix: string;
    username: string;
    password: string;
    weight: number;
    enabled: boolean;
    ownerUserId: string;
    capacityBytes: number;
    capacityCheckedAt: string;
    capacityExceeded: boolean;
};

export type AdminPrivateSettings = {
    channels: AdminModelChannel[];
    promptSync: {
        enabled: boolean;
        cron: string;
    };
    aiLog: {
        localDirectReportEnabled: boolean;
        cleanup: {
            enabled: boolean;
            retentionDays: number;
            cron: string;
        };
    };
    auth: {
        linuxDo: {
            clientId: string;
            clientSecret: string;
        };
    };
    storage: {
        mode: string;
        allowUserProvider: boolean;
        allowUserGlobalProvider: boolean;
        providers: AdminStorageProvider[];
        roundRobinCursor: number;
        capacityCheck: {
            enabled: boolean;
            cron: string;
        };
        capacityLimitBytes: number;
    };
    affiliate: {
        enabled: boolean;
        baseRate: number;
        stepRate: number;
        maxRate: number;
        minSettleCents: number;
    };
};

export type AdminAICallLog = {
    id: string;
    userId: string;
    userDisplayName: string;
    endpoint: string;
    method: string;
    model: string;
    channelId: string;
    channelName: string;
    status: number;
    durationMs: number;
    costCents: number;
    requestBody: string;
    responseBody: string;
    error: string;
    createdAt: string;
};

export type AdminAICallLogListResponse = {
    items: AdminAICallLog[];
    total: number;
};

export async function fetchAdminAICallLogs(token: string, query: AdminUserQuery = {}) {
    return apiGet<AdminAICallLogListResponse>("/api/admin/ai-logs", compactApiParams(query), token);
}

export async function fetchAdminAICallLogDates(token: string) {
    return apiGet<string[]>("/api/admin/ai-logs/dates", undefined, token);
}

export async function deleteAdminAICallLogs(token: string, olderThanDays = 7) {
    return apiDelete<{ removedFiles: number }>(`/api/admin/ai-logs?olderThanDays=${encodeURIComponent(String(olderThanDays))}`, token);
}

export async function deleteAdminAICallLogsByDates(token: string, dates: string[]) {
    return apiPost<{ removedFiles: number }>("/api/admin/ai-logs/by-dates", { dates }, token);
}

export type AdminSettings = {
    public: AdminPublicSettings;
    private: AdminPrivateSettings;
};

export async function fetchAdminSettings(token: string) {
    return apiGet<AdminSettings>("/api/admin/settings", undefined, token);
}

export async function saveAdminSettings(token: string, settings: AdminSettings) {
    return apiPost<AdminSettings>("/api/admin/settings", settings, token);
}

export type AdminChannelActionRequest = {
    index?: number;
    channel: AdminModelChannel;
    model?: string;
};

export async function fetchChannelModels(token: string, payload: AdminChannelActionRequest) {
    return apiPost<string[]>("/api/admin/settings/channel-models", payload, token);
}

export async function testChannelModel(token: string, payload: AdminChannelActionRequest) {
    return apiPost<string>("/api/admin/settings/channel-test", payload, token);
}

// ===== Sprint 2.6 渠道健康度 =====

export type AdminChannelHealthItem = {
    channelId: string;
    channelName: string;
    failureCount: number;
    lastFailureAt: string; // RFC3339；空字符串=从未失败过
    lastStatusCode: number;
    isInCooldown: boolean;
    cooldownRemaining: number; // 秒；0=未冷却
    affectedModels: string[];
};

export type AdminChannelFailLogEntry = {
    channelId: string;
    channelName: string;
    model: string;
    capability: string;
    keyIndex: number;
    statusCode: number;
    errorMessage: string;
    at: string; // RFC3339
};

export type AdminChannelsHealth = {
    summary: {
        totalFailures: number;
        uniqueChannels: number;
        uniqueModels: number;
        longestCooldownRemaining: number;
    };
    channels: AdminChannelHealthItem[];
    recentFailures: AdminChannelFailLogEntry[];
    now: string;
};

export async function fetchChannelsHealth(token: string) {
    return apiGet<AdminChannelsHealth>("/api/admin/channels-health", undefined, token);
}

export async function clearChannelCooldowns(token: string) {
    return apiPost<{ code: number; data: { cleared: number }; msg: string }>(
        "/api/admin/channels-health/clear-cooldowns",
        {},
        token,
    );
}

export type StorageCapacityResult = {
    bytes: number;
    limitBytes: number;
    overLimit: boolean;
    checkedAt: string;
    providerName: string;
};

export async function measureAdminStorageProvider(token: string, payload: { index: number; provider: AdminStorageProvider }) {
    return apiPost<StorageCapacityResult>("/api/admin/storage/measure", payload, token);
}
