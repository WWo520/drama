# 本地开发启动脚本：注入环境变量后启动后端（对齐 .env.example）
# 用法：.\start-backend.ps1 -Role api | worker
param([string]$Role = "api")

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

Set-Location "$PSScriptRoot\..\duanju-backend"

if ($Role -eq "worker") {
    go run ./cmd/worker -role split
} else {
    go run ./cmd/api
}
