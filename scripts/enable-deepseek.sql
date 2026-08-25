-- 快速启用所有 DeepSeek 模型
UPDATE models 
SET enabled = TRUE 
WHERE model_key LIKE '%deepseek%';

-- 查看启用后的结果
SELECT DISTINCT mp.name, m.model_key, m.enabled
FROM model_keys mk
JOIN model_providers mp ON mp.id = mk.provider_id
JOIN models m ON m.provider_id = mp.id
WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%';
