-- 000011_export_notify_works.down.sql
DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS publish_records CASCADE;
DROP TABLE IF EXISTS works CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS export_records CASCADE;
-- 种子数据随表删除（down 仅用于空库回退）
