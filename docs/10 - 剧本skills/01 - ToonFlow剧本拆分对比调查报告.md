# ToonFlow 剧本拆分全链路对比调查报告

> 调查对象：`D:\develop\github\toonflow\Toonflow-app`（Electron 桌面应用，Express 后端 + Socket.IO + SQLite + Vercel AI SDK 多 Agent 架构）
> 调查目的：定位本项目（duanju）叙事单元（Beat）拆分结果过长、"更像一整集而非细分单元"的根因，并借鉴 ToonFlow 的拆分粒度控制策略
> 调查日期：2026-08-21

---

## 一、结论摘要

| 维度 | ToonFlow | duanju 现状 |
|------|----------|-------------|
| 拆分方式 | **多阶段流水线**：剧本 → 导演规划（分场）→ 分镜表（切片段）→ 分镜面板（逐条写入），每阶段独立 Agent、单一职责 | **单次 LLM 一把出**：一个 System Prompt 一次性产出 4 层 JSON（集→单元→镜头块→镜头） |
| 最细单元时长 | **硬性 ≤15 秒**（分镜表铁律），台词按 **4 字/秒** 估时，长台词 >20 字**强制拆镜** | 无单元时长上限，仅约束"每集 3~6 个单元"，单元时长由模型自由分配 |
| 集级篇幅控制 | 剧本正文 **≤1000 字**、台词 **150 字/分钟**、单集时长 ±10 秒、场景段落 20~60 秒 | 仅约束"每集目标时长 %d 分钟，单元时长加总接近该目标" |
| 上下文供给 | 每阶段通过工具（`get_flowData`/`get_novel_events`）**按需拉取**数据，禁止重复读取 | 全部剧本原文一次性塞进单次请求 |
| 结果校验 | 各阶段有"红线/自查清单"+ 监督层 Agent 审核（评分 A/B/C/D） | ParseAndValidate 结构校验 + 坏 JSON 重试 ≤3 次，无语义粒度校验 |
| 实测粒度 | — | **96 个叙事单元中 16 个 >30s、4 个 >60s、最长 120s；71 集平均仅 1.4 个单元/集** |

**核心结论**：duanju 叙事单元过长的根因不是提示词"风格"问题，而是**缺少单元级硬性时长上限与台词-时长换算机制**，且 4 层结构在单次生成中无中间校验点。ToonFlow 通过"场次 → ≤15s 片段 → 镜头行"的逐级下切 + 多条量化铁律把粒度钉死，可直接借鉴。

---

## 二、ToonFlow 全流程链路分析

### 2.1 总体架构

```
小说原文 (o_novel)                      剧本 (o_script，每集一条)
    │ ①事件提取 cleanNovel                  │ ④资产抽取 extractAssets
    ▼                                      ▼
事件表 (o_novel.event) ──②改编流水线──►  资产库 (o_assets/o_image)
                        scriptAgent          │
                        ③逐集写剧本          ▼
                                    ⑤制作流水线 productionAgent
                                    导演规划 → 衍生资产 → 分镜表 → 分镜面板
                                             │
                                             ▼
                            分镜图 (o_storyboard.filePath)
                                             │ ⑥逐分镜生成视频 generateVideo
                                             ▼
                            视频轨道 (o_videoTrack/o_video)
                                             │ ⑦拼接 stitchVideos（xfade 叠化）
                                             ▼
                                     成片预览（工作台视频列表）
```

两条 Agent 主线，均为"决策层（对用户）→ 执行层子 Agent（干活）→ 监督层（审核）"三层结构：

- **scriptAgent**（`src/agents/scriptAgent/`）：小说 → 剧本。子 Agent：故事骨架 / 改编策略 / 剧本编写 / 监督审核
- **productionAgent**（`src/agents/productionAgent/`）：剧本 → 视频。子 Agent：导演规划 / 衍生资产分析 / 衍生资产生成 / 分镜图生成 / 分镜面板写入 / 分镜表构建 / 监督审核

