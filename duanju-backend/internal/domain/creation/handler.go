// creation HTTP 入口：项目 CRUD + 拆分触发 + 层级读（05 TAG 3 · 3.0.x/3.1.x 子集）。
package creation

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// Handler creation 路由处理器。
type Handler struct {
	svc        *Service
	assetSvc   *AssetService
	exportSvc  *ExportService
	canvasSvc  *CanvasService
	memberSvc  *MembershipService
	contentSvc *ContentService
	marketSvc  *MarketService
	adminSvc   *AdminService
	openSvc    *OpenAPIService
}

func NewHandler(svc *Service, assetSvc *AssetService, exportSvc *ExportService, canvasSvc *CanvasService, memberSvc *MembershipService, contentSvc *ContentService, marketSvc *MarketService, adminSvc *AdminService, openSvc *OpenAPIService) *Handler {
	return &Handler{svc: svc, assetSvc: assetSvc, exportSvc: exportSvc, canvasSvc: canvasSvc, memberSvc: memberSvc, contentSvc: contentSvc, marketSvc: marketSvc, adminSvc: adminSvc, openSvc: openSvc}
}

// RegisterRoutes 注册 /projects /episodes /beats 路由（JWT + LoadUserID 由 router 挂载）。
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, jwtMW, loadUserMW gin.HandlerFunc) {
	auth := []gin.HandlerFunc{jwtMW, loadUserMW}
	projects := rg.Group("/projects", auth...)
	{
		projects.GET("", h.list)
		projects.POST("", h.create)
		projects.GET("/:project_id", h.get)
		projects.PUT("/:project_id", h.update)
		projects.DELETE("/:project_id", h.delete)
		projects.POST("/:project_id", h.restore)
		projects.POST("/:project_id/last-opened", h.touchLastOpened)
		projects.POST("/:project_id/split-script", h.splitScript)
		projects.GET("/:project_id/episodes", h.listEpisodes)
	}
	episodes := rg.Group("/episodes", auth...)
	{
		episodes.GET("/:episode_id/beats", h.listBeats)
		episodes.GET("/:episode_id/characters", h.listCharacters)
	}
	uploads := rg.Group("/uploads", auth...)
	{
		uploads.POST("/presign", h.presign)
		uploads.POST("/complete", h.complete)
	}
	assets := rg.Group("/assets", auth...)
	{
		assets.POST("/batch-generate", h.batchGenerate)
	}
	// 9.3.1 生成消耗预览（S2-B06 跨集复用豁免）
	preview := rg.Group("/points", auth...)
	{
		preview.POST("/preview", h.previewPoints)
	}
	// 多变体（S2-B04）：3.2.6 创建变体 / 3.2.7 变体列表
	chars := rg.Group("/characters", auth...)
	{
		chars.GET("/:character_id/variants", h.listVariants)
		chars.POST("/:character_id/variants", h.createVariant)
	}
	// 版本回滚（S2-B05）
	nus := rg.Group("/narrative-units", auth...)
	{
		nus.GET("/:nu_id/versions", h.listVersions)
		nus.POST("/:nu_id/rollback/:version_id", h.rollbackVersion)
	}
	exportGroup := rg.Group("/projects", auth...)
	{
		exportGroup.POST("/:project_id/export", h.triggerExport)
	}
	exportList := rg.Group("/exports", auth...)
	{
		exportList.GET("", h.listExports)
		exportList.GET("/:export_id", h.exportDetail)
	}
	// Step3 视频（S1-F05）：3.3.7a 批量生成镜头块视频
	beats := rg.Group("/beats", auth...)
	{
		beats.POST("/:beat_id/generate-videos", h.generateVideos)
		beats.GET("/:beat_id/shots", h.listBeatShots)
	}
	// 自由画布（S2-B07）：TAG 4.1~4.2 + 4.4 执行
	canvases := rg.Group("/canvases", auth...)
	{
		canvases.POST("", h.createCanvas)
		canvases.GET("/:canvas_id", h.loadCanvas)
		canvases.PUT("/:canvas_id", h.saveCanvas)
		canvases.PUT("/:canvas_id/nodes", h.saveNodes)
		canvases.POST("/:canvas_id/execute", h.executeCanvas)
		canvases.GET("/:canvas_id/snapshots", h.listSnapshots)                    // S2-F03 快照列表
		canvases.POST("/:canvas_id/snapshots/:snapshot_id/restore", h.restoreSnapshot) // 恢复
	}
	// 月卡会员（S2-B09）：9.9 购买/查询 + 9.10 取消 + 4.5 SSE（S2-B10）
	mems := rg.Group("/memberships", auth...)
	{
		mems.POST("/purchase", h.membershipPurchase)
		mems.GET("", h.membershipDetail)
		mems.POST("/cancel", h.membershipCancel)
	}
	rg.POST("/memberships/orders/:order_no/activate", h.membershipActivate)
	rg.GET("/ai/dialog/stream", h.aiDialogSSE)
	// 内容域（S3-B01~B03）：信息流/作品/互动/审核
	feed := rg.Group("/works", auth...)
	{
		feed.GET("", h.contentFeed)
		feed.POST("", h.contentSubmit)
		feed.GET("/:work_id", h.contentWork)
		feed.POST("/:work_id/like", h.contentLike)
		feed.DELETE("/:work_id/like", h.contentUnLike)
		feed.POST("/:work_id/favorite", h.contentFavorite)
		feed.POST("/:work_id/comments", h.contentComment)
		feed.POST("/:work_id/report", h.contentReport)
	}
	// 关注（S3-B02 补齐）
	users := rg.Group("/users", auth...)
	{
		users.POST("/:user_uuid/follow", h.contentFollow)
		users.DELETE("/:user_uuid/follow", h.contentUnfollow)
	}
	rg.GET("/audit/works", h.contentAuditList) // 11.15 待审列表（审核员）
	rg.POST("/audit/works/:work_id/review", h.contentReview)      // 11.16 审核
	rg.POST("/audit/works/:work_id/revoke", h.contentRevoke)      // 11.17 下架
	// 生态（S3-B04~B06）：模板/风格/任务
	tpls := rg.Group("/templates", auth...)
	{
		tpls.GET("", h.marketTemplates)
		tpls.POST("", h.marketCreateTemplate)
		tpls.POST("/:template_id/apply", h.marketApplyTemplate)
	}
	styles := rg.Group("/style-library", auth...)
	{
		styles.GET("", h.marketStyles)
	}
	rg.POST("/projects/:project_id/apply-style", h.marketApplyStyle)
	tasks := rg.Group("/tasks", auth...)
	{
		tasks.GET("", h.marketOrders)
		tasks.POST("", h.marketCreateOrder)
		tasks.POST("/:order_id/intents", h.marketCreateIntent)
		tasks.POST("/:order_id/accept", h.marketAcceptIntent)
		tasks.POST("/:order_id/status", h.marketUpdateOrderStatus)
	}
	// 管理后台（S3-B07/B08）：TAG 11
	adm := rg.Group("/admin")
	{
		adm.GET("/dashboard", h.adminDashboard)                       // 11.0 指标（公开 mock）
		admAuth := adm.Group("", auth...)
		{
			admAuth.GET("/users", h.adminUsers)                       // 11.2 用户列表
			admAuth.PATCH("/users/:user_uuid/ban", h.adminBanUser)     // 11.5 封禁
			admAuth.GET("/works", h.adminWorks)                        // 内容列表
			admAuth.DELETE("/works/:work_id", h.adminDeleteWork)       // 11.73 强制删除
			admAuth.GET("/reports", h.adminReports)                    // 11.74 举报列表
			admAuth.GET("/orders", h.adminOrders)                    // P2 订单管理
			admAuth.GET("/packages", h.adminPackages)                // P2 套餐管理
			admAuth.GET("/models", h.adminModels)                    // P2 模型管理
			admAuth.GET("/audit-logs", h.adminAuditLogs)             // P2 审计日志
		admAuth.GET("/roles", h.adminRoles)                     // P2 角色列表
		admAuth.GET("/platform-config", h.adminPlatformConfig)   // P2 平台配置
		admAuth.PUT("/platform-config", h.adminUpdatePlatformConfig) // P2 更新配置
		admAuth.PUT("/packages", h.adminUpdatePackage)           // P2 更新套餐
		admAuth.PUT("/models", h.adminUpdateModel)               // P2 更新模型
		admAuth.GET("/announcements", h.adminAnnouncements)      // P2 公告列表
		admAuth.GET("/banners", h.adminBanners)                  // P2 Banner 列表
		admAuth.GET("/hints", h.adminHints)                      // P2 提示列表
	}
	}
	// 开发者（S3-B09）：API Key + Webhook
	dev := rg.Group("/api-keys", auth...)
	{
		dev.GET("", h.openListKeys)
		dev.POST("", h.openCreateKey)
		dev.POST("/:key_prefix/revoke", h.openRevokeKey)
	}
	wh := rg.Group("/webhooks", auth...)
	{
		wh.POST("", h.openRegisterWebhook)
	}
	rg.POST("/webhooks/verify", h.openVerifyWebhook)
	// 个人偏好
	me := rg.Group("/me", auth...)
	{
		me.GET("/preferences", h.myPreferences)
		me.PUT("/preferences", h.updateMyPreferences)
	}
}

