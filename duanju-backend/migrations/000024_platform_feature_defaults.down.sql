-- 000024_platform_feature_defaults.down.sql
DELETE FROM platform_configs WHERE config_key IN
    ('feature_register', 'feature_audit', 'feature_comment', 'feature_feed', 'feature_invite_only', 'feature_maintenance');
