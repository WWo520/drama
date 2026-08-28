# Diagnose: asset links readiness + stuck video cards + task states
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"
$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }

$projects = Invoke-RestMethod -Uri "$base/projects?page_size=5" -Headers $H -TimeoutSec 10
foreach ($p in $projects.data.items) {
    Write-Host ("PROJECT {0} status={1} updated={2}" -f $p.title, $p.status, $p.updated_at)
    try { $eps = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/episodes" -Headers $H -TimeoutSec 10 } catch { continue }
    foreach ($e in $eps.data.items) {
        try {
            $links = Invoke-RestMethod -Uri "$base/episodes/$($e.episode_id)/asset-links" -Headers $H -TimeoutSec 10
            $withImg = @($links.data.items | Where-Object { $_.main_image_url -ne '' })
            Write-Host ("  EP{0} asset_links={1} with_img={2}" -f $e.sort_order, @($links.data.items).Count, $withImg.Count)
            # show a sample url prefix and expiry hint
            foreach ($l in ($links.data.items | Select-Object -First 3)) {
                $u = $l.main_image_url
                if ($u.Length -gt 90) { $u = $u.Substring(0, 90) }
                Write-Host ("    [{0}] {1} img={2}" -f $l.asset_kind, $l.name, $(if ($u) { $u } else { 'EMPTY' }))
            }
        } catch {}
        try { $beats = Invoke-RestMethod -Uri "$base/episodes/$($e.episode_id)/beats" -Headers $H -TimeoutSec 10 } catch { continue }
        foreach ($b in $beats.data.items) {
            try {
                $cards = Invoke-RestMethod -Uri "$base/beats/$($b.beat_id)/cards" -Headers $H -TimeoutSec 10
                foreach ($cd in $cards.data.items) {
                    if ($cd.generation_status -in @('QUEUED','RUNNING','PENDING')) {
                        Write-Host ("    CARD beat={0} title={1} status={2} dur={3}s" -f $b.sort_order, $cd.title, $cd.generation_status, $cd.duration_sec)
                    }
                }
            } catch {}
        }
    }
}
