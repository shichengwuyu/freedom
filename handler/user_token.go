package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tigerowo/freedom/service"
)

// 路由：
// POST   /api/v1/user-tokens                创建（明文仅这一次返回）
// GET    /api/v1/user-tokens                列表（不含 key，仅 KeyPrefix 脱敏）
// DELETE /api/v1/user-tokens/:id            删除
// POST   /api/v1/user-tokens/:id/disable    禁用
// POST   /api/v1/user-tokens/:id/enable     启用

type createUserTokenRequest struct {
	Name             string     `json:"name"`
	ExpiredAt        *time.Time `json:"expiredAt,omitempty"`
	BalanceCapCents  int        `json:"balanceCapCents,omitempty"`
	UnlimitedBalance bool       `json:"unlimitedBalance"`
	ModelLimits      []string   `json:"modelLimits,omitempty"`
	AllowIPs         []string   `json:"allowIps,omitempty"`
}

// CreateUserTokenHandler 创建 sk- token。明文仅 raw 字段一次性返回。
func CreateUserTokenHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	var req createUserTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		Fail(w, "名称不能为空")
		return
	}
	if len(name) > 50 {
		Fail(w, "名称过长（最长 50 字符）")
		return
	}
	if req.BalanceCapCents < 0 {
		Fail(w, "BalanceCapCents 不能为负")
		return
	}
	if len(req.ModelLimits) > 50 || len(req.AllowIPs) > 50 {
		Fail(w, "白名单条目数过多（>50）")
		return
	}
	token, raw, err := service.CreateUserToken(user.ID, name, service.UserTokenOptions{
		ExpiredAt:        req.ExpiredAt,
		BalanceCapCents:  req.BalanceCapCents,
		UnlimitedBalance: req.UnlimitedBalance,
		ModelLimits:      req.ModelLimits,
		AllowIPs:         req.AllowIPs,
	})
	if err != nil {
		FailError(w, err)
		return
	}
	// 把 keyPrefix 脱敏后再返给前端
	token.KeyPrefix = modelMask(token.KeyPrefix)
	OK(w, map[string]any{
		"token": token,
		"raw":   raw, // 仅此一次返回！前端必须明确提示用户保存
	})
}

// ListUserTokensHandler 列出当前用户所有 token。KeyHash 字段 GORM tag json:"-" 已屏蔽。
func ListUserTokensHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	tokens, err := service.ListUserTokens(user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	// 双保险：清空 hash 并脱敏 keyPrefix
	for i := range tokens {
		tokens[i].KeyHash = ""
		tokens[i].KeyPrefix = modelMask(tokens[i].KeyPrefix)
	}
	OK(w, map[string]any{"items": tokens, "total": len(tokens)})
}

// DeleteUserTokenHandler 解析 :id 并删除。仅允许删自己名下。
func DeleteUserTokenHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "未登录")
		return
	}
	id := extractUserTokenID(r.URL.Path)
	if id == "" {
		Fail(w, "id 不能为空")
		return
	}
	if err := service.DeleteUserToken(id, user.ID); err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"ok": true})
}

// SetUserTokenStatusHandler 生成 /disable /enable 处理器。
func SetUserTokenStatusHandler(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := service.UserFromContext(r.Context())
		if !ok {
			FailWithStatus(w, http.StatusUnauthorized, "未登录")
			return
		}
		id := extractUserTokenID(r.URL.Path)
		if id == "" {
			Fail(w, "id 不能为空")
			return
		}
		if err := service.SetUserTokenStatus(id, user.ID, status); err != nil {
			FailError(w, err)
			return
		}
		OK(w, map[string]any{"ok": true})
	}
}

// extractUserTokenID 从 URL 路径解析 :id。
// 路径格式：/api/v1/user-tokens/<id>[/disable|/enable]
func extractUserTokenID(path string) string {
	const prefix = "/api/v1/user-tokens/"
	idx := strings.Index(path, prefix)
	if idx < 0 {
		return ""
	}
	rest := path[idx+len(prefix):]
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

// modelMask 给前端展示用的打码（首 7 + "..." + 末 4），内联避免在 handler 引用 model 包细节。
// 与 model.MaskUserTokenKey 行为一致；这里独立一份防止 handler → model 引入额外依赖。
func modelMask(s string) string {
	if len(s) <= 11 {
		return s
	}
	return s[:7] + "..." + s[len(s)-4:]
}
