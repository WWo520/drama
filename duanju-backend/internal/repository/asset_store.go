// asset_store 实现 asset.Store（s3_object_metadata 凭证生命周期）与 Worker-Asset 结果写入。
package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/agent"
	"github.com/duanju/backend/internal/engine/asset"
)

// AssetStore 资产元数据与生成结果。
type AssetStore struct {
	pool *pgxpool.Pool
}

func NewAssetStore(pool *pgxpool.Pool) *AssetStore {
	return &AssetStore{pool: pool}
}

var _ asset.Store = (*AssetStore)(nil)

// CreatePresigned 登记预签名凭证（PRESIGNED；B3.0 唯一约束：bucket+object_key / upload_token）。
func (s *AssetStore) CreatePresigned(ctx context.Context, spaceID, userID int64, bucket, objectKey, contentType string, size int64) (uuid.UUID, error) {
	token := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO s3_object_metadata (bucket, object_key, space_id, created_by_user_id, upload_token,
			content_type, size_bytes, upload_status, delete_after)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'PRESIGNED', NOW() + interval '15 minutes')`,
		bucket, objectKey, spaceID, userID, token, contentType, size)
	if err != nil {
		return uuid.Nil, err
	}
	return token, nil
}

// ConfirmObject 回执确认：upload_token+object_key+用户 锁定单行 → CONFIRMED（B3.0 协议）。
func (s *AssetStore) ConfirmObject(ctx context.Context, token uuid.UUID, objectKey string, userID int64, etag string, size int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE s3_object_metadata
		SET upload_status='CONFIRMED', confirmed_at=NOW(), etag=$4, size_bytes=$5,
		    delete_after=NULL, ref_count=1, updated_at=NOW()
		WHERE upload_token=$1 AND object_key=$2 AND created_by_user_id=$3 AND upload_status='PRESIGNED'`,
		token, objectKey, userID, etag, size)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		// 不存在 / 已确认（幂等：重复确认返回既有结果）/ 用户不匹配 → 过期或伪造
		return asset.ErrTokenExpired
	}
	return nil
}

