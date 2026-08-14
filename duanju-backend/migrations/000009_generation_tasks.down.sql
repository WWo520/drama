-- 000009_generation_tasks.down.sql
DROP TABLE IF EXISTS generation_task_retry_logs CASCADE;
DROP TABLE IF EXISTS model_callback_logs CASCADE;
DROP TABLE IF EXISTS task_dispatch_outbox CASCADE;
DROP TABLE IF EXISTS generation_tasks CASCADE;
