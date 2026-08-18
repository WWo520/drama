// creation 仓储：项目/剧集/叙事单元读写 + 状态机事务（空间隔离校验）。
package creation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound 项目不存在/无权限（20001）。
var ErrNotFound = errors.New("project not found")

// ErrSpaceForbidden 非空间成员（10004 越权）。
var ErrSpaceForbidden = errors.New("space membership required")

// ProjectRow 项目行。
type ProjectRow struct {
	ID            int64
	ProjectUUID   string
	SpaceID       int64
	SpaceUUID     string
	OwnerID       int64
	Title         string
	SourceScriptText string
	CoverImageURL *string
	ProjectType   string
	CurrentStep   string
	Status        string
	TotalEpisodeCount int
	TotalDurationSec  int
	LastOpenedAt  string
	CreatedAt     string
	UpdatedAt     string
}

// Repo creation 数据访问。
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// ResolveSpace 解析 space_uuid → id 并校验成员身份（PERSONAL 空间 owner=成员）。
func (r *Repo) ResolveSpace(ctx context.Context, spaceUUID string, userID int64) (int64, error) {
	var spaceID int64
	err := r.pool.QueryRow(ctx, `
		SELECT s.id FROM spaces s
		JOIN space_members m ON m.space_id = s.id AND m.user_id = $2 AND m.deleted_at IS NULL
		WHERE s.space_uuid = $1 AND s.deleted_at IS NULL AND s.status = 'ACTIVE'`,
		spaceUUID, userID,
	).Scan(&spaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrSpaceForbidden
	}
	return spaceID, err
}

// DefaultSpaceID 用户默认空间。
func (r *Repo) DefaultSpaceID(ctx context.Context, userID int64) (int64, string, error) {
	var id int64
	var uuid string
	err := r.pool.QueryRow(ctx, `
		SELECT s.id, s.space_uuid::text FROM spaces s
		WHERE s.owner_user_id = $1 AND s.is_default = TRUE AND s.deleted_at IS NULL ORDER BY s.id LIMIT 1`,
		userID,
	).Scan(&id, &uuid)
	return id, uuid, err
}

