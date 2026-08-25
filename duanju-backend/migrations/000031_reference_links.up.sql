-- UP: 参考图机制（F10）——叙事单元↔资产链接 + 镜头级参考图覆盖
-- 设计：nu_asset_links 建立叙事单元与资产的多对多关联；shot_asset_refs 支持镜头级覆盖（默认继承 nu 级）

-- 叙事单元 ↔ 资产链接（U5 寻址基础）
CREATE TABLE IF NOT EXISTS nu_asset_links (
  id BIGSERIAL PRIMARY KEY,
  narrative_unit_id BIGINT NOT NULL REFERENCES narrative_units(id) ON DELETE CASCADE,
  asset_kind asset_kind_enum NOT NULL,          -- CHARACTER/SCENE/PROP
  asset_id BIGINT NOT NULL,                      -- characters/scenes/props.id
  variant_id BIGINT NULL,                        -- 指定形象，NULL=默认形象
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (narrative_unit_id, asset_kind, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_nu_asset_links_nu ON nu_asset_links (narrative_unit_id);
CREATE INDEX IF NOT EXISTS idx_nu_asset_links_asset ON nu_asset_links (asset_kind, asset_id);

-- 镜头级参考图覆盖（F10；默认继承 nu 级，可镜头级覆盖）
CREATE TABLE IF NOT EXISTS shot_asset_refs (
  id BIGSERIAL PRIMARY KEY,
  shot_id BIGINT NOT NULL REFERENCES shots(id) ON DELETE CASCADE,
  asset_kind asset_kind_enum NOT NULL,
  asset_id BIGINT NOT NULL,
  variant_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (shot_id, asset_kind, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_shot_asset_refs_shot ON shot_asset_refs (shot_id);
CREATE INDEX IF NOT EXISTS idx_shot_asset_refs_asset ON shot_asset_refs (asset_kind, asset_id);
