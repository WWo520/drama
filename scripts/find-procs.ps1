# 查找 8080(api) 与 duanju worker/api 进程
$ports = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if ($ports) {
  foreach ($pid2 in ($ports.OwningProcess | Select-Object -Unique)) {
    $p = Get-Process -Id $pid2 -ErrorAction SilentlyContinue
    if ($p) { Write-Output ("PORT8080 " + $p.Id + " " + $p.ProcessName + " " + $p.Path) }
  }
} else {
  Write-Output "PORT8080 none"
}
Get-Process -Name "api","worker","duanju*" -ErrorAction SilentlyContinue | ForEach-Object {
  Write-Output ("PROC " + $_.Id + " " + $_.ProcessName + " " + $_.Path)
}
