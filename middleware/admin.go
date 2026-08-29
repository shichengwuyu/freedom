package middleware

import (
	"net/http"
	"strings"

	"github.com/tigerowo/freedom/handler"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
	"github.com/gin-gonic/gin"
)

func AdminAuth(c *gin.Context) {
	user, ok := authUser(c)
	if !ok || user.Role != model.UserRoleAdmin {
		handler.FailWithStatus(c.Writer, http.StatusUnauthorized, "未登录或权限不足")
		c.Abort()
		return
	}
	c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	c.Next()
}

func UserAuth(c *gin.Context) {
	user, ok := authUser(c)
	if !ok || user.Role == model.UserRoleGuest {
		handler.FailWithStatus(c.Writer, http.StatusUnauthorized, "未登录或权限不足")
		c.Abort()
		return
	}
	c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	c.Next()
}

func OptionalAuth(c *gin.Context) {
	if user, ok := authUser(c); ok {
		c.Request = c.Request.WithContext(service.WithUser(c.Request.Context(), user))
	}
	c.Next()
}

func NotFoundJSON(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"code": 1, "data": nil, "msg": "接口不存在"})
}

// AuthCookieName 是存 JWT 的 httpOnly cookie 名（与 handler.AuthCookieName 保持一致）。
const AuthCookieName = "freedom_token"

// authUser 解析请求中的鉴权信息并返回 user。
//
// 鉴权分发顺序（Sprint 1.1 改造）：
//  0) Authorization: Bearer sk-...   → 走 service.CurrentAuthUserByTokenFull，token 注入 ctx
//  1) Cookie: freedom_token=<jwt>    → 走 service.CurrentAuthUser（JWT）
//  2) Authorization: Bearer <jwt>   → 走 service.CurrentAuthUser（兼容旧客户端）
//
// 关键点：authUser 不直接写 ctx，调用方（UserAuth/AdminAuth/OptionalAuth）负责
// service.WithUser(...)；token 注入由 service.CurrentAuthUserByTokenFull 内部完成。
func authUser(c *gin.Context) (model.AuthUser, bool) {
	auth := strings.TrimSpace(c.GetHeader("Authorization"))

	// 0) Bearer sk-...：OpenAI 兼容 sk- token 路径（Sprint 1.1 新增）
	if strings.HasPrefix(auth, "Bearer sk-") {
		raw := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		user, token, ok := service.CurrentAuthUserByTokenFull(raw, c.ClientIP())
		if !ok {
			return model.AuthUser{}, false
		}
		// 把 token 注入 ctx（user 在外层 UserAuth 注入）
		c.Request = c.Request.WithContext(service.WithUserToken(c.Request.Context(), token))
		return user, true
	}

	// 1) Cookie 优先（现有逻辑）
	token := ""
	if cookie, err := c.Cookie(AuthCookieName); err == nil && strings.TrimSpace(cookie) != "" {
		token = cookie
	}
	// 2) 回退到 Authorization: Bearer <jwt>（兼容旧客户端）
	if token == "" && strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	}
	if strings.TrimSpace(token) == "" {
		return model.AuthUser{}, false
	}
	return service.CurrentAuthUser(token)
}
