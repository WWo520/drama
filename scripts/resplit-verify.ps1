# 拆分粒度验证：登录 → 新建项目（复用项目71剧本）→ 触发拆分 → 轮询直到完成
$ErrorActionPreference = "Stop"
$base = "http://localhost:8080/api/v1"

$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$token = $login.data.access_token
$headers = @{ Authorization = "Bearer $token" }
Write-Host "login ok"

# 剧本原文：由 psql 提前导出到 scripts/verify-script.txt（直接读字符串，避免 FileInfo 包装）
$script = [IO.File]::ReadAllText("d:\develop\a_work\duanju\scripts\verify-script.txt", [System.Text.Encoding]::UTF8)
Write-Host ("script length = " + $script.Length)

# 创建项目（DRAFT）—— body 用 UTF-8 字节直传，避免 PS5.1 ANSI 编码中文乱码
$createBody = @{ title = "粒度验证-逃离云端"; source_script_text = $script } | ConvertTo-Json
$created = Invoke-RestMethod -Uri "$base/projects" -Method Post -Headers $headers -Body ([System.Text.Encoding]::UTF8.GetBytes($createBody)) -ContentType "application/json; charset=utf-8" -TimeoutSec 15
$pid2 = $created.data.project_id
Write-Host ("created project: " + $pid2)

# 触发拆分（DRAFT 无需 force；每集 2 分钟 → 单元数下限 5）
$body = @{ force = $false; episode_duration_min = 2 } | ConvertTo-Json
try {
    $resp = Invoke-RestMethod -Uri "$base/projects/$pid2/split-script" -Method Post -Headers $headers -Body $body -ContentType "application/json" -TimeoutSec 15
    Write-Host ("split triggered: " + ($resp | ConvertTo-Json -Depth 3 -Compress))
} catch {
    Write-Host ("split trigger failed: " + $_.ErrorDetails.Message)
    exit 1
}

# 轮询状态（最长 5 分钟）
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 10
    $p = Invoke-RestMethod -Uri "$base/projects/$pid2" -Method Get -Headers $headers -TimeoutSec 10
    $st = $p.data.status
    Write-Host ("poll {0}: status={1}" -f $i, $st)
    if ($st -ne "SCRIPT_SPLITTING") { Write-Host "FINAL=$st"; break }
}
