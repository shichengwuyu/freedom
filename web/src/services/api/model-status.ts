// services/api/model-status.ts
// 模型性能与健康状态 API 封装（对应后端 GET /api/model-status）。
// 后台定时任务每 15 分钟刷新一次缓存，前端每 5 分钟轮询一次拉取最新快照。
// 模块级缓存 + 订阅模式：多个 ModelPicker 实例共享同一份数据，避免重复请求。

import { useEffect, useState, type CSSProperties } from "react";
import { apiGet } from "./request";

// 模型健康度分级（与后端 service.classifyModelHealth 对应）
export type ModelHealth = "normal" | "unstable" | "down" | "unknown";

// 单个模型的性能指标快照
export type ModelStatusItem = {
    model_name: string; // 模型名（与前端下拉框 model 一致）
    avg_latency_ms: number; // 平均生成延迟（毫秒）
    avg_ttft_ms: number; // 平均首字延迟（毫秒）
    success_rate: number; // 成功率（百分比 0-100）
    avg_tps: number; // 平均每秒 token 数
    health: ModelHealth; // 健康度分级
};

// 全部模型性能汇总，含最后刷新时间
export type ModelStatusSummary = {
    models: ModelStatusItem[]; // 全部模型状态列表
    updated_at: number; // Unix 秒，最后一次成功刷新时间
};

// 模块级缓存：多个 ModelPicker 实例共享同一份数据
let cache: ModelStatusSummary | null = null;
let inflight: Promise<ModelStatusSummary | null> | null = null;
let pollingStarted = false;
const subscribers = new Set<(data: ModelStatusSummary | null) => void>();

// 拉取一次模型状态（带请求去重，并发调用只发一个请求）
async function loadModelStatus(): Promise<ModelStatusSummary | null> {
    if (inflight) return inflight;
    inflight = (async () => {
        try {
            const data = await apiGet<ModelStatusSummary>("/api/model-status");
            cache = data;
            return data;
        } catch (err) {
            console.warn("fetch model status failed", err);
            return cache;
        } finally {
            inflight = null;
        }
    })();
    return inflight;
}

// 启动全局轮询（幂等，整个应用只启动一次，5 分钟拉一次后端缓存）
function startPolling() {
    if (pollingStarted) return;
    pollingStarted = true;
    const tick = async () => {
        const prev = cache;
        const data = await loadModelStatus();
        if (data && data !== prev) {
            subscribers.forEach((fn) => fn(data));
        }
    };
    tick();
    setInterval(tick, 5 * 60 * 1000);
}

