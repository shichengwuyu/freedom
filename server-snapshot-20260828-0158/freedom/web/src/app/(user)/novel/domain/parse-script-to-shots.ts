import { nanoid } from "nanoid";
import type { Shot } from "../types";

export function parseScriptToShots(script: string, duration: number): Shot[] {
    const lines = script.split(/\n/).filter((l) => l.trim());
    const shots: Shot[] = [];
    let current: { lines: string[]; title: string } | null = null;

    for (const line of lines) {
        if (/【场景\d+】/.test(line) || /^分镜[—\-—-]?\d*/i.test(line) || /^视频\d+$/i.test(line)) {
            if (current?.lines.length) {
                shots.push({ id: nanoid(), title: current.title, content: current.lines.join("\n"), duration, status: "idle", selected: false, referencedAssetIds: [] });
            }
            const match = line.match(/【场景(\d+)】/);
            current = { title: match ? `场景${match[1]}` : `分镜${shots.length + 1}`, lines: [line] };
        } else {
            if (!current) current = { title: `场景${shots.length + 1}`, lines: [] };
            current.lines.push(line);
        }
    }
    if (current?.lines.length) {
        shots.push({ id: nanoid(), title: current.title, content: current.lines.join("\n"), duration, status: "idle", selected: false, referencedAssetIds: [] });
    }
    return shots.length ? shots : [{ id: nanoid(), title: "场景1", content: script, duration, status: "idle", selected: false, referencedAssetIds: [] }];
}
