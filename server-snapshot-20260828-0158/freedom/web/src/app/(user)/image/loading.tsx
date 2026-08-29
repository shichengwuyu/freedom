// Next.js App Router 路由级 Loading 组件：生图工作台
// 导航到此路由时立刻渲染，替代白屏，提升页面跳转感知速度
export default function ImageLoading() {
    return (
        <div className="flex h-full w-full flex-col gap-3 bg-background p-4 text-stone-950 dark:text-stone-100 md:gap-5 md:p-6">
            {/* Top toolbar skeleton */}
            <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                    <div className="h-6 w-6 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    <div className="h-5 w-28 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                </div>
                <div className="flex items-center gap-2">
                    <div className="h-8 w-20 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    <div className="h-8 w-24 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    <div className="h-9 w-32 animate-pulse rounded bg-stone-900 dark:bg-stone-100" />
                </div>
            </div>

            <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 overflow-hidden md:gap-5 xl:grid-cols-[380px_minmax(0,1fr)]">
                {/* Left: settings panel skeleton */}
                <div className="flex min-h-0 flex-col gap-4 overflow-y-auto rounded-xl border border-stone-200 bg-white p-4 shadow-sm dark:border-stone-800 dark:bg-stone-900 md:p-5">
                    <div className="h-24 w-full animate-pulse rounded-lg bg-stone-200 dark:bg-stone-800" />
                    <div className="space-y-3">
                        <div className="h-3 w-16 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-9 w-full animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    </div>
                    <div className="space-y-3">
                        <div className="h-3 w-20 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="grid grid-cols-2 gap-2">
                            <div className="h-8 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                            <div className="h-8 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        </div>
                    </div>
                    <div className="space-y-3">
                        <div className="h-3 w-24 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-20 w-full animate-pulse rounded bg-stone-200 dark:bg-stone-800" />
                    </div>
                    <div className="grid grid-cols-3 gap-2 pt-2">
                        <div className="h-8 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-8 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-8 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    </div>
                </div>

                {/* Right: results skeleton */}
                <div className="flex min-h-0 flex-col gap-4 overflow-hidden rounded-xl border border-stone-200 bg-white p-4 shadow-sm dark:border-stone-800 dark:bg-stone-900 md:p-5">
                    <div className="flex items-center justify-between">
                        <div className="h-4 w-24 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-6 w-40 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    </div>
                    <div className="grid flex-1 min-h-0 auto-rows-min grid-cols-2 gap-3 overflow-y-auto pr-1 md:grid-cols-3 xl:grid-cols-4">
                        {Array.from({ length: 8 }).map((_, i) => (
                            <div
                                key={i}
                                className="aspect-square animate-pulse rounded-lg border border-stone-200 bg-stone-200 dark:border-stone-800 dark:bg-stone-800"
                            />
                        ))}
                    </div>
                </div>
            </div>
        </div>
    );
}
