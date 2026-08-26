// 资产↔叙事单元/镜头 自动建链（U5/F10，18 - 视频制作 阶段一）：
// 按名称实体匹配写 nu_asset_links / shot_asset_refs / shots.linked_assets_json。
// 对齐 LuxReal「简化模式：描述即生效」——文本中出现资产名即建立引用，
// 供 LoadShotRefs 在视频生成时注入参考图（替代恒空的纯 T2V 降级）。
package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssetLinkItem 参与匹配的资产（Kind: CHARACTER/SCENE/PROP；ID 为对应表主键）。
type AssetLinkItem struct {
	Kind string
	ID   int64
	Name string
}

// dbtx 事务抽象（建链在拆分/分镜重生成事务内执行）。
type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// UnitTextInput 单元级待匹配文本（script_original_text + 镜头文案拼接）。
type UnitTextInput struct {
	NarrativeUnitID int64
	Text            string
}

// ShotTextInput 镜头级待匹配文本（主体/背景/台词/环境音拼接）。
type ShotTextInput struct {
	ShotID          int64
	NarrativeUnitID int64
	Text            string
	RefNames        []string // LLM 显式声明的引用资产名（对齐 LuxReal shots.ref_assets；精确匹配优先于文本匹配）
}

// assetNameKeys 资产名匹配键：全名 + 去描述截断名（首个标点前片段，如
// "茶水间，空间紧凑，一侧放" → "茶水间"）——拆分 LLM 偶发把描述写进 base_name 时的兜底。
func assetNameKeys(name string) []string {
	name = strings.TrimSpace(name)
	keys := []string{name}
	if i := strings.IndexAny(name, "，。、；！？,.;:！？"); i > 1 {
		seg := strings.TrimSpace(name[:i])
		if len([]rune(seg)) >= 2 && seg != name {
			keys = append(keys, seg)
		}
	}
	return keys
}

// matchAssetLinks 名称实体匹配：名称（去空白后 ≥2 字符，或其截断简称）在文本中出现即命中，按资产顺序保序。
func matchAssetLinks(text string, assets []AssetLinkItem) []AssetLinkItem {
	if len(text) == 0 || len(assets) == 0 {
		return nil
	}
	hit := make([]AssetLinkItem, 0, 4)
	for _, a := range assets {
		name := strings.TrimSpace(a.Name)
		if len([]rune(name)) < 2 { // 单字名误命中率过高（如"我"），不参与匹配
			continue
		}
		for _, k := range assetNameKeys(name) {
			if strings.Contains(text, k) {
				hit = append(hit, a)
				break
			}
		}
	}
	return hit
}

// matchShotAssets 镜头资产匹配（显式引用优先，文本匹配兜底）：
// ① 显式引用精确命中（ref 名 == 资产名）；
// ② 显式引用包含兜底：无精确命中且仅一个资产名包含该 ref（≥2 字，如"公寓" → "林晚公寓"）；
// ③ 文本匹配（资产全名/截断简称出现在镜头文本中）。
// 按资产清单序去重保序。
func matchShotAssets(s ShotTextInput, assets []AssetLinkItem) []AssetLinkItem {
	seen := make(map[string]bool, len(assets))
	hit := make([]AssetLinkItem, 0, 4)
	add := func(a AssetLinkItem) {
		k := a.Kind + "#" + strconv.FormatInt(a.ID, 10)
		if !seen[k] {
			seen[k] = true
			hit = append(hit, a)
		}
	}
	for _, ref := range s.RefNames {
		ref = strings.TrimSpace(ref)
		if len([]rune(ref)) < 2 {
			continue // 单字/空引用误命中率过高
		}
		exact := false
		for _, a := range assets {
			if a.Name == ref {
				add(a)
				exact = true
			}
		}
		if exact {
			continue
		}
		var cand []AssetLinkItem
		for _, a := range assets {
			if len([]rune(a.Name)) >= 2 && strings.Contains(a.Name, ref) {
				cand = append(cand, a)
			}
		}
		if len(cand) == 1 { // 唯一包含才命中，多候选视为歧义放弃
			add(cand[0])
		}
	}
	for _, a := range matchAssetLinks(s.Text, assets) {
		add(a)
	}
	return hit
}

// ShotMatchText 镜头匹配文本（主体/背景/台词/环境音）。
func ShotMatchText(subject, background, dialogue, ambient string) string {
	return subject + " " + background + " " + dialogue + " " + ambient
}

