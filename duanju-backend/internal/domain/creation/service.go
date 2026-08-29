// creation 服务：项目 CRUD + 拆分触发（状态机）+ 层级读（03 附录 A 对齐）。
package creation

import (
	"fmt"
	"net/http"
	"strconv"
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
	p, err := s.repo.CreateProject(c.Request.Context(), spaceID, uid, spaceUUID, req.Title, req.SourceScriptText, req.Config)
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

// UpdateConfig 合并更新项目配置快照（PATCH /projects/:id/config）。
func (s *Service) UpdateConfig(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil || len(patch) == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if err := s.repo.UpsertProjectConfig(c.Request.Context(), p.ID, patch); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新项目配置失败")
		return
	}
	httpx.OK(c, gin.H{"updated": true})
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
	// 任务级互斥兜底：项目状态可能与任务状态脱节（手工修数/回滚竞态），
	// 直接查任务表防止同项目拆分任务叠加引发 LLM 额度挤兑
	if active, err := s.repo.HasActiveSplitTask(c.Request.Context(), p.ID); err == nil && active {
		httpx.Fail(c, http.StatusConflict, 30001, "该剧本已有拆分任务进行中，请等待完成后再试")
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

// GenerateVideos 3.3.7a 批量生成单元下全部成片卡视频（VIDEO_SEGMENT_GEN + Outbox + 按秒计费冻结）。
// 成片卡主线（对齐 LuxReal「规划视频任务」）：无卡片时拒绝（引导先规划）；redo 重置后全量重建。
func (s *Service) GenerateVideos(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Resolution string `json:"resolution"`  // P480/P720/P1080；MiniMax H3 为 768P/2K
		VideoModel string `json:"video_model"` // 模型展示名（Seedance 2.5/2.0/1.5、MiniMax H3）
		ModelKey   string `json:"model_key"`   // 模型 key（命中 models.model_key 才参与路由）
		Redo       bool   `json:"redo"`        // 重新生成：重置 SUCCESS 卡片后全量重建
	}
	_ = c.ShouldBindJSON(&req)
	switch strings.TrimSpace(req.Resolution) {
	case "P480", "P720", "P1080", "768P", "2K":
	default:
		req.Resolution = "" // 非法值回落默认 P720（H3 Provider 内归一化为 768P）
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
	cards, err := s.repo.ListCardsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取成片卡失败")
		return
	}
	if len(cards) == 0 {
		httpx.Fail(c, http.StatusBadRequest, 6004, "尚未规划视频任务，请先点击「规划视频任务」将分镜镜头编组成片卡")
		return
	}
	// 资产未就绪门控（阶段一.3）：命中关联但参考图缺失 → 拒绝并引导回 Step2。
	if linked, ready, gerr := s.repo.BeatAssetReadiness(c.Request.Context(), beatID); gerr == nil && linked > 0 && ready < linked {
		httpx.Fail(c, http.StatusBadRequest, 6002,
			fmt.Sprintf("本单元有 %d 个关联资产未生成参考图，请先回到资产阶段完成生成后再制作视频", linked-ready))
		return
	}
	// 重新生成模式：重置已 SUCCESS 卡片与卡内镜头（清旧视频）后全量重建；
	// 非重做模式下存在执行中任务则拒绝重复提交。
	if req.Redo {
		if rerr := s.repo.ResetBeatCardVideos(c.Request.Context(), beatID); rerr != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置视频任务状态失败")
			return
		}
		for i := range cards {
			cards[i].Status = "PENDING"
			if cards[i].GeneratedVideoURL != "" {
				cards[i].GeneratedVideoURL = ""
			}
		}
	} else {
		for _, cd := range cards {
			if cd.Status == "QUEUED" || cd.Status == "RUNNING" {
				httpx.Fail(c, http.StatusConflict, 30003, "本单元已有视频任务在执行中，请稍候")
				return
			}
		}
	}
	eligible := make([]VideoCardDTO, 0, len(cards))
	for _, cd := range cards {
		if cd.Status != "SUCCESS" {
			eligible = append(eligible, cd)
		}
	}
	if len(eligible) == 0 {
		httpx.OK(c, gin.H{"created": 0, "total": len(cards), "frozen_count": 0, "total_cost": 0})
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskIDs, err := s.repo.CreateCardVideoTasks(c.Request.Context(), projectID, spaceID, uid, beatID, eligible, opts)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建视频任务失败")
		return
	}
	// 计费冻结：按卡片时长 × 10 点/秒（不足则任务标记失败并回退卡片状态）
	totalCost, frozenCount := 0, 0
	if s.billing != nil {
		for i, tID := range taskIDs {
			cost := eligible[i].DurationSec * VideoPointsPerSec
			if cost <= 0 {
				continue
			}
			if err := s.billing.Freeze(c.Request.Context(), "USER", uid, tID, cost, &billing.TxnMeta{
				SpaceID: spaceID, OperatorUserID: uid, ProjectID: &projectID,
				Description: fmt.Sprintf("成片卡视频生成 %s", eligible[i].Title),
			}); err != nil {
				_ = s.repo.MarkTaskFailed(c.Request.Context(), tID, "积分不足或冻结失败："+err.Error())
				_, _ = s.repo.pool.Exec(c.Request.Context(),
					`UPDATE video_cards SET generation_status='FAILED_DEAD' WHERE id=$1`, eligible[i].CardID)
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
	httpx.OK(c, gin.H{"created": len(taskIDs), "total": len(cards), "frozen_count": frozenCount, "total_cost": totalCost})
}

// PlanVideoTasks 规划视频任务（对齐 LuxReal Tab3「视频任务规划」）：把单元下连续分镜镜头
// 确定性编组成成片卡（总时长 4~10 秒、2~4 镜头；尾卡不足 4s 并入前一卡）。
// 已有卡片时需 force=true 显式确认（旧卡及其生成的视频一并废弃）。
func (s *Service) PlanVideoTasks(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)
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
	existing, err := s.repo.ListCardsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询已有成片卡失败")
		return
	}
	for _, cd := range existing {
		if cd.Status == "QUEUED" || cd.Status == "RUNNING" {
			httpx.Fail(c, http.StatusConflict, 30003, "本单元有视频任务正在执行，无法重新规划")
			return
		}
	}
	if len(existing) > 0 && !req.Force {
		httpx.Fail(c, http.StatusConflict, 30002,
			fmt.Sprintf("本单元已规划 %d 个视频任务卡，重新规划将清空其已生成的视频，请确认后重试", len(existing)))
		return
	}
	shots, err := s.repo.ListShotsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取镜头失败")
		return
	}
	if len(shots) == 0 {
		httpx.Fail(c, http.StatusBadRequest, 6005, "尚未生成分镜脚本，请先生成分镜脚本再规划视频任务")
		return
	}
	globalOrder, shotsByID, groups := packVideoCards(shots)
	n, err := s.repo.InsertVideoCards(c.Request.Context(), beatID, globalOrder, groups, shotsByID)
	if err != nil {
		s.lg.Error("插入成片卡失败", zap.Error(err), zap.Int64("beat_id", beatID))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "规划视频任务失败")
		return
	}
	// 各卡首镜自动补镜头参考图（首帧锚定，对齐 LuxReal 智能镜头：先有画面基准再生成视频）。
	// 失败仅记日志不阻断规划——锚图缺失只影响衔接质量，视频链路本身可正常降级运行。
	firstShotIDs := make([]int64, 0, len(groups))
	for _, g := range groups {
		if len(g) > 0 {
			firstShotIDs = append(firstShotIDs, g[0])
		}
	}
	spaceID, serr := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if serr == nil {
		if _, terr := s.repo.CreateShotRefImageTasks(c.Request.Context(), projectID, spaceID, uid, firstShotIDs); terr != nil {
			s.lg.Warn("批量创建首镜参考图任务失败（不影响规划）", zap.Error(terr), zap.Int64("beat_id", beatID))
		}
	} else {
		s.lg.Warn("查询空间失败，跳过首镜参考图补齐", zap.Error(serr), zap.Int64("beat_id", beatID))
	}
	httpx.OK(c, gin.H{"created": n, "total_cards": n, "total_shots": len(shots)})
}

