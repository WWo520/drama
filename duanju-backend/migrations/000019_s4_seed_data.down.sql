-- 回滚（000019）
DROP TABLE IF EXISTS hints;
DROP TABLE IF EXISTS banners;
DROP TABLE IF EXISTS announcements;
DELETE FROM models WHERE model_key IN ('seedance-2.0', 'kling-1.5', 'qwen-turbo');
DELETE FROM packages WHERE package_code IN ('PACK_FREE', 'PACK_BASIC', 'PACK_PRO', 'PACK_ENTERPRISE');
DELETE FROM roles WHERE code IN ('ADMIN', 'EDITOR', 'VIEWER');