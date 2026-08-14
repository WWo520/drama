-- 回滚（000018）：恢复普通索引
DROP INDEX IF EXISTS idx_packages_code;
CREATE INDEX idx_packages_code ON packages (package_code);