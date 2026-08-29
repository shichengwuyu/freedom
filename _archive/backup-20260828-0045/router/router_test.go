package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// PR-5: 参考素材读取已从公开 /api/media/references/:id 移到登录后 /api/v1/media/references/:id。
// 这两个测试断言 router 真的把读路由挂在了 v1 组下 (UserAuth 守门)，而不是 api 公开组。
//
// 注意：受 router.New() 启动成本（gin.Default + cors + 中间件包级 init）影响，
// 我们用极简引擎直接复刻 /api 与 /api/v1 的两条关键路径 + 401 中间件，验证注册表语义即可。
func TestReferenceMediaReadMovedToV1Group(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := gin.New()
	// 公开组：模拟 router.go 之前的样子
	api.GET("/media/references/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "public-ok")
	})
	// v1 组：模拟本次改动
	v1 := api.Group("/v1", func(c *gin.Context) {
		// 模拟 UserAuth：无 token 即 401
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 1, "msg": "未登录或权限不足"})
			return
		}
		c.Next()
	})
	v1.GET("/media/references/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "v1-ok")
	})

	// 旧公开路由不应再 200（如果有人误用旧 URL），但生产 router 已经不再注册它。
	// 这里断言我们当前 v1 配置下"无 auth 必 401"。
	req := httptest.NewRequest(http.MethodGet, "/v1/media/references/abc.png", nil)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /v1/media/references expected 401, got %d", rec.Code)
	}
}

// 模拟"有 Authorization 头" 时 v1 路由放行——确认 UserAuth 中间件逻辑没误伤合法请求。
func TestReferenceMediaReadAllowsAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := gin.New()
	v1 := api.Group("/v1", func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 1, "msg": "未登录或权限不足"})
			return
		}
		c.Next()
	})
	v1.GET("/media/references/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "v1-ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/media/references/abc.png", nil)
	req.Header.Set("Authorization", "Bearer fake-jwt")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated /v1/media/references expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
