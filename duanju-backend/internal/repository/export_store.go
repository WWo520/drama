// export_store 实现 agent.ExportStore + 导出 API 数据访问。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/agent"
)

// ExportStore 导出数据访问。
type ExportStore struct {
	pool *pgxpool.Pool
}

func NewExportStore(pool *pgxpool.Pool) *ExportStore {
	return &ExportStore{pool: pool}
}

var _ agent.ExportStore = (*ExportStore)(nil)

func (s *ExportStore) CreateExportRecord(ctx context.Context, userID, spaceID, projectID int64, exportType, resolution string, watermark bool, taskID int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO export_records (user_id, space_id, project_id, export_type, config_resolution, config_watermark, status, generation_task_id)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', $7) RETURNING id`,
		userID, spaceID, projectID, exportType, resolution, watermark, taskID).Scan(&id)
	return id, err
}

// CreateExportTask 创建导出记录 + 生成任务（QUEUED + Outbox 同一事务）。
// episodeID>0 时写入任务参数与 export_range_json，Worker 据此按集合成；0 = 首集兜底。
// 返回 (exportID, taskID, error)——taskID 供计费冻结使用。
func (s *ExportStore) CreateExportTask(ctx context.Context, userID, spaceID, projectID int64, exportID int64, resolution string, watermark bool, episodeID int64) (int64, int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	params, _ := json.Marshal(map[string]any{"export_id": exportID, "resolution": resolution, "watermark": watermark, "episode_id": episodeID})

	var taskID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id,
			task_type, status, priority, request_params_json)
		VALUES ('export-'||gen_random_uuid(), 'export-'||gen_random_uuid(), $1, $2, $3,
		        'FINAL_VIDEO_COMPOSE', 'QUEUED', 5, $4::jsonb)
		RETURNING id`,
		spaceID, userID, projectID, string(params)).Scan(&taskID)
	if err != nil {
		return 0, 0, err
	}

	rangeJSON, _ := json.Marshal(map[string]any{"episode_id": episodeID})
	// 创建导出记录关联任务
	var eid int64
	err = tx.QueryRow(ctx, `
		INSERT INTO export_records (user_id, space_id, project_id, export_type, export_range_json, config_resolution, config_watermark, status, generation_task_id)
		VALUES ($1, $2, $3, 'FINAL_VIDEO_MP4', $4::jsonb, $5, $6, 'PENDING', $7) RETURNING id`,
		userID, spaceID, projectID, rangeJSON, resolution, watermark, taskID).Scan(&eid)
	if err != nil {
		return 0, 0, err
	}

	// 用 eid 更新任务参数（export_id=实际导出记录 ID + 目标集）
	fixedParams, _ := json.Marshal(map[string]any{"export_id": eid, "resolution": resolution, "watermark": watermark, "episode_id": episodeID})
	_, err = tx.Exec(ctx, `UPDATE generation_tasks SET request_params_json=$2 WHERE id=$1`, taskID, string(fixedParams))
	if err != nil {
		return 0, 0, err
	}

	// Outbox
	_, err = tx.Exec(ctx, `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:export', 'export',
		        jsonb_build_object('task_id', $1::int))`, taskID)
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return eid, taskID, nil
}

// ListEpisodeSegments 按 episode 拉取按序的 SUCCESS 镜头成片 URL（nu.sort_order → 块 sort_order → 镜头 sort_order）。
// 排序口径与 ListShotsByBeat 一致；两种主线（SaveShotVideo/SaveCardVideo）的 URL 均落 shots.generated_video_url。
// 同 URL 去重：重规划/重复镜头块会导致新旧 shots 并存、同一视频重复入列 → 成片同画面重复。
func (s *ExportStore) ListEpisodeSegments(ctx context.Context, episodeID int64) ([]agent.ExportSegment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (sh.generated_video_url)
		       sh.generated_video_url, sh.id,
		       nu.sort_order, COALESCE(sg.sort_order, 0), sh.sort_order
		FROM narrative_units nu
		JOIN shots sh ON sh.narrative_unit_id = nu.id AND sh.deleted_at IS NULL
		LEFT JOIN shot_groups sg ON sg.id = sh.shot_group_id
		WHERE nu.episode_id = $1 AND nu.deleted_at IS NULL
		  AND sh.video_generation_status = 'SUCCESS' AND COALESCE(sh.generated_video_url, '') <> ''
		ORDER BY sh.generated_video_url, nu.sort_order, COALESCE(sg.sort_order, 0), sh.sort_order, sh.id`, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type rowT struct {
		url                    string
		shotID                 int64
		nuSort, sgSort, shSort int
	}
	rowsOut := make([]rowT, 0, 16)
	for rows.Next() {
		var r rowT
		if err := rows.Scan(&r.url, &r.shotID, &r.nuSort, &r.sgSort, &r.shSort); err != nil {
			return nil, err
		}
		rowsOut = append(rowsOut, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// DISTINCT ON 以 URL 为首排序列去重后，按业务顺序（稳定插入排序）还原
	for i := 1; i < len(rowsOut); i++ {
		for j := i; j > 0; j-- {
			a, b := rowsOut[j], rowsOut[j-1]
			if a.nuSort < b.nuSort ||
				(a.nuSort == b.nuSort && a.sgSort < b.sgSort) ||
				(a.nuSort == b.nuSort && a.sgSort == b.sgSort && a.shSort < b.shSort) {
				rowsOut[j], rowsOut[j-1] = rowsOut[j-1], rowsOut[j]
			} else {
				break
			}
		}
	}
	out := make([]agent.ExportSegment, 0, len(rowsOut))
	for _, r := range rowsOut {
		out = append(out, agent.ExportSegment{ShotID: r.shotID, URL: r.url})
	}
	return out, nil
}

// GetEpisodeIDsByProject 项目首个（当前实现按 sort_order 取第一集）的 episode id 列表。
func (s *ExportStore) FirstEpisodeID(ctx context.Context, projectID int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM episodes WHERE project_id=$1 AND deleted_at IS NULL ORDER BY sort_order LIMIT 1`, projectID).Scan(&id)
	return id, err
}

func (s *ExportStore) CompleteExport(ctx context.Context, exportID int64, outputURL string, fileSize int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_records SET status='SUCCESS', output_file_url=$2, output_file_size_bytes=$3,
		       expire_at=NOW()+interval '30 days', updated_at=NOW()
		WHERE id=$1 AND status='PENDING'`, exportID, outputURL, fileSize)
	return err
}

