// ─────────────────────────── Types ───────────────────────────

export type AssetType = "character" | "scene" | "prop" | "reference";

export interface Asset {
    id: string;
    name: string;
    alias: string;
    type: AssetType;
    dataUrl: string;
    description: string;
    /** 上传到存储服务后返回的可公开访问 URL（未上传时为空，dataUrl 可能是 blob: 或 base64） */
    url?: string;
    /** 存储服务返回的 storageKey（server: 前缀走服务端，image: 前缀走本地 localforage） */
    storageKey?: string;
}

export interface Shot {
    id: string;
    title: string;
    content: string;
    duration: number;
    status: "idle" | "generating" | "success" | "failed";
    error?: string;
    videoUrl?: string;
    /** 视频本地 storageKey，优先用于读取避免 CORS */
    videoStorageKey?: string;
    progress?: number;
    customPrompt?: string;
    selected: boolean;
    firstFrameAssetId?: string;
    lastFrameAssetId?: string;
    referencedAssetIds: string[];
    /** 该分镜对应的分镜剧本 id（一个分镜剧本 → 一个视频） */
    storyboardId?: string;
    /** 生成时快照的视频模型名（详情展示，避免全局配置变化后信息不准） */
    videoModel?: string;
    /** 生成时快照的画面比例，如 "16:9" */
    aspectRatio?: string;
    /** 生成时快照的分辨率档位，如 "720p" */
    resolution?: string;
    /** 生成时快照的像素尺寸，如 "1280x720" */
    size?: string;
    /** 后端视频任务 ID（用于切页恢复轮询） */
    videoTaskId?: string;
}

/** 原始小说章节（中间栏目录式展示） */
export interface Chapter {
    id: string;
    /** 章节标题，如"第1章 xxx" */
    title: string;
    /** 该章节原始正文（不含标题行） */
    content: string;
    /** P1 b1/b2：本章希望切成几个分镜（默认 1；>=2 时后端 prompt 让模型输出 N 条用 ===SHOT=== 分隔） */
    shotCount?: number;
}

/**
 * 分镜剧本（一个分镜剧本 = 一章整合成的一条完整视频描述词，内部按时间段分层，总时长约 15s）。
 * 一章一条：每 1 章发给文本模型整合产出 1 条分镜剧本。
 * 全局连续编号（分镜1、分镜2…），增/删/重排后统一重排编号，不使用章节名。
 */
export interface Storyboard {
    id: string;
    /** 属于第几组（每 1 章一组，从0开始，等价于第几章） */
    groupIndex: number;
    /** 同组内的分镜序号（一组多条时有值；0时对老数据兼容） */
    shotIndex?: number;
    /** 分镜剧本正文（可直接送去生成视频的镜头描述） */
    content: string;
    /**
     * 单条分镜的业务状态（取代老的"⚠"占位字符串）：
     *   - 未设置 / "completed"：真实可生成视频的分镜；
     *   - "failed"：本条生成失败，content 为空，error 保存失败原因；前端用 s.shotStatus !== "failed" 过滤。
     * 老数据/旧本地兜底可能还在 content 里塞"⚠..."，前端读时统一被 parseStoryboardTaskResult 归一化；
     * 这里给前端 Storyboard 也保留同样的字段以便 UI 正确显示。
     */
    shotStatus?: "completed" | "failed";
    shotError?: string;
    /** 该分镜对应视频的生成状态与产物；queued=已入队等待并发名额 */
    videoStatus?: "idle" | "queued" | "generating" | "success" | "failed";
    videoUrl?: string;
    videoError?: string;
}

export interface NovelProject {
    id: string;
    name: string;
    script: string;
    shots: Shot[];
    /** 按章节解析出的原始小说目录（仅用于中间栏展示） */
    chapters?: Chapter[];
    /** 分镜剧本列表（右侧平铺展示，全局连续编号） */
    storyboards?: Storyboard[];
    /** 当前分镜生成任务 ID（后端任务化后，刷新可据此时轮询恢复进度） */
    storyboardTaskId?: string;
    /** 任务结果索引 → 原始组索引的映射（刷新后恢复轮询时重建 Storyboard 列表用） */
    storyboardGroupMap?: number[];
    /** 资产生图后端任务 ID 列表（用于切页恢复轮询） */
    assetImageTaskIds?: string[];
    createdAt: number;
    updatedAt: number;
}

export type PresetEntry = { label: string; description: string; scriptPrompt: string; videoPrompt: string; assetPrompt: string };

/**
 * 一键出片三态（替代老的两态 `boolean`）：
 *   - off  ：手动模式，每一步独立按钮
 *   - half ：分镜剧本完成后弹"是否继续生图并出片"——给老用户控制节奏
 *   - full ：全自动链（开始分镜 → 抽资产 → 生图 → 校验匹配 → 出片），由「剧本转视频」流程期望承接
 */
export type AutoPilotMode = "off" | "half" | "full";

/**
 * 分镜匹配校验报告：所有分镜里的 @角色 在 assets 表是否有对应图片。
 * `missingByBoard` 记录每个未匹配分镜的角色清单；用于生成视频前的硬校验闸门。
 */
export interface StoryboardCoverageReport {
    /** 有未匹配角色的分镜数（不是未匹配角色总数） */
    boardsWithMissing: number;
    /** 未匹配的 @角色 总数（涉及所有分镜去重前） */
    totalMissing: number;
    /** storyboardId → 未匹配角色名数组（用于 UI 高亮 + 跳转定位） */
    missingByBoard: Map<string, string[]>;
}