每个子 Agent 的 System Prompt 均为独立的 Markdown 技能文件（`data/skills/*.md`），运行时 `fs.readFile` 加载——**提示词外置为文件资产，而非硬编码在代码中**（与本项目 `agent_skills` 表 + 技能透传的设计方向一致）。

### 2.2 阶段①②：小说 → 剧本（scriptAgent 改编流水线）

| 阶段 | 输入 | 产出 | 粒度控制要点 |
|------|------|------|--------------|
| 项目初始化 | 用户对话 | 项目配置（集数/单集时长/原著范围/平台规格/风格/付费策略） | **台词量 = 单集时长 × 150 字/分钟**，禁止硬编码 |
| 阶段1 故事骨架 | 事件表 | `planData.storySkeleton`（三幕结构 + 分集决策 + 删减表 + 付费卡点 + 反转登记表） | 压缩比 ≤40%；每集必须有集末钩子；**硬性规则"一行就是一集"**（禁止"内容单元/叙事体"等中间抽象层）；≤20 集逐集展开，>20 集总览表+关键集展开 |
| 阶段2 改编策略 | 事件表 + 骨架 | `planData.adaptationStrategy` | 删减依据、世界观呈现策略 |
| 阶段3 剧本编写 | 骨架 + 策略 + 章节原文 + 上一集剧本 | `o_script` 每集一条记录（`<scriptItem name="作品名 EPNN：标题">`） | **循环调度：每次只写一集**，单轮 ≤5 集防上下文超载；**正文 ≤1000 字**；场景段落 20~60 秒；单句台词 ≤20 字；纯画面段落 ≤15 秒 |

关键设计：**集是循环逐集生成的，每集是独立的一次 LLM 调用**（携带骨架中"仅当前任务集"的信息 + 上一集剧本用于衔接），从机制上保证"一集 = 一个完整但有限的叙事段"。

### 2.3 阶段④：剧本 → 资产（extractAssets）

`src/routes/script/extractAssets.ts`：按 5 集一批调用 LLM，输出 Zod 结构化资产（`{name, desc, type: role|tool|scene, scriptIds[]}`），区分"新资产"与"已有资产引用"（名称必须与库内完全一致），写入 `o_assets` 并建立剧本↔资产关联（`o_scriptAssets`）。资产图片生成走 `batchGenerateImageAssets`（pLimit 并发控制，异步轮询状态）。

与本项目差异：ToonFlow 资产抽取是**独立阶段、独立可重跑**；本项目资产随拆分 JSON 一次性产出（按集 `origin_episode_id`）。

### 2.4 阶段⑤：剧本 → 分镜（productionAgent 制作流水线，六阶段串行）

```
阶段1 导演规划 (scriptPlan)      阶段2 衍生资产分析   阶段3 衍生资产生成(可选)
        │                                │                     │
        ▼                                ▼                     ▼
阶段4 构建分镜表 (storyboardTable) ──► 阶段5 分镜面板写入 (o_storyboard) ──► 阶段6 分镜图生成
```

**阶段1 导演规划**（`production_execution_director_plan.md`）——只拆场不创作：

- 切场原则：**一个场 = 同一时空下一段连续戏**；切点 = 地点变更 / 时间跳变 / 戏剧单元收束；剧本已有场标则原著保真
- 产出纯结构化契约（给下游 Agent 逐字段读取，不给人读）：分场汇总表（台词条数/字数、情绪浓度 0~10、情绪基调 X→Y）+ 逐场注意事项（情感砸点/一致性锚点/空间距离/环境音/易错提示）+ 场间过渡表（先判断必要性，不必要直接硬切）
- 台词**只忠实计数，不预算时长**——时长换算留给分镜表按语速做

**阶段4 分镜表**（`production_execution_storyboard_table.md`）——粒度控制的核心环节（详见第三章）

**阶段5 分镜面板写入**（`production_execution_storyboard_panel.md`）——把分镜表逐条落库：

- 路由模式：决策层按视频模型能力指定模式（纯文本多参 / 首位帧）
- 每条分镜调用一次 `add_flowData_storyboard({videoDesc, prompt, track, duration, associateAssetsIds, shouldGenerateImage})` 写入 `o_storyboard`
- **数量一致性红线**：写入次数必须 == 写入单位数（组数或数据行数）；**时长一致性红线**：`duration` 必须与分镜表标注完全一致

