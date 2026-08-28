// 导出服务：3.4.1 触发导出 / 3.4.2 详情 / 3.4.3 历史列表。
package creation

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/engine/asset"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// ExportTaskQueue 导出任务创建边界（由 repository.ExportStore 实现）。
type ExportTaskQueue interface {
	// CreateExportTask 创建导出任务（返回 exportID 与 taskID，计费冻结按 taskID 进行；
	// episodeID>0 时把目标集写入任务参数与 export_range_json，供 Worker 按集合成）。
	CreateExportTask(ctx context.Context, userID, spaceID, projectID int64, exportID int64, resolution string, watermark bool, episodeID int64) (int64, int64, error)
	// ListExports 导出历史（3.4.3）。
	ListExports(ctx context.Context, projectID int64, page, pageSize int) ([]map[string]any, int, error)
	// ListExportsByUser 我的全部导出（作品列表「视频成片」Tab）。
	ListExportsByUser(ctx context.Context, userID int64, page, pageSize int) ([]map[string]any, int, error)
	// GetExportByUUID 导出详情（3.4.2；归属校验）。
	GetExportByUUID(ctx context.Context, exportUUID string, userID int64) (map[string]any, error)
	// ResetExportForRetryByUUID 重试重置：清空旧输出 + 状态回 PENDING，返回 (exportID, taskID)。
	ResetExportForRetryByUUID(ctx context.Context, exportUUID string, userID int64) (int64, int64, error)
	// ReenqueueComposeTask 重建合成任务 + Outbox。
	ReenqueueComposeTask(ctx context.Context, exportID, origTaskID int64) error
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

	// 解析请求体（3.4.1：format/resolution/watermark/episode_id；缺省 P720 + 合规水印）
	var req struct {
		Format      string `json:"format"`
		Resolution  string `json:"resolution"`
		Watermark   *bool  `json:"watermark"`
		EpisodeUUID string `json:"episode_id"` // 当前集 UUID；空 = 兼容旧行为（首集）
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

	// 目标集解析：传了 episode_id → 校验归属本项目；未传 → 首集（历史兼容）
	episodeID := int64(0)
	if epUUID := strings.TrimSpace(req.EpisodeUUID); epUUID != "" {
		epID, projID, rerr := s.repo.ResolveEpisode(c.Request.Context(), epUUID)
		if rerr != nil || projID != proj.ID {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在或不属于当前项目")
			return
		}
		episodeID = epID
	} else if fid, ferr := s.repo.FirstEpisodeOf(c.Request.Context(), proj.ID); ferr == nil {
		episodeID = fid
	}

	spaceID := proj.SpaceID
	// 创建导出记录 + 任务（返回 taskID 用于计费冻结）
	exportID, taskID, err := s.exportOps.CreateExportTask(c.Request.Context(), uid, spaceID, proj.ID, proj.ID, resolution, watermark, episodeID)
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

// Preflight 合成前预检：去重后片段统计（弹窗展示"将合成 N 个片段，M 个重复将被跳过"）。
// 支持 ?episode_id=<uuid>：缺省 = 首集（历史兼容）。
func (s *ExportService) Preflight(c *gin.Context, projectUUID string) {
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
	episodeID := int64(0)
	if epUUID := strings.TrimSpace(c.Query("episode_id")); epUUID != "" {
		epID, projID, rerr := s.repo.ResolveEpisode(c.Request.Context(), epUUID)
		if rerr != nil || projID != proj.ID {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在或不属于当前项目")
			return
		}
		episodeID = epID
	} else if fid, ferr := s.repo.FirstEpisodeOf(c.Request.Context(), proj.ID); ferr == nil {
		episodeID = fid
	}
	pf, err := s.repo.PreflightExportCompose(c.Request.Context(), episodeID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "预检失败")
		return
	}
	httpx.OK(c, pf)
}

// Retry 重新合成：签名 URL 过期（403）或 FAILED 记录一键重发起。
// 清空旧输出 → 状态回 PENDING → 重建合成任务（不重复冻结积分：复用原 export 记录）。
func (s *ExportService) Retry(c *gin.Context, exportUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	exportID, taskID, err := s.exportOps.ResetExportForRetryByUUID(c.Request.Context(), exportUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置导出失败")
		return
	}
	if exportID == 0 {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "导出记录不存在或不可重试")
		return
	}
	// 重建合成任务 + Outbox（复用原 export 记录的 resolution/watermark 由仓储端保留）
	if terr := s.exportOps.ReenqueueComposeTask(c.Request.Context(), exportID, taskID); terr != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重建合成任务失败")
		return
	}
	httpx.OK(c, gin.H{"export_id": exportUUID, "status": "PENDING"})
}
