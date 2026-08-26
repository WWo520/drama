-- UP: asset_kind_enum 补充 'CHARACTER'（资产建链/镜头参考图加载均以 'CHARACTER' 表达角色逻辑类型；
-- 'CHARACTER_VARIANT' 用于角色图生成任务 target_kind，两者并存）
ALTER TYPE asset_kind_enum ADD VALUE IF NOT EXISTS 'CHARACTER';
