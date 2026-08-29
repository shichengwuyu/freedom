import { AlertCircle, Check, Copy, Music2, Play, RefreshCw, Trash2, MoveUp, MoveDown, ArrowLeft, ArrowRight, Camera, Video, LoaderCircle, Download, FileText, Layers } from "lucide-react";
import { Button, InputNumber, Select, Tooltip } from "antd";
import { useRef, useState } from "react";
import type { Asset, Shot } from "../types";
import { DetailMeta } from "./config-label";

// ─── Video Shot Detail（详情弹窗内容，保留全部操作）───

export function VideoShotDetail({
    shot, index, total, assets, supportsFrameRefs,
    curAspectRatio, curResolution, curSize, curVideoModel,
    onGenerate, onRetry, onDelete, onMoveUp, onMoveDown,
    onDurationChange, onPromptChange, onFirstFrameAsset, onLastFrameAsset,
    onSaveFrameAsAsset, onDownload, onCopyPrompt,
    onPrev, onNext, onCaptureCurrentFrame, onExtractAudio,
    videoSystemPrompt, resolvedScript,
}: {
    shot: Shot; index: number; total: number; assets: Asset[]; supportsFrameRefs: boolean;
    curAspectRatio: string; curResolution: string; curSize: string; curVideoModel: string;
    onGenerate: () => void; onRetry: () => void; onDelete: () => void;
    onMoveUp: () => void; onMoveDown: () => void;
    onDurationChange: (d: number) => void; onPromptChange: (p: string) => void;
    onFirstFrameAsset: (id: string) => void; onLastFrameAsset: (id: string) => void;
    onSaveFrameAsAsset: (type: "first" | "last") => void;
    onDownload: () => void; onCopyPrompt: () => void;
    onPrev?: () => void; onNext?: () => void;
    /** 当前帧回调：参数为播放器的实际播放位置（秒） */
    onCaptureCurrentFrame?: (seekSeconds?: number) => void; onExtractAudio?: () => void;
    /** 视频系统提示词（配置的videoPrompt，用于生成视频的system prompt） */
    videoSystemPrompt?: string;
    /** 解析@alias后的分镜脚本内容（已替换角色/场景引用） */
    resolvedScript?: string;
}) {
    const videoRef = useRef<HTMLVideoElement>(null);
    const [showFullPrompt, setShowFullPrompt] = useState(false);
    return (
        <div className="space-y-3 py-1">
            {/* Video preview */}
            {shot.status === "success" && shot.videoUrl ? (
                <div className="relative overflow-hidden rounded-lg bg-black">
                    <video ref={videoRef} src={shot.videoUrl} controls className="w-full" />
                </div>
            ) : shot.status === "generating" ? (
                <div className="flex aspect-video items-center justify-center rounded-lg bg-stone-100 dark:bg-stone-800">
                    <span className="flex flex-col items-center gap-2 text-stone-500">
                        <LoaderCircle className="size-8 animate-spin" />
                        <span className="text-xs">{shot.progress !== undefined ? `生成中 ${shot.progress}%` : "生成中"}</span>
                    </span>
                </div>
            ) : (
                <div className="flex aspect-video items-center justify-center rounded-lg bg-stone-100 dark:bg-stone-800">
                    <Video className="size-8 text-stone-300" />
                </div>
            )}

            {shot.status === "failed" && shot.error && (
                <div className="rounded bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-950/30 dark:text-red-400">{shot.error}</div>
            )}

            {/* 视频信息：分辨率 / 比例 / 尺寸 / 模型 / 时长 / 序号 / 状态（已生成用快照值，未生成用当前配置） */}
            <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 rounded-lg bg-stone-50 px-3 py-2.5 text-xs dark:bg-stone-800/60">
                <DetailMeta label="镜号" value={`第 ${index + 1} / ${total} 镜`} />
                <DetailMeta label="状态" value={shot.status === "success" ? "已生成" : shot.status === "generating" ? `生成中${shot.progress !== undefined ? ` ${shot.progress}%` : ""}` : shot.status === "failed" ? "失败" : "未生成"} />
                <DetailMeta label="画面比例" value={shot.aspectRatio || curAspectRatio} />
                <DetailMeta label="分辨率" value={shot.resolution || curResolution} />
                <DetailMeta label="像素尺寸" value={shot.size || curSize} />
                <DetailMeta label="时长" value={`${shot.duration} 秒`} />
                <DetailMeta label="视频模型" value={shot.videoModel || curVideoModel || "—"} span />
            </div>

            {/* 实际提示词（可见 + 可复制）：展示完整的视频生成提示词 */}
            <div>
                <div className="mb-1 flex items-center justify-between">
                    <span className="text-xs font-medium text-stone-500">实际提示词{shot.customPrompt ? "（自定义覆盖）" : "（分镜脚本 + 绑定图片 + 视频提示词）"}</span>
                    <div className="flex gap-1">
                        <Button size="small" type="text" onClick={() => setShowFullPrompt(!showFullPrompt)} className="h-6 px-1 text-[11px] text-stone-400">
                            {showFullPrompt ? "收起详情" : "展开详情"}
                        </Button>
                        <Button size="small" type="text" icon={<Copy className="size-3" />} onClick={onCopyPrompt} className="h-6 px-1 text-[11px] text-stone-400">复制</Button>
                    </div>
                </div>

                {/* 用户提示词（分镜脚本内容，已解析@alias引用） */}
                <div className="mb-2 rounded-lg border border-stone-200 bg-white dark:border-stone-700 dark:bg-stone-900">
                    <div className="flex items-center gap-1 border-b border-stone-100 px-3 py-1.5 text-[10px] font-medium text-stone-400 dark:border-stone-700">
                        <FileText className="size-3" />
                        <span>分镜脚本内容{shot.customPrompt ? "（已用自定义提示词覆盖）" : "（已解析角色/场景引用）"}</span>
                    </div>
                    <p className="thin-scrollbar max-h-32 overflow-y-auto whitespace-pre-wrap px-3 py-2 text-xs leading-5 text-stone-700 dark:text-stone-200">
                        {shot.customPrompt || resolvedScript || shot.content || "（空）"}
                    </p>
                </div>

                {/* 视频系统提示词（展开时显示） */}
                {showFullPrompt && videoSystemPrompt && (
                    <div className="mb-2 rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950/20">
                        <div className="flex items-center gap-1 border-b border-amber-100 px-3 py-1.5 text-[10px] font-medium text-amber-600 dark:border-amber-800 dark:text-amber-500">
                            <Layers className="size-3" />
                            <span>视频系统提示词（videoPrompt · 指导视频模型如何理解分镜脚本）</span>
                        </div>
                        <p className="thin-scrollbar max-h-40 overflow-y-auto whitespace-pre-wrap px-3 py-2 text-xs leading-5 text-amber-900 dark:text-amber-200">
                            {videoSystemPrompt}
                        </p>
                    </div>
                )}

                {/* 完整组合提示词（API实际接收的格式） */}
                {showFullPrompt && (
                    <div className="rounded-lg border border-stone-200 bg-stone-50 dark:border-stone-700 dark:bg-stone-800/60">
                        <div className="flex items-center gap-1 border-b border-stone-100 px-3 py-1.5 text-[10px] font-medium text-stone-500 dark:border-stone-700">
                            <span>完整API请求格式（systemPrompt + prompt）</span>
                        </div>
                        <pre className="thin-scrollbar max-h-40 overflow-y-auto whitespace-pre-wrap break-all px-3 py-2 text-[10px] leading-4 text-stone-500 dark:text-stone-400">
{`[系统提示词]\n${videoSystemPrompt || "(未设置)"}\n\n[用户提示词]\n${shot.customPrompt || resolvedScript || shot.content || "(空)"}`}
                        </pre>
                    </div>
                )}

                {/* Asset references - 绑定的图片 */}
                <div className="mt-2">
                    <div className="mb-1 flex items-center gap-1 text-[10px] font-medium text-stone-500">
                        <span>绑定图片（{shot.referencedAssetIds.length} 个）· 用于视频生成的参考图</span>
                    </div>
                    {shot.referencedAssetIds.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                            {shot.referencedAssetIds.map((aid) => {
                                const a = assets.find((x) => x.id === aid);
                                return a ? (
                                    <span key={aid} className="inline-flex items-center gap-1 rounded border border-stone-200 bg-stone-50 px-1.5 py-0.5 text-[11px] text-stone-600 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-400">
                                        <img src={a.dataUrl} alt={a.name} className="size-3.5 rounded" />{a.alias}
                                    </span>
                                ) : null;
                            })}
                        </div>
                    ) : (
                        <span className="text-[10px] text-stone-400">未绑定图片（无参考图模式生成）</span>
                    )}
                </div>
            </div>

            {/* Frame references */}
            <div className="grid grid-cols-2 gap-3">
                <div>
                    <div className="mb-1 flex items-center gap-1 text-xs text-stone-500">
                        首帧参考
                        {!supportsFrameRefs && <Tooltip title="当前模型不支持首帧参考"><AlertCircle className="size-3 text-stone-400" /></Tooltip>}
                    </div>
                    <Select size="small" placeholder="选择资产" className="w-full" allowClear
                        value={shot.firstFrameAssetId} onChange={onFirstFrameAsset}
                        options={assets.map((a) => ({ label: a.alias, value: a.id }))}
                        disabled={!supportsFrameRefs} />
                </div>
                <div>
                    <div className="mb-1 flex items-center gap-1 text-xs text-stone-500">
                        尾帧参考
                        {!supportsFrameRefs && <Tooltip title="当前模型不支持尾帧参考"><AlertCircle className="size-3 text-stone-400" /></Tooltip>}
                    </div>
                    <Select size="small" placeholder="选择资产" className="w-full" allowClear
                        value={shot.lastFrameAssetId} onChange={onLastFrameAsset}
                        options={assets.map((a) => ({ label: a.alias, value: a.id }))}
                        disabled={!supportsFrameRefs} />
                </div>
            </div>

            {/* Duration */}
            <div className="flex items-center gap-2">
                <span className="text-xs text-stone-500">时长：</span>
                <InputNumber size="small" min={5} max={60} value={shot.duration} onChange={(v) => onDurationChange(v || 15)} addonAfter="秒" className="w-24" />
            </div>

            {/* Custom prompt */}
            <div>
                <div className="mb-1 text-xs text-stone-500">分镜提示词（覆盖默认）</div>
                <textarea value={shot.customPrompt || ""} onChange={(e) => onPromptChange(e.target.value)}
                    className="w-full resize-none rounded border border-stone-200 bg-stone-50 px-2 py-1.5 text-xs outline-none dark:border-stone-700 dark:bg-stone-800 dark:text-stone-300"
                    rows={2} placeholder="不填则使用全局提示词" />
            </div>

            {/* Action buttons */}
            <div className="flex flex-wrap items-center gap-1.5 border-t border-stone-100 pt-3 dark:border-stone-800">
                <Tooltip title="上一个视频"><Button size="small" type="text" icon={<ArrowLeft className="size-3.5" />} onClick={onPrev} disabled={index === 0} className="h-7 w-7 p-0 text-stone-400" /></Tooltip>
                <Tooltip title="下一个视频"><Button size="small" type="text" icon={<ArrowRight className="size-3.5" />} onClick={onNext} disabled={index === total - 1} className="h-7 w-7 p-0 text-stone-400" /></Tooltip>
                <Tooltip title="上移"><Button size="small" type="text" icon={<MoveUp className="size-3.5" />} onClick={onMoveUp} disabled={index === 0} className="h-7 w-7 p-0 text-stone-400" /></Tooltip>
                <Tooltip title="下移"><Button size="small" type="text" icon={<MoveDown className="size-3.5" />} onClick={onMoveDown} disabled={index === total - 1} className="h-7 w-7 p-0 text-stone-400" /></Tooltip>
                <Tooltip title="删除"><Button size="small" type="text" icon={<Trash2 className="size-3.5" />} onClick={onDelete} className="h-7 w-7 p-0 text-stone-400 hover:text-red-500" /></Tooltip>
                <div className="flex-1" />
                {(shot.status === "idle" || shot.status === "generating") && (
                    <Button size="small" type="primary" icon={<Play className="size-3" />} onClick={onGenerate} disabled={shot.status === "generating"}
                        className="h-8 rounded-lg text-xs bg-stone-800 hover:bg-stone-700 dark:bg-stone-200 dark:text-stone-900 dark:hover:bg-stone-300">
                        {shot.status === "generating" ? "生成中..." : "生成"}
                    </Button>
                )}
                {shot.status === "failed" && (
                    <Button size="small" icon={<RefreshCw className="size-3" />} onClick={onRetry} className="h-8 rounded-lg text-xs">重试</Button>
                )}
                {shot.status === "success" && shot.videoUrl && (
                    <>
                        <Tooltip title="截取首帧存为素材"><Button size="small" icon={<Camera className="size-3" />} onClick={() => onSaveFrameAsAsset("first")} className="h-8 rounded-lg text-xs">首帧</Button></Tooltip>
                        <Tooltip title="截取尾帧存为素材"><Button size="small" icon={<Camera className="size-3" />} onClick={() => onSaveFrameAsAsset("last")} className="h-8 rounded-lg text-xs">尾帧</Button></Tooltip>
                        {onCaptureCurrentFrame && <Tooltip title="截取当前播放帧存为素材"><Button size="small" icon={<Camera className="size-3" />} onClick={() => onCaptureCurrentFrame(videoRef.current?.currentTime)} className="h-8 rounded-lg text-xs">当前帧</Button></Tooltip>}
                        {onExtractAudio && <Tooltip title="提取视频音频存为素材"><Button size="small" icon={<Music2 className="size-3" />} onClick={onExtractAudio} className="h-8 rounded-lg text-xs">提取音频</Button></Tooltip>}
                        <Tooltip title="下载视频"><Button size="small" icon={<Download className="size-3" />} onClick={onDownload} className="h-8 rounded-lg text-xs">下载</Button></Tooltip>
                        <Tooltip title="复制提示词"><Button size="small" icon={<Copy className="size-3" />} onClick={onCopyPrompt} className="h-8 rounded-lg text-xs">提示词</Button></Tooltip>
                        <Button size="small" icon={<RefreshCw className="size-3" />} onClick={onRetry} className="h-8 rounded-lg text-xs">重新生成</Button>
                    </>
                )}
            </div>
        </div>
    );
}
