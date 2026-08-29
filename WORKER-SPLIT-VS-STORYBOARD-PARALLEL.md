# 🔄 剧本拆分 + 分镜脚本生成：系统并行机制详解

## 🎯 **两种任务的定义**

| 任务类型 | Handler | 作用 |
|---------|---------|------|
| `SCRIPT_SPLIT` | SplitHandler | 剧本拆分：读全文 → 分集 → 每集设计镜头 → 资产提示词 → 入库 |
| `STORYBOARD_PLAN_V1_LUXCHAT` | StoryboardPlanHandler | 分镜重生成：针对**单个叙事单元**重跑分镜技能（v2+） |

---

## 📐 **核心机制：它们进入同一个队列，由同一个 Worker 的多个 goroutine 并行消费**

### **1. 任务创建（API 侧）**

**剧本拆分入队**（split 服务）：
```sql
INSERT INTO generation_tasks (task_type='SCRIPT_SPLIT', status='QUEUED', ...)

INSERT INTO task_dispatch_outbox (queue_name='split', ...)  -- 投递到 split 队列
```

**分镜生成入队**（storyboard_service.go L266-276）：
```sql
INSERT INTO generation_tasks (task_type='STORYBOARD_PLAN_V1_LUXCHAT', status='QUEUED', ...)

INSERT INTO task_dispatch_outbox (queue_name='split', ...)  -- 同一个 split 队列！
```

> **关键**：两者都投递到 `queue_name='split'`，进入**同一个 Redis ZSet 队列** `duanju:{env}:queue:tasks`。

---

### **2. Worker 的 Handler 注册（main.go L221-236）**

```go
// split 角色 Worker 同时注册两个 Handler！
if *role == "split" {
    handlers = append(handlers, splitHandler)              // → SCRIPT_SPLIT
    handlers = append(handlers, storyboardPlanHandler)     // → STORYBOARD_PLAN_V1_LUXCHAT
}
```

**所以一个 split Worker 就能处理两种任务**，通过 `resolveHandler` 按 task_type 分发。

---

### **3. 并行的真实过程（-c 2 时）**

```
Redis 队列: [任务A: SCRIPT_SPLIT] [任务B: STORYBOARD_PLAN_V1_LUXCHAT]
                        ↓               ↓
              ┌──────────────────────────────────┐
              │        Worker (split, -c 2)       │
              │                                   │
              │  goroutine #1                     │
              │  ├─ Claim → 领取任务 A             │
              │  ├─ resolveHandler(SCRIPT_SPLIT)   │
              │  │      → SplitHandler            │
              │  └─ Execute (15 分钟)              │
              │                                   │
              │  goroutine #2                     │
              │  ├─ Claim → 领取任务 B             │
              │  ├─ resolveHandler(STORYBOARD_    │
              │  │     PLAN_V1_LUXCHAT)            │
              │  │      → StoryboardPlanHandler   │
              │  └─ Execute (3 分钟)               │
              └──────────────────────────────────┘
                     ↑↑↑
          两个任务真正并行执行！互不阻塞！
```

**时间线示例**：
```
T+0s     G1: 领取 SCRIPT_SPLIT → 开始段 A（读剧本、分集）
         G2: 领取 STORYBOARD_PLAN → 开始 LLM 生成分镜
T+1min   G1: 段 A 完成，进入段 B（集1、集2 并发设计镜头）← 内部第二级并行！
         G2: LLM 返回分镜 JSON → 开始逐卡润色
T+3min   G2: 完成 → SaveStoryboardPlan 入库 → 任务 SUCCESS
         G2: 空闲 → 领取下一个排队任务（如果有）
T+15min  G1: 完成所有集数 → 入库 → 任务 SUCCESS
```

---

## 🔍 **两个 Handler 执行细节对比**

### **SplitHandler（剧本拆分，多阶段长任务）**

```
Execute()
├─ 段 A: 结构拆分（1 次 LLM 调用，读取全文 → 分集）
├─ 段 B: 镜头设计（每集 1 次 LLM 调用，2 集并发！）
│         ├─ 集 1: designEpisodeShots → LLM
│         └─ 集 2: designEpisodeShots → LLM  ← 同时进行
├─ 段 C: 资产提示词（后台）
└─ 段 D: 入库 + 计费
```

**特点**：任务内部还有并发（`parallelEpisodeDesignConcurrency=2`），总调用可能 10+ 次 LLM。

---

### **StoryboardPlanHandler（分镜重生成，短任务）**

