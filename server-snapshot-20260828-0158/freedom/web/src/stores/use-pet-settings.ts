"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import { localForageStorage, persistStorage } from "@/lib/localforage-storage";

export type PetCharacterId = "neowow";

/** 用户上传的桌宠 sprite sheet 元数据（动态宠物配置）。 */
export type UploadedPetMeta = {
    id: string;
    name: string;
    url: string;
    storageKey: string;
    bytes: number;
    mimeType: string;
    /** sprite sheet 的总宽度 = cols * cellW。 */
    width: number;
    /** sprite sheet 的总高度 = rows * cellH。 */
    height: number;
    createdAt: string;
    // sprite 网格
    cols: number;
    rows: number;
    cellW: number;
    cellH: number;
    // 待机动画（必填）
    idleStart: number;
    idleEnd: number;
    idleFps: number;
    // 点击动画（可选；未设则点击/双击都用待机）
    clickStart?: number;
    clickEnd?: number;
    clickFps?: number;
};

export type PetSettingsState = {
    enabled: boolean;
    size: number;
    character: PetCharacterId | `uploaded-${string}`;
    broadcastTasks: boolean;
    broadcastNodes: boolean;
    accompany: boolean;
    resetToken: number;
    /** 桌宠贴边侧；null 表示尚未持久化，使用默认 "right"。 */
    side: "left" | "right" | null;
    /** 桌宠在可视高度里的纵向位置（0 = 顶，1 = 底），用于持久化以适应不同窗口高度。 */
    yRatio: number | null;
    /** 用户上传的桌宠图片列表；持久化，未登录时也能本地保存。 */
    uploadedPets: UploadedPetMeta[];
    /** 外部播报消息（节点完成/生成陪伴等），瞬时，不持久化。 */
    externalMessage: string | null;
    setEnabled: (v: boolean) => void;
    setSize: (v: number) => void;
    setCharacter: (v: PetCharacterId | `uploaded-${string}`) => void;
    setBroadcastTasks: (v: boolean) => void;
    setBroadcastNodes: (v: boolean) => void;
    setAccompany: (v: boolean) => void;
    resetPosition: () => void;
    setSide: (v: "left" | "right" | null) => void;
    setYRatio: (v: number | null) => void;
    addUploadedPet: (meta: UploadedPetMeta) => void;
    removeUploadedPet: (id: string) => Promise<void>;
    setExternalMessage: (v: string | null) => void;
};

export const PET_CHARACTER_OPTIONS: { value: PetCharacterId; label: string }[] = [
    { value: "neowow", label: "Neowow" },
];

export const PET_SIZE_MIN = 56;
export const PET_SIZE_MAX = 120;
export const PET_SIZE_DEFAULT = 72;

export const usePetSettings = create<PetSettingsState>()(
    persist(
        (set, get) => ({
            enabled: true,
            size: PET_SIZE_DEFAULT,
            character: "neowow",
            broadcastTasks: false,
            broadcastNodes: false,
            accompany: false,
            resetToken: 0,
            side: null,
            yRatio: null,
            uploadedPets: [],
            externalMessage: null,
            setEnabled: (enabled) => set({ enabled }),
            setSize: (size) => set({ size: Math.max(PET_SIZE_MIN, Math.min(PET_SIZE_MAX, size)) }),
            setCharacter: (character) => set({ character }),
            setBroadcastTasks: (broadcastTasks) => set({ broadcastTasks }),
            setBroadcastNodes: (broadcastNodes) => set({ broadcastNodes }),
            setAccompany: (accompany) => set({ accompany }),
            resetPosition: () => set((state) => ({ resetToken: state.resetToken + 1, side: "right", yRatio: 0.6 })),
            setSide: (side) => set({ side }),
            setYRatio: (yRatio) => set({ yRatio: yRatio === null ? null : Math.max(0, Math.min(1, yRatio)) }),
            addUploadedPet: (meta) => set((state) => ({ uploadedPets: [...state.uploadedPets, meta] })),
            removeUploadedPet: async (id) => {
                const target = get().uploadedPets.find((p) => p.id === id);
                if (target) {
                    const { deleteStoredMedia } = await import("@/services/file-storage");
                    await deleteStoredMedia([target.storageKey]).catch(() => undefined);
                }
                set((state) => ({
                    uploadedPets: state.uploadedPets.filter((p) => p.id !== id),
                    // 如果当前选的就是被删的，回退到 neowow
                    character: state.character === `uploaded-${id}` ? "neowow" : state.character,
                }));
            },
            setExternalMessage: (externalMessage) => set({ externalMessage }),
        }),
        {
            name: "infinite-canvas:pet-settings",
            storage: persistStorage,
            skipHydration: true,
            partialize: (state) => ({
                enabled: state.enabled,
                size: state.size,
                character: state.character,
                broadcastTasks: state.broadcastTasks,
                broadcastNodes: state.broadcastNodes,
                accompany: state.accompany,
                resetToken: state.resetToken,
                side: state.side,
                yRatio: state.yRatio,
                uploadedPets: state.uploadedPets,
            }),
        },
    ),
);
