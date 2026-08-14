// 任务状态机与退避策略（03 E.3 / 01 B.3.1 ⑤）。
package task

import "time"

// 任务状态（02 A4 generation_status_enum 唯一真源）。
const (
	StatusPending        = "PENDING"
	StatusQueued         = "QUEUED"
	StatusRunning        = "RUNNING"
	StatusSuccess        = "SUCCESS"
	StatusFailedRetrying = "FAILED_RETRYING"
	StatusFailedDead     = "FAILED_DEAD"
	StatusCanceledUser   = "CANCELED_USER"
	StatusCanceledTimeout = "CANCELED_TIMEOUT"
	StatusSkipped        = "SKIPPED"
)

// BackoffSeconds 指数退避序列（01 B.3.1 ⑤：15s/60s/180s）。
var BackoffSeconds = []int{15, 60, 180}

// MaxRetry 最大重试次数（03 E.3：重试 3 次仍 DEAD）。
const MaxRetry = 3

// BackoffFor 第 retryCount 次失败后的退避秒数（retryCount 从 1 开始）。
func BackoffFor(retryCount int) int {
	idx := retryCount - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(BackoffSeconds) {
		idx = len(BackoffSeconds) - 1
	}
	return BackoffSeconds[idx]
}

// NextRetryAt 下次重试时间。
func NextRetryAt(retryCount int) time.Time {
	return time.Now().Add(time.Duration(BackoffFor(retryCount)) * time.Second)
}

// LeaseTTL 运行租约（60s 回收；01 B.3.1 ④）。
const LeaseTTL = 60 * time.Second

// HeartbeatInterval Worker 心跳/续租间隔（30s）。
const HeartbeatInterval = 30 * time.Second

// ReapInterval 恢复器扫描间隔。
const ReapInterval = 30 * time.Second
