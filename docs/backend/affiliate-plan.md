# Freedom 推广码 + 一级返佣 技术方案

> 目标：通过「推广码」让老用户邀请新用户，新用户在你运营的官方托管版（SaaS）充值后，老用户按配置比例拿到佣金。
> 设计原则：**只做一级直推返佣**，避免多级分销的合规风险；开源自部署版本不产生分成（充值的钱不经过你）。
>
> **状态：核心代码已落地（2026-08-23）**。详见下方「六、已实现清单」。

---

## 一、现有基础（已具备，无需新建）

`users` 表已存在三个字段，说明邀请体系曾经起步：

| 字段 | 类型 | 现状 |
| --- | --- | --- |
| `aff_code` | string, uniqueIndex | 注册时自动生成（`service/auth.go:107` 调 `newAffCode()`）✅ |
| `aff_count` | int | 已邀请人数冗余统计，暂无人维护 ⚠️ |
| `inviter_id` | string | **注册/OAuth 路径均未写入** ❌（关系未落地） |

余额与流水体系已就绪：
- `users.balance_cents`：账户余额（分）
- `balance_logs`：余额流水，已有 `manual_recharge`（人工补发充值）等类型
- `modelCosts.costCents`：模型扣费配置（充值消费的出口）

**结论**：缺的是两件事 —— ①注册时把邀请人写进 `inviter_id`；②充值成功后给邀请人结算佣金（新表 + 逻辑）。

---

## 二、需要新增的内容

### 2.1 注册时落地邀请关系（改 `service/auth.go`）

在 `Register` 和 `LoginWithLinuxDo`（OAuth 注册分支）里，增加 `inviterCode string` 参数：

```go
// 伪代码
func Register(username, password, inviterCode string) (model.AuthSession, error) {
    // ... 现有校验 ...
    inviterID := ""
    if inviterCode != "" {
        if inv, ok, _ := repository.GetUserByAffCode(inviterCode); ok && inv.ID != "" {
            inviterID = inv.ID          // 写入邀请人
            // 同时 inv.AffCount++ 并保存
        }
    }
    user := model.User{
        // ... 现有字段 ...
        InviterID: inviterID,
    }
    // ...
}
```

- 入口：`POST /api/v1/auth/register` 的 body 增加可选 `inviterCode`；前端注册页/邀请落地页带 `?code=XXX` 自动填入。
- 防自邀、防循环：不允许 `inviterID == 自己`；`inviter_id` 只在注册时写一次，之后不可改。
- **只取一级**：佣金只结算给直接邀请人（`inviter_id`），不向上追溯二级。

### 2.2 新增佣金流水表 `aff_commission_logs`

```sql
CREATE TABLE aff_commission_logs (
  id            VARCHAR(64) PRIMARY KEY,
  inviter_id    VARCHAR(64) NOT NULL,        -- 拿佣金的邀请人
  invitee_id    VARCHAR(64) NOT NULL,        -- 充值的新用户
  recharge_id   VARCHAR(64) NOT NULL,        -- 关联 balance_logs.id（manual_recharge / 在线充值）
  recharge_cents INT NOT NULL,               -- 本次充值金额（分）
  rate          DECIMAL(5,4) NOT NULL,       -- 本次分成比例，如 0.1000
  commission_cents INT NOT NULL,             -- 实发佣金（分）= recharge_cents * rate
  status        VARCHAR(16) NOT NULL,        -- pending / settled / cancelled
  settled_at     VARCHAR(32),                -- 结算时间（打款/入账到余额）
  created_at    VARCHAR(32) NOT NULL
);
-- 索引：邀请人查账单、充值单幂等（同一 recharge 不重复结算）
CREATE INDEX idx_aff_inviter ON aff_commission_logs(inviter_id, created_at);
CREATE UNIQUE INDEX idx_aff_recharge ON aff_commission_logs(recharge_id);
```

> 字段类型沿用现有 `string` 时间风格（`users`/`balance_logs` 都是 string），与项目统一。

