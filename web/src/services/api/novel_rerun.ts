// novel-workflow v2: novel-rerun-layer (核心 UX) API client
// 后端路由: /api/v1/novel/rerun/*

import { apiGet, apiPost } from './request';

export type RerunScope = 'shot' | 'full';
export type RerunLayer = 'video' | 'dubbing' | 'subtitle' | 'composition' | 'full';
export type RerunStatus = 'running' | 'success' | 'failure' | 'canceled';

export interface RerunRecord {
  id: string;
  userId: string;
  projectId: string;
  runId: string;
  scope: RerunScope;
  layer: RerunLayer;
  shotId: string;
  version: number;
  payloadJson: string;
  status: RerunStatus;
  outputUrl: string;
  error: string;
  genericTaskId: string;
  createdAt: string;
  updatedAt: string;
  completedAt: string;
}

// POST /api/v1/novel/rerun/shot
export async function rerunShotLayer(req: {
  runId: string;
  shotId: string;
  layer: 'video' | 'dubbing' | 'subtitle';
  text?: string;
  voiceId?: string;
  speed?: number;
  projectId: string;
}): Promise<RerunRecord> {
  return apiPost<RerunRecord>(
    `/api/v1/novel/rerun/shot?projectId=${encodeURIComponent(req.projectId)}`,
    {
      runId: req.runId,
      shotId: req.shotId,
      layer: req.layer,
      text: req.text,
      voiceId: req.voiceId,
      speed: req.speed,
    },
  );
}

// POST /api/v1/novel/rerun/full
export async function rerunFullLayer(req: {
  runId: string;
  layer: 'subtitle' | 'composition' | 'full';
  projectId: string;
  compositionInput?: any;
  subtitleStyle?: any;
}): Promise<{ record: RerunRecord; composition: any }> {
  return apiPost<{ record: RerunRecord; composition: any }>(
    `/api/v1/novel/rerun/full?projectId=${encodeURIComponent(req.projectId)}`,
    {
      runId: req.runId,
      layer: req.layer,
      compositionInput: req.compositionInput,
      subtitleStyle: req.subtitleStyle,
    },
  );
}

// POST /api/v1/novel/rerun/rollback
export async function rollbackToVersion(recordId: string): Promise<void> {
  await apiPost<void>('/api/v1/novel/rerun/rollback', { recordId });
}

// GET /api/v1/novel/rerun/versions?projectId=...&scope=...&layer=...&shotId=...
export async function listVersions(req: {
  projectId: string;
  scope: RerunScope;
  layer: RerunLayer;
  shotId?: string;
}): Promise<RerunRecord[]> {
  return apiGet<RerunRecord[]>('/api/v1/novel/rerun/versions', req);
}

// GET /api/v1/novel/rerun/latest?...
export async function getLatestVersion(req: {
  projectId: string;
  scope: RerunScope;
  layer: RerunLayer;
  shotId?: string;
}): Promise<RerunRecord | null> {
  return apiGet<RerunRecord | null>('/api/v1/novel/rerun/latest', req);
}

// 4 个分镜重做按钮的中文 label
export const RERUN_LAYER_LABELS: Record<RerunLayer, string> = {
  video: '重做视频',
  dubbing: '重做配音',
  subtitle: '重做字幕',
  composition: '重做合成',
  full: '重做全部',
};
