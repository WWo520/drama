# 剧本拆分失败 - 自动修复脚本

## 🔍 诊断流程

### 第一步：检查项目状态
```sql
-- 查询项目当前状态
SELECT 
    id,
    project_uuid,
    title,
    status,
    step,
    length(source_script_text) as script_len,
    updated_at
FROM projects 
WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
ORDER BY updated_at DESC;
```

### 第二步：检查是否有失败的拆分任务
```sql
-- 查看该项目的所有拆分任务
SELECT 
    gt.id,
    gt.external_task_id,
    gt.status,
    gt.task_type,
    gt.error_message,
    gt.retry_count,
    gt.max_retry_count,
    gt.created_at,
    gt.updated_at
FROM generation_tasks gt
JOIN projects p ON p.id = gt.project_id
WHERE p.project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
  AND gt.task_type = 'SCRIPT_SPLIT'
ORDER BY gt.created_at DESC
LIMIT 10;
```

### 第三步：检查模型密钥配置
```sql
-- 查看所有启用的 DeepSeek/Qwen/GLM 密钥
SELECT 
    mp.name as provider_name,
    mp.model_engine,
    mk.key_id,
    mk.key_prefix,
    mk.enabled,
    mk.model_used,
    mk.created_at,
    CASE 
        WHEN mk.key_prefix LIKE '%test%' THEN '⚠️ 测试密钥（可能失效）'
        WHEN mk.created_at < NOW() - INTERVAL '30 days' THEN '🔴 密钥较旧，建议轮换'
        ELSE '✅ 正常'
    END as status_note
FROM model_keys mk
JOIN model_providers mp ON mp.name = mk.provider_name
WHERE mk.enabled = TRUE 
  AND mp.model_engine IN ('deepseek-v4', 'qwen-plus', 'glm-4')
ORDER BY mp.name, mk.created_at DESC;

-- 统计已用/未用的密钥
SELECT 
    mp.name as provider,
    count(*) as total_keys,
    sum(case when mk.enabled then 1 else 0 end) as enabled_count,
    sum(case when mk.key_prefix like '%test%' then 1 else 0 end) as test_key_count
FROM model_keys mk
JOIN model_providers mp ON mp.name = mk.provider_name
GROUP BY mp.name;
```

---

## 🔧 修复操作

### 场景 A：没有有效密钥 → 添加新密钥
```sql
-- ⚠️ 需要替换为你的真实加密密钥
-- 如果不知道如何生成加密密钥，请联系管理员

-- 1. 插入新的 DeepSeek 密钥（示例）
INSERT INTO model_keys (key_id, provider_name, model_used, encrypted_key, enabled)
VALUES 
    ('key-' || gen_random_uuid(), 
     'deepseek', 
     'deepseek-v4',
     'YOUR_BASE64_ENCODED_ENCRYPTED_KEY_HERE',  -- ⚠️ 替换为真实的加密密钥
     TRUE);
```

### 场景 B：密钥存在但状态异常 → 启用或更新
```sql
-- 更新某个密钥的启用状态
UPDATE model_keys SET enabled = TRUE 
WHERE key_prefix LIKE 'sk-%' AND provider_name = 'deepseek';

-- 或者删除所有测试密钥
UPDATE model_keys SET enabled = FALSE 
WHERE key_prefix LIKE '%test%';
```

### 场景 C：项目状态卡住 → 强制重置
```sql
-- 将项目状态重置为 DRAFT（可以重新开始）
UPDATE projects 
SET 
    status = 'DRAFT',
    step = NULL,
    updated_at = NOW()
WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';

-- 清理相关的拆分任务
DELETE FROM generation_tasks 
WHERE project_id = (
    SELECT id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
)
AND task_type = 'SCRIPT_SPLIT';
```

### 场景 D：已有失败镜头 → 标记清空后重试
```sql
-- 清空所有失败的 shot（软删除）
UPDATE shots 
SET deleted_at = NOW()
WHERE project_id = (
    SELECT id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
)
AND video_generation_status LIKE '%FAIL%';

-- 同样清理相关的叙事单元和剧集
UPDATE shot_groups SET deleted_at = NOW()
WHERE project_id = (
    SELECT id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
);

UPDATE narrative_units SET deleted_at = NOW()
WHERE project_id = (
    SELECT id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
);

UPDATE episodes SET deleted_at = NOW()
WHERE project_id = (
    SELECT id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82'
);

-- 重置项目状态
UPDATE projects 
SET status = 'PARTIAL_ASSETS', updated_at = NOW()
WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
```

---

## ✅ 推荐的最佳修复路径

### 方案一：快速重置并重试（推荐）⭐
适用于：不想保留之前的拆分结果，直接重新来

1. 执行"场景 C"中的 SQL，重置项目为 DRAFT
2. 刷新浏览器页面
3. 点击"拆分剧本"按钮（首次拆分）
4. 等待 AI 处理完成

### 方案二：保持数据，仅重试失败部分
适用于：某些剧情单元已经成功拆分，只希望重试失败的镜头

1. 先运行"场景 D"的 SQL，清空失败部分
2. 刷新浏览器
3. 进入创作页，手动点击失败的镜头进行重试

### 方案三：检查并修复 API 密钥
适用于：推测是密钥问题导致的失败

1. 运行"第三步"的 SQL，检查现有密钥
2. 如果有问题，使用"场景 B"修复
3. 如果不满足需求，使用"场景 A"添加新密钥
4. 再次尝试重新拆分

---

## 🛠️ 自动化脚本

如果你想一键执行最佳修复路径，可以使用下面的 PowerShell 命令：

```powershell
# 注意：这需要你本地安装了 PostgreSQL 命令行工具

$projectUuid = "e9010b35-46aa-4a9f-bda7-85782d9e2e82"

# 1. 获取项目 ID
$projectId = psql -d duanju -t -A -c "SELECT id FROM projects WHERE project_uuid='$projectUuid'"

# 2. 重置项目状态
psql -d duanju -c @"
UPDATE projects 
SET status = 'DRAFT', step = NULL, updated_at = NOW()
WHERE project_uuid = '$projectUuid';

DELETE FROM generation_tasks 
WHERE project_id = $projectId AND task_type = 'SCRIPT_SPLIT';
"@

Write-Host "✅ 项目已重置为 DRAFT 状态，可以在前端重新触发拆分"
```

---

## 📋 验证清单

修复完成后，请确认：

- [ ] 数据库中没有 `FAILED_RETRYING` 或 `FAILED_DEAD` 状态的拆分任务
- [ ] `model_keys` 表中至少有一个启用的非测试密钥
- [ ] 项目状态不是 `SCRIPT_SPLITTING`（应该是 `DRAFT` 或 `PARTIAL_ASSETS`）
- [ ] 刷新浏览器页面后，不再显示红色错误提示

---

## 🎯 下一步行动

请按照以下顺序执行：

1. **立即执行**：诊断 SQL 查询（第一步到第三步），告诉我结果
2. **选择方案**：根据你的需求，选择方案一/二/三
3. **执行修复**：运行对应的修复 SQL
4. **测试验证**：刷新浏览器，确认问题解决

如果需要我帮你判断应该用哪个方案，请先运行诊断 SQL 并把结果贴给我看！
