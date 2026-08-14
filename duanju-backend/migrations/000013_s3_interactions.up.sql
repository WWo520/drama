-- 阶段 3 M2 扩表（S3-B02/B03）：互动 5 张表（comments/likes/favorites/follows/reports）
CREATE TABLE IF NOT EXISTS comments (
    id               BIGSERIAL PRIMARY KEY,
    comment_uuid     UUID NOT NULL DEFAULT gen_random_uuid(),
    work_id          BIGINT NOT NULL,
    user_id          BIGINT NOT NULL,
    parent_comment_id BIGINT,
    content          VARCHAR(1000) NOT NULL,
    status           VARCHAR(32) NOT NULL DEFAULT 'VISIBLE', -- VISIBLE / HIDDEN
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_comments_work ON comments (work_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_comments_user ON comments (user_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS likes (
    id          BIGSERIAL PRIMARY KEY,
    work_id     BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (work_id, user_id)
);

CREATE TABLE IF NOT EXISTS favorites (
    id          BIGSERIAL PRIMARY KEY,
    work_id     BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (work_id, user_id)
);

CREATE TABLE IF NOT EXISTS follows (
    id                BIGSERIAL PRIMARY KEY,
    follower_user_id  BIGINT NOT NULL,
    followee_user_id  BIGINT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (follower_user_id, followee_user_id)
);

CREATE TABLE IF NOT EXISTS reports (
    id              BIGSERIAL PRIMARY KEY,
    work_id         BIGINT NOT NULL,
    reporter_user_id BIGINT NOT NULL,
    reason          VARCHAR(256) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'PENDING', -- PENDING / PROCESSED / DISMISSED
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reports_work ON reports (work_id, status);