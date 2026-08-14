// Package task 调度引擎（E1）：Redis ZSet 优先级队列 + Lua 原子取任务 + 租约续期 + 指数退避 + 死信。
// 对齐 01 B.3.1（5 个核心机制）/ 04 D 章。DB 状态是唯一裁决，Redis 仅为加速层。
package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// 队列与锁 Key（02 E3.5 命名规范）。
const (
	// queueKey Redis ZSet：score = eligible_at_ms*10 + (9-priority)（01 B.3.1 ②）
	queueKeyFmt  = "duanju:%s:queue:tasks"
	dlqKeyFmt    = "duanju:%s:queue:dlq"
	leaseKeyFmt  = "duanju:%s:lock:task:%d"
)

// Queue 优先级队列（Redis ZSet）。
type Queue struct {
	rdb *redis.Client
	env string
}

func NewQueue(rdb *redis.Client, env string) *Queue {
	return &Queue{rdb: rdb, env: env}
}

// enqueueLua 原子入队：ZADD member=taskID, score=eligible*10+(9-priority)。
// member 固定为 task_id，相同 score/member 重复调用幂等（01 B.3.1 ①）。
var enqueueLua = redis.NewScript(`
local key = KEYS[1]
local member = ARGV[1]
local score = tonumber(ARGV[2])
redis.call('ZADD', key, score, member)
return 1
`)

// Enqueue 入队（幂等：重复 ZADD 同 member 同 score 无副作用）。
func (q *Queue) Enqueue(ctx context.Context, taskID int64, eligibleAt time.Time, priority int) error {
	score := float64(eligibleAt.UnixMilli())*10 + float64(9-priority)
	return enqueueLua.Run(ctx, q.rdb, []string{q.queueKey()}, fmt.Sprint(taskID), fmt.Sprint(score)).Err()
}

// claimLua 原子取到期任务：ZRANGEBYSCORE 取最早到期 → 尝试 SETNX 锁（60s）。
// 两步合并为 Lua 原子执行，杜绝两个 Worker 拿到同一任务（01 B.3.1 ③）。
// 锁成功 → ZREM 出队；锁失败 → 返回空（任务留在队列，由锁持有者消费）。
var claimLua = redis.NewScript(`
local queueKey = KEYS[1]
local leaseKey = KEYS[2]
local now = tonumber(ARGV[1])
local leaseOwner = ARGV[2]
local lockTTL = tonumber(ARGV[3])

local members = redis.call('ZRANGEBYSCORE', queueKey, '-inf', now)
for i = 1, #members do
  local member = members[i]
  local ok = redis.call('SET', leaseKey .. ':' .. member, leaseOwner, 'NX', 'EX', lockTTL)
  if ok then
    redis.call('ZREM', queueKey, member)
    return {member, '1'}
  end
end
return {'', '0'}
`)

// Claim 原子取一个到期任务（返回 taskID；无任务返回 0）。
func (q *Queue) Claim(ctx context.Context, leaseOwner string, lockTTL time.Duration) (int64, error) {
	// score = eligible_at_ms*10 + (9-priority)，priority 范围 0~9 → 到期上界 = now_ms*10 + 9
	now := time.Now().UnixMilli()*10 + 9
	res, err := claimLua.Run(ctx, q.rdb, []string{q.queueKey(), q.leaseKeyPrefix()},
		fmt.Sprint(now), leaseOwner, fmt.Sprint(int(lockTTL.Seconds()))).Slice()
	if err != nil {
		return 0, err
	}
	if len(res) != 2 || res[0] == "" {
		return 0, nil
	}
	var id int64
	if _, err := fmt.Sscan(res[0].(string), &id); err != nil {
		return 0, err
	}
	return id, nil
}

// RenewLease 续租（Worker 每 30s；01 B.3.1 ④：Redis 锁 + DB 租约同时续）。
func (q *Queue) RenewLease(ctx context.Context, taskID int64, leaseOwner string, ttl time.Duration) error {
	// Lua 原子续租：仅 owner 匹配才续
	return q.rdb.Eval(ctx, `
local val = redis.call('GET', KEYS[1])
if val == ARGV[1] then
  return redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return 0`, []string{q.leaseKey(taskID)}, leaseOwner, fmt.Sprint(int(ttl.Seconds()))).Err()
}

// Release 释放锁（仅 owner 匹配，Lua 原子 DEL——02 E2 lock 规范）。
func (q *Queue) Release(ctx context.Context, taskID int64, leaseOwner string) error {
	return q.rdb.Eval(ctx, `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`, []string{q.leaseKey(taskID)}, leaseOwner).Err()
}

// Requeue 任务失败重试/宕机回收后重新入队（score 按新 eligibleAt 计算）。
func (q *Queue) Requeue(ctx context.Context, taskID int64, eligibleAt time.Time, priority int) error {
	return q.Enqueue(ctx, taskID, eligibleAt, priority)
}

// PushDLQ 死信（≥3 次失败；管理端手动重跑）。
func (q *Queue) PushDLQ(ctx context.Context, taskID int64, reason string) error {
	return q.rdb.ZAdd(ctx, q.dlqKey(), redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: fmt.Sprintf("%d|%s", taskID, reason),
	}).Err()
}

func (q *Queue) queueKey() string        { return fmt.Sprintf(queueKeyFmt, q.env) }
func (q *Queue) dlqKey() string          { return fmt.Sprintf(dlqKeyFmt, q.env) }
func (q *Queue) leaseKeyPrefix() string  { return fmt.Sprintf("duanju:%s:lock:task", q.env) }
func (q *Queue) leaseKey(id int64) string { return fmt.Sprintf(leaseKeyFmt, q.env, id) }

// ErrTaskNotFound 队列中无任务。
var ErrTaskNotFound = errors.New("task not found in queue")
