// Reaper：租约到期回收 + 重试到点重投（01 B.3.1 ④ 恢复器；DB 状态唯一裁决）。
package task

import (
	"context"
	"log"
	"time"
)

// Reaper 后台恢复器。
type Reaper struct {
	store Store
	queue *Queue
}

func NewReaper(store Store, queue *Queue) *Reaper {
	return &Reaper{store: store, queue: queue}
}

// Run 主循环：每 30s 扫描两类恢复。
func (r *Reaper) Run(ctx context.Context) {
	ticker := time.NewTicker(ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reapOnce(ctx)
		}
	}
}

func (r *Reaper) reapOnce(ctx context.Context) {
	now := time.Now()
	// ① 宕机回收：RUNNING 且租约过期 → QUEUED + 新 Outbox（Worker 崩溃恢复）
	expired, err := r.store.ReapExpired(ctx, now)
	if err != nil {
		log.Printf("reaper: 回收失败: %v", err)
	} else {
		for _, t := range expired {
			// 重新入 Redis（score 按当前时间；B4.1a 的 Outbox 由 Dispatcher 重投，此处双保险）
			_ = r.queue.Requeue(ctx, t.ID, time.Now(), t.Priority)
		}
		if len(expired) > 0 {
			log.Printf("reaper: 回收 %d 个超租约任务", len(expired))
		}
	}

	// ② 重试到点：FAILED_RETRYING(next_retry_at<=now) → QUEUED + 新 Outbox
	due, err := r.store.RequeueDue(ctx, now)
	if err != nil {
		log.Printf("reaper: 重试重投失败: %v", err)
	} else {
		for _, t := range due {
			_ = r.queue.Requeue(ctx, t.ID, time.Now(), t.Priority)
		}
		if len(due) > 0 {
			log.Printf("reaper: 重投 %d 个到点重试任务", len(due))
		}
	}

	// ③ 卡死修复（08 审查 Major#14）：QUEUED 超过 2 分钟未被消费 → 补发 Outbox + 重投 Redis
	stuck, err := r.store.RequeueStuckQueued(ctx, now.Add(-2*time.Minute))
	if err != nil {
		log.Printf("reaper: 卡死 QUEUED 修复失败: %v", err)
	} else {
		for _, t := range stuck {
			_ = r.queue.Requeue(ctx, t.ID, time.Now(), t.Priority)
		}
		if len(stuck) > 0 {
			log.Printf("reaper: 修复 %d 个卡死 QUEUED 任务", len(stuck))
		}
	}
}
