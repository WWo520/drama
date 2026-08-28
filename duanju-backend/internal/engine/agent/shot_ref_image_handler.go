// ShotRefImageHandler：SHOT_REF_IMAGE_GEN 执行器——为单镜头生成专属参考图（对齐 LuxReal「镜头N的参考图」/ 智能镜头）。
// 输入：shot 的分镜提示词（PromptRefined）+ 资产参考图（shot_asset_refs 优先，否则 nu_asset_links），
// 以文本生图（首参考图作 I2I 输入）生成该镜头的镜头级参考图，落 shots.reference_image_url。
// 开发期降级 mock 占位图；生产接入图生模型且禁止 mock 降级。
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

// ShotRefImageStore 镜头参考图生成边界（由 repository.VideoStore 实现）。
type ShotRefImageStore interface {
	// LoadShot 镜头信息（注：可复用 VideoStore.LoadShot 读 PromptRefined）。
	LoadShot(ctx context.Context, shotID int64) (*ShotInfo, error)
	// LoadShotRefs 加载镜头参考图 URL（shot_asset_refs 优先，否则继承 nu_asset_links）。
	LoadShotRefs(ctx context.Context, shotID int64, narrativeUnitID int64) ([]string, error)
	// SaveShotRefImage 记录镜头参考图（shots.reference_image_url）。
	SaveShotRefImage(ctx context.Context, shotID int64, imageURL string, taskID int64) error
}

// ShotRefImageHandler 实现 task.Handler（task_type=SHOT_REF_IMAGE_GEN）。
type ShotRefImageHandler struct {
	store   ShotRefImageStore
	engine  ImageGenClient // 可为 nil：未注入时降级 mock（开发期）
	env     string
	persist ImagePersister // 可选：参考图转存自有 MinIO
}

// SetPersister 注入图片持久化（nil 不转存）。
func (h *ShotRefImageHandler) SetPersister(p ImagePersister) { h.persist = p }

// NewShotRefImageHandler 构造处理器。
func NewShotRefImageHandler(store ShotRefImageStore, engine ImageGenClient, env string) *ShotRefImageHandler {
	return &ShotRefImageHandler{store: store, engine: engine, env: env}
}

// Name 返回任务类型。
func (h *ShotRefImageHandler) Name() string { return "SHOT_REF_IMAGE_GEN" }

// Handles 判定是否处理该任务类型。
func (h *ShotRefImageHandler) Handles(taskType string) bool { return taskType == "SHOT_REF_IMAGE_GEN" }

// Execute 执行：读镜头 → 生成镜头级参考图 → 落库。
func (h *ShotRefImageHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ShotID int64 `json:"shot_id"`
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	if params.ShotID == 0 {
		return nil, fmt.Errorf("缺少 shot_id")
	}
	shot, err := h.store.LoadShot(ctx, params.ShotID)
	if err != nil {
		return nil, err
	}
	prompt := strings.TrimSpace(shot.PromptRefined)
	// 加载镜头已关联的资产参考图作为 I2I 输入（保持角色优先顺序）
	refURLs, _ := h.store.LoadShotRefs(ctx, params.ShotID, shot.NarrativeUnitID)

	imageURL := ""
	isMock := true
	if h.engine != nil && prompt != "" {
		refImage := ""
		if len(refURLs) > 0 {
			refImage = refURLs[0]
		}
		resp, gerr := h.engine.GenerateImage(ctx, model.CapT2I, &model.ImageRequest{
			Prompt:      prompt,
			RefImageURL: refImage,
			AspectRatio: "16:9",
			Resolution:  "1K",
		})
		if gerr == nil && resp.ImageURL != "" {
			imageURL = resp.ImageURL
			isMock = false
		} else {
			if h.env == "production" {
				return nil, fmt.Errorf("真实图生失败（生产禁用 mock 降级）: %w", gerr)
			}
			log.Printf("shot-ref-image: 真实图生失败(%v) 降级 mock shot=%d", gerr, params.ShotID)
		}
	} else if h.env == "production" && (h.engine == nil || prompt == "") {
		return nil, fmt.Errorf("生产环境未配置 T2I 模型或镜头提示词为空")
	}
	if imageURL == "" {
		imageURL = fmt.Sprintf("%s/shot-ref-placeholder.png?shot=%d&t=%d",
			mockCDNBase(), params.ShotID, time.Now().UnixMilli())
	}
	// 真实参考图转存自有 MinIO（同资产图：TOS 签名 24h 过期根因）
	if !isMock {
		imageURL = persistGeneratedImage(ctx, h.persist, shot.ProjectID, "SHOT", params.ShotID, imageURL)
	}
	if err := h.store.SaveShotRefImage(ctx, params.ShotID, imageURL, t.ID); err != nil {
		return nil, err
	}
	log.Printf("shot-ref-image: shot %d 参考图 url=%s is_mock=%v refs=%d", params.ShotID, imageURL, isMock, len(refURLs))
	out, _ := json.Marshal(map[string]any{
		"shot_id":   params.ShotID,
		"image_url": imageURL,
		"is_mock":   isMock,
	})
	return out, nil
}
