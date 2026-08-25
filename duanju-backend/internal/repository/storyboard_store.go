// storyboard_store 实现 agent.StoryboardPlanStore：分镜重生成事务入库（18 - 视频制作 阶段二）。
// 软删旧 shots/groups → 插入新分镜 → narrative_unit_versions 新版本快照 → 单元时长回写 → 资产链重建。
package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/agent"
)

// StoryboardStore 分镜规划数据访问。
type StoryboardStore struct {
	pool *pgxpool.Pool
}

func NewStoryboardStore(pool *pgxpool.Pool) *StoryboardStore {
	return &StoryboardStore{pool: pool}
}

var _ agent.StoryboardPlanStore = (*StoryboardStore)(nil)

// LoadBeatForPlan 加载单元剧本/时长预设/关联资产名（优先 nu_asset_links，空则回退项目级资产）。
func (s *StoryboardStore) LoadBeatForPlan(ctx context.Context, narrativeUnitID int64) (*agent.BeatPlanInput, error) {
	var in agent.BeatPlanInput
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, COALESCE(script_original_text, COALESCE(content, '')), COALESCE(preset_duration_sec, 0)
		FROM narrative_units WHERE id=$1 AND deleted_at IS NULL`, narrativeUnitID).
		Scan(&in.NarrativeUnitID, &in.ProjectID, &in.ScriptOriginalText, &in.PresetDurationSec)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	// 关联资产名（角色/场景优先，供 prompt 显式引用 → 参考图命中）
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(ch.name, sc.base_name, p.name, '')
		FROM nu_asset_links nal
		LEFT JOIN characters ch ON nal.asset_kind='CHARACTER' AND nal.asset_id=ch.id
		LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND nal.asset_id=sc.id
		LEFT JOIN props p ON nal.asset_kind='PROP' AND nal.asset_id=p.id
		WHERE nal.narrative_unit_id=$1
		ORDER BY CASE nal.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END, nal.id ASC
		LIMIT 20`, narrativeUnitID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil && name != "" {
				in.AssetNames = append(in.AssetNames, name)
			}
		}
	}
	// 回退：单元无链（存量项目）时用项目级资产名，保证 prompt 仍可显式引用主体
	if len(in.AssetNames) == 0 {
		assets, aerr := loadProjectAssets(ctx, s.pool, in.ProjectID)
		if aerr == nil {
			for i, a := range assets {
				if i >= 20 {
					break
				}
				in.AssetNames = append(in.AssetNames, a.Name)
			}
		}
	}
	return &in, nil
}

