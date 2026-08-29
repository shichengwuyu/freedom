/**
 * 桌宠角色配置（数据驱动）。
 * 结构对齐 neowow.cn 桌宠详情接口的 jsonData：sheet/grid/states/interactions。
 */
export type PetSheetSpec = {
    url: string;
    width: number;
    height: number;
};

export type PetGridSpec = {
    cols: number;
    rows: number;
    cellW: number;
    cellH: number;
    offsetX: number;
    offsetY: number;
    gapX: number;
    gapY: number;
};

export type PetLoopMode = "loop" | "once" | "pingpong";

export type PetStateSpec = {
    name: string;
    frames: number[];
    fps: number;
    loop: PetLoopMode;
};

export type PetBindings = {
    idle: string;
    hover: string;
    press: string;
    dragLeft: string;
    dragRight: string;
    dragUp: string;
    dragDown: string;
    drag: string;
    click: string;
    dblclick: string;
    message: string;
    sleep: string;
    edgeSnap: string;
    edgeRight: string;
};

export type PetCharacter = {
    id: string;
    name: string;
    sheet: PetSheetSpec;
    grid: PetGridSpec;
    states: PetStateSpec[];
    bindings: PetBindings;
    dragTilt: boolean;
    tiltMaxDeg: number;
    idleTimeoutMs: number;
    defaultState: string;
};

/** 根据帧索引计算其在 sheet 上的像素偏移（col/row 从左到右、从上到下编号）。 */
export function framePosition(character: PetCharacter, frame: number): { x: number; y: number } {
    const grid = character.grid;
    const col = frame % grid.cols;
    const row = Math.floor(frame / grid.cols);
    return {
        x: grid.offsetX + col * (grid.cellW + grid.gapX),
        y: grid.offsetY + row * (grid.cellH + grid.gapY),
    };
}

/** 播放完整个 state 动画所需毫秒。 */
export function stateDuration(state: PetStateSpec): number {
    return (state.frames.length / state.fps) * 1000;
}

export function findState(character: PetCharacter, name: string): PetStateSpec | undefined {
    return character.states.find((state) => state.name === name);
}

export const NEOWOW_CHARACTER: PetCharacter = {
    id: "neowow",
    name: "Neowow",
    sheet: {
        url: "/neowow-sheet.webp",
        width: 3072,
        height: 2080,
    },
    grid: {
        cols: 16,
        rows: 10,
        cellW: 192,
        cellH: 208,
        offsetX: 0,
        offsetY: 0,
        gapX: 0,
        gapY: 0,
    },
    states: [
        {
            name: "待机",
            frames: [
                0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13,
                48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77,
                96, 97, 98, 99, 100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125,
                128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143,
            ],
            fps: 7,
            loop: "loop",
        },
        { name: "悬停", frames: [48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61], fps: 10, loop: "loop" },
        { name: "按下", frames: [112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123], fps: 8, loop: "loop" },
        { name: "右拖", frames: [16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31], fps: 14, loop: "loop" },
        { name: "左拖", frames: [32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47], fps: 14, loop: "loop" },
        { name: "上拖", frames: [144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159], fps: 12, loop: "loop" },
        { name: "下拖", frames: [80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93], fps: 8, loop: "pingpong" },
        { name: "拖拽", frames: [16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31], fps: 14, loop: "loop" },
        { name: "点击", frames: [48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63], fps: 14, loop: "once" },
        { name: "双击", frames: [144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159], fps: 14, loop: "once" },
        { name: "消息", frames: [64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77], fps: 8, loop: "loop" },
        { name: "睡眠", frames: [128, 129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 139, 140, 141, 142, 143], fps: 5, loop: "loop" },
        { name: "吸附", frames: [144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159], fps: 14, loop: "once" },
        { name: "贴边右", frames: [144, 145, 146, 147, 148, 149, 150, 151, 152, 153, 154, 155, 156, 157, 158, 159], fps: 5, loop: "pingpong" },
    ],
    bindings: {
        idle: "待机",
        hover: "悬停",
        press: "按下",
        dragLeft: "左拖",
        dragRight: "右拖",
        dragUp: "上拖",
        dragDown: "下拖",
        drag: "拖拽",
        click: "点击",
        dblclick: "双击",
        message: "消息",
        sleep: "睡眠",
        edgeSnap: "吸附",
        edgeRight: "贴边右",
    },
    dragTilt: true,
    tiltMaxDeg: 22,
    idleTimeoutMs: 12000,
    defaultState: "待机",
};

export const PET_CHARACTERS: PetCharacter[] = [NEOWOW_CHARACTER];

import type { UploadedPetMeta } from "@/stores/use-pet-settings";

/**
 * 把用户上传的 sprite sheet + 填的网格/帧段构造为完整的 PetCharacter：
 * - 必有「待机」状态（idleStart..idleEnd 区间，loop）
 * - 可选「点击」状态（clickStart..clickEnd 区间，once）；未填则点击用待机
 * - 其他交互状态全部映射到「待机」或「点击」
 *
 * 沿用 CanvasAgentCharacter 渲染器，无需改动 sprite 管线。
 */
export function makeUploadedSpritePetCharacter(meta: UploadedPetMeta): PetCharacter {
    const {
        id,
        name,
        url,
        width,
        height,
        cols,
        rows,
        cellW,
        cellH,
        idleStart,
        idleEnd,
        idleFps,
        clickStart,
        clickEnd,
        clickFps,
    } = meta;

    const total = cols * rows;
    const range = (start: number, end: number): number[] => {
        const s = Math.max(0, Math.min(total - 1, Math.floor(start)));
        const e = Math.max(s, Math.min(total - 1, Math.floor(end)));
        const out: number[] = [];
        for (let i = s; i <= e; i++) out.push(i);
        return out;
    };

    const idleName = "待机";
    const clickName = "点击";
    const hasClick = clickStart !== undefined && clickEnd !== undefined && clickFps !== undefined;

    const states: PetStateSpec[] = [
        { name: idleName, frames: range(idleStart, idleEnd), fps: idleFps, loop: "loop" },
    ];
    if (hasClick) {
        states.push({ name: clickName, frames: range(clickStart!, clickEnd!), fps: clickFps!, loop: "once" });
    }

    // 点击 / 双击用点击动画（如果有），其他全部走待机。
    const pressState = hasClick ? clickName : idleName;

    return {
        id: `uploaded-${id}`,
        name,
        sheet: { url, width, height },
        grid: {
            cols,
            rows,
            cellW,
            cellH,
            offsetX: 0,
            offsetY: 0,
            gapX: 0,
            gapY: 0,
        },
        states,
        bindings: {
            idle: idleName,
            hover: idleName,
            press: pressState,
            dragLeft: idleName,
            dragRight: idleName,
            dragUp: idleName,
            dragDown: idleName,
            drag: idleName,
            click: pressState,
            dblclick: pressState,
            message: idleName,
            sleep: idleName,
            edgeSnap: idleName,
            edgeRight: idleName,
        },
        dragTilt: false,
        tiltMaxDeg: 0,
        idleTimeoutMs: 12000,
        defaultState: idleName,
    };
}
