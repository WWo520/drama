// 创作域补齐（S5 接口覆盖率）：叙事单元详情/重新规划/镜头块/分镜/镜头/任务查询/视频辅助。
// 表均为既有表（narrative_units/shot_groups/shots/generation_tasks/export_records）。
package creation

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// StoryboardService 创作域补齐服务。
type StoryboardService struct {
	repo *Repo
}

func NewStoryboardService(repo *Repo) *StoryboardService {
	return &StoryboardService{repo: repo}
}

// ---- 3.1.x 叙事单元 / 镜头块 / 镜头 ----

// BeatDetail 3.1.5 叙事单元详情（含剧本原文）。
func (s *StoryboardService) BeatDetail(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	beatID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	var nuID int64
	var nuUUID, title, scriptText, planningMode, status string
	var presetDur, actualDur, versionNum int
	var sortOrder int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, nu_uuid::text, title, COALESCE(script_original_text,''), COALESCE(planning_mode::text,''),
		       COALESCE(status::text,''), preset_duration_sec, COALESCE(actual_duration_sec,0),
		       COALESCE(version_num,1), sort_order
		FROM narrative_units WHERE id=$1 AND deleted_at IS NULL`, beatID).
		Scan(&nuID, &nuUUID, &title, &scriptText, &planningMode, &status, &presetDur, &actualDur, &versionNum, &sortOrder); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	// 镜头块 + 镜头数
	groups, _ := s.listShotGroups(c, nuID)
	httpx.OK(c, gin.H{
		"beat_id": nuUUID, "title": title, "script_original_text": scriptText,
		"planning_mode": planningMode, "status": status,
		"preset_duration_sec": presetDur, "actual_duration_sec": actualDur,
		"version_num": versionNum, "sort_order": sortOrder,
		"shot_groups": groups,
	})
}

// listShotGroups 镜头块列表（内嵌 shots）。
func (s *StoryboardService) listShotGroups(c *gin.Context, nuID int64) ([]map[string]any, error) {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, sg_uuid::text, COALESCE(title,''), sort_order, duration_sec,
		       COALESCE(generated_video_url,''), COALESCE(video_generation_status::text,''),
		       COALESCE(shot_count,0)
		FROM shot_groups WHERE narrative_unit_id=$1 AND deleted_at IS NULL ORDER BY sort_order ASC`, nuID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var sgUUID, title, genStatus, vURL string
		var order, dur, count int
		if err := rows.Scan(&id, &sgUUID, &title, &order, &dur, &vURL, &genStatus, &count); err != nil {
			continue
		}
		shots := s.listShotsOfGroup(c, id)
		out = append(out, map[string]any{
			"shot_group_id": sgUUID, "title": title, "sort_order": order,
			"duration_sec": dur, "generated_video_url": vURL,
			"video_generation_status": genStatus, "shot_count": len(shots), "shots": shots,
		})
	}
	return out, rows.Err()
}

