package service

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

const (
	maxLicenseKeyLength = 5600
	maxImportBatchSize  = 50000
	maxBatchStock       = 300000
	// 自动生成相关
	licenseKeyExportDir = "data/license-keys"
	maxGenerateBatchSize = 50000
	generatedKeyLength  = 16
)

// licenseKeyAlphabet 不含易混淆字符 0/O/1/l/I，降低用户抄写出错率。
const licenseKeyAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789abcdefghijkmnpqrstuvwxyz"

func randLicenseKey(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	buf := make([]byte, length)
	for i := range b {
		buf[i] = licenseKeyAlphabet[int(b[i])%len(licenseKeyAlphabet)]
	}
	return string(buf), nil
}

// sanitizeBatchNameForFile 把批次名转成安全文件名（去除路径分隔符与非法字符）。
func sanitizeBatchNameForFile(name string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", string(os.PathSeparator), "_",
		":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_",
	)
	s := strings.TrimSpace(r.Replace(name))
	if s == "" {
		s = "batch"
	}
	return s
}

// writeLicenseKeysToFolder 把一批卡密落盘到 data/license-keys/<批次>.txt（一行一个，兼容链动小铺导入格式）。
func writeLicenseKeysToFolder(batchName string, keys []string) (string, error) {
	if err := os.MkdirAll(licenseKeyExportDir, 0o755); err != nil {
		return "", err
	}
	safe := sanitizeBatchNameForFile(batchName)
	path := filepath.Join(licenseKeyExportDir, safe+".txt")
	content := strings.Join(keys, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// GenerateLicenseKeys 管理员自动生成卡密：系统用 crypto/rand 生成唯一随机 key，
// 入库（同一批次面额一致），并落盘到 data/license-keys/<批次>.txt（一行一个，兼容链动小铺导入格式）。
// 返回生成的 key 列表与落盘文件路径。
func GenerateLicenseKeys(adminID, batchName string, cents, count int) (generated []string, filePath string, err error) {
	batchName = strings.TrimSpace(batchName)
	if batchName == "" {
		return nil, "", safeMessageError{message: "批次名必填"}
	}
	if cents <= 0 {
		return nil, "", safeMessageError{message: "面额必须大于 0"}
	}
	if count <= 0 || count > maxGenerateBatchSize {
		return nil, "", safeMessageError{message: fmt.Sprintf("单次生成数量必须在 1 ~ %d 张之间", maxGenerateBatchSize)}
	}
	existingInBatch, err := repository.CountLicenseKeysByBatchName(batchName)
	if err != nil {
		return nil, "", err
	}
	if int64(count)+existingInBatch > int64(maxBatchStock) {
		return nil, "", safeMessageError{
			message: fmt.Sprintf(
				"单批次库存不能超过 %d 张：该批次已存在 %d 张，本次新增 %d 张合计 %d 张",
				maxBatchStock, existingInBatch, count, existingInBatch+int64(count),
			),
		}
	}

	// 1) 生成不重复的候选 key
	candidates := make([]string, 0, count)
	seen := map[string]struct{}{}
	for len(candidates) < count {
		k, e := randLicenseKey(generatedKeyLength)
		if e != nil {
			return nil, "", e
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		candidates = append(candidates, k)
	}

	// 2) 一次性查重（DB 已存在则剔除并补足）
	existingSet, err := repository.ExistingLicenseKeySet(candidates)
	if err != nil {
		return nil, "", err
	}
	unique := candidates[:0]
	for _, k := range candidates {
		if !existingSet[k] {
			unique = append(unique, k)
		}
	}
	for len(unique) < count {
		k, e := randLicenseKey(generatedKeyLength)
		if e != nil {
			return nil, "", e
		}
		if _, ok := seen[k]; ok || existingSet[k] {
			continue
		}
		seen[k] = struct{}{}
		unique = append(unique, k)
	}

	// 3) 入库
	tNow := now()
	pending := make([]model.LicenseKey, len(unique))
	for i, k := range unique {
		pending[i] = model.LicenseKey{
			ID:             uuid.NewString(),
			Key:            k,
			FaceValueCents: cents,
			Status:         model.LicenseKeyStatusUnused,
			BatchName:      batchName,
			CreatedBy:      adminID,
			CreatedAt:      tNow,
			UpdatedAt:      tNow,
		}
	}
	if _, err := repository.BatchInsertLicenseKeys(pending); err != nil {
		return nil, "", err
	}

	// 4) 落盘文件夹
	filePath, err = writeLicenseKeysToFolder(batchName, unique)
	if err != nil {
		// 入库成功但落盘失败：返回错误提示（DB 已有，可重新导出）
		return unique, "", safeMessageError{
			message: fmt.Sprintf("卡密已入库，但写入文件夹失败：%v（可稍后在列表页重新导出）", err),
		}
	}
	return unique, filePath, nil
}

// SanitizeBatchNameForFileExport 导出文件名安全化（供 handler 生成下载文件名）。
func SanitizeBatchNameForFileExport(name string) string {
	return sanitizeBatchNameForFile(name)
}

// ExportLicenseKeysBatch 导出某批次卡密为 TXT 文本：优先读落盘文件，缺失时从 DB 重新拼装。
func ExportLicenseKeysBatch(batchName string) (string, error) {
	safe := sanitizeBatchNameForFile(batchName)
	path := filepath.Join(licenseKeyExportDir, safe+".txt")
	if data, err := os.ReadFile(path); err == nil {
		return string(data), nil
	}
	keys, err := repository.ListLicenseKeyKeysByBatch(batchName)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", safeMessageError{message: "该批次没有卡密"}
	}
	return strings.Join(keys, "\n") + "\n", nil
}

var licenseKeySplitters = []string{"----", "---", "--", "\t", "|"}

// SanitizeLicenseKeyInput 兼容链动小铺批量导入格式：
// 1. 一行一个；空行返回 ("", false)
// 2. 支持「卡号 + 任意分隔符(空格/----/---/--/|/\t) + 密码/备注/区号……」—— 只取**第一段非空 token** 作为卡号（与链动小铺"仅选号模式下第一个空格前的内容向买家展示"逻辑一致）
// 3. 卡号长度 1 ~ 5600 位（与链动小铺一致）
// 4. 卡号保留原始大小写、不插入连字符（因为链动小铺侧发给买家的就是原样字符串，用户回来兑换时必须完全匹配）
func SanitizeLicenseKeyInput(line string) (string, bool) {
	// 去除 UTF-8 BOM（链动小铺导出的 TXT 首行可能带 BOM，会导致卡密首字符为 \uFEFF 而兑换失败）
	line = strings.TrimPrefix(line, "\uFEFF")
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	// 优先按"多字符分隔符"切，再按空格切
	remainder := line
	for _, sep := range licenseKeySplitters {
		if idx := strings.Index(remainder, sep); idx >= 0 {
			remainder = remainder[:idx]
		}
	}
	// 最后按空白切第一段
	fields := strings.Fields(remainder)
	if len(fields) == 0 {
		return "", false
	}
	key := strings.TrimSpace(fields[0])
	if key == "" || len(key) > maxLicenseKeyLength {
		return "", false
	}
	return key, true
}

// MaskLicenseKey 脱敏：前 4 + **** + 后 4；长度 <=8 时全部打码。
func MaskLicenseKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	n := len(key)
	if n <= 8 {
		return strings.Repeat("*", n)
	}
	return key[:4] + "****" + key[n-4:]
}

// ImportLicenseKeys 管理员批量导入。
// 兼容链动小铺 TXT：一行一个；空行无效；卡号+密码/备注(空格/----/---/--/|/tab)取第一段；
// 单次 ≤ 50000 张；单个批次（同 batch_name）库存总数 ≤ 300000 张。
func ImportLicenseKeys(adminID, batchName string, cents int, txtContent []byte) (total, imported, dup, malformed int, malformedSamples []string, err error) {
	batchName = strings.TrimSpace(batchName)
	if batchName == "" {
		return 0, 0, 0, 0, nil, safeMessageError{message: "批次名必填"}
	}
	if cents <= 0 {
		return 0, 0, 0, 0, nil, safeMessageError{message: "面额必须大于 0"}
	}
	raw := strings.ReplaceAll(string(txtContent), "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	// 过滤完全空行（Trim 后也是空）当作无效，不计入总数
	nonEmptyLines := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmptyLines = append(nonEmptyLines, l)
		}
	}
	total = len(nonEmptyLines)
	if total == 0 {
		return 0, 0, 0, 0, nil, safeMessageError{message: "未检测到任何有效卡密行，请按「一行一个」格式填入或上传"}
	}
	if total > maxImportBatchSize {
		return 0, 0, 0, 0, nil, safeMessageError{message: fmt.Sprintf("单次最多导入 %d 张卡密（当前 %d 张），请分次添加", maxImportBatchSize, total)}
	}
	existingInBatch, err := repository.CountLicenseKeysByBatchName(batchName)
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	if int64(total)+existingInBatch > int64(maxBatchStock) {
		return 0, 0, 0, 0, nil, safeMessageError{
			message: fmt.Sprintf(
				"单批次库存不能超过 %d 张：该批次已存在 %d 张，本次新增 %d 张合计 %d 张，请换个批次名或分批次导入",
				maxBatchStock, existingInBatch, total, existingInBatch+int64(total),
			),
		}
	}

	seen := map[string]struct{}{}
	pending := make([]model.LicenseKey, 0, len(nonEmptyLines))
	tNow := now()
	malformedSamples = []string{}

	for _, line := range nonEmptyLines {
		canonical, ok := SanitizeLicenseKeyInput(line)
		if !ok {
			malformed++
			if len(malformedSamples) < 10 {
				malformedSamples = append(malformedSamples, strings.TrimSpace(line))
			}
			continue
		}
		if _, exist := seen[canonical]; exist {
			dup++
			continue
		}
		// DB 唯一性校验
		_, exist, err := repository.GetLicenseKeyByKey(nil, canonical, false)
		if err != nil {
			return 0, 0, 0, 0, nil, err
		}
		if exist {
			dup++
			continue
		}
		seen[canonical] = struct{}{}
		pending = append(pending, model.LicenseKey{
			ID:             uuid.NewString(),
			Key:            canonical,
			FaceValueCents: cents,
			Status:         model.LicenseKeyStatusUnused,
			BatchName:      batchName,
			CreatedBy:      adminID,
			CreatedAt:      tNow,
			UpdatedAt:      tNow,
		})
	}

	inserted, err := repository.BatchInsertLicenseKeys(pending)
	return total, inserted, dup, malformed, malformedSamples, err
}

