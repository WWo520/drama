// 创作域补齐（S5 接口覆盖率）：角色/场景/道具/资产/剧集详情与编辑端点。
package creation

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// AssetDetailService 资产详情服务（复用 Repo）。
type AssetDetailService struct {
	repo *Repo
}

func NewAssetDetailService(repo *Repo) *AssetDetailService {
	return &AssetDetailService{repo: repo}
}

// ---- 3.2.4 修改角色基础信息 ----

// UpdateCharacter PATCH /characters/:character_id。
func (s *AssetDetailService) UpdateCharacter(c *gin.Context, charIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	charID, projectID, err := s.repo.ResolveCharacter(c.Request.Context(), charIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "角色不存在")
		return
	}
	var req struct {
		Name            string   `json:"name"`
		AppearancePrompt string  `json:"appearance_prompt"`
		Tags            []string `json:"tags"`
		Description     string   `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	sets := []string{}
	args := []any{charID}
	if req.Name != "" {
		args = append(args, req.Name)
		sets = append(sets, "name=$"+itoa(len(args)))
	}
	if req.AppearancePrompt != "" {
		args = append(args, req.AppearancePrompt)
		sets = append(sets, "appearance_base_prompt=$"+itoa(len(args)))
	}
	if req.Description != "" {
		args = append(args, req.Description)
		sets = append(sets, "description_notes=$"+itoa(len(args)))
	}
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(req.Tags)
		args = append(args, string(tagsJSON))
		sets = append(sets, "tags=$"+itoa(len(args)))
	}
	if len(sets) == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "无更新字段")
		return
	}
	query := "UPDATE characters SET " + joinComma(sets) + ", updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL"
	if _, err := s.repo.pool.Exec(c.Request.Context(), query, args...); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新角色失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 3.2.6 角色变体详情 ----

// VariantDetail GET /character-variants/:variant_id。
func (s *AssetDetailService) VariantDetail(c *gin.Context, variantIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var variantID, charID, projectID int64
	var variantName, vType, clothing, mainImg, viewsJSON, res, status string
	var aspect string
	var pointCost int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT cv.id, cv.character_id, ch.project_id, COALESCE(cv.variant_name,''),
		       COALESCE(cv.variant_type::text,''), COALESCE(cv.clothing_desc,''),
		       COALESCE(cv.main_image_url,''), COALESCE(cv.multi_view_images_json::text,'[]'),
		       COALESCE(cv.image_resolution,''), COALESCE(cv.aspect_ratio,''),
		       COALESCE(cv.generation_status::text,''), COALESCE(cv.point_cost,0)
		FROM character_variants cv JOIN characters ch ON ch.id=cv.character_id
		WHERE cv.id=$1 AND cv.deleted_at IS NULL`, variantIDStr).
		Scan(&variantID, &charID, &projectID, &variantName, &vType, &clothing, &mainImg, &viewsJSON, &res, &aspect, &status, &pointCost); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "变体不存在")
		return
	}
	_ = variantID
	if s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "变体不存在")
		return
	}
	httpx.OK(c, gin.H{
		"variant_id": variantIDStr, "character_id": charID,
		"variant_name": variantName, "variant_type": vType, "clothing_desc": clothing,
		"main_image_url": mainImg, "multi_view_images": json.RawMessage(viewsJSON),
		"image_resolution": res, "aspect_ratio": aspect,
		"generation_status": status, "point_cost": pointCost,
	})
}

// ---- 3.2.11 修改场景描述 ----

