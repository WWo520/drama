-- 阶段 4 修复：models.model_key / roles.code 唯一索引（M0 缺失）+ seed
DROP INDEX IF EXISTS idx_models_key;
CREATE UNIQUE INDEX idx_models_key ON models (model_key);
DROP INDEX IF EXISTS idx_roles_code;
CREATE UNIQUE INDEX idx_roles_code ON roles (code);

INSERT INTO roles (code, name, permissions, description)
VALUES ('SUPER_ADMIN', '超级管理员', '["dashboard","users","orders","packages","models","platform-config","audit-logs"]'::jsonb, '拥有全部权限'),
       ('OPERATOR', '编辑', '["projects","episodes","beats","assets","canvases","exports"]'::jsonb, '可创建和编辑内容'),
       ('AUDITOR', '访客', '[]'::jsonb, '只读权限')
ON CONFLICT (code) DO NOTHING;

INSERT INTO packages (package_code, name, billing_cycle, price_yuan, points_quota, status, is_recommended, sort_order)
VALUES ('PACK_FREE', '免费版', '', '0.00', 50, 'ACTIVE', FALSE, 1),
       ('PACK_BASIC', '基础会员', 'MONTHLY', '9.90', 500, 'ACTIVE', TRUE, 2),
       ('PACK_PRO', '专业会员', 'MONTHLY', '29.90', 2000, 'ACTIVE', TRUE, 3),
       ('PACK_ENTERPRISE', '企业版', 'ANNUAL', '999.00', 20000, 'ACTIVE', FALSE, 4)
ON CONFLICT (package_code) DO NOTHING;

-- 模型供应商（models.provider_id NOT NULL 依赖）
INSERT INTO model_providers (name, display_name, base_url, enabled, sort_order)
SELECT 'mock-provider', 'Mock 供应商', 'https://mock.local/v1', TRUE, 1
WHERE NOT EXISTS (SELECT 1 FROM model_providers WHERE name='mock-provider');

INSERT INTO models (model_key, name, capability, cost_points_per_call, enabled, sort_order, provider_id)
VALUES ('seedance-2.0', 'Seedance 2.0', 'T2V', 10, TRUE, 1, (SELECT id FROM model_providers WHERE name='mock-provider')),
       ('kling-1.5', 'Kling 1.5', 'T2V', 20, TRUE, 2, (SELECT id FROM model_providers WHERE name='mock-provider')),
       ('qwen-turbo', 'Qwen Turbo', 'LLM', 5, TRUE, 3, (SELECT id FROM model_providers WHERE name='mock-provider'))
ON CONFLICT (model_key) DO NOTHING;