// LinkAssetsInTx 事务内建链：单元级写 nu_asset_links；镜头级写 shot_asset_refs + linked_assets_json。
// 镜头匹配 = 显式引用（ShotTextInput.RefNames，对齐 LuxReal shots.ref_assets）∪ 文本名称匹配。
// 返回（单元链数, 镜头链数, 逐镜头命中资产 shotID→assets）。幂等：ON CONFLICT DO NOTHING，
// linked_assets_json 为全量覆盖（拆分/重生成场景旧链已清理）。
func LinkAssetsInTx(ctx context.Context, tx dbtx, units []UnitTextInput, shots []ShotTextInput, assets []AssetLinkItem) (int, int, map[int64][]AssetLinkItem, error) {
	unitLinks := 0
	for _, u := range units {
		for _, a := range matchAssetLinks(u.Text, assets) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO nu_asset_links (narrative_unit_id, asset_kind, asset_id)
				VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				u.NarrativeUnitID, a.Kind, a.ID); err != nil {
				return unitLinks, 0, nil, err
			}
			unitLinks++
		}
	}
	shotLinks := 0
	shotAssets := make(map[int64][]AssetLinkItem, len(shots))
	for _, s := range shots {
		matched := matchShotAssets(s, assets)
		if len(matched) == 0 {
			continue
		}
		shotAssets[s.ShotID] = matched
		links := make([]map[string]any, 0, len(matched))
		for _, a := range matched {
			if _, err := tx.Exec(ctx, `
				INSERT INTO shot_asset_refs (shot_id, asset_kind, asset_id)
				VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				s.ShotID, a.Kind, a.ID); err != nil {
				return unitLinks, shotLinks, shotAssets, err
			}
			links = append(links, map[string]any{"asset_kind": a.Kind, "asset_id": a.ID, "name": a.Name})
		}
		lj, _ := json.Marshal(links)
		if _, err := tx.Exec(ctx, `UPDATE shots SET linked_assets_json=$2::jsonb WHERE id=$1`,
			s.ShotID, string(lj)); err != nil {
			return unitLinks, shotLinks, shotAssets, err
		}
		shotLinks++
	}
	return unitLinks, shotLinks, shotAssets, nil
}

// loadProjectAssets 项目资产清单（characters/scenes/props → 统一 AssetLinkItem）。
func loadProjectAssets(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, projectID int64) ([]AssetLinkItem, error) {
	rows, err := q.Query(ctx, `
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

// BackfillProjectAssetLinks 存量项目一次性回填（projectID=0 → 全部有拆分产物的项目）。
// 复用拆分链路同一匹配逻辑；幂等（不删旧链，仅补链 + 覆盖空 linked_assets_json）。
func BackfillProjectAssetLinks(ctx context.Context, pool *pgxpool.Pool, projectID int64) (int, error) {
	// 目标项目清单
	var projectIDs []int64
	if projectID > 0 {
		projectIDs = []int64{projectID}
	} else {
		rows, err := pool.Query(ctx, `
			SELECT DISTINCT e.project_id FROM episodes e
			JOIN narrative_units nu ON nu.episode_id = e.id AND nu.deleted_at IS NULL
			WHERE e.deleted_at IS NULL`)
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				projectIDs = append(projectIDs, id)
			}
		}
	}
	total := 0
	for _, pid := range projectIDs {
		n, err := backfillOneProject(ctx, pool, pid)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// backfillOneProject 单项目回填：读单元/镜头文本 → 事务内建链。
func backfillOneProject(ctx context.Context, pool *pgxpool.Pool, projectID int64) (int, error) {
	assets, err := loadProjectAssets(ctx, pool, projectID)
	if err != nil || len(assets) == 0 {
		return 0, err
	}
	// 单元文本（script + content 兜底）
	units := make([]UnitTextInput, 0, 16)
	urows, err := pool.Query(ctx, `
		SELECT id, COALESCE(script_original_text,'') || ' ' || COALESCE(content,'')
		FROM narrative_units WHERE project_id=$1 AND deleted_at IS NULL`, projectID)
	if err != nil {
		return 0, err
	}
	for urows.Next() {
		var u UnitTextInput
		if err := urows.Scan(&u.NarrativeUnitID, &u.Text); err == nil {
			units = append(units, u)
		}
	}
	urows.Close()
	// 镜头文本
	shots := make([]ShotTextInput, 0, 64)
	srows, err := pool.Query(ctx, `
		SELECT id, narrative_unit_id,
		       COALESCE(subject_description,'') || ' ' || COALESCE(background_description,'') || ' '
		       || COALESCE(dialogue_text,'') || ' ' || COALESCE(ambient_sound,'')
		FROM shots WHERE project_id=$1 AND deleted_at IS NULL`, projectID)
	if err != nil {
		return 0, err
	}
	for srows.Next() {
		var s ShotTextInput
		if err := srows.Scan(&s.ShotID, &s.NarrativeUnitID, &s.Text); err == nil {
			shots = append(shots, s)
		}
	}
	srows.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	_, shotLinks, _, err := LinkAssetsInTx(ctx, tx, units, shots, assets)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return shotLinks, nil
}
