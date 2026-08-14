-- 000007_assets.up.sql
-- 资产层：s3_object_metadata / assets / characters / character_variants / scenes / props / generated_images / asset_usage_logs
-- 对齐 02 - 数据库设计.md B3 章（D.1 顺序 9~12）
-- 说明：引用 M1 表（three_d_scenes/storyboards）或后建表（generation_tasks）的字段不建外键约束，仅保留 BTREE 索引；
--       应用层保证引用一致性，M1 扩表时再补 FK。

-- ============ s3_object_metadata（对象元数据唯一物理真源） ============
CREATE TABLE s3_object_metadata (
  id BIGSERIAL PRIMARY KEY,
  object_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  bucket VARCHAR(128) NOT NULL,
  object_key VARCHAR(1024) NOT NULL,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  upload_token UUID NOT NULL,
  content_type VARCHAR(128) NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
  etag VARCHAR(128) NULL,
  checksum_sha256 CHAR(64) NULL,
  upload_status VARCHAR(16) NOT NULL DEFAULT 'PRESIGNED'
    CHECK (upload_status IN ('PRESIGNED','CONFIRMED','DELETE_PENDING','DELETED','EXPIRED')),
  ref_count INTEGER NOT NULL DEFAULT 0 CHECK (ref_count >= 0),
  confirmed_at TIMESTAMPTZ NULL,
  delete_after TIMESTAMPTZ NULL,
  deleted_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uq_s3_object_uuid UNIQUE (object_uuid),
  CONSTRAINT uq_s3_bucket_object_key UNIQUE (bucket, object_key),
  CONSTRAINT uq_s3_upload_token UNIQUE (upload_token)
);
CREATE INDEX idx_s3_object_space_status ON s3_object_metadata(space_id, upload_status, created_at DESC);
CREATE INDEX idx_s3_object_cleanup ON s3_object_metadata(delete_after)
  WHERE upload_status = 'DELETE_PENDING' AND deleted_at IS NULL;
CREATE INDEX idx_s3_object_unconfirmed ON s3_object_metadata(created_at)
  WHERE upload_status = 'PRESIGNED';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON s3_object_metadata FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ assets（统一资产顶层表） ============
