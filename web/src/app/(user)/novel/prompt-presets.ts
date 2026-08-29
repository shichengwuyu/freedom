import { PRESETS, PRESET_KEYS, DEFAULT_SCRIPT_PROMPT, DEFAULT_VIDEO_PROMPT, DEFAULT_ASSET_PROMPT } from "./constants";

/**
 * 取预设的某段 prompt：preset 值为空时回退到 DEFAULT_*（避免前向引用）。
 * 用在 UI 上「点预设 → 一次性填三段」的逻辑。
 */
export function getEffectivePresetValue(key: string, type: "scriptPrompt" | "videoPrompt" | "assetPrompt"): string {
    const preset = PRESETS[key];
    if (!preset) return "";
    const value = preset[type];
    if (value) return value;
    if (key === "general") {
        if (type === "scriptPrompt") return DEFAULT_SCRIPT_PROMPT;
        if (type === "videoPrompt") return DEFAULT_VIDEO_PROMPT;
        return DEFAULT_ASSET_PROMPT;
    }
    return "";
}

/** 当前 prompt 是否匹配某个预设（用于 UI 上高亮当前选中的 preset tab） */
export function matchPreset(text: string, type: "scriptPrompt" | "videoPrompt" | "assetPrompt"): string | null {
    for (const key of PRESET_KEYS) {
        if (getEffectivePresetValue(key, type) === text) return key;
    }
    return null;
}

// 简易 prompt 校验（P1 改造 D）：基于 prompt 文本检测常见错误，给 UI 黄色警告。
//  这些只是提示，不强制阻断保存 —— 因为有的人故意不要某些规则。
export function validatePrompt(tabKey: "script" | "video" | "image", text: string): { level: "ok" | "warn"; warnings: string[] } {
    if (!text || !text.trim()) return { level: "warn", warnings: ["⚠ prompt 为空，使用代码默认 DEFAULT_*"] };
    const warnings: string[] = [];
    // 通用校验
    if (text.length < 80) warnings.push("⚠ prompt 过短（< 80 字），可能过于简略");
    if (text.length > 3000) warnings.push("⚠ prompt 过长（> 3000 字），会显著增加 token 用量");

    if (tabKey === "script") {
        // 分镜剧本 prompt 应该含：对话原文 / @角色 / 时间段分层 / 出场角色 等关键词
        if (!/对话[^]{0,20}原文|100%\s*原文|原文.*零更改/.test(text)) warnings.push("⚠ 缺少「对话 100% 原文」约束（容易被 LLM 改写对话）");
        if (!/@角色|@角色名|@[一-龥]/.test(text)) warnings.push("⚠ 缺少「@角色名」标签约束（无法自动关联角色资产）");
        if (!/(0-\d+?s｜|0-\d+?s\|).*?运镜|时间段/.test(text)) warnings.push("⚠ 缺少「时间段分层运镜」要求（容易出现一镜到底）");
        if (!/剧情全[覆复].*关键|关键剧情|动作.*对话|关键转折/.test(text)) warnings.push("⚠ 缺少「关键剧情全覆盖」约束（容易丢关键剧情）");
    } else if (tabKey === "video") {
        if (!/对话[^]{0,20}原文|100%\s*原文|原文.*零更改/.test(text)) warnings.push("⚠ 缺「对话 100% 原文」约束");
        if (!/运镜[^]{0,30}(变化|丰富|变化)|至少\s*2\s*个运镜|运镜/.test(text)) warnings.push("⚠ 缺少「多运镜变化」约束");
        if (!/位置关系|位置|站位|左侧|右侧|身后|面前/.test(text)) warnings.push("⚠ 缺少「角色位置关系」要求");
        if (!/动作.{0,15}连贯|连贯变化|动作连贯/.test(text)) warnings.push("⚠ 缺少「动作连贯性」要求");
    } else {
        // asset prompt 三段必须都有
        if (!/【角色三视图】/.test(text)) warnings.push("⚠ 缺【角色三视图】段模板");
        if (!/【场景四宫格】/.test(text)) warnings.push("⚠ 缺【场景四宫格】段模板");
        if (!/【道具标准图】/.test(text)) warnings.push("⚠ 缺【道具标准图】段模板");
    }
    return { level: warnings.length > 0 ? "warn" : "ok", warnings };
}
