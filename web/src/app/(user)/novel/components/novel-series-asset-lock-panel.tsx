"use client";

import { useEffect, useState } from "react";
import { App, Button, Card, Empty, Input, Space, Tag, Tooltip } from "antd";
import { Lock, LockOpen, Package, Save } from "lucide-react";
import {
  type SeriesAssetLockParsed,
  getSeriesAssetLock,
  updateSeriesAssetLock,
  lockSeriesAssetLock,
  parseSeriesAssetLock,
  unlockSeriesAssetLock,
} from "@/services/api/series_asset_lock";

const { TextArea } = Input;

interface Props {
  projectId: string;
  onChange?: (lock: SeriesAssetLockParsed | null) => void;
}

const DEFAULT_STYLE_PROMPT = `古风 3D 渲染, 电影感, 高细节, 强光影, 一致色调`;

export default function NovelSeriesAssetLockPanel({ projectId, onChange }: Props) {
  const { message } = App.useApp();
  const [lock, setLock] = useState<SeriesAssetLockParsed | null>(null);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({
    characterIdsText: "",
    sceneIdsText: "",
    propIdsText: "",
    globalStylePrompt: DEFAULT_STYLE_PROMPT,
  });

  const load = async () => {
    try {
      const raw = await getSeriesAssetLock(projectId);
      const parsed = parseSeriesAssetLock(raw);
      setLock(parsed);
      if (parsed) {
        setForm({
          characterIdsText: parsed.characterIds.join(", "),
          sceneIdsText: parsed.sceneIds.join(", "),
          propIdsText: parsed.propIds.join(", "),
          globalStylePrompt: parsed.globalStylePrompt || DEFAULT_STYLE_PROMPT,
        });
      }
      onChange?.(parsed);
    } catch (e) {
      console.error("load lock", e);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const splitIds = (text: string) =>
    text
      .split(/[,\s]+/)
      .map((s) => s.trim())
      .filter(Boolean);

  const handleSave = async () => {
    setBusy(true);
    try {
      await updateSeriesAssetLock({
        projectId,
        characterIds: splitIds(form.characterIdsText),
        sceneIds: splitIds(form.sceneIdsText),
        propIds: splitIds(form.propIdsText),
        globalStylePrompt: form.globalStylePrompt,
      });
      message.success("主资产包已保存");
      setEditing(false);
      await load();
    } catch (e) {
      message.error("保存失败: " + (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const handleLock = async () => {
    setBusy(true);
    try {
      await lockSeriesAssetLock(projectId);
      message.success("已锁定主资产包");
      await load();
    } catch (e) {
      message.error("锁定失败: " + (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const handleUnlock = async () => {
    setBusy(true);
    try {
      await unlockSeriesAssetLock(projectId);
      message.success("已解锁");
      await load();
    } catch (e) {
      message.error("解锁失败: " + (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (!lock && !editing) {
    return (
      <Card
        size="small"
        title={
          <Space>
            <Package className="w-4 h-4" />
            <span>主资产包 (漫剧一致性)</span>
          </Space>
        }
      >
        <Empty description="未配置主资产包。配置后视频生成会强制从主资产包取参考图，保证漫剧一致性。" />
        <div className="text-center mt-2">
          <Button type="primary" onClick={() => setEditing(true)}>
            配置主资产包
          </Button>
        </div>
      </Card>
    );
  }

  if (editing) {
    return (
      <Card
        size="small"
        title={
          <Space>
            <Package className="w-4 h-4" />
            <span>配置主资产包</span>
          </Space>
        }
        extra={
          <Space>
            <Button onClick={() => setEditing(false)}>取消</Button>
            <Button
              type="primary"
              icon={<Save className="w-3.5 h-3.5" />}
              loading={busy}
              onClick={handleSave}
            >
              保存
            </Button>
          </Space>
        }
      >
        <div className="space-y-3">
          <div>
            <div className="text-xs text-neutral-500 mb-1">角色 ID (逗号分隔)</div>
            <Input
              value={form.characterIdsText}
              onChange={(e) => setForm({ ...form, characterIdsText: e.target.value })}
              placeholder="character-001, character-002"
            />
          </div>
          <div>
            <div className="text-xs text-neutral-500 mb-1">场景 ID</div>
            <Input
              value={form.sceneIdsText}
              onChange={(e) => setForm({ ...form, sceneIdsText: e.target.value })}
              placeholder="scene-classroom, scene-forest"
            />
          </div>
          <div>
            <div className="text-xs text-neutral-500 mb-1">道具 ID</div>
            <Input
              value={form.propIdsText}
              onChange={(e) => setForm({ ...form, propIdsText: e.target.value })}
              placeholder="prop-sword, prop-book"
            />
          </div>
          <div>
            <div className="text-xs text-neutral-500 mb-1">全局色调 / 风格 prompt</div>
            <TextArea
              rows={3}
              value={form.globalStylePrompt}
              onChange={(e) => setForm({ ...form, globalStylePrompt: e.target.value })}
              placeholder="古风 3D 渲染, 电影感, 高细节..."
            />
            <div className="text-xs text-neutral-400 mt-1">
              视频生成时会自动追加到 prompt
            </div>
          </div>
        </div>
      </Card>
    );
  }

  return (
    <Card
      size="small"
      title={
        <Space>
          <Package className="w-4 h-4" />
          <span>主资产包 (漫剧一致性)</span>
          <Tag color={lock?.isLocked ? "red" : "default"}>
            {lock?.isLocked ? "已锁定" : "未锁定"}
          </Tag>
        </Space>
      }
      extra={
        <Space>
          <Button size="small" onClick={() => setEditing(true)}>
            编辑
          </Button>
          {lock?.isLocked ? (
            <Button
              size="small"
              icon={<LockOpen className="w-3.5 h-3.5" />}
              loading={busy}
              onClick={handleUnlock}
            >
              解锁
            </Button>
          ) : (
            <Tooltip title="锁定后视频生成强制从主资产包取参考图">
              <Button
                size="small"
                type="primary"
                icon={<Lock className="w-3.5 h-3.5" />}
                loading={busy}
                onClick={handleLock}
              >
                锁定
              </Button>
            </Tooltip>
          )}
        </Space>
      }
    >
      <div className="text-xs space-y-1">
        <div>
          <Tag>角色</Tag>
          {lock?.characterIds.length ? (
            lock.characterIds.map((id) => <Tag key={id}>{id}</Tag>)
          ) : (
            <span className="text-neutral-400">无</span>
          )}
        </div>
        <div>
          <Tag>场景</Tag>
          {lock?.sceneIds.length ? (
            lock.sceneIds.map((id) => <Tag key={id}>{id}</Tag>)
          ) : (
            <span className="text-neutral-400">无</span>
          )}
        </div>
        <div>
          <Tag>道具</Tag>
          {lock?.propIds.length ? (
            lock.propIds.map((id) => <Tag key={id}>{id}</Tag>)
          ) : (
            <span className="text-neutral-400">无</span>
          )}
        </div>
        <div>
          <Tag>全局色调</Tag>
          <span className="text-neutral-600 dark:text-neutral-300">
            {lock?.globalStylePrompt || (
              <span className="text-neutral-400">未设置</span>
            )}
          </span>
        </div>
      </div>
    </Card>
  );
}
