import { apiGet, apiPost, compactApiParams } from "@/services/api/request";

export type PromptStatus = "pending" | "approved" | "rejected";

export type Prompt = {
    id: string;
    title: string;
    coverUrl: string;
    videoUrl: string;
    externalUrl: string;
    prompt: string;
    tags: string[];
    category: string;
    githubUrl: string;
    preview: string;
    status: PromptStatus;
    submittedById: string;
    reviewerId: string;
    createdAt: string;
    updatedAt: string;
};

export const ALL_PROMPTS_OPTION = "全部";

export type PromptListResponse = {
    items: Prompt[];
    tags: string[];
    categories: string[];
    total: number;
};

export async function fetchPrompts({ keyword = "", tag = [], category = ALL_PROMPTS_OPTION, page, pageSize }: { keyword?: string; tag?: string[]; category?: string; page?: number; pageSize?: number } = {}) {
    return apiGet<PromptListResponse>(
        "/api/prompts",
        compactApiParams({
            ...(keyword ? { keyword } : {}),
            ...(tag.length ? { tag } : {}),
            ...(category !== ALL_PROMPTS_OPTION ? { category } : {}),
            ...(page ? { page } : {}),
            ...(pageSize ? { pageSize } : {}),
        }),
    );
}

export async function submitPrompt(token: string, data: { title: string; prompt: string; category: string; coverUrl?: string; videoUrl?: string; tags?: string[]; preview?: string }) {
    return apiPost<Prompt>("/api/prompts/submit", data, token);
}

export function formatPromptDate(value: string) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "" : new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" }).format(date);
}