### 2.3 分成比例配置（放 `settings.public` 或新增 `settings.private.affiliate`）

```json
{
  "affiliate": {
    "enabled": true,
    "rechargeCommissionRate": 0.10,   // 充值金额的 10% 作为邀请人佣金
    "minSettleCents": 100,            // 最低结算阈值（分），低于则累计
    "settleMode": "auto"              // auto=充值后实时入账邀请人余额；manual=后台审核后发放
  }
}
```

- 推荐 `settleMode: "auto"` + `minSettleCents` 阈值，简单且即时反馈，利于传播。
- 比例写在 `private`（不暴露给前端），避免被刷比例。

### 2.4 结算逻辑（核心，放 `service/affiliate.go`）

触发点：**任意一笔充值成功写入 `balance_logs`（type=manual_recharge 或新增 `online_recharge`）之后**。

```go
// SettleCommissionOnRecharge(ctx, recharge BalanceLog)
func SettleCommissionOnRecharge(recharge model.BalanceLog) error {
    invitee, _, _ := repository.GetUserByID(recharge.UserID)
    if invitee.InviterID == "" { return nil }          // 无邀请人，跳过
    inviter, _, _ := repository.GetUserByID(invitee.InviterID)
    if inviter.Role != "user" && inviter.Role != "admin" { return nil }

    rate := getAffiliateRate()                           // 读 settings
    commission := int(math.Round(float64(recharge.Amount) * rate))
    if commission <= 0 { return nil }

    // 幂等：同一 recharge_id 不重复结算（UNIQUE 索引兜底）
    log := model.AffCommissionLog{
        ID: newID("aff"), InviterID: inviter.ID, InviteeID: invitee.ID,
        RechargeID: recharge.ID, RechargeCents: recharge.Amount,
        Rate: rate, CommissionCents: commission, Status: "pending",
    }
    if err := repository.SaveAffCommissionLog(log); err != nil { return err }

    // 入账到邀请人余额
    if err := repository.AddBalance(inviter.ID, commission,
        model.BalanceLogTypeAffCommission, "邀请返佣"); err != nil { return err }
    repository.UpdateAffCommissionLogStatus(log.ID, "settled", now())
    return nil
}
```

- `AddBalance` 复用现有余额变更通道（与 `generation_consume` 同机制），保证 `balance_cents` + `balance_logs` 一致。
- 需新增一个 `BalanceLogType`：`aff_commission`（邀请返佣入账，正数）。

### 2.5 前端展示（最小改动）

- 用户中心「我的邀请」页：展示自己的 `aff_code` + 复制邀请链接 `https://你的域名/register?code=XXX`。
- 邀请数据：已邀人数（`aff_count`）、累计佣金、待结算佣金（读 `aff_commission_logs` 聚合）。
- 后台「用户」页：可按 `inviter_id` 筛选，查看某邀请人的下级与佣金汇总。

---

## 三、流程图

```
新用户点邀请链接 ?code=ABC
        │
        ▼
注册 / OAuth 注册
        │  写入 inviter_id = 邀请人ID，邀请人 aff_count++
        ▼
新用户在官方托管版充值 ¥X
        │  写入 balance_logs(type=manual_recharge, amount=充值分)
        ▼
触发 SettleCommissionOnRecharge
        │  读 settings.affiliate.rechargeCommissionRate
        │  佣金 = 充值分 × rate（幂等：UNIQUE(recharge_id)）
        ▼
邀请人余额 +佣金分（balance_logs type=aff_commission）
        │
        ▼
aff_commission_logs 记一笔 settled
```

---

## 四、合规要点（务必遵守）

1. **一级直推**：佣金只给直接邀请人，不做二级/多级。界面与文案不出现「下线」「团队」「无限代」等词。
2. **仅官方托管版生效**：自部署（fork）用户充值不经过你，不结算。可在文档注明「返佣仅限官方托管实例」。
3. **真实消费锚定**：佣金基于「真实充值」，不是拉新即奖，避免被认定为拉人头。
4. **可提现/可消费**：佣金进余额，可用于生图消费；若要提现需配套提现审核与税务说明（建议初期仅限消费，降低合规面）。
5. **防刷**：同一邀请人短时间内大量新注册且立即充值，后台加风控（如单日佣金上限、新用户冷静期）。

