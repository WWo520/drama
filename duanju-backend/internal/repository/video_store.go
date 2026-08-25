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
// 参考图来源：characters/scenes/props 的 main_image_url 字段。
// 排序：角色图优先、场景次之（对齐 18 - 视频制作：主体一致性权重最高）。
func (s *VideoStore) LoadShotRefs(ctx context.Context, shotID int64, narrativeUnitID int64) ([]string, error) {
	// 1. 优先查镜头级覆盖（shot_asset_refs）
	rows, err := s.pool.Query(ctx, `
		SELECT a.main_image_url FROM shot_asset_refs sar
		LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND sar.asset_id=c.id
		LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sar.asset_id=sc.id
		LEFT JOIN props p ON sar.asset_kind='PROP' AND sar.asset_id=p.id
		LEFT JOIN assets a ON a.id=COALESCE(c.asset_id, sc.asset_id, p.asset_id)
		WHERE sar.shot_id=$1 AND a.main_image_url IS NOT NULL AND a.main_image_url != ''
		ORDER BY CASE sar.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END, sar.id ASC`, shotID)
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
		SELECT a.main_image_url FROM nu_asset_links nal
		LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND nal.asset_id=c.id
		LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND nal.asset_id=sc.id
		LEFT JOIN props p ON nal.asset_kind='PROP' AND nal.asset_id=p.id
		LEFT JOIN assets a ON a.id=COALESCE(c.asset_id, sc.asset_id, p.asset_id)
		WHERE nal.narrative_unit_id=$1 AND a.main_image_url IS NOT NULL AND a.main_image_url != ''
		ORDER BY CASE nal.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END, nal.id ASC`, narrativeUnitID)
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
		       COALESCE(sh.prompt_refined, ''), COALESCE(sh.reference_image_url, '')
		FROM shots sh WHERE sh.id = $1 AND sh.deleted_at IS NULL`, shotID,
	).Scan(&info.ShotID, &info.ShotGroupID, &info.ProjectID, &info.NarrativeUnitID,
		&info.DurationSec, &info.PromptRefined, &info.ReferenceImageURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	return &info, err
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
