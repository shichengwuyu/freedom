"use client";

import {
    AlertCircle,
    ChevronsLeft,
    ChevronsRight,
    ChevronRight,
    ClipboardPaste,
    Download,
    FileText,
    ScrollText,
    FolderOpen,
    LoaderCircle,
    MoveDown,
    MoveUp,
    Pause,
    Pencil,
    Plus,
    RefreshCw,
    Settings2,
    Trash2,
    Upload,
    Video,
    Wand2,
    Coins,
    X,
    Users,
    LayoutGrid,
    Package,
    Play,
    Check,
    Copy,
    Camera,
    Search,
    Film as FilmIcon,
    AlertTriangle,
    UserCheck,
    UserX,
    Clapperboard,
    ImageIcon,
    HelpCircle,
    Sparkles,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { App, Button, Image, Input, Modal, Popover, Select, InputNumber, Progress, Tooltip, Switch } from "antd";
import { nanoid } from "nanoid";
import { saveAs } from "file-saver";

import { ModelPicker } from "@/components/model-picker";
import { BalanceSymbol, formatBalanceYuan, formatCostCents } from "@/constant/balance";
import { useEffectiveConfig, useConfigStore, channelIdForActiveModel, activeChannelKind, defaultConfig, type AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import {
    createVideoGenerationTask,
    pollVideoGenerationTaskStatus,
    listVideoGenerationTasks,
    isCompletedVideoStatus,
    isFailedVideoStatus,
    VIDEO_POLL_INTERVAL_MS,
} from "@/services/api/video";
import {
    createStoryboardTask,
    getStoryboardTask,
    listStoryboardTasks,
    parseStoryboardTaskResult,
    cancelStoryboardTask,
    isShotReady,
} from "@/services/api/storyboard_task";
// 复用与「视频创作台/Agent」一致的渠道解析与鉴权头，避免手写 URL/Header 漏掉渠道路由
import { aiApiUrl, aiHeaders, createCanvasImageTask, pollCanvasImageTaskStatus, listCanvasImageTasks, type CanvasImageTask } from "@/services/api/image";
import { supportsVideoFrameReferences, getModelMaxDuration } from "@/lib/video-model-capabilities";
import { resolveImageUrl, uploadImage } from "@/services/image-storage";
import type { ReferenceImage } from "@/types/image";
import { useAssetStore } from "@/stores/use-asset-store";
// 与后端 executeStoryboardTask 共用提示词构造（断网兜底才走这条路径）；改提示词请同步 service。
import { buildStoryboardAssetRefSection, buildStoryboardUserContent, cleanStoryboardLeadingLabel, renderStoryboardPrompt } from "@/lib/prompts/storyboard";

// ─────────────────────────── Types & Constants (extracted) ───────────────────────────

import type {
    AssetType,
    Asset,
    Shot,
    Chapter,
    Storyboard,
    NovelProject,
    PresetEntry,
    AutoPilotMode,
    StoryboardCoverageReport,
} from "./types";
import {
    DEFAULT_SCRIPT_PROMPT,
    DEFAULT_VIDEO_PROMPT,
    DEFAULT_ASSET_PROMPT,
    SCRIPT_VIDEO_SHARED,
    PRESETS,
    PRESET_KEYS,
    ASSET_TEMPLATES_FALLBACK,
    parseAssetTemplates,
    PROJECTS_STORE_KEY,
    ASSETS_STORE_KEY,
    ACTIVE_PROJECT_KEY,
    CHUNK_SIZE,
    LARGE_SCRIPT_THRESHOLD,
    CHAPTERS_PER_GROUP,
} from "./constants";
import { getEffectivePresetValue, matchPreset, validatePrompt } from "./prompt-presets";
import { parseVendorError, type VendorErrorLevel } from "./vendor-errors";
import { makePromptDynamic } from "./prompt-utils";
import {
    storyboardIsReady,
    extractMentions,
    extractStoryboardHeader,
    resolveMentions,
    validateStoryboardContent,
} from "./domain/extract";
import { parseChapters } from "./domain/parse-chapters";
import {
    loadProjects,
    saveProjects,
    loadAssets,
    saveAssets,
    loadActiveProjectId,
    saveActiveProjectId,
} from "./storage";
import { ConfigLabel, DetailMeta } from "./components/config-label";
import { VideoShotCard } from "./components/video-shot-card";
import { VideoShotDetail } from "./components/video-shot-detail";
import NovelWorkflowLayers from "./components/novel-workflow-layers";
import NovelSeriesAssetLockPanel from "./components/novel-series-asset-lock-panel";
import NovelBgmPicker from "./components/novel-bgm-picker";
import NovelRerunPanel from "./components/novel-rerun-panel";
import NovelCompositionView from "./components/novel-composition-view";
import {
  createNovelWorkflowRun,
  getNovelWorkflowRun,
  type NovelWorkflowRun,
  type NovelWorkflowNode,
} from "@/services/api/novel_workflow";
import { audioBufferToWav } from "./lib/audio-wav";
import { parseScriptToShots } from "./domain/parse-script-to-shots";
import { blobUrlToDataUrl, getVideoBlob, extractFrame } from "./lib/media-utils";

// ─────────────────────────── Component ───────────────────────────

export default function NovelPage() {
    const { message: antMessage, modal: antModal } = App.useApp();
    const effectiveConfig = useEffectiveConfig();
    const isAiConfigReady = useConfigStore((state) => state.isAiConfigReady);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const token = useUserStore((state) => state.token);
    const updateConfig = useConfigStore((state) => state.updateConfig);
    // 账号账户余额（未登录/本地直连为 null，不参与成本计算）
    const userBalanceCents = useUserStore((state) => state.user?.balanceCents ?? null);
    // 管理端配置的模型扣费单价（用于估算单个分镜视频成本）
    const modelCosts = useConfigStore((state) => state.publicSettings?.modelChannel.modelCosts);

    const [projects, setProjects] = useState<NovelProject[]>([]);
    const [activeProjectId, setActiveProjectId] = useState<string | null>(null);
    const [assets, setAssets] = useState<Asset[]>([]);

    // 弹窗开关
    const [showConfigModal, setShowConfigModal] = useState(false);
    const [showPromptModal, setShowPromptModal] = useState(false);
    const [showGuideModal, setShowGuideModal] = useState(false);
    // 左侧面板折叠
    const [leftCollapsed, setLeftCollapsed] = useState(false);
    // 项目重命名
    const [renamingId, setRenamingId] = useState<string | null>(null);
    const [renameValue, setRenameValue] = useState("");
    // 项目搜索关键词
    const [projectSearch, setProjectSearch] = useState("");

    // 三块独立提示词：
    // 优先从管理员在后台配置的 systemPrompts（effectiveConfig.systemPrompts 里的 storyboard* 字段）读取，
    // 空时回退到本文件写死的 DEFAULT_* 常量，保证用户即使不登录/未配置后台也有合理默认值。
    // 状态初始化先用代码默认值，等 useEffect 拿到 effectiveConfig 后再用后台值覆盖（仅一次，不覆盖用户自定义）。
    const promptsAppliedRef = useRef(false);
    const [scriptPrompt, setScriptPrompt] = useState(DEFAULT_SCRIPT_PROMPT);  // 分镜剧本提示词：小说→分镜
    const [videoPrompt, setVideoPrompt] = useState(DEFAULT_VIDEO_PROMPT);      // 分镜视频提示词：分镜→视频描述
    const [assetPrompt, setAssetPrompt] = useState(DEFAULT_ASSET_PROMPT);      // 资产生成提示词：角色/场景/道具生图
    // 弹窗草稿（按正在编辑的 tab 区分）
    const [promptDraft, setPromptDraft] = useState(DEFAULT_SCRIPT_PROMPT);
    const [promptDraftTab, setPromptDraftTab] = useState<"script" | "video" | "image">("script");
    // P1 改造 A：当前选中的预设 key（用户选预设后被记录；"custom" 表示已被用户手动改过）
    const [currentPresetKey, setCurrentPresetKey] = useState<string>("general");
    // P1 改造 B：记录注水时的后台值快照，用于检测后台是否更新（提示"⟳ 后台有更新待拉取"）
    const lastSyncedBackendRef = useRef<{ script: string; video: string; asset: string }>({ script: "", video: "", asset: "" });
    const [shotDuration, setShotDuration] = useState(15);
    const [aspectRatio, setAspectRatio] = useState("16:9");
    const [resolution, setResolution] = useState("720p");
    const [generateInParallel, setGenerateInParallel] = useState(false);
    // 一键出片三态：
    //   off  = 手动模式，每步独立按钮
    //   half = 自动链到分镜剧本完成后停下来弹窗确认是否继续生图
    //   full = 全自动链：开始分镜 → 抽资产 → 生图 → 校验所有 @角色 已绑定 → 出片
    // 严苛闸门（未匹配资源时是否阻止出片）由 allowMissingAssets 控制，默认阻断。
    const [autoPilot, setAutoPilot] = useState<AutoPilotMode>("off");
    /** 豁免开关：开时即使分镜里有 @角色 没绑资产也允许走无参考图模式出片。默认 false 严格。
     * 持久化到 localStorage，老用户刷新页面开关不丢（避免每次重开都得重新打开豁免）。
     */
    const [allowMissingAssets, setAllowMissingAssets] = useState<boolean>(() => {
        if (typeof window === "undefined") return false;
        try { return localStorage.getItem("novel:allow-missing-assets") === "1"; }
        catch { return false; }
    });
    useEffect(() => {
        if (typeof window === "undefined") return;
        try { localStorage.setItem("novel:allow-missing-assets", allowMissingAssets ? "1" : "0"); }
        catch { /* localStorage 不可写时不报错（隐私模式） */ }
    }, [allowMissingAssets]);

    /** 自动重试开关：分镜生成失败时是否自动重试（最多2次，带指数退避） */
    const [autoRetryEnabled, setAutoRetryEnabled] = useState<boolean>(() => {
        if (typeof window === "undefined") return true;
        try { return localStorage.getItem("novel:auto-retry") !== "0"; }
        catch { return true; }
    });
    useEffect(() => {
        if (typeof window === "undefined") return;
        try { localStorage.setItem("novel:auto-retry", autoRetryEnabled ? "1" : "0"); }
        catch { /* localStorage 不可写时不报错 */ }
    }, [autoRetryEnabled]);
    // 资产生成配置（按类型筛选需要自动生成哪些；autoPilot 开启时这些开关才生效）
    const [generateCharacterAssets, setGenerateCharacterAssets] = useState(true);
    const [generateSceneAssets, setGenerateSceneAssets] = useState(true);
    const [generatePropAssets, setGeneratePropAssets] = useState(false);
    const [assetGenerationRunning, setAssetGenerationRunning] = useState(false);
    /** 一站式"剧本转资产"按钮的运行态（合并提取+生图两步时的单独 busy 标记，避免与单步按钮的 loading 冲突） */
    const [scriptToAssetsBusy, setScriptToAssetsBusy] = useState(false);
    /** 剧本转资产三段子阶段：提示词 → 生图 → 对应分镜（仅 UI 展示，不引入新状态机） */
    const [assetStep, setAssetStep] = useState<"prompt" | "image" | "reconcile" | "">("");
    /** 剪贴板导入按钮的 busy 态 */
    const [clipboardBusy, setClipboardBusy] = useState(false);
    // 正在生成中的资产列表（用于显示loading状态）
    const [pendingAssets, setPendingAssets] = useState<{ id: string; name: string; type: AssetType; prompt: string; startTime: number }[]>([]);
    // 生成失败的资产列表（用于显示失败卡片）
    const [failedAssets, setFailedAssets] = useState<{ id: string; name: string; type: AssetType; error: string }[]>([]);
    // 资产生成使用的图片模型（独立于全局默认生图模型，资产弹窗内单独选择）
    const [assetImageModel, setAssetImageModel] = useState("");
    const [assetImageChannelId, setAssetImageChannelId] = useState("");
    // 资产生图通用画风/提示词后缀（追加到角色三视图/场景四宫格/道具提示词末尾）
    const [assetImageStylePrompt, setAssetImageStylePrompt] = useState("");
    // 分镜图片：文本模型提取 → 预览 → 图片模型生成 的两步流程
    // P1 改造 a2：每条加 sourceSnippet 让用户能追溯「这个 description 是从哪段原文抽的」
    // P1 改造 b3：每条加 isExisting 标注「同名资产已存在」，UI 显示「♻️ 已存在」chip，可「保留/重生图」切换
    const [extractedAssets, setExtractedAssets] = useState<Array<{ id: string; name: string; type: AssetType; description: string; sourceSnippet?: string; isExisting?: boolean }>>([]);
    const [extractedStyle, setExtractedStyle] = useState("");
    const [extractingAssets, setExtractingAssets] = useState(false);
    const [extractedListExpanded, setExtractedListExpanded] = useState(false);
    const [stylePromptExpanded, setStylePromptExpanded] = useState(false);
    // P1 改造 a3：资产库搜索关键词（按 alias / name / description 全文搜索；切换 Tab 时保留）
    const [assetSearch, setAssetSearch] = useState("");
    // 提取资产详情弹窗：点卡片查看 / 编辑完整 prompt（系统模板 + 描述 + 画风后缀）
    const [viewAssetId, setViewAssetId] = useState<string | null>(null);
    const [assetDraftName, setAssetDraftName] = useState("");
    const [assetDraftDescription, setAssetDraftDescription] = useState("");
    const [assetDraftType, setAssetDraftType] = useState<AssetType>("character");

    const [pipelineRunning, setPipelineRunning] = useState(false);
    const [pipelineStatus, setPipelineStatus] = useState("");


    // pipelinePhase: 4 段流水线阶段标识，配合底部 stepper 渲染。
// chapter_parse = 章节解析（parseChapters 是同步、几乎瞬时，此阶段可视化靠 UI 自适应完成动画）
// storyboard    = 文本模型逐章生成分镜剧本
// assets        = 抽角色/场景/道具，并按图片模型生图
// video         = 按每条分镜剧本（带 referencedAssetIds）生视频
// done / idle   = 收尾 / 重置
const [pipelinePhase, setPipelinePhase] = useState<"idle" | "chapter_parse" | "storyboard" | "assets_prompt" | "assets_image" | "assets_reconcile" | "video" | "done">("idle");
    // ── 流水线阶段：唯一出口（chokepoint）──
    // 所有阶段切换必须经 setPhase，不要直接调 setPipelinePhase，避免散落 15+ 处导致 stepper 阶段错位、难以审计。
    const setPhase = (next: "idle" | "chapter_parse" | "storyboard" | "assets_prompt" | "assets_image" | "assets_reconcile" | "video" | "done") => {
        setPipelinePhase(next);
    };
    const [abortController, setAbortController] = useState<AbortController | null>(null);
    // 分镜视频批量生成时的 abort controller（用于"停止"按钮）
    const [storyboardVideoAbortController, setStoryboardVideoAbortController] = useState<AbortController | null>(null);
    const [showRewriteResult, setShowRewriteResult] = useState(false);
    const [rewrittenScript, setRewrittenScript] = useState("");
    const [pipelineProgress, setPipelineProgress] = useState({ current: 0, total: 0 });
    const pipelineIdRef = useRef(0);

    const [assetTab, setAssetTab] = useState<AssetType>("character");
    // 资产网格列数：2列（大图）或3列（紧凑）
    const [assetGridCols, setAssetGridCols] = useState<2 | 3>(() => {
        if (typeof window === "undefined") return 3;
        try { return (localStorage.getItem("novel:asset-grid-cols") === "2" ? 2 : 3) as 2 | 3; }
        catch { return 3; }
    });
    useEffect(() => {
        if (typeof window === "undefined") return;
        try { localStorage.setItem("novel:asset-grid-cols", String(assetGridCols)); }
        catch { /* localStorage 不可写时不报错 */ }
    }, [assetGridCols]);
    // 素材编辑（导入后逐张改名/别名/描述）
    const [editingAssetId, setEditingAssetId] = useState<string | null>(null);
    const [editAssetName, setEditAssetName] = useState("");
    const [editAssetAlias, setEditAssetAlias] = useState("");
    const [editAssetDesc, setEditAssetDesc] = useState("");
    const [checkingScenes, setCheckingScenes] = useState(false);
    const [sceneCheckResult, setSceneCheckResult] = useState<{ missing: string[]; matched: string[] } | null>(null);
    // 缺参考图持久化 + 视频生成前二次确认：
    //   当 generateVideoFromStoryboard 检测到分镜剧本里 @角色 在素材库找不到对应 alias 时，
    //   不再只 toast 一次，而是把 unmatched 暂存到 pendingShotRun → 弹 Modal 让用户「仍然继续」或「去上传素材」。
    //   用户选「仍然继续」才真正执行生成；选「去上传素材」则跳到素材库页签让用户上传。
    const [pendingShotRun, setPendingShotRun] = useState<{
        storyboardId: string;
        unmatched: string[];
        linkedIds: string[];
        storyboardContent: string;
        onSettled?: (ok: boolean) => void;
    } | null>(null);

    // 自动链 preview 闸：抽完资产后弹 Modal 让用户「确认生成 / 取消跳过」。
    //   复用 extractedAssets state 让用户能编辑/删除；用户选「确认生成」才调 generateImagesFromExtracted，
    //   然后接 generateAllStoryboardVideos 把视频生成跑完。
    const [pendingAssetPreview, setPendingAssetPreview] = useState<{
        boards: Storyboard[];
    } | null>(null);
    // P1 改造 A：分镜剧本预览闸（手动路径），用户先看分镜跑得好不好，再决定走资产闸
    //   数据结构：存每条 boardId 的状态（是否被「编辑/重新生成/跳过」），以及 boards 列表原样
    const [pendingStoryboardPreview, setPendingStoryboardPreview] = useState<{
        entries: { boardId: string; snapshot: Storyboard }[];
    } | null>(null);
    // 懒加载：大剧本只渲染可视区域的行
    const [scriptVisibleLines, setScriptVisibleLines] = useState(200);
    // 视频网格布局：list | grid
    const [videoLayout, setVideoLayout] = useState<"list" | "grid">("grid");
    const [videoGridCol, setVideoGridCol] = useState(4);
    // 分镜视频卡片是否紧凑显示（只显缩略图+角标，不显标题/操作行，可塞更多）
    const [videoCompact, setVideoCompact] = useState(true);
    // 视频详情弹窗
    const [detailShotId, setDetailShotId] = useState<string | null>(null);
    // 中间栏：章节目录中展开的章节
    const [expandedChapterIds, setExpandedChapterIds] = useState<Set<string>>(new Set());
    // 全局搜索关键词（同时搜索章节、分镜、视频，清空后恢复各自独立搜索）
    const [globalSearch, setGlobalSearch] = useState("");
    // 章节搜索关键词（按标题/正文匹配，用于定位章节）
    const [chapterSearch, setChapterSearch] = useState("");
    // 分镜剧本搜索关键词（按正文匹配定位，也支持"分镜6"或"6"定位到第N条）
    const [storyboardSearch, setStoryboardSearch] = useState("");
    // 分镜视频搜索关键词（按标题/正文匹配定位）
    const [shotSearch, setShotSearch] = useState("");
    // 开始分镜时勾选要处理的章（每章一条）；空 = 全部章。用于"只分部分章节，不必全书分完"
    const [selectedGroups, setSelectedGroups] = useState<Set<number>>(new Set());
    // P1 b1/b2：每章 shotCount（默认 1；>=2 时后端 prompt 引导模型输出 N 条 ===SHOT=== 分隔）
    const [chapterShotCounts, setChapterShotCounts] = useState<Record<number, number>>({});
    // 分镜剧本批量生成视频时勾选的分镜 id 集合（空 = 全部）
    const [selectedStoryboardIds, setSelectedStoryboardIds] = useState<Set<string>>(new Set());
    // 分镜剧本列表容器 ref，用于搜索定位时滚动
    const storyboardListRef = useRef<HTMLDivElement>(null);
    // 章节目录列表容器 ref
    const chapterListRef = useRef<HTMLDivElement>(null);
    // 分镜视频列表容器 ref
    const shotListRef = useRef<HTMLDivElement>(null);
    // 全局搜索命中的首个元素 ref（用于滚动定位）
    const globalSearchResultRef = useRef<HTMLDivElement>(null);
    // 已匹配的分镜 id（用于滚动定位）
    const [matchedStoryboardId, setMatchedStoryboardId] = useState<string | null>(null);
    // 编辑原文弹窗（章节目录下不再有"编辑/目录"切换，改为弹窗编辑原始小说）
    const [showScriptEditor, setShowScriptEditor] = useState(false);
    // 编辑原文弹窗内的草稿值
    const [scriptDraft, setScriptDraft] = useState("");
    // 右侧栏：查看/编辑某个分镜剧本的弹窗
    const [viewStoryboardId, setViewStoryboardId] = useState<string | null>(null);
    // 分镜剧本编辑草稿（查看弹窗内编辑）
    const [storyboardDraft, setStoryboardDraft] = useState("");
    // 分镜编辑弹窗内的分组选择
    const [storyboardGroupDraft, setStoryboardGroupDraft] = useState<number>(0);
    // 解析分镜运行态
    const [parsingStoryboard, setParsingStoryboard] = useState(false);
    // 手动新增分镜剧本时选择的分组（null = 最后一组）
    const [addStoryboardGroupIndex, setAddStoryboardGroupIndex] = useState<number | null>(null);
    // 分镜剧本（组）并发上限：勾选多组时，多组并发发给文本模型改写，跑完按组顺序归位。默认 3（文本模型限流更敏感）。
    const [storyboardConcurrency, setStoryboardConcurrency] = useState(3);
    // 视频生成并发上限（视频并发，超出上限的排队，跑完一个自动补位）。默认 5。
    const [videoConcurrency, setVideoConcurrency] = useState(5);
    // 资产生图并发上限：图片生成调用并发数。默认 3（保守，避免 rolldek/apimart 触限）；可在配置弹窗拉到 8。
    // 串行时 7 张图 ≈ 100s（按 15s/张估算），改 3 并发后 ≈ 30-45s；拉满 8 并发 ≈ 15-20s 但有触限风险。
    const [imageConcurrency, setImageConcurrency] = useState(3);

    const pollingTimersRef = useRef<Map<string, ReturnType<typeof setInterval>>>(new Map());
    // 分镜任务轮询定时器（后端任务化后，前端轮询恢复进度用）
    const storyboardPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
    // 视频并发闸门：running=当前在跑的数量，queue=等待中的任务；每个任务在"完成/失败/出错"任一终态释放并补位
    const videoGateRef = useRef<{ running: number; queue: Array<() => void> }>({ running: 0, queue: [] });
    const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const scriptScrollRef = useRef<HTMLTextAreaElement>(null);
    // projectsRef：始终保持最新 projects 引用，供轮询回调（闭包外）读取最新分镜 id 触发自动视频生成
    const projectsRef = useRef<NovelProject[]>([]);
    useEffect(() => { projectsRef.current = projects; }, [projects]);

    // 视频模型最大时长（用于 b5 警告：shotDuration > maxDuration 会截断）
    const videoModelMaxDuration = getModelMaxDuration(effectiveConfig.videoModel || defaultConfig.videoModel || "");
    const shotDurationExceedsLimit = !!videoModelMaxDuration && shotDuration > videoModelMaxDuration;
    const videoModel = effectiveConfig.videoModel || defaultConfig.videoModel;

    const supportsFrameRefs = useMemo(
        () => supportsVideoFrameReferences(videoModel),
        [videoModel],
    );
    // 单个分镜视频的余额成本（remote 计费且模型有配置时才有值，按秒计费 = 每秒点数 × 时长）
    const shotCredit = useMemo(
        () => formatCostCents({ channelMode: activeChannelKind({ ...effectiveConfig, model: videoModel }), modelCosts, model: videoModel, seconds: shotDuration }),
        [effectiveConfig, modelCosts, videoModel, shotDuration],
    );

    useEffect(() => {
        // 持久化修复：加载项目列表与上次选中的项目 ID，恢复选中态；若该 ID 已不存在则自动选中第一个项目
        void (async () => {
            const [loaded, savedActiveId] = await Promise.all([loadProjects(), loadActiveProjectId()]);
            setProjects(loaded);
            const restoreId = savedActiveId && loaded.some((p) => p.id === savedActiveId)
                ? savedActiveId
                : (loaded[0]?.id ?? null);
            setActiveProjectId(restoreId);
        })();
        void loadAssets().then(async (loadedAssets) => {
            // 修复：检测并转换 blob URL（blob URL 在页面刷新后会失效）
            let needsFix = false;
            const fixedAssets = await Promise.all(
                loadedAssets.map(async (a) => {
                    if (a.dataUrl && a.dataUrl.startsWith("blob:")) {
                        needsFix = true;
                        try {
                            const response = await fetch(a.dataUrl);
                            const blob = await response.blob();
                            const dataUrl = await new Promise<string>((resolve) => {
                                const reader = new FileReader();
                                reader.onload = () => resolve(reader.result as string);
                                reader.onerror = () => resolve(a.dataUrl);
                                reader.readAsDataURL(blob);
                            });
                            return { ...a, dataUrl };
                        } catch {
                            return { ...a, dataUrl: "" }; // 转换失败则清空，避免显示错误
                        }
                    }
                    return a;
                })
            );
            setAssets(fixedAssets);
            if (needsFix) void saveAssets(fixedAssets); // 持久化修复后的资产
        });
    }, []);

    // 持久化修复：activeProjectId 变化时同步落盘，保证刷新后能回到上次操作的项目
    useEffect(() => {
        void saveActiveProjectId(activeProjectId);
    }, [activeProjectId]);

    // ── 把管理员在后台配置的分镜系统提示词注入本地三块独立提示词（首次灌入，后续追踪更新） ──
    // P1 改造 B：原来一次性灌入（promptsAppliedRef.current = true）会吞掉后台改动，
    //   现在改为每次后台 systemPrompts 变化都跟 lastSyncedBackendRef 对比，
    //   用于 UI 上判定"⟳ 后台有更新待拉取"。
    useEffect(() => {
        const sp = effectiveConfig.systemPrompts?.storyboardScript?.trim() ?? "";
        const vp = effectiveConfig.systemPrompts?.storyboardVideo?.trim() ?? "";
        const ip = effectiveConfig.systemPrompts?.storyboardImage?.trim() ?? "";
        const snapshot = { script: sp, video: vp, asset: ip };
        // 首次灌入（保留旧行为）：用户从未改过 local 时 → 用后台值覆盖 local
        if (!promptsAppliedRef.current) {
            const anyConfigured = Boolean(sp || vp || ip);
            if (anyConfigured) {
                if (sp) setScriptPrompt((prev) => prev === DEFAULT_SCRIPT_PROMPT ? sp : prev);
                if (vp) setVideoPrompt((prev) => prev === DEFAULT_VIDEO_PROMPT ? vp : prev);
                if (ip) setAssetPrompt((prev) => prev === DEFAULT_ASSET_PROMPT ? ip : prev);
            }
            lastSyncedBackendRef.current = snapshot;
            promptsAppliedRef.current = true;
            return;
        }
        // 后续每次系统值变化 → 仅更新 lastSyncedBackendRef，UI 据此显示同步徽标
        lastSyncedBackendRef.current = snapshot;
    }, [effectiveConfig.systemPrompts]);

    // 持久化修复：页面卸载前把防抖中的保存立即落盘（兜底，防止"导入后秒刷"防抖未触发就丢失）
    useEffect(() => {
        const handler = () => {
            if (saveTimerRef.current) {
                clearTimeout(saveTimerRef.current);
                saveTimerRef.current = null;
                void saveProjects(projects);
            }
        };
        window.addEventListener("beforeunload", handler);
        return () => window.removeEventListener("beforeunload", handler);
    }, [projects]);

    useEffect(() => {
        return () => {
            pollingTimersRef.current.forEach((t) => clearInterval(t));
            pollingTimersRef.current.clear();
            if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
            // 清理 storyboard 轮询定时器，防止卸载后继续 setState
            if (storyboardPollRef.current) {
                clearInterval(storyboardPollRef.current);
                storyboardPollRef.current = null;
            }
            // 组件卸载时清理 storyboard 视频生成状态
            setStoryboardVideoAbortController(null);
        };
    }, []);

    const activeProject = useMemo(
        () => projects.find((p) => p.id === activeProjectId) || null,
        [projects, activeProjectId],
    );



    // ── novel-workflow v2: workflow run / nodes 状态 ──
    const [novelRun, setNovelRun] = useState<NovelWorkflowRun | null>(null);
    const [novelNodes, setNovelNodes] = useState<NovelWorkflowNode[]>([]);
    const [novelRunId, setNovelRunId] = useState<string | null>(null);
    const [novelBg, setNovelBg] = useState<{ presetId?: string; customId?: string; volume: number; fadeInMs: number; fadeOutMs: number } | null>(null);

    // 加载或创建 novel workflow run (per-project 1 个 active run)
    const refreshNovelRun = useCallback(async () => {
      if (!activeProject) {
        setNovelRun(null);
        setNovelNodes([]);
        setNovelRunId(null);
        return;
      }
      try {
        const runs = await import("@/services/api/novel_workflow").then((m) =>
          m.listNovelWorkflowRuns(activeProject.id, 1, 5),
        );
        if (runs.runs.length > 0) {
          const r = runs.runs[0];
          setNovelRun(r);
          setNovelRunId(r.id);
          const data = await getNovelWorkflowRun(r.id);
          setNovelNodes(data.nodes);
        } else {
          const shotIds = (activeProject.shots || []).map((s) => s.id);
          if (shotIds.length > 0) {
            const r = await createNovelWorkflowRun({
              projectId: activeProject.id,
              mode: "auto",
              shotIds,
              configJson: "",
            });
            setNovelRun(r);
            setNovelRunId(r.id);
            setNovelNodes([]);
          } else {
            setNovelRun(null);
            setNovelNodes([]);
            setNovelRunId(null);
          }
        }
      } catch (e) {
        console.error("refresh novel run", e);
      }
    }, [activeProject]);

    useEffect(() => {
      refreshNovelRun();
    }, [refreshNovelRun]);
    const completedCount = useMemo(
        () => activeProject?.shots.filter((s) => s.status === "success").length || 0,
        [activeProject],
    );
    const failedCount = useMemo(
        () => activeProject?.shots.filter((s) => s.status === "failed").length || 0,
        [activeProject],
    );
    const generatingCount = useMemo(
        () => activeProject?.shots.filter((s) => s.status === "generating").length || 0,
        [activeProject],
    );
    const selectedCount = useMemo(
        () => activeProject?.shots.filter((s) => s.selected).length || 0,
        [activeProject],
    );

    // P1 改造 a3：资产库搜索过滤结果（按 alias / name / description 全文搜；不区分大小写；空串返回全部）
    const filteredAssets = useMemo(() => {
        if (!assetSearch.trim()) return null; // null = 不过滤
        const kw = assetSearch.trim().toLowerCase();
        return assets.filter((a) =>
            a.alias?.toLowerCase().includes(kw)
            || a.name?.toLowerCase().includes(kw)
            || a.description?.toLowerCase().includes(kw)
        );
    }, [assets, assetSearch]);
    const filteredFailedAssets = useMemo(() => {
        if (!assetSearch.trim()) return null;
        const kw = assetSearch.trim().toLowerCase();
        return failedAssets.filter((f) => f.name?.toLowerCase().includes(kw));
    }, [failedAssets, assetSearch]);
    const filteredPendingAssets = useMemo(() => {
        if (!assetSearch.trim()) return null;
        const kw = assetSearch.trim().toLowerCase();
        return pendingAssets.filter((p) => p.name?.toLowerCase().includes(kw));
    }, [pendingAssets, assetSearch]);

    // P1 改造 a4：preview modal 顶部信息条 —— 显示「本次生图会用」的图片模型 + 通道 + 风格后缀
    //  对未配置资产专用模型/通道时回退到全局默认值
    //  修复：不再回退到 effectiveConfig.model（文本模型），避免用文本模型生图
    const previewImageModel = (assetImageModel || effectiveConfig.imageModel || defaultConfig.imageModel || "").trim();
    const previewImageChannel = assetImageChannelId || effectiveConfig.imageChannelId || effectiveConfig.activeChannelId || "";
    const hasStyleHint = !!(assetImageStylePrompt || "").trim();

    // P1 改造 a5：跨章同名资产合并 summary
    //  抽完后传给图片模型的数量 vs 剧本里 @ 别名总出现次数
    //  节省 = 总引用次数 - 去重后独立资产数；负数时（罕见，按 > 0 显示）也不报错
    const dedupeStats = useMemo(() => {
        if (!pendingAssetPreview || extractedAssets.length === 0) return null;
        const boards = pendingAssetPreview.boards;
        const headerMentions = boards.flatMap((b) => [
            ...extractStoryboardHeader(b.content).characters,
            ...extractStoryboardHeader(b.content).scenes,
        ]);
        const atMentions = boards
            .flatMap((b) => Array.from(b.content.matchAll(/@([\u4e00-\u9fa5A-Za-z0-9_]+)/g)).map((m) => m[1]));
        // 道具不在抽取名单里，header 行的角色/场景已包含这些 alias
        const total = headerMentions.length + atMentions.length;
        const distinct = extractedAssets.length;
        return { totalMentions: total, distinctAssets: distinct, saved: Math.max(0, total - distinct) };
    }, [pendingAssetPreview, extractedAssets]);

    // 视频详情弹窗对应的分镜
    const detailShot = useMemo(
        () => activeProject?.shots.find((s) => s.id === detailShotId) || null,
        [activeProject, detailShotId],
    );
    const detailShotIndex = useMemo(
        () => activeProject?.shots.findIndex((s) => s.id === detailShotId) ?? -1,
        [activeProject, detailShotId],
    );
    // 视频详情弹窗中的提示词计算（与 generateShot 中的逻辑完全一致）
    const videoDetailPrompts = useMemo(() => {
        if (!detailShot) return { videoSystemPrompt: "", resolvedScript: "" };
        const resolvedVideoModel = effectiveConfig.videoModel || defaultConfig.videoModel;
        return {
            videoSystemPrompt: makePromptDynamic(videoPrompt, resolvedVideoModel),
            resolvedScript: resolveMentions(detailShot.content, assets),
        };
    }, [detailShot, effectiveConfig.videoModel, videoPrompt, assets]);

    // 当前项目的章节列表（原始小说目录，仅中间栏展示）
    const chapters = useMemo(() => activeProject?.chapters || [], [activeProject]);
    // 当前项目的分镜剧本列表（右侧平铺，全局连续编号）
    const storyboards = useMemo(() => activeProject?.storyboards || [], [activeProject]);
    // 已生成视频的分镜数
    const storyboardVideoDoneCount = useMemo(
        () => storyboards.filter((s) => s.videoStatus === "success" && s.videoUrl).length,
        [storyboards],
    );
    // 章节分组数（每 CHAPTERS_PER_GROUP 章一组）
    const groupCount = useMemo(() => Math.ceil(chapters.length / CHAPTERS_PER_GROUP), [chapters]);

    // 项目加载后自动选中所有章节（仅首次加载时，避免用户编辑后重置）
    const initializedProjectRef = useRef<string | null>(null);
    useEffect(() => {
      if (activeProject && chapters.length > 0 && initializedProjectRef.current !== activeProject.id) {
        initializedProjectRef.current = activeProject.id;
        seedAllGroups(chapters.length);
      }
    }, [activeProject?.id, chapters.length]);
    // 获取选中章节的文本（用于资产提取）
    const getSelectedChapterText = useCallback((): string => {
        if (!activeProject) return "";
        const groups: Chapter[][] = [];
        for (let i = 0; i < chapters.length; i += CHAPTERS_PER_GROUP) {
            groups.push(chapters.slice(i, i + CHAPTERS_PER_GROUP));
        }
        const targetGroups = selectedGroups.size > 0
            ? [...selectedGroups].filter((gi) => gi >= 0 && gi < groups.length).sort((a, b) => a - b)
            : groups.map((_, gi) => gi);
        if (targetGroups.length === 0) return "";
        const texts: string[] = [];
        for (const gi of targetGroups) {
            const groupChapters = groups[gi];
            for (const ch of groupChapters) {
                texts.push(`${ch.title}\n${ch.content}`);
            }
        }
        return texts.join("\n\n");
    }, [activeProject, chapters, selectedGroups]);
    // 章节搜索命中的章节 id 集合（标题或正文包含关键词，忽略大小写）
    const matchedChapterIds = useMemo(() => {
        const kw = chapterSearch.trim().toLowerCase();
        if (!kw) return null;
        return new Set(chapters.filter((ch) => `${ch.title}`.toLowerCase().includes(kw) || `${ch.content}`.toLowerCase().includes(kw)).map((ch) => ch.id));
    }, [chapters, chapterSearch]);
    // 分镜剧本搜索命中的 id 集合（正文包含关键词，也支持"分镜N"或纯数字定位到第N条）
    const matchedStoryboardIds = useMemo(() => {
        const kw = storyboardSearch.trim();
        if (!kw) { setMatchedStoryboardId(null); return null; }
        // 尝试匹配"分镜N"或纯数字 → 定位到第 N 条
        const numMatch = kw.match(/^分镜\s*(\d+)$/i) || kw.match(/^\s*(\d+)\s*$/);
        if (numMatch) {
            const n = parseInt(numMatch[1], 10);
            if (n >= 1 && n <= storyboards.length) {
                const matchedId = storyboards[n - 1].id;
                setMatchedStoryboardId(matchedId);
                return new Set([matchedId]);
            }
        }
        // 否则按正文内容匹配
        const matched = storyboards.filter((sb) => `${sb.content}`.toLowerCase().includes(kw.toLowerCase()));
        if (matched.length > 0) setMatchedStoryboardId(matched[0].id);
        else setMatchedStoryboardId(null);
        return new Set(matched.map((sb) => sb.id));
    }, [storyboards, storyboardSearch]);
    // 分镜视频搜索命中的 shot id 集合（标题或正文包含关键词）
    const matchedShotIds = useMemo(() => {
        const kw = shotSearch.trim().toLowerCase();
        if (!kw) return null;
        const shots = activeProject?.shots || [];
        return new Set(shots.filter((s) => `${s.title}`.toLowerCase().includes(kw) || `${s.content}`.toLowerCase().includes(kw)).map((s) => s.id));
    }, [activeProject, shotSearch]);
    // 全局搜索命中集（章节/分镜/视频三合一）
    const globalMatchedChapterIds = useMemo(() => {
        const kw = globalSearch.trim().toLowerCase();
        if (!kw) return null;
        return new Set(chapters.filter((ch) =>
            `${ch.title}`.toLowerCase().includes(kw) || `${ch.content}`.toLowerCase().includes(kw)
        ).map((ch) => ch.id));
    }, [chapters, globalSearch]);
    const globalMatchedStoryboardIds = useMemo(() => {
        const kw = globalSearch.trim().toLowerCase();
        if (!kw) return null;
        return new Set(storyboards.filter((sb) => `${sb.content}`.toLowerCase().includes(kw)).map((sb) => sb.id));
    }, [storyboards, globalSearch]);
    const globalMatchedShotIds = useMemo(() => {
        const kw = globalSearch.trim().toLowerCase();
        if (!kw) return null;
        const shots = activeProject?.shots || [];
        return new Set(shots.filter((s) => `${s.title}`.toLowerCase().includes(kw) || `${s.content}`.toLowerCase().includes(kw)).map((s) => s.id));
    }, [activeProject, globalSearch]);
    // 全局搜索：首个命中元素的 ref，用于自动滚动定位
    const globalSearchHitRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (!globalSearch.trim() || !globalSearchHitRef.current) return;
        setTimeout(() => {
            globalSearchHitRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
        }, 100);
    }, [globalSearch]);

    // 查看/编辑弹窗对应的分镜剧本
    const viewStoryboard = useMemo(
        () => storyboards.find((s) => s.id === viewStoryboardId) || null,
        [storyboards, viewStoryboardId],
    );

    // 查看/编辑弹窗对应的已提取资产
    const viewAsset = useMemo(
        () => extractedAssets.find((a) => a.id === viewAssetId) || null,
        [extractedAssets, viewAssetId],
    );

    // 资产关联总览：统计所有真实分镜里 @到、但素材库找不到对应资产的名字（缺失=生成视频时不会带参考图）。
    // 失败占位分镜（⚠ 开头）不参与统计。用于列表头部"N 处资产未关联"提醒。
    const missingAssetNames = useMemo(() => {
        const missing = new Set<string>();
        for (const sb of storyboards) {
            if (!storyboardIsReady(sb)) continue;
            for (const name of extractMentions(sb.content)) {
                if (!assets.some((a) => a.alias === name)) missing.add(name);
            }
        }
        return missing;
    }, [storyboards, assets]);

    // 按搜索关键词过滤后的项目列表（不区分大小写）
    const filteredProjects = useMemo(() => {
        const kw = projectSearch.trim().toLowerCase();
        if (!kw) return projects;
        return projects.filter((p) => p.name.toLowerCase().includes(kw));
    }, [projects, projectSearch]);

    const createNewProject = () => {
        // 生成不重复的默认名称：未命名剧本 / 未命名剧本 2 / 未命名剧本 3 …
        const base = "未命名剧本";
        const existing = new Set(projects.map((p) => p.name));
        let name = base;
        let i = 2;
        while (existing.has(name)) { name = `${base} ${i++}`; }
        const project: NovelProject = {
            id: nanoid(), name,
            script: "", shots: [],
            createdAt: Date.now(), updatedAt: Date.now(),
        };
        // 持久化修复：新建项目立即落盘，避免新建后未做其他操作就刷新导致项目丢失
        setProjects((prev) => {
            const next = [project, ...prev];
            void saveProjects(next);
            return next;
        });
        setActiveProjectId(project.id);
        // 新建后立即进入重命名（输入框留空，不带默认文字），若用户不填则回退为唯一默认名
        setRenamingId(project.id);
        setRenameValue("");
    };

    const updateProject = (updater: (p: NovelProject) => NovelProject) => {
        // 持久化修复：所有项目更新（导入小说/编辑原文/章节解析/分镜结果写回等）都立即落盘，避免刷新后丢失
        // 在 setProjects 的函数式回调里拿到最新 next 数组直接保存，规避闭包旧值
        setProjects((prev) => {
            const next = prev.map((p) => p.id === activeProjectId ? updater(p) : p);
            void saveProjects(next);
            return next;
        });
    };

    // 删除项目（带确认框）
    const deleteProject = (id: string) => {
        const target = projects.find((p) => p.id === id);
        antModal.confirm({
            title: "删除剧本项目",
            content: `确定删除「${target?.name || "该项目"}」吗？此操作不可恢复。`,
            okText: "删除", okButtonProps: { danger: true }, cancelText: "取消",
            onOk: () => {
                setProjects((prev) => {
                    const next = prev.filter((p) => p.id !== id);
                    void saveProjects(next);
                    return next;
                });
                if (activeProjectId === id) setActiveProjectId(null);
                antMessage.success("已删除项目");
            },
        });
    };

    // 提交项目重命名
    const commitRename = (id: string) => {
        const name = renameValue.trim();
        // 名称重复校验（忽略自身）
        if (name && projects.some((p) => p.id !== id && p.name === name)) {
            antMessage.warning(`已存在名为「${name}」的项目，请换一个名称`);
            return; // 保持在重命名态，等待用户改名
        }
        setProjects((prev) => {
            const next = prev.map((p) => p.id === id ? { ...p, name: name || p.name, updatedAt: Date.now() } : p);
            void saveProjects(next);
            return next;
        });
        setRenamingId(null);
        setRenameValue("");
    };

    const saveAll = async () => {
        await saveProjects(projects);
        await saveAssets(assets);
    };
    const debouncedSave = useMemo(() => {
        return () => {
            if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
            saveTimerRef.current = setTimeout(() => {
                void saveAll();
                saveTimerRef.current = null;
            }, 800);
        };
    }, [projects, assets]); // eslint-disable-line react-hooks/exhaustive-deps

    // ─── Asset ───

    // 将 blob URL 转换为 data URL（base64），确保可持久化
    // 同步全局素材库的图片资产到当前项目（生图工作台保存的素材自动可见）
    const syncAssetsFromStore = async () => {
        const sharedAssets = useAssetStore.getState().assets.filter((a) => a.kind === "image");
        if (sharedAssets.length === 0) { antMessage.info("全局素材库暂无图片"); return; }
        const newAssets: Asset[] = [];
        for (const sa of sharedAssets) {
            const alias = sa.note?.trim() || sa.title.trim();
            if (!alias) continue;
            const existing = assets.find((a) => a.alias === alias);
            if (existing) continue; // 已存在不重复添加
            // 默认按 title 关键词判断类型，否则用当前 tab
            let type = assetTab;
            const t = (sa.title + " " + alias).toLowerCase();
            if (/\b(角色|人物|演员|英雄|反派)\b/.test(t)) type = "character";
            else if (/\b(场景|地点|环境)\b/.test(t)) type = "scene";
            else if (/\b(道具|物品)\b/.test(t)) type = "prop";
            // 解析图片 URL：素材库中的 coverUrl/dataUrl 可能是 storageKey，需要 resolve 成可访问的 URL
            const rawUrl = sa.coverUrl || sa.data.dataUrl;
            const storageKey = sa.data.storageKey;
            let resolvedUrl = storageKey ? await resolveImageUrl(storageKey, rawUrl) : rawUrl;
            // 如果是 blob URL，转换为 data URL 以便持久化
            if (resolvedUrl.startsWith("blob:")) {
                resolvedUrl = await blobUrlToDataUrl(resolvedUrl);
            }
            newAssets.push({ id: nanoid(), name: sa.title, alias, type, dataUrl: resolvedUrl, description: sa.note || "" });
        }
        if (newAssets.length === 0) { antMessage.info("没有找到需要同步的新素材"); return; }
        const next = [...assets, ...newAssets];
        setAssets(next);
        void saveAssets(next);
        antMessage.success(`已同步 ${newAssets.length} 个素材到当前项目`);
    };

    const addAsset = (file: File, type: AssetType) => {
        const reader = new FileReader();
        reader.onload = () => {
            const dataUrl = reader.result as string;
            const base = file.name.replace(/\.[^/.]+$/, "");
            const asset: Asset = { id: nanoid(), name: base, alias: base, type, dataUrl, description: "" };
            const next = [...assets, asset];
            setAssets(next);
            void saveAssets(next);
            antMessage.success(`资产已添加: ${base}`);
        };
        reader.readAsDataURL(file);
    };

    const deleteAsset = (assetId: string) => {
        const next = assets.filter((a) => a.id !== assetId);
        setAssets(next);
        void saveAssets(next);
        updateProject((p) => ({
            ...p,
            shots: p.shots.map((s) => ({
                ...s,
                referencedAssetIds: s.referencedAssetIds.filter((id) => id !== assetId),
                firstFrameAssetId: s.firstFrameAssetId === assetId ? undefined : s.firstFrameAssetId,
                lastFrameAssetId: s.lastFrameAssetId === assetId ? undefined : s.lastFrameAssetId,
            })),
        }));
    };

    // 更新素材名字/别名（批量导入后逐张补充命名）
    const updateAssetMeta = (assetId: string, name: string, alias: string, desc: string) => {
        const next = assets.map((a) => a.id === assetId ? { ...a, name: name.trim() || a.name, alias: alias.trim() || a.alias, description: desc.trim() } : a);
        setAssets(next);
        void saveAssets(next);
        setEditingAssetId(null);
        antMessage.success("已更新素材信息");
    };

    const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>, type: AssetType) => {
        const files = e.target.files;
        if (!files || files.length === 0) return;
        const imageFiles = Array.from(files).filter((f) => f.type.startsWith("image/"));
        if (imageFiles.length === 0) { antMessage.warning("未选择图片文件"); e.target.value = ""; return; }
        const collected: Asset[] = [];
        imageFiles.forEach((file) => {
            const reader = new FileReader();
            reader.onload = () => {
                const base = file.name.replace(/\.[^/.]+$/, "");
                collected.push({ id: nanoid(), name: base, alias: base, type, dataUrl: reader.result as string, description: "" });
                if (collected.length === imageFiles.length) {
                    setAssets((prev) => { const next = [...prev, ...collected]; void saveAssets(next); return next; });
                    antMessage.success(`已导入 ${collected.length} 个素材`);
                }
            };
            reader.readAsDataURL(file);
        });
        e.target.value = "";
    };

    // handleImportFolder 已合并到 handleFileUpload

    // ─── Script Parsing ───

    const handleParseScript = () => {
        void handleParseStoryboards();
    };

    // ─── Chapter / Storyboard ───

    /** 打开「编辑原文」弹窗：用当前项目正文填充草稿 */
    const openScriptEditor = () => {
        if (!activeProject) { antMessage.warning("请先选择或新建项目"); return; }
        setScriptDraft(activeProject.script || "");
        setShowScriptEditor(true);
    };

    /** 解析出章节后，默认全选所有组（每 1 章一组），用户可再取消勾选想跳过的章 */
    const seedAllGroups = (chapterCount: number) => {
        const n = Math.ceil(chapterCount / CHAPTERS_PER_GROUP);
        setSelectedGroups(new Set(Array.from({ length: n }, (_, i) => i)));
    };

    /** 保存「编辑原文」弹窗内容：写回 script 并按章节重新解析目录 */
    const saveScriptEditor = () => {
        if (!activeProject) return;
        const parsed = parseChapters(scriptDraft);
        updateProject((p) => ({ ...p, script: scriptDraft, chapters: parsed, updatedAt: Date.now() }));
        setExpandedChapterIds(new Set(parsed[0] ? [parsed[0].id] : []));
        seedAllGroups(parsed.length);
        setShowScriptEditor(false);
        antMessage.success(`已保存，解析为 ${parsed.length} 章`);
    };

    /** 由当前 script 重新解析章节目录（仅刷新中间栏目录，不动分镜剧本） */
    const rebuildChapters = () => {
        if (!activeProject) return;
        const parsed = parseChapters(activeProject.script);
        updateProject((p) => ({ ...p, chapters: parsed, updatedAt: Date.now() }));
        // 默认展开第一章，其余折叠
        setExpandedChapterIds(new Set(parsed[0] ? [parsed[0].id] : []));
        seedAllGroups(parsed.length);
        antMessage.success(`已解析为 ${parsed.length} 章`);
    };

    /**
     * 从选中章节的小说文本提取资产列表（角色/场景/道具）及其描述，同时提取画风提示词。
     * 返回 JSON 对象：{ assets: [...], style: "..." }
     */
    const extractAssetsFromNovel = async (novelText: string, controller: AbortController): Promise<{ assets: Array<{ name: string; type: AssetType; description: string }>; style: string }> => {
        const extractSystemPrompt = `你是一个专业的小说视觉分析助手，擅长从文本中提取关键视觉元素，并总结统一的画风风格。`;
        const extractUserPrompt = `请从以下小说文本中提取所有重要的视觉资产（角色、场景、道具），并总结一个适合生成所有图片的统一画风提示词。

要求：
1. 角色：提取所有有名字的角色，描述其外貌特征（年龄、性别、发型、服装等）
2. 场景：提取所有重要的场景/地点，描述其视觉特征（建筑风格、环境、氛围等）
3. 道具：提取所有重要的道具/物品，描述其外观特征（形状、颜色、材质等）
4. style：总结一段统一的画风提示词，适合所有资产图片（如"3D古风动漫风格，精致感，高质量，柔焦，细节刻画，超高清"）

输出格式（严格 JSON，不要输出其他内容）：
{
  "assets": [
    {"name": "角色名", "type": "character", "description": "外貌描述"},
    {"name": "场景名", "type": "scene", "description": "场景描述"},
    {"name": "道具名", "type": "prop", "description": "道具描述"}
  ],
  "style": "统一画风提示词"
}

注意：
- name 必须只是纯名称（如"沈一寻"、"高考考场"），绝对不能包含动作、状态、位置等描述（如"坐在长椅上"、"站在门口"都是错误的）
- name 必须与小说原文中角色的名字完全一致，不可自行改名或加字
- type 只能是 "character"、"scene"、"prop" 之一
- description 要简洁但包含关键视觉特征，用于生成参考图
- 只提取对剧情有重要影响的资产，不要提取次要元素
- style 要简洁（20-50字），描述画风、风格、画质等

小说文本：
${novelText.slice(0, 30000)}`;

        const response = await callTextModel(extractSystemPrompt, extractUserPrompt, controller);
        try {
            const jsonMatch = response.match(/\{[\s\S]*\}/);
            if (!jsonMatch) throw new Error("未找到 JSON 对象");
            const parsed = JSON.parse(jsonMatch[0]);
            const assets = (parsed.assets || []).filter((item: any) =>
                item.name && item.type && item.description &&
                ["character", "scene", "prop"].includes(item.type)
            );
            const style = parsed.style || "";
            return { assets, style };
        } catch (error) {
            console.error("[资产提取] 解析失败:", response);
            throw new Error(`资产列表解析失败: ${(error as Error).message}`);
        }
    };

    /**
     * 根据资产类型构建标准化的生图提示词。
     * 角色 → 三视图，场景 → 四宫格，道具 → 标准物品图。
     * 支持用户自定义画风/通用提示词后缀。
     * 模板来源优先级：
     *   1) 管理员在后台「分镜图片系统提示词」中自定义的值（assetPrompt state，可在提示词编辑弹窗内修改）
     *   2) 代码内写死的 ASSET_TEMPLATES_FALLBACK 兜底
     */
    const buildAssetPrompt = (asset: { name: string; type: AssetType; description: string }, style?: string): string => {
        const styleSuffix = style ? `，${style}` : assetImageStylePrompt ? `，${assetImageStylePrompt}` : "";
        // 每次调用实时解析当前 assetPrompt（避免模块初始化后就固定死），
        // 管理员可在后台改 storyboardImage，再通过 useEffect 注入到 assetPrompt。
        const runtimeTemplates = parseAssetTemplates(assetPrompt || DEFAULT_ASSET_PROMPT);
        const tmplChar = runtimeTemplates["角色三视图"] || ASSET_TEMPLATES_FALLBACK["角色三视图"] || "";
        const tmplScene = runtimeTemplates["场景四宫格"] || ASSET_TEMPLATES_FALLBACK["场景四宫格"] || "";
        const tmplProp = runtimeTemplates["道具标准图"] || ASSET_TEMPLATES_FALLBACK["道具标准图"] || "";
        if (asset.type === "character") return tmplChar + asset.description + styleSuffix;
        if (asset.type === "scene") return tmplScene.replace("{description}", asset.description) + styleSuffix;
        return tmplProp.replace("{description}", asset.description) + styleSuffix;
    };

    /**
     * 为单个资产生成参考图。
     * 使用图片模型根据描述生成参考图，并返回 dataUrl。
     * 使用后端 CanvasImageTask 任务化，切页后可恢复轮询。
     */
    const generateAssetImage = async (
        asset: { name: string; type: AssetType; description: string },
        controller: AbortController
    ): Promise<string> => {
        // 优先使用资产弹窗内选择的图片模型，其次用全局配置
        // 修复：不再回退到 effectiveConfig.model（文本模型），避免用文本模型生图
        const assetModel = assetImageModel || effectiveConfig.imageModel || defaultConfig.imageModel;
        if (!isAiConfigReady(effectiveConfig, assetModel)) {
            throw new Error("请先完成图片模型配置");
        }

        // 使用标准化提示词（角色三视图/场景四宫格/道具）
        const prompt = buildAssetPrompt(asset);

        // 调用图片生成 API（使用资产专用模型和渠道）
        const scopedConfig: AiConfig = {
            ...effectiveConfig,
            model: assetModel,
            activeChannelId: assetImageChannelId || effectiveConfig.imageChannelId || effectiveConfig.activeChannelId,
        };

        // Bug #2（图片引用 2026-08-25）：生图时把"已存在的同 alias 资产图"作为 reference 传入，
        // 让图片模型参考现有视觉保持角色/场景一致性（用户原话："都说了是图片引用，不是图片的提示词引用"）。
        // 首次生图时没有 existing → 传空数组，与旧行为等价；重生成时拿已有图当图参考。
        const existingAsset = assets.find((a) => a.alias === asset.name);
        const references: ReferenceImage[] = existingAsset
            ? [{ id: existingAsset.id, name: existingAsset.name, type: "image/png", dataUrl: existingAsset.dataUrl, url: existingAsset.url, storageKey: existingAsset.storageKey }]
            : [];

        const clientTaskId = `novel-asset-${nanoid()}`;
        const task = await createCanvasImageTask(scopedConfig, prompt, references, { source: "novel", clientTaskId });
        if (activeProject) {
            updateProject((p) => ({ ...p, assetImageTaskIds: [...(p.assetImageTaskIds || []), task.id] }));
        }
        if (task.status === "completed") {
            const dataUrl = task.image_url || task.url || "";
            if (!dataUrl) throw new Error("图片生成返回为空");
            return dataUrl;
        }
        const completedTask = await pollCanvasImageTaskUntilDone(task.id, controller.signal);
        const dataUrl = completedTask.image_url || completedTask.url || "";
        if (!dataUrl) throw new Error("图片生成返回为空");
        return dataUrl;
    };

    /**
     * 第一步：从选中章节提取资产列表（角色/场景/道具）+ 画风提示词，预览后手动生成。
     */
    const extractAssetsStep = async () => {
        const selectedText = getSelectedChapterText();
        if (!selectedText) {
            antMessage.warning("请先勾选要提取的章节");
            return;
        }
        if (!activeProject) { antMessage.warning("请先选择项目"); return; }

        const textModel = effectiveConfig.textModel || defaultConfig.textModel;
        if (!isAiConfigReady(effectiveConfig, textModel)) {
            antMessage.warning("请先完成文本模型配置");
            openConfigDialog(true);
            return;
        }

        setExtractingAssets(true);
        const controller = new AbortController();
        setAbortController(controller);
        try {
            const { assets, style } = await extractAssetsFromNovel(selectedText, controller);
            if (assets.length === 0) {
                antMessage.warning("未从选中章节提取到任何资产");
                setExtractedAssets([]);
                setExtractedStyle("");
                return;
            }
            // 过滤按类型
            const filtered = assets.filter((a) => {
                if (a.type === "character" && !generateCharacterAssets) return false;
                if (a.type === "scene" && !generateSceneAssets) return false;
                if (a.type === "prop" && !generatePropAssets) return false;
                return true;
            });
            if (filtered.length === 0) {
                antMessage.warning("根据勾选的类型，没有可生成的资产");
                setExtractedAssets([]);
                setExtractedStyle("");
                return;
            }
            // 给每条资产分配稳定 id，便于卡片定位和弹窗编辑
            setExtractedAssets(filtered.map((a) => ({ id: nanoid(), name: a.name, type: a.type, description: a.description })));
            setExtractedStyle(style);
            antMessage.success(`提取成功：${filtered.length} 个资产`);
        } catch (error) {
            console.error("[资产提取] 失败:", error);
            antMessage.error(`提取失败: ${(error as Error).message}`);
        } finally {
            setExtractingAssets(false);
            setAbortController(null);
        }
    };

    const IMAGE_POLL_INTERVAL_MS = 3000;
    const IMAGE_POLL_MAX_ATTEMPTS = 200; // ~10 分钟

    /** 轮询 CanvasImageTask 直到完成/失败/超时 */
    const pollCanvasImageTaskUntilDone = async (taskId: string, signal?: AbortSignal): Promise<CanvasImageTask> => {
        for (let attempt = 0; attempt < IMAGE_POLL_MAX_ATTEMPTS; attempt++) {
            if (signal?.aborted) throw new Error("已取消");
            await new Promise((resolve) => setTimeout(resolve, IMAGE_POLL_INTERVAL_MS));
            const task = await pollCanvasImageTaskStatus(taskId);
            if (task.status === "completed") return task;
            if (task.status === "failed") throw new Error(task.error?.message || "图片生成失败");
        }
        throw new Error("图片生成超时");
    };

    /**
     * 第二步：根据预览的资产列表并发生图（默认 3 并发，可通过 imageConcurrency state 调整到 1-8）。
     * 使用后端 CanvasImageTask 任务化，切页后可恢复轮询。
     * 改造要点：
     *   - 改 worker pool 模式：N 个 worker 共享一个待处理队列，N=min(imageConcurrency, targetAssets.length)
     *   - 每个 worker 独立 try-catch，失败记入 failedAssets（不重置旧条目，让历史失败持续可见）
     *   - assetsToGenerate 为 extractedAssets 的同步快照，避免 setExtractedAssets 后 React 状态未提交导致的空跑
     *   - 返回 {success, fail} 计数，让 autoPilot 链判断"全失败时停住"
     */
    const generateImagesFromExtracted = async (
        externalController?: AbortController,
        assetsToGenerate?: Array<{ id: string; name: string; type: AssetType; description: string; sourceSnippet?: string; isExisting?: boolean }>
    ): Promise<{ success: number; fail: number }> => {
        const targetAssets = assetsToGenerate ?? extractedAssets;
        if (targetAssets.length === 0) {
            antMessage.warning("请先提取资产");
            return { success: 0, fail: 0 };
        }
        // 验证实际使用的模型（assetImageModel 优先），而非全局 imageModel
        // 修复：不再回退到 effectiveConfig.model（文本模型），避免用文本模型生图
        const imageModel = assetImageModel || effectiveConfig.imageModel || defaultConfig.imageModel;
        if (!isAiConfigReady(effectiveConfig, imageModel)) {
            antMessage.warning("请先完成图片模型配置");
            openConfigDialog(true);
            return { success: 0, fail: 0 };
        }

        setAssetGenerationRunning(true);
        // 注意：不再 setFailedAssets([])，让历史失败持续可见直到用户主动「重试」按钮
        // （handleRetryFailedAssets 内部清空，或单条 X 按钮）
        const controller = externalController ?? new AbortController();
        const newAssets: Asset[] = [];
        const taskIds: string[] = [];
        let successCount = 0;
        let failCount = 0;

        const scopedConfig: AiConfig = {
            ...effectiveConfig,
            model: imageModel,
            activeChannelId: assetImageChannelId || effectiveConfig.imageChannelId || effectiveConfig.activeChannelId,
        };

        // 提取闭包：单个资产生图完整流程（同步设置 pending / 调 createCanvasImageTask / 写 assets / 失败计入 failedAssets）
        const processOne = async (asset: typeof extractedAssets[number]) => {
            const pendingId = `pending-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
            const prompt = buildAssetPrompt(asset, extractedStyle);
            setPendingAssets((prev) => [...prev, { id: pendingId, name: asset.name, type: asset.type, prompt, startTime: Date.now() }]);
            try {
                // Bug #2（图片引用 2026-08-25）：同 alias 已存在资产图时，把它作为 image reference 传入，
                // 保持角色/场景跨分镜一致；首次生成 existing=undefined → 传空数组，与旧行为等价。
                const existingAsset = assets.find((a) => a.alias === asset.name);
                const references: ReferenceImage[] = existingAsset
                    ? [{ id: existingAsset.id, name: existingAsset.name, type: "image/png", dataUrl: existingAsset.dataUrl, url: existingAsset.url, storageKey: existingAsset.storageKey }]
                    : [];
                const clientTaskId = `novel-asset-${nanoid()}`;
                const task = await createCanvasImageTask(scopedConfig, prompt, references, { source: "novel", clientTaskId });
                taskIds.push(task.id);

                let dataUrl: string;
                if (task.status === "completed") {
                    dataUrl = task.image_url || task.url || "";
                    if (!dataUrl) throw new Error("图片生成返回为空");
                } else {
                    // 后端任务模式 → 轮询直到完成
                    const completedTask = await pollCanvasImageTaskUntilDone(task.id, controller.signal);
                    dataUrl = completedTask.image_url || completedTask.url || "";
                    if (!dataUrl) throw new Error("图片生成返回为空");
                }
                // 现有 assets 已有同名 alias 就不重复入库（去重）
                if (assets.find((a) => a.alias === asset.name)) {
                    setPendingAssets((prev) => prev.filter((p) => p.id !== pendingId));
                    return;
                }
                newAssets.push({
                    id: nanoid(),
                    name: asset.name,
                    alias: asset.name,
                    type: asset.type,
                    dataUrl,
                    description: asset.description,
                });
                successCount++;
            } catch (error) {
                console.error(`[图片生成] 失败: ${asset.name}`, error);
                failCount++;
                setFailedAssets((prev) => [...prev, { id: pendingId, name: asset.name, type: asset.type, error: (error as Error).message }]);
            } finally {
                setPendingAssets((prev) => prev.filter((p) => p.id !== pendingId));
            }
        };

        try {
            // 并发 worker pool：N 个 worker 共享一个待处理队列，N=min(imageConcurrency, extractedAssets.length)
            // 不是 Promise.all 全并发——后者会在 7 张图时同时发 7 个请求容易触上限
            const concurrency = Math.max(1, Math.min(imageConcurrency, targetAssets.length));
            const queue = [...targetAssets];
            const total = queue.length;
            // 进度实时刷新：完成数 = total - queue.length
            const updateProgress = () => {
                const done = total - queue.length;
                setPipelineStatus(`正在生成资产图片：${done}/${total}${failCount > 0 ? `（${failCount} 失败）` : ""}`);
            };
            updateProgress();

            const worker = async () => {
                while (queue.length > 0 && !controller.signal.aborted) {
                    const asset = queue.shift();
                    if (!asset) break;
                    await processOne(asset);
                    updateProgress();
                }
            };
            // 用 inFlight 跟踪，Promise.all 等所有 worker 完成
            const inFlight = Array.from({ length: concurrency }, () => worker());
            await Promise.all(inFlight);
        } catch (error) {
            antMessage.error(`生成失败: ${(error as Error).message}`);
        } finally {
            if (newAssets.length > 0) {
                setAssets((prev) => {
                    const next = [...prev, ...newAssets];
                    void saveAssets(next);
                    return next;
                });
            }
            if (taskIds.length > 0 && activeProject) {
                updateProject((p) => ({ ...p, assetImageTaskIds: [...(p.assetImageTaskIds || []), ...taskIds] }));
            }
            if (failCount === 0) {
                antMessage.success(`分镜图片生成完成：成功 ${successCount} 个`);
            } else if (successCount > 0) {
                antMessage.warning(`分镜图片生成完成：成功 ${successCount} 个，失败 ${failCount} 个（悬停失败卡片可看原因）`);
            } else {
                antMessage.error(`分镜图片生成失败：全部 ${failCount} 个失败（悬停失败卡片可看原因）`);
            }
            setAssetGenerationRunning(false);
            // 手动模式：清掉进度文案；autoPilot 链由 autoPilotContinueFromStoryboards 接管状态
            if (autoPilot === "off") {
                setPipelineStatus("");
            }
        }
        return { success: successCount, fail: failCount };
    };

    /**
     * 自动链 preview 闸专用：从分镜剧本里抽取角色 / 场景 / 道具（含文本模型生成描述），
     * 写入 extractedAssets state 让用户能在 Modal 里预览/编辑/删除，但**不生图**。
     * 用户在 Modal 里点「确认生成」才会走 generateImagesFromExtracted 生成图片。
     * 复用 generateAssetsFromStoryboards 的 1-4 步算法，但不调生图。
     * 返回成功抽到的资产数（0 表示无可抽资产或被中止）。
     */
    const extractAssetsForAutoChain = async (boards: Storyboard[], ctrl: AbortController): Promise<{ count: number; items: Array<{ id: string; name: string; type: AssetType; description: string; sourceSnippet?: string; isExisting?: boolean }> }> => {
        if (!activeProject) return { count: 0, items: [] };
        const textModel = effectiveConfig.textModel || defaultConfig.textModel;
        if (!isAiConfigReady(effectiveConfig, textModel)) {
            throw new Error("请先完成文本模型配置");
        }

        // 1) 从所有剧本里提取名字 → 类型 → 上下文片段（与 generateAssetsFromStoryboards 完全一致）
        const nameTypeMap = new Map<string, { name: string; type: AssetType; contextSnippet: string }>();
        const allScriptText = boards.map((sb) => sb.content || "").filter(Boolean).join("\n\n");

        for (const sb of boards) {
            if (!storyboardIsReady(sb)) continue; // 跳过失败占位
            const snippet = sb.content.slice(0, 800);
            const header = extractStoryboardHeader(sb.content);
            for (const c of header.characters) {
                if (!nameTypeMap.has(c)) nameTypeMap.set(c, { name: c, type: "character", contextSnippet: snippet });
            }
            for (const s of header.scenes) {
                if (!nameTypeMap.has(s)) nameTypeMap.set(s, { name: s, type: "scene", contextSnippet: snippet });
            }
            // 道具：从正文抓 @xxx（与分镜剧本提示词「动作/台词前用 @ 标道具名」对齐）
            for (const m of sb.content.matchAll(/@([^\s,;，。；、]+)/g)) {
                const name = m[1].trim();
                if (!name) continue;
                const existing = nameTypeMap.get(name);
                if (existing && existing.type !== "prop") continue;
                if (!existing) nameTypeMap.set(name, { name, type: "prop", contextSnippet: snippet });
            }
        }

        // 2) 按勾选类型过滤
        const filtered = [...nameTypeMap.values()].filter((it) => {
            if (it.type === "character" && !generateCharacterAssets) return false;
            if (it.type === "scene" && !generateSceneAssets) return false;
            if (it.type === "prop" && !generatePropAssets) return false;
            return true;
        });

        // 3) 跳过已有 alias（精确匹配）
        const newItems = filtered.filter((it) => !assets.find((a) => a.alias === it.name));
        if (newItems.length === 0) {
            setPipelineStatus("autoPilot：所有资产已存在，跳过资产生成");
            return { count: 0, items: [] };
        }

        setPipelineStatus(`autoPilot：正在提取 ${newItems.length} 个资产的描述（角色 ${newItems.filter(i => i.type === "character").length}，场景 ${newItems.filter(i => i.type === "scene").length}，道具 ${newItems.filter(i => i.type === "prop").length}）...`);

        // 4) 文本模型：给定名字+剧本上下文，返回每个 name 的 description
        const described = await extractDescriptionsForAssets(newItems, allScriptText, ctrl);
        if (ctrl.signal.aborted) return { count: 0, items: [] };

        // 5) 写 extractedAssets state（modal 显示 + 用户可编辑/删除）
        // 先清空旧值，避免重复调用时累积重复资产
        // P1 改造 a2：保留 sourceSnippet（来自 contextSnippet，最长 800 字），用于来源追溯 UI
        // P1 改造 b3：标记 isExisting（同 alias 已存在于 assets 库）→ UI 显示「♻️ 已存在」chip，让用户决定是否重新生图
        const existingNames = new Set(assets.map((a) => a.alias || a.name).filter(Boolean));
        const extractedItems = described.map((d) => ({
            id: nanoid(),
            name: d.name,
            type: d.type,
            description: d.description,
            sourceSnippet: d.contextSnippet,
            isExisting: existingNames.has(d.name),
        }));
        setExtractedAssets(extractedItems);
        // 自动链不提取画风，留空即可（buildAssetPrompt 在没 style 时会兜底用 assetImageStylePrompt）
        setExtractedStyle("");

        return { count: described.length, items: extractedItems };
    };

    /**
     * 给定一组「名字 + 类型 + 上下文片段」+ 全量剧本上下文，文本模型返回每个名字对应的 description。
     * 复用 callTextModel 渠道；返回按 items 顺序对齐的结果，文模漏给的名字用 name 占位。
     */
    const extractDescriptionsForAssets = async (
        items: Array<{ name: string; type: AssetType; contextSnippet: string }>,
        scriptContext: string,
        controller: AbortController
    ): Promise<Array<{ name: string; type: AssetType; description: string; contextSnippet: string }>> => {
        const extractSystemPrompt = `你是一个专业的小说视觉分析助手，擅长从分镜剧本中提取角色的外貌、场景的视觉特征、道具的外观。`;
        const extractUserPrompt = `以下是分镜剧本里提取出的资产名（角色/场景/道具），请为每一个生成精炼的视觉描述，用于后续图片模型出图。

要求：
1. 角色：外貌特征（年龄、性别、发型、服装等），严格参考分镜剧本上下文中对该角色的描述
2. 场景：视觉特征（建筑风格、环境、氛围、时段、光线等）
3. 道具：外观特征（形状、颜色、材质、工艺等）
4. description 简洁但包含关键视觉特征，30-80 字
5. 严格基于分镜剧本上下文，不要凭空编造剧本外的信息

资产列表：
${items.map((it, i) => `${i + 1}. [${it.type === "character" ? "角色" : it.type === "scene" ? "场景" : "道具"}] ${it.name}`).join("\n")}

输出格式（严格 JSON，不要输出其他内容）：
{
  "descriptions": [
    {"name": "沈一言", "type": "character", "description": "外貌描述"},
    {"name": "场景名", "type": "scene", "description": "场景描述"},
    {"name": "道具名", "type": "prop", "description": "道具描述"}
  ]
}

分镜剧本上下文（节选）：
${scriptContext.slice(0, 20000)}`;

        const response = await callTextModel(extractSystemPrompt, extractUserPrompt, controller);
        try {
            const jsonMatch = response.match(/\{[\s\S]*\}/);
            if (!jsonMatch) throw new Error("未找到 JSON 对象");
            const parsed = JSON.parse(jsonMatch[0]);
            const descs = (parsed.descriptions || []).filter((item: any) =>
                item.name && item.description && ["character", "scene", "prop"].includes(item.type)
            );
            // 按原始 items 顺序补齐（避免文模漏给某个时整段崩）；保留每个 item 自己的 contextSnippet
            //   用于 P1 改造 a2「来源追溯」：让用户看到「这个 description 是从哪段原文抽的」
            return items.map((it) => {
                const found = descs.find((d: any) => d.name === it.name && d.type === it.type);
                return { name: it.name, type: it.type, description: found?.description || it.name, contextSnippet: it.contextSnippet };
            });
        } catch (error) {
            console.error("[资产生成] 描述提取失败:", response);
            throw new Error(`描述提取失败: ${(error as Error).message}`);
        }
    };

    /**
     * 统一调用文本模型（与视频创作台/Agent 一致的渠道解析与鉴权）。
     * 关键：URL 用 aiApiUrl、Header 用 aiHeaders（会带上 X-Model-Channel-ID 等渠道头），
     * 并按"文本模型"生成 scopedConfig，保证渠道路由到文本模型对应的渠道。
     * 返回模型输出的纯文本，兼容 choices / data.choices 两种结构；空返回会抛错。
     */
    const callTextModel = async (systemContent: string, userContent: string, controller: AbortController): Promise<string> => {
        const textModel = effectiveConfig.textModel || defaultConfig.textModel;
        // 云端渠道必须登录（否则请求没有有效 token，后端会拒绝）
        if (activeChannelKind({ ...effectiveConfig, model: textModel }) === "remote" && !token) {
            throw new Error("请先登录后再使用云端文本模型");
        }
        // scopedConfig：把 active model 指向文本模型，并显式把激活渠道兜底为文本渠道。
        // 关键：与画布 Agent(requestCanvasAgentTurn)保持一致——只覆盖 model 不够，
        // 因为 channelIdForActiveModel 在未配置 textChannelId 时会回退到 activeChannelId，
        // 若不修正，X-Model-Channel-ID 会指向当前激活的图片/视频渠道，导致文本请求路由到
        // 不支持 /chat/completions 的渠道而失败（本地已配 textChannelId 能用、生产用不了的根因）。
        const scopedConfig: AiConfig = {
            ...effectiveConfig,
            model: textModel,
            activeChannelId: effectiveConfig.textChannelId || effectiveConfig.activeChannelId,
            textChannelId: effectiveConfig.textChannelId,
        };
        const resp = await fetch(aiApiUrl(scopedConfig, "/chat/completions"), {
            method: "POST",
            headers: aiHeaders(scopedConfig, "application/json"),
            body: JSON.stringify({
                model: textModel,
                messages: [
                    { role: "system", content: systemContent },
                    { role: "user", content: userContent },
                ],
                stream: false,
            }),
            signal: controller.signal,
        });
        // 无论成功失败都先取原始返回体，便于定位
        const data = await resp.json().catch(() => ({} as Record<string, unknown>));
        if (!resp.ok || (typeof (data as { code?: number }).code === "number" && (data as { code?: number }).code !== 0)) {
            const msg = (data as { error?: { message?: string }; message?: string })?.error?.message
                || (data as { message?: string })?.message
                || `HTTP ${resp.status}`;
            console.error("[分镜] 文本模型请求失败，原始返回:", data);
            throw new Error(`文本模型请求失败: ${msg}`);
        }
        // 兼容 OpenAI 风格 choices / 后端包裹的 data.choices / 直出 content 等结构
        const d = data as {
            choices?: { message?: { content?: string }; text?: string }[];
            data?: { choices?: { message?: { content?: string } }[] };
            message?: { content?: string };
            content?: string;
            output_text?: string;
        };
        const text: string =
            d.choices?.[0]?.message?.content ??
            d.choices?.[0]?.text ??
            d.data?.choices?.[0]?.message?.content ??
            d.message?.content ??
            d.content ??
            d.output_text ??
            "";
        if (!text.trim()) {
            console.error("[分镜] 文本模型返回空内容，原始返回:", data);
            throw new Error("文本模型返回为空，请检查模型配置/额度/分镜剧本提示词");
        }
        return text;
    };

    /**
     * 调用文本模型，把"一组（每 1 章）"正文改写为 1 条完整的分镜视频描述词。
     *
     * 一章一条：一次模型调用直接把整章剧情整合成一条完整的视频描述提示词（内部用 0-4s/4-9s/…
     * 这样的时间段自然分层，总时长不超过 shotDuration 秒），不再二次拆成多个独立【分镜N】。
     * 始终返回长度为 1 的数组（该章对应的那一条分镜剧本）。
     */
    const rewriteGroupToStoryboards = async (groupChapters: Chapter[], groupIndex: number, controller: AbortController, shotCount: number = 1): Promise<string[]> => {
        const startCh = groupIndex * CHAPTERS_PER_GROUP + 1;
        const endCh = startCh + groupChapters.length - 1;
        const chapterLabel = startCh === endCh ? `第${startCh}章` : `第${startCh}~${endCh}章`;
        // 组合本章正文（含章节标题，帮助模型理解上下文与衔接）
        const groupText = groupChapters.map((c) => `${c.title}\n${c.content}`).join("\n\n");
        // 时长用用户设定值（不再硬截断为 15 秒），让文本模型按实际时长写剧情；
        // makePromptDynamic 会在下游根据视频模型最大时长动态调整"15秒"引用
        const dur = Math.max(1, Math.round(shotDuration) || 15);
        const realShotCount = Math.max(1, Math.min(5, shotCount));

        // user content 与 system prompt 通过共享模块构造，与后端 executeStoryboardTask 完全一致；
        // 改提示词只需改 web/src/lib/prompts/storyboard.ts + handler 内对应段。
        // P1 b1/b2：shotCount > 1 时 prompt 引导模型输出 N 条 ===SHOT=== 分隔
        const writeUserContent = buildStoryboardUserContent({
            chapterLabel,
            chapterText: groupText,
            assets,
            shotDuration: dur,
            shotCount: realShotCount,
        });
        const draft = await callTextModel(
            renderStoryboardPrompt(scriptPrompt, dur),
            writeUserContent,
            controller,
        );
        if (controller.signal.aborted) return [];

        // 清理可能残留的【分镜N】/场景N 起始标记
        const cleaned = cleanStoryboardLeadingLabel(draft);
        if (!cleaned) return [];

        // 一章 N 条：shotCount=1 默认整段作为一条；shotCount>1 按 ===SHOT=== 分隔出 N 条
        if (realShotCount > 1 && cleaned.includes("===SHOT===")) {
            return cleaned.split("===SHOT===").map((s) => s.trim()).filter(Boolean);
        }
        return [cleaned];
    };

    /**
     * 轮询后端分镜任务，把已产出分镜实时写回项目（刷新/重开页面后可据 storyboardTaskId 恢复轮询）。
     * groupMap：任务结果索引 → 原始组索引的映射（提交任务时选定的是部分组，需要映射回去）。
     * controller：用于中止轮询（点"停止"时 abort）。
     */
    const pollStoryboardTask = (taskId: string, groupMap: number[], controller: AbortController) => {
        // 清除上一次轮询（避免重复轮询：提交任务时直接调用 + 恢复 effect 都可能触发）
        if (storyboardPollRef.current) { clearInterval(storyboardPollRef.current); storyboardPollRef.current = null; }
        const targetGroupSet = new Set(groupMap);
        // 连续失败计数：任务被删除/后端不可用时自动停止轮询，避免无限空转
        let consecutiveFailures = 0;
        const MAX_FAILURES = 10;
        // 自动重试配置：最多重试2次，指数退避延迟（3s, 6s）
        const MAX_AUTO_RETRIES = 2;
        const retryDelays = [3000, 6000]; // 毫秒

        const stopPolling = () => {
            if (storyboardPollRef.current) { clearInterval(storyboardPollRef.current); storyboardPollRef.current = null; }
        };

        const poll = async () => {
            if (controller.signal.aborted) { stopPolling(); return; }
            try {
                const task = await getStoryboardTask(taskId, token);
                if (controller.signal.aborted) { stopPolling(); return; }
                consecutiveFailures = 0; // 成功拉取，重置失败计数
                const entries = parseStoryboardTaskResult(task.result);

                // 把任务结果写回项目：保留已有分镜的 id 与视频状态，只更新内容；新分镜分配新 id
                updateProject((p) => {
                    const existingByGroup = new Map((p.storyboards || []).map((s) => [s.groupIndex, s]));
                    const kept = (p.storyboards || []).filter((s) => !targetGroupSet.has(s.groupIndex));
                    const updated = entries.map((e) => {
                        const gi = groupMap[e.groupIndex] ?? e.groupIndex;
                        const existing = existingByGroup.get(gi);
                        // 后端 status="failed" 时 content 为空、error 有内容（见 StoryboardTaskResultEntry 类型注释）
                        return {
                            id: existing?.id ?? nanoid(),
                            groupIndex: gi,
                            content: e.status === "failed" ? "" : (e.content ?? ""),
                            shotStatus: e.status,
                            shotError: e.status === "failed" ? (e.error ?? e.content ?? "") : existing?.shotError,
                            videoStatus: existing?.videoStatus ?? ("idle" as const),
                            videoUrl: existing?.videoUrl,
                            videoError: existing?.videoError,
                        } as Storyboard;
                    });
                    return {
                        ...p,
                        storyboards: [...kept, ...updated].sort((a, b) => a.groupIndex - b.groupIndex),
                        updatedAt: Date.now(),
                    };
                });

                setPipelineProgress({ current: task.doneCount, total: task.totalCount || groupMap.length });
                // 覆盖恢复阶段的"正在恢复…"静态文案，按真实进度更新底部状态条，
                // 否则一直挂着"正在恢复分镜任务进度…"，让人误以为任务卡死
                const totalCount = task.totalCount || groupMap.length;
                setPipelineStatus(
                    task.status === "queued"
                        ? `分镜任务已提交，后端排队中（共 ${totalCount} 章）…`
                        : `后端正在生成分镜剧本（共 ${totalCount} 章，已完成 ${task.doneCount} 章）…`
                );

                // 终态：停止轮询，清理状态，汇总反馈
                if (task.status === "completed" || task.status === "failed" || task.status === "canceled") {
                    stopPolling();
                    // autoPilot 模式下不清空进度状态，由 autoPilotContinueFromStoryboards 管理
                    if (autoPilot === "off" || task.status !== "completed") {
                        setParsingStoryboard(false);
                        setAbortController(null);
                        setPipelineStatus("");
                    }
                    // 清除项目中的任务追踪字段（下次分镜可重新提交）
                    updateProject((p) => ({ ...p, storyboardTaskId: undefined, storyboardGroupMap: undefined, updatedAt: Date.now() }));

                    if (task.status === "completed") {
                        const failedEntries = entries.filter((e) => e.status === "failed");
                        const failed = failedEntries.length;
                        const succeeded = entries.length - failed;
                        
                        // 自动重试：如果有失败条目且开启了自动重试
                        if (failed > 0 && autoRetryEnabled && !controller.signal.aborted) {
                            antMessage.info(`检测到 ${failed} 个分镜生成失败，正在自动重试（最多 ${MAX_AUTO_RETRIES} 次）…`);
                            setPipelineStatus(`分镜完成：成功 ${succeeded}，失败 ${failed}，正在自动重试…`);
                            
                            // 获取失败分镜的 id 列表（使用 projectsRef 避免闭包陈旧引用）
                            const latestProject = projectsRef.current.find((p) => p.id === activeProjectId);
                            const failedStoryboards = (latestProject?.storyboards || [])
                                .filter((s) => failedEntries.some((e) => (groupMap[e.groupIndex] ?? e.groupIndex) === s.groupIndex));
                            
                            // 异步执行重试，不阻塞 UI
                            (async () => {
                                let retrySuccess = 0;
                                let retryFail = 0;
                                
                                for (let retry = 0; retry < MAX_AUTO_RETRIES && failedStoryboards.length > 0; retry++) {
                                    if (controller.signal.aborted) break;
                                    
                                    const delay = retryDelays[retry] || retryDelays[retryDelays.length - 1];
                                    setPipelineStatus(`自动重试第 ${retry + 1}/${MAX_AUTO_RETRIES} 次，等待 ${delay / 1000}s…`);
                                    await new Promise((resolve) => setTimeout(resolve, delay));
                                    
                                    const currentBatch = [...failedStoryboards];
                                    let batchSuccess = 0;
                                    
                                    for (const sb of currentBatch) {
                                        if (controller.signal.aborted) break;
                                        try {
                                            await regenerateStoryboard(sb.id);
                                            // 使用 projectsRef 获取最新状态检查重试结果
                                            const currentProject = projectsRef.current.find((p) => p.id === activeProjectId);
                                            const updated = (currentProject?.storyboards || []).find((s) => s.id === sb.id);
                                            if (updated && updated.shotStatus === "completed") {
                                                batchSuccess++;
                                                retrySuccess++;
                                                const idx = failedStoryboards.findIndex((f) => f.id === sb.id);
                                                if (idx > -1) failedStoryboards.splice(idx, 1);
                                            } else {
                                                retryFail++;
                                            }
                                        } catch {
                                            retryFail++;
                                        }
                                    }
                                    
                                    if (batchSuccess === 0) break; // 本次重试无进展，停止重试
                                }
                                
                                // 重试完成后使用 projectsRef 获取最新状态
                                const finalProject = projectsRef.current.find((p) => p.id === activeProjectId);
                                const finalFailed = (finalProject?.storyboards || []).filter((s) => s.shotStatus === "failed").length;
                                const finalSucceeded = (finalProject?.storyboards || []).filter((s) => s.shotStatus === "completed").length;
                                
                                if (finalFailed === 0) {
                                    antMessage.success(`分镜完成（含自动重试）：共 ${entries.length} 章，全部成功！`);
                                } else if (retrySuccess > 0) {
                                    antMessage.warning(`分镜完成（含自动重试）：共 ${entries.length} 章，成功 ${finalSucceeded}，失败 ${finalFailed}（重试修复 ${retrySuccess} 个）`);
                                } else {
                                    antMessage.error(`分镜完成（含自动重试）：共 ${entries.length} 章，成功 ${finalSucceeded}，失败 ${finalFailed}（重试均失败，可手动重试）`);
                                }
                                setPipelineStatus("");
                                
                                // 重试后检查是否继续 autoPilot
                                if (autoPilot !== "off") {
                                    const boardsLike: Storyboard[] = (finalProject?.storyboards || [])
                                        .filter((s) => s.shotStatus === "completed" && !!s.content && s.content.trim().length > 0)
                                        .map((s) => ({
                                            id: s.id,
                                            groupIndex: s.groupIndex,
                                            content: s.content,
                                            shotStatus: "completed" as const,
                                            videoStatus: s.videoStatus || "idle" as const,
                                        }));
                                    if (boardsLike.length > 0) {
                                        setTimeout(() => {
                                            autoPilotContinueFromStoryboards(boardsLike);
                                        }, 100);
                                    }
                                }
                            })();
                        } else {
                            // 不自动重试或无失败，直接汇总
                            if (failed === 0) {
                                antMessage.success(`分镜完成：共 ${task.totalCount} 章，全部成功`);
                            } else if (succeeded > 0) {
                                antMessage.warning(`分镜完成：共 ${task.totalCount} 章，成功 ${succeeded}，失败 ${failed}；失败章节可单独"重新生成"`);
                            } else {
                                antMessage.error(`分镜失败：共 ${task.totalCount} 章，全部失败；请检查文本模型是否可用、额度是否充足`);
                            }
                            
                            // 一键出片：分镜剧本完成后自动提取资产 → 生图 → 生视频
                            if (autoPilot !== "off") {
                                const boardsLike: Storyboard[] = entries
                                    .filter((e) => isShotReady(e))
                                    .map((e) => ({
                                        id: nanoid(),
                                        groupIndex: groupMap[e.groupIndex] ?? e.groupIndex,
                                        content: e.content ?? "",
                                        shotStatus: "completed" as const,
                                        videoStatus: "idle" as const,
                                    }));
                                if (boardsLike.length > 0) {
                                    setTimeout(() => {
                                        autoPilotContinueFromStoryboards(boardsLike);
                                    }, 100);
                                }
                            }
                        }
                    } else if (task.status === "canceled") {
                        // 用户中途取消：保留已产出分镜，结果条目列表可能少于 expected
                        const produced = entries.length;
                        antMessage.info(`已停止分镜任务（已产出 ${produced} 条分镜）`);
                    } else {
                        antMessage.error(`分镜任务失败：${task.error || "未知错误"}`);
                    }
                }
            } catch {
                // 轮询单次失败不中断，但连续失败超过阈值时停止（任务可能已被删除或后端不可用）
                consecutiveFailures += 1;
                if (consecutiveFailures >= MAX_FAILURES) {
                    stopPolling();
                    setParsingStoryboard(false);
                    setAbortController(null);
                    setPipelineStatus("");
                    updateProject((p) => ({ ...p, storyboardTaskId: undefined, storyboardGroupMap: undefined, updatedAt: Date.now() }));
                    antMessage.error("分镜任务轮询连续失败，已停止恢复（任务可能已被删除或后端不可用）");
                }
            }
        };

        // 立即拉一次，随后每 3 秒轮询
        void poll();
        storyboardPollRef.current = setInterval(() => void poll(), 3000);
    };

    /**
     * 开始分镜（后端任务化版）：提交分镜任务到后端 → worker 逐章调文本模型 → 前端轮询拿回进度与已产出分镜。
     * 刷新/重开页面后，只要任务还在跑，loadEffect 会据 storyboardTaskId 自动恢复轮询，不再丢失进度。
     */
    const handleParseStoryboards = async () => {
        if (!activeProject) { antMessage.warning("请先选择或新建项目"); return; }
        if (!activeProject.script.trim()) { antMessage.warning("请先输入或导入剧本内容"); return; }
        if (!isAiConfigReady(effectiveConfig, effectiveConfig.textModel || defaultConfig.textModel)) {
            antMessage.warning("请先完成文本模型配置");
            openConfigDialog(true);
            return;
        }
        // 后端任务 API 需要登录（走 UserAuth 中间件）；未登录时回退到前端直连模式
        if (!token) {
            antMessage.warning("登录后可使用后端分镜任务（支持刷新恢复进度），正在以前端模式分镜…");
            return handleParseStoryboardsLocal();
        }

        // 0) 资产生成已下推到分镜剧本产出后（详见 pollStoryboardTask 终态分支 / handleParseStoryboardsLocal 末尾）

        // 1) 解析/刷新章节目录（丢弃第1章前的简介）。已解析过则沿用现有目录，避免打乱已勾选的组。
        const parsed = chapters.length > 0 ? chapters : parseChapters(activeProject.script);
        if (chapters.length === 0) {
            updateProject((p) => ({ ...p, chapters: parsed, updatedAt: Date.now() }));
            setExpandedChapterIds(new Set(parsed[0] ? [parsed[0].id] : []));
            seedAllGroups(parsed.length);
        }

        // 2) 按每 CHAPTERS_PER_GROUP 章分组
        const groups: Chapter[][] = [];
        for (let i = 0; i < parsed.length; i += CHAPTERS_PER_GROUP) {
            groups.push(parsed.slice(i, i + CHAPTERS_PER_GROUP));
        }

        // 2.1) 只处理勾选的组；未勾选任何组 = 处理全部组
        const targetGroups = selectedGroups.size > 0
            ? [...selectedGroups].filter((gi) => gi >= 0 && gi < groups.length).sort((a, b) => a - b)
            : groups.map((_, gi) => gi);
        if (targetGroups.length === 0) { antMessage.warning("请先勾选要分镜的组"); return; }

        // 3) 构建提交给后端的章节数组（仅勾选的组，按顺序排列）+ 组索引映射
        //    P1 b1/b2：每项带 shotCount，让后端按该数量产出分镜（===SHOT=== 分隔）
        const chaptersForTask = targetGroups.map((gi) => ({
            title: groups[gi].map((c) => c.title).join(" / "),
            content: groups[gi].map((c) => c.content).join("\n\n"),
            shotCount: Math.max(1, chapterShotCounts[gi] ?? 1),
        }));
        const groupMap = targetGroups; // 任务结果索引 i → 原始组索引 targetGroups[i]

        // 4) 解析渠道信息（与 callTextModel 一致的渠道路由：textModel + textChannelId）
        const textModel = effectiveConfig.textModel || defaultConfig.textModel;
        const scopedConfig: AiConfig = {
            ...effectiveConfig,
            model: textModel,
            activeChannelId: effectiveConfig.textChannelId || effectiveConfig.activeChannelId,
            textChannelId: effectiveConfig.textChannelId,
        };
        const kind = activeChannelKind(scopedConfig);
        const channelId = kind === "remote" ? (channelIdForActiveModel(scopedConfig) || "") : "";
        const userChannelId = kind === "local" ? (channelIdForActiveModel(scopedConfig) || "") : "";

        // 5) 提交分镜任务到后端
        const controller = new AbortController();
        setAbortController(controller);
        setParsingStoryboard(true);
        setPhase("storyboard"); // 4 段 stepper：当前处于「分镜剧本」阶段
        setPipelineProgress({ current: 0, total: targetGroups.length });
        setPipelineStatus(`正在提交分镜任务（共 ${targetGroups.length} 章）…`);

        try {
            const task = await createStoryboardTask({
                sourceId: activeProject.id,
                model: textModel,
                channelId,
                userChannelId,
                shotDuration,
                scriptPrompt,
                chapters: chaptersForTask,
                assets: assets.length > 0 ? assets.map((a) => ({
                    alias: a.alias, type: a.type, description: a.description, name: a.name,
                })) : undefined,
            }, token);

            // 6) 记录任务 ID 与组映射到项目（刷新后据此时恢复轮询）
            updateProject((p) => ({
                ...p,
                storyboardTaskId: task.id,
                storyboardGroupMap: groupMap,
                updatedAt: Date.now(),
            }));

            setPipelineStatus(`后端正在生成分镜剧本（共 ${targetGroups.length} 章，逐章产出中）…`);

            // 7) 启动轮询
            pollStoryboardTask(task.id, groupMap, controller);
        } catch (err) {
            const msg = (err as Error).message || "提交分镜任务失败";
            // 后端任务提交失败时，回退到前端直连模式
            antMessage.warning(`后端任务提交失败（${msg}），正在以前端模式分镜…`);
            setParsingStoryboard(false);
            setAbortController(null);
            setPipelineStatus("");
            return handleParseStoryboardsLocal();
        }
    };

    /**
     * 前端直连版分镜（回退用）：未登录或后端任务提交失败时，走原来的前端并发调文本模型逻辑。
     * 逻辑与任务化前完全一致，只是没有刷新恢复能力。
     */
    const handleParseStoryboardsLocal = async () => {
        if (!activeProject) { antMessage.warning("请先选择或新建项目"); return; }
        if (!activeProject.script.trim()) { antMessage.warning("请先输入或导入剧本内容"); return; }
        if (!isAiConfigReady(effectiveConfig, effectiveConfig.textModel || defaultConfig.textModel)) {
            antMessage.warning("请先完成文本模型配置"); openConfigDialog(true); return;
        }

        // 0) 资产生成已下推到分镜剧本产出后（详见本函数末尾 finally 之外的分支 / pollStoryboardTask 终态分支）

        // 1) 解析/刷新章节目录
        const parsed = chapters.length > 0 ? chapters : parseChapters(activeProject.script);
        if (chapters.length === 0) {
            updateProject((p) => ({ ...p, chapters: parsed, updatedAt: Date.now() }));
            setExpandedChapterIds(new Set(parsed[0] ? [parsed[0].id] : []));
            seedAllGroups(parsed.length);
        }

        // 2) 按每 CHAPTERS_PER_GROUP 章分组
        const groups: Chapter[][] = [];
        for (let i = 0; i < parsed.length; i += CHAPTERS_PER_GROUP) {
            groups.push(parsed.slice(i, i + CHAPTERS_PER_GROUP));
        }

        // 2.1) 只处理勾选的组；未勾选任何组 = 处理全部组
        const targetGroups = selectedGroups.size > 0
            ? [...selectedGroups].filter((gi) => gi >= 0 && gi < groups.length).sort((a, b) => a - b)
            : groups.map((_, gi) => gi);
        if (targetGroups.length === 0) { antMessage.warning("请先勾选要分镜的组"); return; }

        // 3) 多组并发改写为分镜剧本
        const controller = new AbortController();
        setAbortController(controller);
        setParsingStoryboard(true);
        setPipelineProgress({ current: 0, total: targetGroups.length });

        try {
            const totalTarget = targetGroups.length;
            let succeeded = 0;
            let done = 0;
            const failedChapters: string[] = [];
            const generatedBoards: Storyboard[] = []; // 本批成功产出的分镜，供后续资产生成用
            const MAX_ATTEMPTS = 2;
            const queue = [...targetGroups];
            const runOne = async (gi: number) => {
                const startCh = gi * CHAPTERS_PER_GROUP + 1;
                const endCh = startCh + groups[gi].length - 1;
                const chLabel = startCh === endCh ? `第${startCh}章` : `第${startCh}~${endCh}章`;
                let lastErr = "";
                try {
                    for (let attempt = 1; attempt <= MAX_ATTEMPTS && !controller.signal.aborted; attempt++) {
                        try {
                            // P1 b1/b2：传入 chapterShotCounts[gi] 让模型按 shotCount 输出 N 条
                            const blocks = await rewriteGroupToStoryboards(groups[gi], gi, controller, chapterShotCounts[gi] ?? 1);
                            if (controller.signal.aborted) return;
                            if (blocks.length > 0 && blocks.some((b) => b.trim())) {
                                const newBoards: Storyboard[] = blocks
                                    .filter((b) => b.trim())
                                    .map((b) => ({ id: nanoid(), groupIndex: gi, content: b, shotStatus: "completed" as const, videoStatus: "idle" as const }));
                                generatedBoards.push(...newBoards);
                                updateProject((p) => ({
                                    ...p,
                                    storyboards: [...(p.storyboards || []).filter((s) => s.groupIndex !== gi), ...newBoards]
                                        .sort((a, b) => a.groupIndex - b.groupIndex),
                                    updatedAt: Date.now(),
                                }));
                                succeeded += 1;
                                return;
                            }
                            lastErr = "模型返回为空";
                        } catch (err) {
                            if (controller.signal.aborted) return;
                            lastErr = (err as Error).message;
                        }
                    }
                    if (controller.signal.aborted) return;
                    failedChapters.push(chLabel);
                    // 前端兜底路径：失败分镜不再写"⚠..."字符串，改用 shotStatus="failed" + shotError。
                    // 保持与后端 storyboardTasks.result 一致的字段语义；storyboardIsReady 据此统一过滤。
                    const placeholder: Storyboard = {
                        id: nanoid(),
                        groupIndex: gi,
                        content: "",
                        shotStatus: "failed",
                        shotError: `${chLabel} 未生成（${lastErr || "生成失败"}），可点"重新生成"重试`,
                        videoStatus: "idle",
                    };
                    updateProject((p) => ({
                        ...p,
                        storyboards: [...(p.storyboards || []).filter((s) => s.groupIndex !== gi), placeholder]
                            .sort((a, b) => a.groupIndex - b.groupIndex),
                        updatedAt: Date.now(),
                    }));
                } finally {
                    done += 1;
                    setPipelineProgress({ current: done, total: totalTarget });
                }
            };
            const worker = async () => {
                while (queue.length > 0 && !controller.signal.aborted) {
                    const gi = queue.shift();
                    if (gi === undefined) break;
                    await runOne(gi);
                }
            };
            setPipelineStatus(`正在并发生成分镜剧本（共 ${totalTarget} 章，并发 ${storyboardConcurrency}）`);
            const workerCount = Math.max(1, Math.min(storyboardConcurrency, totalTarget));
            await Promise.all(Array.from({ length: workerCount }, () => worker()));
            if (!controller.signal.aborted) {
                if (succeeded === totalTarget) {
                    antMessage.success(`分镜完成：应生成 ${totalTarget} 章，成功 ${succeeded}，失败 0`);
                } else if (succeeded > 0) {
                    antMessage.warning(`分镜完成：应生成 ${totalTarget} 章，成功 ${succeeded}，失败 ${failedChapters.length}（${failedChapters.join("、")}）；失败章节已在列表标注，可单独"重新生成"`);
                } else {
                    antMessage.error(`分镜失败：应生成 ${totalTarget} 章，成功 0，失败 ${failedChapters.length}（${failedChapters.join("、")}）；请检查文本模型是否可用、额度是否充足`);
                }

                // autoPilot：自动提取资产 → 生图 → 生视频，跳过所有预览闸门
                // 需要 await，否则 finally 会立刻清理 autoPilot 的进度状态
                if (autoPilot !== "off" && generatedBoards.length > 0) {
                    await autoPilotContinueFromStoryboards(generatedBoards);
                }
            }
        } finally {
            // autoPilot 模式下 autoPilotContinueFromStoryboards 内部已完成清理，
            // 这里只在非 autoPilot 模式下清除状态，避免覆盖 autoPilot 的最终状态
            if (autoPilot === "off") {
                setParsingStoryboard(false);
                setAbortController(null);
                setPipelineStatus("");
            }
        }
    };

    /** 手动新增一个空白分镜剧本（追加到指定分组，null = 最后一组） */
    const addStoryboard = (targetGroupIndex?: number | null) => {
        if (!activeProject) { antMessage.warning("请先选择或新建项目"); return; }
        const effectiveIndex = targetGroupIndex ?? (groupCount > 0 ? groupCount - 1 : 0);
        const board: Storyboard = { id: nanoid(), groupIndex: effectiveIndex, content: "", videoStatus: "idle" };
        updateProject((p) => ({ ...p, storyboards: [...(p.storyboards || []), board], updatedAt: Date.now() }));
        setAddStoryboardGroupIndex(null);
        // 直接打开编辑弹窗
        setViewStoryboardId(board.id);
        setStoryboardDraft("");
        setStoryboardGroupDraft(effectiveIndex);
    };

    /** 删除某个分镜剧本（右侧列表会自动按顺序重排编号） */
    const deleteStoryboard = (id: string) => {
        updateProject((p) => ({ ...p, storyboards: (p.storyboards || []).filter((s) => s.id !== id), updatedAt: Date.now() }));
        if (viewStoryboardId === id) setViewStoryboardId(null);
    };
    /** 批量删除勾选的分镜剧本（带确认框） */
    const batchDeleteStoryboards = () => {
        if (!activeProject) return;
        const ids = selectedStoryboardIds;
        if (ids.size === 0) { antMessage.warning("请先勾选要删除的分镜剧本"); return; }
        antModal.confirm({
            title: "批量删除分镜剧本",
            content: `确定删除选中的 ${ids.size} 条分镜剧本吗？此操作不可恢复。`,
            okText: "删除", okButtonProps: { danger: true }, cancelText: "取消",
            onOk: () => {
                updateProject((p) => ({ ...p, storyboards: (p.storyboards || []).filter((s) => !ids.has(s.id)), updatedAt: Date.now() }));
                // 如果当前查看的分镜在删除集合中,关闭查看弹窗
                if (viewStoryboardId && ids.has(viewStoryboardId)) setViewStoryboardId(null);
                // 清空勾选
                setSelectedStoryboardIds(new Set());
                antMessage.success(`已删除 ${ids.size} 条分镜剧本`);
            },
        });
    };

    /** 保存分镜剧本编辑内容 */
    const saveStoryboardContent = (id: string, content: string) => {
        updateProject((p) => ({ ...p, storyboards: (p.storyboards || []).map((s) => s.id === id ? { ...s, content } : s), updatedAt: Date.now() }));
    };

    /** 修改分镜所属分组，并重新排序 */
    const moveStoryboardToGroup = (id: string, newGroupIndex: number) => {
        updateProject((p) => ({
            ...p,
            storyboards: [...(p.storyboards || []).map((s) => s.id === id ? { ...s, groupIndex: newGroupIndex } : s)]
                .sort((a, b) => a.groupIndex - b.groupIndex),
            updatedAt: Date.now(),
        }));
    };

    /**
     * 解析一条分镜剧本里 @到的角色/资产关联状态，供 UI 展示"哪里缺东西"。
     * 每个 @名字对应一项：linked=素材库里有同别名资产（会带参考图），missing=找不到；
     * 并标出该项是否是"人物(character)"——人物是必要资产，缺失需高亮提醒。
     */
    const getStoryboardAssetLinks = (content: string): { name: string; linked: boolean; isCharacter: boolean }[] => {
        return extractMentions(content).map((name) => {
            const asset = assets.find((a) => a.alias === name);
            // 找到同名资产：按其真实类型判断是否人物；找不到：默认按"人物"对待（分镜里 @ 的绝大多数是人物，缺人物最关键）
            return { name, linked: !!asset, isCharacter: asset ? asset.type === "character" : true };
        });
    };

    /** 重新生成某个分镜剧本（用其对应章节的正文重跑，整章整合为 1 条替换掉这一条） */
    const regenerateStoryboard = async (id: string) => {
        if (!activeProject) return;
        const board = (activeProject.storyboards || []).find((s) => s.id === id);
        if (!board) return;
        if (!isAiConfigReady(effectiveConfig, effectiveConfig.textModel || defaultConfig.textModel)) {
            antMessage.warning("请先完成文本模型配置"); openConfigDialog(true); return;
        }
        const parsed = activeProject.chapters || parseChapters(activeProject.script);
        const groupChapters = parsed.slice(board.groupIndex * CHAPTERS_PER_GROUP, board.groupIndex * CHAPTERS_PER_GROUP + CHAPTERS_PER_GROUP);
        if (groupChapters.length === 0) { antMessage.warning("找不到该分镜对应的章节内容"); return; }
        const controller = new AbortController();
        try {
            antMessage.loading({ content: "重新生成中…", key: "regen-sb" });
            // P1 b1/b2：重新生成也按章节的 shotCount 输出（多镜则拼回多条，但 regenerateStoryboard 只把 blocks[0] 替换当前 board 内容——配合 b2「扩展板」未来可拆多 board）
            const blocks = await rewriteGroupToStoryboards(groupChapters, board.groupIndex, controller, chapterShotCounts[board.groupIndex] ?? 1);
            const replacement = blocks[0] ?? "";
            // 重新生成：成功（blocks 非空）→ 标 completed、清 shotError；失败（blocks 空）→ 保留占位 shotStatus="failed"。
            const newShotStatus: "completed" | "failed" = replacement.trim() ? "completed" : "failed";
            const newShotError = replacement.trim() ? undefined : `重新生成失败（${board.shotError || "模型返回为空"}）`;
            updateProject((p) => ({ ...p, storyboards: (p.storyboards || []).map((s) => s.id === id ? { ...s, content: replacement, shotStatus: newShotStatus, shotError: newShotError } : s), updatedAt: Date.now() }));
            if (newShotStatus === "completed") {
                antMessage.success({ content: "已重新生成该分镜剧本", key: "regen-sb" });
            } else {
                const parsed = parseVendorError(newShotError || "重新生成失败", "text");
                const content = `${parsed.icon} ${parsed.title}${parsed.action ? ` — ${parsed.action}` : ""}`;
                if (parsed.level === "warn") antMessage.warning({ content, key: "regen-sb", duration: 6 });
                else if (parsed.level === "info") antMessage.info({ content, key: "regen-sb", duration: 6 });
                else antMessage.error({ content, key: "regen-sb", duration: 6 });
            }
        } catch (err) {
            antMessage.error({ content: `重新生成失败: ${(err as Error).message}`, key: "regen-sb" });
        }
    };

    /**
     * 基于单个分镜剧本生成一个视频。
     * boardArg：自动化流程里刚产出、还没写进 activeProject 状态的分镜剧本，
     * 直接传入避免因 setState 未刷新而查不到。
     */
    /** 按并发上限从队列取任务启动；running 达到上限就等待，有名额就补位 */
    const pumpVideoQueue = () => {
        const gate = videoGateRef.current;
        while (gate.running < Math.max(1, videoConcurrency) && gate.queue.length > 0) {
            const start = gate.queue.shift();
            if (!start) break;
            gate.running += 1;
            start();
        }
    };

    /** 释放一个视频并发名额，并尝试启动队列里的下一个 */
    const releaseVideoSlot = () => {
        const gate = videoGateRef.current;
        gate.running = Math.max(0, gate.running - 1);
        pumpVideoQueue();
    };

    const generateVideoFromStoryboard = (id: string, boardArg?: Storyboard, onSettled?: (ok: boolean) => void, force?: boolean) => {
        if (!activeProject) return;
        const board = boardArg || (activeProject.storyboards || []).find((s) => s.id === id);
        if (!board?.content.trim()) { antMessage.warning("该分镜剧本内容为空"); onSettled?.(false); return; }
        // 验证分镜内容格式是否正确（防止原小说文本被误当作分镜脚本）
        const validation = validateStoryboardContent(board.content);
        if (!validation.isLikelyStoryboard) {
            const reasonText = validation.reasons.join("、");
            antMessage.warning(`分镜「${board.groupIndex + 1}」的内容格式可能不正确：${reasonText}。建议点击「重新生成」或手动编辑分镜剧本。`);
        }
        // 已有视频且非强制重新生成 → 跳过
        if (!force && board.videoStatus === "success" && board.videoUrl) {
            antMessage.info(`分镜「${board.groupIndex + 1}」已有视频，跳过生成（如需覆盖请点「重新生成」按钮）`);
            onSettled?.(true);
            return;
        }
        if (!isAiConfigReady(effectiveConfig, effectiveConfig.videoModel || defaultConfig.videoModel)) {
            antMessage.warning("请先完成视频模型配置"); openConfigDialog(true); onSettled?.(false); return;
        }
        // 自动关联资产：从分镜剧本头部的"出场角色/场景"抽取 @别名，匹配素材库资产（与视频创作台一致）。
        // 警告只针对"出场角色"行未匹配上的名字；场景/道具不影响参考图，未匹配一律静默。
        const header = extractStoryboardHeader(board.content);
        const linkedIds: string[] = [];
        // 同时关联"出场角色:"行与"场景:"行，未匹配的角色才在 UI 上提示
        const headerMentions = [...header.characters, ...header.scenes];
        for (const mention of new Set(headerMentions)) {
            const asset = assets.find((a) => a.alias === mention);
            if (asset) linkedIds.push(asset.id);
        }
        const unmatched = header.characters.filter((m) => !assets.find((a) => a.alias === m));
        if (unmatched.length === 0) {
            // 所有 @角色 都有对应素材 → 直接生成
            proceedGenerateVideo(id, linkedIds, onSettled);
            return;
        }
        // 严苛闸门：autoPilot 任意"开"档（half/full）默认阻塞未匹配角色，必须补图或开豁免
        //   用户期望：开了「一键出片」任何档位都不能"假装没事"地退化到无参考图模式
        const strictBlock = autoPilot !== "off" && !allowMissingAssets;
        if (strictBlock) {
            antMessage.warning(`分镜「${board.groupIndex + 1}」缺 ${unmatched.length} 个角色参考图（${unmatched.join(",")}）。请先在「资产」面板补充图片，或开启下方的「允许缺资产时出片」开关再试。`);
        }
        // 自动链 + 未触发严苛闸门 → 静默用无参考图模式生成，toast 提示但不阻断
        if (autoPilot !== "off" && !strictBlock) {
            antMessage.info(`autoPilot：分镜「${board.groupIndex + 1}」角色 [${unmatched.join(",")}] 无参考图，将用无参考模式生成`);
            proceedGenerateVideo(id, linkedIds, onSettled);
            return;
        }
        // 手动模式或严苛闸门触发：弹二次确认（持久化：状态写到 pendingShotRun，关闭即清，不打断用户）
        setPendingShotRun({
            storyboardId: id,
            unmatched,
            linkedIds,
            storyboardContent: board.content,
            onSettled,
        });
    };

    /**
     * 实际入队生成视频（受并发上限约束）。
     * 由 generateVideoFromStoryboard（无 unmatched 时）或 二次确认 Modal 「仍然继续」按钮触发。
     */
    const proceedGenerateVideo = (id: string, linkedIds: string[], onSettled?: (ok: boolean) => void) => {
        if (!activeProject) return;
        const board = (activeProject.storyboards || []).find((s) => s.id === id);
        if (!board?.content.trim()) { antMessage.warning("该分镜剧本内容为空"); onSettled?.(false); return; }
        // 每个分镜剧本 = 一个分镜 = 一个视频，用 storyboardId 建立关联
        const shot: Shot = {
            id: nanoid(),
            title: "",
            content: board.content,
            duration: shotDuration,
            status: "idle",
            selected: false,
            referencedAssetIds: linkedIds,
            storyboardId: id,
        };
        // 先落一个待生成的分镜，并把分镜剧本标记为"排队中(queued)"，让用户看到已进入队列
        updateProject((p) => {
            // 清理同一 storyboardId 的旧 shot 轮询定时器，避免旧轮询泄漏（重新生成时旧 shot 被 filter 掉但轮询仍在跑）
            const oldShots = p.shots.filter((s) => s.storyboardId === id);
            for (const old of oldShots) {
                const timer = pollingTimersRef.current.get(old.id);
                if (timer) { clearInterval(timer); pollingTimersRef.current.delete(old.id); }
            }
            return {
                ...p,
                shots: [...p.shots.filter((s) => s.storyboardId !== id), shot],
                storyboards: (p.storyboards || []).map((s) => s.id === id ? { ...s, videoStatus: "queued" } : s),
                updatedAt: Date.now(),
            };
        });
        // 入队：受并发上限约束，真正开始时才把分镜剧本置为 generating；终态时释放名额并补位，并把成败回传给调用方（用于批量统计）
        videoGateRef.current.queue.push(() => {
            const controller = new AbortController();
            // 传入当前项目所有 shots（含本次新 shot），让 buildVideoInput 能找到上一条成功视频做帧衔接
            const allShots = (projectsRef.current.find((p) => p.id === activeProjectId)?.shots ?? []).filter((s) => s.storyboardId !== id);
            allShots.push(shot);
            updateProject((p) => ({ ...p, storyboards: (p.storyboards || []).map((s) => s.id === id ? { ...s, videoStatus: "generating" } : s) }));
            void generateShot(shot, allShots, allShots.indexOf(shot), controller, (ok) => { releaseVideoSlot(); onSettled?.(!!ok); });
        });
        pumpVideoQueue();
    };

    /** 切换某个分镜的批量生成勾选状态 */
    const toggleStoryboardSelect = (id: string) => {
        setSelectedStoryboardIds((prev) => { const next = new Set(prev); next.has(id) ? next.delete(id) : next.add(id); return next; });
    };
    /** 全选/取消全选所有可生成视频的分镜 */
    const toggleAllStoryboardSelect = (checked: boolean) => {
        if (!activeProject) return;
        const allIds = (activeProject.storyboards || []).filter((s) => storyboardIsReady(s)).map((s) => s.id);
        setSelectedStoryboardIds(checked ? new Set(allIds) : new Set());
    };
    const selectedStoryboardCount = useMemo(
        () => (activeProject?.storyboards?.filter((s) => selectedStoryboardIds.has(s.id) && storyboardIsReady(s)).length || 0),
        [activeProject, selectedStoryboardIds],
    );

    /**
     * autoPilot 专用：分镜剧本完成后 → 自动提取资产 → 生图 → 生视频，跳过所有预览闸门
     * 失败策略：生图阶段**全失败**时停住 + 弹错，不再装作继续（避免"视频生成中 0镜"的假状态）；
     * 抽资产或生图部分失败（还有成功）时记入 failedAssets + toast 提示，继续走 video 阶段。
     */
    const autoPilotContinueFromStoryboards = async (boardsLike: Storyboard[]) => {
        console.log("[autoPilot] start with", boardsLike.length, "storyboards, activeProject has", activeProject?.storyboards?.length, "total");
        setPhase("assets_prompt");
        setAssetStep("prompt");
        setPipelineStatus(`autoPilot：正在提取 ${boardsLike.length} 条分镜的角色/场景/道具...`);

        const assetController = new AbortController();
        setAbortController(assetController);
        setParsingStoryboard(true);

        let extractedItems: Array<{ id: string; name: string; type: AssetType; description: string; sourceSnippet?: string; isExisting?: boolean }> = [];
        try {
            const { count, items } = await extractAssetsForAutoChain(boardsLike, assetController);
            extractedItems = items;
            if (count > 0) {
                setExtractedListExpanded(true);
                antMessage.info(`autoPilot：已提取 ${count} 个资产，可在右侧「资产」面板查看提示词`);
            }
        } catch (error) {
            antMessage.warning(`autoPilot 资产提取警告: ${(error as Error).message}`);
        } finally {
            setParsingStoryboard(false);
        }

        if (assetController.signal.aborted) return;

        // 自动生图：返回值 {success, fail} 让本函数判断"全失败时停住"
        setPhase("assets_image");
        setAssetStep("image");
        setPipelineStatus("autoPilot：正在为提取的资产生成图片...");
        let imgResult: { success: number; fail: number } = { success: 0, fail: 0 };
        try {
            imgResult = await generateImagesFromExtracted(assetController, extractedItems);
        } catch (error) {
            antMessage.warning(`autoPilot 生图警告: ${(error as Error).message}`);
        }

        if (assetController.signal.aborted) return;

        // 全失败/空跑停住：避免"视频生成中 0镜"的假状态。
        // 已有部分成功 → 继续跑视频（无参考图模式兜底）；0 成功且无失败=没实际执行生图 → 停下；0 成功且有失败 → 停下让用户介入。
        const noProgress = imgResult.success === 0;
        if (noProgress) {
            antModal.error({
                title: "autoPilot：资产生图全部失败，已停住",
                content: (
                    <div className="space-y-2 text-sm">
                        <p>共 <b className="text-red-500">{imgResult.fail}</b> 个资产生图失败，没有产出可绑定的图片。</p>
                        <p className="text-stone-500">可能原因：图片模型不被上游接受、余额不足、限流、或网络问题。当前配置图片模型：<code className="rounded bg-stone-100 px-1 dark:bg-stone-800">{effectiveConfig.imageModel || defaultConfig.imageModel}</code></p>
                        <p className="text-stone-500">处理方式：</p>
                        <ul className="list-disc pl-5 text-stone-500">
                            <li>在配置弹窗换 <code className="rounded bg-stone-100 px-1 dark:bg-stone-800">imageConcurrency</code> 为 1 测试，避开并发限流</li>
                            <li>换用 agnes-image-* / gpt-image-* 系列图片模型</li>
                            <li>查 docker logs <code className="rounded bg-stone-100 px-1 dark:bg-stone-800">freedom</code> 看上游具体错误</li>
                        </ul>
                        <p className="text-stone-500">修正后点 <b>剧本转资产</b> 重新跑一遍生图，再点 <b>开始分镜</b> 后选 chip=「允许缺资产·开」跑视频。</p>
                    </div>
                ),
                okText: "我知道了",
            });
            setPhase("idle");
            setPipelineStatus("autoPilot 已停住：资产生图全部失败");
            return;
        }

        // ③ 对应分镜：把生好的图按 alias 与分镜 @名字 做匹配，刷新覆盖报告（autoPilot 跳过缺失弹窗，仅提示）
        setPhase("assets_reconcile");
        setAssetStep("reconcile");
        setPipelineStatus("autoPilot：正在把图片对应到分镜剧本…");
        const report = computeCoverage(boardsLike);
        if (report.totalMissing > 0) {
            antMessage.warning(`autoPilot：图片已对应分镜，仍有 ${report.totalMissing} 个角色缺图，将用无参考图模式出片`);
        }

        // 自动批量生视频（跳过缺素材弹窗），传入共享 controller 实现统一停止
        // 注意：generateAllStoryboardVideos 是异步提交，视频生成完成后的状态更新由其内部 onSettled 回调处理
        // 第二参数 skipCoverageCheck=true：autoPilot 链刚跑完 extract+生图，没必要再走严苛闸门
        //   （闸门保护的对象是用户手动路径——自动链已经"知道缺资产还要继续"了）
        setAssetStep("");
        setPhase("video");
        setPipelineStatus("autoPilot：正在批量生成视频（缺素材的分镜将用无参考图模式）...");
        generateAllStoryboardVideos(assetController, /* skipCoverageCheck */ true);
        // autoPilotContinueFromStoryboards 到此结束，视频生成完成后的状态由 generateAllStoryboardVideos 内部处理
    };

    /**
     * P1 改造 A：分镜剧本预览闸 — 用户确认所有分镜剧本后，调抽取资产生成 + 进资产 preview 闸
     * 接收 modifiedEntries（用户编辑过/重新生成过/跳过过的条目），过滤掉跳过的，只对剩余的抽资产
     */
    const handleConfirmStoryboardPreview = async () => {
        if (!pendingStoryboardPreview) return;
        const surviving = pendingStoryboardPreview.entries
            .filter((e) => !e.snapshot.content.startsWith("⚠") && e.snapshot.content.trim().length > 0)
            .map((e) => e.snapshot);
        setPendingStoryboardPreview(null);
        if (surviving.length === 0) {
            antMessage.info("已全部跳过，自动链中断");
            return;
        }
        // 抽资产 → 进资产 preview 闸
        const assetController = new AbortController();
        setAbortController(assetController);
        setParsingStoryboard(true);
        setPipelineStatus("正在从分镜剧本中提取角色/场景/道具...");
        try {
            await extractAssetsForAutoChain(surviving, assetController);
            setPendingAssetPreview({ boards: surviving });
            setExtractedListExpanded(true);
            setPipelineStatus("请预览要生成的资产清单");
        } catch (error) {
            antMessage.warning(`分镜资产生成失败: ${(error as Error).message}`);
        } finally {
            setParsingStoryboard(false);
            setAbortController(null);
        }
    };

    const handleSkipStoryboardPreview = () => {
        if (!pendingStoryboardPreview) return;
        setPendingStoryboardPreview(null);
        antMessage.info("已跳过所有分镜剧本，自动链中断");
    };

    /**
     * 自动链 preview 闸「确认生成」：从 extractedAssets 状态批量生图，完后接 generateAllStoryboardVideos 出片。
     * 用户在 Modal 里点「取消跳过」则跳过资产生成直接出片。
     */
    const handleConfirmAssetPreview = async () => {
        if (!pendingAssetPreview) return;
        setPendingAssetPreview(null);
        if (extractedAssets.length === 0) {
            // 用户在 modal 里删光了所有资产 → 直接接视频生成
            antMessage.info("已跳过资产生成（清单为空），开始生成视频");
            generateAllStoryboardVideos();
            setPhase("video");
            return;
        }
        try {
            await generateImagesFromExtracted();
            antMessage.success("分镜图片生成完成，开始生成视频");
        } catch (error) {
            const parsed = parseVendorError((error as Error).message, "image");
            const content = `${parsed.icon} ${parsed.title}${parsed.action ? ` — ${parsed.action}` : ""}`;
            if (parsed.level === "warn") antMessage.warning({ content, duration: 5 });
            else if (parsed.level === "info") antMessage.info({ content, duration: 5 });
            else antMessage.error({ content, duration: 5 });
        }
        // 不管图片生图成败，**都接** generateAllStoryboardVideos（视频可独立跑，缺参考图由 Modal 兜底）
        generateAllStoryboardVideos();
        setPhase("video");
    };

    const handleSkipAssetPreview = () => {
        if (!pendingAssetPreview) return;
        setPendingAssetPreview(null);
        antMessage.info("已跳过资产生成，开始生成视频");
        generateAllStoryboardVideos();
        setPhase("video");
    };

    /**
     * P1 改造 B-2：failedAssets 一键重试入口
     * 把 failedAssets 列表里的条目重新塞回 extractedAssets 状态（清掉 failedAssets），
     * 然后调 generateImagesFromExtracted 重跑。
     */
    const handleRetryFailedAssets = async () => {
        if (failedAssets.length === 0) return;
        const recoverNames = new Set(failedAssets.map((f) => f.name));
        // 把已失败名字对应的 extractedAssets 还原（如果还在的话）
        const recoverItems = extractedAssets.filter((a) => recoverNames.has(a.name));
        if (recoverItems.length === 0) {
            antMessage.warning("失败条目已不在预览列表里，请先重新抽取资产");
            return;
        }
        setFailedAssets([]);
        setPendingAssets([]);
        try {
            await generateImagesFromExtracted();
            antMessage.success("失败资产已重试");
        } catch (error) {
            antMessage.warning(`重试失败: ${(error as Error).message}`);
        }
        // 接视频生成（与正常流程一致）
        generateAllStoryboardVideos();
        setPhase("video");
        setPendingAssetPreview(null);
    };

    /**
     * P1 改造 B-1：单条资产「重新抽取描述」
     * 调文本模型只对这条的名字+上下文抽新的 description 替换原值；不生图（生成在确认生成时统一跑）
     */
    const handleReextractAssetDescription = async (assetId: string) => {
        const target = extractedAssets.find((a) => a.id === assetId);
        if (!target) return;
        if (!activeProject) return;
        const textModel = effectiveConfig.textModel || defaultConfig.textModel;
        if (!isAiConfigReady(effectiveConfig, textModel)) {
            antMessage.warning("请先完成文本模型配置");
            return;
        }
        const boardText = (pendingAssetPreview?.boards ?? []).map((b) => b.content || "").join("\n\n").slice(0, 6000);
        const controller = new AbortController();
        const key = `reextract-${assetId}`;
        antMessage.loading({ content: `重新抽取「${target.name}」描述中...`, key });
        try {
            const items = [{ name: target.name, type: target.type, contextSnippet: target.description }];
            const described = await extractDescriptionsForAssets(items, boardText, controller);
            if (described.length > 0 && described[0].description.trim()) {
                setExtractedAssets((prev) => prev.map((x) => x.id === assetId ? { ...x, description: described[0].description } : x));
                antMessage.success({ content: `「${target.name}」描述已更新`, key });
            } else {
                antMessage.error({ content: `「${target.name}」重新抽取失败（模型返回空）`, key });
            }
        } catch (error) {
            antMessage.error({ content: `「${target.name}」重新抽取失败: ${(error as Error).message}`, key });
        }
    };

    /** 一键为指定分镜剧本生成视频（统一入队，受并发上限约束）；空集 = 全部，非空集 = 仅选中 */
    const generateAllStoryboardVideos = (abortCtrlOverride?: AbortController, skipCoverageCheck?: boolean) => {
        if (!activeProject) return;
        // 排除失败占位分镜（内容以 ⚠ 开头，非真实分镜，不拿去生成视频）
        const allBoards = (activeProject.storyboards || []).filter((s) => storyboardIsReady(s));
        if (allBoards.length === 0) { antMessage.warning("暂无可生成的分镜剧本"); return; }
        if (!isAiConfigReady(effectiveConfig, effectiveConfig.videoModel || defaultConfig.videoModel)) {
            antMessage.warning("请先完成视频模型配置"); openConfigDialog(true); return;
        }
        // P1 b5：分镜时长超过视频模型上限 → 弹警告让用户选择（不阻止，只是提醒）
        if (shotDurationExceedsLimit) {
            const ok = confirm(`分镜时长 ${shotDuration} 秒超过当前视频模型「${videoModel}」最大支持 ${videoModelMaxDuration} 秒。\n\n· 模型会按 ${videoModelMaxDuration} 秒截断，分镜后半段剧情会被丢弃\n· 建议：缩短分镜时长到 ≤ ${videoModelMaxDuration} 秒，或切到 Seedance 1.5 Pro / Kling V3 等支持更长时长的模型\n\n继续生视频？`);
            if (!ok) return;
        }
        // 决定本次要生成的分镜集合
        const targetIds = selectedStoryboardIds.size > 0 ? selectedStoryboardIds : new Set<string>();
        const selectedOrAll = targetIds.size > 0 ? allBoards.filter((b) => targetIds.has(b.id)) : allBoards;
        if (selectedOrAll.length === 0) { antMessage.warning("暂无可生成的分镜剧本"); return; }
        // 严苛闸门：autoPilot 任意"开"档（half/full）且未开豁免时，未绑定角色的分镜默认禁止出片
        //   - 单条 generateVideoFromStoryboard 也守同一规则（half 也走 modal 让用户确认）
        //   - 自动链调用时传 skipCoverageCheck=true，因为 extract+生图刚跑完，没有"未跑过"的资产
        const strictBlock = autoPilot !== "off" && !allowMissingAssets;
        if (strictBlock && !skipCoverageCheck) {
            const coverage = computeCoverage(selectedOrAll);
            const blockedIds = coverage.boardsWithMissing > 0 ? [...coverage.missingByBoard.keys()] : [];
            if (blockedIds.length > 0) {
                const missingNames = new Set<string>();
                for (const id of blockedIds) {
                    for (const n of coverage.missingByBoard.get(id) || []) missingNames.add(n);
                }
                const firstMissingId = blockedIds[0];
                antModal.warning({
                    title: "一键出片·严苛闸门已拦截",
                    content: (
                        <div className="space-y-2 text-sm">
                            <p>本次 <b className="text-red-500">{blockedIds.length}</b> 个分镜含未绑定的角色（共 {missingNames.size} 个：{[...missingNames].slice(0, 8).join("、")}{missingNames.size > 8 ? "…" : ""}）。</p>
                            <p className="text-stone-500">严苛闸门默认要求所有 <code className="rounded bg-stone-100 px-1 dark:bg-stone-800">@角色</code> 都有对应资产图才会出片，避免无参考图模式出片质量严重下降。</p>
                            <p className="text-stone-500">解决方式（任选其一）：</p>
                            <ul className="list-disc pl-5 text-stone-500">
                                <li>在右侧「资产」面板为这些角色补充/上传图片</li>
                                <li>打开右侧栏下方的「⚠️ 允许缺资产时出片」开关（豁免严苛闸门）</li>
                            </ul>
                        </div>
                    ),
                    okText: "定位到第一个缺图分镜",
                    cancelText: "取消",
                    onOk: () => {
                        // 滚动到第一个未匹配的分镜，让用户能立刻看到
                        requestAnimationFrame(() => {
                            const el = document.querySelector(`[data-storyboard-id="${firstMissingId}"]`);
                            el?.scrollIntoView({ behavior: "smooth", block: "center" });
                            el?.classList.add("ring-2", "ring-red-400");
                            setTimeout(() => el?.classList.remove("ring-2", "ring-red-400"), 2000);
                        });
                    },
                });
                return;
            }
        }
        // 跳过已有成功视频的分镜，其余的按正常流程生成
        const skipBoards = selectedOrAll.filter((s) => s.videoStatus === "success" && s.videoUrl);
        const boards = selectedOrAll.filter((s) => !(s.videoStatus === "success" && s.videoUrl));
        // 创建 abort controller（支持外部传入以停止）
        const ctrl = abortCtrlOverride || new AbortController();
        if (!abortCtrlOverride) setStoryboardVideoAbortController(ctrl);
        // 4 段 stepper：用户触发批量生视频时直接标记为「视频生成」阶段
        setPhase("video");
        setPipelineProgress({ current: 0, total: boards.length });
        // 应生成 / 成功 / 失败统计：每个视频到终态时回调，全部结束后汇总
        const total = boards.length;
        if (total === 0 && skipBoards.length > 0) {
            setStoryboardVideoAbortController(null);
            antMessage.info(`已全部跳过（${skipBoards.length} 个分镜已有视频，如需重新生成请单独点「重新生成」按钮）`);
            return;
        }
        let succeeded = 0;
        let failed = 0;
        let settledCount = 0;
        const onSettled = (ok: boolean) => {
            if (ctrl.signal.aborted) return; // 已停止，忽略回调
            if (ok) succeeded += 1; else failed += 1;
            settledCount += 1;
            // 4 段 stepper：实时刷新当前阶段进度
            setPipelineProgress({ current: settledCount, total });
            if (settledCount === total) {
                setStoryboardVideoAbortController(null);
                // 4 段 stepper：全部结束后标记为完成态
                setPhase(ctrl.signal.aborted ? "idle" : "done");
                // autoPilot 模式下设置最终完成状态
                if (autoPilot !== "off" && !ctrl.signal.aborted) {
                    setPipelineStatus(`${autoPilot === "full" ? "一键出片" : "半自动"}流水线完成！`);
                }
                const skipMsg = skipBoards.length > 0 ? `，跳过 ${skipBoards.length} 个已有视频` : "";
                if (failed === 0) antMessage.success(`视频生成完成：应生成 ${total} 个，成功 ${succeeded}，失败 0${skipMsg}`);
                else if (succeeded > 0) antMessage.warning(`视频生成完成：应生成 ${total} 个，成功 ${succeeded}，失败 ${failed}${skipMsg}；失败分镜可单独点"生成视频"重试`);
                else antMessage.error(`视频生成失败：应生成 ${total} 个，成功 0，失败 ${failed}${skipMsg}；请检查视频模型/额度是否可用`);
            }
        };
        boards.forEach((b) => generateVideoFromStoryboard(b.id, undefined, onSettled));
        antMessage.info(`已加入 ${total} 个视频任务（并发上限 ${videoConcurrency}${skipBoards.length > 0 ? `，跳过 ${skipBoards.length} 个已有视频` : ""}），完成后将汇总成功/失败数`);
    };

    /** 停止正在进行的分镜视频批量生成 */
    const stopStoryboardVideoGeneration = () => {
        storyboardVideoAbortController?.abort();
        pollingTimersRef.current.forEach((t) => clearInterval(t));
        pollingTimersRef.current.clear();
        setStoryboardVideoAbortController(null);
        setPipelineStatus("已停止分镜视频生成");
        antMessage.info("已停止分镜视频生成");
    };
    // 当 pipelineRunning 被外部取消时，也清除 storyboard 视频生成状态
    useEffect(() => {
        if (!pipelineRunning && !storyboardVideoAbortController) {
            // pipeline 停止且 storyboard 视频生成也停止时，清理状态
        }
    }, [pipelineRunning, storyboardVideoAbortController]);

    const toggleChapterExpand = (id: string) => {
        setExpandedChapterIds((prev) => { const next = new Set(prev); next.has(id) ? next.delete(id) : next.add(id); return next; });
    };

    /** 勾选/取消勾选某一章（每章一条）用于"只分部分章节" */
    const toggleGroupSelect = (gi: number) => {
        setSelectedGroups((prev) => { const next = new Set(prev); next.has(gi) ? next.delete(gi) : next.add(gi); return next; });
    };

    const handleImportFileClick = () => {
        const input = document.createElement("input");
        input.type = "file"; input.accept = ".txt,.text";
        input.onchange = (e) => {
            const file = (e.target as HTMLInputElement).files?.[0];
            if (!file) return;
            const reader = new FileReader();
            reader.onload = () => {
                if (!activeProject) return;
                const text = reader.result as string;
                const lines = text.split("\n");
                // 导入即按章节解析目录（原始小说式，第一章展开其余折叠）
                const parsed = parseChapters(text);
                updateProject((p) => ({ ...p, script: text, chapters: parsed, updatedAt: Date.now() }));
                setExpandedChapterIds(new Set(parsed[0] ? [parsed[0].id] : []));
                if (lines.length > scriptVisibleLines) {
                    setScriptVisibleLines(Math.min(lines.length, scriptVisibleLines * 2));
                }
                antMessage.success(`已导入并解析为 ${parsed.length} 章`);
            };
            reader.readAsText(file, "UTF-8");
        };
        input.click();
    };

    // 从剪贴板粘贴剧本文本：与 .txt 导入共用同一条解析管线（parseChapters → updateProject）
    // 注意：navigator.clipboard 需要 HTTPS / localhost，且要用户授权；拿不到权限时回退到 textarea 手贴
    const handleImportClipboard = async () => {
        if (!activeProject) { antMessage.warning("请先选择或新建项目"); return; }
        if (typeof navigator === "undefined" || !navigator.clipboard?.readText) {
            antMessage.warning("当前浏览器不支持剪贴板读取，请改用「导入 .txt」或在文本框手动粘贴");
            return;
        }
        setClipboardBusy(true);
        try {
            const text = await navigator.clipboard.readText();
            if (!text.trim()) { antMessage.warning("剪贴板为空"); return; }
            const parsed = parseChapters(text);
            updateProject((p) => ({ ...p, script: text, chapters: parsed, updatedAt: Date.now() }));
            setExpandedChapterIds(new Set(parsed[0] ? [parsed[0].id] : []));
            const lines = text.split("\n");
            if (lines.length > scriptVisibleLines) {
                setScriptVisibleLines(Math.min(lines.length, scriptVisibleLines * 2));
            }
            // 无章节标题时（如复制 PDF 散文）额外提示，避免用户误以为"解析成功"但实际只存了单章正文
            const extra = parsed.length < 2 ? "（未检测到「第 N 章」类标题，已按单章原文保存；如需多章结构请手动在文本框补 `第1章 ...\\n第2章 ...` 样式）" : "";
            antMessage.success(`已从剪贴板导入并解析为 ${parsed.length} 章${extra}`);
        } catch (e) {
            // 权限拒绝/剪贴板无权限等情况：让用户改用 textarea 兜底
            antMessage.error(`剪贴板读取失败：${(e as Error).message || "请检查浏览器权限或改用「导入 .txt」"}`);
        } finally {
            setClipboardBusy(false);
        }
    };

    /**
     * 剧本转资产（一站式）：把「提取资产描述」+「生图」两个手动步骤合并成一个动作。
     * 复用 extractAssetsForAutoChain + generateImagesFromExtracted 内部逻辑（不复制实现），
     * 用单独的 busy state 避免与单步按钮的 loading 冲突，不污染 autoPilot 的进度状态。
     */
    const scriptToAssets = async () => {
        if (!activeProject) { antMessage.warning("请先选择或新建项目"); return; }
        const boards = (activeProject.storyboards || []).filter((s) => storyboardIsReady(s));
        if (boards.length === 0) { antMessage.warning("暂无分镜剧本可生成资产，请先生成分镜剧本"); return; }
        const textModel = effectiveConfig.textModel || defaultConfig.textModel;
        if (!isAiConfigReady(effectiveConfig, textModel)) {
            antMessage.warning("请先完成文本模型配置"); openConfigDialog(true); return;
        }
        setScriptToAssetsBusy(true);
        setAssetGenerationRunning(true);
        // 6 段 stepper：手动触发"剧本转资产"走 assets_prompt/image/reconcile 三段
        setPhase("assets_prompt");
        setAssetStep("prompt");
        setPipelineStatus("剧本转资产：为资产生成生图提示词…");
        const ctrl = new AbortController();
        setAbortController(ctrl);
        try {
            const { count: extractedCount, items: extractedItems } = await extractAssetsForAutoChain(boards, ctrl);
            if (ctrl.signal.aborted) return;
            if (extractedCount === 0) {
                antMessage.info("未抽出新资产（同 alias 已全部存在），无需生图");
                setPhase("idle");
                return;
            }
            // ② 生图
            setPhase("assets_image");
            setAssetStep("image");
            setPipelineStatus(`剧本转资产：正在为 ${extractedCount} 个资产生成图片…`);
            await generateImagesFromExtracted(ctrl, extractedItems);
            if (ctrl.signal.aborted) return;
            // ③ 对应分镜：把生好的图按 alias 与分镜 @名字 做匹配，刷新覆盖报告
            setPhase("assets_reconcile");
            setAssetStep("reconcile");
            setPipelineStatus("剧本转资产：正在把图片对应到分镜剧本…");
            const report = computeCoverage(boards);
            if (report.totalMissing > 0) {
                antMessage.success(`剧本转资产完成：抽出 ${extractedCount} 个并已生成图片；仍缺 ${report.totalMissing} 个角色图`);
            } else {
                antMessage.success(`剧本转资产完成：抽出 ${extractedCount} 个并已生成图片，分镜角色已全部对应`);
            }
            setPipelineStatus("剧本转资产完成");
        } catch (error) {
            const parsed = parseVendorError((error as Error).message, "image");
            antMessage.error(`剧本转资产失败${parsed.action ? `（${parsed.title}）` : ""}：${(error as Error).message}`);
            setPipelineStatus(`剧本转资产失败：${(error as Error).message}`);
        } finally {
            setScriptToAssetsBusy(false);
            setAssetGenerationRunning(false);
            setAssetStep("");
            if (!ctrl.signal.aborted) setAbortController(null);
            // 状态条清理：成功完成后清掉残留文案，失败保留错误信息便于用户看
            // （其他长跑任务的 finally 里都按"非 autoPilot 就清"处理，我们这里无条件清就好——toast 已经有 success/error）
            if (!ctrl.signal.aborted) setPipelineStatus("");
        }
    };

    /**
     * 匹配校验：统计所有就绪分镜里有 @角色 没绑资产的，生成报告用于闸门/高亮/跳转。
     * 「场景」「道具」不参与严苛闸门——只有 @角色 是出片必须的；未绑场景/道具 走无参考图也无大碍。
     */
    const computeCoverage = (boards: Storyboard[]): StoryboardCoverageReport => {
        const missingByBoard = new Map<string, string[]>();
        for (const b of boards.filter((x) => storyboardIsReady(x))) {
            const chars = extractStoryboardHeader(b.content).characters;
            const missing = chars.filter((m) => !assets.find((a) => a.alias === m));
            if (missing.length > 0) missingByBoard.set(b.id, missing);
        }
        let totalMissing = 0;
        for (const arr of missingByBoard.values()) totalMissing += arr.length;
        return { boardsWithMissing: missingByBoard.size, totalMissing, missingByBoard };
    };

    /**
     * 一键出片三态循环：关 → 半自动 → 全自动 → 关。
     * 三态的语义由 tooltip + 颜色双重传达，避免又回到"开/关"语义模糊的老问题。
     */
    const cycleAutoPilot = () => {
        setAutoPilot((p) => (p === "off" ? "half" : p === "half" ? "full" : "off"));
    };
    /**
     * 跳到资产区去补图：点分镜卡的"缺 N 图"chip 时调用，自动把资产区搜索设为该角色名 + 滚到资产网格（id="novel-asset-grid"）。
     * 桌面宽屏上资产面板肉眼可见；但小窗口/折叠侧栏时资产可能不在视口里，主动滚动更稳。
     */
    const jumpToMissingAssets = (sb: Storyboard) => {
        const links = getStoryboardAssetLinks(sb.content);
        const firstMissing = links.find((l) => !l.linked);
        if (!firstMissing) return;
        setAssetSearch(firstMissing.name);
        antMessage.info(`已聚焦资产库的「${firstMissing.name}」搜索，可上传或绑定同名图片`);
        // 双 rAF：等 setAssetSearch 触发的 re-render 完成后再找元素，避免首次渲染前 dom 还没出来
        requestAnimationFrame(() => requestAnimationFrame(() => {
            const el = document.getElementById("novel-asset-grid");
            el?.scrollIntoView({ behavior: "smooth", block: "start" });
        }));
    };
    const autoPilotLabel = autoPilot === "off" ? "关" : autoPilot === "half" ? "半自动" : "全自动";
    const autoPilotChipClass = autoPilot === "off"
        ? "bg-stone-100 text-stone-500 hover:bg-stone-200 dark:bg-stone-800 dark:text-stone-400 dark:hover:bg-stone-700"
        : autoPilot === "half"
            ? "bg-blue-100 text-blue-700 ring-1 ring-blue-300 hover:bg-blue-200 dark:bg-blue-900/40 dark:text-blue-300 dark:ring-blue-700 dark:hover:bg-blue-900/60"
            : "bg-amber-100 text-amber-700 ring-1 ring-amber-300 hover:bg-amber-200 dark:bg-amber-900/40 dark:text-amber-300 dark:ring-amber-700 dark:hover:bg-amber-900/60";
    const autoPilotTooltip = autoPilot === "off"
        ? "一键出片·关：手动模式，每步独立按钮。点击切换到「半自动」。"
        : autoPilot === "half"
            ? "一键出片·半自动：分镜完成后弹窗确认「是否继续生图并出片」。点击切换到「全自动」。"
            : "一键出片·全自动：分镜→抽资产→生图→校验所有 @角色 已绑定→出片，全程无需点击。再点一下回到「关」。";

    // 懒加载更多行
    const handleScriptScroll = (e: React.UIEvent<HTMLTextAreaElement>) => {
        const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
        if (scrollHeight - scrollTop - clientHeight < 500) {
            setScriptVisibleLines((prev) => Math.min(prev + 200, activeProject?.script.split("\n").length || 0));
        }
    };


    // ─── Auto-match & Scene Check ───

    const handleAutoMatch = () => {
        if (!activeProject) return;
        let matched = 0; const unlinked: string[] = [];
        const updatedShots = activeProject.shots.map((shot) => {
            const shotMentions = extractMentions(shot.content);
            const linkedIds: string[] = [];
            for (const mention of shotMentions) {
                const asset = assets.find((a) => a.alias === mention);
                if (asset) {
                    linkedIds.push(asset.id); matched++;
                }
                else unlinked.push(mention);
            }
            return { ...shot, referencedAssetIds: linkedIds };
        });
        updateProject((p) => ({ ...p, shots: updatedShots, updatedAt: Date.now() }));
        if (unlinked.length > 0) antMessage.warning(`已匹配 ${matched} 个引用，未匹配: ${unlinked.join(", ")}`);
        else antMessage.success(`自动匹配完成，共关联 ${matched} 个资产`);
    };

    const handleCheckScenes = () => {
        if (!activeProject || assets.length === 0) { antMessage.warning("请先上传素材库资产后再检查"); return; }
        setCheckingScenes(true);
        setTimeout(() => {
            const missing: string[] = [];
            const matched: string[] = [];
            for (const shot of activeProject.shots) {
                for (const m of extractMentions(shot.content)) {
                    (assets.find((a) => a.alias === m) ? matched : missing).push(`场景${shot.title}: @${m}`);
                }
            }
            setSceneCheckResult({ missing, matched });
            setCheckingScenes(false);
            if (missing.length === 0) antMessage.success("所有分镜资产引用均已匹配");
            else antMessage.warning(`发现 ${missing.length} 个未匹配的资产引用`);
        }, 400);
    };

    const moveShot = (from: number, to: number) => {
        if (!activeProject) return;
        const s = [...activeProject.shots];
        const [m] = s.splice(from, 1);
        s.splice(to, 0, m);
        updateProject((p) => ({ ...p, shots: s, updatedAt: Date.now() }));
    };

    // ─── Video Generation ───

    function buildVideoInput(shot: Shot, allShots: Shot[], assetList: Asset[]): { references: ReferenceImage[]; firstFrame: ReferenceImage | null; lastFrame: ReferenceImage | null } {
        let firstFrame: ReferenceImage | null = null;
        let lastFrame: ReferenceImage | null = null;
        const references: ReferenceImage[] = [];
        const seenIds = new Set<string>();

        for (const assetId of shot.referencedAssetIds) {
            const asset = assetList.find((a) => a.id === assetId);
            if (asset && !seenIds.has(asset.id)) {
                references.push({ id: asset.id, name: asset.name, type: "image/png", dataUrl: asset.dataUrl, url: asset.url, storageKey: asset.storageKey });
                seenIds.add(asset.id);
            }
        }
        if (shot.firstFrameAssetId) {
            const asset = assetList.find((a) => a.id === shot.firstFrameAssetId);
            if (asset && !seenIds.has(asset.id)) {
                if (supportsFrameRefs) firstFrame = { id: asset.id, name: asset.name, type: "image/png", dataUrl: asset.dataUrl, url: asset.url, storageKey: asset.storageKey };
                else references.push({ id: asset.id, name: asset.name, type: "image/png", dataUrl: asset.dataUrl, url: asset.url, storageKey: asset.storageKey });
                seenIds.add(asset.id);
            }
        }
        if (shot.lastFrameAssetId) {
            const asset = assetList.find((a) => a.id === shot.lastFrameAssetId);
            if (asset && !seenIds.has(asset.id)) {
                if (supportsFrameRefs) lastFrame = { id: asset.id, name: asset.name, type: "image/png", dataUrl: asset.dataUrl, url: asset.url, storageKey: asset.storageKey };
                else references.push({ id: asset.id, name: asset.name, type: "image/png", dataUrl: asset.dataUrl, url: asset.url, storageKey: asset.storageKey });
            }
        }
        // 顺序生成强制衔接：上一条视频的尾帧作为下一条参考图（上一条失败则跳过，不再回退找更早成功的）。
        // 并行生成走独立路径，不强制衔接。
        const useConsecutive = !generateInParallel;
        if (useConsecutive && allShots.length > 0) {
            const idx = allShots.findIndex((s) => s.id === shot.id);
            const prev = idx > 0 ? allShots[idx - 1] : undefined;
            if (prev?.status === "success" && prev.videoUrl) {
                if (supportsFrameRefs) lastFrame = { id: `lastframe-${prev.id}`, name: `${prev.title}-尾帧`, type: "video/mp4", dataUrl: prev.videoUrl };
                if (!seenIds.has(`lastframe-${prev.id}`)) {
                    references.push({ id: `lastframe-${prev.id}`, name: `${prev.title}-尾帧`, type: "video/mp4", dataUrl: prev.videoUrl });
                    seenIds.add(`lastframe-${prev.id}`);
                }
            }
        }
        return { references, firstFrame, lastFrame };
    }

    const generateShot = async (shot: Shot, allShots: Shot[], shotIndex: number, controller: AbortController, onSettled?: (ok?: boolean) => void): Promise<void> => {
        // 保证并发名额只释放一次（无论从哪个终态路径退出）；ok 透传成败给批量统计（未知时不传）
        let settled = false;
        const settle = (ok?: boolean) => { if (!settled) { settled = true; onSettled?.(ok); } };
        // 关键：createVideoGenerationTask 内部按 config.model 判定视频模型分支（JSON/FormData）。
        // 必须把 model 指向"视频模型"，否则会被当成非视频模型走 FormData，触发
        // "This endpoint only supports Content-Type: application/json"。
        const resolvedVideoModel = effectiveConfig.videoModel || defaultConfig.videoModel;
        const resolvedVideoChannel = effectiveConfig.videoChannelId || effectiveConfig.activeChannelId;
        // 根据视频模型动态调整视频提示词中的时长引用（15秒 → 模型支持的最大时长）
        const dynamicVideoPrompt = makePromptDynamic(videoPrompt, resolvedVideoModel);
        // 像素尺寸由画面比例推导（详情展示 & 请求参数共用同一份，避免不一致）
        const resolvedSize = aspectRatio === "16:9" ? "1280x720" : aspectRatio === "9:16" ? "720x1280" : aspectRatio === "21:9" ? "1920x720" : "1024x1024";
        const requestConfig: AiConfig = {
            ...effectiveConfig,
            model: resolvedVideoModel,
            videoModel: resolvedVideoModel,
            activeChannelId: resolvedVideoChannel,
            videoChannelId: resolvedVideoChannel,
            videoSeconds: String(shot.duration),
            size: resolvedSize,
            vquality: resolution,
            systemPrompts: {
                ...(effectiveConfig.systemPrompts || {}),
                video: dynamicVideoPrompt,
            },
        };

        if (!isAiConfigReady(requestConfig, requestConfig.videoModel || requestConfig.model)) {
            antMessage.warning("请先完成视频模型配置");
            openConfigDialog(true);
            settle();
            return;
        }

        const { references, firstFrame, lastFrame } = buildVideoInput(shot, allShots, assets);
        const resolvedScript = resolveMentions(shot.content, assets);
        // 分镜正文本身就是可直接生成视频的镜头描述；视频系统提示词会在 createVideoGenerationTask 内部自动拼接，
        // 这里不要再拼"分镜系统提示词"（那是文本改写用的，拼进来会污染视频提示词）。
        const finalVideoPrompt = shot.customPrompt ? shot.customPrompt : resolvedScript;
        const frameSupported = supportsVideoFrameReferences(requestConfig.videoModel || requestConfig.model);

        try {
            const created = await createVideoGenerationTask(requestConfig, finalVideoPrompt,
                frameSupported ? { references, videoReferences: [], audioReferences: [], firstFrame, lastFrame } : references,
                (progress) => updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, progress } : s) })),
                { source: "video-workbench" },
            );
            if (controller.signal.aborted) { settle(); return; }

            const pollInterval = setInterval(async () => {
                if (controller.signal.aborted) { clearInterval(pollInterval); pollingTimersRef.current.delete(shot.id); settle(); return; }
                try {
                    const task = await pollVideoGenerationTaskStatus(effectiveConfig, created.task);
                    if (task.status === "completed" || task.status === "failed") {
                        clearInterval(pollInterval);
                        pollingTimersRef.current.delete(shot.id);
                        const ok = task.status === "completed";
                        const finalUrl = ok ? (task.video_url || task.url) : undefined;
                        const finalStorageKey = ok ? task.storageKey : undefined;
                        updateProject((p) => ({
                            ...p,
                            shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: ok ? "success" : "failed", videoUrl: finalUrl, videoStorageKey: finalStorageKey, error: ok ? undefined : task.error?.message } : s),
                            // 同步关联分镜剧本的视频状态
                            storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: ok ? "success" : "failed", videoUrl: finalUrl, videoError: ok ? undefined : task.error?.message } : b),
                        }));
                        if (ok) antMessage.success(`${shot.title || "分镜"} 生成完成`);
                        else {
                            // P1 b8：vendor 错误分级 —— 取代原 raw JSON 提示
                            const parsed = parseVendorError(task.error?.message ?? "", "video");
                            const content = (
                                <div className="space-y-1">
                                    <div className="flex items-center gap-1.5 font-medium">{parsed.icon} {parsed.title}</div>
                                    {parsed.action && <div className="text-xs opacity-90">💡 {parsed.action}</div>}
                                    <details className="text-[10px] opacity-60"><summary>原始错误</summary><code className="break-all">{parsed.detail}</code></details>
                                </div>
                            );
                            if (parsed.level === "warn") antMessage.warning({ content, key: `video-fail-${shot.id}`, duration: 6 });
                            else if (parsed.level === "info") antMessage.info({ content, key: `video-fail-${shot.id}`, duration: 6 });
                            else antMessage.error({ content, key: `video-fail-${shot.id}`, duration: 6 });
                        }
                        settle(ok); // 终态：释放并发名额并补位下一个，并回传成败
                    } else {
                        updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, progress: task.progress } : s) }));
                    }
                } catch (err) {
                    if (controller.signal.aborted) { clearInterval(pollInterval); pollingTimersRef.current.delete(shot.id); settle(); return; }
                    clearInterval(pollInterval);
                    pollingTimersRef.current.delete(shot.id);
                    updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: "failed", error: (err as Error).message } : s), storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: "failed", videoError: (err as Error).message } : b) }));
                    settle(false); // 终态：释放并发名额（失败）
                }
            }, VIDEO_POLL_INTERVAL_MS);

            pollingTimersRef.current.set(shot.id, pollInterval);
            updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: "generating", videoTaskId: created.pollId, videoModel: resolvedVideoModel, aspectRatio, resolution, size: resolvedSize } : s) }));
        } catch (error) {
            if (controller.signal.aborted) { settle(); return; }
            updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: "failed", error: (error as Error).message } : s), storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: "failed", videoError: (error as Error).message } : b) }));
            antMessage.error(`生成失败: ${(error as Error).message}`);
            settle(false); // 终态：释放并发名额并补位下一个（失败）
        }
    };

    async function runConcurrent<T>(items: T[], work: (item: T, index: number) => Promise<void>, controller: AbortController, onProgress: (done: number, total: number) => void) {
        let done = 0;
        const total = items.length;
        let active: Array<{ promise: Promise<void>; done: boolean }> = [];
        const all: Promise<void>[] = [];
        async function worker(item: T, index: number) { await work(item, index); done++; onProgress(done, total); }
        for (let i = 0; i < items.length; i++) {
            if (controller.signal.aborted) return;
            const slot = { promise: worker(items[i], i).then(() => { slot.done = true; }).catch(() => { slot.done = true; }), done: false };
            active.push(slot);
            all.push(slot.promise);
            if (active.length >= videoConcurrency) {
                await Promise.race(active.map((s) => s.promise));
                // 移除已完成的 worker，避免 active 数组无限增长
                active = active.filter((s) => !s.done);
            }
        }
        await Promise.allSettled(all);
    }


    const stopPipeline = () => {
        abortController?.abort();
        // 停止分镜任务轮询
        if (storyboardPollRef.current) { clearInterval(storyboardPollRef.current); storyboardPollRef.current = null; }
        pollingTimersRef.current.forEach((t) => clearInterval(t));
        pollingTimersRef.current.clear();
        setParsingStoryboard(false);
        setPipelineRunning(false); setPhase("idle"); setPipelineStatus("已停止");

        // 真取消上游分镜任务：如果当前项目有后端分镜任务在跑，调 cancel 接口让 worker
        // 在当前章节完成后立即停手（避免浪费已扣费额度，也避免刷新后又被恢复轮询）。
        const project = projectsRef.current.find((p) => p.id === activeProjectId);
        const taskId = project?.storyboardTaskId;
        if (taskId && token) {
            cancelStoryboardTask(taskId, token).catch(() => { /* 幂等：失败不阻塞 UI */ });
        }
    };
    // 当 pipelineRunning 被外部取消时，也清除 storyboard 视频生成状态
    useEffect(() => {
        if (!pipelineRunning) setStoryboardVideoAbortController(null);
    }, [pipelineRunning]);

    // 刷新恢复：项目切换/加载时，如果该项目有正在跑的分镜任务（storyboardTaskId），自动恢复轮询
    useEffect(() => {
        if (!activeProject?.storyboardTaskId || !token) return;
        const groupMap = activeProject.storyboardGroupMap ?? [];
        const controller = new AbortController();
        setAbortController(controller);
        // 仅在未处于分镜状态时设置（handleParseStoryboards 已设置则不覆盖）
        if (!parsingStoryboard) {
            setParsingStoryboard(true);
            // 4 段 stepper：恢复阶段也归到「分镜剧本」步骤，否则首屏会停在「章节解析」，
            // 分镜步骤永远显示"待启动"，跟底部"正在恢复…"也不对应。
            setPhase("storyboard");
            setPipelineProgress({ current: 0, total: groupMap.length });
            setPipelineStatus("正在恢复分镜任务进度…");
        }
        pollStoryboardTask(activeProject.storyboardTaskId, groupMap, controller);
        // 清理：依赖变化（切换项目/token 变化）或卸载时停止轮询，避免泄漏
        return () => { controller.abort(); };
        // 仅在 activeProject.storyboardTaskId 变化时恢复，不依赖 groupMap（避免每次渲染都重启轮询）
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [activeProject?.storyboardTaskId, token]);

    // 恢复视频任务轮询：页面加载/项目切换时，同步后端视频任务状态
    // 对 status=generating 且有 videoTaskId 的 shot，拉取后端任务列表判断状态
    const resumeVideoPolling = useCallback((shot: Shot) => {
        if (!shot.videoTaskId || pollingTimersRef.current.has(shot.id)) return;
        // 使用视频模型配置（与 generateShot 创建任务时一致）
        const resolvedVideoModel = effectiveConfig.videoModel || defaultConfig.videoModel;
        const resolvedVideoChannel = effectiveConfig.videoChannelId || effectiveConfig.activeChannelId;
        const requestConfig: AiConfig = {
            ...effectiveConfig,
            model: shot.videoModel || resolvedVideoModel,
            videoModel: resolvedVideoModel,
            activeChannelId: resolvedVideoChannel,
            videoChannelId: resolvedVideoChannel,
        };
        const pollInterval = setInterval(async () => {
            try {
                const task = await pollVideoGenerationTaskStatus(requestConfig, { id: shot.videoTaskId } as any);
                if (isCompletedVideoStatus(task.status) || isFailedVideoStatus(task.status)) {
                    clearInterval(pollInterval);
                    pollingTimersRef.current.delete(shot.id);
                    const ok = isCompletedVideoStatus(task.status);
                    const finalUrl = ok ? (task.video_url || task.url) : undefined;
                    const finalStorageKey = ok ? (task as any).storageKey : undefined;
                    updateProject((p) => ({
                        ...p,
                        shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: ok ? "success" as const : "failed" as const, videoUrl: finalUrl, videoStorageKey: finalStorageKey, error: ok ? undefined : task.error?.message } : s),
                        storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: ok ? "success" as const : "failed" as const, videoUrl: finalUrl, videoError: ok ? undefined : task.error?.message } : b),
                    }));
                    if (ok) antMessage.success(`${shot.title || "分镜"} 生成完成`);
                } else {
                    updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, progress: task.progress } : s) }));
                }
            } catch (err) {
                clearInterval(pollInterval);
                pollingTimersRef.current.delete(shot.id);
                updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: "failed" as const, error: (err as Error).message } : s), storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: "failed" as const, videoError: (err as Error).message } : b) }));
            }
        }, VIDEO_POLL_INTERVAL_MS);
        pollingTimersRef.current.set(shot.id, pollInterval);
    }, [effectiveConfig]);

    useEffect(() => {
        if (!activeProject || !token) return;
        const generatingShots = activeProject.shots.filter((s) => s.status === "generating" && s.videoTaskId);
        if (generatingShots.length === 0) return;
        // 非账户代理模式没有后端任务列表，直接标记失败（同步请求随页面卸载已丢失）
        if (activeChannelKind(effectiveConfig) === "local" && !token) {
            updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.status === "generating" && s.videoTaskId ? { ...s, status: "failed" as const, error: "页面切换导致任务中断（本地渠道模式不支持后台任务）" } : s) }));
            return;
        }
        let cancelled = false;
        listVideoGenerationTasks(effectiveConfig).then((backendTasks) => {
            if (cancelled) return;
            const taskMap = new Map(backendTasks.map((t) => [t.id || t.task_id || t.video_id, t]));
            for (const shot of generatingShots) {
                const backendTask = taskMap.get(shot.videoTaskId);
                if (!backendTask) {
                    // 后端列表里没有 → 可能已完成被清理，尝试单次轮询确认
                    resumeVideoPolling(shot);
                } else if (isCompletedVideoStatus(backendTask.status)) {
                    const finalUrl = backendTask.video_url || backendTask.url;
                    const finalStorageKey = (backendTask as any).storageKey;
                    updateProject((p) => ({
                        ...p,
                        shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: "success" as const, videoUrl: finalUrl, videoStorageKey: finalStorageKey } : s),
                        storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: "success" as const, videoUrl: finalUrl } : b),
                    }));
                } else if (isFailedVideoStatus(backendTask.status)) {
                    updateProject((p) => ({
                        ...p,
                        shots: p.shots.map((s) => s.id === shot.id ? { ...s, status: "failed" as const, error: backendTask.error?.message || "视频生成失败" } : s),
                        storyboards: (p.storyboards || []).map((b) => b.id === shot.storyboardId ? { ...b, videoStatus: "failed" as const, videoError: backendTask.error?.message } : b),
                    }));
                } else {
                    // 仍在跑 → 恢复轮询
                    resumeVideoPolling(shot);
                }
            }
        }).catch(() => {
            if (cancelled) return;
            // 拉取失败 → 逐个恢复轮询
            generatingShots.forEach((shot) => resumeVideoPolling(shot));
        });
        return () => { cancelled = true; };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [activeProject?.id, token]);

    // 恢复资产生图任务：页面加载/项目切换时，同步后端资产生图任务状态
    useEffect(() => {
        if (!activeProject || !token) return;
        const taskIds = activeProject.assetImageTaskIds;
        if (!taskIds || taskIds.length === 0) return;
        // 非账户代理模式没有后端任务列表，直接清理 taskIds（同步请求随页面卸载已丢失）
        if (activeChannelKind(effectiveConfig) === "local" && !token) {
            updateProject((p) => ({ ...p, assetImageTaskIds: [] }));
            return;
        }
        let cancelled = false;
        listCanvasImageTasks(effectiveConfig, ["novel"]).then(async (backendTasks) => {
            if (cancelled) return;
            const taskMap = new Map(backendTasks.map((t) => [t.id, t]));
            const stillRunning: string[] = [];
            const completedAssets: Asset[] = [];
            for (const taskId of taskIds) {
                const backendTask = taskMap.get(taskId);
                if (!backendTask) {
                    // 后端列表没有 → 可能已完成被清理，尝试单次轮询确认
                    try {
                        const task = await pollCanvasImageTaskStatus(taskId);
                        if (task.status === "completed") {
                            const dataUrl = task.image_url || task.url || "";
                            if (dataUrl) {
                                completedAssets.push({
                                    id: nanoid(),
                                    name: task.prompt || "",
                                    alias: task.prompt || "",
                                    type: "reference",
                                    dataUrl,
                                    description: "",
                                });
                            }
                        }
                    } catch { /* 任务已不存在 */ }
                } else if (backendTask.status === "completed") {
                    const dataUrl = backendTask.image_url || backendTask.url || "";
                    if (dataUrl) {
                        completedAssets.push({
                            id: nanoid(),
                            name: backendTask.prompt || "",
                            alias: backendTask.prompt || "",
                            type: "reference",
                            dataUrl,
                            description: "",
                        });
                    }
                } else if (backendTask.status === "failed") {
                    // 失败 → 不处理
                } else {
                    // 仍在跑 → 恢复轮询
                    stillRunning.push(taskId);
                    pollCanvasImageTaskUntilDone(taskId).then((completedTask) => {
                        const dataUrl = completedTask.image_url || completedTask.url || "";
                        if (!dataUrl) return;
                        const asset: Asset = {
                            id: nanoid(),
                            name: completedTask.prompt || "",
                            alias: completedTask.prompt || "",
                            type: "reference",
                            dataUrl,
                            description: "",
                        };
                        setAssets((prev) => {
                            const next = [...prev, asset];
                            void saveAssets(next);
                            return next;
                        });
                    }).catch(() => { /* 失败忽略 */ });
                }
            }
            if (cancelled) return;
            if (completedAssets.length > 0) {
                setAssets((prev) => {
                    const next = [...prev, ...completedAssets];
                    void saveAssets(next);
                    return next;
                });
            }
            // 更新项目：只保留仍在跑的任务 ID
            updateProject((p) => ({ ...p, assetImageTaskIds: stillRunning }));
        }).catch(() => {
            if (cancelled) return;
            // 拉取失败 → 清空 taskIds（无法恢复）
            updateProject((p) => ({ ...p, assetImageTaskIds: [] }));
        });
        return () => { cancelled = true; };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [activeProject?.id, token]);

    // 搜索定位后滚动到对应分镜卡片
    useEffect(() => {
        if (!matchedStoryboardId || !storyboardListRef.current) return;
        const el = storyboardListRef.current.querySelector(`[data-storyboard-id="${matchedStoryboardId}"]`) as HTMLElement | null;
        if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
    }, [matchedStoryboardId]);

    // ─── Shot ops ───

    const toggleShotSelect = (id: string) => {
        if (!activeProject) return;
        updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === id ? { ...s, selected: !s.selected } : s) }));
    };
    const toggleAllSelect = (checked: boolean) => {
        if (!activeProject) return;
        updateProject((p) => ({ ...p, shots: p.shots.map((s) => ({ ...s, selected: checked })) }));
    };
    /** 删除单个分镜视频 */
    const deleteShot = (id: string) => {
        updateProject((p) => ({ ...p, shots: p.shots.filter((s) => s.id !== id) }));
        if (detailShotId === id) setDetailShotId(null);
    };
    /** 批量删除勾选的分镜视频（带确认框） */
    const batchDeleteShots = () => {
        if (!activeProject) return;
        const selected = activeProject.shots.filter((s) => s.selected);
        if (selected.length === 0) { antMessage.warning("请先勾选要删除的分镜视频"); return; }
        antModal.confirm({
            title: "批量删除分镜视频",
            content: `确定删除选中的 ${selected.length} 个分镜视频吗？此操作不可恢复。`,
            okText: "删除", okButtonProps: { danger: true }, cancelText: "取消",
            onOk: () => {
                updateProject((p) => ({ ...p, shots: p.shots.filter((s) => !s.selected) }));
                // 如果当前详情弹窗的视频在删除集合中,关闭详情弹窗
                if (detailShotId && selected.some((s) => s.id === detailShotId)) setDetailShotId(null);
                antMessage.success(`已删除 ${selected.length} 个分镜视频`);
            },
        });
    };
    const updateShotDuration = (id: string, d: number) => { if (!activeProject) return; updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === id ? { ...s, duration: d } : s) })); };
    const updateShotPrompt = (id: string, prompt: string) => { if (!activeProject) return; updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === id ? { ...s, customPrompt: prompt } : s) })); };
    const updateShotFrame = (id: string, field: "firstFrameAssetId" | "lastFrameAssetId", assetId: string | undefined) => {
        if (!activeProject) return;
        updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === id ? { ...s, [field]: assetId } : s) }));
    };

    // ─── Video operations ───

    /** 统一获取视频 Blob：优先从本地 storageKey 读取，否则走 downloadRemoteMedia 代理（避免 CORS） */
    /** 从视频截取帧，上传到存储服务后保存为资产并自动设为当前分镜的首/尾帧参考 */
    const saveFrameAsAsset = async (videoUrl: string, shotId: string, shotTitle: string, type: "first" | "last", storageKey?: string) => {
        const dataUrl = await extractFrame(videoUrl, type, storageKey);
        if (!dataUrl) { antMessage.error("帧提取失败"); return; }
        const hideLoading = antMessage.loading(type === "first" ? "正在保存首帧..." : "正在保存尾帧...", 0);
        try {
            // 上传到存储服务拿到可公开访问 URL + storageKey，避免重新生成时 image URL 无法下载
            const uploaded = await uploadImage(dataUrl);
            const isServer = uploaded.storageKey.startsWith("server:");
            const asset: Asset = {
                id: nanoid(),
                name: `${shotTitle}-${type === "first" ? "首帧" : "尾帧"}`,
                alias: `${shotTitle}-${type === "first" ? "首帧" : "尾帧"}`,
                type: "reference",
                dataUrl: isServer ? uploaded.url : dataUrl,
                url: uploaded.url,
                storageKey: uploaded.storageKey,
                description: `${shotTitle} 的${type === "first" ? "首" : "尾"}帧`,
            };
            const next = [...assets, asset];
            setAssets(next);
            void saveAssets(next);
            // 自动关联到当前分镜的首/尾帧参考，保存的图直接出现在分镜图片里
            updateShotFrame(shotId, type === "first" ? "firstFrameAssetId" : "lastFrameAssetId", asset.id);
            antMessage.success(`已保存为分镜${type === "first" ? "首" : "尾"}帧: ${asset.alias}`);
        } catch (error) {
            // 上传失败（如服务端存储需登录）时仍保存 base64 到素材库，但因无公开 URL 不自动关联
            const asset: Asset = {
                id: nanoid(),
                name: `${shotTitle}-${type === "first" ? "首帧" : "尾帧"}`,
                alias: `${shotTitle}-${type === "first" ? "首帧" : "尾帧"}`,
                type: "reference",
                dataUrl,
                description: `${shotTitle} 的${type === "first" ? "首" : "尾"}帧`,
            };
            const next = [...assets, asset];
            setAssets(next);
            void saveAssets(next);
            antMessage.error(error instanceof Error ? error.message : "帧保存失败");
        } finally {
            hideLoading();
        }
    };

    /** 从视频截取当前播放帧（按播放器 currentTime），上传到存储服务后保存为资产并自动加入当前分镜的参考图列表 */
    const captureCurrentFrameAndSave = async (videoUrl: string, shotId: string, shotTitle: string, storageKey?: string, seekSeconds?: number) => {
        const dataUrl = await extractFrame(videoUrl, "first", storageKey, seekSeconds ?? 0.1);
        if (!dataUrl) { antMessage.error("帧提取失败"); return; }
        const hideLoading = antMessage.loading("正在保存当前帧...", 0);
        try {
            // 上传到存储服务拿到可公开访问 URL + storageKey，避免重新生成时 image URL 无法下载
            const uploaded = await uploadImage(dataUrl);
            const isServer = uploaded.storageKey.startsWith("server:");
            const asset: Asset = {
                id: nanoid(),
                name: `${shotTitle}-当前帧`,
                alias: `${shotTitle}-当前帧`,
                type: "reference",
                dataUrl: isServer ? uploaded.url : dataUrl,
                url: uploaded.url,
                storageKey: uploaded.storageKey,
                description: `${shotTitle} 的当前帧`,
            };
            const next = [...assets, asset];
            setAssets(next);
            void saveAssets(next);
            // 自动加入当前分镜的参考图列表，保存的图直接出现在分镜图片里
            updateProject((p) => ({ ...p, shots: p.shots.map((s) => s.id === shotId ? { ...s, referencedAssetIds: Array.from(new Set([...s.referencedAssetIds, asset.id])) } : s), updatedAt: Date.now() }));
            antMessage.success(`已保存为分镜参考图: ${asset.alias}`);
        } catch (error) {
            // 上传失败（如服务端存储需登录）时仍保存 base64 到素材库，但因无公开 URL 不自动关联
            const asset: Asset = {
                id: nanoid(),
                name: `${shotTitle}-当前帧`,
                alias: `${shotTitle}-当前帧`,
                type: "reference",
                dataUrl,
                description: `${shotTitle} 的当前帧`,
            };
            const next = [...assets, asset];
            setAssets(next);
            void saveAssets(next);
            antMessage.error(error instanceof Error ? error.message : "帧保存失败");
        } finally {
            hideLoading();
        }
    };

    /** 从视频 URL 提取音轨并保存为音频资产（WAV 格式） */
    const extractAudioAndSave = async (videoUrl: string, shotTitle: string, storageKey?: string) => {
        try {
            const blob = await getVideoBlob(videoUrl, storageKey);
            const arrayBuffer = await blob.arrayBuffer();
            const audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)();
            const audioBuffer = await audioCtx.decodeAudioData(arrayBuffer.slice(0));
            const wavBlob = audioBufferToWav(audioBuffer);
            const dataUrl = await new Promise<string>((resolve) => {
                const reader = new FileReader();
                reader.onloadend = () => resolve(reader.result as string);
                reader.readAsDataURL(wavBlob);
            });
            const asset: Asset = {
                id: nanoid(),
                name: `${shotTitle}-音频`,
                alias: `${shotTitle}-音频`,
                type: "reference",
                dataUrl,
                description: `${shotTitle} 提取的音频（${Math.round(audioBuffer.duration)}秒）`,
            };
            const next = [...assets, asset];
            setAssets(next);
            void saveAssets(next);
            antMessage.success(`已保存音频素材: ${asset.alias}（${Math.round(audioBuffer.duration)}s）`);
            audioCtx.close();
        } catch (e) {
            antMessage.error(`音频提取失败: ${e instanceof Error ? e.message : "未知错误"}`);
        }
    };

    /** 将 AudioBuffer 编码为 WAV Blob */
    /** 下载视频：优先从本地 Blob 保存，否则代理下载后用 file-saver 触发真正的浏览器下载 */
    const downloadVideo = async (videoUrl: string, name: string, storageKey?: string) => {
        const hideLoading = antMessage.loading("正在准备下载...", 0);
        try {
            const blob = await getVideoBlob(videoUrl, storageKey);
            saveAs(blob, `${name}.mp4`);
        } catch (error) {
            antMessage.error(error instanceof Error ? error.message : "视频下载失败");
        } finally {
            hideLoading();
        }
    };

    // 批量下载：优先下载勾选中已成功的视频；未勾选则下载全部已成功视频。逐个触发并间隔以避免浏览器拦截
    const batchDownloadVideos = async () => {
        if (!activeProject) return;
        const done = activeProject.shots.filter((s) => s.status === "success" && s.videoUrl);
        const selectedDone = done.filter((s) => s.selected);
        const targets = selectedDone.length > 0 ? selectedDone : done;
        if (targets.length === 0) { antMessage.warning("没有可下载的已生成视频"); return; }
        antMessage.info(`开始下载 ${targets.length} 个视频`);
        for (let i = 0; i < targets.length; i++) {
            const s = targets[i];
            // 用全局连续序号+标题命名，便于按顺序归档
            const seq = activeProject.shots.indexOf(s) + 1;
            await downloadVideo(s.videoUrl!, `${String(seq).padStart(2, "0")}_${s.title || "分镜"}`, s.videoStorageKey);
            // 间隔一小段时间，规避浏览器对连续下载的拦截
            await new Promise((r) => setTimeout(r, 350));
        }
    };

    // ─── UI ───

    return (
        <div className="flex h-full flex-col overflow-hidden bg-stone-50 text-stone-900 dark:bg-stone-950 dark:text-stone-100">
            {/* Main（无顶栏，标题/模型/配置/时长比例分辨率 均并入底部状态栏） */}
            <main className="flex min-h-0 flex-1 gap-6 overflow-hidden p-6">

                {/* ── LEFT: Project + Assets ── */}
                {leftCollapsed ? (
                    <div className="flex w-9 shrink-0 flex-col items-center gap-2 pt-1">
                        <Tooltip title="展开侧边栏" placement="right">
                            <Button size="small" type="text" icon={<ChevronsRight className="size-4" />} onClick={() => setLeftCollapsed(false)} className="h-8 w-8 p-0" />
                        </Tooltip>
                        <Tooltip title="新建剧本" placement="right">
                            <Button size="small" type="text" icon={<Plus className="size-4" />} onClick={createNewProject} className="h-8 w-8 p-0" />
                        </Tooltip>
                    </div>
                ) : (
                    <div className="flex w-[420px] shrink-0 flex-col gap-4 overflow-hidden">
                        {/* ── 剧本项目（上半 50%，与原始小说五五开） ── */}
                        <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
                            <div className="flex items-center justify-between shrink-0">
                                <div className="flex items-center gap-1">
                                    <Tooltip title="收起侧边栏">
                                        <Button size="small" type="text" icon={<ChevronsLeft className="size-4" />} onClick={() => setLeftCollapsed(true)} className="h-7 w-7 p-0" />
                                    </Tooltip>
                                    <span className="text-xs font-medium text-stone-500">剧本项目</span>
                                </div>
                                <Button size="small" type="primary" icon={<Plus className="size-3" />} onClick={createNewProject} className="h-7 rounded-lg text-xs bg-stone-800 hover:bg-stone-700 dark:bg-stone-200 dark:text-stone-900 dark:hover:bg-stone-300">新建</Button>
                            </div>
                            {/* 项目搜索框 */}
                            <Input
                                size="small" allowClear value={projectSearch}
                                onChange={(e) => setProjectSearch(e.target.value)}
                                prefix={<Search className="size-3.5 text-stone-400" />}
                                placeholder="搜索剧本项目"
                                className="!rounded-lg !text-xs shrink-0"
                            />
                            <div className="thin-scrollbar min-h-0 flex-1 overflow-y-auto space-y-0.5">
                                {filteredProjects.map((p) => (
                                    <div key={p.id}
                                        className={`group flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs transition ${
                                            p.id === activeProjectId ? "bg-stone-200 text-stone-900 dark:bg-stone-700 dark:text-stone-100" : "hover:bg-stone-100 dark:hover:bg-stone-800"
                                        }`}>
                                        <Video className="size-3.5 shrink-0 opacity-50" />
                                        {renamingId === p.id ? (
                                            <Input
                                                size="small" autoFocus value={renameValue}
                                                placeholder="输入剧本名称"
                                                onChange={(e) => setRenameValue(e.target.value)}
                                                onPressEnter={() => commitRename(p.id)}
                                                onBlur={() => commitRename(p.id)}
                                                className="min-w-0 flex-1 !text-xs"
                                            />
                                        ) : (
                                            <button onClick={() => setActiveProjectId(p.id)} className="min-w-0 flex-1 truncate text-left">{p.name}</button>
                                        )}
                                        {renamingId !== p.id && (
                                            <>
                                                <span className="opacity-40 group-hover:hidden">{p.shots.length}镜</span>
                                                <span className="hidden items-center gap-0.5 group-hover:flex">
                                                    <Tooltip title="重命名">
                                                        <button onClick={(e) => { e.stopPropagation(); setRenamingId(p.id); setRenameValue(p.name); }}
                                                            className="flex size-5 items-center justify-center rounded hover:bg-stone-300 dark:hover:bg-stone-600"><Pencil className="size-3" /></button>
                                                    </Tooltip>
                                                    <Tooltip title="删除">
                                                        <button onClick={(e) => { e.stopPropagation(); deleteProject(p.id); }}
                                                            className="flex size-5 items-center justify-center rounded text-stone-400 hover:bg-red-100 hover:text-red-500 dark:hover:bg-red-950/40"><Trash2 className="size-3" /></button>
                                                    </Tooltip>
                                                </span>
                                            </>
                                        )}
                                    </div>
                                ))}
                                {projects.length === 0 && (
                                    <div className="py-6 text-center text-xs text-stone-400">暂无项目，点击「新建」</div>
                                )}
                                {projects.length > 0 && filteredProjects.length === 0 && (
                                    <div className="py-6 text-center text-xs text-stone-400">没有匹配「{projectSearch}」的项目</div>
                                )}
                            </div>
                        </div>

                        {/* ── CENTER: Script ── */}
                        <div className="flex min-w-0 flex-1 flex-col gap-4 overflow-hidden">
                            <div className="flex flex-wrap items-center justify-between gap-2 shrink-0">
                                <h2 className="flex shrink-0 items-center gap-2 whitespace-nowrap text-sm font-medium">
                                    原始小说
                                    {chapters.length > 0 && <span className="rounded-full bg-stone-100 px-2 py-0.5 text-[11px] text-stone-500 dark:bg-stone-800 dark:text-stone-400">{chapters.length} 章</span>}
                                    {chapters.length > 0 && (
                                        <span className="flex items-center gap-1.5 text-[11px] text-stone-400">
                                            已选 {selectedGroups.size}/{chapters.length} 章
                                            <button onClick={() => seedAllGroups(chapters.length)} className="rounded px-1 text-stone-500 hover:bg-stone-100 dark:hover:bg-stone-800">全选</button>
                                            <button onClick={() => setSelectedGroups(new Set())} className="rounded px-1 text-stone-500 hover:bg-stone-100 dark:hover:bg-stone-800">清空</button>
                                        </span>
                                    )}
                                </h2>
                                {activeProject && (
                                    <div className="flex flex-wrap items-center justify-end gap-2">
                                        <Tooltip title="查看本界面使用说明与流程">
                                            <Button size="small" icon={<HelpCircle className="size-3" />} onClick={() => setShowGuideModal(true)}>使用说明</Button>
                                        </Tooltip>
                                        <Tooltip title="导入 .txt 小说/剧本文件">
                                            <Button size="small" icon={<FolderOpen className="size-3" />} onClick={handleImportFileClick}>导入</Button>
                                        </Tooltip>
                                        <Tooltip title={clipboardBusy ? "正在读取剪贴板…" : "从剪贴板读取剧本文本（需浏览器授权）"}>
                                            <Button
                                                size="small"
                                                icon={clipboardBusy ? <LoaderCircle className="size-3 animate-spin" /> : <ClipboardPaste className="size-3" />}
                                                onClick={handleImportClipboard}
                                                disabled={clipboardBusy}
                                            >
                                                {clipboardBusy ? "读取中" : "粘贴"}
                                            </Button>
                                        </Tooltip>
                                        {/* 一键出片三态 chip：单击循环 off→half→full→off，颜色与 tooltip 双重传达语义 */}
                                        <Tooltip title={autoPilotTooltip}>
                                            <button
                                                type="button"
                                                onClick={cycleAutoPilot}
                                                className={`flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-medium transition ${autoPilotChipClass}`}
                                            >
                                                <Coins className={`size-3 ${autoPilot === "off" ? "opacity-60" : ""}`} />
                                                一键出片 ·{autoPilotLabel}
                                            </button>
                                        </Tooltip>
                                        {/* 严苛闸门豁免 chip：仅一键出片开了时显示，避免手动模式误开 */}
                                        {autoPilot !== "off" && (
                                            <Tooltip title={allowMissingAssets
                                                ? "豁免已开启：未绑定的 @角色 将用「无参考图」模式出片（出片质量会下降）"
                                                : "默认严格：未绑定的 @角色 默认禁止出片。打开后可让一键出片在缺资产时用无参考图模式继续（出片质量可能下降）"}
                                            >
                                                <button
                                                    type="button"
                                                    onClick={() => setAllowMissingAssets(!allowMissingAssets)}
                                                    className={`flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-medium transition ${
                                                        allowMissingAssets
                                                            ? "bg-orange-100 text-orange-700 ring-1 ring-orange-300 hover:bg-orange-200 dark:bg-orange-900/40 dark:text-orange-300 dark:ring-orange-700 dark:hover:bg-orange-900/60"
                                                            : "bg-stone-100 text-stone-500 hover:bg-stone-200 dark:bg-stone-800 dark:text-stone-400 dark:hover:bg-stone-700"
                                                    }`}
                                                >
                                                    <AlertTriangle className={`size-3 ${allowMissingAssets ? "" : "opacity-60"}`} />
                                                    允许缺资产 ·{allowMissingAssets ? "开" : "关"}
                                                </button>
                                            </Tooltip>
                                        )}
                                        {/* 自动重试 chip：始终显示 */}
                                        <Tooltip title={autoRetryEnabled
                                            ? "已开启：分镜生成失败时自动重试（最多2次，指数退避：3s→6s）"
                                            : "已关闭：分镜生成失败时需要手动点击「重新生成」"}
                                        >
                                            <button
                                                type="button"
                                                onClick={() => setAutoRetryEnabled(!autoRetryEnabled)}
                                                className={`flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-medium transition ${
                                                    autoRetryEnabled
                                                        ? "bg-emerald-100 text-emerald-700 ring-1 ring-emerald-300 hover:bg-emerald-200 dark:bg-emerald-900/40 dark:text-emerald-300 dark:ring-emerald-700 dark:hover:bg-emerald-900/60"
                                                        : "bg-stone-100 text-stone-500 hover:bg-stone-200 dark:bg-stone-800 dark:text-stone-400 dark:hover:bg-stone-700"
                                                }`}
                                            >
                                                <RefreshCw className={`size-3 ${autoRetryEnabled ? "" : "opacity-60"}`} />
                                                自动重试 ·{autoRetryEnabled ? "开" : "关"}
                                            </button>
                                        </Tooltip>
                                        {/* 一站式"剧本转资产"按钮：合并"提取+生图"两步为一次动作（手动模式下也可点） */}
                                        <Tooltip title="从分镜剧本里抽角色/场景/道具并生图（合并两步为一次动作，可在手动模式下使用，与一键出片走的是同一套内部流程）">
                                            <Button
                                                size="small"
                                                icon={scriptToAssetsBusy ? <LoaderCircle className="size-3 animate-spin" /> : <Sparkles className="size-3" />}
                                                onClick={scriptToAssets}
                                                disabled={scriptToAssetsBusy || assetGenerationRunning || parsingStoryboard}
                                            >
                                                {scriptToAssetsBusy ? "生成中" : "剧本转资产"}
                                            </Button>
                                        </Tooltip>
                                        <Tooltip title="对勾选的章（每章一条）发给文本模型产出分镜剧本。不勾选=全部章。">
                                            <Button type="primary" size="small" icon={<Wand2 className="size-3" />} onClick={handleParseScript} disabled={parsingStoryboard}
                                                className="bg-stone-800 hover:bg-stone-700 dark:bg-stone-200 dark:text-stone-900 dark:hover:bg-stone-300">
                                                {parsingStoryboard ? <span className="flex items-center gap-1"><LoaderCircle className="size-3 animate-spin" />分镜中...</span> : `开始分镜${chapters.length > 0 && selectedGroups.size > 0 && selectedGroups.size < groupCount ? ` · ${selectedGroups.size}组` : ""}`}
                                            </Button>
                                        </Tooltip>
                                        {parsingStoryboard && <Button size="small" danger icon={<Pause className="size-3" />} onClick={stopPipeline}>停止</Button>}
                                    </div>
                                )}
                            </div>
        
                            {/* 全局搜索栏（同时搜章节/分镜/视频，命中自动展开定位） */}
                            {activeProject && (
                                <div className="flex items-center gap-2 shrink-0">
                                    <Input
                                        size="small"
                                        allowClear
                                        value={globalSearch}
                                        onChange={(e) => setGlobalSearch(e.target.value)}
                                        prefix={<Search className="size-3.5 text-stone-400" />}
                                        placeholder="全局搜索（章节/分镜/视频）"
                                        className="!rounded-lg !text-xs !w-48"
                                    />
                                    {globalSearch.trim() && (
                                        <span className="text-[11px] text-stone-400 whitespace-nowrap">
                                            章节 {globalMatchedChapterIds?.size ?? 0} ·
                                            分镜 {globalMatchedStoryboardIds?.size ?? 0} ·
                                            视频 {globalMatchedShotIds?.size ?? 0}
                                        </span>
                                    )}
                                </div>
                            )}
        
                            {/* novel-workflow v2: 5 层步骤条 + 主资产包 + BGM + 重做面板
                与 v1 stepper 并存 (v1 = 6 段细粒度 / v2 = 5 层抽象) */}
                            {activeProject && novelRun && (
                                <div className="shrink-0 space-y-2">
                                    <NovelWorkflowLayers
                                        run={novelRun}
                                        nodes={novelNodes}
                                        projectId={activeProject.id}
                                        onRefresh={refreshNovelRun}
                                        onMessage={(m) => m.type === "success" ? antMessage.success(m.text) : antMessage.error(m.text)}
                                    />
                                    <NovelSeriesAssetLockPanel
                                        projectId={activeProject.id}
                                        onChange={(lock) => {
                                            // 锁状态变化时刷新 workflow run (供 v1 stepper 参考)
                                            void refreshNovelRun();
                                        }}
                                    />
                                    <NovelBgmPicker
                                        projectId={activeProject.id}
                                        value={novelBg ?? undefined}
                                        onChange={setNovelBg}
                                    />
                                </div>
                            )}

                            {/* 6 段流水线 stepper：章节解析 → 分镜剧本 → 提示词 → 生图 → 对应分镜 → 视频
                比原来的单根 Progress 更直观；阶段高亮、失败红点、已完成/总数均显示 */}
            {(pipelineRunning || parsingStoryboard || pipelinePhase === "video" || pipelinePhase === "done" || pipelinePhase.startsWith("assets_")) && activeProject && (
                <div className="rounded-xl border border-stone-200 bg-stone-50 px-4 py-3 dark:border-stone-800 dark:bg-stone-900/50 shrink-0">
                    {/* 阶段行：6 段用 → 连接，当前段加 ring-1 高亮；容器窄时横向滚动，保证文字始终水平 */}
                    <div className="thin-scrollbar flex items-stretch gap-2 overflow-x-auto text-xs pb-1">
                        {/* 阶段定义：key、label、子状态文案源、进度 (current/total) */}
                        {(() => {
                            // 各阶段进度：分镜 = 任务进度；生图 = 期望生图数；视频 = 批量进度
                            const assetPending = pendingAssets.length + failedAssets.length;
                            const videoTotal = pipelineProgress.total > 0 && pipelinePhase === "video" ? pipelineProgress.total : undefined;
                            const videoCurrent = pipelinePhase === "video" ? pipelineProgress.current : undefined;
                            const storyboardTotal = (() => {
                                if (pipelinePhase === "storyboard") return pipelineProgress.total;
                                if (["assets_prompt", "assets_image", "assets_reconcile", "video", "done"].includes(pipelinePhase)) return pipelineProgress.total;
                                return undefined;
                            })();
                            const storyboardCurrent = pipelinePhase === "storyboard" ? pipelineProgress.current
                                : (["assets_prompt", "assets_image", "assets_reconcile", "video", "done"].includes(pipelinePhase)) ? pipelineProgress.total
                                : undefined;
                            const reconcileMissing = pipelinePhase === "assets_reconcile" ? computeCoverage(activeProject.storyboards || []).totalMissing : undefined;
                            type Stage = { key: "chapter_parse" | "storyboard" | "assets_prompt" | "assets_image" | "assets_reconcile" | "video"; label: string; status: "done" | "active" | "pending"; progressText?: string; failed?: number };
                            // 阶段完成判断：当前阶段之前的标 done；当前阶段 active；之后 pending
                            const phaseOrder = ["chapter_parse", "storyboard", "assets_prompt", "assets_image", "assets_reconcile", "video"] as const;
                            const phaseMap: Record<"idle" | "chapter_parse" | "storyboard" | "assets_prompt" | "assets_image" | "assets_reconcile" | "video" | "done", typeof phaseOrder[number]> = {
                                idle: "chapter_parse",
                                chapter_parse: "chapter_parse",
                                storyboard: "storyboard",
                                assets_prompt: "assets_prompt",
                                assets_image: "assets_image",
                                assets_reconcile: "assets_reconcile",
                                video: "video",
                                done: "video",
                            };
                            const phaseIdx = phaseOrder.indexOf(phaseMap[pipelinePhase]);
                            const stages: Stage[] = [
                                { key: "chapter_parse", label: "章节解析", status: phaseIdx > 0 ? "done" : phaseIdx === 0 ? "active" : "pending" },
                                { key: "storyboard", label: "分镜剧本", status: phaseIdx > 1 ? "done" : phaseIdx === 1 ? "active" : "pending",
                                  progressText: storyboardTotal ? `${storyboardCurrent ?? 0}/${storyboardTotal}` : undefined },
                                { key: "assets_prompt", label: "提示词", status: phaseIdx > 2 ? "done" : phaseIdx === 2 ? "active" : "pending",
                                  progressText: extractedAssets.length > 0 ? `${extractedAssets.length} 个待生图` : undefined },
                                { key: "assets_image", label: "生图", status: phaseIdx > 3 ? "done" : phaseIdx === 3 ? "active" : "pending",
                                  progressText: assetPending > 0 ? `${assets.length - assetPending} 完成${failedAssets.length > 0 ? ` · ${failedAssets.length} 失败` : ""}${pendingAssets.length > 0 ? ` · ${pendingAssets.length} 进行中` : ""}` : (assets.length > 0 ? `${assets.length}/${assets.length}` : undefined),
                                  failed: failedAssets.length },
                                { key: "assets_reconcile", label: "对应分镜", status: phaseIdx > 4 ? "done" : phaseIdx === 4 ? "active" : "pending",
                                  progressText: reconcileMissing !== undefined ? (reconcileMissing > 0 ? `缺 ${reconcileMissing} 个角色图` : "已全部对应") : undefined },
                                { key: "video", label: "视频生成", status: phaseIdx > 5 ? "done" : phaseIdx === 5 ? "active" : "pending",
                                  progressText: videoTotal ? `${videoCurrent ?? 0}/${videoTotal}` : undefined },
                            ];
                            return stages.map((s, i) => (
                                <div key={s.key} className="flex shrink-0 items-stretch">
                                    <div className={`flex min-w-[132px] w-[132px] flex-col gap-1 rounded-lg border px-2.5 py-1.5 transition ${
                                        s.status === "active" ? "border-stone-800 bg-white shadow-sm ring-1 ring-stone-300 dark:border-stone-200 dark:bg-stone-800 dark:ring-stone-600"
                                        : s.status === "done" ? "border-stone-200 bg-white dark:border-stone-700 dark:bg-stone-800/60"
                                        : "border-dashed border-stone-200 bg-stone-100/50 dark:border-stone-700 dark:bg-stone-900/50"
                                    }`}>
                                        <div className="flex items-center gap-1.5">
                                            <span className={`flex size-4 shrink-0 items-center justify-center rounded-full text-[10px] font-medium ${
                                                s.status === "active" ? "bg-stone-800 text-white dark:bg-stone-200 dark:text-stone-900"
                                                : s.status === "done" ? "bg-emerald-500 text-white"
                                                : "bg-stone-200 text-stone-500 dark:bg-stone-700 dark:text-stone-400"
                                            }`}>
                                                {s.status === "done" ? "✓" : i + 1}
                                            </span>
                                            <span className={`truncate font-medium ${s.status === "pending" ? "text-stone-400 dark:text-stone-500" : "text-stone-700 dark:text-stone-200"}`}>{s.label}</span>
                                        </div>
                                        <div className="flex items-center gap-1 pl-[22px] text-[10px]">
                                            {s.failed && s.failed > 0 ? (
                                                <span className="text-red-500">失败 {s.failed}</span>
                                            ) : s.progressText ? (
                                                <span className="text-stone-500">{s.progressText}</span>
                                            ) : s.status === "active" ? (
                                                <span className="flex items-center gap-1 text-stone-500"><LoaderCircle className="size-2.5 animate-spin" />进行中</span>
                                            ) : s.status === "done" ? (
                                                <span className="text-emerald-600">已完成</span>
                                            ) : (
                                                <span className="text-stone-400">待启动</span>
                                            )}
                                        </div>
                                    </div>
                                    {i < stages.length - 1 && (
                                        <div className={`mx-1 flex w-3 items-center justify-center text-stone-300 dark:text-stone-600`}>
                                            <ChevronRight className="size-3" />
                                        </div>
                                    )}
                                </div>
                            ));
                        })()}
                    </div>
                    {/* 当前阶段详细状态文案（行内） */}
                    {(pipelineRunning || parsingStoryboard) && pipelineStatus && (
                        <div className="mt-2 flex items-center justify-between text-xs text-stone-500 dark:text-stone-400">
                            <span className="flex items-center gap-1"><LoaderCircle className="size-3 animate-spin text-stone-400" />{pipelineStatus}</span>
                        </div>
                    )}
                </div>
            )}

            {/* novel-workflow v2: 整部成片重做面板 (合成后可用) */}
            {activeProject && novelRunId && pipelinePhase === "done" && (
                <div className="shrink-0">
                    <NovelRerunPanel
                        runId={novelRunId}
                        projectId={activeProject.id}
                        scope="full"
                        layer="composition"
                        compositionInput={{
                            shotVideos: (activeProject.shots || []).map((s) => ({
                                shotId: s.id,
                                url: s.videoUrl ?? "",
                                durationMs: (s.duration ?? 4000),
                            })),
                            subtitleStyle: {
                                font: "黑体",
                                size: 36,
                                color: "FFFFFF",
                                outline: "000000",
                                outlineWidth: 2,
                                position: "bottom",
                                marginBottom: 60,
                            },
                            bgmSource: novelBg?.presetId ? { presetId: novelBg.presetId } : {},
                            bgmVolume: novelBg?.volume ?? 0.3,
                            bgmFadeInMs: novelBg?.fadeInMs ?? 0,
                            bgmFadeOutMs: novelBg?.fadeOutMs ?? 0,
                        }}
                        onRerunDone={refreshNovelRun}
                    />
                </div>
            )}
        
                            {activeProject ? (
                                chapters.length > 0 ? (
                                    /* ── 章节目录（原始小说，第一章展开其余折叠，每章一条分镜标注） ── */
                                    <div ref={chapterListRef} className="thin-scrollbar min-h-0 flex-1 overflow-y-auto rounded-xl border border-stone-200 bg-white dark:border-stone-800 dark:bg-stone-900">
                                        {chapters.map((ch, ci) => {
                                            // 优先用全局搜索结果，否则用独立章节搜索
                                            const kw = globalSearch.trim() || chapterSearch.trim();
                                            const matchedSet = globalSearch.trim() ? globalMatchedChapterIds : matchedChapterIds;
                                            const isMatch = !matchedSet || matchedSet.has(ch.id);
                                            if (matchedSet && !isMatch) return null;
                                            // 命中章节自动展开正文便于定位
                                            const expanded = matchedSet ? true : expandedChapterIds.has(ch.id);
                                            const gi = ci; // 一章一组，组号 = 章号
                                            return (
                                                <div key={ch.id} className="border-b border-stone-100 last:border-0 dark:border-stone-800">
                                                    <div className="flex w-full items-center gap-2 px-4 py-2.5 hover:bg-stone-50 dark:hover:bg-stone-800/50">
                                                        <input type="checkbox"
                                                            checked={selectedGroups.has(gi)}
                                                            onChange={() => toggleGroupSelect(gi)}
                                                            className="size-3.5 shrink-0 accent-stone-700 dark:accent-stone-300" />
                                                        <button onClick={() => toggleChapterExpand(ch.id)}
                                                            className="flex min-w-0 flex-1 items-center gap-2 text-left">
                                                            <ChevronsRight className={`size-3.5 shrink-0 text-stone-400 transition-transform ${expanded ? "rotate-90" : ""}`} />
                                                            <span className={`min-w-0 flex-1 truncate text-sm font-medium ${matchedChapterIds ? "text-amber-600 dark:text-amber-400" : ""}`}>{ci + 1}. {ch.title}</span>
                                                        </button>
                                                        {/* P1 b1/b2：本章切几个分镜（默认 1） */}
                                                        <Tooltip title="本章切成几个分镜（>1 时后端引导模型输出 ===SHOT=== 分隔的多条）">
                                                            <label className="flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] text-stone-500 hover:bg-stone-100 dark:hover:bg-stone-800">
                                                                切
                                                                <InputNumber size="small" min={1} max={5} value={chapterShotCounts[gi] ?? 1}
                                                                    onChange={(v) => setChapterShotCounts((prev) => ({ ...prev, [gi]: Math.max(1, Math.min(5, v || 1)) }))}
                                                                    className="!w-12" />
                                                                镜
                                                            </label>
                                                        </Tooltip>
                                                        <span className="shrink-0 text-[10px] text-stone-400">{ch.content.length} 字</span>
                                                    </div>
                                                    {expanded && (
                                                        <div className="px-4 pb-3">
                                                            <p className="thin-scrollbar max-h-64 overflow-y-auto whitespace-pre-wrap rounded-lg bg-stone-50 px-3 py-2 text-xs leading-6 text-stone-600 dark:bg-stone-800 dark:text-stone-300">{ch.content}</p>
                                                        </div>
                                                    )}
                                                </div>
                                            );
                                        })}
                                        {((globalMatchedChapterIds && globalMatchedChapterIds.size === 0) || (matchedChapterIds && matchedChapterIds.size === 0)) && (
                                            <div className="flex flex-col items-center justify-center gap-1 px-4 py-10 text-center text-xs text-stone-400">
                                                <Search className="size-5 text-stone-300" />
                                                没有匹配「{(globalSearch.trim() || chapterSearch.trim())}」的章节
                                            </div>
                                        )}
                                    </div>
                                ) : (
                                    /* ── 未解析章节：提示导入 ── */
                                    <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-xl border-2 border-dashed border-stone-200 bg-stone-50 dark:border-stone-800 dark:bg-stone-900/50">
                                        <div className="text-center">
                                            <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl bg-stone-100 dark:bg-stone-800">
                                                <FileText className="size-6 text-stone-400" />
                                            </div>
                                            <div className="mb-3 text-xs text-stone-400">导入小说原文<br />AI 自动按"第N章"生成章节目录</div>
                                            <Button size="small" icon={<FolderOpen className="size-3" />} onClick={handleImportFileClick}>导入 txt</Button>
                                        </div>
                                    </div>
                                )
                            ) : (
                                <div className="flex flex-1 flex-col items-center justify-center rounded-xl border-2 border-dashed border-stone-200 bg-stone-50 dark:border-stone-800 dark:bg-stone-900/50">
                                    <div className="text-center">
                                        <div className="mx-auto mb-3 flex size-14 items-center justify-center rounded-2xl bg-stone-100 dark:bg-stone-800">
                                            <Video className="size-7 text-stone-400" />
                                        </div>
                                        <div className="mb-1 text-sm font-medium">创建剧本项目</div>
                                        <div className="mb-3 text-xs text-stone-400">输入小说，AI 每章整合为 1 条完整分镜并生成视频</div>
                                        <Button type="primary" icon={<Plus className="size-3.5" />} onClick={createNewProject} className="rounded-lg bg-stone-800 hover:bg-stone-700 dark:bg-stone-200 dark:text-stone-900 dark:hover:bg-stone-300">新建剧本</Button>
                                    </div>
                                </div>
                            )}
                        </div>
                    </div>
                )}

                <div className="flex min-w-0 flex-1 flex-col gap-4 overflow-hidden">
                    {/* 分镜图片 Library（中列 50/50：上半=提取配置+预览，下半=生成图片占位网格） */}
                    <div className="rounded-xl border border-stone-200 dark:border-stone-800 flex min-h-0 flex-1 flex-col overflow-hidden">
                        <div className="flex items-center border-b border-stone-200 dark:border-stone-800 shrink-0">
                            {(["character", "scene", "prop"] as AssetType[]).map((tab) => {
                                const assetCount = assets.filter((a) => a.type === tab).length;
                                const pendingCount = pendingAssets.filter((p) => p.type === tab).length;
                                const failedCount = failedAssets.filter((f) => f.type === tab).length;
                                const totalCount = assetCount + pendingCount + failedCount;
                                const icons: Record<string, React.ReactNode> = {
                                    character: <Users className="size-3" />,
                                    scene: <LayoutGrid className="size-3" />,
                                    prop: <Package className="size-3" />,
                                };
                                return (
                                    <button key={tab} onClick={() => setAssetTab(tab)}
                                        className={`flex flex-1 items-center justify-center gap-1 py-2 text-xs transition ${
                                            assetTab === tab ? "border-b-2 border-stone-800 text-stone-900 dark:border-stone-200 dark:text-stone-100" : "text-stone-500 hover:text-stone-700 dark:hover:text-stone-300"
                                        }`}>
                                        {icons[tab]}
                                        <span>{tab === "character" ? "人物" : tab === "scene" ? "场景" : "道具"}</span>
                                        {totalCount > 0 && (
                                            <span className={`ml-0.5 rounded-full px-1 text-[10px] ${failedCount > 0 ? "bg-red-200 text-red-700 dark:bg-red-800 dark:text-red-200" : pendingCount > 0 ? "bg-amber-200 text-amber-700 dark:bg-amber-800 dark:text-amber-200" : "bg-stone-200 dark:bg-stone-700"}`}>
                                                {failedCount > 0 ? `${assetCount}+${failedCount}失败` : pendingCount > 0 ? `${assetCount}+${pendingCount}` : assetCount}
                                            </span>
                                        )}
                                    </button>
                                );
                            })}
                        </div>

                        {/* 上半 50%：分镜图片生成配置 + 画风提取 + 已提取预览（提取的人物 prompt 区域） */}
                        <div className="border-b border-stone-200 dark:border-stone-800 p-2 space-y-2 min-h-0 flex-1 flex flex-col overflow-hidden">
                            <div className="flex items-center justify-between text-xs text-stone-600 dark:text-stone-400 shrink-0">
                                <span className="font-medium">生成分镜图片</span>
                                <span className="text-[10px]">文本模型提取 → 预览 → 图片模型生成</span>
                            </div>

                            {/* 类型筛选 */}
                            <div className="flex gap-3 text-xs shrink-0">
                                <label className="flex items-center gap-1">
                                    <input type="checkbox" checked={generateCharacterAssets} onChange={(e) => setGenerateCharacterAssets(e.target.checked)} className="size-3 accent-stone-700" />
                                    <span>角色</span>
                                </label>
                                <label className="flex items-center gap-1">
                                    <input type="checkbox" checked={generateSceneAssets} onChange={(e) => setGenerateSceneAssets(e.target.checked)} className="size-3 accent-stone-700" />
                                    <span>场景</span>
                                </label>
                                <label className="flex items-center gap-1">
                                    <input type="checkbox" checked={generatePropAssets} onChange={(e) => setGeneratePropAssets(e.target.checked)} className="size-3 accent-stone-700" />
                                    <span>道具</span>
                                </label>
                            </div>

                            {/* P1 改造 a3：资产库搜索（按 alias / name / description 全文搜；切换 Tab 时保留关键词） */}
                            <Input
                                size="small"
                                allowClear
                                value={assetSearch}
                                onChange={(e) => setAssetSearch(e.target.value)}
                                prefix={<Search className="size-3 text-stone-400" />}
                                placeholder="搜索资产（alias / name / description）"
                                className="!rounded-lg !text-xs shrink-0"
                            />

                            <div className="min-h-0 flex-1 flex flex-col overflow-hidden gap-2">
                                {/* Step 1: 提取 */}
                                {extractedAssets.length === 0 ? (
                                    <div className="shrink-0">
                                        <Button
                                            block
                                            size="small"
                                            icon={<Wand2 className="size-3" />}
                                            onClick={extractAssetsStep}
                                            disabled={extractingAssets}
                                            className="bg-stone-800 text-white hover:bg-stone-700 dark:bg-stone-200 dark:text-stone-900 dark:hover:bg-stone-300"
                                        >
                                            {extractingAssets ? <span className="flex items-center justify-center gap-1"><LoaderCircle className="size-3 animate-spin" />提取中...</span> : selectedGroups.size === 0 ? "请先勾选章节后提取" : "① 从选中章节提取"}
                                        </Button>
                                    </div>
                                ) : (
                                    <>
                                        {/* 画风提示词 - 可折叠 */}
                                        <div className="rounded border border-stone-200 dark:border-stone-700 shrink-0">
                                            <button
                                                onClick={() => setStylePromptExpanded(!stylePromptExpanded)}
                                                className="flex w-full items-center justify-between px-2 py-1.5 text-xs"
                                            >
                                                <span className="text-stone-600 dark:text-stone-400">画风提示词（自动提取）</span>
                                                <ChevronsRight className={`size-3 text-stone-400 transition-transform ${stylePromptExpanded ? "rotate-90" : ""}`} />
                                            </button>
                                            {stylePromptExpanded && (
                                                <div className="border-t border-stone-200 p-2 dark:border-stone-700">
                                                    <textarea
                                                        className="w-full rounded border border-stone-300 bg-white px-2 py-1 text-xs text-stone-800 dark:border-stone-600 dark:bg-stone-800 dark:text-stone-200"
                                                        rows={2}
                                                        placeholder="文本模型自动提取的画风，可修改"
                                                        value={extractedStyle}
                                                        onChange={(e) => setExtractedStyle(e.target.value)}
                                                    />
                                                </div>
                                            )}
                                        </div>

                                        {/* 提取预览 - 按类型筛选，点击展开（占据剩余上半空间滚动） */}
                                        <div className="rounded border border-stone-200 dark:border-stone-700 min-h-0 flex-1 flex flex-col overflow-hidden">
                                            <button
                                                onClick={() => setExtractedListExpanded(!extractedListExpanded)}
                                                className="flex w-full items-center justify-between px-2 py-1.5 text-xs shrink-0"
                                            >
                                                <span className="text-stone-600 dark:text-stone-400">
                                                    已提取 {extractedAssets.length} 个
                                                    <span className="ml-1 text-[10px] text-blue-500">角色{extractedAssets.filter(a => a.type === "character").length}</span>
                                                    <span className="ml-1 text-[10px] text-green-500">场景{extractedAssets.filter(a => a.type === "scene").length}</span>
                                                    <span className="ml-1 text-[10px] text-amber-500">道具{extractedAssets.filter(a => a.type === "prop").length}</span>
                                                </span>
                                                <ChevronsRight className={`size-3 text-stone-400 transition-transform ${extractedListExpanded ? "rotate-90" : ""}`} />
                                            </button>
                                            <div className="thin-scrollbar min-h-0 flex-1 overflow-y-auto border-t border-stone-200 dark:border-stone-700">
                                                {extractedAssets.filter((a) => a.type === assetTab).length === 0 ? (
                                                    <div className="px-2 py-2 text-center text-[10px] text-stone-400">此类型无资产</div>
                                                ) : (
                                                    <div className="space-y-0.5 p-1">
                                                        {extractedAssets.filter((a) => a.type === assetTab).map((a) => {
                                                            const typeBadgeClass = a.type === "character"
                                                                ? "bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-300"
                                                                : a.type === "scene"
                                                                    ? "bg-green-100 text-green-600 dark:bg-green-900 dark:text-green-300"
                                                                    : "bg-amber-100 text-amber-600 dark:bg-amber-900 dark:text-amber-300";
                                                            const typeLabel = a.type === "character" ? "角色" : a.type === "scene" ? "场景" : "道具";
                                                            // 卡片预览：取描述首行（去掉首尾空白）作为一行预览
                                                            const preview = a.description.split(/\n/)[0]?.trim() || "（空描述，点击编辑）";
                                                            return (
                                                                <div key={a.id} className="group flex items-center gap-1.5 rounded-lg border border-stone-100 px-2 py-1.5 hover:bg-stone-50 dark:border-stone-800 dark:hover:bg-stone-800/50">
                                                                    <button
                                                                        type="button"
                                                                        onClick={() => {
                                                                            setViewAssetId(a.id);
                                                                            setAssetDraftName(a.name);
                                                                            setAssetDraftDescription(a.description);
                                                                            setAssetDraftType(a.type);
                                                                        }}
                                                                        className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
                                                                    >
                                                                        <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${typeBadgeClass}`}>{typeLabel}</span>
                                                                        <span className="min-w-0 flex-1 truncate text-xs text-stone-700 dark:text-stone-200">
                                                                            <span className="font-medium">{a.name || "（未命名）"}</span>
                                                                            <span className="ml-1 text-stone-400">·</span>
                                                                            <span className="ml-1 text-stone-500 dark:text-stone-400">{preview}</span>
                                                                        </span>
                                                                    </button>
                                                                    <button
                                                                        type="button"
                                                                        onClick={() => setExtractedAssets((prev) => prev.filter((x) => x.id !== a.id))}
                                                                        className="shrink-0 text-stone-300 hover:text-red-400 dark:text-stone-600 dark:hover:text-red-400"
                                                                        title="删除该资产"
                                                                    >
                                                                        <X className="size-3" />
                                                                    </button>
                                                                </div>
                                                            );
                                                        })}
                                                        {/* 手动新增资产（当前 tab 类型） */}
                                                        <button
                                                            type="button"
                                                            onClick={() => {
                                                                const newAsset = { id: nanoid(), name: "", type: assetTab, description: "" };
                                                                setExtractedAssets((prev) => [...prev, newAsset]);
                                                                setViewAssetId(newAsset.id);
                                                                setAssetDraftName(newAsset.name);
                                                                setAssetDraftDescription(newAsset.description);
                                                                setAssetDraftType(newAsset.type);
                                                            }}
                                                            className="mt-1 flex w-full items-center justify-center gap-1 rounded-lg border border-dashed border-stone-200 py-1.5 text-[11px] text-stone-400 hover:border-stone-300 hover:text-stone-500 dark:border-stone-700 dark:hover:border-stone-600"
                                                        >
                                                            <Plus className="size-3" />
                                                            新增{assetTab === "character" ? "角色" : assetTab === "scene" ? "场景" : "道具"}
                                                        </button>
                                                    </div>
                                                )}
                                            </div>
                                        </div>

                                        {/* 重新提取 + 生成图片 */}
                                        <div className="flex gap-2 shrink-0">
                                            <Button size="small" icon={<RefreshCw className="size-3" />} onClick={extractAssetsStep} className="flex-1 text-xs">重新提取</Button>
                                            <Button
                                                block
                                                size="small"
                                                icon={<ImageIcon className="size-3" />}
                                                onClick={() => generateImagesFromExtracted()}
                                                disabled={assetGenerationRunning}
                                                className="flex-[2] bg-stone-800 text-white hover:bg-stone-700 dark:bg-stone-200 dark:text-stone-900 dark:hover:bg-stone-300"
                                            >
                                                {assetGenerationRunning ? <span className="flex items-center justify-center gap-1"><LoaderCircle className="size-3 animate-spin" />生成中...</span> : `② 生成图片（${extractedAssets.length} 个）`}
                                            </Button>
                                        </div>
                                    </>
                                )}
                            </div>
                        </div>

                        {/* 下半 50%：图片导入按钮 + 网格卡片（生成的图片占位区） */}
                        <div className="p-2 space-y-1.5 min-h-0 flex-1 flex flex-col overflow-hidden">
                            <div className="flex gap-1.5 shrink-0 items-center">
                                <label className="cursor-pointer block flex-1">
                                    <input type="file" accept="image/*" multiple className="hidden" onChange={(e) => handleFileUpload(e, assetTab)} />
                                    <Button size="small" icon={<Upload className="size-3" />} className="w-full text-xs" onClick={(e) => (e.currentTarget.parentElement as HTMLElement).querySelector("input")?.click()}>导入图片</Button>
                                </label>
                                <Button size="small" icon={<RefreshCw className="size-3" />} className="flex-1 text-xs" onClick={() => void syncAssetsFromStore()}>从素材库同步</Button>
                                {/* 列数切换按钮 */}
                                <div className="flex items-center gap-0.5 rounded-lg bg-stone-100 p-0.5 dark:bg-stone-800">
                                    <button
                                        type="button"
                                        onClick={() => setAssetGridCols(2)}
                                        className={`rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
                                            assetGridCols === 2
                                                ? "bg-white text-stone-900 shadow-sm dark:bg-stone-700 dark:text-stone-100"
                                                : "text-stone-500 hover:text-stone-700 dark:text-stone-400 dark:hover:text-stone-300"
                                        }`}
                                        title="大图模式（2列）"
                                    >
                                        2列
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => setAssetGridCols(3)}
                                        className={`rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
                                            assetGridCols === 3
                                                ? "bg-white text-stone-900 shadow-sm dark:bg-stone-700 dark:text-stone-100"
                                                : "text-stone-500 hover:text-stone-700 dark:text-stone-400 dark:hover:text-stone-300"
                                        }`}
                                        title="紧凑模式（3列）"
                                    >
                                        3列
                                    </button>
                                </div>
                            </div>

                            {/* 网格卡片区域（滚动，与提取区五五开） —— P1 改造 a3：按 assetSearch 过滤 */}
                            {(() => {
                                const assetsForTab = (filteredAssets ?? assets).filter((a) => a.type === assetTab);
                                const pendingForTab = (filteredPendingAssets ?? pendingAssets).filter((p) => p.type === assetTab);
                                const failedForTab = (filteredFailedAssets ?? failedAssets).filter((f) => f.type === assetTab);
                                if (assetsForTab.length === 0 && pendingForTab.length === 0 && failedForTab.length === 0) {
                                    return (
                                        <div className="py-4 text-center text-xs text-stone-400">
                                            {assetSearch.trim() ? `没有匹配「${assetSearch}」的资产` : "暂无素材"}
                                        </div>
                                    );
                                }
                                return null;
                            })()}
                            {(!assetSearch.trim() || (filteredAssets ?? assets).filter((a) => a.type === assetTab).length > 0) && (
                                <div id="novel-asset-grid" className={`thin-scrollbar grid ${assetGridCols === 2 ? "grid-cols-2" : "grid-cols-3"} gap-2 min-h-0 flex-1 overflow-y-auto`}>
                                    {/* Pending 生成中的卡片 */}
                                    {(filteredPendingAssets ?? pendingAssets).filter((p) => p.type === assetTab).map((p) => (
                                        <div key={p.id} className="relative overflow-hidden rounded-lg border border-dashed border-stone-300 bg-stone-50 dark:border-stone-700 dark:bg-stone-900">
                                            <div
                                                className="absolute inset-0 opacity-40"
                                                style={{
                                                    backgroundImage: "radial-gradient(circle, rgba(120,113,108,0.35) 1.4px, transparent 1.6px)",
                                                    backgroundSize: "12px 12px",
                                                }}
                                            />
                                            <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 px-2 py-3">
                                                <LoaderCircle className="size-5 animate-spin text-stone-500 dark:text-stone-400" />
                                                <span className="text-xs font-medium text-stone-600 dark:text-stone-300 truncate max-w-full">{p.name}</span>
                                                <span className="text-[10px] text-stone-400">正在生成图片</span>
                                                <Tooltip title={p.prompt} placement="top">
                                                    <span className="line-clamp-3 max-w-full text-center text-[10px] break-all whitespace-pre-wrap text-stone-500 dark:text-stone-400">
                                                        {p.prompt}
                                                    </span>
                                                </Tooltip>
                                            </div>
                                        </div>
                                    ))}

                                    {/* 失败的资产卡片 */}
                                    {failedAssets.filter((f) => f.type === assetTab).map((f) => (
                                        <div key={f.id} className="relative overflow-hidden rounded-lg border border-red-300 bg-red-50 dark:border-red-800 dark:bg-red-950/30" title={f.error || "生成失败"}>
                                            <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 px-2 py-3">
                                                <AlertCircle className="size-5 text-red-500 dark:text-red-400" />
                                                <span className="text-xs font-medium text-red-700 dark:text-red-300 truncate max-w-full">{f.name}</span>
                                                <span className="line-clamp-2 px-1 text-center text-[10px] text-red-600 dark:text-red-300 max-w-full" title={f.error}>
                                                    {f.error || "生成失败"}
                                                </span>
                                            </div>
                                            <button
                                                onClick={() => setFailedAssets((prev) => prev.filter((x) => x.id !== f.id))}
                                                className="absolute top-1 right-1 size-5 flex items-center justify-center rounded bg-white/80 text-red-500 hover:bg-red-100 dark:bg-stone-900/80"
                                                title="关闭"
                                            >
                                                <X className="size-3" />
                                            </button>
                                        </div>
                                    ))}

                                    {/* 已完成的资产卡片 —— P1 改造 a3：按 filteredAssets 过滤 */}
                                    {(filteredAssets ?? assets).filter((a) => a.type === assetTab).map((a) => {
                                        const prompt = a.description ? buildAssetPrompt(a) : "";
                                        return (
                                            <div key={a.id} className="group overflow-hidden rounded-lg border border-stone-200 dark:border-stone-700">
                                                <div className="relative aspect-square bg-stone-100 dark:bg-stone-800">
                                                    {a.dataUrl ? (
                                                        <Image
                                                            src={a.dataUrl} alt={a.name}
                                                            className="!rounded-none h-full w-full object-cover"
                                                            preview={{ mask: <span className="text-[10px] text-white">预览</span> }}
                                                        />
                                                    ) : (
                                                        <div className="flex h-full items-center justify-center">
                                                            <label className="cursor-pointer text-center">
                                                                <input type="file" accept="image/*" className="hidden" onChange={(e) => {
                                                                    const file = e.target.files?.[0];
                                                                    if (!file) return;
                                                                    const reader = new FileReader();
                                                                    reader.onload = () => {
                                                                        const dataUrl = reader.result as string;
                                                                        const next = assets.map((x) => x.id === a.id ? { ...x, dataUrl } : x);
                                                                        setAssets(next);
                                                                        void saveAssets(next);
                                                                    };
                                                                    reader.readAsDataURL(file);
                                                                }} />
                                                                <span className="inline-block rounded border border-dashed border-stone-300 px-2 py-1 text-[10px] text-stone-400 hover:border-stone-400 dark:border-stone-600">上传</span>
                                                            </label>
                                                        </div>
                                                    )}
                                                    {/* 操作按钮 */}
                                                    <div className="absolute top-1 right-1 flex gap-1 opacity-0 transition group-hover:opacity-100">
                                                        <button onClick={() => { setEditingAssetId(a.id); setEditAssetName(a.name); setEditAssetAlias(a.alias); setEditAssetDesc(a.description || ""); }}
                                                            className="size-5 flex items-center justify-center rounded bg-white/80 text-stone-500 hover:bg-white dark:bg-stone-900/80 dark:text-stone-400" title="编辑"><Pencil className="size-3" /></button>
                                                        <button onClick={() => deleteAsset(a.id)}
                                                            className="size-5 flex items-center justify-center rounded bg-white/80 text-stone-500 hover:bg-red-100 hover:text-red-500 dark:bg-stone-900/80 dark:text-stone-400" title="删除"><X className="size-3" /></button>
                                                    </div>
                                                </div>
                                                <div className="p-1.5">
                                                    <div className="text-[11px] font-medium text-stone-800 dark:text-stone-200 truncate">{a.name}</div>
                                                    {a.description && <div className="text-[10px] text-stone-400 truncate">{a.description}</div>}
                                                    {prompt && (
                                                        <button
                                                            onClick={() => { navigator.clipboard.writeText(prompt); antMessage.success("提示词已复制"); }}
                                                            className="mt-0.5 flex w-full items-center justify-center gap-0.5 rounded bg-stone-100 px-1 py-0.5 text-[10px] text-stone-500 hover:bg-stone-200 dark:bg-stone-800 dark:text-stone-400"
                                                            title="复制提示词"
                                                        >
                                                            <Copy className="size-2.5" />
                                                            <span>复制提示词</span>
                                                        </button>
                                                    )}
                                                </div>
                                            </div>
                                        );
                                    })}
                                </div>
                            )}
                        </div>
                    </div>

                    {/* 提示词配置入口：默认打开分镜剧本 tab，因为这是用户改 prompt 最多的地方 */}
                    {activeProject && (
                        <Button block size="small" icon={<Settings2 className="size-3.5" />}
                            onClick={() => { setPromptDraftTab("script"); setPromptDraft(scriptPrompt); setShowPromptModal(true); }}
                            className="rounded-lg text-xs">提示词配置（4 套预设）</Button>
                    )}
                </div>
                {/* ── RIGHT: 分镜剧本目录 + 分镜视频 ── */}
                {activeProject && (
                    <div className="flex w-[420px] shrink-0 flex-col gap-4 overflow-hidden">

                        {/* 分镜剧本列表（主区域：平铺、全局连续编号；一个分镜剧本≈一个视频，可增删改并逐个生成视频） */}
                        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                            <div className="mb-2 flex items-center justify-between shrink-0">
                                <h2 className="flex items-center gap-2 text-sm font-medium">
                                    <FileText className="size-4" />分镜剧本
                                    <span className="rounded-full bg-stone-100 px-2 py-0.5 text-[11px] text-stone-500 dark:bg-stone-800 dark:text-stone-400">{storyboards.length} 条 · {storyboardVideoDoneCount} 已出片</span>
                                    {missingAssetNames.size > 0 && (
                                        <Tooltip title={`以下角色未在素材库关联，生成视频不会带其参考图：${[...missingAssetNames].map((n) => `@${n}`).join("、")}。请在左侧「人物」上传并把「@别名」设为对应名字。`}>
                                            <span className="flex items-center gap-1 rounded-full bg-red-50 px-2 py-0.5 text-[11px] font-medium text-red-500 dark:bg-red-950/40">
                                                <AlertTriangle className="size-3" />{missingAssetNames.size} 个角色缺素材
                                            </span>
                                        </Tooltip>
                                    )}
                                </h2>
                                {storyboards.length > 0 && (
                                    <div className="flex items-center gap-1">
                                        <Input
                                            allowClear size="small"
                                            prefix={<Search className="size-3 text-stone-400" />}
                                            placeholder="搜索分镜（如分镜6或6）"
                                            value={storyboardSearch}
                                            onChange={(e) => setStoryboardSearch(e.target.value)}
                                            className="!h-6 w-36"
                                        />
                                        {/* 全选复选框 */}
                                        <label className="flex items-center gap-1 text-xs text-stone-500 cursor-pointer hover:text-stone-700 dark:hover:text-stone-300">
                                            <input
                                                type="checkbox"
                                                checked={selectedStoryboardIds.size === storyboards.filter((s) => storyboardIsReady(s)).length}
                                                onChange={(e) => toggleAllStoryboardSelect(e.target.checked)}
                                                className="accent-stone-600 size-3"
                                            />
                                            全选
                                        </label>
                                        <Tooltip title="新增一个空白分镜剧本">
                                            <Button size="small" type="text" icon={<Plus className="size-3.5" />} onClick={() => addStoryboard()} className="h-6 w-6 p-0" />
                                        </Tooltip>
                                        {/* 批量删除勾选的分镜剧本 */}
                                        <Tooltip title={selectedStoryboardCount > 0 ? `删除选中的 ${selectedStoryboardCount} 条分镜剧本` : "先勾选分镜剧本再批量删除"}>
                                            <Button size="small" type="text" danger icon={<Trash2 className="size-3.5" />} disabled={selectedStoryboardCount === 0} onClick={batchDeleteStoryboards} className="h-6 w-6 p-0" />
                                        </Tooltip>
                                        {/* 为指定分镜生成视频（支持勾选多个 + 停止） */}
                                        {storyboardVideoAbortController ? (
                                            <Tooltip title="停止当前视频生成">
                                                <Button size="small" type="text" danger icon={<Pause className="size-3.5" />} onClick={stopStoryboardVideoGeneration} className="h-6 w-6 p-0" />
                                            </Tooltip>
                                        ) : (
                                            <Tooltip title={selectedStoryboardCount > 0 ? `为选中的 ${selectedStoryboardCount} 个分镜生成视频` : "为所有分镜剧本生成视频"}>
                                                <Button size="small" type="text" icon={<Coins className="size-3.5" />} disabled={pipelineRunning && !storyboardVideoAbortController} onClick={() => generateAllStoryboardVideos()} className="h-6 w-6 p-0" />
                                            </Tooltip>
                                        )}
                                    </div>
                                )}
                            </div>
                            {storyboards.length > 0 ? (
                                <div ref={storyboardListRef} className="thin-scrollbar min-h-0 flex-1 space-y-0.5 overflow-y-auto rounded-xl border border-stone-200 bg-white p-1 dark:border-stone-800 dark:bg-stone-900">
                                    {storyboards.map((sb, si) => {
                                        // 优先用全局搜索结果，否则用独立分镜搜索
                                        const kw = globalSearch.trim() || storyboardSearch.trim();
                                        const matchedSet = globalSearch.trim() ? globalMatchedStoryboardIds : matchedStoryboardIds;
                                        if (matchedSet && !matchedSet.has(sb.id)) return null;
                                        // 分镜剧本正文首行（去掉【场景N】标记）作为预览标题
                                        const preview = sb.content.replace(/^\s*【?\s*(?:场景|分镜|镜头|视频|Shot|Scene)\s*\d+\s*】?\s*[:：]?\s*/i, "").split(/\n/)[0]?.trim() || "（空白分镜，点击编辑）";
                                        // 失败占位分镜（⚠ 开头）不做资产关联展示
                                        const isPlaceholder = sb.shotStatus === "failed" || (sb.content || "").trimStart().startsWith("⚠");
                                        // 该分镜 @到的角色/资产关联状态（linked=已关联会带图，missing=缺素材）
                                        const assetLinks = isPlaceholder ? [] : getStoryboardAssetLinks(sb.content);
                                        const headerInfo = isPlaceholder ? { characters: [], scenes: [] } : extractStoryboardHeader(sb.content);
                                        // 4 段 stepper 任务 3：缺参考图持久化
                                        //   在「分镜 N」标签旁加红色角标，让用户即使没看到 modal 也能定位有问题的分镜
                                        const unmatchedCount = isPlaceholder ? 0 : headerInfo.characters.filter((m) => !assets.find((a) => a.alias === m)).length;
                                        return (
                                            <div key={sb.id} data-storyboard-id={sb.id} ref={matchedSet && si === 0 ? globalSearchHitRef : undefined} className={`rounded-lg ${
                                                    unmatchedCount > 0 ? "border-2 border-red-300 dark:border-red-700" : matchedSet ? "border-2 border-amber-400 bg-amber-50 dark:border-amber-500 dark:bg-amber-950/30" : ""
                                                }`}>
                                                <div className={`group flex flex-col gap-1 rounded-lg border px-2 py-1.5 hover:bg-stone-50 ${
                                                    unmatchedCount > 0 ? "border-red-200 bg-red-50/30 dark:border-red-900 dark:bg-red-950/10 dark:hover:bg-red-950/20" : "border-stone-100 dark:border-stone-800 dark:hover:bg-stone-800/50"
                                                }`}>
                                                  <div className="flex items-center gap-1.5">
                                                    <button onClick={() => { setViewStoryboardId(sb.id); setStoryboardDraft(sb.content); setStoryboardGroupDraft(sb.groupIndex); }} className="flex min-w-0 flex-1 items-center gap-1.5 text-left">
                                                        <span className="shrink-0 rounded bg-stone-200 px-1.5 py-0.5 text-[10px] font-medium text-stone-600 dark:bg-stone-700 dark:text-stone-300">分镜{si + 1}</span>
                                                        <span className={`min-w-0 flex-1 truncate text-xs ${isPlaceholder ? "text-red-500" : matchedStoryboardIds ? "text-amber-600 dark:text-amber-400" : "text-stone-600 dark:text-stone-300"}`}>{preview}</span>
                                                    </button>
                                                    {unmatchedCount > 0 && (
                                                        <Tooltip title={`该分镜缺 ${unmatchedCount} 个角色参考图（红色背景提示）。点这里聚焦资产库搜索，便于上传同名图片。`}>
                                                            <button
                                                                type="button"
                                                                onClick={(e) => { e.stopPropagation(); jumpToMissingAssets(sb); }}
                                                                className="flex shrink-0 items-center gap-0.5 rounded bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700 ring-1 ring-red-300 hover:bg-red-200 dark:bg-red-950/40 dark:text-red-300 dark:ring-red-800 dark:hover:bg-red-900/60"
                                                            >
                                                                <AlertTriangle className="size-2.5" />缺 {unmatchedCount} 图 · 去补
                                                            </button>
                                                        </Tooltip>
                                                    )}
                                                    {sb.videoStatus === "queued" && <span className="shrink-0 text-[10px] text-stone-400">排队中</span>}
                                                    {sb.videoStatus === "generating" && <LoaderCircle className="size-3 shrink-0 animate-spin text-stone-400" />}
                                                    {sb.videoStatus === "success" && <span className="shrink-0 text-[10px] text-green-600">已出片</span>}
                                                    {sb.videoStatus === "failed" && <span className="shrink-0 text-[10px] text-red-500">失败</span>}
                                                    {/* 勾选框：选择本次批量生成哪些分镜 */}
                                                    <input
                                                        type="checkbox"
                                                        checked={selectedStoryboardIds.has(sb.id)}
                                                        onChange={(e) => { e.stopPropagation(); toggleStoryboardSelect(sb.id); }}
                                                        className="accent-stone-600 size-3 shrink-0"
                                                        title={selectedStoryboardIds.has(sb.id) ? "取消勾选（不从批量生成）" : "勾选（加入批量生成）"}
                                                    />
                                                    <Tooltip title={sb.videoStatus === "success" && sb.videoUrl ? "重新生成视频（覆盖已有视频）" : "根据该分镜剧本生成视频"}>
                                                        <Button size="small" type="text" icon={<Video className="size-3" />} onClick={() => generateVideoFromStoryboard(sb.id, undefined, undefined, sb.videoStatus === "success" && !!sb.videoUrl)} className="h-6 w-6 p-0 opacity-60 group-hover:opacity-100" />
                                                    </Tooltip>
                                                    <Tooltip title="重新生成该分镜剧本">
                                                        <Button size="small" type="text" icon={<RefreshCw className="size-3" />} onClick={() => void regenerateStoryboard(sb.id)} className="h-6 w-6 p-0 opacity-60 group-hover:opacity-100" />
                                                    </Tooltip>
                                                    <Tooltip title="删除该分镜剧本">
                                                        <Button size="small" type="text" danger icon={<Trash2 className="size-3" />} onClick={() => deleteStoryboard(sb.id)} className="h-6 w-6 p-0 opacity-60 group-hover:opacity-100" />
                                                    </Tooltip>
                                                  </div>
                                                  {/* 出场角色行：显示角色名 + 资产缩略图 */}
                                                  {headerInfo.characters.length > 0 && (
                                                    <div className="flex flex-wrap items-center gap-1 pl-0.5 text-[10px]">
                                                      <span className="text-stone-400 shrink-0">出场角色：</span>
                                                      {headerInfo.characters.map((name) => {
                                                        const asset = assets.find((a) => a.alias === name);
                                                        const linked = !!asset;
                                                        return (
                                                          <Tooltip key={name} title={linked ? `已关联素材「${name}」，生成视频会带其参考图` : `未关联：素材库没有别名为「${name}」的资产，生成视频不会带其参考图`}>
                                                            <span className={`flex items-center gap-1 rounded px-1 py-0.5 ${linked ? "bg-green-50 dark:bg-green-950/40" : "bg-red-50 dark:bg-red-950/40"}`}>
                                                              {linked && asset?.dataUrl && (
                                                                <img src={asset.dataUrl} alt={name} className="size-4 rounded object-cover" />
                                                              )}
                                                              <span className={`font-medium ${linked ? "text-green-600 dark:text-green-400" : "text-red-500"}`}>{name}</span>
                                                            </span>
                                                          </Tooltip>
                                                        );
                                                      })}
                                                    </div>
                                                  )}
                                                  {/* 场景行：显示场景名 + 资产缩略图 */}
                                                  {headerInfo.scenes.length > 0 && (
                                                    <div className="flex flex-wrap items-center gap-1 pl-0.5 text-[10px]">
                                                      <span className="text-stone-400 shrink-0">场景：</span>
                                                      {headerInfo.scenes.map((name) => {
                                                        const asset = assets.find((a) => a.alias === name);
                                                        const linked = !!asset;
                                                        return (
                                                          <Tooltip key={name} title={linked ? `已关联素材「${name}」，生成视频会带其参考图` : `未关联：素材库没有别名为「${name}」的资产，生成视频不会带其参考图`}>
                                                            <span className={`flex items-center gap-1 rounded px-1 py-0.5 ${linked ? "bg-blue-50 dark:bg-blue-950/40" : "bg-red-50 dark:bg-red-950/40"}`}>
                                                              {linked && asset?.dataUrl && (
                                                                <img src={asset.dataUrl} alt={name} className="size-4 rounded object-cover" />
                                                              )}
                                                              <span className={`font-medium ${linked ? "text-blue-600 dark:text-blue-400" : "text-red-500"}`}>{name}</span>
                                                            </span>
                                                          </Tooltip>
                                                        );
                                                      })}
                                                    </div>
                                                  )}
                                                  {/* 旧的 @标签行（保留兼容，仅当没有头部信息时显示） */}
                                                  {headerInfo.characters.length === 0 && headerInfo.scenes.length === 0 && assetLinks.length > 0 && (
                                                    <div className="flex flex-wrap items-center gap-1 pl-0.5">
                                                        {assetLinks.map((link) => (
                                                            <span key={link.name} className={`flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium ${link.linked ? "bg-green-50 text-green-600 dark:bg-green-950/40" : "bg-red-50 text-red-500 dark:bg-red-950/40"}`}>
                                                                {link.linked ? <UserCheck className="size-2.5" /> : <UserX className="size-2.5" />}{link.name}
                                                            </span>
                                                        ))}
                                                    </div>
                                                  )}
                                                </div>
                                            </div>
                                        );
                                    })}
                                    {/* 末尾新增按钮（搜索时隐藏） */}
                                    {!matchedStoryboardIds && (
                                        <div className="relative mt-1">
                                            <div className="flex items-center gap-1">
                                                <select
                                                    value={addStoryboardGroupIndex ?? -1}
                                                    onChange={(e) => setAddStoryboardGroupIndex(e.target.value === "-1" ? null : Number(e.target.value))}
                                                    className="rounded border border-stone-200 bg-white px-1.5 py-0.5 text-[10px] text-stone-500 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-400 outline-none"
                                                >
                                                    {groupCount > 0 ? (
                                                        <>
                                                            <option value={-1}>追加到最后</option>
                                                            {Array.from({ length: groupCount }, (_, i) => (
                                                                <option key={i} value={i}>第{i + 1}组</option>
                                                            ))}
                                                        </>
                                                    ) : (
                                                        <option value={-1}>追加到最后</option>
                                                    )}
                                                </select>
                                                <button
                                                    onClick={() => addStoryboard(addStoryboardGroupIndex)}
                                                    className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-dashed border-stone-200 py-1.5 text-[11px] text-stone-400 hover:border-stone-300 hover:text-stone-500 dark:border-stone-700 dark:hover:border-stone-600"
                                                >
                                                    <Plus className="size-3" />新增分镜剧本
                                                </button>
                                            </div>
                                        </div>
                                    )}
                                    {matchedStoryboardIds && matchedStoryboardIds.size === 0 && (
                                        <div className="flex flex-col items-center justify-center gap-1 py-8 text-center text-xs text-stone-400">
                                            <Search className="size-5 text-stone-300" />
                                            没有匹配「{storyboardSearch.trim()}」的分镜
                                        </div>
                                    )}
                                </div>
                            ) : (
                                <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed border-stone-200 dark:border-stone-800">
                                    <div className="text-center text-xs text-stone-400 px-4">
                                        点击中间「开始分镜」<br />每章一条，由分镜剧本提示词把整章整合为 1 条完整分镜剧本
                                    </div>
                                    <Popover
                                        title="选择插入位置"
                                        trigger="click"
                                        placement="top"
                                        content={
                                            <div className="w-40 space-y-1">
                                                <button
                                                    onClick={() => addStoryboard(null)}
                                                    className="w-full rounded px-2 py-1 text-left text-xs hover:bg-stone-100 dark:hover:bg-stone-800"
                                                >
                                                    追加到最后（第{groupCount}组）
                                                </button>
                                                {Array.from({ length: groupCount }, (_, i) => (
                                                    <button
                                                        key={i}
                                                        onClick={() => addStoryboard(i)}
                                                        className="w-full rounded px-2 py-1 text-left text-xs hover:bg-stone-100 dark:hover:bg-stone-800"
                                                    >
                                                        第{i + 1}组（第{Math.min(i * CHAPTERS_PER_GROUP + 1, chapters.length)}章）
                                                    </button>
                                                ))}
                                            </div>
                                        }
                                    >
                                        <Button size="small" icon={<Plus className="size-3" />}>手动新增分镜剧本</Button>
                                    </Popover>
                                </div>
                            )}
                        </div>

                        {/* 分镜视频结果（与分镜剧本五五开，各占一半高度） */}
                        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                            <div className="mb-2 flex items-center justify-between shrink-0">
                                <h2 className="text-sm font-medium flex items-center gap-2">
                                    <Video className="size-4" />
                                    分镜视频
                                    <span className="rounded-full bg-stone-100 px-2 py-0.5 text-[11px] text-stone-500 dark:bg-stone-800 dark:text-stone-400">{activeProject.shots.length} 镜{completedCount > 0 ? ` · ${completedCount} 已生成` : ""}</span>
                                </h2>
                                {activeProject.shots.length > 0 && (
                                    <div className="flex items-center gap-2">
                                        <Input
                                            allowClear size="small"
                                            prefix={<Search className="size-3 text-stone-400" />}
                                            placeholder="搜索"
                                            value={shotSearch}
                                            onChange={(e) => setShotSearch(e.target.value)}
                                            className="!h-6 w-24"
                                        />
                                        <label className="flex items-center gap-1 text-xs text-stone-500 cursor-pointer">
                                            <input type="checkbox" checked={activeProject.shots.every((s) => s.selected)} onChange={(e) => toggleAllSelect(e.target.checked)} className="accent-stone-600 size-3" />全选
                                        </label>
                                        <Tooltip title={selectedCount === 0 ? "先勾选分镜" : `生成选中的 ${selectedCount} 个`}>
                                            <Button size="small" icon={<Coins className="size-3" />} disabled={pipelineRunning || selectedCount === 0}
                                                onClick={() => {
                                                    if (!activeProject) return;
                                                    const ctrl = new AbortController();
                                                    activeProject.shots.filter((s) => s.selected).forEach((shot) => {
                                                        const idx = activeProject.shots.indexOf(shot);
                                                        generateShot(shot, activeProject.shots, idx, ctrl);
                                                    });
                                                }} />
                                        </Tooltip>
                                        {/* 批量删除勾选的分镜视频 */}
                                        <Tooltip title={selectedCount > 0 ? `删除选中的 ${selectedCount} 个分镜视频` : "先勾选分镜视频再批量删除"}>
                                            <Button size="small" type="text" danger icon={<Trash2 className="size-3" />} disabled={selectedCount === 0} onClick={batchDeleteShots} className="!h-6 w-6 p-0" />
                                        </Tooltip>
                                        <Tooltip title={selectedCount > 0 ? `下载勾选中已生成的视频` : "下载全部已生成视频"}>
                                            <Button size="small" icon={<Download className="size-3" />} disabled={completedCount === 0} onClick={() => void batchDownloadVideos()} />
                                        </Tooltip>
                                        <Tooltip title={videoCompact ? "标准卡片（带标题/操作）" : "紧凑缩略图（塞更多）"}>
                                            <Button size="small" type={videoCompact ? "primary" : "default"} icon={<LayoutGrid className="size-3" />}
                                                onClick={() => setVideoCompact((v) => !v)} />
                                        </Tooltip>
                                        <Tooltip title={videoLayout === "grid" ? "列表模式" : "网格模式"}>
                                            <Button size="small" icon={videoLayout === "grid" ? <FilmIcon className="size-3" /> : <LayoutGrid className="size-3" />}
                                                onClick={() => setVideoLayout((v) => v === "grid" ? "list" : "grid")} />
                                        </Tooltip>
                                        {videoLayout === "grid" && (
                                            <Select size="small" value={videoGridCol} onChange={setVideoGridCol} className="w-16"
                                                options={[{ label: "2列", value: 2 }, { label: "3列", value: 3 }, { label: "4列", value: 4 }, { label: "5列", value: 5 }, { label: "6列", value: 6 }]} />
                                        )}
                                    </div>
                                )}
                            </div>

                            {activeProject.shots.length > 0 ? (
                                <div ref={shotListRef} className={`thin-scrollbar overflow-y-auto ${videoLayout === "grid" ? (videoCompact ? "grid gap-1.5" : "grid gap-3") : "space-y-2"}`}
                                    style={videoLayout === "grid" ? { gridTemplateColumns: `repeat(${videoGridCol}, minmax(0, 1fr))` } : undefined}>
                                    {activeProject.shots.map((shot, index) => {
                                        // 优先用全局搜索结果，否则用独立视频搜索
                                        const matchedSet = globalSearch.trim() ? globalMatchedShotIds : matchedShotIds;
                                        if (matchedSet && !matchedSet.has(shot.id)) return null;
                                        return (
                                            <VideoShotCard
                                                key={shot.id}
                                                shot={shot} index={index}
                                                layout={videoLayout} compact={videoCompact && videoLayout === "grid"}
                                                onToggleSelect={() => toggleShotSelect(shot.id)}
                                                onGenerate={() => { const ctrl = new AbortController(); generateShot(shot, activeProject.shots, index, ctrl); }}
                                                onRetry={() => { const ctrl = new AbortController(); generateShot(shot, activeProject.shots, index, ctrl); }}
                                                onOpenDetail={() => setDetailShotId(shot.id)}
                                                onDelete={() => deleteShot(shot.id)}
                                            />
                                        );
                                    })}
                                    {matchedShotIds && matchedShotIds.size === 0 && (
                                        <div className="col-span-full flex flex-col items-center justify-center gap-1 py-8 text-center text-xs text-stone-400">
                                            <Search className="size-5 text-stone-300" />
                                            没有匹配「{shotSearch.trim()}」的分镜视频
                                        </div>
                                    )}
                                </div>
                            ) : (
                                <div className="flex min-h-0 flex-1 items-center justify-center rounded-xl border-2 border-dashed border-stone-200 dark:border-stone-800">
                                    <div className="text-center">
                                        <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-xl bg-stone-100 dark:bg-stone-800">
                                            <Video className="size-6 text-stone-400" />
                                        </div>
                                        <div className="text-xs text-stone-400">在上方分镜剧本点<Video className="inline size-3" />生成视频</div>
                                    </div>
                                </div>
                            )}
                        </div>
                    </div>
                )}
            </main>

            {/* ── Config Modal ── */}
            <Modal title="创作配置" open={showConfigModal} onCancel={() => setShowConfigModal(false)}
                footer={[<Button key="ok" type="primary" onClick={() => setShowConfigModal(false)}>完成</Button>]} width={480}>
                <div className="space-y-4 py-2">
                    <div className="flex items-center justify-between">
                        <div>
                            <ConfigLabel>分镜并发数</ConfigLabel>
                            <div className="mt-0.5 text-[11px] text-stone-400">勾选多组时同时改写的组数上限，跑完按组顺序归位；文本模型限流敏感，一般 3 个。</div>
                        </div>
                        <InputNumber min={1} max={10} value={storyboardConcurrency} onChange={(v) => setStoryboardConcurrency(v || 3)} addonAfter="组" className="w-28" />
                    </div>
                    <div className="flex items-center justify-between">
                        <div>
                            <ConfigLabel>视频并发数</ConfigLabel>
                            <div className="mt-0.5 text-[11px] text-stone-400">同时生成的视频数量上限，超出的排队；一般 5 个。</div>
                        </div>
                        <InputNumber min={1} max={20} value={videoConcurrency} onChange={(v) => setVideoConcurrency(v || 5)} addonAfter="个" className="w-28" />
                    </div>
                    <div className="flex items-center justify-between">
                        <div>
                            <ConfigLabel>资产生图并发数</ConfigLabel>
                            <div className="mt-0.5 text-[11px] text-stone-400">同时生成的资产图片数量上限。默认 3（保守，避免 rolldek/apimart 触限）；拉高可加速但可能 429。串行 7 张约 100s，并发 3 约 30-45s。</div>
                        </div>
                        <InputNumber min={1} max={8} value={imageConcurrency} onChange={(v) => setImageConcurrency(v || 3)} addonAfter="个" className="w-28" />
                    </div>
                    <div className="flex items-center justify-between">
                        <ConfigLabel>每段时长</ConfigLabel>
                        <InputNumber min={5} max={60} value={shotDuration} onChange={(v) => setShotDuration(v || 15)} addonAfter="秒" className="w-28" />
                    </div>
                    <div className="flex items-center justify-between">
                        <ConfigLabel>画面比例</ConfigLabel>
                        <Select value={aspectRatio} onChange={setAspectRatio} className="w-40"
                            options={[{ label: "横屏 16:9", value: "16:9" }, { label: "竖屏 9:16", value: "9:16" }, { label: "方形 1:1", value: "1:1" }, { label: "电影宽屏 21:9", value: "21:9" }]} />
                    </div>
                    <div className="flex items-center justify-between">
                        <ConfigLabel>分辨率</ConfigLabel>
                        <Select value={resolution} onChange={setResolution} className="w-40"
                            options={[{ label: "720p", value: "720p" }, { label: "1080p", value: "1080p" }]} />
                    </div>
                    {!generateInParallel && !supportsFrameRefs && (
                        <div className="flex items-center gap-1 text-[11px] text-stone-500"><AlertCircle className="size-3" />顺序生成强制衔接，但当前视频模型不支持首尾帧参考，衔接仅作为参考图使用</div>
                    )}
                    {!generateInParallel && (
                        <div className="flex items-center gap-1 text-[11px] text-stone-500"><AlertCircle className="size-3" />顺序生成强制衔接：上一条视频的尾帧作为下一条参考图（若上一条失败则跳过）</div>
                    )}
                    <div className="flex items-center justify-between">
                        <ConfigLabel>生成模式</ConfigLabel>
                        <Select value={generateInParallel ? "parallel" : "sequential"} onChange={(v) => setGenerateInParallel(v === "parallel")} className="w-40"
                            options={[{ label: "顺序生成", value: "sequential" }, { label: "并行生成", value: "parallel" }]} />
                    </div>
                </div>
            </Modal>

            {/* ── 使用说明 Modal ── */}
            <Modal title="使用说明" open={showGuideModal} onCancel={() => setShowGuideModal(false)}
                footer={[<Button key="ok" type="primary" onClick={() => setShowGuideModal(false)}>知道了</Button>]} width={640}>
                <div className="space-y-5 py-1 text-sm leading-relaxed">
                    <p className="text-stone-500">这个界面帮你把一段小说/剧本自动变成一系列短视频。整体走「剧本 → 分镜 → 图片 → 视频」的流水线，下面按区块和流程说明。</p>

                    <div>
                        <div className="mb-1.5 flex items-center gap-1.5 font-medium text-stone-800 dark:text-stone-100">
                            <span className="flex size-5 items-center justify-center rounded-full bg-stone-800 text-[11px] text-white dark:bg-stone-200 dark:text-stone-900">1</span>左侧：剧本项目
                        </div>
                        <p className="pl-6.5 text-stone-500">管理你的剧本文件。点「新建」创建空白剧本，或「导入」载入 .txt 小说/剧本。中间「原始小说」区域可直接查看/编辑原文，支持全选/清空要处理的章节。</p>
                    </div>

                    <div>
                        <div className="mb-1.5 flex items-center gap-1.5 font-medium text-stone-800 dark:text-stone-100">
                            <span className="flex size-5 items-center justify-center rounded-full bg-stone-800 text-[11px] text-white dark:bg-stone-200 dark:text-stone-900">2</span>中部：资产库（人物 / 场景 / 道具）
                        </div>
                        <p className="pl-6.5 text-stone-500">分镜生成后，系统会从剧本里自动抽取出角色、场景、道具。在这里点「生成图片」为它们批量生图，作为后续视频的固定视觉素材，保证同一角色在每段视频里长相一致。</p>
                    </div>

                    <div>
                        <div className="mb-1.5 flex items-center gap-1.5 font-medium text-stone-800 dark:text-stone-100">
                            <span className="flex size-5 items-center justify-center rounded-full bg-stone-800 text-[11px] text-white dark:bg-stone-200 dark:text-stone-900">3</span>右侧：分镜剧本 + 视频
                        </div>
                        <p className="pl-6.5 text-stone-500">顶部「开始分镜」按章节产出分镜剧本（一章一条，连续编号）；每条分镜对应生成一个视频片段。点「生成视频」逐条出片，或全部生成后在此预览、勾选下载。</p>
                    </div>

                    <div className="rounded-xl bg-stone-50 p-3 dark:bg-stone-800/50">
                        <div className="mb-2 font-medium text-stone-800 dark:text-stone-100">推荐流程</div>
                        <ol className="space-y-1.5 text-stone-500">
                            <li>① 新建或导入剧本，编辑原文并勾选要处理的章节。</li>
                            <li>② 打开右上角「一键出片」（开启后自动串起后续步骤），或保持关闭手动操作。</li>
                            <li>③ 点「开始分镜」，系统等逐章产出分镜剧本。</li>
                            <li>④ 在资产库为抽取出的角色/场景/道具「生成图片」。</li>
                            <li>⑤ 为每条分镜「生成视频」，顺序模式会自动用上一条的尾帧衔接，画面更连贯。</li>
                            <li>⑥ 在右侧预览视频，勾选需要的片段「下载」。</li>
                        </ol>
                    </div>

                    <div className="flex items-start gap-1.5 text-[12px] text-stone-400">
                        <HelpCircle className="mt-0.5 size-3.5 shrink-0" />
                        <span>「一键出片」开启时会自动完成 ③→④→⑤；关闭时你需要手动点「生成图片」「生成视频」。生成模式（顺序/并行）和时长、比例等可在「创作配置」里调整。</span>
                    </div>
                </div>
            </Modal>

            {/* ── 三合一提示词编辑 Modal ── */}
            <Modal title="提示词配置" open={showPromptModal} onCancel={() => setShowPromptModal(false)} width={760}
                footer={[
                    <Button key="reset" icon={<ClipboardPaste className="size-3 mr-1" />}
                        onClick={() => {
                            const defaults: Record<string, string> = { script: DEFAULT_SCRIPT_PROMPT, video: DEFAULT_VIDEO_PROMPT, image: DEFAULT_ASSET_PROMPT };
                            setPromptDraft(defaults[promptDraftTab]);
                            setCurrentPresetKey("general"); // P1 改造 A
                            antMessage.info("已重置为默认值");
                        }}><span>重置默认</span></Button>,
                    <Button key="cancel" onClick={() => setShowPromptModal(false)}>取消</Button>,
                    <Button key="save" type="primary" onClick={() => {
                        if (promptDraftTab === "script") setScriptPrompt(promptDraft);
                        else if (promptDraftTab === "video") setVideoPrompt(promptDraft);
                        else setAssetPrompt(promptDraft);
                        // P1 改造 A：保存时尝试匹配 preset
                        const matchedKey = matchPreset(promptDraft, promptDraftTab === "script" ? "scriptPrompt" : promptDraftTab === "video" ? "videoPrompt" : "assetPrompt");
                        setCurrentPresetKey(matchedKey ?? "custom");
                        setShowPromptModal(false);
                        antMessage.success("已保存提示词");
                    }}>保存</Button>,
                ]}>
                {/* P1 改造 A：4 套预设选择器 */}
                <div className="mb-3 flex flex-wrap items-center gap-2">
                    <span className="shrink-0 text-xs text-stone-500">预设：</span>
                    {PRESET_KEYS.map((key) => {
                        const preset = PRESETS[key];
                        const isActive = currentPresetKey === key;
                        return (
                            <button key={key} type="button"
                                onClick={() => {
                                    setCurrentPresetKey(key);
                                    const draftVal = getEffectivePresetValue(key, promptDraftTab === "script" ? "scriptPrompt" : promptDraftTab === "video" ? "videoPrompt" : "assetPrompt");
                                    setPromptDraft(draftVal);
                                }}
                                className={`rounded-lg px-3 py-1 text-xs font-medium transition ${
                                    isActive
                                        ? "bg-amber-100 text-amber-700 ring-1 ring-amber-300 dark:bg-amber-900/40 dark:text-amber-300 dark:ring-amber-700"
                                        : "bg-stone-100 text-stone-600 hover:bg-stone-200 dark:bg-stone-800 dark:text-stone-400 dark:hover:bg-stone-700"
                                }`}
                                title={preset.description}>
                                {preset.label}{isActive && <span className="ml-1 text-[10px]">✓</span>}
                            </button>
                        );
                    })}
                    {currentPresetKey === "custom" && (
                        <Tooltip title="当前 prompt 已被你手动改过，不属于任何预设">
                            <span className="rounded-lg bg-stone-200 px-2 py-1 text-[10px] text-stone-600 dark:bg-stone-700 dark:text-stone-400">自定义</span>
                        </Tooltip>
                    )}
                </div>
                {/* P1 改造 B/C：同步徽标 + 从后台重新拉取按钮 */}
                <div className="mb-3 flex flex-wrap items-center gap-2 text-xs">
                    {(() => {
                        const backendSnapshot = lastSyncedBackendRef.current;
                        const localForTab = promptDraftTab === "script" ? scriptPrompt : promptDraftTab === "video" ? videoPrompt : assetPrompt;
                        const backendForTab = promptDraftTab === "script" ? backendSnapshot.script : promptDraftTab === "video" ? backendSnapshot.video : backendSnapshot.asset;
                        const currentBackend = promptDraftTab === "script" ? effectiveConfig.systemPrompts?.storyboardScript?.trim() ?? ""
                            : promptDraftTab === "video" ? effectiveConfig.systemPrompts?.storyboardVideo?.trim() ?? ""
                            : effectiveConfig.systemPrompts?.storyboardImage?.trim() ?? "";
                        const backendChanged = !!currentBackend && currentBackend !== backendForTab;
                        if (backendChanged) {
                            return (
                                <Tooltip title="管理员在后台改了 prompt，点下方按钮拉取">
                                    <span className="flex items-center gap-1 rounded bg-amber-100 px-2 py-0.5 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
                                        ⟳ 后台有更新待拉取
                                    </span>
                                </Tooltip>
                            );
                        }
                        if (backendForTab && localForTab === backendForTab) {
                            return (
                                <Tooltip title="本地与后台一致">
                                    <span className="flex items-center gap-1 rounded bg-emerald-100 px-2 py-0.5 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">
                                        ✓ 已与后台同步
                                    </span>
                                </Tooltip>
                            );
                        }
                        return (
                            <Tooltip title="本地已自定义（与后台不同）">
                                <span className="flex items-center gap-1 rounded bg-stone-200 px-2 py-0.5 text-stone-600 dark:bg-stone-700 dark:text-stone-400">
                                    ✎ 本地已自定义
                                </span>
                            </Tooltip>
                        );
                    })()}
                    {(() => {
                        const curScript = effectiveConfig.systemPrompts?.storyboardScript?.trim() ?? "";
                        const curVideo = effectiveConfig.systemPrompts?.storyboardVideo?.trim() ?? "";
                        const curAsset = effectiveConfig.systemPrompts?.storyboardImage?.trim() ?? "";
                        const backendChanged = curScript !== lastSyncedBackendRef.current.script
                            || curVideo !== lastSyncedBackendRef.current.video
                            || curAsset !== lastSyncedBackendRef.current.asset;
                        if (!backendChanged) return null;
                        return (
                            <Button size="small" type="primary" icon={<RefreshCw className="size-3" />} onClick={() => {
                                if (!confirm("从后台拉取会用后台值覆盖当前 draft（不影响已保存的 state），继续？")) return;
                                if (promptDraftTab === "script" && curScript) setPromptDraft(curScript);
                                else if (promptDraftTab === "video" && curVideo) setPromptDraft(curVideo);
                                else if (promptDraftTab === "image" && curAsset) setPromptDraft(curAsset);
                                lastSyncedBackendRef.current = { script: curScript, video: curVideo, asset: curAsset };
                                antMessage.success("已从后台拉取最新 prompt");
                            }}>从后台重新拉取</Button>
                        );
                    })()}
                </div>
                {/* Tab 切换 */}
                <div className="flex gap-1 mb-3 border-b border-stone-200 dark:border-stone-700 pb-2">
                    {([
                        { key: "script", label: "分镜剧本提示词", desc: "控制小说章节→分镜剧本的改写风格" },
                        { key: "video", label: "分镜视频提示词", desc: "控制分镜剧本→视频描述词的改写风格" },
                        { key: "image", label: "分镜图片提示词", desc: "控制角色三视图/场景四宫格/道具图的生成风格" },
                    ] as const).map((tab) => (
                        <button key={tab.key} type="button" onClick={() => setPromptDraftTab(tab.key)}
                            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                                promptDraftTab === tab.key
                                    ? "bg-stone-800 text-white dark:bg-stone-200 dark:text-stone-900"
                                    : "bg-stone-100 text-stone-500 hover:bg-stone-200 dark:bg-stone-800 dark:text-stone-400 dark:hover:bg-stone-700"
                            }`}>
                            {tab.label}
                        </button>
                    ))}
                </div>
                <div className="mb-2 text-xs text-stone-400">
                    {promptDraftTab === "script" && "分镜剧本提示词：决定每章小说如何被改写为一条完整的分镜视频描述词（含时间段分层、@角色标注等）。"}
                    {promptDraftTab === "video" && "分镜视频提示词：决定分镜剧本如何被改写为可直接生成视频的镜头描述词（运镜、光影、节奏等细节）。"}
                    {promptDraftTab === "image" && "分镜图片提示词：决定角色三视图、场景四宫格、道具图的生成风格。文本模型会自动提取画风追加到此处。"}
                </div>
                {/* P1 改造 D：prompt 校验提示 */}
                {(() => {
                    const result = validatePrompt(promptDraftTab, promptDraft);
                    if (result.warnings.length === 0) {
                        return (
                            <div className="mb-1.5 flex items-center gap-1 text-[11px] text-emerald-600 dark:text-emerald-400">
                                <span>✓ 校验通过：常见约束齐了</span>
                                <span className="ml-2 text-stone-400">· {promptDraft.length} 字</span>
                            </div>
                        );
                    }
                    return (
                        <div className="mb-1.5 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-700 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
                            <div className="flex items-center justify-between">
                                <span className="font-medium">⚠ 校验提示（不强制，仅供参考）</span>
                                <span className="text-[10px] text-amber-600 dark:text-amber-400">{result.warnings.length} 项 · {promptDraft.length} 字</span>
                            </div>
                            <div className="mt-1 space-y-0.5">
                                {result.warnings.map((w, i) => (
                                    <div key={i}>{w}</div>
                                ))}
                            </div>
                        </div>
                    );
                })()}
                {/* P1 b4：📨 查看实际发送给 LLM 的完整消息（折叠预览） */}
                <details className="mb-1.5 rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 text-[11px] dark:border-stone-700 dark:bg-stone-800">
                    <summary className="cursor-pointer text-stone-500 hover:text-stone-700 dark:text-stone-400 dark:hover:text-stone-200">📨 查看实际发送给 LLM 的完整消息（system + user content，点击展开）</summary>
                    <div className="mt-2 space-y-2">
                        {/* System prompt */}
                        <div>
                            <div className="mb-1 flex items-center gap-1 text-stone-600 dark:text-stone-400">
                                <span className="rounded bg-stone-200 px-1.5 text-[10px] font-medium dark:bg-stone-700">SYSTEM</span>
                                <span>系统提示词（{promptDraft.length} 字）</span>
                            </div>
                            <pre className="thin-scrollbar max-h-48 overflow-auto whitespace-pre-wrap rounded bg-white px-2 py-1.5 text-[10.5px] leading-5 text-stone-700 dark:bg-stone-900/60 dark:text-stone-300">{promptDraft || "（空）"}</pre>
                        </div>
                        {/* User content 模板：复用 lib/prompts/storyboard 的 buildStoryboardUserContent，但章节正文用占位 */}
                        <div>
                            <div className="mb-1 flex items-center gap-1 text-stone-600 dark:text-stone-400">
                                <span className="rounded bg-stone-200 px-1.5 text-[10px] font-medium dark:bg-stone-700">USER</span>
                                <span>用户内容（动态章节正文 + 资产参考文档 · 与后端拼接逻辑一致）</span>
                            </div>
                            <pre className="thin-scrollbar max-h-48 overflow-auto whitespace-pre-wrap rounded bg-white px-2 py-1.5 text-[10.5px] leading-5 text-stone-700 dark:bg-stone-900/60 dark:text-stone-300">
{(() => {
                                    const sampleChapter = "第 1 章 · 摘要\n（这里是用户点开始分镜后被发给 LLM 的章节正文示例；实际内容随项目变化）";
                                    const assetRef = buildStoryboardAssetRefSection(
                                        assets.map((a) => ({ alias: a.alias || a.name, type: a.type, description: a.description || "", name: a.name }))
                                    );
                                    if (promptDraftTab === "script") {
                                        return `[角色/场景/道具参考文档]\n${assetRef || "（暂无资产）"}\n\n以下是小说第 1 ~ 1 章的正文，请你作为导演把这一整章剧情【整合成 1 条完整的分镜视频描述词】……\n\n${renderStoryboardPrompt(promptDraft, shotDuration).slice(0, 200)}……\n\n${sampleChapter}`;
                                    } else if (promptDraftTab === "video") {
                                        return `[上一段视频描述词]\n[角色/场景/道具参考文档]\n${assetRef || "（暂无资产）"}\n\n[单镜分镜剧本]\n（{出场角色} + {场景} + {0-Xs｜ …}）`;
                                    } else {
                                        return "[图片生成 system prompt 用于角色三视图 / 场景四宫格 / 道具标准图；user content 由 extractDescriptionsForAssets 动态拼装，正文 + 角色场景道具名 + 类型]\n\n[资产名列表]\n[章节正文前 800 字]";
                                    }
                                })()}
                            </pre>
                        </div>
                    </div>
                </details>
                <textarea value={promptDraft} onChange={(e) => {
                    setPromptDraft(e.target.value);
                    // P1 改造 A：用户改 draft 后立即取消预设高亮（标记为 custom）
                    if (currentPresetKey !== "custom") setCurrentPresetKey("custom");
                }}
                    className="thin-scrollbar h-72 w-full resize-none rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 text-sm leading-6 outline-none dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200" />
            </Modal>

            {/* ── 缺参考图提示 Modal（持久化：generateVideoFromStoryboard 触发，关闭即清） ── */}
            <Modal
                title={<span className="flex items-center gap-2 text-amber-700 dark:text-amber-400"><AlertTriangle className="size-4" />视频将丢 {pendingShotRun?.unmatched.length ?? 0} 个参考图</span>}
                open={!!pendingShotRun}
                onCancel={() => { setPendingShotRun(null); pendingShotRun?.onSettled?.(false); }}
                width={520}
                footer={[
                    <Button key="upload" icon={<Upload className="size-3" />} onClick={() => {
                        // 跳到素材库 + 自动展开角色 tab + 展开提取清单，让用户上传 + 改名
                        const missingName = pendingShotRun?.unmatched[0];
                        setAssetTab("character");
                        setExtractedListExpanded(true);
                        setPendingShotRun(null);
                        pendingShotRun?.onSettled?.(false);
                        if (missingName) antMessage.info(`请上传「${missingName}」等角色的图片，并把「@别名」设为与分镜剧本一致`);
                    }}>去上传素材</Button>,
                    <Button key="continue" type="primary" onClick={() => {
                        if (!pendingShotRun) return;
                        const { storyboardId, linkedIds, onSettled } = pendingShotRun;
                        proceedGenerateVideo(storyboardId, linkedIds, onSettled);
                        setPendingShotRun(null);
                    }}>仍然继续生成</Button>,
                ]}>
                {pendingShotRun && (
                    <div className="space-y-3 text-sm">
                        <div>
                            分镜剧本里引用了这些「出场角色」，但素材库没有匹配别名的资产。视频生成时会丢其参考图，
                            画面可能与剧本描述不一致。
                        </div>
                        <div>
                            <div className="mb-1.5 text-xs font-medium text-stone-500">未关联的角色</div>
                            <div className="flex flex-wrap gap-1.5">
                                {pendingShotRun.unmatched.map((m) => (
                                    <span key={m} className="flex items-center gap-1 rounded bg-red-50 px-2 py-0.5 text-xs text-red-600 dark:bg-red-950/40 dark:text-red-400">
                                        <UserX className="size-3" />{m}
                                    </span>
                                ))}
                            </div>
                        </div>
                        <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
                            💡 推荐：先去素材库上传这些角色的图片，并把「@别名」设为与分镜剧本完全一致，再重新生成。
                        </div>
                    </div>
                )}
            </Modal>

            {/* ── P1 改造 A：分镜剧本预览闸 Modal（autoPilot 时跑分镜完即弹） ── */}
            <Modal
                title={<span className="flex items-center gap-2"><Clapperboard className="size-4 text-stone-600" />预览分镜剧本（{pendingStoryboardPreview?.entries.length ?? 0} 条）</span>}
                open={!!pendingStoryboardPreview}
                onCancel={() => { setPendingStoryboardPreview(null); antMessage.info("已关闭预览，自动链中断"); }}
                width={820}
                footer={[
                    <Button key="skip" onClick={handleSkipStoryboardPreview}>全部跳过</Button>,
                    <Button key="confirm" type="primary" onClick={() => void handleConfirmStoryboardPreview()}>确认继续 → 抽资产</Button>,
                ]}>
                {pendingStoryboardPreview && (
                    <div className="space-y-3">
                        <div className="rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 text-xs text-stone-600 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-300">
                            文本模型把每章改写为 1 条分镜剧本。下方每条可点「编辑 / 重新生成该条 / 跳过」。点「确认继续」才会进入资产抽描述阶段。
                            <span className="ml-2 text-stone-500">（LLM 改写可能丢对话/误改外貌，这一步是堵这洞的关键）</span>
                        </div>
                        <div className="thin-scrollbar max-h-[60vh] space-y-2 overflow-y-auto pr-1">
                            {pendingStoryboardPreview.entries.map((e, idx) => {
                                const sb = e.snapshot;
                                const header = extractStoryboardHeader(sb.content);
                                const preview = sb.content.replace(/^\s*【?\s*(?:场景|分镜|镜头|视频|Shot|Scene)\s*\d+\s*】?\s*[:：]?\s*/i, "").split(/\n/)[0]?.trim() || "（空白分镜，点击编辑）";
                                return (
                                    <div key={e.boardId} className="rounded-lg border border-stone-200 bg-white px-3 py-2 dark:border-stone-700 dark:bg-stone-900/50">
                                        <div className="flex items-center gap-2">
                                            <span className="shrink-0 rounded bg-stone-200 px-1.5 py-0.5 text-[10px] font-medium text-stone-600 dark:bg-stone-700 dark:text-stone-300">分镜 {idx + 1}</span>
                                            <span className="min-w-0 flex-1 truncate text-[11px] text-stone-500 dark:text-stone-400" title={preview}>{preview}</span>
                                            <Button size="small" type="text" icon={<Pencil className="size-3" />} onClick={() => {
                                                const draftText = window.prompt(`编辑分镜 ${idx + 1} 的剧本（直接修改下方文本）：`, sb.content);
                                                if (draftText != null && draftText !== sb.content) {
                                                    setPendingStoryboardPreview((prev) => prev ? {
                                                        ...prev,
                                                        entries: prev.entries.map((x) => x.boardId === e.boardId ? {
                                                            ...x,
                                                            snapshot: { ...x.snapshot, content: draftText },
                                                        } : x),
                                                    } : prev);
                                                    antMessage.success("已编辑该分镜剧本");
                                                }
                                            }} className="h-6 px-2 text-[11px]">编辑</Button>
                                            <Button size="small" type="text" icon={<RefreshCw className="size-3" />} onClick={async () => {
                                                // 重新生成该条：把 snapshot 临时同步到 activeProject，调 regenerateStoryboard(id)，再写回 snapshot
                                                if (!activeProject) return;
                                                const savedBoards = activeProject.storyboards ?? [];
                                                updateProject((p) => ({
                                                    ...p,
                                                    storyboards: savedBoards.concat([{ ...sb, id: e.boardId, content: e.snapshot.content }]),
                                                    updatedAt: Date.now(),
                                                }));
                                                await regenerateStoryboard(e.boardId);
                                                // 拿最新 board 写回 snapshot（用 projectsRef 读最新 state，避免闭包过期）
                                                const proj = projectsRef.current.find((p) => p.id === activeProjectId);
                                                const updated = (proj?.storyboards ?? []).find((x) => x.id === e.boardId);
                                                setPendingStoryboardPreview((prev) => prev ? {
                                                    ...prev,
                                                    entries: prev.entries.map((x) => x.boardId === e.boardId ? {
                                                        ...x,
                                                        snapshot: updated ?? x.snapshot,
                                                    } : x),
                                                } : prev);
                                            }} className="h-6 px-2 text-[11px]">重新生成</Button>
                                            <Button size="small" type="text" danger icon={<Trash2 className="size-3" />} onClick={() => {
                                                setPendingStoryboardPreview((prev) => prev ? {
                                                    ...prev,
                                                    entries: prev.entries.map((x) => x.boardId === e.boardId ? {
                                                        ...x,
                                                        snapshot: { ...x.snapshot, content: "⚠ 跳过该分镜剧本" },
                                                    } : x),
                                                } : prev);
                                                antMessage.info("已标记跳过该分镜剧本（点确认会忽略它）");
                                            }} className="h-6 px-2 text-[11px]">跳过</Button>
                                        </div>
                                        {header.characters.length > 0 && (
                                            <div className="mt-1 flex flex-wrap items-center gap-1 pl-1 text-[10px]">
                                                <span className="text-stone-400">出场角色：</span>
                                                {header.characters.map((m) => (
                                                    <span key={m} className="rounded bg-stone-100 px-1 text-stone-600 dark:bg-stone-800 dark:text-stone-300">@{m}</span>
                                                ))}
                                            </div>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    </div>
                )}
            </Modal>

            {/* ── 自动链 preview 闸 Modal：分镜剧本完成后弹，让用户预览/编辑/删除要生成的资产清单 ── */}
            <Modal
                title={<span className="flex items-center gap-2"><Camera className="size-4 text-stone-600" />预览要生成的资产（{extractedAssets.length} 个）</span>}
                open={!!pendingAssetPreview}
                onCancel={() => { setPendingAssetPreview(null); antMessage.info("已取消预览，自动链中断（视频未生成）"); }}
                width={720}
                footer={[
                    // P1 改造 C：「跳过」藏到「高级」Popover，避免误触丢全部参考图
                    <Popover
                        key="advanced"
                        trigger="click"
                        placement="topRight"
                        content={
                            <div className="w-60 space-y-2 p-1">
                                <div className="text-[11px] font-medium text-stone-600 dark:text-stone-300">高级选项（不推荐）</div>
                                <Button danger block size="small" onClick={() => {
                                    const count = extractedAssets.length;
                                    if (!confirm(`跳过资产生成意味着：\n\n· 所有 ${count} 条角色/场景/道具图都不会生成\n· 视频生成时这些角色都不会带参考图\n· 视频画面可能与剧本描述不一致（人物对不上）\n\n确认跳过？`)) return;
                                    handleSkipAssetPreview();
                                }}>跳过资产生成，直接出片</Button>
                            </div>
                        }>
                        <Button key="advanced-btn">高级 ▾</Button>
                    </Popover>,
                    <Button key="confirm" type="primary" onClick={() => void handleConfirmAssetPreview()}>确认生成 {extractedAssets.length} 个资产</Button>,
                ]}>
                <div className="space-y-3">
                    <div className="rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 text-xs text-stone-600 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-300">
                        一键出片已自动从 {pendingAssetPreview?.boards.length ?? 0} 条分镜剧本里抽出资产清单。
                        点「确认生成」开始按图片模型生图；想跳过看「高级」。
                        <span className="ml-2 text-stone-500">（每条可编辑名称 / 描述 / 重新抽取 / 删除；点 🔁 一键重试上次失败的）</span>
                    </div>
                    {/* P1 改造 a4：顶部「本次生图会用」信息条 */}
                    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 text-[11px] text-stone-600 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-300">
                        <span className="font-medium">本次生图会用：</span>
                        <span className="rounded bg-white px-1.5 py-0.5 font-mono text-stone-700 dark:bg-stone-700 dark:text-stone-200" title="资产生图模型（assetImageModel 或全局 imageModel）">
                            🖼 {previewImageModel || "未配置"}
                        </span>
                        <span className="rounded bg-white px-1.5 py-0.5 font-mono text-stone-700 dark:bg-stone-700 dark:text-stone-200" title="图片通道">
                            📡 {previewImageChannel || "默认"}
                        </span>
                        <span className={`rounded px-1.5 py-0.5 ${hasStyleHint ? "bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300" : "bg-white text-stone-500 dark:bg-stone-700"}`} title="通用风格后缀（追加到所有角色/场景/道具）">
                            🎨 风格后缀：{hasStyleHint ? assetImageStylePrompt : "无"}
                        </span>
                    </div>
                    {/* P1 改造 a5：跨章同名资产合并 summary —— 让用户看到「去重」节省了多少次生图 */}
                    {dedupeStats && dedupeStats.totalMentions > 0 && (
                        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-[11px] text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-300">
                            <span>♻️ 跨章同名已合并：</span>
                            <span className="font-medium">{dedupeStats.totalMentions} 次 @引用 → {dedupeStats.distinctAssets} 个独立资产</span>
                            {dedupeStats.saved > 0 && (
                                <span className="rounded bg-emerald-100 px-1.5 py-0.5 dark:bg-emerald-900/40">
                                    节省 {dedupeStats.saved} 次生图
                                </span>
                            )}
                            <span className="text-emerald-600/70 dark:text-emerald-400/70">（同角色/同场景跨章只生成一次）</span>
                        </div>
                    )}
                    {/* P1 改造 B-2：failedAssets 一键重试入口 */}
                    {failedAssets.length > 0 && (
                        <div className="flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs dark:border-amber-800 dark:bg-amber-950/30">
                            <span className="text-amber-700 dark:text-amber-300">⚠ 上次有 {failedAssets.length} 个资产生图失败：{failedAssets.slice(0, 3).map((f) => f.name).join("、")}{failedAssets.length > 3 ? "..." : ""}</span>
                            <Button size="small" type="primary" icon={<RefreshCw className="size-3" />} onClick={() => void handleRetryFailedAssets()}>🔁 一键重试</Button>
                        </div>
                    )}
                    {/* 角色 / 场景 / 道具 三段式表格：可点开编辑、删除 */}
                    {(["character", "scene", "prop"] as AssetType[]).map((type) => {
                        const items = extractedAssets.filter((a) => a.type === type);
                        if (items.length === 0) return null;
                        const typeLabel = type === "character" ? "角色" : type === "scene" ? "场景" : "道具";
                        const typeColor = type === "character" ? "bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300"
                            : type === "scene" ? "bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
                            : "bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300";
                        return (
                            <div key={type} className="rounded-lg border border-stone-200 bg-white dark:border-stone-700 dark:bg-stone-900/40">
                                <div className="border-b border-stone-200 px-3 py-1.5 text-xs font-medium text-stone-700 dark:border-stone-700 dark:text-stone-200">
                                    {typeLabel} <span className="ml-1 rounded bg-stone-100 px-1.5 text-[10px] text-stone-500 dark:bg-stone-800 dark:text-stone-400">{items.length}</span>
                                </div>
                                <div className="divide-y divide-stone-100 dark:divide-stone-800">
                                    {items.map((a) => {
                                        // P1 改造 a1：完整 prompt = 系统模板 + description + 风格后缀
                                        const fullPrompt = buildAssetPrompt({ name: a.name, type: a.type, description: a.description });
                                        return (
                                        <div key={a.id} className="flex flex-col gap-1.5 px-3 py-2 text-xs">
                                            <div className="flex items-start gap-2">
                                                <span className={`mt-0.5 shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${typeColor}`}>{typeLabel}</span>
                                                <div className="min-w-0 flex-1">
                                                    <div className="flex items-center gap-1.5">
                                                    <div className="font-medium text-stone-700 dark:text-stone-200">{a.name}</div>
                                                    {/* P1 改造 b3：同名资产已存在 chip —— 提示用户「保留现有 vs 重新生图」 */}
                                                    {a.isExisting && (
                                                        <Tooltip title="同名资产已在素材库中。点✕删除本条 = 复用现有（省图模型额度）；否则确认生成 = 用新 description 重新生图覆盖。">
                                                            <span className="flex items-center gap-0.5 rounded bg-amber-100 px-1 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">
                                                                ♻️ 已存在
                                                            </span>
                                                        </Tooltip>
                                                    )}
                                                </div>
                                                    <div className="text-[11px] text-stone-500 dark:text-stone-400">{a.description}</div>
                                                </div>
                                                {/* P1 改造 a1：🔍 查看完整 prompt（系统模板+描述+风格后缀） */}
                                                <Tooltip title="查看「系统模板 + 描述 + 风格后缀」拼成的实际发到图片模型的完整 prompt">
                                                    <Button size="small" type="text" icon={<FileText className="size-3" />} onClick={() => {
                                                        // 复用现有 viewAssetId 机制：拷贝 state + 打开 Modal
                                                        setAssetDraftName(a.name);
                                                        setAssetDraftDescription(a.description);
                                                        setAssetDraftType(a.type);
                                                        setViewAssetId(a.id);
                                                    }} className="h-6 w-6 p-0" />
                                                </Tooltip>
                                                {/* P1 改造 a2：📜 查看抽取来源（章节里哪段话） */}
                                                {a.sourceSnippet && (
                                                    <Tooltip title={`来源：分镜剧本正文前 800 字（点击展开）`}>
                                                        <Button size="small" type="text" icon={<ScrollText className="size-3" />} onClick={() => {
                                                            // 复用 viewAssetId 让 Modal 同步打开；用临时滚动条
                                                            setAssetDraftName(a.name);
                                                            setAssetDraftDescription(a.description);
                                                            setAssetDraftType(a.type);
                                                            setViewAssetId(a.id);
                                                        }} className="h-6 w-6 p-0" />
                                                    </Tooltip>
                                                )}
                                                {/* P1 改造 B-1：单条重新抽取描述按钮 */}
                                                <Tooltip title="用文本模型重新抽这条的描述（不消耗图片额度）">
                                                    <Button size="small" type="text" icon={<RefreshCw className="size-3" />} onClick={() => void handleReextractAssetDescription(a.id)} className="h-6 w-6 p-0" />
                                                </Tooltip>
                                                <Button size="small" type="text" danger icon={<X className="size-3" />} onClick={() => setExtractedAssets((prev) => prev.filter((x) => x.id !== a.id))} className="h-6 w-6 p-0" />
                                            </div>
                                            {/* a2：snippet 折叠显示（默认隐藏，点 📜 才展开） */}
                                            {a.sourceSnippet && (
                                                <details className="ml-1 text-[10px]">
                                                    <summary className="cursor-pointer text-stone-400 hover:text-stone-600 dark:hover:text-stone-300">📜 来源（分镜正文前 800 字）</summary>
                                                    <pre className="thin-scrollbar mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-stone-50 px-2 py-1.5 text-[11px] leading-5 text-stone-600 dark:bg-stone-800/50 dark:text-stone-300">{a.sourceSnippet}</pre>
                                                </details>
                                            )}
                                        </div>
                                        );
                                    })}
                                </div>
                            </div>
                        );
                    })}
                    {extractedAssets.length === 0 && (
                        <div className="rounded-lg border border-dashed border-stone-200 px-3 py-6 text-center text-xs text-stone-400 dark:border-stone-700">
                            资产清单为空。点「跳过资产生成」直接出片，或点「取消」中断自动链。
                        </div>
                    )}
                </div>
            </Modal>

            {/* ── Asset Rename Modal ── */}
            <Modal title="编辑素材" open={!!editingAssetId} onCancel={() => setEditingAssetId(null)} width={400}
                footer={[
                    <Button key="cancel" onClick={() => setEditingAssetId(null)}>取消</Button>,
                    <Button key="save" type="primary" onClick={() => editingAssetId && updateAssetMeta(editingAssetId, editAssetName, editAssetAlias, editAssetDesc)}>保存</Button>,
                ]}>
                <div className="space-y-3 py-2">
                    <div>
                        <div className="mb-1 text-xs text-stone-500">名称</div>
                        <Input value={editAssetName} onChange={(e) => setEditAssetName(e.target.value)} placeholder="素材名称" />
                    </div>
                    <div>
                        <div className="mb-1 text-xs text-stone-500">@别名（剧本引用）</div>
                        <Input value={editAssetAlias} onChange={(e) => setEditAssetAlias(e.target.value)} placeholder="@别名" />
                    </div>
                    <div>
                        <div className="mb-1 text-xs text-stone-500">描述（影响生成提示词）</div>
                        <Input value={editAssetDesc} onChange={(e) => setEditAssetDesc(e.target.value)} placeholder="描述资产外观特征" />
                    </div>
                </div>
            </Modal>

            {/* ── Video Detail Modal ── */}
            {detailShot && (
                <Modal title={detailShot.title} open={!!detailShotId} onCancel={() => setDetailShotId(null)} footer={null} width={720}>
                    <VideoShotDetail
                        shot={detailShot} index={detailShotIndex} total={activeProject?.shots.length || 0}
                        assets={assets} supportsFrameRefs={supportsFrameRefs}
                        curAspectRatio={aspectRatio} curResolution={resolution}
                        curSize={aspectRatio === "16:9" ? "1280x720" : aspectRatio === "9:16" ? "720x1280" : aspectRatio === "21:9" ? "1920x720" : "1024x1024"}
                        curVideoModel={videoModel}
                        videoSystemPrompt={videoDetailPrompts.videoSystemPrompt}
                        resolvedScript={videoDetailPrompts.resolvedScript}
                        onGenerate={() => { if (!activeProject) return; const ctrl = new AbortController(); generateShot(detailShot, activeProject.shots, detailShotIndex, ctrl); }}
                        onRetry={() => { if (!activeProject) return; const ctrl = new AbortController(); generateShot(detailShot, activeProject.shots, detailShotIndex, ctrl); }}
                        onDelete={() => deleteShot(detailShot.id)}
                        onMoveUp={() => detailShotIndex > 0 && moveShot(detailShotIndex, detailShotIndex - 1)}
                        onMoveDown={() => activeProject && detailShotIndex < activeProject.shots.length - 1 && moveShot(detailShotIndex, detailShotIndex + 1)}
                        onPrev={() => { if (detailShotIndex > 0 && activeProject) setDetailShotId(activeProject.shots[detailShotIndex - 1].id); }}
                        onNext={() => { if (activeProject && detailShotIndex < activeProject.shots.length - 1) setDetailShotId(activeProject.shots[detailShotIndex + 1].id); }}
                        onDurationChange={(d) => updateShotDuration(detailShot.id, d)}
                        onPromptChange={(pr) => updateShotPrompt(detailShot.id, pr)}
                        onFirstFrameAsset={(id) => updateShotFrame(detailShot.id, "firstFrameAssetId", id)}
                        onLastFrameAsset={(id) => updateShotFrame(detailShot.id, "lastFrameAssetId", id)}
                        onSaveFrameAsAsset={(type) => { if (detailShot.videoUrl) void saveFrameAsAsset(detailShot.videoUrl, detailShot.id, detailShot.title, type, detailShot.videoStorageKey); }}
                        onCaptureCurrentFrame={(seekSeconds) => { if (detailShot.videoUrl) void captureCurrentFrameAndSave(detailShot.videoUrl, detailShot.id, detailShot.title, detailShot.videoStorageKey, seekSeconds); }}
                        onExtractAudio={() => { if (detailShot.videoUrl) void extractAudioAndSave(detailShot.videoUrl, detailShot.title, detailShot.videoStorageKey); }}
                        onDownload={() => { if (detailShot.videoUrl) void downloadVideo(detailShot.videoUrl, detailShot.title, detailShot.videoStorageKey); }}
                        onCopyPrompt={() => { navigator.clipboard.writeText(detailShot.customPrompt || videoDetailPrompts.videoSystemPrompt + "\n\n" + videoDetailPrompts.resolvedScript); antMessage.success("已复制完整提示词"); }}
                    />
                </Modal>
            )}

            {/* Rewrite Result Modal */}
            {showRewriteResult && rewrittenScript && (
                <Modal title="剧本改写结果" open={showRewriteResult} onCancel={() => setShowRewriteResult(false)} footer={[
                    <Button key="apply" type="primary" onClick={() => {
                        if (!activeProject) return;
                        const shots = parseScriptToShots(rewrittenScript, shotDuration);
                        updateProject((p) => ({ ...p, script: rewrittenScript, shots, updatedAt: Date.now() }));
                        setShowRewriteResult(false);
                        antMessage.success("已应用改写结果");
                    }}>应用并重写</Button>,
                    <Button key="keep" onClick={() => setShowRewriteResult(false)}>保持原剧本</Button>,
                ]} width={700}>
                    <div className="max-h-96 overflow-y-auto rounded-lg border border-stone-200 bg-stone-50 p-4 dark:border-stone-700 dark:bg-stone-800">
                        <pre className="whitespace-pre-wrap text-sm text-stone-700 dark:text-stone-300">{rewrittenScript}</pre>
                    </div>
                </Modal>
            )}

            {/* Scene Check Modal */}
            {sceneCheckResult && (
                <Modal title="场景资产检查" open={!!sceneCheckResult} onCancel={() => setSceneCheckResult(null)} footer={null} width={480}>
                    <div className="space-y-3 max-h-80 overflow-y-auto">
                        {sceneCheckResult.matched.length > 0 && (
                            <div>
                                <div className="mb-1.5 text-xs font-medium text-stone-600 dark:text-stone-400">已匹配</div>
                                {sceneCheckResult.matched.map((m) => (<div key={m} className="mb-0.5 text-xs text-stone-500 dark:text-stone-400">{m}</div>))}
                            </div>
                        )}
                        {sceneCheckResult.missing.length > 0 && (
                            <div>
                                <div className="mb-1.5 text-xs font-medium text-red-600 dark:text-red-400">缺少资产</div>
                                {sceneCheckResult.missing.map((m) => (<div key={m} className="mb-0.5 text-xs text-stone-500 dark:text-stone-400">{m}</div>))}
                            </div>
                        )}
                    </div>
                </Modal>
            )}

            {/* 查看/编辑分镜剧本弹窗（右侧点某条分镜） */}
            {viewStoryboard && (
                <Modal title={`分镜${storyboards.findIndex((s) => s.id === viewStoryboard.id) + 1}（第 ${viewStoryboard.groupIndex + 1} 组）`} open={!!viewStoryboardId}
                    onCancel={() => setViewStoryboardId(null)} width={720}
                    footer={[
                        <Button key="del" danger icon={<Trash2 className="size-3" />} onClick={() => { const id = viewStoryboard.id; setViewStoryboardId(null); deleteStoryboard(id); }}>删除</Button>,
                        <Button key="copy" icon={<Copy className="size-3" />} onClick={() => { navigator.clipboard.writeText(storyboardDraft); antMessage.success("已复制分镜剧本"); }}>复制</Button>,
                        <Button key="regen" icon={<RefreshCw className="size-3" />} onClick={() => { const id = viewStoryboard.id; setViewStoryboardId(null); void regenerateStoryboard(id); }}>重新生成</Button>,
                        <Button key="save" onClick={() => { saveStoryboardContent(viewStoryboard.id, storyboardDraft); antMessage.success("已保存"); }}>保存</Button>,
                        <Button key="gen" type="primary" icon={<Video className="size-3" />} onClick={() => { const id = viewStoryboard.id; saveStoryboardContent(id, storyboardDraft); setViewStoryboardId(null); generateVideoFromStoryboard(id); }}>生成视频</Button>,
                    ]}>
                    <div className="flex items-center gap-2 mb-2">
                        <span className="text-xs text-stone-400 shrink-0">所在分组：</span>
                        <Select size="small" value={storyboardGroupDraft} onChange={(v) => {
                            setStoryboardGroupDraft(v);
                            moveStoryboardToGroup(viewStoryboard.id, v);
                        }} className="w-32"
                            options={Array.from({ length: groupCount }, (_, i) => ({ label: `第${i + 1}组`, value: i }))}
                        />
                        <span className="text-xs text-stone-400">
                            （一个分镜剧本 ≈ 一个约 {shotDuration}s 的视频镜头）
                        </span>
                    </div>
                    {/* P2 c1：@ 别名助手 —— 点击已有 alias chip 自动追加到 textarea 末尾，避免手敲拼错 */}
                    {assets.filter((a) => a.alias).length > 0 && (
                        <div className="mb-2 flex flex-wrap items-center gap-1">
                            <span className="text-[11px] text-stone-400">📛 点 alias 追加到分镜：</span>
                            {assets.filter((a) => a.alias).map((a) => (
                                <button key={a.id} type="button"
                                    onClick={() => {
                                        // 末尾追加「@alias」（空行自动补换行）；自动包进 @ → 自动关联角色资产，无需手敲
                                        const sep = storyboardDraft.length === 0 || storyboardDraft.endsWith("\n") ? "" : "\n";
                                        setStoryboardDraft((prev) => prev + sep + `@${a.alias}`);
                                    }}
                                    className="rounded bg-purple-50 px-1.5 py-0.5 text-[11px] text-purple-700 hover:bg-purple-100 dark:bg-purple-950/40 dark:text-purple-300 dark:hover:bg-purple-900/60"
                                    title={`@${a.alias}（${a.type === "character" ? "角色" : a.type === "scene" ? "场景" : "道具"}）`}>
                                    @{a.alias}
                                </button>
                            ))}
                        </div>
                    )}
                    <textarea value={storyboardDraft} onChange={(e) => setStoryboardDraft(e.target.value)}
                        className="thin-scrollbar h-80 w-full resize-none rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 text-sm leading-6 outline-none dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200" />
                </Modal>
            )}

            {/* 查看/编辑已提取资产弹窗（点卡片查看完整 prompt：系统模板 + 描述 + 画风后缀） */}
            {viewAsset && (() => {
                // 完整提示词预览：系统模板前缀 + 描述 + 画风后缀（与生图模型实际收到的 prompt 一致）。
                // 在弹窗渲染时实时拼装，与 generateImagesFromExtracted 行为完全一致。
                const fullPrompt = buildAssetPrompt(
                    { name: assetDraftName, type: assetDraftType, description: assetDraftDescription },
                    extractedStyle,
                );
                return (
                <Modal
                    title={`${viewAsset.type === "character" ? "角色" : viewAsset.type === "scene" ? "场景" : "道具"}：${viewAsset.name || "（未命名）"}`}
                    open={!!viewAssetId}
                    onCancel={() => setViewAssetId(null)}
                    width={720}
                    footer={[
                        <Button key="del" danger icon={<Trash2 className="size-3" />} onClick={() => {
                            const id = viewAsset.id;
                            setViewAssetId(null);
                            setExtractedAssets((prev) => prev.filter((x) => x.id !== id));
                        }}>删除</Button>,
                        <Button key="copy" icon={<Copy className="size-3" />} onClick={() => {
                            navigator.clipboard.writeText(fullPrompt);
                            antMessage.success("已复制完整提示词");
                        }}>复制完整提示词</Button>,
                        <Button key="save" type="primary" onClick={() => {
                            const id = viewAsset.id;
                            setExtractedAssets((prev) => prev.map((x) => x.id === id ? { ...x, name: assetDraftName.trim(), description: assetDraftDescription, type: assetDraftType } : x));
                            antMessage.success("已保存");
                            setViewAssetId(null);
                        }}>保存</Button>,
                    ]}
                >
                    <div className="space-y-3">
                        <div className="grid grid-cols-[80px_1fr] items-center gap-2">
                            <span className="text-xs text-stone-500">名称</span>
                            <Input
                                size="small"
                                value={assetDraftName}
                                onChange={(e) => setAssetDraftName(e.target.value)}
                                placeholder="例如：段嘉怡"
                            />
                        </div>
                        <div className="grid grid-cols-[80px_1fr] items-center gap-2">
                            <span className="text-xs text-stone-500">类型</span>
                            <Select
                                size="small"
                                value={assetDraftType}
                                onChange={(v) => setAssetDraftType(v)}
                                className="w-32"
                                options={[
                                    { label: "角色", value: "character" },
                                    { label: "场景", value: "scene" },
                                    { label: "道具", value: "prop" },
                                ]}
                            />
                        </div>
                        <div className="grid grid-cols-[80px_1fr] items-start gap-2">
                            <span className="text-xs text-stone-500">描述</span>
                            <textarea
                                value={assetDraftDescription}
                                onChange={(e) => setAssetDraftDescription(e.target.value)}
                                rows={4}
                                placeholder="女，十七岁，长发披肩，面容小巧灵动，常穿简约连衣裙，事故后被白布覆盖"
                                className="thin-scrollbar w-full resize-none rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 text-sm leading-6 outline-none dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200"
                            />
                        </div>
                        {/* 完整提示词预览：系统模板前缀 + 描述 + 画风后缀（提交图片模型时原样发送） */}
                        <div className="grid grid-cols-[80px_1fr] items-start gap-2">
                            <span className="text-xs text-stone-500 pt-1">完整提示词</span>
                            <div className="space-y-1">
                                <div className="text-[10px] text-stone-400">
                                    以下为「系统模板 + 描述 + 画风后缀」完整拼接后的实际生图 prompt（提交图片模型时原样发送）。
                                </div>
                                <textarea
                                    readOnly
                                    value={fullPrompt}
                                    rows={Math.min(10, fullPrompt.split("\n").length + 1)}
                                    className="thin-scrollbar w-full resize-none rounded-lg border border-dashed border-stone-200 bg-stone-50 px-3 py-2 text-[11px] leading-5 text-stone-600 outline-none dark:border-stone-700 dark:bg-stone-800/60 dark:text-stone-300"
                                />
                            </div>
                        </div>
                    </div>
                </Modal>
                );
            })()}

            {/* 编辑原文弹窗：编辑原始小说全文，保存后按"第N章"重新解析章节目录（丢弃简介） */}
            <Modal title="编辑原文" open={showScriptEditor} onCancel={() => setShowScriptEditor(false)} width={760}
                footer={[
                    <Button key="cancel" onClick={() => setShowScriptEditor(false)}>取消</Button>,
                    <Button key="save" type="primary" onClick={saveScriptEditor}>保存并解析章节</Button>,
                ]}>
                <div className="mb-2 text-xs text-stone-400">粘贴或编辑小说原文；保存后会按"第N章 / 第一章 / Chapter N"自动分章，第1章之前的书名/简介/标签会被丢弃。</div>
                <textarea value={scriptDraft} onChange={(e) => setScriptDraft(e.target.value)}
                    placeholder={"第1章 xxx\n正文…\n\n第2章 xxx\n正文…"}
                    className="thin-scrollbar h-[60vh] w-full resize-none rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 text-sm leading-6 outline-none dark:border-stone-700 dark:bg-stone-800 dark:text-stone-200" />
            </Modal>

            {/* 底部状态栏拆 3 行（避免原本一行挤 11 个 label）：
                Row 1 = 标题 + 项目名 + pipeline 状态 + 生成参数（时长/比例/分辨率/首尾帧）
                Row 2 = 模型选择（文本/图片/视频）+ 单镜算力
                Row 3 = 配置按钮 + 账户统计
                「一键出片」开关已上移到顶部主按钮旁（chip 形式），不再出现于此处 */}
            <footer className="shrink-0 space-y-1.5 border-t border-stone-200 bg-white px-6 py-2 dark:border-stone-800 dark:bg-stone-900">
                {/* Row 1: 标题 + 项目名 + pipeline 状态 + 生成参数 */}
                <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-stone-500 dark:text-stone-400">
                    {/* 标题 */}
                    <span className="flex items-center gap-1.5 font-medium text-stone-700 dark:text-stone-200">
                        <span className="flex size-5 items-center justify-center rounded bg-stone-800 text-white dark:bg-stone-200 dark:text-stone-900"><Video className="size-3" /></span>
                        剧本转视频
                    </span>
                    {activeProject && <span className="rounded-full bg-stone-100 px-2 py-0.5 text-stone-600 dark:bg-stone-800 dark:text-stone-300">{activeProject.name}</span>}
                    {(pipelineRunning || parsingStoryboard) && (
                        <span className="flex items-center gap-1 text-stone-500"><LoaderCircle className="size-3 animate-spin" />{pipelineStatus}</span>
                    )}

                    <span className="mx-1 h-3 w-px bg-stone-200 dark:bg-stone-700" />

                    {/* 时长 / 比例 / 分辨率（原顶栏可选项，直接内联可改） */}
                    <label className="flex items-center gap-1">时长
                        <InputNumber size="small" min={5} max={60} value={shotDuration} onChange={(v) => setShotDuration(v || 15)} className="!w-14" />秒
                        {shotDurationExceedsLimit && (
                            <Tooltip title={`当前视频模型 ${videoModel} 最大支持 ${videoModelMaxDuration} 秒；prompt 会自动截断到 ${videoModelMaxDuration} 秒。建议：缩短分镜时长到 ≤ ${videoModelMaxDuration} 秒，或换个支持更长的模型（如 Seedance / Kling V3）。`}>
                                <span className="flex items-center gap-1 rounded bg-rose-100 px-1.5 py-0.5 text-[10px] font-medium text-rose-700 dark:bg-rose-950/40 dark:text-rose-400">
                                    ⚠ 超模型上限 {videoModelMaxDuration}s
                                </span>
                            </Tooltip>
                        )}
                    </label>
                    <label className="flex items-center gap-1">比例
                        <Select size="small" value={aspectRatio} onChange={setAspectRatio} className="w-28"
                            options={[{ label: "16:9 横屏", value: "16:9" }, { label: "9:16 竖屏", value: "9:16" }, { label: "1:1 方形", value: "1:1" }, { label: "21:9 宽屏", value: "21:9" }]} />
                    </label>
                    <label className="flex items-center gap-1">分辨率
                        <Select size="small" value={resolution} onChange={setResolution} className="w-20"
                            options={[{ label: "720p", value: "720p" }, { label: "1080p", value: "1080p" }]} />
                    </label>
                    <span>首尾帧: {supportsFrameRefs ? "支持" : "不支持"}</span>
                </div>

                {/* Row 2: 模型选择（文本/图片/视频）+ 单镜算力 */}
                <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-stone-500 dark:text-stone-400">
                    <span className="mx-1 h-3 w-px bg-stone-200 dark:bg-stone-700" />
                    {/* 文本模型选择（用于「开始分镜」把章节改写成分镜剧本 / 提取分镜图片描述）。
                        默认继承「配置与用户偏好」里的全局文本模型，这里改动会写回全局、与其他页面统一；也可在此单独切换。 */}
                    <span className="flex items-center gap-1">文本
                        <ModelPicker
                            config={effectiveConfig} value={effectiveConfig.textModel || defaultConfig.textModel}
                            channelId={effectiveConfig.textChannelId || effectiveConfig.activeChannelId}
                            capability="text"
                            onChange={(model, channelId) => { updateConfig("textModel", model); if (channelId) updateConfig("textChannelId", channelId); }}
                            placeholder="选择文本模型" onMissingConfig={() => openConfigDialog(true)} variant="quick"
                            className="!h-6 !min-w-32 !text-xs"
                        />
                    </span>

                    {/* 图片模型选择（用于生成分镜图片参考图）。 */}
                    <span className="flex items-center gap-1">图片
                        <ModelPicker
                            // 修复：不再回退到 effectiveConfig.model（文本模型），避免用文本模型生图
                            config={effectiveConfig} value={effectiveConfig.imageModel || defaultConfig.imageModel}
                            channelId={effectiveConfig.imageChannelId || effectiveConfig.activeChannelId}
                            capability="image"
                            onChange={(model, channelId) => { updateConfig("imageModel", model); if (channelId) updateConfig("imageChannelId", channelId); }}
                            placeholder="选择图片模型" onMissingConfig={() => openConfigDialog(true)} variant="quick"
                            className="!h-6 !min-w-32 !text-xs"
                        />
                    </span>

                    {/* 视频模型选择（用于把每条分镜剧本生成视频）。同样默认继承全局配置，改动写回全局。 */}
                    <span className="flex items-center gap-1">视频
                        <ModelPicker
                            config={effectiveConfig} value={effectiveConfig.videoModel || defaultConfig.videoModel}
                            channelId={effectiveConfig.videoChannelId || effectiveConfig.activeChannelId}
                            capability="video"
                            onChange={(model, channelId) => { updateConfig("videoModel", model); if (channelId) updateConfig("videoChannelId", channelId); }}
                            placeholder="选择视频模型" onMissingConfig={() => openConfigDialog(true)} variant="quick"
                            className="!h-6 !min-w-32 !text-xs"
                        />
                    </span>
                    {shotCredit.costCents > 0 && (
                        <Tooltip title={`当前视频模型每个分镜约耗 ${formatBalanceYuan(shotCredit.costCents)} 余额（${shotDuration} 秒${shotCredit.isPerSecond ? " · 按秒计费" : ""}）`}>
                            <span className="flex items-center gap-1 text-stone-500">单镜约耗<BalanceSymbol className="text-[10px] leading-none" />{formatBalanceYuan(shotCredit.costCents)}</span>
                        </Tooltip>
                    )}
                </div>

                {/* Row 3: 配置按钮 + 账户统计 */}
                <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-stone-500 dark:text-stone-400">
                    <span className="mx-1 h-3 w-px bg-stone-200 dark:bg-stone-700" />
                    <Button size="small" icon={<Settings2 className="size-3" />} onClick={() => setShowConfigModal(true)} className="h-6 rounded-lg text-xs">配置</Button>

                    {/* 右侧统计 */}
                    <span className="ml-auto flex items-center gap-3">
                        {userBalanceCents !== null && (
                            <>
                                <Tooltip title={userBalanceCents <= 0 ? "账户余额不足，请前往充值" : "账号账户余额"}>
                                    <span className={`flex items-center gap-1 font-medium ${userBalanceCents <= 0 ? "text-rose-500" : userBalanceCents <= 5 ? "text-amber-500" : "text-emerald-600 dark:text-emerald-400"}`}>
                                        余额<BalanceSymbol className="text-[10px] leading-none" />{formatBalanceYuan(userBalanceCents)}
                                    </span>
                                </Tooltip>
                                <span>·</span>
                            </>
                        )}
                        <span>{projects.length} 项目</span>
                        <span>·</span>
                        <span>{assets.length} 资产</span>
                        <span>·</span>
                        <span>{completedCount} 已完成 / {generatingCount} 生成中 / {failedCount} 失败</span>
                    </span>
                </div>
            </footer>
        </div>
    );
}
