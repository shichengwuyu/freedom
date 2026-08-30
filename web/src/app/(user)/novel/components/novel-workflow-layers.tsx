"use client";

import { useState } from "react";
import { Steps, Badge, Button, Space, Tooltip, Empty } from "antd";
import {
  CheckCircle2,
  CircleDashed,
  CircleSlash,
  Loader2,
  PauseCircle,
  XCircle,
  ChevronDown,
  ChevronRight,
  PlayCircle,
  RotateCcw,
  StopCircle,
} from "lucide-react";
import {
  type NovelWorkflowNode,
  type NovelWorkflowRun,
  startNovelWorkflowNode,
  cancelNovelWorkflowNode,
  retryNovelWorkflowNode,
} from "@/services/api/novel_workflow";

type Status = NovelWorkflowNode["status"];

const LAYER_DEFS = [
  { key: "input", title: "输入", desc: "小说文本" },
  { key: "script", title: "剧本", desc: "分镜剧本" },
  { key: "asset", title: "资产", desc: "角色/场景/道具" },
  { key: "shot", title: "镜头", desc: "视频/配音/字幕" },
  { key: "post", title: "后期", desc: "BGM/合成/导出" },
] as const;

const KIND_LABELS: Record<string, string> = {
  script: "剧本",
  storyboard: "分镜剧本",
  character: "角色",
  scene: "场景",
  prop: "道具",
  "shot-video": "镜头视频",
  "shot-dubbing": "配音",
  "shot-subtitle": "字幕",
  "bgm-pick": "BGM",
  composition: "成片合成",
  export: "导出",
};

const STATUS_META: Record<
  Status,
  {
    textColor: string;
    bgColor: string;
    icon: React.ComponentType<{ className?: string }>;
    label: string;
    antStatus: "wait" | "process" | "finish" | "error" | "default";
  }
> = {
  未启动: { textColor: "text-neutral-400", bgColor: "bg-neutral-100", icon: CircleDashed, label: "未启动", antStatus: "wait" },
  排队中: { textColor: "text-blue-600", bgColor: "bg-blue-50", icon: Loader2, label: "排队中", antStatus: "process" },
  进行中: { textColor: "text-blue-600", bgColor: "bg-blue-50", icon: Loader2, label: "进行中", antStatus: "process" },
  成功: { textColor: "text-green-600", bgColor: "bg-green-50", icon: CheckCircle2, label: "成功", antStatus: "finish" },
  失败: { textColor: "text-red-600", bgColor: "bg-red-50", icon: XCircle, label: "失败", antStatus: "error" },
  跳过: { textColor: "text-neutral-400", bgColor: "bg-neutral-100", icon: CircleSlash, label: "跳过", antStatus: "default" },
  已取消: { textColor: "text-yellow-600", bgColor: "bg-yellow-50", icon: PauseCircle, label: "已取消", antStatus: "default" },
};

interface Props {
  run: NovelWorkflowRun;
  nodes: NovelWorkflowNode[];
  projectId: string;
  onRefresh?: () => void;
  onMessage?: (msg: { type: "success" | "error"; text: string }) => void;
}

