# 诊断：登录 + 项目列表（含原始输出）
try {
    $login = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/auth/login" -Method Post -Body (@{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json) -ContentType "application/json" -TimeoutSec 10
    Write-Host "login ok, token prefix: $($login.data.access_token.Substring(0,20))..."
} catch {
    Write-Host "LOGIN FAILED: $($_.Exception.Message)"
    Write-Host $_.ErrorDetails.Message
    exit 1
}
$headers = @{ Authorization = "Bearer $($login.data.access_token)" }
try {
    $projects = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/projects?page_size=10" -Headers $headers -TimeoutSec 10
    Write-Host "projects total: $($projects.data.total)"
    foreach ($p in $projects.data.items) {
        Write-Host "$($p.project_id) | $($p.status) | $($p.title)"
    }
} catch {
    Write-Host "PROJECTS FAILED: $($_.Exception.Message)"
    Write-Host $_.ErrorDetails.Message
}