// UpdateExportProgress 合成进度阶段回写（Worker 侧阶段推进：DOWNLOADING/COMPOSING/UPLOADING）。
// detail: {"done":3,"total":9,"failed":2}；durationSec>0 时回写成片时长。
func (s *ExportStore) UpdateExportProgress(ctx context.Context, exportID int64, stage string, done, total, failed int, durationSec int) error {
	detail, _ := json.Marshal(map[string]any{"done": done, "total": total, "failed": failed})
	_, err := s.pool.Exec(ctx, `
		UPDATE export_records SET progress_stage=$2, progress_detail_json=$3::jsonb,
		       segments_total=$4, segments_failed=$5,
		       duration_sec=CASE WHEN $6>0 THEN $6 ELSE duration_sec END,
		       updated_at=NOW()
		WHERE id=$1`, exportID, stage, string(detail), total, failed, durationSec)
	return err
}

// ResetExportForRetry 重新合成：清输出/进度，状态回 PENDING（签名 URL 过期或失败重试）。
func (s *ExportStore) ResetExportForRetry(ctx context.Context, exportID int64, userID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE export_records SET status='PENDING', output_file_url=NULL, output_file_size_bytes=NULL,
		       progress_stage=NULL, progress_detail_json='{}'::jsonb, updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL AND status IN ('SUCCESS','FAILED')`, exportID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ResetExportForRetryByUUID 按 UUID 重置导出（Retry 端点），返回 (export_id, generation_task_id)。
// 同时把原 generation_tasks 置 FAILED_DEAD 释放结算（重试走新任务，避免旧任务悬挂）。
func (s *ExportStore) ResetExportForRetryByUUID(ctx context.Context, exportUUID string, userID int64) (int64, int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	var exportID, taskID int64
	err = tx.QueryRow(ctx, `
		UPDATE export_records SET status='PENDING', output_file_url=NULL, output_file_size_bytes=NULL,
		       progress_stage=NULL, progress_detail_json='{}'::jsonb, updated_at=NOW()
		WHERE export_uuid=$1 AND user_id=$2 AND deleted_at IS NULL AND status IN ('SUCCESS','FAILED')
		RETURNING id, COALESCE(generation_task_id, 0)`, exportUUID, userID).Scan(&exportID, &taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, nil // 不可重试（不存在/状态不允许）
		}
		return 0, 0, err
	}
	// 旧合成任务终态化（幂等：仅 RUNNING/QUEUED 置死信）
	if taskID > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE generation_tasks SET status='FAILED_DEAD', error_message='用户重试导出，任务作废', updated_at=NOW()
			WHERE id=$1 AND status IN ('QUEUED','RUNNING')`, taskID); err != nil {
			return 0, 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return exportID, taskID, nil
}

