# 09 - Toonflow 调研方案（剧本解析/资产生成/视频生成/视频合成）

> 生成日期：2026-08-22
> 调研对象：`refer/Toonflow-app`（Electron + Node.js + Express + SQLite）
> 目的：对标 Toonflow 四大核心模块的设计逻辑，为 12 阶段优化提供参考

---

## 一、调研结论速览

| 模块 | Toonflow 方案 | 本项目方案 | 差距 |
|------|-------------|-----------|------|
| **剧本解析** | AI 批量提取资产（5 个/组），tool calling 结构化输出 | 单次 LLM 拆分 + 确定性后处理 | ⚠️ 批量分组策略可借鉴 |
| **资产生成** | 类型配置驱动（role/scene/tool），支持参考图 | 已实现，基本一致 | ✅ 已对齐 |
| **视频生成** | 轨道制（o_videoTrack），多参考图类型（首帧/尾帧/参考图/音频） | beat 级生成，shots/batch | ⚠️ 轨道概念可借鉴 |
| **视频合成** | **前端 Electron 客户端合成**（无后端逻辑） | 后端 mock concat → 待改真实 FFmpeg | 🔴 需补真实合成 |

---

## 二、四大模块详细分析

### 2.1 剧本解析（`routes/script/extractAssets.ts`）

**核心逻辑**：
```
输入：scriptIds[]（剧本 ID 数组）+ projectId + groupSize（默认 5）
处理：
  1. 按 groupSize 分组（chunkArray）
  2. 每组调用 AI（tool calling 模式）：
     - system prompt: "提取剧本中涉及的资产（角色、场景、道具）"
     - 提供已有资产列表（增量识别）
     - 要求返回 newAssets（新资产完整信息）+ existingAssetRefs（已有资产引用）
  3. AI 返回结构化 JSON（zod schema 校验）
  4. 入库：o_assets（新资产）+ o_scriptAssets（关联表）
状态机：extractState（-1失败/0进行中/1成功/2等待）
```

**关键设计**：
- **批量分组**：默认 5 个剧本一组，减少 AI 调用次数
- **增量识别**：AI 同时识别新资产和已有资产（通过名称匹配）
- **tool calling**：强制结构化输出（`resultTool`），避免解析自然语言
- **状态追踪**：extractState 标记每个剧本的提取状态

**与本项目对比**：
- 本项目：单次 LLM 拆分整个剧本 → 确定性后处理（NormalizeEpisodeCount/SplitOversizedUnits/mergeTinyEpisodes）
- Toonflow：剧本已存在，AI 只做资产提取（不拆分）
- **可借鉴**：批量分组策略（当前本项目单次调用，若剧本过长可能超 token 限制）

---

### 2.2 资产生成（`routes/assetsGenerate/generateAssets.ts`）

**核心逻辑**：
```
输入：projectId + model + resolution + id（资产 ID）+ type（role/scene/tool）+ name + prompt + base64（参考图可选）
处理：
  1. 查询项目配置（artStyle 画风）
  2. 根据 type 选择配置（assetTypeConfig）：
     - role: 角色标准四视图
     - scene: 标准场景图
     - tool: 标准道具图
  3. 构建提示词（buildPrompt）：画风 + 名称 + 提示词
  4. 创建图片占位记录（o_image，state=生成中）
  5. 调用 AI 图像生成（u.Ai.Image(model).run()）
     - 支持参考图（base64）
     - 支持分辨率/比例配置
  6. 保存到本地路径（imagePath）
  7. 更新状态为已完成/失败
  8. 返回 OSS 缩略图 URL
```

**关键设计**：
- **类型配置驱动**：`assetTypeConfig` 字典统一管理三种类型的生成参数
- **参考图支持**：`referenceList` 传入 base64 图片（用于保持一致性）
- **异步占位**：先创建 o_image 记录（生成中），AI 完成后更新状态
- **OSS 集成**：生成后上传到 OSS，返回缩略图 URL

**与本项目对比**：
- 本项目：`POST /assets/batch-generate`（集级批量）+ `PATCH /characters/:id`（提示词编辑）
- Toonflow：单资产生成 + 参考图支持
- **可借鉴**：参考图机制（本项目 F10 待实现）

---

### 2.3 视频生成（`routes/production/workbench/generateVideo.ts`）

**核心逻辑**：
```
输入：scriptId + projectId + uploadData（参考图数组）+ prompt + model + mode + duration + resolution + audio + trackId
处理：
  1. 查询项目视频比例（videoRatio）
  2. 根据 uploadData 查询参考图路径：
     - sources=storyboard: 从 o_storyboard 查询分镜板图片
     - sources=assets: 从 o_assets + o_image 查询资产图片
  3. 转 base64 格式
  4. 创建视频占位记录（o_video，state=生成中）
  5. 调用 AI 视频生成（u.Ai.Video(model).run()）
     - referenceList: 参考图数组（支持 image/audio/video 类型）
     - mode: 模式数组（JSON 字符串）
     - duration: 时长
     - aspectRatio: 比例（16:9/9:16）
     - resolution: 分辨率
     - audio: 是否生成音频
  6. 保存到 videoPath
  7. 更新状态为生成成功/失败
```

**参考图类型**（UploadItem.type）：
- `imageReference`: 图片参考
- `startImage`: 首帧
- `endImage`: 尾帧
- `videoReference`: 视频参考
- `audioReference`: 音频参考

**关键设计**：
- **轨道制**：`o_videoTrack`（轨道表）管理多个视频片段，每个轨道对应一个镜头
- **多参考图**：支持首帧/尾帧/参考图/音频等多种类型，提升一致性
- **异步生成**：先返回 videoId，AI 后台生成，前端轮询状态
- **灵活配置**：mode 支持 JSON 数组（多模式组合）

