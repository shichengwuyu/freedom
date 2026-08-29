"use client";

import {
    ApiOutlined,
    ClearOutlined,
    ExclamationCircleOutlined,
    ReloadOutlined,
} from "@ant-design/icons";
import {
    App,
    Alert,
    Button,
    Card,
    Col,
    Empty,
    Popconfirm,
    Row,
    Space,
    Statistic,
    Table,
    Tag,
    Tooltip,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import { useCallback, useEffect, useState } from "react";

import {
    clearChannelCooldowns,
    fetchChannelsHealth,
    type AdminChannelFailLogEntry,
    type AdminChannelHealthItem,
    type AdminChannelsHealth,
} from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

const { Text } = Typography;

// 状态码 Tag 颜色（与 Sprint 2 默认 cooldown 触发规则保持一致）
const statusCodeColor = (code: number) => {
    if (code === 0) return "default"; // 网络错
    if (code === 429) return "orange"; // 限流
    if (code >= 500 && code <= 599) return "red"; // 服务端错
    if (code >= 400 && code <= 499) return "volcano"; // 客户端错（4xx）
    if (code >= 200 && code <= 299) return "green"; // 成功（理论上不会进 fail log，但兜底）
    return "default";
};

const formatCooldown = (seconds: number) => {
    if (seconds <= 0) return "未冷却";
    if (seconds < 60) return `${seconds}s`;
    return `${Math.ceil(seconds / 60)}m`;
};

export default function AdminChannelsHealthPage() {
    const token = useUserStore((s) => s.token);
    const { message } = App.useApp();
    const [data, setData] = useState<AdminChannelsHealth | null>(null);
    const [loading, setLoading] = useState(false);
    const [clearing, setClearing] = useState(false);

    const load = useCallback(async () => {
        if (!token) return;
        setLoading(true);
        try {
            const res = await fetchChannelsHealth(token);
            setData(res);
        } catch (err) {
            const msg = err instanceof Error ? err.message : "加载失败";
            message.error(msg);
        } finally {
            setLoading(false);
        }
    }, [token, message]);

    useEffect(() => {
        void load();
    }, [load]);

    const handleClear = async () => {
        setClearing(true);
        try {
            const res = await clearChannelCooldowns(token);
            const cleared = res.data.cleared;
            if (cleared === 0) {
                message.info("当前无冷却中的渠道");
            } else {
                message.success(`已清空 ${cleared} 个渠道的冷却`);
            }
            await load();
        } catch (err) {
            const msg = err instanceof Error ? err.message : "清空失败";
            message.error(msg);
        } finally {
            setClearing(false);
        }
    };

    // 渠道统计表列
    const channelColumns: ColumnsType<AdminChannelHealthItem> = [
        {
            title: "渠道名",
            dataIndex: "channelName",
            key: "channelName",
            width: 200,
            render: (name: string, record) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{name}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                        {record.channelId}
                    </Text>
                </Space>
            ),
        },
        {
            title: "失败次数",
            dataIndex: "failureCount",
            key: "failureCount",
            width: 100,
            sorter: (a, b) => a.failureCount - b.failureCount,
            defaultSortOrder: "descend",
            render: (n: number) => (
                <Text strong style={{ color: n > 0 ? "#cf1322" : undefined }}>
                    {n}
                </Text>
            ),
        },
        {
            title: "最后失败",
            dataIndex: "lastFailureAt",
            key: "lastFailureAt",
            width: 170,
            render: (t: string) => (t ? dayjs(t).format("YYYY-MM-DD HH:mm:ss") : "-"),
        },
        {
            title: "状态码",
            dataIndex: "lastStatusCode",
            key: "lastStatusCode",
            width: 100,
            render: (code: number) =>
                code === 0 ? (
                    <Tag>网络错</Tag>
                ) : (
                    <Tag color={statusCodeColor(code)}>{code}</Tag>
                ),
        },
        {
            title: "冷却状态",
            key: "cooldown",
            width: 130,
            render: (_, record) =>
                record.isInCooldown ? (
                    <Tag color="orange">{formatCooldown(record.cooldownRemaining)}</Tag>
                ) : (
                    <Tag color="green">正常</Tag>
                ),
        },
        {
            title: "影响模型",
            dataIndex: "affectedModels",
            key: "affectedModels",
            render: (models: string[]) => {
                if (!models || models.length === 0) return <Text type="secondary">-</Text>;
                const preview = models.slice(0, 3).join(", ");
                const more = models.length > 3 ? ` 等 ${models.length} 个` : "";
                return (
                    <Tooltip title={models.join(", ")}>
                        <Text style={{ fontSize: 12 }}>{preview + more}</Text>
                    </Tooltip>
                );
            },
        },
    ];

    // 最近失败表列
    const failureColumns: ColumnsType<AdminChannelFailLogEntry> = [
        {
            title: "时间",
            dataIndex: "at",
            key: "at",
            width: 170,
            render: (t: string) => dayjs(t).format("YYYY-MM-DD HH:mm:ss"),
        },
        {
            title: "渠道",
            dataIndex: "channelName",
            key: "channelName",
            width: 160,
            render: (name: string, record) => (
                <Space direction="vertical" size={0}>
                    <Text>{name}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                        {record.channelId}
                    </Text>
                </Space>
            ),
        },
        {
            title: "模型",
            dataIndex: "model",
            key: "model",
            width: 160,
            render: (m: string) => (m ? <Text code>{m}</Text> : <Text type="secondary">-</Text>),
        },
        {
            title: "能力",
            dataIndex: "capability",
            key: "capability",
            width: 80,
            render: (c: string) => (c ? <Tag>{c}</Tag> : <Text type="secondary">-</Text>),
        },
        {
            title: "Key #",
            dataIndex: "keyIndex",
            key: "keyIndex",
            width: 70,
            align: "right",
            render: (n: number) => (typeof n === "number" ? n : "-"),
        },
        {
            title: "状态码",
            dataIndex: "statusCode",
            key: "statusCode",
            width: 90,
            render: (code: number) =>
                code === 0 ? (
                    <Tag>网络错</Tag>
                ) : (
                    <Tag color={statusCodeColor(code)}>{code}</Tag>
                ),
        },
        {
            title: "错误",
            dataIndex: "errorMessage",
            key: "errorMessage",
            render: (msg: string) =>
                msg ? (
                    <Tooltip title={msg}>
                        <Text
                            style={{ fontSize: 12 }}
                            ellipsis={{ tooltip: false }}
                        >
                            {msg}
                        </Text>
                    </Tooltip>
                ) : (
                    <Text type="secondary">-</Text>
                ),
        },
    ];

    const summary = data?.summary;
    const channels = data?.channels ?? [];
    const recent = data?.recentFailures ?? [];

    return (
        <div style={{ padding: 24 }}>
            <Card
                title={
                    <Space>
                        <ApiOutlined />
                        <span>渠道健康度</span>
                    </Space>
                }
                extra={
                    <Space>
                        <Button
                            icon={<ReloadOutlined />}
                            onClick={() => void load()}
                            loading={loading}
                        >
                            刷新
                        </Button>
                        <Popconfirm
                            title="确定清空所有渠道的冷却状态？"
                            description="清空后所有渠道会立即被恢复，可能导致上游再次过载"
                            okText="确定清空"
                            cancelText="取消"
                            okButtonProps={{ danger: true }}
                            onConfirm={() => void handleClear()}
                        >
                            <Button danger icon={<ClearOutlined />} loading={clearing}>
                                清空冷却
                            </Button>
                        </Popconfirm>
                    </Space>
                }
            >
                <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                    message="监控最近 1000 条失败记录（内存 ring buffer，不持久化）；进程重启后清空。"
                />

                {/* 4 个 KPI 卡片 */}
                <Row gutter={16} style={{ marginBottom: 24 }}>
                    <Col span={6}>
                        <Card>
                            <Statistic
                                title="失败总数（最近 1000 条）"
                                value={summary?.totalFailures ?? 0}
                                valueStyle={{
                                    color: (summary?.totalFailures ?? 0) > 0 ? "#cf1322" : undefined,
                                }}
                                prefix={
                                    (summary?.totalFailures ?? 0) > 0 ? (
                                        <ExclamationCircleOutlined />
                                    ) : undefined
                                }
                            />
                        </Card>
                    </Col>
                    <Col span={6}>
                        <Card>
                            <Statistic
                                title="失败渠道数"
                                value={summary?.uniqueChannels ?? 0}
                                suffix={`/ 总 ${channels.length} 有失败`}
                            />
                        </Card>
                    </Col>
                    <Col span={6}>
                        <Card>
                            <Statistic
                                title="失败模型数"
                                value={summary?.uniqueModels ?? 0}
                            />
                        </Card>
                    </Col>
                    <Col span={6}>
                        <Card>
                            <Statistic
                                title="最长冷却剩余"
                                value={summary?.longestCooldownRemaining ?? 0}
                                suffix="秒"
                                valueStyle={{
                                    color:
                                        (summary?.longestCooldownRemaining ?? 0) > 0
                                            ? "#d48806"
                                            : undefined,
                                }}
                            />
                        </Card>
                    </Col>
                </Row>

                {/* 渠道失败统计表 */}
                <div style={{ marginBottom: 8 }}>
                    <Text strong>渠道失败统计（按失败次数倒序）</Text>
                </div>
                <Table<AdminChannelHealthItem>
                    rowKey="channelId"
                    dataSource={channels}
                    columns={channelColumns}
                    pagination={false}
                    size="middle"
                    loading={loading && channels.length === 0}
                    locale={{
                        emptyText: <Empty description="暂无失败记录" />,
                    }}
                    style={{ marginBottom: 24 }}
                />

                {/* 最近失败列表 */}
                <div style={{ marginBottom: 8 }}>
                    <Text strong>最近失败（最多 100 条）</Text>
                </div>
                <Table<AdminChannelFailLogEntry>
                    rowKey={(record) => `${record.at}-${record.channelId}-${record.keyIndex}`}
                    dataSource={recent}
                    columns={failureColumns}
                    pagination={{ pageSize: 20, showSizeChanger: false }}
                    size="small"
                    loading={loading && recent.length === 0}
                    locale={{
                        emptyText: <Empty description="暂无失败记录" />,
                    }}
                />
            </Card>
        </div>
    );
}
