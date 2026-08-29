# 🤖 Worker 存在的必要性与架构设计原理

## ❓ **核心问题**

> "为什么需要 Worker？为什么不直接把请求发给远程的大模型？"

---

## ✅ **简短回答**

Worker 是**专门用于处理耗时较长的 AI 任务的异步引擎**，它的存在为了解决：

1. **超时问题** - LLM 调用可能需要几分钟，HTTP 请求无法长时间等待
2. **并发控制** - 防止同时发起数百个 LLM 请求压垮系统
3. **可靠重试** - AI 调用失败时自动重试，不丢失任务
4. **进度跟踪** - 实时更新前端进度条，提升用户体验
5. **成本优化** - 控制并发度，避免超出 API Key 配额

---

## 🏗️ **架构对比**

### **方案 A: 无 Worker（直接同步调用）❌**

```
用户点击"拆分剧本" 
    ↓
浏览器发送 HTTP POST /api/split 
    ↓
后端 API 接收请求 → 立即调用 DeepSeek API（同步阻塞）
    │
    ├─→ 第 1 集：等待 30 秒 ←───┐
    │                          │
    ├─→ 第 2 集：等待 30 秒     │ 总共需要 5-30 分钟
    │                          │
    ├─→ 第 3 集：等待 30 秒     │
    │                          │
    └─→ ...                    │
                               │
                         ⚠️ 超时风险！       


典型问题:
├─ Nginx 默认 timeout = 60 秒
├─ 浏览器可能会在 30-60 秒后取消请求
├─ 数据库连接保持过久，耗尽连接池
├─ API Key 被高并发请求瞬间打满
└─ 任何一步失败，整个流程回滚困难
```

**实际案例**:
```go
// ❌ 错误做法：API 直接同步调用 LLM
func SplitHandler(c *gin.Context) {
    // 接收用户剧本
    script := c.PostForm("script")
    
    // 创建任务
    taskID := CreateTask(script)
    
    // ⚠️ 致命问题：直接同步调用 API
    result, err := llm.Chat(ctx, script)  // 可能阻塞 5 分钟！
    
    if err != nil {
        return Error()
    }
    
    return Success(result)  // 永远到不了这里，因为超时了
}
```

---

### **方案 B: 有 Worker（异步队列）✅**

```
用户点击"拆分剧本" 
    ↓
浏览器发送 HTTP POST /api/split 
    ↓
后端 API 接收请求 → 快速写入 Redis 队列 (0.1 秒)
    ↓
立即返回 {"task_id": "xxx", "status": "QUEUED"}
    ↓
    ✅ 前端开始轮询？/api/tasks/:id (每 2 秒刷新进度)
            ↓
    Worker 监听队列 (后台进程独立运行)
    ↓
    Worker 读取任务 → 调用 DeepSeek API → 更新进度
    ↓
    段 A: 结构拆分 (30 秒) → 更新 status=RUNNING, progress=8%
    ↓
    段 B: 镜头设计 (并行×2，3 分钟) → 实时通知前端："正在设计第 1/3 集..."
    ↓
    段 C: 资产提示词 (异步后台)
    ↓
    段 D: 数据库保存 (完成) → status=SUCCESS
    ↓
    ✅ 前端轮询发现 SUCCESS → 展示结果


优势:
✓ API 响应 <1 秒（快 600 倍）
✓ 支持无限时长任务
✓ 可监控每个阶段进度
✓ 失败可重试，不影响用户
✓ 灵活控制并发度（避免 API 限流）
```

**正确实现**:
```go
// ✅ 正确做法：API 只负责入队，Worker 负责执行

// ========== API 端 ==========
func SplitHandler(c *gin.Context) {
    script := c.PostForm("script")
    
    // 1. 验证输入
    if len(script) == 0 {
        c.JSON(400, gin.H{"error": "空剧本"})
        return
    }
    
    // 2. 创建任务记录（快速）
    task := Task{
        Status:   "QUEUED",
        Progress: 0,
        CreatedAt: time.Now(),
    }
    DB.Create(&task)
    
    // 3. 推送到 Redis 队列（< 10ms）
    queue.Push("split", &task)
    
    // 4. 立即返回（< 100ms）
    c.JSON(200, gin.H{
        "task_id": task.ID,
        "status":  "QUEUED",
    })
}

// ========== Worker 端 ==========
for {
    // 从队列取任务
    task := queue.Pop("split")
    
    if task == nil {
        time.Sleep(5 * time.Second) // 无任务时休眠
        continue
    }
    
    // 标记为 RUNNING
    UpdateStatus(task.ID, "RUNNING", 0%)
    
    // 段 A: 结构拆分
    reportProgress("正在通读剧本...")
    structResult := CallDeepSeekAPI(script)
    UpdateProgress(8%)
    
    // 段 B: 镜头设计 (并发优化)
    for _, episode := range structResult.Episodes {
        go designShots(episode)  // 2 集并发！
    }
    UpdateProgress(50%)
    
    // 段 C: 资产提示词 (后台生成)
    generateAssetPrompts(structResult)
    
    // 保存结果
    SaveResult(structResult)
    UpdateStatus(task.ID, "SUCCESS", 100%)
}
```

