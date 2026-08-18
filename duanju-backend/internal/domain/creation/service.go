// creation 服务：项目 CRUD + 拆分触发（状态机）+ 层级读（03 附录 A 对齐）。
package creation

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// Service creation 服务。
type Service struct {
	repo    *Repo
	lg      *zap.Logger
	billing *billing.Engine // 计费引擎（任务冻结/结算；nil 时跳过计费）
}

func NewService(repo *Repo, lg *zap.Logger, b *billing.Engine) *Service {
	return &Service{repo: repo, lg: lg, billing: b}
}

// Create 创建项目（3.0.2）：默认空间缺省 + 自动第 1 集。
func (s *Service) Create(c *gin.Context, req *CreateProjectReq) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	spaceID := int64(0)
	spaceUUID := req.SpaceID
	if spaceUUID == "" {
		var err error
		spaceID, spaceUUID, err = s.repo.DefaultSpaceID(c.Request.Context(), uid)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "未找到默认空间")
			return
		}
	} else {
		var err error
		spaceID, err = s.repo.ResolveSpace(c.Request.Context(), spaceUUID, uid)
		if err != nil {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "空间不存在或无权限")
			return
		}
	}
	p, err := s.repo.CreateProject(c.Request.Context(), spaceID, uid, spaceUUID, req.Title, req.SourceScriptText)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建项目失败")
		return
	}
	httpx.OK(c, toProjectDTO(p))
}

// List 项目列表（3.0.1）。
func (s *Service) List(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	spaceUUID := c.Query("space_id")
	spaceID := int64(0)
	if spaceUUID != "" {
		id, err := s.repo.ResolveSpace(c.Request.Context(), spaceUUID, uid)
		if err != nil {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "空间不存在或无权限")
			return
		}
		spaceID = id
	} else {
		id, _, err := s.repo.DefaultSpaceID(c.Request.Context(), uid)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "未找到默认空间")
			return
		}
		spaceID = id
	}
	page := parseInt(c.Query("page"), 1)
	pageSize := parseInt(c.Query("page_size"), 20)
	rows, total, err := s.repo.ListProjects(c.Request.Context(), spaceID, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取项目列表失败")
		return
	}
	items := make([]ProjectDTO, 0, len(rows))
	for _, r := range rows {
		items = append(items, toProjectDTO(&r))
	}
	httpx.OK(c, gin.H{
		"items": items,
		"pagination": gin.H{
			"page": page, "page_size": pageSize, "total": total,
			"total_pages": (total + pageSize - 1) / pageSize,
		},
	})
}

// Get 项目详情（3.0.3）。
func (s *Service) Get(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	httpx.OK(c, toProjectDTO(p))
}

// Update 更新项目（3.0.4）。
func (s *Service) Update(c *gin.Context, projectUUID string, req *UpdateProjectReq) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if err := s.repo.UpdateProject(c.Request.Context(), p.ID, req.Title, req.CoverImageURL, req.SourceScriptText); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新项目失败")
		return
	}
	httpx.OK(c, nil)
}

// Delete 软删除（3.0.5）。
func (s *Service) Delete(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if err := s.repo.SoftDelete(c.Request.Context(), p.ID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除项目失败")
		return
	}
	httpx.OK(c, nil)
}

// Restore 恢复（3.0.6）：从回收站恢复软删项目。
func (s *Service) Restore(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProjectIncludingDeleted(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if err := s.repo.Restore(c.Request.Context(), p.ID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "恢复项目失败")
		return
	}
	httpx.OK(c, nil)
}

// TouchLastOpened 更新最后打开（3.0.7）。
func (s *Service) TouchLastOpened(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	_ = s.repo.TouchLastOpened(c.Request.Context(), p.ID)
	httpx.OK(c, nil)
}

// SplitScript 触发拆分（3.1.1）：DRAFT/FAILED → SCRIPT_SPLITTING + 建拆分任务（状态机校验）。
func (s *Service) SplitScript(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if p.Status != StatusDraft && p.Status != StatusScriptSplitFailed {
		httpx.Fail(c, http.StatusConflict, 30001, "剧本拆分正在进行中，请勿重复触发")
		return
	}
	// 状态机原子转移
	if _, err := s.repo.TransitionStatus(c.Request.Context(), p.ID, StatusScriptSplitting); err != nil {
		httpx.Fail(c, http.StatusConflict, 30001, "剧本拆分状态不允许此操作")
		return
	}
	// 建拆分任务（S1-B04 调度消费）：剧本来源 = 请求体 script_text 优先，兜底项目 source_script_text
	var req struct {
		ScriptText string `json:"script_text"`
	}
	_ = c.ShouldBindJSON(&req)
	script := strings.TrimSpace(req.ScriptText)
	if script == "" {
		script = strings.TrimSpace(p.SourceScriptText)
	}
	if script == "" {
		// 无剧本：回滚状态机并提示（03 A.2：失败回 DRAFT 可重试）
		_, _ = s.repo.TransitionStatus(c.Request.Context(), p.ID, StatusDraft)
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "请先输入剧本原文")
		return
	}
	if _, err := s.repo.CreateSplitTask(c.Request.Context(), p.ID, p.SpaceID, uid, script); err != nil {
		s.lg.Error("创建拆分任务失败", zap.String("request_id", c.GetString("request_id")), zap.Int64("project_id", p.ID), zap.Error(err))
		// 任务创建失败：回滚状态机（03 A.2：失败回 DRAFT 可重试）
		_, _ = s.repo.TransitionStatus(c.Request.Context(), p.ID, StatusDraft)
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建拆分任务失败")
		return
	}
	httpx.OK(c, gin.H{"project_id": p.ProjectUUID, "status": StatusScriptSplitting})
}

