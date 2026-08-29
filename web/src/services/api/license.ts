import axios from "axios";
import { apiGet, apiPost, apiDelete, compactApiParams } from "@/services/api/request";

export type LicensePurchaseConfig = {
    purchaseURL: string;
};

// 兑换结果：返回到账金额和最新余额
export type AdminRedeemKeyResult = {
    faceValueCentsGranted: number;
    newBalanceCents: number;
};

export type RedeemLogItem = {
    id: string;
    licenseKeyId: string;
    keyMasked: string;
    userId: string;
    userName: string;
    faceValueCents: number;
    createdAt: string;
};

export type RedeemLogListResponse = {
    items: RedeemLogItem[];
    total: number;
};

export type LicenseKeyItem = {
    id: string;
    key: string;
    faceValueCents: number;
    status: "unused" | "used";
    usedBy: string;
    usedAt: string;
    batchName: string;
    createdBy: string;
    createdAt: string;
    updatedAt: string;
};

export type LicenseKeyListResponse = {
    items: LicenseKeyItem[];
    total: number;
};

export type ImportLicenseKeysResult = {
    totalLines: number;
    importedCount: number;
    duplicateCount: number;
    malformedCount: number;
    malformedSamples: string[];
};

export type UserRedeemLogQuery = {
    page?: number;
    pageSize?: number;
};

export type AdminLicenseKeyQuery = {
    page?: number;
    pageSize?: number;
    status?: string;
    batchName?: string;
    keyword?: string;
};

export type AdminRedeemLogQuery = {
    page?: number;
    pageSize?: number;
    userKeyword?: string;
};

export async function getPurchaseConfig() {
    return apiGet<LicensePurchaseConfig>("/api/license/purchase-config");
}

export async function getMyRedeemLogs(token: string, query: UserRedeemLogQuery = {}) {
    return apiGet<RedeemLogListResponse>("/api/v1/license/redeem-logs", compactApiParams(query), token);
}

export async function redeemLicenseKey(token: string, key: string) {
    return apiPost<AdminRedeemKeyResult>("/api/v1/license/redeem", { key }, token);
}

export type BalanceLogItem = {
    id: string;
    userId: string;
    type: "manual_adjust" | "generation_consume" | "generation_refund" | "manual_recharge" | (string & {});
    amount: number;   // 单位：分（cents）
    balance: number;  // 单位：分（cents）
    relatedId: string;
    remark: string;
    extra: string;
    createdAt: string;
};

export type BalanceLogListResponse = {
    items: BalanceLogItem[];
    total: number;
};

export async function getMyBalanceLogs(token: string, query: UserRedeemLogQuery = {}) {
    return apiGet<BalanceLogListResponse>("/api/v1/balance-logs", compactApiParams(query), token);
}

export type MyAffiliateInfo = {
    affCode: string;
    inviterId: string;
    affCount: number;
    totalCommissionCents: number;
    commissionCount: number;
    pendingCommissionCents: number;
    pendingCommissionCount: number;
    currentRate: number;
    nextRate: number;
};

export type AffCommissionItem = {
    id: string;
    inviterId: string;
    inviteeId: string;
    rechargeId: string;
    rechargeCents: number;
    rate: string;
    commissionCents: number;
    status: string;
    settledAt: string;
    createdAt: string;
};

export type AffCommissionListResponse = {
    items: AffCommissionItem[];
    total: number;
};

export async function getMyAffiliateInfo(token: string) {
    return apiGet<MyAffiliateInfo>("/api/v1/affiliate/info", {}, token);
}

export async function getMyAffiliateCommissions(token: string, query: UserRedeemLogQuery = {}) {
    return apiGet<AffCommissionListResponse>("/api/v1/affiliate/commissions", compactApiParams(query), token);
}

export async function adminImportLicenseKeys(
    token: string,
    payload: { file: File; batchName: string; faceValueCents: number },
) {
    const form = new FormData();
    form.append("file", payload.file);
    form.append("batchName", payload.batchName);
    form.append("faceValueCents", String(payload.faceValueCents));

    const baseURL = axios.defaults.baseURL ?? "";
    const full = /^https?:\/\//i.test("/api/admin/license-keys/import")
        ? "/api/admin/license-keys/import"
        : (baseURL || "") + "/api/admin/license-keys/import";

    const res = await axios.post(full, form, {
        headers: {
            "Content-Type": "multipart/form-data",
            Authorization: `Bearer ${token}`,
        },
        validateStatus: () => true,
    });
    const result = res.data as { code: number; data: ImportLicenseKeysResult; msg: string };
    if (res.status < 200 || res.status >= 300 || (result && result.code !== 0)) {
        throw new Error(result?.msg || "导入失败");
    }
    return result.data;
}

export async function adminListLicenseKeys(token: string, query: AdminLicenseKeyQuery = {}) {
    return apiGet<LicenseKeyListResponse>("/api/admin/license-keys", compactApiParams(query), token);
}

export async function adminListRedeemLogs(token: string, query: AdminRedeemLogQuery = {}) {
    return apiGet<RedeemLogListResponse>("/api/admin/license-redeem-logs", compactApiParams(query), token);
}

export async function adminModifyBatchFaceValue(
    token: string,
    body: { batchName: string; faceValueCents: number },
) {
    return apiPost<{ rowsAffected: number }>(
        "/api/admin/license-keys/batch-face-value",
        body,
        token,
    );
}

export type GenerateLicenseKeysResult = {
    generatedCount: number;
    batchName: string;
    filePath: string;
};

// 管理员自动生成卡密（系统 mint 随机 key，入库 + 落盘 TXT）。
export async function adminGenerateLicenseKeys(
    token: string,
    body: { batchName: string; faceValueCents: number; count: number },
) {
    return apiPost<GenerateLicenseKeysResult>(
        "/api/admin/license-keys/generate",
        body,
        token,
    );
}

// 管理员导出某批次卡密为 TXT（blob 下载）。
export async function adminExportLicenseKeysBlob(token: string, batchName: string) {
    const baseURL = axios.defaults.baseURL ?? "";
    const url =
        (baseURL || "") +
        "/api/admin/license-keys/export?batchName=" +
        encodeURIComponent(batchName);
    const res = await axios.get(url, {
        headers: { Authorization: `Bearer ${token}` },
        responseType: "blob",
        validateStatus: () => true,
    });
    if (res.status < 200 || res.status >= 300) {
        throw new Error("导出失败");
    }
    return res.data as Blob;
}