// packVideoCards 语义化编组（对齐 LuxReal「视频任务规划技能」：哪些镜头保持连续由剧情决定）：
// 优先沿 shot_group 边界切卡——组是拆分阶段 LLM 按剧情划定的连贯叙事动作块（同组镜头
// 属同一时空/同一动作线），整组绑一卡不拆散，镜头数随剧情浮动（1~N 镜）；
// 仅当单组总时长超单卡目标上限 10s 时才在组内按时长贪心二段切分。
// 尾卡不足 4s 并入前一卡（MiniMax H3 最短 4s；合并后 ≤15s 由 clampCardDuration 收口）。
// 返回全局镜头号映射 / 镜头索引映射 / 编组列表。
func packVideoCards(shots []ShotVideoDTO) (map[int64]int, map[int64]ShotVideoDTO, [][]int64) {
	globalOrder := make(map[int64]int, len(shots))
	shotsByID := make(map[int64]ShotVideoDTO, len(shots))
	for i, sh := range shots {
		globalOrder[sh.ShotID] = i + 1
		shotsByID[sh.ShotID] = sh
	}
	const (
		targetSec = 10 // 单卡时长目标上限（LuxReal 实测卡片最长 10s）
		minSec    = 4  // 模型最短时长（尾卡合并阈值）
	)
	durOf := func(id int64) int {
		if d := shotsByID[id].DurationSec; d > 0 {
			return d
		}
		return 2
	}

	// ① 沿 shot_group 边界聚合为候选叙事块（保序；shots 已按组排序）
	type block struct {
		ids []int64
		sum int
	}
	blocks := make([]block, 0, 8)
	for _, sh := range shots {
		if n := len(blocks); n > 0 && sh.ShotGroupID != 0 && sh.ShotGroupID == shotsByID[blocks[n-1].ids[len(blocks[n-1].ids)-1]].ShotGroupID {
			j := &blocks[n-1]
			j.ids = append(j.ids, sh.ShotID)
			j.sum += durOf(sh.ShotID)
			continue
		}
		blocks = append(blocks, block{ids: []int64{sh.ShotID}, sum: durOf(sh.ShotID)})
	}

	// ② 组内二次切分（仅当组时长超过目标上限）：≤10s 贪心分段，
	//    剩余段 <4s 且并入前段后 ≤15s 则回并，避免产出生成不了的碎卡
	var out [][]int64
	for _, bl := range blocks {
		if bl.sum <= targetSec {
			out = append(out, bl.ids)
			continue
		}
		var seg []int64
		sum := 0
		for _, id := range bl.ids {
			if len(seg) > 0 && sum+durOf(id) > targetSec {
				out = append(out, seg)
				seg, sum = nil, 0
			}
			seg = append(seg, id)
			sum += durOf(id)
		}
		if len(seg) > 0 {
			out = append(out, seg)
		}
	}

	// ③ 尾卡不足模型最短时长 → 并入前一卡（H3 上限 15s 内才合并，否则保留由 clamp 收口）
	if len(out) >= 2 {
		last := out[len(out)-1]
		lsum := 0
		for _, id := range last {
			lsum += durOf(id)
		}
		prevSum := 0
		for _, id := range out[len(out)-2] {
			prevSum += durOf(id)
		}
		if lsum < minSec && prevSum+lsum <= 15 {
			out[len(out)-2] = append(out[len(out)-2], last...)
			out = out[:len(out)-1]
		}
	}
	return globalOrder, shotsByID, out
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

// ListBeatCards 成片卡列表（Step3 卡片网格轮询；GET /beats/:beat_id/cards）。
func (s *Service) ListBeatCards(c *gin.Context, beatUUID string) {
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
	cards, err := s.repo.ListCardsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取成片卡失败")
		return
	}
	httpx.OK(c, gin.H{"items": cards})
}