func (s *StoryboardService) listShotsOfGroup(c *gin.Context, groupID int64) []map[string]any {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, shot_uuid::text, sort_order, duration_sec, COALESCE(shot_size::text,''),
		       COALESCE(camera_movement::text,''), COALESCE(camera_angle::text,''),
		       COALESCE(subject_description,''), COALESCE(background_description,''),
		       COALESCE(local_light_description,''), COALESCE(dialogue_text,''),
		       COALESCE(ambient_sound,''), COALESCE(prompt_refined,''),
		       COALESCE(video_generation_status::text,''), COALESCE(generated_video_url,'')
		FROM shots WHERE shot_group_id=$1 AND deleted_at IS NULL ORDER BY sort_order ASC`, groupID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	ids := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		var shotUUID, size, move, angle, subj, bg, light, dialog, sound, prompt, vStatus, vURL string
		var order, dur int
		if err := rows.Scan(&id, &shotUUID, &order, &dur, &size, &move, &angle, &subj, &bg, &light, &dialog, &sound, &prompt, &vStatus, &vURL); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"shot_id": shotUUID, "sort_order": order, "duration_sec": dur,
			"shot_size": size, "camera_movement": move, "camera_angle": angle,
			"subject_description": subj, "background_description": bg,
			"local_light_description": light, "dialogue_text": dialog,
			"ambient_sound": sound, "prompt_refined": prompt,
			"video_generation_status": vStatus, "generated_video_url": vURL,
			"_shot_numeric_id": id,
		})
		ids = append(ids, id)
	}
	// 附带每镜头关联资产（分镜脚本视图资产小图与 @ 引用；失败不阻断）
	if refs, rerr := s.repo.ShotAssetRefsByIDs(c.Request.Context(), ids); rerr == nil {
		for _, m := range out {
			if id, ok := m["_shot_numeric_id"].(int64); ok {
				if la := refs[id]; len(la) > 0 {
					m["linked_assets"] = la
				}
			}
			delete(m, "_shot_numeric_id")
		}
	} else {
		for _, m := range out {
			delete(m, "_shot_numeric_id")
		}
	}
	return out
}

// BeatShotGroups 3.1.7 镜头块列表（含内嵌 shots）。
func (s *StoryboardService) BeatShotGroups(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	beatID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	groups, err := s.listShotGroups(c, beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取镜头块失败")
		return
	}
	httpx.OK(c, gin.H{"items": groups})
}

// BeatStoryboards 3.3.2 分镜脚本列表（= 镜头列表扁平化，含分镜图状态）。
func (s *StoryboardService) BeatStoryboards(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	beatID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT sh.id, sh.shot_uuid::text, sh.sort_order, sh.duration_sec,
		       COALESCE(sh.shot_size::text,''), COALESCE(sh.prompt_refined,''),
		       COALESCE(gi.original_url,''), COALESCE(sh.video_generation_status::text,''),
		       COALESCE(sh.generated_video_url,'')
		FROM shots sh
		LEFT JOIN generated_images gi ON gi.asset_id = sh.id AND gi.deleted_at IS NULL
		WHERE sh.narrative_unit_id=$1 AND sh.deleted_at IS NULL
		ORDER BY sh.sort_order ASC`, beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取分镜列表失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var shotUUID, size, prompt, imgURL, vStatus, vURL string
		var order, dur int
		if err := rows.Scan(&id, &shotUUID, &order, &dur, &size, &prompt, &imgURL, &vStatus, &vURL); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"storyboard_id": shotUUID, "shot_id": shotUUID, "sort_order": order,
			"duration_sec": dur, "shot_size": size, "prompt_refined": prompt,
			"image_url": imgURL, "video_generation_status": vStatus, "generated_video_url": vURL,
		})
	}
	httpx.OK(c, gin.H{"items": items})
}

// GenerateStoryboards 3.3.1 生成分镜脚本（18 - 视频制作 阶段二：真实 STORYBOARD_PLAN_V1_LUXCHAT 任务）。
// SCRIPT_SPLIT 拆分直出的分镜为 v1；本端点对单个叙事单元重跑分镜技能（v2+），版本快照可回滚。
func (s *StoryboardService) GenerateStoryboards(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	beatID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	var req struct {
		PlanningMode string `json:"planning_mode"`
		SkillPrompt  string `json:"skill_prompt"` // 分镜技能风格提示词（可选）
	}
	_ = c.ShouldBindJSON(&req)
	// RUNNING 门控：存在生成中视频任务的单元拒绝重规划（Worker 侧 SaveStoryboardPlan 二次校验兜底）
	var running int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT count(*) FROM shots WHERE narrative_unit_id=$1 AND deleted_at IS NULL AND video_generation_status='RUNNING'`,
		beatID).Scan(&running); err == nil && running > 0 {
		httpx.Fail(c, http.StatusConflict, 6003, "本单元有镜头视频正在生成，请等待完成后再重新生成分镜")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskUUID, err := s.createStoryboardPlanTask(c, spaceID, uid, projectID, beatID, req.PlanningMode, req.SkillPrompt)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建分镜生成任务失败")
		return
	}
	httpx.OK(c, gin.H{"task_id": taskUUID, "beat_id": beatUUID, "status": "QUEUED"})
}

// createStoryboardPlanTask 建 STORYBOARD_PLAN_V1_LUXCHAT 任务 + Outbox（queue=split，LLM 类任务）。
func (s *StoryboardService) createStoryboardPlanTask(c *gin.Context, spaceID, uid, projectID, beatID int64, planningMode, skillPrompt string) (string, error) {
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{
		"beat_id":       beatID,
		"planning_mode": planningMode,
		"skill_prompt":  skillPrompt,
	})
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		return "", err
	}
	defer tx.Rollback(c.Request.Context())
	var taskID int64
	if err := tx.QueryRow(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, request_id, idempotency_key, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, 'sbp-'||gen_random_uuid(), 'sbp-'||gen_random_uuid(), $2, $3, $4, 'STORYBOARD_PLAN_V1_LUXCHAT', 'QUEUED', $5::jsonb, 0, 0, 3)
		RETURNING id`,
		taskUUID, spaceID, uid, projectID, string(params)).Scan(&taskID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:sbp', 'split',
		        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
		taskID); err != nil {
		return "", err
	}
	return taskUUID, tx.Commit(c.Request.Context())
}

