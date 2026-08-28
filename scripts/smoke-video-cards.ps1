# Video cards E2E smoke: login -> find beat with shots -> plan -> cards -> generate -> poll
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"

$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }
Write-Host "== login ok =="

$projects = Invoke-RestMethod -Uri "$base/projects?page_size=10" -Headers $H -TimeoutSec 10
$beatId = ""
foreach ($p in $projects.data.items) {
    if ($p.status -in @('PARTIAL_ASSETS','ALL_ASSETS_READY','VIDEO_GENERATING','PARTIAL_VIDEOS','ALL_VIDEOS_READY')) {
        try { $eps = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/episodes" -Headers $H -TimeoutSec 10 } catch { continue }
        foreach ($e in $eps.data.items) {
            try { $beats = Invoke-RestMethod -Uri "$base/episodes/$($e.episode_id)/beats" -Headers $H -TimeoutSec 10 } catch { continue }
            if ($beats.data.items.Count -gt 0) { $beatId = $beats.data.items[0].beat_id; break }
        }
        if ($beatId -ne "") { break }
    }
}
if ($beatId -eq "") { Write-Host "NO_TARGET"; exit 1 }
Write-Host "== target beat=$beatId =="

try {
    $plan = Invoke-RestMethod -Uri "$base/beats/$beatId/plan-video-tasks" -Method Post -Body '{"force":true}' -ContentType "application/json" -Headers $H -TimeoutSec 30
    Write-Host ("PLAN created={0}" -f $plan.data.created)
} catch {
    Write-Host ("PLAN FAILED: " + $_.ErrorDetails.Message)
}

$cards = Invoke-RestMethod -Uri "$base/beats/$beatId/cards" -Headers $H -TimeoutSec 10
$cards.data.items | ForEach-Object {
    Write-Host ("CARD order={0} title={1} dur={2}s shots={3} status={4}" -f $_.sort_order, $_.title, $_.duration_sec, $_.shot_count, $_.generation_status)
}

try {
    $gen = Invoke-RestMethod -Uri "$base/beats/$beatId/generate-videos" -Method Post -Body '{"resolution":"P720"}' -ContentType "application/json" -Headers $H -TimeoutSec 60
    Write-Host ("GENERATE created={0} total={1} frozen={2} cost={3}" -f $gen.data.created, $gen.data.total, $gen.data.frozen_count, $gen.data.total_cost)
} catch {
    Write-Host ("GENERATE FAILED: " + $_.ErrorDetails.Message)
}

for ($i = 0; $i -lt 5; $i++) {
    Start-Sleep 3
    $c2 = Invoke-RestMethod -Uri "$base/beats/$beatId/cards" -Headers $H -TimeoutSec 10
    $summary = ($c2.data.items | ForEach-Object { "{0}:{1}" -f $_.sort_order, $_.generation_status }) -join " "
    Write-Host "POLL$i => $summary"
    $active = @($c2.data.items | Where-Object { $_.generation_status -eq 'QUEUED' -or $_.generation_status -eq 'RUNNING' })
    if ($active.Count -gt 0) { continue } else { break }
}

$final = Invoke-RestMethod -Uri "$base/beats/$beatId/cards" -Headers $H -TimeoutSec 10
$final.data.items | ForEach-Object {
    $shotDone = @($_.shots | Where-Object { $_.video_generation_status -eq 'SUCCESS' }).Count
    Write-Host ("FINAL card={0} status={1} has_url={2} shots_success={3}/{4}" -f $_.title, $_.generation_status, [bool]$_.generated_video_url, $shotDone, @($_.shots).Count)
}
