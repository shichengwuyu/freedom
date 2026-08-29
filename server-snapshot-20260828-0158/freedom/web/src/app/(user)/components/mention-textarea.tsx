"use client";

import { forwardRef, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent, MouseEvent, RefObject, TextareaHTMLAttributes } from "react";
import { createPortal } from "react-dom";
import { AudioLines as AudioIcon, Check, ChevronRight, Film as VideoIcon, FolderOpen, Image as ImageIcon, Music2, PenLine } from "lucide-react";

import type { Asset } from "@/stores/use-asset-store";

// @ 候选项：支持图片 / 视频 / 音频等类型
export type MentionCandidate = {
    id: string; // 唯一 ID
    label: string; // 插入文本里的标签，如"图片1""视频2""音频1"
    title: string; // 候选项标题（文件名等）
    previewUrl?: string; // 预览图 URL（图片/视频封面）
    mediaUrl?: string; // 音频/视频的播放 URL（有则展示播放角标）
    kind: "image" | "video" | "audio" | "text"; // 素材类型，用于分组显示和图标
    index: number; // 同类素材的顺序（用于序号角标显示 1/2/3…）
    asset?: Asset; // 关联的素材库资产（素材库分组点击后需要落库为参考图）
    storageKey?: string; // 最近生成图的存储 key（供调用方落库为参考图）
    mimeType?: string; // 最近生成图的 MIME 类型
};
// @ 触发状态：起始位置 + 搜索关键词
type MentionState = { start: number; query: string };

// 预设提示词（弹层顶部「选择预设提示词」）
export type MentionPromptItem = { id: string; title: string; coverUrl?: string; prompt: string };
// 最近生成图（弹层「最近生成」区）
export type MentionRecentItem = { id: string; title: string; previewUrl: string; storageKey?: string; mimeType?: string };
// 素材库分组（弹层「素材库」区）
export type MentionAssetGroup = { name: string; items: MentionCandidate[] };

type Props = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "onChange" | "value"> & {
    value: string; // 提示词文本
    candidates: MentionCandidate[]; // 自定义候选项列表（支持多种类型）
    onChange: (value: string) => void; // 文本变化回调
    onSubmit?: () => void; // 回车提交回调
    autoSize?: { minRows: number; maxRows: number }; // 自动高度
    className?: string;
    containerClassName?: string;
    // 扩展弹层：预设提示词 / 最近生成图 / 素材库分组（任一项有值时 @ 按钮弹层展示分组菜单）
    promptItems?: MentionPromptItem[];
    recentGenerated?: MentionRecentItem[];
    assetGroups?: MentionAssetGroup[];
    // 在 textarea 左侧渲染独立 @ 按钮（点击打开分组弹层）
    showAtButton?: boolean;
    // 素材类候选（素材库 / 最近生成图）被选中时的回调，返回要插入文本的标签（如"图片3"）；返回 null 取消插入；不传或返回 undefined 则用候选自身 label
    onSelectAsset?: (candidate: MentionCandidate) => string | null | void;
};

/**
 * 通用 @ 提及文本框组件（图片/视频/音频素材都能支持）
 * 输入 @ 后弹出缩略图网格菜单，选择后在光标处插入标签
 * 标签在文本中高亮显示（不区分类型，统一蓝色标签）
 *
 * 扩展能力（可选）：
 * - 传入 promptItems / recentGenerated / assetGroups 后，点击左侧独立 @ 按钮会打开分组弹层
 *   （选择预设提示词 / 最近生成 / 素材库二级分组），选中素材通过 onSelectAsset 落库为参考图并插入标签
 */
