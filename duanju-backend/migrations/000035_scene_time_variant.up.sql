-- 000035_scene_time_variant：场景日夜/时段变体（对齐 LuxReal 场景多态）
-- generated_images 记录生成时段（历史快照），支撑场景按时段切换多张主图。
ALTER TABLE generated_images ADD COLUMN IF NOT EXISTS time_of_day VARCHAR(16) NULL;
CREATE INDEX IF NOT EXISTS idx_generated_images_tod
  ON generated_images(target_kind, target_id, time_of_day)
  WHERE deleted_at IS NULL AND target_kind='SCENE';
