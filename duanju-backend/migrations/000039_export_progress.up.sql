-- 000039_export_progress.up.sql
-- 导出合成进度可视化：export_records 增加进度阶段 / 片段统计 / 预估时长
-- progress_stage: NULL(未开始) / DOWNLOADING / COMPOSING / UPLOADING / DONE
-- progress_detail_json: {"done":3,"total":9,"failed":2} 等阶段明细
ALTER TABLE export_records
  ADD COLUMN IF NOT EXISTS progress_stage VARCHAR(16) NULL,
  ADD COLUMN IF NOT EXISTS progress_detail_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS segments_total INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS segments_failed INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS duration_sec INT NOT NULL DEFAULT 0;
