"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, KeyboardEvent, MouseEvent, PointerEvent } from "react";
import { createPortal } from "react-dom";
import { Image } from "antd";
import { AudioLines as AudioIcon, Check, FileText, Film as VideoIcon, Music2 } from "lucide-react";
import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import type { CanvasResourceReference } from "../utils/canvas-resource-references";

type CanvasPromptChipInputProps = {
    value: string;
    references: CanvasResourceReference[];
    onChange: (value: string) => void;
    onSubmit?: () => void;
    className?: string;
    style?: CSSProperties;
    placeholder?: string;
};

type MentionState = {
    query: string;
    rect: DOMRect | null;
};

type PromptToken =
    | { type: "text"; value: string }
    | { type: "reference"; label: string };

export function CanvasPromptChipInput({ value, references, onChange, onSubmit, className, style, placeholder }: CanvasPromptChipInputProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const editorRef = useRef<HTMLDivElement>(null);
    const composingRef = useRef(false);
    const lastEmittedRef = useRef(value);
    const [mention, setMention] = useState<MentionState | null>(null);
    const [activeIndex, setActiveIndex] = useState(0);
    const [imagePreview, setImagePreview] = useState<string | null>(null);
    const activeReferences = useMemo(() => references.filter((reference) => reference.active), [references]);
    const referenceByLabel = useMemo(() => new Map(activeReferences.map((reference) => [reference.label, reference])), [activeReferences]);
    const activeLabels = useMemo(() => Array.from(new Set(activeReferences.map((reference) => reference.label))).sort((left, right) => right.length - left.length), [activeReferences]);
    const tokens = useMemo(() => parsePromptTokens(value, activeLabels), [activeLabels, value]);
    const candidates = useMemo(() => {
        if (!mention) return [];
        const query = mention.query.trim().toLowerCase();
        if (!query) return activeReferences;
        return activeReferences.filter((reference) => `${reference.label} ${reference.title} ${reference.kind} ${reference.text || ""}`.toLowerCase().includes(query));
    }, [activeReferences, mention]);

    useEffect(() => {
        const editor = editorRef.current;
        if (!editor) return;
        if (document.activeElement === editor && value === lastEmittedRef.current) return;
        editor.textContent = "";
        tokens.forEach((token) => {
            if (token.type === "text") {
                editor.append(document.createTextNode(token.value));
                return;
            }
            const reference = referenceByLabel.get(token.label);
            if (reference) editor.append(createReferenceChip(reference, theme, setImagePreview));
            else editor.append(document.createTextNode(token.label));
        });
        lastEmittedRef.current = value;
    }, [referenceByLabel, theme, tokens, value]);

    const emitChange = (nextValue: string) => {
        lastEmittedRef.current = nextValue;
        onChange(nextValue);
    };

    const closeMention = () => {
        setMention(null);
        setActiveIndex(0);
    };

    const syncMention = () => {
        const text = textBeforeCaret();
        const match = /@([^\s@]*)$/.exec(text);
        if (!match || !activeReferences.length) {
            closeMention();
            return;
        }
        setMention({
            query: match[1] || "",
            rect: getCaretRect(),
        });
        setActiveIndex(0);
    };

    const syncFromEditor = () => {
        const editor = editorRef.current;
        if (!editor) return;
        emitChange(serializePromptEditor(editor));
        syncMention();
    };

    const insertReference = (reference: CanvasResourceReference) => {
        const editor = editorRef.current;
        if (!editor) return;
        removeActiveMention();
        const leadingSpace = document.createTextNode(" ");
        const chip = createReferenceChip(reference, theme, setImagePreview);
        const trailingSpace = document.createTextNode(" ");
        const selection = window.getSelection();
        const range = selection?.rangeCount ? selection.getRangeAt(0) : null;
        if (range) {
            range.insertNode(trailingSpace);
            range.insertNode(chip);
            range.insertNode(leadingSpace);
            range.setStartAfter(trailingSpace);
            range.collapse(true);
            selection?.removeAllRanges();
            selection?.addRange(range);
        } else {
            editor.append(leadingSpace, chip, trailingSpace);
            placeCaretAtEnd(editor);
        }
        closeMention();
        emitChange(serializePromptEditor(editor));
    };

    const showPlaceholder = !value.trim();

    return (
        <div className="relative w-full">
            {showPlaceholder && placeholder ? (
                <div className="pointer-events-none absolute left-3 top-2 text-sm leading-5" style={{ color: theme.node.placeholder }}>
                    {placeholder}
                </div>
            ) : null}

            <div
                ref={editorRef}
                contentEditable
                suppressContentEditableWarning
                role="textbox"
                aria-multiline="true"
                aria-label={placeholder}
                className={`${className || ""} overflow-y-auto whitespace-pre-wrap break-words outline-none`}
                style={{ ...style, cursor: "text" }}
                onInput={() => {
                    if (!composingRef.current) syncFromEditor();
                }}
                onPaste={(event) => {
                    const text = event.clipboardData.getData("text/plain");
                    if (!text) return;

                    event.preventDefault();

                    const selection = window.getSelection();
                    const range = selection?.rangeCount ? selection.getRangeAt(0) : null;
                    if (!range) return;

                    range.deleteContents();

                    const textNode = document.createTextNode(text);
                    range.insertNode(textNode);
                    range.setStartAfter(textNode);
                    range.collapse(true);
                    selection?.removeAllRanges();
                    selection?.addRange(range);

                    syncFromEditor();
                }}
                onCompositionStart={() => {
                    composingRef.current = true;
                }}
                onCompositionEnd={() => {
                    composingRef.current = false;
                    syncFromEditor();
                }}
                onKeyDown={(event: KeyboardEvent<HTMLDivElement>) => {
                    event.stopPropagation();

                    const nativeEvent = event.nativeEvent;
                    const isComposing = composingRef.current || nativeEvent.isComposing || nativeEvent.keyCode === 229;
                    if (isComposing) return;

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

                    if ((event.key === "Backspace" || event.key === "Delete") && deleteAdjacentReference(event.key)) {
                        event.preventDefault();
                        requestAnimationFrame(syncFromEditor);
                        return;
                    }

                    if (event.key === "Enter" && !event.shiftKey && !event.ctrlKey && !event.metaKey && onSubmit) {
                        event.preventDefault();
                        onSubmit();
                        return;
                    }

                    requestAnimationFrame(syncMention);
                }}
                onPointerUp={() => {
                    requestAnimationFrame(syncMention);
                }}
                onBlur={() => {
                    window.setTimeout(closeMention, 120);
                }}
            />

            {mention && candidates.length ? (
                <MentionMenu
                    rect={mention.rect}
                    references={candidates}
                    activeIndex={Math.min(activeIndex, candidates.length - 1)}
                    onActiveIndexChange={setActiveIndex}
                    theme={theme}
                    onSelect={insertReference}
                />
            ) : null}

            {imagePreview ? (
                <Image
                    src={imagePreview}
                    alt="引用图片预览"
                    style={{ display: "none" }}
                    preview={{
                        visible: true,
                        src: imagePreview,
                        onVisibleChange: (visible) => {
                            if (!visible) setImagePreview(null);
                        },
                    }}
                />
            ) : null}
        </div>
    );
}