**阶段6 分镜图生成**：`batchGenerateImage` 异步出图，轮询状态。

### 2.5 阶段⑥⑦：视频生成与最终呈现

- **逐分镜生成视频**（`workbench/generateVideo.ts`）：每条 `o_storyboard`（或其组合）→ 一条 `o_video`；提示词中文展示版/英文模型版双语管理（`promptZh`），用户改过中文则自动翻译回英文；参考图来源可为分镜图或资产图（`sources: storyboard|assets`）
- **拼接成片**（`workbench/stitchVideos.ts`）：多段视频按序 xfade 叠化（默认 0.4s）+ 音频 acrossfade，产物挂回首段轨道，异步执行
- **最终呈现**：工作台视频列表 + 轨道时间线（`o_videoTrack`），前端为打包后的 SPA（`data/web`，Electron 壳加载），通过 Socket.IO 实时接收 Agent 思考流/消息流（`consumeFullStream` 消费 `reasoning-delta/text-delta` 等 chunk 逐字推送到 UI）

### 2.6 状态管理方式

ToonFlow 不用项目级状态机，而是**记录级状态字段 + 阶段产物存在性**双重判断：

| 载体 | 状态表达 |
|------|----------|
| `o_image.state` / `o_storyboard.state` / `o_video.state` | 中文枚举：未生成 / 生成中 / 生成成功 / 生成失败（附 errorReason） |
| `o_novel.eventState` | -1/0/1 事件提取状态 |
| `o_script.extractState` | 资产抽取状态 |
| 阶段推进 | flowData（`o_agentWorkData.data` JSON）中 `scriptPlan/storyboardTable/storyboard` 是否有内容 + 决策层提示词中的"前置条件"约束 |
| 崩溃恢复 | `fixDB.ts` 启动时把所有"生成中"记录回滚为"生成失败（软件退出导致失败）" |

与本项目对比：duanju 用项目级 `project_status_enum` 状态机 + `generation_tasks` 任务表，是服务端多用户场景的合理选择，无需照搬；但 ToonFlow 的**"启动自愈：in-flight 任务置失败可重试"**值得参考。

### 2.7 FlowData：阶段间数据流转契约

`getFlowData(projectId, episodesId)`（`src/routes/production/getFlowData.ts`）按集聚合出下游 Agent 的全部工作区：

```ts
FlowData = {
  script: string,            // o_script.content（该集剧本）
  scriptPlan: string,        // 导演规划 XML（存 o_agentWorkData.data）
  assets: [{id,name,type,prompt,desc,src,derive[]}],  // o_assets+o_image join
  storyboardTable: string,   // 分镜表 XML
  storyboard: [{id,index,duration,prompt,videoDesc,state,src,associateAssetsIds[]}], // o_storyboard
  workbench: {videoList: []}
}
```

要点：**所有阶段共享同一份按"集"隔离的工作区**，每个执行层 Agent 第 1 步一次性读取、此后禁止再读（防上下文膨胀），产出写回对应字段。

---

## 三、提示词工程研究：ToonFlow 如何控制拆分粒度

### 3.1 集级粒度（剧本编写 Agent，`script_execution_script.md`）

```text
- 单集时长控制在【项目配置】指定值 ±10秒，台词量按 150字/分钟 推算（禁止硬编码）
- 剧本正文篇幅紧凑：场景段落正文（不含文件头与剧情梗概）字数通常控制在 1000 字以内
- 每个场景段落 20-60 秒；纯画面段落（无台词）最长 15 秒
- 单句台词 ≤20 字（竖屏阅读速度）；单个角色单次台词尽量 ≤50 字
- 节奏 3-15-45：3 秒内一个情绪冲击 / 15 秒一个剧情变化 / 45 秒一个强期待 / 结尾反转钩子卡死
- 每集 500-800 字须覆盖爆点/虐点/爽点中至少一个
```

**机制**：时长→字数是硬换算（`wordsPerEpisode = episodeDuration × 150`），集的长度被"字数预算"物理卡死，而不是靠模型"感觉"。