---

## 五、落地步骤（建议顺序）

1. `model/` 新增 `AffCommissionLog` 结构体；`model/user.go` 的 `BalanceLogType` 加 `aff_commission`。
2. `repository/` 新增 `SaveAffCommissionLog` / `GetUserByAffCode` / `AddBalance`（或复用现有）/ `UpdateAffCommissionLogStatus`。
3. `service/auth.go` 注册与 OAuth 注册分支写入 `inviter_id` + `aff_count++`。
4. `service/affiliate.go` 实现 `SettleCommissionOnRecharge`，并在充值成功后调用。
5. `settings` 增加 `affiliate` 配置组 + 后台开关。
6. 前端「我的邀请」页 + 注册页 `inviterCode` 入参。
7. `backend-database.md` 补 `aff_commission_logs` 表说明。

> 注：本方案为纯设计，未改动任何现有代码。实施前请确认官方托管版的充值链路（当前 `manual_recharge` 是后台人工补发，需先有用户自助在线充值入口，或把人工补发也视为可结算的充值来源）。

---

## 六、已实现清单（2026-08-23 落地）

| 改动文件 | 内容 |
| --- | --- |
| `model/user.go` | 新增 `BalanceLogTypeAffCommission`（邀请返佣入账）；新增 `AffCommissionLog` 结构体（含 `status` pending/settled/cancelled） |
| `model/setting.go` | `PrivateSetting` 挂 `Affiliate AffiliateSetting`；`AffiliateSetting{Enabled, BaseRate, StepRate, MaxRate, MinSettleCents}` |
| `repository/user.go` | 新增 `GetUserByAffCode`、`SaveAffCommissionLog`、`GetAffCommissionLogByRecharge`（幂等）、`SumAffCommissionByInviter`、`SumAffCommissionPendingByInviter`、`ListPendingAffCommissionInviterIDs`、`ListPendingAffCommissionLogsByInviter(Tx)`、`SettlePendingAffCommissionsByInviterTx` |
| `repository/db.go` | AutoMigrate 增加 `&model.AffCommissionLog{}` |
| `service/affiliate.go`（新） | `SettleCommissionOnConsume`：被邀请人消费后写 `pending` 返佣流水（阶梯比例 + 幂等 + 阈值 + 防自邀），**不实时入账**；`MyAffiliateInfo` 返回已结算/待结算双口径 |
| `service/affiliate_settlement_scheduler.go`（新） | `StartAffiliateSettlementScheduler`：每日 00:10 批结算所有 pending 佣金，按邀请人聚合入账并标记 settled；`RunAffiliateSettlementBatch` 供手动触发 |
| `main.go` | 启动期调用 `service.StartAffiliateSettlementScheduler()` |
| `service/auth.go` | `Register` / `LoginWithLinuxDo` 注册分支写入 `InviterID` + 邀请人 `AffCount++` |
| `handler/auth.go` | `registerRequest` 增加 `inviterCode` 字段并透传 |

**待办（未做，按需）**：
- 自助在线充值入口（当前返佣锚定「卡密兑换」，若要做在线支付需另接支付渠道）
- 防刷风控（单日佣金上限、新用户冷静期）

**已完成（2026-08-23 第二批）**：
- 前端「我的邀请」页：`app/(user)/wallet/page.tsx` 新增「我的邀请」Tab（邀请码、复制邀请链接、已邀人数、累计返佣、返佣流水表）
- 后端接口：`GET /api/v1/affiliate/info`、`GET /api/v1/affiliate/commissions`
- 后台配置 UI：`admin/settings` 私有配置新增「邀请返佣」Card（开关、比例、最低阈值），经 `AdminSaveSettings` 写入 `settings.private.affiliate`
- 类型与编译：`go build ./...` 通过；前端 `tsc --noEmit` 仅余 2 个与本次无关的预存错误（balance-logs 页）