// UpdateScene PATCH /scenes/:scene_id。
func (s *AssetDetailService) UpdateScene(c *gin.Context, sceneIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	sceneID, projectID, err := s.repo.ResolveScene(c.Request.Context(), sceneIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "场景不存在")
		return
	}
	var req struct {
		BasePrompt  string `json:"base_prompt"`
		BaseName    string `json:"base_name"`
		TimeOfDay   string `json:"time_of_day"`
		Weather     string `json:"weather"`
		ExtraPrompt string `json:"extra_prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	sets, args := []string{}, []any{sceneID}
	if req.BasePrompt != "" {
		args = append(args, req.BasePrompt)
		sets = append(sets, "base_prompt=$"+itoa(len(args)))
	}
	if req.BaseName != "" {
		args = append(args, req.BaseName)
		sets = append(sets, "base_name=$"+itoa(len(args)))
	}
	if req.TimeOfDay != "" {
		args = append(args, req.TimeOfDay)
		sets = append(sets, "time_of_day_variant=$"+itoa(len(args)))
	}
	if req.Weather != "" {
		args = append(args, req.Weather)
		sets = append(sets, "weather=$"+itoa(len(args)))
	}
	if req.ExtraPrompt != "" {
		args = append(args, req.ExtraPrompt)
		sets = append(sets, "extra_prompt_for_variant=$"+itoa(len(args)))
	}
	if len(sets) == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "无更新字段")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(),
		"UPDATE scenes SET "+joinComma(sets)+", updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL", args...); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新场景失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 3.2.16 修改道具描述 ----

// UpdateProp PATCH /props/:prop_id。
func (s *AssetDetailService) UpdateProp(c *gin.Context, propIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	propID, projectID, err := s.repo.ResolveProp(c.Request.Context(), propIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "道具不存在")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	sets, args := []string{}, []any{propID}
	if req.Name != "" {
		args = append(args, req.Name)
		sets = append(sets, "name=$"+itoa(len(args)))
	}
	if req.Description != "" {
		args = append(args, req.Description)
		sets = append(sets, "description_prompt=$"+itoa(len(args)))
	}
	if req.Category != "" {
		args = append(args, req.Category)
		sets = append(sets, "category=$"+itoa(len(args)))
	}
	if len(sets) == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "无更新字段")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(),
		"UPDATE props SET "+joinComma(sets)+", updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL", args...); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新道具失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 3.1.3 单集详情 ----

// EpisodeDetail GET /episodes/:episode_id。
func (s *AssetDetailService) EpisodeDetail(c *gin.Context, epUUID string) {
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
	var id int64
	var title, script, step, status string
	var presetDur, actualDur, nuCount, sortOrder int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, title, COALESCE(original_script_text,''), COALESCE(current_step::text,''),
		       COALESCE(status::text,''), COALESCE(preset_total_duration_sec,0),
		       COALESCE(actual_duration_sec,0), COALESCE(narrative_unit_count,0), sort_order
		FROM episodes WHERE id=$1 AND deleted_at IS NULL`, epID).
		Scan(&id, &title, &script, &step, &status, &presetDur, &actualDur, &nuCount, &sortOrder); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在")
		return
	}
	// 单元完成进度
	var doneCount int
	_ = s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT count(*) FROM narrative_units WHERE episode_id=$1 AND status='COMPLETED' AND deleted_at IS NULL`, id).Scan(&doneCount)
	httpx.OK(c, gin.H{
		"episode_id": epUUID, "title": title, "sort_order": sortOrder,
		"original_script_text": script, "current_step": step, "status": status,
		"preset_total_duration_sec": presetDur, "actual_duration_sec": actualDur,
		"narrative_unit_count": nuCount, "completed_unit_count": doneCount,
	})
}

// ---- 3.2.1 资产概览 ----

// EpisodeAssetsOverview GET /episodes/:episode_id/assets/overview。
func (s *AssetDetailService) EpisodeAssetsOverview(c *gin.Context, epUUID string) {
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
	type cnt struct{ total, done int }
	var chars, scenes, props cnt
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*), count(*) FILTER (WHERE generation_status='SUCCESS') FROM characters WHERE origin_episode_id=$1 AND deleted_at IS NULL`, epID).Scan(&chars.total, &chars.done)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*), count(*) FILTER (WHERE generation_status='SUCCESS') FROM scenes WHERE origin_episode_id=$1 AND deleted_at IS NULL`, epID).Scan(&scenes.total, &scenes.done)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*), count(*) FILTER (WHERE generation_status='SUCCESS') FROM props WHERE origin_episode_id=$1 AND deleted_at IS NULL`, epID).Scan(&props.total, &props.done)
	_ = projectID
	httpx.OK(c, gin.H{
		"characters": gin.H{"total": chars.total, "generated": chars.done},
		"scenes":     gin.H{"total": scenes.total, "generated": scenes.done},
		"props":      gin.H{"total": props.total, "generated": props.done},
	})
}

// ---- 3.2.9 / 3.2.14 场景/道具列表 ----

