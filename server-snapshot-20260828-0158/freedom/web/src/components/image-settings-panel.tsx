"use client";

import { type ReactNode, useEffect, useMemo, useState } from "react";
import { ConfigProvider, Switch } from "antd";

import { type CanvasTheme } from "@/lib/canvas-theme";
import { filterImageAspectOptions, filterImageQualityOptions, normalizeImageQualityForModel, normalizeImageSizeForModel } from "@/lib/image-model-capabilities";
import type { AiConfig } from "@/stores/use-config-store";

const qualityOptions = [
    { value: "auto", label: "自动" },
    { value: "high", label: "高" },
    { value: "medium", label: "中" },
    { value: "low", label: "低" },
    { value: "standard", label: "标准" },
    { value: "hd", label: "高清" },
];
const DIMENSION_STEP = 16;
const QUALITY_BASE: Record<string, number> = {
    low: 1024,
    medium: 2048,
    high: 2880,
    standard: 1024,
    hd: 2048,
};

const aspectOptions = [
    { value: "1:1", label: "1:1", width: 1024, height: 1024, icon: "square" },
    { value: "3:2", label: "3:2", width: 1536, height: 1024, icon: "landscape" },
    { value: "2:3", label: "2:3", width: 1024, height: 1536, icon: "portrait" },
    { value: "4:3", label: "4:3", width: 1024, height: 768, icon: "landscape" },
    { value: "3:4", label: "3:4", width: 768, height: 1024, icon: "portrait" },
    { value: "16:9", label: "16:9", width: 1920, height: 1080, icon: "landscape" },
    { value: "9:16", label: "9:16", width: 1080, height: 1920, icon: "portrait" },
    { value: "21:9", label: "21:9", width: 1568, height: 672, icon: "landscape" },
    { value: "1:1-2k", label: "1:1(2k)", size: "2048x2048", width: 2048, height: 2048, icon: "square" },
    { value: "16:9-2k", label: "16:9(2k)", size: "2048x1152", width: 2048, height: 1152, icon: "landscape" },
    { value: "9:16-2k", label: "9:16(2k)", size: "1152x2048", width: 1152, height: 2048, icon: "portrait" },
    { value: "21:9-2k", label: "21:9(2k)", size: "3136x1344", width: 3136, height: 1344, icon: "landscape" },
    { value: "16:9-4k", label: "16:9(4k)", size: "3840x2160", width: 3840, height: 2160, icon: "landscape" },
    { value: "9:16-4k", label: "9:16(4k)", size: "2160x3840", width: 2160, height: 3840, icon: "portrait" },
    { value: "21:9-4k", label: "21:9(4k)", size: "6272x2688", width: 6272, height: 2688, icon: "landscape" },
    { value: "auto", label: "auto", width: 0, height: 0, icon: "auto" },
];

export const imageSizeOptions = aspectOptions.map((item) => ({
    value: item.size || item.value,
    label: item.label,
}));

// 按 modelName 过滤后的尺寸列表（供 QuickSelect 之类外部组件复用，按模型能力裁剪 aspectOptions）
export function imageSizeOptionsForModel(modelName: string): { value: string; label: string }[] {
    return filterImageAspectOptions(imageSizeOptions, modelName);
}

// 完整图片质量选项（含 standard / hd，供 dall-e 等模型）。外部可直接复用并按模型过滤。
export const imageQualityOptions = qualityOptions;

// 按模型过滤后的图片质量选项
export function imageQualityOptionsForModel(modelName: string): { value: string; label: string }[] {
    return filterImageQualityOptions(imageQualityOptions, modelName);
}

type ImageSettingsPanelProps = {
    config: AiConfig;
    onConfigChange: (key: "quality" | "size" | "count", value: string) => void;
    theme: CanvasTheme;
    showTitle?: boolean;
    showSize?: boolean;
    showCount?: boolean;
    className?: string;
    maxCount?: number;
    quickCount?: number;
};

function gcd(a: number, b: number): number {
    while (b) {
        [a, b] = [b, a % b];
    }
    return a;
}

