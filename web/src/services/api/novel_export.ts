// novel-workflow v2: export-layer API client
// 后端路由: /api/v1/novel/export/*

import { apiGet, apiPost } from './request';

export interface ExportMetadata {
  projectId: string;
  projectTitle: string;
  compositionId: string;
  outputUrl: string;
  outputSize: number;
  outputMime: string;
  durationSeconds: number;
  createdAt: string;
  bgmName: string;
  subtitleStyle: string;
  shotCount: number;
  totalNodes: number;
}

export type Platform = 'douyin' | 'xiaohongshu' | 'shipinhao';

// GET /api/v1/novel/export/metadata?compositionId=...
export async function getExportMetadata(compositionId: string): Promise<ExportMetadata> {
  return apiGet<ExportMetadata>('/api/v1/novel/export/metadata', { compositionId });
}

// POST /api/v1/novel/export/caption
export async function generatePlatformCaption(req: {
  platform: Platform;
  projectTitle: string;
  description?: string;
}): Promise<string> {
  const data = await apiPost<{ caption: string }>('/api/v1/novel/export/caption', req);
  return data.caption;
}

// GET /api/v1/novel/export/history?projectId=...
export async function listExportHistory(projectId: string): Promise<{
  id: string;
  outputUrl: string;
  outputSize: number;
  createdAt: string;
  completedAt: string;
}[]> {
  return apiGet<any[]>('/api/v1/novel/export/history', { projectId });
}

// 下载 mp4 (直接用 <a> 标签触发, 不需要 fetch)
// GET /api/v1/novel/export/download?compositionId=...
export function buildDownloadUrl(compositionId: string): string {
  return `/api/v1/novel/export/download?compositionId=${encodeURIComponent(compositionId)}`;
}

// 平台文案（前端也可直接生成，避免每次调后端）
export const PLATFORM_NAMES: Record<Platform, string> = {
  douyin: '抖音',
  xiaohongshu: '小红书',
  shipinhao: '视频号',
};

export const PLATFORM_LIST: Platform[] = ['douyin', 'xiaohongshu', 'shipinhao'];
