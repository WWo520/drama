# start-workers.ps1 - 启动 split + asset worker
param([string]$Role = "split")

$backendDir = "d:\develop\a_work\duanju\duanju-backend"
$envFile = "d:\develop\a_work\duanju\.env"

# 注入环境变量
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

Write-Output "MODEL_KEY_ENC_SECRET len: $($env:MODEL_KEY_ENC_SECRET.Length)"

$outLog = Join-Path $backendDir "bin\worker-$Role.out.log"
$errLog = Join-Path $backendDir "bin\worker-$Role.err.log"

Start-Process -FilePath "go.exe" -ArgumentList "run","./cmd/worker","-role",$Role -WorkingDirectory $backendDir -WindowStyle Hidden -RedirectStandardOutput $outLog -RedirectStandardError $errLog
Write-Output "$Role worker starting, logs: $outLog / $errLog"
