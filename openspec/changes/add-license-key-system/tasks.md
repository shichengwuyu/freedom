## 1. 后端配置 & 数据模型（增量：新表2张 + User表零改动）

- [x] 1.1config/config.go：Config 结构体新增 `LicensePurchaseURL string`（tag `env:"LICENSE_PURCHASE_URL" envDefault:"https://pay.ldxp.cn/shop/35TCHF9A"`）；在 InitDefaults 里不需要额外处理，caarlos0/env 自动处理。同步更新根目录 .env.example：新增一行注释 `# LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A`（默认已内联到envDefault，想改时取消注释即可）
- [x] 1.2model/license_key.go（新建）：
  - 定义 `LicenseKeyStatus = "unused" / "used"` 常量（不新增 revoked，纯余额模式不用撤销）
  - 定义 `LicenseKey`：`ID string`, `Key string gorm:"uniqueIndex"`（canonical XXXX-XXXX-XXXX-XXXX），`Credits int`（面额），`Status LicenseKeyStatus`，`UsedBy string`，`UsedAt string`（RFC3339，空="未使用"），`BatchName string gorm:"index"`，`CreatedBy string`，`CreatedAt string`，`UpdatedAt string`
  - 定义 `LicenseRedeemLog`：`ID string`，`LicenseKeyID string gorm:"index"`，`KeyMasked string`，`UserID string gorm:"index"`，`UserName string`，`Credits int`，`CreatedAt string`
  - 注意：**完全不要 validDays / IsCommercial / LicenseExpireAt 字段**
- [x] 1.3repository/db.go：AutoMigrate 列表末尾追加 `&model.LicenseKey{}`、`&model.LicenseRedeemLog{}`；User 表**不动**（已有的 Credits 字段够用）
- [x] 1.4model/user.go：确认 User 结构体已有 `Credits int` 字段、AuthUser 结构体也有 `Credits int` 字段、PublicUser 函数里有映射 → 如果有就不用改；没有的话补齐三处

## 2. 后端 Repository 层（新文件 repository/license_key.go）

- [x] 2.1实现 `BatchInsertLicenseKeys(keys []model.LicenseKey) (int, error)`：内部用 db.CreateInBatches，chunkSize=100；忽略唯一索引冲突（用 Session(&gorm.Session{SkipDefaultTransaction: true}) + Clauses(clause.OnConflict{DoNothing: true})），返回实际 RowsAffected 或 len(keys)-重复数
- [x] 2.2实现 `GetLicenseKeyByKey(key string, forUpdate bool) (model.LicenseKey, bool, error)`：forUpdate=true 时追加 `Clauses(clause.Locking{Strength:"UPDATE"})` 行锁；false 时普通 Find
- [x] 2.3实现 `MarkLicenseKeyUsed(tx *gorm.DB, id, userId, usedAt string) error`：用 tx（事务内）UPDATE status=used usedBy=userId usedAt=usedAt updatedAt=usedAt WHERE id=? AND status=unused
- [x] 2.4实现 `ListLicenseKeys(q model.Query, status, batchName, keyword string) ([]model.LicenseKey, int64, error)`：分页+排序 createdAt DESC；where：status非空加 status=?；batchName非空加 batchName=?；keyword 非空 加 `key LIKE %keyword%`
- [x] 2.5实现 `SaveLicenseRedeemLog(tx *gorm.DB, log model.LicenseRedeemLog) error`：事务内 INSERT
- [x] 2.6实现 `ListLicenseRedeemLogs(q model.Query, userID, userKeyword string) ([]model.LicenseRedeemLog, int64, error)`：分页 createdAt DESC；userID非空过滤指定用户；userKeyword非空加 `user_name LIKE ?`
- [x] 2.7实现 `ExistsUsedKeyInBatch(batchName string) (bool, error)`：COUNT WHERE batchName=? AND status=used，>0 返回 true
- [x] 2.8实现 `UpdateBatchUnusedCredits(batchName string, credits int) (int64, error)`：先调 ExistsUsedKeyInBatch → true 则返回错误；否则 UPDATE license_keys SET credits=?, updated_at=? WHERE batchName=? AND status=unused 返回 RowsAffected

