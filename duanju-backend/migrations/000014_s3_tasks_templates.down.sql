-- 回滚（000014）
DELETE FROM style_library WHERE style_code LIKE 'gl_%';
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS task_intents;
DROP TABLE IF EXISTS task_orders;