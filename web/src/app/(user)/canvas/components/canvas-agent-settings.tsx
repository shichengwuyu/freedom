"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { App, Button, Divider, Input, InputNumber, Modal, Segmented, Select, Slider, Space, Switch, Upload } from "antd";
import type { UploadProps } from "antd";
import { Trash2, Upload as UploadIcon } from "lucide-react";
import { nanoid } from "nanoid";

import { canvasThemes } from "@/lib/canvas-theme";
import { uploadAssetMediaFile } from "@/services/file-storage";
import { useThemeStore } from "@/stores/use-theme-store";
import { type UploadedPetMeta, PET_SIZE_MAX, PET_SIZE_MIN, usePetSettings } from "@/stores/use-pet-settings";
import { PET_CHARACTERS, makeUploadedSpritePetCharacter } from "../agent/pet-characters";
import { CanvasAgentCharacter } from "./canvas-agent-character";

export type CanvasAgentSettingsProps = {
    open: boolean;
    onClose: () => void;
    onOpenAssistant: () => void;
};

type UploadedFileMeta = { url: string; storageKey: string; bytes: number; mimeType: string };
type PendingUpload = { file: File; uploaded: UploadedFileMeta; width: number; height: number };

export function CanvasAgentSettings({ open, onClose, onOpenAssistant }: CanvasAgentSettingsProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const settings = usePetSettings();
    const { message } = App.useApp();
    const [pending, setPending] = useState<PendingUpload | null>(null);

    // 把用户上传的也构造成 PetCharacter，给预览用；列表合并 Neowow + uploaded
    const uploadedCharacters = useMemo(
        () => settings.uploadedPets.map((p) => makeUploadedSpritePetCharacter(p)),
        [settings.uploadedPets],
    );
    const allCharacters = useMemo(() => [...PET_CHARACTERS, ...uploadedCharacters], [uploadedCharacters]);
    const selectedCharacter = allCharacters.find((c) => c.id === settings.character) ?? PET_CHARACTERS[0];

    const handleOpenAssistant = useCallback(() => {
        onClose();
        onOpenAssistant();
    }, [onClose, onOpenAssistant]);

    const previewSize = Math.min(96, Math.max(56, settings.size));

    // 上传文件 -> 拿到 url -> 测真实尺寸 -> 弹配置 Modal
    const handleUpload: NonNullable<UploadProps["beforeUpload"]> = useCallback(
        async (file) => {
            try {
                const meta = await uploadAssetMediaFile(file as File, "pet-image");
                const { width, height } = await new Promise<{ width: number; height: number }>((resolve) => {
                    const img = new Image();
                    img.onload = () => resolve({ width: img.naturalWidth, height: img.naturalHeight });
                    img.onerror = () => resolve({ width: 64, height: 64 });
                    img.src = meta.url;
                });
                setPending({
                    file: file as File,
                    uploaded: { url: meta.url, storageKey: meta.storageKey, bytes: meta.bytes, mimeType: meta.mimeType },
                    width,
                    height,
                });
            } catch (err) {
                message.error(`上传失败：${err instanceof Error ? err.message : "未知错误"}`);
            }
            return false;
        },
        [message],
    );

    const handleConfirmSprite = useCallback(
        (config: SpriteConfig) => {
            if (!pending) return;
            const uploaded: UploadedPetMeta = {
                id: nanoid(),
                name: config.name,
                url: pending.uploaded.url,
                storageKey: pending.uploaded.storageKey,
                bytes: pending.uploaded.bytes,
                mimeType: pending.uploaded.mimeType,
                width: pending.width,
                height: pending.height,
                createdAt: new Date().toISOString(),
                cols: config.cols,
                rows: config.rows,
                cellW: config.cellW,
                cellH: config.cellH,
                idleStart: config.idleStart,
                idleEnd: config.idleEnd,
                idleFps: config.idleFps,
                clickStart: config.clickEnabled ? config.clickStart : undefined,
                clickEnd: config.clickEnabled ? config.clickEnd : undefined,
                clickFps: config.clickEnabled ? config.clickFps : undefined,
            };
            settings.addUploadedPet(uploaded);
            settings.setCharacter(`uploaded-${uploaded.id}`);
            message.success(`已添加桌宠「${uploaded.name}」`);
            setPending(null);
        },
        [pending, settings, message],
    );

    const handleRemove = useCallback(
        async (id: string, name: string) => {
            const ok = await new Promise<boolean>((resolve) => {
                Modal.confirm({
                    title: `删除「${name}」？`,
                    content: "桌宠图片会从账号里同步删除。",
                    okText: "删除",
                    okType: "danger",
                    cancelText: "取消",
                    onOk: () => resolve(true),
                    onCancel: () => resolve(false),
                });
            });
            if (!ok) return;
            try {
                await settings.removeUploadedPet(id);
                message.success(`已删除「${name}」`);
            } catch (err) {
                message.error(`删除失败：${err instanceof Error ? err.message : "未知错误"}`);
            }
        },
        [settings, message],
    );

    return (
        <Modal title="桌宠设置" open={open} onCancel={onClose} footer={null} centered width={380} destroyOnClose>
            <Space direction="vertical" size="middle" className="w-full pt-2">
                <div className="flex justify-center">
                    <div
                        className="rounded-2xl p-4"
                        style={{ background: theme.toolbar.panel, border: `1px solid ${theme.toolbar.border}` }}
                    >
                        <CanvasAgentCharacter character={selectedCharacter} stateName={selectedCharacter.defaultState} size={previewSize} title="桌宠预览" />
                    </div>
                </div>

                <Row label="显示桌宠">
                    <Switch checked={settings.enabled} onChange={settings.setEnabled} />
                </Row>

                <div>
                    <Row label="桌宠">
                        <Select
                            value={settings.character}
                            onChange={settings.setCharacter}
                            options={[
                                ...PET_CHARACTERS.map((item) => ({ value: item.id, label: item.name })),
                                ...settings.uploadedPets.map((p) => ({ value: `uploaded-${p.id}`, label: `📎 ${p.name}` })),
                            ]}
                            style={{ width: 200 }}
                        />
                    </Row>
                    <div className="pl-[96px] pt-1.5">
                        <Upload accept="image/*" beforeUpload={handleUpload} showUploadList={false} maxCount={1}>
                            <Button size="small" icon={<UploadIcon className="size-3.5" />}>
                                上传桌宠图片
                            </Button>
                        </Upload>
                    </div>
                </div>

                {settings.uploadedPets.length > 0 ? (
                    <div>
                        <div className="text-xs opacity-60">我的桌宠（{settings.uploadedPets.length}）</div>
                        <div className="mt-1 max-h-32 space-y-1 overflow-y-auto">
                            {settings.uploadedPets.map((p) => (
                                <div
                                    key={p.id}
                                    className="flex items-center justify-between rounded border px-2 py-1 text-sm"
                                    style={{ borderColor: theme.toolbar.border }}
                                >
                                    <button
                                        type="button"
                                        className="min-w-0 flex-1 truncate text-left"
                                        title={p.name}
                                        onClick={() => settings.setCharacter(`uploaded-${p.id}`)}
                                    >
                                        📎 {p.name}
                                    </button>
                                    <button
                                        type="button"
                                        className="ml-2 grid size-5 shrink-0 place-items-center rounded opacity-55 transition hover:bg-black/5 hover:opacity-100 dark:hover:bg-white/10"
                                        onClick={() => handleRemove(p.id, p.name)}
                                        aria-label={`删除桌宠 ${p.name}`}
                                    >
                                        <Trash2 className="size-3.5" />
                                    </button>
                                </div>
                            ))}
                        </div>
                    </div>
                ) : null}

                <Row label={`大小 ${settings.size}px`}>
                    <Slider min={PET_SIZE_MIN} max={PET_SIZE_MAX} value={settings.size} onChange={settings.setSize} style={{ width: 200 }} />
                </Row>

                <div>
                    <Row label="气泡提示">
                        <Segmented
                            value="canvas"
                            options={[
                                { value: "canvas", label: "画布中" },
                                { value: "global", label: "全局", disabled: true },
                            ]}
                        />
                    </Row>
                    <div className="pl-[96px] pt-1 text-xs opacity-55">目前只支持画布中</div>
                </div>

                <Row label="Agent 任务播报">
                    <Switch checked={settings.broadcastTasks} onChange={settings.setBroadcastTasks} />
                </Row>
                <Row label="节点完成提醒">
                    <Switch checked={settings.broadcastNodes} onChange={settings.setBroadcastNodes} />
                </Row>
                <Row label="生成中陪伴">
                    <Switch checked={settings.accompany} onChange={settings.setAccompany} />
                </Row>

                <div className="flex items-center justify-between pt-2">
                    <Button onClick={settings.resetPosition}>重置位置</Button>
                    <Button type="primary" onClick={handleOpenAssistant}>
                        打开助手面板
                    </Button>
                </div>
            </Space>

            <SpriteConfigModal
                pending={pending}
                onCancel={() => setPending(null)}
                onConfirm={handleConfirmSprite}
            />
        </Modal>
    );
}

