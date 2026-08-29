/**
 * P1 b8：vendor 错误分级 —— 把 raw HTTP 错误码识别为 actionable 提示 + 操作建议
 * 之前 antMessage.error 直接吐 raw JSON，用户看到「submit HTTP 400 MODEL_NOT_AVAILABLE」不知所措。
 * 现在解析后给出：(1) 一句话原因 (2) 可执行的下一步
 */
export type VendorErrorLevel = "warn" | "error" | "info";

export function parseVendorError(rawMessage: string, scope: "image" | "video" | "text"): {
    level: VendorErrorLevel;
    icon: string;
    title: string;
    detail: string;
    action?: string;
} {
    const msg = rawMessage || "未知错误";
    // 1. 模型下架（之前用户栽过的坑）
    if (/MODEL_NOT_AVAILABLE|模型.*已下架|model.*not.*available/i.test(msg)) {
        return {
            level: "error", icon: "🚫",
            title: "视频/图片模型已下架",
            detail: msg,
            action: "切到 footer「视频/图片模型」下拉换个模型，或刷新页面",
        };
    }
    // 2. 鉴权/未登录
    if (/401|UNAUTHORIZED|未登录|invalid.*api.*key|cookie.*expired/i.test(msg)) {
        return {
            level: "error", icon: "🔐",
            title: "鉴权失败（Cookie/Key 过期）",
            detail: msg,
            action: scope === "video" ? "去「配置 → 供应商」重新粘贴 Cookie" : "重新登录或刷新页面",
        };
    }
    // 3. 余额不足
    if (/INSUFFICIENT|余额不足|insufficient.*balance|points.*not.*enough/i.test(msg)) {
        return {
            level: "error", icon: "💰",
            title: "余额不足",
            detail: msg,
            action: "去供应商后台充值",
        };
    }
    // 4. 限流/超时
    if (/429|rate.*limit|TOO_MANY|timed?\s*out|timeout|ETIMEDOUT|ECONNRESET/i.test(msg)) {
        return {
            level: "warn", icon: "⏳",
            title: "网络/限流/超时",
            detail: msg,
            action: "等几秒重试，或切到其他供应商",
        };
    }
    // 5. 内容审核
    if (/审核|内容.*风险|content.*policy|moderat/i.test(msg)) {
        return {
            level: "warn", icon: "🛡",
            title: "内容被审核拒绝",
            detail: msg,
            action: "改写分镜剧本避免敏感词，或切其他供应商",
        };
    }
    // 6. 默认：原样
    const scopeLabel = scope === "video" ? "视频" : scope === "image" ? "图片" : "文本";
    return {
        level: "error", icon: "❌",
        title: `${scopeLabel}生成失败`,
        detail: msg,
    };
}
