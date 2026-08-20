-- 000024_platform_feature_defaults.up.sql
-- 功能开关默认值：管理端（平台配置→基础设置→功能开关）保存前即有真实记录可读；
-- feature_* 由业务逻辑消费（feature_register 注册拦截 / feature_maintenance 维护模式中间件）。
-- 已存在记录（历史保存过）只保证 is_public=TRUE，不覆盖用户已改的 config_value。
UPDATE platform_configs SET is_public=TRUE
WHERE config_key IN ('feature_register', 'feature_audit', 'feature_comment', 'feature_feed', 'feature_invite_only', 'feature_maintenance');

INSERT INTO platform_configs (config_key, config_value, config_group, value_type, description, is_public)
VALUES
    ('feature_register',    'true',  'feature', 'boolean', '用户注册开关', TRUE),
    ('feature_audit',       'true',  'feature', 'boolean', '内容审核开关', TRUE),
    ('feature_comment',     'true',  'feature', 'boolean', '评论功能开关', TRUE),
    ('feature_feed',        'true',  'feature', 'boolean', '社区发现页开关', TRUE),
    ('feature_invite_only', 'false', 'feature', 'boolean', '邀请注册开关', TRUE),
    ('feature_maintenance', 'false', 'feature', 'boolean', '维护模式开关', TRUE)
ON CONFLICT (config_key) DO NOTHING;