---

## 🎯 **Worker 的核心作用**

### **1. 异步解耦**

| 维度 | 无 Worker | 有 Worker |
|------|---------|----------|
| **请求响应时间** | 300 秒（5 分钟） | 0.1 秒 |
| **HTTP 超时** | ❌ 必然触发 | ✅ 永远不会 |
| **用户体验** | ❌ 页面白屏 5 分钟 | ✅ 立即可见进度 |
| **资源占用** | ❌ DB 连接占 5 分钟 | ✅ DB 连接占 0.1 秒 |

---

### **2. 可靠性保障**

```go
// Worker 内置容错机制
worker.OnError = func(task *Task, err error) {
    // 1. 记录错误日志
    logger.Error("任务失败", "task_id", task.ID, "err", err)
    
    // 2. 自动重试（最多 3 次）
    if task.RetryCount < 3 {
        task.RetryCount++
        queue.Requeue(task)  // 放回队列重新处理
        return
    }
    
    // 3. 彻底失败，标记死信
    UpdateStatus(task.ID, "FAILED_DEAD")
    MarkAsFailedInDB(task.ID)
}
```

**场景对比**:

```
场景：DeepSeek API 暂时不可用

【无 Worker】
└─ API 直接调用失败 → HTTP 500 → 用户看到"请求失败" → 手动重试

【有 Worker】
└─ 第一次：调用失败 → 自动重试 ①
    ↓ 仍然失败
└─ 第二次：调用失败 → 自动重试 ②
    ↓ 仍然失败  
└─ 第三次：调用失败 → 标记 FAILED_DEAD + 发送邮件通知管理员
    ↓
用户收到邮件 → 后台修复配置 → 手动激活任务
```

---

### **3. 并发与限流控制**

**问题**: API Key 通常有限流限制

```python
# DeepSeek API 限流示例
Max RPS (Requests Per Second): 10      # 每秒最多 10 次请求
Max TPM (Tokens Per Minute): 100000    # 每分钟最多 10 万 tokens

如果直接从 API 发起请求:
用户 A: 提交任务 → 立即调用 (RPS +1)
用户 B: 提交任务 → 立即调用 (RPS +2)
用户 C: 提交任务 → 立即调用 (RPS +3)
...
用户 Z: 提交任务 → 立即调用 (RPS +100)  ← 触发 429 错误！
```

**Worker 解决方案**:

```go
// Worker 通过信号量控制并发
const MaxConcurrency = 2  // 最多 2 个 goroutine 同时调用 API

for i := 0; i < MaxConcurrency; i++ {
    go func() {
        for {
            task := queue.Pop()
            
            // 确保不超过并发限制
            // ✅ 每次只有一个 goroutine 能调用 API
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            result := callDeepSeekAPI(task)
        }
    }()
}
```

**效果**:

```
100 个用户同时提交
│
├─ Worker A (goroutine #1) → 处理前 2 个任务 → 平均等待 30 秒
├─ Worker B (goroutine #2) → 处理中 2 个任务 → 平均等待 30 秒
│
└─ 剩余 98 个任务 → 排队等待
   ├─ 第 3 批：Worker A 空闲 → 取出下一个
   ├─ 第 4 批：Worker B 空闲 → 取出下一个
   └─ 平稳通过限流阈值
```

---

### **4. 进度实时追踪**

**无 Worker 的问题**:

```
用户点击 → 页面显示 Loading... → 5 分钟后突然跳转结果页
⚠️ 用户不知道卡在哪里，是否成功？
```

**Worker 的优势**:

```go
// Worker 实时推送进度
func RunSplitTask(taskID int64) {
    // 段 A
    ReportProgress(8%, "正在通读剧本、划分剧集...")
    // 前端每 2 秒刷新 → 看到：8% 正在分析...
    
    // 段 B  
    ReportProgress(15%, "开始 3 集并发镜头设计")
    // 前端显示：15% 正在进行镜头设计
    
    Go routine #1: ReportProgress(33%, "正在设计第 1/3 集...")
    Go routine #2: ReportProgress(50%, "正在设计第 2/3 集...") ← 同时进行！
    
    // 段 C
    ReportProgress(85%, "生成资产提示词")
    
    // 完成
    ReportProgress(100%, "保存至数据库")
}
```

---

### **5. 批量处理与优化**

Worker 可以做一些全局优化：

```go
// Dispatcher: 负载均衡器
func (d *Dispatcher) Dispatch(task *Task) {
    // 检测是否有相似任务在队列中
    similarTasks := FindSimilarTasks(task.ScriptText)
    if len(similarTasks) > 0 {
        // 合并处理，节省 API Token
        MergeAndExecute(similarTasks)
    }
}

// Reaper: 清理僵尸任务
func (r *Reaper) CheckZombies() {
    // 找出长时间 RUNNING 的任务（可能是 Worker 崩溃）
    zombieTasks := Query(`WHERE status='RUNNING' AND updated_at < NOW()-INTERVAL '30 min'`)
    
    for _, z := range zombieTasks {
        // 重置为 QUEUED，让其他 Worker 接手
        UpdateStatus(z.ID, "QUEUED")
    }
}
```