// ShotDetail 3.1.8 单镜头详情（完整 Prompt/资产绑定）。
func (s *StoryboardService) ShotDetail(c *gin.Context, shotUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), shotUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	var nuID, projectID int64
	var size, move, angle, subj, bg, light, dialog, sound, pOrig, pRef, vStatus, vURL, refJSON, linkJSON string
	var order, dur int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT narrative_unit_id, project_id, sort_order, duration_sec,
		       COALESCE(shot_size::text,''), COALESCE(camera_movement::text,''), COALESCE(camera_angle::text,''),
		       COALESCE(subject_description,''), COALESCE(background_description,''),
		       COALESCE(local_light_description,''), COALESCE(dialogue_text,''),
		       COALESCE(ambient_sound,''), COALESCE(prompt_original,''), COALESCE(prompt_refined,''),
		       COALESCE(video_generation_status::text,''), COALESCE(generated_video_url,''),
		       COALESCE(reference_images_json::text,'[]'), COALESCE(linked_assets_json::text,'[]')
		FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).
		Scan(&nuID, &projectID, &order, &dur, &size, &move, &angle, &subj, &bg, &light, &dialog, &sound, &pOrig, &pRef, &vStatus, &vURL, &refJSON, &linkJSON); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	httpx.OK(c, gin.H{
		"shot_id": shotUUID, "sort_order": order, "duration_sec": dur,
		"shot_size": size, "camera_movement": move, "camera_angle": angle,
		"subject_description": subj, "background_description": bg,
		"local_light_description": light, "dialogue_text": dialog,
		"ambient_sound": sound, "prompt_original": pOrig, "prompt_refined": pRef,
		"video_generation_status": vStatus, "generated_video_url": vURL,
		"reference_images": json.RawMessage(refJSON), "linked_assets": json.RawMessage(linkJSON),
	})
}

// ShotsBatch 3.1.9 批量获取多个镜头详情。
func (s *StoryboardService) ShotsBatch(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		ShotIDs []string `json:"shot_ids" binding:"required,min=1,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	items := make([]map[string]any, 0, len(req.ShotIDs))
	for _, sid := range req.ShotIDs {
		shotID, err := s.repo.ResolveShotID(c.Request.Context(), sid)
		if err != nil {
			continue
		}
		var projectID int64
		var prompt, vURL, vStatus string
		if err := s.repo.pool.QueryRow(c.Request.Context(), `
			SELECT project_id, COALESCE(prompt_refined,''), COALESCE(generated_video_url,''),
			       COALESCE(video_generation_status::text,'')
			FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).Scan(&projectID, &prompt, &vURL, &vStatus); err != nil {
			continue
		}
		if s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
			continue
		}
		items = append(items, map[string]any{"shot_id": sid, "prompt_refined": prompt, "generated_video_url": vURL, "video_generation_status": vStatus})
	}
	httpx.OK(c, gin.H{"items": items})
}

