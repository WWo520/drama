// 调度引擎测试：并发取任务无重复（Lua 原子性）+ 入队幂等 + 宕机回收（S1-B04 验收）。
package task

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testQueue 连接本地 Redis（docker compose duanju-redis:6380）；env 唯一避免测试间锁 key 冲突。
func testQueue(t *testing.T) *Queue {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6380", DB: 1})
	require.NoError(t, rdb.Ping(context.Background()).Err())
	t.Cleanup(func() { _ = rdb.Close() })
	env := "test-" + fmt.Sprint(time.Now().UnixNano())
	return NewQueue(rdb, env)
}

func TestEnqueueIdempotent(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	at := time.Now()
	// 重复入队同任务（同 score）→ 幂等，ZSET 只保留 1 个 member
	require.NoError(t, q.Enqueue(ctx, 42, at, 5))
	require.NoError(t, q.Enqueue(ctx, 42, at, 5))
	card := q.rdb.ZCard(ctx, q.queueKey()).Val()
	assert.Equal(t, int64(1), card, "重复入队应幂等")
}

func TestClaimConcurrentNoDuplicate(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	const n = 10
	// 入队 10 个任务（不同 score 模拟不同优先级/时间）
	for i := int64(1); i <= n; i++ {
		require.NoError(t, q.Enqueue(ctx, i, time.Now(), 5))
	}
	// 12 个并发消费者各 Claim 10 次
	claimed := make(chan int64, n*12)
	var wg sync.WaitGroup
	for w := 0; w < 12; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < n; i++ {
				id, err := q.Claim(ctx, "w"+string(rune(worker)), 60*time.Second)
				require.NoError(t, err)
				if id != 0 {
					claimed <- id
				}
			}
		}(w)
	}
	wg.Wait()
	close(claimed)

	seen := map[int64]bool{}
	count := 0
	for id := range claimed {
		require.False(t, seen[id], "任务 %d 被重复领取（Lua 原子性失效）", id)
		seen[id] = true
		count++
	}
	assert.Equal(t, n, count, "应恰好领取 %d 个不同任务", n)
	assert.Equal(t, n, len(seen))
	// 队列应已清空
	assert.Equal(t, int64(0), q.rdb.ZCard(ctx, q.queueKey()).Val())
}

func TestClaimSkipsLockedTask(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	require.NoError(t, q.Enqueue(ctx, 1, time.Now(), 5))
	require.NoError(t, q.Enqueue(ctx, 2, time.Now(), 5))
	// 第一个 Worker 拿到 task 1（持锁 60s）
	id1, err := q.Claim(ctx, "w1", 60*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(1), id1)
	// 第二个 Worker 只能拿到 task 2（task 1 被锁）
	id2, err := q.Claim(ctx, "w2", 60*time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(2), id2)
	// 第三个 Worker 无任务可取
	id3, err := q.Claim(ctx, "w3", 60*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(0), id3)
}

// fakeStore 最小 Store：记录 Requeue 调用（宕机回收路径）。
type fakeStore struct {
	reaped     []*Task
	requeueIDs []int64
}

func (f *fakeStore) ReapExpired(_ context.Context, _ time.Time) ([]*Task, error) {
	return f.reaped, nil
}
func (f *fakeStore) RequeueDue(_ context.Context, _ time.Time) ([]*Task, error) { return nil, nil }
func (f *fakeStore) ClaimForRun(context.Context, int64, string, time.Time) (bool, error) {
	return true, nil
}
func (f *fakeStore) GetTask(context.Context, int64) (*Task, error) { return &Task{}, nil }
func (f *fakeStore) RenewLeaseDB(context.Context, int64, string, time.Time) (bool, error) {
	return true, nil
}
func (f *fakeStore) CompleteTask(context.Context, int64, string, []byte, string) error { return nil }
func (f *fakeStore) FailTask(context.Context, int64, string, string, int16, time.Time) (bool, error) {
	return false, nil
}
func (f *fakeStore) ClaimOutbox(context.Context, string, time.Time) (*OutboxRow, error) { return nil, nil }
func (f *fakeStore) MarkOutboxPublished(context.Context, int64, string) error          { return nil }
func (f *fakeStore) MarkOutboxFailed(context.Context, int64, string, string) error     { return nil }
func (f *fakeStore) EnqueueTask(context.Context, int64, string) error                  { return nil }

// 用真实 Redis 记录 Requeue（重写 Queue.Requeue 调用点不易 mock，这里验证 Reaper 逻辑：
// 过期任务 → Requeue 入队（score 新时间）→ 可再次被 Claim）。
func TestReaperExpiredRequeue(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	// 模拟：过期任务 99 已由 store.ReapExpired 返回
	fake := &fakeStore{reaped: []*Task{{ID: 99, Priority: 5}}}
	reaper := NewReaper(fake, q)
	reaper.reapOnce(ctx)

	// 99 应已重新入队并被 Claim 到
	id, err := q.Claim(ctx, "w-reaper", 60*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(99), id, "宕机回收后任务应可被重新领取")
}

func TestBackoffSequence(t *testing.T) {
	assert.Equal(t, 15, BackoffFor(1))
	assert.Equal(t, 60, BackoffFor(2))
	assert.Equal(t, 180, BackoffFor(3))
	assert.Equal(t, 180, BackoffFor(4), "超过 3 次封顶 180s")
}
