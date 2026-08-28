# Submit generation for all PENDING cards then verify
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"
$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }

$projects = Invoke-RestMethod -Uri "$base/projects?page_size=10" -Headers $H -TimeoutSec 10
foreach ($p in $projects.data.items) {
    if ($p.status -notin @('PARTIAL_ASSETS','ALL_ASSETS_READY','VIDEO_GENERATING','PARTIAL_VIDEOS')) { continue }
    try { $eps = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/episodes" -Headers $H -TimeoutSec 10 } catch { continue }
    foreach ($e in $eps.data.items) {
        try { $beats = Invoke-RestMethod -Uri "$base/episodes/$($e.episode_id)/beats" -Headers $H -TimeoutSec 10 } catch { continue }
        foreach ($b in $beats.data.items) {
            try {
                Invoke-RestMethod -Uri "$base/beats/$($b.beat_id)/generate-videos" -Method Post -Body '{"resolution":"P720"}' -ContentType "application/json" -Headers $H -TimeoutSec 60 | Out-Null
                Write-Host ("SUBMIT beat={0} ok" -f $b.sort_order)
            } catch {}
        }
    }
}
Write-Host "== polling =="
for ($i = 0; $i -lt 6; $i++) {
    Start-Sleep 45
    foreach ($p in $projects.data.items) {
        if ($p.status -notin @('PARTIAL_ASSETS','ALL_ASSETS_READY','VIDEO_GENERATING','PARTIAL_VIDEOS')) { continue }
        try { $eps = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/episodes" -Headers $H -TimeoutSec 10 } catch { continue }
        foreach ($e in $eps.data.items) {
            try { $beats = Invoke-RestMethod -Uri "$base/episodes/$($e.episode_id)/beats" -Headers $H -TimeoutSec 10 } catch { continue }
            foreach ($b in $beats.data.items) {
                try { $cards2 = Invoke-RestMethod -Uri "$base/beats/$($b.beat_id)/cards" -Headers $H -TimeoutSec 10 } catch { continue }
                foreach ($cd in $cards2.data.items) {
                    Write-Host ("POLL$i card={0} {1} url={2}" -f $cd.title, $cd.generation_status, [bool]$cd.generated_video_url)
                }
            }
        }
    }
}
