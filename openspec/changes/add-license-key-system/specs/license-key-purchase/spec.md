## ADDED Requirements

### Requirement: Login page displays credit recharge entry
The unauthenticated login page SHALL display a clearly visible "购买 Credits 卡密" entry so new users know how to acquire initial balance (since default initial balance is 0).

#### Scenario: Click purchase button on login page
- **WHEN** a visitor (not logged-in) clicks the "购买 Credits 卡密" button rendered on the login page
- **THEN** the frontend SHALL first fetch GET /api/license/purchase-config (public endpoint, no auth needed)
- **THEN** frontend SHALL window.open the returned purchaseURL in a new blank tab "_blank"
- **THEN** current page SHALL remain on login page state (no navigation, no reload)

#### Scenario: New user hint text
- **WHEN** login page first renders
- **THEN** below or beside the login form, there SHALL be a hint panel with text such as:
  "新用户注册后默认 Credits = 0。生成图片按 0.04 Credits / 张扣费，请先购买卡密充值。"
  paired with the purchase button above

### Requirement: Purchase entry everywhere else user needs to recharge
Purchase CTA SHALL appear on any touchpoint where balance shortage blocks usage — account page, low balance badge, balance error toast.

#### Scenario: Credits account page PurchaseCTA (covered also by balance-status)
Already described by credits-balance-status spec; SHALL be consistent with the single source-of-truth purchaseURL from config endpoint.

#### Scenario: Top nav mini recharge icon next to balance badge (optional UX)
The top nav CreditsBadge MAY have a small adjacent "+" or wallet/credit-card icon that directly opens purchase URL in a new tab (bypassing account page) as a shortcut. If implemented, it SHALL also use the same `/api/license/purchase-config` data.