export const MentionTextarea = forwardRef<HTMLTextAreaElement, Props>(function MentionTextarea(
    {
        value,
        candidates: externalCandidates,
        onChange,
        onSubmit,
        onKeyDown,
        autoSize,
        className,
        containerClassName,
        style,
        promptItems = [],
        recentGenerated = [],
        assetGroups = [],
        showAtButton = false,
        onSelectAsset,
        ...props
    },
    forwardedRef,
) {
    const textareaRef = useRef<HTMLTextAreaElement | null>(null);
    const overlayRef = useRef<HTMLDivElement | null>(null);
    const atButtonRef = useRef<HTMLButtonElement | null>(null);
    const panelRef = useRef<HTMLDivElement | null>(null);
    // mirror div：用于计算 textarea 内光标相对于视口的坐标
    const mirrorRef = useRef<HTMLDivElement | null>(null);
    const [mention, setMention] = useState<MentionState | null>(null); // 当前 @ 触发状态
    const [activeIndex, setActiveIndex] = useState(0); // 当前选中的候选项索引
    const [hasSelection, setHasSelection] = useState(false); // 是否有文本选区
    const [buttonMenuOpen, setButtonMenuOpen] = useState(false); // 独立 @ 按钮分组弹层是否打开

    const allCandidates = externalCandidates;

    // 根据 @ 关键词过滤候选项
    const candidates = useMemo(() => {
        if (!mention) return [];
        const query = mention.query.trim().toLowerCase();
        if (!query) return allCandidates;
        return allCandidates.filter((item) => `${item.label} ${item.title}`.toLowerCase().includes(query));
    }, [mention, allCandidates]);

    // 需要在文本中高亮的标签列表
    const activeLabels = useMemo(() => allCandidates.map((c) => c.label), [allCandidates]);

    // 自动调整文本框高度
    useEffect(() => {
        if (!autoSize || !textareaRef.current) return;
        const textarea = textareaRef.current;
        const computed = getComputedStyle(textarea);
        const lineHeight = parseFloat(computed.lineHeight) || 24;
        const padding = parseFloat(computed.paddingTop) + parseFloat(computed.paddingTop) || 16;
        const minHeight = lineHeight * autoSize.minRows + padding;
        const maxHeight = lineHeight * autoSize.maxRows + padding;
        textarea.style.height = `${minHeight}px`;
        const scrollHeight = textarea.scrollHeight;
        textarea.style.height = `${Math.min(Math.max(scrollHeight, minHeight), maxHeight)}px`;
    }, [value, autoSize]);

    // 更新文本值并恢复光标位置
    const updateValue = (next: string, selectionStart?: number) => {
        onChange(next);
        if (typeof selectionStart !== "number") return;
        requestAnimationFrame(() => {
            textareaRef.current?.focus();
            textareaRef.current?.setSelectionRange(selectionStart, selectionStart);
        });
    };

    // 关闭 @ 选择菜单
    const closeMention = () => {
        setMention(null);
        setActiveIndex(0);
    };

    const closeButtonMenu = () => {
        setButtonMenuOpen(false);
    };

    // 检测用户是否正在输入 @ 触发引用
    // 允许 @ 紧贴任意字符（含中文）触发：只要求 @ 后面还没出现空白/新的 @，就视为正在输入引用关键词
    const syncMention = (nextValue: string, cursor: number) => {
        const prefix = nextValue.slice(0, cursor);
        const match = /@([^\s@]*)$/.exec(prefix);
        if (!match || !allCandidates.length) {
            closeMention();
            return;
        }
        setMention({ start: cursor - match[1].length - 1, query: match[1] });
        setActiveIndex(0);
    };

    // 选中某个候选项后，将标签插入文本
    const insertReference = (candidate: MentionCandidate) => {
        if (!mention) return;
        const textarea = textareaRef.current;
        const end = textarea?.selectionStart ?? value.length;
        const insertText = `${candidate.label} `;
        const next = `${value.slice(0, mention.start)}${insertText}${value.slice(end)}`;
        closeMention();
        updateValue(next, mention.start + insertText.length);
    };

    // 分组弹层：在光标处插入素材标签（onSelectAsset 优先返回真实标签，返回 null 表示取消插入）
    const insertAssetReference = (candidate: MentionCandidate) => {
        const textarea = textareaRef.current;
        const start = textarea?.selectionStart ?? value.length;
        const insertLabel = onSelectAsset ? onSelectAsset(candidate) : candidate.label;
        if (insertLabel === null) return;
        const label = insertLabel ?? candidate.label;
        const insertText = `${label} `;
        const next = `${value.slice(0, start)}${insertText}${value.slice(start)}`;
        closeButtonMenu();
        updateValue(next, start + insertText.length);
    };

    // 同步遮罩层滚动位置
    const syncOverlayScroll = () => {
        if (!overlayRef.current || !textareaRef.current) return;
        overlayRef.current.scrollTop = textareaRef.current.scrollTop;
        overlayRef.current.scrollLeft = textareaRef.current.scrollLeft;
    };

    // 更新文本选区状态
    const updateSelectionState = () => {
        const textarea = textareaRef.current;
        setHasSelection(Boolean(textarea && textarea.selectionStart !== textarea.selectionEnd));
    };

    // 计算光标相对于视口的坐标（用 mirror div 方法获取准确位置）
    const getCaretCoords = (): { left: number; top: number; bottom: number } | null => {
        const textarea = textareaRef.current;
        if (!textarea) return null;
        try {
            // 创建镜像元素复制 textarea 样式与内容，用于测量光标位置
            const rect = textarea.getBoundingClientRect();
            const computed = getComputedStyle(textarea);
            const cursorPos = textarea.selectionStart ?? value.length;
            const before = value.slice(0, cursorPos);
            const mirror = mirrorRef.current;
            if (mirror) {
                // 同步样式（用 fixed 脱离文档流，left/top 直接对视界坐标，不受 relative 父容器影响）
                mirror.style.position = "fixed";
                mirror.style.visibility = "hidden";
                mirror.style.whiteSpace = "pre-wrap";
                mirror.style.wordWrap = "break-word";
                mirror.style.overflow = "hidden";
                mirror.style.fontFamily = computed.fontFamily;
                mirror.style.fontSize = computed.fontSize;
                mirror.style.fontWeight = computed.fontWeight;
                mirror.style.letterSpacing = computed.letterSpacing;
                mirror.style.lineHeight = computed.lineHeight;
                mirror.style.paddingTop = computed.paddingTop;
                mirror.style.paddingRight = computed.paddingRight;
                mirror.style.paddingBottom = computed.paddingBottom;
                mirror.style.paddingLeft = computed.paddingLeft;
                mirror.style.border = computed.border;
                mirror.style.boxSizing = computed.boxSizing;
                mirror.style.width = `${textarea.offsetWidth}px`;
                mirror.style.height = `${textarea.offsetHeight}px`;
                mirror.style.left = `${rect.left}px`;
                mirror.style.top = `${rect.top}px`;
                mirror.style.marginTop = "0px";
                mirror.style.marginLeft = "0px";
                mirror.innerHTML = `${escapeHtml(before)}<span id="__caret_marker__">|</span>`;
                const marker = mirror.querySelector<HTMLSpanElement>("#__caret_marker__");
                if (marker) {
                    const markerRect = marker.getBoundingClientRect();
                    return {
                        left: markerRect.left,
                        top: markerRect.top,
                        bottom: markerRect.bottom,
                    };
                }
            }
            // 降级：用 textarea 底部 + 内边距估算
            const paddingTop = parseFloat(computed.paddingTop) || 0;
            const paddingLeft = parseFloat(computed.paddingLeft) || 0;
            const lineHeight = parseFloat(computed.lineHeight) || 24;
            return {
                left: rect.left + paddingLeft,
                top: rect.top + paddingTop + lineHeight,
                bottom: rect.top + paddingTop + lineHeight * 2,
            };
        } catch {
            const rect = textarea.getBoundingClientRect();
            const computed = getComputedStyle(textarea);
            const paddingTop = parseFloat(computed.paddingTop) || 0;
            const paddingLeft = parseFloat(computed.paddingLeft) || 0;
            return {
                left: rect.left + paddingLeft,
                top: rect.bottom,
                bottom: rect.bottom + 6,
            };
        }
    };

    // 是否显示高亮遮罩层
    const showOverlay = Boolean(activeLabels.length && !hasSelection);
    const mergedStyle: CSSProperties = {
        ...(style || {}),
        // 显示遮罩时隐藏原始文本，只显示遮罩层的高亮文本
        color: showOverlay ? "transparent" : style?.color,
        caretColor: style?.color || "inherit",
        ...(showOverlay ? { background: "transparent", backgroundColor: "transparent" } : {}),
    };

    // 独立 @ 按钮分组弹层是否可用（有任一扩展内容或候选素材）
    const hasPanelContent = Boolean(promptItems.length || recentGenerated.length || assetGroups.length || allCandidates.length);
    // 分组弹层锚点：@ 按钮位置（未测量到时用 textarea 位置兜底）
    const panelAnchor = buttonMenuOpen ? getAtButtonCoords(atButtonRef, textareaRef) : null;

    // 构建下拉菜单：传入 caret 坐标
    const caretCoords = mention && candidates.length ? getCaretCoords() : null;
    const menu =
        mention && candidates.length && textareaRef.current && caretCoords ? (
            <MentionMenu
                caretCoords={caretCoords}
                candidates={candidates}
                activeIndex={Math.min(activeIndex, candidates.length - 1)}
                onActiveIndexChange={setActiveIndex}
                onSelect={insertReference}
            />
        ) : buttonMenuOpen && panelAnchor && hasPanelContent ? (
            <MentionPanelMenu
                containerRef={panelRef}
                anchor={panelAnchor}
                promptItems={promptItems}
                recentGenerated={recentGenerated}
                assetGroups={assetGroups}
                onSelectPrompt={(promptText) => {
                    onChange(promptText);
                    closeButtonMenu();
                }}
                onSelectRecent={(item) => {
                    insertAssetReference({ id: item.id, label: "", title: item.title, previewUrl: item.previewUrl, kind: "image", index: 0, storageKey: item.storageKey, mimeType: item.mimeType });
                }}
                onSelectAssetCandidate={(candidate) => insertAssetReference(candidate)}
            />
        ) : null;

    return (
        <div className={`flex w-full items-start ${containerClassName || ""}`}>
            {/* 独立 @ 按钮：点击打开分组弹层 */}
            {showAtButton && hasPanelContent ? (
                <div className="flex shrink-0 items-start pt-1.5 pr-1.5">
                    <button
                        ref={atButtonRef}
                        type="button"
                        aria-label="选择参考素材"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={(event) => {
                            event.preventDefault();
                            setButtonMenuOpen((open) => !open);
                            closeMention();
                        }}
                        className={`flex size-8 shrink-0 select-none items-center justify-center rounded-full border text-base font-semibold transition-colors ${
                            buttonMenuOpen
                                ? "border-sky-400 bg-sky-500/10 text-sky-500 dark:text-sky-400"
                                : "border-stone-200 bg-white text-stone-500 hover:border-sky-300 hover:text-sky-500 dark:border-stone-700 dark:bg-stone-900 dark:text-stone-400"
                        }`}
                    >
                        @
                    </button>
                </div>
            ) : null}
            <div className="relative min-w-0 flex-1">
                {/* 高亮遮罩层：显示带有标签高亮的文本 */}
                {showOverlay ? (
                    <div
                        ref={overlayRef}
                        className={`${className || ""} pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words`}
                        style={{ ...style, color: "inherit" }}
                    >
                        <HighlightText value={value || props.placeholder?.toString() || ""} labels={activeLabels} placeholder={!value} />
                    </div>
                ) : null}
                {/* 镜像 div（隐藏）：用于测量光标位置
                    必须挂到 document.body 上，否则祖先节点（backdrop-filter/transform 等）
                    会劫持 position: fixed 的 containing block，导致 caret 坐标算错、弹层跑位 */}
                {typeof document !== "undefined" ? createPortal(<div ref={mirrorRef} aria-hidden="true" />, document.body) : null}
                {/* 原始文本框 */}
                <textarea
                    {...props}
                    ref={(node) => {
                        textareaRef.current = node;
                        if (typeof forwardedRef === "function") forwardedRef(node);
                        else if (forwardedRef) forwardedRef.current = node;
                    }}
                    value={value}
                    className={className}
                    style={mergedStyle}
                    onChange={(event) => {
                        const next = event.target.value;
                        onChange(next);
                        syncMention(next, event.target.selectionStart);
                        closeButtonMenu();
                        requestAnimationFrame(() => {
                            syncOverlayScroll();
                            updateSelectionState();
                        });
                    }}
                    onSelect={(event) => {
                        updateSelectionState();
                        props.onSelect?.(event);
                    }}
                    onKeyUp={(event) => {
                        updateSelectionState();
                        props.onKeyUp?.(event);
                    }}
                    onPointerUp={(event) => {
                        updateSelectionState();
                        props.onPointerUp?.(event);
                    }}
                    onKeyDown={(event) => {
                        // @ 菜单打开时的键盘导航
                        if (mention && candidates.length) {
                            // 网格布局：上下左右都能切换
                            if (event.key === "ArrowRight" || event.key === "ArrowDown") {
                                event.preventDefault();
                                setActiveIndex((index) => (index + 1) % candidates.length);
                                return;
                            }
                            if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
                                event.preventDefault();
                                setActiveIndex((index) => (index - 1 + candidates.length) % candidates.length);
                                return;
                            }
                            if (event.key === "Enter") {
                                event.preventDefault();
                                insertReference(candidates[Math.min(activeIndex, candidates.length - 1)]);
                                return;
                            }
                            if (event.key === "Escape") {
                                event.preventDefault();
                                closeMention();
                                return;
                            }
                        }
                        // 回车提交（非 Shift 组合）
                        if (event.key === "Enter" && onSubmit && !event.ctrlKey && !event.metaKey && !event.shiftKey) {
                            event.preventDefault();
                            onSubmit();
                            return;
                        }
                        onKeyDown?.(event);
                    }}
                    onScroll={(event) => {
                        syncOverlayScroll();
                        props.onScroll?.(event);
                    }}
                    onBlur={(event) => {
                        setHasSelection(false);
                        window.setTimeout(() => {
                            // 焦点仍在 @ 按钮或分组弹层内时不关闭（点击弹层/按钮导致的失焦）
                            const active = document.activeElement;
                            if (active && (active === atButtonRef.current || (panelRef.current && panelRef.current.contains(active)))) {
                                return;
                            }
                            closeMention();
                            closeButtonMenu();
                        }, 120);
                        props.onBlur?.(event);
                    }}
                />
            </div>
            {menu}
        </div>
    );
});

