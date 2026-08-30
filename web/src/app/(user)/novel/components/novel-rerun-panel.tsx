"use client";

import { useEffect, useState } from "react";
import { Button, Card, Empty, Modal, Space, Tag, Tooltip, App } from "antd";
import {
  History,
  Play,
  RefreshCw,
  RotateCcw,
  Undo2,
  Video as VideoIcon,
} from "lucide-react";
import {
  type RerunRecord,
  type RerunLayer,
  RERUN_LAYER_LABELS,
  listVersions,
  rerunShotLayer,
  rerunFullLayer,
  rollbackToVersion,
  getLatestVersion,
} from "@/services/api/novel_rerun";

const { confirm } = Modal;

interface Props {
  runId: string;
  projectId: string;
  shotId?: string;
  scope: "shot" | "full";
  layer: RerunLayer;
  // 当 scope=shot 时必传
  onRerunDone?: () => void;
  // 整部合成时需要
  compositionInput?: any;
  // 整部字幕时需要
  subtitleStyle?: any;
}

export default function NovelRerunPanel({
  runId,
  projectId,
  shotId,
  scope,
  layer,
  onRerunDone,
  compositionInput,
  subtitleStyle,
}: Props) {
  const { message } = App.useApp();
  const [versions, setVersions] = useState<RerunRecord[]>([]);
  const [latest, setLatest] = useState<RerunRecord | null>(null);
  const [busy, setBusy] = useState(false);
  const [showHistory, setShowHistory] = useState(false);

  const loadVersions = async () => {
    try {
      const list = await listVersions({ projectId, scope, layer, shotId });
      setVersions(list);
      setLatest(list[0] || null);
    } catch (e) {
      console.error("load versions failed", e);
    }
  };

  useEffect(() => {
    loadVersions();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId, projectId, shotId, scope, layer]);

  const handleRerun = async () => {
    setBusy(true);
    try {
      if (scope === "shot") {
        if (!shotId) {
          message.error("缺少 shotId");
          return;
        }
        const rec = await rerunShotLayer({
          runId,
          projectId,
          shotId,
          layer: layer as "video" | "dubbing" | "subtitle",
        });
        message.success(`已重做 v${rec.version}`);
      } else {
        const res = await rerunFullLayer({
          runId,
          projectId,
          layer: layer as "subtitle" | "composition" | "full",
          compositionInput,
          subtitleStyle,
        });
        message.success(`已重做 v${res.record.version}`);
      }
      await loadVersions();
      onRerunDone?.();
    } catch (e) {
      message.error("重做失败: " + (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const handleRollback = (record: RerunRecord) => {
    confirm({
      title: `回滚到 v${record.version}?`,
      content: `将采用 ${record.completedAt} 的输出版本。`,
      okText: "回滚",
      cancelText: "取消",
      onOk: async () => {
        try {
          await rollbackToVersion(record.id);
          message.success(`已回滚到 v${record.version}`);
          await loadVersions();
        } catch (e) {
          message.error("回滚失败: " + (e as Error).message);
        }
      },
    });
  };

  const layerLabel = RERUN_LAYER_LABELS[layer] || layer;
  const scopeLabel = scope === "shot" ? `分镜 ${shotId}` : "整部成片";

  return (
    <Card
      size="small"
      title={
        <Space>
          <RotateCcw className="w-4 h-4" />
          <span>重做 · {layerLabel}</span>
          <Tag color="blue">{scopeLabel}</Tag>
          {latest && (
            <Tag color={latest.status === "success" ? "green" : "red"}>
              当前 v{latest.version} · {latest.status}
            </Tag>
          )}
        </Space>
      }
      extra={
        <Space>
          <Button
            size="small"
            icon={<History className="w-3.5 h-3.5" />}
            onClick={() => setShowHistory(true)}
          >
            历史 ({versions.length})
          </Button>
          <Button
            size="small"
            type="primary"
            icon={
              busy ? (
                <RefreshCw className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <Play className="w-3.5 h-3.5" />
              )
            }
            loading={busy}
            onClick={handleRerun}
          >
            {scope === "shot" ? "重做该分镜" : "重新跑整部"}
          </Button>
        </Space>
      }
    >
      <div className="text-xs text-neutral-500">
        提示: 重做会写入新版本（v{(latest?.version || 0) + 1}），可随时回滚到任意历史版本。
      </div>

      <Modal
        open={showHistory}
        title={`${layerLabel} · 重做历史`}
        width={600}
        onCancel={() => setShowHistory(false)}
        footer={null}
      >
        {versions.length === 0 ? (
          <Empty description="暂无重做历史" />
        ) : (
          <Space direction="vertical" className="w-full" size={4}>
            {versions.map((v) => (
              <div
                key={v.id}
                className="flex items-center gap-2 px-3 py-2 rounded border border-neutral-200 dark:border-neutral-800"
              >
                <Tag color={v.status === "success" ? "green" : v.status === "running" ? "blue" : "red"}>
                  v{v.version}
                </Tag>
                <Tag color="default">{v.status}</Tag>
                <span className="text-xs text-neutral-500 flex-1">
                  {v.completedAt || v.createdAt}
                </span>
                {v.outputUrl && scope === "full" && layer === "composition" && (
                  <Tooltip title={v.outputUrl}>
                    <Button size="small" icon={<VideoIcon className="w-3.5 h-3.5" />} type="link">
                      预览
                    </Button>
                  </Tooltip>
                )}
                <Button
                  size="small"
                  icon={<Undo2 className="w-3.5 h-3.5" />}
                  onClick={() => handleRollback(v)}
                  disabled={v.status !== "success"}
                >
                  采用此版本
                </Button>
              </div>
            ))}
          </Space>
        )}
      </Modal>
    </Card>
  );
}
