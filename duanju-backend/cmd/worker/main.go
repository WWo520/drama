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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/domain/creation"
	"github.com/duanju/backend/internal/engine/agent"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/notify"
	"github.com/duanju/backend/internal/engine/task"
	"github.com/duanju/backend/internal/pkg/cryptox"
	"github.com/duanju/backend/internal/pkg/db"
	"github.com/duanju/backend/internal/pkg/logger"
	"github.com/duanju/backend/internal/pkg/platformcfg"
	"github.com/duanju/backend/internal/pkg/redisx"
	"github.com/duanju/backend/internal/repository"
)

// envOr 环境变量兜底读取（worker 全局；main 内赋值实现）。
var envOr func(key, def string) string

func main() {
	role := flag.String("role", "split", "worker 角色: split/asset/video/export")
	concurrency := flag.Int("c", 2, "消费并发数")
	flag.Parse()

	envOr = func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

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
	// MiniMax H3（视频）：content[] 多模态结构（text + image_url role 标注），
	// 参考图 ≤9 张、时长 4~15s、提示词 ≤7000 字；DB 配置厂商 name=minimax 时按名匹配。
	minimax := model.NewMinimaxH3Provider("minimax", []model.Capability{model.CapT2V, model.CapI2V}, "", "")
	// AutoDL.Art 托管 ComfyUI（视频）：免开机 GPU 实例、按量计费的工作流包装 API；
	// 提交 POST {base_url} + 轮询 GET {base_url}/result/{task_id}；
	// DB 配置厂商 name=autodl-comfy，base_url 含工作流 ID（…/comfyui_workflow/{workflow_id}）。
	autodlComfy := model.NewAutodlComfyProvider("autodl-comfy", []model.Capability{model.CapT2V, model.CapI2V})
	mockImage := model.NewMockProvider("mock-image", model.CapT2I, model.CapI2I)
	mockVideo := model.NewMockProvider("mock-video", model.CapT2V, model.CapI2V)
	modelEngine := model.NewEngine([]model.Provider{
		realLLM, mockLLM, volc, jimeng, kling, minimax, autodlComfy, mockImage, mockVideo,
	}, keyStore)

	// Agent 引擎（S1-B05）：SplitHandler 注册为 SCRIPT_SPLIT 执行器
	splitStore := repository.NewSplitStore(pool)
	splitHandler := agent.NewSplitHandler(modelEngine, splitStore)
	// 拆分输出上限（max_tokens）管理端可配：platform_configs.split_max_tokens（30s 缓存实时生效），
	// 未配置/读取失败回退代码默认 32768。
	platformCfgStore := platformcfg.New(pool)
	splitHandler.SetMaxTokensResolver(func(ctx context.Context) int {
		v, _ := platformCfgStore.Int(ctx, "split_max_tokens", agent.DefaultSplitMaxTokens)
		return v
	})

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
	// 真实合成：MinIO + ffmpeg（FFMPEG_PATH 覆盖可执行路径）；S3 未配置时回退 mock 链路。
	exportStore := repository.NewExportStore(pool)
	exportHandler := agent.NewExportHandler(exportStore)
	if cfg.S3.Endpoint != "" && cfg.S3.Bucket != "" {
		mc, merr := minio.New(cfg.S3.Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.S3.AccessKey, cfg.S3.SecretKey, ""),
			Secure: cfg.S3.UseSSL,
		})
		if merr != nil {
			lg.Warn("S3 客户端初始化失败，导出回退 mock", logger.F("err", merr.Error())...)
		} else {
			exportHandler.SetComposer(exportStore, agent.NewFFmpegConcat(exportStore, mc, cfg.S3.Bucket))
			lg.Info("导出真实合成已装配（ffmpeg+MinIO）", logger.F("bucket", cfg.S3.Bucket, "endpoint", cfg.S3.Endpoint)...)
			// 资产图持久化（ENABLE_IMAGE_PERSIST=1 开启）：生成图转存到「远端视频 GPU 可达」
			// 的对象存储，固定 URL 永不过期（火山 TOS 签名 24h 过期 → 图裂的根因）。
			// 存储目标默认复用 S3_*；用 IMGSTORE_* 可指向独立桶——典型场景为火山 TOS 的
			// S3 兼容端点（tos-s3-{region}.volces.com），云端 AutoDL 直接拉取。
			if os.Getenv("ENABLE_IMAGE_PERSIST") == "1" {
				imgEp := envOr("IMGSTORE_ENDPOINT", cfg.S3.Endpoint)
				imgAK := envOr("IMGSTORE_ACCESS_KEY", cfg.S3.AccessKey)
				imgSK := envOr("IMGSTORE_SECRET_KEY", cfg.S3.SecretKey)
				imgBK := envOr("IMGSTORE_BUCKET", cfg.S3.Bucket)
				imgRegion := envOr("IMGSTORE_REGION", "cn-beijing")
				imgPublic := envOr("IMGSTORE_PUBLIC_BASE",
					fmt.Sprintf("https://{bucket}.tos-%s.volces.com", imgRegion)) // TOS 虚拟主机式默认
				if strings.Contains(imgEp, "volces.com") || strings.Contains(imgPublic, "volces.com") {
					// 火山 TOS：minio-go 签名不兼容（实测 SignatureDoesNotMatch），走官方 SDK。
					mp, terr := agent.NewTOSImagePersist(imgRegion, imgEp, imgAK, imgSK, imgBK, imgPublic)
					if terr != nil {
						lg.Warn("TOS 客户端初始化失败，参考图走厂商原始 URL", logger.F("err", terr.Error())...)
					} else {
						if perr := mp.EnsurePublicRead(ctx); perr != nil {
							lg.Warn("桶匿名读策略设置失败（可在控制台手动设为公共读，不影响上传转存）", logger.F("err", perr.Error())...)
						}
						assetHandler.SetPersister(mp)
						shotRefHandler.SetPersister(mp)
						lg.Info("资产图持久化已装配(TOS)", logger.F(
							"endpoint", imgEp, "bucket", imgBK, "public_base", imgPublic)...)
					}
				} else {
					imgSSL := strings.EqualFold(os.Getenv("IMGSTORE_USE_SSL"), "true")
					imgMC, imerr := minio.New(imgEp, &minio.Options{
						Creds:  credentials.NewStaticV4(imgAK, imgSK, ""),
						Secure: imgSSL,
					})
					if imerr != nil {
						lg.Warn("图存客户端初始化失败，参考图走厂商原始 URL", logger.F("err", imerr.Error())...)
					} else {
						mp := agent.NewMinIOImagePersist(imgMC, imgBK, imgPublic)
						if perr := mp.EnsurePublicRead(ctx); perr != nil {
							lg.Warn("桶匿名读策略设置失败（可在控制台手动设为公共读，不影响上传转存）", logger.F("err", perr.Error())...)
						}
						assetHandler.SetPersister(mp)
						shotRefHandler.SetPersister(mp)
						videoHandler.SetPersister(mp)
						lg.Info("资产图持久化已装配(MinIO)", logger.F(
							"endpoint", imgEp, "bucket", imgBK, "public_base", imgPublic)...)
					}
				}
			} else {
				lg.Info("资产图持久化未启用（ENABLE_IMAGE_PERSIST!=1），保留厂商原始 URL")
			}
		}
	} else {
		lg.Info("S3 未配置，导出走 mock 链路")
	}

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

	// WS 订阅 topic 真源是 user:{user_uuid}（JWT uid 即 user_uuid），数字 id 需换查 UUID。
	userUUID := func(ctx context.Context, id int64) string {
		var u string
		if err := pool.QueryRow(ctx, `SELECT user_uuid::text FROM users WHERE id=$1`, id).Scan(&u); err != nil {
			return ""
		}
		return u
	}

	// 拆分真实进度上报（阶段化文案：读剧本→逐集镜头设计→资产扩写→润色→入库）。
	// 写 task progress（断线轮询可见）+ Redis WS 桥推 TASK_PROGRESS（在线即时可见）；
	// 2s 节流防进度风暴。当前执行任务 ID 由 Worker 回调 OnTaskStart 原子设置。
	var curTaskID atomic.Int64
	worker.OnTaskStart = func(t *task.Task) {
		if t.TaskType == "SCRIPT_SPLIT" {
			curTaskID.Store(t.ID)
		}
	}
	if *role == "split" {
		var lastPush int64 // unix nano，atomic 节流
		splitHandler.SetProgressReporter(func(pct int, msg string) {
			taskID := curTaskID.Load()
			if taskID == 0 {
				return
			}
			_, _ = pool.Exec(ctx, `UPDATE generation_tasks SET progress_pct=$2, progress_message=$3 WHERE id=$1`, taskID, pct, msg)
			now := time.Now().UnixNano()
			last := atomic.LoadInt64(&lastPush)
			if now-last < 2*time.Second.Nanoseconds() || !atomic.CompareAndSwapInt64(&lastPush, last, now) {
				return
			}
			var userID int64
			var taskUUID string
			if err := pool.QueryRow(ctx,
				`SELECT user_id, task_uuid::text FROM generation_tasks WHERE id=$1`, taskID).Scan(&userID, &taskUUID); err != nil {
				return
			}
			if uuid := userUUID(ctx, userID); uuid != "" {
				_ = notify.PublishPush(ctx, rdb, cfg.App.Env, notify.PushEnvelope{
					UserID: uuid,
					Type:   notify.TypeTaskProgress,
					Data: map[string]any{
						"task_type":        "SCRIPT_SPLIT",
						"task_uuid":        taskUUID,
						"status":           "RUNNING",
						"progress_pct":     pct,
						"progress_message": msg,
					},
				})
			}
		})
	}

	// 任务终态 WS 推送（跨进程桥：Redis Pub/Sub → API 进程 Hub → 用户 WS）
	worker.Notifier = func(ctx context.Context, t *task.Task, status, reason string) {
		if status == task.StatusFailedDead && t.TaskType == splitHandler.Name() && t.ProjectID != nil {
			// 拆分任务死信兜底回滚：Execute 内部已回滚，但 SaveSplitResult DB 异常等路径漏回滚时，
			// 项目会永久卡在 SCRIPT_SPLITTING（前端无限轮询"拆分中"）。幂等：仅 SCRIPT_SPLITTING 状态生效。
			if rerr := splitStore.MarkSplitFailed(ctx, *t.ProjectID); rerr != nil {
				lg.Warn("拆分任务死信回滚项目状态失败", logger.F("err", rerr.Error(), "project_id", *t.ProjectID)...)
			}
		}
		if status == task.StatusFailedDead && assetHandler.Handles(t.TaskType) {
			// 资产图生成任务死信：回写资产表 FAILED_DEAD（前端进度条消失、按钮恢复，避免卡"生成中"）
			if rerr := assetStore.MarkAssetDeadByTask(ctx, t.ID); rerr != nil {
				lg.Warn("资产任务死信回写失败", logger.F("err", rerr.Error(), "task_id", t.ID)...)
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
		// 读取任务 space_id/user_id/project_id 填充计费快照（空 TxnMeta 会导致
		// point_transactions.related_space_id / operator_user_id 外键失败——曾漏填
		// OperatorUserID=0 无对应用户，结算整体回滚、冻结积分永久卡死）
		meta := &billing.TxnMeta{}
		var spaceID, userID int64
		var projectID *int64
		if err := pool.QueryRow(ctx, `SELECT space_id, user_id, project_id FROM generation_tasks WHERE id=$1`, taskID).Scan(&spaceID, &userID, &projectID); err == nil {
			meta.SpaceID = spaceID
			meta.OperatorUserID = userID
			meta.ProjectID = projectID
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
	// 业务对象终败归位（OnDead）：视频任务死信 → 卡片/镜头 FAILED，防止前端永久"生成中"
	creationRepo := creation.NewRepo(pool)
	worker.OnDead = func(ctx context.Context, t *task.Task, reason string) {
		switch t.TaskType {
		case "VIDEO_SEGMENT_GEN", "VIDEO_GROUP_CONCAT":
			creationRepo.MarkVideoFailedByTask(ctx, t.RequestParamsJSON)
			lg.Warn("视频任务死信归位", logger.F("task_id", t.ID, "type", t.TaskType, "reason", reason)...)
		}
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
