"use client";

import {
    App,
    Button,
    Checkbox,
    Collapse,
    DatePicker,
    Form,
    Input,
    InputNumber,
    Modal,
    Space,
} from "antd";
import dayjs, { type Dayjs } from "dayjs";
import { useState } from "react";
import {
    createUserToken,
    type UserTokenCreateRequest,
    type UserTokenCreateResponse,
} from "@/services/api/user_token";

const { RangePicker } = DatePicker;

type ApiKeyCreateModalProps = {
    open: boolean;
    token: string;
    onClose: () => void;
    onCreated: (res: UserTokenCreateResponse) => void;
};

type FormValues = {
    name: string;
    expiredAt?: Dayjs;
    balanceCapYuan?: number;
    unlimitedBalance?: boolean;
};

export function ApiKeyCreateModal({ open, token, onClose, onCreated }: ApiKeyCreateModalProps) {
    const { message } = App.useApp();
    const [form] = Form.useForm<FormValues>();
    const [submitting, setSubmitting] = useState(false);

    const handleSubmit = async () => {
        try {
            const values = await form.validateFields();
            setSubmitting(true);

            const req: UserTokenCreateRequest = {
                name: values.name.trim(),
            };
            if (values.expiredAt) {
                req.expiredAt = values.expiredAt.toISOString();
            }
            if (values.unlimitedBalance) {
                req.unlimitedBalance = true;
            } else if (values.balanceCapYuan && values.balanceCapYuan > 0) {
                // 元 → 分
                req.balanceCapCents = Math.round(values.balanceCapYuan * 100);
            }

            const res = await createUserToken(token, req);
            message.success("已创建，请立即保存完整 Key");
            form.resetFields();
            onClose();
            onCreated(res);
        } catch (err) {
            // 校验失败 / 接口报错都走这里；axios 错误已由 response 拦截器处理
            const msg = err instanceof Error ? err.message : "创建失败";
            message.error(msg);
        } finally {
            setSubmitting(false);
        }
    };

    const handleCancel = () => {
        form.resetFields();
        onClose();
    };

    return (
        <Modal
            open={open}
            title="创建 API Key"
            onCancel={handleCancel}
            onOk={handleSubmit}
            confirmLoading={submitting}
            okText="创建"
            cancelText="取消"
            width={520}
            destroyOnClose
        >
            <Form
                form={form}
                layout="vertical"
                initialValues={{ unlimitedBalance: false, balanceCapYuan: 0 }}
            >
                <Form.Item
                    name="name"
                    label="名称"
                    rules={[
                        { required: true, message: "请填写名称" },
                        { max: 50, message: "名称最长 50 字符" },
                    ]}
                >
                    <Input placeholder="例如：my-cursor / curl-test" maxLength={50} showCount />
                </Form.Item>

                <Collapse
                    ghost
                    items={[
                        {
                            key: "advanced",
                            label: "高级选项（可选）",
                            children: (
                                <Space direction="vertical" style={{ width: "100%" }} size="middle">
                                    <Form.Item
                                        name="expiredAt"
                                        label="过期时间"
                                        extra="留空表示永不过期"
                                    >
                                        <DatePicker
                                            showTime
                                            style={{ width: "100%" }}
                                            disabledDate={(current) => current && current < dayjs().startOf("day")}
                                            placeholder="选择过期时间"
                                        />
                                    </Form.Item>

                                    <Form.Item
                                        name="unlimitedBalance"
                                        label="独立额度"
                                        valuePropName="checked"
                                    >
                                        <Checkbox>不限制额度（无上限）</Checkbox>
                                    </Form.Item>

                                    <Form.Item
                                        name="balanceCapYuan"
                                        label="或：独立额度上限（元）"
                                        extra="0 = 不限制（用账户余额）；勾选上面「不限制」后本字段被忽略"
                                        dependencies={["unlimitedBalance"]}
                                    >
                                        <InputNumber
                                            min={0}
                                            step={1}
                                            precision={2}
                                            style={{ width: "100%" }}
                                            placeholder="例如：10 表示 1 元"
                                            addonAfter="元"
                                        />
                                    </Form.Item>
                                </Space>
                            ),
                        },
                    ]}
                />
            </Form>
        </Modal>
    );
}
