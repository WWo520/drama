# start-asset-workers.ps1 - 以正确环境变量启动 split + asset worker
$scriptDir = Split-Path -Parent $PSCommandPath
$root = Split-Path -Parent $scriptDir  # d:\develop\a_work\duanju
$backendDir = Join-Path $root "duanju-backend"

# 注入环境变量（与 start-worker.ps1 一致）
$rootEnv = Join-Path $root ".env"
if (Test-Path $rootEnv) {
  $line = Select-String -Path $rootEnv -Pattern "^MODEL_KEY_ENC_SECRET=(.+)$" | Select-Object -First 1
  if ($line) { $env:MODEL_KEY_ENC_SECRET = $line.Matches[0].Groups[1].Value.Trim() }
}
$env:DATABASE_URL = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
$env:REDIS_ADDR = "127.0.0.1:6380"
$env:JWT_SECRET = "dev-only-secret-change-in-prod"
$env:APP_ENV = "dev"
$env:APP_PORT = "8080"
$env:S3_ENDPOINT = "127.0.0.1:9000"
$env:S3_ACCESS_KEY = "duanju"
$env:S3_SECRET_KEY = "duanju_dev_pwd"
$env:S3_BUCKET = "duanju"
$env:S3_USE_SSL = "false"
# 资产图持久化（转存火山 TOS drama2 桶，URL 永不过期）
$env:IMGSTORE_ENDPOINT = "tos-s3-cn-beijing.volces.com"
$env:IMGSTORE_REGION = "cn-beijing"
$env:IMGSTORE_ACCESS_KEY = "AKLTNDUyMjRkNThhNTJlNDA5YzlmNjE3YjUyNTNlNzJiMTM"
$env:IMGSTORE_SECRET_KEY = "WkdGaE9HVTRaRFUwTmpsbE5HSmxaV0l6Tm1aa1pUTmtNalJoT1dOak16QQ=="
$env:IMGSTORE_BUCKET = "drama2"
$env:IMGSTORE_PUBLIC_BASE = "https://{bucket}.tos-cn-beijing.volces.com"
$env:ENABLE_IMAGE_PERSIST = "1"

Write-Host "MODEL_KEY_ENC_SECRET 长度: $($env:MODEL_KEY_ENC_SECRET.Length)"
Write-Host "工作目录: $backendDir"

# 启动 split worker（含 Dispatcher + Reaper）
Write-Host "启动 split worker..."
Start-Process -FilePath "go.exe" -ArgumentList "run","./cmd/worker","-role","split" -WorkingDirectory $backendDir -WindowStyle Hidden -RedirectStandardOutput (Join-Path $backendDir "bin\worker-split.out.log") -RedirectStandardError (Join-Path $backendDir "bin\worker-split.err.log")

Start-Sleep -Seconds 3
Write-Host "启动 asset worker..."
Start-Process -FilePath "go.exe" -ArgumentList "run","./cmd/worker","-role","asset" -WorkingDirectory $backendDir -WindowStyle Hidden -RedirectStandardOutput (Join-Path $backendDir "bin\worker-asset.out.log") -RedirectStandardError (Join-Path $backendDir "bin\worker-asset.err.log")

Write-Host "两个 worker 已启动"
