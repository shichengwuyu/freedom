import { apiGet } from "@/services/api/request";

// Sprint 3 公开定价 API（不需登录）

export type PricingGroup = {
    id: string;
    displayName: string;
    ratio: number;
    isDefault?: boolean;
};

export type PricingModelCell = {
    group: string;
    unitCents: number;
    discount?: string;
};

export type PricingModel = {
    model: string;
    label?: string;
    capability?: string;
    unit: "per_call" | "per_second";
    baseCents: number;
    basePerSec?: number;
    groups: PricingModelCell[];
};

export type PricingResponse = {
    groups: PricingGroup[];
    models: PricingModel[];
    now: string;
};

export async function fetchPricing() {
    return apiGet<PricingResponse>("/api/pricing");
}

export async function listActiveUserGroups(_token?: string) {
    // 公开 API 同样能拿到（不需 token）；保留 token 参数兼容 Sprint 3.5 admin UI
    const res = await apiGet<PricingResponse>("/api/pricing");
    return res.groups;
}
