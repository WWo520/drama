# 🚀 Worker 并行工作机制详解

## 📊 **当前状态**

根据查询结果：**当前没有 Worker 进程在运行！**

你看到的 PID 23080 的 worker.exe 已经不存在了，只有 API 进程 (PID 24568) 在运行。

---

## 🔧 **Worker 架构设计**

### **1. 多角色分离**

Worker 支持 4 种不同的角色：

```powershell
# start-worker.ps1 -role split | asset | video | export | all
-role split    # 剧本拆分 Worker
-role asset    # 资产生成 Worker  
-role video    # 视频生成 Worker
-role export   # 导出合成 Worker
-role all      # 启动所有 4 个 Worker
```

每个 Worker 只处理特定类型的任务，互不干扰。

---

### **2. 单 Worker 内部并发机制**

#### **关键配置参数**

```go
// cmd/worker/main.go
role := flag.String("role", "split", "worker 角色")
concurrency := flag.Int("c", 2, "消费并发数") // ← 核心参数！
```

- **默认值**: `--c 2` = **2 个 goroutine 同时工作**
- **可调整**: 可以手动指定 `--c 4` 或更高

#### **并发执行原理**

```go
// 第 170 行左右
for i := 0; i < *concurrency; i++ {
    go worker.Run(ctx)
}
```

**工作流程:**

```
Worker (角色=split, 并发数=2)
├── goroutine #1 ──→ 监听队列 → 读取任务 → 执行 split
├── goroutine #2 ──→ 监听队列 → 读取任务 → 执行 split
└── 共享资源：
    ├── PostgreSQL 连接池
    ├── Redis 连接池
    └── LLM API 客户端
```

---

### **3. 队列消费模式**

#### **Redis 队列结构**

```redis
Queue: task_queue:split
├── Task #320 (status=RUNNING, priority=5)
├── Task #321 (status=QUEUED, priority=5)
└── Task #322 (status=QUEUED, priority=5)
```

#### **消费流程**

```go
func (w *Worker) Run(ctx context.Context) {
    for {
        // 1. 从 Redis 队列读取任务
        task := queue.Pop(ctx, w.role)
        
        if task == nil {
            time.Sleep(5 * time.Second) // 无任务时休眠
            continue
        }
        
        // 2. 更新任务状态为 RUNNING
        queue.MarkRunning(task.ID)
        
        // 3. 调用对应的 Handler
        handler := w.getHandler(task.TaskType)
        result, err := handler.Execute(ctx, task)
        
        // 4. 完成/失败处理
        if err != nil {
            queue.Fail(task.ID, err)
        } else {
            queue.Complete(task.ID, result)
        }
    }
}
```

---

### **4. 剧本拆分任务的并行优化**

#### **段 A: 结构拆分 (串行)**

```go
// Execute() 方法
// ✅ 单次 API 调用
resp, _ := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
    Prompt: params.ScriptText,  // 整个剧本
    MaxTokens: 16384,
    JSONMode: true,
    NoThinking: true,
})
```

**特点**:
- 每任务 1 次调用
- 不可进一步并行化

---

#### **段 B: 镜头设计 (原串行 → 现并行 ⭐优化)**

**之前（串行）** ❌
```go
totalEps := len(result.Episodes)
for ei := range result.Episodes {
    // 等待上一集完成才开始下一集
    if err := h.designEpisodeShots(ctx, ei, &result.Episodes[ei]); err != nil {
        return nil, err
    }
}
```
**耗时**: N 集 × 单集耗时

---

**现在（并行）** ✅
```go
const parallelEpisodeDesignConcurrency = 2  // 同时处理 2 集

done := make(chan error, totalEps)
sem := make(chan struct{}, parallelEpisodeDesignConcurrency)

var epIdxList []int
for ei := range result.Episodes {
    epIdxList = append(epIdxList, ei)
}

// 并发启动 goroutine
for _, ei := range epIdxList {
    sem <- struct{}{}  // 获取令牌
    go func(epIndex int, eps *[]EpisodeSplit) {
        defer func() { <-sem; done <- nil }()
        
        h.report(..., fmt.Sprintf("正在设计第 %d/%d 集...", epIndex+1, totalEps))
        if err := h.designEpisodeShots(ctx, epIndex, &(*eps)[epIndex]); err != nil {
            done <- err
        }
    }(ei, &result.Episodes)
}

// 等待所有完成
for range epIdxList {
    if err := <-done; err != nil {
        return nil, err
    }
}
```

**耗时**: max(N 集 ÷ 2, 1) × 单集耗时

---

### **5. 实际性能对比**

#### **场景：5 集剧本拆分**

假设每集需要 5 分钟

