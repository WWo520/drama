INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, episode_id, task_type, target_id, status, request_params_json)
SELECT 'diag-threeview-001', 'diag-threeview-001', 6, 7, 84, 164, 'CHAR_IMAGE_GEN', 137, 'QUEUED',
       jsonb_build_object('target_kind','CHARACTER_VARIANT','target_id',137,'cost',0)
RETURNING id;

INSERT INTO task_dispatch_outbox (task_id, dispatch_key, queue_name, payload_json)
SELECT (SELECT max(id) FROM generation_tasks WHERE request_id='diag-threeview-001'),
       'task:diagtv:dispatch:001', 'asset', '{}'::jsonb;
