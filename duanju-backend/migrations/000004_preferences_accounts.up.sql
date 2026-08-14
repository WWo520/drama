-- 000004_preferences_accounts.up.sql
-- 用户偏好与积分账户层：user_preferences / point_accounts（D.1 顺序 4）

-- ============ user_preferences ============
CREATE TABLE user_preferences (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  theme VARCHAR(16) NOT NULL DEFAULT 'light',
  language VARCHAR(8) NOT NULL DEFAULT 'zh-CN',
  timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
  think_level think_level_enum NOT NULL DEFAULT 'LIGHT',
  aigc_watermark BOOLEAN NOT NULL DEFAULT TRUE,
  default_video_model VARCHAR(64) NULL,
  default_image_model VARCHAR(64) NULL,
  default_llm_model VARCHAR(64) NULL,
  default_style_code VARCHAR(64) NULL,
  default_resolution video_resolution_enum NULL,
  default_aspect_ratio image_aspect_ratio_enum NULL,
  auto_save_canvas_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  auto_save_interval_seconds INT NOT NULL DEFAULT 30,
  ws_notification_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  email_notification_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  ui_preferences_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_user_preferences_user ON user_preferences(user_id) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON user_preferences FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ point_accounts（1 用户/团队空间 = 1 行） ============
CREATE TABLE point_accounts (
  id BIGSERIAL PRIMARY KEY,
  owner_type owner_type_enum NOT NULL DEFAULT 'USER',
  owner_id BIGINT NOT NULL,
  current_balance INT NOT NULL DEFAULT 0 CHECK (current_balance >= 0),
  frozen_balance INT NOT NULL DEFAULT 0 CHECK (frozen_balance >= 0),
  lifetime_earned_total BIGINT NOT NULL DEFAULT 0,
  lifetime_spent_total BIGINT NOT NULL DEFAULT 0,
  total_generation_count INT NOT NULL DEFAULT 0,
  last_txn_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_point_accounts_owner_unique ON point_accounts(owner_type, owner_id);
CREATE TRIGGER set_timestamp BEFORE UPDATE ON point_accounts FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ platform_configs（注册赠送积分等平台参数，B9.2） ============
CREATE TABLE platform_configs (
  id SERIAL PRIMARY KEY,
  config_key VARCHAR(128) NOT NULL UNIQUE,
  config_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  config_group VARCHAR(64) NOT NULL DEFAULT 'general',
  value_type VARCHAR(16) NOT NULL DEFAULT 'json',
  description VARCHAR(256) NULL,
  is_public BOOLEAN NOT NULL DEFAULT FALSE,
  updated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_platform_configs_group ON platform_configs(config_group);
CREATE TRIGGER set_timestamp BEFORE UPDATE ON platform_configs FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- 初始数据：注册赠送积分默认 500（后台可配置，非硬编码）
INSERT INTO platform_configs (config_key, config_value, config_group, value_type, description, is_public)
VALUES ('initial_points', '500', 'general', 'number', '注册赠送初始积分', TRUE)
ON CONFLICT (config_key) DO NOTHING;
