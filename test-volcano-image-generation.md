# 火山引擎图片生成测试指南

## 🎯 配置状态确认

### ✅ 已配置项：

**厂商配置：**
- Provider Name: `火山`
- Display Name: `火山`
- Base URL: `https://ark.cn-beijing.volces.com`
- Enabled: ✅ 是

**模型配置：**
- Model Key: `doubao-seedream-4-5-251128`
- Model Name: `豆包 4.5`
- Capability: `T2I` (文生图)
- Cost: 10 积分/次
- Enabled: ✅ 是

**API Key 配置：**
- Key ID: 5
- Key Prefix: `ark-41c83c07...`
- Concurrent Limit: 5
- Priority: 50
- Enabled: ✅ 是

---

## 🚀 测试步骤（前端操作）

### 方法 1：通过管理端查看配置

1. 访问管理端：http://localhost:3001/admin
2. 进入"大模型配置" → "模型列表"标签
3. 确认看到 `火山 - doubao-seedream-4-5-251128` 且状态为启用

### 方法 2：通过用户端实际生成图片

1. 访问用户前端：http://localhost:5173
2. 登录账号：使用你的账号（user_id=7, email 可在数据库中查询）
3. 进入项目 → Step2 创作页
4. 选择任意角色/场景/道具
5. 点击生成按钮，在弹窗中：
   - **提示词**: "一个帅气的年轻男性，黑色短发，穿着现代服装，正面全身照"
   - **模型**: 下拉选择 `doubao-seedream-4-5-251128` (如果显示)
   - **比例**: R9_16 (9:16)
   - **分辨率**: 1K
6. 点击"确定生成"

### 方法 3：创建新的测试角色

如果没有角色可用，可以在前端：
1. 新建项目
2. 拆分剧本（自动创建角色）
3. 或者在角色列表中点击"添加角色变体"

---

## 🔍 后端监控命令

### 查看 Worker 日志

Worker 应该正在运行（PID 应该是当前进程）。打开 PowerShell：

```powershell
# 查看 Worker 进程
Get-Process go | Where-Object { $_.Path -like "*duanju*" }

# 查看 Worker 控制台输出（如果有重定向到文件）
Get-Content d:\develop\a_work\duanju\worker.log -Tail 50
```

### 数据库查询命令

**1. 查看实时任务状态：**
```sql
SELECT 
    id, 
    task_type, 
    status, 
    user_id, 
    frozen_points, 
    thawed_points,
    is_success_refunded,
    created_at 
FROM generation_tasks 
WHERE user_id=7 
ORDER BY created_at DESC 
LIMIT 10;
```

**2. 查看积分账户余额：**
```sql
SELECT current_balance, frozen_balance 
FROM point_accounts 
WHERE owner_type='USER' AND owner_id=7;
```

**3. 查看可用的火山 API Key：**
```sql
SELECT mk.id, mp.name, mk.priority, mk.concurrent_limit
FROM model_keys mk
JOIN model_providers mp ON mk.provider_id = mp.id
WHERE mp.name='火山' AND mk.enabled=true;
```

---

## 📊 预期结果

### 成功情况：

1. **提交任务阶段**：
   - API 响应：`{"code": 0, "data": {"created": 1, "total": 1}}`
   - 数据库插入新记录：status=`QUEUED`, frozen_points=10

2. **Worker 处理阶段**：
   - Worker 控制台打印：
     ```
     asset: CHARACTER_VARIANT 生成图片 target=XX url=... is_mock=false
     ```
   - 任务状态变化：`QUEUED` → `RUNNING` → `SUCCESS`

3. **图片生成完成**：
   - generated_images 表插入新记录
   - assets 表更新 main_image_url
   - 积分扣费：current_balance 减少 10

4. **前端显示**：
   - 资产卡片显示生成的图片 URL
   - 徽章状态变为"已生成"✅

### 失败情况排查：

**错误 1: `provider 未返回 task_id`**
- 原因：火山 API 响应格式不匹配
- 解决：检查 async_provider.go 的 createTask() 解析逻辑

**错误 2: `HTTP 401 Unauthorized`**
- 原因：API Key 无效或过期
- 解决：重新在火山方舟控制台获取 API Key

**错误 3: `model not found` 或 `model disabled`**
- 原因：模型未启用
- 解决：检查 models 表的 enabled 字段

**错误 4: 积分不足**
- 原因：当前余额 < 积分成本
- 解决：充值积分或修改 defaultCost() 返回值

---

## 🔬 深度调试命令

### 查看真实 API 调用（需要 Worker 打印请求体）

修改 worker/main.go 第 72-77 行，添加 debug 日志：

```go
volc := model.NewAsyncTaskProvider("volcengine", ...
```

### 模拟 API 调用测试（独立测试火山接口）

```bash
# 获取你的 API Key（需要从 database 解密，或用明文密钥）
# 假设密钥是 ark-xxxxx（需要实际值）

curl -X POST "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer ark-xxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "一个帅气的年轻男性，黑色短发，穿着现代服装，正面全身照，高清写真",
    "model": "doubao-seedream-4-5-251128",
    "size": "9:16"
  }'
```

---

## ✨ 快速测试命令（PowerShell）

如果你知道 JWT token，可以直接测试：

```powershell
# 替换为你的真实 token 和参数
$token = "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

$body = @{
    episode_id = "71cceeae-d37c-4c6e-87fe-c6bbf7a04ad3"
    items = @(
        @{
            kind = "CHARACTER_VARIANT"
            target_id = 22
            gen = @{
                prompt = "一个帅气的年轻男性，黑色短发，穿着现代服装，正面全身照，高清写真"
                aspect_ratio = "R9_16"
                resolution = "1K"
            }
        }
    )
} | ConvertTo-Json -Depth 10

Invoke-RestMethod `
    -Uri "http://localhost:8080/api/v1/assets/batch-generate" `
    -Headers @{ Authorization = $token } `
    -Method Post `
    -Body $body `
    -ContentType "application/json" | ConvertTo-Json -Depth 10
```

---

## 📝 测试结果反馈模板

请复制以下信息并告诉我：

```markdown
## 测试时间
YYYY-MM-DD HH:MM

## 操作步骤
1. [ ] 登录前端 http://localhost:5173
2. [ ] 进入创作页 Step2
3. [ ] 选择角色 "陆言" (character_id=22)
4. [ ] 点击生成，填写提示词："xxx"
5. [ ] 等待生成完成

## 观察到的现象
- 提交任务时的响应：__________
- Worker 日志输出：__________
- 数据库任务状态变化：__________
- 最终是否显示图片：是/否

## 如果失败，错误提示是什么？
__________
```

---

## 🎉 预期能成功的原因

1. **配置完整**：Provider + Model + API Key 全部就绪
2. **Worker 已重启**：最新的 API 路径 `/api/v3/contents/generations/tasks` 已生效
3. **积分充足**：100,000,500 积分（解冻后）
4. **代码修复**：async_provider.go 已适配火山方舟 Ark 的响应格式

祝你测试成功！🚀
