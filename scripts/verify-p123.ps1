# P1-P3 verify: bind/unbind/duration endpoints + trigger one character image regen (persist check)
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"
$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }
$beatId = "7b307bf0-5091-4e56-8bb8-3117f43e2ff6"

try {
    Invoke-RestMethod -Uri "$base/beats/$beatId/plan-video-tasks" -Method Post -Body '{"force":true}' -ContentType "application/json" -Headers $H -TimeoutSec 20 | Out-Null
    Write-Host "PLAN OK"
} catch { Write-Host ("PLAN FAILED: " + $_.ErrorDetails.Message) }

$cards = Invoke-RestMethod -Uri "$base/beats/$beatId/cards" -Headers $H -TimeoutSec 10
if (@($cards.data.items).Count -eq 0) { Write-Host "NO_CARDS"; exit }
$card0 = $cards.data.items[0]
Write-Host ("CARD {0} dur={1}s status={2}" -f $card0.title, $card0.duration_sec, $card0.generation_status)

# 2) duration edit
try {
    $r = Invoke-RestMethod -Uri "$base/video-cards/$($card0.card_uuid)/duration" -Method Patch -Body '{"duration_sec":12}' -ContentType "application/json" -Headers $H -TimeoutSec 10
    Write-Host ("DURATION OK -> {0}" -f $r.data.duration_sec)
} catch { Write-Host ("DURATION FAILED: " + $_.ErrorDetails.Message) }

# 3) asset links (for bind test)
$eps = Invoke-RestMethod -Uri "$base/projects/3b2ea6a3-3467-48c5-a02a-c40ae91b2bb2/episodes" -Headers $H -TimeoutSec 10
# resolve project id via beats: use project of beat (resolve via cards owner) — fallback use first project list
$projects = Invoke-RestMethod -Uri "$base/projects?page_size=10" -Headers $H -TimeoutSec 10
$linkSource = $null
foreach ($p in $projects.data.items) {
    try { $e2 = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/episodes" -Headers $H -TimeoutSec 10 } catch { continue }
    foreach ($e in $e2.data.items) {
        try { $lk = Invoke-RestMethod -Uri "$base/episodes/$($e.episode_id)/asset-links" -Headers $H -TimeoutSec 10 } catch { continue }
        if (@($lk.data.items).Count -gt 0) { $linkSource = $lk.data.items; break }
    }
    if ($linkSource) { break }
}
if (-not $linkSource) { Write-Host "NO_LINKS"; exit }
$l0 = @($linkSource | Where-Object { $_.main_image_url -ne '' })[0]
if (-not $l0) { $l0 = $linkSource[0] }
Write-Host ("LINK kind={0} id={1} name={2}" -f $l0.asset_kind, $l0.asset_id, $l0.name)

try {
    $rb = Invoke-RestMethod -Uri "$base/video-cards/$($card0.card_uuid)/assets/bind" -Method Post -Body ('{"asset_kind":"' + $l0.asset_kind + '","asset_id":' + $l0.asset_id + '}') -ContentType "application/json" -Headers $H -TimeoutSec 10
    Write-Host ("BIND OK bound_shots={0}" -f $rb.data.bound_shots)
} catch { Write-Host ("BIND FAILED: " + $_.ErrorDetails.Message) }

$c2 = Invoke-RestMethod -Uri "$base/beats/$beatId/cards" -Headers $H -TimeoutSec 10
$cardChips = @(($c2.data.items | Where-Object card_uuid -eq $card0.card_uuid)[0].shots | ForEach-Object { @($_.linked_assets).Count })
Write-Host ("AFTER BIND chips per shot: " + ($cardChips -join ','))

try {
    Invoke-RestMethod -Uri "$base/video-cards/$($card0.card_uuid)/assets/bind?asset_kind=$($l0.asset_kind)&asset_id=$($l0.asset_id)" -Method Delete -Headers $H -TimeoutSec 10 | Out-Null
    Write-Host "UNBIND OK"
} catch { Write-Host ("UNBIND FAILED: " + $_.ErrorDetails.Message) }

# 1) persist check: regen one character variant image via batch-generate minimal? Use dialog route if exists: PATCH prompt not needed.
# Simplest真实链路: POST /assets/batch-generate with one CHARACTER item (per_asset_count=1)
$charId = $l0.asset_id; $kind = $l0.asset_kind
if ($kind -ne 'CHARACTER') {
    # find first character from DB-backed list endpoint of same episode if available, else skip persist test
    Write-Host "PERSIST-TEST skipped (first link is not CHARACTER)"
} else {
    $pid2 = ($projects.data.items | Where-Object title)[0]
    foreach ($p in $projects.data.items) {
        try { $e2 = Invoke-RestMethod -Uri "$base/projects/$($p.project_id)/episodes" -Headers $H -TimeoutSec 10 } catch { continue }
        foreach ($e in $e2.data.items) {
            try {
                $body = @{ episode_id = $e.episode_id; model_code = ''; items = @(@{ asset_kind = 'CHARACTER'; target_id = [int64]$charId }) ; per_asset_count = 1 } | ConvertTo-Json -Depth 5
                $g = Invoke-RestMethod -Uri "$base/assets/batch-generate" -Method Post -Body $body -ContentType "application/json" -Headers $H -TimeoutSec 20
                Write-Host ("REGEN submitted task=" + $g.data.created)
            } catch {
                # insufficient points etc -> stop trying
                Write-Host ("REGEN failed: " + $_.ErrorDetails.Message)
            }
            break
        }
        break
    }
}
