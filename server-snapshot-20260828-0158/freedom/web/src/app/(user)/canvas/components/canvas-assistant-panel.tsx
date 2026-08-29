"use client";

import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import {
    History,
    Bot,
    PanelRightClose,
    Plus,
    RotateCcw,
    Sparkles,
    Trash2,
    Video,
    X,
    FileText,
    Copy,
    Check,
} from "lucide-react";
import { Button, Modal, Tooltip, Upload, Select, InputNumber, Input, message as antMessage } from "antd";
import { motion } from "motion/react";
import { nanoid } from "nanoid";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { ImageGenerationPending } from "@/components/image-generation-pending";
import { canvasThemes } from "@/lib/canvas-theme";
import { cn } from "@/lib/utils";
import { imageToDataUrl } from "@/services/image-storage";
import { useAssetStore } from "@/stores/use-asset-store";
import { useConfigStore, useEffectiveConfig, selectableModelChannels } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { createCanvasAgentState, runCanvasAgent } from "../agent/canvas-agent-runtime";
import type { CanvasAgentContext } from "../agent/canvas-agent-context";
import type { CanvasAgentAction, CanvasAgentToolResult } from "../agent/canvas-agent-tools";
import {
    CanvasNodeType,
    type AutoPilotMode,
    type CanvasAgentConfig,
    type CanvasAgentState,
    type CanvasAssistantMessage,
    type CanvasAssistantReference,
    type CanvasAssistantSession,
    type CanvasNodeData,
} from "../types";
import { isCanvasImageNodeType } from "../utils/canvas-panorama";
import { flushCanvasPersistence } from "../stores/use-canvas-store";
import { AssistantReferenceChip, CanvasAssistantComposer } from "./canvas-assistant-composer";

const PANEL_MOTION_MS = 500;
const PANEL_MOTION_SECONDS = PANEL_MOTION_MS / 1000;

type CanvasAssistantPanelProps = {
    nodes: CanvasNodeData[];
    selectedNodeIds: Set<string>;
    sessions: CanvasAssistantSession[];
    activeSessionId: string | null;
    agentConfig: CanvasAgentConfig;
    width: number;
    onWidthChange: (width: number) => void;
    onSelectNodeIds: (ids: Set<string>) => void;
    onSessionsChange: (sessions: CanvasAssistantSession[], activeSessionId: string | null) => void;
    onAgentConfigChange: (patch: Partial<CanvasAgentConfig>) => void;
    onPasteImage: (file: File) => void;
    onOpenUpload: () => void;
    onOpenAssets: () => void;
    getAgentContext: (state: CanvasAgentState) => CanvasAgentContext;
    onExecuteAction: (action: CanvasAgentAction, messageReferenceNodeIds: string[]) => Promise<CanvasAgentToolResult>;
    collapsed: boolean;
    onCollapseStart: () => void;
    onCollapse: () => void;
    initialRequest?: { prompt: string; references: CanvasAssistantReference[] } | null;
    onInitialRequestConsumed?: () => void;
};

type PendingDeleteConfirmation = {
    title: string;
    resolve: (confirmed: boolean) => void;
};

