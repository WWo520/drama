// ExportHandler：Worker-Export（FINAL_VIDEO_COMPOSE / ENGINEER_FILE_EXPORT）。
// mock 视频合成（模拟 FFmpeg concat）+ 成片 URL + export_records 回写（30 天过期）。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/duanju/backend/internal/engine/task"
)

// ExportStore 导出 Worker 数据边界。
type ExportStore interface {
	// CreateExportRecord 创建导出记录（PENDING；返回 export_id）。
	CreateExportRecord(ctx context.Context, userID, spaceID, projectID int64, exportType, resolution string, watermark bool, taskID int64) (int64, error)
	// CompleteExport 完成导出：SUCCESS + 文件信息 + 30 天过期。
	CompleteExport(ctx context.Context, exportID int64, outputURL string, fileSize int64) error
	// GetExportDetail 导出详情。
	GetExportDetail(ctx context.Context, exportID int64) (*ExportResult, error)
}

// ExportResult 导出详情。
type ExportResult struct {
	ExportID   int64
	ExportUUID string
	Status     string
	OutputURL  string
	FileSize   int64
	ExpireAt   time.Time
}

// ExportHandler 导出 Worker（FINAL_VIDEO_COMPOSE）。
type ExportHandler struct {
	store ExportStore
}

func NewExportHandler(store ExportStore) *ExportHandler {
	return &ExportHandler{store: store}
}

func (h *ExportHandler) Name() string { return "FINAL_VIDEO_COMPOSE" }

func (h *ExportHandler) Handles(taskType string) bool {
	return taskType == "FINAL_VIDEO_COMPOSE" || taskType == "ENGINEER_FILE_EXPORT"
}

// Execute mock 视频合成：生成成片 URL → 回写 export_records。
func (h *ExportHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ExportID   int64  `json:"export_id"`
		Resolution string `json:"resolution"`
		Watermark  bool   `json:"watermark"`
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil || params.ExportID == 0 {
		return nil, fmt.Errorf("导出任务参数无效")
	}

	// mock 视频合成（模拟 FFmpeg concat + 字幕烧录 + 水印注入）
	log.Printf("export: 开始合成 export_id=%d res=%s watermark=%v", params.ExportID, params.Resolution, params.Watermark)
	time.Sleep(500 * time.Millisecond) // mock 处理延迟

	// mock 成片 URL（模拟上传 OSS 结果）
	outputURL := fmt.Sprintf("https://mock.cdn.example.com/exports/%d-%s.mp4",
		params.ExportID, time.Now().Format("20060102150405"))
	fileSize := int64(10 * 1024 * 1024) // mock 10MB

	// 写入 export_records
	if err := h.store.CompleteExport(ctx, params.ExportID, outputURL, fileSize); err != nil {
		return nil, err
	}
	log.Printf("export: 完成 export_id=%d url=%s size=%d", params.ExportID, outputURL, fileSize)

	out, _ := json.Marshal(map[string]any{
		"export_id":  params.ExportID,
		"output_url": outputURL,
		"file_size":  fileSize,
		"is_mock":    true,
	})
	return out, nil
}