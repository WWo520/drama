DROP INDEX IF EXISTS idx_generated_images_tod;
ALTER TABLE generated_images DROP COLUMN IF EXISTS time_of_day;