// ExpireStale 清理过期未确认凭证（生命周期兜底：PRESIGNED 超 15min → EXPIRED）。
func (s *AssetStore) ExpireStale(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE s3_object_metadata SET upload_status='EXPIRED', updated_at=NOW()
		WHERE upload_status='PRESIGNED' AND delete_after < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ---- Worker-Asset 结果写入 ----

// AssetTarget 资产生成目标（对齐 agent.AssetTarget）。
type AssetTarget = agent.AssetTarget

// SaveGeneratedImage 记录生成图片（generated_images）+ 更新业务表 main_image_url + asset_usage_logs。
// 返回生成图片 URL。
func (s *AssetStore) SaveGeneratedImage(ctx context.Context, t *AssetTarget, imageURL string, taskID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// generated_images 记录（含弹窗生成参数快照，供历史面板展示）
	var giID int64
	gen := t.Gen
	promptSnap, modelCode, styleName, aspectRatio, timeOfDay := "", "", "", "", ""
	// 与 asset_handler 默认构图逻辑一致：无弹窗参数（批量生成）时角色默认开启三视图
	useDefaultComp := t.Kind == "CHARACTER_VARIANT"
	if gen != nil {
		promptSnap, modelCode, styleName, aspectRatio, timeOfDay = gen.Prompt, gen.ModelCode, gen.StyleName, gen.AspectRatio, gen.TimeOfDay
		useDefaultComp = gen.UseDefaultComposition
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO generated_images (asset_id, state, original_url, generation_task_id, prompt,
			target_kind, target_id, model_code, style_name, aspect_ratio, use_default_composition, time_of_day)
		VALUES ($1, 'SUCCESS', $2, $3, NULLIF($4,''), $5, $6, NULLIF($7,''), NULLIF($8,''), NULLIF($9,''), $10, NULLIF($11,'')) RETURNING id`,
		nullableAssetID(t.AssetID), imageURL, taskID, promptSnap,
		t.Kind, t.TargetID, modelCode, styleName, aspectRatio, useDefaultComp, timeOfDay).Scan(&giID)
	if err != nil {
		return err
	}

	// 按类型更新业务表
	switch t.Kind {
	case "CHARACTER_VARIANT":
		// target 双语义：弹窗选中形象/衍生形象任务传 variant.id；角色级生成传 character.id。
		// ① 先按 variant.id 精确更新（只写该形象）——此前 UPDATE 仅按 character_id 匹配，
		//    variant.id 零命中导致提取形象的生成图直接丢失（实测 character 163 八个形象全无图）。
		tag, err := tx.Exec(ctx, `
			UPDATE character_variants SET generation_status='SUCCESS', main_image_url=$1, point_cost=$2, generation_task_id=$3
			WHERE id=$4 AND deleted_at IS NULL`,
			imageURL, t.PointCost, taskID, t.TargetID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// ② target=character.id：只更新默认形象（最小 id 变体）——
			//    此前无 WHERE 限制全量 UPDATE，曾把该角色全部形象覆盖成同一张图（形象切换看不到差异）。
			tag, err = tx.Exec(ctx, `
				UPDATE character_variants SET generation_status='SUCCESS', main_image_url=$1, point_cost=$2, generation_task_id=$3
				WHERE id = (SELECT cv.id FROM character_variants cv WHERE cv.character_id=$4 AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1)`,
				imageURL, t.PointCost, taskID, t.TargetID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				// ③ 首次生成：角色尚无任何变体 → INSERT 创建默认形象
				if _, err := tx.Exec(ctx, `
					INSERT INTO character_variants (character_id, variant_name, generation_status, main_image_url, point_cost, generation_task_id)
					SELECT $4, COALESCE((SELECT name FROM characters WHERE id=$4), '默认形象'), 'SUCCESS', $1, $2, $3
					WHERE NOT EXISTS (SELECT 1 FROM character_variants WHERE character_id=$4 AND deleted_at IS NULL)`,
					imageURL, t.PointCost, taskID, t.TargetID); err != nil {
					return err
				}
			}
		}
	case "SCENE":
		var todVal, weatherVal any
		if gen != nil {
			if gen.TimeOfDay != "" {
				todVal = gen.TimeOfDay
			}
			if gen.Weather != "" {
				weatherVal = gen.Weather
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE scenes SET generation_status='SUCCESS', main_image_url=$1, point_cost=$2, generation_task_id=$3,
			       time_of_day_variant=COALESCE($5::time_of_day_enum, time_of_day_variant),
			       weather=COALESCE($6::weather_enum, weather)
			WHERE id=$4`,
			imageURL, t.PointCost, taskID, t.TargetID, todVal, weatherVal); err != nil {
			return err
		}
	case "PROP":
		if _, err := tx.Exec(ctx, `
			UPDATE props SET generation_status='SUCCESS', main_image_url=$1, point_cost=$2, generation_task_id=$3
			WHERE id=$4`,
			imageURL, t.PointCost, taskID, t.TargetID); err != nil {
			return err
		}
	}

	// asset_usage_logs 跨集复用判定（B3.10：同资产同集只记 1 次）
	_, _ = tx.Exec(ctx, `
		INSERT INTO asset_usage_logs (asset_kind, asset_id, project_id, episode_id, is_first_time_use_project_wide,
			is_first_time_use_episode_wide, point_cost_applied, generation_task_id, user_notified)
		SELECT $1, $2, $3, $4,
		       NOT EXISTS (SELECT 1 FROM asset_usage_logs WHERE asset_kind=$1 AND asset_id=$2 AND project_id=$3),
		       NOT EXISTS (SELECT 1 FROM asset_usage_logs WHERE asset_kind=$1 AND asset_id=$2 AND episode_id=$4),
		       $5, $6, FALSE
		WHERE NOT EXISTS (SELECT 1 FROM asset_usage_logs WHERE asset_kind=$1 AND asset_id=$2 AND episode_id=$4)
		ON CONFLICT (asset_kind, asset_id, episode_id) DO NOTHING`,
		t.Kind, t.TargetID, t.ProjectID, t.EpisodeID, t.PointCost, taskID)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// 项目状态推进：全部资产就绪 PARTIAL_ASSETS → ALL_ASSETS_READY（03 A.2，幂等；失败不阻断任务，下次生成重试）
	_ = s.CheckAssetsReady(ctx, t.ProjectID)
	return nil
}

