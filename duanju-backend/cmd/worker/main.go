// cmd/worker 入口：4 类 Worker 进程（split/asset/video/export）+ 调度引擎（Dispatcher/Reaper/消费循环）。
// -role 区分角色；split 角色额外启动 Dispatcher 与 Reaper（S1-B05 起注册具体 Handler）。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/engine/agent"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
	"github.com/duanju/backend/internal/pkg/cryptox"
	"github.com/duanju/backend/internal/pkg/db"
	"github.com/duanju/backend/internal/pkg/logger"
	"github.com/duanju/backend/internal/pkg/redisx"
	"github.com/duanju/backend/internal/repository"
)

func main() {
	role := flag.String("role", "split", "worker 角色: split/asset/video/export")
	concurrency := flag.Int("c", 2, "消费并发数")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	lg := logger.New(cfg.App.Env)
	defer func() { _ = lg.Sync() }()

	pool, err := db.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		lg.Fatal("数据库连接失败", logger.F("err", err)...)
	}
	defer pool.Close()
	rdb, err := redisx.New(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		lg.Fatal("Redis 连接失败", logger.F("err", err)...)
	}
	defer rdb.Close()

	// 调度引擎装配（S1-B04）
	store := repository.NewTaskStore(pool)
	queue := task.NewQueue(rdb, cfg.App.Env)

	// 模型引擎（B06）：DB 表驱动（model_providers/models/model_keys）+ mock 降级。
	// 管理端配置了厂商/模型/Key 后自动切真实调用；无配置时降级 mock 链路（开发期）。
	// 加密密钥：MODEL_KEY_ENC_SECRET（缺失回退 JWT_SECRET，仅开发期可用）。
	encSecret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if encSecret == "" {
		encSecret = cfg.JWT.Secret
	}
	keyStore := repository.NewModelKeyStore(pool, cryptox.DeriveKey(encSecret), true)
	// 真实 LLM Provider（OpenAI 兼容：DeepSeek/Qwen/GLM 等，按厂商 name 匹配注册）
	realLLM := model.NewOpenAICompatProvider("openai-compat", model.CapLLM)
	mockLLM := model.NewMockProvider("mock-llm", model.CapLLM)
	// 真实异步生成 Provider（火山引擎即梦/可灵风格：创建任务 + 轮询，覆盖 T2I/T2V/I2V）。
	// DB 配置厂商 name 为 volcengine/jimeng/kling 等时按名匹配；未配置时回退 mock（开发期）。
	volc := model.NewAsyncTaskProvider("volcengine", []model.Capability{model.CapT2I, model.CapI2I, model.CapT2V, model.CapI2V}, model.AsyncProviderOption{
		ImageCreatePath: "/api/v1/images/generations",
		ImageQueryPath:  "/api/v1/images/generations/{id}",
		VideoCreatePath: "/api/v1/videos/generations",
		VideoQueryPath:  "/api/v1/videos/generations/{id}",
	})
	jimeng := model.NewAsyncTaskProvider("jimeng", []model.Capability{model.CapT2I, model.CapI2I, model.CapT2V, model.CapI2V}, model.AsyncProviderOption{
		ImageCreatePath: "/api/v1/images/generations",
		ImageQueryPath:  "/api/v1/images/generations/{id}",
		VideoCreatePath: "/api/v1/videos/generations",
		VideoQueryPath:  "/api/v1/videos/generations/{id}",
	})
	kling := model.NewAsyncTaskProvider("kling", []model.Capability{model.CapT2I, model.CapI2I, model.CapT2V, model.CapI2V}, model.AsyncProviderOption{
		ImageCreatePath: "/v1/images/text2image",
		ImageQueryPath:  "/v1/images/text2image/{id}",
		VideoCreatePath: "/v1/videos/text2video",
		VideoQueryPath:  "/v1/videos/text2video/{id}",
	})
	mockImage := model.NewMockProvider("mock-image", model.CapT2I, model.CapI2I)
	mockVideo := model.NewMockProvider("mock-video", model.CapT2V, model.CapI2V)
	modelEngine := model.NewEngine([]model.Provider{
		realLLM, mockLLM, volc, jimeng, kling, mockImage, mockVideo,
	}, keyStore)

	// Agent 引擎（S1-B05）：SplitHandler 注册为 SCRIPT_SPLIT 执行器
	splitStore := repository.NewSplitStore(pool)
	splitHandler := agent.NewSplitHandler(modelEngine, splitStore)

	// Worker-Asset（S1-B06）：三种图生成任务
	assetStore := repository.NewAssetStore(pool)
	assetHandler := agent.NewAssetHandler(assetStore, modelEngine)

	// Worker-Video（S1-B07）：Step3 视频生成
	videoStore := repository.NewVideoStore(pool)
	videoHandler := agent.NewVideoHandler(videoStore, modelEngine)

	// Worker-Export（S1-B08）：Step4 导出 MP4
	exportStore := repository.NewExportStore(pool)
	exportHandler := agent.NewExportHandler(exportStore)

	// Dispatcher：Outbox → Redis ZADD（仅 split 角色启动，避免多角色重复投递竞争）
	if *role == "split" {
		dispatcher := task.NewDispatcher(store, queue)
		go dispatcher.Run(ctx)
		lg.Info("Dispatcher 已启动")
	}
	// Reaper：租约回收 + 重试重投（split 角色启动）
	if *role == "split" {
		reaper := task.NewReaper(store, queue)
		go reaper.Run(ctx)
		lg.Info("Reaper 已启动")
	}

	// 消费 Worker：按角色注册 Handler（split 注册 SplitHandler + AssetHandler）
	var handlers []task.Handler
	if *role == "split" {
		handlers = append(handlers, splitHandler)
	}
	if *role == "split" || *role == "asset" {
		handlers = append(handlers, assetHandler)
	}
	if *role == "split" || *role == "video" {
		handlers = append(handlers, videoHandler)
	}
	if *role == "split" || *role == "export" {
		handlers = append(handlers, exportHandler)
	}
	worker := task.NewWorker(store, queue, handlers)

	// 计费引擎（S1-B09）：任务结算回调（成功扣费/失败解冻）
	billingStore := repository.NewBillingStore(pool)
	billingEngine := billing.NewEngine(billingStore)
	worker.Settler = func(ctx context.Context, taskID int64, deliverable bool) error {
		// H04 生产加固：Worker 任务结算审计（operator=SYSTEM）
		action := "TASK_FAILED"
		if deliverable {
			action = "TASK_SETTLED"
		}
		if _, aerr := pool.Exec(ctx, `
			INSERT INTO audit_logs (operator_role, action, target_type, target_id, detail_json, risk_level)
			VALUES ('SYSTEM', $1, 'TASK', $2, jsonb_build_object('deliverable', $3::boolean), 'INFO')`,
			action, taskID, deliverable); aerr != nil {
			lg.Warn("审计写入失败", logger.F("err", aerr.Error())...)
		}
		return billingEngine.Settle(ctx, taskID, deliverable, 0, &billing.TxnMeta{})
	}
	for i := 0; i < *concurrency; i++ {
		go worker.Run(ctx)
	}
	lg.Info("Worker 已启动", logger.F("role", *role, "concurrency", *concurrency)...)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	lg.Info("Worker 正在退出...")
	_ = zap.S()
}
