-- 阶段 5 补齐：接口覆盖率 89 个未实现端点所需的缺失表（admin 运营模块）。
-- 已有表不重复建；本迁移只建 coverage 清单中无对应表的实体。

-- ============ marketing_activities（11.79 活动管理） ============
CREATE TABLE IF NOT EXISTS marketing_activities (
  id              BIGSERIAL PRIMARY KEY,
  activity_code   VARCHAR(64) NOT NULL UNIQUE,  -- signup_bonus / first_purchase / festival...
  name            VARCHAR(128) NOT NULL,
  activity_type   VARCHAR(32) NOT NULL DEFAULT 'POINTS',  -- POINTS|COUPON|RANKING
  reward_points   INT NOT NULL DEFAULT 0,
  start_at        TIMESTAMPTZ,
  end_at          TIMESTAMPTZ,
  status          VARCHAR(16) NOT NULL DEFAULT 'DRAFT',  -- DRAFT|ONLINE|OFFLINE
  description     TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ agent_memory（11.61 智能体记忆管理） ============
CREATE TABLE IF NOT EXISTS agent_memory (
  id              BIGSERIAL PRIMARY KEY,
  user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE,
  project_id      BIGINT REFERENCES projects(id) ON DELETE CASCADE,
  memory_type     VARCHAR(32) NOT NULL DEFAULT 'SESSION',  -- SESSION|SUMMARY|KNOWLEDGE
  content         TEXT NOT NULL,
  token_count     INT NOT NULL DEFAULT 0,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_user ON agent_memory(user_id, created_at DESC);

-- ============ categories（11.18 分类/标签列表） ============
CREATE TABLE IF NOT EXISTS categories (
  id              BIGSERIAL PRIMARY KEY,
  code            VARCHAR(64) NOT NULL UNIQUE,
  name            VARCHAR(128) NOT NULL,
  parent_id       BIGINT REFERENCES categories(id) ON DELETE SET NULL,
  sort_order      INT NOT NULL DEFAULT 0,
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ courses（11.87 课程管理） ============
CREATE TABLE IF NOT EXISTS courses (
  id              BIGSERIAL PRIMARY KEY,
  course_uuid     UUID NOT NULL DEFAULT gen_random_uuid(),
  title           VARCHAR(128) NOT NULL,
  course_type     VARCHAR(32) NOT NULL DEFAULT 'BEGINNER',  -- BEGINNER|ADVANCED
  cover_url       TEXT,
  video_url       TEXT,
  sort_order      INT NOT NULL DEFAULT 0,
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ directive_skills（11.57 导演叙事技能库管理） ============
CREATE TABLE IF NOT EXISTS directive_skills (
  id              BIGSERIAL PRIMARY KEY,
  skill_code      VARCHAR(64) NOT NULL UNIQUE,
  name            VARCHAR(128) NOT NULL,
  description     TEXT,
  prompt_template TEXT,
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ languages（11.69 多语言配置） ============
CREATE TABLE IF NOT EXISTS languages (
  id              BIGSERIAL PRIMARY KEY,
  lang_code       VARCHAR(16) NOT NULL UNIQUE,  -- zh-CN / en-US / ja-JP
  name            VARCHAR(64) NOT NULL,
  is_default      BOOLEAN NOT NULL DEFAULT FALSE,
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO languages (lang_code, name, is_default) VALUES ('zh-CN', '简体中文', TRUE)
  ON CONFLICT (lang_code) DO NOTHING;

-- ============ task_queues（11.73 任务并发/队列管理） ============
CREATE TABLE IF NOT EXISTS task_queues (
  id              BIGSERIAL PRIMARY KEY,
  queue_name      VARCHAR(64) NOT NULL UNIQUE,  -- split/asset/video/export
  queue_length    INT NOT NULL DEFAULT 0,
  consumer_count  INT NOT NULL DEFAULT 0,
  consume_rate_per_min INT NOT NULL DEFAULT 0,
  dead_letter_count INT NOT NULL DEFAULT 0,
  status          VARCHAR(16) NOT NULL DEFAULT 'RUNNING',
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ rate_limits（11.47 接口限流配置列表） ============
CREATE TABLE IF NOT EXISTS rate_limits (
  id              BIGSERIAL PRIMARY KEY,
  route_pattern   VARCHAR(128) NOT NULL UNIQUE,  -- /api/v1/orders
  limit_per_min   INT NOT NULL DEFAULT 60,
  burst           INT NOT NULL DEFAULT 10,
  scope           VARCHAR(16) NOT NULL DEFAULT 'IP',  -- IP|USER|API_KEY
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============ themes（11.65 界面主题配置） ============
CREATE TABLE IF NOT EXISTS themes (
  id              BIGSERIAL PRIMARY KEY,
  theme_code      VARCHAR(32) NOT NULL UNIQUE,
  name            VARCHAR(64) NOT NULL,
  primary_color   VARCHAR(16) NOT NULL DEFAULT '#0B0B1A',
  accent_color    VARCHAR(16) NOT NULL DEFAULT '#F5B23C',
  is_default      BOOLEAN NOT NULL DEFAULT FALSE,
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO themes (theme_code, name, primary_color, accent_color, is_default)
  VALUES ('dark_cinema', '暗色电影', '#0B0B1A', '#F5B23C', TRUE)
  ON CONFLICT (theme_code) DO NOTHING;

-- ============ think_levels（11.63 思考等级配置） ============
CREATE TABLE IF NOT EXISTS think_levels (
  id              BIGSERIAL PRIMARY KEY,
  level_code      VARCHAR(32) NOT NULL UNIQUE,  -- FAST / BALANCED / DEEP
  name            VARCHAR(64) NOT NULL,
  max_tokens      INT NOT NULL DEFAULT 2048,
  temperature     NUMERIC(3,2) NOT NULL DEFAULT 0.30,
  description     TEXT,
  status          VARCHAR(16) NOT NULL DEFAULT 'ON',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO think_levels (level_code, name, max_tokens, temperature, description) VALUES
  ('FAST', '快速模式', 2048, 0.30, '低延迟，适合批量初稿'),
  ('BALANCED', '均衡模式', 4096, 0.50, '默认模式，质量与速度平衡'),
  ('DEEP', '深度模式', 8192, 0.70, '高思考深度，适合精修')
  ON CONFLICT (level_code) DO NOTHING;
