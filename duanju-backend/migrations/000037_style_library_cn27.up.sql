-- 000037 风格库统一为 27 项中文风格（对齐 asset_handler.stylePromptSuffix / 资产弹窗 STYLES）。
-- 动机：原 seed（gl_neo_noir 赛博黑金等英文后缀）与资产生成弹窗的 27 项中文风格体系不一致，
-- 用户在首页选的风格与资产生成实际注入的风格对不上。统一后：
--   首页风格库选「国风3D」→ ApplyStyle 写入项目 config.prompt_suffix（中文后缀）
--   → 与资产弹窗 stylePromptSuffix 同一套提示词。
-- thumbnail_url 留空，由 cmd/genstylethumbs 工具用火山 Seedream 生成预览图后回填。

-- 1) 软删旧 27 项（保留审计痕迹，风格 code 体系整体换代）
UPDATE style_library SET deleted_at = NOW(), enabled = FALSE WHERE deleted_at IS NULL;

-- 2) 插入 27 项中文风格（name + prompt_suffix_static 与 asset_handler.go stylePromptSuffix 一致）
INSERT INTO style_library (style_code, scope, library_layer, category, name, prompt_suffix_static, is_default, sort_order, enabled)
SELECT s.style_code, 'PUBLIC_OFFICIAL'::template_scope_enum, 'OFFICIAL_PUBLIC_LIBRARY'::style_layer_enum,
       s.category::style_category_enum, s.name, s.prompt_suffix, s.is_default, s.sort_order, TRUE
FROM (VALUES
    ('style_realistic','GENERAL_REALISTIC','通用写实','写实摄影风格，真实人物质感，自然光影，电影级真实感，照片级细节，禁止动漫/插画/卡通化',TRUE,1),
    ('style_ancient_real','HISTORICAL_COSTUME_SHENGUO_FENG','古风写实','中国古代写实风格，传统服饰质感真实，古典光影氛围，写实摄影质感',FALSE,2),
    ('style_wuxia_fantasy','HISTORICAL_COSTUME_SHENGUO_FENG','武侠玄幻','武侠玄幻风格，飘逸衣袂，仙侠气场，戏剧化光影，气势恢宏',FALSE,3),
    ('style_modern_real','MODERN_URBAN','现代写实','现代都市写实风格，当代服装与场景，自然光摄影质感，真实生活气息',FALSE,4),
    ('style_post_apoc','WAR_PERIOD_SOVIET','末世废土','末世废土风格，破败荒凉，锈蚀质感，灰暗色调，荒芜苍凉氛围',FALSE,5),
    ('style_future_sci','CYBERPUNK_NEO_NOIR','未来科幻','未来科幻风格，赛博科技感，冷色调霓虹，机械质感，高级感光效',FALSE,6),
    ('style_guofeng_2d','GUOFENG_CLASSIC','国风2D','国风二维动画风格，中国风配色，扁平化插画线条，东方美学构图',FALSE,7),
    ('style_guofeng_3d','GUOFENG_CLASSIC','国风3D','国风三维动画风格，中国古典元素三维建模渲染，皮克斯级3D质感，东方角色设计，精致3D材质光影',FALSE,8),
    ('style_us_cartoon','COMIC_CARTOON','美式卡通','美式卡通风格，夸张比例，鲜明色彩，迪士尼/卡通频道式动画质感',FALSE,9),
    ('style_jp_anime','COMIC_CARTOON','清新日漫','日系动漫风格，清新色彩，赛璐璐上色，日式动漫画风，大眼睛角色',FALSE,10),
    ('style_kr_webtoon','COMIC_CARTOON','韩式网漫','韩国网络漫画风格，精致五官，柔和渐变上色，网漫式渲染',FALSE,11),
    ('style_shaw_wuxia','DIRECTOR_SHAW_BROTHERS','邵氏武侠','邵氏武侠电影风格，七十年代港式武侠质感，胶片颗粒，经典武打片色调',FALSE,12),
    ('style_cel_3d','COMIC_CARTOON','三渲二','三渲二风格（3D建模+2D渲染），卡通渲染质感，动画番剧画面',FALSE,13),
    ('style_gufeng_wuxia','HISTORICAL_COSTUME_SHENGUO_FENG','古风武侠','古风武侠风格，中国古代武侠气质，飘逸写意，江湖氛围',FALSE,14),
    ('style_wong_kar_wai','DIRECTOR_WONG_KAR_WAI','王家卫美学','王家卫电影美学，浓郁色彩，抽帧慢镜情绪感，霓虹夜色，暧昧光影',FALSE,15),
    ('style_tsui_hark','DIRECTOR_XU_HARK','徐克美学','徐克电影美学，奇诡武侠视觉，夸张色彩构图，天马行空的东方奇幻',FALSE,16),
    ('style_aesthetic_real','GENERAL_REALISTIC','唯美写实','唯美写实风格，柔和光线，精致细节，梦幻唯美氛围，高质感人像',FALSE,17),
    ('style_fantasy_illus','ROMANCE_IDYLLIC_MIYAZAKI','幻彩插画','幻彩插画风格，绚丽色彩，梦幻光效，艺术化插画表现',FALSE,18),
    ('style_chibi','COMIC_CARTOON','Q版卡通','Q版卡通风格，二头身可爱比例，圆润造型，明亮色彩',FALSE,19),
    ('style_color_pencil','COMIC_CARTOON','彩铅绘本','彩铅手绘绘本风格，彩色铅笔质感，温暖笔触，童书插画感',FALSE,20),
    ('style_claymation','COMIC_CARTOON','黏土动画','黏土动画风格，粘土质感角色，定格动画质感，柔和材质',FALSE,21),
    ('style_watercolor','GUOFENG_CLASSIC','淡雅水彩','淡雅水彩风格，水彩晕染，透明色调，清新雅致',FALSE,22),
    ('style_jp_healing','ROMANCE_IDYLLIC_MIYAZAKI','日系治愈','日系治愈风格，温暖柔和色调，治愈系氛围，新海诚式光影',FALSE,23),
    ('style_dual_city','MODERN_URBAN','双城风格','双城对比风格，城市光影反差，现代与传统交融视觉',FALSE,24),
    ('style_cn_dreamcore','MODERN_URBAN','中式梦核','中式梦核风格，千禧年怀旧中国场景，梦境般超现实氛围，旧照片色调',FALSE,25),
    ('style_ink_wash','GUOFENG_CLASSIC','水墨写意','中国水墨写意风格，留白意境，墨色晕染，文人画气质',FALSE,26),
    ('style_none','GENERAL_REALISTIC','无特定风格','',FALSE,27)
) AS s(style_code, category, name, prompt_suffix, is_default, sort_order);