// RedeemLicenseKey 用户兑换卡密（事务原子：行锁 + 状态更新 + 加余额 + 2条流水）。
func RedeemLicenseKey(userID, userName, rawKey string) (creditsGranted, newBalance int, err error) {
	canonicalKey, ok := SanitizeLicenseKeyInput(rawKey)
	if !ok {
		return 0, 0, safeMessageError{message: fmt.Sprintf("卡密不能为空，单张卡密长度最多 %d 个字符", maxLicenseKeyLength)}
	}

	db, err := repository.DB()
	if err != nil {
		return 0, 0, err
	}

	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if tx.Error != nil {
		return 0, 0, tx.Error
	}

	key, exists, err := repository.GetLicenseKeyByKey(tx, canonicalKey, true)
	if err != nil {
		tx.Rollback()
		return 0, 0, err
	}
	if !exists {
		tx.Rollback()
		return 0, 0, safeMessageError{message: "卡密不存在，请检查输入是否正确"}
	}
	if key.Status != model.LicenseKeyStatusUnused {
		tx.Rollback()
		return 0, 0, safeMessageError{message: "该卡密已被使用"}
	}

	tNow := now()
	grant := key.FaceValueCents

	// 1. 标记已使用（带并发保护 WHERE status=unused）
	if err := repository.MarkLicenseKeyUsed(tx, key.ID, userID, tNow); err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	// 2. 加用户余额
	user, ok, err := repository.RefundUserBalanceTx(tx, userID, grant, tNow)
	if err != nil || !ok {
		tx.Rollback()
		if err != nil {
			return 0, 0, err
		}
		return 0, 0, safeMessageError{message: "更新余额失败，请重试"}
	}

	// 3. 兑换流水 LicenseRedeemLog
	if err := repository.SaveLicenseRedeemLog(tx, model.LicenseRedeemLog{
		ID:             uuid.NewString(),
		LicenseKeyID:   key.ID,
		KeyMasked:      MaskLicenseKey(key.Key),
		UserID:         userID,
		UserName:       userName,
		FaceValueCents: grant,
		CreatedAt:      tNow,
	}); err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	// 4. BalanceLog 充值入账
	if err := repository.SaveBalanceLogTx(tx, model.BalanceLog{
		ID:        uuid.NewString(),
		UserID:    userID,
		Type:      model.BalanceLogTypeManualRecharge,
		Amount:    grant,
		Balance:   user.BalanceCents,
		RelatedID: key.ID,
		Remark:    fmt.Sprintf("卡密兑换 %s，批次=%s", MaskLicenseKey(key.Key), key.BatchName),
		CreatedAt: tNow,
	}); err != nil {
		tx.Rollback()
		return 0, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, 0, err
	}

	return grant, user.BalanceCents, nil
}

