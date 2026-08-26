# 本地开发启动脚本：注入环境变量后启动后端（对齐 .env.example）
# 用法：.\start-backend.ps1 -Role api | worker
param([string]$Role = "api")

# 模型密钥加密主密钥：与 Worker 侧保持一致（从仓库根 .env 读取，缺失则回退 JWT_SECRET）
# 用 $PSCommandPath 定位脚本目录（$PSScriptRoot 在部分宿主环境可能为 NULL）
$scriptDir = Split-Path -Parent $PSCommandPath
$rootEnv = Join-Path $scriptDir "..\.env"
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

Set-Location "$scriptDir\..\duanju-backend"

if ($Role -eq "worker") {
    go run ./cmd/worker -role split
} else {
    go run ./cmd/api
}
