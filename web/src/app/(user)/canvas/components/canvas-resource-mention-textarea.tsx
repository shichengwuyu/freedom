"use client";

import { forwardRef, useMemo, useRef, useState } from "react";
import type { CSSProperties, MouseEvent, PointerEvent, TextareaHTMLAttributes } from "react";
import { createPortal } from "react-dom";
import { AudioLines as AudioIcon, Check, FileText, Film as VideoIcon, Music2 } from "lucide-react";

import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasResourceReference } from "../utils/canvas-resource-references";

type MentionState = {
    start: number;
    query: string;
};

type Props = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "onChange" | "value"> & {
    value: string;
    references: CanvasResourceReference[];
    onChange: (value: string) => void;
    onSubmit?: () => void;
    containerClassName?: string;
    highlightLabels?: boolean;
};

export const CanvasResourceMentionTextarea = forwardRef<HTMLTextAreaElement, Props>(function CanvasResourceMentionTextarea({ value, references, onChange, onSubmit, onKeyDown, className, containerClassName, style, highlightLabels = true, ...props }, forwardedRef) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const textareaRef = useRef<HTMLTextAreaElement | null>(null);
    const overlayRef = useRef<HTMLDivElement | null>(null);
    // mirror div：用于计算 textarea 内光标相对于视口的坐标
    const mirrorRef = useRef<HTMLDivElement | null>(null);
    const [mention, setMention] = useState<MentionState | null>(null);
    const [activeIndex, setActiveIndex] = useState(0);
    const [hasSelection, setHasSelection] = useState(false);
    const candidates = useMemo(() => {
        if (!mention) return [];
        const query = mention.query.trim().toLowerCase();
        const activeReferences = references.filter((item) => item.active);
        if (!query) return activeReferences;
        return activeReferences.filter((item) => `${item.label} ${item.title} ${item.kind} ${item.text || ""}`.toLowerCase().includes(query));
    }, [mention, references]);
    const activeLabels = useMemo(() => (highlightLabels ? Array.from(new Set(references.filter((item) => item.active).map((item) => item.label))).sort((a, b) => b.length - a.length) : []), [highlightLabels, references]);

    const updateValue = (next: string, selectionStart?: number) => {
        onChange(next);
        if (typeof selectionStart !== "number") return;
        requestAnimationFrame(() => {
            textareaRef.current?.focus();
            textareaRef.current?.setSelectionRange(selectionStart, selectionStart);
        });
    };

    const closeMention = () => {
        setMention(null);
        setActiveIndex(0);
    };

    const syncMention = (nextValue: string, cursor: number) => {
        const prefix = nextValue.slice(0, cursor);
        const match = /(^|\s)@([^\s@]*)$/.exec(prefix);
        if (!match || !references.some((item) => item.active)) {
            closeMention();
            return;
        }
        setMention({ start: cursor - match[2].length - 1, query: match[2] });
        setActiveIndex(0);
    };

    const insertReference = (reference: CanvasResourceReference) => {
        if (!mention) return;
        const textarea = textareaRef.current;
        const end = textarea?.selectionStart ?? value.length;
        const insertText = `${reference.label} `;
        const next = `${value.slice(0, mention.start)}${insertText}${value.slice(end)}`;
        closeMention();
        updateValue(next, mention.start + insertText.length);
    };

    const syncOverlayScroll = () => {
        if (!overlayRef.current || !textareaRef.current) return;
        overlayRef.current.scrollTop = textareaRef.current.scrollTop;
        overlayRef.current.scrollLeft = textareaRef.current.scrollLeft;
    };

    const updateSelectionState = () => {
        const textarea = textareaRef.current;
        setHasSelection(Boolean(textarea && textarea.selectionStart !== textarea.selectionEnd));
    };

    // 计算光标相对于视口的坐标（用 mirror div 方法获取准确位置）
    const getCaretCoords = (): { left: number; top: number; bottom: number } | null => {
        const textarea = textareaRef.current;
        if (!textarea) return null;
        try {
            const rect = textarea.getBoundingClientRect();
            const computed = getComputedStyle(textarea);
            const cursorPos = textarea.selectionStart ?? value.length;
            const before = value.slice(0, cursorPos);
            const mirror = mirrorRef.current;
            if (mirror) {
                mirror.style.position = "absolute";
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
                    return { left: markerRect.left, top: markerRect.top, bottom: markerRect.bottom };
                }
            }
            const paddingTop = parseFloat(computed.paddingTop) || 0;
            const paddingLeft = parseFloat(computed.paddingLeft) || 0;
            const lineHeight = parseFloat(computed.lineHeight) || 24;
            return { left: rect.left + paddingLeft, top: rect.top + paddingTop + lineHeight, bottom: rect.top + paddingTop + lineHeight * 2 };
        } catch {
            const rect = textarea.getBoundingClientRect();
            return { left: rect.left, top: rect.bottom, bottom: rect.bottom + 6 };
        }
    };

    const showOverlay = Boolean(activeLabels.length && !hasSelection);
    const mergedStyle = {
        ...(style || {}),
        color: showOverlay ? "transparent" : style?.color,
        caretColor: style?.color || theme.node.text,
        ...(showOverlay ? { background: "transparent", backgroundColor: "transparent" } : {}),
    } as CSSProperties;

    const caretCoords = mention && candidates.length ? getCaretCoords() : null;
    const menu = mention && candidates.length && caretCoords ? <MentionMenu caretCoords={caretCoords} references={candidates} activeIndex={Math.min(activeIndex, candidates.length - 1)} onActiveIndexChange={setActiveIndex} theme={theme} onSelect={insertReference} /> : null;

    return (
        <div className={`relative h-full w-full ${containerClassName || ""}`}>
            {showOverlay ? (
                <div ref={overlayRef} className={`${className || ""} pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words`} style={{ ...style, color: theme.node.text }}>
                    <MentionHighlightText value={value || props.placeholder?.toString() || ""} labels={activeLabels} placeholder={!value} />
                </div>
            ) : null}
            {/* 镜像 div（隐藏）：用于测量光标位置
                必须挂到 document.body 上，否则祖先节点（backdrop-filter/transform 等）
                会劫持 position: fixed 的 containing block，导致 caret 坐标算错、弹层跑位 */}
            {typeof document !== "undefined" ? createPortal(<div ref={mirrorRef} aria-hidden="true" />, document.body) : null}
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
                    window.setTimeout(closeMention, 120);
                    props.onBlur?.(event);
                }}
            />
            {menu}
        </div>
    );
});

