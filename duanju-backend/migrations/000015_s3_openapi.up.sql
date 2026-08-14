-- 阶段 3 M4 扩表（S3-B09）：api_keys + webhooks
CREATE TABLE IF NOT EXISTS api_keys (
    id              BIGSERIAL PRIMARY KEY,
    api_key_hashed  VARCHAR(128) NOT NULL UNIQUE,
    api_key_prefix  VARCHAR(16) NOT NULL,
    user_id         BIGINT NOT NULL,
    name            VARCHAR(64) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    quota_per_min   INTEGER NOT NULL DEFAULT 60,
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys (user_id, status);

CREATE TABLE IF NOT EXISTS webhooks (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    url           VARCHAR(512) NOT NULL,
    secret        VARCHAR(128) NOT NULL,
    events        JSONB NOT NULL DEFAULT '["TASK_COMPLETED"]',
    status        VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);