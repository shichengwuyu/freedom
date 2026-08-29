package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// UserTokenOptions CreateUserToken 的可选参数。零值 = 不限制。
type UserTokenOptions struct {
	ExpiredAt        *time.Time
	BalanceCapCents  int
	UnlimitedBalance bool
	ModelLimits      []string
	AllowIPs         []string
}

// CreateUserToken 生成新 sk- token。返回的 raw 明文仅此一次返回，库内只存 SHA-256 hash。
func CreateUserToken(userID, name string, opts UserTokenOptions) (model.UserToken, string, error) {
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID == "" {
		return model.UserToken{}, "", safeMessageError{message: "userID 不能为空"}
	}
	if name == "" {
		return model.UserToken{}, "", safeMessageError{message: "名称不能为空"}
	}
	if opts.BalanceCapCents < 0 {
		return model.UserToken{}, "", safeMessageError{message: "BalanceCapCents 不能为负"}
	}
	if len(opts.ModelLimits) > 50 || len(opts.AllowIPs) > 50 {
		return model.UserToken{}, "", safeMessageError{message: "白名单条目数过多（>50）"}
	}

	raw := "sk-fk-" + randomURLSafe(32)
	hash := hashToken(raw)
	// KeyPrefix 存原明文前 12 位（含 sk-fk- 前缀），用于前端列表识别但不暴露完整 key。
	prefix := raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}

	now := time.Now().UTC()
	t := model.UserToken{
		ID:               newID("utok"),
		UserID:           userID,
		Name:             name,
		KeyPrefix:        prefix,
		KeyHash:          hash,
		Status:           model.UserTokenStatusActive,
		ExpiredAt:        opts.ExpiredAt,
		BalanceCapCents:  opts.BalanceCapCents,
		UnlimitedBalance: opts.UnlimitedBalance,
		ModelLimits:      strings.Join(opts.ModelLimits, ","),
		AllowIPs:         strings.Join(opts.AllowIPs, "\n"),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repository.SaveUserToken(&t); err != nil {
		return model.UserToken{}, "", err
	}
	return t, raw, nil
}

// ListUserTokens 列出当前用户的 token 列表。KeyHash 字段 GORM tag 已 json:"-" 屏蔽，前端不会看到。
func ListUserTokens(userID string) ([]model.UserToken, error) {
	return repository.ListUserTokensByUser(userID)
}

// DeleteUserToken 删除当前用户自己的 token。
func DeleteUserToken(id, userID string) error {
	if strings.TrimSpace(id) == "" {
		return safeMessageError{message: "id 不能为空"}
	}
	return repository.DeleteUserToken(id, userID)
}

// SetUserTokenStatus 修改 token 状态（active/disabled）。
func SetUserTokenStatus(id, userID, status string) error {
	if strings.TrimSpace(id) == "" {
		return safeMessageError{message: "id 不能为空"}
	}
	if status != model.UserTokenStatusActive && status != model.UserTokenStatusDisabled {
		return safeMessageError{message: "status 非法（仅允许 active/disabled）"}
	}
	// 仅允许修改本人 token
	tok, err := repository.GetUserTokenByID(id)
	if err != nil {
		return safeMessageError{message: "token 不存在"}
	}
	if tok.UserID != userID {
		return safeMessageError{message: "无权操作该 token"}
	}
	return repository.UpdateUserTokenStatus(id, status)
}

// CurrentAuthUserByToken 校验 sk- token 并返回对应 user。token 本身由调用方在 service 层
// 单独查一次后注入 ctx（参见 CurrentAuthUserByTokenFull）。
func CurrentAuthUserByToken(raw, clientIP string) (model.AuthUser, bool) {
	user, _, ok := CurrentAuthUserByTokenFull(raw, clientIP)
	return user, ok
}

// CurrentAuthUserByTokenFull 校验 sk- token，返回 user + 完整 token 记录。调用方拿到 token
// 后应再调用 WithUserToken 注入 ctx，下游可用 service.UserTokenFromContext 取回。
func CurrentAuthUserByTokenFull(raw, clientIP string) (model.AuthUser, model.UserToken, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "sk-fk-") {
		return model.AuthUser{}, model.UserToken{}, false
	}
	hash := hashToken(raw)
	tok, err := repository.GetUserTokenByHash(hash)
	if err != nil || tok == nil {
		return model.AuthUser{}, model.UserToken{}, false
	}
	if tok.Status != model.UserTokenStatusActive {
		return model.AuthUser{}, model.UserToken{}, false
	}
	if tok.ExpiredAt != nil && time.Now().UTC().After(*tok.ExpiredAt) {
		// 自动转 expired（best-effort，不影响响应）
		go func(id string) {
			_ = repository.UpdateUserTokenStatus(id, model.UserTokenStatusExpired)
		}(tok.ID)
		return model.AuthUser{}, model.UserToken{}, false
	}
	if tok.AllowIPs != "" && !ipAllowed(clientIP, tok.AllowIPs) {
		return model.AuthUser{}, model.UserToken{}, false
	}
	user, ok, err := repository.GetUserByID(tok.UserID)
	if err != nil || !ok {
		return model.AuthUser{}, model.UserToken{}, false
	}
	if user.Status == model.UserStatusBan {
		return model.AuthUser{}, model.UserToken{}, false
	}
	// 异步更新 last_used（best-effort）
	go func(id, ip string) {
		_ = repository.UpdateUserTokenLastUsed(id, ip, time.Now().UTC())
	}(tok.ID, clientIP)
	return model.PublicUser(user), *tok, true
}

// hashToken 算 SHA-256 hex 字符串（64 字符）。重命名以避开 service.storage 已有的 sha256Hex。
func hashToken(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// randomURLSafe 生成 N 字节随机数并 base64url 编码（无 padding）。32 字节 → 43 字符。
func randomURLSafe(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 在 Linux 上不会失败；防御性回退到时间戳 hash
		now := time.Now().UTC().UnixNano()
		for i := 0; i < n; i++ {
			b[i] = byte(now >> (i % 8 * 8))
		}
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ipAllowed 检查 clientIP 是否在 allowList（换行分隔；支持单 IP 和 CIDR 混合）。
func ipAllowed(clientIP, allowList string) bool {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return false
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, line := range strings.Split(allowList, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "/") {
			if _, cidr, err := net.ParseCIDR(line); err == nil && cidr.Contains(ip) {
				return true
			}
		} else {
			if parsed := net.ParseIP(line); parsed != nil && parsed.Equal(ip) {
				return true
			}
		}
	}
	return false
}
