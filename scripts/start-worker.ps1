# Local dev worker launcher: inject env vars then start worker (split/asset/video/export)
# Usage: .\start-worker.ps1 -Role split | asset | video | export | all
# Note: split role also starts Dispatcher + Reaper (task scheduling core); all = 4 roles.
param([string]$Role = "split")

# 清除残留环境变量（确保回退到 JWT_SECRET）
Remove-Item Env:MODEL_KEY_ENC_SECRET -ErrorAction SilentlyContinue

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

switch ($Role) {
  "asset" { go run ./cmd/worker -role asset }
  "video" { go run ./cmd/worker -role video }
  "export" { go run ./cmd/worker -role export }
  "all" {
    Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "$PSScriptRoot\start-worker.ps1", "-Role", "split"
    Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "$PSScriptRoot\start-worker.ps1", "-Role", "asset"
    Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "$PSScriptRoot\start-worker.ps1", "-Role", "video"
    Start-Process powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "$PSScriptRoot\start-worker.ps1", "-Role", "export"
    Write-Output "4 worker processes started (split/asset/video/export)"
  }
  default { go run ./cmd/worker -role split }
}
