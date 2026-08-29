"use client";

import { useEffect, useRef, useState } from "react";

import {
    findState,
    framePosition,
    type PetCharacter,
} from "../agent/pet-characters";

export type CanvasAgentCharacterProps = {
    character: PetCharacter;
    stateName?: string;
    /** 显示高度（px），宽度按 cell 比例自动。 */
    size?: number;
    className?: string;
    title?: string;
};

function nextFrame(prev: number, length: number, loop: "loop" | "once" | "pingpong", dirRef: { current: 1 | -1 }): number {
    if (length <= 1) return 0;
    if (loop === "once") return Math.min(prev + 1, length - 1);
    if (loop === "pingpong") {
        let next = prev + dirRef.current;
        if (next >= length - 1) {
            dirRef.current = -1;
            next = length - 1;
        } else if (next <= 0) {
            dirRef.current = 1;
            next = 0;
        }
        return next;
    }
    return (prev + 1) % length;
}

export function CanvasAgentCharacter({
    character,
    stateName = character.defaultState,
    size = 72,
    className,
    title,
}: CanvasAgentCharacterProps) {
    const state = findState(character, stateName) ?? character.states[0];
    const [frameIndex, setFrameIndex] = useState(0);
    const dirRef = useRef<1 | -1>(1);

    useEffect(() => {
        setFrameIndex(0);
        dirRef.current = 1;
        const intervalMs = Math.max(40, 1000 / state.fps);
        const timer = window.setInterval(() => {
            setFrameIndex((prev) => nextFrame(prev, state.frames.length, state.loop, dirRef));
        }, intervalMs);
        return () => window.clearInterval(timer);
    }, [state, stateName]);

    const frame = state.frames[Math.min(frameIndex, state.frames.length - 1)];
    const pos = framePosition(character, frame);
    const grid = character.grid;
    const aspect = grid.cellW / grid.cellH;
    const width = Math.round(size * aspect);
    const height = size;
    // sheet 等比缩放，让单个 cell 正好铺满容器；否则 192×208 的 cell 在 ~66×72 容器里
    // 只显示 cell 左上角一小片，截掉大半（之前只能看到"半个头"就是这个原因）。
    const cellScale = size / grid.cellH;

    return (
        <div
            role={title ? "img" : undefined}
            aria-label={title}
            title={title}
            className={className}
            style={{
                width,
                height,
                backgroundImage: `url(${character.sheet.url})`,
                backgroundSize: `${character.sheet.width * cellScale}px ${character.sheet.height * cellScale}px`,
                backgroundPosition: `-${pos.x * cellScale}px -${pos.y * cellScale}px`,
                backgroundRepeat: "no-repeat",
            }}
        />
    );
}
