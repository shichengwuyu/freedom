"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";

import { canvasThemes } from "@/lib/canvas-theme";
import { useThemeStore } from "@/stores/use-theme-store";
import { usePetSettings } from "@/stores/use-pet-settings";
import { PET_CHARACTERS, findState, makeUploadedSpritePetCharacter, stateDuration, type PetCharacter } from "../agent/pet-characters";
import { CanvasAgentCharacter } from "./canvas-agent-character";

const EDGE_PADDING = 32;
const TOP_PADDING = 80;
const BOTTOM_PADDING = 100;
const TAP_THRESHOLD = 6;
const BUBBLE_MS = 4200;
const DEFAULT_Y_RATIO = 0.6;

const PET_LINES = [
    "一段旁白，我帮你配上声音~",
    "需要生图还是配乐？跟我说~",
    "这几个节点要不要我帮你连起来？",
    "有灵感了？丢给我~",
    "在呢，随时开工~",
    "要不要我把剧本拆成分镜？",
];

/** 根据持久化的 yRatio（0-1）和当前窗口尺寸计算桌宠的 y 像素位置。 */
function computeY(yRatio: number | null, petSize: number): number {
    if (typeof window === "undefined") return 200;
    const span = Math.max(0, window.innerHeight - BOTTOM_PADDING - petSize - TOP_PADDING);
    const ratio = yRatio ?? DEFAULT_Y_RATIO;
    return TOP_PADDING + Math.max(0, Math.min(1, ratio)) * span;
}

export type CanvasAgentPetProps = {
    onOpenAssistant: () => void;
};

