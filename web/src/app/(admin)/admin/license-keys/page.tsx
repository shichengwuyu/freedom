"use client";

import {
    Alert,
    App,
    Button,
    Card,
    Col,
    Descriptions,
    Form,
    Input,
    InputNumber,
    Modal,
    Result,
    Row,
    Space,
    Table,
    Tabs,
    Tag,
    Tooltip,
    Typography,
    Upload,
    type UploadFile,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
    CloudUploadOutlined,
    DownloadOutlined,
    EditOutlined,
    ExportOutlined,
    InboxOutlined,
    KeyOutlined,
    ReloadOutlined,
    SafetyOutlined,
    SearchOutlined,
    WarningOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import { useEffect, useMemo, useState } from "react";

import {
    adminExportLicenseKeysBlob,
    adminGenerateLicenseKeys,
    adminImportLicenseKeys,
    adminListLicenseKeys,
    adminListRedeemLogs,
    adminModifyBatchFaceValue,
    type GenerateLicenseKeysResult,
    type ImportLicenseKeysResult,
    type LicenseKeyItem,
    type RedeemLogItem,
} from "@/services/api/license";
import { useUserStore } from "@/stores/use-user-store";

function maskKey(key?: string) {
    if (!key) return "-";
    const clean = key.replace(/-/g, "");
    if (clean.length !== 16) return key;
    return `${clean.slice(0, 4)}-${clean.slice(4, 8)}-****-****`;
}

type StatusFilter = "" | "unused" | "used";

export default function AdminLicenseKeysPage() {
    return <AdminLicenseKeysContent />;
}

function AdminLicenseKeysContent() {
    const { message, modal } = App.useApp();
    const token = useUserStore((state) => state.token);

    // -------- 导入区 --------
    const [importForm] = Form.useForm<{ batchName: string; faceValueCents: number }>();
    const [file, setFile] = useState<UploadFile | null>(null);
    const [rawFile, setRawFile] = useState<File | null>(null);
    const [fileLinesEstimateN, setFileLinesEstimateN] = useState<number>(0);
    const [importLoading, setImportLoading] = useState(false);
    const [importResult, setImportResult] =
        useState<ImportLicenseKeysResult | null>(null);
    const [selfCheckSamples, setSelfCheckSamples] = useState<string[]>([]);
    const [importedBatchName, setImportedBatchName] = useState<string>("");
    const [importedBatchFaceValueCents, setImportedBatchFaceValueCents] = useState<number>(0);

    // -------- 自动生成区 --------
    const [genForm] = Form.useForm<{ batchName: string; faceValueCents: number; count: number }>();
    const [genLoading, setGenLoading] = useState(false);
    const [genResult, setGenResult] = useState<GenerateLicenseKeysResult | null>(null);

    // -------- Tab 1 卡密列表 --------
    const [tab, setTab] = useState<"keys" | "redeems">("keys");
    const [statusFilter, setStatusFilter] = useState<StatusFilter>("");
    const [batchFilter, setBatchFilter] = useState("");
    const [keywordFilter, setKeywordFilter] = useState("");
    const [keyPage, setKeyPage] = useState(1);
    const [keyPageSize, setKeyPageSize] = useState(20);
    const [keyTotal, setKeyTotal] = useState(0);
    const [keyList, setKeyList] = useState<LicenseKeyItem[]>([]);
    const [keyLoading, setKeyLoading] = useState(false);
    const [editFaceOpen, setEditFaceOpen] = useState(false);
    const [editFaceLoading, setEditFaceLoading] = useState(false);
    const [editFaceForm] = Form.useForm<{ batchName: string; faceValueCents: number }>();

    // -------- Tab 2 兑换记录 --------
    const [userKeyword, setUserKeyword] = useState("");
    const [redeemPage, setRedeemPage] = useState(1);
    const [redeemPageSize, setRedeemPageSize] = useState(20);
    const [redeemTotal, setRedeemTotal] = useState(0);
    const [redeemList, setRedeemList] = useState<RedeemLogItem[]>([]);
    const [redeemLoading, setRedeemLoading] = useState(false);

    const handlePickFile = (fileObj: File) => {
        setRawFile(fileObj);
        setFile({
            uid: fileObj.name,
            name: fileObj.name,
            status: "done",
            originFileObj: fileObj,
        } as UploadFile);
        setImportResult(null);
        setSelfCheckSamples([]);
        fileObj.text()
            .then((txt) => {
                const lines = txt.split(/\r?\n/).filter((l) => l.trim().length > 0);
                setFileLinesEstimateN(lines.length);
            })
            .catch(() => setFileLinesEstimateN(0));
        return false;
    };

    const confirmImport = async () => {
        if (!token) return;
        const values = await importForm.validateFields();
        if (!rawFile) {
            throw new Error("请先上传 TXT 文件");
        }
        try {
            setImportLoading(true);
            const result = await adminImportLicenseKeys(token, {
                file: rawFile,
                batchName: values.batchName,
                // 前端按「元」输入，入库统一转成分（×100），与用户余额单位（分）对齐。
                faceValueCents: Math.round(values.faceValueCents * 100),
            });
            setImportResult(result);
            setImportedBatchName(values.batchName);
            setImportedBatchFaceValueCents(values.faceValueCents);
            setSelfCheckSamples([]);
            message.success(
                `导入完成，成功入库 ${result.importedCount} 张卡密`,
                3,
            );
        } catch (err) {
            modal.error({
                title: "导入失败",
                content: err instanceof Error ? err.message : "请重试",
            });
        } finally {
            setImportLoading(false);
        }
    };

    const startImport = async () => {
        let values: { batchName: string; faceValueCents: number };
        try {
            values = await importForm.validateFields();
        } catch {
            return;
        }
        if (!rawFile) {
            message.warning("请先上传 TXT 文件");
            return;
        }
        const linesInfo =
            fileLinesEstimateN > 0
                ? `共约 ${fileLinesEstimateN} 行`
                : "已选择文件（行数未解析）";
        modal.confirm({
            title: "确认导入卡密批次",
            icon: <SafetyOutlined />,
            content: (
                <div className="space-y-2 leading-7">
                    <div>
                        批次：
                        <Typography.Text strong>
                            「{values.batchName}」
                        </Typography.Text>
                    </div>
                    <div>
                        面额：
                        <Typography.Text strong type="success">
                            ¥{values.faceValueCents.toFixed(2)}
                        </Typography.Text>{" "}
                        / 每张
                    </div>
                    <div>{linesInfo}</div>
                    <Alert
                        type="warning"
                        showIcon
                        className="mt-3"
                        title="导入后该批次在未被兑换前可以整批修改面额；一旦有任意用户完成兑换，将无法再修改面额。"
                    />
                </div>
            ),
            okText: "确认导入",
            cancelText: "取消",
            onOk: confirmImport,
        });
    };

    const confirmGenerate = async () => {
        if (!token) return;
        const values = await genForm.validateFields();
        try {
            setGenLoading(true);
            const result = await adminGenerateLicenseKeys(token, {
                batchName: values.batchName,
                // 前端按「元」输入，入库统一转成分（×100）。
                faceValueCents: Math.round(values.faceValueCents * 100),
                count: values.count,
            });
            setGenResult(result);
            message.success(
                `已自动生成 ${result.generatedCount} 张卡密并保存到服务器文件夹`,
                3,
            );
        } catch (err) {
            modal.error({
                title: "生成失败",
                content: err instanceof Error ? err.message : "请重试",
            });
        } finally {
            setGenLoading(false);
        }
    };

    const downloadExport = async (batch: string) => {
        if (!token) return;
        try {
            const blob = await adminExportLicenseKeysBlob(token, batch);
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = `${batch}.txt`;
            a.click();
            URL.revokeObjectURL(url);
        } catch (err) {
            modal.error({
                title: "下载失败",
                content: err instanceof Error ? err.message : "请重试",
            });
        }
    };

    // 导出当前筛选条件下的全部兑换记录为 CSV（带 BOM，Excel 中文不乱码）。
    const exportRedeemsCsv = async () => {
        if (!token) return;
        try {
            const res = await adminListRedeemLogs(token, {
                page: 1,
                pageSize: 100000,
                userKeyword,
            });
            const items = res.items || [];
            if (!items.length) {
                message.info("没有可导出的兑换记录");
                return;
            }
            const header = ["时间", "用户名", "用户ID", "卡密(脱敏)", "到账(元)"];
            const esc = (s: string) =>
                `"${String(s ?? "").replace(/"/g, '""')}"`;
            const lines = items.map((r) =>
                [
                    r.createdAt || "",
                    r.userName || "",
                    r.userId || "",
                    r.keyMasked || "",
                    (Number(r.faceValueCents) / 100).toFixed(2),
                ]
                    .map(esc)
                    .join(","),
            );
            const csv =
                "﻿" + [header.join(","), ...lines].join("\r\n");
            const blob = new Blob([csv], {
                type: "text/csv;charset=utf-8",
            });
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = `兑换记录_${dayjs().format("YYYYMMDD_HHmmss")}.csv`;
            a.click();
            URL.revokeObjectURL(url);
            message.success(`已导出 ${items.length} 条兑换记录`);
        } catch (err) {
            modal.error({
                title: "导出失败",
                content: err instanceof Error ? err.message : "请重试",
            });
        }
    };

    const runSelfCheck = async () => {
        if (!token || tab !== "keys") {
            message.info("请在卡密列表 Tab 执行自检（从批次筛选出刚导入的批次后再点此按钮）");
            return;
        }
        // 按当前 batchName 筛选（用户一般在刚导入后点，这时 batchFilter 应已填入 importedBatchName）
        const batch = batchFilter || importedBatchName;
        if (!batch) {
            message.warning("请先在下方筛选中选择要自检的批次名");
            return;
        }
        try {
            const { items } = await adminListLicenseKeys(token, {
                page: 1,
                pageSize: 500,
                status: "unused",
                batchName: batch,
            });
            if (!items || items.length === 0) {
                message.warning("该批次下没有可抽验的 unused 卡密");
                return;
            }
            // Fisher-Yates 抽 5 条
            const pool = [...items];
            for (let i = pool.length - 1; i > 0; i--) {
                const j = Math.floor(Math.random() * (i + 1));
                [pool[i], pool[j]] = [pool[j], pool[i]];
            }
            const picked = pool.slice(0, Math.min(5, pool.length));
            setSelfCheckSamples(picked.map((k) => maskKey(k.key)));
            modal.info({
                title: "🔍 导入自检（随机抽样 5 条）",
                width: 720,
                content: (
                    <div className="space-y-3 leading-7">
                        <Alert
                            type="warning"
                            showIcon
                            title="请复制这 5 条原始卡密去链动小铺 ldxp.cn 商家后台 → 该商品 → 库存查询，验证是否同时存在于平台库存中。如果不存在，说明你上传的 TXT 不是同一份文件！"
                        />
                        <div className="space-y-1.5">
                            {picked.map((k, idx) => (
                                <div
                                    key={k.id || idx}
                                    className="rounded-lg border border-stone-200 bg-stone-50 px-3 py-2 dark:border-stone-700 dark:bg-stone-900"
                                >
                                    <div className="text-xs text-stone-500">
                                        #{idx + 1} · 入库原始（ldxp 库存搜这个）
                                    </div>
                                    <div className="mt-0.5 flex items-center justify-between gap-3">
                                        <Typography.Text
                                            copyable
                                            className="font-mono tracking-widest"
                                        >
                                            {k.key}
                                        </Typography.Text>
                                        <Tag
                                            color={
                                                k.status === "unused"
                                                    ? "green"
                                                    : "default"
                                            }
                                        >
                                            {k.status === "unused"
                                                ? "未兑换"
                                                : "已兑"}
                                        </Tag>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                ),
            });
        } catch (err) {
            modal.error({
                title: "自检失败",
                content: err instanceof Error ? err.message : "请稍后重试",
            });
        }
    };

    // 搜索卡密列表
    const refreshKeys = () => {
        if (!token) return;
        setKeyLoading(true);
        void adminListLicenseKeys(token, {
            page: keyPage,
            pageSize: keyPageSize,
            status: statusFilter,
            batchName: batchFilter,
            keyword: keywordFilter,
        })
            .then((res) => {
                setKeyList(res.items || []);
                setKeyTotal(Number(res.total || 0));
            })
            .catch((err) =>
                message.error(err instanceof Error ? err.message : "加载失败"),
            )
            .finally(() => setKeyLoading(false));
    };

    const refreshRedeems = () => {
        if (!token) return;
        setRedeemLoading(true);
        void adminListRedeemLogs(token, {
            page: redeemPage,
            pageSize: redeemPageSize,
            userKeyword,
        })
            .then((res) => {
                setRedeemList(res.items || []);
                setRedeemTotal(Number(res.total || 0));
            })
            .catch((err) =>
                message.error(err instanceof Error ? err.message : "加载失败"),
            )
            .finally(() => setRedeemLoading(false));
    };

    useEffect(() => {
        if (tab === "keys") refreshKeys();
    }, [tab, keyPage, keyPageSize, statusFilter, batchFilter, keywordFilter, token]); // eslint-disable-line react-hooks/exhaustive-deps

    useEffect(() => {
        if (tab === "redeems") refreshRedeems();
    }, [tab, redeemPage, redeemPageSize, userKeyword, token]); // eslint-disable-line react-hooks/exhaustive-deps

    const openModifyBatchFaceValue = () => {
        editFaceForm.setFieldsValue({
            batchName: batchFilter || importedBatchName || "",
            faceValueCents: 0,
        });
        setEditFaceOpen(true);
    };

    const submitModifyBatchFaceValue = async () => {
        if (!token) return;
        const values = await editFaceForm.validateFields();
        setEditFaceLoading(true);
        try {
            const res = await adminModifyBatchFaceValue(token, {
                batchName: values.batchName,
                // 前端按「元」输入，入库统一转成分（×100）。
                faceValueCents: Math.round(values.faceValueCents * 100),
            });
            message.success(
                `已修改 ${res.rowsAffected} 张 unused 卡密的面额为 ¥${values.faceValueCents.toFixed(2)}`,
            );
            setEditFaceOpen(false);
            refreshKeys();
        } catch (err) {
            modal.error({
                title: "修改失败",
                content: err instanceof Error ? err.message : "该批次可能已开兑（有已兑换记录），无法修改面额。",
            });
        } finally {
            setEditFaceLoading(false);
        }
    };

    const keyColumns: ColumnsType<LicenseKeyItem> = useMemo(
        () => [
            {
                title: "卡密（脱敏）",
                dataIndex: "key",
                width: 240,
                render: (val) => (
                    <span className="font-mono text-sm tracking-wider text-stone-700 dark:text-stone-200">
                        {maskKey(val)}
                    </span>
                ),
            },
            {
                title: "批次名",
                dataIndex: "batchName",
                width: 200,
                ellipsis: true,
                render: (val) => (
                    <Tooltip title={val}>
                        <Tag color="geekblue">{val || "-"}</Tag>
                    </Tooltip>
                ),
            },
            {
                title: "面额（元）",
                dataIndex: "faceValueCents",
                width: 120,
                render: (val) => (
                    <Typography.Text strong>
                        ¥{(Number(val) / 100).toFixed(2)}
                    </Typography.Text>
                ),
            },
            {
                title: "状态",
                dataIndex: "status",
                width: 100,
                render: (val) =>
                    val === "used" ? (
                        <Tag color="red">已兑</Tag>
                    ) : (
                        <Tag color="green">未兑</Tag>
                    ),
            },
            {
                title: "使用者 / 导入者",
                width: 180,
                render: (_, row) => (
                    <div className="text-xs leading-5">
                        {row.status === "used" ? (
                            <>
                                <div>
                                    使用者：
                                    <Typography.Text copyable>
                                        {row.usedBy || "-"}
                                    </Typography.Text>
                                </div>
                                <div className="text-stone-400">
                                    {row.usedAt
                                        ? dayjs(row.usedAt).format(
                                              "MM-DD HH:mm",
                                          )
                                        : "-"}
                                </div>
                            </>
                        ) : (
                            <div className="text-stone-500">
                                导入者：
                                <Typography.Text copyable>
                                    {row.createdBy || "-"}
                                </Typography.Text>
                            </div>
                        )}
                    </div>
                ),
            },
            {
                title: "导入时间",
                dataIndex: "createdAt",
                width: 170,
                render: (val) =>
                    val
                        ? dayjs(val).format("YYYY-MM-DD HH:mm:ss")
                        : "-",
            },
        ],
        [],
    );

    const redeemColumns: ColumnsType<RedeemLogItem> = useMemo(
        () => [
            {
                title: "时间",
                dataIndex: "createdAt",
                width: 180,
                render: (val) =>
                    val
                        ? dayjs(val).format("YYYY-MM-DD HH:mm:ss")
                        : "-",
            },
            {
                title: "用户名",
                dataIndex: "userName",
                width: 160,
                render: (_, row) => (
                    <div>
                        <Typography.Text strong>
                            {row.userName || "-"}
                        </Typography.Text>
                        <div className="text-xs text-stone-400">
                            <Typography.Text copyable>
                                {row.userId}
                            </Typography.Text>
                        </div>
                    </div>
                ),
            },
            {
                title: "卡密（脱敏）",
                dataIndex: "keyMasked",
                width: 220,
                render: (val) => (
                    <span className="font-mono text-sm tracking-wider">
                        {val || "-"}
                    </span>
                ),
            },
            {
                title: "到账（元）",
                dataIndex: "faceValueCents",
                width: 130,
                render: (val) => (
                    <Typography.Text strong type="success">
                        +¥{(Number(val) / 100).toFixed(2)}
                    </Typography.Text>
                ),
            },
        ],
        [],
    );

    return (
        <div style={{ padding: 24 }}>
            <Space direction="vertical" size={16} style={{ width: "100%" }}>
                {/* 自动生成区 */}
                <div className="rounded-xl border border-stone-200 bg-white p-6 shadow-sm dark:border-stone-800 dark:bg-stone-950/40">
                    <div className="mb-5 flex items-center gap-2">
                        <div className="inline-flex size-9 items-center justify-center rounded-xl bg-stone-100 text-stone-700 dark:bg-stone-800 dark:text-stone-200">
                            <KeyOutlined />
                        </div>
                        <div>
                            <h2 className="m-0 text-base font-semibold text-stone-900 dark:text-stone-100">
                                自动生成卡密
                            </h2>
                            <p className="m-0 mt-0.5 text-xs text-stone-500 dark:text-stone-400">
                                系统用随机串自动 mint 卡密，入库并保存为 TXT 到服务器文件夹，你拿去链动小铺上架即可。
                            </p>
                        </div>
                    </div>

                    {genResult ? (
                        <Result
                            status="success"
                            title="生成完成"
                            subTitle={
                                <>批次「{genResult.batchName}」· 成功生成 {genResult.generatedCount} 张卡密</>
                            }
                        >
                            <div className="space-y-3">
                                <Alert
                                    type="info"
                                    showIcon
                                    className="text-left"
                                    message="卡密已保存到服务器文件夹"
                                    description={
                                        <code className="rounded bg-stone-100 px-2 py-0.5 font-mono text-[12px] dark:bg-stone-900">
                                            {genResult.filePath}
                                        </code>
                                    }
                                />
                                <Space>
                                    <Button
                                        type="primary"
                                        icon={<DownloadOutlined />}
                                        onClick={() => downloadExport(genResult.batchName)}
                                    >
                                        下载 TXT（一行一张，可直接上传链动小铺）
                                    </Button>
                                    <Button onClick={() => setGenResult(null)}>
                                        再生成一批
                                    </Button>
                                </Space>
                            </div>
                        </Result>
                    ) : (
                        <Form form={genForm} layout="vertical" requiredMark={false}>
                            <Row gutter={16} align="top">
                                <Col xs={24} md={8}>
                                    <Form.Item
                                        label="批次名"
                                        name="batchName"
                                        rules={[{ required: true, message: "必填，如 20面额-202608第一批" }]}
                                    >
                                        <Input placeholder="如：20面额-202608第一批" allowClear />
                                    </Form.Item>
                                </Col>
                                <Col xs={24} md={8}>
                                    <Form.Item
                                        label="每张面额（元）"
                                        name="faceValueCents"
                                        rules={[
                                            { required: true, message: "必填，必须 > 0" },
                                            {
                                                validator: (_, v) =>
                                                    Number(v) > 0
                                                        ? Promise.resolve()
                                                        : Promise.reject("面额必须 > 0"),
                                            },
                                        ]}
                                    >
                                        <InputNumber
                                            style={{ width: "100%" }}
                                            min={1}
                                            step={1}
                                            placeholder="例如：20"
                                        />
                                    </Form.Item>
                                </Col>
                                <Col xs={24} md={8}>
                                    <Form.Item
                                        label="生成数量"
                                        name="count"
                                        rules={[
                                            { required: true, message: "必填，1 ~ 50000" },
                                            {
                                                validator: (_, v) =>
                                                    Number(v) >= 1 && Number(v) <= 50000
                                                        ? Promise.resolve()
                                                        : Promise.reject("数量必须在 1 ~ 50000"),
                                            },
                                        ]}
                                    >
                                        <InputNumber
                                            style={{ width: "100%" }}
                                            min={1}
                                            max={50000}
                                            step={100}
                                            placeholder="例如：1000"
                                        />
                                    </Form.Item>
                                </Col>
                            </Row>
                            <Button
                                type="primary"
                                size="large"
                                icon={<KeyOutlined />}
                                loading={genLoading}
                                onClick={confirmGenerate}
                            >
                                开始生成
                            </Button>
                        </Form>
                    )}
                </div>

                {/* 导入区 */}
                <div className="rounded-xl border border-stone-200 bg-white p-6 shadow-sm dark:border-stone-800 dark:bg-stone-950/40">
                    <div className="mb-5 flex items-center gap-2">
                        <div className="inline-flex size-9 items-center justify-center rounded-xl bg-stone-100 text-stone-700 dark:bg-stone-800 dark:text-stone-200">
                            <CloudUploadOutlined />
                        </div>
                        <div>
                            <h2 className="m-0 text-base font-semibold text-stone-900 dark:text-stone-100">
                                批量导入卡密（TXT）
                            </h2>
                            <p className="m-0 mt-0.5 text-xs text-stone-500 dark:text-stone-400">
                                与链动小铺后台导出的 TXT 格式 <b className="text-emerald-600 dark:text-emerald-400">100% 兼容</b>，不用再二次处理，直接上传即可。
                            </p>
                        </div>
                    </div>

                    <Alert
                        type="error"
                        showIcon
                        icon={<WarningOutlined />}
                        title="【关键提醒】导入的 TXT 卡密文件必须与你在链动小铺 ldxp.cn 对应商品上传的 TXT 完全同一份！"
                        description={
                            <>
                                否则买家收到的卡在本系统查不到，会造成用户投诉和退款纠纷。
                                <br />
                                <strong className="text-red-600 dark:text-red-400">
                                    正确流程：先在 ldxp 上传 TXT 成功 → 再拿同一份文件在这里导入 → 导入后使用下方「🔍
                                    导入自检」功能抽检 5 条，确认在 ldxp 库存里也存在。
                                </strong>
                            </>
                        }
                        className="mb-5"
                    />

                    <Alert
                        type="info"
                        showIcon
                        title={<span className="leading-7"><b>格式与上限 · 与链动小铺完全对齐</b></span>}
                        description={
                            <ul className="list-disc space-y-1 pl-6 pt-1 text-[13px] leading-7 text-stone-700 dark:text-stone-300">
                                <li><b>一行一张</b>，完全空行会被自动忽略，不计入总数</li>
                                <li>
                                    如果一行里同时包含「卡号 + 密码/区号/备注」，<b>第一个空格或分隔符（---- / --- / -- / | / 制表符）之前的内容会被作为卡号入库</b>
                                    （和链动小铺「仅选号模式下，第一个空格前的内容将向买家展示」的规则完全一致）
                                </li>
                                <li>单张卡号/卡密 <b>最长 5600 位</b>，保留原样大小写和字符；用户在本页「兑换卡密」里粘贴 ldxp 发给他的完整字符串即可成功</li>
                                <li>单次最多导入 <b>5 万张</b>；单个批次（同批次名）总库存不能超过 <b>30 万张</b>，超过请分次/换批次名</li>
                            </ul>
                        }
                        className="mb-5"
                    />

                    <div className="mb-5 rounded-lg border border-dashed border-stone-300 bg-stone-50/60 p-4 text-xs leading-7 text-stone-600 dark:border-stone-700 dark:bg-stone-900/40 dark:text-stone-300">
                        <div className="mb-2 font-semibold text-stone-800 dark:text-stone-100">📄 示例格式（所有形式后端均支持）</div>
                        <div className="grid gap-x-8 gap-y-3 sm:grid-cols-2">
                            <div>
                                <div className="mb-1 text-stone-500 dark:text-stone-400">① 纯卡号（最常用）</div>
                                <pre className="rounded-md border border-stone-200 bg-white px-3 py-2 font-mono text-[12px] shadow-sm dark:border-stone-800 dark:bg-stone-950">kami1
kami2
Abc-123-Xyz-999</pre>
                            </div>
                            <div>
                                <div className="mb-1 text-stone-500 dark:text-stone-400">② 卡号 + 空格密码 / 卡号----密码</div>
                                <pre className="rounded-md border border-stone-200 bg-white px-3 py-2 font-mono text-[12px] shadow-sm dark:border-stone-800 dark:bg-stone-950">kaha03   kami1
kaha04----kami3</pre>
                            </div>
                            <div>
                                <div className="mb-1 text-stone-500 dark:text-stone-400">③ 选号模式 · 区号 + 卡号 + 密码</div>
                                <pre className="rounded-md border border-stone-200 bg-white px-3 py-2 font-mono text-[12px] shadow-sm dark:border-stone-800 dark:bg-stone-950">德玛西亚一区 kaha01  mima1
德玛西亚二区 kaha02--mima2</pre>
                            </div>
                            <div>
                                <div className="mb-1 text-stone-500 dark:text-stone-400">④ 空行 / 前后空格</div>
                                <pre className="rounded-md border border-stone-200 bg-white px-3 py-2 font-mono text-[12px] shadow-sm dark:border-stone-800 dark:bg-stone-950">
  kami-a001  

kami-a002
</pre>
                            </div>
                        </div>
                    </div>

                    <Form
                        form={importForm}
                        layout="vertical"
                        requiredMark={false}
                    >
                        <Row gutter={16} align="top">
                            <Col xs={24} md={16}>
                                <Form.Item
                                    label="TXT 卡密文件（每行一张）"
                                    required
                                    tooltip="最大 50MB；支持 ldxp 导出的原始 TXT 直接上传"
                                >
                                    <Upload.Dragger
                                        beforeUpload={handlePickFile}
                                        multiple={false}
                                        accept=".txt,text/plain"
                                        maxCount={1}
                                        fileList={file ? [file] : []}
                                        onRemove={() => {
                                            setFile(null);
                                            setRawFile(null);
                                            setFileLinesEstimateN(0);
                                            setImportResult(null);
                                        }}
                                        height={110}
                                    >
                                        <p className="ant-upload-drag-icon">
                                            <InboxOutlined />
                                        </p>
                                        <p className="ant-upload-hint">
                                            点击或拖拽 .txt 文件到此处（一行一张卡密；重复项或格式错会自动标注在导入报告）
                                        </p>
                                    </Upload.Dragger>
                                </Form.Item>
                            </Col>
                            <Col xs={24} md={8}>
                                <Form.Item
                                    label="批次名"
                                    name="batchName"
                                    rules={[
                                        {
                                            required: true,
                                            message: "必填，如 20面额-202608第一批",
                                        },
                                    ]}
                                >
                                    <Input
                                        placeholder="如：20面额-202608第一批"
                                        allowClear
                                    />
                                </Form.Item>
                                <Form.Item
                                    label="每张面额（元）"
                                    name="faceValueCents"
                                    rules={[
                                        {
                                            required: true,
                                            message: "必填，必须 > 0",
                                        },
                                        {
                                            validator: (_, v) =>
                                                Number(v) > 0
                                                    ? Promise.resolve()
                                                    : Promise.reject(
                                                          "面额必须 > 0",
                                                      ),
                                        },
                                    ]}
                                    tooltip="这个值必须与 ldxp 商品描述的面额一致！本批次每张卡密兑换后给用户加多少金额（分）。"
                                >
                                    <InputNumber
                                        style={{ width: "100%" }}
                                        min={1}
                                        step={1}
                                        placeholder="例如：20"
                                    />
                                </Form.Item>
                            </Col>
                        </Row>

                        <Space size={12} wrap>
                            <Button
                                type="primary"
                                size="large"
                                icon={<CloudUploadOutlined />}
                                loading={importLoading}
                                onClick={startImport}
                            >
                                开始导入
                            </Button>
                            <Button
                                size="large"
                                icon={<KeyOutlined />}
                                onClick={runSelfCheck}
                                disabled={!importResult && !batchFilter}
                            >
                                🔍 导入自检（随机抽样 5 条）
                            </Button>
                            {selfCheckSamples.length ? (
                                <Tooltip
                                    title={selfCheckSamples.join("\n")}
                                    placement="top"
                                >
                                    <Tag color="purple">
                                        已生成 {selfCheckSamples.length} 条自检样本
                                    </Tag>
                                </Tooltip>
                            ) : null}
                        </Space>
                    </Form>

                    {importResult ? (
                        <div className="mt-6">
                            <Result
                                status={
                                    importResult.importedCount > 0
                                        ? "success"
                                        : "warning"
                                }
                                title={
                                    importResult.importedCount > 0
                                        ? "导入完成"
                                        : "本次没有新卡密导入"
                                }
                                subTitle={
                                    importedBatchName ? (
                                        <>
                                            批次「{importedBatchName}」· 面额{" "}
                                            {importedBatchFaceValueCents.toLocaleString()}{" "}
（元）
                                        </>
                                    ) : undefined
                                }
                            >
                                <Descriptions
                                    column={{ xs: 1, sm: 2 }}
                                    size="small"
                                    bordered
                                >
                                    <Descriptions.Item label="解析行数（去空前）">
                                        {importResult.totalLines}
                                    </Descriptions.Item>
                                    <Descriptions.Item label="成功导入">
                                        <Tag color="green" bordered>
                                            {importResult.importedCount}
                                        </Tag>
                                    </Descriptions.Item>
                                    <Descriptions.Item label="重复（系统已存在或本批次重复）">
                                        <Tag color="gold" bordered>
                                            {importResult.duplicateCount}
                                        </Tag>
                                    </Descriptions.Item>
                                    <Descriptions.Item label="格式错误">
                                        <Tag color="red" bordered>
                                            {importResult.malformedCount}
                                        </Tag>
                                    </Descriptions.Item>
                                </Descriptions>
                                {importResult.malformedSamples?.length ? (
                                    <div className="mt-5">
                                        <div className="mb-2 text-sm font-medium text-stone-700 dark:text-stone-300">
                                            格式错误样本（前 {
                                                importResult.malformedSamples.length
                                            } 条）
                                        </div>
                                        <Table
                                            size="small"
                                            rowKey={(r, i) => String(i)}
                                            dataSource={
                                                importResult.malformedSamples
                                            }
                                            pagination={false}
                                            columns={[
                                                {
                                                    title: "#",
                                                    width: 60,
                                                    render: (_, __, i) => i + 1,
                                                },
                                                {
                                                    title: "原始行",
                                                    dataIndex: "raw",
                                                    render: (_, row) => (
                                                        <code className="rounded bg-stone-100 px-2 py-0.5 font-mono text-[11px] dark:bg-stone-900">
                                                            {String(row)}
                                                        </code>
                                                    ),
                                                },
                                            ]}
                                        />
                                    </div>
                                ) : null}
                            </Result>
                        </div>
                    ) : null}
                </div>

                {/* Tabs */}
                <Tabs
                    activeKey={tab}
                    onChange={(k) => setTab(k as "keys" | "redeems")}
                    items={[
                        {
                            key: "keys",
                            label: "卡密列表",
                            children: (
                                <div>
                                    <Card size="small" className="mb-4">
                                        <Row
                                            gutter={12}
                                            align="bottom"
                                            justify="space-between"
                                            wrap
                                        >
                                            <Col flex="auto" style={{ minWidth: 280 }}>
                                                <Space size={12} wrap>
                                                    <div style={{ width: 150 }}>
                                                        <Typography.Text
                                                            type="secondary"
                                                            style={{
                                                                fontSize: 12,
                                                                display:
                                                                    "block",
                                                                marginBottom: 4,
                                                            }}
                                                        >
                                                            状态
                                                        </Typography.Text>
                                                        <select
                                                            className="h-9 w-full rounded-md border border-stone-300 bg-white px-2 text-sm text-stone-800 focus:border-indigo-500 focus:outline-none dark:border-stone-700 dark:bg-stone-950 dark:text-stone-100"
                                                            value={
                                                                statusFilter
                                                            }
                                                            onChange={(e) => {
                                                                setStatusFilter(
                                                                    e.target
                                                                        .value as StatusFilter,
                                                                );
                                                                setKeyPage(1);
                                                            }}
                                                        >
                                                            <option value="">
                                                                全部
                                                            </option>
                                                            <option value="unused">
                                                                未兑
                                                            </option>
                                                            <option value="used">
                                                                已兑
                                                            </option>
                                                        </select>
                                                    </div>
                                                    <div style={{ width: 220 }}>
                                                        <Typography.Text
                                                            type="secondary"
                                                            style={{
                                                                fontSize: 12,
                                                                display:
                                                                    "block",
                                                                marginBottom: 4,
                                                            }}
                                                        >
                                                            批次名
                                                        </Typography.Text>
                                                        <Input
                                                            allowClear
                                                            placeholder="精确匹配批次名"
                                                            value={
                                                                batchFilter
                                                            }
                                                            onChange={(e) => {
                                                                setBatchFilter(
                                                                    e.target
                                                                        .value,
                                                                );
                                                                setKeyPage(1);
                                                            }}
                                                            onPressEnter={() =>
                                                                refreshKeys()
                                                            }
                                                        />
                                                    </div>
                                                    <div style={{ width: 240 }}>
                                                        <Typography.Text
                                                            type="secondary"
                                                            style={{
                                                                fontSize: 12,
                                                                display:
                                                                    "block",
                                                                marginBottom: 4,
                                                            }}
                                                        >
                                                            关键词搜索（卡密/批次/使用者）
                                                        </Typography.Text>
                                                        <Input
                                                            allowClear
                                                            prefix={
                                                                <SearchOutlined />
                                                            }
                                                            value={
                                                                keywordFilter
                                                            }
                                                            onChange={(e) => {
                                                                setKeywordFilter(
                                                                    e.target
                                                                        .value,
                                                                );
                                                                setKeyPage(1);
                                                            }}
                                                            onPressEnter={() =>
                                                                refreshKeys()
                                                            }
                                                        />
                                                    </div>
                                                </Space>
                                            </Col>
                                            <Col flex="none">
                                                <Space>
                                                    <Button onClick={refreshKeys}>
                                                        <ReloadOutlined />
                                                        刷新
                                                    </Button>
                                                    <Button
                                                        type="primary"
                                                        icon={
                                                            <EditOutlined />
                                                        }
                                                        onClick={
                                                            openModifyBatchFaceValue
                                                        }
                                                    >
                                                        整批修改面额
                                                    </Button>
                                                </Space>
                                            </Col>
                                        </Row>
                                    </Card>

                                    <Table<LicenseKeyItem>
                                        rowKey="id"
                                        size="middle"
                                        loading={keyLoading}
                                        dataSource={keyList}
                                        columns={keyColumns}
                                        scroll={{ x: 1080 }}
                                        pagination={{
                                            current: keyPage,
                                            pageSize: keyPageSize,
                                            total: keyTotal,
                                            showSizeChanger: true,
                                            showQuickJumper: true,
                                            pageSizeOptions: [
                                                "20",
                                                "50",
                                                "100",
                                            ],
                                            onChange: (p, ps) => {
                                                setKeyPage(p);
                                                setKeyPageSize(ps);
                                            },
                                        }}
                                    />
                                </div>
                            ),
                        },
                        {
                            key: "redeems",
                            label: "兑换记录",
                            children: (
                                <div>
                                    <Card size="small" className="mb-4">
                                        <Row
                                            gutter={12}
                                            align="bottom"
                                            justify="space-between"
                                        >
                                            <Col style={{ width: 320 }}>
                                                <Typography.Text
                                                    type="secondary"
                                                    style={{
                                                        fontSize: 12,
                                                        display: "block",
                                                        marginBottom: 4,
                                                    }}
                                                >
                                                    用户名 / 卡密掩码
                                                </Typography.Text>
                                                <Input
                                                    allowClear
                                                    prefix={
                                                        <SearchOutlined />
                                                    }
                                                    placeholder="输入用户名搜索"
                                                    value={userKeyword}
                                                    onChange={(e) => {
                                                        setUserKeyword(
                                                            e.target.value,
                                                        );
                                                        setRedeemPage(1);
                                                    }}
                                                    onPressEnter={() =>
                                                        refreshRedeems()
                                                    }
                                                />
                                            </Col>
                                            <Col>
                                                <Space>
                                                    <Button onClick={refreshRedeems}>
                                                        <ReloadOutlined />
                                                        查询
                                                    </Button>
                                                    <Button
                                                        icon={<ExportOutlined />}
                                                        onClick={exportRedeemsCsv}
                                                    >
                                                        CSV 导出
                                                    </Button>
                                                </Space>
                                            </Col>
                                        </Row>
                                    </Card>
                                    <Table<RedeemLogItem>
                                        rowKey="id"
                                        size="middle"
                                        loading={redeemLoading}
                                        dataSource={redeemList}
                                        columns={redeemColumns}
                                        scroll={{ x: 820 }}
                                        pagination={{
                                            current: redeemPage,
                                            pageSize: redeemPageSize,
                                            total: redeemTotal,
                                            showSizeChanger: true,
                                            showQuickJumper: true,
                                            pageSizeOptions: [
                                                "20",
                                                "50",
                                                "100",
                                            ],
                                            onChange: (p, ps) => {
                                                setRedeemPage(p);
                                                setRedeemPageSize(ps);
                                            },
                                        }}
                                    />
                                </div>
                            ),
                        },
                    ]}
                />
            </Space>

            <Modal
                open={editFaceOpen}
                title="整批修改未兑换卡密面额"
                onCancel={() => setEditFaceOpen(false)}
                confirmLoading={editFaceLoading}
                onOk={submitModifyBatchFaceValue}
                width={520}
            >
                <Alert
                    type="warning"
                    showIcon
                    className="mb-4"
                    title="仅该批次内全部未兑换的卡密才会被修改；一旦该批次存在任意一张已兑换卡密，整个批次将被锁定不可修改。"
                />
                <Form
                    form={editFaceForm}
                    layout="vertical"
                    requiredMark={false}
                >
                    <Form.Item
                        label="批次名"
                        name="batchName"
                        rules={[
                            {
                                required: true,
                                message: "必填",
                            },
                        ]}
                    >
                        <Input placeholder="例如：20面额-202608第一批" />
                    </Form.Item>
                    <Form.Item
                        label="新面额（元）"
                        name="faceValueCents"
                        rules={[
                            { required: true, message: "必填，必须 > 0" },
                            {
                                validator: (_, v) =>
                                    Number(v) > 0
                                        ? Promise.resolve()
                                        : Promise.reject("面额必须 > 0"),
                            },
                        ]}
                    >
                        <InputNumber
                            style={{ width: "100%" }}
                            min={1}
                            step={1}
                            placeholder="例如：30（想把 20 改成 30 就填 30）"
                        />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