/** 独立 @ 按钮的视口坐标（用于分组弹层锚定） */
function getAtButtonCoords(atButtonRef: RefObject<HTMLButtonElement | null>, textareaRef: RefObject<HTMLTextAreaElement | null>) {
    const button = atButtonRef.current;
    if (button) {
        const rect = button.getBoundingClientRect();
        return { left: rect.left, top: rect.top, bottom: rect.bottom };
    }
    const textarea = textareaRef.current;
    if (textarea) {
        const rect = textarea.getBoundingClientRect();
        return { left: rect.left, top: rect.top, bottom: rect.bottom };
    }
    return null;
}

/**
 * 高亮文本组件：将 @ 标签（图片1、视频2、音频1 等）在文本中高亮显示
 */
function HighlightText({ value, labels, placeholder }: { value: string; labels: string[]; placeholder: boolean }) {
    if (placeholder) return <span className="opacity-45">{value}</span>;
    if (!labels.length) return <>{value}</>;
    const pattern = new RegExp(`(${labels.map(escapeRegExp).join("|")})`, "g");
    return (
        <>
            {value.split(pattern).map((part, index) =>
                labels.includes(part) ? (
                    <span key={`${part}-${index}`} className="rounded-md bg-sky-500/16 px-1 py-0.5 font-medium text-sky-600 ring-1 ring-sky-500/24 dark:text-sky-400">
                        {part}
                    </span>
                ) : (
                    <span key={`${part}-${index}`}>{part}</span>
                ),
            )}
        </>
    );
}

