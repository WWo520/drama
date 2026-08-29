# restart-asset-worker.ps1 - 停止 asset worker 并重启（加载新代码）
# 停止所有 asset 相关进程
$procs = Get-CimInstance Win32_Process | Where-Object { $_.Name -match 'worker.exe|go.exe' -and $_.CommandLine -match 'cmd/worker.*asset' }
foreach ($p in $procs) {
    Write-Output "stopping PID=$($p.ProcessId) NAME=$($p.Name)"
    Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
}
Start-Sleep -Seconds 2
Write-Output "asset worker stopped"

# 重新启动 asset worker（注入环境变量）
$backendDir = "d:\develop\a_work\duanju\duanju-backend"
$envFile = "d:\develop\a_work\duanju\.env"
$line = Select-String -Path $envFile -Pattern "^MODEL_KEY_ENC_SECRET=(.+)$" | Select-Object -First 1
if ($line) { $env:MODEL_KEY_ENC_SECRET = $line.Matches[0].Groups[1].Value.Trim() }
$env:DATABASE_URL = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
$env:REDIS_ADDR = "127.0.0.1:6380"
$env:JWT_SECRET = "dev-only-secret-change-in-prod"
$env:APP_ENV = "dev"
$env:S3_ENDPOINT = "127.0.0.1:9000"
$env:S3_ACCESS_KEY = "duanju"
$env:S3_SECRET_KEY = "duanju_dev_pwd"
$env:S3_BUCKET = "duanju"
$env:S3_USE_SSL = "false"
$env:IMGSTORE_ENDPOINT = "tos-s3-cn-beijing.volces.com"
$env:IMGSTORE_REGION = "cn-beijing"
$env:IMGSTORE_ACCESS_KEY = "AKLTNDUyMjRkNThhNTJlNDA5YzlmNjE3YjUyNTNlNzJiMTM"
$env:IMGSTORE_SECRET_KEY = "WkdGaE9HVTRaRFUwTmpsbE5HSmxaV0l6Tm1aa1pUTmtNalJoT1dOak16QQ=="
$env:IMGSTORE_BUCKET = "drama2"
$env:IMGSTORE_PUBLIC_BASE = "https://{bucket}.tos-cn-beijing.volces.com"
$env:ENABLE_IMAGE_PERSIST = "1"

Start-Process -FilePath "go.exe" -ArgumentList "run","./cmd/worker","-role","asset" -WorkingDirectory $backendDir -WindowStyle Hidden -RedirectStandardOutput "$backendDir\bin\worker-asset.out.log" -RedirectStandardError "$backendDir\bin\worker-asset.err.log"
Write-Output "asset worker restarted (code reloaded)"