function computeAspectDimensions(option: { width: number; height: number; size?: string }, quality: string): string {
    if (option.size) return option.size;
    const basePixels = QUALITY_BASE[quality] || QUALITY_BASE.low;
    const w = option.width;
    const h = option.height;
    if (w <= 0 || h <= 0) return "auto";
    const a = gcd(w, h);
    const unit = Math.round(Math.sqrt((basePixels * basePixels) / ((w / a) * (h / a))) / DIMENSION_STEP) * DIMENSION_STEP;
    return `${(w / a) * unit}x${(h / a) * unit}`;
}

function formatSizeLabel(size: string): string {
    const match = size?.match(/^(\d+)x(\d+)$/);
    if (!match) return size;
    const w = Number(match[1]);
    const h = Number(match[2]);
    if (w >= 1000 && h >= 1000) return `${(w / 1000).toFixed(1)}K×${(h / 1000).toFixed(1)}K`;
    return size;
}

export function ImageSettingsPanel({ config, onConfigChange, theme, showTitle = true, showSize = true, showCount = true, className = "w-[320px] space-y-4 rounded-2xl px-1 py-0.5", maxCount = 15, quickCount = 10 }: ImageSettingsPanelProps) {
    const [snapDimensionToStep, setSnapDimensionToStep] = useState(true);
    const modelName = config.model || config.imageModel || "";
    // 按模型裁剪可选项；未识别模型返回全集
    const availableQualityOptions = useMemo(() => filterImageQualityOptions(qualityOptions, modelName), [modelName]);
    const availableAspectOptions = useMemo(() => filterImageAspectOptions(aspectOptions, modelName), [modelName]);
    // 当前值若不在合法集 → 自动 snap 到首项（迁移策略「自动 snap」）
    const quality = normalizeImageQualityForModel(modelName, config.quality || "auto") || availableQualityOptions[0]?.value || "auto";
    const effectiveQuality = (quality && QUALITY_BASE[quality]) ? quality : availableQualityOptions.find((item) => item.value === "low") ? "low" : availableQualityOptions[0]?.value || "low";
    const count = Math.max(1, Math.min(maxCount, Math.floor(Math.abs(Number(config.count)) || 1)));
    const activeSize = normalizeImageSizeForModel(modelName, config.size || "auto");
    // 存储值若不在当前模型合法集（如旧节点的 "21:9"），回写为 snap 后的显示值，
    // 避免「面板看着是 1:1、实际发出 21:9」的错位。
    useEffect(() => {
        if (activeSize && config.size && activeSize !== config.size) {
            onConfigChange("size", activeSize);
        }
    }, [activeSize, config.size, onConfigChange]);
    const selectedAspect = availableAspectOptions.find((item) => (item.size || item.value) === activeSize || item.value === activeSize);
    const dimensions = readSizeDimensions(activeSize, selectedAspect || availableAspectOptions[0] || aspectOptions[0]);
    const currentPixelSize = selectedAspect && selectedAspect.width > 0
        ? computeAspectDimensions(selectedAspect, effectiveQuality)
        : (activeSize && /^\d+x\d+$/.test(activeSize) ? activeSize : "auto");
    const selectAspect = (value: string) => {
        const option = availableAspectOptions.find((item) => item.value === value);
        onConfigChange("size", option?.size || option?.value || "auto");
    };
    const updateDimension = (key: "width" | "height", value: number | null) => {
        const next = Math.max(1, Math.floor(value || dimensions[key] || 1024));
        const width = key === "width" ? next : dimensions.width;
        const height = key === "height" ? next : dimensions.height;
        onConfigChange("size", `${alignDimension(width, snapDimensionToStep)}x${alignDimension(height, snapDimensionToStep)}`);
    };

    return (
        <ImageSettingsTheme theme={theme}>
            <div
                className={className}
                style={{ color: theme.node.text }}
                onMouseDown={(event) => {
                    event.stopPropagation();
                    if (event.target instanceof HTMLInputElement) return;
                    if (document.activeElement instanceof HTMLInputElement && event.currentTarget.contains(document.activeElement)) document.activeElement.blur();
                }}
            >
                {showTitle ? <div className="text-lg font-semibold">图像设置</div> : null}
                <div className="space-y-2.5">
                    <SettingTitle color={theme.node.muted}>质量</SettingTitle>
                    <div className="grid grid-cols-4 gap-2.5">
                        {availableQualityOptions.map((item) => (
                            <OptionPill key={item.value} selected={quality === item.value} theme={theme} onClick={() => onConfigChange("quality", item.value)}>
                                {item.label}
                            </OptionPill>
                        ))}
                    </div>
                </div>
                {showSize ? (
                    <>
                        <div className="space-y-2.5">
                            <div className="flex items-center justify-between gap-3">
                                <SettingTitle color={theme.node.muted}>尺寸</SettingTitle>
                                <div className="flex items-center gap-2">
                                    <span className="text-xs font-medium" style={{ color: theme.node.muted }}>
                                        16倍数对齐
                                    </span>
                                    <span title="输入完成后自动向上补成 16 的倍数" onMouseDown={(event) => event.stopPropagation()}>
                                        <Switch size="small" checked={snapDimensionToStep} onChange={setSnapDimensionToStep} />
                                    </span>
                                </div>
                            </div>
                            <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2.5">
                                <DimensionInput prefix="W" value={dimensions.width} disabled={activeSize === "auto"} theme={theme} alignToStep={snapDimensionToStep} onChange={(value) => updateDimension("width", value)} />
                                <span className="text-lg opacity-45">↔</span>
                                <DimensionInput prefix="H" value={dimensions.height} disabled={activeSize === "auto"} theme={theme} alignToStep={snapDimensionToStep} onChange={(value) => updateDimension("height", value)} />
                            </div>
                        </div>
                        <div className="space-y-2.5">
                            <SettingTitle color={theme.node.muted}>宽高比</SettingTitle>
                            <div className="grid grid-cols-4 gap-2.5">
                                {availableAspectOptions.map((item) => (
                                    <button
                                        key={item.value}
                                        type="button"
                                        className="flex h-[80px] cursor-pointer flex-col items-center justify-center gap-1 rounded-xl border bg-transparent text-sm transition hover:opacity-80"
                                        style={{ borderColor: selectedAspect?.value === item.value ? theme.node.text : theme.node.stroke, background: "transparent", color: theme.node.text }}
                                        onMouseDown={(event) => event.stopPropagation()}
                                        onClick={() => selectAspect(item.value)}
                                    >
                                        <AspectIcon type={item.icon} width={item.width} height={item.height} color={theme.node.text} />
                                        <span className="text-sm font-medium">{item.label}</span>
                                        {item.icon !== "auto" ? (
                                            <span className="text-[10px] leading-tight" style={{ color: theme.node.muted }}>
                                                {computeAspectDimensions(item, effectiveQuality)}
                                            </span>
                                        ) : null}
                                    </button>
                                ))}
                            </div>
                            {currentPixelSize && currentPixelSize !== "auto" ? (
                                <div className="mt-1 flex items-center gap-1 text-xs" style={{ color: theme.node.muted }}>
                                    <span>将使用</span>
                                    <span className="font-semibold" style={{ color: theme.node.text }}>{currentPixelSize}</span>
                                </div>
                            ) : null}
                        </div>
                    </>
                ) : null}
                {showCount ? (
                    <div className="space-y-2.5">
                        <SettingTitle color={theme.node.muted}>生成张数</SettingTitle>
                        <div className="grid grid-cols-4 gap-2.5">
                            {Array.from({ length: quickCount }, (_, index) => index + 1).map((value) => (
                                <OptionPill key={value} selected={count === value} theme={theme} onClick={() => onConfigChange("count", String(value))}>
                                    {value} 张
                                </OptionPill>
                            ))}
                            <CountInput value={count} max={maxCount} theme={theme} onChange={(value) => onConfigChange("count", String(value || 1))} />
                        </div>
                    </div>
                ) : null}
            </div>
        </ImageSettingsTheme>
    );
}