// UpdateShot 镜头编辑（Step3 高价值快批量：编辑提示词 / 调整时长 / 分镜字段微调）。
// 仅更新传入的非空字段；不动 video_generation_status（已生成视频不重置，
// 用户改完提示词后自行点「重新生成」应用新值——避免编辑即清视频造成误删）。
func (s *StoryboardService) UpdateShot(c *gin.Context, shotUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		PromptRefined         *string `json:"prompt_refined"`
		DurationSec           *int    `json:"duration_sec"`
		SubjectDescription    *string `json:"subject_description"`
		BackgroundDescription *string `json:"background_description"`
		DialogueText          *string `json:"dialogue_text"`
		AmbientSound          *string `json:"ambient_sound"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.PromptRefined == nil && req.DurationSec == nil && req.SubjectDescription == nil &&
		req.BackgroundDescription == nil && req.DialogueText == nil && req.AmbientSound == nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "无可更新字段")
		return
	}
	if req.DurationSec != nil && (*req.DurationSec < 1 || *req.DurationSec > 30) {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "时长须在 1~30 秒之间")
		return
	}
	if req.PromptRefined != nil && len([]rune(*req.PromptRefined)) > 8000 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "提示词过长（≤8000 字）")
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
	// 逐字段 COALESCE 合并（nil 字段保留原值）
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE shots SET
			prompt_refined         = COALESCE($2, prompt_refined),
			duration_sec           = COALESCE($3, duration_sec),
			subject_description    = COALESCE($4, subject_description),
			background_description = COALESCE($5, background_description),
			dialogue_text          = COALESCE($6, dialogue_text),
			ambient_sound          = COALESCE($7, ambient_sound),
			updated_at             = NOW()
		WHERE id=$1 AND deleted_at IS NULL`,
		shotID, req.PromptRefined, req.DurationSec, req.SubjectDescription,
		req.BackgroundDescription, req.DialogueText, req.AmbientSound); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新镜头失败")
		return
	}
	// 回读最新值（前端就地刷新）
	var dur int
	var prompt string
	_ = s.repo.pool.QueryRow(c.Request.Context(),
		`SELECT duration_sec, COALESCE(prompt_refined,'') FROM shots WHERE id=$1`, shotID).Scan(&dur, &prompt)
	httpx.OK(c, gin.H{"shot_id": shotUUID, "duration_sec": dur, "prompt_refined": prompt})
}

// ---- 3.3.7b/3.3.8 镜头块视频 ----

// GenerateShotGroupVideo 3.3.7b 单个镜头块视频生成（18 - 视频制作 阶段三：补 Outbox 真实派发）。
func (s *StoryboardService) GenerateShotGroupVideo(c *gin.Context, sgUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	sgID, projectID, err := s.repo.ResolveShotGroup(c.Request.Context(), sgUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头块不存在")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	// 建块内全部非 SUCCESS 镜头的 VIDEO_SEGMENT_GEN 任务（逐镜头生成，Outbox 派发）
	var beatID int64
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT narrative_unit_id FROM shot_groups WHERE id=$1`, sgID).Scan(&beatID)
	shots, err := s.repo.ListShotsByGroup(c.Request.Context(), sgID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取镜头失败")
		return
	}
	taskIDs, err := s.repo.CreateVideoTasks(c.Request.Context(), projectID, spaceID, uid, beatID, shots, VideoGenOpts{})
	if err != nil || len(taskIDs) == 0 {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建视频任务失败")
		return
	}
	httpx.OK(c, gin.H{"created": len(taskIDs), "total": len(shots), "status": "QUEUED"})
}

// ConcatShotGroupVideo 3.3.8 镜头块拼接（VIDEO_GROUP_CONCAT，Outbox 真实派发 → Worker 校验全部 SUCCESS 后标记 COMPOSED）。
func (s *StoryboardService) ConcatShotGroupVideo(c *gin.Context, sgUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	sgID, projectID, err := s.repo.ResolveShotGroup(c.Request.Context(), sgUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头块不存在")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskUUID, err := s.createDispatchedTask(c, spaceID, uid, projectID, "VIDEO_GROUP_CONCAT",
		map[string]any{"shot_group_id": sgID}, "video")
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建拼接任务失败")
		return
	}
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED"})
}

// createDispatchedTask 建任务 + Outbox 真实派发（18 - 视频制作 阶段三：替代只 INSERT 不派发的 mock）。
func (s *StoryboardService) createDispatchedTask(c *gin.Context, spaceID, uid, projectID int64, taskType string, params map[string]any, queue string) (string, error) {
	taskUUID := newUUID()
	pj, _ := json.Marshal(params)
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		return "", err
	}
	defer tx.Rollback(c.Request.Context())
	var taskID int64
	if err := tx.QueryRow(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, request_id, idempotency_key, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, 'disp-'||gen_random_uuid(), 'disp-'||gen_random_uuid(), $2, $3, $4, $5, 'QUEUED', $6::jsonb, 0, 0, 3)
		RETURNING id`,
		taskUUID, spaceID, uid, projectID, taskType, string(pj)).Scan(&taskID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:'||$2, $2,
		        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
		taskID, queue); err != nil {
		return "", err
	}
	return taskUUID, tx.Commit(c.Request.Context())
}

