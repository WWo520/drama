// split_store 实现 agent.SplitStore：四层结构事务入库 + 项目状态转移（03 A.4 实现要点）。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/agent"
)

// SplitStore 拆分结果写库。
type SplitStore struct {
	pool *pgxpool.Pool
}

func NewSplitStore(pool *pgxpool.Pool) *SplitStore {
	return &SplitStore{pool: pool}
}

var _ agent.SplitStore = (*SplitStore)(nil)

// GetProjectInfo 项目归属与剧本。
func (s *SplitStore) GetProjectInfo(ctx context.Context, projectID int64) (*agent.ProjectInfo, error) {
	var info agent.ProjectInfo
	err := s.pool.QueryRow(ctx, `
		SELECT space_id, owner_user_id, COALESCE(source_script_text, '') FROM projects WHERE id = $1`,
		projectID,
	).Scan(&info.SpaceID, &info.OwnerID, &info.ScriptText)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	return &info, err
}

// SaveSplitResult 事务：集/单元/块/镜头 四层写入 + 项目状态 SCRIPT_SPLITTING→PARTIAL_ASSETS。
// 对齐 03 A.4：同一事务内锁项目校验状态幂等 → 批量插入 → 更新状态/统计 → 提交。
func (s *SplitStore) SaveSplitResult(ctx context.Context, projectID int64, result *agent.SplitResult) (*agent.SplitStats, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 锁项目校验状态（幂等：仅 SCRIPT_SPLITTING 或 FAILED 可写入；重复回调拒绝）
	var status string
	if err := tx.QueryRow(ctx, `SELECT status::text FROM projects WHERE id = $1 FOR UPDATE`, projectID).Scan(&status); err != nil {
		return nil, err
	}
	if status != "SCRIPT_SPLITTING" && status != "SCRIPT_SPLIT_FAILED" {
		return nil, agent.ErrStateConflict
	}

	stats := &agent.SplitStats{}

	// 清空旧拆分产物（重试场景：级联删旧 episodes/units/groups/shots）
	if _, err := tx.Exec(ctx, `UPDATE shots SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE shot_groups SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE narrative_units SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE episodes SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE characters SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}

	totalDuration := 0
	for ei, ep := range result.Episodes {
		var epID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO episodes (project_id, sort_order, title, original_script_text, current_step, status, narrative_unit_count)
			VALUES ($1, $2, $3, $4, 'ASSET_GENERATION', 'PARTIAL_ASSETS', $5) RETURNING id`,
			projectID, ei+1, ep.Title, firstScript(ep), len(ep.NarrativeUnits)).Scan(&epID); err != nil {
			return nil, err
		}
		stats.Episodes++
		for bi, beat := range ep.NarrativeUnits {
			var nuID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO narrative_units (episode_id, project_id, sort_order, title, preset_duration_sec, actual_duration_sec,
					content, script_original_text, status, version_num)
				VALUES ($1, $2, $3, $4, $5, $5, $6, $7, 'SUCCESS', 1) RETURNING id`,
				epID, projectID, bi+1, beat.Title, beat.DurationSec, beat.ScriptOriginalText, beat.ScriptOriginalText).Scan(&nuID); err != nil {
				return nil, err
			}
			stats.Beats++
			for gi, sg := range beat.ShotGroups {
				var sgID int64
				if err := tx.QueryRow(ctx, `
					INSERT INTO shot_groups (narrative_unit_id, project_id, sort_order, group_type, title, duration_sec, shot_count)
					VALUES ($1, $2, $3, 'NAMED_SEGMENT', $4, $5, $6) RETURNING id`,
					nuID, projectID, gi+1, sg.Title, sg.DurationSec, len(sg.Shots)).Scan(&sgID); err != nil {
					return nil, err
				}
				stats.Groups++
				for si, sh := range sg.Shots {
					if _, err := tx.Exec(ctx, `
						INSERT INTO shots (shot_group_id, narrative_unit_id, project_id, sort_order, duration_sec,
							shot_size, camera_movement, camera_angle, subject_description, background_description,
							local_light_description, dialogue_text, ambient_sound, prompt_original, prompt_refined)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
						sgID, nuID, projectID, si+1, sh.DurationSec, sh.ShotSize, sh.CameraMovement, sh.CameraAngle,
						sh.SubjectDescription, sh.BackgroundDescription, sh.LocalLightDescription,
						sh.DialogueText, sh.AmbientSound, promptOriginal(sh), sh.PromptRefined); err != nil {
						return nil, err
					}
					stats.Shots++
					totalDuration += sh.DurationSec
				}
			}
		}
	}

	// 角色抽取入库（Step1 产出 → characters 表；origin_episode_id 为 1-based 集序号 → 映射实际集 ID）
	if len(result.Characters) > 0 {
		for _, ch := range result.Characters {
			var epID int64
			if ch.OriginEpisodeID > 0 {
				_ = tx.QueryRow(ctx, `
					SELECT id FROM episodes WHERE project_id=$1 AND sort_order=$2 AND deleted_at IS NULL`,
					projectID, ch.OriginEpisodeID).Scan(&epID)
			}
			// tags 列是 JSONB：字符串 → JSON 数组
			tagsJSON, _ := json.Marshal(splitTags(ch.Tags))
			if _, err := tx.Exec(ctx, `
				INSERT INTO characters (project_id, code, name, tags, description_notes, origin_episode_id)
				VALUES ($1, $2, $3, $4::jsonb, $5, NULLIF($6, 0))
				ON CONFLICT (project_id, code) WHERE deleted_at IS NULL DO NOTHING`,
				projectID, ch.Code, ch.Name, string(tagsJSON), ch.DescriptionNotes, epID); err != nil {
				return nil, err
			}
		}
	}

	// 项目状态转移：SCRIPT_SPLITTING → PARTIAL_ASSETS（03 A.2 拆分成功），冗余统计更新
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET status='PARTIAL_ASSETS', current_step='ASSET_GENERATION',
		       total_episode_count=$2, total_duration_sec=$3
		WHERE id=$1`,
		projectID, stats.Episodes, totalDuration); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return stats, nil
}

// splitTags 逗号分隔标签 → 字符串数组（供 JSONB 序列化）。
func splitTags(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func firstScript(ep agent.EpisodeSplit) string {
	for _, b := range ep.NarrativeUnits {
		if b.ScriptOriginalText != "" {
			return b.ScriptOriginalText
		}
	}
	return ""
}

func promptOriginal(sh agent.Shot) string {
	return sh.SubjectDescription + " " + sh.BackgroundDescription
}