export function ImageSettingsTheme({ theme, children }: { theme: CanvasTheme; children: ReactNode }) {
    return (
        <ConfigProvider
            theme={{
                token: { colorBgContainer: theme.toolbar.panel, colorBgElevated: theme.toolbar.panel, colorBorder: theme.node.stroke, colorPrimary: theme.node.activeStroke, colorText: theme.node.text, colorTextLightSolid: theme.node.panel },
                components: { Button: { defaultBg: theme.toolbar.panel, defaultBorderColor: theme.node.stroke, defaultColor: theme.node.text } },
            }}
        >
            {children}
        </ConfigProvider>
    );
}

export function imageQualityLabel(value: string) {
    return ({ auto: "自动", high: "高", medium: "中", low: "低", standard: "标准", hd: "高清" } as Record<string, string>)[value] || value;
}

export function imageSizeLabel(size: string) {
    const option = aspectOptions.find((item) => (item.size || item.value) === size || item.value === size);
    if (option) return option.label;
    const match = size?.match(/^(\d+)x(\d+)$/);
    if (match) return `${match[1]}×${match[2]}`;
    return size;
}

function OptionPill({ selected, theme, onClick, children }: { selected: boolean; theme: CanvasTheme; onClick: () => void; children: ReactNode }) {
    return (
        <button
            type="button"
            className="h-9 cursor-pointer rounded-full border px-2 text-sm transition hover:opacity-80"
            style={{ background: "transparent", borderColor: selected ? theme.node.text : theme.node.stroke, color: theme.node.text }}
            onMouseDown={(event) => event.stopPropagation()}
            onClick={onClick}
        >
            {children}
        </button>
    );
}