// CheckAssetsReady 项目资产就绪检查：每个角色至少一个 SUCCESS variant，
// 且全部场景/道具均为 SUCCESS（无则自然满足）→ 推进 ALL_ASSETS_READY。
func (s *AssetStore) CheckAssetsReady(ctx context.Context, projectID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE projects SET status='ALL_ASSETS_READY', current_step='VIDEO_PRODUCTION', updated_at=NOW()
		WHERE id=$1 AND status='PARTIAL_ASSETS'
		  AND NOT EXISTS (
		      SELECT 1 FROM characters c WHERE c.project_id=$1 AND c.deleted_at IS NULL
		        AND NOT EXISTS (SELECT 1 FROM character_variants cv
		                        WHERE cv.character_id=c.id AND cv.deleted_at IS NULL AND cv.generation_status='SUCCESS'))
		  AND NOT EXISTS (SELECT 1 FROM scenes WHERE project_id=$1 AND deleted_at IS NULL AND generation_status <> 'SUCCESS')
		  AND NOT EXISTS (SELECT 1 FROM props  WHERE project_id=$1 AND deleted_at IS NULL AND generation_status <> 'SUCCESS')`,
		projectID)
	return err
}

// LoadAssetTarget 按任务参数解析目标（target_kind/target_id）。
func (s *AssetStore) LoadAssetTarget(ctx context.Context, targetKind string, targetID int64) (*agent.AssetTarget, error) {
	t := &agent.AssetTarget{Kind: targetKind, TargetID: targetID}
	switch targetKind {
	case "CHARACTER_VARIANT":
		// 前端传 character.id（首次生成时 variant 尚不存在）→ 查 characters 行；
		// 若已是衍生图重生成（传 variant.id），则回退查 character_variants。
		err := s.pool.QueryRow(ctx, `
			SELECT 0, c.project_id, c.origin_episode_id, c.asset_id,
			       COALESCE(c.edited_prompt, c.description_notes, '')
			FROM characters c WHERE c.id = $1 AND c.deleted_at IS NULL`, targetID).
			Scan(&t.PointCost, &t.ProjectID, &t.EpisodeID, &t.AssetID, &t.Prompt)
		if errors.Is(err, pgx.ErrNoRows) {
			// 回退：按 variant.id 查——提示词 = 角色基线 + 变体服装/外观描述
			// （变体的 clothing_desc/extra_appearance_prompt 必须进提示词，否则所有形象出图相同）
			err2 := s.pool.QueryRow(ctx, `
				SELECT cv.point_cost, c.project_id, c.origin_episode_id, cv.asset_id,
				       concat_ws('，',
				           NULLIF(COALESCE(c.edited_prompt, c.description_notes, ''), ''),
				           NULLIF(COALESCE(cv.clothing_desc, ''), ''),
				           NULLIF(COALESCE(cv.extra_appearance_prompt, ''), ''))
				FROM character_variants cv JOIN characters c ON c.id = cv.character_id
				WHERE cv.id = $1`, targetID).
				Scan(&t.PointCost, &t.ProjectID, &t.EpisodeID, &t.AssetID, &t.Prompt)
			if errors.Is(err2, pgx.ErrNoRows) {
				return nil, asset.ErrTokenExpired
			}
			return t, err2
		}
		return t, err
	case "SCENE":
		// 时段/天气翻译移至 asset_handler（Go 侧完整枚举映射）——
		// 原 SQL CASE 用旧枚举字面量（DUSK/NIGHT/SUNNY/RAINY/SNOWY），PostgreSQL 会把
		// WHEN 字面量 cast 为 time_of_day_enum/weather_enum，非法值直接 22P02 报错
		// （曾致 SCENE_IMAGE_GEN 全部失败）；此处仅返回原始枚举值。
		err := s.pool.QueryRow(ctx, `
SELECT point_cost, project_id, origin_episode_id, asset_id,
       COALESCE(edited_prompt, COALESCE(base_prompt,'')),
       COALESCE(time_of_day_variant::text,''), COALESCE(weather::text,'')
FROM scenes WHERE id = $1`,
			targetID).Scan(&t.PointCost, &t.ProjectID, &t.EpisodeID, &t.AssetID, &t.Prompt, &t.TimeOfDay, &t.Weather)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, asset.ErrTokenExpired
		}
		return t, err
	case "PROP":
		err := s.pool.QueryRow(ctx, `
			SELECT point_cost, project_id, origin_episode_id, asset_id, COALESCE(edited_prompt, description_prompt, '')
			FROM props WHERE id = $1`,
			targetID).Scan(&t.PointCost, &t.ProjectID, &t.EpisodeID, &t.AssetID, &t.Prompt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, asset.ErrTokenExpired
		}
		return t, err
	}
	return nil, errors.New("unknown asset kind: " + targetKind)
}

// IsAssetUsedBefore 跨集复用判定（S2-B06）：同项目该资产已生成过 → 本次免扣费。
func (s *AssetStore) IsAssetUsedBefore(ctx context.Context, projectID int64, kind string, targetID int64) (bool, error) {
	var one int
	err := s.pool.QueryRow(ctx, `
		SELECT 1 FROM asset_usage_logs
		WHERE asset_kind=$1 AND asset_id=$2 AND project_id=$3 LIMIT 1`,
		kind, targetID, projectID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ListVariants 角色变体列表（3.2.7）。
func (s *AssetStore) ListVariants(ctx context.Context, characterID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, variant_name, variant_type::text, COALESCE(main_image_url,''), generation_status::text, point_cost
		FROM character_variants WHERE character_id=$1 AND deleted_at IS NULL ORDER BY id`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, cost int64
		var name, vtype, url, status string
		rows.Scan(&id, &name, &vtype, &url, &status, &cost)
		out = append(out, map[string]any{
			"variant_id": id, "variant_name": name, "variant_type": vtype,
			"main_image_url": url, "generation_status": status, "point_cost": cost,
		})
	}
	return out, rows.Err()
}