// RegenerateCard 单成片卡重新生成（POST /video-cards/:card_id/regenerate）：重置 → 建任务 → 计费冻结。
func (s *Service) RegenerateCard(c *gin.Context, cardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	cb, err := s.repo.ResolveCard(c.Request.Context(), cardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "成片卡不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), cb.ProjectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if cb.Status == "QUEUED" || cb.Status == "RUNNING" {
		httpx.Fail(c, http.StatusConflict, 30003, "该视频任务正在执行中，请稍候")
		return
	}
	// 重置卡片与卡内镜头状态（清旧视频）
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置状态失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE video_cards SET generation_status='PENDING', generated_video_url=NULL, updated_at=NOW()
		WHERE id=$1`, cb.ID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置状态失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE shots SET video_generation_status='PENDING', generated_video_url=NULL, updated_at=NOW()
		WHERE video_card_id=$1 AND deleted_at IS NULL`, cb.ID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置状态失败")
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置状态失败")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), cb.ProjectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskIDs, err := s.repo.CreateCardVideoTasks(c.Request.Context(), cb.ProjectID, spaceID, uid, cb.NarrativeUnitID,
		[]VideoCardDTO{{CardID: cb.ID, Title: cb.Title, DurationSec: cb.DurationSec}}, VideoGenOpts{})
	if err != nil || len(taskIDs) == 0 {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建视频任务失败")
		return
	}
	totalCost := cb.DurationSec * VideoPointsPerSec
	frozenCount := 0
	if s.billing != nil && totalCost > 0 {
		if err := s.billing.Freeze(c.Request.Context(), "USER", uid, taskIDs[0], totalCost, &billing.TxnMeta{
			SpaceID:        spaceID,
			OperatorUserID: uid,
			ProjectID:      &cb.ProjectID,
			Description:    fmt.Sprintf("成片卡重生成 %s", cb.Title),
		}); err != nil {
			_ = s.repo.MarkTaskFailed(c.Request.Context(), taskIDs[0], "积分不足或冻结失败："+err.Error())
			_, _ = s.repo.pool.Exec(c.Request.Context(),
				`UPDATE video_cards SET generation_status='FAILED_DEAD' WHERE id=$1`, cb.ID)
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "积分不足，无法重新生成")
			return
		}
		frozenCount = 1
	}
	httpx.OK(c, gin.H{"task_id": taskIDs[0], "card_id": cb.ID, "frozen_count": frozenCount, "total_cost": totalCost})
}