func (h *Handler) list(c *gin.Context) {
	h.svc.List(c)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	h.svc.Create(c, &req)
}

func (h *Handler) get(c *gin.Context) {
	h.svc.Get(c, c.Param("project_id"))
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateProjectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	h.svc.Update(c, c.Param("project_id"), &req)
}

func (h *Handler) delete(c *gin.Context) {
	h.svc.Delete(c, c.Param("project_id"))
}

func (h *Handler) restore(c *gin.Context) {
	h.svc.Restore(c, c.Param("project_id"))
}

func (h *Handler) touchLastOpened(c *gin.Context) {
	h.svc.TouchLastOpened(c, c.Param("project_id"))
}

func (h *Handler) splitScript(c *gin.Context) {
	h.svc.SplitScript(c, c.Param("project_id"))
}

func (h *Handler) listEpisodes(c *gin.Context) {
	h.svc.ListEpisodes(c, c.Param("project_id"))
}

func (h *Handler) listBeats(c *gin.Context) {
	h.svc.ListBeats(c, c.Param("episode_id"))
}

func (h *Handler) listCharacters(c *gin.Context) {
	// 从 episode_id 反查 project_id（路由注册为 /episodes/:episode_id/characters）
	epUUID := c.Param("episode_id")
	epID, projectID, err := h.svc.repo.ResolveEpisode(c.Request.Context(), epUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在")
		return
	}
	_ = epID
	projectUUID, err := h.svc.repo.ResolveProjectUUID(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在")
		return
	}
	h.assetSvc.ListCharacters(c, projectUUID)
}

