# 剧本拆分失败 - 智能修复工具

## 🎯 问题分析

根据用户截图和代码分析：
1. ✅ 项目《逃离云端的人》已有完整拆分结果（3 集）
2. ❌ 点击"重新拆分"后触发失败
3. ❌ 后端 LLM 调用失败，重试 4 次后状态变为 `SCRIPT_SPLIT_FAILED`
4. ❌ 前端显示"剧本解析失败，请修改剧本后重试"

## 🔍 自动诊断流程

### Step 1: 获取 PostgreSQL 连接信息
```powershell
# 从.env.example 或 .env 中读取数据库配置
Get-Content "d:\develop\a_work\duanju\.env.example" | Select-String "POSTGRES_DSN"
```

### Step 2: 诊断项目状态
```sql
-- 快速获取项目详情
SELECT 
    p.id,
    p.project_uuid,
    p.title,
    p.status,
    length(p.source_script_text) as script_len,
    (SELECT COUNT(*) FROM generation_tasks gt 
     WHERE gt.project_id = p.id AND gt.task_type = 'SCRIPT_SPLIT' AND status LIKE '%FAIL%') as failed_task_count,
    (SELECT count(*) FROM model_keys mk JOIN model_providers mp ON mp.name = mk.provider_name
     WHERE mk.enabled = TRUE AND mp.model_engine IN ('deepseek-v4','qwen-plus','glm-4')) as valid_key_count,
    p.updated_at
FROM projects p
WHERE p.project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
```

### Step 3: 检查最近失败的任务详情
```sql
-- 查看具体的错误信息
SELECT 
    id,
    external_task_id,
    status,
    error_message,
    retry_count,
    request_params_json->>'script_text' as script_preview,
    created_at,
    updated_at
FROM generation_tasks
WHERE task_type = 'SCRIPT_SPLIT' 
  AND status LIKE '%FAIL%'
ORDER BY created_at DESC
LIMIT 3;
```

---

## 🛠️ 一键修复方案

### 方案 A：如果模型密钥不足 → 添加新密钥（优先推荐） ⭐

**适用场景**：诊断结果显示 `valid_key_count < 2`（需要至少 2 个备用密钥）

**操作步骤**：
1. 获取有效的 DeepSeek API Key（联系管理员或查看文档）
2. 使用加密工具生成加密后的密钥（见下文说明）
3. 执行插入 SQL

**自动生成密钥示例**（需要知道加密密钥）：
```sql
-- ⚠️ 这行需要替换为真实的加密逻辑
-- 参考：cmd/api/main.go 中的 ModelKeyEncryptor
INSERT INTO model_keys (key_id, provider_name, model_used, encrypted_key, enabled)
VALUES (
    'key-' || gen_random_uuid(),
    'deepseek',
    'deepseek-v4',
    cryptox.Encrypt('sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'), -- 使用实际的有效密钥
    TRUE
);
```

### 方案 B：如果确定要重置项目 → 快速恢复

**适用场景**：任何情况下的快速修复

```sql
-- 1. 获取项目 ID
DO $$
DECLARE
    v_project_id BIGINT;
BEGIN
    SELECT id INTO v_project_id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
    
    -- 2. 清理所有相关数据（软删除）
    UPDATE shots SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE shot_groups SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE narrative_units SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE episodes SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE characters SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE scenes SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE props SET deleted_at = NOW() WHERE project_id = v_project_id;
    
    -- 3. 删除失败的拆分任务
    DELETE FROM generation_tasks WHERE project_id = v_project_id AND task_type = 'SCRIPT_SPLIT';
    
    -- 4. 重置项目状态
    UPDATE projects 
    SET status = 'DRAFT', step = NULL, updated_at = NOW()
    WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
    
    RAISE NOTICE '✅ 项目已成功重置为 DRAFT 状态！';
END $$;
```

### 方案 C：智能修复（混合策略）⭐⭐⭐ **强烈推荐**

**优势**：先确保密钥有效，再重置项目，最后尝试自动重试

