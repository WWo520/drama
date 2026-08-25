// VideoHandler：Worker-Video（VIDEO_SEGMENT_GEN / VIDEO_GROUP_CONCAT / FINAL_VIDEO_COMPOSE）。
// 真实/降级视频生成 + 进度回写 + shots 状态 CAS 更新（回调幂等：RUNNING→SUCCESS 仅一次）。
// 18 - 视频制作 阶段一：参考图注入（去重 + 上限 + 尾部全局约束指令）与生成参数透传。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// VideoStore 视频 Worker 数据边界。
type VideoStore interface {
	// LoadShot 按 shot_id 获取镜头信息（项目/关联资源）。
	LoadShot(ctx context.Context, shotID int64) (*ShotInfo, error)
	// LoadShotRefs 加载镜头参考图 URL 列表（F10：shot_asset_refs 优先，否则继承 nu_asset_links）。
	LoadShotRefs(ctx context.Context, shotID int64, narrativeUnitID int64) ([]string, error)
	// SaveShotVideo 记录视频生成结果：shots.generated_video_url + video_generation_status=SUCCESS + 回写进度。
	// 幂等：status='PENDING' 或已失败才写入，已 SUCCESS 的直接返回（回调幂等）。
	SaveShotVideo(ctx context.Context, shotID int64, videoURL string, progressPct int, taskID int64) error
	// UpdateProgress 更新任务进度（独立调用，不阻塞主流程）。
	UpdateProgress(ctx context.Context, taskID int64, pct int, msg string) error
	// UpdateShotGroupStatus 更新镜头块状态（所有子镜头完成后）。
	UpdateShotGroupStatus(ctx context.Context, shotGroupID int64) error
	// ConcatShotGroup 镜头块拼接（VIDEO_GROUP_CONCAT）：块内全部 SUCCESS 才标记完成，否则报错。
	ConcatShotGroup(ctx context.Context, shotGroupID int64) error
}

// ShotInfo 镜头信息。
type ShotInfo struct {
	ShotID            int64
	ShotGroupID       int64
	ProjectID         int64
	NarrativeUnitID   int64
	DurationSec       int
	PromptRefined     string
	ReferenceImageURL string // 镜头级参考图（对齐 LuxReal「镜头N的参考图」）
}

// videoTaskTypes VIDEO_SEGMENT_GEN 等任务类型。
var videoTaskTypes = map[string]bool{
	"VIDEO_SEGMENT_GEN":  true,
	"VIDEO_GROUP_CONCAT": true,
}

// videoTailConstraint 尾部全局约束指令（对齐 LuxReal：Agent 自动追加、不可删改的硬约束，
// 钉住三件事——忠于脚本、忠于参考图、禁字幕）。
const videoTailConstraint = "\n严格按照分镜脚本进行生成，参考上传图的人物形象、场景和道具。不要生成字幕。"

// maxShotRefImages 单镜头参考图上限（火山 Seedance 多图参考上限 4 张；
// LoadShotRefs 已按 角色→场景→道具 排序，截断保留主体一致性权重最高的前 N 张）。
const maxShotRefImages = 4

// VideoHandler 视频生成 Worker。
type VideoHandler struct {
	store  VideoStore
	engine VideoGenClient // 可为 nil：未注入时降级 mock（开发期）
	env    string         // 08 审查 Major#16：生产环境禁止 mock 降级
}

// VideoGenClient 视频生成能力（由 ModelEngine 实现注入）。
type VideoGenClient interface {
	GenerateVideo(ctx context.Context, capability model.Capability, req *model.VideoRequest) (*model.VideoResponse, error)
}

func NewVideoHandler(store VideoStore, engine VideoGenClient, env string) *VideoHandler {
	return &VideoHandler{store: store, engine: engine, env: env}
}

func (h *VideoHandler) Name() string { return "VIDEO_SEGMENT_GEN" }

func (h *VideoHandler) Handles(taskType string) bool { return videoTaskTypes[taskType] }

