-- 阶段 4 修复（P2 admin）：packages.package_code 唯一约束（M0 建表时缺失）
CREATE UNIQUE INDEX IF NOT EXISTS idx_packages_code ON packages (package_code);