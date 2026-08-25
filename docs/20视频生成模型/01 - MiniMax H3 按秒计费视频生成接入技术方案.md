# MiniMax H3 按秒计费视频生成接入技术方案

> **Workspace:** d:\develop\a_work\duanju
> **现状基线：** 视频生成链路已有完整骨架（`VIDEO_SEGMENT_GEN` 任务 → `ModelEngine.GenerateVideo(CapT2V)` → Provider），当前 T2V 仅接入 `AsyncTaskProvider`（火山/可灵形态）与 mock，`OpenAICompatProvider` 的 Video 未实现。本方案在**不重造链路**的前提下，新增 MiniMax H3 视频厂商，并补上"**双轨计费**"这一能力——**用户侧按次扣固定项目积分，厂商侧（MiniMax）按秒仅作后台成本核算，不转嫁给用户**。

---

## 目录

1. 接入背景与能力缺口分析
2. MiniMax H3 API 接入准备工作
3. 管理端配置项
4. 前端实现要点
5. 前后端交互流程设计
6. 错误处理与异常应对策略
7. 性能优化建议
8. 测试验收标准
9. 实施与回滚清单

---

## 一、接入背景与能力缺口分析

### 1.1 现有链路（代码证据）

```
前端 Step3 (GenVideoModal)
  └─ POST /beats/:beat_id/generate-videos
       └─ UpdateVideoStatus → VIDEO_GENERATING
            └─ Worker: VIDEO_SEGMENT_GEN (VideoHandler.Execute)
                 └─ ModelEngine.GenerateVideo(CapT2V, &VideoRequest{...})
                      ├─ AsyncTaskProvider   (火山即梦/可灵：POST 创建任务 → 轮询 → 结果URL)
                      ├─ MockProvider        (开发期 mock 样例视频)
                      └─ OpenAICompatProvider (Video 未实现 ✅ 恰好留给 MiniMax)
```

