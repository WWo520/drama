// task_store 实现 engine/task.Store 接口（generation_tasks / task_dispatch_outbox 的调度读写）。
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/task"
)

// TaskStore 调度引擎 DB 访问。
type TaskStore struct {
	pool *pgxpool.Pool
}

func NewTaskStore(pool *pgxpool.Pool) *TaskStore {
	return &TaskStore{pool: pool}
}

var _ task.Store = (*TaskStore)(nil)

func (s *TaskStore) GetTask(ctx context.Context, id int64) (*task.Task, error) {
	var t task.Task
	err := s.pool.QueryRow(ctx, `
		SELECT id, task_uuid::text, task_type::text, status::text, priority, retry_count, max_retry_count,
		       request_params_json, space_id, user_id, project_id, COALESCE(lease_owner,''), enqueued_at, next_retry_at
		FROM generation_tasks WHERE id = $1`, id,
	).Scan(&t.ID, &t.TaskUUID, &t.TaskType, &t.Status, &t.Priority, &t.RetryCount, &t.MaxRetry,
		&t.RequestParamsJSON, &t.SpaceID, &t.UserID, &t.ProjectID, &t.LeaseOwner, &t.EnqueuedAt, &t.NextRetryAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, task.ErrTaskNotFound
	}
	return &t, err
}

func (s *TaskStore) ClaimForRun(ctx context.Context, id int64, owner string, leaseExpiresAt time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE generation_tasks SET status='RUNNING', lease_owner=$2, lease_expires_at=$3, started_at=NOW()
		WHERE id=$1 AND status='QUEUED' AND (lease_owner IS NULL OR lease_owner=$2)`,
		id, owner, leaseExpiresAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *TaskStore) RenewLeaseDB(ctx context.Context, id int64, owner string, leaseExpiresAt time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE generation_tasks SET lease_expires_at=$3, heartbeat_at=NOW()
		WHERE id=$1 AND status='RUNNING' AND lease_owner=$2`,
		id, owner, leaseExpiresAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *TaskStore) CompleteTask(ctx context.Context, id int64, owner string, resultJSON []byte, output string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE generation_tasks SET status='SUCCESS', completed_at=NOW(), lease_owner=NULL, lease_expires_at=NULL,
		       progress_pct=100, result_output_json=COALESCE($3::jsonb, '{}'::jsonb), user_display_error=NULL
		WHERE id=$1 AND status='RUNNING' AND lease_owner=$2`,
		id, owner, resultJSON)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return task.ErrTaskNotFound
	}
	return nil
}

func (s *TaskStore) FailTask(ctx context.Context, id int64, owner, reason string, retryCount int16, nextRetryAt time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var maxRetry int16
	if err := tx.QueryRow(ctx,
		`SELECT max_retry_count FROM generation_tasks WHERE id=$1 AND status='RUNNING' AND lease_owner=$2 FOR UPDATE`,
		id, owner).Scan(&maxRetry); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, task.ErrTaskNotFound
		}
		return false, err
	}

	dead := retryCount >= maxRetry
	nextStatus := task.StatusFailedRetrying
	if dead {
		nextStatus = task.StatusFailedDead
	}
	retryable := nextStatus == task.StatusFailedRetrying
	// 写重试日志（B4.3 generation_task_retry_logs）
	_, _ = tx.Exec(ctx, `
		INSERT INTO generation_task_retry_logs (task_id, retry_num, from_status, to_status, reason, backoff_seconds)
		VALUES ($1, $2, 'RUNNING', $3, $4, $5)`,
		id, retryCount, nextStatus, reason, task.BackoffFor(int(retryCount)))

	if retryable {
		_, err = tx.Exec(ctx, `
			UPDATE generation_tasks SET status='FAILED_RETRYING', lease_owner=NULL, lease_expires_at=NULL,
			       retry_count=$2, error_code='MODEL_TIMEOUT', error_message=$3, next_retry_at=$4
			WHERE id=$1`,
			id, retryCount, reason, nextRetryAt)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE generation_tasks SET status='FAILED_DEAD', lease_owner=NULL, lease_expires_at=NULL,
			       retry_count=$2, error_code='MODEL_TIMEOUT', error_message=$3, next_retry_at=NULL
			WHERE id=$1`,
			id, retryCount, reason)
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return dead, nil
}