// Execute 执行视频生成：解析目标 → 加载参考图 → 真实/降级生成 → shots 状态 CAS（回调幂等）。
func (h *VideoHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ShotID      int64  `json:"shot_id"`
		ShotGroupID int64  `json:"shot_group_id"`
		Resolution  string `json:"resolution"` // 弹窗提交（P480/P720/P1080）；空=默认 P720
		ModelKey    string `json:"model_key"`  // 弹窗提交；空=keystore 默认路由
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	// 镜头块拼接（3.3.8）：块内全部 SUCCESS → 标记 COMPOSED。
	if t.TaskType == "VIDEO_GROUP_CONCAT" {
		return h.executeGroupConcat(ctx, params.ShotGroupID)
	}
	if params.ShotID == 0 {
		return nil, fmt.Errorf("缺少 shot_id")
	}

	// 加载镜头信息
	shot, err := h.store.LoadShot(ctx, params.ShotID)
	if err != nil {
		return nil, err
	}

	// F10: 加载镜头参考图（shot_asset_refs 优先，否则继承 nu_asset_links）
	refURLs, err := h.store.LoadShotRefs(ctx, params.ShotID, shot.NarrativeUnitID)
	if err != nil {
		log.Printf("video: 加载参考图失败 shot=%d: %v", shot.ShotID, err)
		refURLs = nil // 降级：无参考图继续生成
	}
	// 镜头级专属参考图（对齐 LuxReal「镜头N的参考图」）：作为最高优先级参考图注入，
	// 与角色/场景基础资产图（LoadShotRefs 内角色优先排序）合并后去重截断。
	if strings.TrimSpace(shot.ReferenceImageURL) != "" {
		refURLs = append([]string{shot.ReferenceImageURL}, refURLs...)
	}
	refURLs = dedupeRefs(refURLs)
	if len(refURLs) > 0 {
		log.Printf("video: shot %d 参考图 %d 张 target=%s", shot.ShotID, len(refURLs), refURLs[0])
	}

	// 阶段性进度更新（模拟分步处理：提交→排队→渲染→完成）
	h.store.UpdateProgress(ctx, t.ID, 10, "模型排队中…")
	time.Sleep(50 * time.Millisecond) // mock 处理延迟

	h.store.UpdateProgress(ctx, t.ID, 40, "场景渲染中…")
	time.Sleep(100 * time.Millisecond)

	h.store.UpdateProgress(ctx, t.ID, 70, "镜头合成中…")
	time.Sleep(100 * time.Millisecond)

	// 提示词 = 润色提示词 + 尾部全局约束指令（对齐 LuxReal：忠于脚本/忠于参考图/禁字幕）。
	prompt := shot.PromptRefined + videoTailConstraint
	resolution := params.Resolution
	if resolution == "" {
		resolution = "P720"
	}

	// 真实视频生成（B06 ModelEngine.GenerateVideo）：配置了 T2V 厂商/Key 后走真实调用；
	// 无配置或调用失败降级 mock 样例视频（开发期链路保持可用）。
	videoURL := ""
	isMock := true
	if h.engine != nil && shot.PromptRefined != "" {
		resp, gerr := h.engine.GenerateVideo(ctx, model.CapT2V, &model.VideoRequest{
			Prompt:       prompt,
			RefImageURLs: refURLs, // F10: 多参考图注入
			DurationSec:  shot.DurationSec,
			Resolution:   resolution,
			ModelKey:     params.ModelKey,
		})
		if gerr == nil && resp.VideoURL != "" {
			videoURL = resp.VideoURL
			isMock = false
		} else {
			// 08 审查 Major#16：生产环境禁止降级 mock（避免按成功扣费给用户样例视频）
			if h.env == "production" {
				return nil, fmt.Errorf("真实视频生成失败（生产禁用 mock 降级）: %w", gerr)
			}
			log.Printf("video: 真实视频生成失败(%v)，降级 mock shot=%d", gerr, shot.ShotID)
		}
	} else if h.env == "production" && (h.engine == nil || shot.PromptRefined == "") {
		return nil, fmt.Errorf("生产环境未配置 T2V 模型或镜头提示词为空")
	}
	if videoURL == "" {
		videoURL = fmt.Sprintf("%s/sample.mp4?shot=%d&t=%d",
			mockCDNBase(), shot.ShotID, time.Now().UnixMilli())
	}
	log.Printf("video: shot %d 视频 url=%s is_mock=%v refs=%d", shot.ShotID, videoURL, isMock, len(refURLs))

	// 回调幂等：SaveShotVideo 内 CAS（仅 PENDING/FAILED 状态写入，已 SUCCESS 跳过）
	if err := h.store.SaveShotVideo(ctx, shot.ShotID, videoURL, 100, t.ID); err != nil {
		return nil, err
	}

	// 更新镜头块状态（检查同块内所有镜头是否全部完成）
	if err := h.store.UpdateShotGroupStatus(ctx, shot.ShotGroupID); err != nil {
		log.Printf("video: 更新镜头块状态 shot_group=%d: %v", shot.ShotGroupID, err)
	}

	out, _ := json.Marshal(map[string]any{
		"video_url":     videoURL,
		"shot_id":       shot.ShotID,
		"shot_group_id": shot.ShotGroupID,
		"progress_pct":  100,
		"is_mock":       isMock,
		"ref_images":    len(refURLs),
	})
	return out, nil
}

// executeGroupConcat 镜头块拼接（VIDEO_GROUP_CONCAT）。
func (h *VideoHandler) executeGroupConcat(ctx context.Context, shotGroupID int64) ([]byte, error) {
	if shotGroupID == 0 {
		return nil, fmt.Errorf("缺少 shot_group_id")
	}
	if err := h.store.ConcatShotGroup(ctx, shotGroupID); err != nil {
		return nil, err
	}
	log.Printf("video: shot_group %d 拼接完成（COMPOSED）", shotGroupID)
	out, _ := json.Marshal(map[string]any{
		"shot_group_id": shotGroupID,
		"status":        "COMPOSED",
	})
	return out, nil
}

// dedupeRefs 参考图去重 + 截断上限（角色→场景→道具序保持）。
func dedupeRefs(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
		if len(out) >= maxShotRefImages {
			break
		}
	}
	return out
}