export function CanvasAssistantPanel({
    nodes,
    selectedNodeIds,
    sessions,
    activeSessionId,
    agentConfig,
    width,
    onWidthChange,
    onSelectNodeIds,
    onSessionsChange,
    onAgentConfigChange,
    onPasteImage,
    onOpenUpload,
    onOpenAssets,
    getAgentContext,
    onExecuteAction,
    collapsed,
    onCollapseStart,
    onCollapse,
    initialRequest,
    onInitialRequestConsumed,
}: CanvasAssistantPanelProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const effectiveConfig = useEffectiveConfig();
    const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
    const cleanupImages = useAssetStore((state) => state.cleanupImages);
    const abortControllersRef = useRef<Map<string, AbortController>>(new Map());
    const consumedInitialRequestRef = useRef<typeof initialRequest>(null);
    const pendingDeleteRef = useRef<PendingDeleteConfirmation | null>(null);
    const messageListRef = useRef<HTMLDivElement>(null);
    const [view, setView] = useState<"chat" | "history">("chat");
    const [prompt, setPrompt] = useState("");
    const [activeMessageIds, setActiveMessageIds] = useState<Set<string>>(new Set());
    const [checkedChatIds, setCheckedChatIds] = useState<string[]>([]);
    const [deleteChatIds, setDeleteChatIds] = useState<string[]>([]);
    const [resizing, setResizing] = useState(false);
    const [removedReferenceIds, setRemovedReferenceIds] = useState<Set<string>>(new Set());
    const [pendingDelete, setPendingDelete] = useState<PendingDeleteConfirmation | null>(null);
    const [initialSession] = useState(createSession);
    const safeSessions = sessions.length ? sessions : [initialSession];
    const resolvedActiveSessionId = activeSessionId && safeSessions.some((session) => session.id === activeSessionId) ? activeSessionId : safeSessions[0]?.id || null;
    const sessionsRef = useRef<CanvasAssistantSession[]>(safeSessions);
    const activeSessionIdRef = useRef<string | null>(resolvedActiveSessionId);

    useEffect(() => {
        sessionsRef.current = safeSessions;
        activeSessionIdRef.current = resolvedActiveSessionId;
    }, [resolvedActiveSessionId, sessions]);

    useEffect(() => () => {
        abortControllersRef.current.forEach((controller) => controller.abort());
        abortControllersRef.current.clear();
        pendingDeleteRef.current?.resolve(false);
        pendingDeleteRef.current = null;
    }, []);

    const activeSession = safeSessions.find((session) => session.id === resolvedActiveSessionId) || safeSessions[0] || null;
    const historySessions = safeSessions.filter((session) => session.messages.length > 0);
    const messages = activeSession?.messages || [];
    const hasMessages = messages.length > 0;
    const selectedNodeKey = useMemo(() => Array.from(selectedNodeIds).sort().join(","), [selectedNodeIds]);

    useEffect(() => {
        if (view !== "chat") return;
        const frame = window.requestAnimationFrame(() => {
            const element = messageListRef.current;
            if (element) element.scrollTop = element.scrollHeight;
        });
        return () => window.cancelAnimationFrame(frame);
    }, [messages, view]);
    const allSelectedReferences = useMemo(() => buildAssistantReferences(nodes, selectedNodeIds), [nodes, selectedNodeIds]);
    const selectedReferences = useMemo(() => allSelectedReferences.filter((item) => !removedReferenceIds.has(item.id)), [allSelectedReferences, removedReferenceIds]);
    const iconButtonStyle = { color: theme.node.muted };
    const settleDeleteConfirmation = (confirmed: boolean) => {
        const pending = pendingDeleteRef.current;
        if (!pending) return;
        pendingDeleteRef.current = null;
        setPendingDelete(null);
        pending.resolve(confirmed);
    };

    useEffect(() => {
        setRemovedReferenceIds(new Set());
    }, [selectedNodeKey]);

    const commitSessions = (nextSessions: CanvasAssistantSession[], nextActiveSessionId = activeSessionIdRef.current) => {
        sessionsRef.current = nextSessions;
        activeSessionIdRef.current = nextActiveSessionId;
        onSessionsChange(nextSessions, nextActiveSessionId);
    };

    const updateSession = (sessionId: string, updater: (session: CanvasAssistantSession) => CanvasAssistantSession) => {
        commitSessions(sessionsRef.current.map((session) => (session.id === sessionId ? updater(session) : session)));
    };

    const appendMessage = (sessionId: string, message: CanvasAssistantMessage) => {
        updateSession(sessionId, (session) => ({
            ...session,
            title: session.messages.length ? session.title : message.text.slice(0, 18) || "新对话",
            messages: [...session.messages, message],
            updatedAt: new Date().toISOString(),
        }));
    };

    const updateMessage = (sessionId: string, messageId: string, patch: Partial<CanvasAssistantMessage>) => {
        updateSession(sessionId, (session) => ({
            ...session,
            messages: session.messages.map((message) => (message.id === messageId ? { ...message, ...patch } : message)),
            updatedAt: new Date().toISOString(),
        }));
    };

    const startChatSession = () => {
        if (activeSession && activeSession.messages.length === 0) {
            commitSessions(sessionsRef.current, activeSession.id);
            return;
        }
        const session = createSession();
        commitSessions([session, ...sessionsRef.current], session.id);
    };

    const removeSessions = (ids: string[]) => {
        const next = safeSessions.filter((session) => !ids.includes(session.id));
        if (!next.length) {
            const session = createSession();
            commitSessions([session], session.id);
        } else {
            const currentActiveSessionId = activeSessionIdRef.current;
            commitSessions(next, currentActiveSessionId && ids.includes(currentActiveSessionId) ? next[0].id : currentActiveSessionId);
        }
        cleanupImages({ sessions: next });
        setCheckedChatIds((previous) => previous.filter((id) => !ids.includes(id)));
    };

    const clearSessions = () => {
        const session = createSession();
        commitSessions([session], session.id);
        setCheckedChatIds([]);
        cleanupImages({ sessions: [session] });
    };

    const sendMessage = async (text: string, savedReferences?: CanvasAssistantReference[], autoPilotMode?: AutoPilotMode) => {
        const session = activeSession || createSession();
        if (!activeSession) {
            commitSessions([session], session.id);
        }

        const references = savedReferences || selectedReferences;
        const messageReferenceNodeIds = references.map((reference) => reference.id);
        const userMessage: CanvasAssistantMessage = { id: nanoid(), role: "user", text, references, status: "success" };
        const assistantId = nanoid();
        appendMessage(session.id, userMessage);
        appendMessage(session.id, { id: assistantId, role: "assistant", text: "", status: "thinking", activity: "正在理解画布和创作目标" });
        setPrompt("");

        // 文本模型与渠道优先用画布内「Agent 模型」下拉选中的（agentConfig），
        // 再回退到全局生效配置里的 textModel / textChannelId（全局配置弹窗已移除独立文本模型选择，
        // 故画布内选择才是用户实际可操作入口）。
        const agentTextChannelId = agentConfig.textChannelId || effectiveConfig.textChannelId || effectiveConfig.activeChannelId;
        let agentTextModel = agentConfig.textModel || effectiveConfig.textModel || effectiveConfig.model || effectiveConfig.textModels?.[0] || "";

        const buildRequestConfig = (model: string) => ({
            ...effectiveConfig,
            model,
            textModel: model,
            activeChannelId: agentTextChannelId,
            textChannelId: agentTextChannelId,
        });

        if (!isAiConfigReady(buildRequestConfig(agentTextModel), agentTextModel)) {
            // 区分「模型不可用 / 渠道不可用 / 根本未配置」三种情况，给出对应的诊断提示；
            // 避免「下拉里选过还被拦」这种与下拉展示不一致的体验。
            const selectable = selectableModelChannels(effectiveConfig);
            const modelInAnyChannel = agentTextModel && selectable.some((c) => (c.models || []).includes(agentTextModel));
            const userPickedExplicitly = Boolean(agentConfig.textModel);

            if (!modelInAnyChannel) {
                if (!userPickedExplicitly) {
                    // 兜底：自动选第一个可调用的文本模型，避免无意义的拦截。
                    const fallback = (effectiveConfig.textModels || []).find((m) => isAiConfigReady(buildRequestConfig(m), m));
                    if (fallback) {
                        agentTextModel = fallback;
                    } else {
                        updateMessage(session.id, assistantId, {
                            text: "尚未配置可用的文本模型。请在输入框旁的「Agent 模型」下拉中选择一个文本模型，或先在全局配置里完成模型与渠道配置。",
                            status: "error",
                            activity: undefined,
                        });
                        return;
                    }
                } else {
                    updateMessage(session.id, assistantId, {
                        text: `已选的 Agent 文本模型「${agentTextModel}」已不可用（可能被管理员下架），请重新选择一个。`,
                        status: "error",
                        activity: undefined,
                    });
                    return;
                }
            } else {
                // 模型在某个渠道里，但渠道本身不可调用（一般是本地直连缺 apiKey）。
                updateMessage(session.id, assistantId, {
                    text: `已选的 Agent 文本模型「${agentTextModel}」所在渠道缺少 API Key 或配置不完整，请在全局配置里补全后再继续。`,
                    status: "error",
                    activity: undefined,
                });
                return;
            }
        }

        const requestConfig = buildRequestConfig(agentTextModel);

        // 如果指定了 autoPilot 模式，更新 agentState
        const initialState = autoPilotMode 
            ? { ...session.agentState, autoPilot: autoPilotMode }
            : session.agentState;

        const controller = new AbortController();
        abortControllersRef.current.set(assistantId, controller);
        setActiveMessageIds((prev) => new Set(prev).add(assistantId));
        try {
            const modelReferences = await Promise.all(
                references.map(async (reference) => {
                    if (!reference.dataUrl) return reference;
                    try {
                        return { ...reference, dataUrl: await imageToDataUrl(reference) };
                    } catch {
                        return reference;
                    }
                }),
            );
            const result = await runCanvasAgent({
                config: requestConfig,
                initialState,
                protocolMessages: session.protocolMessages,
                userText: text,
                references: modelReferences,
                getContext: getAgentContext,
                executeAction: async (action) => {
                    if (action.name !== "delete_node") return onExecuteAction(action, messageReferenceNodeIds);
                    const nodeId = typeof action.arguments.nodeId === "string" ? action.arguments.nodeId : "";
                    const node = nodes.find((item) => item.id === nodeId);
                    const confirmed = await new Promise<boolean>((resolve) => {
                        const pending = { title: node?.title || "未命名节点", resolve };
                        pendingDeleteRef.current = pending;
                        setPendingDelete(pending);
                    });
                    return confirmed ? onExecuteAction(action, messageReferenceNodeIds) : { ok: false, code: "delete_cancelled", message: "用户取消删除，原节点已保留" };
                },
                signal: controller.signal,
                onEvent: (event) => updateMessage(session.id, assistantId, { status: event.status, activity: event.label }),
                onCheckpoint: (checkpoint) =>
                    updateSession(session.id, (current) => ({
                        ...current,
                        agentState: checkpoint.state,
                        protocolMessages: checkpoint.protocolMessages,
                        updatedAt: new Date().toISOString(),
                    })),
            });
            updateSession(session.id, (current) => ({
                ...current,
                agentState: result.state,
                protocolMessages: result.protocolMessages,
                messages: current.messages.map((message) =>
                    message.id === assistantId ? { ...message, text: result.reply, status: "success", activity: undefined } : message,
                ),
                updatedAt: new Date().toISOString(),
            }));
        } catch (error) {
            const stopped = error instanceof Error && error.name === "AbortError";
            updateMessage(session.id, assistantId, {
                text: stopped ? "已停止继续执行。已经创建的节点和已经提交的媒体任务会保留。" : error instanceof Error ? error.message : "Agent 执行失败",
                status: stopped ? "waiting" : "error",
                activity: undefined,
            });
            // 停止/出错后强制落盘：让「已创建的节点会保留」的承诺与实际持久化状态一致。
            // 延迟到下一宏任务，等 React 把最近一次 setNodes 同步进 store 后再 flush。
            if (stopped) {
                setTimeout(() => flushCanvasPersistence(), 0);
            }
        } finally {
            abortControllersRef.current.delete(assistantId);
            setActiveMessageIds((prev) => {
                const next = new Set(prev);
                next.delete(assistantId);
                return next;
            });
        }
    };

    const sendMessageRef = useRef(sendMessage);
    sendMessageRef.current = sendMessage;

    useEffect(() => {
        if (!initialRequest || consumedInitialRequestRef.current === initialRequest) return;
        consumedInitialRequestRef.current = initialRequest;
        onInitialRequestConsumed?.();
        void sendMessageRef.current(initialRequest.prompt, initialRequest.references);
    }, [initialRequest, onInitialRequestConsumed]);

    const submit = async () => {
        const text = prompt.trim();
        if (!text) return;
        await sendMessage(text);
    };

    const retryMessage = (message: CanvasAssistantMessage) => {
        const index = messages.findIndex((item) => item.id === message.id);
        const user = messages.slice(0, index).findLast((item) => item.role === "user");
        if (user) void sendMessage(user.text, user.references);
    };

    const startResize = () => {
        const move = (event: MouseEvent) => onWidthChange(Math.min(760, Math.max(320, window.innerWidth - event.clientX)));
        const stop = () => {
            setResizing(false);
            document.body.style.cursor = "";
            document.body.style.userSelect = "";
            document.removeEventListener("mousemove", move);
            document.removeEventListener("mouseup", stop);
        };
        setResizing(true);
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";
        document.addEventListener("mousemove", move);
        document.addEventListener("mouseup", stop);
    };

    const collapse = () => {
        onCollapseStart();
        window.setTimeout(onCollapse, PANEL_MOTION_MS);
    };

    return (
        <motion.div
            className="flex shrink-0"
            initial={{ width: 0, opacity: 0 }}
            animate={{ width: collapsed ? 0 : width + 1, opacity: collapsed ? 0 : 1 }}
            transition={{ duration: resizing ? 0 : PANEL_MOTION_SECONDS, ease: [0.22, 1, 0.36, 1] }}
            style={{ overflow: "clip", pointerEvents: collapsed ? "none" : undefined }}
        >
            <motion.aside
                className="relative flex shrink-0 flex-col border-l"
                initial={{ x: 48 }}
                animate={{ x: collapsed ? 28 : 0 }}
                transition={{ duration: resizing ? 0 : PANEL_MOTION_SECONDS, ease: [0.22, 1, 0.36, 1] }}
                style={{ width, background: theme.node.panel, borderColor: theme.node.stroke, color: theme.node.text }}
            >
                <button type="button" className="absolute inset-y-0 left-0 z-40 w-4 -translate-x-1/2 cursor-col-resize" onPointerDown={(event) => event.stopPropagation()} onMouseDown={startResize} aria-label="调整右侧面板宽度" />
                <div className="flex items-center justify-between border-b px-4 py-3" style={{ borderColor: theme.node.stroke }}>
                    <div className="flex items-center gap-2 text-sm font-medium">
                        <Bot className="size-4" />
                        {view === "history" ? "历史记录" : "创作 Agent"}
                    </div>
                    <div className="flex items-center gap-1">
                        {view === "history" ? (
                            <>
                                <Tooltip title="删除选中">
                                    <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<Trash2 className="size-4" />} disabled={!checkedChatIds.length} onClick={() => setDeleteChatIds(checkedChatIds)} />
                                </Tooltip>
                                <Tooltip title="删除全部">
                                    <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<X className="size-4" />} disabled={!historySessions.length} onClick={() => setDeleteChatIds(historySessions.map((session) => session.id))} />
                                </Tooltip>
                            </>
                        ) : null}
                        <Tooltip title={view === "history" ? "返回对话" : "历史记录"}>
                            <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<History className="size-4" />} onClick={() => setView(view === "history" ? "chat" : "history")} />
                        </Tooltip>
                        <Tooltip title="新对话">
                            <Button
                                type="text"
                                shape="circle"
                                className="!h-8 !w-8 !min-w-8"
                                style={iconButtonStyle}
                                icon={<Plus className="size-4" />}
                                disabled={!hasMessages}
                                onClick={() => {
                                    startChatSession();
                                    setView("chat");
                                }}
                            />
                        </Tooltip>
                        <Tooltip title="收起对话">
                            <Button type="text" shape="circle" className="!h-8 !w-8 !min-w-8" style={iconButtonStyle} icon={<PanelRightClose className="size-4" />} onClick={collapse} />
                        </Tooltip>
                    </div>
                </div>

                <div ref={messageListRef} className="thin-scrollbar min-h-0 flex-1 overflow-y-auto px-4 py-4">
                    {view === "history" ? (
                        <AssistantHistory
                            sessions={historySessions}
                            activeSession={activeSession}
                            checkedIds={checkedChatIds.filter((id) => historySessions.some((session) => session.id === id))}
                            onToggleChecked={(id, checked) => setCheckedChatIds((previous) => (checked ? [...new Set([...previous, id])] : previous.filter((item) => item !== id)))}
                            onOpen={(id) => {
                                commitSessions(sessionsRef.current, id);
                                setView("chat");
                            }}
                            onDelete={(id) => setDeleteChatIds([id])}
                        />
                    ) : messages.length ? (
                        <AssistantMessages messages={messages} onRetry={retryMessage} />
                    ) : (
                        <AssistantEmptyState />
                    )}
                </div>

                {view === "chat" ? (
                    <>
                        {pendingDelete ? (
                            <div className="mx-2 mb-2 overflow-hidden rounded-xl border" style={{ background: theme.node.fill, borderColor: theme.node.stroke }}>
                                <div className="min-w-0 px-3 py-2.5">
                                    <div className="truncate text-sm font-medium">删除「{pendingDelete.title}」？</div>
                                    <div className="mt-0.5 text-xs opacity-55">相关连线和任务记录将按现有逻辑清理</div>
                                </div>
                                <div className="grid grid-cols-2 border-t" style={{ borderColor: theme.node.stroke }}>
                                    <button type="button" className="h-9 cursor-pointer border-0 bg-transparent text-sm" style={{ color: theme.node.text }} onClick={() => settleDeleteConfirmation(false)}>取消</button>
                                    <button type="button" className="h-9 cursor-pointer border-0 border-l bg-transparent text-sm font-medium" style={{ borderColor: theme.node.stroke, color: "#ef4444" }} onClick={() => settleDeleteConfirmation(true)}>确认删除</button>
                                </div>
                            </div>
                        ) : null}
                        {!hasMessages ? <SuggestionPrompts onPick={(text) => setPrompt(text)} /> : null}
                        <CanvasAssistantComposer
                            prompt={prompt}
                            isRunning={activeMessageIds.size > 0}
                            references={selectedReferences}
                            agentConfig={agentConfig}
                            onAgentConfigChange={onAgentConfigChange}
                            onPromptChange={setPrompt}
                            onSubmit={submit}
                            onStop={() => {
                                settleDeleteConfirmation(false);
                                abortControllersRef.current.forEach((controller) => controller.abort());
                                abortControllersRef.current.clear();
                                setActiveMessageIds(new Set());
                            }}
                            onOpenUpload={onOpenUpload}
                            onOpenAssets={onOpenAssets}
                            onRemoveReference={(id) => {
                                setRemovedReferenceIds((previous) => new Set(previous).add(id));
                                if (selectedNodeIds.has(id)) onSelectNodeIds(new Set(Array.from(selectedNodeIds).filter((nodeId) => nodeId !== id)));
                            }}
                            onPasteImage={onPasteImage}
                        />
                    </>
                ) : null}

                <Modal
                    title="删除对话记录？"
                    open={deleteChatIds.length > 0}
                    centered
                    onCancel={() => setDeleteChatIds([])}
                    footer={
                        <>
                            <Button onClick={() => setDeleteChatIds([])}>取消</Button>
                            <Button
                                danger
                                type="primary"
                                onClick={() => {
                                    deleteChatIds.length === historySessions.length ? clearSessions() : removeSessions(deleteChatIds);
                                    setDeleteChatIds([]);
                                }}
                            >
                                删除
                            </Button>
                        </>
                    }
                >
                    <p className="text-sm opacity-60">将删除 {deleteChatIds.length} 条对话记录，此操作不可撤销</p>
                </Modal>
            </motion.aside>
        </motion.div>
    );
}