### 3.2 单元级粒度（分镜表 Agent，`production_execution_storyboard_table.md`）

这是与本项目"叙事单元"最对应的层级。其铁律（原文摘录）：

```text
【铁律优先级】：台词零删改 > 出场人物完整 > 只描述动作状态 > 长台词/长VO拆镜规则

2. 每个片段 ≤15 秒，单片段台词字数超载时，拆分为多个片段。
3. 长台词/长 VO 强制拆镜：同一片段中长台词超过 20 字必须拆多个连续镜头，
   每镜换视角/景别，按语义停顿点切，不平均切分。
5. 台词时间分配：根据情绪和语气分配，不平均切分，台词按照 4字/秒 进行计算。
```

切分方法（执行流程第 3 步"结构化草案"）：

```text
1. 估时长：通读该场台词原文，按 4 字/秒估算各条台词时长
2. 切片段：沿叙事顺序，在情绪转折点/动作段落/说话人切换处下刀，切成若干 ≤15 秒的片段
3. 设计片段过渡：明确写出连接两个片段的桥梁元素（动作/情绪/视线/声音）
4. 片段内切镜：处理长台词/长 VO 的拆镜，确认每个镜头都有景别/视角变化
5. 全员在场校验：对照出场人物，确认本场每人在各片段中都有视觉落点
```

输出格式即粒度载体——每个片段一行表头 + 镜头行表格：

```text
### 片段一（约10s）
**引用资产名称**：[苏晚卿, 凌玄, 青云令, 大殿]
**引用资产ID**：[101, 100, 202, 300]
| 序号 | 画面描述 | 时长 | 景别 | 运镜 | 台词 | 音效 |
| 1 | … | 5 | 近景 | 缓推 |  | … |
| 2 | … | 5 | 近景 | 缓推 | 林刚暴怒说：『…』 | … |
```

**关键洞察**：ToonFlow 的"单元"不是按剧情段落切的，而是**按"≤15s + 台词字数÷4字/秒"物理预算切的**——剧情段落（场）只是外层容器，片段边界由时长预算和语义停顿点决定。这正是"细分的 Beat"与"完整的剧集"的分界线。

### 3.3 粒度控制的四个可迁移机制

1. **时长换算公式前置**：`4字/秒`（镜头级）、`150字/分钟`（集级）写进提示词并要求"先估时再下刀"，把时长控制从"事后校验"变为"生成前预算"
2. **硬性单元上限**：`每个片段 ≤15 秒` 是无条件铁律，且附带超载处理动作（"拆分为多个片段"），模型有明确的超限出口
3. **切点规则显式化**：在"情绪转折点 / 动作段落 / 说话人切换处"下刀——给模型具体的下刀位置判据，而非"拆得细一点"这类模糊指令
4. **逐级下切、层层校验**：场（时空连续）→ 片段（≤15s 预算）→ 镜头行（景别/运镜/台词逐项），每级都有自查红线（数量一致性、时长一致性、台词零删改），任何一级跑偏都会在下一级的对账中暴露

---

## 四、与 duanju split_handler 的差异分析

### 4.1 现状提示词（`internal/engine/agent/split_handler.go`）

```text
1. 先分集再分单元：…每集含 3~6 个叙事单元…
2. 每集目标时长约 %d 分钟：叙事单元 duration_sec 加总应接近该目标…
3. 每个叙事单元 2~3 个镜头块，每个镜头块 2~4 个镜头；
```

结构性问题：

| # | 问题 | 说明 |
|---|------|------|
| 1 | **单元时长无上限** | 只约束"加总接近集时长"，2 分钟集 ÷ 3 单元 → 单元均值 40s；模型为凑结构常给 60~120s，等于把一整段戏塞进一个单元 |
| 2 | **无台词-时长换算** | 没有"4字/秒"这类物理标尺，单元该多长完全靠模型语义判断 |
| 3 | **四层结构单次生成** | 集/单元/块/镜头 + 角色/场景/道具全在一个 8192 token 的 JSON 里，长剧本时模型倾向压缩层级（减少单元数）以省 token |
| 4 | **无下刀判据** | 没有告诉模型"在哪里切"（情绪转折/说话人切换/动作段落），只有"每集 3~6 个"的数量约束 |
| 5 | **校验只看结构不看粒度** | ParseAndValidate 只验 JSON 合法性与枚举值，不验单元时长分布 |