/**
 * @ 引用下拉菜单：参考素材缩略图网格（最多 4 列）
 * 每张图显示：大缩略图 + 右下角序号角标 + 左上角类型角标（视频/音频） + 选中对勾
 */
function MentionMenu({
    caretCoords,
    candidates,
    activeIndex,
    onActiveIndexChange,
    onSelect,
}: {
    caretCoords: { left: number; top: number; bottom: number };
    candidates: MentionCandidate[];
    activeIndex: number;
    onActiveIndexChange: (index: number) => void;
    onSelect: (candidate: MentionCandidate) => void;
}) {
    const selectedRef = useRef(false);
    const itemSize = 56;
    const gap = 6;
    const gridCols = Math.min(4, Math.max(1, candidates.length));
    const gridRows = Math.ceil(candidates.length / gridCols);
    const menuPadding = 8;
    const menuWidth = gridCols * (itemSize + gap) - gap + menuPadding * 2;
    const menuHeight = gridRows * (itemSize + gap) - gap + menuPadding * 2 + 20;

    // 弹层紧跟 @ 字符之后（光标右侧）显示：左边缘 = 光标 x + 2px
    let left = caretCoords.left + 2;
    if (left + menuWidth > window.innerWidth - 8) {
        // 右侧空间不足：向左展开，但保持右边缘贴着 @ 字符后面，避免弹层与 @ 脱节
        left = Math.max(8, caretCoords.left + 2 - menuWidth + 4);
    }
    const spaceBelow = window.innerHeight - caretCoords.bottom - 8;
    const spaceAbove = caretCoords.top - 8;
    const showAbove = spaceBelow < menuHeight && spaceAbove >= spaceBelow;
    const top = showAbove
        ? clamp(caretCoords.top - menuHeight - 2, 8, window.innerHeight - menuHeight - 8)
        : clamp(caretCoords.bottom + 2, 8, window.innerHeight - menuHeight - 8);

    const stopPropagation = (event: PointerEvent | MouseEvent) => {
        event.stopPropagation();
    };

    const select = (candidate: MentionCandidate) => {
        if (selectedRef.current) return;
        selectedRef.current = true;
        onSelect(candidate);
    };

    return createPortal(
        <div
            className="fixed z-[120] rounded-2xl border border-stone-200 bg-white/95 p-2.5 shadow-[0_20px_60px_rgba(15,23,42,.28)] ring-1 ring-black/5 backdrop-blur-xl dark:border-stone-700 dark:bg-stone-900/95"
            onPointerDown={stopPropagation}
            onMouseDown={stopPropagation}
            onClick={(event) => event.stopPropagation()}
            style={{ left, top, width: menuWidth }}
        >
            <div className="mb-1 flex items-center justify-between px-0.5">
                <span className="text-[11px] font-medium text-stone-500 dark:text-stone-400">选择参考素材</span>
                <span className="text-[10px] text-stone-400 dark:text-stone-500">
                    方向键 · 回车 · ESC
                </span>
            </div>
            {/* 缩略图网格 */}
            <div
                className="grid"
                style={{
                    gridTemplateColumns: `repeat(${gridCols}, minmax(0, 1fr))`,
                    gap: `${gap}px`,
                }}
            >
                {candidates.map((candidate, index) => {
                    const isActive = index === activeIndex;
                    return (
                        <button
                            key={`${candidate.kind}-${candidate.id}-${index}`}
                            type="button"
                            className="group relative aspect-square overflow-hidden rounded-xl border-2 transition-all duration-150"
                            style={{
                                borderColor: isActive ? "var(--ant-color-primary, #1677ff)" : "transparent",
                                background: isActive ? "var(--ant-color-primary-bg, #e6f4ff)" : "transparent",
                                boxShadow: isActive ? "0 0 0 3px color-mix(in srgb, var(--ant-color-primary, #1677ff) 20%, transparent)" : undefined,
                            }}
                            onPointerEnter={() => onActiveIndexChange(index)}
                            onPointerDown={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                select(candidate);
                            }}
                            onClick={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                select(candidate);
                            }}
                        >
                            {/* 不同类型的缩略内容 */}
                            {candidate.kind === "image" && candidate.previewUrl ? (
                                <img
                                    src={candidate.previewUrl}
                                    alt={candidate.title}
                                    className="h-full w-full object-cover transition-transform duration-150 group-hover:scale-[1.04]"
                                    draggable={false}
                                />
                            ) : candidate.kind === "video" ? (
                                <>
                                    {candidate.previewUrl ? (
                                        <img
                                            src={candidate.previewUrl}
                                            alt={candidate.title}
                                            className="h-full w-full object-cover transition-transform duration-150 group-hover:scale-[1.04]"
                                            draggable={false}
                                        />
                                    ) : (
                                        <span className="flex h-full w-full items-center justify-center bg-stone-100 dark:bg-stone-800">
                                            <VideoIcon className="h-4 w-4 text-stone-400" />
                                        </span>
                                    )}
                                    <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent pt-4 pb-1">
                                        <span className="mx-1 inline-flex items-center gap-1 text-[10px] font-medium text-white">
                                            <VideoIcon className="h-3 w-3" /> 视频
                                        </span>
                                    </span>
                                </>
                            ) : candidate.kind === "audio" ? (
                                <span className="flex h-full w-full flex-col items-center justify-center gap-1 bg-gradient-to-br from-violet-50 to-fuchsia-50 dark:from-violet-950/40 dark:to-fuchsia-950/40">
                                    <AudioIcon className="h-4 w-4 text-violet-500" />
                                    <Music2 className="absolute bottom-1.5 right-1.5 h-3 w-3 text-violet-400" />
                                </span>
                            ) : (
                                <span className="flex h-full w-full items-center justify-center bg-stone-100 dark:bg-stone-800">
                                    <ImageIcon className="h-4 w-4 text-stone-400" />
                                </span>
                            )}
                            {/* 序号角标（右下角） */}
                            <span className="absolute right-0.5 bottom-0.5 flex min-w-[18px] items-center justify-center rounded-full bg-black/60 px-1 py-0.5 text-[9px] font-semibold text-white shadow-sm ring-1 ring-white/20 backdrop-blur-sm">
                                {candidate.index + 1}
                            </span>
                            {/* 选中态对勾 */}
                            {isActive ? (
                                <span className="absolute left-0.5 top-0.5 flex size-4 items-center justify-center rounded-full bg-sky-500 text-white shadow-md ring-1 ring-white">
                                    <Check className="h-2.5 w-2.5" strokeWidth={3} />
                                </span>
                            ) : null}
                        </button>
                    );
                })}
            </div>
        </div>,
        document.body,
    );
}

