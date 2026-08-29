import { nanoid } from "nanoid";
import type { Chapter } from "../types";

/**
 * 把原始小说按"第N章 / 章节标题"切分为章节列表。
 * 支持：第1章 / 第一章 / Chapter 1 等常见写法；无标题时整体归为一章。
 */
export function parseChapters(script: string): Chapter[] {
    if (!script.trim()) return [];
    const lines = script.split(/\n/);
    // 匹配章节标题行：第1章 / 第一章 / 第 12 章 / 第三卷 / Chapter 3 等
    // 说明：不能用 \b（中文后 \b 不生效），改用"数字/中文数字 + 章回节卷 + 结尾或分隔符"结构，
    // 这样既能匹配裸标题"第一章"，也能匹配"第1章 关于…"，同时不会误命中正文里的"这是第一章的内容"。
    const titleReg = /^[\s　]*(第[\s　]*[0-9零一二三四五六七八九十百千两]+[\s　]*[章回节卷]|Chapter\s*\d+|CHAPTER\s*\d+)(?:[\s　：:、.．\-—].*)?$/i;
    const chapters: Chapter[] = [];
    let cur: { title: string; body: string[] } | null = null;
    const pushCur = () => {
        // 只有"有标题"的段落才算正式章节；标题为空的前置段落（书名/简介/标签等）直接丢弃。
        if (cur && cur.title.trim() && cur.body.join("").trim() !== undefined) {
            chapters.push({ id: nanoid(), title: cur.title.trim(), content: cur.body.join("\n").trim() });
        }
    };
    for (const line of lines) {
        // 跳过整行的分隔线（如 ==== / ---- ）
        if (/^\s*[=\-*]{5,}\s*$/.test(line)) continue;
        if (titleReg.test(line.trim())) {
            pushCur();
            cur = { title: line.trim(), body: [] };
        } else {
            if (!cur) cur = { title: "", body: [] };
            cur.body.push(line);
        }
    }
    pushCur();
    // 一章都没识别出来 → 整篇作为单章
    if (chapters.length === 0) {
        return [{ id: nanoid(), title: "全文", content: script.trim() }];
    }
    return chapters;
}
