"use client";

import { useEffect, useState } from "react";
import { App, Button, Card, Empty, Slider, Space, Tag, Tooltip, Upload } from "antd";
import { Music, Pause, Play, Upload as UploadIcon, Volume2 } from "lucide-react";
import {
  type BgmPreset,
  listBgmPresets,
  uploadBgmCustom,
} from "@/services/api/bgm";

interface Props {
  projectId: string;
  value?: { presetId?: string; customId?: string };
  onChange?: (v: {
    presetId?: string;
    customId?: string;
    volume: number;
    fadeInMs: number;
    fadeOutMs: number;
  }) => void;
}

export default function NovelBgmPicker({ projectId, value, onChange }: Props) {
  const { message } = App.useApp();
  const [presets, setPresets] = useState<BgmPreset[]>([]);
  const [selectedId, setSelectedId] = useState<string | undefined>(value?.presetId);
  const [playingId, setPlayingId] = useState<string | null>(null);
  const [volume, setVolume] = useState(0.3);
  const [fadeIn, setFadeIn] = useState(0);
  const [fadeOut, setFadeOut] = useState(0);
  const [audio, setAudio] = useState<HTMLAudioElement | null>(null);

  useEffect(() => {
    listBgmPresets()
      .then(setPresets)
      .catch((e) => console.error("load bgm presets", e));
  }, []);

  useEffect(() => {
    return () => {
      if (audio) {
        audio.pause();
      }
    };
  }, [audio]);

  const handlePreview = (id: string, url: string) => {
    if (playingId === id && audio) {
      audio.pause();
      setPlayingId(null);
      return;
    }
    if (audio) audio.pause();
    const a = new Audio(url);
    a.play().catch((e) => message.error("播放失败: " + e.message));
    a.onended = () => setPlayingId(null);
    setAudio(a);
    setPlayingId(id);
  };

  const handleSelect = (id: string) => {
    setSelectedId(id);
    onChange?.({ presetId: id, volume, fadeInMs: fadeIn, fadeOutMs: fadeOut });
  };

  const handleUpload = async (file: File) => {
    try {
      const bc = await uploadBgmCustom({
        projectId,
        title: file.name.replace(/\.[^.]+$/, ""),
        file,
      });
      message.success(`已上传 BGM: ${bc.title}`);
    } catch (e) {
      message.error("上传失败: " + (e as Error).message);
    }
    return false;
  };

  const handleParamChange = (v: number, fi: number, fo: number) => {
    setVolume(v);
    setFadeIn(fi);
    setFadeOut(fo);
    if (selectedId) {
      onChange?.({
        presetId: selectedId,
        volume: v,
        fadeInMs: fi,
        fadeOutMs: fo,
      });
    }
  };

  return (
    <Card
      size="small"
      title={
        <Space>
          <Music className="w-4 h-4" />
          <span>BGM 选曲</span>
          {selectedId && <Tag color="blue">已选</Tag>}
        </Space>
      }
      extra={
        <Upload
          accept="audio/mpeg,audio/wav"
          showUploadList={false}
          beforeUpload={handleUpload}
        >
          <Button size="small" icon={<UploadIcon className="w-3.5 h-3.5" />}>
            上传自定义
          </Button>
        </Upload>
      }
    >
      {presets.length === 0 ? (
        <Empty description="暂无 BGM 预设（manifest 未配置或 mp3 缺失）" />
      ) : (
        <div className="space-y-2 max-h-72 overflow-y-auto">
          {presets.map((p) => (
            <div
              key={p.id}
              onClick={() => p.available && handleSelect(p.id)}
              className={`flex items-center gap-2 px-3 py-2 rounded border cursor-pointer transition-colors ${
                selectedId === p.id
                  ? "border-blue-500 bg-blue-50 dark:bg-blue-950"
                  : "border-neutral-200 dark:border-neutral-800 hover:border-neutral-300"
              } ${!p.available ? "opacity-50" : ""}`}
            >
              <Button
                size="small"
                type="text"
                icon={
                  playingId === p.id ? (
                    <Pause className="w-3.5 h-3.5" />
                  ) : (
                    <Play className="w-3.5 h-3.5" />
                  )
                }
                onClick={(e) => {
                  e.stopPropagation();
                  if (p.available && p.availablePath)
                    handlePreview(p.id, p.availablePath);
                }}
                disabled={!p.available}
              />
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate">{p.title}</div>
                <div className="text-xs text-neutral-400 truncate">{p.description}</div>
                <div className="flex gap-1 mt-0.5 flex-wrap">
                  {p.tags.map((t) => (
                    <Tag key={t} color="default" className="text-[10px] py-0">
                      {t}
                    </Tag>
                  ))}
                </div>
              </div>
              {!p.available && (
                <Tooltip title="mp3 文件缺失">
                  <Tag color="warning">不可用</Tag>
                </Tooltip>
              )}
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 pt-3 border-t border-neutral-200 dark:border-neutral-800 space-y-2">
        <div className="flex items-center gap-2">
          <Volume2 className="w-3.5 h-3.5 text-neutral-500" />
          <span className="text-xs text-neutral-500 w-12">音量</span>
          <Slider
            className="flex-1"
            min={0}
            max={1}
            step={0.05}
            value={volume}
            onChange={(v) => handleParamChange(v, fadeIn, fadeOut)}
          />
          <span className="text-xs text-neutral-500 w-10 text-right">
            {Math.round(volume * 100)}%
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-neutral-500 w-12">淡入</span>
          <Slider
            className="flex-1"
            min={0}
            max={10000}
            step={100}
            value={fadeIn}
            onChange={(v) => handleParamChange(volume, v as number, fadeOut)}
          />
          <span className="text-xs text-neutral-500 w-12 text-right">
            {(fadeIn / 1000).toFixed(1)}s
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-neutral-500 w-12">淡出</span>
          <Slider
            className="flex-1"
            min={0}
            max={10000}
            step={100}
            value={fadeOut}
            onChange={(v) => handleParamChange(volume, fadeIn, v as number)}
          />
          <span className="text-xs text-neutral-500 w-12 text-right">
            {(fadeOut / 1000).toFixed(1)}s
          </span>
        </div>
      </div>
    </Card>
  );
}
