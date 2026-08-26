# MiniMax H3 按秒计费视频生成接入技术方案

> **Workspace:** d:\develop\a_work\duanju
> **现状基线：** 视频生成链路已有完整骨架（`VIDEO_SEGMENT_GEN` 任务 → `ModelEngine.GenerateVideo(CapT2V)` → Provider），当前 T2V 仅接入 `AsyncTaskProvider`（火山/可灵形态）与 mock，`OpenAICompatProvider` 的 Video 未实现。本方案在**不重造链路**的前提下，新增 MiniMax H3 视频厂商，并补上"**双轨计费**"这一能力——**用户侧按次扣固定项目积分，厂商侧（MiniMax 按秒 / AutoDL GPU 租金折算）仅作后台成本核算，不转嫁给用户**。
>
> **接入路径（2026-08 调研结论）：** MiniMax H3 已于 2026-08-03 开源（H3-Base 的 FL2VA / Ref2VA 权重），因此有两条可选路径：
> - **路径 A（AutoDL 自部署，本文主推）**：在 AutoDL 租 GPU 实例，用 SGLang 部署开源 H3 权重，经 AutoDL"自定义服务"公网地址对外提供 V2 兼容 API。成本 = GPU 按量租金；多图参考（短剧核心能力）用 Ref2VA 检查点；上限 768P（2K 超分模块未开源，如需 2K 需叠加官方 Regenerate-2K API）。
> - **路径 B（MiniMax 官方 API）**：直连 `https://api.minimaxi.com`，2K 直出，按量计费 768P 0.50 元/秒、2K 0.80 元/秒，无需 GPU。
> 两条路径的请求/响应契约相同（V2 接口），Provider 只需换 BaseURL + Key，可互为备份路由。

---

## 目录

1. 接入背景与能力缺口分析
2. 接入准备工作（AutoDL 自部署 / MiniMax 官方 API 条件清单）
3. 管理端配置项
4. 前端实现要点
5. 多图参考提示词模板（Ref2VA，短剧向）
6. 前后端交互流程设计
7. 错误处理与异常应对策略
8. 性能优化建议
9. 测试验收标准
10. 实施与回滚清单
11. 注意事项

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

### 1.2 MiniMax H3 官方 V2 API 能力速览（2026-08）

H3 走 **V2 多模态接口**（注意：不是旧版 Hailuo 的 `v1/video_generation`，字段完全不同）：

| 项 | 规格 |
|---|---|
| 端点 | `POST /v2/video_generation`（创建，返回 `task_id`）；`GET /v2/query/video_generation?task_id=`（轮询） |
| 鉴权 | `Authorization: Bearer <API Key>`（V2 无需 GroupId 路径参数） |
| model | 固定 `MiniMax-H3` |
| 输入 | `content[]` 多模态数组：`text`（必填非空）+ `image_url` / `video_url` / `audio_url`，用 `role` 标注用途 |
| 五种模式 | ① 纯 T2V（仅 text）② 首帧 I2V（`role=first_frame`）③ 尾帧（`last_frame`）④ 首尾帧（各 1 张）⑤ **多模态参考 R2V**（`reference_image` ≤9 张 + `reference_video` ≤3 段 + `reference_audio` ≤3 段，合计 ≤12 个文件）；**首尾帧 role 与参考 role 互斥，不可混用** |
| duration | 整数 4–15 秒（注意：字段名是 `duration`，不是 `seconds`） |
| resolution | `768P` / `2K`（没有 P720/P1080 档） |
| ratio | T2V 必填且不可 `adaptive`：`21:9/16:9/4:3/1:1/3:4/9:16`；I2V/R2V 由参考输入决定，恒 `adaptive` |
| 输出 | 原生 32kHz 立体声（音画同生）、24FPS、短边 768 或 2K |
| 图片限制 | JPG/JPEG/PNG/WEBP/HEIC/HEIF，单张 ≤30MB，宽高 [256,5760]px，长宽比 [0.4,2.5] |
| 请求体 | 总大小 ≤64MB；**参考图务必走公网 URL，勿 Base64** |
| Prompt | ≤7000 字符 |
| 任务状态 | `succeeded` / `failed` / `cancelled`（+ 处理中间态）；结果取 `task.content.url` |
| 三模块 | H3-Context-IR（提示词增强，托管 API `POST /v2/h3_context_ir`）、H3-Base（开源，768P）、H3-Regenerate-2K（2K 超分，托管 API `POST /v2/video_regeneration`，未开源） |

### 1.3 能力缺口：双轨计费（用户按次 + 成本按秒/按 GPU 折算）

| 缺口 | 现状 | 影响 |
|---|---|---|
| **用户计费粒度** | `models.cost_points_per_call` 按**单次调用**计费 | ✅ 正好匹配"用户侧一次性扣固定项目积分"，**无需**改用户计费口径 |
| **厂商成本粒度** | 仅计算 `cost_points_per_call`，**无成本核算字段** | 路径 B 按**生成秒数 × 单价**（768P 0.5 元/s、2K 0.8 元/s）；路径 A 按 **GPU 租金折算**（元/小时 ÷ 有效产出）。需单独记录作为后台成本/对账依据，且**不转嫁用户** |
| **厂商差异** | `AsyncTaskProvider` 以火山/可灵为模板，字段固定（`duration`） | H3 V2 字段名不同（`content[]` + `role` + `ratio` + `resolution` 枚举 768P/2K），查询走 `GET /v2/query/video_generation`，建议独立 Provider |
| **多图参考** | 现有 `VideoRequest.RefImageURLs` 仅支持"首图"语义 | H3 的 R2V 支持 ≤9 张参考图，且**参考图 role 与首尾帧互斥**——需把镜头关联资产（角色/场景/道具图）组装为 `reference_image` 序列，并在 prompt 中声明每张图的用途（见第五章模板） |

