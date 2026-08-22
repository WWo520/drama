-- UP: 对齐粒度铁律（docs/10 - 剧本skills 01/02 调查报告 P0）：
-- SPLIT_STANDARD 模板原"每集由 3~6 个叙事单元构成"为固定数量约束，
-- 与基础提示词的动态下限（ceil(集时长/25)）及单元 ≤30s 铁律冲突，更新为粒度底座描述。
UPDATE agent_skills
SET template_content = '均衡节奏：每集由多个叙事单元构成（单元数不少于按目标时长折算的下限，单个单元不超过 30 秒），每个单元承担明确叙事目标（铺垫/冲突/转折/钩子），集与集之间结尾保留适度悬念，适配通用短剧题材。',
    updated_at = now()
WHERE skill_code = 'SPLIT_STANDARD' AND deleted_at IS NULL;