```go
// storyboard_plan_handler.go Execute()
func (h *StoryboardPlanHandler) Execute(ctx, t) ([]byte, error) {
    // 1. 读取叙事单元（剧本原文 + 时长 + 资产清单）
    beat, _ := h.store.LoadBeatForPlan(ctx, params.BeatID)
    
    // 2. LLM 生成分镜（坏 JSON 自动重试 3 次）
    resp, _ := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
        Prompt: beat.ScriptOriginalText,
        System: fmt.Sprintf(storyboardPlanSystemPrompt, beat.PresetDurationSec, assetList),
        MaxTokens: 8192, JSONMode: true, NoThinking: true,
    })
    
    // 3. 逐卡润色（失败回退，不阻塞）
    h.polishGroups(ctx, groups)
    
    // 4. 事务替换入库（软删旧 shots → 插新分镜 → 版本快照）
    stats, _ := h.store.SaveStoryboardPlan(ctx, params.BeatID, groups)
}
```

**特点**：单次 LLM 调用 + 润色，通常 1-3 分钟完成。**短任务占便宜**——可能比长任务先完成。

---

## ⚠️ **两者之间的依赖关系（重要！）**

### **场景 1: 拆分进行中，无法对不存在的单元发起分镜**

```
用户点击"生成分镜" → GenerateStoryboards(beatUUID)
         ↓
ResolveNarrativeUnit(beatUUID) → 叙事单元还不存在（拆分未完成）
         ↓
❌ 返回 404 "叙事单元不存在"
```

**所以分镜任务只能针对已存在的单元**（拆分完成后的单元，或历史项目的单元）。

### **场景 2: 拆分完成，用户立刻对某单元"重新生成分镜"**

```
用户点击"重新生成分镜" → GenerateStoryboards
         ↓
RUNNING 门控检查: 该单元是否有视频正在生成？
         ↓
✅ 无 → 创建 STORYBOARD_PLAN_V1_LUXCHAT 任务 → 入队
         ↓
Worker G2 领取 → 并行执行（与别的拆分任务同时进行）
```

### **场景 3: 拆分中 + 对已完成单元重新规划（常见）**

```
拆分任务 A (SCRIPT_SPLIT) 正在 G1 上执行（处理第 2/3 集）
用户又对第 1 集单元发起分镜重生成（任务 B）

队列: [任务B: STORYBOARD_PLAN] （任务 A 已在执行）

G2 空闲 → 领取任务 B → 并行执行！
         ↓
虽然任务 A 还在跑，但任务 B 只操作自己的单元（beat_id=1），
两者操作不同的数据行（shots 表不同单元），完全无冲突！
```

---

## 📊 **并发场景总结表**

| 场景 | 队列中的任务 | 执行方式 | 冲突风险 |
|------|------------|---------|---------|
| 同时提交 1 个拆分 + 1 个分镜 | [A: SPLIT, B: PLAN] | G1→A, G2→B 并行 | 无（不同数据） |
| 2 个拆分 + 1 个分镜 | [A, B: SPLIT, C: PLAN] | G1→A, G2→B；C 排队等 G1 完成 | 无 |
| 1 个拆分进行中 + 对已完成单元分镜 | [C: PLAN] | G1→A(运行中), G2→C 并行 | 无（不同单元） |
| 拆分未完成就点分镜 | 创建失败 | 404 返回 | 无法发生 |

---

## 🔧 **控制并行的参数**

```bash
# Worker 并发数决定"同时执行几个任务"
go run ./cmd/worker -role split -c 2   # 最多 2 个任务并行
go run ./cmd/worker -role split -c 4   # 最多 4 个任务并行

# 再加一个 Worker 实例
# 终端1: go run ./cmd/worker -role split -c 2
# 终端2: go run ./cmd/worker -role split -c 2
# → 最多 4 个任务并行（2 个进程 × 2 个 goroutine）
```

**注意**：任务 A（拆分）内部段 B 还有 2 集并发，所以 1 个拆分任务在 LLM 层面可能同时有 2 个请求；再加 1 个分镜任务，就是 3 个 LLM 请求同时进行。

---

## 🎯 **核心结论**

1. **共享队列 + 多 goroutine 竞争消费**：剧本拆分和分镜生成进入同一个 Redis 队列，Worker 的每个 goroutine 独立领取任务，天然并行。

2. **Handler 分发**：Worker 按 task_type 自动路由——`SCRIPT_SPLIT` → SplitHandler，`STORYBOARD_PLAN_V1_LUXCHAT` → StoryboardPlanHandler，互不干扰。

3. **短任务先完成**：分镜生成（~3 分钟）通常比拆分（~15 分钟）先结束，腾出 goroutine 处理新任务。

4. **数据隔离**：分镜任务只操作指定叙事单元的数据，与拆分任务写不同行，**无锁冲突、无脏写**。

5. **唯一限制**：分镜任务必须等拆分创建出单元后才能发起（API 层 404 门控 + RUNNING 视频门控）。

**一句话**：系统把两种任务丢进同一个"任务池"，Worker 里的每个工人（goroutine）各拿一个任务同时干活——拆分任务自己还会再叫帮手（内部并行），分镜任务一个人轻装上阵。这就是完整的并行机制！🚀