**结论**：MiniMax H3 的接入是"**新增一个 Provider + 成本核算字段 + 管理端/前端配置 + 多图参考 prompt 组装**"，复用既有任务队列、路由、降级、心跳机制。**用户收费保持"按次固定积分"（`cost_points_per_call`），按秒/按 GPU 成本仅落入后台核算，不在前端向用户展示费用。**

---

## 二、接入准备工作（AutoDL 自部署 / MiniMax 官方 API 条件清单）

### 2.1 路径选型对比

| 维度 | 路径 A：AutoDL 自部署（SGLang + 开源权重） | 路径 B：MiniMax 官方 API |
|---|---|---|
| 前置门槛 | AutoDL 账号 + 个人实名 + GPU 预算 | MiniMax 开放平台账号 + 实名 + 充值 |
| 最高分辨率 | **768P**（2K 需再叠加官方 Regenerate-2K API，未开源） | **2K 直出**（一次调用含三模块） |
| 多图参考 | ✅ Ref2VA 检查点（≤9 图 + 3 视频 + 3 音频） | ✅ 同左（同一 V2 接口） |
| 成本模型 | GPU 按量租金（元/小时，如 4×80G 卡机型；以控制台实价为准），关机不收计算费 | 768P 0.50 元/秒、2K 0.80 元/秒（按量；视频资源包**不支持** H3） |
| 冷启动 | 实例开机 + 模型加载需数分钟；低频使用需"按需开机"策略 | 无 |
| 运维 | 自管（权重下载、SGLang 升级、显存 OOM 排查） | 无 |
| 合规 | 数据不出自管范围（参考图仅传到自己的实例） | 参考图上传 MiniMax |

> **短剧场景建议**：主角一致性依赖多图参考（R2V），768P 已满足短剧分发（竖屏 9:16）；若走 AutoDL 长期开机，成本可控且参考图不出自管域。追求极致画质再开路径 B 的 2K。两条路径 Provider 契约一致，可同时录入、按 priority 路由互备。

### 2.2 路径 A：AutoDL 自部署条件清单（需逐一准备）

