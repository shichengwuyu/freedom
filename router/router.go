package router

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/handler"
	"github.com/tigerowo/freedom/middleware"
)

func New() *gin.Engine {
	router := gin.Default()
	router.RedirectTrailingSlash = false
	// 信任常见反向代理（nginx/Cloudflare），使 c.ClientIP() 返回真实客户端 IP。
	_ = router.SetTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	// HTTP server 超时由 main.go 中的 http.Server ReadTimeout/WriteTimeout 控制，
	// 此处不再设置 per-request middleware 超时（Gin 不原生支持中断正在执行的 handler）。
	router.Use(corsMiddleware())
	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	// GET /api/model-status：各模型性能与健康状态快照（公开接口，后台每 15 分钟刷新一次）
	api.GET("/model-status", gin.WrapF(handler.ModelStatus))
	api.POST("/auth/register", gin.WrapF(handler.Register))
	api.POST("/auth/login", gin.WrapF(handler.Login))
	api.GET("/auth/linux-do/authorize", gin.WrapF(handler.LinuxDoAuthorize))
	api.GET("/auth/linux-do/callback", gin.WrapF(handler.LinuxDoCallback))
	api.GET("/auth/me", middleware.OptionalAuth, gin.WrapF(handler.CurrentUser))
	api.GET("/settings", gin.WrapF(handler.Settings))
	// Sprint 3：公开定价 API（不需登录）
	api.GET("/pricing", gin.WrapF(handler.GetPricing))
	api.GET("/storage/config", gin.WrapF(handler.StorageConfig))
	// ========== P0 新增：多供应商云端切换（公开接口）==========
	// GET /api/vendors：列出所有启用供应商（脱敏，不含 ClientSecret），无需登录
	api.GET("/vendors", gin.WrapF(handler.Vendors))
	api.GET("/media/references/:id", func(c *gin.Context) {
		handler.ReferenceMedia(c.Writer, c.Request, c.Param("id"))
	})
	api.HEAD("/media/references/:id", func(c *gin.Context) {
		handler.ReferenceMedia(c.Writer, c.Request, c.Param("id"))
	})
	api.GET("/files/:id", func(c *gin.Context) {
		handler.FileInfo(c.Writer, c.Request, c.Param("id"))
	})
	api.GET("/files/:id/content", func(c *gin.Context) {
		handler.FileContent(c.Writer, c.Request, c.Param("id"))
	})
	api.POST("/ai/direct-request", gin.WrapF(handler.PrepareDirectAIRequest))
	api.GET("/license/purchase-config", gin.WrapF(handler.LicensePurchaseConfig))
	api.GET("/announcements/latest", gin.WrapF(handler.LatestAnnouncements))
	v1 := api.Group("/v1", middleware.UserAuth)
	v1.POST("/images/generations", gin.WrapF(handler.AIImagesGenerations))
	v1.POST("/images/edits", gin.WrapF(handler.AIImagesEdits))
	v1.POST("/responses", gin.WrapF(handler.AIResponses))
	v1.POST("/chat/completions", gin.WrapF(handler.AIChatCompletions))
	v1.POST("/audio/speech", gin.WrapF(handler.AIAudioSpeech))
	// Sprint 1.1：用户自建 API Key（OpenAI 兼容 sk- 格式）CRUD
	v1.POST("/user-tokens", gin.WrapF(handler.CreateUserTokenHandler))
	v1.GET("/user-tokens", gin.WrapF(handler.ListUserTokensHandler))
	// Sprint 4：通用 task 查询（仅 Sprint 4 之后新增能力的 task；旧 4 套表各自有接口）
	v1.GET("/tasks", gin.WrapF(handler.UserTasks))
	// novel-workflow v2：工作流编排层 HTTP API
	v1.POST("/novel/workflows", gin.WrapF(handler.CreateNovelWorkflowRun))
	v1.GET("/novel/workflows", gin.WrapF(handler.ListNovelWorkflowRuns))
	v1.GET("/novel/workflows/:id", gin.WrapF(handler.GetNovelWorkflowRun))
	v1.POST("/novel/workflows/:id/start", gin.WrapF(handler.StartNovelWorkflowRun))
	v1.POST("/novel/workflows/:id/nodes/:nodeId/start", gin.WrapF(handler.StartNovelWorkflowNode))
	v1.POST("/novel/workflows/:id/nodes/:nodeId/cancel", gin.WrapF(handler.CancelNovelWorkflowNode))
	v1.POST("/novel/workflows/:id/nodes/:nodeId/retry", gin.WrapF(handler.RetryNovelWorkflowNode))
	// novel-workflow v2：shot-dubbing-node
	v1.POST("/novel/dubbing/dispatch", gin.WrapF(handler.DispatchShotDubbing))
	v1.POST("/novel/dubbing/dispatch-project", gin.WrapF(handler.DispatchProjectDubbing))
	v1.GET("/novel/dubbing", gin.WrapF(handler.ListShotDubbings))
	// novel-workflow v2：shot-subtitle-node
	v1.POST("/novel/subtitle/dispatch", gin.WrapF(handler.DispatchShotSubtitle))
	v1.POST("/novel/subtitle/dispatch-project", gin.WrapF(handler.DispatchProjectSubtitle))
	v1.PUT("/novel/subtitle/:projectId/:shotId/lines", gin.WrapF(handler.UpdateSubtitleLines))
	v1.GET("/novel/subtitle", gin.WrapF(handler.GetShotSubtitle))   // 旧路径, 单条（?projectId=&shotId=）
	v1.GET("/novel/subtitles", gin.WrapF(handler.ListShotSubtitles)) // 列表（?projectId=）
	// novel-workflow v2：bgm-layer
	v1.GET("/bgm/presets", gin.WrapF(handler.ListBgmPresets))    // 公开
	v1.GET("/bgm/custom", gin.WrapF(handler.ListBgmCustoms))
	v1.POST("/bgm/custom/upload", gin.WrapF(handler.UploadBgmCustom))
	v1.DELETE("/bgm/custom/:id", gin.WrapF(handler.DeleteBgmCustom))
	// novel-workflow v2：composition-layer
	v1.POST("/novel/composition", gin.WrapF(handler.CreateCompositionTask))
	v1.GET("/novel/composition", gin.WrapF(handler.ListCompositionTasks))
	v1.GET("/novel/composition/:id", gin.WrapF(handler.GetCompositionTask))
	v1.POST("/novel/composition/:id/start", gin.WrapF(handler.StartCompositionTask))
	v1.POST("/novel/composition/:id/stop", gin.WrapF(handler.StopCompositionTask))
	v1.POST("/novel/composition/:id/retry", gin.WrapF(handler.RetryCompositionTask))
	v1.DELETE("/user-tokens/:id", func(c *gin.Context) {
		handler.DeleteUserTokenHandler(c.Writer, c.Request)
	})
	v1.POST("/user-tokens/:id/disable", gin.WrapF(handler.SetUserTokenStatusHandler("disabled")))
	v1.POST("/user-tokens/:id/enable", gin.WrapF(handler.SetUserTokenStatusHandler("active")))
	v1.POST("/canvas/tasks/delete", gin.WrapF(handler.DeleteUserCanvasTasks))
	v1.POST("/canvas/image-tasks", gin.WrapF(handler.CreateCanvasImageTask))
	v1.GET("/canvas/image-tasks", gin.WrapF(handler.UserCanvasImageTasks))
	v1.POST("/canvas/image-tasks/status", gin.WrapF(handler.BatchCanvasImageTasks))
	v1.GET("/canvas/image-tasks/:id", func(c *gin.Context) {
		handler.GetCanvasImageTask(c.Writer, c.Request, c.Param("id"))
	})
	v1.DELETE("/canvas/image-tasks/:id", func(c *gin.Context) {
		handler.DeleteUserCanvasImageTask(c.Writer, c.Request, c.Param("id"))
	})
	v1.POST("/canvas/audio-tasks", gin.WrapF(handler.CreateCanvasAudioTask))
	v1.GET("/canvas/audio-tasks/:id", func(c *gin.Context) {
		handler.GetCanvasAudioTask(c.Writer, c.Request, c.Param("id"))
	})
	v1.POST("/ai-logs", gin.WrapF(handler.ClientAICallLog))
	v1.POST("/videos", gin.WrapF(handler.AIVideos))
	v1.GET("/video-tasks", gin.WrapF(handler.UserVideoTasks))
	v1.DELETE("/video-tasks/:id", func(c *gin.Context) {
		handler.DeleteUserVideoTask(c.Writer, c.Request, c.Param("id"))
	})
	v1.POST("/storyboard-tasks", gin.WrapF(handler.CreateStoryboardTaskHandler))
	v1.GET("/storyboard-tasks", gin.WrapF(handler.ListUserStoryboardTasksHandler))
	v1.GET("/storyboard-tasks/:id", func(c *gin.Context) {
		handler.GetStoryboardTaskHandler(c.Writer, c.Request, c.Param("id"))
	})
	v1.DELETE("/storyboard-tasks/:id", func(c *gin.Context) {
		handler.DeleteUserStoryboardTaskHandler(c.Writer, c.Request, c.Param("id"))
	})
	// POST /api/v1/storyboard-tasks/:id/cancel — 用户中途停止时分镜任务，标记 canceled；worker 跳过已标记任务
	v1.POST("/storyboard-tasks/:id/cancel", func(c *gin.Context) {
		handler.CancelStoryboardTaskHandler(c.Writer, c.Request, c.Param("id"))
	})
	v1.POST("/media/references", gin.WrapF(handler.UploadReferenceMedia))
	v1.GET("/videos/:id", func(c *gin.Context) {
		handler.AIVideo(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/videos/:id/content", func(c *gin.Context) {
		handler.AIVideoContent(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/workflows", gin.WrapF(handler.UserWorkflows))
	v1.POST("/workflows", gin.WrapF(handler.SaveUserWorkflow))
	v1.POST("/workflows/agent-draft", gin.WrapF(handler.DraftUserWorkflow))
	v1.DELETE("/workflows/:id", func(c *gin.Context) {
		handler.DeleteUserWorkflow(c.Writer, c.Request, c.Param("id"))
	})
	v1.POST("/storage/measure", gin.WrapF(handler.MeasureUserStorageProvider))
	v1.POST("/files", gin.WrapF(handler.UploadFile))
	v1.DELETE("/files/:id", func(c *gin.Context) {
		handler.DeleteFile(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/user-config", gin.WrapF(handler.UserConfig))
	v1.POST("/user-config/model", gin.WrapF(handler.SaveUserModelConfig))
	v1.POST("/user-config/storage", gin.WrapF(handler.SaveUserStorageProvider))
	v1.POST("/user/profile", gin.WrapF(handler.UpdateMyProfile))
	v1.GET("/canvas/projects", gin.WrapF(handler.UserCanvasProjects))
	v1.POST("/canvas/projects", gin.WrapF(handler.SaveUserCanvasProject))
	v1.POST("/canvas/projects/sync", gin.WrapF(handler.SyncUserCanvasProjects))
	v1.POST("/canvas/projects/delete", gin.WrapF(handler.DeleteUserCanvasProjects))
	v1.GET("/user-data/image-history", gin.WrapF(handler.UserImageHistory))
	v1.POST("/user-data/image-history", gin.WrapF(handler.SaveUserImageHistory))
	v1.GET("/generation-logs/videos", gin.WrapF(handler.UserVideoGenerationLogs))
	v1.POST("/generation-logs/videos", gin.WrapF(handler.SaveUserVideoGenerationLogs))
	v1.POST("/generation-logs/videos/delete", gin.WrapF(handler.DeleteUserVideoGenerationLogs))
	v1.DELETE("/generation-logs/videos/:id", func(c *gin.Context) {
		handler.DeleteUserVideoGenerationLog(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/generation-logs/images", gin.WrapF(handler.UserImageGenerationLogs))
	v1.POST("/generation-logs/images", gin.WrapF(handler.SaveUserImageGenerationLogs))
	v1.POST("/generation-logs/images/delete", gin.WrapF(handler.DeleteUserImageGenerationLogs))
	v1.DELETE("/generation-logs/images/:id", func(c *gin.Context) {
		handler.DeleteUserImageGenerationLog(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/user-data/assets", gin.WrapF(handler.UserAssetData))
	v1.POST("/user-data/assets", gin.WrapF(handler.SaveUserAssetData))
	v1.POST("/license/redeem", gin.WrapF(handler.RedeemLicenseKey))
	v1.GET("/license/redeem-logs", gin.WrapF(handler.MyRedeemLogs))
	v1.GET("/balance-logs", gin.WrapF(handler.MyBalanceLogs))
	v1.GET("/affiliate/info", gin.WrapF(handler.MyAffiliateInfo))
	v1.GET("/affiliate/commissions", gin.WrapF(handler.MyAffiliateCommissions))
	// ========== P0 新增：用户级供应商接口（需登录，放在 /api/v1 组下 UserAuth）==========
	// GET  /api/v1/vendor/accounts：当前用户绑定的供应商账户列表（脱敏）
	v1.GET("/vendor/accounts", gin.WrapF(handler.VendorAccounts))
	// POST /api/v1/vendor/activate：切换激活供应商（body: { vendorType }）
	v1.POST("/vendor/activate", gin.WrapF(handler.ActivateVendor))
	// POST /api/v1/vendor/bind-cookie：用 AccessToken / Cookie / AccessKey 绑定供应商账户
	v1.POST("/vendor/bind-cookie", gin.WrapF(handler.BindVendorByCookie))
	// POST /api/v1/vendor/refresh-models：手动刷新某家供应商的可用模型快照
	v1.POST("/vendor/refresh-models", gin.WrapF(handler.VendorRefreshModels))
	// POST /api/v1/vendor/refresh-balance：手动刷新某家供应商的余额/套餐快照
	v1.POST("/vendor/refresh-balance", gin.WrapF(handler.VendorRefreshBalance))
	// POST /api/v1/vendor/unbind：解绑某家供应商账户（回落官方模式）
	v1.POST("/vendor/unbind", gin.WrapF(handler.VendorUnbind))
	// POST /api/v1/vendor/capture-sample：浏览器插件嗅探到的内部接口样本回传（UpDream/NewWow 用）
	v1.POST("/vendor/capture-sample", gin.WrapF(handler.VendorCaptureSample))
	// GET  /api/v1/vendor/samples：列出当前用户被嗅探到的样本（调试用）
	v1.GET("/vendor/samples", gin.WrapF(handler.VendorListSamples))
	// POST /api/v1/vendor/clear-samples：清空样本（body: { vendorType }，空=清空全部）
	v1.POST("/vendor/clear-samples", gin.WrapF(handler.VendorClearSamples))
	// POST /api/v1/vendor/estimate-cost：实时估算当前参数组合的扣费额度（body: { vendorType, capability, model, quality, size, count, refImageCount, refVideoCount, hasSound }）
	v1.POST("/vendor/estimate-cost", gin.WrapF(handler.VendorEstimateCost))
	api.GET("/proxy-image", gin.WrapF(handler.ProxyImage))
	api.GET("/proxy-media", gin.WrapF(handler.ProxyMedia))
	api.GET("/prompts", middleware.OptionalAuth, gin.WrapF(handler.Prompts))
	api.POST("/prompts/submit", middleware.UserAuth, gin.WrapF(handler.SubmitPrompt))
	api.GET("/assets", middleware.OptionalAuth, gin.WrapF(handler.Assets))
	api.POST("/admin/login", gin.WrapF(handler.AdminLogin))

	admin := api.Group("/admin", middleware.AdminAuth)
	admin.GET("/users", gin.WrapF(handler.AdminUsers))
	admin.POST("/users", gin.WrapF(handler.AdminSaveUser))
	admin.POST("/users/:id/balance", func(c *gin.Context) {
		handler.AdminAdjustUserBalance(c.Writer, c.Request, c.Param("id"))
	})
	admin.DELETE("/users/:id", func(c *gin.Context) {
		handler.AdminDeleteUser(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/balance-logs", gin.WrapF(handler.AdminBalanceLogs))
	// 后台调整用户余额到目标值（POST 而非手写流水，保证余额与流水一致）。
	admin.POST("/balance-logs", gin.WrapF(handler.AdminSaveBalanceLog))
	admin.GET("/ai-logs/dates", gin.WrapF(handler.AdminAICallLogDates))
	admin.GET("/ai-logs", gin.WrapF(handler.AdminAICallLogs))
	// Sprint 2：渠道失败诊断（最近 100 条）
	admin.GET("/channel-fail-logs", gin.WrapF(handler.AdminChannelFailLogs))
	// Sprint 2.6：渠道健康度汇总（一次性返回 KPI + 渠道统计 + 最近失败）
	admin.GET("/channels-health", gin.WrapF(handler.AdminChannelsHealth))
	admin.POST("/channels-health/clear-cooldowns", gin.WrapF(handler.AdminClearCooldowns))
	admin.DELETE("/ai-logs/by-dates", gin.WrapF(handler.AdminDeleteAICallLogsByDates))
	admin.DELETE("/ai-logs", gin.WrapF(handler.AdminDeleteAICallLogs))
	admin.GET("/settings", gin.WrapF(handler.AdminSettings))
	admin.POST("/settings", gin.WrapF(handler.AdminSaveSettings))
	admin.POST("/settings/channel-models", gin.WrapF(handler.AdminChannelModels))
	admin.POST("/settings/channel-test", gin.WrapF(handler.AdminTestChannelModel))
	admin.POST("/storage/measure", gin.WrapF(handler.AdminMeasureStorageProvider))
	admin.GET("/prompt-categories", gin.WrapF(handler.AdminPromptCategories))
	admin.POST("/prompt-categories/sync", gin.WrapF(handler.AdminSyncPromptCategories))
	admin.POST("/prompt-categories/sync-all", gin.WrapF(handler.AdminSyncAllPromptCategories))
	admin.GET("/prompts", gin.WrapF(handler.AdminPrompts))
	admin.POST("/prompts", gin.WrapF(handler.AdminSavePrompt))
	admin.POST("/prompts/batch-delete", gin.WrapF(handler.AdminDeletePrompts))
	admin.DELETE("/prompts/:id", func(c *gin.Context) {
		handler.AdminDeletePrompt(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/prompts/pending", gin.WrapF(handler.AdminPendingPrompts))
	admin.GET("/prompts/rejected", gin.WrapF(handler.AdminRejectedPrompts))
	admin.POST("/prompts/:id/approve", func(c *gin.Context) {
		handler.AdminApprovePrompt(c.Writer, c.Request, c.Param("id"))
	})
	admin.POST("/prompts/:id/reject", func(c *gin.Context) {
		handler.AdminRejectPrompt(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/assets", gin.WrapF(handler.AdminAssets))
	admin.POST("/assets", gin.WrapF(handler.AdminSaveAsset))
	admin.DELETE("/assets/:id", func(c *gin.Context) {
		handler.AdminDeleteAsset(c.Writer, c.Request, c.Param("id"))
	})
	admin.POST("/license-keys/import", gin.WrapF(handler.AdminImportLicenseKeys))
	admin.GET("/license-keys", gin.WrapF(handler.AdminListLicenseKeys))
	admin.GET("/license-redeem-logs", gin.WrapF(handler.AdminListRedeemLogs))
	admin.POST("/license-keys/batch-face-value", gin.WrapF(handler.AdminModifyBatchFaceValue))
	admin.POST("/license-keys/generate", gin.WrapF(handler.AdminGenerateLicenseKeys))
	admin.GET("/license-keys/export", gin.WrapF(handler.AdminExportLicenseKeys))
	admin.GET("/announcements", gin.WrapF(handler.AdminListAnnouncements))
	admin.POST("/announcements", gin.WrapF(handler.AdminSaveAnnouncement))
	admin.POST("/announcements/delete", gin.WrapF(handler.AdminDeleteAnnouncement))

	router.NoRoute(middleware.NotFoundJSON)

	return router
}

// corsMiddleware 处理跨域请求。
// 安全策略：仅允许配置的 CORS_ALLOWED_ORIGINS（逗号分隔）中的 Origin；
// 未配置时仅允许 localhost 开发环境来源，防止跨站凭证窃取。
func corsMiddleware() gin.HandlerFunc {
	allowed := parseAllowedOrigins()
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && isAllowedOrigin(origin, allowed) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS, HEAD")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id, X-Model-Channel-ID, X-User-Model-Channel-ID")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func parseAllowedOrigins() map[string]bool {
	allowed := map[string]bool{
		"http://localhost:3000": true,
		"http://127.0.0.1:3000": true,
	}
	// 从环境变量 CORS_ALLOWED_ORIGINS 读取额外的允许来源
	if extra := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowed[o] = true
			}
		}
	}
	// 如果配置了 PUBLIC_BASE_URL，自动加入
	if base := strings.TrimSpace(config.Cfg.PublicBaseURL); base != "" {
		allowed[base] = true
	}
	return allowed
}

func isAllowedOrigin(origin string, allowed map[string]bool) bool {
	if allowed[origin] {
		return true
	}
	// 仅在未配置 PUBLIC_BASE_URL 时允许 localhost 任意端口（开发环境）。
	// 生产环境配置了 PUBLIC_BASE_URL 后，不再放行 localhost，防止本机其他服务携带 cookie 发起跨域请求。
	if strings.TrimSpace(config.Cfg.PublicBaseURL) == "" &&
		(strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:")) {
		return true
	}
	return false
}
