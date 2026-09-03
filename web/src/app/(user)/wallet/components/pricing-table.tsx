"use client";

import { DollarOutlined, ReloadOutlined } from "@ant-design/icons";
import { Alert, App, Button, Empty, Skeleton, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import { useCallback, useEffect, useMemo, useState } from "react";

import { fetchPricing, type PricingModel, type PricingResponse } from "@/services/api/pricing";

const { Text } = Typography;

type PricingTableProps = {
    // 当前用户 groupId（高亮所在列；空字符串 = 未登录/默认）
    currentUserGroupId?: string;
};

const capabilityLabels: Record<string, { label: string; color: string }> = {
    text: { label: "文本", color: "blue" },
    image: { label: "图片", color: "green" },
    video: { label: "视频", color: "purple" },
    audio: { label: "音频", color: "orange" },
    embedding: { label: "向量", color: "cyan" },
};

const unitLabels: Record<string, string> = {
    per_call: " / 次",
    per_second: " / 秒",
};

export function PricingTable({ currentUserGroupId }: PricingTableProps) {
    const { message } = App.useApp();
    const [data, setData] = useState<PricingResponse | null>(null);
    const [loading, setLoading] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const res = await fetchPricing();
            setData(res);
        } catch (err) {
            const msg = err instanceof Error ? err.message : "加载价目表失败";
            message.error(msg);
        } finally {
            setLoading(false);
        }
    }, [message]);

    useEffect(() => {
        void load();
    }, [load]);

    const groups = useMemo(() => data?.groups ?? [], [data]);
    const models = useMemo(() => data?.models ?? [], [data]);

    // 表格列：基础信息列 + 每个 group 一列
    const columns = useMemo<ColumnsType<PricingModel>>(() => {
        const base: ColumnsType<PricingModel> = [
            {
                title: "模型",
                dataIndex: "model",
                key: "model",
                width: 220,
                render: (m: string, record) => (
                    <Space direction="vertical" size={0}>
                        <Text strong>{record.label || m}</Text>
                        {record.label && <Text type="secondary" style={{ fontSize: 12 }}>{m}</Text>}
                    </Space>
                ),
            },
            {
                title: "能力",
                dataIndex: "capability",
                key: "capability",
                width: 80,
                render: (c: string) => {
                    const info = capabilityLabels[c];
                    if (!info) return <Text type="secondary">-</Text>;
                    return <Tag color={info.color}>{info.label}</Tag>;
                },
            },
            {
                title: "基础价",
                key: "baseCents",
                width: 100,
                align: "right",
                render: (_, record) => {
                    const cents = record.unit === "per_second" ? record.basePerSec ?? 0 : record.baseCents;
                    if (cents <= 0) return <Text type="secondary">未定价</Text>;
                    return (
                        <Text type="secondary">
                            ¥{(cents / 100).toFixed(2)}{unitLabels[record.unit] ?? ""}
                        </Text>
                    );
                },
            },
        ];
        // 每个 group 一列
        const groupCols: ColumnsType<PricingModel> = groups.map((g) => ({
            title: (
                <Space direction="vertical" size={0} align="center">
                    <Space size={4}>
                        <Text strong>{g.displayName}</Text>
                        {currentUserGroupId === g.id && <Tag color="blue">当前</Tag>}
                    </Space>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                        {(g.ratio * 100).toFixed(0)}%
                    </Text>
                </Space>
            ),
            key: g.id,
            align: "right" as const,
            width: 130,
            render: (_, record) => {
                const cell = record.groups.find((c) => c.group === g.id);
                if (!cell || cell.unitCents <= 0) {
                    return <Text type="secondary">-</Text>;
                }
                const isCurrent = currentUserGroupId === g.id;
                return (
                    <Space direction="vertical" size={0} align="end">
                        <Text strong style={{ color: isCurrent ? "#1677ff" : undefined }}>
                            ¥{(cell.unitCents / 100).toFixed(2)}
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {unitLabels[record.unit] ?? ""}
                            </Text>
                        </Text>
                        {cell.discount && <Tag color="green" style={{ marginRight: 0 }}>{cell.discount}</Tag>}
                    </Space>
                );
            },
        }));
        return [...base, ...groupCols];
    }, [groups, currentUserGroupId]);

    return (
        <div>
            <Space style={{ marginBottom: 12 }}>
                <DollarOutlined />
                <Text strong>模型价目表</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>
                    公开接口（无需登录）；group 倍率由管理员在系统设置里配置
                </Text>
                <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    onClick={() => void load()}
                    loading={loading}
                >
                    刷新
                </Button>
            </Space>

            {currentUserGroupId && (
                <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 12 }}
                    message={
                        <span>
                            当前用户组：<Tag color="blue">{currentUserGroupId}</Tag>
                            {groups.find((g) => g.id === currentUserGroupId) && (
                                <Text type="secondary">
                                    （显示名：{groups.find((g) => g.id === currentUserGroupId)?.displayName}）
                                </Text>
                            )}
                        </span>
                    }
                />
            )}

            {loading && !data ? (
                <Skeleton active />
            ) : models.length === 0 ? (
                <Empty description="暂无可显示的模型" />
            ) : (
                <Table<PricingModel>
                    rowKey="model"
                    dataSource={models}
                    columns={columns}
                    pagination={false}
                    size="middle"
                    scroll={{ x: "max-content" }}
                />
            )}
        </div>
    );
}
