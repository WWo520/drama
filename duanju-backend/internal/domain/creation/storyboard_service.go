// 创作域补齐（S5 接口覆盖率）：叙事单元详情/重新规划/镜头块/分镜/镜头/任务查询/视频辅助。
// 表均为既有表（narrative_units/shot_groups/shots/generation_tasks/export_records）。
package creation

import (
	"encoding/json"
	"net/http"

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
		})
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

// GenerateStoryboards 3.3.1 生成分镜脚本（开发期：已由拆分产生分镜，此处幂等返回现状 + is_mock 标记）。
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
	var planningMode string
	_ = c.ShouldBindJSON(&struct {
		PlanningMode *string `json:"planning_mode"`
	}{&planningMode})
	// 开发期：分镜由 SCRIPT_SPLIT 阶段产生，此端点幂等返回现有分镜数（生产接入 LLM 重生成）
	var count int
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM shots WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, beatID).Scan(&count)
	httpx.OK(c, gin.H{"beat_id": beatUUID, "storyboard_count": count, "is_mock": true})
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

// ---- 3.3.7b/3.3.8 镜头块视频 ----

// GenerateShotGroupVideo 3.3.7b 单个镜头块视频生成。
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
	// 建 VIDEO_GROUP_CONCAT 任务（开发期 mock 执行）
	taskID, err := s.createGroupTask(c, spaceID, uid, projectID, sgID, "VIDEO_GROUP_CONCAT")
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建视频任务失败")
		return
	}
	httpx.OK(c, gin.H{"task_id": taskID, "status": "QUEUED", "is_mock": true})
}

// ConcatShotGroupVideo 3.3.8 镜头块 FFmpeg concat 拼接。
func (s *StoryboardService) ConcatShotGroupVideo(c *gin.Context, sgUUID string) {
	s.GenerateShotGroupVideo(c, sgUUID)
}

// createGroupTask 建镜头块级任务。
func (s *StoryboardService) createGroupTask(c *gin.Context, spaceID, uid, projectID, sgID int64, taskType string) (string, error) {
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"shot_group_id": sgID})
	_, err := s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, $5, 'QUEUED', $6::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, taskType, string(params))
	return taskUUID, err
}

// ---- 3.3.9 叙事单元整片合成 ----

// ComposeBeatVideo 3.3.9 叙事单元整片视频合成。
func (s *StoryboardService) ComposeBeatVideo(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	_, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"beat_uuid": beatUUID})
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, 'FINAL_VIDEO_COMPOSE', 'QUEUED', $5::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, string(params)); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建合成任务失败")
		return
	}
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED", "is_mock": true})
}

// ---- 3.3.7c 重新规划 ----

// ReplanBeat 3.3.7c 叙事单元重新规划（开发期：重建规划任务）。
func (s *StoryboardService) ReplanBeat(c *gin.Context, beatUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	_, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), beatUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	var req struct {
		PlanningMode string `json:"planning_mode"`
	}
	_ = c.ShouldBindJSON(&req)
	spaceID, _ := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"beat_uuid": beatUUID, "planning_mode": req.PlanningMode})
	_, _ = s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, 'BEAT_REPLAN', 'QUEUED', $5::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, string(params))
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED", "is_mock": true})
}

// ---- 3.3.3 分镜详情 / 3.3.6 分镜图编辑 / 3.3.7 分镜视频 ----

// StoryboardDetail 3.3.3 分镜详情（= 单镜头详情，分镜与镜头同实体）。
func (s *StoryboardService) StoryboardDetail(c *gin.Context, storyboardUUID string) {
	s.ShotDetail(c, storyboardUUID)
}

// StoryboardImageEdit 3.3.6 编辑单张分镜图（开发期 mock：任务入队）。
func (s *StoryboardService) StoryboardImageEdit(c *gin.Context, storyboardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), storyboardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "分镜不存在")
		return
	}
	var projectID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT project_id FROM shots WHERE id=$1`, shotID).Scan(&projectID); err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "分镜不存在")
		return
	}
	var req struct {
		Prompt    string `json:"prompt" binding:"required"`
		RefAssetID string `json:"reference_asset_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	spaceID, _ := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"shot_id": shotID, "prompt": req.Prompt, "reference_asset_id": req.RefAssetID})
	_, _ = s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, 'STORYBOARD_IMAGE_EDIT', 'QUEUED', $5::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, string(params))
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED", "is_mock": true})
}

// StoryboardVideo 3.3.7 提交单个分镜视频生成任务。
func (s *StoryboardService) StoryboardVideo(c *gin.Context, storyboardUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), storyboardUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "分镜不存在")
		return
	}
	var projectID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT project_id FROM shots WHERE id=$1`, shotID).Scan(&projectID); err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "分镜不存在")
		return
	}
	spaceID, _ := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"shot_id": shotID})
	_, _ = s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, 'VIDEO_SEGMENT_GEN', 'QUEUED', $5::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, string(params))
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED", "is_mock": true})
}

// BatchGenerateStoryboardImage 3.3.5 批量生成分镜图。
func (s *StoryboardService) BatchGenerateStoryboardImage(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		ShotIDs     []string `json:"shot_ids" binding:"required,min=1,max=50"`
		Concurrency int      `json:"concurrency"`
		ForceRegen  bool     `json:"force_regen"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	created := 0
	for _, sid := range req.ShotIDs {
		shotID, err := s.repo.ResolveShotID(c.Request.Context(), sid)
		if err != nil {
			continue
		}
		var projectID int64
		if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT project_id FROM shots WHERE id=$1`, shotID).Scan(&projectID); err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
			continue
		}
		spaceID, _ := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
		taskUUID := newUUID()
		params, _ := json.Marshal(map[string]any{"shot_id": shotID, "force_regen": req.ForceRegen})
		if _, err := s.repo.pool.Exec(c.Request.Context(), `
			INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
				status, request_params_json, progress_pct, retry_count, max_retry_count)
			VALUES ($1, $2, $3, $4, 'STORYBOARD_IMAGE_GEN', 'QUEUED', $5::jsonb, 0, 0, 3)`,
			taskUUID, spaceID, uid, projectID, string(params)); err == nil {
			created++
		}
	}
	if created == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "无有效分镜可生成")
		return
	}
	httpx.OK(c, gin.H{"created": created, "total": len(req.ShotIDs), "is_mock": true})
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

// VideoTTS 3.3.12 提交 TTS 配音任务。
func (s *StoryboardService) VideoTTS(c *gin.Context, videoUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	shotID, err := s.repo.ResolveShotID(c.Request.Context(), videoUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "视频不存在")
		return
	}
	var projectID int64
	var dialog string
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT project_id, COALESCE(dialogue_text,'') FROM shots WHERE id=$1 AND deleted_at IS NULL`, shotID).
		Scan(&projectID, &dialog); err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "视频不存在")
		return
	}
	var req struct {
		VoiceCode string `json:"voice_code" binding:"omitempty"`
	}
	_ = c.ShouldBindJSON(&req)
	spaceID, _ := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"shot_id": shotID, "dialogue_text": dialog, "voice_code": req.VoiceCode})
	_, _ = s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, 'TTS_VOICE_GEN', 'QUEUED', $5::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, string(params))
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED", "is_mock": true})
}

func newUUID() string {
	return uuid.NewString()
}
