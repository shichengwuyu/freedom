import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { nanoid } from "nanoid";
import { localForageStorage } from "@/lib/localforage-storage";
import { listCanvasProjects, saveCanvasProject, syncCanvasProjects } from "@/services/api/canvas-tasks";
import { fetchUserConfig } from "@/services/api/user-config";
import { useUserStore } from "@/stores/use-user-store";
import type { CanvasBackgroundMode } from "@/lib/canvas-theme";
import type { CanvasAgentConfig, CanvasAssistantSession, CanvasConnection, CanvasNodeData, CanvasPendingAgentRequest, ViewportTransform } from "../types";

export type CanvasSidePanelState = {
    open: boolean;
    width: number;
};

export const DEFAULT_CANVAS_SIDE_PANEL: CanvasSidePanelState = { open: true, width: 280 };
export const DEFAULT_CANVAS_AGENT_PANEL: CanvasSidePanelState = { open: false, width: 390 };

export type CanvasProject = {
    id: string;
    title: string;
    createdAt: string;
    updatedAt: string;
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    chatSessions: CanvasAssistantSession[];
    activeChatId: string | null;
    agentConfig: CanvasAgentConfig | null;
    autoTitlePending: boolean;
    pendingAgentRequest?: CanvasPendingAgentRequest;
    backgroundMode: CanvasBackgroundMode;
    showImageInfo: boolean;
    viewport: ViewportTransform;
    sidePanel: CanvasSidePanelState;
    agentPanel: CanvasSidePanelState;
};

type CanvasStore = {
    hydrated: boolean;
    projects: CanvasProject[];
    createProject: (title?: string, options?: { agentConfig?: CanvasAgentConfig; pendingAgentRequest?: CanvasPendingAgentRequest }) => string;
    importProject: (project: Partial<CanvasProject>) => string;
    openProject: (id: string) => CanvasProject | null;
    renameProject: (id: string, title: string) => void;
    deleteProjects: (ids: string[]) => void;
    updateProject: (id: string, patch: Partial<Pick<CanvasProject, "nodes" | "connections" | "chatSessions" | "activeChatId" | "agentConfig" | "autoTitlePending" | "backgroundMode" | "showImageInfo" | "viewport" | "sidePanel" | "agentPanel" | "pendingAgentRequest">>) => void;
    syncWithRemote: (token: string, syncEnabled: boolean) => Promise<void>;
    setSyncEnabled: (enabled: boolean) => void;
};

const initialViewport: ViewportTransform = { x: 0, y: 0, k: 1 };
const CANVAS_STORE_KEY = "freedom:canvas_store";
type PersistedCanvasState = Pick<CanvasStore, "projects">;
let saveTimer: ReturnType<typeof setTimeout> | null = null;
let queuedPersistState: PersistedCanvasState | null = null;
let accountCanvasSyncEnabled = false;
const projectSaveTimers = new Map<string, ReturnType<typeof setTimeout>>();
// 后端同步飞行锁：避免同一时刻多次触发同步请求
let canvasRemoteSyncInFlight = false;
// 上次同步的 token + 时间戳，做短时间缓存（5 秒内不再请求）
let lastSyncCache: { token: string; timestamp: number } | null = null;
const REMOTE_SYNC_COOLDOWN_MS = 5000;

function queueProjectSave(project: CanvasProject) {
    const token = useUserStore.getState().token;
    const syncEnabled = accountCanvasSyncEnabled;
    const previous = projectSaveTimers.get(project.id);
    if (previous) clearTimeout(previous);

    projectSaveTimers.set(
        project.id,
        setTimeout(() => {
            projectSaveTimers.delete(project.id);
            if (
                !token ||
                !syncEnabled ||
                !accountCanvasSyncEnabled ||
                useUserStore.getState().token !== token
            ) {
                return;
            }
            void saveCanvasProject(token, project).catch(() => undefined);
        }, 400),
    );
}

function cancelProjectSaves(ids: string[]) {
    ids.forEach((id) => {
        const timer = projectSaveTimers.get(id);
        if (!timer) return;
        clearTimeout(timer);
        projectSaveTimers.delete(id);
    });
}

