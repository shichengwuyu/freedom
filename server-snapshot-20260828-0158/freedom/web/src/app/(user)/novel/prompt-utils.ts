import { getModelMaxDuration } from "@/lib/video-model-capabilities";

/**
 * 根据视频模型的最大时长，动态替换提示词中的"15秒"引用。
 * 分镜.txt（DEFAULT_VIDEO_PROMPT）和 DEFAULT_SCRIPT_PROMPT 中包含多处"15秒"硬编码，
 * 需要根据当前选中视频模型的支持时长动态调整，避免模型输出超出能力范围的时长描述。
 */
export function makePromptDynamic(basePrompt: string, modelName: string): string {
    const maxDur = getModelMaxDuration(modelName);
    if (!maxDur || maxDur >= 15) return basePrompt;
    return basePrompt
        .replace(/15\s*秒/g, `${maxDur}秒`)
        .replace(/不超过\s*15\s*秒/g, `不超过 ${maxDur}秒`)
        .replace(/在\s*15\s*秒以/g, `在 ${maxDur}秒以`)
        .replace(/不可超过\s*15\s*秒/g, `不可超过 ${maxDur}秒`)
        .replace(/总时长xx秒整/g, `总时长${maxDur}秒整`);
}
