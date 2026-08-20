# 验证：source_script_text 返回 + 登录用户限流 120/min（连续 25 次不 429）
$login = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/auth/login" -Method Post -Body (@{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json) -ContentType "application/json" -TimeoutSec 10
$headers = @{ Authorization = "Bearer $($login.data.access_token)" }

$projects = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects?page_size=10" -Headers $headers -TimeoutSec 10
$first = $projects.data.items | Select-Object -First 1
if ($first) {
    $detail = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects/$($first.project_id)" -Headers $headers -TimeoutSec 10
    $hasField = $detail.data.PSObject.Properties.Name -contains "source_script_text"
    $len = if ($detail.data.source_script_text) { $detail.data.source_script_text.Length } else { 0 }
    Write-Host "DETAIL: has_source_script_text=$hasField len=$len status=$($detail.data.status)"
} else {
    Write-Host "DETAIL: no project found"
}

$hits429 = 0
$limitHeader = ""
for ($i = 1; $i -le 25; $i++) {
    try {
        $r = Invoke-WebRequest -Uri "http://localhost:8080/api/v1/points" -Headers $headers -TimeoutSec 10 -UseBasicParsing
        $limitHeader = $r.Headers["X-RateLimit-Limit"]
    } catch {
        if ($_.Exception.Response.StatusCode -eq 429) { $hits429++ } else { Write-Host "ERR $($_.Exception.Message)" }
    }
}
Write-Host "RATE: 25 reqs, 429_count=$hits429, X-RateLimit-Limit=$limitHeader"
