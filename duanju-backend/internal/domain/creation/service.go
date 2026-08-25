// creation 服务：项目 CRUD + 拆分触发（状态机）+ 层级读（03 附录 A 对齐）。
package creation

import (
	"fmt"
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

// SplitScript 触发拆分（3.1.1）：DRAFT/FAILED 首次拆分；PARTIAL_ASSETS/ALL_ASSETS_READY 需 force=true 重新拆分（覆盖旧产物）。
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
	var req struct {
		ScriptText         string `json:"script_text"`
		Force              bool   `json:"force"`                // 重新拆分标记（已有拆分结果时必须显式确认）
		EpisodeDurationMin int    `json:"episode_duration_min"` // 单集目标时长（分钟；0=后端默认）
		SplitSkillCode     string `json:"split_skill_code"`     // 剧本拆分技能（agent_skills.skill_code；空=默认标准拆分）
	}
	_ = c.ShouldBindJSON(&req)
	// 拆分技能：状态转移前校验（无效 code 直接拒绝，避免状态回滚）
	skillPrompt := ""
	if code := strings.TrimSpace(req.SplitSkillCode); code != "" {
		tpl, err := s.repo.GetSkillTemplate(c.Request.Context(), code)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "拆分技能不存在或已停用")
			return
		}
		skillPrompt = tpl
	}
	switch p.Status {
	case StatusDraft, StatusScriptSplitFailed:
		// 首次拆分/失败重试：直接放行
	case StatusPartialAssets, StatusAllAssetsReady:
		// 重新拆分：需 force 显式确认（前端二次弹窗），否则拒绝避免误操作
		if !req.Force {
			httpx.Fail(c, http.StatusConflict, 30001, "项目已有拆分结果，重新拆分需确认覆盖")
			return
		}
	case StatusScriptSplitting:
		httpx.Fail(c, http.StatusConflict, 30001, "剧本拆分正在进行中，请勿重复触发")
		return
	default:
		httpx.Fail(c, http.StatusConflict, 30001, "当前项目阶段不允许重新拆分（请回到资产阶段前操作）")
		return
	}
	// 状态机原子转移
	if _, err := s.repo.TransitionStatus(c.Request.Context(), p.ID, StatusScriptSplitting); err != nil {
		httpx.Fail(c, http.StatusConflict, 30001, "剧本拆分状态不允许此操作")
		return
	}
	// 建拆分任务（S1-B04 调度消费）：剧本来源 = 请求体 script_text 优先，兜底项目 source_script_text
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
	if _, err := s.repo.CreateSplitTask(c.Request.Context(), p.ID, p.SpaceID, uid, script, req.EpisodeDurationMin, skillPrompt); err != nil {
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
// 18 - 视频制作：弹窗参数（分辨率/模型）落任务参数；关联资产未就绪门控（对齐 LuxReal code=6002）。
func (s *Service) GenerateVideos(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Resolution string `json:"resolution"`  // P480/P720/P1080
		VideoModel string `json:"video_model"` // 模型展示名（Seedance 2.5/2.0/1.5）
		ModelKey   string `json:"model_key"`   // 模型 key（命中 models.model_key 才参与路由）
	}
	_ = c.ShouldBindJSON(&req)
	switch strings.TrimSpace(req.Resolution) {
	case "P480", "P720", "P1080":
	default:
		req.Resolution = "" // 非法值回落默认 P720
	}
	opts := VideoGenOpts{
		Resolution: strings.TrimSpace(req.Resolution),
		VideoModel: strings.TrimSpace(req.VideoModel),
		ModelKey:   s.repo.ResolveVideoModelKey(c.Request.Context(), strings.TrimSpace(req.ModelKey)),
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
	// 资产未就绪门控（阶段一.3）：命中关联但参考图缺失 → 拒绝并引导回 Step2；
	// 无关联资产（纯 T2V 单元）或全部就绪才放行。
	if linked, ready, gerr := s.repo.BeatAssetReadiness(c.Request.Context(), beatID); gerr == nil && linked > 0 && ready < linked {
		httpx.Fail(c, http.StatusBadRequest, 6002,
			fmt.Sprintf("本单元有 %d 个关联资产未生成参考图，请先回到资产阶段完成生成后再制作视频", linked-ready))
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
	taskIDs, err := s.repo.CreateVideoTasks(c.Request.Context(), projectID, spaceID, uid, beatID, shots, opts)
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

// RegenerateShot 单镜头重生成（F11：POST /shots/:shot_id/regenerate）。
// 重置 shot 状态为 PENDING → 创建新任务 → 计费冻结 → 入队。
func (s *Service) RegenerateShot(c *gin.Context, shotUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	// 解析 shot_uuid → shot_id
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), shotUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	// 查询 shot 所属项目并校验权限
	var projectID, beatID int64
	var durationSec int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT sh.project_id, sh.narrative_unit_id, sh.duration_sec
		FROM shots sh WHERE sh.id=$1 AND sh.deleted_at IS NULL`, shotID).
		Scan(&projectID, &beatID, &durationSec); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	// 重置 shot 状态为 PENDING（允许重新生成）
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE shots SET video_generation_status='PENDING', generated_video_url=NULL, updated_at=NOW()
		WHERE id=$1`, shotID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置镜头状态失败")
		return
	}
	// 查询空间并创建任务
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	// 创建 VIDEO_SEGMENT_GEN 任务（单镜头；参数走默认 P720 + 默认模型路由）
	taskIDs, err := s.repo.CreateVideoTasks(c.Request.Context(), projectID, spaceID, uid, beatID, []ShotVideoDTO{{ShotID: shotID, DurationSec: durationSec, VideoStatus: "PENDING"}}, VideoGenOpts{})
	if err != nil || len(taskIDs) == 0 {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建视频任务失败")
		return
	}
	// 计费冻结：按镜头时长 × 10 点/秒
	totalCost := durationSec * 10
	frozenCount := 0
	if s.billing != nil {
		if err := s.billing.Freeze(c.Request.Context(), "USER", uid, taskIDs[0], totalCost, &billing.TxnMeta{
			SpaceID:        spaceID,
			OperatorUserID: uid,
			ProjectID:      &projectID,
			Description:    fmt.Sprintf("单镜头重生成 shot_id=%d", shotID),
		}); err != nil {
			// 冻结失败：任务标记失败
			_ = s.repo.MarkTaskFailed(c.Request.Context(), taskIDs[0], "积分冻结失败")
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "积分不足，无法重新生成")
			return
		}
		frozenCount = 1
	}
	httpx.OK(c, gin.H{"task_id": taskIDs[0], "shot_id": shotID, "frozen_count": frozenCount, "total_cost": totalCost})
}