// CreateProject 创建项目事务：项目 + 第 1 集（05 3.0.2 创建即进 Step1）。
func (r *Repo) CreateProject(ctx context.Context, spaceID, ownerID int64, spaceUUID, title, scriptText string) (*ProjectRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var p ProjectRow
	err = tx.QueryRow(ctx, `
		INSERT INTO projects (space_id, owner_user_id, title, source_script_text)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id, project_uuid::text, title, project_type::text, current_step::text, status::text,
		          total_episode_count, total_duration_sec,
		          to_char(last_opened_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
		spaceID, ownerID, title, scriptText,
	).Scan(&p.ID, &p.ProjectUUID, &p.Title, &p.ProjectType, &p.CurrentStep, &p.Status,
		&p.TotalEpisodeCount, &p.TotalDurationSec, &p.LastOpenedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.SpaceID = spaceID
	p.SpaceUUID = spaceUUID
	p.OwnerID = ownerID

	// 自动创建第 1 集（D.1 顺序 6：episodes）
	if _, err := tx.Exec(ctx, `
		INSERT INTO episodes (project_id, sort_order, title) VALUES ($1, 1, '第1集')`,
		p.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProjects 项目列表（3.0.1：我的当前空间；按 last_opened_at 排序）。
func (r *Repo) ListProjects(ctx context.Context, spaceID int64, page, pageSize int) ([]ProjectRow, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM projects WHERE space_id = $1 AND deleted_at IS NULL`, spaceID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.project_uuid::text, p.title, p.cover_image_url, p.project_type::text,
		       p.current_step::text, p.status::text, p.total_episode_count, p.total_duration_sec,
		       to_char(p.last_opened_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       s.space_uuid::text
		FROM projects p JOIN spaces s ON s.id = p.space_id
		WHERE p.space_id = $1 AND p.deleted_at IS NULL
		ORDER BY p.last_opened_at DESC
		LIMIT $2 OFFSET $3`,
		spaceID, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]ProjectRow, 0)
	for rows.Next() {
		var p ProjectRow
		if err := rows.Scan(&p.ID, &p.ProjectUUID, &p.Title, &p.CoverImageURL, &p.ProjectType,
			&p.CurrentStep, &p.Status, &p.TotalEpisodeCount, &p.TotalDurationSec,
			&p.LastOpenedAt, &p.CreatedAt, &p.UpdatedAt, &p.SpaceUUID); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// GetProject 项目详情（按 uuid + 空间成员校验）。
func (r *Repo) GetProject(ctx context.Context, projectUUID string, userID int64) (*ProjectRow, error) {
	var p ProjectRow
	err := r.pool.QueryRow(ctx, `
		SELECT p.id, p.project_uuid::text, p.title, p.cover_image_url, p.project_type::text,
		       p.current_step::text, p.status::text, p.total_episode_count, p.total_duration_sec,
		       to_char(p.last_opened_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       s.space_uuid::text, p.space_id, p.owner_user_id, COALESCE(p.source_script_text, '')
		FROM projects p
		JOIN spaces s ON s.id = p.space_id
		JOIN space_members m ON m.space_id = p.space_id AND m.user_id = $2 AND m.deleted_at IS NULL
		WHERE p.project_uuid = $1 AND p.deleted_at IS NULL`,
		projectUUID, userID,
	).Scan(&p.ID, &p.ProjectUUID, &p.Title, &p.CoverImageURL, &p.ProjectType,
		&p.CurrentStep, &p.Status, &p.TotalEpisodeCount, &p.TotalDurationSec,
		&p.LastOpenedAt, &p.CreatedAt, &p.UpdatedAt, &p.SpaceUUID, &p.SpaceID, &p.OwnerID, &p.SourceScriptText)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProjectIncludingDeleted 项目详情（含软删，供恢复接口使用，权限校验同 GetProject）。
func (r *Repo) GetProjectIncludingDeleted(ctx context.Context, projectUUID string, userID int64) (*ProjectRow, error) {
	var p ProjectRow
	err := r.pool.QueryRow(ctx, `
		SELECT p.id, p.project_uuid::text, p.title, p.cover_image_url, p.project_type::text,
		       p.current_step::text, p.status::text, p.total_episode_count, p.total_duration_sec,
		       to_char(p.last_opened_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       s.space_uuid::text, p.space_id, p.owner_user_id, COALESCE(p.source_script_text, '')
		FROM projects p
		JOIN spaces s ON s.id = p.space_id
		JOIN space_members m ON m.space_id = p.space_id AND m.user_id = $2 AND m.deleted_at IS NULL
		WHERE p.project_uuid = $1`,
		projectUUID, userID,
	).Scan(&p.ID, &p.ProjectUUID, &p.Title, &p.CoverImageURL, &p.ProjectType,
		&p.CurrentStep, &p.Status, &p.TotalEpisodeCount, &p.TotalDurationSec,
		&p.LastOpenedAt, &p.CreatedAt, &p.UpdatedAt, &p.SpaceUUID, &p.SpaceID, &p.OwnerID, &p.SourceScriptText)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProject 更新标题/封面（3.0.4）。
func (r *Repo) UpdateProject(ctx context.Context, id int64, title, coverURL, scriptText *string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE projects SET
		  title = COALESCE($2, title),
		  cover_image_url = COALESCE($3, cover_image_url),
		  source_script_text = COALESCE($4, source_script_text)
		WHERE id = $1`,
		id, title, coverURL, scriptText)
	return err
}

// SoftDelete 软删除（3.0.5，回收站）。
func (r *Repo) SoftDelete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE projects SET deleted_at = NOW() WHERE id = $1`, id)
	return err
}

// Restore 恢复（3.0.6）。
func (r *Repo) Restore(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE projects SET deleted_at = NULL WHERE id = $1`, id)
	return err
}

// TouchLastOpened 更新最近打开（3.0.7）。
func (r *Repo) TouchLastOpened(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE projects SET last_opened_at = NOW() WHERE id = $1`, id)
	return err
}

// TransitionStatus 状态机原子转移：FOR UPDATE 锁行 → 校验 → 更新（03 A.2 触发条件）。
// 返回旧状态；非法转移返回 ErrInvalidTransition 且不修改。
func (r *Repo) TransitionStatus(ctx context.Context, id int64, to string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var from string
	if err := tx.QueryRow(ctx, `SELECT status::text FROM projects WHERE id = $1 FOR UPDATE`, id).Scan(&from); err != nil {
		return "", err
	}
	if err := ValidateTransition(from, to); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE projects SET status = $2, current_step = $3 WHERE id = $1`,
		id, to, StepForStatus(to)); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return from, nil
}

