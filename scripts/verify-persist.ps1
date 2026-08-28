# P1 verify: regenerate character image -> expect persisted MinIO fixed URL
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"
$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }

$projects = Invoke-RestMethod -Uri "$base/projects?page_size=10" -Headers $H -TimeoutSec 10
$proj = $projects.data.items | Where-Object { $_.status -eq 'VIDEO_GENERATING' } | Select-Object -First 1
if (-not $proj) { Write-Host "NO_TARGET_PROJECT"; exit }
Write-Host ("TARGET {0}" -f $proj.title)

$eps = Invoke-RestMethod -Uri "$base/projects/$($proj.project_id)/episodes" -Headers $H -TimeoutSec 10
$body = @{ episode_id = $eps.data.items[0].episode_id; model_code = ''; items = @(@{ kind = 'CHARACTER_VARIANT'; target_id = [int64]114 }); per_asset_count = 1 } | ConvertTo-Json -Depth 5
try {
    $g = Invoke-RestMethod -Uri "$base/assets/batch-generate" -Method Post -Body $body -ContentType "application/json" -Headers $H -TimeoutSec 30
    Write-Host ("REGEN created={0} cost={1}" -f $g.data.created, $g.data.total_cost)
} catch {
    Write-Host ("REGEN FAILED: " + $_.ErrorDetails.Message)
}
for ($i = 0; $i -lt 8; $i++) {
    Start-Sleep 4
    try {
        $h = Invoke-RestMethod -Uri "$base/asset-dialog/CHARACTER_VARIANT/$($l0variant)/history" -Headers $H -TimeoutSec 10
    } catch {}
}
