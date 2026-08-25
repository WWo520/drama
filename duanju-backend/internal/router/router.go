// Package router 路由总入口（对齐 04 B.1：routes_user 用户端分组）。
package router

import (
	"context"
	"net/http"
	"os"
	"strings"
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
	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/notify"
	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/cryptox"
	"github.com/duanju/backend/internal/pkg/httpx"
	"github.com/duanju/backend/internal/pkg/jwt"
	"github.com/duanju/backend/internal/pkg/logger"
	"github.com/duanju/backend/internal/pkg/platformcfg"
	"github.com/duanju/backend/internal/repository"
)

// Setup 装配 Gin 引擎：中间件链 + 路由分组。
func Setup(cfg *config.Config, lg *zap.Logger, pool *pgxpool.Pool, rdb *redis.Client) *gin.Engine {
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 08 审查 Major#7：默认可信代理为空（直连部署），否则 X-Forwarded-For 可伪造 ClientIP 绕过限流/锁定/指纹
	// 若部署在 LB/网关后，用 TRUSTED_PROXIES 环境变量（逗号分隔 CIDR）配置
	if proxies := os.Getenv("TRUSTED_PROXIES"); proxies != "" {
		if err := r.SetTrustedProxies(strings.Split(proxies, ",")); err != nil {
			lg.Fatal("TRUSTED_PROXIES 配置错误", logger.F("err", err)...)
		}
	} else if err := r.SetTrustedProxies(nil); err != nil {
		lg.Fatal("可信代理配置失败", logger.F("err", err)...)
	}
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

	// 平台配置读取器（30s TTL 缓存）：维护模式中间件 + 注册开关 + 公开配置接口共用
	pfStore := platformcfg.New(pool)
	// 维护模式（feature_maintenance=true 拦截用户端；/auth /admin /healthz 放行）
	v1.Use(middleware.Maintenance(pfStore))

	jwtMW := middleware.JWT(jwtMgr, rdb, cfg.App.Env)

	// 公开路由组：仅 IP 限流（未登录 20/min）
	v1Pub := v1.Group("", middleware.RateLimit(rdb, pool, cfg.App.Env))

	// auth 模块（公开端点 + /auth/me 自带 JWT）
	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, jwtMgr, rdb, cfg.App.Env, lg, pfStore)
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
	space.NewHandler(spaceSvc, cfg.App.Env).RegisterRoutes(v1Auth)

	// 计费引擎（S1-B09）：任务冻结/结算（API 侧 Freeze + worker Settler）
	billingStore := repository.NewBillingStore(pool)
	billingEngine := billing.NewEngine(billingStore)

	// creation 模块（项目 CRUD + 拆分触发 + 层级读，S1-B03）
	creationRepo := creation.NewRepo(pool)
	creationSvc := creation.NewService(creationRepo, lg, billingEngine)

	// 模型配置密钥（model_keys.key_encrypted 加解密；缺失回退 JWT_SECRET，仅开发期）
	modelEncSecret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if modelEncSecret == "" && cfg.App.Env == "production" {
		lg.Fatal("生产环境必须配置 MODEL_KEY_ENC_SECRET（厂商密钥加密主密钥，禁止回退 JWT_SECRET）")
	}
	if modelEncSecret == "" {
		modelEncSecret = cfg.JWT.Secret
	}
	modelKeyStore := repository.NewModelKeyStore(pool, cryptox.DeriveKey(modelEncSecret), false)
	// LLM 引擎（资产弹窗 AI 优化提示词 + 剧本自动生成形象；未配置厂商时 mock 降级）
	dialogLLM := model.NewEngine([]model.Provider{
		model.NewOpenAICompatProvider("openai-compat", model.CapLLM),
		model.NewMockProvider("mock-llm", model.CapLLM),
	}, modelKeyStore)

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
	assetSvc := creation.NewAssetService(creationRepo, assetClient, assetStore, billingEngine, dialogLLM)

	// 头像读取：users.avatar_url 存 MinIO object key，读时现签 GET（24h）返回；
	// 前端 useAvatarUrl 拿到签名 URL 直接渲染 img（预签名 URL 无需鉴权，永不过期体验）。
	v1Auth.GET("/users/me/avatar", func(c *gin.Context) {
		key, err := authRepo.GetAvatarKey(c.Request.Context(), c.GetString(middleware.CtxUserUUID))
		if err != nil || key == "" {
			httpx.OK(c, gin.H{"avatar_url": ""})
			return
		}
		signed, err := assetClient.PresignDownload(c.Request.Context(), key)
		if err != nil {
			httpx.Fail(c, 500, httpx.CodeInternal, "头像地址签发失败")
			return
		}
		httpx.OK(c, gin.H{"avatar_url": signed})
	})

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

	// 开发者（S3-B09）：API Key + Webhook（secret 落库加密密钥：优先 WEBHOOK_ENC_SECRET，回退 MODEL_KEY_ENC_SECRET）
	webhookEncSecret := os.Getenv("WEBHOOK_ENC_SECRET")
	if webhookEncSecret == "" {
		webhookEncSecret = os.Getenv("MODEL_KEY_ENC_SECRET")
	}
	if webhookEncSecret == "" {
		webhookEncSecret = cfg.JWT.Secret // 开发期究底回退（生产由上方 MODEL_KEY_ENC_SECRET 强制检查兜底）
	}
	openStore := repository.NewOpenAPIStore(pool, cryptox.DeriveKey(webhookEncSecret))
	openSvc := creation.NewOpenAPIService(openStore)

	// S5 补齐服务集（接口覆盖率 89 端点）
	extra := &creation.ExtraServices{
		Storyboard:  creation.NewStoryboardService(creationRepo),
		AssetDetail: creation.NewAssetDetailService(creationRepo, dialogLLM),
		Biz:         creation.NewBizService(creationRepo),
		AdminExtra:  creation.NewAdminExtraService(creationRepo, modelKeyStore),
		CanvasExtra: creation.NewCanvasExtraService(creationRepo, cfg.App.Env),
	}

	creation.NewHandler(creationSvc, assetSvc, exportSvc, canvasSvc, memberSvc, contentSvc, marketSvc, adminSvc, openSvc, cfg.App.Env).RegisterRoutes(v1Auth, adminMW, auditMW, extra)

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

	// 公开运营内容（10.14~10.16）：平台公开配置（is_public=true）/ 公告（生效时间窗）/ 轮播（启用中）
	v1Pub.GET("/platform/public-config", func(c *gin.Context) {
		items, err := pfStore.PublicList(c.Request.Context())
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取公开配置失败")
			return
		}
		httpx.OK(c, gin.H{"items": items})
	})
	v1Pub.GET("/announcements", func(c *gin.Context) {
		items, err := adminStore.ListPublicAnnouncements(c.Request.Context())
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取公告失败")
			return
		}
		httpx.OK(c, gin.H{"items": items})
	})
	v1Pub.GET("/banners", func(c *gin.Context) {
		items, err := adminStore.ListPublicBanners(c.Request.Context())
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取轮播失败")
			return
		}
		httpx.OK(c, gin.H{"items": items})
	})

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
	alipayPubKey := envOrDefault("ALIPAY_PUBLIC_KEY", "")
	// 08 审查 Critical#1：生产缺 RSA2 公钥时回调验签 fail-closed 会拒收全部真实回调，直接拒绝启动避免资损静默
	if cfg.App.Env == "production" && alipayPubKey == "" {
		lg.Fatal("生产环境必须配置 ALIPAY_PUBLIC_KEY（支付宝 RSA2 验签公钥）")
	}
	alipaySvc := billing.NewAlipayService(orderStore, billing.AlipayConfig{
		AppID:      envOrDefault("ALIPAY_APP_ID", "2021000000000000"),
		MockSecret: envOrDefault("ALIPAY_MOCK_SECRET", "dev-alipay-mock-secret"),
		PublicKey:  alipayPubKey,
		UseMock:    useMock,
	})
	// 首次到账 → WS 推送积分变化（经 Redis 桥统一转发；前端据此停止订单状态轮询）
	// WS 订阅 topic 真源是 user:{user_uuid}（JWT uid 即 user_uuid），数字 id 需换查 UUID。
	alipaySvc.OnPaid = func(ctx context.Context, userID int64, points int) {
		var userUUID string
		if err := pool.QueryRow(ctx, `SELECT user_uuid::text FROM users WHERE id=$1`, userID).Scan(&userUUID); err != nil || userUUID == "" {
			return
		}
		_ = notify.PublishPush(ctx, rdb, cfg.App.Env, notify.PushEnvelope{
			UserID: userUUID,
			Type:   notify.TypePointsChanged,
			Data:   map[string]any{"points": points, "source": "order_paid", "ts": time.Now().Format(time.RFC3339)},
		})
	}
	payHandler := billing.NewPaymentHandler(alipaySvc)
	v1Auth.POST("/orders", payHandler.CreateOrder)
	v1Auth.GET("/orders/:order_no/status", payHandler.Status)
	v1Pub.POST("/orders/:order_no/pay-callback", payHandler.PayCallback) // 支付宝服务端回调（无鉴权，验签保护）
	// 开发环境模拟支付到账（仅 dev；生产不注册——与 C2/C3 同策略移除攻击面）
	if cfg.App.Env != "production" {
		v1Auth.POST("/orders/:order_no/mock-pay", payHandler.MockPay)
	}

	// 通知引擎（S1-B11）：WS 端点 + TAG 8 通知 API
	notifyHub := notify.NewHub()
	go notifyHub.Run()
	// 跨进程推送桥：Worker 任务终态/积分变化经 Redis Pub/Sub → Hub 定向推送（轮询对齐 LuxReal 的 WS 驱动基础）
	notify.StartRedisBridge(rdb, cfg.App.Env, notifyHub)
	wsEndpoint := notify.NewWSEndpoint(notifyHub, jwtMgr, cfg.App.Env)
	r.GET("/ws/v1", wsEndpoint.Handle) // WS 升级（token 鉴权）
	notifyStore := repository.NewNotifyStore(pool)
	notifyAPI := notify.NewNotifyAPI(notifyStore)
	v1Auth.GET("/notifications", notifyAPI.List)
	v1Auth.PUT("/notifications/:notification_id/read", notifyAPI.MarkRead)
	v1Auth.PUT("/notifications/read-all", notifyAPI.MarkAllRead)

	return r
}
