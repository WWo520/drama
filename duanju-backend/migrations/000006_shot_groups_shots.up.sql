-- 000006_shot_groups_shots.up.sql
-- 镜头块与单镜头层：shot_groups / shots（D.1 顺序 7~8）

-- ============ shot_groups（HeroShotGroup TypeA/B） ============
CREATE TABLE shot_groups (
  id BIGSERIAL PRIMARY KEY,
  sg_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  narrative_unit_id BIGINT NOT NULL REFERENCES narrative_units(id) ON DELETE RESTRICT,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  sort_order INT NOT NULL DEFAULT 1,
  group_type shot_group_type_enum NOT NULL DEFAULT 'NAMED_SEGMENT',
  internal_code VARCHAR(64) NULL,
  title VARCHAR(256) NULL,
  duration_sec INT NOT NULL DEFAULT 0,
  pre_anchor_video_url VARCHAR(512) NULL,
  generated_video_url VARCHAR(512) NULL,
  reference_images_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  video_generation_status generation_status_enum NOT NULL DEFAULT 'PENDING',
  shot_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_shot_groups_uuid ON shot_groups(sg_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_shot_groups_nu_sort ON shot_groups(narrative_unit_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_shot_groups_project ON shot_groups(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_shot_groups_video_status ON shot_groups(video_generation_status) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON shot_groups FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ shots（镜头块内 2-4 个） ============
CREATE TABLE shots (
  id BIGSERIAL PRIMARY KEY,
  shot_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  shot_group_id BIGINT NOT NULL REFERENCES shot_groups(id) ON DELETE RESTRICT,
  narrative_unit_id BIGINT NOT NULL REFERENCES narrative_units(id) ON DELETE RESTRICT,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  sort_order INT NOT NULL DEFAULT 1,
  duration_sec INT NOT NULL DEFAULT 5,
  shot_size shot_size_enum NULL,
  camera_movement camera_move_enum NULL,
  camera_angle camera_angle_enum NULL,
  subject_description TEXT NULL,
  background_description TEXT NULL,
  local_light_description TEXT NULL,
  dialogue_text TEXT NULL,
  dialogue_meta_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ambient_sound TEXT NULL,
  prompt_original TEXT NULL,
  prompt_refined TEXT NULL,
  reference_images_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  linked_assets_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ai_model video_model_enum NULL,
  video_generation_status generation_status_enum NOT NULL DEFAULT 'PENDING',
  generated_video_url VARCHAR(512) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_shots_uuid ON shots(shot_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_shots_sg_sort ON shots(shot_group_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_shots_nu ON shots(narrative_unit_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_shots_project ON shots(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_shots_video_status ON shots(video_generation_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_shots_ai_model ON shots(ai_model) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON shots FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();
