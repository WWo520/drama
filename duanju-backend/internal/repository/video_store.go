// video_store 实现 agent.VideoStore：镜头加载/视频回写（CAS 幂等）/ 进度更新/镜头块状态。
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/agent"
)

// VideoStore 视频数据访问。
type VideoStore struct {
	pool *pgxpool.Pool
}

func NewVideoStore(pool *pgxpool.Pool) *VideoStore {
	return &VideoStore{pool: pool}
}

var _ agent.VideoStore = (*VideoStore)(nil)

// LoadShotRefs 加载镜头参考图 URL 列表（F10：shot_asset_refs 优先；缺 SCENE 时从 nu_asset_links 补齐继承）。
// 参考图来源（业务表口径，Step2 生成图片的落点）：
//
//	角色 → character_variants.main_image_url（首个变体）；场景 → scenes.main_image_url；道具 → props.main_image_url。
//
// 注意：assets 表为通用资产表（无 main_image_url 列），不可作取图来源。
// 排序：角色图优先、场景次之（对齐 18 - 视频制作：主体一致性权重最高）。
// 场景补齐（防场景漂移）：对话/特写镜头常只命中角色（背景描述未写场景名导致匹配不中），
// 此类镜头若整组不回退会丢失场景参考 → 画面漂移到别的地点（用户实测"餐厅对话答话人不在餐厅"根因）。
// 故镜头级缺 SCENE 关联时自动继承叙事单元级场景链（同一单元=同一地点）；角色/道具仍以镜头级为准（对话轮换主体各异）。
func (s *VideoStore) LoadShotRefs(ctx context.Context, shotID int64, narrativeUnitID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.url FROM (
			SELECT COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, '') AS url,
			       CASE sar.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, sar.id AS rid
			FROM shot_asset_refs sar
			LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT main_image_url FROM character_variants
			    WHERE character_id=c.id AND deleted_at IS NULL ORDER BY id LIMIT 1
			) cv ON TRUE
			LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON sar.asset_kind='PROP' AND p.id=sar.asset_id AND p.deleted_at IS NULL
			WHERE sar.shot_id=$1
			UNION ALL
			SELECT COALESCE(sc.main_image_url, '') AS url, 2 AS ord, nal.id AS rid
			FROM nu_asset_links nal
			JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			WHERE nal.narrative_unit_id=$2 AND nal.asset_kind='SCENE'
			  AND NOT EXISTS (SELECT 1 FROM shot_asset_refs x WHERE x.shot_id=$1 AND x.asset_kind='SCENE')
		) t WHERE t.url IS NOT NULL AND t.url != ''
		ORDER BY t.ord, t.rid`, shotID, narrativeUnitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	urls := []string{}
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			continue
		}
		urls = append(urls, url)
	}
	return urls, rows.Err()
}

// LoadShot 镜头信息。
func (s *VideoStore) LoadShot(ctx context.Context, shotID int64) (*agent.ShotInfo, error) {
	var info agent.ShotInfo
	err := s.pool.QueryRow(ctx, `
		SELECT sh.id, sh.shot_group_id, sh.project_id, sh.narrative_unit_id, sh.duration_sec,
		       COALESCE(sh.prompt_refined, ''), COALESCE(sh.reference_image_url, ''),
		       COALESCE(sh.shot_size::text,''), COALESCE(sh.camera_movement::text,''),
		       COALESCE(sh.camera_angle::text,''),
		       COALESCE(sh.subject_description,''), COALESCE(sh.background_description,''),
		       COALESCE(sh.local_light_description,''), COALESCE(sh.dialogue_text,''),
		       COALESCE(sh.ambient_sound,'')
		FROM shots sh WHERE sh.id = $1 AND sh.deleted_at IS NULL`, shotID,
	).Scan(&info.ShotID, &info.ShotGroupID, &info.ProjectID, &info.NarrativeUnitID,
		&info.DurationSec, &info.PromptRefined, &info.ReferenceImageURL,
		&info.ShotSize, &info.CameraMovement, &info.CameraAngle,
		&info.Subject, &info.Background, &info.Light, &info.Dialogue, &info.AmbientSound)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	return &info, err
}

// RefAsset 参考资产配对定义在 agent 包（接口归属方），此处仅实现。

// LoadShotRefAssets 加载镜头参考资产配对列表（镜头级 shot_asset_refs 优先；缺 SCENE 时从 nu_asset_links 补齐，
// 口径与 LoadShotRefs 的场景补齐一致——防"餐厅对话答话人不在餐厅"式场景漂移）。
// 仅返回有参考图的资产（与 LoadShotRefs 取图口径一致：角色→首个变体、场景/道具→自身 main_image_url），
// 排序角色→场景→道具（主体一致性权重最高）。名称+URL 同源同序，供提示词 <Picture N> 显式绑定。
func (s *VideoStore) LoadShotRefAssets(ctx context.Context, shotID int64, narrativeUnitID int64) []agent.RefAsset {
	refs := s.queryRefAssets2(ctx, `
		SELECT t.kind, t.name, t.url FROM (
			SELECT sar.asset_kind::text AS kind, COALESCE(c.name, sc.base_name, p.name, '') AS name,
			       COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, '') AS url,
			       CASE sar.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, sar.id AS rid
			FROM shot_asset_refs sar
			LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT main_image_url FROM character_variants
			    WHERE character_id=c.id AND deleted_at IS NULL ORDER BY id LIMIT 1
			) cv ON TRUE
			LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON sar.asset_kind='PROP' AND p.id=sar.asset_id AND p.deleted_at IS NULL
			WHERE sar.shot_id=$1
			UNION ALL
			SELECT 'SCENE' AS kind, sc.base_name AS name, COALESCE(sc.main_image_url, '') AS url,
			       2 AS ord, nal.id AS rid
			FROM nu_asset_links nal
			JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			WHERE nal.narrative_unit_id=$2 AND nal.asset_kind='SCENE'
			  AND NOT EXISTS (SELECT 1 FROM shot_asset_refs x WHERE x.shot_id=$1 AND x.asset_kind='SCENE')
		) t WHERE t.name != '' AND t.url != '' ORDER BY t.ord, t.rid`, shotID, narrativeUnitID)
	if len(refs) > 0 {
		return refs
	}
	// 镜头级完全无关联 → 整组继承叙事单元级
	return s.queryRefAssets(ctx, `
		SELECT t.kind, t.name, t.url FROM (
			SELECT nal.asset_kind::text AS kind, COALESCE(c.name, sc.base_name, p.name, '') AS name,
			       COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, '') AS url,
			       CASE nal.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, nal.id AS rid
			FROM nu_asset_links nal
			LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND c.id=nal.asset_id AND c.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT main_image_url FROM character_variants
			    WHERE character_id=c.id AND deleted_at IS NULL ORDER BY id LIMIT 1
			) cv ON TRUE
			LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON nal.asset_kind='PROP' AND p.id=nal.asset_id AND p.deleted_at IS NULL
			WHERE nal.narrative_unit_id=$1
		) t WHERE t.name != '' AND t.url != '' ORDER BY t.ord, t.rid`, narrativeUnitID)
}

func (s *VideoStore) queryRefAssets(ctx context.Context, sql string, arg int64) []agent.RefAsset {
	rows, err := s.pool.Query(ctx, sql, arg)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]agent.RefAsset, 0)
	for rows.Next() {
		var r agent.RefAsset
		if err := rows.Scan(&r.Kind, &r.Name, &r.URL); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// queryRefAssets2 双参版本（镜头级 shot_id + 单元级 narrative_unit_id 场景补齐查询）。
func (s *VideoStore) queryRefAssets2(ctx context.Context, sql string, shotID, narrativeUnitID int64) []agent.RefAsset {
	rows, err := s.pool.Query(ctx, sql, shotID, narrativeUnitID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]agent.RefAsset, 0)
	for rows.Next() {
		var r agent.RefAsset
		if err := rows.Scan(&r.Kind, &r.Name, &r.URL); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// LoadShotAssetNames 加载镜头关联资产名（镜头级 shot_asset_refs 优先；缺 SCENE 时从 nu_asset_links 补齐，
// 口径与 LoadShotRefs 的场景补齐一致；对齐 LuxReal @资产引用：视频 prompt 中以 @名 显式标记引用资产）。
func (s *VideoStore) LoadShotAssetNames(ctx context.Context, shotID int64, narrativeUnitID int64) []string {
	names := s.queryAssetNames2(ctx, `
		SELECT t.name FROM (
			SELECT COALESCE(c.name, sc.base_name, p.name, '') AS name,
			       CASE sar.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, sar.id AS rid
			FROM shot_asset_refs sar
			LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
			LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON sar.asset_kind='PROP' AND p.id=sar.asset_id AND p.deleted_at IS NULL
			WHERE sar.shot_id=$1
			UNION ALL
			SELECT sc.base_name AS name, 2 AS ord, nal.id AS rid
			FROM nu_asset_links nal
			JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			WHERE nal.narrative_unit_id=$2 AND nal.asset_kind='SCENE'
			  AND NOT EXISTS (SELECT 1 FROM shot_asset_refs x WHERE x.shot_id=$1 AND x.asset_kind='SCENE')
		) t WHERE t.name != '' ORDER BY t.ord, t.rid`, shotID, narrativeUnitID)
	if len(names) > 0 {
		return names
	}
	return s.queryAssetNames(ctx, `
		SELECT t.name FROM (
			SELECT COALESCE(c.name, sc.base_name, p.name, '') AS name,
			       CASE nal.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, nal.id AS rid
			FROM nu_asset_links nal
			LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND c.id=nal.asset_id AND c.deleted_at IS NULL
			LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON nal.asset_kind='PROP' AND p.id=nal.asset_id AND p.deleted_at IS NULL
			WHERE nal.narrative_unit_id=$1
		) t WHERE t.name != '' ORDER BY t.ord, t.rid`, narrativeUnitID)
}

func (s *VideoStore) queryAssetNames(ctx context.Context, sql string, arg int64) []string {
	rows, err := s.pool.Query(ctx, sql, arg)
	if err != nil {
		return nil
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// queryAssetNames2 双参版本（镜头级 shot_id + 单元级 narrative_unit_id 场景补齐查询）。
func (s *VideoStore) queryAssetNames2(ctx context.Context, sql string, shotID, narrativeUnitID int64) []string {
	rows, err := s.pool.Query(ctx, sql, shotID, narrativeUnitID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// SaveShotRefImage 记录镜头级参考图（shots.reference_image_url，对齐 LuxReal「镜头N的参考图」）。
func (s *VideoStore) SaveShotRefImage(ctx context.Context, shotID int64, imageURL string, taskID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE shots SET reference_image_url=$2, updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`, shotID, imageURL)
	return err
}

