"use client";

import { useEffect, useId, useMemo, useState } from "react";
import { Cpu } from "lucide-react";
import { App } from "antd";

import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { activeChannelKind, filterModelsByCapability, selectableModelChannels, useConfigStore, type AiConfig, type ModelCapability } from "@/stores/use-config-store";
import { useModelStatus, healthDotStyle, resolveModelHealthDisplay, healthDisplayRank, type ModelStatusSummary } from "@/services/api/model-status";
import { modelCostCents, modelCostDetail } from "@/constant/balance";

type ModelPickerProps = {
    config: AiConfig;
    value?: string;
    channelId?: string;
    capability?: ModelCapability;
    onChange: (model: string, channelId?: string) => void;
    className?: string;
    fullWidth?: boolean;
    placeholder?: string;
    onMissingConfig?: () => void;
    variant?: "default" | "quick";
};

// 当前 rolldek / apimart 上游 /v1/images/generations 端点都不接 gemini-3-pro / gemini-3.1-flash-image-preview
// （错误文案与 apimart 一字不差，强烈暗示 rolldek 透传到 apimart）。vendor 列表仍把它们列在
// availableModels，下拉里能选、提交必败。先在前端下拉里硬隐藏，等接入真正接 Gemini 官方 API 的渠道再放开。
const UNSUPPORTED_GEMINI_IMAGE_PREVIEW_MODELS = new Set<string>([
    "gemini-3-pro-image-preview",
    "gemini-3.1-flash-image-preview",
]);

