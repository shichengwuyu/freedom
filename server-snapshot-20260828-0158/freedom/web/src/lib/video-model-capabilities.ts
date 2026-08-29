import type { AdminModelCost } from "@/services/api/admin";

export function modelKey(modelName: string) {
    return modelName.trim().toLowerCase().replace(/[._/]+/g, "-");
}

// 视频模型的参考/音频能力集合
export type VideoModelCapabilities = {
    refVideo: boolean; // 是否支持视频参考上传
    refAudio: boolean; // 是否支持音频参考上传
    genAudio: boolean; // 是否支持生成同步音频
};

// 视频模型的「可选项」能力集合（分辨率 / 时长 / 比例）。
// null 表示不限制，调用方继续显示全套默认。
export type VideoModelOptions = {
    resolutions: string[]; // 规范形态：去 p、去空格、小写。如 ["480","720","1080","2k","4k"]，filter 通过 normalizeResolutionKey 兼容 "720p"
    durations: number[]; // 时长档（秒），不含 -1（智能）
    aspectRatios: readonly string[]; // ["16:9","9:16","1:1","4:3","3:4","21:9","adaptive"]
    maxDuration: number; // 默认时长上限（秒），同 getModelMaxDuration
};

// 完整可选能力（refVideo/refAudio/genAudio + options），调用方一次拿全。
export type VideoModelFullCapabilities = VideoModelCapabilities & {
    options: VideoModelOptions | null;
};

// 统一取视频模型能力：优先用后台按模型配置（cost），后台未配置该项（undefined）时回退到白名单推断
export function resolveVideoModelCapabilities(modelName: string, cost?: AdminModelCost): VideoModelCapabilities {
    return {
        refVideo: cost?.refVideo ?? supportsVideoFrameReferences(modelName),
        refAudio: cost?.refAudio ?? supportsVideoAudioGeneration(modelName),
        genAudio: cost?.genAudio ?? supportsVideoAudioGeneration(modelName),
    };
}

// 一次拿全：参考/音频能力 + 可选项能力
export function resolveVideoModelFullCapabilities(modelName: string, cost?: AdminModelCost): VideoModelFullCapabilities {
    return {
        ...resolveVideoModelCapabilities(modelName, cost),
        options: getVideoModelOptions(modelName),
    };
}

export function supportsVideoFrameReferences(modelName: string) {
    const model = modelKey(modelName);
    return (
        model === "bytedance-seedance-2" ||
        model === "bytedance-seedance-2-fast" ||
        model === "bytedance-seedance-2-mini" ||
        model === "bytedance-seedance-2-5" ||
        model.includes("doubao-seedance-2-5") ||
        model.includes("doubao-seedance-2-0") ||
        model.includes("doubao-seedance-1-5") ||
        model.includes("doubao-seedance-1-0") ||
        // sd-* 是 Seedance 的渠道简写命名（如 sd-2.0-fast-720p ≡ seedance-2.0-fast）
        model.includes("sd-") ||
        model === "wan-2-7-image-to-video" ||
        model === "bytedance-v1-lite-image-to-video" ||
        model === "hailuo-02-image-to-video-standard" ||
        model === "hailuo-02-image-to-video-pro" ||
        model === "kling-v2-1-pro" ||
        model === "kling-v2-5-turbo-image-to-video-pro" ||
        // kling-v3 / kling-3-0 系列（含 omni 全名：kling-3-0-omni-720p / kling-3-0-omni-1080p / kling-3-0-motion-control 排除）
        (model.includes("kling-v3") || model.includes("kling-3-0")) && !model.includes("motion-control") ||
        model.includes("kling-3-0-omni") ||
        model === "minimax-h3-image-to-video" ||
        model === "minimax-h3" ||
        model === "happyhorse-1-1" ||
        (model.includes("veo3-1") && model.includes("official")) ||
        model.includes("minimax-hailuo-02") ||
        model.includes("skyreels-v4") ||
        model.includes("pixverse-v6") ||
        model.includes("viduq3") ||
        model.includes("vidu-q3")
    );
}

