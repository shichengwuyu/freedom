"use client";

import {
    App,
    Button,
    Form,
    Input,
    Modal,
    Popconfirm,
    Space,
    Table,
    Tooltip,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
    DeleteOutlined,
    EditOutlined,
    PlusOutlined,
    ReloadOutlined,
    SearchOutlined,
    SoundOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import {
    adminDeleteAnnouncement,
    adminListAnnouncements,
    adminSaveAnnouncement,
    type AnnouncementItem,
} from "@/services/api/announcement";
import { useUserStore } from "@/stores/use-user-store";

export default function AdminAnnouncementsPage() {
    return <AdminAnnouncementsContent />;
}

function AdminAnnouncementsContent() {
    const { message, modal } = App.useApp();
    const token = useUserStore((state) => state.token);
    const queryClient = useQueryClient();

    const [keyword, setKeyword] = useState("");
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [total, setTotal] = useState(0);
    const [items, setItems] = useState<AnnouncementItem[]>([]);
    const [loading, setLoading] = useState(false);

    const [editOpen, setEditOpen] = useState(false);
    const [editLoading, setEditLoading] = useState(false);
    const [editing, setEditing] = useState<AnnouncementItem | null>(null);
    const [editForm] = Form.useForm<{ content: string }>();

    const loadList = async () => {
        if (!token) return;
        try {
            setLoading(true);
            const res = await adminListAnnouncements(token, { keyword, page, pageSize });
            setItems(res.items ?? []);
            setTotal(Number(res.total ?? 0));
        } catch (err) {
            message.error(err instanceof Error ? err.message : "加载公告列表失败");
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        void loadList();
    }, [token, keyword, page, pageSize]);

    const openCreate = () => {
        setEditing(null);
        editForm.setFieldsValue({ content: "" });
        setEditOpen(true);
    };

    const openEdit = (row: AnnouncementItem) => {
        setEditing(row);
        editForm.setFieldsValue({ content: row.content });
        setEditOpen(true);
    };

    const handleSave = async () => {
        if (!token) return;
        const values = await editForm.validateFields();
        try {
            setEditLoading(true);
            await adminSaveAnnouncement(token, {
                id: editing?.id,
                content: values.content,
            });
            message.success(editing?.id ? "公告已更新" : "公告已新增");
            setEditOpen(false);
            // 使公告缓存失效（若未来使用 React Query）
            await queryClient.invalidateQueries({ queryKey: ["announcements"] });
            void loadList();
        } catch (err) {
            modal.error({
                title: "保存失败",
                content: err instanceof Error ? err.message : "请重试",
            });
        } finally {
            setEditLoading(false);
        }
    };

    const handleDelete = async (id: string) => {
        if (!token) return;
        try {
            await adminDeleteAnnouncement(token, id);
            message.success("公告已删除");
            void loadList();
        } catch (err) {
            message.error(err instanceof Error ? err.message : "删除失败");
        }
    };

    const columns: ColumnsType<AnnouncementItem> = [
        {
            title: "序号",
            key: "index",
            width: 72,
            align: "center",
            render: (_v, _r, idx) => (page - 1) * pageSize + idx + 1,
        },
        {
            title: "公告内容",
            dataIndex: "content",
            ellipsis: true,
            render: (text: string) => (
                <Tooltip title={text} placement="topLeft">
                    <Typography.Paragraph
                        style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-word" }}
                        ellipsis={{ rows: 2 }}
                    >
                        {text}
                    </Typography.Paragraph>
                </Tooltip>
            ),
        },
        {
            title: "创建时间",
            dataIndex: "createdAt",
            width: 180,
            render: (v: string) => (v ? dayjs(v).format("YYYY-MM-DD HH:mm:ss") : "-"),
            sorter: (a, b) => dayjs(a.createdAt).valueOf() - dayjs(b.createdAt).valueOf(),
            defaultSortOrder: "descend",
        },
        {
            title: "更新时间",
            dataIndex: "updatedAt",
            width: 180,
            render: (v: string) => (v ? dayjs(v).format("YYYY-MM-DD HH:mm:ss") : "-"),
        },
        {
            title: "操作",
            key: "actions",
            width: 160,
            fixed: "right",
            render: (_v, row) => (
                <Space size={4}>
                    <Button
                        type="link"
                        size="small"
                        icon={<EditOutlined />}
                        onClick={() => openEdit(row)}
                    >
                        编辑
                    </Button>
                    <Popconfirm
                        title="确认删除这条公告？"
                        description="删除后不可恢复"
                        okText="删除"
                        okButtonProps={{ danger: true }}
                        cancelText="取消"
                        onConfirm={() => handleDelete(row.id)}
                    >
                        <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                            删除
                        </Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    return (
        <div style={{ padding: 24 }}>
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    marginBottom: 16,
                    flexWrap: "wrap",
                    gap: 12,
                }}
            >
                <Space.Compact style={{ width: 360 }}>
                    <Input
                        allowClear
                        prefix={<SearchOutlined />}
                        placeholder="搜索公告内容关键词"
                        value={keyword}
                        onChange={(e) => {
                            setKeyword(e.target.value);
                            setPage(1);
                        }}
                        onPressEnter={() => setPage(1)}
                    />
                </Space.Compact>
                <Space>
                    <Button
                        icon={<ReloadOutlined />}
                        onClick={() => void loadList()}
                        disabled={loading}
                    >
                        刷新
                    </Button>
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                        新增公告
                    </Button>
                </Space>
            </div>

            <Table<AnnouncementItem>
                rowKey="id"
                loading={loading}
                columns={columns}
                dataSource={items}
                pagination={{
                    current: page,
                    pageSize,
                    total,
                    showSizeChanger: true,
                    showQuickJumper: true,
                    pageSizeOptions: ["10", "20", "50", "100"],
                    showTotal: (t, range) => `第 ${range[0]}-${range[1]} 条 / 共 ${t} 条`,
                    onChange: (p, ps) => {
                        setPage(p);
                        setPageSize(ps);
                    },
                }}
                scroll={{ x: 900 }}
            />

            <Modal
                title={
                    <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <SoundOutlined />
                        {editing?.id ? "编辑公告" : "新增公告"}
                    </span>
                }
                open={editOpen}
                onCancel={() => setEditOpen(false)}
                confirmLoading={editLoading}
                onOk={handleSave}
                okText="保存"
                cancelText="取消"
                destroyOnHidden
                width={640}
            >
                <Form
                    form={editForm}
                    layout="vertical"
                    style={{ marginTop: 8 }}
                    preserve={false}
                >
                    <Form.Item
                        label="公告内容"
                        name="content"
                        rules={[
                            { required: true, message: "请输入公告内容" },
                            { max: 2000, message: "内容不能超过 2000 字" },
                        ]}
                    >
                        <Input.TextArea
                            rows={8}
                            showCount
                            maxLength={2000}
                            placeholder="请输入公告正文，支持多行文本。保存后自动记录创建/更新时间，首页将按时间倒序显示最新 10 条。"
                        />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