## 3. 后端 Service 层（新文件 service/license_key.go，核心业务）

- [x] 3.1补常量：在 model/user.go 的 CreditLogType 旁边（或 model/credit_log 里）新增 `CreditLogTypeLicenseRedeem CreditLogType = "license_redeem"`
- [x] 3.2`NormalizeLicenseKey(input string) (string, bool)`：input = strings.TrimSpace → 去全角空格/制表符等→strings.ToUpper→删除所有'-'→校验每个字符都是0-9A-Z且长度=16→返回失败false；否则按位置 [0:4]-[4:8]-[8:12]-[12:16] 插入连字符得到 canonical 格式返回 true
- [x] 3.3`MaskLicenseKey(key string) string`：按连字符拆4段，把第3、4段替换成"****"（形如 XXXX-XXXX-****-****）
- [x] 3.4`ImportLicenseKeys(adminID, batchName string, credits int, txtContent []byte) (total, imported, dup, malformed int, malformedSamples []string, err error)`：
  - batchName = strings.TrimSpace(batchName)，必填；credits<=0 直接返回错误；txtContent 转 UTF-8 string，按 `\n` + `\r\n` split 逐行处理
  - 流程：初始化 seen map[string]struct{} 存本批次 unique；对每行 raw 先 Normalize → 失败则 malformed++ 且 malformedSamples 不满 10 条时 append(raw); 成功则 normalizedKey；若 seen 里已有 → dup++ continue; 否则查 DB 存在 GetLicenseKeyByKey → 存在则 dup++ continue; 否则 append(待入库列表)，加入 seen
  - 待入库列表组装成 []model.LicenseKey（ID=uuid.New、Key=normalized、Credits=credits、Status=unused、BatchName=batchName、CreatedBy=adminID、CreatedAt=nowRFC3339、UpdatedAt=same），调 repository.BatchInsertLicenseKeys，return 全部计数
- [x] 3.5`RedeemLicenseKey(userID, userName, rawKey string) (creditsGranted, newBalance int, err error)`（核心兑换！必须事务！）：
  - 步骤 0：Normalize → 失败返回 "卡密格式不正确" 的 safeError
  - 步骤 1：调 db.Begin() 启事务；defer rollback
  - 步骤 2：用 FOR UPDATE 行锁 GetLicenseKeyByKey(canonical, true) → 不存在返回 "卡密不存在"
  - 步骤 3：校验 key.Status == unused → 否则返回 "该卡密已被使用"（**没有过期校验**！纯余额）
  - 步骤 4：now := time.Now().UTC().Format(time.RFC3339)；keyCredits := key.Credits
  - 步骤 5：MarkLicenseKeyUsed(tx, key.ID, userID, now) → 若 RowsAffected!=1（并发临界）返回 "该卡密已被使用"
  - 步骤 6：加用户余额：直接复用 repository/user.go 里现成的 **RefundUserCredits(userID, keyCredits, now)**（因为加余额语义和退款是同操作）→ ok=false 则事务回滚返回错误
  - 步骤 7：写兑换流水：`SaveLicenseRedeemLog(tx, LicenseRedeemLog{uuid.New, key.ID, MaskLicenseKey(key.Key), userID, userName, keyCredits, now})`
  - 步骤 8：写 CreditLog 充值入账：`repository.SaveCreditLog(CreditLog{uuid.New, userID, CreditLogTypeLicenseRedeem, keyCredits, fmt.Sprintf("卡密兑换 %s，批次=%s", MaskLicenseKey(key.Key), key.BatchName), key.ID, now})`
  - 步骤 9：COMMIT；creditsGranted=keyCredits；查询最新 user.Credits 作为 newBalance；返回 (creditsGranted, newBalance, nil)
- [x] 3.6`ListMyRedeemLogs(userID string, q model.Query) ([]model.LicenseRedeemLog, int64, error)`：简单封装 repository.ListLicenseRedeemLogs(q, userID, "")
- [x] 3.7`GetPurchaseConfig() string`：返回 `config.Cfg.LicensePurchaseURL`
- [x] 3.8管理员侧封装：`AdminListLicenseKeys / AdminListRedeemLogs / AdminModifyBatchUnusedCredits(batchName string, credits int) (rows int64, err error)` 对应 repository 函数，只做参数校验 & 时间戳

