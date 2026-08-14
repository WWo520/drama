// video_store 实现 agent.VideoStore：镜头加载/视频回写（CAS 幂等）/ 进度更新/镜头块状态。
package repository

import (
	"context"
	"errors"

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

// LoadShot 镜头信息。
func (s *VideoStore) LoadShot(ctx context.Context, shotID int64) (*agent.ShotInfo, error) {
	var info agent.ShotInfo
	err := s.pool.QueryRow(ctx, `
		SELECT sh.id, sh.shot_group_id, sh.project_id, sh.narrative_unit_id, sh.duration_sec,
		       COALESCE(sh.prompt_refined, '')
		FROM shots sh WHERE sh.id = $1 AND sh.deleted_at IS NULL`, shotID,
	).Scan(&info.ShotID, &info.ShotGroupID, &info.ProjectID, &info.NarrativeUnitID,
		&info.DurationSec, &info.PromptRefined)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	return &info, err
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