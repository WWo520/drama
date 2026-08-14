-- 000003_users_spaces.up.sql
-- 用户与空间协作层：users / spaces / roles / user_roles / space_members / space_invitations
-- 对齐 02 - 数据库设计.md B1 章。users.default_space_id 与 spaces.owner_user_id 存在循环引用，
-- 先建表后 ALTER 补外键。全部业务表外键 ON DELETE RESTRICT（B.0.1 铁律）。

-- ============ users ============
CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  user_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  default_space_id BIGINT NULL,
  email VARCHAR(128) NULL,
  phone VARCHAR(20) NULL,
  password_hash VARCHAR(255) NOT NULL,
  nickname VARCHAR(64) NOT NULL,
  avatar_url VARCHAR(512) NULL,
  bio VARCHAR(512) NULL,
  status user_status_enum NOT NULL DEFAULT 'ACTIVE',
  vip_level SMALLINT NOT NULL DEFAULT 0,
  vip_expire_at TIMESTAMPTZ NULL,
  language VARCHAR(8) NOT NULL DEFAULT 'zh-CN',
  aigc_watermark BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_users_uuid ON users(user_uuid) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL AND email IS NOT NULL;
CREATE UNIQUE INDEX idx_users_phone ON users(phone) WHERE deleted_at IS NULL AND phone IS NOT NULL;
CREATE INDEX idx_users_status ON users(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_vip_level ON users(vip_level) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_vip_expire_at ON users(vip_expire_at) WHERE deleted_at IS NULL AND vip_expire_at IS NOT NULL;
CREATE INDEX idx_users_created_at ON users(created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_default_space ON users(default_space_id) WHERE deleted_at IS NULL AND default_space_id IS NOT NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ spaces ============
CREATE TABLE spaces (
  id BIGSERIAL PRIMARY KEY,
  space_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
  name VARCHAR(128) NOT NULL,
  avatar_url VARCHAR(512) NULL,
  description VARCHAR(512) NULL,
  type space_type_enum NOT NULL DEFAULT 'PERSONAL',
  owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  quota_storage_bytes BIGINT NOT NULL DEFAULT 10737418240,
  quota_member_count INT NOT NULL DEFAULT 1,
  quota_compute_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  status space_status_enum NOT NULL DEFAULT 'ACTIVE',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_spaces_uuid ON spaces(space_uuid) WHERE deleted_at IS NULL;
CREATE INDEX idx_spaces_owner_user_id ON spaces(owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_spaces_type_status ON spaces(type, status) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_spaces_default_user_unique ON spaces(owner_user_id)
  WHERE deleted_at IS NULL AND is_default = TRUE;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON spaces FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- users.default_space_id 补外键（循环依赖）
ALTER TABLE users ADD CONSTRAINT fk_users_default_space
  FOREIGN KEY (default_space_id) REFERENCES spaces(id) ON DELETE RESTRICT;

-- ============ roles（平台 RBAC 小表） ============
CREATE TABLE roles (
  id SERIAL PRIMARY KEY,
  code platform_role_code_enum NOT NULL UNIQUE,
  name VARCHAR(64) NOT NULL,
  permissions JSONB NOT NULL DEFAULT '{}'::jsonb,
  description VARCHAR(256) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_roles_permissions ON roles USING GIN(permissions);
CREATE TRIGGER set_timestamp BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ user_roles ============
CREATE TABLE user_roles (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  role_id INT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
  granted_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_user_roles_unique ON user_roles(user_id, role_id) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON user_roles FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ space_members ============
CREATE TABLE space_members (
  id BIGSERIAL PRIMARY KEY,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  role user_role_enum NOT NULL DEFAULT 'MEMBER',
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  invited_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_space_members_unique ON space_members(space_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_space_members_user_id ON space_members(user_id) WHERE deleted_at IS NULL;
CREATE TRIGGER set_timestamp BEFORE UPDATE ON space_members FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ============ space_invitations ============
CREATE TABLE space_invitations (
  id BIGSERIAL PRIMARY KEY,
  space_id BIGINT NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
  inviter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  invitee_account VARCHAR(128) NOT NULL,
  invite_token VARCHAR(64) NOT NULL,
  role user_role_enum NOT NULL DEFAULT 'MEMBER',
  status invitation_status_enum NOT NULL DEFAULT 'PENDING',
  expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
  accepted_at TIMESTAMPTZ NULL,
  accepted_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_space_invitations_token ON space_invitations(invite_token) WHERE deleted_at IS NULL;
CREATE INDEX idx_space_invitations_space_status ON space_invitations(space_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_space_invitations_expires_at ON space_invitations(expires_at)
  WHERE deleted_at IS NULL AND status = 'PENDING';
CREATE TRIGGER set_timestamp BEFORE UPDATE ON space_invitations FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();