func (s *TaskStore) RequeueDue(ctx context.Context, now time.Time) ([]*task.Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, priority FROM generation_tasks
		WHERE status='FAILED_RETRYING' AND next_retry_at <= $1 AND deleted_at IS NULL
		FOR UPDATE SKIP LOCKED`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*task.Task
	for rows.Next() {
		var t task.Task
		if err := rows.Scan(&t.ID, &t.Priority); err != nil {
			return nil, err
		}
		// 转 QUEUED + 新 Outbox（同一事务：B4.1a 幂等 dispatch_key 唯一）
		if _, err := s.pool.Exec(ctx, `
			WITH upd AS (
			  UPDATE generation_tasks SET status='QUEUED', enqueued_at=NOW()
			  WHERE id=$1 AND status='FAILED_RETRYING' RETURNING id
			)
			INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
			SELECT 'TASK_DISPATCH', id, 'task:'||id||':dispatch:'||(SELECT count(*)+1 FROM task_dispatch_outbox WHERE task_id=id), 'retry',
			       jsonb_build_object('task_id', id)
			FROM upd
			ON CONFLICT (dispatch_key) DO NOTHING`, t.ID); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *TaskStore) ClaimOutbox(ctx context.Context, owner string, leaseExpiresAt time.Time) (*task.OutboxRow, error) {
	var o task.OutboxRow
	err := s.pool.QueryRow(ctx, `
		UPDATE task_dispatch_outbox SET status='PROCESSING', lease_owner=$1, lease_expires_at=$2,
		       attempt_count = attempt_count + 1
		WHERE id = (
		  SELECT id FROM task_dispatch_outbox
		  WHERE status IN ('PENDING','PROCESSING') AND next_attempt_at <= NOW()
		    AND (lease_expires_at IS NULL OR lease_expires_at < NOW())
		  ORDER BY created_at LIMIT 1
		  FOR UPDATE SKIP LOCKED
		)
		RETURNING id, task_id, dispatch_key, queue_name, payload_json, status::text, attempt_count, next_attempt_at, lease_owner, lease_expires_at`,
		owner, leaseExpiresAt).Scan(&o.ID, &o.TaskID, &o.DispatchKey, &o.QueueName, &o.PayloadJSON,
		&o.Status, &o.AttemptCount, &o.NextAttemptAt, &o.LeaseOwner, &o.LeaseExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *TaskStore) MarkOutboxPublished(ctx context.Context, id int64, owner string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE task_dispatch_outbox SET status='PUBLISHED', published_at=NOW(), lease_owner=NULL, lease_expires_at=NULL
		WHERE id=$1 AND status='PROCESSING' AND lease_owner=$2`, id, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return task.ErrTaskNotFound
	}
	return nil
}

func (s *TaskStore) MarkOutboxFailed(ctx context.Context, id int64, owner, errMsg string) error {
	// 指数退避（5s/30s/120s）→ 超 10 次转 DEAD（B4.1a 发布失败阈值）
	tag, err := s.pool.Exec(ctx, `
		UPDATE task_dispatch_outbox SET status = CASE WHEN attempt_count >= 10 THEN 'DEAD' ELSE 'PENDING' END,
		       lease_owner=NULL, lease_expires_at=NULL,
		       next_attempt_at = CASE WHEN attempt_count >= 10 THEN next_attempt_at
		                              ELSE NOW() + (interval '5 seconds' * power(2, LEAST(attempt_count, 5))) END,
		       last_error_code='REDIS_ZADD_FAILED', last_error_message=$3
		WHERE id=$1 AND status='PROCESSING' AND lease_owner=$2`, id, owner, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return task.ErrTaskNotFound
	}
	return nil
}

func (s *TaskStore) ReapExpired(ctx context.Context, now time.Time) ([]*task.Task, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, priority FROM generation_tasks
		WHERE status='RUNNING' AND lease_expires_at < $1 AND deleted_at IS NULL
		FOR UPDATE SKIP LOCKED`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*task.Task
	for rows.Next() {
		var t task.Task
		if err := rows.Scan(&t.ID, &t.Priority); err != nil {
			return nil, err
		}
		// 回 QUEUED + 新 Outbox（B4.1a 同一事务；enqueued_at 刷新）
		if _, err := s.pool.Exec(ctx, `
			WITH upd AS (
			  UPDATE generation_tasks SET status='QUEUED', enqueued_at=NOW(), lease_owner=NULL, lease_expires_at=NULL,
			         retry_count = retry_count + 1
			  WHERE id=$1 AND status='RUNNING' RETURNING id
			)
			INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
			SELECT 'TASK_DISPATCH', id, 'task:'||id||':dispatch:reap-'||extract(epoch from now()), 'reap',
			       jsonb_build_object('task_id', id)
			FROM upd`, t.ID); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *TaskStore) EnqueueTask(ctx context.Context, taskID int64, queueName string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE generation_tasks SET status='QUEUED', enqueued_at=NOW()
		WHERE id=$1 AND status IN ('PENDING','FAILED_RETRYING')`, taskID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1, 'task:'||$1||':dispatch:'||floor(random()*1000000)::int, $2,
		        jsonb_build_object('task_id', $1))`, taskID, queueName); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
