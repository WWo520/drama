-- DOWN: 回滚 SPLIT_STANDARD 模板文案。
UPDATE agent_skills
SET template_content = '均衡节奏：每集由 3~6 个叙事单元构成，每个单元承担明确叙事目标（铺垫/冲突/转折/钩子），集与集之间结尾保留适度悬念，适配通用短剧题材。',
    updated_at = now()
WHERE skill_code = 'SPLIT_STANDARD' AND deleted_at IS NULL;