function DimensionInput({ prefix, value, disabled, theme, alignToStep, onChange }: { prefix: string; value: number; disabled: boolean; theme: CanvasTheme; alignToStep: boolean; onChange: (value: number | null) => void }) {
    const commit = (input: HTMLInputElement) => {
        const next = alignDimension(Math.max(1, Math.floor(Number(input.value) || value || 1024)), alignToStep);
        input.value = String(next);
        onChange(next);
    };

    return (
        <label className="flex h-9 overflow-hidden rounded-xl text-sm" style={{ background: theme.node.fill, color: theme.node.text, opacity: disabled ? 0.55 : 1 }}>
            <span className="grid w-9 place-items-center" style={{ color: theme.node.muted }}>
                {prefix}
            </span>
            <input
                type="number"
                min={1}
                disabled={disabled}
                className="min-w-0 flex-1 bg-transparent px-2 outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                defaultValue={value || ""}
                key={`${prefix}-${value}`}
                onBlur={(event) => commit(event.currentTarget)}
                onKeyDown={(event) => {
                    if (event.key === "Enter") event.currentTarget.blur();
                }}
                onMouseDown={(event) => event.stopPropagation()}
            />
        </label>
    );
}

function CountInput({ value, max, theme, onChange }: { value: number; max: number; theme: CanvasTheme; onChange: (value: number | null) => void }) {
    return (
        <label className="col-span-2 flex h-9 overflow-hidden rounded-full border text-sm" style={{ borderColor: theme.node.stroke, color: theme.node.text }}>
            <input
                type="number"
                min={1}
                max={max}
                className="min-w-0 flex-1 bg-transparent px-3 text-center outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                style={{ color: theme.node.text, WebkitTextFillColor: theme.node.text }}
                value={value || ""}
                onChange={(event) => onChange(Number(event.target.value) || null)}
                onMouseDown={(event) => event.stopPropagation()}
            />
        </label>
    );
}

function AspectIcon({ type, width, height, color }: { type: string; width: number; height: number; color: string }) {
    if (type === "auto") return null;
    const ratio = width / Math.max(1, height);
    const boxWidth = ratio >= 1 ? 24 : Math.max(10, 24 * ratio);
    const boxHeight = ratio >= 1 ? Math.max(10, 24 / ratio) : 24;
    return (
        <span className="grid h-7 w-9 place-items-center">
            <span className="border-2" style={{ width: boxWidth, height: boxHeight, borderColor: color }} />
        </span>
    );
}

function SettingTitle({ children, color }: { children: string; color: string }) {
    return (
        <div className="text-xs font-medium" style={{ color }}>
            {children}
        </div>
    );
}

function readSizeDimensions(size: string, fallback: { width: number; height: number }) {
    const match = size?.match(/^(\d+)x(\d+)$/);
    return {
        width: match ? Number(match[1]) : fallback.width,
        height: match ? Number(match[2]) : fallback.height,
    };
}

function alignDimension(value: number, enabled: boolean) {
    return enabled ? Math.ceil(value / DIMENSION_STEP) * DIMENSION_STEP : value;
}

export function imageFormatLabel(format: string) {
    const map: Record<string, string> = { png: "PNG", jpeg: "JPEG", webp: "WebP" };
    return map[format] || format;
}