// SaveShotVideo 回调幂等：仅 shot 的 video_generation_status 不是 SUCCESS 时才写入（CAS 防重）。
// 即使重复回调也不会覆盖已完成结果。
func (s *VideoStore) SaveShotVideo(ctx context.Context, shotID int64, videoURL string, progressPct int, taskID int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE shots SET generated_video_url=$2, video_generation_status='SUCCESS'
		WHERE id=$1 AND video_generation_status != 'SUCCESS' AND deleted_at IS NULL`,
		shotID, videoURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// shot 已 SUCCESS（重复回调/乱序），静默跳过（幂等——04 附录 C.5）
		return nil
	}
	// 首次成功：项目全部镜头完成后推进状态 → ALL_VIDEOS_READY（03 A.2）
	var projectID int64
	if err := s.pool.QueryRow(ctx, `SELECT project_id FROM shots WHERE id=$1`, shotID).Scan(&projectID); err == nil {
		_ = s.CompleteVideoIfAllDone(ctx, projectID)
	}
	return nil
}

// UpdateProgress 更新任务进度（独立调用）。
func (s *VideoStore) UpdateProgress(ctx context.Context, taskID int64, pct int, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE generation_tasks SET progress_pct=$2, progress_message=$3 WHERE id=$1`,
		taskID, pct, msg)
	return err
}