export function CanvasAgentPet({ onOpenAssistant }: CanvasAgentPetProps) {
    const theme = canvasThemes[useThemeStore((state) => state.theme)];
    const petSettings = usePetSettings();

    // 选中的桌宠：内置 Neowow 或用户上传的动态 sprite；查不到时回退到首个内置角色。
    const character: PetCharacter =
        PET_CHARACTERS.find((item) => item.id === petSettings.character) ??
        (() => {
            const target = petSettings.uploadedPets.find((p) => `uploaded-${p.id}` === petSettings.character);
            return target ? makeUploadedSpritePetCharacter(target) : PET_CHARACTERS[0];
        })();
    const bindings = character.bindings;
    const petSize = petSettings.size;
    const enabled = petSettings.enabled;
    const resetToken = petSettings.resetToken;
    const grid = character.grid;
    const petWidth = Math.round(petSize * (grid.cellW / grid.cellH));

    // side / y 直接从 store 派生，确保刷新后位置保留。
    const side: "left" | "right" = petSettings.side ?? "right";
    const y = computeY(petSettings.yRatio, petSize);

    const [dragging, setDragging] = useState(false);
    const [dragX, setDragX] = useState(0);
    const [dragY, setDragY] = useState<number | null>(null);
    const [bubble, setBubble] = useState<string | null>(null);
    const [stateName, setStateName] = useState<string>(character.defaultState);
    const [dragDir, setDragDir] = useState<"left" | "right" | "up" | "down" | null>(null);

    const dragRef = useRef<{ startX: number; startY: number; originX: number; originY: number; moved: boolean } | null>(null);
    const sideRef = useRef<"left" | "right">(side);
    const bubbleTimerRef = useRef<number | null>(null);
    const stateTimerRef = useRef<number | null>(null);
    const draggingRef = useRef(false);
    const stateNameRef = useRef<string>(character.defaultState);

    sideRef.current = side;
    draggingRef.current = dragging;
    stateNameRef.current = stateName;

    /** 当前帧桌宠 y（拖动时跟随指针，否则取自持久化比例）。 */
    const currentY = dragging && dragY !== null ? dragY : y;

    const clampY = useCallback(
        (value: number) => {
            const max = (typeof window !== "undefined" ? window.innerHeight : 800) - BOTTOM_PADDING - petSize;
            return Math.max(TOP_PADDING, Math.min(max, value));
        },
        [petSize],
    );

    /** 把当前 y 转回可持久化的 yRatio 并写入 store。 */
    const persistCurrentY = useCallback(
        (value: number) => {
            const max = (typeof window !== "undefined" ? window.innerHeight : 800) - BOTTOM_PADDING - petSize;
            const span = Math.max(0, max - TOP_PADDING);
            const ratio = span > 0 ? (value - TOP_PADDING) / span : DEFAULT_Y_RATIO;
            petSettings.setYRatio(Math.max(0, Math.min(1, ratio)));
        },
        [petSettings, petSize],
    );

    const toRestState = useCallback(() => {
        setStateName(sideRef.current === "right" ? bindings.edgeRight : bindings.idle);
    }, [bindings]);

    const later = useCallback((fn: () => void, ms: number) => {
        if (stateTimerRef.current) window.clearTimeout(stateTimerRef.current);
        stateTimerRef.current = window.setTimeout(() => {
            stateTimerRef.current = null;
            fn();
        }, ms);
    }, []);

    // 随机间隔冒泡说话
    useEffect(() => {
        let nextTimer: number;
        const saySomething = () => {
            const line = PET_LINES[Math.floor(Math.random() * PET_LINES.length)];
            setBubble(line);
            if (!draggingRef.current) setStateName(bindings.message);
            if (bubbleTimerRef.current) window.clearTimeout(bubbleTimerRef.current);
            bubbleTimerRef.current = window.setTimeout(() => {
                setBubble(null);
                if (stateNameRef.current === bindings.message) setStateName(character.bindings.idle);
            }, BUBBLE_MS);
            nextTimer = window.setTimeout(saySomething, 9000 + Math.random() * 9000);
        };
        nextTimer = window.setTimeout(saySomething, 4000 + Math.random() * 4000);
        return () => {
            window.clearTimeout(nextTimer);
            if (bubbleTimerRef.current) window.clearTimeout(bubbleTimerRef.current);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // 外部播报消息（节点完成 / 生成陪伴）
    useEffect(() => {
        const external = petSettings.externalMessage;
        if (!external) return;
        setBubble(external);
        setStateName(bindings.message);
        if (bubbleTimerRef.current) window.clearTimeout(bubbleTimerRef.current);
        bubbleTimerRef.current = window.setTimeout(() => {
            setBubble(null);
            toRestState();
        }, BUBBLE_MS);
    }, [petSettings.externalMessage, bindings, toRestState]);

    // 静止状态（待机/贴边右）超时 → 睡眠
    useEffect(() => {
        const rest = [bindings.idle, bindings.edgeRight];
        if (!rest.includes(stateName)) return;
        const timer = window.setTimeout(() => setStateName(bindings.sleep), character.idleTimeoutMs);
        return () => window.clearTimeout(timer);
    }, [stateName, bindings, character.idleTimeoutMs]);

    // 收到重置令牌时回到默认位置（store 内已重置 side / yRatio，这里只清 UI 临时态）
    useEffect(() => {
        dragRef.current = null;
        setDragging(false);
        setDragX(0);
        setDragY(null);
        setBubble(null);
        setStateName(bindings.edgeRight);
    }, [resetToken, bindings]);

    // 组件卸载清理
    useEffect(() => {
        return () => {
            if (bubbleTimerRef.current) window.clearTimeout(bubbleTimerRef.current);
            if (stateTimerRef.current) window.clearTimeout(stateTimerRef.current);
        };
    }, []);

    if (!enabled) return null;

    const edgeX = (s: "left" | "right") => (s === "left" ? EDGE_PADDING : window.innerWidth - EDGE_PADDING - petWidth);

    const onPointerDown = useCallback(
        (event: ReactPointerEvent<HTMLDivElement>) => {
            setStateName(bindings.press);
            const originX = edgeX(sideRef.current);
            dragRef.current = { startX: event.clientX, startY: event.clientY, originX, originY: currentY, moved: false };
            event.currentTarget.setPointerCapture(event.pointerId);
            setDragging(true);
            setDragX(originX);
        },
        [bindings, currentY],
    );

    const onPointerMove = useCallback(
        (event: ReactPointerEvent<HTMLDivElement>) => {
            const drag = dragRef.current;
            if (!drag) return;
            const dx = event.clientX - drag.startX;
            const dy = event.clientY - drag.startY;
            if (Math.abs(dx) > TAP_THRESHOLD || Math.abs(dy) > TAP_THRESHOLD) drag.moved = true;
            if (drag.moved) {
                setBubble(null);
                setDragX(drag.originX + dx);
                const nextY = clampY(drag.originY + dy);
                setDragY(nextY);
                const dir = Math.abs(dx) >= Math.abs(dy) ? (dx >= 0 ? "right" : "left") : (dy >= 0 ? "down" : "up");
                setDragDir(dir);
                setStateName(dir === "left" ? bindings.dragLeft : dir === "right" ? bindings.dragRight : dir === "up" ? bindings.dragUp : bindings.dragDown);
            }
        },
        [clampY, bindings],
    );

    const onPointerUp = useCallback(
        (event: ReactPointerEvent<HTMLDivElement>) => {
            const drag = dragRef.current;
            dragRef.current = null;
            setDragging(false);
            setDragDir(null);
            if (!drag) return;
            if (!drag.moved) {
                // 点击：播一次点击动画后回静止
                setStateName(bindings.click);
                later(() => toRestState(), stateDuration(findState(character, bindings.click) ?? character.states[0]));
                onOpenAssistant();
                return;
            }
            // 拖拽松手：吸附动画后贴边，并把位置持久化到 store
            const rawEndX = drag.originX + (event.clientX - drag.startX);
            const endY = clampY(drag.originY + (event.clientY - drag.startY));
            const nextSide = rawEndX + petWidth / 2 < window.innerWidth / 2 ? "left" : "right";
            petSettings.setSide(nextSide);
            persistCurrentY(endY);
            setDragY(endY);
            setStateName(bindings.edgeSnap);
            later(() => setStateName(nextSide === "right" ? bindings.edgeRight : bindings.idle), stateDuration(findState(character, bindings.edgeSnap) ?? character.states[0]));
        },
        [bindings, character, later, onOpenAssistant, petWidth, petSettings, persistCurrentY, clampY, toRestState],
    );

    const onPointerEnter = useCallback(() => {
        setStateName(bindings.hover);
    }, [bindings]);

    const onPointerLeave = useCallback(() => {
        if (!draggingRef.current) toRestState();
    }, [toRestState]);

    const onDoubleClick = useCallback(() => {
        setStateName(bindings.dblclick);
        later(() => toRestState(), stateDuration(findState(character, bindings.dblclick) ?? character.states[0]));
    }, [bindings, character, later, toRestState]);

    const x = dragging ? dragX : edgeX(side);

    const tilt =
        dragging && character.dragTilt && dragDir
            ? (dragDir === "right" ? character.tiltMaxDeg * 0.5 : dragDir === "left" ? -character.tiltMaxDeg * 0.5 : 0)
            : 0;

    return (
        <div
            className="fixed z-[150] select-none"
            style={{
                left: x,
                top: currentY,
                width: petWidth,
                height: petSize,
                cursor: dragging ? "grabbing" : "grab",
                touchAction: "none",
                transform: `rotate(${tilt}deg)`,
                transformOrigin: "50% 100%",
                transition: dragging ? "none" : "left .35s cubic-bezier(.2,.8,.2,1), top .35s cubic-bezier(.2,.8,.2,1)",
            }}
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onPointerCancel={onPointerUp}
            onPointerEnter={onPointerEnter}
            onPointerLeave={onPointerLeave}
            onDoubleClick={onDoubleClick}
            role="button"
            aria-label="画布助手"
            title="拖动我 / 点击打开助手"
        >
            {bubble ? (
                <div
                    className="pointer-events-none absolute bottom-full mb-2 w-max max-w-[240px] rounded-xl border px-3 py-2 text-sm leading-5 shadow-lg"
                    style={{
                        background: theme.node.panel,
                        borderColor: theme.node.stroke,
                        color: theme.node.text,
                        left: side === "left" ? 0 : undefined,
                        right: side === "right" ? 0 : undefined,
                    }}
                >
                    {bubble}
                </div>
            ) : null}
            <CanvasAgentCharacter character={character} stateName={stateName} size={petSize} title="拖动我 / 点击打开助手" />
        </div>
    );
}
