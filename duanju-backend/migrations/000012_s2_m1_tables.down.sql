-- 回滚迁移（000012）
DROP TABLE IF EXISTS packages;
DROP TABLE IF EXISTS user_memberships;
DROP TABLE IF EXISTS canvas_auto_save_snapshots;
DROP TABLE IF EXISTS canvas_templates;
DROP TABLE IF EXISTS canvas_edges;
DROP TABLE IF EXISTS canvas_nodes;
DROP TABLE IF EXISTS canvas_projects;
ALTER TABLE orders DROP COLUMN IF EXISTS space_id;