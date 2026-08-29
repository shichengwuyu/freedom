import type { ComponentProps } from "react";
import { Coins } from "lucide-react";

export function BalanceSymbol({ className, ...props }: ComponentProps<"span">) {
    return (
        <span {...props} className={`inline-flex items-center justify-center ${className || ""}`}>
            <Coins className="size-[1em] fill-current" strokeWidth={2.4} />
        </span>
    );
}

export type ModelCostCents = {
    model: string;
    costCents: number;
    unit?: "per_call" | "per_second";
    costCentsPerSecond?: number;
};

// 单次扣费（分），即"这张图多少钱"。
export const PER_IMAGE_COST_CENTS = 4; // 0.04 元 = 4 分（前端静态估算常量，与历史 0.04 同价段）

export function modelCostCents(modelCosts: ModelCostCents[] | undefined, model: string): number {
    return modelCosts?.find((item) => item.model === model)?.costCents || 0;
}

export function modelCostDetail(modelCosts: ModelCostCents[] | undefined, model: string): ModelCostCents | undefined {
    return modelCosts?.find((item) => item.model === model);
}

export function requestCostCents(options: {
    channelMode: string;
    modelCosts?: ModelCostCents[];
    model: string;
    count?: string | number;
    seconds?: string | number;
}): number {
    if (options.channelMode !== "remote") return 0;
    const detail = modelCostDetail(options.modelCosts, options.model);
    if (!detail) return 0;
    const count = Math.max(1, Math.floor(Math.abs(Number(options.count)) || 1));
    // 按秒计费：视频模型按秒数 * 每秒扣费（分）
    if (detail.unit === "per_second" && detail.costCentsPerSecond && detail.costCentsPerSecond > 0) {
        const seconds = Math.max(1, Math.floor(Math.abs(Number(options.seconds)) || 1));
        return detail.costCentsPerSecond * seconds;
    }
    return (detail.costCents || 0) * count;
}

export function formatCostCents(
    options: {
        channelMode: string;
        modelCosts?: ModelCostCents[];
        model: string;
        count?: string | number;
        seconds?: string | number;
    },
): { costCents: number; unit: string; isPerSecond: boolean } {
    if (options.channelMode !== "remote") return { costCents: 0, unit: "", isPerSecond: false };
    const detail = modelCostDetail(options.modelCosts, options.model);
    if (!detail) return { costCents: 0, unit: "", isPerSecond: false };
    const count = Math.max(1, Math.floor(Math.abs(Number(options.count)) || 1));
    if (detail.unit === "per_second" && detail.costCentsPerSecond && detail.costCentsPerSecond > 0) {
        // 按钮显示「每秒单价」，不要乘秒数（否则会把 N 秒总价误标成“/秒”）。实际总价由 requestCostCents 计算。
        return { costCents: detail.costCentsPerSecond, unit: `/秒`, isPerSecond: true };
    }
    return { costCents: (detail.costCents || 0) * count, unit: "/次", isPerSecond: false };
}

/**
 * 把内部整数余额格式化为 "¥X.XX" 显示。
 */
export function formatBalanceYuan(cents: number): string {
    const yuan = (cents || 0) / 100;
    return `¥${yuan.toFixed(2)}`;
}

/**
 * 把分（cents）数字格式化为带千分位的"纯数字 + 元"文本，没有 ¥ 符号。
 */
export function formatBalancePlainYuan(cents: number): string {
    const yuan = (cents || 0) / 100;
    return `${yuan.toFixed(2)}`;
}
