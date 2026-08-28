-- 000038_video_cards.up.sql
-- 视频任务规划（对齐 LuxReal Step3「成片卡」两级结构）：
-- 分镜脚本（shots，2~3s/镜头）≠ 视频生成任务；「规划视频任务」把连续镜头编组成
-- 成片卡 video_cards（2~4 镜头、4~10s），视频模型按卡生成一段连续画面。
-- shots.video_card_id 标记镜头归属卡片；卡片 SUCCESS 时其镜头同步置 SUCCESS
-- （保持 CompleteVideoIfAllDone / Step4 展平逻辑兼容）。

CREATE TABLE IF NOT EXISTS video_cards (
  id BIGSERIAL PRIMARY KEY,
  card_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  narrative_unit_id BIGINT NOT NULL REFERENCES narrative_units(id) ON DELETE RESTRICT,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  title VARCHAR(128) NULL,
  sort_order INT NOT NULL DEFAULT 1,
  duration_sec INT NOT NULL DEFAULT 0,   -- 卡内镜头时长加总（展示与计价口径）
  shot_count INT NOT NULL DEFAULT 0,
  prompt_text TEXT NULL,                 -- 用户自定义提示词（空=按分镜脚本三段式自动组装）
  ai_model VARCHAR(64) NULL,             -- 最近一次生成的模型记录
  resolution VARCHAR(16) NULL,
  generation_status generation_status_enum NOT NULL DEFAULT 'PENDING',
  generated_video_url VARCHAR(1024) NULL,
  point_cost INT NOT NULL DEFAULT 0,
  generation_task_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
-- card_uuid 全局唯一引用键（含软删行，避免 UUID 复用歧义）
CREATE UNIQUE INDEX IF NOT EXISTS idx_video_cards_uuid ON video_cards(card_uuid);
CREATE INDEX IF NOT EXISTS idx_video_cards_nu_sort ON video_cards(narrative_unit_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_video_cards_project ON video_cards(project_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_video_cards_video_status ON video_cards(generation_status) WHERE deleted_at IS NULL;

ALTER TABLE shots ADD COLUMN IF NOT EXISTS video_card_id BIGINT REFERENCES video_cards(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_shots_video_card ON shots(video_card_id) WHERE deleted_at IS NULL;
