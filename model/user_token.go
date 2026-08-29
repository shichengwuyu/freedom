package model

import "time"

// UserTokenStatus 用户 API Key 状态。
const (
	// UserTokenStatusActive 正常可用。
	UserTokenStatusActive = "active"
	// UserTokenStatusDisabled 用户主动禁用。
	UserTokenStatusDisabled = "disabled"
	// UserTokenStatusExhausted 余额耗尽（Sprint 2 渠道选择器后续在 consume 时自动转）。
	UserTokenStatusExhausted = "exhausted"
	// UserTokenStatusExpired 已过期（CurrentAuthUserByToken 命中 ExpiredAt 时自动转）。
	UserTokenStatusExpired = "expired"
)

// UserToken 用户自建 API Key（OpenAI 兼容 sk- 格式）。
//
// 用途：
//  1) 外部 OpenAI SDK / Cursor / Cline / curl 直接对接 Freedom 当网关用
//  2) Admin 端按 token 维度审计用量、限速、封禁
//
// 字段约束：
//   - KeyHash：sha256(明文) hex；唯一索引；明文永不落库，创建时一次性返回
//   - KeyPrefix：明文前 12 位（含 sk-fk-），用于前端列表识别
//   - ModelLimits / AllowIPs：白名单，空字符串 = 不限
//   - BalanceCapCents > 0 时只扣本字段额度；否则扣用户全局余额
//   - UnlimitedBalance = true 时 BalanceCapCents 字段被忽略
type UserToken struct {
	ID               string     `json:"id" gorm:"primaryKey"`
	UserID           string     `json:"userId" gorm:"index"`
	Name             string     `json:"name"`
	KeyPrefix        string     `json:"keyPrefix" gorm:"type:varchar(32)"`
	KeyHash          string     `json:"-" gorm:"type:varchar(128);uniqueIndex"`
	Status           string     `json:"status" gorm:"type:varchar(16);index"`
	ExpiredAt        *time.Time `json:"expiredAt,omitempty"`
	BalanceCapCents  int        `json:"balanceCapCents"`
	UsedCents        int        `json:"usedCents"`
	UnlimitedBalance bool       `json:"unlimitedBalance"`
	ModelLimits      string     `json:"modelLimits" gorm:"type:varchar(512)"`
	AllowIPs         string     `json:"allowIps" gorm:"type:varchar(512)"`
	LastUsedIP       string     `json:"lastUsedIp"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// MaskUserTokenKey 给前端展示的打码 key：前 7 + "..." + 末 4（new-api MaskTokenKey 风格）。
// 输入应是 KeyPrefix 或明文；输出始终是脱敏后的展示串。
func MaskUserTokenKey(key string) string {
	if len(key) <= 11 {
		return key
	}
	return key[:7] + "..." + key[len(key)-4:]
}
