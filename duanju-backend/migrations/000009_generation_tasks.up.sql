-- 000009_generation_tasks.up.sql
-- 生成任务层：generation_tasks / task_dispatch_outbox / model_callback_logs / generation_task_retry_logs
-- 对齐 02 - 数据库设计.md B4 章（D.1 顺序 15）。计费三字段与任务状态机唯一裁决均在此表。

-- ============ generation_tasks（生成任务主表，统一异步任务） ============
CREATE TABLE generation_tasks (
  id BIGSERIAL PRIMARY KEY,
  task_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  request_id VARCHAR(128) NOT NULL,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  project_id BIGINT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  episode_id BIGINT NULL REFERENCES episodes(id) ON DELETE RESTRICT,
  task_type task_type_enum NOT NULL,
  asset_kind asset_kind_enum NULL,
  target_id BIGINT NULL,
  parent_task_id BIGINT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  status generation_status_enum NOT NULL DEFAULT 'PENDING',
  priority SMALLINT NOT NULL DEFAULT 5,
  ai_model_requested VARCHAR(64) NULL,
  ai_model_actual_used VARCHAR(64) NULL,
  request_params_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  progress_pct SMALLINT NOT NULL DEFAULT 0,
  progress_message VARCHAR(512) NULL,
  retry_count SMALLINT NOT NULL DEFAULT 0,
  max_retry_count SMALLINT NOT NULL DEFAULT 3,
  result_output_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code VARCHAR(32) NULL,
  error_message TEXT NULL,
  user_display_error VARCHAR(512) NULL,
  queue_wait_ms INT NULL,
  execution_ms INT NULL,
  total_execution_ms INT NULL,
  gpu_node_id VARCHAR(128) NULL,
  external_task_id VARCHAR(64) NULL,
  lease_owner VARCHAR(128) NULL,
  lease_expires_at TIMESTAMPTZ NULL,
  heartbeat_at TIMESTAMPTZ NULL,
  next_retry_at TIMESTAMPTZ NULL,
  enqueued_at TIMESTAMPTZ NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  charged_account_id BIGINT NULL REFERENCES point_accounts(id) ON DELETE RESTRICT,
  charged_owner_type owner_type_enum NULL,
  charged_owner_id BIGINT NULL,
  frozen_points INT NOT NULL DEFAULT 0,
  thawed_points INT NOT NULL DEFAULT 0,
  is_success_refunded BOOLEAN NOT NULL DEFAULT FALSE,
  point_cost_final INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_gen_tasks_uuid ON generation_tasks(task_uuid);
CREATE UNIQUE INDEX idx_gen_tasks_request_id ON generation_tasks(request_id);
CREATE UNIQUE INDEX idx_gen_tasks_idempotency_key ON generation_tasks(idempotency_key);
CREATE INDEX idx_gen_tasks_status_updated ON generation_tasks(status, updated_at DESC)
  WHERE deleted_at IS NULL AND status NOT IN ('SUCCESS','FAILED_DEAD','CANCELED_USER','CANCELED_TIMEOUT');
CREATE INDEX idx_gen_tasks_space_type ON generation_tasks(space_id, task_type) WHERE deleted_at IS NULL;
CREATE INDEX idx_gen_tasks_user ON generation_tasks(user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_gen_tasks_charged_account ON generation_tasks(charged_account_id, created_at DESC)
  WHERE deleted_at IS NULL AND charged_account_id IS NOT NULL;
CREATE INDEX idx_gen_tasks_project ON generation_tasks(project_id) WHERE deleted_at IS NULL AND project_id IS NOT NULL;
CREATE INDEX idx_gen_tasks_episode ON generation_tasks(episode_id) WHERE deleted_at IS NULL AND episode_id IS NOT NULL;
CREATE INDEX idx_gen_tasks_parent ON generation_tasks(parent_task_id) WHERE deleted_at IS NULL AND parent_task_id IS NOT NULL;
CREATE INDEX idx_gen_tasks_error_code ON generation_tasks(error_code) WHERE deleted_at IS NULL AND error_code IS NOT NULL;
CREATE INDEX idx_gen_tasks_started_at ON generation_tasks(started_at DESC) WHERE deleted_at IS NULL AND started_at IS NOT NULL;
CREATE INDEX idx_gen_tasks_completed_at ON generation_tasks(completed_at DESC) WHERE deleted_at IS NULL AND completed_at IS NOT NULL;
CREATE INDEX idx_gen_tasks_recovery ON generation_tasks(status, lease_expires_at)
  WHERE deleted_at IS NULL AND status = 'RUNNING';
CREATE INDEX idx_gen_tasks_retry_due ON generation_tasks(next_retry_at)
  WHERE deleted_at IS NULL AND status = 'FAILED_RETRYING';
CREATE INDEX idx_gen_tasks_queued_repair ON generation_tasks(enqueued_at, created_at)
  WHERE deleted_at IS NULL AND status = 'QUEUED';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON generation_tasks FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ task_dispatch_outbox（DB→Redis 可靠投递记录） ============
CREATE TABLE task_dispatch_outbox (
  id BIGSERIAL PRIMARY KEY,
  event_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  event_type VARCHAR(32) NOT NULL DEFAULT 'TASK_DISPATCH' CHECK (event_type = 'TASK_DISPATCH'),
  task_id BIGINT NOT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  dispatch_key VARCHAR(160) NOT NULL,
  queue_name VARCHAR(32) NOT NULL,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  status task_outbox_status_enum NOT NULL DEFAULT 'PENDING',
  attempt_count SMALLINT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  lease_owner VARCHAR(128) NULL,
  lease_expires_at TIMESTAMPTZ NULL,
  published_at TIMESTAMPTZ NULL,
  last_error_code VARCHAR(64) NULL,
  last_error_message VARCHAR(512) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_task_outbox_event_uuid ON task_dispatch_outbox(event_uuid);
CREATE UNIQUE INDEX idx_task_dispatch_outbox_dispatch_key ON task_dispatch_outbox(dispatch_key);
CREATE UNIQUE INDEX idx_task_dispatch_outbox_unpublished_task ON task_dispatch_outbox(task_id)
  WHERE status IN ('PENDING','PROCESSING');
CREATE INDEX idx_task_dispatch_outbox_claim ON task_dispatch_outbox(status, next_attempt_at, created_at)
  WHERE status IN ('PENDING','PROCESSING');
CREATE INDEX idx_task_dispatch_outbox_lease ON task_dispatch_outbox(lease_expires_at)
  WHERE status = 'PROCESSING';
CREATE INDEX idx_task_dispatch_outbox_dead ON task_dispatch_outbox(created_at DESC)
  WHERE status = 'DEAD';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON task_dispatch_outbox FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ model_callback_logs（模型回调审计） ============
CREATE TABLE model_callback_logs (
  id BIGSERIAL PRIMARY KEY,
  provider_id BIGINT NOT NULL REFERENCES model_providers(id) ON DELETE RESTRICT,
  task_id BIGINT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  external_task_id VARCHAR(64) NOT NULL,
  callback_status VARCHAR(32) NOT NULL,
  signature_valid BOOLEAN NOT NULL DEFAULT FALSE,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  processed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_model_callback_provider_task ON model_callback_logs(provider_id, external_task_id)
  WHERE signature_valid = TRUE;
CREATE INDEX idx_model_callback_task_created ON model_callback_logs(task_id, created_at DESC)
  WHERE task_id IS NOT NULL;

-- ============ generation_task_retry_logs（重试日志） ============
CREATE TABLE generation_task_retry_logs (
  id BIGSERIAL PRIMARY KEY,
  task_id BIGINT NOT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  retry_num SMALLINT NOT NULL,
  from_status generation_status_enum NOT NULL,
  to_status generation_status_enum NOT NULL,
  reason VARCHAR(256) NULL,
  backoff_seconds INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_gtrl_task ON generation_task_retry_logs(task_id, retry_num);
