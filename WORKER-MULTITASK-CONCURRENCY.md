# 🔄 Worker 多任务并发机制详解（基于源码）

## 📐 **总体架构：两级并发**

```
┌─────────────────────────────────────────────────────────┐
│                  Redis ZSet 优先级队列                    │
│        duanju:{env}:queue:tasks (score=到期时间+优先级)     │
│                                                         │
│   ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│   │ 任务 A   │ │ 任务 B   │ │ 任务 C   │ │ 任务 D   │  │
│   │ 剧本拆分  │ │ 视频生成  │ │ 剧本拆分  │ │ 资产生成  │  │
│   └──────────┘ └──────────┘ └──────────┘ └──────────┘  │
└─────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐
│  Worker 实例 #1   │ │  Worker 实例 #2   │ │  Worker 实例 #3   │
│  (split 角色)      │ │  (split 角色)      │ │  (video 角色)      │
│                   │ │                   │ │                   │
│ goroutine #1 ──→  │ │ goroutine #1 ──→  │ │ goroutine #1 ──→  │
│ goroutine #2 ──→  │ │ goroutine #2 ──→  │ │ goroutine #2 ──→  │
└───────────────────┘ └───────────────────┘ └───────────────────┘
```

---

## 1️⃣ **第一级并发：Worker 进程内部的多个 goroutine**

### **源码：cmd/worker/main.go**

```go
for i := 0; i < *concurrency; i++ {
    go worker.Run(ctx)  // 启动 N 个消费循环 goroutine
}
```

### **每个 goroutine 的执行循环 (worker.go Run)**

```go
func (w *Worker) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }
        if !w.processOnce(ctx) {   // 处理一个任务
            time.Sleep(500 * time.Millisecond)  // 没任务休眠 500ms
        }
    }
}
```

### **并发效果**

假设当前 Worker 配置 `-c 2`（2 个 goroutine）：

```
goroutine #1:  领取任务 A → 执行剧本拆分（调用 DeepSeek API，耗时 15 分钟）
goroutine #2:  领取任务 B → 执行另一个剧本拆分（同时调用 DeepSeek API）
                    ↑
              两个任务同时在进行！互不阻塞！
```

---

## 2️⃣ **取任务机制：Redis Lua 原子领取（防重复消费）**

### **源码：queue.go - claimLua**

```lua
-- 原子操作：找出到期任务 → 尝试加锁 → 锁成功则出队
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
```

### **为什么需要 Lua 原子操作？**

**问题**：多个 goroutine（或多个 Worker 实例）同时从队列取任务，如何保证同一任务不被两个消费者重复取走？

```
场景：2 个 goroutine 同时调用 Claim

goroutine #1:  ZRANGEBYSCORE → 找到任务 A
               SETNX lock:task:941 → ✅ 成功（获得锁）
               ZREM → 任务 A 出队 ✓

goroutine #2:  ZRANGEBYSCORE → 找到任务 A（尚未出队）
               SETNX lock:task:941 → ❌ 失败（锁被 #1 持有）
               → 继续找下一个任务 B
               SETNX lock:task:942 → ✅ 成功
               ZREM → 任务 B 出队 ✓
```

**结果**：每个任务只会被一个消费者领取，**天然防止重复执行**！

### **优先级排序**

```go
// 队列 score = eligible_at_ms*10 + (9-priority)
// priority 范围 0~9，数字越小优先级越高
score := float64(eligibleAt.UnixMilli())*10 + float64(9-priority)
```

**效果**：
- 任务按"可执行时间"排序（先到期先处理）
- 同等时间下，高优先级任务（priority=1）排在最前
- 例如：priority=1 → score = t*10+8；priority=9 → score = t*10+0

---

## 3️⃣ **DB 状态裁决：Redis 只是加速层，数据库才是最终裁决**

### **源码：worker.go - processOnce**