export function supportsVideoAudioGeneration(modelName: string) {
    const model = modelKey(modelName);
    if (model.includes("motion-control")) return false;
    return (
        model === "kling-2-6-text-to-video" ||
        model === "kling-2-6-image-to-video" ||
        model === "kling-text-to-video" ||
        model === "kling-image-to-video" ||
        model === "bytedance-seedance-2" ||
        model === "bytedance-seedance-2-fast" ||
        model === "bytedance-seedance-2-mini" ||
        model === "bytedance-seedance-2-5" ||
        model.includes("doubao-seedance-2-5") ||
        model.includes("doubao-seedance-2-0") ||
        model.includes("doubao-seedance-1-5") ||
        // sd-* 是 Seedance 的渠道简写命名，同步音频能力与 Seedance 一致
        model.includes("sd-") ||
        model.includes("bytedance-seedance-1-5") ||
        model === "wan-2-6-flash-image-to-video" ||
        model === "wan-2-6-flash-video-to-video" ||
        (model.includes("veo") && model.includes("official")) ||
        model === "wan2-6" ||
        model === "wan2-6-i2v-flash" ||
        model.includes("kling-v2-6") ||
        model.includes("kling-2-6") ||
        ((model.includes("kling-v3") || model.includes("kling-3-0")) && !model.includes("turbo")) ||
        model.includes("pixverse-v6") ||
        model.includes("viduq3-pro") ||
        model.includes("vidu-q3-pro") ||
        model.includes("viduq3-turbo")
    );
}

/**
 * 返回视频模型支持的最大时长（秒），用于"模型支持 15s 就默认 15s"的默认时长策略。
 * 模型切换时把默认时长设为该模型支持的最大值；业务上限统一为 15 秒。
 * 依据：seedanceDurationOptions、klingV3DurationOptions、normalizeVideoSecondsForModel 的各模型档位。
 */
export function getModelMaxDuration(modelName: string): number {
    const key = modelKey(modelName);
    // Seedance 系列（含渠道简写 sd-*）：档位 4/5/6/8/10/12/15，最大 15
    if (key.includes("seedance") || key.includes("doubao-seedance") || key.includes("sd-")) return 15;
    // Kling V3：档位 3/15，最大 15
    if (key.includes("kling-v3") || key.includes("kling-3-0")) return 15;
    // Kling 非 V3（2.6 等）：档位 5/10，最大 10
    if (key.includes("kling")) return 10;
    // Sora-2：档位 4/8/12/16/20，业务上限 15
    if (key.includes("sora-2")) return 15;
    // Veo3.1：固定 8
    if (key.includes("veo3-1") || key.includes("veo-3-1")) return 8;
    // 海螺 02：档位 5/10
    if (key.includes("minimax-hailuo-02")) return 10;
    // 海螺 2-3：档位 6/10
    if (key.includes("minimax-hailuo-2-3")) return 10;
    // Omni-Flash-Ext：档位 4/6/8/10
    if (key.includes("omni-flash-ext")) return 10;
    // Wan2.5：档位 5/10
    if (key.includes("wan2-5") || key.includes("wan2.5")) return 10;
    // Wan2.6：档位 5/10/15
    if (key === "wan2-6") return 15;
    // 默认（grok-imagine-video / agnes-video 等）：走通用 1~30，业务上限 15
    return 15;
}

// ═══════════════════════════════════════════════════════════════════
// 模型 → 可选项能力映射
// ═══════════════════════════════════════════════════════════════════

const ALL_ASPECT_RATIOS = ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"] as const;
const KLING_RATIOS = ["16:9", "9:16", "1:1"] as const; // Kling V2.6/V3 一致

// resolution 内部统一用「无 p」规范形态（"720"/"1080"/"2k"/"4k"），与配置存的 vquality 对齐；
// 接 Seedance 系列（存的 "720p" 带 p）时由 filter 用 normalizeResolutionKey 兼容。
const RES_480 = "480";
const RES_720 = "720";
const RES_1080 = "1080";
const RES_2K = "2k";
const RES_4K = "4k";