// SaveStoryboardPlan 事务：RUNNING 门控 → 软删旧分镜 → 插入新分镜 → 版本快照 → 单元回写 → 资产链重建。
func (s *StoryboardStore) SaveStoryboardPlan(ctx context.Context, narrativeUnitID int64, groups []agent.ShotGroup) (*agent.SplitStats, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// RUNNING 门控（对齐 18 方案风险项）：存在生成中视频任务的单元拒绝重规划（防外键/状态残留）
	var running int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM shots
		WHERE narrative_unit_id=$1 AND deleted_at IS NULL AND video_generation_status='RUNNING'`,
		narrativeUnitID).Scan(&running); err != nil {
		return nil, err
	}
	if running > 0 {
		return nil, agent.ErrBeatBusy
	}

	stats := &agent.SplitStats{}
	totalDuration := 0
	// 软删旧分镜（保留数据可回滚）
	if _, err := tx.Exec(ctx, `UPDATE shots SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, narrativeUnitID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE shot_groups SET deleted_at=NOW() WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, narrativeUnitID); err != nil {
		return nil, err
	}
	// 插入新分镜（收集镜头文本供资产链重建）
	shotTexts := make([]ShotTextInput, 0, 16)
	unitText := ""
	for gi, sg := range groups {
		var sgID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO shot_groups (narrative_unit_id, project_id, sort_order, group_type, title, duration_sec, shot_count)
			VALUES ($1, (SELECT project_id FROM narrative_units WHERE id=$1), $2, 'NAMED_SEGMENT', $3, $4, $5) RETURNING id`,
			narrativeUnitID, gi+1, sg.Title, int(sg.DurationSec), len(sg.Shots)).Scan(&sgID); err != nil {
			return nil, err
		}
		stats.Groups++
		for si, sh := range sg.Shots {
			var shotID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO shots (shot_group_id, narrative_unit_id, project_id, sort_order, duration_sec,
					shot_size, camera_movement, camera_angle, subject_description, background_description,
					local_light_description, dialogue_text, ambient_sound, prompt_original, prompt_refined)
				VALUES ($1, $2, (SELECT project_id FROM narrative_units WHERE id=$2), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
				sgID, narrativeUnitID, si+1, int(sh.DurationSec), sh.ShotSize, sh.CameraMovement, sh.CameraAngle,
				sh.SubjectDescription, sh.BackgroundDescription, sh.LocalLightDescription,
				sh.DialogueText, sh.AmbientSound,
				sh.SubjectDescription+" "+sh.BackgroundDescription, sh.PromptRefined).Scan(&shotID); err != nil {
				return nil, err
			}
			stats.Shots++
			totalDuration += int(sh.DurationSec)
			text := ShotMatchText(sh.SubjectDescription, sh.BackgroundDescription, sh.DialogueText, sh.AmbientSound)
			shotTexts = append(shotTexts, ShotTextInput{ShotID: shotID, NarrativeUnitID: narrativeUnitID, Text: text})
			unitText += " " + text
		}
	}

	// 版本快照 + 单元时长/版本号回写（历史版本弹窗可回滚）
	sbJSON, _ := json.Marshal(groups)
	if _, err := tx.Exec(ctx, `
		INSERT INTO narrative_unit_versions (narrative_unit_id, version_num, label, script_snapshot, storyboard_json_snapshot, created_from)
		SELECT $1, COALESCE((SELECT MAX(version_num) FROM narrative_unit_versions WHERE narrative_unit_id=$1),0)+1,
		       '分镜重生成', COALESCE(nu.script_original_text,''), $2::jsonb, 'AGENT_STORYBOARD_PLAN'
		FROM narrative_units nu WHERE nu.id=$1`,
		narrativeUnitID, string(sbJSON)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE narrative_units SET actual_duration_sec=$2, version_num=version_num+1, updated_at=NOW()
		WHERE id=$1`, narrativeUnitID, totalDuration); err != nil {
		return nil, err
	}

	// 资产链重建：清旧链 → 按新镜头文本重新匹配（重生成可能引入/移除资产引用）
	var projectID int64
	if err := tx.QueryRow(ctx, `SELECT project_id FROM narrative_units WHERE id=$1`, narrativeUnitID).Scan(&projectID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM nu_asset_links WHERE narrative_unit_id=$1`, narrativeUnitID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM shot_asset_refs WHERE shot_id IN (SELECT id FROM shots WHERE narrative_unit_id=$1)`,
		narrativeUnitID); err != nil {
		return nil, err
	}
	assets, err := loadProjectAssetsTx(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if len(assets) > 0 {
		units := []UnitTextInput{{NarrativeUnitID: narrativeUnitID, Text: unitText}}
		ul, sl, lerr := LinkAssetsInTx(ctx, tx, units, shotTexts, assets)
		if lerr != nil {
			return nil, lerr
		}
		stats.AssetUnitLinks = ul
		stats.AssetShotLinks = sl
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return stats, nil
}

// loadProjectAssetsTx 事务内加载项目资产清单（同 loadProjectAssets，pgx.Tx 版本）。
func loadProjectAssetsTx(ctx context.Context, tx pgx.Tx, projectID int64) ([]AssetLinkItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT 'CHARACTER', id, name FROM characters WHERE project_id=$1 AND deleted_at IS NULL AND COALESCE(name,'') != ''
		UNION ALL
		SELECT 'SCENE', id, base_name FROM scenes WHERE project_id=$1 AND deleted_at IS NULL AND COALESCE(base_name,'') != ''
		UNION ALL
		SELECT 'PROP', id, name FROM props WHERE project_id=$1 AND deleted_at IS NULL AND COALESCE(name,'') != ''
		ORDER BY 1, 2`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AssetLinkItem, 0, 16)
	for rows.Next() {
		var a AssetLinkItem
		if err := rows.Scan(&a.Kind, &a.ID, &a.Name); err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
