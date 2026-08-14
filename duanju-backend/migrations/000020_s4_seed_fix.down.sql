-- 回滚（000020）
DELETE FROM models WHERE model_key IN ('seedance-2.0', 'kling-1.5', 'qwen-turbo');
DELETE FROM packages WHERE package_code IN ('PACK_FREE', 'PACK_BASIC', 'PACK_PRO', 'PACK_ENTERPRISE');
DELETE FROM roles WHERE code IN ('SUPER_ADMIN', 'OPERATOR', 'AUDITOR');
DROP INDEX IF EXISTS idx_roles_code;
DROP INDEX IF EXISTS idx_models_key;