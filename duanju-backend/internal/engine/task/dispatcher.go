// Dispatcher：Outbox → Redis ZADD 可靠投递（B4.1a 发布协议；01 B.3.1 ①）。
package task

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

// Dispatcher 独立后台协程：以短租约领取 Outbox 后执行 Redis ZADD。
type Dispatcher struct {
	store Store
	queue *Queue
	owner string
}

func NewDispatcher(store Store, queue *Queue) *Dispatcher {
	return &Dispatcher{
		store: store,
		queue: queue,
		owner: "dispatcher-" + uuid.NewString()[:8],
	}
}

// Run 主循环：每 1s 轮询一次 Outbox。
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.dispatchOnce(ctx)
		}
	}
}

func (d *Dispatcher) dispatchOnce(ctx context.Context) {
	o, err := d.store.ClaimOutbox(ctx, d.owner, time.Now().Add(30*time.Second))
	if err != nil {
		log.Printf("dispatcher: ClaimOutbox 失败: %v", err)
		return
	}
	if o == nil {
		return
	}
	// 解析 payload 的 eligible_at_ms/priority（payload_json 为最小投递快照）
	var eligibleAt time.Time
	priority := 5
	// 简化：当前 Outbox payload 不含完整字段时用立即投递；B05 起携带完整快照
	eligibleAt = time.Now()

	// Redis Lua ZADD（member=task_id，重复调用幂等）
	if err := d.queue.Enqueue(ctx, o.TaskID, eligibleAt, priority); err != nil {
		log.Printf("dispatcher: ZADD 失败 task=%d err=%v", o.TaskID, err)
		_ = d.store.MarkOutboxFailed(ctx, o.ID, d.owner, err.Error())
		return
	}
	// 仅 owner 匹配写 PUBLISHED + generation_tasks.enqueued_at（B4.1a 协议）
	if err := d.store.MarkOutboxPublished(ctx, o.ID, d.owner); err != nil {
		log.Printf("dispatcher: MarkPublished 失败 task=%d err=%v", o.TaskID, err)
	}
}