// 立即持久化：绕过 400ms 防抖，把当前画布状态同步落盘（本地 IndexedDB）并触发后端保存。
// 用于「退出/切换页面、停止 Agent」等时刻，避免最近一次节点变更还停留在防抖计时器里就被丢弃。
export function flushCanvasPersistence(): void {
    // 1) 本地持久化：清掉防抖计时器，立即写 IndexedDB
    if (saveTimer) {
        clearTimeout(saveTimer);
        saveTimer = null;
    }
    const projects = useCanvasStore.getState().projects;
    const nextState: PersistedCanvasState = { projects };
    queuedPersistState = nextState;
    void localForageStorage.setItem(
        CANVAS_STORE_KEY,
        JSON.stringify({ state: nextState, version: 0 }),
    );

    // 2) 后端保存：把所有排队中的项目保存立即发出（仅在登录 + 开启同步时）
    const token = useUserStore.getState().token;
    const canSyncRemote = Boolean(token) && accountCanvasSyncEnabled;
    const pendingIds = Array.from(projectSaveTimers.keys());
    pendingIds.forEach((id) => {
        const timer = projectSaveTimers.get(id);
        if (timer) clearTimeout(timer);
        projectSaveTimers.delete(id);
        if (!canSyncRemote) return;
        const project = projects.find((item) => item.id === id);
        if (!project) return;
        void saveCanvasProject(token as string, project).catch(() => undefined);
    });
}

async function reconcileCanvasProjects(
    token: string,
    remoteProjects: CanvasProject[],
    localProjects: CanvasProject[],
) {
    const remoteById = new Map(
        remoteProjects.map((project) => [project.id, project]),
    );
    const missingProjects = localProjects.filter(
        (project) => !remoteById.has(project.id),
    );
    const existingLocalProjects = localProjects.filter((project) =>
        remoteById.has(project.id),
    );
    const projects = missingProjects.length
        ? await syncCanvasProjects(token, missingProjects)
            .then((syncedProjects) =>
                mergeCanvasProjects(
                    syncedProjects,
                    existingLocalProjects,
                ),
            )
            .catch(() =>
                mergeCanvasProjects(remoteProjects, localProjects),
            )
        : mergeCanvasProjects(remoteProjects, existingLocalProjects);

    localProjects.forEach((project) => {
        const remote = remoteById.get(project.id);
        if (
            remote &&
            Date.parse(project.updatedAt || "") >
            Date.parse(remote.updatedAt || "")
        ) {
            queueProjectSave(project);
        }
    });

    return projects;
}

// 【异步非阻塞】后端同步：本地水合完成后，在后台静默拉取云端数据并合并
// 不再阻塞 store 的 rehydrate 流程，页面可以立刻用本地数据渲染
async function syncCanvasWithRemoteInBackground(name: string) {
    // 冷却节流：短时间内重复触发直接跳过（路由切换防抖）
    const token = useUserStore.getState().token;
    if (!token) return;
    const now = Date.now();
    if (lastSyncCache && lastSyncCache.token === token && now - lastSyncCache.timestamp < REMOTE_SYNC_COOLDOWN_MS) {
        return;
    }
    if (canvasRemoteSyncInFlight) return;
    canvasRemoteSyncInFlight = true;
    try {
        const [userConfig, remoteProjects] = await Promise.all([
            fetchUserConfig(token),
            listCanvasProjects(token),
        ]);
        lastSyncCache = { token, timestamp: Date.now() };
        accountCanvasSyncEnabled = userConfig.syncCapabilities?.userData === true;

        const localParsed = await localForageStorage.getItem(name);
        const localProjects = localParsed
            ? ((JSON.parse(localParsed) as StorageValue<CanvasStore>).state as PersistedCanvasState)?.projects || []
            : [];
        const localHasData = Array.isArray(localProjects) && localProjects.length > 0;

        let mergedProjects: CanvasProject[] | null = null;

        if (accountCanvasSyncEnabled && localHasData) {
            mergedProjects = await reconcileCanvasProjects(token, remoteProjects, localProjects);
        } else if (remoteProjects.length > 0 && (accountCanvasSyncEnabled || !localHasData)) {
            mergedProjects = remoteProjects;
        }

        if (mergedProjects) {
            // 只在数据确实有差异时才更新 store 和持久化，减少无意义 re-render
            const currentProjects = useCanvasStore.getState().projects;
            const idsDiffer =
                currentProjects.length !== mergedProjects.length ||
                currentProjects.some((p, i) => p.id !== mergedProjects[i]?.id);
            if (idsDiffer) {
                const nextState = { projects: mergedProjects };
                queuedPersistState = nextState;
                useCanvasStore.setState(nextState);
                await localForageStorage.setItem(name, JSON.stringify({ state: nextState, version: 0 }));
            }
        }
    } catch (error) {
        console.error("Failed to sync canvas projects from remote (background)", error);
    } finally {
        canvasRemoteSyncInFlight = false;
    }
}

