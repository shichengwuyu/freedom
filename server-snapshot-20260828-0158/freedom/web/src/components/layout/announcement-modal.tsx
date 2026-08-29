"use client";

import { Bell, X } from "lucide-react";
import { App, Button, Modal, Switch, Tag, Typography, theme } from "antd";
import dayjs from "dayjs";
import localforage from "localforage";
import { useCallback, useEffect, useState } from "react";

import { getLatestAnnouncements, type AnnouncementItem } from "@/services/api/announcement";
import { useConfigStore } from "@/stores/use-config-store";
import { useEffectiveConfig } from "@/stores/use-config-store";

const STORAGE_KEY = "announcement:dismiss";
const MAX_DISPLAY = 10;

type DismissRecord = {
    forever: boolean; // 以后不再提醒
    dismissDate?: string; // YYYY-MM-DD 今日不再提醒的日期
};

// 计算相对时间（手动实现，避免 dayjs relativeTime 插件依赖）
function formatRelative(dateStr: string): string {
    const now = dayjs();
    const target = dayjs(dateStr);
    const diffMs = now.valueOf() - target.valueOf();
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return "刚刚";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}分钟前`;
    const diffHour = Math.floor(diffMin / 60);
    if (diffHour < 24) return `${diffHour}小时前`;
    const diffDay = Math.floor(diffHour / 24);
    if (diffDay < 30) return `${diffDay}天前`;
    const diffMonth = Math.floor(diffDay / 30);
    if (diffMonth < 12) return `${diffMonth}个月前`;
    const diffYear = Math.floor(diffMonth / 12);
    return `${diffYear}年前`;
}

// 绝对时间格式化： YYYY/M/D HH:mm:ss
function formatAbsolute(dateStr: string): string {
    return dayjs(dateStr).format("YYYY/M/D HH:mm:ss");
}

function todayKey(): string {
    return dayjs().format("YYYY-MM-DD");
}

async function readDismiss(): Promise<DismissRecord> {
    try {
        const raw = (await localforage.getItem(STORAGE_KEY)) as DismissRecord | null;
        return raw ?? { forever: false };
    } catch {
        return { forever: false };
    }
}

async function writeDismiss(rec: DismissRecord) {
    try {
        await localforage.setItem(STORAGE_KEY, rec);
    } catch {
        // ignore
    }
}

export function AnnouncementModal() {
    const { token } = theme.useToken();
    const { message } = App.useApp();
    const siteNotice = useConfigStore((state) => state.publicSettings?.siteNotice);
    const _ = useEffectiveConfig(); // 确保 store 初始化

    const [open, setOpen] = useState(false);
    const [items, setItems] = useState<AnnouncementItem[]>([]);
    const [neverAgain, setNeverAgain] = useState(false);
    const [initialized, setInitialized] = useState(false);

    const latestTime = items[0]?.createdAt;
    const modalTitle = siteNotice?.title?.trim() || "系统公告";
    const enabled = siteNotice?.enabled !== false;

    const loadAndDecide = useCallback(async () => {
        try {
            const [resp, dismiss] = await Promise.all([
                getLatestAnnouncements().catch(() => ({ items: [] as AnnouncementItem[] })),
                readDismiss(),
            ]);
            const list = (resp?.items ?? []).slice(0, MAX_DISPLAY);
            setItems(list);
            setNeverAgain(!!dismiss.forever);
            if (!enabled) return;
            if (list.length === 0) return;
            if (dismiss.forever) return;
            if (dismiss.dismissDate && dismiss.dismissDate === todayKey()) return;
            setOpen(true);
        } catch (err) {
            message.error(err instanceof Error ? err.message : "公告加载失败");
        } finally {
            setInitialized(true);
        }
    }, [enabled, message]);

    useEffect(() => {
        void loadAndDecide();
    }, [loadAndDecide]);

    const handleClose = useCallback(() => {
        setOpen(false);
    }, []);

    const handleDismissToday = useCallback(async () => {
        await writeDismiss({ forever: neverAgain, dismissDate: todayKey() });
        setOpen(false);
    }, [neverAgain]);

    const handleNeverToggle = useCallback(async (checked: boolean) => {
        setNeverAgain(checked);
        const current = await readDismiss();
        const next: DismissRecord = { ...current, forever: checked };
        if (!checked) {
            // 取消"以后不再提醒"时，如果今日已被静默也一并清除，便于立刻验证效果
            delete next.dismissDate;
        }
        await writeDismiss(next);
    }, []);

    if (!initialized && items.length === 0) return null;

    return (
        <Modal
            open={open}
            centered
            closable={false}
            width={720}
            maskClosable={false}
            keyboard
            onCancel={handleClose}
            title={null}
            footer={null}
            styles={{ body: { padding: 0 } }}
            classNames={{ body: "!rounded-2xl" }}
        >
            <div
                style={{
                    padding: "16px 20px",
                    borderBottom: `1px solid ${token.colorBorderSecondary}`,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                }}
            >
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <Bell className="size-5" style={{ color: token.colorWarning }} />
                    <Typography.Text strong style={{ fontSize: 17, color: token.colorText }}>
                        {modalTitle}
                    </Typography.Text>
                    <Tag color="blue" style={{ margin: 0, borderRadius: 999 }}>
                        显示最新{MAX_DISPLAY}条
                    </Tag>
                    {latestTime && (
                        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                            {formatAbsolute(latestTime)}
                        </Typography.Text>
                    )}
                </div>
                <Button
                    type="text"
                    icon={<X className="size-4" />}
                    onClick={handleClose}
                    aria-label="关闭公告"
                    style={{ color: token.colorTextTertiary }}
                />
            </div>

            <div
                style={{
                    maxHeight: "55vh",
                    overflowY: "auto",
                    padding: "8px 12px 20px 12px",
                    background: token.colorBgContainer,
                }}
            >
                {items.length === 0 ? (
                    <div style={{ padding: "60px 0", textAlign: "center", color: token.colorTextTertiary }}>
                        暂无公告
                    </div>
                ) : (
                    <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
                        {items.map((it, idx) => (
                            <li
                                key={it.id}
                                style={{
                                    position: "relative",
                                    padding: "14px 10px 14px 30px",
                                    borderBottom:
                                        idx === items.length - 1
                                            ? "none"
                                            : `1px dashed ${token.colorBorderSecondary}`,
                                }}
                            >
                                {/* 时间轴圆点 */}
                                <span
                                    aria-hidden
                                    style={{
                                        position: "absolute",
                                        left: 10,
                                        top: 22,
                                        width: 10,
                                        height: 10,
                                        borderRadius: "50%",
                                        background: token.colorPrimary,
                                        boxShadow: `0 0 0 3px ${token.colorPrimaryBg}`,
                                    }}
                                />
                                {/* 内容行 */}
                                <Typography.Paragraph
                                    style={{
                                        margin: 0,
                                        fontSize: 14,
                                        lineHeight: 1.7,
                                        color: token.colorText,
                                        whiteSpace: "pre-wrap",
                                        wordBreak: "break-word",
                                    }}
                                >
                                    {it.content}
                                </Typography.Paragraph>
                                {/* 时间行 */}
                                <div
                                    style={{
                                        marginTop: 8,
                                        fontSize: 12,
                                        color: token.colorTextTertiary,
                                        display: "flex",
                                        gap: 10,
                                        alignItems: "center",
                                    }}
                                >
                                    <span>{formatRelative(it.createdAt)}</span>
                                    <span style={{ opacity: 0.9 }}>{formatAbsolute(it.createdAt)}</span>
                                </div>
                            </li>
                        ))}
                    </ul>
                )}
            </div>

            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "12px 20px",
                    borderTop: `1px solid ${token.colorBorderSecondary}`,
                    background: token.colorBgElevated,
                    borderRadius: "0 0 16px 16px",
                }}
            >
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <Typography.Text style={{ fontSize: 13, color: token.colorTextSecondary }}>
                        以后不再提醒
                    </Typography.Text>
                    <Switch checked={neverAgain} onChange={handleNeverToggle} size="default" />
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <Button onClick={handleDismissToday}>今日不再提醒</Button>
                    <Button type="primary" onClick={handleClose}>
                        关闭
                    </Button>
                </div>
            </div>
        </Modal>
    );
}
