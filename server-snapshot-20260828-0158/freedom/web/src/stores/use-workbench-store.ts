"use client";

import { create } from "zustand";
import { persist, type PersistStorage, type StorageValue } from "zustand/middleware";

import { localForageStorage } from "@/lib/localforage-storage";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";

// 生图工作台需要跨页面保留的输入状态
type ImageWorkbenchState = {
    prompt: string;
    references: ReferenceImage[];
};

// 视频创作台需要跨页面保留的输入状态
type VideoWorkbenchState = {
    prompt: string;
    negativePrompt: string;
    references: ReferenceImage[];
    firstFrame: ReferenceImage | null;
    lastFrame: ReferenceImage | null;
    videoReferences: ReferenceVideo[];
    audioReferences: ReferenceAudio[];
};

// setter 支持直接传值或函数式更新，和组件里原来的 setState 用法保持一致
type Updater<T> = T | ((prev: T) => T);

type WorkbenchStore = {
    image: ImageWorkbenchState;
    video: VideoWorkbenchState;
    setImagePrompt: (value: Updater<string>) => void;
    setImageReferences: (value: Updater<ReferenceImage[]>) => void;
    setVideoPrompt: (value: Updater<string>) => void;
    setVideoNegativePrompt: (value: Updater<string>) => void;
    setVideoReferences: (value: Updater<ReferenceImage[]>) => void;
    setVideoFirstFrame: (value: Updater<ReferenceImage | null>) => void;
    setVideoLastFrame: (value: Updater<ReferenceImage | null>) => void;
    setVideoReferenceVideos: (value: Updater<ReferenceVideo[]>) => void;
    setVideoReferenceAudios: (value: Updater<ReferenceAudio[]>) => void;
};

const WORKBENCH_STORE_KEY = "freedom:workbench_store";

const emptyImage: ImageWorkbenchState = { prompt: "", references: [] };
const emptyVideo: VideoWorkbenchState = { prompt: "", negativePrompt: "", references: [], firstFrame: null, lastFrame: null, videoReferences: [], audioReferences: [] };

// 解析函数式或直接值更新
function resolve<T>(value: Updater<T>, prev: T): T {
    return typeof value === "function" ? (value as (prev: T) => T)(prev) : value;
}

const workbenchStorage: PersistStorage<WorkbenchStore> = {
    getItem: async (name) => {
        const value = await localForageStorage.getItem(name);
        if (!value) return null;
        return JSON.parse(value) as StorageValue<WorkbenchStore>;
    },
    setItem: (name, value) => localForageStorage.setItem(name, JSON.stringify(value)),
    removeItem: (name) => localForageStorage.removeItem(name),
};

export const useWorkbenchStore = create<WorkbenchStore>()(
    persist(
        (set) => ({
            image: emptyImage,
            video: emptyVideo,
            setImagePrompt: (value) => set((state) => ({ image: { ...state.image, prompt: resolve(value, state.image.prompt) } })),
            setImageReferences: (value) => set((state) => ({ image: { ...state.image, references: resolve(value, state.image.references) } })),
            setVideoPrompt: (value) => set((state) => ({ video: { ...state.video, prompt: resolve(value, state.video.prompt) } })),
            setVideoNegativePrompt: (value) => set((state) => ({ video: { ...state.video, negativePrompt: resolve(value, state.video.negativePrompt) } })),
            setVideoReferences: (value) => set((state) => ({ video: { ...state.video, references: resolve(value, state.video.references) } })),
            setVideoFirstFrame: (value) => set((state) => ({ video: { ...state.video, firstFrame: resolve(value, state.video.firstFrame) } })),
            setVideoLastFrame: (value) => set((state) => ({ video: { ...state.video, lastFrame: resolve(value, state.video.lastFrame) } })),
            setVideoReferenceVideos: (value) => set((state) => ({ video: { ...state.video, videoReferences: resolve(value, state.video.videoReferences) } })),
            setVideoReferenceAudios: (value) => set((state) => ({ video: { ...state.video, audioReferences: resolve(value, state.video.audioReferences) } })),
        }),
        {
            name: WORKBENCH_STORE_KEY,
            storage: workbenchStorage,
            partialize: (state) => ({ image: state.image, video: state.video }) as StorageValue<WorkbenchStore>["state"],
        },
    ),
);