## 4. 后端 Handler + Router

- [x] 4.1handler/license_key.go（新建）：
  - `LicensePurchaseConfig(w,r)`：公共（不用鉴权），直接 `OK(w, map[string]any{"purchaseURL": service.GetPurchaseConfig()})`
  - `RedeemLicenseKey(w,r)`：UserFromContext 拿 user；Bind JSON `{key string}`；trim key；调 service.RedeemLicenseKey；成功 `OK(w, {creditsGranted, newCreditsBalance})`，失败 FailError
  - `MyRedeemLogs(w,r)`：UserFromContext；ParseQuery page/pageSize → model.Query → 封装 OK(ListMyRedeemLogs)
- [x] 4.2handler/admin_license_key.go（新建）：
  - `AdminImportLicenseKeys(w,r)`：管理员鉴权；r.ParseMultipartForm(32MB)；读 file 字段 form.File → Open → io.ReadAll 拿 bytes；读 batchName/credits 表单字段（credits用fmt.Atoi转int）；adminID=currentUser.ID；调 service.ImportLicenseKeys → OK 返回 {totalLines, importedCount, duplicateCount, malformedCount, malformedSamples}
  - `AdminListLicenseKeys(w,r)`：query page/pageSize/status/batchName/keyword → service → OK 分页
  - `AdminListRedeemLogs(w,r)`：query page/pageSize/userKeyword → service → OK 分页
  - `AdminModifyBatchFaceValue(w,r)`：Bind JSON `{batchName, credits}` → credits>0 校验 → service.AdminModifyBatchUnusedCredits → OK({rowsAffected}) 或 FailError("已开兑，不可改")
- [x] 4.3router/router.go：按项目现有 WrapF / WrapH 模式注册：
  - 公共 API（r.Group("/api") 无中间件）：GET `"/license/purchase-config"` → handler.LicensePurchaseConfig
  - 登录用户组 v1（UserAuthWrapF 现有组）：POST `"/license/redeem"`、GET `"/license/redeem-logs"`
  - 管理员组 admin（AdminAuth现有组）：POST `"/license-keys/import"`、GET `"/license-keys"`、GET `"/license-redeem-logs"`、POST `"/license-keys/batch-face-value"`

## 5. 前端 API 层 + User Store

- [x] 5.1web/src/services/api/license.ts（新建）：
  - `export function getPurchaseConfig() { return request.get<any, {purchaseURL:string}>('/api/license/purchase-config').then(r=>r.data) }`
  - `export function redeemLicenseKey(key: string) { return request.post<any, {creditsGranted:number, newCreditsBalance:number}>('/api/v1/license/redeem', {key}).then(r=>r.data) }`
  - `export function getMyRedeemLogs(params: PaginationParams) { return request.get<any, PaginatedResponse<RedeemLogItem>>('/api/v1/license/redeem-logs', {params}).then(r=>r.data) }`
  - `export function adminImportLicenseKeys(payload: {file: File, batchName: string, credits: number})`：FormData append 文件/字段 → multipart POST `/api/admin/license-keys/import` → 返回 {totalLines,importedCount,duplicateCount,malformedCount,malformedSamples:string[]}
  - `export function adminListLicenseKeys(params: {page,pageSize,status,batchName,keyword})` GET `/api/admin/license-keys`
  - `export function adminListRedeemLogs(params: {page,pageSize,userKeyword})` GET `/api/admin/license-redeem-logs`
  - `export function adminModifyBatchFaceValue(body:{batchName:string,credits:number})` POST `/api/admin/license-keys/batch-face-value`
  - 顶部定义 TS 接口 RedeemLogItem {id,keyMasked,credits,createdAt} 等
- [x] 5.2 web/src/stores/use-user-store.ts：**不需要**新增任何字段（credits已经有了）；在 redeem 成功回调处，本地 optimistic：`set({user: {...state.user, credits: newCreditsBalance}})` + 触发 toast；并调用 refetch me 接口确保一致

## 6. 前端用户端：登录页 + 顶栏CreditsBadge + Credits账户页

