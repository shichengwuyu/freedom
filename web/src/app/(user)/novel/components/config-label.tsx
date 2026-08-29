import React from "react";

export function ConfigLabel({ children }: { children: React.ReactNode }) {
    return <span className="text-xs font-medium text-stone-500 shrink-0">{children}：</span>;
}

// 详情弹窗里的一条元信息（标签 + 值）；span 为 true 时占满两列
export function DetailMeta({ label, value, span }: { label: string; value: string; span?: boolean }) {
    return (
        <div className={`flex items-center gap-1.5 ${span ? "col-span-2" : ""}`}>
            <span className="shrink-0 text-stone-400">{label}</span>
            <span className="min-w-0 truncate font-medium text-stone-700 dark:text-stone-200" title={value}>{value}</span>
        </div>
    );
}