// UpdateNarrativeUnit 编辑叙事单元（F4：PATCH /narrative-units/:nu_id）。
// 编辑前自动保存版本快照（S2-B05），编辑后 version_num+1。
func (s *Service) UpdateNarrativeUnit(c *gin.Context, nuUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Title              string `json:"title"`
		Content            string `json:"content"`
		ScriptOriginalText string `json:"script_original_text"`
		PresetDurationSec  *int   `json:"preset_duration_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	// 解析 nu_uuid → (id, project_id) 并校验权限
	nuID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), nuUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	// 读取当前内容（用于版本快照）
	var curTitle, curContent, curScript string
	var curDur *int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(title,''), COALESCE(content,''), COALESCE(script_original_text,''), preset_duration_sec
		FROM narrative_units WHERE id=$1 AND deleted_at IS NULL`, nuID).
		Scan(&curTitle, &curContent, &curScript, &curDur); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	// 保存编辑前版本快照（回滚用）
	_ = s.repo.SaveVersionSnapshot(c.Request.Context(), nuID, uid, curScript, curContent)
	// 更新字段（仅更新非空字段）
	if req.Title != "" && req.Title != curTitle {
		curTitle = req.Title
	}
	if req.Content != "" && req.Content != curContent {
		curContent = req.Content
	}
	if req.ScriptOriginalText != "" && req.ScriptOriginalText != curScript {
		curScript = req.ScriptOriginalText
	}
	if req.PresetDurationSec != nil {
		curDur = req.PresetDurationSec
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE narrative_units SET title=$2, content=$3, script_original_text=$4,
		       preset_duration_sec=$5, version_num=version_num+1, updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`,
		nuID, curTitle, curContent, curScript, curDur); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新叙事单元失败")
		return
	}
	httpx.OK(c, gin.H{"nu_id": nuUUID, "title": curTitle, "version_num": 0})
}

// DeleteNarrativeUnit 删除叙事单元（F4：DELETE /narrative-units/:nu_id）。
// 软删除：级联标记 shot_groups/shots 删除（保留数据可回滚）。
func (s *Service) DeleteNarrativeUnit(c *gin.Context, nuUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	nuID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), nuUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	// 软删除 shots（先删除子表，避免外键冲突）
	if _, err := tx.Exec(c.Request.Context(), `UPDATE shots SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, nuID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除镜头失败")
		return
	}
	// 软删除 shot_groups
	if _, err := tx.Exec(c.Request.Context(), `UPDATE shot_groups SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, nuID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除镜头块失败")
		return
	}
	// 软删除叙事单元
	if _, err := tx.Exec(c.Request.Context(), `UPDATE narrative_units SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, nuID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除叙事单元失败")
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除失败")
		return
	}
	httpx.OK(c, nil)
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

// ListSkills 用户端技能列表（kind 过滤；当前仅开放 SCRIPT_SPLIT 供 Step1 拆分弹窗选择）。
func (s *Service) ListSkills(c *gin.Context) {
	kind := c.DefaultQuery("kind", "SCRIPT_SPLIT")
	if kind != "SCRIPT_SPLIT" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "不支持的技能类型")
		return
	}
	items, err := s.repo.ListSkills(c.Request.Context(), kind)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取技能列表失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func toProjectDTO(p *ProjectRow) ProjectDTO {
	cover := ""
	if p.CoverImageURL != nil {
		cover = *p.CoverImageURL
	}
	return ProjectDTO{
		ProjectID:         p.ProjectUUID,
		SpaceID:           p.SpaceUUID,
		Title:             p.Title,
		CoverImageURL:     cover,
		SourceScriptText:  p.SourceScriptText,
		ProjectType:       p.ProjectType,
		CurrentStep:       p.CurrentStep,
		Status:            p.Status,
		TotalEpisodeCount: p.TotalEpisodeCount,
		TotalDurationSec:  p.TotalDurationSec,
		LastOpenedAt:      p.LastOpenedAt,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
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
