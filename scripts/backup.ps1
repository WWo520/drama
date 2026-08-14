# 阶段 0 B07：单机环境每日备份脚本（PG 全备 + MinIO 快照，dry-run 支持）
# 用法：
#   scripts/backup.ps1          # 执行备份
#   scripts/backup.ps1 -DryRun  # 演练（不实际备份）
param(
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$backupDir = Join-Path $PSScriptRoot "..\backups"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"

# ---- PG 全备 ----
$pgContainer = "duanju-postgres"
$pgDumpFile = Join-Path $backupDir "pg_duanju_$stamp.dump"
New-Item -ItemType Directory -Force -Path $backupDir | Out-Null

if ($DryRun) {
    Write-Host "[dry-run] docker exec $pgContainer pg_dump -U duanju -Fc duanju -f /tmp/backup.dump"
    Write-Host "[dry-run] docker cp $pgContainer:/tmp/backup.dump $pgDumpFile"
} else {
    docker exec $pgContainer pg_dump -U duanju -Fc duanju -f /tmp/backup.dump
    docker cp "$pgContainer`:/tmp/backup.dump" $pgDumpFile
    Write-Host "[backup] PG 备份完成: $pgDumpFile"
}

# ---- MinIO 快照（mc alias 需提前配置） ----
$mcAlias = "local"
if ($DryRun) {
    Write-Host "[dry-run] mc mirror $mcAlias/duanju $(Join-Path $backupDir 'minio_duanju')"
} else {
    $exists = docker exec duanju-minio sh -c "command -v mc" 2>$null
    if ($LASTEXITCODE -eq 0) {
        # 容器内 mc 镜像使用 alias local 指向自身
        docker exec duanju-minio mc mirror local/duanju "$(Join-Path $backupDir 'minio_duanju_')$stamp" --overwrite
        Write-Host "[backup] MinIO 快照完成: $backupDir"
    } else {
        Write-Host "[warn] MinIO 快照跳过：请先配置 mc alias $mcAlias"
    }
}

# ---- 保留策略：保留最近 14 天 ----
Get-ChildItem (Join-Path $backupDir "pg_*.dump") -ErrorAction SilentlyContinue |
    Where-Object { $_.LastWriteTime -lt (Get-Date).AddDays(-14) } |
    Remove-Item -Force
Write-Host "[backup] 完成。"
