// novel-workflow v2: bgm-layer API client
// 后端路由: /api/v1/bgm/*

import { apiGet, apiDelete } from './request';

export interface BgmPreset {
  id: string;
  title: string;
  tags: string[];
  fileName: string;
  durationSeconds: number;
  description: string;
  available: boolean;
  availablePath?: string;
}

export interface BgmCustom {
  id: string;
  userId: string;
  projectId: string;
  title: string;
  tagsJson: string;
  fileUrl: string;
  mimeType: string;
  sizeBytes: number;
  durationSeconds: number;
  createdAt: string;
  updatedAt: string;
}

// GET /api/v1/bgm/presets?tag=... （公开）
export async function listBgmPresets(tag?: string): Promise<BgmPreset[]> {
  return apiGet<BgmPreset[]>('/api/v1/bgm/presets', tag ? { tag } : undefined);
}

// GET /api/v1/bgm/custom?projectId=... （登录）
export async function listBgmCustoms(projectId: string): Promise<BgmCustom[]> {
  return apiGet<BgmCustom[]>('/api/v1/bgm/custom', { projectId });
}

// POST /api/v1/bgm/custom/upload （multipart）
export async function uploadBgmCustom(req: {
  projectId: string;
  title: string;
  tags?: string;
  file: File;
}): Promise<BgmCustom> {
  const form = new FormData();
  form.append('projectId', req.projectId);
  form.append('title', req.title);
  if (req.tags) form.append('tags', req.tags);
  form.append('file', req.file);
  const res = await fetch('/api/v1/bgm/custom/upload', {
    method: 'POST',
    body: form,
    credentials: 'include',
  });
  if (!res.ok) throw new Error(`upload failed: ${res.status}`);
  const json = await res.json();
  if (json.code !== 0) throw new Error(json.msg || 'upload failed');
  return json.data;
}

// DELETE /api/v1/bgm/custom/:id
export async function deleteBgmCustom(id: string): Promise<void> {
  await apiDelete<void>(`/api/v1/bgm/custom/${encodeURIComponent(id)}`);
}