// EpisodeScenes GET /episodes/:episode_id/scenes。
func (s *AssetDetailService) EpisodeScenes(c *gin.Context, epUUID string) {
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
		SELECT id, COALESCE(code,''), COALESCE(base_name,''), COALESCE(main_image_url,''),
		       COALESCE(generation_status::text,''), COALESCE(point_cost,0)
		FROM scenes WHERE origin_episode_id=$1 AND deleted_at IS NULL ORDER BY id ASC`, epID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取场景失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var code, name, img, status string
		var cost int
		if err := rows.Scan(&id, &code, &name, &img, &status, &cost); err != nil {
			continue
		}
		items = append(items, map[string]any{"scene_id": id, "code": code, "base_name": name, "main_image_url": img, "generation_status": status, "point_cost": cost})
	}
	httpx.OK(c, gin.H{"items": items})
}

// EpisodeProps GET /episodes/:episode_id/props。
func (s *AssetDetailService) EpisodeProps(c *gin.Context, epUUID string) {
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
		SELECT id, COALESCE(code,''), COALESCE(name,''), COALESCE(category,''),
		       COALESCE(main_image_url,''), COALESCE(generation_status::text,''), COALESCE(point_cost,0)
		FROM props WHERE origin_episode_id=$1 AND deleted_at IS NULL ORDER BY id ASC`, epID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取道具失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var code, name, category, img, status string
		var cost int
		if err := rows.Scan(&id, &code, &name, &category, &img, &status, &cost); err != nil {
			continue
		}
		items = append(items, map[string]any{"prop_id": id, "code": code, "name": name, "category": category, "main_image_url": img, "generation_status": status, "point_cost": cost})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 3.2.20/21/22 衍生资产 ----

// AssetDerivatives GET /assets/:asset_id/derivatives。
func (s *AssetDetailService) AssetDerivatives(c *gin.Context, assetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	assetID, projectID, err := s.resolveAsset(c, assetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, asset_uuid::text, COALESCE(type::text,''), COALESCE(sub_type,''),
		       COALESCE(name,''), COALESCE(status::text,'')
		FROM assets WHERE parent_id=$1 AND deleted_at IS NULL ORDER BY id DESC`, assetID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取衍生资产失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var auuid, atype, subtype, name, status string
		if err := rows.Scan(&id, &auuid, &atype, &subtype, &name, &status); err != nil {
			continue
		}
		items = append(items, map[string]any{"asset_id": auuid, "id": id, "type": atype, "sub_type": subtype, "name": name, "status": status})
	}
	httpx.OK(c, gin.H{"items": items})
}

// AssetDerive POST /assets/:asset_id/derive。
func (s *AssetDetailService) AssetDerive(c *gin.Context, assetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	assetID, projectID, err := s.resolveAsset(c, assetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	var req struct {
		DeriveType string `json:"derive_type" binding:"required"` // OUTFIT|POSE|VIRTUAL_BG|ANGLE
		Prompt     string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	spaceID, _ := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	taskUUID := newUUID()
	params, _ := json.Marshal(map[string]any{"parent_asset_id": assetID, "derive_type": req.DeriveType, "prompt": req.Prompt})
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generation_tasks (task_uuid, space_id, user_id, project_id, task_type,
			status, request_params_json, progress_pct, retry_count, max_retry_count)
		VALUES ($1, $2, $3, $4, 'ASSET_DERIVE', 'QUEUED', $5::jsonb, 0, 0, 3)`,
		taskUUID, spaceID, uid, projectID, string(params)); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建衍生任务失败")
		return
	}
	httpx.OK(c, gin.H{"task_id": taskUUID, "status": "QUEUED", "is_mock": true})
}

// AssetPreviewPoints POST /assets/:asset_id/preview-points。
func (s *AssetDetailService) AssetPreviewPoints(c *gin.Context, assetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	assetID, projectID, err := s.resolveAsset(c, assetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	// 复用判定 + 默认单价
	var kind string
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT COALESCE(type::text,'CHARACTER') FROM assets WHERE id=$1`, assetID).Scan(&kind)
	cost := defaultCost("CHARACTER_VARIANT")
	switch kind {
	case "SCENE":
		cost = defaultCost("SCENE")
	case "PROP":
		cost = defaultCost("PROP")
	}
	exempt := false
	var n int
	_ = s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT count(*) FROM asset_usage_logs WHERE project_id=$1 AND asset_id=$2`, projectID, assetID).Scan(&n)
	exempt = n > 0
	if exempt {
		cost = 0
	}
	httpx.OK(c, gin.H{"total_cost": cost, "actual_cost": cost, "exempt": exempt, "billing_scope": "USER"})
}

// resolveAsset 资产 id 解析（assets 表主键为数字 id，也可按 asset_uuid）。
func (s *AssetDetailService) resolveAsset(c *gin.Context, idStr string) (int64, int64, error) {
	var id, projectID int64
	// 先按数字 id，失败按 uuid
	if n, err := strconv.ParseInt(idStr, 10, 64); err == nil && n > 0 {
		if e := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id, project_id FROM assets WHERE id=$1 AND deleted_at IS NULL`, n).Scan(&id, &projectID); e == nil {
			return id, projectID, nil
		}
	}
	err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id, project_id FROM assets WHERE asset_uuid=$1::uuid AND deleted_at IS NULL`, idStr).Scan(&id, &projectID)
	return id, projectID, err
}

// itoa 快速 int → string。
func itoa(n int) string {
	return strconv.Itoa(n)
}

// joinComma 逗号连接 SQL SET 子句。
func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}