// UpdateCardPrompt 编辑成片卡提示词（PATCH /video-cards/:card_id/prompt；空串恢复自动组装）。
func (s *Service) UpdateCardPrompt(c *gin.Context, cardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		PromptText string `json:"prompt_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	cb, err := s.repo.ResolveCard(c.Request.Context(), cardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "成片卡不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), cb.ProjectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if err := s.repo.SetCardPrompt(c.Request.Context(), cb.ID, strings.TrimSpace(req.PromptText)); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存提示词失败")
		return
	}
	httpx.OK(c, gin.H{"card_id": cb.ID})
}

// UpdateCardDuration 调整成片卡送模型时长（PATCH /video-cards/:card_id/duration；
// 对齐 LuxReal 卡头时长下拉：4~15s 合法区间，越界 clamp，已 SUCCESS 的卡下次重新生成生效）。
func (s *Service) UpdateCardDuration(c *gin.Context, cardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		DurationSec int `json:"duration_sec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.DurationSec <= 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：duration_sec 必须为正整数")
		return
	}
	cb, err := s.repo.ResolveCard(c.Request.Context(), cardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "成片卡不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), cb.ProjectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if cb.Status == "QUEUED" || cb.Status == "RUNNING" {
		httpx.Fail(c, http.StatusConflict, 30003, "该视频任务正在执行中，无法调整时长")
		return
	}
	dur := req.DurationSec
	if dur < 4 {
		dur = 4
	} else if dur > 15 {
		dur = 15
	}
	if err := s.repo.SetCardDuration(c.Request.Context(), cb.ID, dur); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存时长失败")
		return
	}
	httpx.OK(c, gin.H{"card_id": cb.ID, "duration_sec": dur})
}

