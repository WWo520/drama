// AssetHandler：Worker-Asset（CHAR_IMAGE_GEN/SCENE_IMAGE_GEN/PROP_IMAGE_GEN 执行器）。
// 开发期 mock 图片生成（生成占位图 URL）；生产接入图生模型（B06 ModelEngine.GenerateImage）。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/duanju/backend/internal/engine/task"
)

// AssetStore 资产生成边界（由 repository.AssetStore 实现）。
type AssetStore interface {
	// LoadAssetTarget 解析任务目标（kind/target_id → 项目/集/积分）。
	LoadAssetTarget(ctx context.Context, targetKind string, targetID int64) (*AssetTarget, error)
	// SaveGeneratedImage 记录生成图 + 更新业务表 + usage log（事务）。
	SaveGeneratedImage(ctx context.Context, t *AssetTarget, imageURL string, taskID int64) error
}

// AssetTarget 资产生成目标（与 repository.AssetTarget 对齐，接口隔离避免依赖方向反转）。
type AssetTarget struct {
	Kind      string
	TargetID  int64
	ProjectID int64
	EpisodeID int64
	AssetID   *int64
	PointCost int
}

// AssetHandler 实现 task.Handler（三种资产生成任务）。
type AssetHandler struct {
	store AssetStore
}

func NewAssetHandler(store AssetStore) *AssetHandler {
	return &AssetHandler{store: store}
}

// Name 支持的任务类型。
func (h *AssetHandler) Name() string { return "CHAR_IMAGE_GEN" }

// 支持的目标资产类型（asset_kind_enum 真源：CHARACTER_VARIANT/SCENE/PROP）。
var assetTaskTypes = map[string]bool{
	"CHARACTER_VARIANT": true,
	"SCENE":             true,
	"PROP":              true,
}

// Handles 判断是否处理该任务类型（task_type_enum：三种图生成任务）。
func (h *AssetHandler) Handles(taskType string) bool {
	switch taskType {
	case "CHAR_IMAGE_GEN", "SCENE_IMAGE_GEN", "PROP_IMAGE_GEN":
		return true
	}
	return false
}

// Execute 执行：解析目标 → mock 生成图 URL → 事务入库。
func (h *AssetHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		TargetKind string `json:"target_kind"`
		TargetID   int64  `json:"target_id"`
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	if !assetTaskTypes[params.TargetKind] || params.TargetID == 0 {
		return nil, fmt.Errorf("非法资产生成参数: kind=%s id=%d", params.TargetKind, params.TargetID)
	}
	target, err := h.store.LoadAssetTarget(ctx, params.TargetKind, params.TargetID)
	if err != nil {
		return nil, err
	}

	// mock 图片生成（S1-B06 开发期：生成占位 URL；S1-B07 接入真实图生模型）
	imageURL := fmt.Sprintf("https://mock.cdn.example.com/generated/%s/%d-%d.png",
		params.TargetKind, params.TargetID, time.Now().UnixMilli())
	log.Printf("asset: %s 生成 mock 图片 target=%d url=%s", params.TargetKind, params.TargetID, imageURL)

	// 事务入库（generated_images + 业务表 + usage log）
	if err := h.store.SaveGeneratedImage(ctx, target, imageURL, t.ID); err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]any{
		"image_url":   imageURL,
		"target_kind": params.TargetKind,
		"target_id":   params.TargetID,
		"is_mock":     true,
	})
	return out, nil
}
