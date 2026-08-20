// VideoHandler：Worker-Video（VIDEO_SEGMENT_GEN / VIDEO_GROUP_CONCAT / FINAL_VIDEO_COMPOSE）。
// mock 视频生成 + 进度回写 + shots 状态 CAS 更新（回调幂等：RUNNING→SUCCESS 仅一次）。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// VideoStore 视频 Worker 数据边界。
type VideoStore interface {
	// LoadShot 按 shot_id 获取镜头信息（项目/关联资源）。
	LoadShot(ctx context.Context, shotID int64) (*ShotInfo, error)
	// SaveShotVideo 记录视频生成结果：shots.generated_video_url + video_generation_status=SUCCESS + 回写进度。
	// 幂等：status='PENDING' 或已失败才写入，已 SUCCESS 的直接返回（回调幂等）。
	SaveShotVideo(ctx context.Context, shotID int64, videoURL string, progressPct int, taskID int64) error
	// UpdateProgress 更新任务进度（独立调用，不阻塞主流程）。
	UpdateProgress(ctx context.Context, taskID int64, pct int, msg string) error
	// UpdateShotGroupStatus 更新镜头块状态（所有子镜头完成后）。
	UpdateShotGroupStatus(ctx context.Context, shotGroupID int64) error
}

// ShotInfo 镜头信息。
type ShotInfo struct {
	ShotID      int64
	ShotGroupID int64
	ProjectID   int64
	NarrativeUnitID int64
	DurationSec int
	PromptRefined string
}

// videoTaskTypes VIDEO_SEGMENT_GEN 等任务类型。
var videoTaskTypes = map[string]bool{
	"VIDEO_SEGMENT_GEN":  true,
	"VIDEO_GROUP_CONCAT": true,
}

// VideoHandler 视频生成 Worker。
type VideoHandler struct {
	store  VideoStore
	engine VideoGenClient // 可为 nil：未注入时降级 mock（开发期）
}

// VideoGenClient 视频生成能力（由 ModelEngine 实现注入）。
type VideoGenClient interface {
	GenerateVideo(ctx context.Context, capability model.Capability, req *model.VideoRequest) (*model.VideoResponse, error)
}

func NewVideoHandler(store VideoStore, engine VideoGenClient) *VideoHandler {
	return &VideoHandler{store: store, engine: engine}
}

func (h *VideoHandler) Name() string { return "VIDEO_SEGMENT_GEN" }

func (h *VideoHandler) Handles(taskType string) bool { return videoTaskTypes[taskType] }

// Execute 执行视频生成：解析目标 → mock 视频生成 → 进度更新 → shots 状态 CAS（回调幂等）。
func (h *VideoHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ShotID int64 `json:"shot_id"`
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	if params.ShotID == 0 {
		return nil, fmt.Errorf("缺少 shot_id")
	}

	// 加载镜头信息
	shot, err := h.store.LoadShot(ctx, params.ShotID)
	if err != nil {
		return nil, err
	}

	// 阶段性进度更新（模拟分步处理：提交→排队→渲染→完成）
	h.store.UpdateProgress(ctx, t.ID, 10, "模型排队中…")
	time.Sleep(50 * time.Millisecond) // mock 处理延迟

	h.store.UpdateProgress(ctx, t.ID, 40, "场景渲染中…")
	time.Sleep(100 * time.Millisecond)

	h.store.UpdateProgress(ctx, t.ID, 70, "镜头合成中…")
	time.Sleep(100 * time.Millisecond)

	// 真实视频生成（B06 ModelEngine.GenerateVideo）：配置了 T2V 厂商/Key 后走真实调用；
	// 无配置或调用失败降级 mock 样例视频（开发期链路保持可用）。
	videoURL := ""
	isMock := true
	if h.engine != nil && shot.PromptRefined != "" {
		resp, gerr := h.engine.GenerateVideo(ctx, model.CapT2V, &model.VideoRequest{
			Prompt:      shot.PromptRefined,
			DurationSec: shot.DurationSec,
			Resolution:  "P720",
		})
		if gerr == nil && resp.VideoURL != "" {
			videoURL = resp.VideoURL
			isMock = false
		} else {
			log.Printf("video: 真实视频生成失败(%v)，降级 mock shot=%d", gerr, shot.ShotID)
		}
	}
	if videoURL == "" {
		videoURL = fmt.Sprintf("%s/sample.mp4?shot=%d&t=%d",
			mockCDNBase(), shot.ShotID, time.Now().UnixMilli())
	}
	log.Printf("video: shot %d 视频 url=%s is_mock=%v", shot.ShotID, videoURL, isMock)

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
	})
	return out, nil
}