// BindCardAsset 成片卡绑定参考资产（POST /video-cards/:card_id/assets/bind）：
// 拖拽语义对齐 LuxReal——把资产 chip 绑进卡作为参考图；作用于卡内全部镜头。
func (s *Service) BindCardAsset(c *gin.Context, cardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		AssetKind string `json:"asset_kind"` // CHARACTER/SCENE/PROP
		AssetID   int64  `json:"asset_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AssetID == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：asset_kind/asset_id 必填")
		return
	}
	switch req.AssetKind {
	case "CHARACTER", "SCENE", "PROP":
	default:
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "asset_kind 非法")
		return
	}
	cb, err := s.repo.ResolveCard(c.Request.Context(), cardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "成片卡不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), cb.ProjectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	n, err := s.repo.BindCardAsset(c.Request.Context(), cb.ID, req.AssetKind, req.AssetID)
	if err != nil {
		s.lg.Warn("卡级资产绑定失败", zap.Error(err), zap.Int64("card_id", cb.ID))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "绑定资产失败")
		return
	}
	httpx.OK(c, gin.H{"card_id": cb.ID, "bound_shots": n})
}

// BindShotAsset 镜头级绑定参考资产（POST /shots/:shot_id/assets/bind）：
// 精准到单个镜头；镜头级显式引用生效后不再回落单元级继承（LoadShotRefAssets 口径）。
func (s *Service) BindShotAsset(c *gin.Context, shotUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		AssetKind string `json:"asset_kind"` // CHARACTER/SCENE/PROP
		AssetID   int64  `json:"asset_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AssetID == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：asset_kind/asset_id 必填")
		return
	}
	switch req.AssetKind {
	case "CHARACTER", "SCENE", "PROP":
	default:
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "asset_kind 非法")
		return
	}
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), shotUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	var projectID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(),
		`SELECT project_id FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).Scan(&projectID); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	inserted, err := s.repo.BindShotAsset(c.Request.Context(), shotID, req.AssetKind, req.AssetID)
	if err != nil {
		s.lg.Warn("镜头级资产绑定失败", zap.Error(err), zap.Int64("shot_id", shotID))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "绑定资产失败")
		return
	}
	httpx.OK(c, gin.H{"shot_id": shotUUID, "inserted": inserted})
}

// UnbindShotAsset 镜头级解绑参考资产（DELETE /shots/:shot_id/assets/bind）。
// removed=false 表示该引用来自叙事单元级继承（无镜头级显式行可删），需前端提示。
func (s *Service) UnbindShotAsset(c *gin.Context, shotUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	assetKind := c.Query("asset_kind")
	assetID, _ := strconv.ParseInt(c.Query("asset_id"), 10, 64)
	switch assetKind {
	case "CHARACTER", "SCENE", "PROP":
	default:
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "asset_kind 非法")
		return
	}
	if assetID == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 asset_id")
		return
	}
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), shotUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	var projectID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(),
		`SELECT project_id FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).Scan(&projectID); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	n, err := s.repo.UnbindShotAsset(c.Request.Context(), shotID, assetKind, assetID)
	if err != nil {
		s.lg.Warn("镜头级资产解绑失败", zap.Error(err), zap.Int64("shot_id", shotID))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "解绑资产失败")
		return
	}
	httpx.OK(c, gin.H{"shot_id": shotUUID, "removed": n > 0})
}

// UnbindCardAsset 解绑成片卡参考资产（DELETE /video-cards/:card_id/assets/bind）。
func (s *Service) UnbindCardAsset(c *gin.Context, cardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	assetKind := c.Query("asset_kind")
	assetID, _ := strconv.ParseInt(c.Query("asset_id"), 10, 64)
	switch assetKind {
	case "CHARACTER", "SCENE", "PROP":
	default:
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "asset_kind 非法")
		return
	}
	if assetID == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 asset_id")
		return
	}
	cb, err := s.repo.ResolveCard(c.Request.Context(), cardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "成片卡不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), cb.ProjectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	if err := s.repo.UnbindCardAsset(c.Request.Context(), cb.ID, assetKind, assetID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "解绑失败")
		return
	}
	httpx.OK(c, gin.H{"card_id": cb.ID})
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
		Config:            p.Config,
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
