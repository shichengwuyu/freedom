"use client";

import {
    App,
    Alert,
    Button,
    Empty,
    Popconfirm,
    Space,
    Table,
    Tag,
    Tooltip,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
    DeleteOutlined,
    KeyOutlined,
    PauseCircleOutlined,
    PlayCircleOutlined,
    PlusOutlined,
    ReloadOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import { useCallback, useEffect, useState } from "react";
import {
    deleteUserToken,
    disableUserToken,
    enableUserToken,
    listUserTokens,
    type UserToken,
    type UserTokenCreateResponse,
    type UserTokenStatus,
} from "@/services/api/user_token";
import { useCopyText } from "@/hooks/use-copy-text";
import { ApiKeyCreateModal } from "./api-key-create-modal";
import { ApiKeyRevealModal } from "./api-key-reveal-modal";

const { Text } = Typography;

const statusColorMap: Record<UserTokenStatus, string> = {
    active: "green",
    disabled: "default",
    exhausted: "orange",
    expired: "red",
};

const statusLabelMap: Record<UserTokenStatus, string> = {
    active: "启用",
    disabled: "已禁用",
    exhausted: "已耗尽",
    expired: "已过期",
};

function formatCents(cents: number) {
    return `¥${(cents / 100).toFixed(2)}`;
}

type ApiKeyManagerProps = {
    token: string;
};

export function ApiKeyManager({ token }: ApiKeyManagerProps) {
    const { message } = App.useApp();
    const copyText = useCopyText();

    const [items, setItems] = useState<UserToken[]>([]);
    const [loading, setLoading] = useState(false);
    const [createOpen, setCreateOpen] = useState(false);
    const [reveal, setReveal] = useState<{ open: boolean; raw: string }>({
        open: false,
        raw: "",
    });
    const [acting, setActing] = useState<string | null>(null); // 当前正在操作的 token id

    const reload = useCallback(async () => {
        setLoading(true);
        try {
            const res = await listUserTokens(token);
            setItems(res.items || []);
        } catch (err) {
            const msg = err instanceof Error ? err.message : "加载失败";
            message.error(msg);
        } finally {
            setLoading(false);
        }
    }, [token, message]);

    useEffect(() => {
        void reload();
    }, [reload]);

    const handleCreated = (res: UserTokenCreateResponse) => {
        setReveal({ open: true, raw: res.raw });
        void reload();
    };

    const handleDelete = async (id: string, name: string) => {
        setActing(id);
        try {
            await deleteUserToken(token, id);
            message.success(`已删除「${name}」`);
            void reload();
        } catch (err) {
            const msg = err instanceof Error ? err.message : "删除失败";
            message.error(msg);
        } finally {
            setActing(null);
        }
    };

    const handleToggle = async (item: UserToken) => {
        setActing(item.id);
        try {
            if (item.status === "active") {
                await disableUserToken(token, item.id);
                message.success(`已禁用「${item.name}」`);
            } else {
                await enableUserToken(token, item.id);
                message.success(`已启用「${item.name}」`);
            }
            void reload();
        } catch (err) {
            const msg = err instanceof Error ? err.message : "操作失败";
            message.error(msg);
        } finally {
            setActing(null);
        }
    };

    const columns: ColumnsType<UserToken> = [
        {
            title: "名称",
            dataIndex: "name",
            key: "name",
            width: 160,
            render: (name: string) => <Text strong>{name}</Text>,
        },
        {
            title: "Key",
            dataIndex: "keyPrefix",
            key: "keyPrefix",
            width: 200,
            render: (prefix: string) => (
                <Tooltip title="点击复制完整 Key Prefix（仅用于识别，列表永远不展示完整 key）">
                    <Text
                        code
                        style={{ cursor: "pointer" }}
                        onClick={() => copyText(prefix, "已复制 Key Prefix")}
                    >
                        {prefix}
                    </Text>
                </Tooltip>
            ),
        },
        {
            title: "状态",
            dataIndex: "status",
            key: "status",
            width: 90,
            render: (status: UserTokenStatus) => (
                <Tag color={statusColorMap[status]}>{statusLabelMap[status]}</Tag>
            ),
        },
        {
            title: "额度用量",
            key: "quota",
            width: 160,
            render: (_, record) => {
                if (record.unlimitedBalance) {
                    return <Text>{formatCents(record.usedCents)} / 无限</Text>;
                }
                if (record.balanceCapCents > 0) {
                    return (
                        <Text>
                            {formatCents(record.usedCents)} / {formatCents(record.balanceCapCents)}
                        </Text>
                    );
                }
                return <Text type="secondary">{formatCents(record.usedCents)}（用账户余额）</Text>;
            },
        },
        {
            title: "最后使用",
            key: "lastUsed",
            width: 200,
            render: (_, record) => {
                if (!record.lastUsedAt) {
                    return <Text type="secondary">从未使用</Text>;
                }
                return (
                    <Space direction="vertical" size={0}>
                        <Text>{dayjs(record.lastUsedAt).format("YYYY-MM-DD HH:mm")}</Text>
                        {record.lastUsedIp && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {record.lastUsedIp}
                            </Text>
                        )}
                    </Space>
                );
            },
        },
        {
            title: "过期",
            dataIndex: "expiredAt",
            key: "expiredAt",
            width: 130,
            render: (t: string | null) => {
                if (!t) return <Text type="secondary">永不过期</Text>;
                return dayjs(t).format("YYYY-MM-DD");
            },
        },
        {
            title: "操作",
            key: "actions",
            width: 200,
            render: (_, record) => {
                const isActive = record.status === "active";
                const canEnable = !isActive && record.status !== "expired";
                return (
                    <Space>
                        <Tooltip
                            title={
                                record.status === "expired"
                                    ? "已过期，无法启用"
                                    : isActive
                                    ? "禁用后该 Key 立即失效"
                                    : "恢复使用"
                            }
                        >
                            <Button
                                type="link"
                                size="small"
                                icon={
                                    isActive ? <PauseCircleOutlined /> : <PlayCircleOutlined />
                                }
                                disabled={acting === record.id || !canEnable}
                                onClick={() => void handleToggle(record)}
                            >
                                {isActive ? "禁用" : "启用"}
                            </Button>
                        </Tooltip>
                        <Popconfirm
                            title={`删除 Key「${record.name}」？`}
                            description="删除后使用该 Key 的请求将立即 401，无法恢复。"
                            okText="删除"
                            cancelText="取消"
                            okButtonProps={{ danger: true }}
                            onConfirm={() => void handleDelete(record.id, record.name)}
                        >
                            <Button
                                type="link"
                                danger
                                size="small"
                                icon={<DeleteOutlined />}
                                disabled={acting === record.id}
                            >
                                删除
                            </Button>
                        </Popconfirm>
                    </Space>
                );
            },
        },
    ];

    return (
        <div>
            <Alert
                type="info"
                showIcon
                style={{ marginBottom: 12 }}
                message={
                    <span>
                        <KeyOutlined /> API Key 用于 OpenAI SDK / Cursor / Cline / curl 等外部客户端
                        直接对接 Freedom，调用 <Text code>/v1/chat/completions</Text>{" "}
                        <Text code>/v1/images/generations</Text> 等端点。
                    </span>
                }
            />

            <div style={{ marginBottom: 12, display: "flex", justifyContent: "space-between" }}>
                <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => setCreateOpen(true)}
                >
                    创建 API Key
                </Button>
                <Button icon={<ReloadOutlined />} onClick={() => void reload()} loading={loading}>
                    刷新
                </Button>
            </div>

            <Table<UserToken>
                rowKey="id"
                dataSource={items}
                columns={columns}
                loading={loading}
                pagination={false}
                size="middle"
                locale={{
                    emptyText: (
                        <Empty
                            image={Empty.PRESENTED_IMAGE_SIMPLE}
                            description="还没有 API Key，点击右上角「创建 API Key」开始"
                        />
                    ),
                }}
            />

            <ApiKeyCreateModal
                open={createOpen}
                token={token}
                onClose={() => setCreateOpen(false)}
                onCreated={handleCreated}
            />
            <ApiKeyRevealModal
                open={reveal.open}
                raw={reveal.raw}
                onClose={() => setReveal({ open: false, raw: "" })}
            />
        </div>
    );
}