// GenerateShotRefImage 为单镜头生成镜头级参考图（SHOT_REF_IMAGE_GEN，
// 对齐 LuxReal「镜头N的参考图」/ 智能镜头；任务经 Outbox 真实派发，由 asset worker 消费）。
func (s *StoryboardService) GenerateShotRefImage(c *gin.Context, shotIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	shotID, err := strconv.ParseInt(shotIDStr, 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "镜头 ID 非法")
		return
	}
	var projectID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT project_id FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).Scan(&projectID); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "镜头不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskUUID, err := s.createDispatchedTask(c, spaceID, uid, projectID, "SHOT_REF_IMAGE_GEN",
		map[string]any{"shot_id": shotID}, "asset")
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建镜头参考图任务失败")
		return
	}
	httpx.OK(c, gin.H{"task_id": taskUUID, "shot_id": shotID})
}

// ---- 3.3.9 叙事单元整片合成 ----

// ComposeBeatVideo 3.3.9 叙事单元整片视频合成（18 - 视频制作 阶段三：诚实化——
// 原 mock 只 INSERT 不派发必留僵尸任务；单元级成片由 Step4 项目导出（FINAL_VIDEO_COMPOSE
// 走 export_records 真实链路）承载，本端点改为完成度校验 + 可合成清单返回）。
func (s *StoryboardService) ComposeBeatVideo(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	beatID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	shots, err := s.repo.ListShotsByBeat(c.Request.Context(), beatID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取镜头失败")
		return
	}
	total, done := len(shots), 0
	for _, sh := range shots {
		if sh.VideoStatus == "SUCCESS" {
			done++
		}
	}
	if done < total {
		httpx.Fail(c, http.StatusConflict, 6003,
			fmt.Sprintf("本单元还有 %d 个镜头未完成视频生成，无法合成", total-done))
		return
	}
	httpx.OK(c, gin.H{
		"beat_id": beatUUID, "status": "READY", "shots_done": done, "shots_total": total,
		"hint": "单元内镜头已全部完成，可前往 Step4 成片预览进行项目级合成导出",
	})
}

// ---- 3.3.7c 重新规划 ----

// ReplanBeat 3.3.7c 叙事单元重新规划（18 - 视频制作 阶段二：收敛到 STORYBOARD_PLAN 真实任务，
// 原 BEAT_REPLAN 为枚举外硬编码 INSERT（DB 层即失败）已移除）。
func (s *StoryboardService) ReplanBeat(c *gin.Context, beatUUID string) {
	s.GenerateStoryboards(c, beatUUID)
}

// ---- 3.3.3 分镜详情 / 3.3.6 分镜图编辑 / 3.3.7 分镜视频 ----

// StoryboardDetail 3.3.3 分镜详情（= 单镜头详情，分镜与镜头同实体）。
func (s *StoryboardService) StoryboardDetail(c *gin.Context, storyboardUUID string) {
	s.ShotDetail(c, storyboardUUID)
}

// StoryboardImageEdit 3.3.6 编辑单张分镜图（暂未开放：原 STORYBOARD_IMAGE_EDIT 为枚举外任务类型，
// 硬编码 INSERT 在 DB 层即报枚举非法；接口保留形状，待扩枚举后接入真实链路）。
func (s *StoryboardService) StoryboardImageEdit(c *gin.Context, storyboardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "分镜图编辑功能暂未开放，敬请期待")
}

