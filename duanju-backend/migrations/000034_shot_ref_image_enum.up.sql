-- UP: 镜头级参考图生成任务类型（对齐 LuxReal「镜头N的参考图」/ 智能镜头）
ALTER TYPE task_type_enum ADD VALUE IF NOT EXISTS 'SHOT_REF_IMAGE_GEN';