func (h *Handler) presign(c *gin.Context) {
	var req UploadPresignReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	h.assetSvc.Presign(c, &req)
}

func (h *Handler) complete(c *gin.Context) {
	var req UploadCompleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	h.assetSvc.Complete(c, &req)
}

func (h *Handler) batchGenerate(c *gin.Context) {
	var req BatchGenerateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	h.assetSvc.BatchGenerate(c, &req)
}

func (h *Handler) previewPoints(c *gin.Context) {
	h.assetSvc.PreviewPoints(c)
}

func (h *Handler) triggerExport(c *gin.Context) {
	h.exportSvc.Trigger(c, c.Param("project_id"))
}

func (h *Handler) listExports(c *gin.Context) {
	h.exportSvc.List(c)
}

func (h *Handler) exportDetail(c *gin.Context) {
	h.exportSvc.Detail(c, c.Param("export_id"))
}

func (h *Handler) generateVideos(c *gin.Context) {
	h.svc.GenerateVideos(c, c.Param("beat_id"))
}

func (h *Handler) listVariants(c *gin.Context) {
	h.assetSvc.ListVariants(c, c.Param("character_id"))
}

func (h *Handler) listBeatShots(c *gin.Context) {
	h.svc.ListBeatShots(c, c.Param("beat_id"))
}

