// Next.js App Router 路由级 Loading 组件：画布列表页
// 导航到此路由时立刻渲染，替代白屏，提升页面跳转感知速度
export default function CanvasLoading() {
    return (
        <main className="h-full overflow-auto bg-background text-stone-950 dark:text-stone-100">
            <div className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-6 py-10">
                {/* Header skeleton */}
                <header className="flex flex-wrap items-end justify-between gap-4 border-b border-stone-200 pb-6 dark:border-stone-800">
                    <div>
                        <div className="h-3 w-10 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="mt-3 h-8 w-32 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                    </div>
                    <div className="flex items-center gap-2">
                        <div className="h-9 w-24 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-9 w-24 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-9 w-28 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                        <div className="h-9 w-28 animate-pulse rounded bg-stone-900 dark:bg-stone-100" />
                    </div>
                </header>

                {/* Project card grid skeleton */}
                <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
                    {Array.from({ length: 6 }).map((_, i) => (
                        <div
                            key={i}
                            className="overflow-hidden rounded-xl border border-stone-200 bg-white dark:border-stone-800 dark:bg-stone-900"
                        >
                            <div className="h-40 animate-pulse bg-stone-200 dark:bg-stone-800" />
                            <div className="space-y-3 p-4">
                                <div className="h-4 w-3/4 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                                <div className="h-3 w-1/2 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                                <div className="flex justify-between pt-2">
                                    <div className="h-3 w-16 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                                    <div className="h-3 w-20 animate-pulse rounded bg-stone-200 dark:bg-stone-700" />
                                </div>
                            </div>
                        </div>
                    ))}
                </div>
            </div>
        </main>
    );
}
