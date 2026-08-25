-- 000032_asset_dialog_align：资产详情弹窗对齐 LuxReal
-- 1) 三类资产表加 edited_prompt（弹窗编辑值；NULL=未编辑，用拆分默认提示词）
ALTER TABLE characters ADD COLUMN IF NOT EXISTS edited_prompt TEXT NULL;
ALTER TABLE scenes     ADD COLUMN IF NOT EXISTS edited_prompt TEXT NULL;
ALTER TABLE props      ADD COLUMN IF NOT EXISTS edited_prompt TEXT NULL;

-- 2) generated_images 扩展：收藏/目标关联/生成参数（生成历史面板）
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS is_favorite BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS target_kind VARCHAR(24) NULL;
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS target_id BIGINT NULL;
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS model_code VARCHAR(64) NULL;
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS style_name VARCHAR(64) NULL;
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS aspect_ratio VARCHAR(8) NULL;
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS use_default_composition BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_generated_images_target ON generated_images (target_kind, target_id)
  WHERE deleted_at IS NULL AND target_kind IS NOT NULL;