/**
 * 缩略图网格菜单：支持图片/视频/音频/文本四种类型
 * 每项显示大缩略图 + 右下角序号角标 + 选中态对勾
 * 定位跟随光标位置：默认在光标下方，空间不足则翻到上方
 */
function MentionMenu({
    rect,
    references,
    activeIndex,
    onActiveIndexChange,
    theme,
    onSelect,
}: {
    rect: DOMRect | null;
    references: CanvasResourceReference[];
    activeIndex: number;
    onActiveIndexChange: (index: number) => void;
    theme: (typeof canvasThemes)[keyof typeof canvasThemes];
    onSelect: (reference: CanvasResourceReference) => void;
}) {
    const selectedRef = useRef(false);
    const itemSize = 84;
    const gap = 8;
    const gridCols = Math.min(4, Math.max(1, references.length));
    const gridRows = Math.ceil(references.length / gridCols);
    const menuPadding = 10;
    const menuWidth = gridCols * (itemSize + gap) - gap + menuPadding * 2;
    const menuHeight = gridRows * (itemSize + gap) - gap + menuPadding * 2 + 24;

    const anchor = rect || new DOMRect(16, 16, 0, 0);
    const left = clamp(anchor.left, 8, window.innerWidth - menuWidth - 8);
    const spaceBelow = window.innerHeight - anchor.bottom - 8;
    const spaceAbove = anchor.top - 8;
    const showAbove = spaceBelow < menuHeight && spaceAbove >= spaceBelow;
    const top = showAbove
        ? clamp(anchor.top - menuHeight - 6, 8, window.innerHeight - menuHeight - 8)
        : clamp(anchor.bottom + 6, 8, window.innerHeight - menuHeight - 8);

    const selectReference = (reference: CanvasResourceReference) => {
        if (selectedRef.current) return;
        selectedRef.current = true;
        onSelect(reference);
    };
    const stopCanvasInteraction = (event: PointerEvent | MouseEvent) => {
        event.stopPropagation();
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
            <div className="mb-2 flex items-center justify-between px-1">
                <span className="text-xs font-medium opacity-60">选择参考素材</span>
                <span className="text-[11px] opacity-45">方向键选择 · 回车确认 · ESC 关闭</span>
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
                            <span className="absolute right-1 bottom-1 flex min-w-[22px] items-center justify-center rounded-full bg-black/60 px-1.5 py-0.5 text-[10px] font-semibold text-white shadow-sm ring-1 ring-white/20 backdrop-blur-sm">
                                {index + 1}
                            </span>
                            {/* 选中态对勾 */}
                            {isActive ? (
                                <span className="absolute left-1 top-1 flex size-5 items-center justify-center rounded-full bg-[#2f80ff] text-white shadow-md ring-2 ring-white">
                                    <Check className="h-3 w-3" strokeWidth={3} />
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
                <span className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent pt-4 pb-1">
                    <span className="mx-1 inline-flex items-center gap-1 text-[10px] font-medium text-white">
                        <VideoIcon className="h-3 w-3" /> 视频
                    </span>
                </span>
            </>
        );
    }
    if (reference.kind === "audio") {
        return (
            <span className="flex h-full w-full flex-col items-center justify-center gap-1" style={{ background: "linear-gradient(135deg, rgba(139,92,246,0.15), rgba(217,70,239,0.15))" }}>
                <AudioIcon className="h-6 w-6 text-violet-500" />
                <Music2 className="absolute bottom-2 right-2 h-4 w-4 text-violet-400" />
            </span>
        );
    }
    // 文本类型
    return (
        <span className="flex h-full w-full flex-col items-center justify-center gap-1 bg-black/5 p-1">
            <FileText className="h-6 w-6 opacity-40" />
            <span className="line-clamp-3 text-center text-[9px] leading-tight opacity-50">{reference.text || reference.title}</span>
        </span>
    );
}

function createReferenceChip(
    reference: CanvasResourceReference,
    theme: (typeof canvasThemes)[keyof typeof canvasThemes],
    onImagePreview: (url: string) => void,
) {
    const wrapper = document.createElement("span");
    wrapper.contentEditable = "false";
    wrapper.dataset.refLabel = reference.label;
    if (reference.kind === "image" && reference.previewUrl) {
        const image = document.createElement("img");
        image.src = reference.previewUrl;
        image.alt = reference.title;
        image.className = "size-6 rounded object-cover";
        wrapper.className = "mx-px inline-flex size-6 items-center justify-center overflow-hidden rounded align-middle";
        wrapper.appendChild(image);
        wrapper.addEventListener("click", (event) => {
            event.preventDefault();
            event.stopPropagation();
            onImagePreview(reference.previewUrl || "");
        });
        return wrapper;
    }

    wrapper.className = "mx-px inline-flex h-6 max-w-40 items-center justify-center overflow-hidden rounded-md border px-1 text-xs leading-none align-middle";
    wrapper.style.background = theme.toolbar.panel;
    wrapper.style.borderColor = theme.node.stroke;
    wrapper.style.color = theme.node.text;
    wrapper.title = reference.text || reference.title;
    const text = document.createElement("span");
    text.className = "block truncate";
    text.textContent = reference.kind === "text" ? reference.text || reference.title : reference.label;
    wrapper.appendChild(text);
    return wrapper;
}

function serializePromptEditor(editor: HTMLElement) {
    return serializePromptNodes(editor.childNodes).replace(/\uFEFF/g, "");
}

function serializePromptNodes(nodes: NodeListOf<ChildNode>) {
    let result = "";
    nodes.forEach((node) => {
        if (node.nodeType === Node.TEXT_NODE) {
            result += node.textContent || "";
            return;
        }
        if (!(node instanceof HTMLElement)) return;
        const referenceLabel = node.dataset.refLabel;
        if (referenceLabel) {
            result += referenceLabel;
            return;
        }
        if (node.tagName === "BR") {
            result += "\n";
            return;
        }
        const content = serializePromptNodes(node.childNodes);
        const isBlock = node.tagName === "DIV" || node.tagName === "P";
        if (isBlock && result && !result.endsWith("\n")) result += "\n";
        result += content;
        if (isBlock && !content) result += "\n";
    });
    return result;
}

function removeActiveMention() {
    const selection = window.getSelection();
    if (!selection?.rangeCount) return;
    const range = selection.getRangeAt(0);
    const text = textBeforeCaret();
    const match = /@([^\s@]*)$/.exec(text);
    if (!match) return;
    range.setStart(range.startContainer, Math.max(0, range.startOffset - (match[1] || "").length - 1));
    range.deleteContents();
}

function deleteAdjacentReference(key: string) {
    const selection = window.getSelection();
    if (!selection?.rangeCount || !selection.isCollapsed) return false;
    const range = selection.getRangeAt(0);
    const target = adjacentReferenceNode(range, key);
    if (!target) return false;
    target.parentNode?.normalize();
    const previousSibling = target.previousSibling;
    const nextSibling = target.nextSibling;
    if (previousSibling?.nodeType === Node.TEXT_NODE) previousSibling.textContent = (previousSibling.textContent || "").replace(/[ \u00A0]$/, "");
    if (nextSibling?.nodeType === Node.TEXT_NODE) nextSibling.textContent = (nextSibling.textContent || "").replace(/^[ \u00A0]/, "");
    const nextCaretNode = document.createTextNode("");
    target.replaceWith(nextCaretNode);
    range.setStart(nextCaretNode, 0);
    range.collapse(true);
    selection.removeAllRanges();
    selection.addRange(range);
    return true;
}

function adjacentReferenceNode(range: Range, key: string) {
    const container = range.startContainer;
    const offset = range.startOffset;
    const previous = key === "Backspace";
    if (container.nodeType === Node.TEXT_NODE) {
        const text = container.textContent || "";
        if ((previous && offset > 0) || (!previous && offset < text.length)) return null;
        return findReferenceSibling(container, previous);
    }
    const children = Array.from(container.childNodes);
    return findReferenceSibling(children[previous ? offset - 1 : offset] || container, previous, true);
}

function findReferenceSibling(node: Node, previous: boolean, includeSelf = false): HTMLElement | null {
    let current: Node | null = includeSelf ? node : previous ? node.previousSibling : node.nextSibling;
    while (current && current.nodeType === Node.TEXT_NODE && !(current.textContent || "").trim()) {
        current = previous ? current.previousSibling : current.nextSibling;
    }
    return current instanceof HTMLElement && current.dataset.refLabel ? current : null;
}

function textBeforeCaret() {
    const selection = window.getSelection();
    if (!selection?.rangeCount) return "";
    const range = selection.getRangeAt(0).cloneRange();
    const editor = closestPromptEditor(range.startContainer);
    if (!editor) return "";
    range.setStart(editor, 0);
    return range.toString();
}

function getCaretRect(): DOMRect | null {
    const selection = window.getSelection();
    if (!selection?.rangeCount) return null;
    const range = selection.getRangeAt(0).cloneRange();
    range.collapse(true);
    const rect = range.getBoundingClientRect();
    if (rect.width || rect.height || rect.left || rect.top) return rect;
    const editor = closestPromptEditor(range.startContainer);
    return editor?.getBoundingClientRect() || null;
}

function closestPromptEditor(node: Node) {
    const element = node instanceof Element ? node : node.parentElement;
    return element?.closest("[contenteditable='true']") || null;
}

function placeCaretAtEnd(element: HTMLElement) {
    const range = document.createRange();
    range.selectNodeContents(element);
    range.collapse(false);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
}

function parsePromptTokens(value: string, labels: string[]): PromptToken[] {
    if (!labels.length) return value ? [{ type: "text", value }] : [];
    const pattern = new RegExp(`(${labels.map(escapeRegExp).join("|")})`, "g");
    const tokens: PromptToken[] = [];
    let lastIndex = 0;
    for (const match of value.matchAll(pattern)) {
        if (match.index === undefined) continue;
        if (match.index > lastIndex) {
            tokens.push({
                type: "text",
                value: value.slice(lastIndex, match.index),
            });
        }
        tokens.push({
            type: "reference",
            label: match[0],
        });
        lastIndex = match.index + match[0].length;
    }
    if (lastIndex < value.length) {
        tokens.push({
            type: "text",
            value: value.slice(lastIndex),
        });
    }
    return tokens;
}

function escapeRegExp(value: string) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function clamp(value: number, min: number, max: number) {
    if (max < min) return min;
    return Math.min(Math.max(value, min), max);
}