- [x] 6.1 login/page.tsx：在卡片底部增加 Panel：
  - 标题小字 "Credits 充值说明"
  - 正文："新用户默认 Credits=0，生成图片按 0.04/张扣费，需先购买充值卡密。"
  - 蓝色 Button【💳 购买 Credits 卡密】：onClick 调用 getPurchaseConfig() 然后 window.open(purchaseURL,'_blank')
- [x] 6.2 components/layout/app-top-nav.tsx：在用户头像 Dropdown 之前新增独立组件 `<CreditsBadge />`：
  - 显示 `Credits: ${user.credits.toFixed(2)}`，字号大 1~2 号、加粗
  - 颜色：user.credits>5 → default；0<user.credits≤5 → orange/警告；=0 → red/danger + pulse动画
  - Tooltip：`当前余额约可生成 ${Math.floor(user.credits / 0.04)} 张图片` +（低余额追加"建议及时充值"）
  - onClick：router.push('/credits')
  - 右侧加一个小 `+` Icon Button：点击调 getPurchaseConfig() → window.open 新标签打开购卡
- [x] 6.3 app/(user)/credits/page.tsx（新建）：4 个 section 自上而下：
  1. BalanceCard：Ant Card，居中"当前 Credits 余额 **{user.credits.toFixed(2)}**" subtitle：约可生成 Math.floor(credits/0.04) 张图片；credits<=5 时内联 primary button【立即充值】= window.open(purchaseURL)
  2. PurchaseCTACard：标题"购买 Credits 充值卡密" + desc："点击前往官方发卡平台链动小铺（ldxp.cn）购买，付款后自动发货卡密，发货后复制卡密到下方『兑换卡密』即可到账。" + LARGE Button【前往购买卡密】→ getPurchaseConfig → window.open；下方红色警告文字：⚠️ "付款前请确认商品面额，购买后即时发货不退款！"
  3. RedeemFormCard：Form + Input.TextArea（或 Input）placeholder="XXXX-XXXX-XXXX-XXXX，支持直接粘贴16位纯卡号" + Button【立即兑换】loading=true时禁用；成功 message.success(`兑换成功！到账 ${grant} Credits，当前余额 ${balance}`) + store 乐观更新 user.credits；错误时 message.error(err.message)
  4. HistoryTabs：Tabs.TabPane 1【充值记录（兑换）】Table 列：卡密掩码/到账Credits/兑换时间，分页（调 getMyRedeemLogs）；TabPane 2【消费&退款明细】复用已有 credit-logs 接口（有的话直接用；没有就 TabPane 里单独实现调 GET /api/v1/credit-logs，列：时间/类型/变动额/备注/关联ID，已有后端应该有）
- [x] 6.4 全局余额不足错误处理：在全局 request error interceptor 里，当 message 匹配"Credits 余额不足"或后端约定业务码时：message.error 带 2 个按钮：<button>立即充值</button>（window.open purchaseURL）和 <a>查看账户</a>（router.push /credits）

## 7. 前端管理员后台：卡密管理页 + 菜单入口