const ALL_RESOLUTIONS_5K = [RES_480, RES_720, RES_1080, RES_2K, RES_4K] as const;
const STANDARD_RESOLUTIONS_3K = [RES_480, RES_720, RES_1080] as const;
const NO_4K_RESOLUTIONS = [RES_480, RES_720, RES_1080, RES_2K] as const;
const KLING_RESOLUTIONS = [RES_720, RES_1080] as const;
const KLING_V3_RESOLUTIONS = [RES_720, RES_1080, RES_4K] as const;
const SEEDANCE_RESOLUTIONS = [RES_480, RES_720, RES_1080] as const;

const SEEDANCE_DURATIONS = [4, 5, 6, 8, 10, 12, 15] as const;
const KLING_V26_DURATIONS = [5, 10] as const;
const KLING_V3_DURATIONS = [3, 15] as const;
const VEO31_DURATIONS = [8] as const;
const SORA2_DURATIONS = [4, 8, 12, 16, 20] as const;
const HAILUO_02_DURATIONS = [5, 10] as const;
const HAILUO_2_3_DURATIONS = [6, 10] as const;
const WAN25_DURATIONS = [5, 10] as const;
const WAN26_DURATIONS = [5, 10, 15] as const;
const WAN26_FLASH_DURATIONS = [5, 10] as const;
const STANDARD_DURATIONS = [5, 6, 8, 10, 12] as const;

function seedanceOptions(): VideoModelOptions {
    return {
        resolutions: [...SEEDANCE_RESOLUTIONS],
        durations: [...SEEDANCE_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 15,
    };
}

function seedanceFastOrMiniOptions(): VideoModelOptions {
    return {
        // fast / mini 没有 1080p
        resolutions: [RES_480, RES_720],
        durations: [...SEEDANCE_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 15,
    };
}

function klingV26Options(): VideoModelOptions {
    return {
        resolutions: [...KLING_RESOLUTIONS],
        durations: [...KLING_V26_DURATIONS],
        aspectRatios: KLING_RATIOS,
        maxDuration: 10,
    };
}

function klingV3Options(): VideoModelOptions {
    return {
        resolutions: [...KLING_V3_RESOLUTIONS],
        durations: [...KLING_V3_DURATIONS],
        aspectRatios: KLING_RATIOS,
        maxDuration: 15,
    };
}

function veo31Options(): VideoModelOptions {
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...VEO31_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 8,
    };
}

function sora2Options(): VideoModelOptions {
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...SORA2_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 15,
    };
}

function hailuoOptions(durations: readonly number[]): VideoModelOptions {
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...durations],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: getModelMaxDuration("minimax-hailuo-02"),
    };
}

function hailuo23Options(): VideoModelOptions {
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...HAILUO_2_3_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 10,
    };
}

function wan25Options(): VideoModelOptions {
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...WAN25_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 10,
    };
}

function wan26Options(): VideoModelOptions {
    return {
        resolutions: [...NO_4K_RESOLUTIONS],
        durations: [...WAN26_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 15,
    };
}

function wan26FlashOptions(): VideoModelOptions {
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...WAN26_FLASH_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 10,
    };
}

