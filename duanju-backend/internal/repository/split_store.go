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

// MarkSplitFailed 拆分失败回滚：项目 SCRIPT_SPLITTING → SCRIPT_SPLIT_FAILED（幂等，前端可重新拆分）。
func (s *SplitStore) MarkSplitFailed(ctx context.Context, projectID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE projects SET status='SCRIPT_SPLIT_FAILED', updated_at=NOW()
		WHERE id=$1 AND status='SCRIPT_SPLITTING' AND deleted_at IS NULL`, projectID)
	return err
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
	if _, err := tx.Exec(ctx, `UPDATE scenes SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE props SET deleted_at=NOW() WHERE project_id=$1`, projectID); err != nil {
		return nil, err
	}

	totalDuration := 0
	// 资产建链收集器（U5/F10：单元文本 + 镜头文本，资产入库后统一匹配写链）
	unitTexts := make([]UnitTextInput, 0, 16)
	shotTexts := make([]ShotTextInput, 0, 64)
	for ei, ep := range result.Episodes {
		var epID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO episodes (project_id, sort_order, title, original_script_text, current_step, status, narrative_unit_count)
			VALUES ($1, $2, $3, $4, 'ASSET_GENERATION', 'PARTIAL_ASSETS', $5) RETURNING id`,
			projectID, ei+1, ep.Title, episodeScript(ep), len(ep.NarrativeUnits)).Scan(&epID); err != nil {
			return nil, err
		}
		stats.Episodes++
		for bi, beat := range ep.NarrativeUnits {
			var nuID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO narrative_units (episode_id, project_id, sort_order, title, preset_duration_sec, actual_duration_sec,
					content, script_original_text, status, version_num)
				VALUES ($1, $2, $3, $4, $5, $5, $6, $7, 'SUCCESS', 1) RETURNING id`,
				epID, projectID, bi+1, beat.Title, int(beat.DurationSec), beat.ScriptOriginalText, beat.ScriptOriginalText).Scan(&nuID); err != nil {
				return nil, err
			}
			stats.Beats++
			// 版本 v1 快照（S2-B05）：拆分成功即写 narrative_unit_versions，供 Step3 历史版本弹窗使用。
			// 分镜快照存空数组：拆分不直出分镜（回滚 v1 仅恢复剧本原文，不恢复分镜）
			sbJSON, _ := json.Marshal([]agent.ShotGroup{})
			if _, err := tx.Exec(ctx, `
				INSERT INTO narrative_unit_versions (narrative_unit_id, version_num, label, script_snapshot, storyboard_json_snapshot, created_from)
				VALUES ($1, 1, '拆分快照', $2, $3::jsonb, 'AGENT_AUTO_SPLIT')`,
				nuID, beat.ScriptOriginalText, string(sbJSON)); err != nil {
				return nil, err
			}
			// 单元文本收集（原文，供资产建链匹配）
			unitTexts = append(unitTexts, UnitTextInput{NarrativeUnitID: nuID, Text: beat.ScriptOriginalText})
			// 拆分不再直出分镜（对齐 LuxReal 手动生成流程）：拆分只产出 集→单元 两层 + 资产；
			// 分镜脚本由用户在 Step3 逐单元手动点「生成分镜脚本」（STORYBOARD_PLAN 任务）产出，
			// 生成时按单元剧本 + 关联资产重新规划并重建资产链。LLM 产出的 ShotGroups 仅留档于
			// v1 版本快照（storyboard_json_snapshot），不再写 shots/shot_groups 表。
			totalDuration += int(beat.DurationSec)
		}
	}

	// 集级资产抽取入库（Step1 产出 → characters/scenes/props 表；origin_episode_id 为 1-based 集序号 → 映射实际集 ID）
	resolveEp := func(sortOrder int) int64 {
		if sortOrder <= 0 {
			return 0
		}
		var epID int64
		_ = tx.QueryRow(ctx, `
			SELECT id FROM episodes WHERE project_id=$1 AND sort_order=$2 AND deleted_at IS NULL`,
			projectID, sortOrder).Scan(&epID)
		return epID
	}
	// 资产清单（含 DB ID，供资产建链匹配）
	linkAssets := make([]AssetLinkItem, 0, len(result.Characters)+len(result.Scenes)+len(result.Props))
	if len(result.Characters) > 0 {
		for _, ch := range result.Characters {
			epID := resolveEp(ch.OriginEpisodeID)
			// tags 列是 JSONB：字符串 → JSON 数组
			tagsJSON, _ := json.Marshal(splitTags(ch.Tags))
			var charID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO characters (project_id, code, name, tags, description_notes, origin_episode_id)
				VALUES ($1, $2, $3, $4::jsonb, $5, NULLIF($6, 0))
				ON CONFLICT (project_id, code) WHERE deleted_at IS NULL DO UPDATE SET updated_at=NOW() RETURNING id`,
				projectID, ch.Code, ch.Name, string(tagsJSON), ch.DescriptionNotes, epID).Scan(&charID); err != nil {
				return nil, err
			}
			stats.Characters++
			linkAssets = append(linkAssets, AssetLinkItem{Kind: "CHARACTER", ID: charID, Name: ch.Name})
		}
	}
	if len(result.Scenes) > 0 {
		for _, sc := range result.Scenes {
			epID := resolveEp(sc.OriginEpisodeID)
			var sceneID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO scenes (project_id, code, base_name, base_prompt, origin_episode_id)
				VALUES ($1, $2, $3, $4, NULLIF($5, 0))
				ON CONFLICT (project_id, code) WHERE deleted_at IS NULL DO UPDATE SET updated_at=NOW() RETURNING id`,
				projectID, sc.Code, sc.BaseName, sc.BasePrompt, epID).Scan(&sceneID); err != nil {
				return nil, err
			}
			stats.Scenes++
			linkAssets = append(linkAssets, AssetLinkItem{Kind: "SCENE", ID: sceneID, Name: sc.BaseName})
		}
	}
	if len(result.Props) > 0 {
		for _, p := range result.Props {
			epID := resolveEp(p.OriginEpisodeID)
			var propID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO props (project_id, code, name, category, description_prompt, origin_episode_id)
				VALUES ($1, $2, $3, $4::prop_category_enum, $5, NULLIF($6, 0))
				ON CONFLICT (project_id, code) WHERE deleted_at IS NULL DO UPDATE SET updated_at=NOW() RETURNING id`,
				projectID, p.Code, p.Name, sanitizePropCategory(p.Category), p.Description, epID).Scan(&propID); err != nil {
				return nil, err
			}
			stats.Props++
			linkAssets = append(linkAssets, AssetLinkItem{Kind: "PROP", ID: propID, Name: p.Name})
		}
	}

	// 资产自动建链（U5/F10，18 - 视频制作 阶段一）：单元/镜头文本实体匹配 →
	// nu_asset_links + shot_asset_refs + shots.linked_assets_json，供视频生成注入参考图。
	if len(linkAssets) > 0 {
		unitLinks, shotLinks, _, lerr := LinkAssetsInTx(ctx, tx, unitTexts, shotTexts, linkAssets)
		if lerr != nil {
			return nil, lerr
		}
		stats.AssetUnitLinks = unitLinks
		stats.AssetShotLinks = shotLinks
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

// propCategories prop_category_enum 合法集（000002 枚举真源）。
var propCategories = map[string]bool{
	"WEAPON": true, "CLOTHING_ACCESSORY": true, "FOOD_DRINK": true, "STATIONERY_BOOK": true,
	"JEWELRY_ORNAMENT": true, "FURNITURE_DECOR": true, "ELECTRONIC_APPLIANCE": true, "VEHICLE": true,
	"TOOL_MUSICAL_INSTRUMENT": true, "TOY": true, "PLANT_FLOWER": true, "ANIMAL_PET": true,
	"LIGHTING_SOURCE": true, "DOCUMENT_SCROLL_MAP": true, "FLAG_BANNER": true, "OTHER": true,
}

// sanitizePropCategory 非法/缺失类别归一为 OTHER（避免枚举插入报错）。
func sanitizePropCategory(cat string) string {
	if propCategories[cat] {
		return cat
	}
	return "OTHER"
}

// episodeScript 集级剧本原文：拼接该集全部叙事单元的原文片段（保持全集原文完整展示）。
func episodeScript(ep agent.EpisodeSplit) string {
	parts := make([]string, 0, len(ep.NarrativeUnits))
	for _, b := range ep.NarrativeUnits {
		if t := strings.TrimSpace(b.ScriptOriginalText); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func promptOriginal(sh agent.Shot) string {
	return sh.SubjectDescription + " " + sh.BackgroundDescription
}
