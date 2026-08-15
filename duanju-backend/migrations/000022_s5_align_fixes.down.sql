-- 阶段 5 对齐修复回滚（000022_s5_align_fixes.down.sql）
-- 仅回滚本迁移新增的内容，不触碰其他迁移拥有的表（shot_groups/canvas_templates/style_library 等）。

DROP INDEX IF EXISTS idx_assets_variant_name;
DROP INDEX IF EXISTS idx_assets_multi_episode_refs;

ALTER TABLE assets DROP COLUMN IF EXISTS variant_name;
ALTER TABLE assets DROP COLUMN IF EXISTS appearance_desc;
ALTER TABLE assets DROP COLUMN IF EXISTS multi_episode_refs;
ALTER TABLE assets DROP COLUMN IF EXISTS last_generated_at;
