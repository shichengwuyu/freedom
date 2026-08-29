"use client";

import { ArrowLeft, ExternalLink } from "lucide-react";
import { useState } from "react";
import Link from "next/link";
import { Button } from "antd";

// Vidmore 水印去除工具页面的 URL
const VIDMORE_URL = "https://www.vidmore.com/watermark-remover/";

export default function ImageWatermarkPage() {
    // iframe 是否加载失败（被 X-Frame-Options 拦截等）
    const [iframeFailed, setIframeFailed] = useState(false);

    return (
        <div className="mx-auto max-w-5xl px-6 py-8">
            {/* 顶部导航：返回 + 标题 */}
            <div className="mb-6 flex items-center gap-3">
                <Link href="/tools">
                    <Button type="text" icon={<ArrowLeft className="size-4" />} className="text-stone-600 dark:text-stone-300">
                        返回
                    </Button>
                </Link>
                <h1 className="text-2xl font-semibold text-stone-950 dark:text-stone-100">图片去水印</h1>
            </div>

            {/* 工具说明 */}
            <div className="mb-4 flex items-center justify-between rounded-lg border border-stone-200 bg-stone-50 px-4 py-3 dark:border-stone-700 dark:bg-stone-800">
                <p className="text-sm text-stone-600 dark:text-stone-300">
                    使用 Vidmore 在线水印去除工具，上传图片后涂抹水印区域即可自动擦除。
                </p>
                <Button
                    type="primary"
                    size="small"
                    icon={<ExternalLink className="size-3.5" />}
                    onClick={() => window.open(VIDMORE_URL, "_blank", "noopener,noreferrer")}
                >
                    在新窗口打开
                </Button>
            </div>

            {/* iframe 嵌入 Vidmore 工具页面；若被拦截则展示降级提示 */}
            {!iframeFailed ? (
                <div className="overflow-hidden rounded-xl border border-stone-200 dark:border-stone-700">
                    <iframe
                        src={VIDMORE_URL}
                        className="h-[calc(100vh-220px)] min-h-[600px] w-full"
                        title="Vidmore 水印去除"
                        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-downloads"
                        onError={() => setIframeFailed(true)}
                        onLoad={(event) => {
                            // 尝试检测是否被拦截：如果 iframe 内容无法访问则降级
                            try {
                                const iframe = event.currentTarget;
                                // 如果能访问 contentDocument 且为 null 说明跨域被拦截，但不一定代表加载失败
                                // 真正的拦截检测靠 onerror 和超时
                                if (!iframe.contentDocument && !iframe.contentWindow) {
                                    setIframeFailed(true);
                                }
                            } catch {
                                // 跨域访问 contentDocument 会抛错，这是正常的（说明 iframe 确实加载了跨域内容）
                            }
                        }}
                    />
                </div>
            ) : (
                /* 降级：iframe 被拦截时展示引导卡片 */
                <div className="flex flex-col items-center justify-center rounded-xl border border-stone-200 bg-white py-16 dark:border-stone-700 dark:bg-stone-900">
                    <p className="mb-4 text-center text-sm text-stone-500 dark:text-stone-400">
                        该工具无法在页面内嵌入，请点击下方按钮在新窗口中使用
                    </p>
                    <Button
                        type="primary"
                        size="large"
                        icon={<ExternalLink className="size-4" />}
                        onClick={() => window.open(VIDMORE_URL, "_blank", "noopener,noreferrer")}
                    >
                        打开 Vidmore 水印去除工具
                    </Button>
                </div>
            )}
        </div>
    );
}
