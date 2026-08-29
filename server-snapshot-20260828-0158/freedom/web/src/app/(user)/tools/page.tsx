"use client";

import { Image as ImageIcon, FileText, Images, Eraser, ChevronRight } from "lucide-react";
import Link from "next/link";

// 工具入口页：所有功能均为跳转卡片，点击进入独立页面
export default function ToolsPage() {
    return (
        <div className="mx-auto max-w-7xl px-6 py-8">
            <div className="mb-8">
                <h1 className="text-2xl font-semibold text-stone-950 dark:text-stone-100">更多工具</h1>
                <p className="mt-2 text-sm text-stone-500 dark:text-stone-400">
                    AI 驱动的图片视频处理工具，让创作更高效
                </p>
            </div>

            {/* 四个功能卡片，全部跳转到独立页面 */}
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <ToolCard
                    href="/prompts"
                    icon={<FileText className="size-5" />}
                    title="提示词库"
                    desc="浏览并复用精选提示词，快速套用到创作中"
                />
                <ToolCard
                    href="/assets"
                    icon={<Images className="size-5" />}
                    title="我的素材"
                    desc="管理你收藏与上传的图片、视频等素材"
                />
                <ToolCard
                    href="/tools/prompt-reverse"
                    icon={<FileText className="size-5" />}
                    title="提示词反推"
                    desc="上传图片或视频，AI 自动分析生成提示词"
                />
                <ToolCard
                    href="/tools/image-watermark"
                    icon={<Eraser className="size-5" />}
                    title="图片去水印"
                    desc="上传图片，AI 自动识别并去除水印"
                />
            </div>
        </div>
    );
}

// 统一样式的功能卡片，渲染为跳转链接
function ToolCard({ href, icon, title, desc }: {
    href: string; icon: React.ReactNode; title: string; desc: string;
}) {
    return (
        <Link
            href={href}
            className="group flex w-full items-center gap-4 rounded-xl border border-stone-200 bg-white px-5 py-4 text-left transition hover:border-stone-300 hover:shadow-sm dark:border-stone-700 dark:bg-stone-900 dark:hover:border-stone-600"
        >
            <span className="flex size-11 shrink-0 items-center justify-center rounded-lg bg-stone-100 text-stone-700 transition group-hover:bg-stone-950 group-hover:text-white dark:bg-stone-800 dark:text-stone-200 dark:group-hover:bg-stone-100 dark:group-hover:text-stone-900">
                {icon}
            </span>
            <div className="min-w-0 flex-1">
                <div className="text-base font-medium text-stone-950 dark:text-stone-100">{title}</div>
                <p className="mt-0.5 truncate text-sm text-stone-500 dark:text-stone-400">{desc}</p>
            </div>
            <ChevronRight className="size-5 shrink-0 text-stone-400 transition group-hover:translate-x-0.5 group-hover:text-stone-700 dark:group-hover:text-stone-200" />
        </Link>
    );
}
