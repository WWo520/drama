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

// LoadShotRefs 加载镜头参考图 URL 列表（F10：shot_asset_refs 优先，否则继承 nu_asset_links）。
// 参考图来源（业务表口径，Step2 生成图片的落点）：
//
//	角色 → character_variants.main_image_url（首个变体）；场景 → scenes.main_image_url；道具 → props.main_image_url。
//
// 注意：assets 表为通用资产注册表（无 main_image_url 列），不可作取图来源。
// 排序：角色图优先、场景次之（对齐 18 - 视频制作：主体一致性权重最高）。
func (s *VideoStore) LoadShotRefs(ctx context.Context, shotID int64, narrativeUnitID int64) ([]string, error) {
	// 1. 优先查镜头级覆盖（shot_asset_refs）
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
		) t WHERE t.url IS NOT NULL AND t.url != ''
		ORDER BY t.ord, t.rid`, shotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			continue
		}
		urls = append(urls, url)
	}
	if len(urls) > 0 {
		return urls, nil // 镜头级覆盖存在，直接返回
	}

	// 2. 镜头级无覆盖，继承叙事单元级（nu_asset_links）
	rows2, err := s.pool.Query(ctx, `
		SELECT t.url FROM (
			SELECT COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, '') AS url,
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
		) t WHERE t.url IS NOT NULL AND t.url != ''
		ORDER BY t.ord, t.rid`, narrativeUnitID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	urls = []string{}
	for rows2.Next() {
		var url string
		if err := rows2.Scan(&url); err != nil {
			continue
		}
		urls = append(urls, url)
	}
	return urls, nil
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

// LoadShotRefAssets 加载镜头参考资产配对列表（镜头级 shot_asset_refs 优先，回退单元级 nu_asset_links）。
// 仅返回有参考图的资产（与 LoadShotRefs 取图口径一致：角色→首个变体、场景/道具→自身 main_image_url），
// 排序角色→场景→道具（主体一致性权重最高）。名称+URL 同源同序，供提示词 <Picture N> 显式绑定。
func (s *VideoStore) LoadShotRefAssets(ctx context.Context, shotID int64, narrativeUnitID int64) []agent.RefAsset {
	// 1. 镜头级覆盖
	refs := s.queryRefAssets(ctx, `
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
		) t WHERE t.name != '' AND t.url != '' ORDER BY t.ord, t.rid`, shotID)
	if len(refs) > 0 {
		return refs
	}
	// 2. 回退叙事单元级
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

// LoadShotAssetNames 加载镜头关联资产名（镜头级 shot_asset_refs 优先，回退单元级 nu_asset_links；
// 对齐 LuxReal @资产引用：视频 prompt 中以 @名 显式标记引用资产）。
func (s *VideoStore) LoadShotAssetNames(ctx context.Context, shotID int64, narrativeUnitID int64) []string {
	names := s.queryAssetNames(ctx, `
		SELECT t.name FROM (
			SELECT COALESCE(c.name, sc.base_name, p.name, '') AS name,
			       CASE sar.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END AS ord, sar.id AS rid
			FROM shot_asset_refs sar
			LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
			LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON sar.asset_kind='PROP' AND p.id=sar.asset_id AND p.deleted_at IS NULL
			WHERE sar.shot_id=$1
		) t WHERE t.name != '' ORDER BY t.ord, t.rid`, shotID)
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

// CompleteVideoIfAllDone 项目下全部镜头 SUCCESS 后推进状态：VIDEO_GENERATING/PARTIAL_VIDEOS → ALL_VIDEOS_READY。
func (s *VideoStore) CompleteVideoIfAllDone(ctx context.Context, projectID int64) error {
	var remaining int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM shots
		WHERE project_id=$1 AND deleted_at IS NULL AND video_generation_status != 'SUCCESS'`,
		projectID).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		_, err := s.pool.Exec(ctx, `
			UPDATE projects SET status='ALL_VIDEOS_READY', current_step='FINAL_PREVIEW'
			WHERE id=$1 AND status IN ('VIDEO_GENERATING','PARTIAL_VIDEOS')`, projectID)
		return err
	}
	// 有失败镜头 → PARTIAL_VIDEOS（仅当此前处于 VIDEO_GENERATING 且存在非 SUCCESS 非 PENDING）
	var failed int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM shots
		WHERE project_id=$1 AND deleted_at IS NULL AND video_generation_status='FAILED'`,
		projectID).Scan(&failed); err != nil {
		return err
	}
	if failed > 0 {
		_, err := s.pool.Exec(ctx, `
			UPDATE projects SET status='PARTIAL_VIDEOS', current_step='VIDEO_PRODUCTION'
			WHERE id=$1 AND status='VIDEO_GENERATING'`, projectID)
		return err
	}
	return nil
}