// StoryboardVideo 3.3.7 单个分镜视频生成（18 - 视频制作 阶段三：路由已指向 /shots/:id/regenerate
// 真实链路（计费 + Outbox），本方法保留兼容占位；storyboard 与 shot 同实体）。
func (s *StoryboardService) StoryboardVideo(c *gin.Context, storyboardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "请使用单镜头重新生成接口（POST /shots/:shot_id/regenerate）")
}

// BatchGenerateStoryboardImage 3.3.5 批量生成分镜图（暂未开放：STORYBOARD_IMAGE_GEN 枚举外，同上）。
func (s *StoryboardService) BatchGenerateStoryboardImage(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "分镜图批量生成功能暂未开放，敬请期待")
}

// ---- 3.3.14 / 3.3.16 任务查询/重试 ----

// TaskDetail 3.3.14 查询单个任务状态/进度（统一轮询接口）。
func (s *StoryboardService) TaskDetail(c *gin.Context, taskUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var taskID, userID, projectID int64
	var tType, status, msg, errMsg, result string
	var progress, retry, maxRetry int
	var errCode int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, user_id, COALESCE(project_id,0), task_type::text, status::text,
		       COALESCE(progress_pct,0), COALESCE(progress_message,''), retry_count, max_retry_count,
		       COALESCE(error_code,0), COALESCE(error_message,''), COALESCE(result_output_json::text,'')
		FROM generation_tasks WHERE task_uuid=$1 AND deleted_at IS NULL`, taskUUID).
		Scan(&taskID, &userID, &projectID, &tType, &status, &progress, &msg, &retry, &maxRetry, &errCode, &errMsg, &result); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "任务不存在")
		return
	}
	if userID != uid && projectID > 0 {
		// 非本人任务：校验项目权限
		if s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "任务不存在")
			return
		}
	}
	httpx.OK(c, gin.H{
		"task_id": taskUUID, "task_type": tType, "status": status,
		"progress_pct": progress, "progress_message": msg,
		"retry_count": retry, "max_retry_count": maxRetry,
		"error_code": errCode, "error_message": errMsg,
		"result": json.RawMessage(result),
	})
}

// TaskRetry 3.3.16 重试失败任务。
func (s *StoryboardService) TaskRetry(c *gin.Context, taskUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var taskID, userID int64
	var status string
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, user_id, status::text FROM generation_tasks WHERE task_uuid=$1 AND deleted_at IS NULL`, taskUUID).
		Scan(&taskID, &userID, &status); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "任务不存在")
		return
	}
	if userID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权重试该任务")
		return
	}
	if status != "FAILED" && status != "FAILED_DEAD" && status != "TIMEOUT" {
		httpx.Fail(c, http.StatusBadRequest, 20009, "仅失败状态任务可重试")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE generation_tasks SET status='QUEUED', retry_count=retry_count+1, error_message=NULL, updated_at=NOW()
		WHERE id=$1`, taskID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重试失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 3.3.10 / 3.3.12 视频辅助 ----

// VideoTrackTimeline 3.3.10 读取多轨时间线（视频/图片/音频三轨）。
func (s *StoryboardService) VideoTrackTimeline(c *gin.Context, videoUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	// video_id 按 shot_uuid 解析（单镜头视频时间线）
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), videoUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "视频不存在")
		return
	}
	var projectID int64
	var dur int
	var vURL, refJSON string
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT project_id, duration_sec, COALESCE(generated_video_url,''),
		       COALESCE(reference_images_json::text,'[]')
		FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).Scan(&projectID, &dur, &vURL, &refJSON); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "视频不存在")
		return
	}
	if s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "视频不存在")
		return
	}
	httpx.OK(c, gin.H{
		"video_id": videoUUID, "duration_sec": dur,
		"tracks": []map[string]any{
			{"track_type": "VIDEO", "clips": []map[string]any{{"url": vURL, "start_sec": 0, "end_sec": dur}}},
			{"track_type": "IMAGE", "clips": json.RawMessage(refJSON)},
			{"track_type": "AUDIO", "clips": []map[string]any{}},
		},
	})
}

// VideoTTS 3.3.12 提交 TTS 配音任务（暂未开放：TTS_VOICE_GEN 为枚举外任务类型，
// 硬编码 INSERT 在 DB 层即报枚举非法；待扩枚举后接入真实链路）。
func (s *StoryboardService) VideoTTS(c *gin.Context, videoUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "AI 配音功能暂未开放，敬请期待")
}

// CreateUnitAsset POST /narrative-units/:nu_id/assets/create（Step3 补充缺失资产）：
// 视频制作时发现分镜引用的资产在 Step2 未被抽取到（LLM 抽取有遗漏），在此补建：
// 事务内创建 characters/scenes/props 记录并挂 nu_asset_links 到当前叙事单元，
// origin_episode_id 归属其所在剧集；同名资产幂等复用。生成参考图由前端复用 batch-generate。
func (s *StoryboardService) CreateUnitAsset(c *gin.Context, nuUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		AssetKind   string `json:"asset_kind" binding:"required"` // CHARACTER/SCENE/PROP
		Name        string `json:"name" binding:"required,min=1,max=64"`
		Description string `json:"description" binding:"omitempty,max=2000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：asset_kind/name 必填")
		return
	}
	switch req.AssetKind {
	case "CHARACTER", "SCENE", "PROP":
	default:
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "asset_kind 非法")
		return
	}
	_, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), nuUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	assetID, epSort, err := s.repo.CreateLinkedAsset(c.Request.Context(), nuUUID, req.AssetKind, req.Name, req.Description)
	if err != nil {
		log.Printf("补充关联资产失败: %v (nu=%s kind=%s)", err, nuUUID, req.AssetKind)
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建资产失败")
		return
	}
	kind := req.AssetKind
	genKind := kind // 前端 batch-generate 的 target kind：CHARACTER→CHARACTER_VARIANT 由前端映射
	httpx.OK(c, gin.H{"asset_id": assetID, "asset_kind": kind, "episode_sort_order": epSort, "gen_kind": genKind})
}

