// novel-workflow v2: shot-subtitle-node API client
// 后端路由: /api/v1/novel/subtitle/*

import { apiGet, apiPost, apiPut } from './request';

export interface SubtitleLine {
  startMs: number;
  endMs: number;
  text: string;
}

export interface ShotSubtitle {
  id: string;
  userId: string;
  projectId: string;
  shotId: string;
  linesJson: string;
  status: '' | 'success' | 'failure' | 'skipped';
  error: string;
  createdAt: string;
  updatedAt: string;
}

export interface SubtitleStyle {
  font: string;
  size: number;
  color: string;
  outline: string;
  outlineWidth: number;
  background: string;
  position: 'bottom' | 'center' | 'top';
  marginBottom: number;
}

// POST /api/v1/novel/subtitle/dispatch
export async function dispatchShotSubtitle(req: {
  projectId: string;
  shotId: string;
  text: string;
  shotDurationMs?: number;
}): Promise<void> {
  await apiPost<void>('/api/v1/novel/subtitle/dispatch', req);
}

// POST /api/v1/novel/subtitle/dispatch-project
export async function dispatchProjectSubtitle(req: {
  projectId: string;
  shots: { shotId: string; text: string; shotDurationMs?: number }[];
}): Promise<void> {
  await apiPost<void>('/api/v1/novel/subtitle/dispatch-project', req);
}

// PUT /api/v1/novel/subtitle/:projectId/:shotId/lines
export async function updateSubtitleLines(
  projectId: string,
  shotId: string,
  lines: SubtitleLine[],
): Promise<void> {
  await apiPut<void>(
    `/api/v1/novel/subtitle/${encodeURIComponent(projectId)}/${encodeURIComponent(shotId)}/lines`,
    { lines },
  );
}

// GET /api/v1/novel/subtitle?projectId=...&shotId=...
export async function getShotSubtitle(
  projectId: string,
  shotId: string,
): Promise<{ subtitle: ShotSubtitle; lines: SubtitleLine[] } | null> {
  return apiGet<{ subtitle: ShotSubtitle; lines: SubtitleLine[] } | null>(
    '/api/v1/novel/subtitle',
    { projectId, shotId },
  );
}

// GET /api/v1/novel/subtitles?projectId=...
export async function listShotSubtitles(projectId: string): Promise<ShotSubtitle[]> {
  return apiGet<ShotSubtitle[]>('/api/v1/novel/subtitles', { projectId });
}

// 工具：按字数线性切分（前端预览时和后端一致）
export function computeTimelineClient(
  text: string,
  shotDurationMs = 4000,
): SubtitleLine[] {
  if (!text || !text.trim()) return [];
  const puncts = new Set([',', '，', '.', '。', '!', '！', '?', '？', ';', '；', ':', '：', '、', '\n']);
  const lines: string[] = [];
  let cur = '';
  for (const ch of text) {
    cur += ch;
    if (puncts.has(ch)) {
      const t = cur.trim();
      if (t) lines.push(t);
      cur = '';
    }
  }
  if (cur.trim()) lines.push(cur.trim());

  const totalChars = lines.reduce((sum, l) => sum + Array.from(l).length, 0);
  if (totalChars === 0) return [];

  const out: SubtitleLine[] = [];
  let cursor = 0;
  for (let i = 0; i < lines.length; i++) {
    const chars = Array.from(lines[i]).length;
    let endMs: number;
    if (i === lines.length - 1) {
      endMs = shotDurationMs;
    } else {
      endMs = cursor + Math.floor((chars * shotDurationMs) / totalChars);
      if (endMs <= cursor) endMs = cursor + 1;
    }
    out.push({ startMs: cursor, endMs, text: lines[i] });
    cursor = endMs;
  }
  return out;
}