// SaveVersionSnapshot 拆分成功时写叙事单元版本快照（S2-B05）。
func (s *AssetStore) SaveVersionSnapshot(ctx context.Context, narrativeUnitID, userID int64, script, storyboard string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO narrative_unit_versions (narrative_unit_id, version_num, label, script_snapshot, storyboard_json_snapshot, created_by_user_id, created_from)
		SELECT $1, COALESCE(MAX(version_num),0)+1, '自动快照', $3, $4, $2, 'AGENT_AUTO_SPLIT'
		FROM narrative_unit_versions WHERE narrative_unit_id=$1`,
		narrativeUnitID, userID, script, storyboard)
	return err
}

// ListVersions 版本历史（3.1.5a）。
func (s *AssetStore) ListVersions(ctx context.Context, narrativeUnitID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, version_num, label, created_from::text,
		       to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM narrative_unit_versions WHERE narrative_unit_id=$1 ORDER BY version_num DESC LIMIT 20`, narrativeUnitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, ver int64
		var label, from, created string
		rows.Scan(&id, &ver, &label, &from, &created)
		out = append(out, map[string]any{"version_id": id, "version_num": ver, "label": label, "created_from": from, "created_at": created})
	}
	return out, rows.Err()
}

// RollbackVersion 回滚到指定版本（S2-B05）：恢复 narrative_units 脚本/内容字段；
// 18 - 视频制作 阶段二：同时从 storyboard_json_snapshot 恢复该版本的 shots/shot_groups。
func (s *AssetStore) RollbackVersion(ctx context.Context, narrativeUnitID, versionID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE narrative_units nu SET content=v.script_snapshot, script_original_text=v.script_snapshot, version_num=v.version_num
		FROM narrative_unit_versions v
		WHERE v.id=$2 AND v.narrative_unit_id=$1 AND nu.id=$1`, narrativeUnitID, versionID); err != nil {
		return err
	}
	// 恢复该版本分镜：软删当前 shots/groups → 快照 JSON 重建（快照为 []agent.ShotGroup 序列化）
	var sbJSON string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(storyboard_json_snapshot::text, '') FROM narrative_unit_versions
		WHERE id=$1 AND narrative_unit_id=$2`, versionID, narrativeUnitID).Scan(&sbJSON); err == nil && sbJSON != "" && sbJSON != "null" {
		var groups []agent.ShotGroup
		if jerr := json.Unmarshal([]byte(sbJSON), &groups); jerr == nil && len(groups) > 0 {
			// 解绑并作废旧成片卡（回滚后 shots 全新，旧卡镜头归属失效）
			if _, derr := tx.Exec(ctx, `UPDATE shots SET video_card_id=NULL WHERE narrative_unit_id=$1`, narrativeUnitID); derr != nil {
				return derr
			}
			if _, derr := tx.Exec(ctx, `UPDATE video_cards SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, narrativeUnitID); derr != nil {
				return derr
			}
			if _, derr := tx.Exec(ctx, `UPDATE shots SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, narrativeUnitID); derr != nil {
				return derr
			}
			if _, derr := tx.Exec(ctx, `UPDATE shot_groups SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, narrativeUnitID); derr != nil {
				return derr
			}
			totalDur := 0
			shotGlobal := 0 // 单元内全局递增编号（跨块连续，避免按 sort_order 排序时交错重复观感）
			for gi, sg := range groups {
				var sgID int64
				if qerr := tx.QueryRow(ctx, `
				INSERT INTO shot_groups (narrative_unit_id, project_id, sort_order, group_type, title, duration_sec, shot_count)
				VALUES ($1, (SELECT project_id FROM narrative_units WHERE id=$1), $2, 'NAMED_SEGMENT', $3, $4, $5) RETURNING id`,
					narrativeUnitID, gi+1, sg.Title, int(sg.DurationSec), len(sg.Shots)).Scan(&sgID); qerr != nil {
					return qerr
				}
				for _, sh := range sg.Shots {
					shotGlobal++
					if _, ierr := tx.Exec(ctx, `
					INSERT INTO shots (shot_group_id, narrative_unit_id, project_id, sort_order, duration_sec,
						shot_size, camera_movement, camera_angle, subject_description, background_description,
						local_light_description, dialogue_text, ambient_sound, prompt_original, prompt_refined)
					VALUES ($1, $2, (SELECT project_id FROM narrative_units WHERE id=$2), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
						sgID, narrativeUnitID, shotGlobal, int(sh.DurationSec), sh.ShotSize, sh.CameraMovement, sh.CameraAngle,
						sh.SubjectDescription, sh.BackgroundDescription, sh.LocalLightDescription,
						sh.DialogueText, sh.AmbientSound,
						sh.SubjectDescription+" "+sh.BackgroundDescription, sh.PromptRefined); ierr != nil {
						return ierr
					}
					totalDur += int(sh.DurationSec)
				}
			}
			if _, uerr := tx.Exec(ctx, `UPDATE narrative_units SET actual_duration_sec=$2 WHERE id=$1`,
				narrativeUnitID, totalDur); uerr != nil {
				return uerr
			}
		}
	}
	// 状态机：回滚后保持 PARTIAL_ASSETS（可重新规划）
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET status='PARTIAL_ASSETS', current_step='ASSET_GENERATION'
		WHERE id=(SELECT project_id FROM narrative_units WHERE id=$1) AND status IN ('ALL_VIDEOS_READY','VIDEO_GENERATING','PARTIAL_VIDEOS')`,
		narrativeUnitID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateVariant 创建角色变体（3.2.6）：插入 PENDING 行 + 资产生成任务。
func (s *AssetStore) CreateVariant(ctx context.Context, projectID, characterID, userID int64, variantName, variantType, clothingDesc, extraAppearancePrompt string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var vid int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO character_variants (character_id, variant_name, variant_type, clothing_desc, extra_appearance_prompt, generation_status, point_cost)
		VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), 'QUEUED', 10) RETURNING id`,
		characterID, variantName, variantType, clothingDesc, extraAppearancePrompt).Scan(&vid); err != nil {
		return 0, err
	}
	// 生成任务 + Outbox
	params, _ := json.Marshal(map[string]any{"target_kind": "CHARACTER_VARIANT", "target_id": vid, "cost": 10})
	var taskID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, priority, request_params_json)
		VALUES ('variant-'||gen_random_uuid(), 'variant-'||gen_random_uuid(),
		        (SELECT space_id FROM projects WHERE id=$1), $2, $1, 'CHAR_IMAGE_GEN', 'QUEUED', 5, $3::jsonb)
		RETURNING id`,
		projectID, userID, string(params)).Scan(&taskID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:variant', 'asset',
		        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
		taskID); err != nil {
		return 0, err
	}
	return vid, tx.Commit(ctx)
}

var kindToTaskType = map[string]string{
	"CHARACTER_VARIANT": "CHAR_IMAGE_GEN",
	"SCENE":             "SCENE_IMAGE_GEN",
	"PROP":              "PROP_IMAGE_GEN",
}

// CreateAssetTask 创建资产生成任务（QUEUED + Outbox 同一事务；任务参数带 target_kind/target_id）。
func (s *AssetStore) CreateAssetTask(ctx context.Context, spaceID, userID, projectID, episodeID int64, kind string, targetID int64, cost int) (int64, error) {
	return s.CreateAssetTaskWithGen(ctx, spaceID, userID, projectID, episodeID, kind, targetID, cost, nil)
}

// CreateAssetTaskWithGen 建资产生成任务（可携带弹窗生成参数 gen）。
func (s *AssetStore) CreateAssetTaskWithGen(ctx context.Context, spaceID, userID, projectID, episodeID int64, kind string, targetID int64, cost int, gen *agent.GenParams) (int64, error) {
	taskType, ok := kindToTaskType[kind]
	if !ok {
		return 0, errors.New("unknown asset kind: " + kind)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// 参数 JSON 在 Go 端序列化（避免 pgx 枚举/text 推断歧义）
	paramsMap := map[string]any{"target_kind": kind, "target_id": targetID, "cost": cost}
	if gen != nil {
		paramsMap["gen"] = gen
	}
	params, _ := json.Marshal(paramsMap)

	var taskID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, episode_id,
			task_type, status, priority, request_params_json)
		VALUES ('asset-'||gen_random_uuid(), 'asset-'||gen_random_uuid(), $1, $2, $3, $4, $5, 'QUEUED', 5, $6::jsonb)
		RETURNING id`,
		spaceID, userID, projectID, episodeID, taskType, string(params)).Scan(&taskID)
	if err != nil {
		return 0, err
	}
	// Outbox 可靠投递（B4.1a 同一事务）
	_, err = tx.Exec(ctx, `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:asset-'||floor(random()*1000000)::int, 'asset',
		        jsonb_build_object('task_id', $1::int))`,
		taskID)
	if err != nil {
		return 0, err
	}
	// 提交即回写资产表 QUEUED（前端进度条/轮询依赖；对齐视频任务预置 RUNNING 的做法）。
	// 幂等：per_asset_count>1 多任务只置一次；重试任务不覆盖进行中状态。
	if err := markAssetQueuedTx(ctx, tx, kind, targetID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return taskID, nil
}