// CreateSplitTask 创建拆分任务 + Outbox（S1-B04 调度引擎消费；B05 前任务停留在 PENDING）。
func (r *Repo) CreateSplitTask(ctx context.Context, projectID, spaceID, userID int64, scriptText string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var taskID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, request_params_json)
		VALUES ('split-'||gen_random_uuid(), 'split-'||gen_random_uuid(), $1, $2, $3, 'SCRIPT_SPLIT', 'QUEUED',
		        jsonb_build_object('script_text', $4::text))
		RETURNING id`,
		spaceID, userID, projectID, scriptText,
	).Scan(&taskID)
	if err != nil {
		return 0, err
	}
	// Outbox 可靠投递（B4.1a）：同一事务
	_, err = tx.Exec(ctx, `
		INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
		VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:1', 'split',
		        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
		taskID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return taskID, nil
}

// ListEpisodes 剧集列表（3.1.2）。
func (r *Repo) ListEpisodes(ctx context.Context, projectID int64) ([]EpisodeDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT episode_uuid::text, sort_order, title, status::text, current_step::text,
		       narrative_unit_count, actual_duration_sec
		FROM episodes WHERE project_id = $1 AND deleted_at IS NULL ORDER BY sort_order`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EpisodeDTO, 0)
	for rows.Next() {
		var e EpisodeDTO
		if err := rows.Scan(&e.EpisodeID, &e.SortOrder, &e.Title, &e.Status, &e.CurrentStep,
			&e.NarrativeUnitCount, &e.ActualDurationSec); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ShotVideoDTO 单镜头视频状态（3.3.7a 触发/轮询用）。
type ShotVideoDTO struct {
	ShotID            int64  `json:"shot_id"`
	ShotUUID          string `json:"shot_uuid"`
	SortOrder         int    `json:"sort_order"`
	DurationSec       int    `json:"duration_sec"`
	VideoStatus       string `json:"video_generation_status"`
	GeneratedVideoURL string `json:"generated_video_url"`
}

// ListShotsByBeat beat 下全部单镜头（Step3 分镜视频列表；3.3.7a）。
func (r *Repo) ListShotsByBeat(ctx context.Context, beatID int64) ([]ShotVideoDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sh.id, sh.shot_uuid::text, sh.sort_order, sh.duration_sec,
		       COALESCE(sh.video_generation_status::text, 'PENDING'),
		       COALESCE(sh.generated_video_url, '')
		FROM shots sh WHERE sh.narrative_unit_id=$1 AND sh.deleted_at IS NULL ORDER BY sh.sort_order`,
		beatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ShotVideoDTO, 0)
	for rows.Next() {
		var s ShotVideoDTO
		if err := rows.Scan(&s.ShotID, &s.ShotUUID, &s.SortOrder, &s.DurationSec, &s.VideoStatus, &s.GeneratedVideoURL); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateVideoTasks beat 下所有 PENDING shots 批量建 VIDEO_SEGMENT_GEN 任务（3.3.7a，Outbox 同事务）。
// 返回创建的任务 ID 列表（计费冻结按任务维度进行）。
func (r *Repo) CreateVideoTasks(ctx context.Context, projectID, spaceID, userID, beatID int64, shots []ShotVideoDTO) ([]int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	taskIDs := make([]int64, 0, len(shots))
	for _, sh := range shots {
		if sh.VideoStatus == "SUCCESS" {
			continue
		}
		params, _ := json.Marshal(map[string]any{"shot_id": sh.ShotID})
		var taskID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, request_params_json)
			VALUES ('video-'||gen_random_uuid(), 'video-'||gen_random_uuid(), $1, $2, $3, 'VIDEO_SEGMENT_GEN', 'QUEUED', $4::jsonb)
			RETURNING id`,
			spaceID, userID, projectID, string(params)).Scan(&taskID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
			VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:video', 'video',
			        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
			taskID); err != nil {
			return nil, err
		}
		// 预置 shot 状态 RUNNING（幂等：仅 PENDING 可置）
		if _, err := tx.Exec(ctx, `
			UPDATE shots SET video_generation_status='RUNNING' WHERE id=$1 AND video_generation_status='PENDING'`,
			sh.ShotID); err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)
	}
	if len(taskIDs) > 0 {
		// 项目状态推进：PARTIAL_ASSETS/ALL_ASSETS_READY → VIDEO_GENERATING（03 A.2）
		if _, err := tx.Exec(ctx, `
			UPDATE projects SET status='VIDEO_GENERATING', current_step='VIDEO_PRODUCTION'
			WHERE id=$1 AND status IN ('PARTIAL_ASSETS','ALL_ASSETS_READY')`, projectID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

// MarkTaskFailed 标记任务失败（计费冻结失败等场景回滚任务状态）。
func (r *Repo) MarkTaskFailed(ctx context.Context, taskID int64, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE generation_tasks SET status='FAILED', error_message=$2, user_display_error=$2, updated_at=NOW()
		WHERE id=$1`, taskID, reason)
	return err
}

