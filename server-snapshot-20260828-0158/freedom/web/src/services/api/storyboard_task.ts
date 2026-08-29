// 分镜生成任务 API 客户端
// 后端任务化后，分镜生成不再由前端长连接驱动，而是：
//   1) 前端提交任务（POST /api/v1/storyboard-tasks），后端 worker 逐章调文本模型
//   2) 前端轮询任务状态（GET /api/v1/storyboard-tasks/:id），实时拿回已产出分镜与进度
//   3) 刷新/重开页面后只要任务还在，轮询即可恢复进度，不再丢失
// 所有接口走 { code, data, msg } 信封，由 request.ts 的 apiGet/apiPost/apiDelete 解包。

import { apiGet, apiPost, apiDelete } from "@/services/api/request";

// 分镜任务的章节输入（与前端 Chapter 对齐，提交时序列化为 JSON 字符串）
export interface StoryboardTaskChapter {
    title: string;
    content: string;
}

// 分镜任务的资产输入（与前端 Asset 对齐，提交时序列化为 JSON 字符串）
export interface StoryboardTaskAsset {
    alias: string;
    type: "character" | "scene" | "prop" | "reference";
    description: string;
    name: string;
}

// 分镜任务的结果条目（后端逐章追加，前端据 groupIndex/shotIndex 重建 Storyboard 列表）
// 取代了老的 "⚠" 占位字符串：status 字段直接表达单条分镜的成功/失败态。
export type StoryboardTaskShotStatus = "completed" | "failed";

export interface StoryboardTaskResultEntry {
    groupIndex: number;
    shotIndex?: number; // 同组内的分镜序号（一组多条时有值）
    status: StoryboardTaskShotStatus; // completed/failed；failed 时 content 为空、error 有内容
    content?: string;
    error?: string;
}

// 向后兼容：解析历史数据时把"⚠ ..."字符串反序列化为 status="failed" 条目。
// 新数据已不再写"⚠..."，但前端可能读到旧 JSON，统一做归一化避免 UI 误判。
function normalizeShotEntry(raw: any): StoryboardTaskResultEntry | null {
    if (!raw || typeof raw !== "object") return null;
    const content = typeof raw.content === "string" ? raw.content : "";
    const status: StoryboardTaskShotStatus = raw.status === "failed"
        || (content && content.trimStart().startsWith("⚠"))
            ? "failed"
            : "completed";
    return {
        groupIndex: Number(raw.groupIndex) || 0,
        shotIndex: typeof raw.shotIndex === "number" ? raw.shotIndex : undefined,
        status,
        content: status === "failed" ? "" : content,
        error: typeof raw.error === "string" ? raw.error
            : (status === "failed" ? content : ""),
    };
}

// 分镜任务状态：queued=排队 | running=执行中 | completed=完成 | failed=失败 | canceled=用户取消
export type StoryboardTaskStatus = "queued" | "running" | "completed" | "failed" | "canceled";

// 分镜任务完整数据（后端 StoryboardTaskResponse 返回）
export interface StoryboardTask {
    id: string;
    source: string;
    sourceId: string;
    model: string;
    status: StoryboardTaskStatus;
    progress: number;
    doneCount: number;
    totalCount: number;
    shotDuration: number;
    result: string; // JSON 字符串 [{groupIndex, content}]
    error: string;
    createdAt: string;
    updatedAt: string;
    startedAt: string;
    completedAt: string;
}

// 创建分镜任务的入参
export interface CreateStoryboardTaskInput {
    clientTaskId?: string; // 可选，前端预生成 id 用于幂等
    sourceId: string; // 前端 NovelProject.id
    model: string;
    channelId?: string; // X-Model-Channel-ID（云端渠道）
    userChannelId?: string; // 用户本地渠道
    shotDuration: number;
    scriptPrompt: string;
    chapters: StoryboardTaskChapter[];
    assets?: StoryboardTaskAsset[];
}

// createStoryboardTask 创建分镜生成任务，后端 worker 随即开始逐章生成
export async function createStoryboardTask(input: CreateStoryboardTaskInput, token: string): Promise<StoryboardTask> {
    return apiPost<StoryboardTask>(
        "/api/v1/storyboard-tasks",
        {
            clientTaskId: input.clientTaskId ?? "",
            sourceId: input.sourceId,
            model: input.model,
            channelId: input.channelId ?? "",
            userChannelId: input.userChannelId ?? "",
            shotDuration: input.shotDuration,
            scriptPrompt: input.scriptPrompt,
            chapters: JSON.stringify(input.chapters),
            assets: input.assets ? JSON.stringify(input.assets) : "",
        },
        token,
    );
}

// getStoryboardTask 查询单个分镜任务（轮询用），返回最新状态与已产出分镜
export async function getStoryboardTask(id: string, token: string): Promise<StoryboardTask> {
    return apiGet<StoryboardTask>(`/api/v1/storyboard-tasks/${encodeURIComponent(id)}`, undefined, token);
}

// listStoryboardTasks 列出当前用户的分镜任务（用于刷新后恢复进度与查看历史）
export async function listStoryboardTasks(token: string): Promise<StoryboardTask[]> {
    return apiGet<StoryboardTask[]>("/api/v1/storyboard-tasks", undefined, token);
}

// deleteStoryboardTask 删除单个分镜任务
export async function deleteStoryboardTask(id: string, token: string): Promise<{ deleted: boolean }> {
    return apiDelete<{ deleted: boolean }>(`/api/v1/storyboard-tasks/${encodeURIComponent(id)}`, token);
}

// parseStoryboardTaskResult 解析任务的 result 字段为分镜条目数组
// 后端 result 是 JSON 字符串 [{groupIndex, status, content}]，逐章追加；老数据可能含"⚠"占位，归一化为 status="failed"
// 空/非法时返回空数组
export function parseStoryboardTaskResult(result: string): StoryboardTaskResultEntry[] {
    if (!result) return [];
    try {
        const parsed = JSON.parse(result);
        if (!Array.isArray(parsed)) return [];
        const out: StoryboardTaskResultEntry[] = [];
        for (const item of parsed) {
            const norm = normalizeShotEntry(item);
            if (norm) out.push(norm);
        }
        return out;
    } catch {
        // 解析失败忽略，返回空数组
    }
    return [];
}

// 单条分镜是否"真实可生成"（completed 且 content 非空）
export function isShotReady(entry: StoryboardTaskResultEntry | undefined | null): boolean {
    return !!entry && entry.status === "completed" && !!entry.content && entry.content.trim().length > 0;
}

// cancelStoryboardTask 用户中途停止：调后端 cancel 接口，标记 task.status=canceled。
// 后端已在下一个章节开始前看到 status=canceled 后立即停止，不会覆盖已有 result。
export async function cancelStoryboardTask(id: string, token: string): Promise<StoryboardTask> {
    return apiPost<StoryboardTask>(`/api/v1/storyboard-tasks/${encodeURIComponent(id)}/cancel`, {}, token);
}
