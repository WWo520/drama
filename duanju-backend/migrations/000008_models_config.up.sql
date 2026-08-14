-- 000008_models_config.up.sql
-- 模型治理与配置层：model_providers / models / model_keys / agent_skills / style_library（D.1 顺序 13~14）
-- 说明：style_library.lighting_preset_id 引用 M1 表 three_d_scenes，M0 不建外键。

-- ============ model_providers（模型厂商） ============
CREATE TABLE model_providers (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  display_name VARCHAR(128) NOT NULL,
  capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
  base_url VARCHAR(512) NOT NULL,
  api_config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  webhook_secret VARCHAR(512) NULL,
  logo_url VARCHAR(512) NULL,
  website_url VARCHAR(512) NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_model_providers_name ON model_providers(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_model_providers_enabled ON model_providers(enabled, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_model_providers_capabilities ON model_providers USING GIN(capabilities) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON model_providers FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ models（模型表） ============
CREATE TABLE models (
  id BIGSERIAL PRIMARY KEY,
  provider_id BIGINT NOT NULL REFERENCES model_providers(id) ON DELETE RESTRICT,
  model_key VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  capability model_capability_enum NOT NULL,
  description VARCHAR(512) NULL,
  params_template_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  supported_resolutions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  supported_ratios_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  cost_points_per_call INT NOT NULL DEFAULT 0,
  cost_points_per_second INT NOT NULL DEFAULT 0,
  min_vip_level_required SMALLINT NOT NULL DEFAULT 0,
  is_default_for_capability BOOLEAN NOT NULL DEFAULT FALSE,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_models_key ON models(model_key) WHERE deleted_at IS NULL;
CREATE INDEX idx_models_provider ON models(provider_id, enabled) WHERE deleted_at IS NULL;
CREATE INDEX idx_models_capability ON models(capability, enabled, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_models_default_cap ON models(capability)
  WHERE deleted_at IS NULL AND is_default_for_capability = TRUE AND enabled = TRUE;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON models FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ model_keys（模型 API Key，AES 加密存储） ============
CREATE TABLE model_keys (
  id BIGSERIAL PRIMARY KEY,
  provider_id BIGINT NOT NULL REFERENCES model_providers(id) ON DELETE RESTRICT,
  name VARCHAR(128) NOT NULL,
  key_encrypted VARCHAR(512) NOT NULL,
  key_prefix VARCHAR(16) NULL,
  rate_limit_rpm INT NOT NULL DEFAULT 60,
  rate_limit_qps INT NOT NULL DEFAULT 10,
  concurrent_limit INT NOT NULL DEFAULT 5,
  daily_quota_limit BIGINT NULL,
  daily_quota_used BIGINT NOT NULL DEFAULT 0,
  daily_quota_reset_date DATE NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  priority INT NOT NULL DEFAULT 0,
  last_used_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_model_keys_provider_enabled ON model_keys(provider_id, enabled, priority) WHERE deleted_at IS NULL;
CREATE INDEX idx_model_keys_last_used ON model_keys(last_used_at DESC)
  WHERE deleted_at IS NULL AND enabled = TRUE;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON model_keys FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ agent_skills（智能体技能） ============
CREATE TABLE agent_skills (
  id BIGSERIAL PRIMARY KEY,
  skill_code VARCHAR(64) NOT NULL,
  kind agent_skill_kind_enum NOT NULL,
  name VARCHAR(128) NOT NULL,
  description VARCHAR(512) NULL,
  template_content TEXT NOT NULL,
  llm_model_key VARCHAR(64) NULL,
  version_num INT NOT NULL DEFAULT 1,
  input_schema_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  output_schema_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_agent_skills_code ON agent_skills(skill_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_agent_skills_kind ON agent_skills(kind, enabled, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_agent_skills_tags ON agent_skills USING GIN(tags) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON agent_skills FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ style_library（画面风格库） ============
CREATE TABLE style_library (
  id BIGSERIAL PRIMARY KEY,
  style_code VARCHAR(64) NOT NULL,
  scope template_scope_enum NOT NULL DEFAULT 'PUBLIC_OFFICIAL',
  library_layer style_layer_enum NOT NULL DEFAULT 'OFFICIAL_PUBLIC_LIBRARY',
  category style_category_enum NOT NULL DEFAULT 'GENERAL_REALISTIC',
  name VARCHAR(128) NOT NULL,
  description TEXT NULL,
  thumbnail_url VARCHAR(512) NULL,
  preview_image_urls_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  preview_video_url VARCHAR(512) NULL,
  prompt_suffix_static TEXT NULL,
  camera_preset_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  lighting_preset_id BIGINT NULL,
  color_grading_lut_url VARCHAR(512) NULL,
  creator_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  creator_space_id BIGINT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  reference_works_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  suitable_genres_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  sort_order INT NOT NULL DEFAULT 0,
  min_vip_level_required SMALLINT NOT NULL DEFAULT 0,
  used_count INT NOT NULL DEFAULT 0,
  like_count INT NOT NULL DEFAULT 0,
  official_star_rating NUMERIC(2,1) NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_style_library_code ON style_library(style_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_style_library_layer_category ON style_library(library_layer, category, sort_order)
  WHERE deleted_at IS NULL AND enabled = TRUE;
CREATE INDEX idx_style_library_scope ON style_library(scope, category) WHERE deleted_at IS NULL;
CREATE INDEX idx_style_library_is_default ON style_library(is_default)
  WHERE deleted_at IS NULL AND is_default = TRUE AND enabled = TRUE;
CREATE INDEX idx_style_library_used ON style_library(used_count DESC)
  WHERE deleted_at IS NULL AND enabled = TRUE;
CREATE INDEX idx_style_library_tags ON style_library USING GIN(tags) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON style_library FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();
