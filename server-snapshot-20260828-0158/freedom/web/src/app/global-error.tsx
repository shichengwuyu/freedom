"use client";

import { useEffect } from "react";

/**
 * 全局错误边界：捕获根布局渲染异常。
 * 这是 Next.js App Router 的最高级错误边界，当 root layout 自身抛出异常时生效。
 */
export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
    useEffect(() => {
        console.error("全局渲染错误:", error);
    }, [error]);

    return (
        <html>
            <body>
                <div style={{ display: "flex", minHeight: "100vh", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: "16px", padding: "32px", fontFamily: "system-ui, sans-serif" }}>
                    <h2 style={{ fontSize: "20px", fontWeight: 600, color: "#292524" }}>应用发生严重错误</h2>
                    <p style={{ fontSize: "14px", color: "#78716c" }}>
                        {error.message || "渲染过程中发生未知错误"}
                    </p>
                    <button
                        onClick={reset}
                        style={{ padding: "8px 16px", borderRadius: "6px", backgroundColor: "#0284c7", color: "#fff", fontSize: "14px", fontWeight: 500, border: "none", cursor: "pointer" }}
                    >
                        重试
                    </button>
                </div>
            </body>
        </html>
    );
}
