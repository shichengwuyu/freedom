// 图片模型能力表（quality / aspectRatio / 像素限制）。
//
// 单一真相：被 ImageSettingsPanel / image-size QuickSelect 共用。视频端用 @/lib/video-model-capabilities。
//
// 命中顺序：
// 1) modelKey(modelName) 命中白名单 → 用白名单
// 2) 未命中 → 返回 null，调用方继续显示全部选项（保守，默认放行）
//
// 任何图像尺寸/比例新增，先在这里登记，禁止在面板里再写内联硬编码 if。

import { modelKey } from "@/lib/video-model-capabilities";

export type ImageQuality = "auto" | "high" | "medium" | "low" | "standard" | "hd";

// 比例白名单：保存成与 image-settings-panel 的 aspectOptions.value 对齐的子串。
// "1:1"、"3:2"、"2:3"、"4:3"、"3:4"、"16:9"、"9:16"、"21:9" 全集
// "*-2k" / "*-4k" 单独控制
export const ASPECT_VALUES = [
    "1:1",
    "3:2",
    "2:3",
    "4:3",
    "3:4",
    "16:9",
    "9:16",
    "21:9",
] as const;

// 高清档位的 2k/4k 后缀（保留以备外部逻辑引用，过滤时仍由 cap.aspectRatios 全权决定）
export const HIGH_RES_2K_SUFFIX = "2k" as const;
export const HIGH_RES_4K_SUFFIX = "4k" as const;

export type ImageModelCapabilities = {
    qualities: ImageQuality[];          // 允许出现的质量档位
    aspectRatios: readonly string[];    // 比例白名单（基础 + 高清档）
    maxDimension?: number;              // 自由尺寸模式下宽高上限（像素），限长边
};

const DEFAULT_QUALITIES: ImageQuality[] = ["auto", "high", "medium", "low"];

// 家族 helper —— 用最小集合覆盖一类模型
function dallE(): ImageModelCapabilities {
    // dall-e-3 / dall-e-2：固定三档比例，质量只有 standard/hd（≈ medium/high）
    return {
        qualities: ["standard", "hd"],
        aspectRatios: ["1:1", "16:9", "9:16"],
        maxDimension: 1792,
    };
}

function gptImage1(): ImageModelCapabilities {
    // gpt-image-1 / gpt-4o-image：支持 auto/high/medium/low，比例齐
    return {
        qualities: [...DEFAULT_QUALITIES],
        aspectRatios: [...ASPECT_VALUES],
        maxDimension: 1536,
    };
}

function nanoBanana(): ImageModelCapabilities {
    // nano-banana / imagen / gemini-*-image：通吃 2k 档，常量
    return {
        qualities: [...DEFAULT_QUALITIES],
        aspectRatios: [...ASPECT_VALUES, "2k"],
        maxDimension: 2048,
    };
}

function geminiImage(): ImageModelCapabilities {
    // gemini 图片（gemini-3-pro-image-preview / gemini-3.1-flash-image-preview / gemini-omni-character 等）：
    // 官方云端走 ai.go 的 OpenAI 直转，没有 apimart 的 snapGeminiAspectRatio 兜底，
    // 所以这里必须严格给 5 个比例，多给的 2:3 / 3:2 / 21:9 会被上游拒（unsupported image size）。
    return {
        qualities: [...DEFAULT_QUALITIES],
        aspectRatios: ["1:1", "3:4", "4:3", "9:16", "16:9", "2k"],
        maxDimension: 2048,
    };
}

function flux(): ImageModelCapabilities {
    // flux / flux-pro / kontext / qwen-image / ideogram / recraft / midjourney / sdxl：标准画像（不支持 2k/4k）
    return {
        qualities: [...DEFAULT_QUALITIES],
        aspectRatios: [...ASPECT_VALUES],
        maxDimension: 1536,
    };
}

function seedream(): ImageModelCapabilities {
    // seedream(3/4/5)：固定比例三档（豆包主打的 1:1/4:3/16:9/9:16/3:4/2:3/21:9/2k/4k）
    return {
        qualities: [...DEFAULT_QUALITIES],
        aspectRatios: [...ASPECT_VALUES, "2k", "4k"],
        maxDimension: 4096,
    };
}

function agnesImage(): ImageModelCapabilities {
    // agnes-image / grok-imagine（非 video 变体）：标准
    return {
        qualities: [...DEFAULT_QUALITIES],
        aspectRatios: [...ASPECT_VALUES],
        maxDimension: 1536,
    };
}

