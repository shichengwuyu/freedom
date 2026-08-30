"use client";

import { useEffect, useState } from "react";
import { App, Button, Card, Modal, Progress, Space, Tag, Tooltip } from "antd";
import {
  CheckCircle2,
  CircleDashed,
  Download,
  Film,
  Loader2,
  Play,
  RotateCcw,
  XCircle,
} from "lucide-react";
import {
  type CompositionTask,
  COMPOSITION_STEPS,
  getCompositionTask,
  retryCompositionTask,
  startCompositionTask,
  stopCompositionTask,
} from "@/services/api/novel_composition";

interface Props {
  compositionId: string;
  onRefresh?: () => void;
  onExport?: () => void;
}

const STATUS_COLORS: Record<string, string> = {
  未启动: "default",
  排队中: "processing",
  进行中: "processing",
  成功: "success",
  失败: "error",
  跳过: "default",
  已取消: "warning",
};

export default function NovelCompositionView({
  compositionId,
  onRefresh,
  onExport,
}: Props) {
  const { message } = App.useApp();
  const [modal, modalContextHolder] = Modal.useModal();
  const [task, setTask] = useState<CompositionTask | null>(null);
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const t = await getCompositionTask(compositionId);
      setTask(t);
    } catch (e) {
      console.error("load composition", e);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [compositionId]);

  const handleStart = async () => {
    setBusy(true);
    try {
      await startCompositionTask(compositionId);
      message.success("合成已启动");
      await load();
    } catch (e) {
      message.error("启动失败: " + (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const handleStop = () => {
    modal.confirm({
      title: "停止合成?",
      content: "停止后任务状态变为「已取消」，已生成的中间文件会保留。",
      onOk: async () => {
        try {
          await stopCompositionTask(compositionId);
          message.success("已停止");
          await load();
        } catch (e) {
          message.error("停止失败: " + (e as Error).message);
        }
      },
    });
  };

  const handleRetry = async () => {
    setBusy(true);
    try {
      await retryCompositionTask(compositionId);
      message.success("已重试");
      await load();
    } catch (e) {
      message.error("重试失败: " + (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (!task) {
    return <Card size="small" loading />;
  }

  const stepProgress = (() => {
    try {
      const p = task.progressJson ? JSON.parse(task.progressJson) : {};
      return {
        current: p.currentStep || 0,
        message: p.lastMessage || "",
      };
    } catch {
      return { current: 0, message: "" };
    }
  })();

  const percentByStep = (stepIdx: number) => {
    if (task.status === "成功") return 100;
    if (task.status === "失败") return 0;
    if (stepIdx < stepProgress.current) return 100;
    if (stepIdx === stepProgress.current) return 60;
    return 0;
  };

  return (
    <>
    {modalContextHolder}
    <Card
      size="small"
      title={
        <Space>
          <Film className="w-4 h-4" />
          <span>成片合成</span>
          <Tag color={STATUS_COLORS[task.status] || "default"}>{task.status}</Tag>
          {task.status === "进行中" && stepProgress.message && (
            <span className="text-xs text-neutral-500">{stepProgress.message}</span>
          )}
        </Space>
      }
      extra={
        <Space>
          {task.status === "未启动" && (
            <Button
              type="primary"
              icon={<Play className="w-3.5 h-3.5" />}
              loading={busy}
              onClick={handleStart}
            >
              开始合成
            </Button>
          )}
          {(task.status === "排队中" || task.status === "进行中") && (
            <Button
              danger
              icon={<XCircle className="w-3.5 h-3.5" />}
              onClick={handleStop}
            >
              停止
            </Button>
          )}
          {task.status === "失败" && (
            <Button
              type="primary"
              icon={<RotateCcw className="w-3.5 h-3.5" />}
              loading={busy}
              onClick={handleRetry}
            >
              重试
            </Button>
          )}
          {task.status === "已取消" && (
            <Button
              type="primary"
              icon={<Play className="w-3.5 h-3.5" />}
              loading={busy}
              onClick={handleStart}
            >
              重启
            </Button>
          )}
          {task.status === "成功" && task.outputUrl && (
            <Button
              type="primary"
              icon={<Download className="w-3.5 h-3.5" />}
              href={task.outputUrl}
              download
            >
              下载 mp4
            </Button>
          )}
          {task.status === "成功" && onExport && (
            <Button onClick={onExport}>导出</Button>
          )}
          {onRefresh && (
            <Button
              size="small"
              icon={<RotateCcw className="w-3.5 h-3.5" />}
              onClick={onRefresh}
            />
          )}
        </Space>
      }
    >
      <div className="space-y-2">
        {COMPOSITION_STEPS.map((s) => {
          const isDone = percentByStep(s.idx) === 100;
          const isActive = percentByStep(s.idx) > 0 && !isDone;
          const Icon = isDone
            ? CheckCircle2
            : isActive
            ? Loader2
            : CircleDashed;
          const color = isDone
            ? "text-green-500"
            : isActive
            ? "text-blue-500 animate-spin"
            : "text-neutral-300";
          return (
            <div key={s.idx} className="flex items-center gap-2">
              <Icon className={`w-4 h-4 ${color}`} />
              <div className="text-sm flex-1">
                {s.idx}. {s.label}
              </div>
              <div className="w-32">
                <Progress
                  percent={percentByStep(s.idx)}
                  size="small"
                  status={
                    task.status === "失败" && isActive
                      ? "exception"
                      : isDone
                      ? "success"
                      : isActive
                      ? "active"
                      : "normal"
                  }
                  showInfo={false}
                />
              </div>
            </div>
          );
        })}
      </div>

      {task.error && (
        <Tooltip title={task.error}>
          <div className="text-xs text-red-500 mt-2 truncate">
            错误: {task.error}
          </div>
        </Tooltip>
      )}
      {task.stderrTail && (
        <Tooltip title={task.stderrTail}>
          <div className="text-xs text-neutral-400 mt-1 truncate">
            stderr: {task.stderrTail}
          </div>
        </Tooltip>
      )}
    </Card>
    </>
  );
}
