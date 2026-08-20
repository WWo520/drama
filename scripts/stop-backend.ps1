# 查找并停止正在运行的 api / worker（go run + 子进程 api.exe/worker.exe）
$procs = Get-CimInstance Win32_Process | Where-Object {
    $_.CommandLine -like '*cmd/api*' -or $_.CommandLine -like '*cmd/worker*'
}
foreach ($p in $procs) {
    Write-Output "stopping PID=$($p.ProcessId) NAME=$($p.Name)"
    Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
}
# go-build 临时目录中的编译产物子进程
foreach ($exe in Get-Process -Name api, worker -ErrorAction SilentlyContinue) {
    if ($exe.Path -like '*go-build*') {
        Write-Output "stopping child PID=$($exe.Id) PATH=$($exe.Path)"
        Stop-Process -Id $exe.Id -Force -ErrorAction SilentlyContinue
    }
}
Write-Output "done"