// ListBeats 叙事单元列表（3.1.4）。
func (r *Repo) ListBeats(ctx context.Context, episodeID int64) ([]BeatDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT nu_uuid::text, sort_order, title, preset_duration_sec, actual_duration_sec,
		       status::text, version_num, COALESCE(content, ''), COALESCE(script_original_text, '')
		FROM narrative_units WHERE episode_id = $1 AND deleted_at IS NULL ORDER BY sort_order`,
		episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BeatDTO, 0)
	for rows.Next() {
		var b BeatDTO
		if err := rows.Scan(&b.BeatID, &b.SortOrder, &b.Title, &b.PresetDurationSec, &b.ActualDurationSec,
			&b.Status, &b.VersionNum, &b.Content, &b.ScriptOriginalText); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ResolveProjectUUID project_id → project_uuid（供 handler 路由转换）。
func (r *Repo) ResolveProjectUUID(ctx context.Context, projectID int64) (string, error) {
	var uuid string
	err := r.pool.QueryRow(ctx, `SELECT project_uuid::text FROM projects WHERE id = $1`, projectID).Scan(&uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return uuid, err
}

// SpaceIDByProject 项目所属空间（批量生成等场景）。
func (r *Repo) SpaceIDByProject(ctx context.Context, projectID int64) (int64, error) {
	var spaceID int64
	err := r.pool.QueryRow(ctx, `SELECT space_id FROM projects WHERE id = $1`, projectID).Scan(&spaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return spaceID, err
}

// ResolveEpisode episode_uuid → episode_id + project_id。
func (r *Repo) ResolveEpisode(ctx context.Context, episodeUUID string) (int64, int64, error) {
	var epID, projectID int64
	err := r.pool.QueryRow(ctx, `SELECT id, project_id FROM episodes WHERE episode_uuid = $1 AND deleted_at IS NULL`, episodeUUID).Scan(&epID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	return epID, projectID, err
}

// CheckProjectAccess 校验用户是项目所属空间的成员（跨空间隔离铁律 D2）。
func (r *Repo) CheckProjectAccess(ctx context.Context, projectID, userID int64) error {
	var one int
	err := r.pool.QueryRow(ctx, `
		SELECT 1 FROM projects p
		JOIN space_members m ON m.space_id = p.space_id AND m.user_id = $2 AND m.deleted_at IS NULL
		WHERE p.id = $1 AND p.deleted_at IS NULL`,
		projectID, userID,
	).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ResolveProjectByBeat beat_uuid → project_id。
func (r *Repo) ResolveProjectByBeat(ctx context.Context, beatUUID string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT project_id FROM narrative_units WHERE nu_uuid = $1 AND deleted_at IS NULL`, beatUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: beat", ErrNotFound)
	}
	return id, err
}

