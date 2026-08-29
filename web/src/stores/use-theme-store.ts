import { create } from "zustand";
import { persist } from "zustand/middleware";
import { persistStorage } from "@/lib/localforage-storage";

export type ThemeName = "light" | "dark";

type ThemeStore = {
    theme: ThemeName;
    setTheme: (theme: ThemeName) => void;
};

export const useThemeStore = create<ThemeStore>()(
    persist(
        (set) => ({
            theme: "dark",
            setTheme: (theme) => set({ theme }),
        }),
        { name: "freedom:theme_store", storage: persistStorage },
    ),
);
