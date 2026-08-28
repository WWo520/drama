# Check Asset Images - 诊断资产图片显示问题
param(
    [string]$EpisodeId = "1",
    [string]$BaseUrl = "http://localhost:5173"
)

Write-Host "======================================" -ForegroundColor Cyan
Write-Host "资产图片显示问题诊断工具" -ForegroundColor Cyan
Write-Host "时间：$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" -ForegroundColor Gray
Write-Host "Episode ID: $EpisodeId" -ForegroundColor Yellow
Write-Host "======================================" -ForegroundColor Cyan
Write-Host ""

# 检查角色列表
Write-Host "[1/4] 获取角色列表..." -ForegroundColor Green
try {
    # 需要通过后端 API，先启动后端服务
    $charUrl = "http://localhost:8089/api/episodes/$($EpisodeId)/characters"
    Write-Host "  URL: $charUrl" -ForegroundColor Gray
    
    # 检查后端是否运行
    $response = Invoke-WebRequest -Uri $charUrl -Method Get -TimeoutSec 5 -ErrorAction SilentlyContinue
    
    if ($response) {
        $data = $response.Content | ConvertFrom-Json
        $items = $data.items
        
        Write-Host ""
        Write-Host "  角色统计:" -ForegroundColor Cyan
        Write-Host "    总角色数：$($items.Count)" -ForegroundColor White
        
        $withImage = 0
        $noImage = 0
        $failed = 0
        
        foreach ($item in $items) {
            if ([string]::IsNullOrWhiteSpace($item.main_image_url)) {
                $noImage++
                Write-Host "  ❌ [$($item.id)] $($item.name)" -ForegroundColor Red
                Write-Host "     状态：$($item.generation_status), 无图" -ForegroundColor DarkGray
            } elseif ($item.main_image_url -like "*volces.com*" -and $item.main_image_url -like "*X-Tos-Expires*") {
                $withImage++
                Write-Host "  ⚠️  [$($item.id)] $($item.name)" -ForegroundColor Yellow
                Write-Host "     状态：$($item.generation_status), TOS URL 可能过期 (已转存)" -ForegroundColor DarkGray
            } else {
                $withImage++
                Write-Host "  ✓ [$($item.id)] $($item.name)" -ForegroundColor Green
                Write-Host "     状态：$($item.generation_status), 图片长度:$($item.main_image_url.Length)" -ForegroundColor DarkGray
            }
            
            if ($item.generation_status -eq "FAILED_DEAD") {
                $failed++
            }
        }
        
        Write-Host ""
        Write-Host "  统计：" -ForegroundColor Cyan
        Write-Host "    ✓ 有图：$withImage" -ForegroundColor Green
        Write-Host "    ❌ 无图：$noImage" -ForegroundColor Red
        Write-Host "    ⚠️  失败：$failed" -ForegroundColor Yellow
        
        if ($noImage -gt 0) {
            Write-Host ""
            Write-Host "  💡 建议操作:" -ForegroundColor Yellow
            Write-Host "    1. 在 Step2 资产页面点击缺失角色的再生成按钮" -ForegroundColor Gray
            Write-Host "    2. 检查生成任务是否在 generation_tasks 表中成功" -ForegroundColor Gray
        }
    } else {
        Write-Host "  ⚠️  后端服务未启动或端口不匹配" -ForegroundColor Yellow
        Write-Host "    请运行：.\scripts\start-backend.ps1" -ForegroundColor Gray
    }
} catch {
    Write-Host "  请求失败：$_" -ForegroundColor Red
}

Write-Host ""
Write-Host "[2/4] 获取场景列表..." -ForegroundColor Green
try {
    $sceneUrl = "http://localhost:8089/api/episodes/$($EpisodeId)/scenes"
    $sceneResponse = Invoke-WebRequest -Uri $sceneUrl -Method Get -TimeoutSec 5 -ErrorAction SilentlyContinue
    
    if ($sceneResponse) {
        $sceneData = $sceneResponse.Content | ConvertFrom-Json
        $sceneItems = $sceneData.items
        
        $withImage = 0
        $noImage = 0
        
        foreach ($scene in $sceneItems) {
            if ([string]::IsNullOrWhiteSpace($scene.main_image_url)) {
                $noImage++
                Write-Host "  ❌ SCENE-$($scene.scene_id): $($scene.base_name) - 无图" -ForegroundColor Red
            } else {
                $withImage++
            }
        }
        
        Write-Host "    总场景：$($sceneItems.Count), 有图：$withImage, 无图：$noImage" -ForegroundColor White
    }
} catch {
    Write-Host "  请求失败：后端可能未启动" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "[3/4] 获取道具列表..." -ForegroundColor Green
try {
    $propUrl = "http://localhost:8089/api/episodes/$($EpisodeId)/props"
    $propResponse = Invoke-WebRequest -Uri $propUrl -Method Get -TimeoutSec 5 -ErrorAction SilentlyContinue
    
    if ($propResponse) {
        $propData = $propResponse.Content | ConvertFrom-Json
        $propItems = $propData.items
        
        $withImage = 0
        $noImage = 0
        
        foreach ($prop in $propItems) {
            if ([string]::IsNullOrWhiteSpace($prop.main_image_url)) {
                $noImage++
                Write-Host "  ❌ PROP-$($prop.prop_id): $($prop.name) - 无图" -ForegroundColor Red
            } else {
                $withImage++
            }
        }
        
        Write-Host "    总道具：$($propItems.Count), 有图：$withImage, 无图：$noImage" -ForegroundColor White
    }
} catch {
    Write-Host "  请求失败：后端可能未启动" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "[4/4] 检查浏览器控制台可能的错误..." -ForegroundColor Green
Write-Host "  请在浏览器中打开 DevTools (F12)，查看 Console 标签:" -ForegroundColor Gray
Write-Host "  可能的错误类型:" -ForegroundColor Yellow
Write-Host "    - 404 Not Found: 图片 URL 不存在" -ForegroundColor DarkGray
Write-Host "    - CORS 错误：跨域问题（开发环境）" -ForegroundColor DarkGray
Write-Host "    - 网络超时：TOS URL 加载慢" -ForegroundColor DarkGray

Write-Host ""
Write-Host "======================================" -ForegroundColor Cyan
Write-Host "💡 常见问题排查步骤:" -ForegroundColor Cyan
Write-Host "======================================" -ForegroundColor Cyan
Write-Host "1. 确认后端服务是否运行" -ForegroundColor White
Write-Host "   → ps | Where-Object {$_.ProcessName -like '*api*' or $_.ProcessName -like '*worker*'}" -ForegroundColor Gray
Write-Host ""
Write-Host "2. 检查后端日志" -ForegroundColor White
Write-Host "   → Get-Content worker-diag.log -Tail 50" -ForegroundColor Gray
Write-Host ""
Write-Host "3. 重新生成缺失资产" -ForegroundColor White
Write-Host "   → 在 UI 的 Step2 页面，点击缺失图片的'+'或'重新生成'按钮" -ForegroundColor Gray
Write-Host ""
Write-Host "4. 检查生成的图片 URL 是否持久化" -ForegroundColor White
Write-Host "   → 运行 backend/scripts/check-asset-status.go" -ForegroundColor Gray
Write-Host ""
Write-Host "======================================" -ForegroundColor Cyan
