-- 阶段 4 修复（P2 admin）：idx_packages_code 重建为唯一索引（000017 被 IF NOT EXISTS 跳过）
DROP INDEX IF EXISTS idx_packages_code;
CREATE UNIQUE INDEX idx_packages_code ON packages (package_code);