// ListMyRedeemLogs 用户查看自己的兑换记录。
func ListMyRedeemLogs(userID string, q model.Query) ([]model.LicenseRedeemLog, int64, error) {
	return repository.ListLicenseRedeemLogs(q, userID, "")
}

// ListMyBalanceLogs 用户查看自己的余额流水。
func ListMyBalanceLogs(userID string, q model.Query) (model.BalanceLogList, error) {
	logs, total, err := repository.ListBalanceLogs(model.Query{
		Keyword:  userID,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		return model.BalanceLogList{}, err
	}
	return model.BalanceLogList{Items: logs, Total: int(total)}, nil
}

// GetPurchaseConfig 返回购买跳转链接。
func GetPurchaseConfig() string {
	return strings.TrimSpace(config.Cfg.LicensePurchaseURL)
}

// ---------- 管理员侧封装 ----------

func AdminListLicenseKeys(q model.Query, status, batchName, keyword string) ([]model.LicenseKey, int64, error) {
	return repository.ListLicenseKeys(q, status, batchName, keyword)
}

func AdminListRedeemLogs(q model.Query, userKeyword string) ([]model.LicenseRedeemLog, int64, error) {
	return repository.ListLicenseRedeemLogs(q, "", userKeyword)
}

func AdminModifyBatchUnusedFaceValueCents(batchName string, cents int) (int64, error) {
	batchName = strings.TrimSpace(batchName)
	if batchName == "" {
		return 0, safeMessageError{message: "批次名必填"}
	}
	if cents <= 0 {
		return 0, safeMessageError{message: "面额必须大于 0"}
	}
	affected, err := repository.UpdateBatchUnusedFaceValueCents(batchName, cents, now())
	// 业务错误（如"批次已开兑，不可修改面额"）需透传给前端，避免被 FailError 统一掩盖成"操作失败"
	if err != nil && strings.Contains(err.Error(), "批次已开兑") {
		return 0, safeMessageError{message: err.Error()}
	}
	return affected, err
}