export function ModelPicker({ config, value, channelId, capability, onChange, className, fullWidth = false, placeholder = "选择模型", onMissingConfig, variant = "default" }: ModelPickerProps) {
    const { message } = App.useApp();
    const pickerId = useId();
    const [open, setOpen] = useState(false);
    // 订阅模型健康状态（模块级缓存，多个 Picker 共享，不会重复请求）
    const modelStatus = useModelStatus();
    // 用公开配置里的计费表判断模型是否免费（costCents 为 0 或无记录即免费）
    const modelCosts = useConfigStore((s) => s.publicSettings?.modelChannel.modelCosts);
    const channelOptions = useMemo(() => {
        // 全局单一事实来源：供应商模式只显示该供应商模型，官方模式显示云端模型 + 用户真实添加的本地渠道。
        const channels = selectableModelChannels(config);
        // 展示名优先级：公开配置"系统可用模型"表别名 > 渠道 modelLabels > 真实模型ID。API 请求仍用 model 本身。
        const costLabels = config.modelCostLabels || {};
        const flat = channels.flatMap((channel) => (channel.models ?? []).map((model) => ({ key: `${channel.id}::${model}`, channelId: channel.id, channelName: channel.name, model, label: costLabels[model] || channel.modelLabels?.[model] || model })));
        // Bug（2026-08-25，多 channel 重复）：用户的 localChannels 里 N 个 channel 都加了同一 model
        // （常见于「云端免费 / 云端免费/另起一份 / 自定义渠道」并存时，每个都复制粘贴了同样 5 个 model），
        // 单次 dedupByKey 按 (channelId, model) 去重跨 channel 无效，下拉里「official-image-2.0-flash」出现 N 次。
        // 加按 model 全局去重：保留首次出现的 channel（onChange 仍写回 imageModel，下游按 model 路由）。
        const dedupByKey = Array.from(new Map(flat.map((it) => [it.key, it])).values());
        const dedupByModel = Array.from(new Map(dedupByKey.map((it) => [it.model, it])).values());
        if (!capability) return dedupByModel;
        return dedupByModel.filter((item) => filterModelsByCapability([item.model], capability).length > 0);
    }, [capability, config]);
    // 付费渠道里查不到定价的模型直接隐藏：本该收费却缺价（上游未返回），避免以"免费"误导用户。
    // 免费渠道（渠道名含"免费"）与本地渠道（自定义渠道）一律保留（本地不计费，云端 modelCosts 表查不到也不应隐藏）。
    const isVendor = Boolean(config.activeVendorType) && config.activeVendorType !== "official";
    const visibleOptions = useMemo(() => {
        if (isVendor) return channelOptions;
        return channelOptions.filter((item) => {
            const channelName = item.channelName || "";
            const isFreeChannel = channelName.includes("免费");
            const isLocalChannel = channelName.includes("自定义") || channelName.includes("本地") || (item.channelId || "").startsWith("local-") || item.channelId === "local-default";
            if (isFreeChannel || isLocalChannel) return true;
            // rolldek / apimart 都不收的 image preview 模型直接隐藏，避免「下拉能选 / 提交必败」
            if (UNSUPPORTED_GEMINI_IMAGE_PREVIEW_MODELS.has(item.model)) return false;
            // 付费云端渠道：modelCosts 里必须能查到该模型才有资格显示，否则隐藏
            return Boolean(modelCostDetail(modelCosts, item.model));
        });
    }, [channelOptions, isVendor, modelCosts]);
    const currentOption = useMemo(() => {
        if (!value) return undefined;
        // 优先在可见列表里找；找不到（已选中的模型恰被隐藏）时回退全量，保证已选中态不丢失
        return visibleOptions.find((item) => item.model === value && item.channelId === channelId) || visibleOptions.find((item) => item.model === value) || channelOptions.find((item) => item.model === value && item.channelId === channelId) || channelOptions.find((item) => item.model === value);
    }, [channelId, channelOptions, visibleOptions, value]);
    // 下拉列表排序：绿色（可用/免费）排前、黄色（不稳定）居中、红色（故障/不可用/付费缺价）沉底，
    // 同权重内保持原有渠道/模型顺序（稳定排序），避免每次刷新抖动。
    const options = useMemo(() => {
        return [...visibleOptions].sort((a, b) => {
            const ra = healthDisplayRank(a.model, modelStatus, modelCostCents(modelCosts, a.model) === 0);
            const rb = healthDisplayRank(b.model, modelStatus, modelCostCents(modelCosts, b.model) === 0);
            return ra - rb;
        });
    }, [visibleOptions, modelCosts, modelStatus]);
    const current = value || "";
    const currentValue = current && currentOption ? currentOption.key : "";

    useEffect(() => {
        if (value && currentOption?.channelId && channelId !== currentOption.channelId) onChange(value, currentOption.channelId);
    }, [channelId, currentOption?.channelId, onChange, value]);

    useEffect(() => {
        const closeOtherPicker = (event: Event) => {
            if ((event as CustomEvent<string>).detail !== pickerId) setOpen(false);
        };
        window.addEventListener("model-picker-open", closeOtherPicker);
        return () => window.removeEventListener("model-picker-open", closeOtherPicker);
    }, [pickerId]);

    // quick 变体：使用原生 select，样式与 QuickSelect 一致
    if (variant === "quick") {
        return (
            <select
                className={cn(
                    "h-11 min-w-0 rounded-xl border border-stone-200 bg-background px-3 text-sm text-stone-900 outline-none dark:border-stone-800 dark:text-stone-100",
                    fullWidth && "w-full",
                    className,
                )}
                value={currentValue}
                onMouseDown={() => {
                    if (!options.length) {
                        if (activeChannelKind(config) === "local") {
                            onMissingConfig?.();
                        } else {
                            message.warning("暂无可用模型，请联系管理员在后台配置模型渠道并勾选可用模型");
                        }
                    }
                }}
                onChange={(event) => {
                    const option = options.find((item) => item.key === event.target.value);
                    if (option) onChange(option.model, option.channelId);
                }}
            >
                {!currentValue && <option value="">{placeholder}</option>}
                {options.length ? (
                    options.map((option) => (
                        <option key={option.key} value={option.key}>
                            {option.label}
                            {resolveModelHealthDisplay(option.model, modelStatus, modelCostCents(modelCosts, option.model) === 0).mark}
                            {option.channelName ? ` (${option.channelName})` : ""}
                        </option>
                    ))
                ) : (
                    <option value="" disabled>
                        {activeChannelKind(config) === "remote" ? "暂无可用模型" : "请先到配置里拉取模型列表"}
                    </option>
                )}
            </select>
        );
    }

    return (
        <Select
            open={open}
            value={current ? currentValue : ""}
            onOpenChange={(nextOpen) => {
                if (nextOpen && !options.length) {
                    if (activeChannelKind(config) === "local") {
                        onMissingConfig?.();
                    } else {
                        message.warning("暂无可用模型，请联系管理员在后台配置模型渠道并勾选可用模型");
                    }
                    return;
                }
                if (nextOpen) window.dispatchEvent(new CustomEvent("model-picker-open", { detail: pickerId }));
                setOpen(nextOpen);
            }}
            onValueChange={(nextValue) => {
                const option = options.find((item) => item.key === nextValue);
                if (option) onChange(option.model, option.channelId);
            }}
        >
            <SelectTrigger
                className={cn(
                    "canvas-composer-model-picker h-8 w-fit max-w-full gap-2 rounded-full border border-input bg-transparent px-3 text-sm font-normal shadow-sm transition-colors",
                    fullWidth ? "w-full min-w-0 justify-start" : "min-w-[9rem] justify-start",
                    "data-[state=open]:border-ring data-[state=open]:ring-2 data-[state=open]:ring-ring/20",
                    className,
                )}
                onMouseDown={(event) => event.stopPropagation()}
                onPointerDown={(event) => event.stopPropagation()}
                title={currentOption?.label || current || placeholder}
            >
                <ModelIcon model={current} />
                <span className="canvas-model-picker-text min-w-0 flex-1 truncate text-left">{currentOption?.label || current || placeholder}</span>
            </SelectTrigger>
            <SelectContent
                data-canvas-no-zoom
                className="z-[1200] w-80 max-w-[calc(100vw-24px)] rounded-xl border border-border/70 bg-popover p-1 shadow-xl"
                position="popper"
                align="start"
                side="bottom"
                sideOffset={6}
                onPointerDown={(event) => event.stopPropagation()}
                onMouseDown={(event) => event.stopPropagation()}
            >
                {options.length ? (
                    options.map((option) => (
                        <SelectItem key={option.key} value={option.key} textValue={`${option.label} ${option.channelName}`}>
                            <ModelLabel model={option.model} label={option.label} channelName={option.channelName} status={modelStatus} />
                        </SelectItem>
                    ))
                ) : (
                    <SelectItem value="__empty__" disabled>
                        {activeChannelKind(config) === "remote" ? "暂无可用模型" : "请先到配置里拉取模型列表"}
                    </SelectItem>
                )}
            </SelectContent>
        </Select>
    );
}