---

## 七、生产部署域名与 CORS（2026-08-23 补）

部署到 `https://xiaoyxiao.xyz` 等公网域名时：

1. **邀请链接自动用真实域名**：钱包页「我的邀请」用 `window.location.origin` 动态拼 `/register?inviterCode=XXX`，部署后自动为站点域名，不会写死 localhost。前提是用户经域名（nginx 反代）访问。
2. **CORS 必须放开**：后端 `router.go` 默认仅放行 `localhost`。服务器 `.env` 需设以下任一变量把生产域名加白名单，否则前端所有接口（含邀请/充值）被浏览器跨域拦截：
   ```text
   PUBLIC_BASE_URL=https://xiaoyxiao.xyz
   # 或
   CORS_ALLOWED_ORIGINS=https://xiaoyxiao.xyz
   ```
3. 详见 `docs/overview/docker.md`「生产部署域名与 CORS 检查」一节。

---

## 八、返佣逻辑变更：按消费额阶梯返佣（2026-08-23 修正）

**重要修正**：最初实现按「充值额（卡密兑换）固定比例」返佣，与产品预期不符。实际规则是**按被邀请人的消费额阶梯返佣**：

- 触发点：被邀请人每次模型消费扣费时（`ConsumeUserBalanceWithHold` 成功后），而非充值（兑换卡密）时。
- 比例：按**邀请人当前邀请人数**阶梯计算，封顶 10%：
  - 1人=5%, 2人=6%, 3人=7%, 4人=8%, 5人=9%, **6人及以上=10%（封顶）**
  - 公式：`rate = min(baseRate + (affCount-1)*stepRate, maxRate)`
- 实现：`service/affiliate.go` 的 `SettleCommissionOnConsume`（替代原 `SettleCommissionOnRecharge`）；幂等键用消费占用 `hold.ID`；平台让利，额外给邀请人余额，不从被邀请人扣。
- 配置：`model/setting.go` 的 `AffiliateSetting` 改为 `enabled/baseRate/stepRate/maxRate/minSettleCents`（默认 0.05/0.01/0.10/1）；后台设置 UI 与前端类型同步更新。
- 前端「我的邀请」页额外展示：当前等级比例 + 再邀 1 人升至下一比例（或已达封顶）。

## 九、结算时机变更：T+1 每日批结算（2026-08-23 修正）

**重要修正**：返佣从「消费后实时入账」改为「**每日定时批结算（T+1 日结）**」。

- 消费时：`SettleCommissionOnConsume` 只写入 `status=pending` 的 `aff_commission_logs`，**不直接给邀请人加余额**。
- 每日批结算：新增 `service/affiliate_settlement_scheduler.go` 的 `StartAffiliateSettlementScheduler()`，cron `10 0 * * *`（部署时区下即每日 00:10）触发 `runAffiliateSettlement`：
  - 扫描所有 `status=pending` 的佣金，按 `inviter_id` 去重分组；
  - 每位邀请人在一个事务内聚合佣金 → 余额入账（`RefundUserBalanceTx`，平台让利）→ 写 `balance_logs`（type=`aff_commission`）→ 批量标记这些日志 `settled`；
  - 事务内二次查询 pending 防并发重复入账；单人失败不阻断其他人（记日志继续）；进程重启后立即补跑一次（`runAffiliateSettlement`）。
- 新增仓库方法：`ListPendingAffCommissionInviterIDs`、`ListPendingAffCommissionLogsByInviter(Tx)`、`SettlePendingAffCommissionsByInviterTx`、`SumAffCommissionPendingByInviter`。
- `MyAffiliateInfo` 现在同时返回 `pendingCommissionCents/pendingCommissionCount`（待结算）与 `totalCommissionCents/commissionCount`（已结算），前端「我的邀请」页用琥珀色卡片展示「待结算返佣（每日 00:10 入账）」。
- 日结好处：降低消费抖动、避免中途退款产生返佣争议、批量入账减少 DB 写放大。
