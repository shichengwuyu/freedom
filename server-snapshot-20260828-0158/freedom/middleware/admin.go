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

func authUser(c *gin.Context) (model.AuthUser, bool) {
	// 优先从 httpOnly cookie 读取 token（安全），回退到 Authorization header（兼容旧客户端）。
	token := ""
	if cookie, err := c.Cookie(AuthCookieName); err == nil && strings.TrimSpace(cookie) != "" {
		token = cookie
	}
	if token == "" {
		token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	if strings.TrimSpace(token) == "" {
		return model.AuthUser{}, false
	}
	return service.CurrentAuthUser(token)
}
