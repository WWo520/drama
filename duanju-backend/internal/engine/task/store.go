// Store 接口：调度引擎访问 generation_tasks / task_dispatch_outbox 的 DB 边界（由 repository 包实现）。
package task

import (
	"context"
	"time"
)

// Task 任务快照（调度引擎所需字段子集）。
type Task struct {
	ID           int64
	TaskUUID     string
	TaskType     string
	Status       string
	Priority     int
	RetryCount   int16
	MaxRetry     int16
	RequestParamsJSON []byte
	SpaceID      int64
	UserID       int64
	ProjectID    *int64
	LeaseOwner   string
	EnqueuedAt   *time.Time
	NextRetryAt  *time.Time
}

// OutboxRow Outbox 记录。
type OutboxRow struct {
	ID            int64
	TaskID        int64
	DispatchKey   string
	QueueName     string
	PayloadJSON   []byte
	Status        string
	AttemptCount  int16
	NextAttemptAt time.Time
	LeaseOwner    string
	LeaseExpiresAt *time.Time
}

// Store 调度引擎数据访问接口。
type Store interface {
	// ClaimForRun 原子领取：QUEUED→RUNNING（DB CAS 最终裁决；仅当 lease_owner 匹配自己）。
	// 返回 false 表示未抢到（其他 Worker 已领取）。
	ClaimForRun(ctx context.Context, taskID int64, leaseOwner string, leaseExpiresAt time.Time) (bool, error)

	// GetTask 按 ID 查任务。
	GetTask(ctx context.Context, taskID int64) (*Task, error)

	// RenewLeaseDB 续 DB 租约（心跳；仅 owner 匹配）。
	RenewLeaseDB(ctx context.Context, taskID int64, leaseOwner string, leaseExpiresAt time.Time) (bool, error)

	// CompleteTask 成功结算：RUNNING→SUCCESS + 结果。
	CompleteTask(ctx context.Context, taskID int64, leaseOwner string, resultJSON []byte, output string) error

	// FailTask 失败：RUNNING→FAILED_RETRYING（可重试）或 FAILED_DEAD（超限），并写重试日志。
	// 返回 (是否进入死信, 错误)。
	FailTask(ctx context.Context, taskID int64, leaseOwner string, reason string, retryCount int16, nextRetryAt time.Time) (dead bool, err error)

	// RequeueQueued 重试到点任务：FAILED_RETRYING(next_retry_at<=now) → QUEUED + 新 Outbox（同一事务）。
	// 返回需要入 Redis 的任务列表。
	RequeueDue(ctx context.Context, now time.Time) ([]*Task, error)

	// ClaimOutbox 领取待投递 Outbox（FOR UPDATE SKIP LOCKED）。
	ClaimOutbox(ctx context.Context, owner string, leaseExpiresAt time.Time) (*OutboxRow, error)

	// MarkOutboxPublished 投递成功（仅 owner 匹配）。
	MarkOutboxPublished(ctx context.Context, id int64, owner string) error

	// MarkOutboxFailed 投递失败：退避重试或转 DEAD。
	MarkOutboxFailed(ctx context.Context, id int64, owner string, errMsg string) error

	// ReapExpired 回收租约过期的 RUNNING 任务（→QUEUED，DB 恢复器；01 B.3.1 ④）。
	ReapExpired(ctx context.Context, now time.Time) ([]*Task, error)

	// EnqueueTask 提交任务入队：PENDING→QUEUED + Outbox（Worker 重试场景用，事务内）。
	EnqueueTask(ctx context.Context, taskID int64, queueName string) error
}