```go
// ① Redis Lua 原子取到期任务（加速层）
taskID, err := w.queue.Claim(ctx, w.owner, w.leaseTTL)

// ② DB CAS 最终裁决：QUEUED→RUNNING（数据库状态唯一裁决）
claimed, err := w.store.ClaimForRun(ctx, taskID, w.owner, time.Now().Add(w.leaseTTL))

if !claimed {
    // 其他 Worker 已领取：释放 Redis 锁
    w.queue.Release(ctx, taskID, w.owner)
    return false
}
```

### **双保险机制**

```
第一道保险（Redis）:  SETNX 锁防止并发领取
第二道保险（DB）:      UPDATE ... WHERE status='QUEUED' 
                      ↑ 如果状态已不是 QUEUED，UPDATE 影响行数为 0 → 领取失败
```

**即使 Redis 锁失效**（例如锁过期），DB 的 CAS 也会拦截重复执行！

---

## 4️⃣ **任务类型分发：Handler 注册机制**

### **源码：cmd/worker/main.go**

```go
// 按角色注册 Handler
var handlers []task.Handler

if *role == "split" {
    handlers = append(handlers, splitHandler)          // 剧本拆分
    handlers = append(handlers, storyboardPlanHandler) // 分镜计划
}
if *role == "split" || *role == "asset" {
    handlers = append(handlers, assetHandler)          // 资产生成
    handlers = append(handlers, shotRefHandler)        // 镜头参考图
}
if *role == "split" || *role == "video" {
    handlers = append(handlers, videoHandler)          // 视频生成
}
if *role == "split" || *role == "export" {
    handlers = append(handlers, exportHandler)         // 导出合成
}
```

### **Handler 匹配逻辑 (worker.go resolveHandler)**

```go
func (w *Worker) resolveHandler(taskType string) (Handler, bool) {
    // ① 精确匹配（task_type 完全一致）
    if h, ok := w.handlers[taskType]; ok {
        return h, true
    }
    // ② 模糊匹配（multiHandler 支持多种类型）
    for _, h := range w.handlers {
        if mh, ok := h.(multiHandler); ok && mh.Handles(taskType) {
            return h, true
        }
    }
    return nil, false
}
```

### **关键机制：能力不匹配时放回队列**

```go
// 无执行器：本 Worker 不具备该任务类型能力
// RUNNING→QUEUED 释放（不计重试、不判死信），等有执行器的 Worker 消费
if _, uerr := w.store.UnclaimTask(ctx, taskID, w.owner); uerr != nil {
    ...
}
w.queue.Requeue(ctx, taskID, time.Now().Add(10*time.Second), t.Priority)
```

**例子**：
```
video 角色的 Worker 抢到了 SCRIPT_SPLIT 任务
→ 发现自己没有 split Handler
→ 释放任务回队列（10 秒后重投）
→ 等待 split 角色的 Worker 来消费
```

---

## 5️⃣ **具体场景模拟**

### **场景：用户提交 3 个剧本拆分 + 2 个视频生成**

```
队列 (Redis ZSet):
┌──────────────────────────────────────┐
│ 任务 A: SCRIPT_SPLIT (priority=5)    │
│ 任务 B: SCRIPT_SPLIT (priority=5)    │
│ 任务 C: SCRIPT_SPLIT (priority=5)    │
│ 任务 D: VIDEO_SEGMENT_GEN (priority=5)│
│ 任务 E: VIDEO_GROUP_CONCAT (priority=5)│
└──────────────────────────────────────┘

Worker #1 (split 角色, -c 2):
├─ goroutine #1: Claim → 任务 A → 执行剧本拆分
├─ goroutine #2: Claim → 任务 B → 执行剧本拆分
└─ 两个任务并行执行（同时调用 DeepSeek API）

Worker #2 (video 角色, -c 2):
├─ goroutine #1: Claim → 任务 D → 执行视频生成
├─ goroutine #2: Claim → 任务 E → 执行视频生成
└─ 两个任务并行执行（同时调用视频 API）

任务 C 留在队列 → 等待 Worker #1 的 goroutine 完成后领取

时间线:
T+0s    A 执行中 (Worker1-G1)  D 执行中 (Worker2-G1)
        B 执行中 (Worker1-G2)  E 执行中 (Worker2-G2)
T+15分  A 完成 → G1 空闲 → 领取任务 C
        B 完成 → G2 空闲 → 无任务 → 休眠 500ms
T+30分  C 完成 → 全部任务完成
```