const canvasStorage: PersistStorage<CanvasStore> = {
    getItem: async (name) => {
        // ✅ 优化：不再 waitForUserStoreHydration()，不再阻塞式 fetch 后端
        // 先立刻返回本地数据，让页面尽快渲染；云端同步放到后台异步执行
        const localValue = await localForageStorage.getItem(name);
        if (!localValue) return null;
        const localParsed = JSON.parse(localValue) as StorageValue<CanvasStore>;
        queuedPersistState = localParsed.state as PersistedCanvasState;
        return localParsed;
    },

    setItem: (name, value) => {
        const nextState = value.state as PersistedCanvasState;
        if (
            queuedPersistState &&
            queuedPersistState.projects === nextState.projects
        ) {
            return;
        }
        queuedPersistState = nextState;
        if (saveTimer) clearTimeout(saveTimer);
        saveTimer = setTimeout(() => {
            saveTimer = null;
            void localForageStorage.setItem(name, JSON.stringify(value));
        }, 400);
    },
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useCanvasStore = create<CanvasStore>()(
    persist(
        (set, get) => ({
            hydrated: false,
            projects: [],
            createProject: (title = "未命名画布", options) => {
                const now = new Date().toISOString();
                const id = nanoid();
                const project: CanvasProject = {
                    id,
                    title,
                    createdAt: now,
                    updatedAt: now,
                    nodes: [],
                    connections: [],
                    chatSessions: [],
                    activeChatId: null,
                    agentConfig: options?.agentConfig || null,
                    autoTitlePending: true,
                    pendingAgentRequest: options?.pendingAgentRequest,
                    backgroundMode: "lines",
                    showImageInfo: false,
                    viewport: initialViewport,
                    sidePanel: DEFAULT_CANVAS_SIDE_PANEL,
                    agentPanel: options?.pendingAgentRequest ? { ...DEFAULT_CANVAS_AGENT_PANEL, open: true } : DEFAULT_CANVAS_AGENT_PANEL,
                };
                set((state) => ({
                    projects: [project, ...state.projects],
                }));
                queueProjectSave(project);
                return id;
            },
            importProject: (source) => {
                const now = new Date().toISOString();
                const project: CanvasProject = {
                    id: nanoid(),
                    title: source.title || "导入画布",
                    createdAt: source.createdAt || now,
                    updatedAt: now,
                    nodes: source.nodes || [],
                    connections: source.connections || [],
                    chatSessions: source.chatSessions || [],
                    activeChatId: source.activeChatId || null,
                    agentConfig: source.agentConfig || null,
                    autoTitlePending: false,
                    backgroundMode: source.backgroundMode || "lines",
                    showImageInfo: source.showImageInfo || false,
                    viewport: source.viewport || initialViewport,
                    sidePanel: source.sidePanel || DEFAULT_CANVAS_SIDE_PANEL,
                    agentPanel: source.agentPanel || DEFAULT_CANVAS_AGENT_PANEL,
                };
                set((state) => ({
                    projects: [project, ...state.projects],
                }));
                queueProjectSave(project);
                return project.id;
            },
            openProject: (id) =>
                get().projects.find((item) => item.id === id) || null,
            renameProject: (id, title) => {
                const project = get().projects.find(
                    (item) => item.id === id,
                );
                if (!project) return;
                const nextProject = {
                    ...project,
                    title: title.trim() || project.title,
                    autoTitlePending: false,
                    updatedAt: new Date().toISOString(),
                };
                set((state) => ({
                    projects: state.projects.map((item) =>
                        item.id === id ? nextProject : item,
                    ),
                }));
                queueProjectSave(nextProject);
            },
            deleteProjects: (ids) => {
                cancelProjectSaves(ids);
                set((state) => ({
                    projects: state.projects.filter(
                        (project) => !ids.includes(project.id),
                    ),
                }));
            },
            updateProject: (id, patch) => {
                const project = get().projects.find(
                    (item) => item.id === id,
                );
                if (!project) return;
                const nextProject = {
                    ...project,
                    ...patch,
                    updatedAt: new Date().toISOString(),
                };
                set((state) => ({
                    projects: state.projects.map((item) =>
                        item.id === id ? nextProject : item,
                    ),
                }));
                queueProjectSave(nextProject);
            },
            syncWithRemote: async (token, syncEnabled) => {
                accountCanvasSyncEnabled = syncEnabled;
                if (!syncEnabled) return;
                const localProjects = get().projects;
                const remoteProjects = await listCanvasProjects(token).catch(
                    () => null,
                );
                if (!remoteProjects) return;
                const projects = await reconcileCanvasProjects(
                    token,
                    remoteProjects,
                    localProjects,
                );
                if (saveTimer) {
                    clearTimeout(saveTimer);
                    saveTimer = null;
                }
                const nextState = { projects };
                queuedPersistState = nextState;
                set(nextState);
                await localForageStorage.setItem(
                    CANVAS_STORE_KEY,
                    JSON.stringify({ state: nextState, version: 0 }),
                );
            },
            setSyncEnabled: (enabled) => {
                accountCanvasSyncEnabled = enabled;
            },
        }),
        {
            name: CANVAS_STORE_KEY,
            storage: canvasStorage,
            partialize: (state) =>
                ({
                    projects: state.projects,
                }) as StorageValue<CanvasStore>["state"],
            onRehydrateStorage: () => {
                // 本地水合完成后，立刻把 hydrated 设为 true 让页面渲染
                // 然后启动后台异步同步云端数据（不阻塞渲染）
                return (state, error) => {
                    if (error) {
                        console.error("Canvas store rehydrate error", error);
                    }
                    useCanvasStore.setState({ hydrated: true });
                    // 启动异步后台同步（setTimeout 放入宏任务，避开当前水合事务）
                    setTimeout(() => {
                        void syncCanvasWithRemoteInBackground(CANVAS_STORE_KEY);
                    }, 0);
                    // 用户 store 完成水合后再触发一次兜底同步（处理「token 注入时机晚于 canvas 水合」的情况）。
                    // 注：本项目 zustand 5 在 SSR/构建期 useUserStore.persist 为 undefined（persist API 未稳定挂在实例上），
                    // 直接用 .persist.hasHydrated() 会在 next build / 客户端水合时崩溃。改用 isReady 状态变化判定用户数据就绪。
                    if (!useUserStore.getState().isReady) {
                        const unsub = useUserStore.subscribe((state, prev) => {
                            if (state.isReady && !prev.isReady) {
                                unsub();
                                setTimeout(() => {
                                    void syncCanvasWithRemoteInBackground(CANVAS_STORE_KEY);
                                }, 0);
                            }
                        });
                    }
                };
            },
        },
    ),
);

export function mergeCanvasProjects(
    remoteProjects: CanvasProject[],
    localProjects: CanvasProject[],
): CanvasProject[] {
    const projects = new Map<string, CanvasProject>();
    [...localProjects, ...remoteProjects].forEach((project) => {
        const previous = projects.get(project.id);
        if (
            !previous ||
            Date.parse(project.updatedAt || "") >=
            Date.parse(previous.updatedAt || "")
        ) {
            projects.set(project.id, project);
        }
    });
    return Array.from(projects.values()).sort(
        (a, b) =>
            Date.parse(b.updatedAt || "") -
            Date.parse(a.updatedAt || ""),
    );
}