// ReenqueueComposeTask 重试重建合成任务 + Outbox（复用 export 记录的 resolution/watermark 配置）。
func (s *ExportStore) ReenqueueComposeTask(ctx context.Context, exportID, _origTaskID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// 读原导出记录配置（含目标集：export_range_json.episode_id）
	var spaceID, userID, projectID int64
	var resolution string
	var watermark bool
	var episodeID int64
	err = tx.QueryRow(ctx, `
		SELECT space_id, user_id, project_id, COALESCE(config_resolution::text,'P720'), config_watermark,
		       COALESCE((export_range_json->>'episode_id')::bigint, 0)
		FROM export_records WHERE id=$1 AND deleted_at IS NULL`, exportID).
		Scan(&spaceID, &userID, &projectID, &resolution, &watermark, &episodeID)
	if err != nil {
		return err
	}
	params, _ := json.Marshal(map[string]any{"export_id": exportID, "resolution": resolution, "watermark": watermark, "episode_id": episodeID})
	var newTaskID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id,
			task_type, status, priority, request_params_json)
		VALUES ('export-retry-'||gen_random_uuid(), 'export-retry-'||gen_random_uuid(), $1, $2, $3,
		        'FINAL_VIDEO_COMPOSE', 'QUEUED', 5, $4::jsonb)
		RETURNING id`,
		spaceID, userID, projectID, string(params)).Scan(&newTaskID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE export_records SET generation_task_id=$2, updated_at=NOW() WHERE id=$1`, exportID, newTaskID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:export', 'export',
		        jsonb_build_object('task_id', $1::int))`, newTaskID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetExportByUUID 导出详情（3.4.2；归属校验：仅导出者可见）。
func (s *ExportStore) GetExportByUUID(ctx context.Context, exportUUID string, userID int64) (map[string]any, error) {
	var uuid, status, etype, res, url string
	var wm bool
	var size int64
	var exp, created time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT export_uuid::text, status::text, export_type::text, config_resolution::text, config_watermark,
		       COALESCE(output_file_url,''), COALESCE(output_file_size_bytes,0), expire_at, created_at
		FROM export_records WHERE export_uuid=$1 AND user_id=$2 AND deleted_at IS NULL`, exportUUID, userID).
		Scan(&uuid, &status, &etype, &res, &wm, &url, &size, &exp, &created)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"export_id": uuid, "status": status, "export_type": etype, "resolution": res,
		"watermark": wm, "output_url": url, "file_size": size,
		"expire_at": exp.Format(time.RFC3339), "created_at": created.Format(time.RFC3339),
	}, nil
}

