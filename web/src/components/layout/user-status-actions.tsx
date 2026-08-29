"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, RefObject } from "react";
import { App, Avatar, Button, Dropdown, Input, Modal, Space, Tooltip, Typography, Upload, message } from "antd";
import type { UploadFile, UploadProps } from "antd";
import type { UploadChangeParam } from "antd/es/upload/interface";
import { CopyOutlined } from "@ant-design/icons";
import { Gift, Headphones, Keyboard, LogOut, RefreshCw, Settings2, Shield, ShoppingCart, UserRoundCog, Wand2 } from "lucide-react";
import type { ItemType } from "antd/es/menu/interface";
import Link from "next/link";
import axios from "axios";

import { AnimatedThemeToggler } from "@/components/ui/animated-theme-toggler";
import { BalanceSymbol, formatBalanceYuan } from "@/constant/balance";
import { canvasThemes } from "@/lib/canvas-theme";
import { useConfigStore } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";
import { useVendorStore } from "@/stores/use-vendor-store";
import { redeemLicenseKey } from "@/services/api/license";

function copyText(text: string, toast = "已复制") {
    try {
        void navigator.clipboard.writeText(text);
        void message.success(toast);
    } catch {
        void message.warning("当前环境不支持自动复制");
    }
}

type UserStatusActionsProps = {
    showConfig?: boolean;
    variant?: "default" | "canvas";
    onOpenShortcuts?: () => void;
    accountOpen?: boolean;
    onAccountOpenChange?: (open: boolean) => void;
    accountRef?: RefObject<HTMLDivElement | null>;
    getPopupContainer?: (node: HTMLElement) => HTMLElement;
};