function MentionHighlightText({ value, labels, placeholder }: { value: string; labels: string[]; placeholder: boolean }) {
    if (placeholder) return <span className="opacity-45">{value}</span>;
    if (!labels.length) return <>{value}</>;
    const pattern = new RegExp(`(${labels.map(escapeRegExp).join("|")})`, "g");
    return (
        <>
            {value.split(pattern).map((part, index) =>
                labels.includes(part) ? (
                    <span key={`${part}-${index}`} className="rounded-md bg-[#2f80ff]/16 px-1 py-0.5 font-medium text-[#2f80ff] ring-1 ring-[#2f80ff]/24">
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
 * 缩略图网格菜单：支持图片/视频/音频/文本四种类型
 * 每项显示大缩略图 + 右下角序号角标 + 选中态对勾
 * 定位跟随光标位置：默认在光标下方，空间不足则翻到上方
 */
function MentionMenu({
    caretCoords,
    references,
    activeIndex,
    onActiveIndexChange,
    theme,
    onSelect,
}: {
    caretCoords: { left: number; top: number; bottom: number };
    references: CanvasResourceReference[];
    activeIndex: number;
    onActiveIndexChange: (index: number) => void;
    theme: (typeof canvasThemes)[keyof typeof canvasThemes];
    onSelect: (reference: CanvasResourceReference) => void;
}) {
    const selectedRef = useRef(false);
    const itemSize = 56;
    const gap = 6;
    const gridCols = Math.min(4, Math.max(1, references.length));
    const gridRows = Math.ceil(references.length / gridCols);
    const menuPadding = 8;
    const menuWidth = gridCols * (itemSize + gap) - gap + menuPadding * 2;
    const menuHeight = gridRows * (itemSize + gap) - gap + menuPadding * 2 + 20;

    // 菜单位置：水平跟随光标左对齐，垂直在光标下方，空间不够则翻到上方
    const left = clamp(caretCoords.left, 8, window.innerWidth - menuWidth - 8);
    const spaceBelow = window.innerHeight - caretCoords.bottom - 8;
    const spaceAbove = caretCoords.top - 8;
    const showAbove = spaceBelow < menuHeight && spaceAbove >= spaceBelow;
    const top = showAbove
        ? clamp(caretCoords.top - menuHeight - 2, 8, window.innerHeight - menuHeight - 8)
        : clamp(caretCoords.bottom + 2, 8, window.innerHeight - menuHeight - 8);

    const stopCanvasInteraction = (event: PointerEvent | MouseEvent) => {
        event.stopPropagation();
    };
    const selectReference = (reference: CanvasResourceReference) => {
        if (selectedRef.current) return;
        selectedRef.current = true;
        onSelect(reference);
    };

    return createPortal(
        <div
            data-canvas-resource-mention-menu="true"
            className="fixed z-[120] rounded-2xl border p-2.5 shadow-2xl backdrop-blur-xl"
            style={{ left, top, width: menuWidth, background: theme.toolbar.panel, borderColor: theme.toolbar.border, color: theme.node.text }}
            onPointerDown={stopCanvasInteraction}
            onMouseDown={stopCanvasInteraction}
            onClick={(event) => event.stopPropagation()}
        >
            {/* 标题栏 */}
            <div className="mb-1 flex items-center justify-between px-0.5">
                <span className="text-[11px] font-medium opacity-60">选择参考素材</span>
                <span className="text-[10px] opacity-45">方向键 · 回车 · ESC</span>
            </div>
            {/* 缩略图网格 */}
            <div className="grid" style={{ gridTemplateColumns: `repeat(${gridCols}, minmax(0, 1fr))`, gap: `${gap}px` }}>
                {references.map((reference, index) => {
                    const isActive = index === activeIndex;
                    return (
                        <button
                            key={`${reference.kind}-${reference.id}-${index}`}
                            type="button"
                            className="group relative aspect-square overflow-hidden rounded-xl border-2 transition-all duration-150"
                            style={{
                                borderColor: isActive ? "#2f80ff" : "transparent",
                                background: isActive ? "rgba(47,128,255,0.12)" : "transparent",
                            }}
                            onPointerEnter={() => onActiveIndexChange(index)}
                            onPointerDown={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                selectReference(reference);
                            }}
                            onClick={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                selectReference(reference);
                            }}
                        >
                            <ReferenceThumbnail reference={reference} />
                            {/* 序号角标（右下角） */}
                            <span className="absolute right-0.5 bottom-0.5 flex min-w-[18px] items-center justify-center rounded-full bg-black/60 px-1 py-0.5 text-[9px] font-semibold text-white shadow-sm ring-1 ring-white/20 backdrop-blur-sm">
                                {index + 1}
                            </span>
                            {/* 选中态对勾 */}
                            {isActive ? (
                                <span className="absolute left-0.5 top-0.5 flex size-4 items-center justify-center rounded-full bg-[#2f80ff] text-white shadow-md ring-1 ring-white">
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
 * 缩略图渲染：根据类型显示不同内容
 */
function ReferenceThumbnail({ reference }: { reference: CanvasResourceReference }) {
    if (reference.kind === "image" && reference.previewUrl) {
        return <img src={reference.previewUrl} alt={reference.title} className="h-full w-full object-cover transition-transform duration-150 group-hover:scale-[1.04]" draggable={false} />;
    }
    if (reference.kind === "video" && reference.previewUrl) {
        return (
            <>
                <video src={reference.previewUrl} className="h-full w-full object-cover transition-transform duration-150 group-hover:scale-[1.04]" muted preload="metadata" />
                <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent pt-3 pb-0.5">
                    <span className="mx-1 inline-flex items-center gap-0.5 text-[9px] font-medium text-white">
                        <VideoIcon className="h-2.5 w-2.5" /> 视频
                    </span>
                </span>
            </>
        );
    }
    if (reference.kind === "audio") {
        return (
            <span className="flex h-full w-full flex-col items-center justify-center gap-1" style={{ background: "linear-gradient(135deg, rgba(139,92,246,0.15), rgba(217,70,239,0.15))" }}>
                <AudioIcon className="h-4 w-4 text-violet-500" />
                <Music2 className="absolute bottom-1.5 right-1.5 h-3 w-3 text-violet-400" />
            </span>
        );
    }
    // 文本类型
    return (
        <span className="flex h-full w-full flex-col items-center justify-center gap-1 bg-black/5 p-0.5">
            <FileText className="h-4 w-4 opacity-40" />
            <span className="line-clamp-2 text-center text-[8px] leading-tight opacity-50">{reference.text || reference.title}</span>
        </span>
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
