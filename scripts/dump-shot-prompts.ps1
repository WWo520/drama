# Dump storyboard shots text quality for a beat
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080/api/v1"

$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
$login = Invoke-RestMethod -Uri "$base/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
$H = @{ Authorization = "Bearer $($login.data.access_token)" }

# use the beat from previous smoke
$beatId = $args[0]
if (-not $beatId) { $beatId = "7b307bf0-5091-4e56-8bb8-3117f43e2ff6" }
Write-Host "== beat=$beatId =="

$d = Invoke-RestMethod -Uri "$base/beats/$beatId" -Headers $H -TimeoutSec 10
$no = 0
foreach ($g in $d.data.shot_groups) {
    foreach ($s in $g.shots) {
        $no++
        Write-Host ("---- SHOT {0} ({1}s | {2} | {3} | {4}) ----" -f $no, $s.duration_sec, $s.shot_size, $s.camera_movement, $s.camera_angle)
        Write-Host ("SUBJECT : {0}" -f $s.subject_description)
        Write-Host ("BG      : {0}" -f $s.background_description)
        Write-Host ("LIGHT   : {0}" -f $s.local_light_description)
        Write-Host ("DLG     : {0}" -f $s.dialogue_text)
        Write-Host ("SOUND   : {0}" -f $s.ambient_sound)
        Write-Host ("REFINED : {0}" -f $s.prompt_refined)
    }
}