const ASSISTANT_MARKDOWN_COMPONENTS: Components = {
    a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" className="font-medium underline underline-offset-4" />,
};

function AssistantMarkdown({ children }: { children: string }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <div
            className={cn(
                "min-w-0 whitespace-normal break-words",
                "[&_p]:my-2 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0",
                "[&_h1]:mb-2 [&_h1]:mt-4 [&_h1]:text-lg [&_h1]:font-semibold [&_h1:first-child]:mt-0",
                "[&_h2]:mb-2 [&_h2]:mt-4 [&_h2]:text-base [&_h2]:font-semibold [&_h2:first-child]:mt-0",
                "[&_h3]:mb-1.5 [&_h3]:mt-3 [&_h3]:font-semibold [&_h3:first-child]:mt-0",
                "[&_h4]:my-2 [&_h4]:font-semibold",
                "[&_ul]:my-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:my-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-1",
                "[&_blockquote]:my-2 [&_blockquote]:border-l-2 [&_blockquote]:border-[color:var(--agent-markdown-border)] [&_blockquote]:pl-3 [&_blockquote]:opacity-80",
                "[&_hr]:my-3 [&_hr]:border-0 [&_hr]:border-t [&_hr]:border-[color:var(--agent-markdown-border)]",
                "[&_code]:rounded [&_code]:bg-[var(--agent-markdown-surface)] [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em]",
                "[&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:bg-[var(--agent-markdown-surface)] [&_pre]:p-3",
                "[&_pre_code]:bg-transparent [&_pre_code]:p-0",
                "[&_table]:my-2 [&_table]:w-full [&_table]:border-collapse [&_th]:border-b [&_th]:border-[color:var(--agent-markdown-border)] [&_th]:px-2 [&_th]:py-1.5 [&_th]:text-left [&_td]:border-b [&_td]:border-[color:var(--agent-markdown-border)] [&_td]:px-2 [&_td]:py-1.5",
            )}
            style={
                {
                    "--agent-markdown-surface": theme.toolbar.itemHover,
                    "--agent-markdown-border": theme.node.stroke,
                } as CSSProperties
            }
        >
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={ASSISTANT_MARKDOWN_COMPONENTS} skipHtml>
                {children}
            </ReactMarkdown>
        </div>
    );
}