func (h *Handler) createVariant(c *gin.Context) {
	h.assetSvc.CreateVariant(c, c.Param("character_id"))
}

func (h *Handler) listVersions(c *gin.Context) {
	h.assetSvc.ListVersions(c, c.Param("nu_id"))
}

func (h *Handler) rollbackVersion(c *gin.Context) {
	h.assetSvc.RollbackVersion(c, c.Param("nu_id"), c.Param("version_id"))
}

func (h *Handler) createCanvas(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		SpaceID string `json:"space_id"`
		Name    string `json:"name" binding:"required,min=1,max=128"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	spaceID, _, err := h.svc.repo.DefaultSpaceID(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取空间失败")
		return
	}
	uuid, err := h.canvasSvc.store.CreateCanvas(c.Request.Context(), spaceID, 0, uid, req.Name)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建画布失败")
		return
	}
	httpx.OK(c, gin.H{"canvas_id": uuid})
}

func (h *Handler) loadCanvas(c *gin.Context) {
	h.canvasSvc.Load(c, c.Param("canvas_id"))
}

func (h *Handler) saveCanvas(c *gin.Context) {
	h.canvasSvc.Save(c, c.Param("canvas_id"))
}

func (h *Handler) saveNodes(c *gin.Context) {
	h.canvasSvc.SaveNodes(c, c.Param("canvas_id"))
}

func (h *Handler) executeCanvas(c *gin.Context) {
	h.canvasSvc.Execute(c, c.Param("canvas_id"))
}

func (h *Handler) membershipPurchase(c *gin.Context) {
	h.memberSvc.Purchase(c)
}

func (h *Handler) membershipDetail(c *gin.Context) {
	h.memberSvc.Detail(c)
}

func (h *Handler) membershipCancel(c *gin.Context) {
	h.memberSvc.Cancel(c)
}

func (h *Handler) membershipActivate(c *gin.Context) {
	h.memberSvc.Activate(c, c.Param("order_no"))
}

func (h *Handler) aiDialogSSE(c *gin.Context) {
	h.memberSvc.AIDialogSSE(c)
}

func (h *Handler) listSnapshots(c *gin.Context) {
	h.canvasSvc.ListSnapshots(c, c.Param("canvas_id"))
}

func (h *Handler) restoreSnapshot(c *gin.Context) {
	h.canvasSvc.RestoreSnapshot(c, c.Param("canvas_id"), c.Param("snapshot_id"))
}

func (h *Handler) contentFeed(c *gin.Context) { h.contentSvc.Feed(c) }
func (h *Handler) contentSubmit(c *gin.Context) { h.contentSvc.Submit(c) }
func (h *Handler) contentWork(c *gin.Context) { h.contentSvc.Work(c, c.Param("work_id")) }
func (h *Handler) contentLike(c *gin.Context) { h.contentSvc.Like(c, c.Param("work_id")) }
func (h *Handler) contentUnLike(c *gin.Context) { h.contentSvc.UnLike(c, c.Param("work_id")) }
func (h *Handler) contentFavorite(c *gin.Context) { h.contentSvc.Favorite(c, c.Param("work_id")) }
func (h *Handler) contentComment(c *gin.Context) { h.contentSvc.Comment(c, c.Param("work_id")) }
func (h *Handler) contentReport(c *gin.Context) { h.contentSvc.Report(c, c.Param("work_id")) }
func (h *Handler) contentFollow(c *gin.Context) { h.contentSvc.Follow(c, c.Param("user_uuid")) }
func (h *Handler) contentUnfollow(c *gin.Context) { h.contentSvc.Unfollow(c, c.Param("user_uuid")) }
func (h *Handler) contentAuditList(c *gin.Context) { h.contentSvc.AuditList(c) }
func (h *Handler) contentReview(c *gin.Context) { h.contentSvc.Review(c, c.Param("work_id")) }
func (h *Handler) contentRevoke(c *gin.Context) { h.contentSvc.Revoke(c, c.Param("work_id")) }

func (h *Handler) marketTemplates(c *gin.Context) { h.marketSvc.ListTemplates(c) }
func (h *Handler) marketCreateTemplate(c *gin.Context) { h.marketSvc.CreateTemplate(c) }
func (h *Handler) marketApplyTemplate(c *gin.Context) { h.marketSvc.ApplyTemplate(c, c.Param("template_id")) }
func (h *Handler) marketStyles(c *gin.Context) { h.marketSvc.ListStyles(c) }
func (h *Handler) marketApplyStyle(c *gin.Context) { h.marketSvc.ApplyStyle(c, c.Param("project_id")) }
func (h *Handler) marketOrders(c *gin.Context) { h.marketSvc.ListOrders(c) }
func (h *Handler) marketCreateOrder(c *gin.Context) { h.marketSvc.CreateOrder(c) }
func (h *Handler) marketCreateIntent(c *gin.Context) { h.marketSvc.CreateIntent(c, c.Param("order_id")) }
func (h *Handler) marketAcceptIntent(c *gin.Context) { h.marketSvc.AcceptIntent(c, c.Param("order_id")) }
func (h *Handler) marketUpdateOrderStatus(c *gin.Context) { h.marketSvc.UpdateOrderStatus(c, c.Param("order_id")) }

func (h *Handler) adminDashboard(c *gin.Context) { h.adminSvc.Dashboard(c) }
func (h *Handler) adminUsers(c *gin.Context) { h.adminSvc.ListUsers(c) }
func (h *Handler) adminBanUser(c *gin.Context) { h.adminSvc.BanUser(c, c.Param("user_uuid")) }
func (h *Handler) adminWorks(c *gin.Context) { h.adminSvc.ListWorks(c) }
func (h *Handler) adminDeleteWork(c *gin.Context) { h.adminSvc.DeleteWork(c, c.Param("work_id")) }
func (h *Handler) adminReports(c *gin.Context) { h.adminSvc.ListReports(c) }
func (h *Handler) adminOrders(c *gin.Context) { h.adminSvc.ListOrders(c) }
func (h *Handler) adminPackages(c *gin.Context) { h.adminSvc.ListPackages(c) }
func (h *Handler) adminModels(c *gin.Context) { h.adminSvc.ListModels(c) }
func (h *Handler) adminAuditLogs(c *gin.Context) { h.adminSvc.ListAuditLogs(c) }
func (h *Handler) adminRoles(c *gin.Context) { h.adminSvc.ListRoles(c) }
func (h *Handler) adminPlatformConfig(c *gin.Context) { h.adminSvc.ListPlatformConfigs(c) }
func (h *Handler) myPreferences(c *gin.Context) { h.adminSvc.MyPreferences(c) }
func (h *Handler) updateMyPreferences(c *gin.Context) { h.adminSvc.UpdatePreferences(c) }
func (h *Handler) adminUpdatePlatformConfig(c *gin.Context) { h.adminSvc.UpdatePlatformConfig(c) }
func (h *Handler) adminUpdatePackage(c *gin.Context) { h.adminSvc.UpdatePackage(c) }
func (h *Handler) adminUpdateModel(c *gin.Context) { h.adminSvc.UpdateModel(c) }
func (h *Handler) adminAnnouncements(c *gin.Context) { h.adminSvc.ListAnnouncements(c) }
func (h *Handler) adminBanners(c *gin.Context) { h.adminSvc.ListBanners(c) }
func (h *Handler) adminHints(c *gin.Context) { h.adminSvc.ListHints(c) }

func (h *Handler) openCreateKey(c *gin.Context) { h.openSvc.CreateKey(c) }
func (h *Handler) openListKeys(c *gin.Context) { h.openSvc.ListKeys(c) }
func (h *Handler) openRevokeKey(c *gin.Context) { h.openSvc.RevokeKey(c, c.Param("key_prefix")) }
func (h *Handler) openRegisterWebhook(c *gin.Context) { h.openSvc.RegisterWebhook(c) }
func (h *Handler) openVerifyWebhook(c *gin.Context) { h.openSvc.VerifyWebhookSignature(c, c.PostForm("url")) }