function standardOptions(): VideoModelOptions {
    // grok-imagine / happyhorse-1-1 / pixverse-v6 / vidu-q3 / skyreels / agnes-video 等
    return {
        resolutions: [...STANDARD_RESOLUTIONS_3K],
        durations: [...STANDARD_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 15,
    };
}

function premiumOptions(): VideoModelOptions {
    // 4K 通吃的：顶级旗舰（如 sora / 某些官方 veo）
    return {
        resolutions: [...ALL_RESOLUTIONS_5K],
        durations: [...STANDARD_DURATIONS],
        aspectRatios: ALL_ASPECT_RATIOS,
        maxDuration: 15,
    };
}

function motionControlOptions(): VideoModelOptions {
    return {
        // 动作控制类，时长档与画面比例受限（多数 kling-motion-control）
        resolutions: [...KLING_RESOLUTIONS],
        durations: [...KLING_V26_DURATIONS],
        aspectRatios: KLING_RATIOS,
        maxDuration: 10,
    };
}

// 白名单登记表
const OPTIONS_TABLE: Array<{ test: (k: string) => boolean; cap: () => VideoModelOptions }> = [
    // Seedance 全系列（含渠道简写 sd-*，fast / mini 由下一行单独再 clamp）
    { test: (k) => k.includes("seedance") || k.includes("doubao-seedance") || k.includes("sd-"), cap: seedanceOptions },
    // Seedance fast / mini（含 sd-*-fast / sd-*-mini）：走窄一档
    { test: (k) => (k.includes("seedance") || k.includes("doubao-seedance") || k.includes("sd-")) && (k.includes("fast") || k.includes("mini")), cap: seedanceFastOrMiniOptions },
    // Kling V3
    { test: (k) => k.includes("kling-v3") || k.includes("kling-3-0"), cap: klingV3Options },
    // Kling V2.6
    { test: (k) => k.includes("kling-v2-6") || k.includes("kling-2-6"), cap: klingV26Options },
    // Kling 非 V3 非 V2.6：通用 kling 5/10 档
    { test: (k) => k.includes("kling"), cap: klingV26Options },
    // Veo3.1
    { test: (k) => k.includes("veo3-1") || k.includes("veo-3-1"), cap: veo31Options },
    // Veo 系列（非 3.1）：按标准
    { test: (k) => k.includes("veo"), cap: standardOptions },
    // Sora-2
    { test: (k) => k.includes("sora-2"), cap: sora2Options },
    // Sora 其它：标准
    { test: (k) => k.includes("sora"), cap: sora2Options },
    // 海螺 02
    { test: (k) => k.includes("minimax-hailuo-02"), cap: () => hailuoOptions(HAILUO_02_DURATIONS) },
    // 海螺 2.3
    { test: (k) => k.includes("minimax-hailuo-2-3"), cap: hailuo23Options },
    // 海螺其它：标准
    { test: (k) => k.includes("hailuo") || k.includes("minimax"), cap: () => hailuoOptions(HAILUO_02_DURATIONS) },
    // Wan2.5
    { test: (k) => k.includes("wan2-5") || k.includes("wan2.5"), cap: wan25Options },
    // Wan2.6 flash
    { test: (k) => k.includes("wan-2-6-flash") || k.includes("wan2-6-i2v-flash") || k.includes("wan2-6-i2v-flash"), cap: wan26FlashOptions },
    // Wan2.6 顶级
    { test: (k) => k === "wan2-6" || k === "wan-2-6" || k === "wan/2-6", cap: wan26Options },
    // Wan2.7 图片生成视频：通吃
    { test: (k) => k.includes("wan2-7") || k.includes("wan2.7") || k.includes("wan/2-7"), cap: premiumOptions },
    // Omni-Flash 系列（gemini-omni-flash 视频）：标准
    { test: (k) => k.includes("omni-flash"), cap: standardOptions },
    // 动作控制
    { test: (k) => k.includes("motion-control"), cap: motionControlOptions },
    // happyhorse / skyreels / pixverse-v6 / vidu-q3
    { test: (k) => k.includes("happyhorse") || k.includes("skyreels") || k.includes("pixverse") || k.includes("vidu") || k.includes("viduq"), cap: standardOptions },
    // grok-imagine 视频
    { test: (k) => k.includes("grok-imagine") && k.includes("video"), cap: standardOptions },
    // agnes-video
    { test: (k) => k.includes("agnes-video"), cap: standardOptions },
    // hunyuan-video / cogvideo / lumina：标准
    { test: (k) => k.includes("hunyuan-video") || k.includes("cogvideo") || k.includes("lumina"), cap: standardOptions },
];

export function getVideoModelOptions(modelName: string): VideoModelOptions | null {
    const k = modelKey(modelName);
    if (!k) return null;
    for (const row of OPTIONS_TABLE) {
        if (row.test(k)) return lockResolutionIfNamed(row.cap(), k);
    }
    return null;
}

// 模型名把分辨率焊死（如 seedance-2.0-1080p / kling-3.0-omni-720p / seedance-2.5-480p /
// seedance-2.0-fast-431-480p）：官方云端走 ai.go 的 OpenAI 直转，没有后端归一化，
// 只暴露该分辨率，避免用户选了模型名不支持的档位被上游拒绝。
function lockResolutionIfNamed(cap: VideoModelOptions, k: string): VideoModelOptions {
    const m = k.match(/-(\d+)p$/);
    if (!m) return cap;
    const fixed = m[1];
    if (!cap.resolutions.map(normalizeResolutionKey).includes(fixed)) return cap;
    return { ...cap, resolutions: [fixed] };
}

// 当前选中的分辨率（"720"/"1080"/"720p"/"2k"/"4k" 等）不在模型允许集合 → 落到首项。
// 未识别模型（null）→ 原样返回。canonical 形态：去 p、去空格、小写。
function normalizeResolutionKey(value: string): string {
    return String(value || "").trim().toLowerCase().replace(/p$/i, "");
}
export function normalizeVideoResolutionForModel(modelName: string, current: string): string {
    const opts = getVideoModelOptions(modelName);
    if (!opts) return current;
    const normalized = normalizeResolutionKey(current);
    const allowedKeys = opts.resolutions.map(normalizeResolutionKey);
    if (allowedKeys.includes(normalized)) return current;
    // 兜底：保留输入是否带 p 的格式
    const fallback = opts.resolutions[0];
    return /p$/i.test(String(current).trim()) && !/p$/i.test(fallback) ? `${fallback}p` : fallback;
}

// 当前选中的秒数（如 "6"、"15"、"-1"）不在模型允许集合 → 落到首项。
// "-1"（智能）只对 Seedance 等可选项启用，其它模型回退到首个合法值。
export function normalizeVideoSecondsForModel(modelName: string, current: string): string {
    const opts = getVideoModelOptions(modelName);
    if (!opts) return current;
    if (String(current).trim() === "-1") return current; // 智能档保持透传
    const num = Math.floor(Number(current));
    if (!Number.isFinite(num)) return String(opts.durations[0]);
    if (opts.durations.includes(num)) return String(num);
    // 选最大的合法档作为兜底（避免超模型允许范围）
    const fallback = [...opts.durations].sort((a, b) => a - b).find((d) => d >= num) ?? opts.durations[0];
    return String(fallback);
}

// 当前选中的 size（"16:9" / "1280x720" / "auto"）：若为基础比例且不在白名单，回退到首项。
export function normalizeVideoAspectRatioForModel(modelName: string, current: string): string {
    const opts = getVideoModelOptions(modelName);
    if (!opts) return current;
    if (current === "auto" || current === "adaptive") return current;
    const allowed = new Set(opts.aspectRatios);
    if (allowed.has(current)) return current;
    // 用户输入了自定义 WxH 不在比例白名单里也保留
    if (/^\d+x\d+$/.test(current)) return current;
    return opts.aspectRatios[0] || "16:9";
}

// 通用辅助：把基础 option 列表过滤到模型允许的子集。
export function filterVideoResolutionOptions<T extends { value: string }>(options: readonly T[], modelName: string): T[] {
    const opts = getVideoModelOptions(modelName);
    if (!opts) return [...options];
    const allowed = new Set(opts.resolutions.map(normalizeResolutionKey));
    return options.filter((item) => allowed.has(normalizeResolutionKey(item.value)));
}

export function filterVideoAspectRatioOptions<T extends { value: string }>(options: readonly T[], modelName: string): T[] {
    const opts = getVideoModelOptions(modelName);
    if (!opts) return [...options];
    const allowed = new Set(opts.aspectRatios);
    return options.filter((item) => allowed.has(item.value));
}

export function filterVideoDurationOptions<T extends { value: number | string }>(options: readonly T[], modelName: string): T[] {
    const opts = getVideoModelOptions(modelName);
    if (!opts) return [...options];
    const allowed = new Set(opts.durations);
    // -1（智能）选项总是透传：白名单说明里只看具体秒数
    return options.filter((item) => {
        const v = item.value;
        if (v === -1) return true;
        if (typeof v === "number") return allowed.has(v);
        const num = Math.floor(Number(v));
        return Number.isFinite(num) && allowed.has(num);
    });
}
