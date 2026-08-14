-- 000005_projects_pipeline.up.sql
-- 项目流水线层：projects / episodes / narrative_units / narrative_unit_versions（D.1 顺序 5~7）
-- 说明：projects.source_template_id 引用 M1 表 canvas_templates，M0 不建外键，仅保留索引。

-- ============ projects ============
CREATE TABLE projects (
  id BIGSERIAL PRIMARY KEY,
  project_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  title VARCHAR(256) NOT NULL,
  cover_image_url VARCHAR(512) NULL,
  project_type project_type_enum NOT NULL DEFAULT 'SHORT_DRAMA_AGENT',
  source_script_text TEXT NULL,
  script_file_upload_token VARCHAR(128) NULL,
  script_file_url VARCHAR(512) NULL,
  source_template_id BIGINT NULL,
  total_episode_count INT NOT NULL DEFAULT 0,
  total_duration_sec INT NOT NULL DEFAULT 0,
  current_step project_step_enum NOT NULL DEFAULT 'SCRIPT_SPLIT',
  status project_status_enum NOT NULL DEFAULT 'DRAFT',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_projects_uuid ON projects(project_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_space_status ON projects(space_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_owner_user ON projects(owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_type ON projects(project_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_last_opened_at ON projects(last_opened_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_tags ON projects USING GIN(tags) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_space_last_opened ON projects(space_id, last_opened_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_source_template ON projects(source_template_id) WHERE deleted_at IS NULL AND source_template_id IS NOT NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ episodes ============
CREATE TABLE episodes (
  id BIGSERIAL PRIMARY KEY,
  episode_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  sort_order INT NOT NULL DEFAULT 1,
  title VARCHAR(256) NOT NULL,
  preset_total_duration_sec INT NULL,
  actual_duration_sec INT NOT NULL DEFAULT 0,
  original_script_text TEXT NULL,
  script_file_url VARCHAR(512) NULL,
  current_step project_step_enum NOT NULL DEFAULT 'SCRIPT_SPLIT',
  status project_status_enum NOT NULL DEFAULT 'DRAFT',
  narrative_unit_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_episodes_uuid ON episodes(episode_uuid) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_episodes_project_sort ON episodes(project_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_episodes_status ON episodes(project_id, status) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON episodes FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ narrative_units（Step1 拆分产物） ============
CREATE TABLE narrative_units (
  id BIGSERIAL PRIMARY KEY,
  nu_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  episode_id BIGINT NOT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  sort_order INT NOT NULL DEFAULT 1,
  title VARCHAR(256) NOT NULL,
  preset_duration_sec INT NULL,
  actual_duration_sec INT NOT NULL DEFAULT 0,
  content TEXT NULL,
  script_original_text TEXT NULL,
  linked_asset_count INT NOT NULL DEFAULT 0,
  planning_mode planning_mode_enum NULL,
  status generation_status_enum NOT NULL DEFAULT 'PENDING',
  version_num INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_narrative_units_uuid ON narrative_units(nu_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_narrative_units_episode_sort ON narrative_units(episode_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_narrative_units_project ON narrative_units(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_narrative_units_status ON narrative_units(status) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON narrative_units FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ narrative_unit_versions ============
CREATE TABLE narrative_unit_versions (
  id BIGSERIAL PRIMARY KEY,
  narrative_unit_id BIGINT NOT NULL REFERENCES narrative_units(id) ON DELETE RESTRICT,
  version_num INT NOT NULL,
  label VARCHAR(128) NULL,
  script_snapshot TEXT NULL,
  storyboard_json_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_from version_from_enum NOT NULL DEFAULT 'USER_MANUAL_EDIT',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_nu_versions_unique ON narrative_unit_versions(narrative_unit_id, version_num);
CREATE INDEX idx_nu_versions_created_at ON narrative_unit_versions(narrative_unit_id, created_at DESC);