### **如果只有一个 Worker（split 角色）**

```
Worker #1 (split 角色, -c 2):
├─ goroutine #1: Claim → 任务 A → 剧本拆分
├─ goroutine #2: Claim → 任务 B → 剧本拆分
└─ 注意：split 角色注册了 videoHandler！

Worker #1 (split 角色) 的 handlers:
├─ splitHandler      → SCRIPT_SPLIT ✓
├─ assetHandler      → ASSET_* ✓  
├─ shotRefHandler    → SHOT_REF_IMAGE ✓
├─ videoHandler      → VIDEO_SEGMENT_GEN ✓
└─ exportHandler     → EXPORT_* ✓

所以任务 D/E 也会被这个 Worker 处理：
T+15分  A 完成 → G1 → 领取任务 C（或 D）
T+15分+ε B 完成 → G2 → 领取任务 D
```

---

## 6️⃣ **租约与心跳：长任务防重复**

### **问题：任务执行 15 分钟，如何防止其他 Worker 误以为"卡死"而重复领取？**

### **解决方案：租约 + 心跳续期**

```go
// 领取时设置租约（默认 60 秒）
claimed, err := w.store.ClaimForRun(ctx, taskID, w.owner, time.Now().Add(w.leaseTTL))

// 独立 goroutine 每 30 秒续租一次
func (w *Worker) heartbeat(ctx context.Context, taskID int64) {
    ticker := time.NewTicker(HeartbeatInterval)  // 30s
    for {
        select {
        case <-ticker.C:
            // 续 Redis 锁
            w.queue.RenewLease(ctx, taskID, w.owner, w.leaseTTL)
            // 续 DB 租约
            w.store.RenewLeaseDB(ctx, taskID, w.owner, expires)
        }
    }
}
```

### **租约机制流程**

```
T+0s     Worker#1 领取任务 A → 租约到期时间 = T+60s
T+30s    Worker#1 心跳 → 租约延长到 T+90s
T+60s    Worker#1 心跳 → 租约延长到 T+120s
...
T+15分   Worker#1 完成任务 → 释放锁 ✓

=====================================

万一 Worker#1 崩溃:
T+0s     Worker#1 领取任务 A → 租约 = T+60s
T+30s    Worker#1 崩溃！（没有心跳）
T+60s    租约过期！
T+90s    Reaper 扫描发现 → 回收任务 A → QUEUED
T+90s+ε  Worker#2 领取任务 A → 重新执行 ✓
```

**结果**：任务不会因为 Worker 崩溃而永久丢失！

---

## 7️⃣ **Dispatcher 与 Reaper：可靠投递与故障恢复**

### **Dispatcher（每 1 秒轮询 Outbox）**

```
API 创建任务 → 写入 DB + Outbox 表（outbox 未发布）
                  ↓
Dispatcher 每 1s 扫描 Outbox
                  ↓
领取未发布的 Outbox → Redis ZADD 入队 → 标记 PUBLISHED
```

**作用**：保证任务不会"丢了"——即使 Redis 入队失败，Outbox 还在 DB 里，下次重试。

### **Reaper（每 30 秒扫描）**

```
① 回收超租约任务：RUNNING 但租约过期 → QUEUED（Worker 崩溃恢复）
② 重试到点任务：FAILED_RETRYING 且 next_retry_at <= now → QUEUED
③ 卡死修复：QUEUED 超过 2 分钟 → 补发 Outbox + 重投 Redis
```

---

## 8️⃣ **任务内并发（剧本拆分的段 B 优化）**

### **不仅是多个任务并行，单个任务内部也并行！**

