-- 阶段 4 生产加固（H01）：回调日志表
CREATE TABLE IF NOT EXISTS pay_callback_logs (
    id              BIGSERIAL PRIMARY KEY,
    params_snapshot JSONB NOT NULL DEFAULT '{}',
    order_no        VARCHAR(64),
    pay_transaction_id VARCHAR(128),
    verification_step INT NOT NULL DEFAULT 0,
    result          VARCHAR(16) NOT NULL, -- success / fail
    fail_reason     VARCHAR(256),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pay_callback_order ON pay_callback_logs (order_no, created_at DESC);