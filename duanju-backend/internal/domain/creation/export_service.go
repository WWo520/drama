// 导出服务：3.4.1 触发导出 / 3.4.2 详情 / 3.4.3 历史列表。
package creation

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/engine/asset"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// ExportTaskQueue 导出任务创建边界（由 repository.ExportStore 实现）。
type ExportTaskQueue interface {
	// CreateExportTask 创建导出任务（返回 exportID 与 taskID，计费冻结按 taskID 进行）。
	CreateExportTask(ctx context.Context, userID, spaceID, projectID int64, exportID int64, resolution string, watermark bool) (int64, int64, error)
	// ListExports 导出历史（3.4.3）。
	ListExports(ctx context.Context, projectID int64, page, pageSize int) ([]map[string]any, int, error)
	// ListExportsByUser 我的全部导出（作品列表「视频成片」Tab）。
	ListExportsByUser(ctx context.Context, userID int64, page, pageSize int) ([]map[string]any, int, error)
	// GetExportByUUID 导出详情（3.4.2；归属校验）。
	GetExportByUUID(ctx context.Context, exportUUID string, userID int64) (map[string]any, error)
}

// ExportService 导出相关服务。
type ExportService struct {
	repo      *Repo
	asset     *asset.Client
	exportOps ExportTaskQueue
	billing   *billing.Engine // 计费引擎（导出冻结；nil 跳过）
}

func NewExportService(repo *Repo, ac *asset.Client, eq ExportTaskQueue, b *billing.Engine) *ExportService {
	return &ExportService{repo: repo, asset: ac, exportOps: eq, billing: b}
}

// Trigger 触发导出（3.4.1）：创建导出记录 + 导出任务 + Outbox。
func (s *ExportService) Trigger(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	proj, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	// 检查状态：仅 ALL_VIDEOS_READY 或 PARTIAL_VIDEOS 可导出（03 附录 A.2 UNITS_DONE）
	if proj.Status != "ALL_VIDEOS_READY" && proj.Status != "PARTIAL_VIDEOS" && proj.Status != "COMPLETED" {
		httpx.Fail(c, http.StatusBadRequest, 70001, "项目未完成，不能导出成片")
		return
	}

	// 解析请求体（3.4.1：format/resolution/watermark；缺省 P720 + 合规水印）
	var req struct {
		Format     string `json:"format"`
		Resolution string `json:"resolution"`
		Watermark  *bool  `json:"watermark"`
	}
	_ = c.ShouldBindJSON(&req)
	resolution := req.Resolution
	if resolution == "" {
		resolution = "P720"
	}
	watermark := true
	if req.Watermark != nil {
		watermark = *req.Watermark
	}

	spaceID := proj.SpaceID
	// 创建导出记录 + 任务（返回 taskID 用于计费冻结）
	exportID, taskID, err := s.exportOps.CreateExportTask(c.Request.Context(), uid, spaceID, proj.ID, proj.ID, resolution, watermark)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建导出任务失败")
		return
	}
	// 计费冻结：成片时长 × 1 点/秒（LuxReal 分界：导出按时长计费）
	if s.billing != nil && proj.TotalDurationSec > 0 {
		cost := proj.TotalDurationSec * ExportPointsPerSec
		if ferr := s.billing.Freeze(c.Request.Context(), "USER", uid, taskID, cost, &billing.TxnMeta{
			SpaceID: spaceID, OperatorUserID: uid, ProjectID: &proj.ID, Description: "成片导出",
		}); ferr != nil {
			_ = s.repo.MarkTaskFailed(c.Request.Context(), taskID, "积分不足或冻结失败："+ferr.Error())
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "积分不足，无法导出")
			return
		}
	}
	httpx.OK(c, gin.H{"export_id": exportID, "status": "PENDING"})
}

// List 导出历史（3.4.3）。
func (s *ExportService) List(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 50 || pageSize < 1 {
		pageSize = 20
	}
	// 无 project_id：返回我的全部导出（作品列表「视频成片」Tab）
	projectUUID := c.Query("project_id")
	if projectUUID == "" {
		items, total, err := s.exportOps.ListExportsByUser(c.Request.Context(), uid, page, pageSize)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取导出记录失败")
			return
		}
		httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
		return
	}
	proj, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	// 导出历史（3.4.3）
	items, total, err := s.exportOps.ListExports(c.Request.Context(), proj.ID, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取导出历史失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// Detail 导出详情（3.4.2）。
func (s *ExportService) Detail(c *gin.Context, exportUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	item, err := s.exportOps.GetExportByUUID(c.Request.Context(), exportUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "导出记录不存在")
		return
	}
	httpx.OK(c, item)
}