export default function NovelWorkflowLayers({
  run,
  nodes,
  projectId,
  onRefresh,
  onMessage,
}: Props) {
  const [expandedLayer, setExpandedLayer] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const byLayer: Record<string, NovelWorkflowNode[]> = {};
  for (const n of nodes) {
    if (!byLayer[n.layer]) byLayer[n.layer] = [];
    byLayer[n.layer].push(n);
  }

  const currentLayer = (() => {
    for (const def of LAYER_DEFS) {
      const ns = byLayer[def.key] || [];
      if (ns.some((n) => n.status === "进行中" || n.status === "排队中")) {
        return def.key;
      }
    }
    for (let i = LAYER_DEFS.length - 1; i >= 0; i--) {
      const def = LAYER_DEFS[i];
      const ns = byLayer[def.key] || [];
      if (ns.length > 0 && ns.every((n) => n.status === "成功" || n.status === "跳过")) {
        return def.key;
      }
    }
    return "input";
  })();

  const currentStepIndex = LAYER_DEFS.findIndex((d) => d.key === currentLayer);

  const handleNodeAction = async (
    action: "start" | "cancel" | "retry",
    node: NovelWorkflowNode,
  ) => {
    setBusy(node.id);
    try {
      if (action === "start") {
        await startNovelWorkflowNode(run.id, node.nodeId);
        onMessage?.({ type: "success", text: `已启动 ${KIND_LABELS[node.nodeKind] || node.nodeId}` });
      } else if (action === "cancel") {
        await cancelNovelWorkflowNode(run.id, node.nodeId);
        onMessage?.({ type: "success", text: `已取消 ${KIND_LABELS[node.nodeKind] || node.nodeId}` });
      } else {
        await retryNovelWorkflowNode(run.id, node.nodeId);
        onMessage?.({ type: "success", text: `已重试 ${KIND_LABELS[node.nodeKind] || node.nodeId}` });
      }
      onRefresh?.();
    } catch (e) {
      onMessage?.({ type: "error", text: `${action} 失败: ${(e as Error).message}` });
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="bg-white dark:bg-neutral-900 rounded-lg border border-neutral-200 dark:border-neutral-800 p-4">
      <div className="flex items-center justify-between mb-4">
        <div>
          <div className="text-sm font-medium text-neutral-700 dark:text-neutral-200">
            novel-workflow v2 流水线
          </div>
          <div className="text-xs text-neutral-400 mt-0.5">
            总体状态: {run.overallStatus} · {run.successNodes} 成功 / {run.failedNodes} 失败 / {run.pendingNodes} 待跑
          </div>
        </div>
        {onRefresh && (
          <Button
            size="small"
            icon={<RotateCcw className="w-3.5 h-3.5" />}
            onClick={onRefresh}
          >
            刷新
          </Button>
        )}
      </div>

      <Steps
        current={currentStepIndex >= 0 ? currentStepIndex : 0}
        size="small"
        className="mb-4"
        items={LAYER_DEFS.map((def) => {
          const ns = byLayer[def.key] || [];
          const counts = ns.reduce(
            (acc, n) => {
              acc[n.status] = (acc[n.status] || 0) + 1;
              return acc;
            },
            {} as Record<string, number>,
          );
          return {
            title: (
              <button
                type="button"
                onClick={() =>
                  setExpandedLayer(expandedLayer === def.key ? null : def.key)
                }
                className="inline-flex items-center gap-1 text-left"
              >
                {expandedLayer === def.key ? (
                  <ChevronDown className="w-3.5 h-3.5 text-neutral-400" />
                ) : (
                  <ChevronRight className="w-3.5 h-3.5 text-neutral-400" />
                )}
                {def.title}
              </button>
            ),
            description: (
              <Space size={4} wrap>
                {ns.length === 0 ? (
                  <span className="text-xs text-neutral-400">{def.desc}</span>
                ) : (
                  <>
                    {counts["进行中"] + counts["排队中"] > 0 && (
                      <Badge
                        status="processing"
                        text={`${counts["进行中"] + counts["排队中"]} 进行`}
                      />
                    )}
                    {counts["成功"] > 0 && (
                      <Badge status="success" text={`${counts["成功"]} 成功`} />
                    )}
                    {counts["失败"] > 0 && (
                      <Badge status="error" text={`${counts["失败"]} 失败`} />
                    )}
                    {counts["跳过"] > 0 && (
                      <Badge status="default" text={`${counts["跳过"]} 跳过`} />
                    )}
                    {counts["已取消"] > 0 && (
                      <Badge status="warning" text={`${counts["已取消"]} 取消`} />
                    )}
                  </>
                )}
              </Space>
            ),
          };
        })}
      />

      {expandedLayer && (
        <div className="border-t border-neutral-200 dark:border-neutral-800 pt-3 mt-2">
          {(byLayer[expandedLayer] || []).length === 0 ? (
            <Empty description="该层暂无节点" />
          ) : (
            <div className="space-y-2">
              {byLayer[expandedLayer].map((n) => {
                const meta = STATUS_META[n.status] || STATUS_META["未启动"];
                const Icon = meta.icon;
                return (
                  <div
                    key={n.id}
                    className="flex items-center gap-3 px-3 py-2 rounded border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-950"
                  >
                    <div className={`w-6 h-6 rounded flex items-center justify-center ${meta.bgColor}`}>
                      <Icon
                        className={`w-4 h-4 ${meta.textColor} shrink-0 ${
                          n.status === "进行中" || n.status === "排队中" ? "animate-spin" : ""
                        }`}
                      />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium truncate">
                        {KIND_LABELS[n.nodeKind] || n.nodeKind}
                        <span className="text-xs text-neutral-400 ml-2">#{n.nodeId}</span>
                      </div>
                      {n.error && (
                        <Tooltip title={n.error}>
                          <div className="text-xs text-red-500 truncate">{n.error}</div>
                        </Tooltip>
                      )}
                    </div>
                    <Space size={4}>
                      {n.status === "未启动" && (
                        <Button
                          size="small"
                          icon={<PlayCircle className="w-3.5 h-3.5" />}
                          loading={busy === n.id}
                          onClick={() => handleNodeAction("start", n)}
                        >
                          开始
                        </Button>
                      )}
                      {(n.status === "排队中" || n.status === "进行中") && (
                        <Button
                          size="small"
                          danger
                          icon={<StopCircle className="w-3.5 h-3.5" />}
                          loading={busy === n.id}
                          onClick={() => handleNodeAction("cancel", n)}
                        >
                          停止
                        </Button>
                      )}
                      {n.status === "失败" && (
                        <Button
                          size="small"
                          icon={<RotateCcw className="w-3.5 h-3.5" />}
                          loading={busy === n.id}
                          onClick={() => handleNodeAction("retry", n)}
                        >
                          重试
                        </Button>
                      )}
                      {n.status === "已取消" && (
                        <Button
                          size="small"
                          icon={<RotateCcw className="w-3.5 h-3.5" />}
                          loading={busy === n.id}
                          onClick={() => handleNodeAction("start", n)}
                        >
                          重启
                        </Button>
                      )}
                    </Space>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
