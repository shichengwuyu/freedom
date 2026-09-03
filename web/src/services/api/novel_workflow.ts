// novel-workflow v2: 工作流编排层 API client
// 后端路由: /api/v1/novel/workflows/*

import { apiGet, apiPost } from './request';

export type NovelWorkflowMode = 'auto' | 'manual' | 'quick' | 'custom';

export type NovelNodeStatus =
  | '未启动'
  | '排队中'
  | '进行中'
  | '成功'
  | '失败'
  | '跳过'
  | '已取消';

export interface NovelWorkflowRun {
  id: string;
  userId: string;
  userGroupCode: string;
  projectId: string;
  mode: NovelWorkflowMode;
  overallStatus:
    | '未启动'
    | '进行中'
    | '已完成'
    | '部分失败'
    | '已停止';
  configJson: string;
  totalNodes: number;
  successNodes: number;
  failedNodes: number;
  skippedNodes: number;
  canceledNodes: number;
  pendingNodes: number;
  lastOutputUrl: string;
  lastOutputAt: string;
  error: string;
  result: string;
  createdAt: string;
  updatedAt: string;
  startedAt: string;
  completedAt: string;
}

export interface NovelWorkflowNode {
  id: string;
  runId: string;
  userId: string;
  projectId: string;
  nodeId: string;
  layer: 'input' | 'script' | 'asset' | 'shot' | 'post';
  nodeKind: string;
  nodeTitle: string;
  shotId: string;
  shotIndex: number;
  dependsOnJson: string;
  status: NovelNodeStatus;
  progress: number;
  stepMessage: string;
  outputUrl: string;
  genericTaskId: string;
  error: string;
  createdAt: string;
  updatedAt: string;
  startedAt: string;
  completedAt: string;
}

// POST /api/v1/novel/workflows
export async function createNovelWorkflowRun(req: {
  projectId: string;
  mode: NovelWorkflowMode;
  shotIds: string[];
  configJson?: string;
}): Promise<NovelWorkflowRun> {
  const data = await apiPost<NovelWorkflowRun>('/api/v1/novel/workflows', req);
  return data;
}

// GET /api/v1/novel/workflows?projectId=...
export async function listNovelWorkflowRuns(
  projectId: string,
  page = 1,
  pageSize = 20,
): Promise<{
  runs: NovelWorkflowRun[];
  total: number;
  page: number;
  pageSize: number;
}> {
  return apiGet<{
    runs: NovelWorkflowRun[];
    total: number;
    page: number;
    pageSize: number;
  }>('/api/v1/novel/workflows', { projectId, page, pageSize });
}

// GET /api/v1/novel/workflows/:id
export async function getNovelWorkflowRun(runId: string): Promise<{
  run: NovelWorkflowRun;
  nodes: NovelWorkflowNode[];
}> {
  return apiGet<{ run: NovelWorkflowRun; nodes: NovelWorkflowNode[] }>(
    `/api/v1/novel/workflows/${runId}`,
  );
}

// POST /api/v1/novel/workflows/:id/start
export async function startNovelWorkflowRun(runId: string): Promise<NovelWorkflowRun> {
  return apiPost<NovelWorkflowRun>(
    `/api/v1/novel/workflows/${runId}/start`,
  );
}

// POST /api/v1/novel/workflows/:id/nodes/:nodeId/start
export async function startNovelWorkflowNode(
  runId: string,
  nodeId: string,
): Promise<void> {
  await apiPost<void>(
    `/api/v1/novel/workflows/${runId}/nodes/${nodeId}/start`,
  );
}

// POST /api/v1/novel/workflows/:id/nodes/:nodeId/cancel
export async function cancelNovelWorkflowNode(
  runId: string,
  nodeId: string,
): Promise<void> {
  await apiPost<void>(
    `/api/v1/novel/workflows/${runId}/nodes/${nodeId}/cancel`,
  );
}

// POST /api/v1/novel/workflows/:id/nodes/:nodeId/retry
export async function retryNovelWorkflowNode(
  runId: string,
  nodeId: string,
): Promise<void> {
  await apiPost<void>(
    `/api/v1/novel/workflows/${runId}/nodes/${nodeId}/retry`,
  );
}