```sql
-- 智能修复脚本（一次性完成所有操作）
DO $$
DECLARE
    v_project_id BIGINT;
    v_valid_keys INTEGER;
BEGIN
    -- Step 1: 统计有效密钥数量
    SELECT count(*) INTO v_valid_keys FROM model_keys mk 
    JOIN model_providers mp ON mp.name = mk.provider_name
    WHERE mk.enabled = TRUE 
      AND NOT mk.key_prefix LIKE '%test%'
      AND mp.model_engine IN ('deepseek-v4','qwen-plus','glm-4');
    
    -- Step 2: 如果没有足够密钥，提示用户
    IF v_valid_keys = 0 THEN
        RAISE WARNING '⚠️ 警告：未检测到有效的模型密钥！请先添加密钥后再重试。';
        -- 可以尝试启用测试密钥作为临时方案
        UPDATE model_keys SET enabled = TRUE WHERE key_prefix LIKE '%test%' LIMIT 1;
        RAISE NOTICE '已启用一个测试密钥作为临时方案。';
    END IF;
    
    -- Step 3: 获取项目 ID
    SELECT id INTO v_project_id FROM projects WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
    
    -- Step 4: 确认有剧本内容
    IF (SELECT length(source_script_text) FROM projects WHERE id = v_project_id) = 0 THEN
        RAISE EXCEPTION '❌ 错误：项目中没有剧本内容，请先补充剧本原文！';
    END IF;
    
    -- Step 5: 清理旧的拆分数据
    UPDATE shots SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE shot_groups SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE narrative_units SET deleted_at = NOW() WHERE project_id = v_project_id;
    UPDATE episodes SET deleted_at = NOW() WHERE project_id = v_project_id;
    
    -- Step 6: 删除失败的拆分任务
    DELETE FROM generation_tasks WHERE project_id = v_project_id AND task_type = 'SCRIPT_SPLIT';
    
    -- Step 7: 重置项目状态为 DRAFT
    UPDATE projects 
    SET status = 'DRAFT', 
        step = NULL, 
        updated_at = NOW(),
        total_episode_count = 0,
        total_duration_sec = 0
    WHERE project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
    
    -- Step 8: 输出结果
    RAISE NOTICE '========================================';
    RAISE NOTICE '✅ 智能修复完成！';
    RAISE NOTICE '  有效密钥数：%', v_valid_keys;
    RAISE NOTICE '  剧本长度：%字符', (SELECT length(source_script_text) FROM projects WHERE id = v_project_id);
    RAISE NOTICE '  项目状态已重置为：DRAFT';
    RAISE NOTICE '  请在浏览器刷新页面并重新点击"拆分剧本"按钮';
    RAISE NOTICE '========================================';
    
END $$;
```

---

## 📝 使用说明

### 方式一：手动执行修复（推荐新手）

1. **打开你的数据库管理工具**（如 pgAdmin, DBeaver 等）
2. **连接到 duanju 数据库**
3. **依次执行**：
   - 诊断 SQL（查看当前状态）
   - 选择一种修复方案的 SQL
   - 执行验证 SQL

### 方式二：PowerShell 脚本一键修复

我已经为你创建了完整的修复脚本文件：
- [scripts/fix-split-failure.ps1](file:///d:/develop/a_work/duanju/scripts/fix-split-failure.ps1)

**使用方法**：
```powershell
cd d:\develop\a_work\duanju
.\scripts\fix-split-failure.ps1
```

### 方式三：PostgreSQL 命令行（高级用户）

```powershell
# 1. 首先诊断
psql -d duanju -f scripts/diagnose.sql

# 2. 然后修复
psql -d duanju -f scripts/repair-smart.sql
```

---

## 🧪 验证修复成功

修复完成后，请确认以下几点：

```sql
-- 快速验证查询
SELECT 
    p.title,
    p.status,
    CASE 
        WHEN p.status = 'DRAFT' THEN '✅ 可以重新拆分'
        WHEN p.status = 'PARTIAL_ASSETS' THEN '✅ 已拆分成功'
        WHEN p.status = 'SCRIPT_SPLIT_FAILED' THEN '❌ 仍然失败，需要进一步排查'
        ELSE '⚠️ 其他状态'
    END as status_check,
    (SELECT count(*) FROM model_keys mk 
     JOIN model_providers mp ON mp.name = mk.provider_name
     WHERE mk.enabled = TRUE AND mp.model_engine LIKE '%llm%') as available_keys
FROM projects p
WHERE p.project_uuid = 'e9010b35-46aa-4a9f-bda7-85782d9e2e82';
```

预期结果应该是：
- ✅ status_check = "可以重新拆分" 或 "已拆分成功"
- ✅ available_keys ≥ 1

---

## 🎯 下一步建议

修复成功后，建议采取以下措施防止问题再次发生：

1. **定期轮换密钥**（每月检查一次）
2. **设置监控告警**（拆分失败率 > 5% 时通知管理员）
3. **前端优化**（显示更明确的错误提示，如"API 调用失败"vs"JSON 格式错误"）
4. **增加重试机制**（前端检测到失败后可以自动重试 1-2 次）

---

## 🆘 遇到问题？

如果上述方法都不能解决问题，请提供以下信息：

1. 执行的诊断 SQL 结果截图
2. 浏览器控制台的错误日志（F12 → Console）
3. Network 面板中那个红色请求的 Response Body
4. Worker 日志中是否有 "split:" 开头的错误信息

我可以根据这些信息提供更精准的解决方案！
