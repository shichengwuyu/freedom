import { AlertCircle, Check, Download, Copy, Music2, Play, RefreshCw, Trash2, MoveUp, MoveDown, ArrowLeft, ArrowRight, Camera, Video, LoaderCircle } from "lucide-react";
import { Button, InputNumber, Select, Tooltip } from "antd";
import { useRef } from "react";
import type { Asset, Shot } from "../types";
import { DetailMeta } from "./config-label";

// ─── Video Shot Card（右栏卡片，默认收起，只显缩略/状态，点击打开详情弹窗）───

export function VideoShotCard({
    shot, index, layout, compact = false,
    onToggleSelect, onGenerate, onRetry, onOpenDetail, onDelete,
}: {
    shot: Shot; index: number; layout: "list" | "grid"; compact?: boolean;
    onToggleSelect: () => void;
    onGenerate: () => void; onRetry: () => void; onOpenDetail: () => void;
    onDelete: () => void;
}) {
    const statusBadge = (
        <span className={`flex size-5 shrink-0 items-center justify-center rounded text-[10px] font-medium ${
            shot.status === "success" ? "bg-stone-800 text-white dark:bg-stone-200 dark:text-stone-900"
            : shot.status === "generating" ? "bg-stone-400 text-white"
            : shot.status === "failed" ? "bg-red-500 text-white"
            : "bg-stone-200 text-stone-500 dark:bg-stone-700 dark:text-stone-400"
        }`}>
            {shot.status === "success" ? <Check className="size-3" /> : index + 1}
        </span>
    );

    // 紧凑模式：仅缩略图 + 序号/状态角标，点击打开详情弹窗看完整信息（可塞 10-20 个）
    if (compact) {
        return (
            <button onClick={onOpenDetail} title={shot.title}
                className={`group relative block aspect-video w-full overflow-hidden rounded-md border bg-black/90 transition-all hover:ring-2 hover:ring-stone-400 ${
                    shot.status === "failed" ? "border-red-300" : shot.status === "success" ? "border-stone-300" : "border-stone-200 dark:border-stone-700"
                }`}>
                {shot.status === "success" && shot.videoUrl ? (
                    <>
                        <video src={shot.videoUrl} className="h-full w-full object-cover" muted preload="metadata" />
                        <span className="absolute inset-0 flex items-center justify-center bg-black/20 opacity-0 transition group-hover:opacity-100">
                            <Play className="size-5 text-white" />
                        </span>
                    </>
                ) : (
                    <span className="flex h-full w-full items-center justify-center">
                        {shot.status === "generating" ? <LoaderCircle className="size-4 animate-spin text-white/80" />
                            : shot.status === "failed" ? <AlertCircle className="size-4 text-red-300" />
                            : <Video className="size-4 text-white/40" />}
                    </span>
                )}
                {/* 左上：序号/状态 */}
                <span className="absolute left-1 top-1 flex size-4 items-center justify-center rounded text-[9px] font-medium leading-none bg-black/60 text-white">
                    {shot.status === "success" ? <Check className="size-2.5" /> : index + 1}
                </span>
                {/* 生成中进度 */}
                {shot.status === "generating" && shot.progress !== undefined && (
                    <span className="absolute bottom-1 left-1 rounded bg-black/60 px-1 text-[9px] leading-none text-white">{shot.progress}%</span>
                )}
                {/* 右下：时长 */}
                <span className="absolute bottom-1 right-1 rounded bg-black/60 px-1 text-[9px] leading-none text-white">{shot.duration}s</span>
                {/* 勾选 */}
                <span className="absolute right-1 top-1" onClick={(e) => { e.stopPropagation(); }}>
                    <input type="checkbox" checked={shot.selected} onChange={onToggleSelect} className="accent-stone-600 size-3" title="勾选批量生成" />
                </span>
                {/* 左下：单个删除（hover 显示，生成中隐藏避免与进度角标重叠；阻止冒泡避免打开详情） */}
                {shot.status !== "generating" && (
                    <button
                        onClick={(e) => { e.stopPropagation(); onDelete(); }}
                        title="删除该分镜视频"
                        className="absolute bottom-1 left-1 flex size-4 items-center justify-center rounded bg-black/60 text-white opacity-0 transition group-hover:opacity-100 hover:bg-red-500"
                    >
                        <Trash2 className="size-2.5" />
                    </button>
                )}
            </button>
        );
    }

    return (
        <div className={`overflow-hidden rounded-lg border transition-all hover:shadow-sm ${
            shot.status === "generating" ? "border-stone-300 bg-stone-50 dark:border-stone-700 dark:bg-stone-900/50"
            : shot.status === "success" ? "border-stone-300 bg-white dark:border-stone-700 dark:bg-stone-900"
            : shot.status === "failed" ? "border-red-200 bg-red-50 dark:border-red-900 dark:bg-red-950/20"
            : "border-stone-200 bg-white dark:border-stone-800 dark:bg-stone-900"
        }`}>
            {/* 视频缩略/占位，点击打开详情 */}
            <button onClick={onOpenDetail} className="relative block w-full bg-black/90 aspect-video group">
                {shot.status === "success" && shot.videoUrl ? (
                    <>
                        <video src={shot.videoUrl} className="h-full w-full object-cover" muted preload="metadata" />
                        <span className="absolute inset-0 flex items-center justify-center bg-black/20 opacity-0 transition group-hover:opacity-100">
                            <Play className="size-8 text-white" />
                        </span>
                    </>
                ) : (
                    <span className="flex h-full w-full items-center justify-center">
                        {shot.status === "generating" ? (
                            <span className="flex flex-col items-center gap-1 text-white/80">
                                <LoaderCircle className="size-6 animate-spin" />
                                <span className="text-[10px]">{shot.progress !== undefined ? `${shot.progress}%` : "生成中"}</span>
                            </span>
                        ) : shot.status === "failed" ? (
                            <AlertCircle className="size-7 text-red-300" />
                        ) : (
                            <Video className="size-7 text-white/40" />
                        )}
                    </span>
                )}
                {/* 序号/状态角标 */}
                <span className="absolute left-1.5 top-1.5">{statusBadge}</span>
                {/* 时长角标 */}
                <span className="absolute bottom-1.5 right-1.5 rounded bg-black/60 px-1.5 py-0.5 text-[10px] text-white">{shot.duration}秒</span>
                {/* 勾选（阻止冒泡） */}
                <span className="absolute right-1.5 top-1.5" onClick={(e) => e.stopPropagation()}>
                    <input type="checkbox" checked={shot.selected} onChange={onToggleSelect} className="accent-stone-600 size-3.5" title="勾选批量生成" />
                </span>
            </button>

            {/* 底部信息行 */}
            <div className={`flex items-center gap-1.5 px-2.5 py-2 ${layout === "list" ? "text-xs" : "text-[11px]"}`}>
                <button onClick={onOpenDetail} className="min-w-0 flex-1 truncate text-left font-medium">{shot.title}</button>
                {shot.referencedAssetIds.length > 0 && (
                    <Tooltip title={`${shot.referencedAssetIds.length} 个资产已关联`}>
                        <span className="rounded-full bg-stone-100 px-1.5 py-0.5 text-[10px] text-stone-500 dark:bg-stone-800 dark:text-stone-400 shrink-0">{shot.referencedAssetIds.length}</span>
                    </Tooltip>
                )}
                {shot.status === "idle" && (
                    <Tooltip title="生成"><Button size="small" type="text" icon={<Play className="size-3.5" />} onClick={(e) => { e.stopPropagation(); onGenerate(); }} className="h-6 w-6 p-0" /></Tooltip>
                )}
                {shot.status === "failed" && (
                    <Tooltip title="重试"><Button size="small" type="text" icon={<RefreshCw className="size-3.5" />} onClick={(e) => { e.stopPropagation(); onRetry(); }} className="h-6 w-6 p-0" /></Tooltip>
                )}
                <Tooltip title="删除该分镜视频">
                    <Button size="small" type="text" danger icon={<Trash2 className="size-3.5" />} onClick={(e) => { e.stopPropagation(); onDelete(); }} className="h-6 w-6 p-0" />
                </Tooltip>
            </div>
        </div>
    );
}
