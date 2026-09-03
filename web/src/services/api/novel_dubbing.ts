// novel-workflow v2: shot-dubbing-node API client
// 后端路由: /api/v1/novel/dubbing/*

import { apiGet, apiPost } from './request';

export interface ShotDubbing {
  id: string;
  userId: string;
  projectId: string;
  shotId: string;
  text: string;
  voiceId: string;
  speed: number;
  ttsModel: string;
  audioUrl: string;
  durationMs: number;
  bytes: number;
  mimeType: string;
  status: '' | 'success' | 'failure' | 'skipped';
  error: string;
  genericTaskId: string;
  balanceLogId: string;
  costCents: number;
  createdAt: string;
  updatedAt: string;
  completedAt: string;
}

export interface DispatchShotDubbingReq {
  projectId: string;
  shotId: string;
  text: string;
  voiceId?: string;
  speed?: number;
}

export interface DispatchProjectDubbingReq {
  projectId: string;
  voiceId?: string;
  speed?: number;
  shots: { shotId: string; text: string }[];
}

// POST /api/v1/novel/dubbing/dispatch
export async function dispatchShotDubbing(req: DispatchShotDubbingReq): Promise<void> {
  await apiPost<void>('/api/v1/novel/dubbing/dispatch', req);
}

// POST /api/v1/novel/dubbing/dispatch-project
export async function dispatchProjectDubbing(req: DispatchProjectDubbingReq): Promise<void> {
  await apiPost<void>('/api/v1/novel/dubbing/dispatch-project', req);
}

// GET /api/v1/novel/dubbing?projectId=...
export async function listShotDubbings(projectId: string): Promise<ShotDubbing[]> {
  return apiGet<ShotDubbing[]>('/api/v1/novel/dubbing', { projectId });
}