- [x] 7.1导航菜单：在 admin 侧边（或 app-top-admin-nav）增加菜单项"卡密管理"→ 路由 /admin/license-keys
- [x] 7.2app/(admin)/admin/license-keys/page.tsx：顶部导入区 + Tabs：
  - **顶部导入区（醒目）**：
    - 大红警告 Descriptions：`⚠️ 【关键提醒】导入的TXT卡密文件 **必须与您在链动小铺（ldxp.cn）对应商品上传的TXT完全同一份**。否则买家收到的卡在系统查不到会造成投诉！建议先在 ldxp 上传成功后，用同一文件在这里导入。`
    - Form 字段：Upload（accept=".txt"，单个文件，maxSize 50MB 提示）；Input（batchName placeholder="如：20面额-202608第一批" 必填）；InputNumber（credits placeholder="20" 必填>0 tooltip="本批次每张卡密的面额，单位 Credits" 说明"这个值必须与 ldxp 商品面额一致！"）；Button【开始导入】
    - 提交时先弹 Modal.confirm：`批次「${batchName}」共 ${fileLinesEstimateN} 行左右，面额 ${credits} Credits，确认导入吗？导入后该批次如果还未被兑换，可以整批次修改面额；一旦有任意用户完成兑换，将无法再修改面额。`
    - 成功后 Result 组件显示：解析 ${total} 行，成功导入 ${imported}，重复（系统中已存在或本批次重复）${dup}，格式错误 ${malformed}；malformedSamples 非空时 Table 显示前10条 raw line
    - 加一个自检功能：导入成功后显示一个"🔍 导入自检（随机抽样5条）"Button → 点击后从本批次随机抽5个 canonical 密钥（去头段后展示：XXXX-XX**-****-****）提示管理员"请复制这5条去 ldxp.cn 商家后台 → 该商品 → 库存查询，验证是否同时存在于平台库存中，若不存在说明文件不是同一份！"
  - **Tab 1 卡密列表**：筛选条件 Row：Select status（unused/used/全部）、Input batchName 搜索、Input keyword、查询 Button；Table 列：卡密掩码/批次名/Credits/状态/使用者用户名/使用时间/导入时间；分页；顶上加一个 Button【整批修改面额】→ Modal Select 批次名 + InputNumber 新 Credits → 提交 AdminModifyBatchFaceValue，返回错误"已开兑，不能改"时弹红色提示
  - **Tab 2 兑换记录**：筛选 Input userKeyword，查询 Button；Table 列：时间/用户名/卡密掩码/Credits；分页；右上加一个 Button【CSV导出】（这版不实现可以暂时 disabled tooltip 后续开发）

## 8. 联调验证 & 验收（端到端走一遍）

- [ ] 8.1 停掉旧后端 → `go run .` 重新启动：检查日志 AutoMigrate 执行成功，license_keys / license_redeem_logs 两张表建出，无报错；User 表无 ALTER 指令（因为没加字段）
- [ ] 8.2 用任意工具生成 10 个 XXXX-XXXX-XXXX-XXXX 卡密 → 保存为 keys-test.txt（故意中间加 2 行空行 + 1 行格式乱码）
- [ ] 8.3 管理员登录 → 卡密管理 → 上传 keys-test.txt batchName="测试20面额" credits=20 → 确认导入报告：解析 13 行 / 导入 10 / 重复 0 / malformed 1（空行也算的话空2+乱1=malformed3，核对实现的空行计数逻辑）
- [ ] 8.4 二次导入同一份 → imported=0 dup=10 幂等
- [ ] 8.5 "整批修改面额"→改50 → 成功rowsAffected=10；立刻再兑1张后再试修改 → 返回错误"已存在兑换记录" ✔
- [ ] 8.6 新注册普通用户 A（默认 Credits=0）→ 登录
  - 顶栏 Credits 显示 0.00，红色+pulse，点击跳 /credits ✔
  - /credits 页面兑换表单 → 粘贴 1 个纯 16 位小写卡号 → 兑换成功：creditsGranted=50，newBalance=50 ✔；顶栏 CreditsBadge 实时更新为 50.00 ✔
  - 再兑同一张卡 → 报错"已被使用"余额不变 ✔；再输入乱"abc 123"→ 格式错 ✔；输入"ZZZZ-ZZZZ-ZZZZ-ZZZZ"（规范但不存在）→ 卡密不存在 ✔
- [ ] 8.7 用户 A 去画布中生成 1 张图片（模型价格 0.04）→ 成功后余额 49.96 ✔；查 CreditLog 出现两条：一条 license_redeem +50、一条 consume -0.04 ✔
- [ ] 8.8 管理员卡密列表筛 status=used → 看到 1 条兑换记录，对应用户 A；兑换记录 Tab 看到 A 的用户名和 50 credits ✔
- [ ] 8.9 未登录访问登录页 → 底部有【购买 Credits 卡密】按钮 → 点击新标签打开 https://pay.ldxp.cn/shop/35TCHF9A（或自定义 .env 配置的值）✔
- [ ] 8.10 并发兑1张卡密：用 Postman Runner 或写个小脚本 20 goroutine 同时请求兑1张新的 → 最终仅 1 次成功，其余全返回"已被使用"，余额只+1次面额 ✔（事务原子性验证）
