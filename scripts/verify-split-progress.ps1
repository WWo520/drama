# Trigger resplit for project 98 then poll progress messages
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"
$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }

$projects = Invoke-RestMethod -Uri "$base/projects?page_size=10" -Headers $H -TimeoutSec 10
$p = $projects.data.items | Where-Object { $_.status -eq 'PARTIAL_ASSETS' } | Select-Object -First 1
if (-not $p) { Write-Host "NO_RESPLIT_TARGET"; exit }
Write-Host ("TARGET project status=" + $p.status)

try {
    $r = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/split-script" -Method Post -Body '{"force":true,"episode_duration_min":2}' -ContentType "application/json" -Headers $H -TimeoutSec 20
    Write-Host ("RESPLIT OK status=" + $r.data.status)
} catch {
    Write-Host ("RESPLIT FAILED: " + $_.ErrorDetails.Message)
}
for ($i = 0; $i -lt 4; $i++) {
    Start-Sleep 12
    # read task progress via generic tasks query (TaskDetail by generation_tasks id unknown; query admin-free endpoint /tasks)
    try {
        $d = Invoke-RestMethod -Uri "$base/tasks?page=1&page_size=3&task_type=SCRIPT_SPLIT" -Headers $H -TimeoutSec 10
        $t0 = $d.data.items[0]
        if ($t0) {
            Write-Host ("PROGRESS pct={0} msg={1} status={2}" -f $t0.progress_pct, $t0.progress_message, $t0.status)
        } else { Write-Host "no items" }
    } catch {
        Write-Host ("poll err: " + $_.Exception.Message)
    }
}