/**
 * 独立 @ 按钮打开的「分组弹层」：
 * 顶部选择预设提示词 → 最近生成 → 素材库（人物/场景/物品/风格/其它 二级分组）
 * 点击素材库分组后进入该组缩略图网格，可返回上级
 */
function MentionPanelMenu({
    containerRef,
    anchor,
    promptItems,
    recentGenerated,
    assetGroups,
    onSelectPrompt,
    onSelectRecent,
    onSelectAssetCandidate,
}: {
    containerRef: RefObject<HTMLDivElement | null>;
    anchor: { left: number; top: number; bottom: number };
    promptItems: MentionPromptItem[];
    recentGenerated: MentionRecentItem[];
    assetGroups: MentionAssetGroup[];
    onSelectPrompt: (prompt: string) => void;
    onSelectRecent: (item: MentionRecentItem) => void;
    onSelectAssetCandidate: (candidate: MentionCandidate) => void;
}) {
    const [subGroup, setSubGroup] = useState<string | null>(null);
    const menuWidth = 316;
    const estimatedHeight = Math.min(430, 76 + promptItems.length * 26 + (recentGenerated.length ? 84 : 0) + assetGroups.length * 32);

    const left = clamp(anchor.left, 8, window.innerWidth - menuWidth - 8);
    const spaceBelow = window.innerHeight - anchor.bottom - 8;
    const spaceAbove = anchor.top - 8;
    const showAbove = spaceBelow < estimatedHeight && spaceAbove >= spaceBelow;
    const top = showAbove ? clamp(anchor.top - estimatedHeight, 8, Math.max(8, window.innerHeight - estimatedHeight - 8)) : clamp(anchor.bottom + 4, 8, window.innerHeight - estimatedHeight - 8);

    const stopPropagation = (event: PointerEvent | MouseEvent) => {
        event.stopPropagation();
    };

    const subGroupItems = subGroup ? assetGroups.find((group) => group.name === subGroup)?.items || [] : [];

    return createPortal(
        <div
            ref={containerRef}
            className="fixed z-[120] w-[316px] overflow-hidden rounded-2xl border border-stone-200 bg-white/95 shadow-[0_20px_60px_rgba(15,23,42,.28)] ring-1 ring-black/5 backdrop-blur-xl dark:border-stone-700 dark:bg-stone-900/95"
            onPointerDown={stopPropagation}
            onMouseDown={stopPropagation}
            onClick={(event) => event.stopPropagation()}
            style={{ left, top }}
        >
            {subGroup ? (
                <>
                    <div className="flex items-center gap-1.5 border-b border-stone-100 px-2.5 py-2 dark:border-stone-800">
                        <button
                            type="button"
                            className="flex size-6 items-center justify-center rounded-md text-stone-500 transition-colors hover:bg-stone-100 dark:text-stone-400 dark:hover:bg-stone-800"
                            onClick={() => setSubGroup(null)}
                            aria-label="返回素材库分组"
                        >
                            <ChevronRight className="size-3.5 rotate-180" />
                        </button>
                        <span className="text-xs font-medium text-stone-700 dark:text-stone-300">{subGroup}</span>
                    </div>
                    <div className="thin-scrollbar grid max-h-[340px] grid-cols-4 gap-2 overflow-y-auto p-2.5">
                        {subGroupItems.map((candidate) => (
                            <button
                                key={`${candidate.kind}-${candidate.id}`}
                                type="button"
                                className="group relative aspect-square overflow-hidden rounded-xl border border-stone-200/80 transition-colors hover:border-sky-400 dark:border-stone-700/80"
                                onClick={(event) => {
                                    event.preventDefault();
                                    event.stopPropagation();
                                    onSelectAssetCandidate(candidate);
                                }}
                            >
                                {candidate.kind === "image" && candidate.previewUrl ? (
                                    <img src={candidate.previewUrl} alt={candidate.title} className="h-full w-full object-cover" draggable={false} />
                                ) : candidate.kind === "video" ? (
                                    <span className="flex h-full w-full items-center justify-center bg-stone-100 dark:bg-stone-800">
                                        <VideoIcon className="h-4 w-4 text-stone-400" />
                                    </span>
                                ) : candidate.kind === "audio" ? (
                                    <span className="flex h-full w-full items-center justify-center bg-gradient-to-br from-violet-50 to-fuchsia-50 dark:from-violet-950/40 dark:to-fuchsia-950/40">
                                        <AudioIcon className="h-4 w-4 text-violet-500" />
                                    </span>
                                ) : (
                                    <span className="flex h-full w-full items-center justify-center bg-stone-100 dark:bg-stone-800">
                                        <ImageIcon className="h-4 w-4 text-stone-400" />
                                    </span>
                                )}
                                <span className="absolute right-0.5 bottom-0.5 flex min-w-[16px] items-center justify-center rounded-full bg-black/60 px-1 py-px text-[9px] font-semibold text-white ring-1 ring-white/20">
                                    {candidate.index + 1}
                                </span>
                            </button>
                        ))}
                        {!subGroupItems.length ? <div className="col-span-4 py-6 text-center text-xs text-stone-400">该分组暂无素材</div> : null}
                    </div>
                </>
            ) : (
                <div className="thin-scrollbar max-h-[420px] overflow-y-auto p-1.5">
                    {/* 顶部标题行 */}
                    <div className="flex items-center justify-between px-2 pb-1 pt-1.5">
                        <span className="text-[11px] font-medium text-stone-500 dark:text-stone-400">选择参考素材</span>
                        <span className="text-[10px] text-stone-400 dark:text-stone-500">点击即添加到参考图</span>
                    </div>
                    {/* 预设提示词 */}
                    {promptItems.length ? (
                        <div className="border-t border-stone-100 px-1 py-1.5 dark:border-stone-800">
                            <div className="px-1 pb-1 text-[11px] font-medium text-stone-500 dark:text-stone-400">选择预设提示词</div>
                            <div className="thin-scrollbar max-h-[120px] overflow-y-auto">
                                {promptItems.map((item) => (
                                    <button
                                        key={item.id}
                                        type="button"
                                        className="flex w-full items-center gap-2 rounded-lg px-1.5 py-1.5 text-left transition-colors hover:bg-stone-100 dark:hover:bg-stone-800"
                                        onClick={(event) => {
                                            event.preventDefault();
                                            event.stopPropagation();
                                            onSelectPrompt(item.prompt);
                                        }}
                                    >
                                        <span className="grid size-6 shrink-0 place-items-center rounded-md bg-sky-500/10 text-sky-500 dark:text-sky-400">
                                            <PenLine className="size-3.5" />
                                        </span>
                                        <span className="line-clamp-1 text-xs text-stone-700 dark:text-stone-300">{item.title}</span>
                                    </button>
                                ))}
                            </div>
                        </div>
                    ) : null}
                    {/* 最近生成 */}
                    {recentGenerated.length ? (
                        <div className="border-t border-stone-100 px-1 py-1.5 dark:border-stone-800">
                            <div className="px-1 pb-1.5 text-[11px] font-medium text-stone-500 dark:text-stone-400">最近生成</div>
                            <div className="flex gap-1.5 overflow-x-auto px-0.5 pb-0.5">
                                {recentGenerated.map((item) => (
                                    <button
                                        key={item.id}
                                        type="button"
                                        className="group relative size-11 shrink-0 overflow-hidden rounded-lg border border-stone-200/80 transition-colors hover:border-sky-400 dark:border-stone-700/80"
                                        title={item.title}
                                        onClick={(event) => {
                                            event.preventDefault();
                                            event.stopPropagation();
                                            onSelectRecent(item);
                                        }}
                                    >
                                        <img src={item.previewUrl} alt={item.title} className="h-full w-full object-cover" draggable={false} />
                                    </button>
                                ))}
                            </div>
                        </div>
                    ) : null}
                    {/* 素材库分组 */}
                    {assetGroups.length ? (
                        <div className="border-t border-stone-100 px-1 py-1.5 dark:border-stone-800">
                            <div className="px-1 pb-1 text-[11px] font-medium text-stone-500 dark:text-stone-400">素材库</div>
                            <div>
                                {assetGroups.map((group) => (
                                    <button
                                        key={group.name}
                                        type="button"
                                        className="flex w-full items-center justify-between rounded-lg px-1.5 py-1.5 text-left transition-colors hover:bg-stone-100 dark:hover:bg-stone-800"
                                        onClick={() => setSubGroup(group.name)}
                                    >
                                        <span className="flex items-center gap-2 text-xs text-stone-700 dark:text-stone-300">
                                            <FolderOpen className="size-3.5 text-amber-500/90" />
                                            {group.name}
                                        </span>
                                        <span className="flex items-center gap-1">
                                            <span className="text-[10px] text-stone-400 dark:text-stone-500">{group.items.length}</span>
                                            <ChevronRight className="size-3.5 text-stone-400" />
                                        </span>
                                    </button>
                                ))}
                            </div>
                        </div>
                    ) : null}
                </div>
            )}
        </div>,
        document.body,
    );
}

function clamp(value: number, min: number, max: number) {
    if (max < min) return min;
    return Math.min(Math.max(value, min), max);
}

function escapeRegExp(value: string) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function escapeHtml(text: string) {
    const div = document.createElement("div");
    div.textContent = text;
    return div.innerHTML;
}