// React hook：订阅模型状态变化，返回最新快照。
// 多个组件挂载时共享同一个轮询定时器和缓存，卸载时自动取消订阅。
export function useModelStatus(): ModelStatusSummary | null {
    const [status, setStatus] = useState<ModelStatusSummary | null>(cache);
    useEffect(() => {
        startPolling();
        const fn = (data: ModelStatusSummary | null) => setStatus(data);
        subscribers.add(fn);
        // 首次挂载时若已有缓存，同步一次（避免初始 null 态闪烁）
        if (cache && !status) setStatus(cache);
        return () => {
            subscribers.delete(fn);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    return status;
}

// 根据模型名查健康度（找不到返回 unknown）
export function getModelHealth(modelName: string, status: ModelStatusSummary | null): ModelHealth {
    if (!status || !Array.isArray(status.models)) return "unknown";
    // 精确匹配
    let item = status.models.find((m) => m.model_name === modelName);
    if (!item) {
        // 尝试匹配短名（去掉提供商前缀，如 "openai/gpt-oss-20b" -> "gpt-oss-20b"）
        const shortName = modelName.includes("/") ? modelName.split("/").pop() : modelName;
        item = status.models.find((m) => m.model_name === shortName);
    }
    if (!item) {
        // 反向匹配：API 中的名可能是短名，尝试把 API 名作为短名匹配
        item = status.models.find((m) => {
            if (!m.model_name.includes("/")) return false;
            return m.model_name.split("/").pop() === modelName || m.model_name.split("/").pop() === (modelName.includes("/") ? modelName.split("/").pop() : modelName);
        });
    }
    return item?.health ?? "unknown";
}

// 健康度对应的圆点颜色类（Tailwind）
export function healthColorClass(health: ModelHealth): string {
    switch (health) {
        case "normal":
            return "bg-emerald-500";
        case "unstable":
            return "bg-amber-500";
        case "down":
            return "bg-rose-500";
        default:
            // unknown 视为不可用，同样用红色标记
            return "bg-rose-500";
    }
}

// 健康度对应的中文标签
export function healthText(health: ModelHealth): string {
    switch (health) {
        case "normal":
            return "正常";
        case "unstable":
            return "不稳定";
        case "down":
            return "故障";
        default:
            // unknown 视为不可用
            return "不可用";
    }
}

// 健康度颜色映射（单一事实来源，供 healthDotStyle / resolveModelHealthDisplay 复用）
export const HEALTH_COLOR_MAP: Record<ModelHealth, string> = {
    normal: "#10b981",
    unstable: "#f59e0b",
    down: "#f43f5e",
    unknown: "#f43f5e", // unknown 视为不可用，红色
};

// 健康度对应的圆点内联样式（避免 Tailwind JIT purge 动态类名）
export function healthDotStyle(health: ModelHealth): CSSProperties {
    return {
        display: "inline-block",
        width: 12,
        height: 12,
        minWidth: 12,
        minHeight: 12,
        borderRadius: "9999px",
        backgroundColor: HEALTH_COLOR_MAP[health],
        flexShrink: 0,
    };
}

// 健康度对应的简短文字标记（用于原生 <option> 场景，无法显示彩色圆点）
export function healthMark(health: ModelHealth): string {
    switch (health) {
        case "normal":
            return " ✓";
        case "unstable":
            return " ⚠";
        case "down":
            return " ✗";
        default:
            // unknown 视为不可用，用 🔴 标记
            return " 🔴";
    }
}

// 展示用的健康度结果（是否渲染圆点 + 颜色 + 文案 + 文字标记）
export type ModelHealthDisplay = {
    health: ModelHealth;
    color: string;
    text: string;
    mark: string;
    show: boolean;
};

// 无状态模型的展示规则（单一事实来源）：
// - status 尚未加载完成 → 不显示圆点（避免加载瞬间把付费模型误标红）
// - status 已加载但仍 unknown：免费模型(costCents 为 0 或无记录) → 绿色可用；付费模型 → 红色🔴不可用
// 有明确状态的模型按原状态显示（normal 绿 / unstable 黄 / down 红）。
export function resolveModelHealthDisplay(model: string, status: ModelStatusSummary | null, isFree: boolean): ModelHealthDisplay {
    const raw = getModelHealth(model, status);
    if (raw !== "unknown") {
        return { health: raw, color: HEALTH_COLOR_MAP[raw], text: healthText(raw), mark: healthMark(raw), show: true };
    }
    if (!status) {
        return { health: "unknown", color: "", text: "", mark: "", show: false };
    }
    if (isFree) {
        return { health: "normal", color: HEALTH_COLOR_MAP.normal, text: "正常", mark: " ✓", show: true };
    }
    return { health: "down", color: HEALTH_COLOR_MAP.down, text: "不可用", mark: " 🔴", show: true };
}

// 模型在下拉列表里的展示排序权重（单一事实来源）：
// 0 = 绿色（正常 / 免费且安全，排最前）；1 = 黄色（不稳定，居中）；
// 2 = 无圆点（状态尚未加载完成，靠后）；3 = 红色（故障 / 不可用 / 付费缺价，排最后）。
// 让健康的模型更容易被选中，不可用的沉到列表底部。
export function healthDisplayRank(model: string, status: ModelStatusSummary | null, isFree: boolean): number {
    const disp = resolveModelHealthDisplay(model, status, isFree);
    if (!disp.show) return 2;
    if (disp.color === HEALTH_COLOR_MAP.normal) return 0;
    if (disp.color === HEALTH_COLOR_MAP.unstable) return 1;
    return 3; // 红色：故障 / 不可用 / 付费缺价
}
