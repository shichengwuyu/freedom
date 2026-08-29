"use client";

import { useEffect } from "react";

/**
 * 路由级错误边界：捕获页面渲染异常，避免白屏。
 * Next.js App Router 自动将未捕获的渲染错误传递给此组件。
 */
export default function Error({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
    useEffect(() => {
        console.error("页面渲染错误:", error);
    }, [error]);

    return (
        <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 p-8">
            <h2 className="text-xl font-semibold text-stone-800">页面出了点问题</h2>
            <p className="text-sm text-stone-500">
                {error.message || "渲染过程中发生未知错误"}
            </p>
            <button
                onClick={reset}
                className="rounded-md bg-sky-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-sky-700"
            >
                重试
            </button>
        </div>
    );
}
