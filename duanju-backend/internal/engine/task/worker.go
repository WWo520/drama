// Worker 循环：Claim → DB CAS 领取 → 执行 → 心跳续租 → 成功/失败结算（01 B.3.1 ③④⑤）。
package task

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

// Handler 任务执行器（按 task_type 分发；由 Worker 注册方实现，如 Worker-Split）。
type Handler interface {
	// Name 任务类型（task_type_enum 值）。
	Name() string
	// Execute 执行任务，返回结果 JSON（成功时）或错误。
	Execute(ctx context.Context, t *Task) (resultJSON []byte, err error)
}

// multiHandler 可选接口：一个 Handler 处理多种任务类型（如 Worker-Asset 三种图生成）。
type multiHandler interface {
	Handles(taskType string) bool
}

// resolveHandler 按任务类型解析执行器（先精确匹配 Name，再尝试 multiHandler）。
func (w *Worker) resolveHandler(taskType string) (Handler, bool) {
	if h, ok := w.handlers[taskType]; ok {
		return h, true
	}
	for _, h := range w.handlers {
		if mh, ok := h.(multiHandler); ok && mh.Handles(taskType) {
			return h, true
		}
	}
	return nil, false
}

// Worker 单 Worker 消费循环（concurrency 控制由调用方起多个 goroutine）。
type Worker struct {
	store    Store
	queue    *Queue
	owner    string
	handlers map[string]Handler
	leaseTTL time.Duration
	// Settler 任务结算回调（S1-B09 计费集成：成功扣费/失败解冻）
	Settler func(ctx context.Context, taskID int64, deliverable bool) error
}

func NewWorker(store Store, queue *Queue, handlers []Handler) *Worker {
	m := make(map[string]Handler, len(handlers))
	for _, h := range handlers {
		m[h.Name()] = h
	}
	return &Worker{
		store:    store,
		queue:    queue,
		owner:    "worker-" + uuid.NewString()[:8],
		handlers: m,
		leaseTTL: LeaseTTL,
	}
}

// Run 单消费循环：取任务→执行→结算；无任务时休眠 500ms。
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !w.processOnce(ctx) {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) bool {
	// ① Redis Lua 原子取到期任务（ZRANGEBYSCORE + SETNX 锁 + ZREM）
	taskID, err := w.queue.Claim(ctx, w.owner, w.leaseTTL)
	if err != nil || taskID == 0 {
		return false
	}
	// ② DB CAS 最终裁决：QUEUED→RUNNING（Redis 锁只是加速层，DB 状态唯一裁决）
	claimed, err := w.store.ClaimForRun(ctx, taskID, w.owner, time.Now().Add(w.leaseTTL))
	if err != nil {
		w.queue.Release(ctx, taskID, w.owner)
		return false
	}
	if !claimed {
		// 其他 Worker 已领取：释放 Redis 锁（仅 owner 匹配才删，安全）
		w.queue.Release(ctx, taskID, w.owner)
		return false
	}

	t, err := w.store.GetTask(ctx, taskID)
	if err != nil {
		w.queue.Release(ctx, taskID, w.owner)
		return false
	}

	// ③ 心跳续租（独立 goroutine，30s 间隔；Redis 锁 + DB 租约同时续）
	heartbeatCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go w.heartbeat(heartbeatCtx, taskID)

	// ④ 执行
	h, ok := w.resolveHandler(t.TaskType)
	if !ok {
		// 无执行器：按失败处理（避免任务永久卡死）
		retryCount := t.RetryCount + 1
		dead, ferr := w.store.FailTask(ctx, taskID, w.owner, "no handler for task_type: "+t.TaskType, retryCount, NextRetryAt(int(retryCount)))
		if ferr != nil {
			log.Printf("worker: task %d no-handler FailTask 失败: %v", taskID, ferr)
		} else if dead {
			_ = w.queue.PushDLQ(ctx, taskID, "no handler")
			log.Printf("worker: task %d 无执行器进入死信", taskID)
		} else {
			log.Printf("worker: task %d 无执行器失败待重试(retry=%d)", taskID, retryCount)
		}
		w.queue.Release(ctx, taskID, w.owner)
		return true
	}
	result, execErr := h.Execute(heartbeatCtx, t)

	if execErr != nil {
		// ⑤ 失败：指数退避或死信（≥3 次转 FAILED_DEAD + DLQ）
		dead, ferr := w.store.FailTask(ctx, taskID, w.owner, execErr.Error(), t.RetryCount+1, NextRetryAt(int(t.RetryCount)+1))
		if ferr == nil {
			if dead {
				_ = w.queue.PushDLQ(ctx, taskID, execErr.Error())
				log.Printf("worker: task %d 进入死信: %v", taskID, execErr)
				// 计费结算：失败全额解冻（永不倒扣）
				if w.Settler != nil {
					if serr := w.Settler(ctx, taskID, false); serr != nil {
						log.Printf("worker: task %d 失败解冻失败: %v", taskID, serr)
					}
				}
			} else {
				// 退避后重试：转 QUEUED + 新 Outbox（同一事务已由 FailTask 预留 next_retry_at；
				// RequeueDue 扫描到点后自动重投）
				log.Printf("worker: task %d 失败待重试(retry=%d): %v", taskID, t.RetryCount+1, execErr)
			}
		}
	} else {
		_ = w.store.CompleteTask(ctx, taskID, w.owner, result, "")
		// 计费结算：成功扣费（幂等：Settle 内部防重）
		if w.Settler != nil {
			if serr := w.Settler(ctx, taskID, true); serr != nil {
				log.Printf("worker: task %d 计费结算失败: %v", taskID, serr)
			}
		}
	}
	w.queue.Release(ctx, taskID, w.owner)
	return true
}

// heartbeat 心跳续租：30s 间隔，续 Redis 锁与 DB 租约（01 B.3.1 ④）。
func (w *Worker) heartbeat(ctx context.Context, taskID int64) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expires := time.Now().Add(w.leaseTTL)
			_ = w.queue.RenewLease(ctx, taskID, w.owner, w.leaseTTL)
			_, _ = w.store.RenewLeaseDB(ctx, taskID, w.owner, expires)
		}
	}
}