// ResolveBeatID beat_uuid → narrative_units.id。
func (r *Repo) ResolveBeatID(ctx context.Context, beatUUID string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM narrative_units WHERE nu_uuid = $1 AND deleted_at IS NULL`, beatUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: beat", ErrNotFound)
	}
	return id, err
}

// ResolveNarrativeUnit nu_uuid → (id, project_id)。
func (r *Repo) ResolveNarrativeUnit(ctx context.Context, nuUUID string) (int64, int64, error) {
	var id, projectID int64
	err := r.pool.QueryRow(ctx, `SELECT id, project_id FROM narrative_units WHERE nu_uuid = $1 AND deleted_at IS NULL`, nuUUID).Scan(&id, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("%w: nu", ErrNotFound)
	}
	return id, projectID, err
}

// ResolveCharacter character_id（数字）→ (id, project_id)。
func (r *Repo) ResolveCharacter(ctx context.Context, charIDStr string) (int64, int64, error) {
	var id, projectID int64
	charID, err := strconv.ParseInt(charIDStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: character", ErrNotFound)
	}
	err = r.pool.QueryRow(ctx, `SELECT id, project_id FROM characters WHERE id = $1 AND deleted_at IS NULL`, charID).Scan(&id, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("%w: character", ErrNotFound)
	}
	return id, projectID, err
}

// ResolveVersionID 版本 uuid → id（narrative_unit_versions 无 uuid 列时用 id 直传）。
func (r *Repo) ResolveVersionID(ctx context.Context, versionUUID string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM narrative_unit_versions WHERE id = $1`, versionUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: version", ErrNotFound)
	}
	return id, err
}

// ResolveShotID 镜头 uuid → id。
func (r *Repo) ResolveShotID(ctx context.Context, shotUUID string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM shots WHERE shot_uuid=$1::uuid AND deleted_at IS NULL`, shotUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: shot", ErrNotFound)
	}
	return id, err
}

// ResolveShotGroup 镜头块 uuid → (id, project_id)。
func (r *Repo) ResolveShotGroup(ctx context.Context, sgUUID string) (int64, int64, error) {
	var id, projectID int64
	err := r.pool.QueryRow(ctx, `SELECT id, project_id FROM shot_groups WHERE sg_uuid=$1::uuid AND deleted_at IS NULL`, sgUUID).Scan(&id, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("%w: shot_group", ErrNotFound)
	}
	return id, projectID, err
}

// ResolveScene 场景 id → (id, project_id)。
func (r *Repo) ResolveScene(ctx context.Context, sceneIDStr string) (int64, int64, error) {
	var id, projectID int64
	sceneID, err := strconv.ParseInt(sceneIDStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: scene", ErrNotFound)
	}
	err = r.pool.QueryRow(ctx, `SELECT id, project_id FROM scenes WHERE id = $1 AND deleted_at IS NULL`, sceneID).Scan(&id, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("%w: scene", ErrNotFound)
	}
	return id, projectID, err
}

// ResolveProp 道具 id → (id, project_id)。
func (r *Repo) ResolveProp(ctx context.Context, propIDStr string) (int64, int64, error) {
	var id, projectID int64
	propID, err := strconv.ParseInt(propIDStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: prop", ErrNotFound)
	}
	err = r.pool.QueryRow(ctx, `SELECT id, project_id FROM props WHERE id = $1 AND deleted_at IS NULL`, propID).Scan(&id, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("%w: prop", ErrNotFound)
	}
	return id, projectID, err
}