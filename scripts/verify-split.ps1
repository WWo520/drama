# 验证：触发拆分（force）→ 轮询项目状态 → 输出剧集/单元（mock 应产出 1 集 2 单元）
$login = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/auth/login" -Method Post -Body (@{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json) -ContentType "application/json" -TimeoutSec 10
$headers = @{ Authorization = "Bearer $($login.data.access_token)" }

$projects = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects?page_size=10" -Headers $headers -TimeoutSec 10
$first = $projects.data.items | Select-Object -First 1
$pid2 = $first.project_id
Write-Host "project=$pid2 status=$($first.status)"

try {
    $split = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects/$pid2/split-script" -Method Post -Headers $headers -Body (@{ force = $true; script_text = $first.source_script_text } | ConvertTo-Json) -ContentType "application/json" -TimeoutSec 10
    Write-Host "split triggered: task=$($split.data.task_uuid) status=$($split.data.status)"
} catch {
    Write-Host "SPLIT FAILED: $($_.Exception.Message)"; Write-Host $_.ErrorDetails.Message; exit 1
}

$final = ""
for ($i = 0; $i -lt 24; $i++) {
    Start-Sleep -Seconds 5
    $p = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects/$pid2" -Headers $headers -TimeoutSec 10
    Write-Host "poll $i : $($p.data.status)"
    if ($p.data.status -ne "SCRIPT_SPLITTING") { $final = $p.data.status; break }
}
if (-not $final) { Write-Host "STILL SPLITTING after 120s"; exit 1 }

$eps = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects/$pid2/episodes" -Headers $headers -TimeoutSec 10
Write-Host "episodes: $($eps.data.items.Count)"
foreach ($ep in $eps.data.items) {
    Write-Host "  ep $($ep.episode_id) $($ep.title)"
    $beats = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/episodes/$($ep.episode_id)/beats" -Headers $headers -TimeoutSec 10
    Write-Host "    beats: $($beats.data.items.Count)"
    foreach ($b in $beats.data.items) { Write-Host "      beat $($b.beat_id) $($b.status)" }
}
