-- 阶段 4 修复：运营三表（000019 从未成功，重建）
CREATE TABLE IF NOT EXISTS announcements (
    id         BIGSERIAL PRIMARY KEY,
    title      VARCHAR(128) NOT NULL,
    content    TEXT NOT NULL,
    start_at   TIMESTAMPTZ NOT NULL,
    end_at     TIMESTAMPTZ NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS banners (
    id         BIGSERIAL PRIMARY KEY,
    title      VARCHAR(64) NOT NULL,
    image_url  TEXT,
    jump_url   TEXT,
    position   VARCHAR(32),
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hints (
    id          BIGSERIAL PRIMARY KEY,
    category    VARCHAR(32),
    title       VARCHAR(64) NOT NULL,
    description TEXT NOT NULL,
    step        INTEGER NOT NULL DEFAULT 0,
    position    INTEGER NOT NULL DEFAULT 0
);