// 计费定价（对齐 LuxReal 免费/收费分界：拆分免费、资产图扣点、视频按秒计费、导出按时长计费）。
const (
	VideoPointsPerSec  = 10 // 视频生成 P720：10 点/秒（Seedance 成本 ~5 点/秒，定价留平台利润）
	ExportPointsPerSec = 1  // 导出合成：1 点/秒（FFmpeg 成本低）
)

// GenerateVideos 3.3.7a 批量生成 beat 下全部镜头视频（VIDEO_SEGMENT_GEN + Outbox + 按秒计费冻结）。
func (s *Service) GenerateVideos(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	projectID, err := s.repo.ResolveProjectByBeat(c.Request.Context(), beatUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	beatID, err := s.repo.ResolveBeatID(c.Request.Context(), beatUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	shots, err := s.repo.ListShotsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取镜头失败")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskIDs, err := s.repo.CreateVideoTasks(c.Request.Context(), projectID, spaceID, uid, beatID, shots)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建视频任务失败")
		return
	}
	// 计费冻结：按镜头时长 × 10 点/秒（未成功镜头；不足则任务标记失败并回退 shot 状态）
	totalCost, frozenCount := 0, 0
	if s.billing != nil {
		for i, tID := range taskIDs {
			dur := 0
			// 找到对应 shot 的时长（按顺序过滤 SUCCESS 后对齐）
			idx := 0
			for _, sh := range shots {
				if sh.VideoStatus == "SUCCESS" {
					continue
				}
				if idx == i {
					dur = sh.DurationSec
					break
				}
				idx++
			}
			cost := dur * VideoPointsPerSec
			if cost <= 0 {
				continue
			}
			if err := s.billing.Freeze(c.Request.Context(), "USER", uid, tID, cost, &billing.TxnMeta{
				SpaceID: spaceID, OperatorUserID: uid, ProjectID: &projectID, Description: "视频生成",
			}); err != nil {
				// 余额不足等：任务标记失败，shot 状态回退
				_ = s.repo.MarkTaskFailed(c.Request.Context(), tID, "积分不足或冻结失败："+err.Error())
				continue
			}
			totalCost += cost
			frozenCount++
		}
		if frozenCount == 0 && len(taskIDs) > 0 {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "积分不足，无法生成视频")
			return
		}
	}
	httpx.OK(c, gin.H{"created": len(taskIDs), "total": len(shots), "frozen_count": frozenCount, "total_cost": totalCost})
}

// ListBeatShots 单镜头视频列表（Step3 轮询）。
func (s *Service) ListBeatShots(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	projectID, err := s.repo.ResolveProjectByBeat(c.Request.Context(), beatUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	beatID, err := s.repo.ResolveBeatID(c.Request.Context(), beatUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	shots, err := s.repo.ListShotsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取镜头失败")
		return
	}
	httpx.OK(c, gin.H{"items": shots})
}

// ListEpisodes 剧集列表（3.1.2）。
func (s *Service) ListEpisodes(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	eps, err := s.repo.ListEpisodes(c.Request.Context(), p.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取剧集列表失败")
		return
	}
	httpx.OK(c, gin.H{"items": eps})
}

// ListBeats 叙事单元列表（3.1.4，经 episode 校验项目权限）。
func (s *Service) ListBeats(c *gin.Context, episodeUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	epID, projectID, err := s.repo.ResolveEpisode(c.Request.Context(), episodeUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在")
		return
	}
	// 项目权限校验（空间成员）
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	beats, err := s.repo.ListBeats(c.Request.Context(), epID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取叙事单元失败")
		return
	}
	httpx.OK(c, gin.H{"items": beats})
}

func toProjectDTO(p *ProjectRow) ProjectDTO {
	cover := ""
	if p.CoverImageURL != nil {
		cover = *p.CoverImageURL
	}
	return ProjectDTO{
		ProjectID:      p.ProjectUUID,
		SpaceID:        p.SpaceUUID,
		Title:          p.Title,
		CoverImageURL:  cover,
		ProjectType:    p.ProjectType,
		CurrentStep:    p.CurrentStep,
		Status:         p.Status,
		TotalEpisodeCount: p.TotalEpisodeCount,
		TotalDurationSec:  p.TotalDurationSec,
		LastOpenedAt:   p.LastOpenedAt,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func userID(c *gin.Context) int64 {
	return c.GetInt64(middleware.CtxUserID)
}

func parseInt(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	if n == 0 {
		return def
	}
	return n
}
