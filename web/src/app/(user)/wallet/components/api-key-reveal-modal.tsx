"use client";

import { Checkbox, Modal, Typography, message } from "antd";
import { useState } from "react";
import { useCopyText } from "@/hooks/use-copy-text";

const { Paragraph, Text } = Typography;

type ApiKeyRevealModalProps = {
    open: boolean;
    raw: string;
    onClose: () => void;
};

// 创建 token 后的"明文一次展示"弹窗。
// 设计要点：
//   1) 只读 input 显示 sk-fk-... 明文
//   2) 大号「复制 Key」按钮（带 toast）
//   3) 「我已保存」checkbox 未勾选时关闭按钮 disabled，防止误关
//   4) 给出最小可用 curl 示例
export function ApiKeyRevealModal({ open, raw, onClose }: ApiKeyRevealModalProps) {
    const [saved, setSaved] = useState(false);
    const copyText = useCopyText();

    // 关闭时清空 checkbox 状态（下次再开时是新的）
    const handleClose = () => {
        if (!saved) return;
        setSaved(false);
        onClose();
    };

    const handleCopy = () => {
        copyText(raw, "已复制 Key，请妥善保存");
    };

    return (
        <Modal
            open={open}
            title={
                <span style={{ color: "#d4380d" }}>
                    ⚠️ 请立即保存你的 API Key
                </span>
            }
            onCancel={handleClose}
            closable={saved}
            maskClosable={saved}
            keyboard={saved}
            okText={saved ? "关闭" : "请先勾选「我已保存」"}
            okButtonProps={{ disabled: !saved, danger: false }}
            cancelButtonProps={{ style: { display: "none" } }}
            onOk={handleClose}
            width={560}
            destroyOnClose
        >
            <Paragraph>
                <Text strong>这是你的 API Key，仅此一次展示。</Text>
                关闭后无法再次查看，列表里只能看到脱敏的
                <Text code>sk-fk-...xxxx</Text>。请立即保存到安全的地方（如密码管理器）。
            </Paragraph>

            <div
                style={{
                    padding: 12,
                    background: "#fafafa",
                    border: "1px solid #d9d9d9",
                    borderRadius: 4,
                    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
                    fontSize: 13,
                    wordBreak: "break-all",
                    userSelect: "all",
                }}
            >
                {raw}
            </div>

            <div style={{ marginTop: 8 }}>
                <a onClick={handleCopy}>📋 复制 Key</a>
            </div>

            <Paragraph style={{ marginTop: 16 }} copyable={{ text: raw, tooltips: ["复制", "已复制"] }}>
                <Text type="secondary">使用方式：</Text>
                <br />
                <Text code style={{ fontSize: 12 }}>
                    curl -H &quot;Authorization: Bearer &lt;上面这串&gt;&quot; \<br />
                    &nbsp;&nbsp;https://your-freedom.com/v1/chat/completions \<br />
                    &nbsp;&nbsp;-H &quot;Content-Type: application/json&quot; \<br />
                    &nbsp;&nbsp;-d &apos;&#123;&quot;model&quot;:&quot;gpt-4o-mini&quot;,&quot;messages&quot;:[...]&#125;&apos;
                </Text>
            </Paragraph>

            <Checkbox
                checked={saved}
                onChange={(e) => setSaved(e.target.checked)}
                style={{ marginTop: 8 }}
            >
                我已保存到安全的地方（关闭后无法再看到完整 Key）
            </Checkbox>
        </Modal>
    );
}
