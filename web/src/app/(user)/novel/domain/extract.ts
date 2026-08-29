import type { Asset, Storyboard } from "../types";

// storyboardIsReady 分镜是否"真实可生成视频"：shotStatus≠"failed" 且 content 非空。
// 同时兼容历史/本地兜底仍以"⚠"开头的内容：parseStoryboardTaskResult 已经做归一化；前端 Storyboard 也带 shotStatus。
export function storyboardIsReady(s: Storyboard): boolean {
    if (s.shotStatus === "failed") return false;
    if (!s.content || !s.content.trim()) return false;
    if (s.content.trimStart().startsWith("⚠")) return false; // 兜底：老数据未迁移
    return true;
}

export function extractMentions(text: string): string[] {
    // 只从分镜剧本开头的"出场角色 / 场景"两行提取名字
    // 例：出场角色：@沈一寻；@段嘉怡 → [沈一寻, 段嘉怡]（自动去除@前缀以匹配 asset.alias）
    const names = new Set<string>();
    const addFromLine = (regex: RegExp) => {
        const m = text.match(regex);
        if (!m) return;
        const items = m[1]
            .split(/[，,、;；\s]+/)
            .map((s) => s.trim().replace(/^@/, ""))
            .filter(Boolean);
        for (const it of items) names.add(it);
    };
    addFromLine(/出场角色[：:]\s*(.+?)(?:\n|$)/);
    addFromLine(/场景[：:]\s*(.+?)(?:\n|$)/);
    return [...names];
}

// 解析分镜剧本头部的"出场角色"和"场景"行
// 返回 { characters: [名字数组], scenes: [名字数组] }
export function extractStoryboardHeader(text: string): { characters: string[]; scenes: string[] } {
    const parseLine = (regex: RegExp): string[] => {
        const m = text.match(regex);
        if (!m) return [];
        return m[1].split(/[，,、;；\s]+/).map((s) => s.trim().replace(/^@/, "")).filter(Boolean);
    };
    return {
        characters: parseLine(/出场角色[：:]\s*(.+?)(?:\n|$)/),
        scenes: parseLine(/场景[：:]\s*(.+?)(?:\n|$)/),
    };
}

export function resolveMentions(text: string, assets: Asset[]): string {
    let result = text;
    for (const asset of assets) {
        if (asset.alias) {
            const desc = asset.description
                ? `${asset.alias}（参考：${asset.name}，${asset.description}）`
                : asset.alias;
            result = result.replace(
                new RegExp(`@${asset.alias.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`, "g"),
                desc,
            );
        }
    }
    return result;
}

/**
 * 检查分镜剧本内容是否看起来像正确的视频描述脚本（而非原小说文本）。
 * 正确的分镜脚本应包含：出场角色/场景头、时间段分层（如 0-4s｜）、@角色名 引用。
 * 返回 { isLikelyStoryboard, reasons: string[] } — reasons 为空时表示格式正确。
 */
export function validateStoryboardContent(content: string): { isLikelyStoryboard: boolean; reasons: string[] } {
    const reasons: string[] = [];
    if (!content || !content.trim()) {
        return { isLikelyStoryboard: false, reasons: ["分镜内容为空"] };
    }
    const text = content.trim();

    // 检查是否有"出场角色"行
    const hasCharacterHeader = /出场角色[：:]/.test(text);
    if (!hasCharacterHeader) reasons.push('缺少"出场角色："行');

    // 检查是否有"场景"行
    const hasSceneHeader = /场景[：:]/.test(text);
    if (!hasSceneHeader) reasons.push('缺少"场景："行');

    // 检查是否有时间段标记（如 0-4s｜）
    const hasTimeSegment = /\d+\s*[-~到]\s*\d+\s*秒?[sS]?[｜|]/.test(text);
    if (!hasTimeSegment) reasons.push("缺少时间段分层（如 0-4s｜）");

    // 检查是否有@角色引用
    const hasMention = /@[\u4e00-\u9fff\w]+/.test(text);
    if (!hasMention) reasons.push("缺少@角色名引用");

    // 检查内容是否过于像原小说文本（段落无格式、无特殊标记）
    const formatMarkers = [/出场角色[：:]/, /场景[：:]/, /@[\u4e00-\u9fff\w]+/, /\d+\s*[-~到]\s*\d+\s*秒?[sS]?[｜|]/];
    const markerCount = formatMarkers.filter((r) => r.test(text)).length;

    // 至少需要 2 个格式标记才算合格
    const isLikelyStoryboard = markerCount >= 2;

    return { isLikelyStoryboard, reasons };
}
