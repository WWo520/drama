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
// 返回 (exportID, taskID, error)——taskID 供计费冻结使用。
func (s *ExportStore) CreateExportTask(ctx context.Context, userID, spaceID, projectID int64, exportID int64, resolution string, watermark bool) (int64, int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	params, _ := json.Marshal(map[string]any{"export_id": exportID, "resolution": resolution, "watermark": watermark})

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

	// 创建导出记录关联任务
	var eid int64
	err = tx.QueryRow(ctx, `
		INSERT INTO export_records (user_id, space_id, project_id, export_type, config_resolution, config_watermark, status, generation_task_id)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', $7) RETURNING id`,
		userID, spaceID, projectID, "FINAL_VIDEO_MP4", resolution, watermark, taskID).Scan(&eid)
	if err != nil {
		return 0, 0, err
	}

	// 用 eid 更新任务参数（export_id=实际导出记录 ID）
	fixedParams, _ := json.Marshal(map[string]any{"export_id": eid, "resolution": resolution, "watermark": watermark})
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

func (s *ExportStore) CompleteExport(ctx context.Context, exportID int64, outputURL string, fileSize int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_records SET status='SUCCESS', output_file_url=$2, output_file_size_bytes=$3,
		       expire_at=NOW()+interval '30 days', updated_at=NOW()
		WHERE id=$1 AND status='PENDING'`, exportID, outputURL, fileSize)
	return err
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
func (s *ExportStore) ListExports(ctx context.Context, projectID int64, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 { page = 1 }
	if pageSize < 1 || pageSize > 50 { pageSize = 20 }
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM export_records WHERE project_id=$1 AND deleted_at IS NULL`, projectID).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT export_uuid::text, status::text, export_type::text, config_resolution::text, config_watermark,
		       COALESCE(output_file_url,''), COALESCE(output_file_size_bytes,0), expire_at, created_at
		FROM export_records WHERE project_id=$1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, projectID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var uuid, status, etype, res, url string
		var wm bool
		var size int64
		var exp, created time.Time
		rows.Scan(&uuid, &status, &etype, &res, &wm, &url, &size, &exp, &created)
		out = append(out, map[string]any{
			"export_id": uuid, "status": status, "export_type": etype, "resolution": res,
			"watermark": wm, "output_url": url, "file_size": size,
			"expire_at": exp.Format(time.RFC3339), "created_at": created.Format(time.RFC3339),
		})
	}
	return out, total, rows.Err()
}