## Context

当前 freedom 已有完整的 Credits 预付费扣费闭环（代码层验证已实现）：

```
 已有的 Credits 计费（本方案不改动）
════════════════════════════════════════════════════════
 AI 请求前                                     请求失败
 service.ModelCost() 算价格              service.RefundUserCredits()
        ↓ credits × 张数 ↓                        ↑  自动退款
        ↓                                   失败回调
        ↓ repository.ConsumeUserCredits() ──┐
        ↓   (余额不足则拦截，够就 Credits-=) │
        ↓                                     │
 调用上游接口生成图片/视频 ──── 成功 ────────┘
                                     不做退款
 流水：repository.SaveCreditLog(type consume/refund)
════════════════════════════════════════════════════════
```

**扣减代码位置**：
- 图片/文字/音频：[handler/ai.go#L130-L185](file:///f:/trae/wifi/freedom-main/handler/ai.go#L130-L185)
- 视频创建：[handler/video_task.go#L75-L118](file:///f:/trae/wifi/freedom-main/handler/video_task.go#L75-L118)
- Credits DB 操作：[repository/user.go#L82-L129](file:///f:/trae/wifi/freedom-main/repository/user.go#L82-L129)

**现在缺的商业闭环（本方案补齐）**：
```
新注册用户 → Credits = 0 (不赠送) → 想用？
                                  ↓
                     点【充值】跳 ldxp.cn/shop/35TCHF9A
                                  ↓
                       买20元卡密 → 收到 XXXX-XXXX-XXXX-XXXX
                                  ↓
                     回系统【兑换卡密】 → Credits += 20 ✅
                                  ↓
                       生成图片扣0.04/张，扣完又停
```

发卡平台选用 **链动小铺 ldxp.cn**（专业发卡网，资金直清），商家操作流：
1. 商家用工具批量生成卡密TXT（一行一个，格式 XXXX-XXXX-XXXX-XXXX）
2. ldxp 商家后台创建不同面额商品（如"20 Credits充值卡"/"100 Credits充值卡"），上传同一份TXT入库
3. 发布商品，获取商品链接 `https://pay.ldxp.cn/shop/35TCHF9A`
4. **同一 TXT** 在 freedom 管理员后台批量导入，指定对应 Credits 面额（如 20）
5. 买家付款 → ldxp 自动发一张卡密 → 买家回系统兑换 → 余额累加 → 可使用

技术栈约束：Go 1.25 + Gin + GORM；Next.js 16 + Ant Design 6；model→repository→service→handler→router 分层。

## Goals / Non-Goals

**Goals:**
- 完成 Credits 商业闭环：购买卡密 → 兑换 → 加余额 → AI扣费
- 多处入口跳转 ldxp 商品链接，降低充值门槛
- 顶栏醒目展示 Credits 余额，低余额提醒充值
- 管理员可批量导入TXT卡密，统一设定面额，支持按批次/状态/关键词查询卡密和兑换记录
- 卡密兑换具备：格式兼容（连字符/无连字符）、唯一性校验、状态校验、**事务原子性**（防止并发重复兑换）
- 所有 Credits 变动（充值、消费、退款）均有 CreditLog 流水，可对账

**Non-Goals:**
- **不做**任何"免费版/商用版"身份区分（没有 IsCommercial 字段）
- **不做**任何授权到期时间（没有 LicenseExpireAt、没有 validDays），纯余额永续
- **不做**卡密生成功能（商家自己/ldxp 工具生成TXT，系统仅导入）
- **不做**卡密撤销功能（ldxp卖出后无法追回，有误操作直接改数据库）
- **不做**ldxp 支付回调自动入库（首版仅手动导入TXT，够用即可）
- **不做**任何图片/视频扣费逻辑修改（已完整实现，沿用）

## Decisions

### 1. 卡密格式与兼容
- **决策**：标准格式 `XXXX-XXXX-XXXX-XXXX`（16个字母数字 + 3连字符）；服务端 Normalize 流程：trim→去空格→全转大写→去连字符后校验纯字母数字、长度=16→重新插入连字符作为 canonical key 入库/查询
- **理由**：ldxp 通用格式；容错好（用户复制的卡密有时没连字符、有小写、混空格）；组合空间 36^16 无法暴力枚举
- **替代**：纯UUID（太长体验差）、纯数字（易爆破）

### 2. 卡密与流水数据模型
- **决策**：新建 LicenseKey（id, key[唯一索引], Credits, Status=unused/used, UsedBy, UsedAt, BatchName, CreatedBy, CreatedAt, UpdatedAt）；新建 LicenseRedeemLog（id, LicenseKeyID, KeyMasked, UserID, UserName, Credits, CreatedAt）
- **理由**：LicenseKey 做唯一索引+status 防重复兑换；LicenseRedeemLog 独立审计表，方便管理员对账；**无 validDays / 过期时间字段**（纯卡密余额）
- **替代**：把卡密塞进 CreditLog metadata（丢失状态、批次、去重等管理能力）

### 3. 兑换流程的事务原子性（最高风险点）
- **决策**：在 GORM 事务内执行：① BEGIN → ② `SELECT * FROM license_keys WHERE key=? FOR UPDATE` 行锁 → ③ 校验 status=unused → ④ UPDATE license_keys SET status=used, usedBy=?, usedAt=? → ⑤ `UPDATE users SET credits = credits + @credits, updated_at=? WHERE id=?`（加余额）→ ⑥ INSERT license_redeem_logs → ⑦ INSERT credit_logs（type=license_redeem, delta=+credits, remark=卡密掩码）→ ⑧ COMMIT。任一步失败 → ROLLBACK，返回中文错误
- **理由**：并发重复兑换是最严重bug，必须通过DB行锁+事务保证严格一次；CreditLog统一写入保证对账不缺充值入账记录（与已有消费/refund流水同表）
- **替代**：应用层唯一约束+唯一索引 → 在临界区仍会触发 DB 唯一索引异常，语义不清且需额外兜底

### 4. TXT 批量导入
- **决策**：管理员上传 text/plain TXT + 表单字段 batchName(必填) + credits(必填>0)。后端：按 \n 切行 → 逐行 Normalize（格式错计入 malformedSamples）→ 用 Go map 在内存里做本批次去重 → 对每个 unique key 先做 SELECT 查库是否已存在 → 存在计入 duplicate → 仅对 genuinely new 的 keys 用 GORM CreateInBatches 批量入库；最后返回 {total, imported, duplicate, malformed, malformedSamples[0:10]}
- **理由**：与 ldxp "一行一个卡密"TXT格式完全一致，商家复制同一份文件即可；导入报告让管理员当场发现格式错/重复问题
- **替代**：Excel/CSV（增加复杂度，无意义）、先全塞再靠唯一索引过滤（报错难读，批量中断风险）

### 5. 导入批次面额修正
- **决策**：管理员选择批次名 → 填新 credits → 后端：先查 `WHERE batchName=? AND status=used` 有任意记录 → 拒绝修改返回"该批次已开兑不可改面额"；否则 `UPDATE license_keys SET credits=? WHERE batchName=? AND status=unused` 返回受影响行数
- **理由**：运营填错面额是高概率事故（如20元卡填成2000），未开兑的批次允许救回；已开兑的批次锁住，避免对账时出现"同批次面额混乱"
- **替代**：删除批次重新导入（浪费导入操作且丢created审计信息）

### 6. 购买链接配置
- **决策**：config.Config 加 `LicensePurchaseURL string`，tag `env:"LICENSE_PURCHASE_URL" envDefault:"https://pay.ldxp.cn/shop/35TCHF9A"`；前端通过公共 GET `/api/license/purchase-config` 取 JSON {purchaseURL}
- **理由**：换 ldxp 商品链接/域名仅改 .env 即可，不用重编译前后端；前后端共用同一来源，不会出现硬编码不一致

### 7. 顶栏余额展示与交互
- **决策**：顶栏右侧用户区左侧**独立组件CreditsBadge**：大号字体显示 `Credits: 23.56`（保留2位小数），悬停tooltip "生成图片约可使用 X 张（按0.04/张估算）"；颜色：credits>5 默认蓝、≤5橙色、=0红色并脉冲动画；点击整个徽章跳转 `/credits`；生成图片时若 ConsumeUserCredits 触发 rowsAffected=0 返回余额不足错误，前端 toast 提示并带【立即充值】按钮跳购买链接
- **理由**：纯余额模型用户最关心"我还剩多少钱、什么时候要充"，必须让余额随处可见
- **替代**：只在账户页面显示（用户每次用都要点进去看，体验差，流失率高）

## Risks / Trade-offs

- **[风险] 并发重复兑换同一张卡 → 加余额两次** → 强制事务 + FOR UPDATE 行锁；补一层 DB 唯一索引（key）兜底；加单测模拟并发 20 goroutine 兑 1 张卡，验证仅 1 次成功
- **[风险] ldxp 上传 TXT 与 canvas 导入 TXT 不是同一份 → 用户买的卡在系统查不到 → 投诉** → 导入页大红色警告贴8号字"务必导入与ldxp上传完全相同的TXT文件！"；导入页提供"导入后随机抽5条 key 回显" → 管理员复制去 ldxp 商品后台库存搜索做自检
- **[风险] 导入时填错 credits 面额 → 发100元卡填成10元 → 大亏** → 导入弹 Modal 二次确认"批次【X】共 N 条，面额 Credits=Y，确认无误吗？"；未开兑的批次提供整批次修改面额救回功能；已开兑锁定
- **[风险] 余额=0 的用户白嫖** → 在 `service.ConsumeUserCredits` 处已用 `WHERE id=? AND credits >= ?` 做原子比较，余额不足 rowsAffected=0 → handler.FailError 返回"Credits 余额不足，请充值"，前端引导充值
- **[权衡] 新注册用户是否赠送初始额度** → 明确**不送**（用户指定）；若后续需营销赠送，直接在 service.CreateUser 末尾 credits 设为某值即可（或在管理员后台手动加 Credits 按钮+CreditLog 类型 admin_grant）

## Migration Plan

1. **部署**：
   - 后端 go build → 启动 → GORM AutoMigrate 自动建 license_keys、license_redeem_logs 表（增量式，零风险）；User 表无字段变更
   - 前端构建部署 → 登录页/顶栏/Credits账户页 自然出现
2. **回滚**：所有改动追加式（新表/新路由/新页面），直接替换旧二进制 + 旧前端构建即可回滚，不影响老用户使用（不过没充值的用户仍然 Credits=0 无法用）
3. **运营上线前 Checklist**：
   - [ ] 用工具生成第一批卡密TXT：如20面额500张 + 100面额200张
   - [ ] ldxp.cn 商家后台创建商品"20 Credits充值卡" → 上传同一份 → 发布 → 验证链接可访问
   - [ ] canvas 管理员后台 → 卡密管理 → 导入同一份TXT batchName="20面额-第一批" credits=20 → 核对导入报告
   - [ ] 同理导入100面额批次
   - [ ] 用测试账号走一遍：买（ldxp自购）→ 复制卡密→canvas兑换→余额+20→生成1张图→余额-0.04→CreditLog可见两条记录 → ✅ 通过
4. **数据迁移**：无历史数据。现有用户 Credits 余额、CreditLog 保持不变。

## Open Questions

- 是否需要在用户账户页增加"申请试玩额度"按钮（用户点按钮后通知管理员，管理员手动加额度）？（适合内测拉新阶段，目前不做，可后续追加）
- 卡密面额档位：ldxp 上会放几档？（导入时 credits 字段按档位手动填，不需要代码里固化档位列表，灵活即可）
- 是否需要顶栏 Credits 余额旁边放 mini "充值"按钮（直接 window.open 购卡链接，不跳中间页）？ → 推荐直接做成"余额数字点击跳/credits + 旁边一个小图标充值按钮直接新标签打开"
