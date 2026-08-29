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
	env        string // 08 审查 Critical#3：开发期激活端点生产不注册
}

func NewHandler(svc *Service, assetSvc *AssetService, exportSvc *ExportService, canvasSvc *CanvasService, memberSvc *MembershipService, contentSvc *ContentService, marketSvc *MarketService, adminSvc *AdminService, openSvc *OpenAPIService, env string) *Handler {
	return &Handler{svc: svc, assetSvc: assetSvc, exportSvc: exportSvc, canvasSvc: canvasSvc, memberSvc: memberSvc, contentSvc: contentSvc, marketSvc: marketSvc, adminSvc: adminSvc, openSvc: openSvc, env: env}
}

// ExtraServices 补齐服务集（S5 接口覆盖率）。
type ExtraServices struct {
	Storyboard  *StoryboardService
	AssetDetail *AssetDetailService
	Biz         *BizService
	AdminExtra  *AdminExtraService
	CanvasExtra *CanvasExtraService
}

// RegisterRoutes 注册 /projects /episodes /beats 路由（JWT + LoadUserID 由 router 组挂载；adminMW/auditMW 为 RBAC 角色中间件）。
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, adminMW, auditMW gin.HandlerFunc, extra *ExtraServices) {
	projects := rg.Group("/projects")
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
		projects.PATCH("/:project_id/config", h.updateProjectConfig) // 项目配置快照合并更新
	}
	episodes := rg.Group("/episodes")
	{
		episodes.GET("/:episode_id/beats", h.listBeats)
		episodes.GET("/:episode_id/characters", h.listCharacters)
		episodes.POST("/:episode_id/reextract-assets", h.reextractEpisodeAssets)
	}
	skills := rg.Group("/skills")
	{
		skills.GET("", h.listSkills)
	}
	uploads := rg.Group("/uploads")
	{
		uploads.POST("/presign", h.presign)
		uploads.POST("/complete", h.complete)
	}
	assets := rg.Group("/assets")
	{
		assets.POST("/batch-generate", h.batchGenerate)
		// 资产详情弹窗（对齐 LuxReal）：初始数据/提示词编辑/重置/AI优化/历史/换图
		// 独立 /asset-dialog 组（避免与 /assets/:asset_id 通配符冲突）
	}
	dialog := rg.Group("/asset-dialog")
	{
		dialog.GET("/:kind/:target_id/dialog", func(c *gin.Context) { extra.AssetDetail.DialogData(c, c.Param("kind"), c.Param("target_id")) })
		dialog.PUT("/:kind/:target_id/prompt", func(c *gin.Context) { extra.AssetDetail.UpdatePrompt(c, c.Param("kind"), c.Param("target_id")) })
		dialog.POST("/:kind/:target_id/prompt/reset", func(c *gin.Context) { extra.AssetDetail.ResetPrompt(c, c.Param("kind"), c.Param("target_id")) })
		dialog.POST("/:kind/:target_id/prompt/optimize", func(c *gin.Context) { extra.AssetDetail.OptimizePrompt(c, c.Param("kind"), c.Param("target_id")) })
		dialog.GET("/:kind/:target_id/history", func(c *gin.Context) { extra.AssetDetail.History(c, c.Param("kind"), c.Param("target_id")) })
		dialog.POST("/:kind/:target_id/set-image", func(c *gin.Context) { extra.AssetDetail.SetImage(c, c.Param("kind"), c.Param("target_id")) })
	}
	// 生成历史收藏
	rg.POST("/generated-images/:id/favorite", func(c *gin.Context) { extra.AssetDetail.ToggleFavorite(c, c.Param("id")) })
	// 9.3.1 生成消耗预览（S2-B06 跨集复用豁免）
	preview := rg.Group("/points")
	{
		preview.POST("/preview", h.previewPoints)
	}
	// 多变体（S2-B04）：3.2.6 创建变体 / 3.2.7 变体列表 + 剧本自动生成形象
	chars := rg.Group("/characters")
	{
		chars.GET("/:character_id/variants", h.listVariants)
		chars.POST("/:character_id/variants", h.createVariant)
		chars.POST("/:character_id/generate-variants", h.generateVariantsFromScript)
	}
	// 版本回滚（S2-B05）
	nus := rg.Group("/narrative-units")
	{
		nus.GET("/:nu_id/versions", h.listVersions)
		nus.POST("/:nu_id/rollback/:version_id", h.rollbackVersion)
		nus.PATCH("/:nu_id", h.updateNarrativeUnit)  // F4: 叙事单元编辑（title/content/script_original_text/preset_duration_sec）
		nus.DELETE("/:nu_id", h.deleteNarrativeUnit) // F4: 叙事单元删除（软删除级联）
		// Step3 补充缺失资产：创建角色/场景/道具 + 挂 nu_asset_links 到该单元（origin 归属所在剧集）
		nus.POST("/:nu_id/assets/create", func(c *gin.Context) { extra.Storyboard.CreateUnitAsset(c, c.Param("nu_id")) })
	}
	exportGroup := rg.Group("/projects")
	{
		exportGroup.POST("/:project_id/export", h.triggerExport)
		exportGroup.GET("/:project_id/export/preflight", h.exportPreflight)
	}
	exportList := rg.Group("/exports")
	{
		exportList.GET("", h.listExports)
		exportList.GET("/:export_id", h.exportDetail)
		exportList.POST("/:export_id/retry", h.exportRetry)
	}
	// Step3 视频（S1-F05）：3.3.7a 批量生成镜头块视频
	beats := rg.Group("/beats")
	{
		beats.POST("/:beat_id/generate-videos", h.generateVideos)
		beats.GET("/:beat_id/shots", h.listBeatShots)
	}
	// 自由画布（S2-B07）：TAG 4.1~4.2 + 4.4 执行
	canvases := rg.Group("/canvases")
	{
		canvases.GET("", h.canvasList) // 我的画布列表（作品列表「画布」Tab）
		canvases.POST("", h.createCanvas)
		canvases.GET("/:canvas_id", h.loadCanvas)
		canvases.PUT("/:canvas_id", h.saveCanvas)
		canvases.PUT("/:canvas_id/nodes", h.saveNodes)
		canvases.POST("/:canvas_id/execute", h.executeCanvas)
		canvases.GET("/:canvas_id/snapshots", h.listSnapshots)                         // S2-F03 快照列表
		canvases.POST("/:canvas_id/snapshots/:snapshot_id/restore", h.restoreSnapshot) // 恢复
	}
	// 月卡会员（S2-B09）：9.9 购买/查询 + 9.10 取消 + 4.5 SSE（S2-B10）
	mems := rg.Group("/memberships")
	{
		mems.POST("/purchase", h.membershipPurchase)
		mems.GET("", h.membershipDetail)
		mems.POST("/cancel", h.membershipCancel)
		// 08 审查 Critical#3：开发期零支付激活端点，生产环境不注册（防免费开会员）
		if h.env != "production" {
			mems.POST("/orders/:order_no/activate", h.membershipActivate) // 开发期激活（鉴权+归属校验）
		}
	}
	rg.GET("/ai/dialog/stream", h.aiDialogSSE)
	// 内容域（S3-B01~B03）：信息流/作品/互动/审核
	feed := rg.Group("/works")
	{
		feed.GET("", h.contentFeed)
		feed.POST("", h.contentSubmit)
		feed.GET("/:work_id", h.contentWork)
		feed.POST("/:work_id/like", h.contentLike)
		feed.DELETE("/:work_id/like", h.contentUnLike)
		feed.POST("/:work_id/favorite", h.contentFavorite)
		feed.POST("/:work_id/comments", h.contentComment)
		feed.GET("/:work_id/comments", func(c *gin.Context) { h.contentSvc.Comments(c, c.Param("work_id")) })
		feed.POST("/:work_id/report", h.contentReport)
	}
	// 关注（S3-B02 补齐）
	users := rg.Group("/users")
	{
		users.POST("/:user_uuid/follow", h.contentFollow)
		users.DELETE("/:user_uuid/follow", h.contentUnfollow)
	}
	// 审核（11.15~11.17）：SUPER_ADMIN/AUDITOR 角色（RBAC）
	audit := rg.Group("/audit", auditMW)
	{
		audit.GET("/works", h.contentAuditList)
		audit.POST("/works/:work_id/review", h.contentReview)
		audit.POST("/works/:work_id/revoke", h.contentRevoke)
	}
	// 生态（S3-B04~B06）：模板/风格/任务
	tpls := rg.Group("/templates")
	{
		tpls.GET("", h.marketTemplates)
		tpls.POST("", h.marketCreateTemplate)
		tpls.POST("/:template_id/apply", h.marketApplyTemplate)
	}
	styles := rg.Group("/style-library")
	{
		styles.GET("", h.marketStyles)
	}
	rg.POST("/projects/:project_id/apply-style", h.marketApplyStyle)
	tasks := rg.Group("/tasks")
	{
		tasks.GET("", h.marketOrders)
		tasks.POST("", h.marketCreateOrder)
		tasks.POST("/:order_id/intents", h.marketCreateIntent)
		tasks.POST("/:order_id/accept", h.marketAcceptIntent)
		tasks.POST("/:order_id/status", h.marketUpdateOrderStatus)
	}
	// 管理后台（S3-B07/B08）：TAG 11（RBAC：SUPER_ADMIN）
	adm := rg.Group("/admin", adminMW)
	{
		adm.GET("/dashboard", h.adminDashboard)                  // 11.0 指标
		adm.GET("/users", h.adminUsers)                          // 11.2 用户列表
		adm.PATCH("/users/:user_uuid/ban", h.adminBanUser)       // 11.5 封禁
		adm.GET("/works", h.adminWorks)                          // 内容列表
		adm.DELETE("/works/:work_id", h.adminDeleteWork)         // 11.73 强制删除
		adm.GET("/reports", h.adminReports)                      // 11.74 举报列表
		adm.GET("/orders", h.adminOrders)                        // P2 订单管理
		adm.GET("/packages", h.adminPackages)                    // P2 套餐管理
		adm.GET("/models", h.adminModels)                        // P2 模型管理
		adm.GET("/audit-logs", h.adminAuditLogs)                 // P2 审计日志
		adm.GET("/roles", h.adminRoles)                          // P2 角色列表
		adm.GET("/platform-config", h.adminPlatformConfig)       // P2 平台配置
		adm.PUT("/platform-config", h.adminUpdatePlatformConfig) // P2 更新配置
		adm.PUT("/packages", h.adminUpdatePackage)               // P2 更新套餐
		adm.PUT("/models", h.adminUpdateModel)                   // P2 更新模型
		adm.GET("/announcements", h.adminAnnouncements)          // P2 公告列表
		adm.GET("/banners", h.adminBanners)                      // P2 Banner 列表
		adm.GET("/hints", h.adminHints)                          // P2 提示列表
	}
	// 开发者（S3-B09）：API Key + Webhook
	dev := rg.Group("/api-keys")
	{
		dev.GET("", h.openListKeys)
		dev.POST("", h.openCreateKey)
		dev.POST("/:key_prefix/revoke", h.openRevokeKey)
	}
	wh := rg.Group("/webhooks")
	{
		wh.POST("", h.openRegisterWebhook)
	}
	rg.POST("/webhooks/verify", h.openVerifyWebhook)
	// 个人偏好
	me := rg.Group("/me")
	{
		me.GET("/preferences", h.myPreferences)
		me.PUT("/preferences", h.updateMyPreferences)
	}
	if extra == nil {
		return
	}
	h.registerExtraRoutes(rg, extra, adminMW)
}

