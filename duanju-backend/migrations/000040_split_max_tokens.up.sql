-- 000040_split_max_tokens.up.sql
-- 剧本拆分单次 LLM 输出上限（max_tokens）改为管理端可配置：
-- 平台配置 → 基础设置 → AI 限制；worker 执行 SCRIPT_SPLIT 时实时读取（platformcfg 30s 缓存）。
-- 未配置/读取失败回退代码默认 32768。
INSERT INTO platform_configs (config_key, config_value, config_group, value_type, description, is_public)
VALUES ('split_max_tokens', '32768', 'ai_limits', 'number', '剧本拆分单次输出上限(max_tokens)，样例：8192/16384/32768/65536', FALSE)
ON CONFLICT (config_key) DO NOTHING;
