-- 000037 回滚：软删 27 项中文风格（保留数据不物理删除）。
UPDATE style_library SET deleted_at = NOW(), enabled = FALSE
WHERE style_code LIKE 'style\_%' ESCAPE '\' AND deleted_at IS NULL;