### 4.2 实测数据佐证（2026-08-21 本地库）

```sql
-- narrative_units（deleted_at IS NULL）
   96 个单元 | 平均 21s | min 10s | max 120s | >30s: 16 个 | >60s: 4 个
-- episodes
   71 集 | 平均 1.4 个单元/集
```

**71 集平均仅 1.4 个单元**——大量项目是"一集 = 一个单元"，叙事单元退化为集的复制品；个别单元 120s（2 分钟）已等于整集时长。这正是用户反馈"更像完整剧集而非细分 Beat"的数据形态。注意：提示词要求"每集 3~6 个单元"但实测 1.4，说明**纯数量描述在长文本单次生成中约束力不足**，与 ToonFlow"逐集循环调用"的做法形成鲜明对比。

### 4.3 能力对照表

| 能力 | ToonFlow | duanju |
|------|----------|--------|
| 集级时长预算 | 150字/分钟 ±10s | 有（%d 分钟），但无字数换算 |
| 单元/片段时长上限 | ≤15s 铁律 | 无 |
| 台词估时公式 | 4字/秒 | 无 |
| 下刀位置判据 | 情绪转折/动作段/说话人切换 | 无 |
| 拆分执行方式 | 逐集独立调用，先分场再切片段（两次下切） | 单次全量 JSON |
| 中间产物 | scriptPlan / storyboardTable 可人工审阅 | 无中间产物 |
| 粒度校验 | 红线清单 + 监督层审核 | 仅结构校验 |
| 技能外置 | skills/*.md 文件 | agent_skills 表 + split_skill_prompt 透传（机制已具备） |

---

## 五、优化建议（按优先级）

### P0 · 提示词改造（改动最小、见效最快，仅动 `splitSystemPrompt`）

1. **给叙事单元加硬上限**：`每个叙事单元 duration_sec ≤ 30 秒；若某段剧情按 4字/秒 估算超过 30 秒，必须拆分为多个连续单元`（30s 是 ToonFlow 15s 片段的 2 倍，适配本项目"单元含 2~3 个镜头块"的结构）
2. **引入台词估时公式**：`估算单元时长的依据：台词字数 ÷ 4字/秒 + 纯画面动作时间（无台词段落 ≤15 秒）`
3. **给出下刀判据**：`在以下位置切分叙事单元：情绪转折点、说话人切换、动作段落结束、场景内时间跳变；禁止把一段完整冲突的起因-经过-结果塞进同一单元`
4. **反转单元与镜头的数量关系**：由"每集 3~6 单元"改为"每集单元数 = ceil(集时长×60 ÷ 25) 上下浮动"，让单元数随时长自动放大（2 分钟集 → 约 5 个单元起步）

### P1 · 结构校验兜底（动 `ParseAndValidate` / SaveSplitResult 前校验）

5. 校验单元时长分布：`max(duration_sec) > 40` 或 `单元数 < ceil(集时长/0.5)×0.6` 时判定为粒度不达标，走已有的坏 JSON 重试通道（把粒度校验并入 maxTry=3 的重试判定）

### P2 · 拆分两段化（对齐 ToonFlow"先分场再切片段"，较大改造）

6. 第一阶段 LLM 只输出"集 + 集内场次（场头/出场人物/台词字数）"；第二阶段**逐集**调用 LLM 把场次切分为 ≤30s 单元与镜头。逐集调用同时解决长剧本单次生成的 token 压缩问题
7. 技能库配合：将上述粒度铁律沉淀进 `SPLIT_STANDARD` 技能模板的正文，使 10 个剧本技能共享同一套粒度底座，只在"下刀偏好"（悬念点/情绪点/场景点）上分化

### 不建议照搬的部分

- ToonFlow 的多 Agent 决策/执行/监督三层架构依赖人机交互式确认（每阶段等用户回复），与本项目"一键拆分、异步任务"的产品形态不符
- 其 XML 标签流式写入（`<storyboardTable>`）是为 Socket.IO 流式 UI 设计，本项目 JSON + 任务队列无需变更

---

## 附录 A · 关键源文件索引

**ToonFlow**（`D:\develop\github\toonflow\Toonflow-app`）：

| 文件 | 职责 |
|------|------|
| `src/agents/scriptAgent/index.ts` | 改编流水线决策层 + 子 Agent 调度 |
| `src/agents/scriptAgent/tools.ts` | get_novel_events / get_planData / get_novel_text / get_script_content |
| `src/agents/productionAgent/index.ts` | 制作流水线决策层 + 7 个执行子 Agent |
| `data/skills/script_agent_decision.md` | 改编决策层：项目初始化参数表、三阶段流水线、集数循环规则 |
| `data/skills/script_execution_skeleton.md` | 故事骨架：三幕/分集决策/付费卡点/模式B一行一集硬规则 |
| `data/skills/script_execution_script.md` | 单集剧本：1000 字上限、150字/分钟、节奏 3-15-45 |
| `data/skills/production_execution_director_plan.md` | 导演规划：切场原则、台词计数、情绪浓度 |
| `data/skills/production_execution_storyboard_table.md` | 分镜表：≤15s 片段、4字/秒、长台词拆镜铁律 |
| `data/skills/production_execution_storyboard_panel.md` | 分镜面板：add_flowData_storyboard 逐条写入、数量/时长一致性 |
| `src/routes/production/getFlowData.ts` | FlowData 工作区聚合（按集隔离） |
| `src/routes/script/extractAssets.ts` | 剧本→资产抽取（Zod 结构化、新旧资产区分） |
| `src/routes/production/workbench/generateVideo.ts` | 逐分镜视频生成（中英提示词双语管理） |
| `src/routes/production/workbench/stitchVideos.ts` | 成片拼接（xfade 叠化 + acrossfade） |
| `src/lib/initDB.ts` | SQLite 表结构（o_project/o_novel/o_script/o_assets/o_image/o_storyboard/o_video/o_videoTrack/o_agentWorkData） |
| `src/lib/fixDB.ts` | 启动自愈：in-flight 记录置失败 |

**duanju**：

| 文件 | 职责 |
|------|------|
| `duanju-backend/internal/engine/agent/split_handler.go` | 拆分执行器（本报告改造目标） |
| `duanju-backend/internal/domain/creation/service.go` | SplitScript 入口、技能校验透传 |
| `duanju-backend/migrations/000028_agent_skill_split_seeds.up.sql` | 10 个 SCRIPT_SPLIT 技能种子 |

## 附录 B · ToonFlow 分镜表铁律原文（全文引用，供技能模板改写参考）

```text
1. 分镜设计做到优秀即可，不追求唯一解。镜头间景别视角注意错开。
2. 每个片段 ≤15 秒，单片段台词字数超载时，拆分为多个片段。
3. 长台词/长 VO 强制拆镜：同一片段中超过 20 字必须拆多个连续镜头，
   每镜换视角/景别，按语义停顿点切，不平均切分。
4. 台词零删改铁律：剧本中所有台词、VO、系统播报必须 100% 逐字搬运。
5. 台词时间分配：根据情绪和语气分配，不平均切分，台词按照 4字/秒 计算。
6. 在场人物不能消失：剧本没写离开就还在场，必须有视觉痕迹。
7. 群演处理：微动作服务当前戏核情绪，禁止抢主角戏、禁止配台词。
8. 人物外观交给图片资产：服装、发型、长相不进分镜提示词。
9. 画面描述只描述谁做了什么动作、姿态、表情、当下正在发生的状态变化。
10. 声音只写两类：环境音 + 音效。禁止写 BGM、配乐、音乐。
11. VO 音画同步：文字内容 100% 原样写在分镜末尾的 VO 里。
12. 同场内的分镜设计需要考虑切镜的连贯性和流畅性。
```
