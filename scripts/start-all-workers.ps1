# Start-All-Workers.ps1 - 启动所有 Worker 并检测状态
param(
    [switch]$VerboseMode = $false
)

Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "🚀 启动所有 Worker 进程" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# 清理旧进程
Write-Host "🧹 清理旧 Worker 进程..." -ForegroundColor Yellow
Get-Process | Where-Object {$_.Path -like "*worker.exe"} | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

# 配置路径
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$backendDir = Join-Path $scriptDir "..\duanju-backend"

if (-not (Test-Path $backendDir)) {
    Write-Host "❌ 错误：无法找到 backend 目录：$backendDir" -ForegroundColor Red
    exit 1
}

# 定义 Worker 角色和参数
$workers = @(
    @{ Role="split"; Concurrency=2; Description="剧本拆分" },
    @{ Role="asset"; Concurrency=4; Description="资产生成" },
    @{ Role="video"; Concurrency=2; Description="视频生成" }
)

# 启动各个 Worker
$processes = @{}

foreach ($w in $workers) {
    Write-Host "[${w.Role}] 启动${w.Description}Worker (并发数=${w.Concurrency})..." -ForegroundColor Green
    
    $args = "./cmd/worker", "-role", $w.Role, "-c", $w.Concurrency.ToString()
    
    $procInfo = Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Normal", 
        "-Command", "cd '$backendDir'; Write-Host 'Starting $_($w.Role)...' ; go run $args" -PassThru -Wait:$false
    
    $processes[$w.Role] = $procInfo
    Start-Sleep -Milliseconds 500
}

Write-Host ""
Write-Host "✅ 等待 Worker 初始化完成..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# 验证 Worker 是否启动成功
Write-Host ""
Write-Host "🔍 检查 Worker 状态:" -ForegroundColor Cyan
Write-Host "─────────────────────────────────" 

$activeWorkers = 0
foreach ($w in $workers) {
    try {
        $running = Get-Process | Where-Object {$_.Path -like "*worker*" -and $_.Id -in ($processes[$w.Role].Id)} | Select-Object -First 1
        if ($running) {
            $status = "✅ 运行中 (PID: $($running.Id))"
            $color = "Green"
            $activeWorkers++
        } else {
            $status = "⚠️  未响应"
            $color = "Yellow"
        }
    } catch {
        $status = "❌ 未启动"
        $color = "Red"
    }
    
    Write-Host "  [$($w.Role)] $($w.Description): $status" -ForegroundColor $color
}

Write-Host "─────────────────────────────────"
Write-Host "已启动 $activeWorkers / $($workers.Count) 个 Worker" -ForegroundColor $(if($activeWorkers -eq $workers.Count){'Green'}else{'Yellow'})

# 检测当前任务状态
Write-Host ""
Write-Host "📊 检测当前剧本拆分任务:" -ForegroundColor Cyan
Write-Host "─────────────────────────────────"

try {
    # 查询最新任务
    $result = & "$PSScriptRoot\go" run ".\$PSScriptDir\check-split-params.go" 2>$null | Out-String
    
    if ($result) {
        # 提取最关键信息
        $lines = $result -split "`n"
        foreach ($line in $lines) {
            if ($line -match "#(\d+) \[split-.*?\]") {
                $taskId = $matches[1]
                Write-Host "  📋 任务 #$taskId" -ForegroundColor White
                
            } elseif ($line -match "状态：(.*)") {
                $status = $matches[1]
                $emoji = switch($status) {
                    "SUCCESS" { "✅" }
                    "RUNNING" { "🔄" }
                    "QUEUED" { "⏳" }
                    "FAILED_RETRYING" { "⚠️" }
                    "FAILED_DEAD" { "❌" }
                    default { "?" }
                }
                Write-Host "     ⚡ 状态：$emoji $status" -ForegroundColor $(
                    if($status -eq "SUCCESS"){"Green"}
                    elseif($status -eq "RUNNING"){"Cyan"}
                    elseif($status -eq "QUEUED"){"Yellow"}
                    else{"Red"}
                )
                
            } elseif ($line -match "创建时间 (.*)") {
                $time = $matches[1]
                Write-Host "     📅 创建：$time" -ForegroundColor Gray
            }
        }
    }
} catch {
    Write-Host "  ❌ 无法查询任务状态" -ForegroundColor Red
}

Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

if ($activeWorkers -eq $workers.Count) {
    Write-Host "✅ 所有 Worker 已成功启动！" -ForegroundColor Green
    Write-Host ""
    Write-Host "💡 下一步操作：" -ForegroundColor Yellow
    Write-Host "  1. 刷新浏览器页面" -ForegroundColor White
    Write-Host "  2. 进入《逃离云端的人》项目" -ForegroundColor White
    Write-Host "  3. 重新输入或选择剧本原文" -ForegroundColor White
    Write-Host "  4. 点击“拆分剧本”按钮" -ForegroundColor White
    Write-Host ""
    Write-Host "✨ 现在可以使用并行优化的分镜生成功能了！" -ForegroundColor Cyan
} else {
    Write-Host "⚠️  部分 Worker 启动失败，请检查控制台输出" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "💡 建议：" -ForegroundColor Yellow
    Write-Host "  1. 查看启动日志中的错误信息" -ForegroundColor White
    Write-Host "  2. 确保 Redis 服务正在运行" -ForegroundColor White
    Write-Host "  3. 确保数据库连接正常" -ForegroundColor White
    Write-Host "  4. 手动运行：cd duanju-backend; go run ./cmd/worker -role split" -ForegroundColor White
}

Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
