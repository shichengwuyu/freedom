"use client";

import {
    CreditCardFilled,
    DollarOutlined,
    GiftOutlined,
    HistoryOutlined,
    KeyOutlined,
    ReloadOutlined,
    ShoppingCartOutlined,
    CustomerServiceOutlined,
    CopyOutlined,
    ShareAltOutlined,
} from "@ant-design/icons";
import {
    App,
    Button,
    Card,
    Modal,
    Space,
    Table,
    Tabs,
    Tag,
    Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import {
    getMyBalanceLogs,
    getMyRedeemLogs,
    getPurchaseConfig,
    redeemLicenseKey,
    getMyAffiliateInfo,
    getMyAffiliateCommissions,
    type BalanceLogItem,
    type RedeemLogItem,
    type MyAffiliateInfo,
    type AffCommissionItem,
} from "@/services/api/license";
import { useUserStore } from "@/stores/use-user-store";
import { useConfigStore } from "@/stores/use-config-store";
import { useCopyText } from "@/hooks/use-copy-text";
import { ApiKeyManager } from "./components/api-key-manager";
import { PricingTable } from "./components/pricing-table";

// 单张图片估算成本：4 分 = ¥0.04（前端静态估算常量，仅做估算提示用）。
const PER_IMAGE_COST_CENTS = 4;

const balanceLogTypeLabels: Record<string, { label: string; color: string }> = {
    manual_adjust: { label: "后台调整", color: "blue" },
    generation_consume: { label: "模型消费", color: "red" },
    generation_refund: { label: "失败返还", color: "cyan" },
    manual_recharge: { label: "卡密充值", color: "green" },
};

export default function WalletPage() {
    return <WalletPageContent />;
}

function WalletPageContent() {
    const { message } = App.useApp();
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const contactSupport = useConfigStore((state) => state.publicSettings?.contactSupport);
    const copyText = useCopyText();

    const [purchaseURL, setPurchaseURL] = useState<string>("");
    const [contactModalOpen, setContactModalOpen] = useState(false);

    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(10);
    const [total, setTotal] = useState(0);
    const [balanceLogs, setBalanceLogs] = useState<BalanceLogItem[]>([]);
    const [loading, setLoading] = useState(false);

    // 兑换卡密
    const [redeemModalOpen, setRedeemModalOpen] = useState(false);
    const [redeemKey, setRedeemKey] = useState("");
    const [redeeming, setRedeeming] = useState(false);

    // 兑换记录
    const [redeemLogs, setRedeemLogs] = useState<RedeemLogItem[]>([]);
    const [redeemLogTotal, setRedeemLogTotal] = useState(0);
    const [redeemLogPage, setRedeemLogPage] = useState(1);
    const [redeemLogPageSize, setRedeemLogPageSize] = useState(10);
    const [redeemLogLoading, setRedeemLogLoading] = useState(false);
    const [activeTab, setActiveTab] = useState("movements");

    // 我的邀请
    const [affInfo, setAffInfo] = useState<MyAffiliateInfo | null>(null);
    const [affLoading, setAffLoading] = useState(false);
    const [affCommissionLogs, setAffCommissionLogs] = useState<AffCommissionItem[]>([]);
    const [affCommissionTotal, setAffCommissionTotal] = useState(0);
    const [affCommissionPage, setAffCommissionPage] = useState(1);
    const [affCommissionPageSize, setAffCommissionPageSize] = useState(10);
    const [affCommissionLoading, setAffCommissionLoading] = useState(false);

    const inviteLink = useMemo(() => {
        if (!affInfo?.affCode) return "";
        const origin = typeof window !== "undefined" ? window.location.origin : "";
        return `${origin}/register?inviterCode=${affInfo.affCode}`;
    }, [affInfo?.affCode]);

    const hasContactInfo = Boolean(
        contactSupport?.enabled &&
            (contactSupport.wechat ||
                contactSupport.qq ||
                contactSupport.qqGroup ||
                contactSupport.wechatQr ||
                contactSupport.qqGroupQr)
    );

    const balanceCents = user?.balanceCents ?? 0;
    const approxImages = useMemo(() => {
        if (balanceCents <= 0) return 0;
        return Math.max(0, Math.floor(balanceCents / PER_IMAGE_COST_CENTS));
    }, [balanceCents]);

    useEffect(() => {
        void getPurchaseConfig()
            .then((cfg) => setPurchaseURL(cfg.purchaseURL || ""))
            .catch(() => {});
    }, []);

    useEffect(() => {
        if (!token) return;
        setLoading(true);
        void getMyBalanceLogs(token, { page, pageSize })
            .then((res) => {
                setBalanceLogs(res.items || []);
                setTotal(Number(res.total || 0));
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "加载失败"))
            .finally(() => setLoading(false));
    }, [message, page, pageSize, token]);

    const openPurchase = () => {
        if (!purchaseURL) {
            void message.warning("购买地址加载中，请稍候再试");
            return;
        }
        window.open(purchaseURL, "_blank", "noopener,noreferrer");
    };

    // 加载兑换记录
    useEffect(() => {
        if (!token || activeTab !== "redeems") return;
        setRedeemLogLoading(true);
        void getMyRedeemLogs(token, { page: redeemLogPage, pageSize: redeemLogPageSize })
            .then((res) => {
                setRedeemLogs(res.items || []);
                setRedeemLogTotal(Number(res.total || 0));
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "加载失败"))
            .finally(() => setRedeemLogLoading(false));
    }, [message, token, activeTab, redeemLogPage, redeemLogPageSize]);

    // 加载我的邀请信息（进入邀请 tab 时）
    useEffect(() => {
        if (!token || activeTab !== "invite") return;
        setAffLoading(true);
        void getMyAffiliateInfo(token)
            .then((info) => setAffInfo(info))
            .catch((err) => message.error(err instanceof Error ? err.message : "加载邀请信息失败"))
            .finally(() => setAffLoading(false));
    }, [message, token, activeTab]);

    // 加载邀请返佣流水
    useEffect(() => {
        if (!token || activeTab !== "invite") return;
        setAffCommissionLoading(true);
        void getMyAffiliateCommissions(token, { page: affCommissionPage, pageSize: affCommissionPageSize })
            .then((res) => {
                setAffCommissionLogs(res.items || []);
                setAffCommissionTotal(Number(res.total || 0));
            })
            .catch((err) => message.error(err instanceof Error ? err.message : "加载返佣流水失败"))
            .finally(() => setAffCommissionLoading(false));
    }, [message, token, activeTab, affCommissionPage, affCommissionPageSize]);

    const handleRedeem = async () => {
        const key = redeemKey.trim();
        if (!key) {
            void message.warning("请输入卡密");
            return;
        }
        setRedeeming(true);
        try {
            const res = await redeemLicenseKey(token, key);
            void message.success(`兑换成功，到账 ¥${(res.faceValueCentsGranted / 100).toFixed(2)}，当前余额 ¥${(res.newBalanceCents / 100).toFixed(2)}`);
            setRedeemKey("");
            setRedeemModalOpen(false);
            void hydrateUser();
        } catch (err) {
            void message.error(err instanceof Error ? err.message : "兑换失败");
        } finally {
            setRedeeming(false);
        }
    };

    const balanceColumns: ColumnsType<BalanceLogItem> = [
        {
            title: "时间",
            dataIndex: "createdAt",
            width: 200,
            render: (val) => (val ? dayjs(val).format("YYYY-MM-DD HH:mm:ss") : "-"),
        },
        {
            title: "类型",
            dataIndex: "type",
            width: 130,
            render: (val) => {
                const info = balanceLogTypeLabels[val] || {
                    label: val || "-",
                    color: "default",
                };
                return <Tag color={info.color}>{info.label}</Tag>;
            },
        },
        {
            title: "变动金额",
            dataIndex: "amount",
            width: 140,
            render: (val) => {
                const cents = Number(val) || 0;
                const yuan = cents / 100;
                const display = `${cents >= 0 ? "+" : ""}¥${yuan.toFixed(2)}`;
                return (
                    <Typography.Text strong type={cents >= 0 ? "success" : "danger"}>
                        {display}
                    </Typography.Text>
                );
            },
        },
        {
            title: "变动后余额",
            dataIndex: "balance",
            width: 160,
            render: (val) => {
                const yuan = (Number(val) || 0) / 100;
                return `¥${yuan.toFixed(2)}`;
            },
        },
        {
            title: "备注",
            dataIndex: "remark",
            ellipsis: true,
            render: (val) => (
                <Typography.Text type="secondary">{val || "-"}</Typography.Text>
            ),
        },
        {
            title: "关联 ID",
            dataIndex: "relatedId",
            width: 220,
            render: (val) =>
                val ? (
                    <Typography.Text copyable className="font-mono text-xs">
                        {val}
                    </Typography.Text>
                ) : (
                    "-"
                ),
        },
    ];

    const redeemLogColumns: ColumnsType<RedeemLogItem> = [
        {
            title: "时间",
            dataIndex: "createdAt",
            width: 200,
            render: (val) => (val ? dayjs(val).format("YYYY-MM-DD HH:mm:ss") : "-"),
        },
        {
            title: "卡密",
            dataIndex: "keyMasked",
            width: 200,
            render: (val) => (
                <Typography.Text className="font-mono text-xs">{val || "-"}</Typography.Text>
            ),
        },
        {
            title: "面额",
            dataIndex: "faceValueCents",
            width: 120,
            render: (val) => {
                const yuan = (Number(val) || 0) / 100;
                return (
                    <Typography.Text strong type="success">
                        +¥{yuan.toFixed(2)}
                    </Typography.Text>
                );
            },
        },
        {
            title: "账号",
            dataIndex: "userName",
            render: (val) => val || "-",
        },
    ];

    const affCommissionColumns: ColumnsType<AffCommissionItem> = [
        {
            title: "时间",
            dataIndex: "createdAt",
            width: 200,
            render: (val) => (val ? dayjs(val).format("YYYY-MM-DD HH:mm:ss") : "-"),
        },
        {
            title: "被邀请人消费",
            dataIndex: "rechargeCents",
            width: 150,
            render: (val) => {
                const yuan = (Number(val) || 0) / 100;
                return `¥${yuan.toFixed(2)}`;
            },
        },
        {
            title: "分成比例",
            dataIndex: "rate",
            width: 110,
            render: (val) => {
                const rate = Number(val) || 0;
                return `${Math.round(rate * 100)}%`;
            },
        },
        {
            title: "返佣金额",
            dataIndex: "commissionCents",
            width: 140,
            render: (val) => {
                const yuan = (Number(val) || 0) / 100;
                return (
                    <Typography.Text strong type="success">
                        +¥{yuan.toFixed(2)}
                    </Typography.Text>
                );
            },
        },
        {
            title: "状态",
            dataIndex: "status",
            width: 100,
            render: (val) => {
                if (val === "settled") return <Tag color="green">已结算</Tag>;
                if (val === "pending") return <Tag color="gold">待结算</Tag>;
                if (val === "cancelled") return <Tag color="default">已取消</Tag>;
                return <Tag>{val || "-"}</Tag>;
            },
        },
    ];

    return (
        <main className="h-full min-h-0 overflow-y-auto">
            <div className="mx-auto max-w-5xl px-6 py-8">
                <header className="mb-6 flex items-center justify-between gap-4">
                    <div>
                        <h1 className="text-2xl font-semibold tracking-tight text-stone-900 dark:text-stone-100">
                            账户余额
                        </h1>
                        <p className="mt-1 text-sm text-stone-500 dark:text-stone-400">
                            查看当前余额与历史流水，余额按人民币元（¥）显示。
                        </p>
                    </div>
                    <Space>
                        <Button
                            icon={<GiftOutlined />}
                            onClick={() => setRedeemModalOpen(true)}
                        >
                            兑换卡密
                        </Button>
                        <Button
                            type="primary"
                            icon={<ShoppingCartOutlined />}
                            onClick={openPurchase}
                            disabled={!purchaseURL}
                        >
                            购买充值卡密
                        </Button>
                        {hasContactInfo ? (
                            <Button
                                icon={<CustomerServiceOutlined />}
                                onClick={() => setContactModalOpen(true)}
                            >
                                联系客服
                            </Button>
                        ) : null}
                    </Space>
                </header>

                {/* 余额卡片 */}
                <Card
                    variant="borderless"
                    className="mb-6 shadow-sm ring-1 ring-stone-200 dark:ring-stone-800"
                >
                    <div className="grid grid-cols-1 items-center gap-6 md:grid-cols-5">
                        <div className="md:col-span-3">
                            <div className="mb-2 text-sm font-medium text-stone-500 dark:text-stone-400">
                                当前余额
                            </div>
                            <div className="flex items-baseline gap-2 tabular-nums">
                                <span className="text-5xl font-bold text-stone-900 dark:text-stone-50">
                                    ¥{(balanceCents / 100).toFixed(2)}
                                </span>
                            </div>
                            <div className="mt-3 text-sm leading-6 text-stone-500 dark:text-stone-400">
                                按 ¥0.04 / 张估算，当前余额约可生成{" "}
                                <span className="font-semibold text-stone-800 dark:text-stone-200">
                                    {approxImages.toLocaleString()}
                                </span>{" "}
                                张图片（仅参考，实际扣费以模型定价为准）。
                            </div>
                            {balanceCents < 500 ? (
                                <div className="mt-4">
                                    <Space>
                                        <Button
                                            type="primary"
                                            size="large"
                                            onClick={openPurchase}
                                            disabled={!purchaseURL}
                                        >
                                            立即充值
                                        </Button>
                                        {hasContactInfo ? (
                                            <Button
                                                size="large"
                                                icon={<CustomerServiceOutlined />}
                                                onClick={() => setContactModalOpen(true)}
                                            >
                                                联系客服
                                            </Button>
                                        ) : null}
                                        <Link href="/image">
                                            <Button size="large">去生成图片</Button>
                                        </Link>
                                    </Space>
                                </div>
                            ) : null}
                        </div>
                        <div className="md:col-span-2">
                            <div className="rounded-2xl border border-dashed border-stone-200 bg-stone-50 p-5 dark:border-stone-700 dark:bg-stone-900/40">
                                <div className="mb-2 flex items-center gap-1.5 font-medium text-stone-800 dark:text-stone-200">
                                    <CreditCardFilled />
                                    <span>余额扣费说明</span>
                                </div>
                                <ul className="space-y-1.5 pl-0.5 text-sm leading-6 text-stone-600 dark:text-stone-300">
                                    <li>· 每次调用模型按管理员后台配置的金额扣费</li>
                                    <li>· 不同模型扣费不同，单价由管理员设定</li>
                                    <li>· AI 调用失败时会自动原路退还</li>
                                </ul>
                            </div>
                        </div>
                    </div>
                </Card>

                {/* 余额流水 */}
                <Card
                    variant="borderless"
                    className="mb-6 shadow-sm ring-1 ring-stone-200 dark:ring-stone-800"
                >
                    <Tabs
                        activeKey={activeTab}
                        onChange={setActiveTab}
                        items={[
                            {
                                key: "movements",
                                label: (
                                    <span>
                                        <HistoryOutlined className="mr-1" />
                                        余额流水
                                    </span>
                                ),
                                children: (
                                    <div className="pt-2">
                                        <div className="mb-3 flex justify-end">
                                            <Button
                                                size="small"
                                                icon={<ReloadOutlined />}
                                                onClick={() => {
                                                    setPage(1);
                                                    setLoading(true);
                                                    void getMyBalanceLogs(token, {
                                                        page: 1,
                                                        pageSize,
                                                    })
                                                        .then((res) => {
                                                            setBalanceLogs(res.items || []);
                                                            setTotal(Number(res.total || 0));
                                                        })
                                                        .finally(() => setLoading(false));
                                                }}
                                            >
                                                刷新
                                            </Button>
                                        </div>
                                        <Table<BalanceLogItem>
                                            rowKey="id"
                                            size="middle"
                                            loading={loading}
                                            dataSource={balanceLogs}
                                            columns={balanceColumns}
                                            pagination={{
                                                current: page,
                                                pageSize,
                                                total,
                                                showSizeChanger: true,
                                                pageSizeOptions: ["10", "20", "50"],
                                                onChange: (p, ps) => {
                                                    setPage(p);
                                                    setPageSize(ps);
                                                },
                                            }}
                                        />
                                    </div>
                                ),
                            },
                            {
                                key: "redeems",
                                label: (
                                    <span>
                                        <GiftOutlined className="mr-1" />
                                        兑换记录
                                    </span>
                                ),
                                children: (
                                    <div className="pt-2">
                                        <div className="mb-3 flex justify-end">
                                            <Button
                                                size="small"
                                                icon={<ReloadOutlined />}
                                                onClick={() => {
                                                    setRedeemLogPage(1);
                                                    setRedeemLogLoading(true);
                                                    void getMyRedeemLogs(token, { page: 1, pageSize: redeemLogPageSize })
                                                        .then((res) => {
                                                            setRedeemLogs(res.items || []);
                                                            setRedeemLogTotal(Number(res.total || 0));
                                                        })
                                                        .finally(() => setRedeemLogLoading(false));
                                                }}
                                            >
                                                刷新
                                            </Button>
                                        </div>
                                        <Table<RedeemLogItem>
                                            rowKey="id"
                                            size="middle"
                                            loading={redeemLogLoading}
                                            dataSource={redeemLogs}
                                            columns={redeemLogColumns}
                                            pagination={{
                                                current: redeemLogPage,
                                                pageSize: redeemLogPageSize,
                                                total: redeemLogTotal,
                                                showSizeChanger: true,
                                                pageSizeOptions: ["10", "20", "50"],
                                                onChange: (p, ps) => {
                                                    setRedeemLogPage(p);
                                                    setRedeemLogPageSize(ps);
                                                },
                                            }}
                                        />
                                    </div>
                                ),
                            },
                            {
                                key: "invite",
                                label: (
                                    <span>
                                        <ShareAltOutlined className="mr-1" />
                                        我的邀请
                                    </span>
                                ),
                                children: (
                                    <div className="pt-2">
                                        {affLoading || !affInfo ? (
                                            <div className="flex justify-center py-10 text-stone-400">
                                                加载中…
                                            </div>
                                        ) : (
                                            <>
                                                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                                                    <div className="rounded-xl border border-stone-200 bg-stone-50 p-4 dark:border-stone-700 dark:bg-stone-900/40">
                                                        <div className="text-xs text-stone-500 dark:text-stone-400">已邀请好友</div>
                                                        <div className="mt-1 text-2xl font-bold tabular-nums">{affInfo.affCount}</div>
                                                    </div>
                                                    <div className="rounded-xl border border-stone-200 bg-stone-50 p-4 dark:border-stone-700 dark:bg-stone-900/40">
                                                        <div className="text-xs text-stone-500 dark:text-stone-400">累计已结算返佣</div>
                                                        <div className="mt-1 text-2xl font-bold tabular-nums text-emerald-600 dark:text-emerald-400">
                                                            ¥{((affInfo.totalCommissionCents || 0) / 100).toFixed(2)}
                                                        </div>
                                                    </div>
                                                    <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-700/60 dark:bg-amber-900/20">
                                                        <div className="text-xs text-amber-600 dark:text-amber-400">待结算返佣（每日 00:10 入账）</div>
                                                        <div className="mt-1 text-2xl font-bold tabular-nums text-amber-600 dark:text-amber-400">
                                                            ¥{((affInfo.pendingCommissionCents || 0) / 100).toFixed(2)}
                                                        </div>
                                                    </div>
                                                </div>

                                                <div className="mt-4 rounded-xl border border-stone-200 p-4 dark:border-stone-700">
                                                    <div className="mb-2 text-sm font-medium text-stone-700 dark:text-stone-200">
                                                        我的邀请码
                                                    </div>
                                                    <div className="flex flex-wrap items-center gap-2">
                                                        <Typography.Text copyable className="font-mono text-base">
                                                            {affInfo.affCode}
                                                        </Typography.Text>
                                                        <Button
                                                            size="small"
                                                            icon={<CopyOutlined />}
                                                            onClick={() => copyText(inviteLink, "邀请链接已复制")}
                                                        >
                                                            复制邀请链接
                                                        </Button>
                                                    </div>
                                                    <div className="mt-2 text-xs text-stone-500 dark:text-stone-400">
                                                        好友通过此链接注册并产生消费，你可获得其消费金额阶梯比例的返佣。返佣每日 00:10 统一结算入账（仅官方托管版生效）。
                                                    </div>
                                                    <div className="mt-1 break-all rounded-lg bg-stone-100 px-2 py-1 font-mono text-xs text-stone-600 dark:bg-stone-800 dark:text-stone-300">
                                                        {inviteLink}
                                                    </div>
                                                    <div className="mt-2 text-xs text-stone-500 dark:text-stone-400">
                                                        当前返佣比例：<span className="font-semibold text-emerald-600 dark:text-emerald-400">{Math.round((affInfo.currentRate || 0) * 100)}%</span>
                                                        {affInfo.affCount >= 0 && (affInfo.nextRate || 0) > (affInfo.currentRate || 0) ? (
                                                            <span> · 再邀请 1 人升至 {Math.round((affInfo.nextRate || 0) * 100)}%</span>
                                                        ) : (affInfo.currentRate || 0) >= 0.1 ? (
                                                            <span> · 已达封顶 10%</span>
                                                        ) : null}
                                                    </div>
                                                </div>

                                                <div className="mb-3 mt-5 flex items-center justify-between">
                                                    <div className="text-sm font-medium text-stone-700 dark:text-stone-200">
                                                        返佣流水
                                                    </div>
                                                    <Button
                                                        size="small"
                                                        icon={<ReloadOutlined />}
                                                        onClick={() => {
                                                            setAffCommissionPage(1);
                                                            setAffCommissionLoading(true);
                                                            void getMyAffiliateCommissions(token, { page: 1, pageSize: affCommissionPageSize })
                                                                .then((res) => {
                                                                    setAffCommissionLogs(res.items || []);
                                                                    setAffCommissionTotal(Number(res.total || 0));
                                                                })
                                                                .finally(() => setAffCommissionLoading(false));
                                                        }}
                                                    >
                                                        刷新
                                                    </Button>
                                                </div>
                                                <Table<AffCommissionItem>
                                                    rowKey="id"
                                                    size="middle"
                                                    loading={affCommissionLoading}
                                                    dataSource={affCommissionLogs}
                                                    columns={affCommissionColumns}
                                                    pagination={{
                                                        current: affCommissionPage,
                                                        pageSize: affCommissionPageSize,
                                                        total: affCommissionTotal,
                                                        showSizeChanger: true,
                                                        pageSizeOptions: ["10", "20", "50"],
                                                        onChange: (p, ps) => {
                                                            setAffCommissionPage(p);
                                                            setAffCommissionPageSize(ps);
                                                        },
                                                    }}
                                                />
                                            </>
                                        )}
                                    </div>
                                ),
                            },
                            {
                                key: "apiKeys",
                                label: (
                                    <span>
                                        <KeyOutlined className="mr-1" />
                                        API Key
                                    </span>
                                ),
                                children: (
                                    <div className="pt-2">
                                        <ApiKeyManager token={token} />
                                    </div>
                                ),
                            },
                            {
                                // Sprint 3：价目表
                                key: "pricing",
                                label: (
                                    <span>
                                        <DollarOutlined className="mr-1" />
                                        价目表
                                    </span>
                                ),
                                children: (
                                    <div className="pt-2">
                                        <PricingTable currentUserGroupId={user?.groupId ?? ""} />
                                    </div>
                                ),
                            },
                        ]}
                    />
                </Card>

                <footer className="pb-8 text-center text-xs text-stone-400 dark:text-stone-500">
                    余额单位为人民币元（¥），由管理员在后台按模型配置扣费。如有充值问题请联系发卡平台客服。
                </footer>
            </div>
            {hasContactInfo ? (
                <Modal
                    title="联系客服"
                    open={contactModalOpen}
                    onCancel={() => setContactModalOpen(false)}
                    footer={
                        <Button onClick={() => setContactModalOpen(false)}>关闭</Button>
                    }
                    destroyOnHidden
                >
                    <Space direction="vertical" size={16} style={{ width: "100%" }}>
                        {contactSupport?.wechat ? (
                            <div className="flex items-center justify-between gap-3 rounded-lg border border-stone-200 bg-stone-50 p-3 dark:border-stone-700 dark:bg-stone-900/50">
                                <div>
                                    <Typography.Text type="secondary" className="text-xs">微信号</Typography.Text>
                                    <div className="mt-0.5 font-mono text-base font-semibold">{contactSupport.wechat}</div>
                                </div>
                                <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(contactSupport.wechat!, "微信号已复制")}>复制</Button>
                            </div>
                        ) : null}
                        {contactSupport?.qq ? (
                            <div className="flex items-center justify-between gap-3 rounded-lg border border-stone-200 bg-stone-50 p-3 dark:border-stone-700 dark:bg-stone-900/50">
                                <div>
                                    <Typography.Text type="secondary" className="text-xs">QQ 号</Typography.Text>
                                    <div className="mt-0.5 font-mono text-base font-semibold">{contactSupport.qq}</div>
                                </div>
                                <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(contactSupport.qq!, "QQ 号已复制")}>复制</Button>
                            </div>
                        ) : null}
                        {contactSupport?.qqGroup ? (
                            <div className="flex items-center justify-between gap-3 rounded-lg border border-stone-200 bg-stone-50 p-3 dark:border-stone-700 dark:bg-stone-900/50">
                                <div>
                                    <Typography.Text type="secondary" className="text-xs">QQ 群号</Typography.Text>
                                    <div className="mt-0.5 font-mono text-base font-semibold">{contactSupport.qqGroup}</div>
                                </div>
                                <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(contactSupport.qqGroup!, "QQ 群号已复制")}>复制</Button>
                            </div>
                        ) : null}
                        <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
                            {contactSupport?.wechatQr ? (
                                <div className="text-center">
                                    <div className="mb-2 text-xs text-stone-500 dark:text-stone-400">微信 / 微信群二维码</div>
                                    <img src={contactSupport.wechatQr} alt="微信二维码" style={{ width: 160, height: 160, objectFit: "contain", borderRadius: 8, border: "1px solid var(--ant-color-border)" }} />
                                </div>
                            ) : contactSupport?.wechat ? (
                                <div className="text-center">
                                    <div className="mb-2 text-xs text-stone-500 dark:text-stone-400">添加微信号咨询</div>
                                    <div className="rounded-lg border border-stone-200 bg-stone-50 p-3 font-mono text-base font-semibold dark:border-stone-700 dark:bg-stone-900/50">
                                        {contactSupport.wechat}
                                    </div>
                                </div>
                            ) : null}
                            {contactSupport?.qqGroupQr ? (
                                <div className="text-center">
                                    <div className="mb-2 text-xs text-stone-500 dark:text-stone-400">QQ 群二维码</div>
                                    <img src={contactSupport.qqGroupQr} alt="QQ群二维码" style={{ width: 160, height: 160, objectFit: "contain", borderRadius: 8, border: "1px solid var(--ant-color-border)" }} />
                                </div>
                            ) : null}
                        </div>
                        {contactSupport?.remark ? (
                            <Typography.Text type="secondary" className="block text-center text-xs">
                                {contactSupport.remark}
                            </Typography.Text>
                        ) : null}
                    </Space>
                </Modal>
            ) : null}
            <Modal
                title="兑换卡密"
                open={redeemModalOpen}
                onCancel={() => {
                    setRedeemModalOpen(false);
                    setRedeemKey("");
                }}
                onOk={handleRedeem}
                confirmLoading={redeeming}
                okText="兑换"
                cancelText="取消"
                destroyOnHidden
            >
                <Space direction="vertical" size={12} style={{ width: "100%" }}>
                    <Typography.Text type="secondary" className="text-sm">
                        请输入您购买的卡密，兑换后金额将自动充入余额。
                    </Typography.Text>
                    <input
                        type="text"
                        value={redeemKey}
                        onChange={(e) => setRedeemKey(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === "Enter") void handleRedeem();
                        }}
                        placeholder="输入卡密"
                        className="w-full rounded-lg border border-stone-300 px-3 py-2 text-sm outline-none focus:border-stone-500 dark:border-stone-600 dark:bg-stone-900 dark:text-stone-100"
                        disabled={redeeming}
                    />
                </Space>
            </Modal>
        </main>
    );
}
