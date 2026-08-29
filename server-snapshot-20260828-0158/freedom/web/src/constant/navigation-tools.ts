import { ImagePlus, Maximize2, Video, Wrench, VideoIcon } from "lucide-react";

export const navigationTools = [
    {
        slug: "canvas",
        label: "我的画布",
        icon: Maximize2,
    },
    {
        slug: "image",
        label: "生图工作台",
        icon: ImagePlus,
    },
    {
        slug: "video",
        label: "视频创作台",
        icon: Video,
    },
    {
        slug: "novel",
        label: "剧本转视频",
        icon: VideoIcon,
    },
    {
        slug: "tools",
        label: "更多工具",
        icon: Wrench,
    },
] as const;

export type NavigationToolSlug = (typeof navigationTools)[number]["slug"];
