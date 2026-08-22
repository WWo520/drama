-- UP: agent_skill_kind_enum 扩展 SCRIPT_SPLIT（剧本拆分技能）。
-- 注意：PG 不允许在新增枚举值的同一事务内引用该值，故种子数据独立放在 000028。
ALTER TYPE agent_skill_kind_enum ADD VALUE IF NOT EXISTS 'SCRIPT_SPLIT';