type SpriteConfig = {
    name: string;
    cols: number;
    rows: number;
    cellW: number;
    cellH: number;
    idleStart: number;
    idleEnd: number;
    idleFps: number;
    clickEnabled: boolean;
    clickStart: number;
    clickEnd: number;
    clickFps: number;
};

function SpriteConfigModal({
    pending,
    onCancel,
    onConfirm,
}: {
    pending: PendingUpload | null;
    onCancel: () => void;
    onConfirm: (config: SpriteConfig) => void;
}) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const open = pending !== null;
    const defaultName = pending?.file.name.replace(/\.[^.]+$/, "") || "自定义桌宠";
    const [name, setName] = useState(defaultName);
    const [cols, setCols] = useState(4);
    const [rows, setRows] = useState(4);
    const [cellW, setCellW] = useState(128);
    const [cellH, setCellH] = useState(128);
    const [idleStart, setIdleStart] = useState(0);
    const [idleEnd, setIdleEnd] = useState(13);
    const [idleFps, setIdleFps] = useState(7);
    const [clickEnabled, setClickEnabled] = useState(false);
    const [clickStart, setClickStart] = useState(14);
    const [clickEnd, setClickEnd] = useState(17);
    const [clickFps, setClickFps] = useState(14);

    // 每次重新打开时把默认值同步
    useEffect(() => {
        if (pending) {
            setName(pending.file.name.replace(/\.[^.]+$/, "") || "自定义桌宠");
            setCols(4);
            setRows(4);
            setCellW(128);
            setCellH(128);
            setIdleStart(0);
            setIdleEnd(Math.max(0, 4 * 4 - 1));
            setIdleFps(7);
            setClickEnabled(false);
            setClickStart(Math.min(15, 4 * 4 - 1));
            setClickEnd(Math.min(15, 4 * 4 - 1));
            setClickFps(14);
        }
    }, [pending]);

    const totalFrames = cols * rows;
    const lastFrame = Math.max(0, totalFrames - 1);

    const idleValid = idleEnd >= idleStart && idleStart >= 0 && idleEnd <= lastFrame;
    const clickValid = !clickEnabled || (clickEnd >= clickStart && clickStart >= 0 && clickEnd <= lastFrame);

    if (!pending) return null;

    const previewMeta = makeUploadedSpritePetCharacter({
        id: "__preview__",
        name,
        url: pending.uploaded.url,
        storageKey: pending.uploaded.storageKey,
        bytes: pending.uploaded.bytes,
        mimeType: pending.uploaded.mimeType,
        width: pending.width,
        height: pending.height,
        createdAt: new Date().toISOString(),
        cols,
        rows,
        cellW,
        cellH,
        idleStart,
        idleEnd,
        idleFps,
        clickStart: clickEnabled ? clickStart : undefined,
        clickEnd: clickEnabled ? clickEnd : undefined,
        clickFps: clickEnabled ? clickFps : undefined,
    });

    return (
        <Modal
            title="配置桌宠参数"
            open={open}
            onCancel={onCancel}
            width={520}
            okButtonProps={{ disabled: !idleValid || !clickValid || !name.trim() }}
            onOk={() =>
                onConfirm({
                    name: name.trim() || defaultName,
                    cols,
                    rows,
                    cellW,
                    cellH,
                    idleStart,
                    idleEnd,
                    idleFps,
                    clickEnabled,
                    clickStart,
                    clickEnd,
                    clickFps,
                })
            }
        >
            <div className="grid grid-cols-[1fr_120px] gap-4 py-2">
                <div className="space-y-3">
                    <Field label="桌宠名字">
                        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={defaultName} />
                    </Field>

                    <div className="grid grid-cols-2 gap-3">
                        <Field label="列数 cols">
                            <InputNumber min={1} max={64} value={cols} onChange={(v) => setCols(Number(v) || 1)} className="w-full" />
                        </Field>
                        <Field label="行数 rows">
                            <InputNumber min={1} max={64} value={rows} onChange={(v) => setRows(Number(v) || 1)} className="w-full" />
                        </Field>
                        <Field label="单帧宽 (px)">
                            <InputNumber min={8} max={1024} value={cellW} onChange={(v) => setCellW(Number(v) || 8)} className="w-full" />
                        </Field>
                        <Field label="单帧高 (px)">
                            <InputNumber min={8} max={1024} value={cellH} onChange={(v) => setCellH(Number(v) || 8)} className="w-full" />
                        </Field>
                    </div>

                    <Divider plain className="!my-2 text-xs">待机动画（必填）</Divider>
                    <div className="grid grid-cols-3 gap-3">
                        <Field label="起始帧">
                            <InputNumber min={0} max={lastFrame} value={idleStart} onChange={(v) => setIdleStart(Number(v) || 0)} className="w-full" />
                        </Field>
                        <Field label="结束帧">
                            <InputNumber min={0} max={lastFrame} value={idleEnd} onChange={(v) => setIdleEnd(Number(v) || 0)} className="w-full" />
                        </Field>
                        <Field label="帧率 fps">
                            <InputNumber min={1} max={30} value={idleFps} onChange={(v) => setIdleFps(Number(v) || 1)} className="w-full" />
                        </Field>
                    </div>
                    <div className="text-xs opacity-55">总 {totalFrames} 帧（{cols}×{rows}），帧索引 0-{lastFrame}，从左到右、从上到下编号</div>

                    <Divider plain className="!my-2 text-xs">点击动画（可选）</Divider>
                    <div className="flex items-center gap-2">
                        <Switch size="small" checked={clickEnabled} onChange={setClickEnabled} />
                        <span className="text-xs opacity-65">启用后，点击 / 双击桌宠播放这段动画（once），其余状态继续用待机</span>
                    </div>
                    {clickEnabled ? (
                        <div className="grid grid-cols-3 gap-3">
                            <Field label="起始帧">
                                <InputNumber min={0} max={lastFrame} value={clickStart} onChange={(v) => setClickStart(Number(v) || 0)} className="w-full" />
                            </Field>
                            <Field label="结束帧">
                                <InputNumber min={0} max={lastFrame} value={clickEnd} onChange={(v) => setClickEnd(Number(v) || 0)} className="w-full" />
                            </Field>
                            <Field label="帧率 fps">
                                <InputNumber min={1} max={30} value={clickFps} onChange={(v) => setClickFps(Number(v) || 1)} className="w-full" />
                            </Field>
                        </div>
                    ) : null}
                </div>

                <div className="flex flex-col items-center gap-1">
                    <div
                        className="flex w-full items-center justify-center rounded-xl border p-2"
                        style={{ background: theme.toolbar.panel, borderColor: theme.toolbar.border, minHeight: 96 }}
                    >
                        <CanvasAgentCharacter character={previewMeta} stateName={previewMeta.defaultState} size={88} title="配置预览" />
                    </div>
                    <div className="text-[10px] opacity-55">实时预览</div>
                </div>
            </div>

            {(!idleValid || !clickValid) ? (
                <div className="mt-2 text-xs text-red-500">
                    {!idleValid ? `待机帧段须在 0-${lastFrame} 范围内且起 ≤ 止。` : null}
                    {!clickValid ? `点击帧段须在 0-${lastFrame} 范围内且起 ≤ 止。` : null}
                </div>
            ) : null}
        </Modal>
    );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <div className="space-y-1">
            <div className="text-xs opacity-65">{label}</div>
            {children}
        </div>
    );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <div className="flex items-center justify-between">
            <span className="text-sm opacity-75">{label}</span>
            <div className="flex items-center">{children}</div>
        </div>
    );
}
