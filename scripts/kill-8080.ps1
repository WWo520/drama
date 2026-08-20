# 重启后端 API：停掉 8080 监听进程后重新启动
$p = (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1).OwningProcess
if ($p) {
    Stop-Process -Id $p -Force
    Write-Host "killed old api pid: $p"
    Start-Sleep -Seconds 1
} else {
    Write-Host "no listener on 8080"
}