// markAssetQueuedTx 资产表置 QUEUED（事务内）：角色 upsert 默认变体行，场景/道具直接更新。
func markAssetQueuedTx(ctx context.Context, tx pgx.Tx, kind string, targetID int64) error {
	switch kind {
	case "CHARACTER_VARIANT":
		// 首次生成无变体行 → INSERT 默认形象（QUEUED）；已有行且非进行中 → 置 QUEUED
		if _, err := tx.Exec(ctx, `
			INSERT INTO character_variants (character_id, variant_name, generation_status)
			SELECT $1, COALESCE((SELECT name FROM characters WHERE id=$1), '默认形象'), 'QUEUED'
			WHERE NOT EXISTS (SELECT 1 FROM character_variants WHERE character_id=$1 AND deleted_at IS NULL)`,
			targetID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE character_variants SET generation_status='QUEUED'
			WHERE character_id=$1 AND deleted_at IS NULL AND generation_status NOT IN ('QUEUED','RUNNING')`,
			targetID)
		return err
	case "SCENE":
		_, err := tx.Exec(ctx, `
			UPDATE scenes SET generation_status='QUEUED'
			WHERE id=$1 AND deleted_at IS NULL AND generation_status NOT IN ('QUEUED','RUNNING')`, targetID)
		return err
	case "PROP":
		_, err := tx.Exec(ctx, `
			UPDATE props SET generation_status='QUEUED'
			WHERE id=$1 AND deleted_at IS NULL AND generation_status NOT IN ('QUEUED','RUNNING')`, targetID)
		return err
	}
	return nil
}

// MarkAssetRunning 资产表置 RUNNING（Worker 认领执行时；前端进度条"生成中"）。
// 幂等：仅 QUEUED/PENDING 可置（兼容存量任务：创建时未回写 QUEUED 的旧数据）。
// CHARACTER_VARIANT 兼容两种 target：批量生成传 character_id，CreateVariant 传 variant id。
func (s *AssetStore) MarkAssetRunning(ctx context.Context, targetKind string, targetID int64) error {
	switch targetKind {
	case "CHARACTER_VARIANT":
		_, err := s.pool.Exec(ctx, `
			UPDATE character_variants SET generation_status='RUNNING'
			WHERE (character_id=$1 OR id=$1) AND deleted_at IS NULL AND generation_status IN ('QUEUED','PENDING')`, targetID)
		return err
	case "SCENE":
		_, err := s.pool.Exec(ctx, `
			UPDATE scenes SET generation_status='RUNNING'
			WHERE id=$1 AND deleted_at IS NULL AND generation_status IN ('QUEUED','PENDING')`, targetID)
		return err
	case "PROP":
		_, err := s.pool.Exec(ctx, `
			UPDATE props SET generation_status='RUNNING'
			WHERE id=$1 AND deleted_at IS NULL AND generation_status IN ('QUEUED','PENDING')`, targetID)
		return err
	}
	return nil
}

// MarkAssetGenFailed 资产表置 FAILED_DEAD（仅 QUEUED/RUNNING 可置；计费冻结失败等即时失败回滚）。
func (s *AssetStore) MarkAssetGenFailed(ctx context.Context, targetKind string, targetID int64) error {
	return s.markAssetFailed(ctx, targetKind, targetID)
}

// MarkAssetDeadByTask 任务死信后回写资产表 FAILED_DEAD（防前端卡"生成中"、按钮死锁）。
// 仅处理三类图生成任务；只覆盖仍处于 QUEUED/RUNNING 的行（不覆盖后续成功的终态）。
func (s *AssetStore) MarkAssetDeadByTask(ctx context.Context, taskID int64) error {
	var taskType string
	var paramsJSON []byte
	if err := s.pool.QueryRow(ctx, `
		SELECT task_type::text, request_params_json FROM generation_tasks WHERE id=$1`, taskID).
		Scan(&taskType, &paramsJSON); err != nil {
		return err
	}
	switch taskType {
	case "CHAR_IMAGE_GEN", "SCENE_IMAGE_GEN", "PROP_IMAGE_GEN":
	default:
		return nil
	}
	var params struct {
		TargetKind string `json:"target_kind"`
		TargetID   int64  `json:"target_id"`
	}
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		return err
	}
	if params.TargetID == 0 {
		return nil
	}
	return s.markAssetFailed(ctx, params.TargetKind, params.TargetID)
}

// markAssetFailed 资产表 QUEUED/RUNNING → FAILED_DEAD（进度条消失、按钮恢复）。
// CHARACTER_VARIANT 兼容 character_id（批量）与 variant id（CreateVariant）两种 target。
func (s *AssetStore) markAssetFailed(ctx context.Context, targetKind string, targetID int64) error {
	switch targetKind {
	case "CHARACTER_VARIANT":
		_, err := s.pool.Exec(ctx, `
			UPDATE character_variants SET generation_status='FAILED_DEAD'
			WHERE (character_id=$1 OR id=$1) AND deleted_at IS NULL AND generation_status IN ('QUEUED','RUNNING')`, targetID)
		return err
	case "SCENE":
		_, err := s.pool.Exec(ctx, `
			UPDATE scenes SET generation_status='FAILED_DEAD'
			WHERE id=$1 AND deleted_at IS NULL AND generation_status IN ('QUEUED','RUNNING')`, targetID)
		return err
	case "PROP":
		_, err := s.pool.Exec(ctx, `
			UPDATE props SET generation_status='FAILED_DEAD'
			WHERE id=$1 AND deleted_at IS NULL AND generation_status IN ('QUEUED','RUNNING')`, targetID)
		return err
	}
	return nil
}

// nullableAssetID *int64 → any（pgx 可空参数）。
func nullableAssetID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

// ListCharacters 角色列表（3.2.2）：项目内角色 + 默认变体缩略图。
func (s *AssetStore) ListCharacters(ctx context.Context, projectID int64) ([]map[string]any, error) {
	// 聚合该角色全部形象变体（id/name/main_image_url/generation_status），供前端形象名条切换。
	// 主图/状态取第一个变体（与历史口径一致）。跨集复用 assets.items 前缀与前端 CharacterItem 对齐。
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.code, c.name, c.tags, c.description_notes,
		       COALESCE((SELECT cv.main_image_url FROM character_variants cv WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1), ''),
		       COALESCE((SELECT cv.generation_status::text FROM character_variants cv WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1), 'PENDING'),
		       c.origin_episode_id,
		       COALESCE((SELECT json_agg(json_build_object(
		           'id', cv.id,
		           'name', COALESCE(cv.variant_name,''),
		           'main_image_url', COALESCE(cv.main_image_url,''),
		           'generation_status', COALESCE(cv.generation_status::text,'PENDING')
		       ) ORDER BY cv.id) FROM character_variants cv WHERE cv.character_id=c.id AND cv.deleted_at IS NULL), '[]'::json)
		FROM characters c WHERE c.project_id=$1 AND c.deleted_at IS NULL ORDER BY c.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var code, name, tags, notes, thumb, status string
		var originEp *int64
		var variantsJson []byte
		if err := rows.Scan(&id, &code, &name, &tags, &notes, &thumb, &status, &originEp, &variantsJson); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":                id,
			"code":              code,
			"name":              name,
			"tags":              tags,
			"description_notes": notes,
			"main_image_url":    thumb,
			"generation_status": status,
			"origin_episode_id": originEp,
			"variants":          json.RawMessage(variantsJson),
		})
	}
	return out, rows.Err()
}
