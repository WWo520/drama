-- 000038_video_cards.down.sql
DROP INDEX IF EXISTS idx_shots_video_card;
ALTER TABLE shots DROP COLUMN IF EXISTS video_card_id;
DROP TABLE IF EXISTS video_cards;