CREATE TABLE assets (
  id BIGSERIAL PRIMARY KEY,
  asset_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  origin_episode_id BIGINT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  type asset_type_enum NOT NULL,
  sub_type VARCHAR(32) NULL,
  code VARCHAR(64) NULL,
  name VARCHAR(128) NOT NULL,
  description TEXT NULL,
  prompt TEXT NULL,
  reusable_episode_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  character_extra_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
  scene_extra_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
  prop_extra_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
  parent_id BIGINT NULL REFERENCES assets(id) ON DELETE RESTRICT,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  files_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  status asset_status_enum NOT NULL DEFAULT 'GENERATING',
  source VARCHAR(16) NOT NULL DEFAULT 'GENERATED',
  is_reusable_project_wide BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_assets_uuid ON assets(asset_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_space_type ON assets(space_id, type) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_project_type ON assets(project_id, type) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_owner ON assets(owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_origin_episode ON assets(origin_episode_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_parent ON assets(parent_id) WHERE deleted_at IS NULL AND parent_id IS NOT NULL;
CREATE INDEX idx_assets_name ON assets(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_tags ON assets USING GIN(tags) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_assets_project_code_unique ON assets(project_id, code)
  WHERE deleted_at IS NULL AND code IS NOT NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON assets FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ characters（角色主表） ============
CREATE TABLE characters (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  asset_id BIGINT NULL REFERENCES assets(id) ON DELETE RESTRICT,
  origin_episode_id BIGINT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  appearance_base_prompt TEXT NULL,
  ai_model_for_image image_model_enum NOT NULL DEFAULT 'LUX_BANANA_2',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  description_notes TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_characters_asset ON characters(asset_id) WHERE deleted_at IS NULL AND asset_id IS NOT NULL;
CREATE UNIQUE INDEX idx_characters_project_code ON characters(project_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_characters_project ON characters(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_characters_tags ON characters USING GIN(tags) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON characters FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ character_variants（角色变体） ============
CREATE TABLE character_variants (
  id BIGSERIAL PRIMARY KEY,
  character_id BIGINT NOT NULL REFERENCES characters(id) ON DELETE RESTRICT,
  asset_id BIGINT NULL REFERENCES assets(id) ON DELETE RESTRICT,
  variant_name VARCHAR(128) NOT NULL,
  variant_type character_variant_enum NOT NULL DEFAULT 'COSTUME',
  clothing_desc TEXT NULL,
  extra_appearance_prompt TEXT NULL,
  generation_params JSONB NOT NULL DEFAULT '{}'::jsonb,
  main_image_url VARCHAR(512) NULL,
  multi_view_images_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  image_resolution image_resolution_enum NOT NULL DEFAULT '2K',
  aspect_ratio image_aspect_ratio_enum NOT NULL DEFAULT 'R9_16',
  generation_status generation_status_enum NOT NULL DEFAULT 'PENDING',
  point_cost INT NOT NULL DEFAULT 0,
  generation_task_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_char_variants_asset ON character_variants(asset_id) WHERE deleted_at IS NULL AND asset_id IS NOT NULL;
CREATE INDEX idx_char_variants_character ON character_variants(character_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_char_variants_status ON character_variants(generation_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_char_variants_task ON character_variants(generation_task_id) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON character_variants FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ scenes（场景表） ============
CREATE TABLE scenes (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  asset_id BIGINT NULL REFERENCES assets(id) ON DELETE RESTRICT,
  origin_episode_id BIGINT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  code VARCHAR(64) NOT NULL,
  base_name VARCHAR(128) NOT NULL,
  time_of_day_variant time_of_day_enum NOT NULL DEFAULT 'DAY_BRIGHT',
  weather weather_enum NULL,
  base_prompt TEXT NULL,
  extra_prompt_for_variant TEXT NULL,
  ai_model_for_image image_model_enum NOT NULL DEFAULT 'LUX_BANANA_2',
  main_image_url VARCHAR(512) NULL,
  aspect_ratio image_aspect_ratio_enum NOT NULL DEFAULT 'R16_9',
  image_resolution image_resolution_enum NOT NULL DEFAULT '2K',
  generation_status generation_status_enum NOT NULL DEFAULT 'PENDING',
  point_cost INT NOT NULL DEFAULT 0,
  generation_task_id BIGINT NULL,
  three_d_scene_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_scenes_asset ON scenes(asset_id) WHERE deleted_at IS NULL AND asset_id IS NOT NULL;
CREATE UNIQUE INDEX idx_scenes_project_code ON scenes(project_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_scenes_project_tod ON scenes(project_id, time_of_day_variant) WHERE deleted_at IS NULL;
CREATE INDEX idx_scenes_status ON scenes(generation_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_scenes_3d ON scenes(three_d_scene_id) WHERE deleted_at IS NULL AND three_d_scene_id IS NOT NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON scenes FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ props（道具表） ============
CREATE TABLE props (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  asset_id BIGINT NULL REFERENCES assets(id) ON DELETE RESTRICT,
  origin_episode_id BIGINT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  category prop_category_enum NOT NULL DEFAULT 'OTHER',
  description_prompt TEXT NULL,
  main_image_url VARCHAR(512) NULL,
  aspect_ratio image_aspect_ratio_enum NOT NULL DEFAULT 'R1_1',
  image_resolution image_resolution_enum NOT NULL DEFAULT '2K',
  generation_status generation_status_enum NOT NULL DEFAULT 'PENDING',
  point_cost INT NOT NULL DEFAULT 0,
  generation_task_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_props_asset ON props(asset_id) WHERE deleted_at IS NULL AND asset_id IS NOT NULL;
CREATE UNIQUE INDEX idx_props_project_code ON props(project_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_props_project_category ON props(project_id, category) WHERE deleted_at IS NULL;
CREATE INDEX idx_props_status ON props(generation_status) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON props FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ generated_images（资产图/分镜帧图统一） ============
CREATE TABLE generated_images (
  id BIGSERIAL PRIMARY KEY,
  gi_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  asset_id BIGINT NULL REFERENCES assets(id) ON DELETE RESTRICT,
  storyboard_id BIGINT NULL,
  shot_id BIGINT NULL REFERENCES shots(id) ON DELETE RESTRICT,
  prompt TEXT NULL,
  original_url VARCHAR(512) NOT NULL,
  thumb_url VARCHAR(512) NULL,
  width_px INT NULL,
  height_px INT NULL,
  file_size_bytes BIGINT NULL,
  state generation_status_enum NOT NULL DEFAULT 'PENDING',
  error_reason VARCHAR(512) NULL,
  ref_asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  generation_task_id BIGINT NULL,
  sort_order INT NULL,
  seed_value BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_generated_images_uuid ON generated_images(gi_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_generated_images_asset ON generated_images(asset_id) WHERE deleted_at IS NULL AND asset_id IS NOT NULL;
CREATE INDEX idx_generated_images_storyboard ON generated_images(storyboard_id, sort_order)
  WHERE deleted_at IS NULL AND storyboard_id IS NOT NULL;
CREATE INDEX idx_generated_images_shot ON generated_images(shot_id) WHERE deleted_at IS NULL AND shot_id IS NOT NULL;
CREATE INDEX idx_generated_images_state ON generated_images(state) WHERE deleted_at IS NULL;
CREATE INDEX idx_generated_images_task ON generated_images(generation_task_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_generated_images_ref_assets ON generated_images USING GIN(ref_asset_ids) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON generated_images FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ asset_usage_logs（跨集复用判定流水，计费核心） ============
CREATE TABLE asset_usage_logs (
  id BIGSERIAL PRIMARY KEY,
  asset_kind asset_kind_enum NOT NULL,
  asset_id BIGINT NOT NULL,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  episode_id BIGINT NOT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  narrative_unit_id BIGINT NULL REFERENCES narrative_units(id) ON DELETE RESTRICT,
  is_first_time_use_project_wide BOOLEAN NOT NULL DEFAULT TRUE,
  is_first_time_use_episode_wide BOOLEAN NOT NULL DEFAULT TRUE,
  point_cost_applied INT NOT NULL DEFAULT 0,
  generation_task_id BIGINT NULL,
  user_notified BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_asset_usage_unique ON asset_usage_logs(asset_kind, asset_id, episode_id);
CREATE INDEX idx_asset_usage_project_episode ON asset_usage_logs(project_id, episode_id);
CREATE INDEX idx_asset_usage_kind_asset ON asset_usage_logs(asset_kind, asset_id, episode_id);
CREATE INDEX idx_asset_usage_task ON asset_usage_logs(generation_task_id) WHERE generation_task_id IS NOT NULL;