export function UserStatusActions({ showConfig = true, variant = "default", onOpenShortcuts, accountOpen, onAccountOpenChange, accountRef, getPopupContainer }: UserStatusActionsProps) {
    const theme = useThemeStore((state) => state.theme);
    const setTheme = useThemeStore((state) => state.setTheme);
    const user = useUserStore((state) => state.user);
    const token = useUserStore((state) => state.token);
    const logout = useUserStore((state) => state.clearSession);
    const updateProfile = useUserStore((state) => state.updateProfile);
    const hydrateUser = useUserStore((state) => state.hydrateUser);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const purchaseURL = useConfigStore((state) => state.purchaseURL);
    const contactSupport = useConfigStore((state) => state.publicSettings?.contactSupport);
    const canvasTheme = canvasThemes[theme];

    // —— 统一外部点击关闭逻辑 ——
    // 非受控模式（普通页面）时使用内部状态；受控模式（画布页面）时使用外部传入的 accountOpen
    const [innerAccountOpen, setInnerAccountOpen] = useState(false);
    const innerAccountRef = useRef<HTMLDivElement>(null);
    const realAccountOpen = accountOpen === undefined ? innerAccountOpen : accountOpen;
    const realAccountRef: RefObject<HTMLDivElement | null> = accountRef ?? innerAccountRef;

    // 内部 onOpenChange：受控/非受控模式都走这里
    const handleAccountOpenChange = useCallback(
        (nextOpen: boolean) => {
            if (onAccountOpenChange) {
                onAccountOpenChange(nextOpen);
            } else {
                setInnerAccountOpen(nextOpen);
            }
        },
        [onAccountOpenChange],
    );

    // 统一外部点击检测：捕获阶段监听 pointerdown，避免被画布的 stopPropagation 吞掉
    useEffect(() => {
        if (!realAccountOpen) return;
        const onPointerDown = (event: PointerEvent) => {
            const target = event.target as Node | null;
            if (!target) return;
            // 1) 点击在头像触发按钮区域内，不关闭
            if (realAccountRef.current?.contains(target)) return;
            // 2) 点击在 Ant Design Dropdown 弹出菜单（Portal 渲染）内，不关闭
            //    antd dropdown 弹出层通常带有 .ant-dropdown / .ant-dropdown-menu 等特征
            let node: Node | null = target;
            while (node instanceof HTMLElement) {
                const cls = node.className;
                if (typeof cls === "string" && (cls.includes("ant-dropdown") || cls.includes("ant-popover") || cls.includes("ant-menu"))) {
                    return;
                }
                node = node.parentNode;
            }
            // 其他情况：关闭菜单
            handleAccountOpenChange(false);
        };
        document.addEventListener("pointerdown", onPointerDown, true);
        return () => document.removeEventListener("pointerdown", onPointerDown, true);
    }, [realAccountOpen, realAccountRef, handleAccountOpenChange]);
    // —— 统一外部点击关闭逻辑 END ——

    const [avatarModalOpen, setAvatarModalOpen] = useState(false);
    const [contactModalOpen, setContactModalOpen] = useState(false);
    const [contactTab] = useState("contact");
    const [tempAvatarUrl, setTempAvatarUrl] = useState("");
    const [displayNameInput, setDisplayNameInput] = useState("");
    const [savingAvatar, setSavingAvatar] = useState(false);
    const [uploading, setUploading] = useState(false);

    const userName = user?.displayName || user?.username || "";
    const activeVendorType = useConfigStore((state) => state.config.activeVendorType);
    const vendorAccounts = useVendorStore((state) => state.accounts);
    const refreshVendorBalance = useVendorStore((state) => state.refreshBalance);
    const refreshVendorModels = useVendorStore((state) => state.refreshVendorModels);
    const vendorLoading = useVendorStore((state) => state.isLoading);
    const activeVendorAccount = activeVendorType === "official"
        ? null
        : vendorAccounts.find((a) => a.vendorType === activeVendorType);
    // 余额：激活的是 vendor 且 vendor 有 balanceText → 显示 vendor 余额；否则显示官方账户余额（¥X.XX）
    const activeBalanceText = activeVendorAccount?.balanceText?.trim();
    const showVendorBalance = Boolean(activeBalanceText);
    const isVendorBalance = activeVendorType !== "official" && Boolean(activeVendorAccount);
    const balanceCents = user?.balanceCents ?? 0;
    const balanceLabel = showVendorBalance ? activeBalanceText! : formatBalanceYuan(balanceCents);
    // vendor 模式下：点击 chip 触发刷新余额（去掉单独的「刷新余额」按钮）
    // 官方模式下：点击 chip 跳 /wallet 看充值说明
    const balanceTooltip = isVendorBalance
        ? `点击刷新 ${activeVendorAccount!.vendorType} 余额（从供应商服务器重新拉取）`
        : "当前账户余额 · 点击查看充值说明";
    const onBalanceChipClick = useCallback(() => {
        if (!isVendorBalance || vendorLoading) return;
        void refreshVendorBalance(activeVendorType);
    }, [isVendorBalance, vendorLoading, refreshVendorBalance, activeVendorType]);
    const avatarUrl = user?.avatarUrl?.trim();
    const avatarText = (userName.trim()[0] || "U").toUpperCase();
    // ✅ 修复：只要 enabled=true 就显示图标；即使联系方式为空，也显示按钮，弹窗提示管理员尚未填写具体信息
    const hasContactInfo = Boolean(contactSupport?.enabled);
    // 是否有实际联系方式（用于弹窗中展示具体的微信号/QQ号等）
    const hasContactDetail = Boolean(
        contactSupport?.wechat || contactSupport?.qq || contactSupport?.qqGroup || contactSupport?.wechatQr || contactSupport?.qqGroupQr,
    );

    const naturalIconClass = "inline-flex size-7 shrink-0 items-center justify-center text-stone-600 transition hover:text-stone-950 dark:text-stone-300 dark:hover:text-white [&_svg]:size-4";
    const iconStyle: CSSProperties | undefined = variant === "canvas" ? { color: canvasTheme.node.text } : undefined;
    const avatarStyle: CSSProperties | undefined = variant === "canvas" ? { borderColor: canvasTheme.toolbar.border, color: canvasTheme.node.text, background: "transparent" } : undefined;
    const defaultTextColorClass =
        variant === "canvas" ? "" : "text-stone-700 dark:text-stone-200";

    const openAvatarEditor = useCallback(() => {
        setTempAvatarUrl(avatarUrl || "");
        setDisplayNameInput(user?.displayName || "");
        setAvatarModalOpen(true);
    }, [avatarUrl, user]);

    const onOpenAvatarChange = useCallback(
        (open: boolean) => {
            setAvatarModalOpen(open);
            if (!open) {
                setTempAvatarUrl(avatarUrl || "");
                setDisplayNameInput(user?.displayName || "");
            }
        },
        [avatarUrl, user],
    );

    const openPurchase = useCallback(() => {
        if (!purchaseURL) {
            void message.warning("管理员尚未配置购买卡密链接");
            return;
        }
        window.open(purchaseURL, "_blank", "noopener,noreferrer");
    }, [purchaseURL]);

    // 兑换卡密
    const [redeemModalOpen, setRedeemModalOpen] = useState(false);
    const [redeemKey, setRedeemKey] = useState("");
    const [redeeming, setRedeeming] = useState(false);
    const handleRedeem = useCallback(async () => {
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
    }, [redeemKey, token, hydrateUser]);

    const uploadProps: UploadProps = useMemo(
        () => ({
            name: "file",
            accept: "image/*",
            showUploadList: false,
            beforeUpload: async (file: File) => {
                if (!user) {
                    void message.warning("请先登录");
                    return Upload.LIST_IGNORE;
                }
                const maxSize = 5 * 1024 * 1024;
                if (file.size > maxSize) {
                    void message.warning("头像图片不能超过 5MB");
                    return Upload.LIST_IGNORE;
                }
                setUploading(true);
                try {
                    const form = new FormData();
                    form.append("file", file);
                    let baseURL = "";
                    try {
                        baseURL = axios.defaults.baseURL ?? "";
                    } catch {
                        baseURL = "";
                    }
                    const full = /^https?:\/\//i.test("/api/v1/media/references")
                        ? "/api/v1/media/references"
                        : (baseURL || "") + "/api/v1/media/references";
                    const res = await axios.post<{ code: number; data: { url?: string; URL?: string }; msg?: string }>(full, form, {
                        withCredentials: true,
                        headers: {
                            "Content-Type": "multipart/form-data",
                            ...(token ? { Authorization: `Bearer ${token}` } : {}),
                        },
                        validateStatus: () => true,
                    });
                    const result = res.data;
                    if (res.status < 200 || res.status >= 300 || (result && result.code !== 0)) {
                        throw new Error(result?.msg || "上传失败");
                    }
                    const uploadedUrl = (result.data?.url || result.data?.URL || "").toString();
                    if (!uploadedUrl) throw new Error("上传返回缺少头像 URL");
                    // 将绝对 URL 转为相对路径，避免因 PublicBaseURL 配置为内网 IP 导致浏览器无法加载
                    let finalUrl = uploadedUrl;
                    try {
                        const u = new URL(uploadedUrl, window.location.origin);
                        if (u.origin !== window.location.origin) {
                            finalUrl = u.pathname + u.search;
                        }
                    } catch {
                        // 非标准 URL 保持原样
                    }
                    setTempAvatarUrl(finalUrl);
                    void message.success("头像上传成功");
                } catch (error) {
                    const msg = error instanceof Error ? error.message : "上传失败";
                    if (msg.includes("PUBLIC_BASE_URL")) {
                        void message.warning("请先在系统设置里配置 PUBLIC_BASE_URL 后再上传；或直接填入一个可访问的头像图片 URL");
                    } else {
                        void message.error(msg);
                    }
                } finally {
                    setUploading(false);
                }
                return Upload.LIST_IGNORE;
            },
            onChange(info: UploadChangeParam<UploadFile>) {
                if (info.file.status === "error") {
                    // no-op, handled in beforeUpload
                }
            },
        }),
        [token, user],
    );

    const saveAvatar = useCallback(async () => {
        try {
            setSavingAvatar(true);
            await updateProfile({
                avatarUrl: tempAvatarUrl.trim(),
                displayName: displayNameInput.trim(),
            });
            setAvatarModalOpen(false);
            void message.success("资料已更新");
        } catch (error) {
            const msg = error instanceof Error ? error.message : "保存失败";
            void message.error(msg);
        } finally {
            setSavingAvatar(false);
        }
    }, [tempAvatarUrl, displayNameInput, updateProfile]);

    const menuItems: ItemType[] = [
        { key: "user", disabled: true, label: <span className="font-medium text-current">{userName}</span> },
        {
            key: "balance",
            icon: <BalanceSymbol className="text-sm leading-none" />,
            label: (
                <div className="flex items-center justify-between gap-4">
                    <span className="text-stone-500 dark:text-stone-400">
                        {showVendorBalance ? `${activeVendorAccount!.vendorType} 余额` : "账户余额"}
                    </span>
                    <span className="font-semibold tabular-nums">{balanceLabel}</span>
                </div>
            ),
        },
        {
            key: "purchase",
            icon: <ShoppingCart className="size-4" />,
            label: (
                <button type="button" className="w-full text-left" onClick={openPurchase} disabled={!purchaseURL}>
                    购买卡密 / 充值
                </button>
            ),
        },
        {
            key: "redeem",
            icon: <Gift className="size-4" />,
            label: (
                <button type="button" className="w-full text-left" onClick={() => setRedeemModalOpen(true)}>
                    兑换卡密
                </button>
            ),
        },
        ...(hasContactInfo
            ? [
                  {
                      key: "support",
                      icon: <Headphones className="size-4" />,
                      label: (
                          <button type="button" className="w-full text-left" onClick={() => { setContactModalOpen(true); }}>
                              联系客服
                          </button>
                      ),
                  } as ItemType,
              ]
            : []),
        {
            key: "balance-page",
            icon: <Wand2 className="size-4" />,
            label: <Link href="/wallet">充值说明</Link>,
        },
        { type: "divider" },
        {
            key: "avatar",
            icon: <UserRoundCog className="size-4" />,
            label: (
                <button type="button" className="w-full text-left" onClick={openAvatarEditor}>
                    更换头像 / 昵称
                </button>
            ),
        },
        ...(user?.role === "admin" ? [{ key: "admin", icon: <Shield className="size-4" />, label: <Link href="/admin">管理后台</Link> } as ItemType] : []),
        ...(onOpenShortcuts ? [{ key: "shortcuts", icon: <Keyboard className="size-4" />, label: "快捷键", onClick: onOpenShortcuts } as ItemType] : []),
        { type: "divider" },
        { key: "logout", icon: <LogOut className="size-4" />, label: "退出登录", onClick: logout },
    ];

    const balanceChipInner = (
        <>
            <BalanceSymbol className={variant === "canvas" ? "text-sm leading-none" : "text-sm leading-none text-amber-500 dark:text-amber-400"} />
            <span>{balanceLabel}</span>
        </>
    );
    const chipClass = variant === "canvas"
        ? "flex h-8 shrink-0 items-center gap-1.5 px-1.5 text-xs font-medium tabular-nums opacity-75 transition hover:opacity-100"
        : `flex h-8 shrink-0 items-center gap-1.5 rounded-full border border-stone-200 bg-stone-50/80 px-2.5 text-xs font-medium tabular-nums transition hover:bg-stone-100 dark:border-stone-700 dark:bg-stone-800/60 dark:hover:bg-stone-800 ${defaultTextColorClass}`;
    const chipStyle = variant === "canvas" ? { color: canvasTheme.node.text } : undefined;
    const balanceChip = isVendorBalance ? (
        <Tooltip title={balanceTooltip} placement="bottom">
            <button
                type="button"
                onClick={onBalanceChipClick}
                disabled={vendorLoading}
                aria-label={balanceTooltip}
                className={chipClass}
                style={chipStyle}
            >
                {balanceChipInner}
            </button>
        </Tooltip>
    ) : (
        <Tooltip title={balanceTooltip} placement="bottom">
            <Link href="/wallet" className={chipClass} style={chipStyle}>
                {balanceChipInner}
            </Link>
        </Tooltip>
    );

    return (
        <>
            <div className="inline-flex shrink-0 items-center gap-2.5">
                {showConfig ? (
                    <button type="button" className={naturalIconClass} style={iconStyle} onClick={() => openConfigDialog(false)} aria-label="配置" title="配置">
                        <Settings2 className="size-4" />
                    </button>
                ) : null}
                <AnimatedThemeToggler theme={theme} onThemeChange={setTheme} className={naturalIconClass} style={iconStyle} aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"} title={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"} />
                {user ? balanceChip : null}
                {user && activeVendorType === "official" && purchaseURL ? (
                    <Tooltip title="去链动小铺购买卡密" placement="bottom">
                        <button
                            type="button"
                            onClick={openPurchase}
                            className={
                                variant === "canvas"
                                    ? "inline-flex h-8 shrink-0 items-center gap-1 px-2 text-xs font-semibold tabular-nums opacity-85 transition hover:opacity-100"
                                    : `inline-flex h-8 shrink-0 items-center gap-1 rounded-full border border-stone-200 bg-white px-2.5 text-xs font-semibold shadow-sm transition hover:-translate-y-px hover:bg-stone-50 dark:border-stone-700 dark:bg-stone-800 dark:hover:bg-stone-700 ${defaultTextColorClass}`
                            }
                            style={variant === "canvas" ? { color: canvasTheme.node.text } : undefined}
                        >
                            <ShoppingCart className="size-3.5" />
                            <span>购买卡密</span>
                        </button>
                    </Tooltip>
                ) : null}
                {user && activeVendorType === "official" ? (
                    <Tooltip title="输入卡密兑换余额" placement="bottom">
                        <button
                            type="button"
                            onClick={() => setRedeemModalOpen(true)}
                            className={
                                variant === "canvas"
                                    ? "inline-flex h-8 shrink-0 items-center gap-1 px-2 text-xs font-semibold tabular-nums opacity-85 transition hover:opacity-100"
                                    : `inline-flex h-8 shrink-0 items-center gap-1 rounded-full border border-stone-200 bg-white px-2.5 text-xs font-semibold shadow-sm transition hover:-translate-y-px hover:bg-stone-50 dark:border-stone-700 dark:bg-stone-800 dark:hover:bg-stone-700 ${defaultTextColorClass}`
                            }
                            style={variant === "canvas" ? { color: canvasTheme.node.text } : undefined}
                        >
                            <Gift className="size-3.5" />
                            <span>兑换卡密</span>
                        </button>
                    </Tooltip>
                ) : null}
                {/* 独立刷新模型按钮：仅 vendor 模式可见。chip 点击触发刷新余额，模型列表单独刷新。 */}
                {user && isVendorBalance ? (
                    <Tooltip title={`刷新 ${activeVendorAccount!.vendorType} 模型列表（重新拉取供应商最新可用模型）`} placement="bottom">
                        <button
                            type="button"
                            onClick={() => void refreshVendorModels(activeVendorType)}
                            disabled={vendorLoading}
                            aria-label={`刷新 ${activeVendorAccount!.vendorType} 模型列表`}
                            className={
                                variant === "canvas"
                                    ? naturalIconClass
                                    : "inline-flex size-8 shrink-0 items-center justify-center rounded-full border border-stone-200 bg-white text-stone-600 shadow-sm transition hover:-translate-y-px hover:text-stone-900 disabled:opacity-50 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-300 dark:hover:text-white [&_svg]:size-4"
                            }
                            style={variant === "canvas" ? iconStyle : undefined}
                        >
                            <RefreshCw className={`size-4 ${vendorLoading ? "animate-spin" : ""}`} />
                        </button>
                    </Tooltip>
                ) : null}
                {user && hasContactInfo ? (
                    <Tooltip title="联系客服" placement="bottom">
                        <button
                            type="button"
                            onClick={() => { setContactModalOpen(true); }}
                            className={
                                variant === "canvas"
                                    ? naturalIconClass
                                    : `inline-flex size-8 shrink-0 items-center justify-center rounded-full border border-stone-200 bg-white text-stone-600 shadow-sm transition hover:-translate-y-px hover:text-stone-900 dark:border-stone-700 dark:bg-stone-800 dark:text-stone-300 dark:hover:text-white [&_svg]:size-4`
                            }
                            aria-label="联系客服"
                        >
                            <Headphones className="size-4" />
                        </button>
                    </Tooltip>
                ) : null}
                {!user && onOpenShortcuts ? (
                    <button type="button" className={naturalIconClass} style={iconStyle} onClick={() => {}} aria-label="快捷键" title="快捷键">
                        <Keyboard className="size-4" />
                    </button>
                ) : null}
                {!user ? (
                    <Link href="/login" className="px-1.5 text-sm font-medium text-stone-600 underline-offset-4 transition hover:text-stone-950 hover:underline dark:text-stone-300 dark:hover:text-stone-100" style={iconStyle}>
                        登录
                    </Link>
                ) : null}
                {user ? (
                    <div ref={realAccountRef}>
                        <Dropdown
                            open={realAccountOpen}
                            onOpenChange={handleAccountOpenChange}
                            trigger={["click"]}
                            placement="bottomRight"
                            getPopupContainer={getPopupContainer}
                            styles={{ root: { minWidth: 220 } }}
                            menu={{ items: menuItems }}
                        >
                            <button type="button" className="flex size-7 shrink-0 items-center justify-center rounded-full bg-transparent p-0 text-[0] leading-[0] transition" aria-label="账户菜单">
                                <Avatar
                                    size={24}
                                    src={avatarUrl ? <img src={avatarUrl} alt={userName} referrerPolicy="no-referrer" /> : undefined}
                                    alt={userName}
                                    className="!flex !items-center !justify-center border border-stone-300 bg-transparent text-[11px] font-semibold text-stone-800 transition hover:border-stone-500 hover:text-stone-950 dark:border-stone-700 dark:text-stone-100 dark:hover:border-stone-400 dark:hover:text-white"
                                    style={avatarStyle}
                                >
                                    {avatarText}
                                </Avatar>
                            </button>
                        </Dropdown>
                    </div>
                ) : null}
            </div>

            <Modal
                title="更换头像 / 昵称"
                open={avatarModalOpen}
                onCancel={() => onOpenAvatarChange(false)}
                onOk={saveAvatar}
                okText="保存"
                cancelText="取消"
                confirmLoading={savingAvatar}
                destroyOnHidden
            >
                <div className="flex flex-col gap-4 py-2">
                    <div className="flex items-center gap-4">
                        <Avatar
                            size={72}
                            src={tempAvatarUrl ? <img src={tempAvatarUrl} alt="头像预览" referrerPolicy="no-referrer" /> : undefined}
                            className="!flex !items-center !justify-center border border-stone-200 bg-stone-50 text-2xl font-semibold dark:border-stone-700 dark:bg-stone-800"
                        >
                            {(displayNameInput.trim() || userName.trim() || "U")[0]?.toUpperCase()}
                        </Avatar>
                        <div className="flex flex-col gap-2">
                            <Upload {...uploadProps}>
                                <Button icon={<UserRoundCog className="size-4" />} loading={uploading}>
                                    上传本地图片
                                </Button>
                            </Upload>
                            <Typography.Text type="secondary" className="text-xs">
                                建议 1:1 正方形，不超过 5MB；也可直接粘贴图片 URL
                            </Typography.Text>
                        </div>
                    </div>
                    <Space direction="vertical" size={12} style={{ width: "100%" }}>
                        <div>
                            <Typography.Text type="secondary" className="mb-1 block text-xs">
                                头像图片 URL
                            </Typography.Text>
                            <Input
                                allowClear
                                placeholder="https:// 开头的图片直链"
                                value={tempAvatarUrl}
                                onChange={(e) => setTempAvatarUrl(e.target.value)}
                            />
                        </div>
                        <div>
                            <Typography.Text type="secondary" className="mb-1 block text-xs">
                                显示昵称（选填）
                            </Typography.Text>
                            <Input
                                allowClear
                                maxLength={50}
                                placeholder="留空则显示用户名"
                                value={displayNameInput}
                                onChange={(e) => setDisplayNameInput(e.target.value)}
                            />
                        </div>
                    </Space>
                </div>
            </Modal>

            {hasContactInfo ? (
                <Modal
                    title={
                        <span className="flex items-center gap-2">
                            <Headphones className="text-blue-500" />
                            <span>联系客服</span>
                        </span>
                    }
                    open={contactModalOpen}
                    onCancel={() => setContactModalOpen(false)}
                    footer={<Button onClick={() => setContactModalOpen(false)}>关闭</Button>}
                    destroyOnHidden
                    styles={{ body: { paddingBottom: 8 } }}
                >
                    <div className="mb-4">
                    </div>
                    {contactTab === "contact" ? (
                        hasContactDetail ? (
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
                        ) : (
                            // ✅ 管理员开启了"联系客服"但尚未填写任何联系方式时，给出友好提示
                            <div className="py-4 text-center">
                                <Headphones className="mx-auto mb-3 size-10 text-stone-300 dark:text-stone-600" />
                                <Typography.Text type="secondary" className="block text-sm">
                                    管理员已开启联系客服，但尚未填写具体联系方式。
                                </Typography.Text>
                                <Typography.Text type="secondary" className="mt-1 block text-xs">
                                    请稍后重试，或通知管理员补充客服信息。
                                </Typography.Text>
                            </div>
                        )
                    ) : null}
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
                    <Input
                        allowClear
                        value={redeemKey}
                        onChange={(e) => setRedeemKey(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === "Enter") void handleRedeem();
                        }}
                        placeholder="输入卡密"
                        disabled={redeeming}
                    />
                </Space>
            </Modal>

        </>
    );
}