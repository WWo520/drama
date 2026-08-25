# 剧本解析失败诊断报告

## 🔍 问题分析

### 截图中的情况
- ✅ 项目标题：《逃离云端的人》
- ✅ 已有完整拆分结果：3 集、7 个叙事单元
- ✅ 剧本原文完整存在（1280 字）
- ❌ **顶部红色警告：剧本解析失败，请修改剧本后重试**
- ❌ **第一集状态：拆分失败（红色标签）**

### 根因定位

通过代码分析，发现问题出在前端 `Step1Page.tsx` 第 34 行的状态徽章函数：

```typescript
// Step1Page.tsx:34
function beatStatusBadge(status: string): { label: string; cls: string; running?: boolean } {
  const s = (status || "").toUpperCase();
  if (s.includes("FAIL")) return { label: "解析失败", cls: "unit-status-running", running: true };
  if (s.includes("ING") || s.includes("GENERAT") || s.includes("RUNNING")) 
    return { label: "解析中", cls: "unit-status-running", running: true };
  return { label: "解析完成", cls: "unit-status-done" };
}
```

### 数据库状态枚举

根据 `migrations/000002_enum_types.up.sql:28` 的定义：

```sql
CREATE TYPE generation_status_enum AS ENUM (
  'PENDING',     -- 等待队列
  'QUEUED',      -- 已排队
  'RUNNING',     -- 执行中
  'SUCCESS',     -- 成功
  'FAILED_RETRYING',   -- ⚠️ 失败待重试（包含 FAIL）
  'FAILED_DEAD',       -- ⚠️ 失败死信（包含 FAIL）
  'CANCELED_USER',     -- 用户取消
  'CANCELED_TIMEOUT',  -- 超时取消
  'SKIPPED',           -- 跳过
  'PAUSED'             -- 暂停
);
```

## 🔴 问题真相

当某个镜头（shot）或叙事单元（beat）的视频生成任务进入 `FAILED_RETRYING` 或 `FAILED_DEAD` 状态时：

1. 前端检测到 `video_generation_status` 字段包含 "FAIL" 字符串
2. `beatStatusBadge()` 返回 `{ label: "解析失败", ... }`
3. UI 显示红色 "解析失败" 徽章
4. **同时触发了第 194 行的全局错误提示**：
   ```typescript
   if (p.status === "SCRIPT_SPLIT_FAILED") {
     toast.error("剧本拆解失败，请修改剧本后重试");
   }
   ```

⚠️ **这是个误报！** 项目本身已经拆分成功（`status = PARTIAL_ASSETS`），只是个别镜头的视频生成了失败了。

## 💡 解决方案

### 方案 A：优化前端判断逻辑（推荐）⭐

修改 `Step1Page.tsx` 第 194 行的判断条件：

```typescript
// 原有代码（line 194-197）
if (p.status === "SCRIPT_SPLIT_FAILED") {
  toast.error("剧本拆解失败，请修改剧本后重试");
}

// 修改为更智能的判断
if (p.status === "SCRIPT_SPLIT_FAILED") {
  toast.error("剧本拆解失败，请修改剧本后重试");
} else if (p.status === "PARTIAL_ASSETS" || p.status === "ALL_ASSETS_READY") {
  // 项目已完成，但有部分失败
  const failedCount = beats.filter(b => 
    b.video_generation_status?.toUpperCase().includes('FAIL')
  ).length;
  
  if (failedCount > 0) {
    toast.warning(`部分镜头解析失败 (${failedCount}/${beats.length})，可在列表中标记重试`);
  }
}
```

**好处**：
- ✅ 避免对成功的拆分项目误报错误
- ✅ 提供更有针对性的警告信息
- ✅ 用户体验更好

### 方案 B：手动重试失败的镜头

如果某些镜头确实失败了，可以在列表中点击重试按钮重新生成。

### 方案 C：清理失败数据

如果需要重新拆分整个项目：

1. 点击 "重新拆分" 按钮
2. 勾选覆盖确认
3. 系统会清空旧的单元数据和失败的镜头数据

## 📊 诊断命令

如果你想查看具体哪个镜头失败了，可以使用以下 SQL：

```sql
-- 查看该项目的失败镜头统计
SELECT 
  video_generation_status,
  COUNT(*) as count
FROM shots
WHERE project_id = (
  SELECT id FROM projects 
  WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
)
GROUP BY video_generation_status;

-- 查看详细的失败镜头信息
SELECT 
  s.id,
  bg.title as shot_group_title,
  b.title as beat_title,
  s.duration_sec,
  s.video_generation_status,
  s.generated_video_url,
  e.title as episode_title
FROM shots s
JOIN shot_groups bg ON s.shot_group_id = bg.id
JOIN narrative_units b ON bg.narrative_unit_id = b.id
JOIN episodes e ON b.episode_id = e.id
WHERE s.project_id = (
  SELECT id FROM projects 
  WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
)
ORDER BY s.created_at DESC;
```

## 🎯 建议优先级

1. **立即**：实施方案 A 的前端优化，避免误报误导用户
2. **短期**：添加失败镜头的批量重试功能
3. **中期**：实现自动重试机制（针对 FAILED_RETRYING）
4. **长期**：完善分级的错误提示体系（error/warning/info）
