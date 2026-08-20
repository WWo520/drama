-- 000026_withdrawals.up.sql
-- 提现审核域：创作者提现申请（管理端审核流转 PENDING → APPROVED/REJECTED）。
CREATE TABLE IF NOT EXISTS withdrawal_requests (
  id                  BIGSERIAL PRIMARY KEY,
  request_uuid        UUID NOT NULL DEFAULT gen_random_uuid(),
  user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  amount_yuan         NUMERIC(10,2) NOT NULL CHECK (amount_yuan > 0),
  payee_name          VARCHAR(64) NOT NULL,
  payee_account       VARCHAR(128) NOT NULL,
  status              VARCHAR(16) NOT NULL DEFAULT 'PENDING', -- PENDING / APPROVED / REJECTED
  review_note         TEXT NULL,
  reviewed_by_user_id BIGINT NULL REFERENCES users(id),
  reviewed_at         TIMESTAMPTZ NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_withdrawal_status ON withdrawal_requests(status, created_at DESC);
CREATE TRIGGER set_timestamp BEFORE UPDATE ON withdrawal_requests FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();