| # | 条件 | 说明 |
|---|---|---|
| A1 | **AutoDL 账号 + 实名认证** | [autodl.com](https://www.autodl.com) 手机号注册；个人实名即可创建容器实例、使用"容器实例 Pro API"。（**企业认证**仅在做"弹性部署 API / 开放任意端口"时才必需；6006/6008 端口自带公网映射，无需企业认证） |
| A2 | **账户充值** | GPU 按量计费。机型建议：单检查点推理 **4×A100 80G 或 4×H100 80G**（BF16 官方权重）；预算有限可用社区量化权重（Comfy-Org INT8 版约 43GB，消费级卡可跑但非服务化形态，不推荐生产）。具体价格以控制台为准 |
| A3 | **数据盘扩容** | 权重存放 `autodl-tmp`。官方 HF 仓库 `MiniMaxAI/MiniMax-H3`：FL2VA（T2V/首尾帧）与 Ref2VA（多图参考）两个检查点，建议数据盘预留 **≥200GB**；国内用 ModelScope 镜像 `MiniMax/MiniMax-H3` 或开启 AutoDL 学术加速下载 |
| A4 | **SGLang 服务化部署** | `pip install sglang` 后：`sglang serve --model-path MiniMax-H3 --num-gpus 4 --ulysses-degree 4 --performance-mode speed --host 0.0.0.0 --port 6006 --model-variant ref2va --api-key <自定强随机令牌>`。短剧以多图参考为主 → 单实例只跑 **ref2va**；如同时要纯 T2V/首尾帧，再开一台 fl2va 实例 |
| A5 | **公网服务地址（= 管理端 BaseURL）** | SGLang 监听 **6006 端口**（AutoDL 仅 6006/6008 映射公网），在控制台"**自定义服务**"入口复制地址，形如 `https://u39-xxxx.seetacloud.com:8443`。这就是管理端 `model_providers.base_url` 要填的值 |
| A6 | **API Key（= 管理端密钥）** | 即 A4 启动参数 `--api-key` 自定令牌（SGLang 校验 `Authorization: Bearer <令牌>`）。**没有平台代发 Key，令牌由我们自己生成并保密**；建议 32 位随机串，泄漏可重启更换 |
| A7 | **（可选）实例 API 自动化** | 需"任务来了开机、空闲关机"省钱时用：容器实例 Pro API（个人实名可用，HOST `https://api.autodl.com`，Token 在 控制台→账号→设置→**开发者Token**）；或弹性部署 ReplicaSet（**需企业认证**，容器故障自动替补）。v1 阶段可先手动开关机 |
| A8 | **参考图可达性** | 实例需能拉取我方资产图公网 URL（火山 Ark 图片 URL 等）；AutoDL 实例默认可访问公网，无需额外白名单 |

### 2.3 路径 B：MiniMax 官方 API 条件清单

| # | 条件 | 说明 |
|---|---|---|
| B1 | **开放平台账号 + 实名** | [platform.minimaxi.com](https://platform.minimaxi.com)（国内）/ platform.minimax.io（海外）注册并实名 |
| B2 | **API Key** | 「账户管理 → 接口密钥」创建，**仅创建时明文展示一次**；V2 接口 Bearer 鉴权，**不需要 GroupId** |
| B3 | **充值余额** | 按量计费：`MiniMax-H3` 视频 **2K 0.80 元/秒、768P 0.50 元/秒**（一次性包含 Context-IR/Base/Regenerate 三模块）；视频资源包（点数）暂不支持 H3，只能按量或商务定制 |
| B4 | **并发/限流确认** | 确认账号级 QPS/并发上限（决定管理端 `model_keys.concurrent_limit`）；按量档默认较保守，扩容联系商务 |
| B5 | **（可选）Context-IR 单独调用** | 路径 A 若要提示词增强（官方强烈建议），需另备同一把官方 Key 调 `POST /v2/h3_context_ir`（按 token 计费） |

### 2.4 Toonflow 参考实现调研（refer/Toonflow-app）

Toonflow（`refer/Toonflow-app`）是已商用跑通的 AI 短剧制作应用，其"GPU 云（智星云，与 AutoDL 同类）+ 远程模型 API"模式与我们的目标场景一致。关键机制（代码证据）：

| 机制 | 实现 | 对我们的启示 |
|---|---|---|
| **可编程供应商系统** | vendor 适配器为 TS 文件（`data/vendor/minimax.ts` 等），在 [vm.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/utils/vm.ts) 的 vm2 沙盒中热加载——**设置中心直接编辑 TS 即时生效，无需改源码/重启**（README L123） | 我方 Go 后端 Provider 虽不可热编程，但管理端 `model_providers/models/model_keys` 三表配置 + 独立 `MiniMaxH3Provider` 等价于 Toonflow vendor 的 `inputs(apiKey, baseUrl)`；若后续模型频繁上新，可评估"字段映射配置化" |
| **vendor = 声明式模型清单 + 适配函数** | 每个 vendor 声明 `inputs`（apiKey/baseUrl）+ `models`（含多参考声明 `["imageReference:9","videoReference:3","audioReference:3"]`、durationResolutionMap）+ `videoRequest` 适配函数 | 其 Seedance 多参考代码（toonflow.ts L524-551）实际请求格式 `references: [{role:"reference_image", type:"image_url", image_url:{url}}]` 与本方案 5.1 的 H3 V2 组装一致，**相互印证** |
| **多图参考 prompt 模板** | `data/modelPrompt/video/universalMulti-parameterMode.md`：`[References]` 段 `@图N : [资产名参考图]` 逐图声明 + `[Instruction]` 段英文正文内嵌 `@图N` 引用（见 5.2 补充） | 生产验证过的图序引用结构，吸收进本方案提示词模板 |
| **GPU 云部署形态** | Toonflow 本体是纯 Node 编排服务（无 GPU 推理），智星云教程：租 4090 → Windows 镜像（内置 Toonflow 桌面版）→ RDP → 设置中心填 API Key/请求地址 | 证明"AutoDL 类 GPU 云 + 编排服务 + 远程生成 API"模式可行。Toonflow 内置 `minimax.ts` 仅有旧 Hailuo v1 模型（2.3/02），**无 H3**——但其 vendor 可编程，新增一个 H3 V2 vendor 即可接入，这正是本方案 Provider 的做法 |
| **轮询与工具链** | 沙盒注入 `pollTask`（默认 3s 间隔/可传参）、`zipImage`（参考图压至目标体积）、`urlToBase64` | 参考图体积控制与轮询工具化值得对齐：我方 Worker 已有 pollTask，参考图压缩可在 Provider 前置处理 |

> **结论**：Toonflow 的"在 AutoDL 类平台配置并使用 MiniMax H3"= ① Toonflow 编排服务跑在 GPU 云实例；② 通过可编程 vendor 指向 H3 的 HTTP 端点（官方或自部署 V2 兼容服务）。它**没有**在 AutoDL 实例上直接跑 H3 推理（其内置 minimax vendor 走 api.minimaxi.com 旧版 v1 接口）。我方方案与该模式兼容：路径 A（AutoDL 跑 SGLang 出 V2 端点）/ 路径 B（直连官方）均可作为 `MiniMaxH3Provider` 的 BaseURL。

### 2.5 凭据汇总（管理端录入对照表）

| 配置项 | 路径 A：AutoDL 自部署 | 路径 B：MiniMax 官方 |
|---|---|---|
| 厂商 BaseURL | `https://<实例自定义服务地址>.seetacloud.com:8443`（6006 映射） | `https://api.minimaxi.com` |
| API Key | 自定 SGLang `--api-key` 令牌（我方生成） | 官方「接口密钥」页创建（仅明文一次） |
| GroupId | 不需要 | 不需要（V2） |
| model | `MiniMax-H3` | `MiniMax-H3` |
| 能力 | `T2V` + `I2V/R2V`（多图参考） | 同左 |
| 成本口径 | GPU 元/小时折算（录入估算单价） | 768P 0.50 元/秒、2K 0.80 元/秒 |

---

## 三、管理端配置项

### 3.1 模型与厂商录入（复用现有 admin 端点）

管理端通过既有 `POST /api/v1/admin/models` / `model-keys` 等接口（[admin_extra_service.go](file:///d:/develop/a_work/duanju/duanju-backend/internal/domain/creation/admin_extra_service.go#L758-L769)）录入。**建议两条路径各录一条 provider（如 `minimax-h3-autodl`、`minimax-h3-official`），模型层共用 model_key，路由层按 priority 互备。**

**厂商 `model_providers`**（以 AutoDL 主路径为例）：
| 字段 | 值 | 备注 |
|---|---|---|
| name | `minimax-h3-autodl` | 唯一标识（与 Worker 注册名一致，避免火山中英文不一致的教训） |
| display_name | `MiniMax H3（海螺·自部署）` | 展示名 |
| capabilities | `["T2V","I2V"]` | provider 能力声明（`mp.capabilities ? $1` 参与路由） |
| base_url | `https://u39-xxxx.seetacloud.com:8443` | AutoDL 自定义服务地址；路径 B 填 `https://api.minimaxi.com` |

**模型 `models`**：
| 字段 | 值 | 备注 |
|---|---|---|
| provider_id | minimax-h3-autodl 的 id | |
| model_key | `MiniMax-H3` | 透传给厂商 API 的模型名（官方枚举值，注意大小写） |
| capability | `T2V` | 路由时 `m.capability::text = 'T2V'`；R2V 由"是否带参考图"在 Provider 内分流 |
| cost_points_per_call | `100` | **用户侧按次扣费**（一次性固定积分，与秒数无关） |
| enabled | true | |

**API Key `model_keys`**：
| 字段 | 值 | 备注 |
|---|---|---|
| provider_id | minimax-h3-autodl | |
| name | `h3-autodl-prod` | |
| key_encrypted | (Admin 传明文→后端加密) | 路径 A=自定令牌；路径 B=官方 Key |
| rate_limit_rpm | 依配额 | 路径 A 取决于单实例吞吐；路径 B 依账号档位 |
| concurrent_limit | 建议 1~2 | 自部署=GPU 并发上限（4 卡 BF16 建议 1）；官方按档位 |
| priority | 10 | 主路径高优；另一路径设 5 作备份 |

### 3.2 双轨计费规则（用户按次 + 成本按秒/按 GPU 折算）

**用户侧（对用户收费，保持按次）**：沿用 `models.cost_points_per_call`——每次 `VIDEO_SEGMENT_GEN` 任务提交，扣固定积分（如 100），**与秒数无关**。幂等（重试不重复扣，对齐资产 `point_cost_applied` 机制）。这是唯一与用户积分余额交互的口径。

**成本侧（后台核算，不转嫁用户）**：
- 路径 B（官方）：`成本(元) = 实际生成秒数 × 单价`（768P 0.50 / 2K 0.80）。
- 路径 A（AutoDL）：`成本(元) = GPU 小时租金 × 占用时长`；为对齐同一核算口径，可按"单条视频有效生成耗时（含排队）× 机型折算单价"折算成等效元/秒录入。

**迁移** `新增成本核算字段`（供后台对账，不参与用户扣费）：
```sql
-- 算法表：每模型配置"每秒成本单价（元）"，用于成本报表与厂商对账
ALTER TABLE models ADD COLUMN IF NOT EXISTS cost_per_second_amount NUMERIC(12,4) NOT NULL DEFAULT 0;
-- 任务成本流水：视频任务完成后记录实际耗时与成本金额
ALTER TABLE generation_tasks ADD COLUMN IF NOT EXISTS cost_amount NUMERIC(12,4) NOT NULL DEFAULT 0;
ALTER TABLE generation_tasks ADD COLUMN IF NOT EXISTS cost_duration_sec INT NOT NULL DEFAULT 0;
```

**成本公式（仅后台核算）**：
```
厂商成本(元) = 实际生成秒数 × models.cost_per_second_amount
```
- 视频任务 SUCCESS 后，由 Worker 按 `实际秒数`（来自 `VideoRequest.DurationSec` 或厂商返回的视频时长）计算成本，写入 `generation_tasks.cost_amount / cost_duration_sec`。
- **管理端成本模块**：按厂商/模型/日聚合成本报表（`SUM(cost_amount)`），与 AutoDL GPU 账单（路径 A）或 MiniMax 账单（路径 B）**每日对账**；不出现在用户端。

**管理端计费规则管理**（新增模块，两栏）：
- 用户计费栏：`cost_points_per_call`（固定积分/次）展示与编辑——这是用户看到的收费。
- 成本核算栏：`cost_per_second_amount`（元/秒）展示与编辑——仅后台对账用，标注"厂商成本，不转嫁用户"。
- 成本监控：日内成本报表 + 超阈值告警（防异常调用耗光预算/GPU 空转烧钱）。

### 3.3 任务队列监控

复用现有 `generation_tasks` 与 `task_store.go`，管理端任务列表可新增**视频任务列**：

| 监控项 | 实现 |
|---|---|
| 任务类型审计 | `task_type = VIDEO_SEGMENT_GEN`，区分 `model_used`（应显示 `MiniMax-H3`）与 provider（autodl/official） |
| 状态着色 | QUEUED（排队）/ RUNNING（生成中，含进度 pct）/ SUCCESS / FAILED_DEAD |
| 进度回写 | H3 为轮询制，进度近似难实时——采用**分档提示**（10% 已提交 → 40% 引擎处理中 → 90% 渲染中 → 100% 完成），由 Provider 轮询到中间态/耗时阈值时映射 |
| 失败率/耗时 | 统计视频任务 P50/P95 耗时与失败率，用于调 `concurrent_limit` 与 `videoTimeout`；路径 A 另监控实例开机状态与显存 |

### 3.4 日志记录

视频链路独立日志 tag：`video: shot=... model=MiniMax-H3 provider=autodl task=...`。
- **Request 日志**：入参摘要（prompt 截断、duration、ratio、resolution、refs 数量与 role 分布）——不落完整 prompt 全文（隐私+体积）。
- **Provider 事件**：createTask 返回的 `task_id`、每次轮询的状态、最终成功/失败、耗时（对齐现有 `video_handler.go` 的 `log.Printf`）。
- **错误日志**：厂商错误码埋点（400 参数错含"role 混用/图片超限"、429 限流、5xx 服务端、自部署额外区分 502 网关=实例关机/加载中）→ 管理端错误报表。
- **审计**：成本核算流水（task_id、duration_sec、单价、成本金额）入库，便于对账。

---

## 四、前端实现要点

### 4.1 视频参数输入界面（Step3 GenVideoModal 扩展）

现有 `MODEL_OPTIONS`（Seedance 2.5/2.0/1.5）为静态数组，改为**由后端 `/api/v1/models?capability=T2V` 驱动**：
- 动态渲染模型列表（新增 `MiniMax H3 海螺` 一项），每个模型携带 `cost_points_per_call` 与预估说明。
- 新增参数（对齐 H3 V2 枚举）：
  - `时长`：4–15 秒任意整数（提供 5/8/10/15 快捷档）；
  - `分辨率`：`768P` / `2K`（路径 A 仅 768P 可选，2K 置灰并提示"需官方 API"）；
  - `比例`：`21:9 / 16:9 / 4:3 / 1:1 / 3:4 / 9:16`（短剧默认 9:16；带参考图时该选项隐藏——R2V 比例由参考图自适应 `adaptive`）。
- 参考图注入：已有 `RefImageURLs`（镜头级+单元级），提示"MiniMax H3 支持最多 9 张多图参考（角色/场景/道具），未就绪资产将走纯文生视频"（对齐 P1 软门控）。

### 4.2 生成进度展示

- 复用 `useWsDrivenPoll` + `TASK_PROGRESS`；H3 轮询制下后端按分档回写进度（10/40/90/100），前端 ShotCard 进度条按 `pct` 渲染。
- 长耗时提示：视频生成 30s~5min（自部署 4 卡 BF16 更久），悬浮条显示"视频渲染中…（约 x 秒）"，提供"返回 Step2 继续处理资产"不阻塞。
- 终态：TASK_SUCCESS → 刷新项目状态（对齐现有 `refreshVideos` 的 `videoDoneRef`）。

### 4.3 结果预览

- ShotCard 成功态视频 `<video>` 预览（复用现有 `generated_video_url`）；H3 产出**自带立体声音频**，预览控件保留音量。
- Step4 成片预览：`ALL_VIDEOS_READY`/`PARTIAL_VIDEOS` 合成导出（现有逻辑不改）；H3 产出的 URL 走同一 `generated_video_url` 落地。

### 4.4 费用预估（用户侧按次，不展示厂商成本）

- 弹窗"预计消耗"改为**固定积分单价**：`本轮预计消耗 X 积分`（读 `models.cost_points_per_call`），与秒数无关。
- **不向用户展示按秒费用**：厂商成本属后台核算，前端零暴露（仅可显示"预计生成时长约 x 秒"作等待提示，非计费口径）。
- **余额校验前置**：调用 `POST /api/v1/points/preview`（对资产同接口，传本次固定积分），余额不足则置灰"开始生成"并提示充值。
- 取消静态 `factor`（4/3/2）与"时长×单价"估算，改为读 `cost_points_per_call`。

---

## 五、多图参考提示词模板（Ref2VA，短剧向）

H3 多图参考（R2V）的核心约定：**参考图以 `role=reference_image` 依序传入 content 数组，prompt 中必须显式声明"第 N 张图是什么用途"**，模型才能把正确的角色身份/场景风格绑定到画面。官方样片的通用句式即 "Use Figure 1 as ... reference, Figure 2 as ... reference"。

### 5.1 请求组装规则

```json
POST /v2/video_generation
{
  "model": "MiniMax-H3",
  "content": [
    { "type": "text", "text": "<提示词，见 5.2 模板>" },
    { "type": "image_url", "image_url": { "url": "<图1：男主角色图>" }, "role": "reference_image" },
    { "type": "image_url", "image_url": { "url": "<图2：女主角色图>" }, "role": "reference_image" },
    { "type": "image_url", "image_url": { "url": "<图3：场景图>" },   "role": "reference_image" }
  ],
  "resolution": "768P",
  "duration": 8,
  "ratio": "adaptive"
}
```

- **图序约定（后端组装顺序，prompt 与之严格对应）**：① 出场角色变体图（按镜头对白/戏份排序）→ ② 场景主图 → ③ 关键道具图；总数 ≤9 张，超限时按"角色 > 场景 > 道具"优先级裁剪。
- **role 互斥**：出现任意 `reference_image` 后，**不得**再传 `first_frame`/`last_frame`（反之亦然），混用直接 400。我方链路统一走 R2V（refs 全部作为参考），不做首帧模式。
- 带参考图时 `ratio` 恒 `adaptive`（由参考输入决定）；纯 T2V 时 `ratio` 必填且不可 `adaptive`。
- 图片走公网 URL（总请求体 ≤64MB）；prompt ≤7000 字符。
- 纯 T2V（无任何就绪资产图）时 content 仅含 text，落回首节模式。

### 5.2 提示词模板（镜头级，中文可直接使用）

H3 稳定支持中文提示词（11 种语言含中文）。模板分六段，方括号为占位符，由 `LoadShotRefs` + 镜头字段填充：

```
[1 任务与画风]
生成一段 {duration} 秒、{ratio|9:16 竖屏} 的短剧实拍镜头，{风格关键词：如都市情感短剧/古装悬疑，电影感打光，写实质感}。

[2 参考图约定]
- 图1：男主角{角色名}的外貌与服装参考，全程保持其面部特征、发型和体型一致；
- 图2：女主角{角色名}的外貌与服装参考，全程保持其面部特征、发型和体型一致；
- 图3：场景{地点名}的空间参考，还原布局、陈设与光线氛围。

[3 镜头语言]
{景别与运镜：如中近景，缓慢推镜 / 过肩正反打 / 手持跟拍}。

[4 表演与节拍]
{按时间顺序的动作与情绪 beats：0-3 秒女主低头搅动咖啡，抬眼；3-6 秒男主入画落座，两人对视；6-8 秒女主别过头，男主开口。}

[5 台词与音频]
对白：{「台词内容」—角色名}。
环境音：{如咖啡馆嘈杂人声、杯碟轻碰}。
音乐：{如轻柔钢琴，克制不抢戏}。

[6 约束]
两人身份与服装严格遵循参考图；{负面项：如无恐怖元素、无跳吓、无花字、不出现字幕}。
```

**要点**：
- **图序声明是硬要求**：每张参考图必须在 [2] 段有一一对应的"图N：…"用途句（角色→外貌一致性；场景→空间还原；道具→特定物件出现）。缺失用途声明的图会被模型弱利用。
- **身份一致性**：角色参考句固定使用"保持面部特征/发型/体型一致"措辞（官方样片同款句式 "Keep both leads' identities consistent"）。
- **音画同生**：H3 原生生成 32kHz 立体声，[5] 段环境音/对白/音乐描述会直接驱动音轨生成——短剧对白镜头务必填写台词。
- **首尾帧需求**：如某镜头需精确控制首帧（如分镜关键帧），不能用 R2V，改走 FL2VA（`role=first_frame`，prompt 只描述运动），v1 暂不做。

**变体：`@图N` 引用式模板（Toonflow 生产验证，推荐用于批量自动化）**

Toonflow（`refer/Toonflow-app/data/modelPrompt/video/universalMulti-parameterMode.md`）用两段式英文结构替代逐段中文声明，正文以 `@图N` 内嵌引用，更紧凑且由 Agent 自动生成。其核心规则：资产按输入顺序从 `@图1` 连续编号（不按类型归组），分镜图编号接续资产之后；正文无需描述角色外观（由参考图负责）。示例：

```
[References]
@图1 : [沈辞参考图]
@图2 : [苏锦参考图]
@图3 : [城楼参考图]
@图4 : [分镜图1]

[Instruction]
@图1 standing alone atop the city wall, hands clasped behind back, robes billowing in the wind,
@图2 ascending the steps toward @图1 , expression worried,
set in the ancient city wall environment of @图3 ,
wide shot transitioning to medium tracking shot, cinematic,
resolute determination shifting to concern, dusk cold-toned side-backlit atmosphere,
no dialogue, wind howling, fabric flapping, footsteps on stone.
```

两种模板二选一即可（同一镜头内不混用）：5.2 六段中文模板直观可控，适合用户可见的编辑场景；`@图N` 式紧凑、适合由拆分 Agent 按分镜 12 字段（画面/场景/资产/时长/景别/运镜/动作/情绪/光影/台词/音效/资产ID）自动批量渲染。**实现建议**：Provider 侧同时支持，`generation_tasks` 参数带 `prompt_style: "sectional" | "at"`，默认 `at`。

### 5.3 官方增强路径：H3-Context-IR（可选开关）

官方强烈建议先把自由文本 prompt 送 `POST /v2/h3_context_ir` 增强为结构化提示词（含 `integrated_multimodal_description`（镜头综合描述）/ `overall_soundscape`（整体音景）/ `non_diegetic_music`（非剧情音乐）三字段），质量提升显著，但按 token 另行计费且仅官方 API 提供。**v1 决策：不接 Context-IR，直接用 5.2 模板手写结构化 prompt（模板六段即按 IR 输出结构设计）；管理端预留 `enable_context_ir` 开关，后续按成本效果决定。**

### 5.4 与现有链路的映射

| 现有字段/逻辑 | H3 R2V 映射 |
|---|---|
| `LoadShotRefs`（角色→character_variants 首个变体；场景/道具→main_image_url） | 依序组装为 `reference_image` 数组（≤9），与 prompt [2] 段图序一一对应 |
| `linked_assets`（Step3 镜头关联资产） | 决定参考图集合与裁剪优先级（角色>场景>道具） |
| `VideoRequest.RefImageURLs` | Provider 内按顺序转 `content[].image_url.url` |
| `VideoRequest.Prompt` | 5.2 模板渲染结果 |
| `VideoRequest.DurationSec` | `duration`（4–15 整数，后端 clamp） |
| `VideoRequest.Resolution` | `resolution`（归一化为 768P/2K，拒绝 P720/P1080 等旧枚举） |
| aspect_ratio | 有参考图→`adaptive`；纯 T2V→透传用户选择（不可 adaptive） |

---

## 六、前后端交互流程设计

### 6.1 时序（提交到成片）

```
前端 Step3 选 MiniMax H3 + 时长/分辨率/比例  ① 参数提交
  │
  ├─ POST /api/v1/points/preview（预估固定积分，余额校验）
  │
  ├─ POST /beats/:beat_id/generate-videos
  │     Body: { model_key:"MiniMax-H3", duration_sec, resolution, aspect_ratio }
  │         └─ 后端按次校验积分余额 → 扣固定 cost_points_per_call → 状态 VIDEO_GENERATING → 建 VIDEO_SEGMENT_GEN 任务
  │              （成本不在此扣用户）
  │
  └─ Worker: VideoHandler.Execute
        ├─ LoadShot → LoadShotRefs（角色/场景/道具参考图）→ dedupe → ≤9 张裁剪
        ├─ 渲染 5.2 多图参考 prompt（图序与 refs 严格对应）
        ├─ ModelEngine.GenerateVideo(CapT2V, VideoRequest{model_key, duration_sec, resolution, refs})
        │     └─ MiniMaxH3Provider: POST {BaseURL}/v2/video_generation → task_id
        │           ├─ BaseURL = AutoDL 自定义服务地址（路径 A）或 api.minimaxi.com（路径 B）
        │           └─ 轮询 GET /v2/query/video_generation?task_id=（10s~300s 智能退避）
        │                 └─ task.status: succeeded → 取 task.content.url；failed/cancelled → 报错
        │           └─ SUCCESS 后成本核算：cost_amount = 实际秒数 × cost_per_second_amount（仅后台，不扣用户）
        └─ SaveShotVideo（CAS 幂等）→ UpdateShotGroupStatus → 推送 TASK_SUCCESS
              └─ 前端 WS 驱动刷新 shots → Step4 解锁
```

### 6.2 接口契约（新增 Provider）

**新增 `MiniMaxH3Provider`**（独立于 `AsyncTaskProvider`，V2 形态），关键字段映射：

| 我方 VideoRequest | MiniMax H3 V2 API | 说明 |
|---|---|---|
| ModelKey | `model` | 固定 `MiniMax-H3` |
| Prompt | `content[0] = {type:"text", text}` | 必填非空；多图参考时含图序声明 |
| RefImageURLs | `content[i] = {type:"image_url", image_url:{url}, role:"reference_image"}` | ≤9 张，公网 URL；与 first/last_frame 互斥 |
| DurationSec | `duration` | 整数 4–15，越界 clamp 后仍需用户侧限档 |
| Resolution | `resolution` | `768P`/`2K` 枚举归一化 |
| AspectRatio | `ratio` | 有参考图→`adaptive`；纯 T2V→必填六选一 |
| — | 认证 | `Authorization: Bearer <Key>`（无 GroupId） |

**响应与轮询**：
- 创建成功：`200 {"task_id": "424010985738629"}`。
- 轮询：`GET /v2/query/video_generation?task_id=` → `task.status`（`succeeded`/`failed`/`cancelled` + 中间态），成功取 `task.content.url`（视频公网 URL，含音频轨）。
- 可选 `callback_url` 回调免轮询（v1 不用，保持轮询与现有 `pollTask` 一致）。

**/api/v1/models 返回扩展**：模型对象返回 `cost_points_per_call`（用户计费口径，前端预估用）。新增后台字段 `cost_per_second_amount` 仅管理端读取，`/models` 用户接口**不暴露**（避免泄漏厂商成本）。

**成本核算落库**：`generation_tasks` 新增 `cost_amount / cost_duration_sec`，任务 SUCCESS 时写入（仅后台对账，不参与用户余额扣减）。

---

## 七、错误处理与异常应对策略

| 异常 | 策略 | 代码点 |
|---|---|---|
| **厂商 400 参数错**（role 混用、duration 越界、参考图>9 张/超 30MB/比例不支持） | 不可重试错误，直接 FAILED，前端 toast 展示厂商 message（V2 错误体含错误类型 + message + `request_id`） | Provider `NewRetryableError(0,...)` 之外返回可读错误；handler 记录 |
| **厂商 429 限流** | 可重试：进入 30s Key 冷却 → 下一 Key；轮询阶段 429 等待后继续（不重建任务，避免重复计费） | 对齐 `async_provider.pollTask` 的 429 分支 |
| **厂商 5xx / 自部署 502** | 可重试 + Key 切换；路径 A 的 502/超时优先怀疑实例关机或模型加载中 → 告警"检查 AutoDL 实例状态"，可配置自动开机（A7） | engine.go 路由层 |
| **任务超时（>30min）** | 返回可重试错误 → 上层重试；单 Key 并发信号量释放 | `videoTimeout` 30min 默认；自部署 BF16 建议按 `duration` 动态放宽 |
| **余额不足** | 提交前拦截，前端提示充值；扣费失败回滚任务状态 | CreateVideoTasks 事务内校验 |
| **任务执行中断**（worker 崩溃） | ReapExpired 回收 + 分档回滚项目状态（拆分修复的经验复用）；`VIDEO_GENERATING` 兜底回退 `PARTIAL_VIDEOS`/`ALL_VIDEOS_READY` | task_store.ReapExpired + main.go Notifier 回滚 |
| **重复扣费**（重试） | 幂等：`point_cost_applied`/任务级已扣费标记，重试不再扣 | 对齐资产 CreateAssetTask |
| **生产降级** | 生产环境禁止 mock 降级（Major#16），`h.env=="production"` 直接报错；开发期可 mock 样例 | video_handler.go L154-159 |
| **鉴权失败** | Key 解密失败跳过该 Key 并告警；全部损坏明确报错，不静默降级；路径 A 令牌更换后管理端同步更新 | model_keystore.go decryptKey |

---

## 八、性能优化建议

| 维度 | 建议 |
|---|---|
| **并发** | 路径 A 单实例 `concurrent_limit=1`（4 卡 BF16 串行最稳）；官方按档位 1~2。多 Key/多实例轮询 + priority 分级 |
| **轮询间隔** | 智能退避：前 60s 每 10s 查，其后每 20s；避免高频查询触发限流（对齐 `pollTask` 的 `pollInterval` 动态化） |
| **超时** | `videoTimeout` 按 `duration` 线性放宽：`max(5min, duration×30 + 120s)`（自部署 BF16 更慢，系数取宽） |
| **进度节流** | Progress 上报 250ms 节流 + 分档（10/40/90/100），防消息风暴（429 治理经验） |
| **实例成本**（路径 A 专属） | 低峰关机（只收数据盘存储费）+ 任务队列驱动开机（容器实例 Pro API）；冷启动含权重加载约 5~10min，需在 `videoTimeout` 外单独预算 |
| **缓存** | 模型列表 `/models?capability=T2V` 接入 `cachedApi` TTL（60s），前端重复挂载不重复请求 |
| **限流** | 后端 `rate_limit` 生产 120/min 已稳；视频提交接口建议单独 `Limit` 或按套餐放宽（用户单次提交为低频操作） |
| **成本监控** | 成本流水入库 → 管理端日内成本报表，超阈值告警（防异常调用打爆官方余额/GPU 空转烧钱） |

---

## 九、测试验收标准

### 9.1 功能验收（端到端）

| # | 场景 | 预期 |
|---|---|---|
| 1 | Step3 模型列表出现 MiniMax H3，参数（时长 4–15/分辨率 768P・2K/比例）可配 | 列表动态渲染，预估积分按**次**固定显示 |
| 2 | 余额充足时提交生成 | 按次扣费成功（流水可查），任务进入 RUNNING |
| 3 | 余额不足提交 | 拦截，前端提示充值，不建任务 |
| 4 | 生成成功 | `generated_video_url` 写回，ShotCard 可预览（含音频），Step4 时间线徽标更新 |
| 5 | 多镜头并发 | 并发上限生效（自部署=1），失败镜头 PARTIAL_VIDEOS 可重试 |
| 6 | **多图参考** | 镜头关联 2 角色+1 场景时，请求含 3 张 `reference_image` 且 prompt 图序声明一一对应；成片角色外貌与参考一致 |
| 7 | 参考图超限 | 关联资产 >9 张时按 角色>场景>道具 裁剪，prompt 同步裁剪，不报 400 |
| 8 | 无资产图 | 纯 T2V：`ratio` 必填非 adaptive，正常出片 |
| 9 | role 混用防护 | 前端/后端不出现 first_frame 与 reference_image 混用请求 |
| 10 | 分档进度 | 进度条 10→40→90→100 正常推进 |

### 9.2 计费验收（双轨）

| # | 场景 | 预期 |
|---|---|---|
| 1 | 生成一次视频 | 用户扣固定 `cost_points_per_call`（如 100 积分），与秒数无关；流水可查 |
| 2 | 视频任务重试 | 不重复扣用户积分（幂等） |
| 3 | 任务 SUCCESS 后 | `generation_tasks.cost_amount = 实际秒数 × cost_per_second_amount`（仅后台成本），用户余额未再变 |
| 4 | 成本报表 | 管理端按日聚合 `SUM(cost_amount)` 与 AutoDL GPU 账单（路径 A）/ MiniMax 账单（路径 B）一致 |
| 5 | 前端暴露 | 用户端**看不到**按秒费用，只显示固定积分预估 |

### 9.3 稳定性验收

- **限流**：并发 5 个任务时，无账号级被 ban；429 自动 Key/实例切换；轮询期 429 不重建任务。
- **超时**：模拟厂商超时，任务可重试且不重复计费；项目状态不卡死。
- **自部署专项**：实例关机时提交任务 → 明确报错+告警（不静默排队到超时）；实例加载中 502 → 重试或降级到路径 B（若双路由）。
- **降级**：生产环境无 mock；开发环境 mock 正常。
- **日志**：成本流水、provider 事件、错误码均有痕迹，管理端可查。

### 9.4 回归

- 拆分、资产、原有 T2V（火山/可灵）链路不受影响（MoE 按能力路由）。
- 积分余额、充值、扣费整体闭环正常。
- 前端 `tsc -b`、后端 `go build ./... && go test ./internal/...` 全绿。

---

## 十、实施与回滚清单

### 10.1 实施步骤（顺序）

| 步骤 | 内容 |
|---|---|
| 1 | **迁移**：`models` 加 `cost_per_second_amount`（后台成本单价，默认 0）；`generation_tasks` 加 `cost_amount / cost_duration_sec`（成本核算） |
| 2 | **后端 Provider**：实现 `MiniMaxH3Provider.GenerateVideo`（V2 create+query，`content[]`+role+`duration`/`ratio`/`resolution` 映射与归一化），在 provider 注册处按能力注入（MoE 分派） |
| 3 | **多图参考组装**：`LoadShotRefs` 产物按 角色>场景>道具 排序裁剪 ≤9 张；5.2 模板渲染（图序声明与 refs 严格对应）；role 互斥校验 |
| 4 | **用户计费**：`CreateVideoTasks` 按次扣 `cost_points_per_call`（幂等） |
| 5 | **成本核算**：视频任务 SUCCESS 后 Worker 按 `实际秒数 × cost_per_second_amount` 写入 `cost_amount`；`/models` 返回 `cost_points_per_call`（不暴露成本字段） |
| 6 | **管理端**：厂商/模型/Key 录入（双路径可并存）+ 用户计费（固定积分/次）+ 成本核算（元/秒 + 成本报表/对账）+ 任务队列监控列 |
| 7 | **前端**：`GenVideoModal` 模型列表动态化 + 时长(4–15)/分辨率(768P・2K)/比例参数 + 固定积分预估 + 余额预校验；`MODEL_OPTIONS` 静态数组移除或仅作占位 |
| 8 | **联调**：AutoDL 开实例部署 SGLang(ref2va) → 管理端录入 BaseURL+令牌 → 端到端多图参考视频任务验证；有余量再录官方 API 作备份路由 |

### 10.2 验收命令

```bash
# 后端
cd duanju-backend
go build ./... && go test ./internal/engine/model/... ./internal/domain/creation/...
# 前端
cd apps/web-user && pnpm exec tsc -b
```

### 10.3 回滚

- **代码**：Provider 注册处移除 MiniMax 分派即可回退到火山/可灵 + mock（不涉及拆分/资产）。
- **配置**：管理端将 minimax 厂商/模型/Key `enabled=false`；路由自动绕过（`model_providers.enabled` 参与过滤）。
- **数据**：`cost_per_second_amount` 列可空，不影响既有模型；流水留存可审计。

---

## 十一、注意事项

1. **"按秒/按 GPU"只属于成本，不属于用户收费**：用户侧保持"按次扣固定项目积分"（`cost_points_per_call`）；H3 的按秒计费（官方）或 GPU 租金折算（AutoDL）仅作为后台成本核算（`cost_amount`），做成本报表与对账，**前端绝不向用户展示按秒费用**。
2. **字段命名差异（V2，易踩坑）**：H3 V2 用 `duration`（整数 4–15，**不是** `seconds`）、`ratio`（**不是** `aspect_ratio`）、`resolution` 枚举 `768P`/`2K`（没有 P720/P1080）、查询 `GET /v2/query/video_generation`；旧 Hailuo v1 的字段/路径不适用。参考图走 `content[]` + `role`，**reference 与 first/last_frame 互斥**。
3. **路径 A 上限**：自部署只能 768P；2K 依赖官方 Regenerate-2K API（未开源、另计费）。前端 2K 选项需按 provider 能力 gating。
4. **并发保护**：视频是高成本长耗时任务，`concurrent_limit` 宁可保守（自部署 1），并加"每分钟最多 N 个视频任务"的用量护栏，防预算被单用户打爆。
5. **生产禁降级**：已在上游 `video_handler.go` 强制，接入新厂商不得绕过；开发期 mock 保留。
6. **合规**：视频生成输入需过内容安全（提示词/参考图），建议接入审核中间件（对齐 `content_report` 与审核流程），避免生成违规内容。
7. **成本审计**：成本流水建议记录 `duration_sec + unit + amount + provider(autodl/official)`，与厂商账单每日对账；路径 A 另记实例开机时段，防"忘关机空转"。
8. **Prompt 体积**：prompt ≤7000 字符、请求体 ≤64MB、参考图走公网 URL；多图参考 prompt 的图序声明段是质量关键，裁剪 refs 时必须同步裁剪声明。
