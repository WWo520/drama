// Package router 路由总入口（对齐 04 B.1：routes_user 用户端分组）。
package router

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/domain/auth"
	"github.com/duanju/backend/internal/domain/creation"
	"github.com/duanju/backend/internal/domain/space"
	"github.com/duanju/backend/internal/engine/asset"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/engine/notify"
	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/cryptox"
	"github.com/duanju/backend/internal/pkg/httpx"
	"github.com/duanju/backend/internal/pkg/jwt"
	"github.com/duanju/backend/internal/pkg/logger"
	"github.com/duanju/backend/internal/repository"
)

// Setup 装配 Gin 引擎：中间件链 + 路由分组。
func Setup(cfg *config.Config, lg *zap.Logger, pool *pgxpool.Pool, rdb *redis.Client) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 中间件链（对齐 04 C.2 调用顺序：RequestID → Recovery → CORS → AccessLog → 限流）
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(lg),
		middleware.CORS(cfg.App.Env),
		middleware.AccessLog(lg),
	)

	// 健康检查（B03 验收：启动空服务返回健康检查）
	r.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, gin.H{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
	})

	jwtMgr := jwt.NewManager(jwt.Config{
		Secret:     cfg.JWT.Secret,
		AccessTTL:  cfg.JWT.AccessTTL,
		RefreshTTL: 7 * 24 * time.Hour,
	})

	// 用户端分组 /api/v1（05 A 章 Base URL）
	v1 := r.Group("/api/v1")

	jwtMW := middleware.JWT(jwtMgr, rdb, cfg.App.Env)

	// 公开路由组：仅 IP 限流（未登录 20/min）
	v1Pub := v1.Group("", middleware.RateLimit(rdb, pool, cfg.App.Env))

	// auth 模块（公开端点 + /auth/me 自带 JWT）
	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, jwtMgr, rdb, cfg.App.Env, lg)
	auth.NewHandler(authSvc, jwtMgr, rdb, cfg.App.Env).RegisterRoutes(v1Pub)

	// 受保护路由组：JWT → LoadUserID → 限流（登录用户 60/min；修复限流顺序：先解析身份再限流）
	spaceRepo := space.NewRepo(pool)
	loadUserMW := middleware.LoadUserID(func(ctx context.Context, userUUID string) (int64, error) {
		return spaceRepo.GetUserIDByUUID(ctx, userUUID)
	})
	v1Auth := v1.Group("", jwtMW, loadUserMW, middleware.RateLimit(rdb, pool, cfg.App.Env))

	// RBAC 角色中间件（L2：admin=SUPER_ADMIN；audit=SUPER_ADMIN/AUDITOR）
	adminMW := middleware.RequireRole(pool, "SUPER_ADMIN")
	auditMW := middleware.RequireRole(pool, "SUPER_ADMIN", "AUDITOR")

	// space 模块（个人空间子集：JWT + 用户 ID 解析）
	spaceBilling := repository.NewSpaceBillingStore(pool)
	spaceSvc := space.NewService(spaceRepo, spaceBilling)
	space.NewHandler(spaceSvc).RegisterRoutes(v1Auth)

	// 计费引擎（S1-B09）：任务冻结/结算（API 侧 Freeze + worker Settler）
	billingStore := repository.NewBillingStore(pool)
	billingEngine := billing.NewEngine(billingStore)

	// creation 模块（项目 CRUD + 拆分触发 + 层级读，S1-B03）
	creationRepo := creation.NewRepo(pool)
	creationSvc := creation.NewService(creationRepo, lg, billingEngine)

	// 资产引擎 E3（S1-B06：MinIO 预签名）+ 资产服务
	assetStore := repository.NewAssetStore(pool)
	assetClient, err := asset.New(asset.Config{
		Endpoint:  cfg.S3.Endpoint,
		AccessKey: cfg.S3.AccessKey,
		SecretKey: cfg.S3.SecretKey,
		Bucket:    cfg.S3.Bucket,
		UseSSL:    cfg.S3.UseSSL,
	}, assetStore)
	if err != nil {
		lg.Fatal("S3 客户端初始化失败", logger.F("err", err)...)
	}
	assetSvc := creation.NewAssetService(creationRepo, assetClient, assetStore, billingEngine)

	// 导出服务（S1-B08）：触发导出 + 列表 + 详情
	exportStore := repository.NewExportStore(pool)
	exportSvc := creation.NewExportService(creationRepo, assetClient, exportStore, billingEngine)

	// 画布服务（S2-B07）：画布 CRUD + 工作流执行
	canvasStore := repository.NewCanvasStore(pool)
	canvasSvc := creation.NewCanvasService(canvasStore)

	// 会员服务（S2-B09）：月卡购买/查询/取消 + SSE
	membershipStore := repository.NewMembershipStore(pool)
	memberSvc := creation.NewMembershipService(membershipStore)

	// 内容域（S3-B01~B03）：信息流/作品/互动/审核
	contentStore := repository.NewContentStore(pool)
	contentSvc := creation.NewContentService(contentStore, creationRepo)

	// 生态（S3-B04~B06）：模板/风格/任务
	marketStore := repository.NewMarketStore(pool)
	marketSvc := creation.NewMarketService(marketStore, creationRepo)

	// 管理后台（S3-B07）：指标/用户/内容/举报
	adminStore := repository.NewAdminStore(pool)
	adminSvc := creation.NewAdminService(adminStore)

	// 开发者（S3-B09）：API Key + Webhook
	openStore := repository.NewOpenAPIStore(pool)
	openSvc := creation.NewOpenAPIService(openStore)

	// S5 补齐服务集（接口覆盖率 89 端点）
	// 模型配置密钥（model_keys.key_encrypted 加解密；缺失回退 JWT_SECRET，仅开发期）
	modelEncSecret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if modelEncSecret == "" {
		modelEncSecret = cfg.JWT.Secret
	}
	modelKeyStore := repository.NewModelKeyStore(pool, cryptox.DeriveKey(modelEncSecret), false)
	extra := &creation.ExtraServices{
		Storyboard:  creation.NewStoryboardService(creationRepo),
		AssetDetail: creation.NewAssetDetailService(creationRepo),
		Biz:         creation.NewBizService(creationRepo),
		AdminExtra:  creation.NewAdminExtraService(creationRepo, modelKeyStore),
		CanvasExtra: creation.NewCanvasExtraService(creationRepo),
	}

	creation.NewHandler(creationSvc, assetSvc, exportSvc, canvasSvc, memberSvc, contentSvc, marketSvc, adminSvc, openSvc).RegisterRoutes(v1Auth, adminMW, auditMW, extra)

	// 厂商异步生成回调（无鉴权，签名由厂商侧保证）
	v1Pub.POST("/internal/model-callback/:provider", func(c *gin.Context) {
		extra.CanvasExtra.ModelCallback(c, c.Param("provider"))
	})

	// 公开查询端点（7.x 发现 / 9.x 套餐 / 10.x 模型 —— 无需登录）
	v1Pub.GET("/feed", contentSvc.PublicFeed)
	v1Pub.GET("/works/public", contentSvc.PublicWorksSearch)
	v1Pub.GET("/works/:work_id/public", func(c *gin.Context) { contentSvc.PublicWorkDetail(c, c.Param("work_id")) })
	v1Pub.GET("/packages", extra.Biz.ListPackages)
	v1Pub.GET("/models", extra.Biz.ListModels)
	v1Pub.GET("/point-packages", extra.Biz.ListPointPackages)

	// 计费模块（S1-B09）：9.3 积分账户 / 9.4 积分流水
	pointsAPI := billing.NewPointsAPI(billingStore)
	v1Auth.GET("/points", pointsAPI.Me)
	v1Auth.GET("/points/transactions", pointsAPI.Transactions)

	// 支付模块（S1-B10）：9.5 下单 / 9.7 回调 / 9.8 状态轮询
	orderStore := repository.NewOrderStore(pool)
	// envOrDefault 返回环境变量值或默认值。
	envOrDefault := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	// 生产环境强制关闭 mock 支付（P0 修复：防伪造回调）；mock 密钥生产环境不使用
	useMock := cfg.App.Env != "production" && os.Getenv("ALIPAY_USE_MOCK") != "false"
	alipaySvc := billing.NewAlipayService(orderStore, billing.AlipayConfig{
		AppID:      envOrDefault("ALIPAY_APP_ID", "2021000000000000"),
		MockSecret: envOrDefault("ALIPAY_MOCK_SECRET", "dev-alipay-mock-secret"),
		UseMock:    useMock,
	})
	payHandler := billing.NewPaymentHandler(alipaySvc)
	v1Auth.POST("/orders", payHandler.CreateOrder)
	v1Auth.GET("/orders/:order_no/status", payHandler.Status)
	v1Pub.POST("/orders/:order_no/pay-callback", payHandler.PayCallback) // 支付宝服务端回调（无鉴权，验签保护）

	// 通知引擎（S1-B11）：WS 端点 + TAG 8 通知 API
	notifyHub := notify.NewHub()
	go notifyHub.Run()
	wsEndpoint := notify.NewWSEndpoint(notifyHub, jwtMgr, cfg.App.Env)
	r.GET("/ws/v1", wsEndpoint.Handle) // WS 升级（token 鉴权）
	notifyStore := repository.NewNotifyStore(pool)
	notifyAPI := notify.NewNotifyAPI(notifyStore)
	v1Auth.GET("/notifications", notifyAPI.List)
	v1Auth.PUT("/notifications/:notification_id/read", notifyAPI.MarkRead)
	v1Auth.PUT("/notifications/read-all", notifyAPI.MarkAllRead)

	return r
}
