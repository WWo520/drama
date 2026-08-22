-- DOWN: 软删剧本拆分技能种子（与项目软删惯例一致）。
UPDATE agent_skills SET deleted_at = NOW() WHERE kind = 'SCRIPT_SPLIT' AND deleted_at IS NULL;