| 阶段 | 旧方案 (串行) | 新方案 (并行=2) | 加速比 |
|------|-------------|---------------|--------|
| 段 A | 2 分钟 | 2 分钟 | 1x |
| 段 B | 5 集×5 分=25 分 | 3 集×5 分=15 分 | 1.67x |
| 段 C | 异步后台 | 异步后台 | - |
| 段 D | 4 并发 | 4 并发 | - |
| **总计** | **~30 分钟** | **~20 分钟** | **1.5x** |

#### **长剧本场景：10 集**

| 阶段 | 旧方案 | 新方案 | 加速比 |
|------|-------|-------|--------|
| 段 B | 10 集×5 分=50 分 | 5 集×5 分=25 分 | **2x** |

---

## 🎯 **多个 Worker 协同工作**

### **多 Worker 场景**

如果你同时启动了 3 个 Worker：

```bash
# Worker 1: 剧本拆分 (并发=2)
go run ./cmd/worker -role split -c 2

# Worker 2: 资产生成 (并发=4)  
go run ./cmd/worker -role asset -c 4

# Worker 3: 视频生成 (并发=2)
go run ./cmd/worker -role video -c 2
```

### **任务分配机制**

```
总队列 (Redis):
├── split 队列 → 由 Worker 1 的 2 个 goroutine 处理
├── asset 队列 → 由 Worker 2 的 4 个 goroutine 处理  
├── video 队列 → 由 Worker 3 的 2 个 goroutine 处理
└── export 队列 → 无人处理 (未启动 Worker 4)
```

**关键点**:
- 不同角色的 Worker 处理不同的队列
- 同一角色的多个 Worker 会竞争消费同一个队列
- 使用 Redis 的原子操作确保消息只被一个 Worker 处理

---

## 💡 **如何检查当前 Worker 状态**

### **方法 1: 查看进程**
```powershell
Get-Process | Where-Object {$_.Path -like "*worker*"} | 
    Select-Object Id, ProcessName, Path, StartTime, CPU
```

### **方法 2: 查看数据库任务**
```sql
SELECT COUNT(*), status 
FROM generation_tasks 
WHERE task_type = 'SCRIPT_SPLIT'
GROUP BY status;
```

### **方法 3: 查看 Worker 日志**
查看控制台输出是否显示：
```
✅ 数据库连接成功
✅ Redis 连接成功
🎯 Worker 启动成功 (角色=split, 并发数=2)
👂 监听队列：split
⏱️ 轮询间隔：5 秒
```

---

## 📈 **性能调优建议**

### **1. 增加全局并发**

如果 DeepSeek API 允许高并发，可以增加单个 Worker 的并发数：

```bash
# 原来：2 goroutine
# 改为：4 goroutine
go run ./cmd/worker -role split -c 4
```

### **2. 增加 Worker 实例**

如果有高性能机器，可以同时运行多个 Worker：

```bash
# 终端 1
go run ./cmd/worker -role split -c 2

# 终端 2  
go run ./cmd/worker -role split -c 2
# 总并发=4
```

### **3. 监控 API 限流**

注意观察 DeepSeek API 的限流策略：
- 请求频率限制 (RPS)
- Token 配额限制 (TPM)

根据实际限制调整并发数，避免触发限流。

---

## ⚠️ **注意事项**

### **1. 并发过高可能触发 API 限流**

如果看到频繁的错误：
```
provider error 429: rate limit exceeded
```

说明并发太高了，需要降低 `-c` 的值。

### **2. 内存占用**

每个 goroutine 会持有：
- LLM 响应缓冲
- 上下文数据
- 中间结果

建议每台机器最多 4-8 个并发 Worker goroutine。

### **3. 数据库连接池**

确保 PostgreSQL 连接池足够大：

```go
pool, err := db.New(ctx, cfg.Postgres.DSN)
// pool.MaxConns 应该 >= 所有 Worker 并发数之和
```

---

## 🔍 **总结**

**当前的并行工作机制包含两个层次:**

### **层次 1: Worker 级并发**
- 每个 Worker 内部有 N 个 goroutine (默认 2)
- 每个 goroutine 独立监听并处理队列中的任务
- 实现多任务并行执行

### **层次 2: 任务内并行 (已实施)**
- 剧本拆分的段 B 进一步优化
- 将 N 集串行改为 2 集并行
- 理论加速比 1.5x ~ 2x

**双重并行保障**:
1. **多任务并行** (Worker 级) - 同时处理多个剧本
2. **单任务并行** (任务级) - 同时处理同一剧本的多集

**预期效果**: 
- 5 集剧本从 ~30 分钟 → ~20 分钟 (-33%)
- 10 集剧本从 ~55 分钟 → ~30 分钟 (-45%)

---

**这就是你现在使用的并行工作机制！如果还没有启动 Worker，请按照 URGENT-WORKER-NOT-RUNNING.md 中的步骤启动。** 🚀
