## ADDED Requirements

### Requirement: Top navigation displays user Credits balance prominently
The application top navigation bar SHALL display the current logged-in user's Credits balance as a visible interactive badge.

#### Scenario: Normal balance (> 5 credits)
- **WHEN** a logged-in user has Credits balance = 23.56 (>5)
- **THEN** top nav user area SHALL render a clearly visible "CreditsBadge" component displaying formatted number "23.56"
  with neutral or theme-default color
- **THEN** hovering the badge SHALL show a tooltip: "当前余额约可生成 {Math.floor(balance / 0.04)} 张图片（按图片模型参考价0.04/张估算）"
- **THEN** clicking anywhere on the badge SHALL navigate the user to `/credits` account page

#### Scenario: Low balance (between 0 exclusive and 5 inclusive)
- **WHEN** user has balance = 3.48 (0 < balance ≤ 5)
- **THEN** CreditsBadge number SHALL render with warning/orange color background / ring highlight
- **THEN** tooltip SHALL additionally include hint "余额偏低，建议及时充值"
- **THEN** clicking SHALL still navigate to `/credits`

#### Scenario: Zero balance (brand new / run out)
- **WHEN** user has balance = 0.00
- **THEN** CreditsBadge SHALL render with danger/red color and a subtle attention pulse animation
- **THEN** tooltip SHALL include strong hint "余额为0，充值后即可使用AI功能"
- **THEN** clicking SHALL navigate to `/credits` and optionally auto-focus the purchase CTA section

### Requirement: Credits account page shows balance + recharge + records
The route `/credits` SHALL be available to logged-in users and display the full recharge & history experience.

#### Scenario: Initial page layout sections in order
- **WHEN** user navigates to `/credits`
- **THEN** page SHALL render 4 sections top-to-bottom:
  1. **BalanceCard** — 大号 Credits 余额显示，附"估算可用张数"subtext；若余额≤5则内联显示"立即充值"小按钮
  2. **PurchaseCTA** — 标题"获取 Credits 充值卡密" + 简述"购买链接跳转到官方发卡平台链动小铺，付款后自动发货卡密" + 大按钮【前往购买卡密】→ onClick: GET /api/license/purchase-config then window.open(purchaseURL, '_blank') + 附小字提示"⚠️ 收到卡密后复制 → 到下方【兑换卡密】填入即可到账"
  3. **RedeemForm** — Input field placeholder="请输入卡密 XXXX-XXXX-XXXX-XXXX（支持粘贴16位纯字母数字）"+ 按钮【立即兑换】，兑换成功 message.success 返回 `creditsGranted` 和新余额，失败返回中文错误；提交期间按钮 loading 禁用重复点击
  4. **HistoryTabs** — Tab 1: "充值记录（卡密兑换）" → table from GET /api/v1/license/redeem-logs 列：卡密掩码|到账Credits|兑换时间，分页；Tab 2: "消费明细" → table from existing GET /api/v1/credit-logs 复用现有接口，列：时间|类型|变动Credits|关联信息

### Requirement: Insufficient-balance failure triggers recharge prompt
When an AI generation request (image/video/audio/chat) fails due to insufficient Credits balance, the frontend error handling SHALL surface a recharge action.

#### Scenario: Insufficient-balance error on canvas/workbench
- **WHEN** user submits a generation request and backend returns error whose message matches "Credits 余额不足" or code equivalent
- **THEN** frontend error toast/notification SHALL not only show the error text but also INCLUDE an action button "立即充值"
- **THEN** clicking the recharge action SHALL open the purchase URL in a new tab (via GET /api/license/purchase-config)
- **THEN** an additional link "查看账户 →" SHALL navigate to `/credits` page

### Requirement: Purchase link retrieved from backend config (no hardcode)
Purchase URL exposed for frontend SHALL always match config.LicensePurchaseURL (via backend endpoint) and defaults to ldxp shop link.

#### Scenario: Frontend retrieves purchase config publicly (no login required)
- **WHEN** frontend or visitor calls GET /api/license/purchase-config
- **THEN** endpoint SHALL return 200 {purchaseURL: string}
- **THEN** purchaseURL SHALL default to "https://pay.ldxp.cn/shop/35TCHF9A" if LICENSE_PURCHASE_URL env not set
- **THEN** if env var LICENSE_PURCHASE_URL is set, endpoint SHALL return the configured value
