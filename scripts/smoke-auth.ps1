# 登录链路冒烟测试：注册 → 登录
$ErrorActionPreference = "Continue"

function Show($tag, $resp) {
    Write-Host "==== $tag ===="
    $resp | ConvertTo-Json -Depth 5
}

$regBody = @{ email = "test1@example.com"; password = "Test123456"; nickname = "smoke" } | ConvertTo-Json
try {
    $reg = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/auth/register" -Method Post -Body $regBody -ContentType "application/json" -TimeoutSec 10
    Show "register" $reg
} catch {
    Write-Host "register failed: $($_.ErrorDetails.Message)"
}

$loginBody = @{ email = "test1@example.com"; password = "Test123456" } | ConvertTo-Json
try {
    $login = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/auth/login" -Method Post -Body $loginBody -ContentType "application/json" -TimeoutSec 10
    Show "login" $login
    # 带 token 验证会员详情（曾 500）与积分
    $headers = @{ Authorization = "Bearer $($login.data.access_token)" }
    try {
        $mem = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/memberships" -Method Get -Headers $headers -TimeoutSec 10
        Show "memberships" $mem
    } catch {
        Write-Host "memberships failed: $($_.ErrorDetails.Message)"
    }
    try {
        $pts = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/points" -Method Get -Headers $headers -TimeoutSec 10
        Show "points" $pts
    } catch {
        Write-Host "points failed: $($_.ErrorDetails.Message)"
    }
} catch {
    Write-Host "login failed: $($_.ErrorDetails.Message)"
}