// UpdateShotGroupStatus 检查镜头块所有镜头是否全部完成，是则标记 shot_group 完成。
func (s *VideoStore) UpdateShotGroupStatus(ctx context.Context, shotGroupID int64) error {
	// 查询镜头块中未完成的镜头数
	var remaining int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM shots
		WHERE shot_group_id=$1 AND deleted_at IS NULL AND video_generation_status != 'SUCCESS'`,
		shotGroupID).Scan(&remaining)
	if err != nil {
		return err
	}
	if remaining == 0 {
		// 全部完成 → 标记镜头块完成
		_, err = s.pool.Exec(ctx, `
			UPDATE shot_groups SET video_generation_status='SUCCESS', generated_video_url='COMPOSED' 
			WHERE id=$1 AND video_generation_status != 'SUCCESS'`, shotGroupID)
	}
	return err
}

// MarkVideoGenerating 视频生成开始：项目状态 PARTIAL_ASSETS → VIDEO_GENERATING（幂等）。
func (s *VideoStore) MarkVideoGenerating(ctx context.Context, projectID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE projects SET status='VIDEO_GENERATING', current_step='VIDEO_PRODUCTION'
		WHERE id=$1 AND status IN ('PARTIAL_ASSETS','ALL_ASSETS_READY')`, projectID)
	return err
}

