-- 000011_export_notify_works.up.sql
-- 导出记录 / 消息通知 / 作品发布 / 审计日志层（D.1 顺序 17/19/20/21）
-- 同时写入 M0 积分包 3 档默认定价（后台可配置，非硬编码，对齐 03 E.6.2）

-- ============ export_records（导出记录） ============
CREATE TABLE export_records (
  id BIGSERIAL PRIMARY KEY,
  export_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  export_type export_type_enum NOT NULL,
  export_range_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  config_resolution video_resolution_enum NULL,
  config_watermark BOOLEAN NOT NULL DEFAULT TRUE,
  config_subtitle_language VARCHAR(8) NULL,
  output_file_url VARCHAR(512) NULL,
  output_file_size_bytes BIGINT NULL,
  point_cost INT NOT NULL DEFAULT 0,
  status generation_status_enum NOT NULL DEFAULT 'PENDING',
  generation_task_id BIGINT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  downloaded_at TIMESTAMPTZ NULL,
  expire_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_export_records_uuid ON export_records(export_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_export_records_user_created ON export_records(user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_export_records_project ON export_records(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_export_records_space_type ON export_records(space_id, export_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_export_records_status ON export_records(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_export_records_expire ON export_records(expire_at)
  WHERE deleted_at IS NULL AND status = 'SUCCESS';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON export_records FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ notifications（通知表） ============
CREATE TABLE notifications (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  type notification_type_enum NOT NULL,
  title VARCHAR(256) NOT NULL,
  content TEXT NULL,
  data_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  link_url VARCHAR(512) NULL,
  is_read BOOLEAN NOT NULL DEFAULT FALSE,
  read_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_notifications_user_read ON notifications(user_id, is_read, created_at DESC)
  WHERE deleted_at IS NULL;
CREATE INDEX idx_notifications_user_type ON notifications(user_id, type, created_at DESC)
  WHERE deleted_at IS NULL;

-- ============ works（作品表） ============
CREATE TABLE works (
  id BIGSERIAL PRIMARY KEY,
  work_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  project_id BIGINT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  kind work_kind_enum NOT NULL DEFAULT 'PROJECT',
  title VARCHAR(256) NOT NULL,
  description TEXT NULL,
  cover_url VARCHAR(512) NULL,
  media_url VARCHAR(512) NULL,
  duration_sec INT NOT NULL DEFAULT 0,
  resolution VARCHAR(16) NULL,
  publish_status publish_status_enum NOT NULL DEFAULT 'UNPUBLISHED',
  audit_note VARCHAR(512) NULL,
  view_count INT NOT NULL DEFAULT 0,
  like_count INT NOT NULL DEFAULT 0,
  favorite_count INT NOT NULL DEFAULT 0,
  comment_count INT NOT NULL DEFAULT 0,
  share_count INT NOT NULL DEFAULT 0,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  is_deleted_recyclebin BOOLEAN NOT NULL DEFAULT FALSE,
  published_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_works_uuid ON works(work_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_works_space_publish ON works(space_id, publish_status) WHERE deleted_at IS NULL;
CREATE INDEX idx_works_owner ON works(owner_user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_works_publish_status_published ON works(publish_status, published_at DESC)
  WHERE deleted_at IS NULL AND publish_status = 'PUBLISHED';
CREATE INDEX idx_works_kind ON works(kind) WHERE deleted_at IS NULL;
CREATE INDEX idx_works_view_count ON works(view_count DESC)
  WHERE deleted_at IS NULL AND publish_status = 'PUBLISHED';
CREATE INDEX idx_works_tags ON works USING GIN(tags)
  WHERE deleted_at IS NULL AND publish_status = 'PUBLISHED';
CREATE INDEX idx_works_recyclebin ON works(owner_user_id, is_deleted_recyclebin)
  WHERE deleted_at IS NULL AND is_deleted_recyclebin = TRUE;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON works FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ publish_records（发布审核记录，不可变） ============
CREATE TABLE publish_records (
  id BIGSERIAL PRIMARY KEY,
  work_id BIGINT NOT NULL REFERENCES works(id) ON DELETE RESTRICT,
  action publish_action_enum NOT NULL,
  operator_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  submitter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  note VARCHAR(512) NULL,
  snapshot_before_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  snapshot_after_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_publish_records_work ON publish_records(work_id, created_at DESC);
CREATE INDEX idx_publish_records_operator ON publish_records(operator_user_id, created_at DESC)
  WHERE operator_user_id IS NOT NULL;

-- ============ audit_logs（审计日志，不可变） ============
CREATE TABLE audit_logs (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  operator_role VARCHAR(32) NULL,
  action VARCHAR(64) NOT NULL,
  target_type VARCHAR(32) NULL,
  target_id BIGINT NULL,
  detail_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  client_ip VARCHAR(64) NULL,
  user_agent VARCHAR(512) NULL,
  request_id VARCHAR(128) NULL,
  risk_level VARCHAR(16) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_user_action ON audit_logs(user_id, action, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_audit_target ON audit_logs(target_type, target_id, created_at DESC)
  WHERE target_type IS NOT NULL AND target_id IS NOT NULL;
CREATE INDEX idx_audit_action_created ON audit_logs(action, created_at DESC);
CREATE INDEX idx_audit_risk ON audit_logs(risk_level, created_at DESC) WHERE risk_level IS NOT NULL;

-- ============ 种子数据：M0 积分包 3 档（后台可配置，对齐 03 E.6.2） ============
INSERT INTO point_packages (pkg_code, name, points, bonus_points, price_yuan, original_price_yuan, effective_days, sort_order)
VALUES
  ('POINTS_PKG_SMALL',  '小包', 500,   0,     49.00,  49.00,  365, 1),
  ('POINTS_PKG_MEDIUM', '中包', 2500,  1250,  199.00, 199.00, 365, 2),
  ('POINTS_PKG_LARGE',  '大包', 15000, 7500,  999.00, 999.00, 365, 3)
ON CONFLICT (pkg_code) WHERE deleted_at IS NULL DO NOTHING;