**与本项目对比**：
- 本项目：`POST /beats/:id/generate-videos`（beat 级批量）+ `POST /shots/batch`（镜头批量）
- Toonflow：轨道制（每个轨道一个镜头），参考图类型更丰富
- **可借鉴**：
  - 轨道概念（本项目 F11 单镜头重生成可参考）
  - 多参考图类型（首帧/尾帧/音频参考，本项目 F10 待实现）

---

### 2.4 视频合成（**前端 Electron 客户端**）

**核心发现**：
- **后端无合成逻辑**：搜索 `compose|merge|concat|ffmpeg|export.*video` 无结果
- **轨道表管理**：`o_videoTrack` 存储多个视频片段（每个片段对应一个镜头）
- **前端合成**：Electron 客户端读取轨道数据，使用前端视频编辑库（如 FFmpeg.wasm 或原生 API）合成最终视频

**轨道数据结构**（`o_videoTrack`）：
```typescript
interface TrackItem {
  id?: number;
  prompt: string;                    // 视频提示词
  state: "未生成" | "生成中" | "已完成" | "生成失败";
  duration?: number;                 // 时长
  selectVideoId?: number;            // 选中的视频 ID
  medias: TrackMedia[];              // 参考图数组
  videoList: VideoItem[];            // 视频列表（同一轨道可能有多个生成版本）
}
```

**与本项目对比**：
- 本项目：`export_handler.go` mock concat（模拟 FFmpeg）→ 待改真实 FFmpeg
- Toonflow：前端合成（无后端逻辑）
- **决策点**：
  - **方案 A**：后端 FFmpeg 合成（本项目当前方向，U6）
  - **方案 B**：前端合成（Toonflow 方案，需引入 FFmpeg.wasm 或 Electron 原生 API）
  - **建议**：坚持后端合成（方案 A），因为：
    1. 本项目是 Web 应用（非 Electron），前端合成性能受限
    2. 后端合成可支持高分辨率/大文件
    3. 已有 `export_handler.go` 基础，只需去 mock 改真实 FFmpeg

---

## 三、可借鉴的设计模式

### 3.1 批量分组策略（剧本解析）
- **Toonflow**：scriptIds 按 5 个一组分批调用 AI
- **本项目**：单次调用整个剧本
- **建议**：若剧本超长（>10 万字），可考虑分组调用（避免 token 超限）

### 3.2 参考图机制（资产/视频生成）
- **Toonflow**：支持 base64 参考图（资产）+ 多类型参考图（视频：首帧/尾帧/参考图/音频）
- **本项目**：F10 待实现（镜头↔资产图绑定）
- **建议**：参考 Toonflow 的 `referenceList` 设计，支持多类型参考图

### 3.3 轨道制视频管理
- **Toonflow**：`o_videoTrack` 表管理多个视频片段，每个轨道对应一个镜头
- **本项目**：beat 级生成，shots/batch
- **建议**：F11 单镜头重生成可参考轨道概念（每个 shot 一个轨道，支持多版本）

### 3.4 类型配置驱动（资产生成）
- **Toonflow**：`assetTypeConfig` 字典统一管理三种类型的生成参数
- **本项目**：已实现类似逻辑（角色/场景/道具）
- **评价**：已对齐，无需改动

---

## 四、不建议借鉴的设计

### 4.1 前端视频合成
- **原因**：本项目是 Web 应用，前端合成性能受限（大文件/高分辨率）
- **坚持**：后端 FFmpeg 合成（U6 已有基础）

### 4.2 SQLite 本地数据库
- **Toonflow**：better-sqlite3（本地 SQLite）
- **本项目**：PostgreSQL（生产级）
- **评价**：本项目更适合多端协作/云端部署，无需改动

---

## 五、实施建议（优先级排序）

| 优先级 | 事项 | 来源 | 工作量 |
|--------|------|------|--------|
| **P0** | F10 参考图机制（参考 Toonflow 的 referenceList 设计） | Toonflow 视频生成 | 3 天 |
| **P1** | F11 单镜头重生成（参考 Toonflow 的轨道制） | Toonflow 轨道表 | 2 天 |
| **P2** | 剧本解析批量分组（若剧本超长） | Toonflow 剧本解析 | 1 天 |
| **暂缓** | 前端视频合成（不推荐） | Toonflow 前端合成 | N/A |

---

## 六、关键文件索引

| 模块 | Toonflow 文件 | 核心逻辑 |
|------|-------------|---------|
| 剧本解析 | `src/routes/script/extractAssets.ts` | AI 批量提取资产（tool calling） |
| 资产生成 | `src/routes/assetsGenerate/generateAssets.ts` | 类型配置驱动 + 参考图支持 |
| 视频生成 | `src/routes/production/workbench/generateVideo.ts` | 轨道制 + 多参考图类型 |
| 视频合成 | **前端 Electron 客户端**（无后端文件） | 轨道表管理，前端合成 |
| 导演规划 | `data/skills/production_execution_director_plan.md` | 拆分场次 + 情绪分析 + 场间过渡 |
| 分镜表 | `data/skills/production_execution_storyboard_table.md` | 分镜表生成逻辑 |

---

## 七、结论

Toonflow 是一个成熟的 AI 短剧工具，其**资产提取的批量分组策略**、**视频生成的多参考图类型**、**轨道制视频管理**值得借鉴。但其**前端视频合成**方案不适合本项目（Web 应用性能受限），应坚持后端 FFmpeg 合成方向。

**下一步**：
1. 参考 Toonflow 的 `referenceList` 设计，实现 F10 参考图机制
2. 参考 Toonflow 的轨道制，优化 F11 单镜头重生成
3. 继续推进 U6 真实 FFmpeg 后端合成
