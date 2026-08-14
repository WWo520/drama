-- 阶段 2 M1 扩表（02 D.2）：orders 团队充值 + 画布 5 张 + 月卡会员 + 套餐
-- S2-B03 / S2-B07 / S2-B09

-- orders：团队充值订单（空间账户）
ALTER TABLE orders ADD COLUMN IF NOT EXISTS space_id BIGINT;

-- 画布项目（B5.1）
CREATE TABLE IF NOT EXISTS canvas_projects (
    id           BIGSERIAL PRIMARY KEY,
    canvas_uuid  UUID NOT NULL DEFAULT gen_random_uuid(),
    space_id     BIGINT NOT NULL,
    project_id   BIGINT,               -- 关联创作项目（可空：独立画布）
    name         VARCHAR(128) NOT NULL,
    data         JSONB NOT NULL DEFAULT '{}',
    version      INTEGER NOT NULL DEFAULT 1,
    created_by   BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_canvas_projects_uuid ON canvas_projects (canvas_uuid) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_canvas_projects_space ON canvas_projects (space_id) WHERE deleted_at IS NULL;

-- 画布节点（B5.2）
CREATE TABLE IF NOT EXISTS canvas_nodes (
    id          BIGSERIAL PRIMARY KEY,
    canvas_id   BIGINT NOT NULL REFERENCES canvas_projects (id),
    node_uuid   UUID NOT NULL DEFAULT gen_random_uuid(),
    node_type   VARCHAR(64) NOT NULL,   -- 7 类节点：INPUT/SCRIPT_SPLIT/ASSET_GEN/VIDEO_GEN/EXPORT/OUTPUT/LLM
    position_x  DOUBLE PRECISION NOT NULL DEFAULT 0,
    position_y  DOUBLE PRECISION NOT NULL DEFAULT 0,
    config      JSONB NOT NULL DEFAULT '{}',
    status      VARCHAR(32) NOT NULL DEFAULT 'READY',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_canvas_nodes_canvas ON canvas_nodes (canvas_id) WHERE deleted_at IS NULL;

-- 画布边（B5.3）
CREATE TABLE IF NOT EXISTS canvas_edges (
    id             BIGSERIAL PRIMARY KEY,
    canvas_id      BIGINT NOT NULL REFERENCES canvas_projects (id),
    edge_uuid      UUID NOT NULL DEFAULT gen_random_uuid(),
    source_node_id BIGINT NOT NULL,
    target_node_id BIGINT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_canvas_edges_canvas ON canvas_edges (canvas_id) WHERE deleted_at IS NULL;

-- 画布模板（B5.4）
CREATE TABLE IF NOT EXISTS canvas_templates (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    data        JSONB NOT NULL DEFAULT '{}',
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

-- 画布自动保存快照（B5.5）
CREATE TABLE IF NOT EXISTS canvas_auto_save_snapshots (
    id          BIGSERIAL PRIMARY KEY,
    canvas_id   BIGINT NOT NULL REFERENCES canvas_projects (id),
    data        JSONB NOT NULL DEFAULT '{}',
    version     INTEGER NOT NULL DEFAULT 1,
    created_by  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_canvas_snapshots_canvas ON canvas_auto_save_snapshots (canvas_id, created_at DESC);

-- 月卡会员（B8.5）
CREATE TABLE IF NOT EXISTS user_memberships (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    plan_code   VARCHAR(32) NOT NULL,   -- BASIC/PRO/ENTERPRISE/ULTIMATE
    status      VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_memberships_user ON user_memberships (user_id, status);

-- 积分包/套餐（B8.3）
CREATE TABLE IF NOT EXISTS packages (
    id               BIGSERIAL PRIMARY KEY,
    package_code     VARCHAR(64) NOT NULL UNIQUE,
    name             VARCHAR(128) NOT NULL,
    package_type     VARCHAR(32) NOT NULL,  -- POINTS / MEMBERSHIP
    price_yuan       NUMERIC(10,2) NOT NULL,
    points_granted   INTEGER NOT NULL DEFAULT 0,
    subscription_days INTEGER NOT NULL DEFAULT 0,
    plan_code        VARCHAR(32),
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
