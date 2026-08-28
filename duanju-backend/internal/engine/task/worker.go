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
	// Notifier 任务终态通知回调（可选；WS 推送 TASK_SUCCESS/TASK_FAILED，轮询对齐 LuxReal 的 WS 驱动基础）
	Notifier func(ctx context.Context, t *Task, status string, reason string)
	// OnDead 业务对象终败归位回调（可选）：任务进入 FAILED_DEAD 死信时，按任务类型
	// 回写业务实体状态（如成片卡 RUNNING 僵尸 → FAILED_DEAD），否则前端永远显示"生成中"。
	OnDead func(ctx context.Context, t *Task, reason string)
	// OnTaskStart 任务开始执行回调（可选）：Handler 借此感知当前任务 ID 上报进度
	// （handler 为长生命周期单例，进度闭包需从回调取 taskID 而非构造期绑定）。
	OnTaskStart func(t *Task)
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
		// 08 审查 Major#14：DB 错误时任务回 Redis 队列（不丢弃），避免永久卡 QUEUED
		w.queue.Release(ctx, taskID, w.owner)
		if rerr := w.queue.Requeue(ctx, taskID, time.Now().Add(5*time.Second), 5); rerr != nil {
			log.Printf("worker: task %d DB 错误且回队失败: %v（等待 Reaper 卡死修复）", taskID, rerr)
		}
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

	// 进度上报基点（Handler 通过回调感知当前任务 ID）
	if w.OnTaskStart != nil {
		w.OnTaskStart(t)
	}

	// ④ 执行
	h, ok := w.resolveHandler(t.TaskType)
	if !ok {
		// 无执行器：本 Worker 不具备该任务类型能力（如 asset 角色抢到 SCRIPT_SPLIT）。
		// 能力不匹配不是任务错误：RUNNING→QUEUED 释放（不计重试、不判死信），等有执行器的 Worker 消费；
		// 否则无执行器的 Worker 会把好任务重试 3 次刷成 FAILED_DEAD，且项目状态无人回滚永久卡死。
		if _, uerr := w.store.UnclaimTask(ctx, taskID, w.owner); uerr != nil {
			// DB 释放失败：仅释放 Redis 锁，靠租约过期由 Reaper 回收（Major#14 双保险）
			log.Printf("worker: task %d 无执行器 DB 释放失败(%v)，等待租约过期回收", taskID, uerr)
		}
		// 延迟 10s 重入 Redis，避免本 Worker 立即重复领取
		if rerr := w.queue.Requeue(ctx, taskID, time.Now().Add(10*time.Second), t.Priority); rerr != nil {
			log.Printf("worker: task %d 无执行器重入队失败(%v)，等待 Outbox 重投", taskID, rerr)
		}
		w.queue.Release(ctx, taskID, w.owner)
		log.Printf("worker: task %d 类型 %s 本 Worker 无执行器，已释放回队列等待可执行 Worker", taskID, t.TaskType)
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
				// 终态通知（WS 推送 TASK_FAILED，前端据此刷新并停止轮询）
				if w.Notifier != nil {
					w.Notifier(ctx, t, StatusFailedDead, execErr.Error())
				}
				// 业务对象终败归位（成片卡/镜头 RUNNING 僵尸 → FAILED 态）
				if w.OnDead != nil {
					w.OnDead(ctx, t, execErr.Error())
				}
			} else {
				// 退避后重试：转 QUEUED + 新 Outbox（同一事务已由 FailTask 预留 next_retry_at；
				// RequeueDue 扫描到点后自动重投）
				log.Printf("worker: task %d 失败待重试(retry=%d): %v", taskID, t.RetryCount+1, execErr)
			}
		}
	} else {
		// 08 审查 Major#13：CompleteTask 失败（DB/租约丢失）→ 跳 Settler，不结算
		errComplete := w.store.CompleteTask(ctx, taskID, w.owner, result, "")
		if errComplete != nil {
			log.Printf("worker: task %d 完成失败 (err=%v)，跳过计费结算", taskID, errComplete)
			w.queue.Release(ctx, taskID, w.owner)
			return true
		}
		// 计费结算：成功扣费（幂等：Settle 内部防重）
		if w.Settler != nil {
			if serr := w.Settler(ctx, taskID, true); serr != nil {
				log.Printf("worker: task %d 计费结算失败：%v", taskID, serr)
			}
		}
		// 终态通知（WS 推送 TASK_SUCCESS，前端据此刷新并停止轮询）
		if w.Notifier != nil {
			w.Notifier(ctx, t, StatusSuccess, "")
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
			// 08 审查 Major#13:心跳续租失败仅记录（Reaper 会回收超租约 RUNNING 任务回 QUEUED）
			w.queue.RenewLease(ctx, taskID, w.owner, w.leaseTTL)
			if _, err := w.store.RenewLeaseDB(ctx, taskID, w.owner, expires); err != nil {
				log.Printf("worker: task %d 心跳续租失败：%v，等待 Reaper 回收", taskID, err)
			}
		}
	}
}
