-- DOWN: 回滚镜头级参考图字段
ALTER TABLE shots DROP COLUMN IF EXISTS reference_image_url;
