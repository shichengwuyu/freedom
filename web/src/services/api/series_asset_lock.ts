// novel-workflow v2: series-asset-lock (漫剧一致性) API client
// 后端路由: /api/v1/novel/series-asset-lock/*

import { apiGet, apiPost, apiPut } from './request';

export interface SeriesAssetLock {
  id: string;
  userId: string;
  projectId: string;
  characterIdsJson: string;
  sceneIdsJson: string;
  propIdsJson: string;
  globalStylePrompt: string;
  isLocked: boolean;
  lockedAt: string;
  unlockedAt: string;
  createdAt: string;
  updatedAt: string;
}

// 解析后的主资产包（前端易用视图）
export interface SeriesAssetLockParsed {
  id: string;
  projectId: string;
  characterIds: string[];
  sceneIds: string[];
  propIds: string[];
  globalStylePrompt: string;
  isLocked: boolean;
  lockedAt: string;
}

// GET /api/v1/novel/series-asset-lock?projectId=...
export async function getSeriesAssetLock(projectId: string): Promise<SeriesAssetLock | null> {
  return apiGet<SeriesAssetLock | null>('/api/v1/novel/series-asset-lock', { projectId });
}

// PUT /api/v1/novel/series-asset-lock?projectId=...
export async function updateSeriesAssetLock(req: {
  projectId: string;
  characterIds: string[];
  sceneIds: string[];
  propIds: string[];
  globalStylePrompt: string;
}): Promise<SeriesAssetLock> {
  return apiPut<SeriesAssetLock>(
    `/api/v1/novel/series-asset-lock?projectId=${encodeURIComponent(req.projectId)}`,
    {
      characterIds: req.characterIds,
      sceneIds: req.sceneIds,
      propIds: req.propIds,
      globalStylePrompt: req.globalStylePrompt,
    },
  );
}

// POST /api/v1/novel/series-asset-lock/lock?projectId=...
export async function lockSeriesAssetLock(projectId: string): Promise<SeriesAssetLock> {
  return apiPost<SeriesAssetLock>(
    `/api/v1/novel/series-asset-lock/lock?projectId=${encodeURIComponent(projectId)}`,
  );
}

// POST /api/v1/novel/series-asset-lock/unlock?projectId=...
export async function unlockSeriesAssetLock(projectId: string): Promise<SeriesAssetLock> {
  return apiPost<SeriesAssetLock>(
    `/api/v1/novel/series-asset-lock/unlock?projectId=${encodeURIComponent(projectId)}`,
  );
}

// 工具：把后端 JSON 解析为 string[] 视图
export function parseSeriesAssetLock(lock: SeriesAssetLock | null): SeriesAssetLockParsed | null {
  if (!lock) return null;
  return {
    id: lock.id,
    projectId: lock.projectId,
    characterIds: lock.characterIdsJson ? JSON.parse(lock.characterIdsJson) : [],
    sceneIds: lock.sceneIdsJson ? JSON.parse(lock.sceneIdsJson) : [],
    propIds: lock.propIdsJson ? JSON.parse(lock.propIdsJson) : [],
    globalStylePrompt: lock.globalStylePrompt,
    isLocked: lock.isLocked,
    lockedAt: lock.lockedAt,
  };
}
