// novel-workflow v2: composition-layer API client
// 后端路由: /api/v1/novel/composition/*

import { apiGet, apiPost } from './request';

export type CompositionStatus =
  | '未启动'
  | '排队中'
  | '进行中'
  | '成功'
  | '失败'
  | '跳过'
  | '已取消';

export interface CompositionShotVideo {
  shotId: string;
  url: string;
  durationMs: number;
}

export interface CompositionShotDubbing {
  shotId: string;
  url: string;
  durationMs: number;
}

export interface CompositionShotSubtitle {
  shotId: string;
  lines: { startMs: number; endMs: number; text: string }[];
}

export interface CompositionBGMSource {
  presetId?: string;
  customId?: string;
}

export interface SubtitleStyleJSON {
  font?: string;
  size?: number;
  color?: string;
  outline?: string;
  outlineWidth?: number;
  background?: string;
  position?: 'bottom' | 'center' | 'top';
  marginBottom?: number;
}

export interface CompositionInput {
  shotVideos: CompositionShotVideo[];
  shotDubbings?: CompositionShotDubbing[];
  shotSubtitles?: CompositionShotSubtitle[];
  bgmSource?: CompositionBGMSource;
  bgmVolume?: number;
  bgmFadeInMs?: number;
  bgmFadeOutMs?: number;
  subtitleStyle?: SubtitleStyleJSON;
}

export interface CompositionTask {
  id: string;
  userId: string;
  projectId: string;
  workflowRunId: string;
  workflowNodeId: string;
  genericTaskId: string;
  inputJson: string;
  progressJson: string;
  outputUrl: string;
  outputSize: number;
  outputMime: string;
  status: CompositionStatus;
  error: string;
  stderrTail: string;
  createdAt: string;
  updatedAt: string;
  startedAt: string;
  completedAt: string;
}

// POST /api/v1/novel/composition
export async function createCompositionTask(req: {
  projectId: string;
  workflowRunId?: string;
  workflowNodeId?: string;
  input: CompositionInput;
}): Promise<CompositionTask> {
  return apiPost<CompositionTask>('/api/v1/novel/composition', req);
}

// GET /api/v1/novel/composition/:id
export async function getCompositionTask(id: string): Promise<CompositionTask> {
  return apiGet<CompositionTask>(`/api/v1/novel/composition/${encodeURIComponent(id)}`);
}

// POST /api/v1/novel/composition/:id/start
export async function startCompositionTask(id: string): Promise<CompositionTask> {
  return apiPost<CompositionTask>(`/api/v1/novel/composition/${encodeURIComponent(id)}/start`);
}

// POST /api/v1/novel/composition/:id/stop
export async function stopCompositionTask(id: string): Promise<void> {
  await apiPost<void>(`/api/v1/novel/composition/${encodeURIComponent(id)}/stop`);
}

// POST /api/v1/novel/composition/:id/retry
export async function retryCompositionTask(id: string): Promise<CompositionTask> {
  return apiPost<CompositionTask>(`/api/v1/novel/composition/${encodeURIComponent(id)}/retry`);
}

// GET /api/v1/novel/composition?projectId=...
export async function listCompositionTasks(
  projectId: string,
  page = 1,
  pageSize = 20,
): Promise<{ tasks: CompositionTask[]; total: number; page: number; pageSize: number }> {
  return apiGet<{ tasks: CompositionTask[]; total: number; page: number; pageSize: number }>(
    '/api/v1/novel/composition',
    { projectId, page, pageSize },
  );
}

// 5 步骤固定列表（前端展示用）
export const COMPOSITION_STEPS = [
  { idx: 1, label: '归一化镜头视频' },
  { idx: 2, label: '拼接' },
  { idx: 3, label: '混音' },
  { idx: 4, label: '烧字幕' },
  { idx: 5, label: '输出' },
] as const;
