// 分镜剧本（小说章节 → 单条分镜视频描述词）的提示词构造。
// 前后端共用：service/handler 端 executeStoryboardTask 内嵌同语义的 Go 副本；
// 前端 handleParseStoryboardsLocal（断网/未登录兜底）调用本模块保持一致。
// 改动提示词时务必同步 handler/storyboard_task.go 内 executeStoryboardTask 的同段 user prompt。

export interface StoryboardAssetInput {
    alias: string;
    type: "character" | "scene" | "prop" | "reference";
    description: string;
    name: string;
}

const ASSET_TYPE_LABEL: Record<StoryboardAssetInput["type"], string> = {
    character: "角色",
    scene: "场景",
    prop: "道具",
    reference: "参考",
};

/**
 * 把 shotDuration 中的"15秒"等硬编码替换为实际秒数；不限 15 以下，> 15 也能替换（如 30 秒长分镜）。
 */
export function renderStoryboardPrompt(basePrompt: string, shotDuration: number): string {
    const dur = Math.max(1, Math.min(120, Math.round(shotDuration) || 8));
    return basePrompt.replace(/15\s*秒/g, `${dur}秒`);
}

/**
 * 构造"角色/场景/道具参考文档"段（注入到 user content 顶部）。
 * 与 handler/handler/storyboard_task.go 的 assetRefSection 段落一一对应。
 */
export function buildStoryboardAssetRefSection(assets: StoryboardAssetInput[]): string {
    if (assets.length === 0) return "";
    const lines = [
        `【角色/场景/道具参考文档】（以下资产的名称用于"出场角色/场景"行引用，描述用于了解外观，分镜描述必须严格参考）：`,
    ];
    for (const a of assets) {
        const label = ASSET_TYPE_LABEL[a.type] ?? "道具";
        const desc = a.description || a.name;
        lines.push(`- ${a.alias}（${label}）：${desc}`);
    }
    lines.push(`注意：出场角色/场景行中的名称必须与上述列表一致，角色外观、服装、场景描述需严格参考上述描述，不可自行编造。`, "");
    return lines.join("\n");
}

/**
 * 构造单章 user content。一章 N 条（默认 1）：模型整合整章剧情为 ShotCount 条分镜，>1 时用 ===SHOT=== 分隔。
 * 与 handler/storyboard_task.go 的 userContentParts 段落一一对应；更改需同步 Go。
 */
export function buildStoryboardUserContent(params: {
    chapterLabel: string;
    chapterText: string;
    assets: StoryboardAssetInput[];
    shotDuration: number;
    shotCount?: number; // P1 b1/b2：缺省/0/1 = 一条；>=2 时 prompt 引导模型输出 N 条
}): string {
    const dur = Math.max(1, Math.min(120, Math.round(params.shotDuration) || 8));
    const assetRef = buildStoryboardAssetRefSection(params.assets);
    const shotCount = Math.max(1, params.shotCount ?? 1);
    const multiShotHint = shotCount > 1
        ? `- 【分镜数】本章需要拆成 ${shotCount} 条分镜。请输出 ${shotCount} 条独立分镜，每条之间用 3 个连续的 "===SHOT==="（英文等号）独占一行分隔：`
        : "";
    const parts: (string | undefined)[] = [];
    if (assetRef) parts.push(assetRef);
    parts.push(
        `以下是小说${params.chapterLabel}的正文，请你作为导演把这一整章剧情【整合成 ${shotCount} 条分镜视频描述词】：`,
        multiShotHint,
        `- 总时长不超过 ${dur} 秒；用 0-Xs｜、Xs-Ys｜ 这样的时间段把镜头在同一条描述内部自然分层，每个时间段是一次运镜/一个机位，至少 2 个运镜段；`,
        `- 开头单独两行标注「出场角色：角色1；角色2；角色3」和「场景：场景1；场景2」，随后用 { } 包裹按时间段展开的运镜描述；`,
        `- 详细描述画面构图、人物位置关系、连贯的人物动作、台词、运镜、光影，按剧情推进自然衔接，承接上一章；`,
        `- 台词零更改一字不动，说话/动作/表情前必须用"@角色名"的形式标注，以便系统自动关联角色资产；不要遗漏关键情节，也不要凭空添加剧本外的剧情；`,
        `- 【重要】角色外观、服装、场景描述必须严格参考上方【角色/场景/道具参考文档】中的描述，不可自行编造。`,
        `- 只输出分镜描述词本身，不要输出解释、总结或额外文字。`,
        ``,
        params.chapterText,
    );
    return parts.filter((p) => p !== undefined && p !== "").join("\n");
}

/**
 * 清理模型输出开头残留的【分镜N】/场景N/镜头N 起始标记。
 * 与 handler/storyboard_task.go 的 storyboardLeadingLabelRegex 对齐。
 */
const STORYBOARD_LEADING_LABEL_REGEX = /^\s*【?\s*(?:场景|分镜|镜头|视频|Shot|Scene)\s*\d+\s*】?\s*[:：]?\s*/i;

export function cleanStoryboardLeadingLabel(draft: string): string {
    return STORYBOARD_LEADING_LABEL_REGEX.test(draft)
        ? draft.replace(STORYBOARD_LEADING_LABEL_REGEX, "").trim()
        : draft.trim();
}