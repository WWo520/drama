-- 000021_works_recommend.up.sql
-- 作品管理：推荐位字段（管理端"推荐作品"运营配置；默认不推荐）。
ALTER TABLE works ADD COLUMN IF NOT EXISTS is_recommended BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_works_recommended ON works(is_recommended) WHERE deleted_at IS NULL AND publish_status = 'PUBLISHED';
