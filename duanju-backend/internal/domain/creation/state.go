// Package creation 创作模块：项目 CRUD + 流水线状态机（对齐 03 附录 A + 02 A2 枚举）。
package creation

import (
	"errors"
	"fmt"
)

// 项目状态（02 A2 project_status_enum 唯一真源）。
const (
	StatusDraft            = "DRAFT"
	StatusScriptSplitting  = "SCRIPT_SPLITTING"
	StatusScriptSplitFailed = "SCRIPT_SPLIT_FAILED"
	StatusPartialAssets    = "PARTIAL_ASSETS"
	StatusAllAssetsReady   = "ALL_ASSETS_READY"
	StatusVideoGenerating  = "VIDEO_GENERATING"
	StatusPartialVideos    = "PARTIAL_VIDEOS"
	StatusAllVideosReady   = "ALL_VIDEOS_READY"
	StatusExporting        = "EXPORTING"
	StatusCompleted        = "COMPLETED"
	StatusArchived         = "ARCHIVED"
	StatusCancelled        = "CANCELLED"
)

// 项目 Step（02 A2 project_step_enum）。
const (
	StepScriptSplit   = "SCRIPT_SPLIT"
	StepAssetGen      = "ASSET_GENERATION"
	StepVideoProd     = "VIDEO_PRODUCTION"
	StepFinalPreview  = "FINAL_PREVIEW"
)

// ErrInvalidTransition 非法状态转移（S1-B03 验收：状态机非法转移被拒）。
var ErrInvalidTransition = errors.New("invalid state transition")

// allowedTransitions 状态转移表（03 附录 A.1/A.2 成功主链 + 失败回退 + 用户操作）。
// key: 当前状态 → 允许的下一状态集合。
var allowedTransitions = map[string]map[string]bool{
	StatusDraft: {
		StatusScriptSplitting: true, // 提交拆分（03 A.2 DRAFT 行）
		StatusArchived:        true, // 用户归档
		StatusCancelled:       true, // 用户取消
	},
	StatusScriptSplitting: {
		StatusScriptSplitFailed: true, // 拆分失败（A.2 失败回 DRAFT 可重试）
		StatusPartialAssets:     true, // 拆分成功（映射 EPISODES_READY）
	},
	StatusScriptSplitFailed: {
		StatusScriptSplitting: true, // 重新拆分
		StatusDraft:           true, // 回到草稿重试
		StatusCancelled:       true,
	},
	StatusPartialAssets: {
		StatusScriptSplitting: true, // 重新拆分（覆盖旧拆分产物；限资产阶段，进入视频阶段后不可重拆）
		StatusAllAssetsReady:   true, // 资产全部成功
		StatusPartialAssets:    true, // 部分失败可重试（自身保持）
		StatusVideoGenerating:  true, // 可直接进 Step3（部分资产可用时）
		StatusCancelled:        true,
	},
	StatusAllAssetsReady: {
		StatusScriptSplitting: true, // 重新拆分（覆盖旧拆分产物）
		StatusVideoGenerating: true, // 生成分镜/视频
		StatusPartialAssets:   true, // 重新生成资产（回到 Step2）
		StatusArchived:        true,
		StatusCancelled:       true,
	},
	StatusVideoGenerating: {
		StatusPartialVideos:  true, // 部分成功
		StatusAllVideosReady: true, // 全部成功
		StatusCancelled:      true,
	},
	StatusPartialVideos: {
		StatusVideoGenerating: true, // 重试失败镜头
		StatusAllVideosReady:  true, // 补齐成功
		StatusCancelled:       true,
	},
	StatusAllVideosReady: {
		StatusExporting:  true, // 触发导出
		StatusCompleted:  true, // 导出完成
		StatusArchived:   true,
		StatusCancelled:  true,
	},
	StatusExporting: {
		StatusCompleted:     true, // 导出成功
		StatusAllVideosReady: true, // 导出失败回退（A.2：导出失败回到 UNITS_DONE）
		StatusCancelled:     true,
	},
	StatusCompleted: {
		StatusArchived: true, // 归档
	},
	StatusArchived: {
		StatusDraft: true, // 取消归档
	},
	StatusCancelled: {},
}

// ValidateTransition 校验状态转移；非法返回 ErrInvalidTransition。
func ValidateTransition(from, to string) error {
	allowed, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: 未知源状态 %s", ErrInvalidTransition, from)
	}
	if !allowed[to] {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
	}
	return nil
}

// StepForStatus 状态 → 前端 Step 映射（03 附录 A.5）。
func StepForStatus(status string) string {
	switch status {
	case StatusDraft, StatusScriptSplitting, StatusScriptSplitFailed:
		return StepScriptSplit
	case StatusPartialAssets, StatusAllAssetsReady:
		return StepAssetGen
	case StatusVideoGenerating, StatusPartialVideos, StatusAllVideosReady:
		return StepVideoProd
	case StatusExporting, StatusCompleted, StatusArchived, StatusCancelled:
		return StepFinalPreview
	default:
		return StepScriptSplit
	}
}