func (s *ExportStore) GetExportDetail(ctx context.Context, exportID int64) (*agent.ExportResult, error) {
	var r agent.ExportResult
	err := s.pool.QueryRow(ctx, `
		SELECT id, export_uuid::text, status::text, COALESCE(output_file_url,''), COALESCE(output_file_size_bytes,0), expire_at
		FROM export_records WHERE id=$1`, exportID).Scan(
		&r.ExportID, &r.ExportUUID, &r.Status, &r.OutputURL, &r.FileSize, &r.ExpireAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, agent.ErrProjectNotFound
	}
	return &r, err
}

// ListExports 导出列表（分页）。
// ListExportsByUser 我的全部导出（作品列表「视频成片」Tab，user_id 归属）。
func (s *ExportStore) ListExportsByUser(ctx context.Context, userID int64, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM export_records WHERE user_id=$1 AND deleted_at IS NULL`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.export_uuid::text, e.status::text, e.export_type::text, COALESCE(e.config_resolution::text,''),
		       COALESCE(e.config_watermark,FALSE), COALESCE(e.output_file_url,''), COALESCE(e.output_file_size_bytes,0),
		       e.expire_at, e.created_at, COALESCE(p.title,''), COALESCE(p.project_uuid::text,'')
		FROM export_records e LEFT JOIN projects p ON p.id=e.project_id
		WHERE e.user_id=$1 AND e.deleted_at IS NULL
		ORDER BY e.created_at DESC LIMIT $2 OFFSET $3`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, status, etype, res, url, projectTitle, projectUUID string
		var wm bool
		var size int64
		var exp, created time.Time
		if err := rows.Scan(&uuid, &status, &etype, &res, &wm, &url, &size, &exp, &created, &projectTitle, &projectUUID); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"export_id": uuid, "status": status, "export_type": etype, "resolution": res,
			"watermark": wm, "output_url": url, "file_size": size,
			"expire_at": exp.Format(time.RFC3339), "created_at": created.Format(time.RFC3339),
			"project_title": projectTitle, "project_id": projectUUID,
		})
	}
	return out, total, rows.Err()
}

func (s *ExportStore) ListExports(ctx context.Context, projectID int64, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM export_records WHERE project_id=$1 AND deleted_at IS NULL`, projectID).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT export_uuid::text, status::text, export_type::text, config_resolution::text, config_watermark,
		       COALESCE(output_file_url,''), COALESCE(output_file_size_bytes,0), expire_at, created_at,
		       COALESCE(progress_stage,''), progress_detail_json, segments_total, segments_failed, COALESCE(duration_sec,0)
		FROM export_records WHERE project_id=$1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, projectID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var uuid, status, etype, res, url, stage string
		var wm bool
		var size, segTotal, segFailed, durSec int64
		var detailJSON []byte
		var exp, created time.Time
		if err := rows.Scan(&uuid, &status, &etype, &res, &wm, &url, &size, &exp, &created,
			&stage, &detailJSON, &segTotal, &segFailed, &durSec); err != nil {
			continue
		}
		detail := map[string]any{}
		_ = json.Unmarshal(detailJSON, &detail)
		out = append(out, map[string]any{
			"export_id": uuid, "status": status, "export_type": etype, "resolution": res,
			"watermark": wm, "output_url": url, "file_size": size,
			"expire_at": exp.Format(time.RFC3339), "created_at": created.Format(time.RFC3339),
			"progress_stage":  stage,
			"progress_detail": detail,
			"segments_total":  segTotal,
			"segments_failed": segFailed,
			"duration_sec":    durSec,
		})
	}
	return out, total, rows.Err()
}
