## Why

当前 freedom 用户 credits 算力点计费机制已完整实现（图片生成按模型价格×数量预扣 credits，失败自动退款，流水记入 CreditLog）。但用户注册后 credits=0 **不赠送初始额度**，无法直接使用，需要一套**预付费 credits 充值卡密**体系完成商业闭环：

- 通过链动小铺 `https://pay.ldxp.cn/shop/35TCHF9A` 售卡（20元=20 credits、30元=30 credits 等多档位）
- 买家付款→ldxp自动发卡密→买家在 freedom 兑换卡密→credits 余额累加→生成图片扣余额

## What Changes

- **新增卡密兑换功能**：登录用户输入卡密→累加对应面额 credits 到账户余额→写入 CreditLog（类型 license_redeem）+ 兑换流水
- **新增卡密数据模型**：`LicenseKey`（key 唯一索引、credits面额、status、usedBy、usedAt、batchName、createdBy、createdAt）、`LicenseRedeemLog`（兑换记录）
- **新增"购买卡密"入口**：登录页、顶栏 Credits 余额旁、专用兑换页面三处按钮跳转 `https://pay.ldxp.cn/shop/35TCHF9A`
- **新增 Credits 余额突出展示**：顶栏大字显示当前 Credits 余额，余额≤5 醒目提醒充值
- **新增管理员卡密导入与查询**：管理员后台批量导入 TXT 卡密（一行一个，与ldxp上传同一份），统一设定 credits 面额；支持按批次、状态、关键词筛选查询卡密列表、全量兑换记录
- **无到期时间、无商用版身份**：本系统纯 Credits 预付费余额模式，不区分免费/商用身份、不设置任何授权过期时间

## Capabilities

### New Capabilities

- `license-key-redeem`: 用户端卡密兑换能力——输入卡密（兼容带/不带连字符）、格式校验、唯一性/状态校验、事务原子累加 Credits 余额 + 写入兑换流水 + 写入 CreditLog 充值入账
- `license-key-purchase`: 卡密购买入口能力——登录页、顶栏、兑换页面三处跳转链接 `https://pay.ldxp.cn/shop/35TCHF9A` 新标签打开；余额不足/为零时主动弹出充值引导
- `license-key-admin`: 管理员卡密管理能力——TXT 批量导入（解析、格式规范化、去重入库）、卡密列表分页查询（按状态/批次名/关键词筛选）、批次 unused 状态下整批修正面额、全量兑换记录分页查询
- `credits-balance-status`: Credits 余额展示能力——顶栏突出显示余额数、低余额高亮提醒、点击余额跳转充值/兑换页面、兑换页面显示"当前余额+累计充值+明细Tab"

### Modified Capabilities

- `user-auth`: 用户注册默认 Credits=0 不赠送初始额度（与现有 CreateUser 默认值对齐，无需行为变更）；CurrentUser / AuthSession 继续返回 credits 字段用于余额展示，**不需要**新增身份或时间字段

## Impact

**关键约束（不变的部分）**：
- 现有 AI 生成按模型价格扣 Credits、失败退款、CreditLog 消费流水写入 = **已完整实现，本项目不改**
- 扣减入口位置：`handler/ai.go` 181行 `ConsumeUserCredits`、`repository/user.go` 的 `ConsumeUserCredits/RefundUserCredits/SaveCreditLog` = **沿用不碰**

**后端新增文件**：
- `model/license_key.go`：LicenseKey（无 validDays）、LicenseRedeemLog（无 validDays）、LicenseKeyStatus 枚举（unused/used）
- `repository/license_key.go`：批量 INSERT、按 key 查询（支持行锁）、更新 used 状态、卡密列表筛选、兑换流水记录、批量面额修改
- `service/license_key.go`：NormalizeLicenseKey、MaskLicenseKey、ImportLicenseKeys（TXT解析+面额设定+去重）、RedeemLicenseKey（事务原子：校验→key置used→Credits+= →写兑换Log→写CreditLog）、列表查询、GetPurchaseConfig、批次面额修正
- `handler/license_key.go`：LicensePurchaseConfig（公共）、RedeemLicenseKey（登录）、MyRedeemLogs（登录）
- `handler/admin_license_key.go`：AdminImportLicenseKeys（multipart TXT）、AdminListLicenseKeys、AdminListRedeemLogs、AdminModifyBatchUnusedCredits
- `router/router.go` 追加路由注册

**后端修改文件**：
- `config/config.go` + `.env.example`：增加 `LicensePurchaseURL` 字段，默认值 `https://pay.ldxp.cn/shop/35TCHF9A`
- `repository/db.go`：AutoMigrate 追加 LicenseKey、LicenseRedeemLog
- `model/user.go`：**不需要**增加 IsCommercial/LicenseExpireAt 字段（纯余额不使用），保持现状即可

**前端新增文件**：
- `web/src/services/api/license.ts`：卡密相关 API 封装
- `web/src/app/(user)/credits/page.tsx`：Credits 账户页面（余额卡→购买CTA→兑换表单→充值记录Tab→消费记录Tab）
- `web/src/app/(admin)/admin/license-keys/page.tsx`：管理员卡密管理页（导入表单+卡密列表Tab+兑换记录Tab）

**前端修改文件**：
- `web/src/app/login/page.tsx`：登录表单下增加【购买 Credits 卡密】按钮
- `web/src/components/layout/app-top-nav.tsx`：用户区突出显示 Credits 余额数字（大字），余额≤5时橙色高亮；点击余额区域跳转 /credits；用户菜单增加"Credits充值/兑换"菜单项
- `web/src/stores/use-user-store.ts`：无需新增字段，已有 credits 字段直接用于余额展示；兑换成功后本地 optimistic 更新 credits += granted
