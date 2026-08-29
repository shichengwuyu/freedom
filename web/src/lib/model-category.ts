// 统一的模型分类启发式（image / video / text / audio）。
//
// 画布下拉过滤（use-config-store.modelMatchesCapability）和 admin 后台
// 「系统设置 → 模型渠道」（admin/settings/guessModelCategory）必须共用同一份真相，
// 否则同一个模型在不同 UI 会被分到不同类别。
//
// 2026-08-17 抽离：之前 use-config-store.ts 和 admin/settings/page.tsx
// 各自维护一份，字符串规则已经分叉（比如 admin 有 `wan2-1.5`，config-store
// 没有；config-store 有 `gemini-omni-video`，admin 没有）。
//
// 规约：
// - 不要在这用 `includes("gemini")` 通配（会把带 image 的 gemini 误归视频）
// - 任何只想改一边的改动都视为分叉修复的副作用，先在这改一处
// - 新增供应商模型分类关键词，往下面对应数组里加子串即可

export type ModelCategory = "image" | "video" | "text" | "audio";

// 兼容 export 别名（前端组件如 model-picker.tsx 已 import ModelCapability）
export type ModelCapability = ModelCategory;

// 视频模型子串关键词。注意顺序无关，但调试时保持语义分组。
const VIDEO_KEYWORDS: readonly string[] = [
    "video",
    "gemini-omni-flash",
    "gemini-omni-video",
    "seedance",
    "sd-",
    "sora",
    "veo",
    "veo3.1",
    "veo-3.1",
    "kling",
    "hailuo",
    "minimax",
    "minimax-video",
    "skyreels",
    "happyhorse",
    "runway",
    "aleph",
    "vidu",
    "pixverse",
    "omni-flash",
    "infinitalk",
    "wan2-5", "wan2.5",
    "wan2-6", "wan2.6",
    "wan2-7", "wan2.7",
    "wan2-7-r2v", "wan2.7-r2v",
    "wan2-7-videoedit", "wan2.7-videoedit",
    "wan/2-5", "wan/2-6",
    "wan/2-7-text-to-video", "wan/2-7-image-to-video",
    "wan/2-7-videoedit", "wan/2-7-r2v",
    "wan2-1.5", "wan/2-1-5",
    "ke-video",
    "videoedit",
    "image-to-video", "img2vid",
    "text-to-video", "text2vid",
    "lumina",
    "hunyuan-video",
    "shengshidian",
    "cogvideo",
    "/video",
    "video-",
];

const AUDIO_KEYWORDS: readonly string[] = [
    "audio", "tts", "speech", "voice", "music", "sound",
    "elevenlabs", "suno", "lyrics", "vocal", "midi", "wav",
    "mimo",
];

const IMAGE_KEYWORDS: readonly string[] = [
    "image",
    "nano-banana",
    "seedream",
    "gpt-image",
    "dall-e", "dalle",
    "imagen",
    "gemini-2.5-flash",
    "gemini-3-pro",
    "gemini-3.1-flash",
    "flux",
    "kontext",
    "4o-image", "4o image", "gpt-4o-image",
    "z-image",
    "qwen/image", "qwen2/image",
    "qwen/text-to-image", "qwen2/text-to-image",
    "ideogram",
    "recraft",
    "sdxl", "stable-diffusion",
    "midjourney",
    "wan2-7-image", "wan2.7-image", "wan/2-7-image",
    "topaz/image",
    "gemini-omni-character",
    "agnes-image",
    "text-to-image",
    "image-gen",
];

function includesAny(value: string, keywords: readonly string[]): boolean {
    for (const kw of keywords) {
        if (value.includes(kw)) return true;
    }
    return false;
}

export function isVideoModelName(model: string): boolean {
    const value = model.toLowerCase();
    if (includesAny(value, VIDEO_KEYWORDS)) return true;
    // grok-imagine 变体：含 /upscale 或 /extend 视为视频
    if (value.includes("grok-imagine") && (value.includes("/upscale") || value.includes("/extend"))) {
        return true;
    }
    return false;
}

export function isAudioModelName(model: string): boolean {
    return includesAny(model.toLowerCase(), AUDIO_KEYWORDS);
}

export function isImageModelName(model: string): boolean {
    const value = model.toLowerCase();
    if (isVideoModelName(model) || isAudioModelName(model)) return false;
    if (includesAny(value, IMAGE_KEYWORDS)) return true;
    // grok-imagine 不是视频变体时视为图片
    if (value.includes("grok-imagine") && !value.includes("video")) return true;
    return false;
}

export function isTextModelName(model: string): boolean {
    return !isImageModelName(model) && !isVideoModelName(model) && !isAudioModelName(model);
}

export function guessModelCategory(model: string): ModelCategory {
    if (isImageModelName(model)) return "image";
    if (isVideoModelName(model)) return "video";
    if (isAudioModelName(model)) return "audio";
    return "text";
}
