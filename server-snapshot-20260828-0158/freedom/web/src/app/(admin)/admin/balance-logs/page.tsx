"use client";

import { PlusOutlined, ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import { ProTable, type ProColumns } from "@ant-design/pro-components";
import { Button, Card, Col, Form, Input, InputNumber, Modal, Row, Space, Tag, Tooltip, Typography } from "antd";
import dayjs from "dayjs";
import { useEffect, useState } from "react";

import type { AdminBalanceLog } from "@/services/api/admin";
import { useAdminBalanceLogs } from "./use-admin-balance-logs";

type BalanceLogFormValues = Partial<AdminBalanceLog>;

const balanceLogTypeLabels: Record<string, string> = {
    manual_adjust: "后台调整",
    generation_consume: "模型消费",
    generation_refund: "失败返还",
    manual_recharge: "卡密充值",
};

export default function AdminBalanceLogsPage() {
    const { logs, keyword, page, pageSize, total, isLoading, searchLogs, changePage, changePageSize, resetFilters, refreshLogs, saveLog: saveAdminLog } = useAdminBalanceLogs();
    const [form] = Form.useForm<BalanceLogFormValues>();
    const [keywordText, setKeywordText] = useState(keyword);
    const [editingLog, setEditingLog] = useState<{ userId: string; balance: number } | null>(null);

    useEffect(() => setKeywordText(keyword), [keyword]);

    const saveLog = async () => {
        const value = await form.validateFields();
        await saveAdminLog({ userId: value.userId ?? "", balance: Number(value.balance) || 0 });
        setEditingLog(null);
    };

    const columns: ProColumns<AdminBalanceLog>[] = [
        {
            title: "用户",
            dataIndex: "userId",
            width: 240,
            render: (_, item) => (
                <Space direction="vertical" size={0}>
                    <Typography.Text copyable>{item.userId}</Typography.Text>
                    {item.userDisplayName ? (
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            {item.userDisplayName}
                        </Typography.Text>
                    ) : null}
                </Space>
            ),
        },
        {
            title: "类型",
            dataIndex: "type",
            width: 140,
            render: (_, item) => <Tag>{balanceLogTypeLabels[item.type] || item.type || "-"}</Tag>,
        },
        {
            title: "变动金额",
            dataIndex: "amount",
            width: 120,
            render: (_, item) => {
                const cents = Number(item.amount) || 0;
                const yuan = cents / 100;
                return (
                    <Typography.Text type={cents >= 0 ? "success" : "danger"}>
                        {cents >= 0 ? "+" : ""}¥{yuan.toFixed(2)}
                    </Typography.Text>
                );
            },
        },
        {
            title: "变动后余额",
            dataIndex: "balance",
            width: 140,
            render: (_, item) => {
                const yuan = (Number(item.balance) || 0) / 100;
                return `¥${yuan.toFixed(2)}`;
            },
        },
        {
            title: "备注",
            dataIndex: "remark",
            ellipsis: true,
            render: (_, item) => <Typography.Text type="secondary">{item.remark || "-"}</Typography.Text>,
        },
        {
            title: "创建时间",
            dataIndex: "createdAt",
            width: 180,
            render: (_, item) => <Typography.Text type="secondary">{item.createdAt ? dayjs(item.createdAt).format("YYYY-MM-DD HH:mm:ss") : "-"}</Typography.Text>,
        },
        {
            title: "操作",
            key: "actions",
            width: 96,
            align: "right",
            render: () => <Typography.Text type="secondary">—</Typography.Text>,
        },
    ];

    return (
        <main style={{ padding: 24 }}>
            <Space direction="vertical" size={16} style={{ width: "100%" }}>
                <Card variant="borderless">
                    <Form layout="vertical">
                        <Row gutter={16} align="bottom">
                            <Col flex="360px">
                                <Form.Item label="关键词">
                                    <Input.Search value={keywordText} placeholder="搜索用户 ID、类型、备注或关联 ID" allowClear enterButton={<SearchOutlined />} onSearch={() => searchLogs(keywordText)} onChange={(event) => setKeywordText(event.target.value)} />
                                </Form.Item>
                            </Col>
                            <Col flex="none">
                                <Form.Item>
                                    <Space>
                                        <Button
                                            onClick={() => {
                                                setKeywordText("");
                                                resetFilters();
                                            }}
                                        >
                                            重置
                                        </Button>
                                        <Button type="primary" icon={<ReloadOutlined />} onClick={() => searchLogs(keywordText)}>
                                            查询
                                        </Button>
                                    </Space>
                                </Form.Item>
                            </Col>
                        </Row>
                    </Form>
                </Card>
                <ProTable<AdminBalanceLog>
                    rowKey="id"
                    columns={columns}
                    dataSource={logs}
                    loading={isLoading}
                    search={false}
                    defaultSize="middle"
                    tableLayout="fixed"
                    cardProps={{ variant: "borderless" }}
                    headerTitle={
                        <Space>
                            <Typography.Text strong>余额日志</Typography.Text>
                            <Tag>{total} 条</Tag>
                        </Space>
                    }
                    options={{ density: true, setting: true, reload: () => void refreshLogs() }}
                    toolBarRender={() => [
                        <Button key="add" type="primary" icon={<PlusOutlined />} onClick={() => setEditingLog({ userId: "", balance: 0 })}>
                            调整余额
                        </Button>,
                    ]}
                    pagination={{
                        current: page,
                        pageSize,
                        total,
                        showSizeChanger: true,
                        pageSizeOptions: [10, 20, 50, 100],
                        showTotal: (value) => `共 ${value} 条`,
                        onChange: (nextPage, nextPageSize) => (nextPageSize !== pageSize ? changePageSize(nextPageSize) : changePage(nextPage)),
                    }}
                />
            </Space>

            <Modal title="调整用户余额" open={Boolean(editingLog)} width={520} onCancel={() => setEditingLog(null)} onOk={() => void saveLog()} okText="保存" cancelText="取消" destroyOnHidden>
                <Form form={form} layout="vertical" requiredMark={false}>
                    <Row gutter={14}>
                        <Col span={24}>
                            <Form.Item name="userId" label="用户 ID" rules={[{ required: true, message: "请输入用户 ID" }]}>
                                <Input placeholder="要调整余额的用户 ID" />
                            </Form.Item>
                        </Col>
                        <Col span={24}>
                            <Form.Item name="balance" label="调整后余额（分）" rules={[{ required: true, message: "请输入调整后的余额" }]}>
                                <InputNumber min={0} precision={0} style={{ width: "100%" }} placeholder="例如 10000 表示 ¥100.00" />
                            </Form.Item>
                        </Col>
                    </Row>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        余额流水为对账记录，调整会写入一条 manual_adjust 流水并同步更新用户真实余额，不可手动编辑或删除历史流水。
                    </Typography.Text>
                </Form>
            </Modal>
        </main>
    );
}
