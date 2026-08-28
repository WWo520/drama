# Trigger replan on a beat then poll task completion
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"
$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }
$beatId = "7b307bf0-5091-4e56-8bb8-3117f43e2ff6"

try {
    $r = Invoke-RestMethod -Uri "$base/beats/$beatId/replan" -Method Post -Body '{}' -ContentType "application/json" -Headers $H -TimeoutSec 20
    Write-Host ("REPLAN OK: " + ($r.data | ConvertTo-Json -Compress))
} catch {
    Write-Host ("REPLAN FAILED: " + $_.ErrorDetails.Message)
}
for ($i = 0; $i -lt 12; $i++) {
    Start-Sleep 6
    try {
        $d = Invoke-RestMethod -Uri "$base/beats/$beatId" -Headers $H -TimeoutSec 10
        $n = 0
        foreach ($g in $d.data.shot_groups) { $n += $g.shots.Count }
        Write-Host ("POLL$i shots=$n first_dur=" + $(if ($n -gt 0) { $d.data.shot_groups[0].shots[0].duration_sec } else { '-' }))
        if ($n -gt 0 -and $n -ne 4) { break }
    } catch { Write-Host "poll err: $($_.Exception.Message)" }
}
