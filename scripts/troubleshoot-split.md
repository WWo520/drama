# AI 剧本拆分故障诊断指南

## 一、快速诊断命令

### 1. 查看最近拆分任务状态
```sql
-- 查询最近失败的拆分任务
SELECT 
    id,
    external_task_id,
    status,
    error_message,
    retry_count,
    max_retry_count,
    created_at,
    updated_at
FROM generation_tasks
WHERE task_type = 'SCRIPT_SPLIT'
ORDER BY created_at DESC
LIMIT 20;
```

### 2. 检查卡在 SCRIPT_SPLITTING 状态的项目
```sql
-- 查找卡住的项目
SELECT 
    p.id as project_id,
    p.title,
    p.status as project_status,
    p.step,
    gt.id as task_id,
    gt.status as task_status,
    gt.error_message,
    gt.retry_count,
    p.created_at,
    p.updated_at
FROM projects p
LEFT JOIN generation_tasks gt ON gt.project_id = p.id AND gt.task_type = 'SCRIPT_SPLIT'
WHERE p.step = 'SCRIPT_SPLIT' 
  AND p.status = 'SCRIPT_SPLITTING'
ORDER BY p.updated_at DESC
LIMIT 20;
```

### 3. 检查模型密钥配置
```sql
-- 检查可用的模型密钥
SELECT 
    mp.name as provider_name,
    mp.model_engine,
    mk.key_id,
    mk.key_prefix,
    mk.enabled,
    mk.created_at
FROM model_keys mk
JOIN model_providers mp ON mp.name = mk.provider_name
WHERE mk.enabled = TRUE 
  AND mp.model_engine IN ('deepseek-v4', 'qwen-plus', 'glm-4')
ORDER BY mp.name, mk.created_at;
```

### 4. 查看拆分任务错误详情
```sql
-- 获取失败任务的详细错误信息
SELECT 
    id,
    external_task_id,
    status,
    error_message,
    request_params_json->>'split_skill_prompt' as skill_prompt,
    response_data_json
FROM generation_tasks
WHERE task_type = 'SCRIPT_SPLIT' 
  AND status IN ('FAILED_RETRYING', 'FAILED_DEAD')
ORDER BY created_at DESC
LIMIT 10;
```

## 二、常见错误模式与解决方案

### 错误 1: 401 Authentication Failed
**症状**: 
- `error_message` 包含 "401" 或 "auth" 关键字
- 任务状态为 FAILED_RETRYING

**原因**: 
- 模型密钥无效或被禁用
- MODEL_KEY_ENC_SECRET 未配置导致密钥解密失败

**解决方案**:
```sql
-- 1. 检查并启用密钥
UPDATE model_keys SET enabled = TRUE WHERE key_prefix LIKE 'sk-%';

-- 2. 或者添加新的有效密钥
INSERT INTO model_keys (key_id, provider_name, model_used, encrypted_key, enabled)
VALUES 
    ('key-' || gen_random_uuid(), 'deepseek', 'deepseek-v4', 'YOUR_ENCRYPTED_KEY', TRUE);
```

### 错误 2: no available key
**症状**: 
- `error_message` 包含 "no available key"
- 所有密钥的 enabled=False

**解决方案**:
```sql
-- 启用至少一个密钥
UPDATE model_keys SET enabled = TRUE WHERE ROWNUM = 1;
```

### 错误 3: JSON 解析失败 / 结构校验失败
**症状**: 
- 任务重试 4 次后进入 FAILED_DEAD
- `error_message` 包含 "非法 JSON" 或 "结构校验失败"

**原因**:
- LLM 输出不符合严格 schema
- 原文摘录覆盖率不足 85%
- 叙事单元超过 30 秒或摘录超 120 字

**解决方案**:
```bash
# 查看具体错误详情
psql -d duanju -c "SELECT id, error_message FROM generation_tasks WHERE task_type='SCRIPT_SPLIT' ORDER BY created_at DESC LIMIT 5;"
```

### 错误 4: Worker 未运行
**症状**:
- 任务一直处于 QUEUED 状态
- Outbox 中有记录但未进入 Redis 队列

**检查命令**:
```bash
# 检查 worker 进程
pgrep -f "role=split"

# 启动 worker（如未运行）
cd duanju-backend
go run cmd/worker/main.go -role=split -c=2
```

## 三、手动修复卡住的任务

### 场景 A: 强制重置项目状态到可重试
```sql
-- 将卡住的项目状态重置为拆分失败（允许重新触发）
UPDATE projects 
SET 
    status = 'SCRIPT_SPLIT_FAILED',
    step = 'SCRIPT_SPLIT',
    updated_at = NOW()
WHERE status = 'SCRIPT_SPLITTING' 
  AND step = 'SCRIPT_SPLIT'
  AND updated_at < NOW() - INTERVAL '30 minutes';
```

### 场景 B: 删除失败的重试任务
```sql
-- 清理重复的失败任务（谨慎操作）
DELETE FROM generation_tasks
WHERE task_type = 'SCRIPT_SPLIT'
  AND status IN ('FAILED_RETRYING', 'FAILED_DEAD')
  AND created_at < NOW() - INTERVAL '1 hour';
```

### 场景 C: 重建拆分任务
```sql
-- 删除旧任务并重新触发
DELETE FROM generation_tasks 
WHERE project_id = <PROJECT_ID> AND task_type = 'SCRIPT_SPLIT';

-- 注意：需要在前端重新点击"拆分剧本"按钮
```

## 四、关键日志路径

### Backend Logs
- **Worker 日志**: 查看控制台输出，重点关注 "split:" 前缀的日志
- **LLM 调用日志**: 搜索 "拆分 LLM 调用失败"
- **重试日志**: 搜索 "split: 第 N 次...重试"

### 前端日志
- 浏览器 Console 中的 WebSocket 消息
- `/api/v1/projects/{id}/split-script` 请求响应

## 五、预防性措施

### 1. 定期监控健康检查 SQL
```sql
-- 每日运行此检查
SELECT 
    status,
    count(*) as task_count,
    array_agg(distinct task_type) as types
FROM generation_tasks
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY status;
```

### 2. 确保模型密钥轮换
- 定期检查 `model_keys.enabled` 状态
- 准备备用密钥

### 3. Worker 进程守护
建议使用 systemd 或 supervisor 守护 worker 进程

## 六、联系支持

如果以上方案都无法解决问题，请提供以下信息:
1. 任务 ID (`generation_tasks.id`)
2. 完整错误消息 (`error_message`)
3. 任务创建时间 (`created_at`)
4. Worker 日志片段