---

## 🔄 **完整的异步工作流**

```mermaid
sequenceDiagram
    participant User as 用户浏览器
    participant API as API 服务器
    participant Queue as Redis 队列
    participant Worker as Worker (后台进程)
    participant LLM as DeepSeek API
    participant DB as PostgreSQL

    User->>API: POST /api/split (脚本内容)
    API->>API: 验证输入
    API->>DB: INSERT INTO generation_tasks (status='QUEUED')
    API->>Queue: LPUSH task_queue:split task_id
    API-->>User: JSON {"task_id": 941, "status": "QUEUED"}
    
    User->>User: 开始轮询 ?/tasks/941
    
    loop 每 2 秒
        User->>API: GET /api/tasks/941
        API->>DB: SELECT * FROM tasks WHERE id=941
        API-->>User: JSON {progress: 8%, detail: "正在分析..."}
    end
    
    Note over Worker: Worker 每 5 秒轮询一次队列
    Worker->>Queue: RPOP task_queue:split
    Queue-->>Worker: task_id=941
    
    Worker->>DB: UPDATE tasks SET status='RUNNING'
    Worker->>LLM: 调用 DeepSeek API (段 A)
    LLM-->>Worker: 返回结构拆分结果
    
    Worker->>DB: UPDATE tasks SET progress=8%
    
    // 段 B 并发执行
    Worker->>LLM: 调用 DeepSeek API (集 1, 并发 1)
    Worker->>LLM: 调用 DeepSeek API (集 2, 并发 2)
    LLM-->>Worker: 返回集 1 分镜结果
    LLM-->>Worker: 返回集 2 分镜结果
    
    Worker->>DB: UPDATE tasks SET progress=50%
    
    // 段 C 后台生成
    Worker->>DB: UPDATE tasks SET progress=85%
    
    Worker->>DB: INSERT split_results INTO database
    Worker->>DB: UPDATE tasks SET status='SUCCESS', progress=100%
```

---

## 💰 **成本分析**

| 成本项 | 无 Worker | 有 Worker |
|--------|---------|----------|
| **服务器 CPU** | 100% (阻塞等待) | <5% (仅队列管理) |
| **数据库连接** | 1 个连接 × 5 分钟 = 5 分钟连接池占用 | 0.1 秒连接池占用 |
| **网络带宽** | 持续保持长连接 | 短时连接 |
| **API Key 配额** | 可能被瞬时高并发耗尽 | 可控速率通过 |
| **人力成本** | 需频繁手动重试失败任务 | 自动重试，减少人工干预 |

---

## 🔍 **Worker 类型对照表**

项目中启动的不同角色 Worker：

| Worker 角色 | 负责任务 | 为什么要独立进程 |
|-----------|---------|----------------|
| **split** | 剧本拆分 (多步骤 LLM 调用) | 耗时最长，需要并发优化 |
| **asset** | 资产图片生成 (T2I API) | 批量生成图片，高并发场景 |
| **video** | 视频合成 (T2V API) | 渲染时间长，单任务~10 分钟 |
| **export** | 导出最终视频 (FFmpeg) | 计算密集型，CPU 密集 |

每种 Worker 针对不同类型的任务做了专门的优化：

```bash
# 剧本拆分 Worker
cd duanju-backend && \
$env:DATABASE_URL="..." && \
$env:REDIS_ADDR="..." && \
go run ./cmd/worker -role split -c 2  # 并发数=2

# 资产生成 Worker
go run ./cmd/worker -role asset -c 4  # 并发数=4 (更快)

# 视频生成 Worker
go run ./cmd/worker -role video -c 2  # 并发数=2
```

---

## 🎯 **总结**

### **Worker 的核心价值**:

1. **解耦**: 将耗时任务从 HTTP 请求中剥离 → 提升 API 响应速度 600 倍
2. **可靠**: 自动重试、错误处理、死信归位 → 降低任务丢失率 100%
3. **可控**: 并发度限制、API 限流防护 → 避免超出配额
4. **可观测**: 实时进度、详细日志 → 提升用户体验
5. **可优化**: 并行化、批量处理、智能调度 → 性能提升 30%-50%

### **如果没有 Worker**:

- ❌ 所有 LLM 调用都必须同步阻塞 HTTP 请求
- ❌ 浏览器/中间层必然触发超时
- ❌ 用户体验极差，频繁失败
- ❌ 无法控制 API 并发，容易超出限额
- ❌ 无法提供实时进度反馈

**结论**: Worker 不是多余的，而是**必须存在的异步任务引擎**。它是现代 AI 应用的基础设施，类似于 Celery (Python)、Sidekiq (Ruby)、Hangfire (.NET) 等框架的作用。
