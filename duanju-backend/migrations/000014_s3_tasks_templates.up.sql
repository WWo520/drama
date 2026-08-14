-- 阶段 3 M3 扩表（S3-B04/B05/B06）：task_orders + task_intents + templates + 27 风格 seed
CREATE TABLE IF NOT EXISTS task_orders (
    id               BIGSERIAL PRIMARY KEY,
    order_uuid       UUID NOT NULL DEFAULT gen_random_uuid(),
    order_no         VARCHAR(64) NOT NULL UNIQUE,
    space_id         BIGINT NOT NULL,
    publisher_user_id BIGINT NOT NULL,
    title            VARCHAR(200) NOT NULL,
    description      TEXT,
    requirement_json JSONB NOT NULL DEFAULT '{}',
    reward_points    INTEGER NOT NULL DEFAULT 0,
    status           VARCHAR(32) NOT NULL DEFAULT 'OPEN',  -- OPEN/ACCEPTED/IN_PROGRESS/SUBMITTED/COMPLETED/CANCELLED
    assignee_user_id BIGINT,
    deadline_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_task_orders_status ON task_orders (status, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS task_intents (
    id          BIGSERIAL PRIMARY KEY,
    order_id    BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    cover_letter VARCHAR(500),
    status      VARCHAR(32) NOT NULL DEFAULT 'PENDING',  -- PENDING/ACCEPTED/REJECTED/WITHDRAWN
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (order_id, user_id)
);

-- 模板集市（S3-B04）
CREATE TABLE IF NOT EXISTS templates (
    id           BIGSERIAL PRIMARY KEY,
    template_uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    name         VARCHAR(128) NOT NULL,
    description  TEXT,
    data         JSONB NOT NULL DEFAULT '{}',
    cover_url    VARCHAR(512),
    is_system    BOOLEAN NOT NULL DEFAULT FALSE,
    price_points INTEGER NOT NULL DEFAULT 0,
    used_count   INTEGER NOT NULL DEFAULT 0,
    created_by   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_templates_system ON templates (is_system, used_count DESC) WHERE deleted_at IS NULL;

-- 27 风格初始化（S3-B06，02 B9.10 E7）：若 style_library 为空则 seed
INSERT INTO style_library (style_code, scope, library_layer, category, name, prompt_suffix_static, is_default, sort_order, enabled)
SELECT s.style_code, s.scope::template_scope_enum, s.library_layer::style_layer_enum, s.category::style_category_enum, s.name, s.prompt_suffix_static, s.is_default, s.sort_order, TRUE
FROM (VALUES
    ('gl_neo_noir','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GENERAL_REALISTIC','赛博黑金','neo-noir, high contrast, gold accents on black, cinematic',
     TRUE, 1),
    ('gl_cinematic','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GENERAL_REALISTIC','电影质感','cinematic lighting, shallow depth of field, anamorphic, film grain',
     FALSE, 2),
    ('gl_documentary','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GENERAL_REALISTIC','纪录片','natural light, handheld, realistic textures, neutral tones',
     FALSE, 3),
    ('gl_fantasy_light','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','ROMANCE_IDYLLIC_MIYAZAKI','奇幻暖光','warm golden hour light, magical atmosphere, soft glow',
     FALSE, 4),
    ('gl_tech_dark','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','CYBERPUNK_NEO_NOIR','暗黑科技','dark sci-fi, neon reflections, metallic surfaces, moody',
     FALSE, 5),
    ('gl_retro_8090','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','MODERN_URBAN','8090复古','vintage 80s/90s aesthetic, VHS grain, warm faded colors',
     FALSE, 6),
    ('gl_sweet_daily','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','ROMANCE_IDYLLIC_MIYAZAKI','甜宠日常','bright soft light, pastel colors, cozy daily life, bokeh',
     FALSE, 7),
    ('gl_ancient_drama','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','HISTORICAL_COSTUME_SHENGUO_FENG','古装大片','ancient costume drama, grand set, silk textures, epic scale',
     FALSE, 8),
    ('gl_horror_dark','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','HORROR_SUSPENSE_JAPANESE','暗黑悬疑','low key lighting, cold blue tones, suspense, ominous shadows',
     FALSE, 9),
    ('gl_docu_grey','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GENERAL_REALISTIC','纪实灰调','muted grey palette, documentary realism, grainy texture',
     FALSE, 10),
    ('gl_anime_2d','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','COMIC_CARTOON','二次元2D','2D anime style, cel shading, vibrant colors, clean lines',
     FALSE, 11),
    ('gl_pixel_retro','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','COMIC_CARTOON','像素复古','pixel art style, 8-bit palette, retro game aesthetic',
     FALSE, 12),
    ('gl_paper_ink','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GUOFENG_CLASSIC','水墨国风','chinese ink wash painting, paper texture, minimal brush strokes',
     FALSE, 13),
    ('gl_glitch_art','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','CYBERPUNK_NEO_NOIR','故障艺术','glitch effects, chromatic aberration, digital distortion',
     FALSE, 14),
    ('gl_minimal_white','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','MODERN_URBAN','极简白','minimalist white space, clean composition, soft shadows',
     FALSE, 15),
    ('gl_oil_painting','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GENERAL_REALISTIC','油画质感','oil painting texture, rich brush strokes, classical art',
     FALSE, 16),
    ('gl_watercolor','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','GUOFENG_CLASSIC','水彩','watercolor wash, translucent layers, delicate paper grain',
     FALSE, 17),
    ('gl_hand_drawn','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','COMIC_CARTOON','手绘风','hand drawn illustration, sketch lines, warm paper tone',
     FALSE, 18),
    ('gl_cyberpunk','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','CYBERPUNK_NEO_NOIR','赛博朋克','cyberpunk, neon city, rain reflections, purple pink glow',
     FALSE, 19),
    ('gl_film_noir','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','SUSPENSE_THRILLER_HITCHCOCK','黑白电影','classic film noir, black and white, hard shadows, chiaroscuro',
     FALSE, 20),
    ('gl_soft_romance','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','ROMANCE_IDYLLIC_MIYAZAKI','柔光浪漫','soft romantic light, dreamy bokeh, warm pink tones',
     FALSE, 21),
    ('gl_sports_dynamic','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','YOUTHS_CAMPUS','运动动感','dynamic motion, fast shutter, energetic colors, action',
     FALSE, 22),
    ('gl_space_epic','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','WAR_PERIOD_SOVIET','太空史诗','space epic, stars, nebulae, vast scale, cosmic light',
     FALSE, 23),
    ('gl_post_apoc','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','WAR_PERIOD_SOVIET','末日废土','post apocalyptic, dusty wasteland, desaturated, rugged',
     FALSE, 24),
    ('gl_fairy_tale','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','COMIC_CARTOON','童话绘本','fairy tale storybook, whimsical, storybook illustration',
     FALSE, 25),
    ('gl_silhouette','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','SUSPENSE_THRILLER_HITCHCOCK','剪影意境','silhouette composition, dramatic backlight, poetic minimal',
     FALSE, 26),
    ('gl_editorial','PUBLIC_OFFICIAL','OFFICIAL_PUBLIC_LIBRARY','MODERN_URBAN','杂志大片','editorial fashion, magazine cover, bold typography feel',
     FALSE, 27)
) AS s(style_code, scope, library_layer, category, name, prompt_suffix_static, is_default, sort_order)
WHERE NOT EXISTS (SELECT 1 FROM style_library);