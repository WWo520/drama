-- 阶段 5 对齐修复（对齐审查 F01/F03：多变体 + 跨集复用字段）。
-- 注意：本迁移只做安全的 ADD COLUMN IF NOT EXISTS；
-- 版本管理已由 narrative_unit_versions 表承载（无需 storyboard_versions 重复建设）；
-- 风格库已由 style_library 表承载（000014 已初始化 27 项，无需 user_styles 重复建设）；
-- 画布模板已由 canvas_templates 表承载（000012 已建，无需重建）；
-- shot_groups 已有 92 行真实数据（000006 建表），严禁 DROP 重建。

-- ============ assets 表扩展字段（F01 & F03） ============

-- 新增变体名称和外观描述（F01 - 多变体系统）
ALTER TABLE assets ADD COLUMN IF NOT EXISTS variant_name VARCHAR(64);
ALTER TABLE assets ADD COLUMN IF NOT EXISTS appearance_desc TEXT;

-- 新增跨集复用引用数组（F03 - 跨集资产复用不重复扣费）
-- JSONB 类型存储 episode_uuid 数组：["episode_uuid_1","episode_uuid_2",...]
ALTER TABLE assets ADD COLUMN IF NOT EXISTS multi_episode_refs JSONB DEFAULT '[]'::jsonb;

-- 添加最近一次生成时间（用于判断是否需要重生）
ALTER TABLE assets ADD COLUMN IF NOT EXISTS last_generated_at TIMESTAMPTZ;

-- 索引：基于 variant_name 的查询优化
CREATE INDEX IF NOT EXISTS idx_assets_variant_name ON assets(variant_name) WHERE variant_name IS NOT NULL;

-- 索引：基于 multi_episode_refs 的跨集查询
CREATE INDEX IF NOT EXISTS idx_assets_multi_episode_refs ON assets USING GIN(multi_episode_refs)
  WHERE jsonb_array_length(multi_episode_refs) > 0;

-- 填充已有资产的变体名（按 type 枚举兜底：CHARACTER/SCENE/PROP/SHOT/MATERIAL）
UPDATE assets SET variant_name = CASE type
  WHEN 'CHARACTER' THEN '默认造型'
  WHEN 'SCENE' THEN '日景'
  WHEN 'PROP' THEN '标准'
  ELSE '默认'
END WHERE variant_name IS NULL AND type IS NOT NULL;

COMMENT ON COLUMN assets.variant_name IS '资产变体名（如干练老板娘装/日景）';
COMMENT ON COLUMN assets.multi_episode_refs IS '跨集引用数组（[episode_uuid,...]）';
