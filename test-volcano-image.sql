-- 创建资产生成任务测试火山 API
-- 先查询相关信息

SELECT 
    c.id as character_id,
    c.name as character_name,
    c.edited_prompt as prompt,
    e.episode_uuid,
    e.id as episode_id
FROM characters c
JOIN episodes e ON c.origin_episode_id = e.id
WHERE e.project_id = 28
ORDER BY c.id
LIMIT 1;
