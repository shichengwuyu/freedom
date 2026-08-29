import localforage from "localforage";
import type { Asset, NovelProject } from "./types";
import { PROJECTS_STORE_KEY, ASSETS_STORE_KEY, ACTIVE_PROJECT_KEY } from "./constants";

// ─────────────────────────── Store ───────────────────────────

export async function loadProjects(): Promise<NovelProject[]> {
    try { const s = await localforage.getItem<string>(PROJECTS_STORE_KEY); return s ? JSON.parse(s) : []; } catch { return []; }
}
export async function saveProjects(p: NovelProject[]) {
    try { await localforage.setItem(PROJECTS_STORE_KEY, JSON.stringify(p)); } catch {}
}
export async function loadAssets(): Promise<Asset[]> {
    try { const s = await localforage.getItem<string>(ASSETS_STORE_KEY); return s ? JSON.parse(s) : []; } catch { return []; }
}
export async function saveAssets(a: Asset[]) {
    try { await localforage.setItem(ASSETS_STORE_KEY, JSON.stringify(a)); } catch {}
}
// 持久化修复：读写当前选中项目 ID，配合 projects 列表恢复选中态
export async function loadActiveProjectId(): Promise<string | null> {
    try {
        const s = await localforage.getItem<string>(ACTIVE_PROJECT_KEY);
        return s && s.trim() ? s : null;
    } catch { return null; }
}
export async function saveActiveProjectId(id: string | null) {
    try { await localforage.setItem(ACTIVE_PROJECT_KEY, id ?? ""); } catch {}
}
