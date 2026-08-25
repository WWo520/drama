// cmd/worker 入口：4 类 Worker 进程（split/asset/video/export）+ 调度引擎（Dispatcher/Reaper/消费循环）。
// -role 区分角色；split 角色额外启动 Dispatcher 与 Reaper（S1-B05 起注册具体 Handler）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/engine/agent"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/notify"
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
	// 真实异步生成 Provider（火山引擎即梦/可灵风格：创建任务 + 轮询，覆盖 T2V/I2V）。
	// DB 配置厂商 name 为 volcengine/jimeng/kling 等时按名匹配；未配置时回退 mock（开发期）。
	// 火山 Seedream 图片走同步 images/generations（见 volcImage）；视频（Seedance）走 contents/generations/tasks。
	volcVideo := model.NewAsyncTaskProvider("volcengine", []model.Capability{model.CapT2V, model.CapI2V}, model.AsyncProviderOption{
		VideoCreatePath: "/api/v3/contents/generations/tasks",
		VideoQueryPath:  "/api/v3/contents/generations/tasks/{id}",
	})
	volcImage := model.NewVolcImageProvider("volcengine", []model.Capability{model.CapT2I, model.CapI2I}, "")
	volc := model.MergeProviders(volcImage, volcVideo)
	jimeng := model.NewAsyncTaskProvider("jimeng", []model.Capability{model.CapT2I, model.CapI2I, model.CapT2V, model.CapI2V}, model.AsyncProviderOption{
		ImageCreatePath: "/api/v3/contents/generations/tasks",
		ImageQueryPath:  "/api/v3/contents/generations/tasks/{id}",
		VideoCreatePath: "/api/v3/contents/generations/tasks",
		VideoQueryPath:  "/api/v3/contents/generations/tasks/{id}",
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

	// 18 - 视频制作 阶段二：分镜脚本独立生成（STORYBOARD_PLAN_V1_LUXCHAT，单叙事单元重跑分镜技能）
	storyboardStore := repository.NewStoryboardStore(pool)
	storyboardPlanHandler := agent.NewStoryboardPlanHandler(modelEngine, storyboardStore)

	// Worker-Asset（S1-B06）：三种图生成任务
	assetStore := repository.NewAssetStore(pool)
	assetHandler := agent.NewAssetHandler(assetStore, modelEngine, cfg.App.Env)

	// Worker-Video（S1-B07）：Step3 视频生成
	videoStore := repository.NewVideoStore(pool)
	videoHandler := agent.NewVideoHandler(videoStore, modelEngine, cfg.App.Env)

	// 18 - 视频制作：镜头级参考图生成（对齐 LuxReal「镜头N的参考图」，SHOT_REF_IMAGE_GEN，归入 asset 能力）
	shotRefHandler := agent.NewShotRefImageHandler(videoStore, modelEngine, cfg.App.Env)

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
		handlers = append(handlers, storyboardPlanHandler) // 18 - 视频制作：分镜独立生成
	}
	if *role == "split" || *role == "asset" {
		handlers = append(handlers, assetHandler)
		handlers = append(handlers, shotRefHandler) // 镜头级参考图生成（asset 能力）
	}
	if *role == "split" || *role == "video" {
		handlers = append(handlers, videoHandler)
	}
	if *role == "split" || *role == "export" {
		handlers = append(handlers, exportHandler)
	}
	worker := task.NewWorker(store, queue, handlers)

	// 任务终态 WS 推送（跨进程桥：Redis Pub/Sub → API 进程 Hub → 用户 WS；轮询对齐 LuxReal 的 WS 驱动基础）
	// 注意：WS 订阅 topic 真源是 user:{user_uuid}（JWT uid 即 user_uuid），数字 id 需换查 UUID。
	userUUID := func(ctx context.Context, id int64) string {
		var u string
		if err := pool.QueryRow(ctx, `SELECT user_uuid::text FROM users WHERE id=$1`, id).Scan(&u); err != nil {
			return ""
		}
		return u
	}
	worker.Notifier = func(ctx context.Context, t *task.Task, status, reason string) {
		if status == task.StatusFailedDead && t.TaskType == splitHandler.Name() && t.ProjectID != nil {
			// 拆分任务死信兜底回滚：Execute 内部已回滚，但 SaveSplitResult DB 异常等路径漏回滚时，
			// 项目会永久卡在 SCRIPT_SPLITTING（前端无限轮询"拆分中"）。幂等：仅 SCRIPT_SPLITTING 状态生效。
			if rerr := splitStore.MarkSplitFailed(ctx, *t.ProjectID); rerr != nil {
				lg.Warn("拆分任务死信回滚项目状态失败", logger.F("err", rerr.Error(), "project_id", *t.ProjectID)...)
			}
		}
		uuid := userUUID(ctx, t.UserID)
		if uuid == "" {
			return
		}
		msgType := notify.TypeTaskSuccess
		if status == task.StatusFailedDead {
			msgType = notify.TypeTaskFailed
		}
		data := map[string]any{
			"task_uuid": t.TaskUUID,
			"task_type": t.TaskType,
			"status":    status,
			"ts":        time.Now().Format(time.RFC3339),
		}
		if reason != "" {
			data["reason"] = reason
		}
		if perr := notify.PublishPush(ctx, rdb, cfg.App.Env, notify.PushEnvelope{
			UserID: uuid,
			Type:   msgType,
			Data:   data,
		}); perr != nil {
			lg.Debug("任务终态推送失败", logger.F("err", perr.Error())...)
		}
	}

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
		// 读取任务 space_id 填充计费快照（空 TxnMeta 会导致 point_transactions.related_space_id 外键失败）
		meta := &billing.TxnMeta{}
		var spaceID, userID int64
		if err := pool.QueryRow(ctx, `SELECT space_id, user_id FROM generation_tasks WHERE id=$1`, taskID).Scan(&spaceID, &userID); err == nil {
			meta.SpaceID = spaceID
		}
		if err := billingEngine.Settle(ctx, taskID, deliverable, 0, meta); err != nil {
			if errors.Is(err, billing.ErrAlreadySettled) {
				return nil // 幂等：已结算（重复回调），不重复推送
			}
			return err
		}
		// 积分余额变化推送（前端据此刷新余额/停止轮询；topic 用 user_uuid）
		if userID > 0 {
			if uuid := userUUID(ctx, userID); uuid != "" {
				if perr := notify.PublishPush(ctx, rdb, cfg.App.Env, notify.PushEnvelope{
					UserID: uuid,
					Type:   notify.TypePointsChanged,
					Data:   map[string]any{"source": "task_settle", "ts": time.Now().Format(time.RFC3339)},
				}); perr != nil {
					lg.Debug("积分变化推送失败", logger.F("err", perr.Error())...)
				}
			}
		}
		return nil
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
