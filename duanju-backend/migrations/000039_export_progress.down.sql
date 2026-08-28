ALTER TABLE export_records
  DROP COLUMN IF EXISTS progress_stage,
  DROP COLUMN IF EXISTS progress_detail_json,
  DROP COLUMN IF EXISTS segments_total,
  DROP COLUMN IF EXISTS segments_failed,
  DROP COLUMN IF EXISTS duration_sec;
