-- UP: 镜头级参考图（对齐 LuxReal「镜头N的参考图」）——每镜头可绑定一张专属参考图，
-- 视频生成时作为最高优先级参考图注入（角色/场景基础资产图优先，其次镜头专属图）。
ALTER TABLE shots ADD COLUMN IF NOT EXISTS reference_image_url TEXT NOT NULL DEFAULT '';