// EpisodeAssetLinks GET /episodes/:episode_id/asset-links（17 - 工作台对齐）。
// 返回剧集下各叙事单元的关联资产清单（nu_asset_links JOIN 名称/参考图/生成状态），
// 供 Step3「关联资产」名称占位展示与单元级就绪度判断。
// 取图口径（业务表，与 LoadShotRefs 一致）：角色→首个变体、场景/道具→自身 main_image_url；
// assets 表为通用注册表（无 main_image_url 列），不可作取图来源。
func (s *StoryboardService) EpisodeAssetLinks(c *gin.Context, epUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	epID, projectID, err := s.repo.ResolveEpisode(c.Request.Context(), epUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在")
		return
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT nu.nu_uuid::text, t.kind, t.asset_id, t.name, t.main_image_url, t.generation_status
		FROM (
			SELECT nal.narrative_unit_id, nal.asset_kind::text AS kind, nal.asset_id,
			       COALESCE(c.name, sc.base_name, p.name, '') AS name,
			       COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, '') AS main_image_url,
			       COALESCE(cv.generation_status, sc.generation_status::text, p.generation_status::text, '') AS generation_status,
			       CASE nal.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, nal.id AS rid
			FROM nu_asset_links nal
			LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND c.id=nal.asset_id AND c.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT main_image_url, generation_status::text AS generation_status
			    FROM character_variants WHERE character_id=c.id AND deleted_at IS NULL ORDER BY id LIMIT 1
			) cv ON TRUE
			LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON nal.asset_kind='PROP' AND p.id=nal.asset_id AND p.deleted_at IS NULL
		) t
		JOIN narrative_units nu ON nu.id = t.narrative_unit_id AND nu.deleted_at IS NULL
		WHERE nu.episode_id = $1
		ORDER BY nu.sort_order ASC, t.ord, t.rid`, epID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取关联资产失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var assetID int64
		var beatUUID, kind, name, img, status string
		if err := rows.Scan(&beatUUID, &kind, &assetID, &name, &img, &status); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"beat_id": beatUUID, "asset_kind": kind, "asset_id": assetID,
			"name": name, "main_image_url": img, "generation_status": status,
		})
	}
	httpx.OK(c, gin.H{"items": items})
}

func newUUID() string {
	return uuid.NewString()
}