// ConcatShotGroup 镜头块拼接（3.3.8，VIDEO_GROUP_CONCAT）：块内镜头全部 SUCCESS 才标记块完成
// （generated_video_url='COMPOSED'，对齐 LuxReal「拼接成段」语义；块内有未完成镜头返回错误）。
func (s *VideoStore) ConcatShotGroup(ctx context.Context, shotGroupID int64) error {
	var remaining int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM shots
		WHERE shot_group_id=$1 AND deleted_at IS NULL AND video_generation_status != 'SUCCESS'`,
		shotGroupID).Scan(&remaining); err != nil {
		return err
	}
	if remaining > 0 {
		return fmt.Errorf("镜头块内还有 %d 个镜头未完成，无法拼接", remaining)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE shot_groups SET video_generation_status='SUCCESS', generated_video_url='COMPOSED'
		WHERE id=$1 AND deleted_at IS NULL`, shotGroupID)
	return err
}

// LoadCard 加载成片卡信息（card 模式生成入口；deleted_at 过滤）。
func (s *VideoStore) LoadCard(ctx context.Context, cardID int64) (*agent.CardInfo, error) {
	var info agent.CardInfo
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, narrative_unit_id, duration_sec, COALESCE(title,''), COALESCE(prompt_text,'')
		FROM video_cards WHERE id=$1 AND deleted_at IS NULL`, cardID).
		Scan(&info.CardID, &info.ProjectID, &info.NarrativeUnitID, &info.DurationSec, &info.Title, &info.PromptText)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	return &info, err
}

// LoadCardShots 按顺序加载卡内镜头（sort_order 单元内全局递增；含 8 字段分镜结构）。
func (s *VideoStore) LoadCardShots(ctx context.Context, cardID int64) ([]*agent.ShotInfo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sh.id, sh.shot_group_id, sh.project_id, sh.narrative_unit_id, sh.duration_sec,
		       COALESCE(sh.prompt_refined, ''), COALESCE(sh.reference_image_url, ''),
		       COALESCE(sh.shot_size::text,''), COALESCE(sh.camera_movement::text,''),
		       COALESCE(sh.camera_angle::text,''),
		       COALESCE(sh.subject_description,''), COALESCE(sh.background_description,''),
		       COALESCE(sh.local_light_description,''), COALESCE(sh.dialogue_text,''),
		       COALESCE(sh.ambient_sound,'')
		FROM shots sh WHERE sh.video_card_id=$1 AND sh.deleted_at IS NULL
		ORDER BY sh.sort_order, sh.id`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*agent.ShotInfo, 0, 4)
	for rows.Next() {
		info := &agent.ShotInfo{}
		if err := rows.Scan(&info.ShotID, &info.ShotGroupID, &info.ProjectID, &info.NarrativeUnitID,
			&info.DurationSec, &info.PromptRefined, &info.ReferenceImageURL,
			&info.ShotSize, &info.CameraMovement, &info.CameraAngle,
			&info.Subject, &info.Background, &info.Light, &info.Dialogue, &info.AmbientSound); err != nil {
			continue
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// SaveCardVideo 回调幂等：卡片非 SUCCESS 才写入；成功时卡内 shots 同步置 SUCCESS
// （Step4 展平 / CompleteVideoIfAllDone 按镜头统计，保持兼容）。
func (s *VideoStore) SaveCardVideo(ctx context.Context, cardID int64, videoURL string, progressPct int, taskID int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE video_cards SET generated_video_url=$2, generation_status='SUCCESS', generation_task_id=$3, updated_at=NOW()
		WHERE id=$1 AND generation_status!='SUCCESS' AND deleted_at IS NULL`,
		cardID, videoURL, taskID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // 已 SUCCESS（重复回调/乱序），幂等跳过
	}
	// 卡内镜头同步置成功（视频为整段连续画面，天然覆盖全部卡内镜头）
	if _, err := s.pool.Exec(ctx, `
		UPDATE shots SET generated_video_url=$2, video_generation_status='SUCCESS', updated_at=NOW()
		WHERE video_card_id=$1 AND deleted_at IS NULL AND video_generation_status != 'SUCCESS'`,
		cardID, videoURL); err != nil {
		return err
	}
	var projectID int64
	if err := s.pool.QueryRow(ctx, `SELECT project_id FROM video_cards WHERE id=$1`, cardID).Scan(&projectID); err == nil {
		_ = s.CompleteVideoIfAllDone(ctx, projectID)
	}
	return nil
}

// CompleteVideoIfAllDone 出片进度驱动的项目状态机推进（修复合成按钮禁用缺口）：
//   - 项目内任一镜头 SUCCESS 且尚未进入导出阶段 → PARTIAL_VIDEOS（Step4 可预览与合成）
//   - 全部镜头 SUCCESS → ALL_VIDEOS_READY（含 PARTIAL_ASSETS 兜底，防状态断链）
//   - 永不回退 EXPORTING/COMPLETED/ARCHIVED 等后续状态
//
// 幂等：无 SUCCESS 镜头时不动任何状态。
func (s *VideoStore) CompleteVideoIfAllDone(ctx context.Context, projectID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE projects p SET
		  status = CASE WHEN EXISTS (
		      SELECT 1 FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
		        AND s.video_generation_status::text <> 'SUCCESS'
		  ) THEN 'PARTIAL_VIDEOS'::project_status_enum
		      ELSE 'ALL_VIDEOS_READY'::project_status_enum END,
		  current_step = CASE WHEN EXISTS (
		      SELECT 1 FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
		        AND s.video_generation_status::text <> 'SUCCESS'
		  ) THEN 'VIDEO_PRODUCTION'::project_step_enum ELSE 'FINAL_PREVIEW'::project_step_enum END,
		  updated_at = NOW()
		WHERE p.id=$1 AND p.status='VIDEO_GENERATING'
		  AND EXISTS (SELECT 1 FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
		        AND s.video_generation_status::text='SUCCESS')`, projectID)
	return err
}