function ModelLabel({ model, label, channelName, status }: { model: string; label?: string; channelName?: string; status: ModelStatusSummary | null }) {
    const modelCosts = useConfigStore((s) => s.publicSettings?.modelChannel.modelCosts);
    const isFree = modelCostCents(modelCosts, model) === 0;
    const disp = resolveModelHealthDisplay(model, status, isFree);
    return (
        <span className="flex min-w-0 items-center gap-2">
            <ModelIcon model={model} />
            <span className="truncate">{label || model}</span>
            {disp.show ? <span style={healthDotStyle(disp.health)} title={disp.text} /> : null}
            {channelName ? <span className="ml-auto max-w-24 shrink-0 truncate text-xs opacity-50">{channelName}</span> : null}
        </span>
    );
}

function ModelIcon({ model }: { model: string }) {
    const icon = resolveModelIcon(model);
    return icon ? <img src={icon} alt="" className="size-4 shrink-0 dark:invert" /> : <Cpu className="size-4 shrink-0 opacity-70" />;
}

function resolveModelIcon(model: string) {
    const name = model.toLowerCase();
    if (name.includes("claude") || name.includes("anthropic")) return "/icons/claude.svg";
    if (name.includes("gemini") || name.includes("google")) return "/icons/gemini.svg";
    if (name.includes("gpt") || name.includes("openai")) return "/icons/openai.svg";
    if (name.includes("grok") || name.includes("grok")) return "/icons/grok.svg";
    if (name.includes("deepseek") || name.includes("deepseek")) return "/icons/deepseek.svg";
    if (name.includes("glm") || name.includes("glm")) return "/icons/glm.svg";
    return "";
}