- 能力枚举：`CapT2V`（文生视频）/ `CapI2V`（图生视频）定义于 [provider.go](file:///d:/develop/a_work/duanju/duanju-backend/internal/engine/model/provider.go#L11-L20)。
- 任务驱动模型：Worker 单任务模型，`GenerateVideo` 为**同步阻塞**（创建任务→轮询→返回 URL），适配 `AsyncTaskProvider` 的 `pollTask`（[async_provider.go](file:///d:/develop/a_work/duanju/duanju-backend/internal/engine/model/async_provider.go#L116-L152)），视频任务超时默认 30min。
- 配置真源：`model_providers / models / model_keys` 三表，`ModelKeyStore` 提供管理端增删改查（[model_keystore.go](file:///d:/develop/a_work/duanju/duanju-backend/internal/repository/model_keystore.go)）。

### 1.2 能力缺口：双轨计费（用户按次 + 成本按秒）

| 缺口 | 现状 | 影响 |
|---|---|---|
| **用户计费粒度** | `models.cost_points_per_call` 按**单次调用**计费 | ✅ 正好匹配"用户侧一次性扣固定项目积分"，**无需**改用户计费口径 |
| **厂商成本粒度** | 仅计算 `cost_points_per_call`，**无成本核算字段** | MiniMax H3 对我方按**生成秒数**计费（成本），需单独记录 `时长 × 单价` 作为后台成本/对账依据，且**不转嫁用户** |
| **厂商差异** | `AsyncTaskProvider` 以火山/可灵为模板，字段固定（`duration`） | MiniMax H3 字段名不同（`seconds`/`aspect_ratio`/`resolution`），状态字段也不同（`task_status`） |

**结论**：MiniMax H3 的接入是"**新增一个 Provider + 成本核算字段 + 管理端/前端配置**"，复用既有任务队列、路由、降级、心跳机制。**用户收费保持"按次固定积分"（`cost_points_per_call`），"按秒"仅落入后台成本核算，不在前端向用户展示费用。**

---

## 二、MiniMax H3 API 接入准备工作

### 2.1 账号与权限

| 步骤 | 说明 |
|---|---|
| 账号注册 | 在 [AutoDL 控制台](https://www.autodl.com/home) 注册并完成实名认证（视频生成属合规敏感能力，需企业/个人实名） |
| 开通服务 | 进入"模型服务"→ MiniMax H3（海螺/Hailuo 3）视频生成，开通并确认支持 **T2V 按秒计费** 档位 |
| 获取 API Key | 在"API 密钥"页创建 Key 并**立即复制**（仅创建时明文展示一次）；同时记录 **GroupId**（部分 MiniMax 接口鉴权需 `group_id` 路径参数） |
| 计费确认 | 核对单价（元/秒）→ 换算为本平台"积分/秒"（见 3.2 汇率），并确认余额充足、有无免费额度/并发上限 |

### 2.2 权限与合规配置

- **IP 白名单**：若 AutoDL 提供"仅允许指定 IP 调用"，将本后端出口 IP（部署机 `curl ifconfig.me`）加入白名单；容器化部署需关注 NAT 出口地址是否稳定。
- **并发配额**：确认账号级 QPS/RPM/并发任务上限（决定 `model_keys.concurrent_limit` 配置值）。
- **回调/结果获取**：MiniMax H3 采用"提交任务返回 task_id + 轮询结果"模式，**无需**注册 webhook 回调域名（免费/标准档）。若使用企业版回调能力，需在 AutoDL 配置回调地址并走内网。

### 2.3 所需凭据汇总（用于管理端录入）

| 配置项 | 值示例 | 说明 |
|---|---|---|
| 厂商 BaseURL | `https://api.minimax.chat/v1` | MiniMax 开放平台 API 域 |
| API Key | `eyJhbGciOi...` | 密文入库（后端 encryptKey 加密） |
| GroupId | `167xxxxxxxxx` | 部分接口需拼进路径；若 BaseURL 已含 group_id 则留空 |
| 模型 model_key | `minimax-hailuo-3` | MiniMax H3 视频模型名 |
| 能力 | `T2V`（可加 `I2V`） | model_capability_enum |

---

## 三、管理端配置项

### 3.1 模型与厂商录入（复用现有 admin 端点）

管理端通过既有 `POST /api/v1/admin/models` / `model-keys` 等接口（[admin_extra_service.go](file:///d:/develop/a_work/duanju/duanju-backend/internal/domain/creation/admin_extra_service.go#L758-L769)）录入：

**厂商 `model_providers`**：
| 字段 | 值 | 备注 |
|---|---|---|
| name | `minimax` | 唯一标识 |
| display_name | `MiniMax 海螺 H3` | 展示名 |
| capabilities | `["T2V","I2V"]` | provider 能力声明（`mp.capabilities ? $1` 参与路由） |
| base_url | `https://api.minimax.chat/v1` | 由 `KeyCredential.BaseURL` 传入 Provider |

**模型 `models`**：
| 字段 | 值 | 备注 |
|---|---|---|
| provider_id | minimax 的 id | |
| model_key | `minimax-hailuo-3` | 透传给厂商 API |
| capability | `T2V` | 路由时 `m.capability::text = 'T2V'` |
| cost_points_per_call | `100` | **用户侧按次扣费**（一次性固定积分，与秒数无关） |
| enabled | true | |

**API Key `model_keys`**：
| 字段 | 值 | 备注 |
|---|---|---|
| provider_id | minimax | |
| name | `minimax-h3-prod` | |
| key_encrypted | (Admin 传明文→后端加密) | 复用 `CreateModelKey` 的 encryptKey |
| rate_limit_rpm | 依账号配额 | |
| concurrent_limit | 建议 1~2 | 视频耗时，过低并发保护 |
| priority | 10 | 最高（可高于火山，做首选路由） |

### 3.2 双轨计费规则（用户按次 + 成本按秒）

**用户侧（对用户收费，保持按次）**：沿用 `models.cost_points_per_call`——每次 `VIDEO_SEGMENT_GEN` 任务提交，扣固定积分（如 100），**与秒数无关**。幂等（重试不重复扣，对齐资产 `point_cost_applied` 机制）。这是唯一与用户积分余额交互的口径。

**成本侧（对厂商成本核算，按秒）**：MiniMax H3 对我方按生成秒数计费，作为**后台成本**记录，不转嫁用户、不进用户余额扣减。

**迁移** `新增成本核算字段`（供后台对账，不参与用户扣费）：
```sql
-- 算法表：每模型配置"每秒成本单价（元）"，用于成本报表与厂商对账
ALTER TABLE models ADD COLUMN IF NOT EXISTS cost_per_second_amount NUMERIC(12,4) NOT NULL DEFAULT 0;
-- 任务成本流水：视频任务完成后记录实际耗时与成本金额
-- 复用 generation_tasks 已有字段，新增成本列（可空，成本侧专用）
ALTER TABLE generation_tasks ADD COLUMN IF NOT EXISTS cost_amount NUMERIC(12,4) NOT NULL DEFAULT 0;
ALTER TABLE generation_tasks ADD COLUMN IF NOT EXISTS cost_duration_sec INT NOT NULL DEFAULT 0;
```

**成本公式（仅后台核算）**：
```
厂商成本(元) = 实际生成秒数 × models.cost_per_second_amount
```
- 视频任务 SUCCESS 后，由 Worker/回调按 `实际秒数`（来自 `VideoRequest.DurationSec` 或厂商返回的 video duration）计算成本，写入 `generation_tasks.cost_amount / cost_duration_sec`。
- **管理端成本模块**：按厂商/模型/日聚合成本报表（`SUM(cost_amount)`），与 AutoDL 账单**每日对账**；不出现在用户端。

**管理端计费规则管理**（新增模块，两栏）：
- 用户计费栏：`cost_points_per_call`（固定积分/次）展示与编辑——这是用户看到的收费。
- 成本核算栏：`cost_per_second_amount`（元/秒）展示与编辑——仅后台对账用，标注"厂商成本，不转嫁用户"。
- 成本监控：日内成本报表 + 超阈值告警（防异常调用耗光预算）。

### 3.3 任务队列监控

复用现有 `generation_tasks` 与 `task_store.go`，管理端任务列表可新增**视频任务列**：

| 监控项 | 实现 |
|---|---|
| 任务类型审计 | `task_type = VIDEO_SEGMENT_GEN`，区分 `model_used`（应显示 `minimax-hailuo-3`） |
| 状态着色 | QUEUED（排队）/ RUNNING（生成中，含进度 pct）/ SUCCESS / FAILED_DEAD |
| 进度回写 | MiniMax H3 为轮询制，进度近似难实时——采用**分档提示**（10% 已提交 → 40% 引擎处理中 → 90% 渲染中 → 100% 完成），由 Provider 查询到 `submitted/processing/render` 状态时映射 |
| 失败率/耗时 | 统计视频任务 P50/P95 耗时与失败率，用于调 `concurrent_limit` 与 `videoTimeout` |

### 3.4 日志记录

视频链路独立日志 tag：`video: shot=... model=minimax-hailuo-3 task=...`。
- **Request 日志**：入参摘要（prompt 截断、seconds、aspect_ratio、refs 数量）——不落完整 prompt 全文（隐私+体积）。
- **Provider 事件**：createTask 返回的 `task_id`、每次轮询的状态、最终成功/失败、耗时（对齐现有 `video_handler.go` 的 `log.Printf`）。
- **错误日志**：厂商错误码埋点（4xx 参数错、429 限流、5xx 服务端）→ 管理端错误报表。
- **审计**：按秒扣费流水（task_id、duration_sec、单价、扣费积分、余额）入库，便于对账。

---

## 四、前端实现要点

### 4.1 视频参数输入界面（Step3 GenVideoModal 扩展）

现有 `MODEL_OPTIONS`（Seedance 2.5/2.0/1.5）为静态数组，改为**由后端 `/api/v1/models?capability=T2V` 驱动**：
- 动态渲染模型列表（新增 `MiniMax H3 海螺` 一项），每个模型携带 `cost_points_per_second` 与预估说明。
- 新增参数：`时长`（秒，步进 5，对齐 MiniMax H3 支持的 5/10 秒档位）、`分辨率`（P720/P1080）、`比例`（16:9 / 9:16，对齐上游场景比例）。
- 参考图注入：已有 `RefImageURLs`（镜头级+单元级），提示"MiniMax H3 支持多图参考，未就绪资产将走纯文生视频"（对齐 P1 软门控）。

### 4.2 生成进度展示

- 复用 `useWsDrivenPoll` + `TASK_PROGRESS`；MiniMax 轮询制下后端按分档回写进度（10/40/90/100），前端 ShotCard 进度条按 `pct` 渲染。
- 长耗时提示：视频生成 30s~5min，悬浮条显示"视频渲染中…（约 x 秒）"，提供"返回 Step2 继续处理资产"不阻塞。
- 终态：TASK_SUCCESS → 刷新项目状态（对齐现有 `refreshVideos` 的 `videoDoneRef`）。

### 4.3 结果预览

- ShotCard 成功态视频 `<video>` 预览（复用现有 `generated_video_url`）。
- Step4 成片预览：`ALL_VIDEOS_READY`/`PARTIAL_VIDEOS` 合成导出（现有逻辑不改）；MiniMax 产出的 URL 走同一 `generated_video_url` 落地。

### 4.4 费用预估（用户侧按次，不展示厂商成本）

- 弹窗"预计消耗"改为**固定积分单价**：`本轮预计消耗 X 积分`（读 `models.cost_points_per_call`），与秒数无关。
- **不向用户展示按秒费用**：厂商按秒成本属后台核算，前端零暴露（仅可显示"预计生成时长约 x 秒"作等待提示，非计费口径）。
- **余额校验前置**：调用 `POST /api/v1/points/preview`（对资产同接口，传本次固定积分），余额不足则置灰"开始生成"并提示充值。
- 取消静态 `factor`（4/3/2）与"时长×单价"估算，改为读 `cost_points_per_call`。

---

## 五、前后端交互流程设计

### 5.1 时序（提交到成片）

```
前端 Step3 选 MiniMax H3 + 时长/分辨率/比例  ① 参数提交
  │
  ├─ POST /api/v1/points/preview（预估固定积分，余额校验）
  │
  ├─ POST /beats/:beat_id/generate-videos
  │     Body: { model_key:"minimax-hailuo-3", duration_sec, resolution, aspect_ratio }
  │         └─ 后端按次校验积分余额 → 扣固定 cost_points_per_call → 状态 VIDEO_GENERATING → 建 VIDEO_SEGMENT_GEN 任务
  │              （成本不在此扣用户）
  │
  └─ Worker: VideoHandler.Execute
        ├─ LoadShot → LoadShotRefs（参考图）→ dedupe 注入
        ├─ ModelEngine.GenerateVideo(CapT2V, VideoRequest{model_key, duration_sec, resolution, refs})
        │     └─ MiniMaxVideoProvider: POST /video_generation (task_id)
        │           └─ 轮询 GET /query/video_generation/{task_id}（20s~300s）
        │                 └─ 状态映射 queue→processing→render→succeed → 取 video_url
        │           └─ SUCCESS 后成本核算：cost_amount = 实际秒数 × cost_per_second_amount（仅后台，不扣用户）
        └─ SaveShotVideo（CAS 幂等）→ UpdateShotGroupStatus → 推送 TASK_SUCCESS
              └─ 前端 WS 驱动刷新 shots → Step4 解锁
```

### 5.2 接口契约（新增）

**后端新增 Provider**：`MiniMaxVideoProvider`（或为 `AsyncTaskProvider` 加 MiniMax 形态），关键字段映射：

| VideoRequest | MiniMax H3 API |
|---|---|
| Prompt | `prompt`（含 user 快捷指令 `text` 的兼容；H3 用 `prompt`） |
| DurationSec | `seconds`（注意：非 `duration`） |
| Resolution | `resolution`（P720/P1080，MiniMax 有档位；非 P 前缀需归一化） |
| RefImageURLs | `image_url`（单图/多图） |
| ModelKey | `model` |
| aspect_ratio | `aspect_ratio`（16:9 / 9:16，透传弹窗；不传则默认 16:9） |

**/api/v1/models 返回扩展**：模型对象返回 `cost_points_per_call`（用户计费口径，前端预估用）。新增后台字段 `cost_per_second_amount` 仅管理端读取，`/models` 用户接口**不暴露**（避免泄漏厂商成本）。

**成本核算落库**：`generation_tasks` 新增 `cost_amount / cost_duration_sec`，任务 SUCCESS 时写入（仅后台对账，不参与用户余额扣减）。

---

## 六、错误处理与异常应对策略

| 异常 | 策略 | 代码点 |
|---|---|---|
| **厂商 400 参数错**（如 seconds 越界、比例不支持） | 不可重试错误，直接 FAILED，前端 toast 展示厂商 message | Provider `NewRetryableError(0,...)` 之外返回可读错误；handler 记录 |
| **厂商 429 限流** | 可重试：进入 30s Key 冷却 → 下一 Key；轮询阶段 429 等待后继续（不重建任务，避免重复计费） | 对齐 `async_provider.pollTask` 的 429 分支 |
| **厂商 5xx** | 可重试 + Key 切换 | engine.go 路由层 |
| **任务超时（>30min）** | 返回可重试错误 → 上层重试；单 Key 并发信号量释放 | `videoTimeout` 30min 默认；建议按 `seconds` 动态放宽 |
| **余额不足** | 提交前拦截，前端提示充值；扣费失败回滚任务状态 | CreateVideoTasks 事务内校验 |
| **任务执行中断**（worker 崩溃） | ReapExpired 回收 + 分档回滚项目状态（拆分修复的经验复用）；`VIDEO_GENERATING` 兜底回退 `PARTIAL_VIDEOS`/`ALL_VIDEOS_READY` | task_store.ReapExpired + main.go Notifier 回滚 |
| **重复扣费**（重试） | 幂等：`point_cost_applied`/任务级已扣费标记，重试不再扣 | 对齐资产 CreateAssetTask |
| **生产降级** | 生产环境禁止 mock 降级（Major#16），`h.env=="production"` 直接报错；开发期可 mock 样例 | video_handler.go L154-159 |
| **鉴权失败** | Key 解密失败跳过该 Key 并告警；全部损坏明确报错，不静默降级 | model_keystore.go decryptKey |

---

## 七、性能优化建议

| 维度 | 建议 |
|---|---|
| **并发** | MiniMax H3 单 Key `concurrent_limit` 建议 1~2（视频耗 CPU/带宽，过高易触发账号限流）；可多 Key 轮询 + priority 分级（主 Key 高、备 Key 低） |
| **轮询间隔** | 智能退避：前 60s 每 10s 查，其后每 20s；避免高频查询触发账号侧限流（对齐 `pollTask` 的 `pollInterval` 动态化） |
| **超时** | `videoTimeout` 按 `seconds` 线性放宽：`max(5min, seconds×20 + 60s)`；避免固定 30min 浪费 Key 占用 |
| **进度节流** | Progress 上报 250ms 节流 + 分档（10/40/90/100），防消息风暴（本轮 429 治理经验） |
| **缓存** | 模型列表 `/models?capability=T2V` 接入 `cachedApi` TTL（60s），前端重复挂载不重复请求 |
| **限流** | 后端 `rate_limit` 生产 120/min 已稳；视频提交接口建议单独 `Limit` 或按套餐放宽（用户单次提交为低频操作） |
| **成本监控** | 按秒扣费的流水入库 → 管理端日内成本报表，超阈值告警（防异常调用耗光预算） |

---

## 八、测试验收标准

### 8.1 功能验收（端到端）

| # | 场景 | 预期 |
|---|---|---|
| 1 | Step3 模型列表出现 MiniMax H3，参数（时长/分辨率/比例）可配 | 列表动态渲染，预估积分按秒计算 |
| 2 | 余额充足时提交生成 | 按秒扣费成功（流水可查），任务进入 RUNNING |
| 3 | 余额不足提交 | 拦截，前端提示充值，不建任务 |
| 4 | 生成成功 | `generated_video_url` 写回，ShotCard 可预览，Step4 时间线徽标更新 |
| 5 | 多镜头并发 | 并发上限生效，Key 不超额；失败镜头 PARTIAL_VIDEOS 可重试 |
| 6 | 参考图注入 | 有资产图时走图生视频；无资产图纯文生视频（不报错） |
| 7 | 分数档进度 | 进度条 10→40→90→100 正常推进 |

### 8.2 计费验收（双轨）

| # | 场景 | 预期 |
|---|---|---|
| 1 | 生成一次视频 | 用户扣固定 `cost_points_per_call`（如 100 积分），与秒数无关；流水可查 |
| 2 | 视频任务重试 | 不重复扣用户积分（幂等） |
| 3 | 任务 SUCCESS 后 | `generation_tasks.cost_amount = 实际秒数 × cost_per_second_amount`（仅后台成本），用户余额未再变 |
| 4 | 成本报表 | 管理端按日聚合 `SUM(cost_amount)` 与 AutoDL 账单一致 |
| 5 | 前端暴露 | 用户端**看不到**按秒费用，只显示固定积分预估 |

### 8.3 稳定性验收

- **限流**：并发 5 个任务时，无账号级被 ban；429 自动 Key 切换；轮询期 429 不重建任务。
- **超时**：模拟厂商超时，任务可重试且不重复计费；项目状态不卡死。
- **降级**：生产环境无 mock；开发环境 mock 正常。
- **日志**：按秒扣费流水、provider 事件、错误码均有痕迹，管理端可查。

### 8.4 回归

- 拆分、资产、原有 T2V（火山/可灵）链路不受影响（MoE 按能力路由）。
- 积分余额、充值、扣费整体闭环正常。
- 前端 `tsc -b`、后端 `go build ./... && go test ./internal/...` 全绿。

---

## 九、实施与回滚清单

### 9.1 实施步骤（顺序）

| 步骤 | 内容 |
|---|---|
| 1 | **迁移**：`models` 加 `cost_per_second_amount`（后台成本单价，默认 0）；`generation_tasks` 加 `cost_amount / cost_duration_sec`（成本核算） |
| 2 | **后端 Provider**：实现 `MiniMaxVideoProvider.GenerateVideo`（create+query 任务，字段映射 `seconds/aspect_ratio/resolution`），在 provider 注册处按能力注入（MoE 分派） |
| 3 | **用户计费**：`CreateVideoTasks` 按次扣 `cost_points_per_call`（幂等） |
| 4 | **成本核算**：视频任务 SUCCESS 后 Worker 按 `实际秒数 × cost_per_second_amount` 写入 `cost_amount`；`/models` 返回 `cost_points_per_call`（不暴露成本字段） |
| 5 | **管理端**：厂商/模型/Key 录入 + 用户计费（固定积分/次）+ 成本核算（元/秒 + 成本报表/对账）+ 任务队列监控列 |
| 6 | **前端**：`GenVideoModal` 模型列表动态化 + 时长/比例参数 + 固定积分预估 + 余额预校验；`MODEL_OPTIONS` 静态数组移除或仅作占位 |
| 7 | **联调**：AutoDL 申请真实 Key → 管理端录入 → 端到端视频任务验证 |

### 9.2 验收命令

```bash
# 后端
cd duanju-backend
go build ./... && go test ./internal/engine/model/... ./internal/domain/creation/...
# 前端
cd apps/web-user && pnpm exec tsc -b
```

### 9.3 回滚

- **代码**：Provider 注册处移除 MiniMax 分派即可回退到火山/可灵 + mock（不涉及拆分/资产）。
- **配置**：管理端将 minimax 厂商/模型/Key `enabled=false`；路由自动绕过（`model_providers.enabled` 参与过滤）。
- **数据**：`cost_points_per_second` 列可空，不影响既有模型；流水留存可审计。

---

## 十、注意事项

1. **"按秒"只属于成本，不属于用户收费**：用户侧保持"按次扣固定项目积分"（`cost_points_per_call`）；MiniMax H3 的按秒计费仅作为后台成本核算（`cost_amount`），做成本报表与厂商对账，**前端绝不向用户展示按秒费用**。二者严格分离，避免误把成本转嫁给用户或造成对账不一致。
2. **字段命名差异**：MiniMax H3 用 `seconds` 非 `duration`，若复用 `AsyncTaskProvider` 需加字段映射，建议独立 Provider 更清晰。
3. **并发保护**：视频是高成本长耗时任务，`concurrent_limit` 宁可保守（1~2），并加"每分钟最多 N 个视频任务"的用量护栏，防预算被单用户打爆。
4. **生产禁降级**：已在上游 `video_handler.go` 强制，接入新厂商不得绕过；开发期 mock 保留。
5. **合规**：视频生成输入需过内容安全（提示词/参考图），建议接入审核中间件（对齐 `content_report` 与审核流程），避免生成违规内容。
6. **成本审计**：按秒扣费建议记录 `duration_sec + unit + amount + balance_after`，与厂商账单每日对账。