// registerExtraRoutes S5 补齐路由（89 个 coverage 端点）。
// 注意：参数名必须与同前缀已有路由一致（gin 同层级通配符同名约束）。
func (h *Handler) registerExtraRoutes(rg *gin.RouterGroup, extra *ExtraServices, adminMW gin.HandlerFunc) {
	// ---- 叙事单元/镜头块/镜头/分镜 ----
	beats := rg.Group("/beats")
	{
		beats.GET("/:beat_id", func(c *gin.Context) { extra.Storyboard.BeatDetail(c, c.Param("beat_id")) })
		beats.POST("/:beat_id/compose-video", func(c *gin.Context) { extra.Storyboard.ComposeBeatVideo(c, c.Param("beat_id")) })
		beats.POST("/:beat_id/replan", func(c *gin.Context) { extra.Storyboard.ReplanBeat(c, c.Param("beat_id")) })
		beats.POST("/:beat_id/plan-video-tasks", h.planVideoTasks) // 规划视频任务（成片卡编组）
		beats.GET("/:beat_id/cards", h.listBeatCards)              // 成片卡列表（Step3 网格轮询）
		beats.GET("/:beat_id/shot-groups", func(c *gin.Context) { extra.Storyboard.BeatShotGroups(c, c.Param("beat_id")) })
		beats.GET("/:beat_id/storyboards", func(c *gin.Context) { extra.Storyboard.BeatStoryboards(c, c.Param("beat_id")) })
		beats.POST("/:beat_id/storyboards/generate", func(c *gin.Context) { extra.Storyboard.GenerateStoryboards(c, c.Param("beat_id")) })
	}
	shotGroups := rg.Group("/shot-groups")
	{
		shotGroups.POST("/:shot_group_id/concat-video", func(c *gin.Context) { extra.Storyboard.ConcatShotGroupVideo(c, c.Param("shot_group_id")) })
		shotGroups.POST("/:shot_group_id/generate-video", func(c *gin.Context) { extra.Storyboard.GenerateShotGroupVideo(c, c.Param("shot_group_id")) })
	}
	shots := rg.Group("/shots")
	{
		shots.GET("/:shot_id", func(c *gin.Context) { extra.Storyboard.ShotDetail(c, c.Param("shot_id")) })
		// Step3 高价值快批量：镜头编辑（提示词 / 时长 / 分镜字段微调）
		shots.PATCH("/:shot_id", func(c *gin.Context) { extra.Storyboard.UpdateShot(c, c.Param("shot_id")) })
		shots.POST("/batch", extra.Storyboard.ShotsBatch)
		shots.POST("/:shot_id/regenerate", h.regenerateShot) // F11: 单镜头重生成（旧链路兼容）
		// 成片卡（规划视频任务产物；主线生成链路）
		cards := rg.Group("/video-cards")
		{
			cards.POST("/:card_id/regenerate", h.regenerateCard)
			cards.PATCH("/:card_id/prompt", h.updateCardPrompt)
			cards.PATCH("/:card_id/duration", h.updateCardDuration)
			// 参考图拖拽绑定（对齐 LuxReal 拖资产 chip 进卡）：绑定作用于卡内全部镜头
			cards.POST("/:card_id/assets/bind", h.bindCardAsset)
			cards.DELETE("/:card_id/assets/bind", h.unbindCardAsset)
		}
		// 18 - 视频制作：生成镜头级参考图（对齐 LuxReal「镜头N的参考图」，SHOT_REF_IMAGE_GEN）
		shots.POST("/:shot_id/generate-reference-image", func(c *gin.Context) {
			extra.Storyboard.GenerateShotRefImage(c, c.Param("shot_id"))
		})
		// 镜头级参考资产绑定（对齐 LuxReal 拖资产 chip 到单镜头）：精准到单个镜头的 shot_asset_refs
		shots.POST("/:shot_id/assets/bind", h.bindShotAsset)
		shots.DELETE("/:shot_id/assets/bind", h.unbindShotAsset)
	}
	storyboards := rg.Group("/storyboards")
	{
		storyboards.GET("/:storyboard_id", func(c *gin.Context) { extra.Storyboard.StoryboardDetail(c, c.Param("storyboard_id")) })
		storyboards.POST("/:storyboard_id/image/edit", func(c *gin.Context) { extra.Storyboard.StoryboardImageEdit(c, c.Param("storyboard_id")) })
		// 18 - 视频制作 阶段三：单分镜视频 → 复用单镜头重生成真实链路（计费 + Outbox）
		storyboards.POST("/:storyboard_id/video", func(c *gin.Context) {
			h.svc.RegenerateShot(c, c.Param("storyboard_id"))
		})
		storyboards.POST("/batch-generate-image", extra.Storyboard.BatchGenerateStoryboardImage)
	}
	tasks := rg.Group("/tasks")
	{
		tasks.GET("/:order_id", func(c *gin.Context) { extra.Storyboard.TaskDetail(c, c.Param("order_id")) })
		tasks.POST("/:order_id/retry", func(c *gin.Context) { extra.Storyboard.TaskRetry(c, c.Param("order_id")) })
	}
	videos := rg.Group("/videos")
	{
		videos.GET("/:video_id/track-timeline", func(c *gin.Context) { extra.Storyboard.VideoTrackTimeline(c, c.Param("video_id")) })
		videos.POST("/:video_id/tts", func(c *gin.Context) { extra.Storyboard.VideoTTS(c, c.Param("video_id")) })
	}
	// ---- 角色/场景/道具/资产/剧集 ----
	rg.PATCH("/characters/:character_id", func(c *gin.Context) { extra.AssetDetail.UpdateCharacter(c, c.Param("character_id")) })
	rg.GET("/character-variants/:variant_id", func(c *gin.Context) { extra.AssetDetail.VariantDetail(c, c.Param("variant_id")) })
	rg.DELETE("/character-variants/:variant_id", func(c *gin.Context) { extra.AssetDetail.DeleteCharacterVariant(c, c.Param("variant_id")) })
	rg.POST("/character-variants/:variant_id/views", func(c *gin.Context) { extra.AssetDetail.AddVariantView(c, c.Param("variant_id")) })
	rg.DELETE("/character-variants/:variant_id/views/:index", func(c *gin.Context) { extra.AssetDetail.DeleteVariantView(c, c.Param("variant_id"), c.Param("index")) })
	rg.PATCH("/scenes/:scene_id", func(c *gin.Context) { extra.AssetDetail.UpdateScene(c, c.Param("scene_id")) })
	rg.PATCH("/props/:prop_id", func(c *gin.Context) { extra.AssetDetail.UpdateProp(c, c.Param("prop_id")) })
	epGroup := rg.Group("/episodes")
	{
		epGroup.GET("/:episode_id", func(c *gin.Context) { extra.AssetDetail.EpisodeDetail(c, c.Param("episode_id")) })
		epGroup.GET("/:episode_id/assets/overview", func(c *gin.Context) { extra.AssetDetail.EpisodeAssetsOverview(c, c.Param("episode_id")) })
		epGroup.GET("/:episode_id/scenes", func(c *gin.Context) { extra.AssetDetail.EpisodeScenes(c, c.Param("episode_id")) })
		epGroup.GET("/:episode_id/props", func(c *gin.Context) { extra.AssetDetail.EpisodeProps(c, c.Param("episode_id")) })
		// 17 - 工作台对齐：叙事单元↔资产关联清单（Step3 关联资产占位 + 就绪度）
		epGroup.GET("/:episode_id/asset-links", func(c *gin.Context) { extra.Storyboard.EpisodeAssetLinks(c, c.Param("episode_id")) })
	}
	assetGroup := rg.Group("/assets")
	{
		assetGroup.GET("/:asset_id/derivatives", func(c *gin.Context) { extra.AssetDetail.AssetDerivatives(c, c.Param("asset_id")) })
		assetGroup.POST("/:asset_id/derive", func(c *gin.Context) { extra.AssetDetail.AssetDerive(c, c.Param("asset_id")) })
		assetGroup.POST("/:asset_id/preview-points", func(c *gin.Context) { extra.AssetDetail.AssetPreviewPoints(c, c.Param("asset_id")) })
	}
	// ---- 项目导出历史 / 回收站 ----
	rg.GET("/projects/:project_id/exports", func(c *gin.Context) { h.exportSvc.List(c) })
	rg.GET("/projects/:project_id/assets", func(c *gin.Context) { extra.AssetDetail.ProjectAssets(c, c.Param("project_id")) })
	recycle := rg.Group("/recycle-bin")
	{
		recycle.POST("/:work_id/restore", func(c *gin.Context) { h.contentSvc.RestoreWork(c, c.Param("work_id")) })
		recycle.DELETE("/:work_id", func(c *gin.Context) { h.contentSvc.DeleteWorkForever(c, c.Param("work_id")) })
	}
	// ---- 内容域 ----
	// 公开端点（/feed /works/public /works/:work_id/public）由 router 层挂 v1Pub（无需登录）
	rg.POST("/works/:work_id/publish", func(c *gin.Context) { h.contentSvc.PublishWork(c, c.Param("work_id")) })
	rg.PUT("/comments/:comment_id", func(c *gin.Context) { h.contentSvc.EditComment(c, c.Param("comment_id")) })
	// ---- 任务接单 ----
	taskOrders := rg.Group("/task-orders")
	{
		taskOrders.GET("", h.marketSvc.TaskOrderSquare)
		taskOrders.GET("/:task_order_id", func(c *gin.Context) { h.marketSvc.TaskOrderDetail(c, c.Param("task_order_id")) })
		taskOrders.POST("/:task_order_id/close", func(c *gin.Context) { h.marketSvc.CloseTaskOrder(c, c.Param("task_order_id")) })
		taskOrders.GET("/:task_order_id/intents", func(c *gin.Context) { h.marketSvc.TaskOrderIntents(c, c.Param("task_order_id")) })
	}
	rg.GET("/me/task-intents", h.marketSvc.MyTaskIntents)
	// ---- 商业域 ----
	// 公开端点（/packages /models /point-packages）由 router 层挂 v1Pub（无需登录）
	rg.GET("/membership", extra.Biz.MembershipDetail)
	rg.POST("/membership/cancel", extra.Biz.MembershipCancel)
	// ---- 画布域 ----
	canvasGroup := rg.Group("/canvas")
	{
		canvasGroup.GET("/:project_id", func(c *gin.Context) { extra.CanvasExtra.CanvasByProject(c, c.Param("project_id")) })
		canvasGroup.POST("/:project_id/nodes-validate", func(c *gin.Context) { extra.CanvasExtra.CanvasNodesValidate(c, c.Param("project_id")) })
		canvasGroup.POST("/:project_id/workflow-execute", func(c *gin.Context) { extra.CanvasExtra.CanvasWorkflowExecute(c, c.Param("project_id")) })
		canvasGroup.GET("/console/episodes", extra.CanvasExtra.ConsoleEpisodes)
	}
	// ---- 内部回调（无鉴权，在 router 层公开组注册） ----
	// ---- Admin 补齐 ----
	admX := rg.Group("/admin", adminMW)
	{
		admX.GET("/activities", extra.AdminExtra.Activities)
		admX.GET("/agent-memory", extra.AdminExtra.AgentMemory)
		admX.GET("/agent-skills", extra.AdminExtra.AgentSkills)
		admX.GET("/auth/permissions", extra.AdminExtra.AuthPermissions)
		admX.GET("/categories", extra.AdminExtra.Categories)
		admX.GET("/comments", extra.AdminExtra.Comments)
		admX.GET("/courses", extra.AdminExtra.Courses)
		admX.GET("/dashboard/summary", extra.AdminExtra.DashboardSummary)
		admX.GET("/trends", extra.AdminExtra.Trends) // Dashboard 趋势图按日序列
		admX.GET("/directive-skills", extra.AdminExtra.DirectiveSkills)
		admX.GET("/languages", extra.AdminExtra.Languages)
		admX.GET("/model-keys", extra.AdminExtra.ModelKeys)
		admX.GET("/platform/config", extra.AdminExtra.PlatformConfig)
		admX.PUT("/platform/config", h.adminUpdatePlatformConfig) // 单 key upsert（config_key/config_value）
		admX.GET("/point-packages", extra.AdminExtra.PointPackages)
		admX.GET("/providers", extra.AdminExtra.ModelProvidersList)
		admX.POST("/providers", extra.AdminExtra.ModelProviderCreate)
		admX.PUT("/providers/:provider_id", func(c *gin.Context) { extra.AdminExtra.ModelProviderUpdate(c, c.Param("provider_id")) })
		admX.DELETE("/providers/:provider_id", func(c *gin.Context) { extra.AdminExtra.ModelProviderDelete(c, c.Param("provider_id")) }) // 删除厂商（级联软删模型与密钥）
		admX.GET("/model-list", extra.AdminExtra.ModelListAdmin)                                                                         // 11.39 模型列表（含厂商）
		admX.POST("/model-create", extra.AdminExtra.ModelCreate)                                                                         // 新建模型
		admX.DELETE("/model-delete/:model_id", func(c *gin.Context) { extra.AdminExtra.ModelDelete(c, c.Param("model_id")) })            // 删除模型（软删除）
		admX.GET("/model-key-list", extra.AdminExtra.ModelKeyList)                                                                       // 11.43 密钥列表（隐藏密文）
		admX.POST("/model-key-create", extra.AdminExtra.ModelKeyCreate)
		admX.PUT("/model-key-update/:key_id", func(c *gin.Context) { extra.AdminExtra.ModelKeyUpdate(c, c.Param("key_id")) })
		admX.POST("/model-key-revoke/:key_id", func(c *gin.Context) { extra.AdminExtra.ModelKeyRevoke(c, c.Param("key_id")) })
		admX.GET("/queues", extra.AdminExtra.Queues)
		admX.GET("/rate-limits", extra.AdminExtra.RateLimits)
		admX.GET("/spaces", extra.AdminExtra.Spaces)
		admX.PUT("/spaces/:space_id/status", func(c *gin.Context) { extra.AdminExtra.SpaceStatus(c, c.Param("space_id")) })
		admX.GET("/statistics", extra.AdminExtra.Statistics)
		admX.GET("/style-library", extra.AdminExtra.StyleLibrary)
		admX.GET("/themes", extra.AdminExtra.Themes)
		admX.GET("/think-levels", extra.AdminExtra.ThinkLevels)
		admX.PUT("/announcements", h.adminSvc.CreateAnnouncement)         // 公告新增
		admX.PUT("/announcements/:id", h.adminSvc.UpdateAnnouncement)     // 公告更新
		admX.DELETE("/announcements/:id", h.adminSvc.DeleteAnnouncement)  // 公告删除
		admX.POST("/banners", h.adminSvc.CreateBanner)                    // Banner 新增
		admX.PUT("/banners/:id", h.adminSvc.UpdateBanner)                 // Banner 更新
		admX.DELETE("/banners/:id", h.adminSvc.DeleteBanner)              // Banner 删除
		admX.PUT("/point-packages", h.adminSvc.UpdatePointPackage)        // 积分包 upsert
		admX.PUT("/rate-limits/:id", h.adminSvc.UpdateRateLimit)          // 限流配置更新
		admX.PUT("/themes/:id", h.adminSvc.UpdateTheme)                   // 主题更新
		admX.PUT("/style-library/:id", h.adminSvc.UpdateStyle)            // 风格库更新
		admX.GET("/withdrawals", h.adminSvc.ListWithdrawals)              // 提现申请列表
		admX.POST("/withdrawals/:id/review", h.adminSvc.ReviewWithdrawal) // 提现审核
		admX.GET("/users/:user_uuid", func(c *gin.Context) { extra.AdminExtra.UserDetail(c, c.Param("user_uuid")) })
		admX.PUT("/users/:user_uuid/membership", func(c *gin.Context) { extra.AdminExtra.UserMembership(c, c.Param("user_uuid")) })
		admX.PUT("/users/:user_uuid/points", func(c *gin.Context) { extra.AdminExtra.UserPoints(c, c.Param("user_uuid")) })
		admX.PUT("/users/:user_uuid/roles", func(c *gin.Context) { extra.AdminExtra.UserRoles(c, c.Param("user_uuid")) })
		admX.PUT("/users/:user_uuid/status", func(c *gin.Context) { extra.AdminExtra.UserStatus(c, c.Param("user_uuid")) })
		admX.POST("/works/:work_id/audit", func(c *gin.Context) { extra.AdminExtra.WorkAudit(c, c.Param("work_id")) })
		admX.PUT("/works/:work_id/unlist", func(c *gin.Context) { extra.AdminExtra.WorkUnlist(c, c.Param("work_id")) })
		admX.GET("/tasks", extra.AdminExtra.Tasks)                // 11.57 算力任务列表
		admX.GET("/tasks/stats", extra.AdminExtra.TaskStats)      // 任务统计（队列/耗时）
		admX.POST("/reports/:id/review", h.adminSvc.ReviewReport) // A2: 举报处理（通过/驳回）
		admX.POST("/tasks/:task_id/retry", func(c *gin.Context) { extra.AdminExtra.TaskRetryAdmin(c, c.Param("task_id")) })
		admX.POST("/tasks/:task_id/cancel", func(c *gin.Context) { extra.AdminExtra.TaskCancelAdmin(c, c.Param("task_id")) })
		admX.GET("/tasks/:task_id", func(c *gin.Context) { extra.AdminExtra.TaskDetailAdmin(c, c.Param("task_id")) })
		admX.PUT("/tasks/:task_id/priority", func(c *gin.Context) { extra.AdminExtra.TaskPriorityAdmin(c, c.Param("task_id")) })
		admX.PUT("/works/:work_id", func(c *gin.Context) { extra.AdminExtra.WorkUpdate(c, c.Param("work_id")) })
		admX.PUT("/works/:work_id/recommend", func(c *gin.Context) { extra.AdminExtra.WorkRecommend(c, c.Param("work_id")) })
		admX.POST("/users", extra.AdminExtra.AdminCreateUser) // 管理端新增用户
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

func (h *Handler) updateProjectConfig(c *gin.Context) {
	h.svc.UpdateConfig(c, c.Param("project_id"))
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

func (h *Handler) listSkills(c *gin.Context) {
	h.svc.ListSkills(c)
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

func (h *Handler) exportPreflight(c *gin.Context) {
	h.exportSvc.Preflight(c, c.Param("project_id"))
}

func (h *Handler) exportRetry(c *gin.Context) {
	h.exportSvc.Retry(c, c.Param("export_id"))
}

func (h *Handler) generateVideos(c *gin.Context) {
	h.svc.GenerateVideos(c, c.Param("beat_id"))
}

// regenerateShot 单镜头重生成（F11：POST /shots/:shot_id/regenerate）。
func (h *Handler) regenerateShot(c *gin.Context) {
	h.svc.RegenerateShot(c, c.Param("shot_id"))
}

func (h *Handler) listVariants(c *gin.Context) {
	h.assetSvc.ListVariants(c, c.Param("character_id"))
}

func (h *Handler) listBeatShots(c *gin.Context) {
	h.svc.ListBeatShots(c, c.Param("beat_id"))
}

// planVideoTasks 规划视频任务（成片卡编组；POST /beats/:beat_id/plan-video-tasks）。
func (h *Handler) planVideoTasks(c *gin.Context) {
	h.svc.PlanVideoTasks(c, c.Param("beat_id"))
}

// listBeatCards 成片卡列表（GET /beats/:beat_id/cards）。
func (h *Handler) listBeatCards(c *gin.Context) {
	h.svc.ListBeatCards(c, c.Param("beat_id"))
}

// regenerateCard 单成片卡重新生成（POST /video-cards/:card_id/regenerate）。
func (h *Handler) regenerateCard(c *gin.Context) {
	h.svc.RegenerateCard(c, c.Param("card_id"))
}

// updateCardPrompt 编辑成片卡提示词（PATCH /video-cards/:card_id/prompt）。
func (h *Handler) updateCardPrompt(c *gin.Context) {
	h.svc.UpdateCardPrompt(c, c.Param("card_id"))
}

// updateCardDuration 调整成片卡时长（PATCH /video-cards/:card_id/duration）。
func (h *Handler) updateCardDuration(c *gin.Context) {
	h.svc.UpdateCardDuration(c, c.Param("card_id"))
}

// bindCardAsset 成片卡绑定参考资产（POST /video-cards/:card_id/assets/bind）。
func (h *Handler) bindCardAsset(c *gin.Context) {
	h.svc.BindCardAsset(c, c.Param("card_id"))
}

// unbindCardAsset 解绑成片卡参考资产（DELETE /video-cards/:card_id/assets/bind）。
func (h *Handler) unbindCardAsset(c *gin.Context) {
	h.svc.UnbindCardAsset(c, c.Param("card_id"))
}

// bindShotAsset 镜头级绑定参考资产（POST /shots/:shot_id/assets/bind）。
func (h *Handler) bindShotAsset(c *gin.Context) {
	h.svc.BindShotAsset(c, c.Param("shot_id"))
}

// unbindShotAsset 镜头级解绑参考资产（DELETE /shots/:shot_id/assets/bind）。
func (h *Handler) unbindShotAsset(c *gin.Context) {
	h.svc.UnbindShotAsset(c, c.Param("shot_id"))
}

func (h *Handler) createVariant(c *gin.Context) {
	h.assetSvc.CreateVariant(c, c.Param("character_id"))
}

func (h *Handler) generateVariantsFromScript(c *gin.Context) {
	h.assetSvc.GenerateVariantsFromScript(c, c.Param("character_id"))
}

func (h *Handler) reextractEpisodeAssets(c *gin.Context) {
	h.assetSvc.ReextractEpisodeAssets(c, c.Param("episode_id"))
}

func (h *Handler) listVersions(c *gin.Context) {
	h.assetSvc.ListVersions(c, c.Param("nu_id"))
}

func (h *Handler) rollbackVersion(c *gin.Context) {
	h.assetSvc.RollbackVersion(c, c.Param("nu_id"), c.Param("version_id"))
}

// updateNarrativeUnit 叙事单元编辑（F4：PATCH /narrative-units/:nu_id）。
func (h *Handler) updateNarrativeUnit(c *gin.Context) {
	h.svc.UpdateNarrativeUnit(c, c.Param("nu_id"))
}

// deleteNarrativeUnit 叙事单元删除（F4：DELETE /narrative-units/:nu_id）。
func (h *Handler) deleteNarrativeUnit(c *gin.Context) {
	h.svc.DeleteNarrativeUnit(c, c.Param("nu_id"))
}

func (h *Handler) canvasList(c *gin.Context) { h.canvasSvc.List(c) }

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
	spaceID := int64(0)
	if req.SpaceID != "" {
		// 指定空间：校验成员身份
		id, err := h.svc.repo.ResolveSpace(c.Request.Context(), req.SpaceID, uid)
		if err != nil {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "空间不存在或无权限")
			return
		}
		spaceID = id
	} else {
		id, _, err := h.svc.repo.DefaultSpaceID(c.Request.Context(), uid)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取空间失败")
			return
		}
		spaceID = id
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

func (h *Handler) contentFeed(c *gin.Context)      { h.contentSvc.Feed(c) }
func (h *Handler) contentSubmit(c *gin.Context)    { h.contentSvc.Submit(c) }
func (h *Handler) contentWork(c *gin.Context)      { h.contentSvc.Work(c, c.Param("work_id")) }
func (h *Handler) contentLike(c *gin.Context)      { h.contentSvc.Like(c, c.Param("work_id")) }
func (h *Handler) contentUnLike(c *gin.Context)    { h.contentSvc.UnLike(c, c.Param("work_id")) }
func (h *Handler) contentFavorite(c *gin.Context)  { h.contentSvc.Favorite(c, c.Param("work_id")) }
func (h *Handler) contentComment(c *gin.Context)   { h.contentSvc.Comment(c, c.Param("work_id")) }
func (h *Handler) contentReport(c *gin.Context)    { h.contentSvc.Report(c, c.Param("work_id")) }
func (h *Handler) contentFollow(c *gin.Context)    { h.contentSvc.Follow(c, c.Param("user_uuid")) }
func (h *Handler) contentUnfollow(c *gin.Context)  { h.contentSvc.Unfollow(c, c.Param("user_uuid")) }
func (h *Handler) contentAuditList(c *gin.Context) { h.contentSvc.AuditList(c) }
func (h *Handler) contentReview(c *gin.Context)    { h.contentSvc.Review(c, c.Param("work_id")) }
func (h *Handler) contentRevoke(c *gin.Context)    { h.contentSvc.Revoke(c, c.Param("work_id")) }

func (h *Handler) marketTemplates(c *gin.Context)      { h.marketSvc.ListTemplates(c) }
func (h *Handler) marketCreateTemplate(c *gin.Context) { h.marketSvc.CreateTemplate(c) }
func (h *Handler) marketApplyTemplate(c *gin.Context) {
	h.marketSvc.ApplyTemplate(c, c.Param("template_id"))
}
func (h *Handler) marketStyles(c *gin.Context)      { h.marketSvc.ListStyles(c) }
func (h *Handler) marketApplyStyle(c *gin.Context)  { h.marketSvc.ApplyStyle(c, c.Param("project_id")) }
func (h *Handler) marketOrders(c *gin.Context)      { h.marketSvc.ListOrders(c) }
func (h *Handler) marketCreateOrder(c *gin.Context) { h.marketSvc.CreateOrder(c) }
func (h *Handler) marketCreateIntent(c *gin.Context) {
	h.marketSvc.CreateIntent(c, c.Param("order_id"))
}
func (h *Handler) marketAcceptIntent(c *gin.Context) {
	h.marketSvc.AcceptIntent(c, c.Param("order_id"))
}
func (h *Handler) marketUpdateOrderStatus(c *gin.Context) {
	h.marketSvc.UpdateOrderStatus(c, c.Param("order_id"))
}

func (h *Handler) adminDashboard(c *gin.Context)            { h.adminSvc.Dashboard(c) }
func (h *Handler) adminUsers(c *gin.Context)                { h.adminSvc.ListUsers(c) }
func (h *Handler) adminBanUser(c *gin.Context)              { h.adminSvc.BanUser(c, c.Param("user_uuid")) }
func (h *Handler) adminWorks(c *gin.Context)                { h.adminSvc.ListWorks(c) }
func (h *Handler) adminDeleteWork(c *gin.Context)           { h.adminSvc.DeleteWork(c, c.Param("work_id")) }
func (h *Handler) adminReports(c *gin.Context)              { h.adminSvc.ListReports(c) }
func (h *Handler) adminOrders(c *gin.Context)               { h.adminSvc.ListOrders(c) }
func (h *Handler) adminPackages(c *gin.Context)             { h.adminSvc.ListPackages(c) }
func (h *Handler) adminModels(c *gin.Context)               { h.adminSvc.ListModels(c) }
func (h *Handler) adminAuditLogs(c *gin.Context)            { h.adminSvc.ListAuditLogs(c) }
func (h *Handler) adminRoles(c *gin.Context)                { h.adminSvc.ListRoles(c) }
func (h *Handler) adminPlatformConfig(c *gin.Context)       { h.adminSvc.ListPlatformConfigs(c) }
func (h *Handler) myPreferences(c *gin.Context)             { h.adminSvc.MyPreferences(c) }
func (h *Handler) updateMyPreferences(c *gin.Context)       { h.adminSvc.UpdatePreferences(c) }
func (h *Handler) adminUpdatePlatformConfig(c *gin.Context) { h.adminSvc.UpdatePlatformConfig(c) }
func (h *Handler) adminUpdatePackage(c *gin.Context)        { h.adminSvc.UpdatePackage(c) }
func (h *Handler) adminUpdateModel(c *gin.Context)          { h.adminSvc.UpdateModel(c) }
func (h *Handler) adminAnnouncements(c *gin.Context)        { h.adminSvc.ListAnnouncements(c) }
func (h *Handler) adminBanners(c *gin.Context)              { h.adminSvc.ListBanners(c) }
func (h *Handler) adminHints(c *gin.Context)                { h.adminSvc.ListHints(c) }

func (h *Handler) openCreateKey(c *gin.Context)       { h.openSvc.CreateKey(c) }
func (h *Handler) openListKeys(c *gin.Context)        { h.openSvc.ListKeys(c) }
func (h *Handler) openRevokeKey(c *gin.Context)       { h.openSvc.RevokeKey(c, c.Param("key_prefix")) }
func (h *Handler) openRegisterWebhook(c *gin.Context) { h.openSvc.RegisterWebhook(c) }
func (h *Handler) openVerifyWebhook(c *gin.Context) {
	h.openSvc.VerifyWebhookSignature(c, c.PostForm("url"))
}
