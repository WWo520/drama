-- 000021_works_recommend.down.sql
DROP INDEX IF EXISTS idx_works_recommended;
ALTER TABLE works DROP COLUMN IF EXISTS is_recommended;
