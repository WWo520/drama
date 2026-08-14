-- 000010_billing.up.sql
-- 商业化计费层：point_packages / packages / orders / point_transactions / billing_records / user_memberships
-- 对齐 02 - 数据库设计.md B8 章（D.1 顺序 16/18）。财务流水表不可软删（C5 铁律）。

-- ============ point_packages（积分包） ============
CREATE TABLE point_packages (
  id BIGSERIAL PRIMARY KEY,
  pkg_code VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  points INT NOT NULL DEFAULT 0,
  bonus_points INT NOT NULL DEFAULT 0,
  price_yuan NUMERIC(10,2) NOT NULL DEFAULT 0,
  original_price_yuan NUMERIC(10,2) NULL,
  effective_days INT NOT NULL DEFAULT 365,
  is_limited_offer BOOLEAN NOT NULL DEFAULT FALSE,
  limited_offer_expire_at TIMESTAMPTZ NULL,
  sort_order INT NOT NULL DEFAULT 0,
  status VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_point_packages_code ON point_packages(pkg_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_point_packages_status_sort ON point_packages(status, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_point_packages_limited ON point_packages(is_limited_offer, limited_offer_expire_at)
  WHERE deleted_at IS NULL AND is_limited_offer = TRUE AND status = 'ON';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON point_packages FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ packages（套餐） ============
CREATE TABLE packages (
  id BIGSERIAL PRIMARY KEY,
  package_code VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  version VARCHAR(32) NOT NULL DEFAULT 'basic',
  billing_cycle VARCHAR(16) NOT NULL DEFAULT 'monthly',
  price_yuan NUMERIC(10,2) NOT NULL DEFAULT 0,
  original_price_yuan NUMERIC(10,2) NULL,
  points_quota INT NOT NULL DEFAULT 0,
  points_monthly_grant INT NOT NULL DEFAULT 0,
  storage_quota_gb INT NOT NULL DEFAULT 10,
  member_limit INT NOT NULL DEFAULT 1,
  concurrent_task_limit INT NOT NULL DEFAULT 2,
  vip_level_granted SMALLINT NOT NULL DEFAULT 0,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  features_list_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  is_first_purchase_bonus_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  first_purchase_bonus_points INT NOT NULL DEFAULT 0,
  sort_order INT NOT NULL DEFAULT 0,
  is_recommended BOOLEAN NOT NULL DEFAULT FALSE,
  status VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_packages_code ON packages(package_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_packages_version_cycle ON packages(version, billing_cycle, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_packages_recommended ON packages(is_recommended, sort_order)
  WHERE deleted_at IS NULL AND status = 'ON';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON packages FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ user_memberships（会员订阅） ============
CREATE TABLE user_memberships (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  package_id BIGINT NOT NULL REFERENCES packages(id) ON DELETE RESTRICT,
  package_code VARCHAR(64) NOT NULL,
  status subscription_status_enum NOT NULL DEFAULT 'ACTIVE',
  renew_strategy VARCHAR(32) NOT NULL DEFAULT 'MANUAL_ONE_TIME',
  start_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  current_period_end TIMESTAMPTZ NOT NULL,
  cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
  monthly_points_grant_balance INT NOT NULL DEFAULT 0,
  next_grant_at TIMESTAMPTZ NULL,
  total_cycles_paid INT NOT NULL DEFAULT 1,
  last_renew_order_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_memberships_user_status ON user_memberships(user_id, status);
CREATE INDEX idx_user_memberships_period_end ON user_memberships(current_period_end)
  WHERE status = 'ACTIVE';
CREATE INDEX idx_user_memberships_next_grant ON user_memberships(next_grant_at)
  WHERE status = 'ACTIVE' AND next_grant_at IS NOT NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON user_memberships FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ orders（订单表） ============
CREATE TABLE orders (
  id BIGSERIAL PRIMARY KEY,
  order_no VARCHAR(64) NOT NULL,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  order_type VARCHAR(32) NOT NULL,
  package_id BIGINT NULL REFERENCES packages(id) ON DELETE RESTRICT,
  point_package_id BIGINT NULL REFERENCES point_packages(id) ON DELETE RESTRICT,
  plan_code VARCHAR(64) NULL,
  original_amount_yuan NUMERIC(10,2) NOT NULL DEFAULT 0,
  discount_amount_yuan NUMERIC(10,2) NOT NULL DEFAULT 0,
  first_purchase_bonus_points INT NOT NULL DEFAULT 0,
  paid_amount_yuan NUMERIC(10,2) NOT NULL DEFAULT 0,
  pay_channel VARCHAR(32) NULL,
  pay_status payment_status_enum NOT NULL DEFAULT 'PENDING',
  pay_transaction_id VARCHAR(128) NULL,
  points_granted_total INT NOT NULL DEFAULT 0,
  subscription_days_granted INT NOT NULL DEFAULT 0,
  related_membership_id BIGINT NULL REFERENCES user_memberships(id) ON DELETE RESTRICT,
  idempotency_key VARCHAR(128) NOT NULL,
  paid_at TIMESTAMPTZ NULL,
  receipt_url VARCHAR(512) NULL,
  invoice_requested BOOLEAN NOT NULL DEFAULT FALSE,
  expired_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 minutes',
  refunded_at TIMESTAMPTZ NULL,
  refund_reason VARCHAR(256) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_orders_order_no ON orders(order_no);
CREATE UNIQUE INDEX idx_orders_idempotency_key ON orders(idempotency_key);
CREATE UNIQUE INDEX idx_orders_pay_transaction ON orders(pay_transaction_id)
  WHERE pay_transaction_id IS NOT NULL;
CREATE INDEX idx_orders_user_created ON orders(user_id, created_at DESC);
CREATE INDEX idx_orders_pay_status ON orders(pay_status, created_at DESC);
CREATE INDEX idx_orders_pay_channel ON orders(pay_channel, created_at DESC) WHERE pay_channel IS NOT NULL;
CREATE INDEX idx_orders_expired ON orders(expired_at) WHERE pay_status = 'PENDING';
CREATE INDEX idx_orders_paid_at ON orders(paid_at DESC) WHERE pay_status = 'PAID';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ point_transactions（积分流水，不可变财务表） ============
CREATE TABLE point_transactions (
  id BIGSERIAL PRIMARY KEY,
  txn_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  account_id BIGINT NOT NULL REFERENCES point_accounts(id) ON DELETE RESTRICT,
  owner_type owner_type_enum NOT NULL,
  owner_id BIGINT NOT NULL,
  txn_type point_txn_type_enum NOT NULL,
  direction txn_direction_enum NOT NULL,
  amount INT NOT NULL DEFAULT 0 CHECK (amount >= 0),
  balance_after INT NOT NULL DEFAULT 0,
  frozen_after INT NOT NULL DEFAULT 0,
  related_order_id BIGINT NULL REFERENCES orders(id) ON DELETE RESTRICT,
  related_generation_task_id BIGINT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  related_space_id BIGINT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  operator_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  related_project_id BIGINT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  related_export_id BIGINT NULL,
  cross_episode_reuse_exempt_amount INT NOT NULL DEFAULT 0,
  description VARCHAR(512) NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_point_txn_uuid ON point_transactions(txn_uuid);
CREATE UNIQUE INDEX idx_point_txn_idempotency_key ON point_transactions(idempotency_key);
CREATE INDEX idx_point_txn_account_created ON point_transactions(account_id, created_at DESC);
CREATE INDEX idx_point_txn_owner_created ON point_transactions(owner_type, owner_id, created_at DESC);
CREATE INDEX idx_point_txn_space_operator_created ON point_transactions(related_space_id, operator_user_id, created_at DESC)
  WHERE related_space_id IS NOT NULL AND operator_user_id IS NOT NULL;
CREATE INDEX idx_point_txn_type ON point_transactions(txn_type, created_at DESC);
CREATE INDEX idx_point_txn_direction ON point_transactions(direction, created_at DESC);
CREATE INDEX idx_point_txn_task ON point_transactions(related_generation_task_id)
  WHERE related_generation_task_id IS NOT NULL;
CREATE INDEX idx_point_txn_order ON point_transactions(related_order_id) WHERE related_order_id IS NOT NULL;

-- ============ billing_records（计费记录，任务消耗明细） ============
CREATE TABLE billing_records (
  id BIGSERIAL PRIMARY KEY,
  billing_account_id BIGINT NOT NULL REFERENCES point_accounts(id) ON DELETE RESTRICT,
  billed_owner_type owner_type_enum NOT NULL,
  billed_owner_id BIGINT NOT NULL,
  operator_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  task_id BIGINT NOT NULL REFERENCES generation_tasks(id) ON DELETE RESTRICT,
  model_key VARCHAR(64) NULL,
  task_type task_type_enum NULL,
  cost_points INT NOT NULL DEFAULT 0,
  cross_episode_exempt_points INT NOT NULL DEFAULT 0,
  billing_details_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  point_txn_id BIGINT NULL REFERENCES point_transactions(id) ON DELETE RESTRICT,
  billed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_billing_records_task ON billing_records(task_id);
CREATE UNIQUE INDEX idx_billing_records_point_txn ON billing_records(point_txn_id)
  WHERE point_txn_id IS NOT NULL;
CREATE INDEX idx_billing_user_created ON billing_records(user_id, billed_at DESC);
CREATE INDEX idx_billing_space_operator_created ON billing_records(space_id, operator_user_id, billed_at DESC);
CREATE INDEX idx_billing_account_created ON billing_records(billing_account_id, billed_at DESC);
CREATE INDEX idx_billing_space_created ON billing_records(space_id, billed_at DESC);
CREATE INDEX idx_billing_task_type ON billing_records(task_type, billed_at DESC) WHERE task_type IS NOT NULL;