function MessageActions({
    message,
    onRetry,
    strokeColor,
}: {
    message: CanvasAssistantMessage;
    onRetry: (message: CanvasAssistantMessage) => void;
    strokeColor: string;
}) {
    const [copied, setCopied] = useState(false);

    // 复制 Agent 回复正文到剪贴板；优先用 navigator.clipboard，失败时回退到 execCommand。
    const handleCopy = async () => {
        const text = message.text || "";
        if (!text) return;
        try {
            if (navigator.clipboard?.writeText) {
                await navigator.clipboard.writeText(text);
            } else {
                const textarea = document.createElement("textarea");
                textarea.value = text;
                textarea.style.position = "fixed";
                textarea.style.opacity = "0";
                document.body.appendChild(textarea);
                textarea.select();
                document.execCommand("copy");
                document.body.removeChild(textarea);
            }
            setCopied(true);
            antMessage.info("已复制");
            window.setTimeout(() => setCopied(false), 1500);
        } catch {
            antMessage.error("复制失败，请手动选择文本复制");
        }
    };

    return (
        <div className="flex items-center gap-1.5">
            <Button
                shape="circle"
                size="small"
                style={{ borderColor: strokeColor }}
                icon={copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                onClick={handleCopy}
                title="复制"
            />
            <Button
                shape="circle"
                size="small"
                style={{ borderColor: strokeColor }}
                icon={<RotateCcw className="size-3.5" />}
                onClick={() => onRetry(message)}
                title="重试"
            />
        </div>
    );
}

function AssistantMessages({ messages, onRetry }: { messages: CanvasAssistantMessage[]; onRetry: (message: CanvasAssistantMessage) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <>
            {messages.map((message) => {
                const running = message.status === "thinking" || message.status === "running";
                return (
                    <div key={message.id} className={cn("flex flex-col gap-2", message.role === "user" ? "items-end" : "items-start")}>
                        {message.text ? (
                            <div
                                className="max-w-[88%] whitespace-pre-wrap rounded-2xl px-3 py-2 text-sm leading-6"
                                style={
                                    message.role === "user"
                                        ? { background: theme.toolbar.activeBg, color: theme.toolbar.activeText }
                                        : message.status === "error"
                                            ? { background: theme.node.fill, color: theme.node.text }
                                            : { background: theme.node.fill, color: theme.node.text }
                                }
                            >
                                {message.role === "assistant" ? (
                                    <div className="mb-1 flex items-center gap-1.5 text-xs opacity-60">
                                        <Bot className="size-3.5" />
                                        Agent
                                    </div>
                                ) : null}
                                {message.role === "assistant" ? <AssistantMarkdown>{message.text}</AssistantMarkdown> : message.text}
                            </div>
                        ) : null}
                        {message.references?.length ? <MessageReferences message={message} /> : null}
                        {running ? <ImageGenerationPending compact label={message.activity || "正在执行"} className="w-[250px] rounded-2xl border" /> : null}
                        {message.role === "assistant" && !running && message.text ? (
                            <MessageActions message={message} onRetry={onRetry} strokeColor={theme.node.stroke} />
                        ) : null}
                    </div>
                );
            })}
        </>
    );
}

function AssistantHistory({
    sessions,
    activeSession,
    checkedIds,
    onToggleChecked,
    onOpen,
    onDelete,
}: {
    sessions: CanvasAssistantSession[];
    activeSession: CanvasAssistantSession | null;
    checkedIds: string[];
    onToggleChecked: (id: string, checked: boolean) => void;
    onOpen: (id: string) => void;
    onDelete: (id: string) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <div className="space-y-1">
            {sessions.map((session) => (
                <div key={session.id} className="group flex items-center gap-2 rounded-lg px-2 py-1.5 transition" style={session.id === activeSession?.id ? { background: theme.node.fill } : undefined}>
                    <input type="checkbox" className="size-4" style={{ accentColor: theme.node.text }} checked={checkedIds.includes(session.id)} onChange={(event) => onToggleChecked(session.id, event.target.checked)} />
                    <button type="button" className="min-w-0 flex-1 text-left text-sm" onClick={() => onOpen(session.id)}>
                        <span className="block truncate">{session.title}</span>
                        <span className="text-xs opacity-50">{session.messages.length} 条消息</span>
                    </button>
                    <Button type="text" shape="circle" size="small" className="opacity-0 transition group-hover:opacity-100" icon={<Trash2 className="size-3.5" />} onClick={() => onDelete(session.id)} title="删除" />
                </div>
            ))}
        </div>
    );
}

function MessageReferences({ message }: { message: CanvasAssistantMessage }) {
    return (
        <div className={cn("flex max-w-[88%] flex-wrap gap-2", message.role === "user" ? "justify-end" : "justify-start")}>
            {message.references?.map((item) => <AssistantReferenceChip key={item.id} item={item} />)}
        </div>
    );
}

function nodeToReference(node: CanvasNodeData): CanvasAssistantReference | null {
    if (isCanvasImageNodeType(node.type) && node.metadata?.content) {
        return {
            id: node.id,
            type: node.type,
            title: node.title,
            dataUrl: node.metadata.content,
            storageKey: node.metadata.storageKey,
            mimeType: node.metadata.mimeType,
        };
    }
    if (node.type === CanvasNodeType.Text && node.metadata?.content) {
        return { id: node.id, type: node.type, title: node.title, text: node.metadata.content };
    }
    if ((node.type === CanvasNodeType.Video || node.type === CanvasNodeType.Audio) && node.metadata?.content) {
        return {
            id: node.id,
            type: node.type,
            title: node.title,
            url: node.metadata.content,
            storageKey: node.metadata.storageKey,
            mimeType: node.metadata.mimeType,
        };
    }
    return null;
}

function buildAssistantReferences(nodes: CanvasNodeData[], selectedNodeIds: Set<string>) {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    return Array.from(selectedNodeIds)
        .map((id) => nodeById.get(id))
        .filter((node): node is CanvasNodeData => Boolean(node))
        .map(nodeToReference)
        .filter((item): item is CanvasAssistantReference => Boolean(item));
}

function createSession(): CanvasAssistantSession {
    const now = new Date().toISOString();
    return {
        id: nanoid(),
        title: "新对话",
        messages: [],
        agentState: createCanvasAgentState(),
        protocolMessages: [],
        createdAt: now,
        updatedAt: now,
    };
}

const SUGGESTION_PROMPTS = [
    { id: "short-video", icon: Video, title: "创作短视频", prompt: "帮我创作一个 30 秒的短视频，主题是 XXX" },
    { id: "script", icon: FileText, title: "写脚本", prompt: "帮我写一个短视频故事脚本" },
    { id: "post-production", icon: Sparkles, title: "素材后期", prompt: "为当前画布上的素材加上转场和配乐" },
] as const;

function SuggestionPrompts({ onPick }: { onPick: (prompt: string) => void }) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <div className="px-3 pb-2">
            <div className="grid grid-cols-3 gap-2">
                {SUGGESTION_PROMPTS.map((item) => {
                    const Icon = item.icon;
                    return (
                        <button
                            key={item.id}
                            type="button"
                            onClick={() => onPick(item.prompt)}
                            className="flex flex-col items-start gap-1.5 rounded-xl border px-2.5 py-2 text-left transition hover:opacity-80"
                            style={{ background: theme.toolbar.panel, borderColor: theme.node.stroke }}
                        >
                            <span className="grid size-6 place-items-center rounded-md" style={{ background: theme.node.fill, color: theme.node.muted }}>
                                <Icon className="size-3.5" />
                            </span>
                            <span className="text-xs font-medium leading-tight">{item.title}</span>
                        </button>
                    );
                })}
            </div>
        </div>
    );
}

function AssistantEmptyState() {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];

    return (
        <div className="flex h-full flex-col items-center justify-center px-6 text-center">
            <div
                className="grid size-12 place-items-center rounded-2xl"
                style={{ background: theme.node.fill, color: theme.node.muted }}
            >
                <Bot className="size-6" />
            </div>
            <div className="mt-3 text-base font-medium">Hi，我是画布助手</div>
            <div className="mt-1.5 max-w-[260px] text-sm leading-6 opacity-65">
                告诉我你想做什么，我能帮你创建节点、连接素材、写脚本、做分镜、调整配置。
            </div>
            <div className="mt-3 text-xs opacity-40">选中画布节点会在输入框里以参考图加入</div>
        </div>
    );
}
