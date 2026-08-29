import { Tooltip } from "antd";
import Link from "next/link";
import { Coins } from "lucide-react";
import type { AuthUser } from "@/services/api/auth";

interface BalanceBadgeProps {
    user: AuthUser | null;
    iconOnly?: boolean;
}

/**
 * 顶栏「余额」徽标：登录后展示用户账户余额（¥X.XX），未登录则显示提示。
 * 重构自 BalanceBadge（2026-08-17），去掉"余额"概念，直接用 ¥ 显示。
 */
export function BalanceBadge({ user, iconOnly }: BalanceBadgeProps) {
    if (!user) {
        if (iconOnly) {
            return (
                <Tooltip title="登录后查看账户余额" placement="bottom">
                    <Link
                        href="/login"
                        className="inline-flex items-center gap-1 text-stone-500 hover:text-stone-700 text-sm transition"
                    >
                        <Coins className="size-4" strokeWidth={2.4} />
                    </Link>
                </Tooltip>
            );
        }
        return (
            <Tooltip title="登录后查看账户余额" placement="bottom">
                <Link
                    href="/login"
                    className="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-stone-500 hover:text-stone-700 hover:bg-stone-100 text-sm font-medium transition"
                >
                    <Coins className="size-4" strokeWidth={2.4} />
                    <span>¥0.00</span>
                </Link>
            </Tooltip>
        );
    }

    const yuan = (user.balanceCents / 100).toFixed(2);
    const lowBalance = user.balanceCents < 500; // 5 元以下提醒充值

    if (iconOnly) {
        return (
            <Tooltip title={`账户余额 ¥${yuan}，点击查看充值说明`} placement="bottom">
                <Link
                    href="/wallet"
                    className={`inline-flex items-center gap-1 text-sm font-semibold transition ${
                        lowBalance ? "text-amber-600 hover:text-amber-700" : "text-sky-600 hover:text-sky-700"
                    }`}
                >
                    <Coins className="size-4" strokeWidth={2.4} />
                </Link>
            </Tooltip>
        );
    }

    return (
        <Tooltip title={`账户余额 ¥${yuan}，点击查看充值说明`} placement="bottom">
            <Link
                href="/wallet"
                className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-semibold transition ${
                    lowBalance
                        ? "text-amber-700 bg-amber-50 hover:bg-amber-100 ring-1 ring-amber-200"
                        : "text-sky-700 bg-sky-50 hover:bg-sky-100 ring-1 ring-sky-200"
                }`}
            >
                <Coins className="size-4" strokeWidth={2.4} />
                <span>¥{yuan}</span>
            </Link>
        </Tooltip>
    );
}
