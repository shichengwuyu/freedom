import localforage from "localforage";
import { createJSONStorage } from "zustand/middleware";
import type { StateStorage } from "zustand/middleware";

localforage.config({
    name: "freedom",
    storeName: "app_state",
});

// 底层用 localforage（IndexedDB），失败时回退到 localStorage。
// 直接作为 StateStorage 使用（供 store 手动调用 getItem/setItem/removeItem）。
export const localForageStorage: StateStorage = {
    getItem: async (name) => {
        if (typeof window === "undefined") return null;
        try {
            return (await localforage.getItem<string>(name)) || null;
        } catch {
            return window.localStorage.getItem(name);
        }
    },
    setItem: async (name, value) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.setItem(name, value);
        } catch {
            window.localStorage.setItem(name, value);
        }
    },
    removeItem: async (name) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.removeItem(name);
        } catch {
            window.localStorage.removeItem(name);
        }
    },
};

// zustand v5 persist 中间件需要的 PersistStorage 类型适配器。
// 用 createJSONStorage 包裹 StateStorage，解决 v5 类型兼容性问题。
export const persistStorage = createJSONStorage(() => localForageStorage);