```go
const parallelEpisodeDesignConcurrency = 2

// 串行改并发：同时设计 2 集的分镜镜头
for _, ei := range epIdxList {
    sem <- struct{}{}  // 信号量控制并发=2
    go func(epIndex int, eps *[]EpisodeSplit) {
        defer func() { <-sem; done <- nil }()
        h.designEpisodeShots(ctx, epIndex, &(*eps)[epIndex])
    }(ei, &result.Episodes)
}
```

### **完整并发层次图**

```
┌────────────────────────────────────────────────────┐
│            多任务并发（Worker 级）                    │
│                                                    │
│  任务 A (拆分) ──→ goroutine #1                     │
│                    └─→ 段 A: 单次调用               │
│                    └─→ 段 B: 集1↔集2 并行 ←─┐        │
│                                             │        │
│  任务 B (拆分) ──→ goroutine #2             │        │
│                    └─→ 段 A: 单次调用       │        │
│                    └─→ 段 B: 集1↔集2 并行 ←─┼── 任务内│
│                                             │  并发  │
│  任务 D (视频) ──→ goroutine #3             │        │
│  任务 E (视频) ──→ goroutine #4             │        │
│                                             │        │
└─────────────────────────────────────────────┴────────┘

最大并发数:
  = Worker实例数 × 每实例goroutine数 × 任务内并发数
  = 2 × 2 × 2 = 8 个 DeepSeek API 同时调用（上限）
```

---

## 📊 **并发参数配置指南**

### **控制并发的方式**

```bash
# 1. 单 Worker 并发数（goroutine 数）
go run ./cmd/worker -role split -c 2    # 2 个 goroutine
go run ./cmd/worker -role split -c 4    # 4 个 goroutine

# 2. 多 Worker 实例
# 终端 1
go run ./cmd/worker -role split -c 2
# 终端 2  
go run ./cmd/worker -role split -c 2
# 总并发 = 2 + 2 = 4

# 3. 任务内并发（代码内常量）
const parallelEpisodeDesignConcurrency = 2  # 单任务内同时设计 2 集
```

### **不同角色的默认配置建议**

| 角色 | 建议并发 | 原因 |
|------|---------|------|
| split | 2 | LLM 调用昂贵，避免 API 限流 |
| asset | 4 | 图片生成 API 并发上限高 |
| video | 2 | 视频生成耗时，单任务内部已并行 |

---

## ⚠️ **注意事项**

### **1. 并发过高 → API 限流**

```
触发 429 错误: rate limit exceeded
→ Worker 重试 3 次后进入 FAILED_DEAD
→ 需要降低并发数
```

### **2. 同一任务不会重复执行**

- Redis Lua 锁：`SETNX leaseKey:taskID owner EX 60`
- DB CAS：`UPDATE ... WHERE status='QUEUED'`
- 双重保险，即使一个失效另一个也能拦截

### **3. 长任务不会丢失**

- 心跳续租：每 30 秒续期，防止 Reaper 误回收
- Worker 崩溃：租约过期 → Reaper 回收 → 重新排队执行

### **4. 不同类型的任务可以混合处理**

- split 角色的 Worker 注册了所有 Handler（split/asset/video/export）
- 一个 Worker 可以处理任意类型的任务
- 但如果任务类型不匹配，会安全放回队列等待正确 Worker

---

## 🎯 **总结**

| 问题 | 答案 |
|------|------|
| **多个任务如何并发？** | 每个 Worker 内部启动 N 个 goroutine，各自独立循环取任务执行 |
| **如何防止重复消费？** | Redis Lua 原子锁 + DB CAS 双重裁决 |
| **如何保证优先级？** | ZSet score = 到期时间*10 + 优先级，先到期/高优先级先出队 |
| **长任务如何处理？** | 租约 + 每 30s 心跳续期，崩溃后 Reaper 回收重投 |
| **不同类型任务？** | Handler 按 task_type 分发，无执行器则放回队列 |
| **任务内并行？** | 剧本拆分段 B 使用信号量并发设计多集镜头（2 集同时） |