// 白名单登记表。顺序无关：先精确匹配，再走兜底
const CAPABILITY_TABLE: Array<{ test: (k: string) => boolean; cap: () => ImageModelCapabilities }> = [
    // dall-e 家族
    { test: (k) => k === "dall-e-3" || k.includes("dall-e"), cap: dallE },
    // gpt-image / 4o-image
    { test: (k) => k.includes("gpt-image") || k.includes("gpt-4o-image") || k === "4o-image", cap: gptImage1 },
    // nano-banana / imagen
    { test: (k) => k.includes("nano-banana") || k.includes("imagen"), cap: nanoBanana },
    // gemini 图片：只认 5 个比例（官方云端 Gemini image 模型，无 apimart 兜底吸附）
    { test: (k) => (k.includes("gemini") && k.includes("image")) || k.includes("gemini-omni-character"), cap: geminiImage },
    // seedream 家族
    { test: (k) => k.includes("seedream"), cap: seedream },
    // flux / kontext / qwen-image / ideogram / recraft / midjourney / sdxl
    { test: (k) => k.includes("flux") || k.includes("kontext"), cap: flux },
    { test: (k) => k.includes("qwen/image") || k.includes("qwen2/image") || k.includes("qwen/text-to-image") || k.includes("qwen2/text-to-image"), cap: flux },
    { test: (k) => k.includes("ideogram") || k.includes("recraft") || k.includes("midjourney"), cap: flux },
    { test: (k) => k.includes("sdxl") || k.includes("stable-diffusion"), cap: flux },
    // agnes / grok-imagine
    { test: (k) => k.includes("agnes-image") || k === "grok-imagine" || (k.includes("grok-imagine") && !k.includes("video")), cap: agnesImage },
    // wan2-7-image
    { test: (k) => k.includes("wan2-7-image") || k.includes("wan2.7-image") || k.includes("wan/2-7-image"), cap: seedream },
    // topaz/image
    { test: (k) => k.includes("topaz/image"), cap: flux },
];

export function resolveImageModelCapabilities(modelName: string): ImageModelCapabilities | null {
    const k = modelKey(modelName);
    if (!k) return null;
    for (const row of CAPABILITY_TABLE) {
        if (row.test(k)) return row.cap();
    }
    return null;
}

export function filterImageQualityOptions<T extends { value: string }>(options: readonly T[], modelName: string): T[] {
    const cap = resolveImageModelCapabilities(modelName);
    if (!cap) return [...options];
    return options.filter((item) => cap.qualities.includes(item.value as ImageQuality));
}

// 当前 aspect preset 通过 imageSizeOptions 暴露时（如 "1:1-2k" / "16:9-4k"），严格匹配 cap.aspectRatios；
// "auto" 永远保留；白名单外的渲染一律过滤。
// 之前版本有「基础比例在白名单但其高清档被允许」的宽松规则，会让 dall-e 之类不允许 2k 的模型漏出 "9:16-2k"，
// 这条规则下掉 —— 高清档必须显式写在 cap.aspectRatios 里才能被看见。
export function filterImageAspectOptions<T extends { value: string }>(options: readonly T[], modelName: string): T[] {
    const cap = resolveImageModelCapabilities(modelName);
    if (!cap) return [...options];
    const allowed = new Set(cap.aspectRatios);
    return options.filter((item) => item.value === "auto" || allowed.has(item.value));
}

// 当前选中的 quality 不在合法集 → 落在首项；值缺失则视为空并由调用方兜底
export function normalizeImageQualityForModel(modelName: string, current: string): string {
    const cap = resolveImageModelCapabilities(modelName);
    if (!cap) return current;
    if (cap.qualities.includes(current as ImageQuality)) return current;
    return cap.qualities[0] || "auto";
}

// 当前选中的 size 标准化：
//  - "auto" 永远透传
//  - 预设比例（如 "1:1" / "16:9-2k"）：若在白名单则保留，否则 snap 到首项
//  - 自定义 WxH（如 "1920x1080"）：若长边超 cap.maxDimension，按比例缩放到上限内
export function normalizeImageSizeForModel(modelName: string, current: string): string {
    const cap = resolveImageModelCapabilities(modelName);
    if (!cap) return current;
    if (current === "auto") return current;
    const preset = current.match(/^(\d+)x(\d+)$/);
    if (!preset) {
        if (cap.aspectRatios.includes(current)) return current;
        return cap.aspectRatios[0] || "auto";
    }
    if (!cap.maxDimension) return current;
    const w = Number(preset[1]);
    const h = Number(preset[2]);
    const longSide = Math.max(w, h);
    if (longSide <= cap.maxDimension) return current;
    const scale = cap.maxDimension / longSide;
    return `${Math.floor(w * scale)}x${Math.floor(h * scale)}`;
}
