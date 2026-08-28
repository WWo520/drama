// creation 仓储：项目/剧集/叙事单元读写 + 状态机事务（空间隔离校验）。
package creation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound 项目不存在/无权限（20001）。
var ErrNotFound = errors.New("project not found")

// ErrSpaceForbidden 非空间成员（10004 越权）。
var ErrSpaceForbidden = errors.New("space membership required")

// ProjectRow 项目行。
type ProjectRow struct {
	ID                int64
	ProjectUUID       string
	SpaceID           int64
	SpaceUUID         string
	OwnerID           int64
	Title             string
	SourceScriptText  string
	CoverImageURL     *string
	ProjectType       string
	CurrentStep       string
	Status            string
	TotalEpisodeCount int
	TotalDurationSec  int
	LastOpenedAt      string
	CreatedAt         string
	UpdatedAt         string
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
		SELECT p.id, p.project_uuid::text, p.title, COALESCE(p.cover_image_url, fb.url), p.project_type::text,
		       p.current_step::text, p.status::text, p.total_episode_count, p.total_duration_sec,
		       to_char(p.last_opened_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(p.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       s.space_uuid::text
		FROM projects p JOIN spaces s ON s.id = p.space_id
		LEFT JOIN LATERAL (
			SELECT url FROM (
				(SELECT sh.reference_image_url AS url FROM shots sh
				  WHERE sh.project_id = p.id AND sh.deleted_at IS NULL
				    AND COALESCE(sh.reference_image_url, '') <> ''
				  ORDER BY sh.id LIMIT 1)
				UNION ALL
				(SELECT sc.main_image_url FROM scenes sc
				  WHERE sc.project_id = p.id AND sc.deleted_at IS NULL
				    AND COALESCE(sc.main_image_url, '') <> ''
				  ORDER BY sc.id LIMIT 1)
				UNION ALL
				(SELECT cv.main_image_url FROM characters ch
				  JOIN character_variants cv ON cv.character_id = ch.id
				 WHERE ch.project_id = p.id AND ch.deleted_at IS NULL AND cv.deleted_at IS NULL
				   AND COALESCE(cv.main_image_url, '') <> ''
				 ORDER BY cv.id LIMIT 1)
			) t LIMIT 1
		) fb ON TRUE
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
// splitSkillPrompt 为剧本拆分技能提示词（agent_skills.template_content，空=默认标准拆分）。
func (r *Repo) CreateSplitTask(ctx context.Context, projectID, spaceID, userID int64, scriptText string, episodeDurationMin int, splitSkillPrompt string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var taskID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, request_params_json)
		VALUES ('split-'||gen_random_uuid(), 'split-'||gen_random_uuid(), $1, $2, $3, 'SCRIPT_SPLIT', 'QUEUED',
		        jsonb_build_object('script_text', $4::text, 'episode_duration_min', $5::int, 'split_skill_prompt', $6::text))
		RETURNING id`,
		spaceID, userID, projectID, scriptText, episodeDurationMin, splitSkillPrompt,
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

// ShotVideoDTO 单镜头视频状态（3.3.7a 触发/轮询用；成片卡内嵌摘要）。
type ShotVideoDTO struct {
	ShotID            int64                `json:"shot_id"`
	ShotUUID          string               `json:"shot_uuid"`
	SortOrder         int                  `json:"sort_order"`
	DurationSec       int                  `json:"duration_sec"`
	VideoStatus       string               `json:"video_generation_status"`
	GeneratedVideoURL string               `json:"generated_video_url"`
	LinkedAssets      []ShotLinkedAssetDTO `json:"linked_assets,omitempty"`
	ShotGroupID       int64                `json:"shot_group_id,omitempty"`
	// 摘要字段（仅成片卡视图填充；ListShotsByBeat 不返回）
	SubjectDescription string `json:"subject_description,omitempty"`
	DialogueText       string `json:"dialogue_text,omitempty"`
}

// ShotLinkedAssetDTO 镜头关联资产（shot_asset_refs JOIN 业务表：名称/参考图/生成状态，
// 供 Step3 镜头卡资产小图与 @ 引用展示；口径与 LoadShotRefs 一致）。
type ShotLinkedAssetDTO struct {
	AssetKind        string `json:"asset_kind"`
	AssetID          int64  `json:"asset_id"`
	Name             string `json:"name"`
	MainImageURL     string `json:"main_image_url"`
	GenerationStatus string `json:"generation_status"`
}

// ShotAssetRefsByIDs 批量查询镜头关联资产（按 shot_id 分组；角色→场景→道具排序）。
// 取图口径（业务表）：角色→首个变体、场景/道具→自身 main_image_url。
func (r *Repo) ShotAssetRefsByIDs(ctx context.Context, shotIDs []int64) (map[int64][]ShotLinkedAssetDTO, error) {
	out := make(map[int64][]ShotLinkedAssetDTO)
	if len(shotIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT sar.shot_id, sar.asset_kind::text, sar.asset_id,
		       COALESCE(c.name, sc.base_name, p.name, ''),
		       COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, ''),
		       COALESCE(cv.generation_status, sc.generation_status::text, p.generation_status::text, '')
		FROM shot_asset_refs sar
		LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
		LEFT JOIN LATERAL (
		    SELECT main_image_url, generation_status::text AS generation_status
		    FROM character_variants WHERE character_id=c.id AND deleted_at IS NULL ORDER BY id LIMIT 1
		) cv ON TRUE
		LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
		LEFT JOIN props p ON sar.asset_kind='PROP' AND p.id=sar.asset_id AND p.deleted_at IS NULL
		WHERE sar.shot_id = ANY($1)
		ORDER BY CASE sar.asset_kind WHEN 'CHARACTER' THEN 1 WHEN 'SCENE' THEN 2 ELSE 3 END, sar.id`, shotIDs)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var shotID int64
		var a ShotLinkedAssetDTO
		if err := rows.Scan(&shotID, &a.AssetKind, &a.AssetID, &a.Name, &a.MainImageURL, &a.GenerationStatus); err != nil {
			continue
		}
		out[shotID] = append(out[shotID], a)
	}
	return out, rows.Err()
}

// ListShotsByBeat beat 下全部单镜头（Step3 分镜视频列表；3.3.7a）。
// 附带每镜头关联资产（linked_assets，资产小图/@引用展示；失败不阻断列表）。
func (r *Repo) ListShotsByBeat(ctx context.Context, beatID int64) ([]ShotVideoDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sh.id, sh.shot_uuid::text, sh.sort_order, sh.duration_sec,
		       COALESCE(sh.video_generation_status::text, 'PENDING'),
		       COALESCE(sh.generated_video_url, ''),
		       COALESCE(sh.shot_group_id, 0)
		FROM shots sh
		LEFT JOIN shot_groups sg ON sg.id = sh.shot_group_id
		WHERE sh.narrative_unit_id=$1 AND sh.deleted_at IS NULL
		ORDER BY COALESCE(sg.sort_order, 0), sh.sort_order, sh.id`,
		beatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ShotVideoDTO, 0)
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var s ShotVideoDTO
		if err := rows.Scan(&s.ShotID, &s.ShotUUID, &s.SortOrder, &s.DurationSec,
			&s.VideoStatus, &s.GeneratedVideoURL, &s.ShotGroupID); err != nil {
			return nil, err
		}
		out = append(out, s)
		ids = append(ids, s.ShotID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if refs, rerr := r.ShotAssetRefsByIDs(ctx, ids); rerr == nil {
		for i := range out {
			out[i].LinkedAssets = refs[out[i].ShotID]
		}
	}
	return out, nil
}

// ListShotsByGroup 镜头块下全部镜头（3.3.7b 镜头块批量生成用）。
func (r *Repo) ListShotsByGroup(ctx context.Context, shotGroupID int64) ([]ShotVideoDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sh.id, sh.shot_uuid::text, sh.sort_order, sh.duration_sec,
		       COALESCE(sh.video_generation_status::text, 'PENDING'),
		       COALESCE(sh.generated_video_url, '')
		FROM shots sh WHERE sh.shot_group_id=$1 AND sh.deleted_at IS NULL ORDER BY sh.sort_order`,
		shotGroupID)
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

// VideoGenOpts 视频生成参数（弹窗提交；18 - 视频制作 阶段三参数落库，VideoHandler 优先读取）。
type VideoGenOpts struct {
	Resolution string // P480/P720/P1080；空=默认 P720
	ModelKey   string // 模型 key（须命中 models.model_key 才参与路由）；空=keystore 默认路由
	VideoModel string // 用户选择的模型展示名（落 ai_model_requested 仅作记录，不参与路由）
}

// CreateVideoTasks beat 下所有 PENDING shots 批量建 VIDEO_SEGMENT_GEN 任务（3.3.7a，Outbox 同事务）。
// 返回创建的任务 ID 列表（计费冻结按任务维度进行）。
func (r *Repo) CreateVideoTasks(ctx context.Context, projectID, spaceID, userID, beatID int64, shots []ShotVideoDTO, opts VideoGenOpts) ([]int64, error) {
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
		p := map[string]any{"shot_id": sh.ShotID}
		if opts.Resolution != "" {
			p["resolution"] = opts.Resolution
		}
		if opts.ModelKey != "" {
			p["model_key"] = opts.ModelKey
		}
		params, _ := json.Marshal(p)
		var taskID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, ai_model_requested, request_params_json)
			VALUES ('video-'||gen_random_uuid(), 'video-'||gen_random_uuid(), $1, $2, $3, 'VIDEO_SEGMENT_GEN', 'QUEUED', NULLIF($4,''), $5::jsonb)
			RETURNING id`,
			spaceID, userID, projectID, opts.VideoModel, string(params)).Scan(&taskID); err != nil {
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

// VideoCardDTO 成片卡（对齐 LuxReal「成片卡」：一次视频生成任务 = 2~4 个连续分镜镜头的合并段）。
type VideoCardDTO struct {
	CardID            int64          `json:"card_id"`
	CardUUID          string         `json:"card_uuid"`
	Title             string         `json:"title"`
	SortOrder         int            `json:"sort_order"`
	DurationSec       int            `json:"duration_sec"`
	ShotCount         int            `json:"shot_count"`
	Status            string         `json:"generation_status"` // PENDING/QUEUED/RUNNING/SUCCESS
	GeneratedVideoURL string         `json:"generated_video_url"`
	PromptText        string         `json:"prompt_text"`
	Shots             []ShotVideoDTO `json:"shots,omitempty"`
}

// ListCardsByBeat 单元下全部成片卡（含卡内镜头摘要与关联资产；Step3 卡片网格数据源）。
func (r *Repo) ListCardsByBeat(ctx context.Context, beatID int64) ([]VideoCardDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT vc.id, vc.card_uuid::text, COALESCE(vc.title,''), vc.sort_order, vc.duration_sec, vc.shot_count,
		       vc.generation_status::text, COALESCE(vc.generated_video_url,''), COALESCE(vc.prompt_text,'')
		FROM video_cards vc
		WHERE vc.narrative_unit_id=$1 AND vc.deleted_at IS NULL
		ORDER BY vc.sort_order`, beatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]VideoCardDTO, 0, 6)
	cardIDs := make([]int64, 0, 6)
	for rows.Next() {
		var v VideoCardDTO
		if err := rows.Scan(&v.CardID, &v.CardUUID, &v.Title, &v.SortOrder, &v.DurationSec,
			&v.ShotCount, &v.Status, &v.GeneratedVideoURL, &v.PromptText); err != nil {
			return nil, err
		}
		out = append(out, v)
		cardIDs = append(cardIDs, v.CardID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	// 卡内镜头批量查询（含主体/台词一行摘要）
	type cardShotRow struct {
		cardID  int64
		shot    ShotVideoDTO
		subject string
	}
	shotRows := make(map[int64][]cardShotRow, len(out))
	allShotIDs := make([]int64, 0, len(out)*4)
	srows, err := r.pool.Query(ctx, `
		SELECT sh.video_card_id, sh.id, sh.shot_uuid::text, sh.sort_order, sh.duration_sec,
		       COALESCE(sh.video_generation_status::text,'PENDING'), COALESCE(sh.generated_video_url,''),
		       COALESCE(sh.subject_description,''), COALESCE(sh.dialogue_text,'')
		FROM shots sh WHERE sh.deleted_at IS NULL AND sh.video_card_id = ANY($1)
		ORDER BY sh.sort_order, sh.id`, cardIDs)
	if err == nil {
		defer srows.Close()
		for srows.Next() {
			var r2 cardShotRow
			r2.shot = ShotVideoDTO{}
			if err := srows.Scan(&r2.cardID, &r2.shot.ShotID, &r2.shot.ShotUUID, &r2.shot.SortOrder,
				&r2.shot.DurationSec, &r2.shot.VideoStatus, &r2.shot.GeneratedVideoURL,
				&r2.subject, &r2.shot.DialogueText); err != nil {
				continue
			}
			shotRows[r2.cardID] = append(shotRows[r2.cardID], r2)
			allShotIDs = append(allShotIDs, r2.shot.ShotID)
		}
	}
	refs, _ := r.ShotAssetRefsByIDs(ctx, allShotIDs)
	subjects := map[int64]string{}
	for _, csList := range shotRows {
		for _, cs := range csList {
			subjects[cs.shot.ShotID] = cs.subject
		}
	}
	_ = subjects
	for ci := range out {
		rowsList := shotRows[out[ci].CardID]
		out[ci].Shots = make([]ShotVideoDTO, 0, len(rowsList))
		for _, cs := range rowsList {
			cs.shot.LinkedAssets = refs[cs.shot.ShotID]
			cs.shot.SubjectDescription = cs.subject
			out[ci].Shots = append(out[ci].Shots, cs.shot)
		}
		if out[ci].ShotCount == 0 {
			out[ci].ShotCount = len(rowsList)
		}
	}
	return out, nil
}

// InsertVideoCards 规划视频任务入库（事务）：作废旧卡 → 按编组插入新卡 → 绑定镜头。
// groups 为连续镜头编组（每个元素是 shots.ShotID 列表），标题按全局镜头号拼接。
func (r *Repo) InsertVideoCards(ctx context.Context, beatID int64, globalOrder map[int64]int, groups [][]int64, shotsByID map[int64]ShotVideoDTO) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// 作废旧卡并解绑（重新规划语义：旧卡及其视频一并废弃）
	if _, err := tx.Exec(ctx, `UPDATE shots SET video_card_id=NULL WHERE narrative_unit_id=$1`, beatID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE video_cards SET deleted_at=NOW(), updated_at=NOW()
		WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, beatID); err != nil {
		return 0, err
	}
	n := 0
	for gi, ids := range groups {
		if len(ids) == 0 {
			continue
		}
		firstNo, lastNo := globalOrder[ids[0]], globalOrder[ids[len(ids)-1]]
		title := fmt.Sprintf("镜头%d~镜头%d", firstNo, lastNo)
		if firstNo == lastNo {
			title = fmt.Sprintf("镜头%d", firstNo)
		}
		totalDur := 0
		for _, id := range ids {
			totalDur += shotsByID[id].DurationSec
		}
		var cardID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO video_cards (narrative_unit_id, project_id, sort_order, title, duration_sec, shot_count)
			VALUES ($1, (SELECT project_id FROM narrative_units WHERE id=$1), $2, $3, $4, $5)
			RETURNING id`,
			beatID, gi+1, title, totalDur, len(ids)).Scan(&cardID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE shots SET video_card_id=$2 WHERE id = ANY($1)`, ids, cardID); err != nil {
			return 0, err
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

// CreateCardVideoTasks 为非 SUCCESS 成片卡批量建 VIDEO_SEGMENT_GEN 任务（params.card_id，
// Outbox 同事务同 CreateVideoTasks 模式）；队列 'video'，预置卡片 RUNNING。
func (r *Repo) CreateCardVideoTasks(ctx context.Context, projectID, spaceID, userID, beatID int64, cards []VideoCardDTO, opts VideoGenOpts) ([]int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	taskIDs := make([]int64, 0, len(cards))
	for _, cd := range cards {
		p := map[string]any{"card_id": cd.CardID}
		if opts.Resolution != "" {
			p["resolution"] = opts.Resolution
		}
		if opts.ModelKey != "" {
			p["model_key"] = opts.ModelKey
		}
		params, _ := json.Marshal(p)
		var taskID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, ai_model_requested, request_params_json)
			VALUES ('video-'||gen_random_uuid(), 'video-'||gen_random_uuid(), $1, $2, $3, 'VIDEO_SEGMENT_GEN', 'QUEUED', NULLIF($4,''), $5::jsonb)
			RETURNING id`,
			spaceID, userID, projectID, opts.VideoModel, string(params)).Scan(&taskID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
			VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:video', 'video',
			        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
			taskID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE video_cards SET generation_status='RUNNING', generation_task_id=$2, updated_at=NOW()
			WHERE id=$1`, cd.CardID, taskID); err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)
	}
	if len(taskIDs) > 0 {
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

// CreateShotRefImageTasks 为指定镜头批量创建镜头参考图任务（SHOT_REF_IMAGE_GEN，asset 队列）。
// 规划视频任务后为各卡首镜自动补图：作为视频生成的「首帧画面基准」锚点，
// 显著降低跨卡拼接的画面跳变。已有参考图的镜头自动跳过；单条失败不阻断批量。
func (r *Repo) CreateShotRefImageTasks(ctx context.Context, projectID, spaceID, userID int64, shotIDs []int64) ([]int64, error) {
	if len(shotIDs) == 0 {
		return nil, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	taskIDs := make([]int64, 0, len(shotIDs))
	for _, sid := range shotIDs {
		var hasRef bool
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(reference_image_url,'') <> '' FROM shots WHERE id=$1 AND deleted_at IS NULL`,
			sid).Scan(&hasRef); err != nil {
			continue // 镜头不存在/已删 → 跳过
		}
		if hasRef {
			continue
		}
		params, _ := json.Marshal(map[string]any{"shot_id": sid})
		var taskID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO generation_tasks (request_id, idempotency_key, space_id, user_id, project_id, task_type, status, request_params_json)
			VALUES ('shotref-'||gen_random_uuid(), 'shotref-'||gen_random_uuid(), $1, $2, $3, 'SHOT_REF_IMAGE_GEN', 'QUEUED', $4::jsonb)
			RETURNING id`,
			spaceID, userID, projectID, string(params)).Scan(&taskID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_dispatch_outbox (event_type, task_id, dispatch_key, queue_name, payload_json)
			VALUES ('TASK_DISPATCH', $1::bigint, 'task:'||$1::text||':dispatch:asset', 'asset',
			        jsonb_build_object('task_id', $1::int, 'eligible_at_ms', extract(epoch from now())*1000, 'priority', 5, 'attempt_no', 1))`,
			taskID); err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

// ResetBeatCardVideos 重做模式重置单元下全部成片卡与卡内镜头为 PENDING（清空旧视频 URL）。
func (r *Repo) ResetBeatCardVideos(ctx context.Context, beatID int64) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE video_cards SET generation_status='PENDING', generated_video_url=NULL, point_cost=0, updated_at=NOW()
		WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, beatID); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE shots SET video_generation_status='PENDING', generated_video_url=NULL, updated_at=NOW()
		WHERE narrative_unit_id=$1 AND deleted_at IS NULL`, beatID)
	return err
}

// ResolveCard 解析 card_uuid → 基础信息（重生成/提示词编辑用）。
type CardBasic struct {
	ID              int64
	ProjectID       int64
	NarrativeUnitID int64
	Title           string
	DurationSec     int
	Status          string
	PromptText      string
}

func (r *Repo) ResolveCard(ctx context.Context, cardUUID string) (*CardBasic, error) {
	var cb CardBasic
	err := r.pool.QueryRow(ctx, `
		SELECT id, project_id, narrative_unit_id, COALESCE(title,''), duration_sec,
		       generation_status::text, COALESCE(prompt_text,'')
		FROM video_cards WHERE card_uuid=$1 AND deleted_at IS NULL`, cardUUID).
		Scan(&cb.ID, &cb.ProjectID, &cb.NarrativeUnitID, &cb.Title, &cb.DurationSec, &cb.Status, &cb.PromptText)
	if err != nil {
		return nil, err
	}
	return &cb, nil
}

// SetCardPrompt 更新成片卡自定义提示词（空串恢复自动组装）。
func (r *Repo) SetCardPrompt(ctx context.Context, cardID int64, promptText string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE video_cards SET prompt_text=NULLIF($2,''), updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`,
		cardID, promptText)
	return err
}

// SetCardDuration 更新成片卡送模型时长（对齐 LuxReal 卡头时长下拉；4~15s 由服务层 clamp）。
func (r *Repo) SetCardDuration(ctx context.Context, cardID int64, durationSec int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE video_cards SET duration_sec=$2, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`,
		cardID, durationSec)
	return err
}

// BindCardAsset 卡级绑定参考资产：对卡内全部存活镜头写 shot_asset_refs（去重）。
// 对齐 LuxReal 拖资产 chip 进卡的精准绑定语义；返回实际新增行数。
func (r *Repo) BindCardAsset(ctx context.Context, cardID int64, assetKind string, assetID int64) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		WITH card_shots AS (
			SELECT id FROM shots WHERE video_card_id=$1 AND deleted_at IS NULL
		)
		INSERT INTO shot_asset_refs (shot_id, asset_kind, asset_id)
		SELECT cs.id, $2::asset_kind_enum, $3
		FROM card_shots cs
		WHERE NOT EXISTS (
			SELECT 1 FROM shot_asset_refs sar
			WHERE sar.shot_id = cs.id AND sar.asset_kind = $2::asset_kind_enum AND sar.asset_id = $3
		)`, cardID, assetKind, assetID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// UnbindCardAsset 卡级解绑参考资产：移除卡内全部镜头的该资产显式引用
// （无镜头级覆盖后 LoadShotRefAssets 自动回落单元级链，不会断参考）。
func (r *Repo) UnbindCardAsset(ctx context.Context, cardID int64, assetKind string, assetID int64) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM shot_asset_refs sar
		USING shots sh
		WHERE sar.shot_id = sh.id AND sh.video_card_id=$1
		  AND sar.asset_kind = $2::asset_kind_enum AND sar.asset_id = $3`,
		cardID, assetKind, assetID)
	return err
}

// BindShotAsset 镜头级绑定参考资产（精准到单个镜头，对齐 LuxReal @ 引用语义）：
// 单镜头 upsert shot_asset_refs + 同步 linked_assets_json 快照。
// 注意：镜头级一旦存在任一显式行，LoadShotRefAssets 即完全使用镜头级列表（不再混合单元级继承）。
func (r *Repo) BindShotAsset(ctx context.Context, shotID int64, assetKind string, assetID int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO shot_asset_refs (shot_id, asset_kind, asset_id)
		VALUES ($1, $2::asset_kind_enum, $3)
		ON CONFLICT (shot_id, asset_kind, asset_id) DO NOTHING`,
		shotID, assetKind, assetID)
	if err != nil {
		return false, err
	}
	if err := r.SyncShotLinkedAssets(ctx, shotID); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UnbindShotAsset 镜头级解绑：仅删除该镜头的显式引用行。
// 返回删除行数；0 行表示该引用来自叙事单元级继承（无法按镜头单独摘除），
// 由上层决定提示文案。解绑后同步刷新快照（显式行清空则回落展示单元级继承项）。
func (r *Repo) UnbindShotAsset(ctx context.Context, shotID int64, assetKind string, assetID int64) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM shot_asset_refs
		WHERE shot_id=$1 AND asset_kind=$2::asset_kind_enum AND asset_id=$3`,
		shotID, assetKind, assetID)
	if err != nil {
		return 0, err
	}
	n := tag.RowsAffected()
	if n > 0 {
		if uerr := r.SyncShotLinkedAssets(ctx, shotID); uerr != nil {
			return n, uerr
		}
	}
	return n, nil
}

// SyncShotLinkedAssets 重算单镜头的 linked_assets_json 快照：
// 存在显式行 → 快照=显式引用集；否则快照=叙事单元级继承集（与生成侧 LoadShotRefAssets 口径一致）。
func (r *Repo) SyncShotLinkedAssets(ctx context.Context, shotID int64) error {
	var explicit int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM shot_asset_refs WHERE shot_id=$1`, shotID).Scan(&explicit); err != nil {
		return err
	}
	// 统一列序：name / kind / id（pad 占位保证两分支结构一致）
	sqlExplicit := `
		SELECT COALESCE(c.name, sc.base_name, p.name, '') AS name,
		       sar.asset_kind::text AS kind, sar.asset_id AS id
		FROM shot_asset_refs sar
		LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
		LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
		LEFT JOIN props p ON sar.asset_kind='PROP' AND p.id=sar.asset_id AND p.deleted_at IS NULL
		WHERE sar.shot_id=$1`
	sqlInherit := `
		SELECT COALESCE(c.name, sc.base_name, p.name, '') AS name,
		       nal.asset_kind::text AS kind, nal.asset_id AS id
		FROM nu_asset_links nal
		LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND c.id=nal.asset_id AND c.deleted_at IS NULL
		LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
		LEFT JOIN props p ON nal.asset_kind='PROP' AND p.id=nal.asset_id AND p.deleted_at IS NULL
		WHERE nal.narrative_unit_id=(SELECT narrative_unit_id FROM shots WHERE id=$1)`
	q := sqlExplicit
	if explicit == 0 {
		q = sqlInherit
	}
	rows, err := r.pool.Query(ctx, q, shotID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type snap struct {
		id   int64
		kind string
		name string
	}
	items := make([]snap, 0, 8)
	for rows.Next() {
		var it snap
		if err := rows.Scan(&it.name, &it.kind, &it.id); err != nil {
			continue
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		key := it.kind + ":" + strconv.FormatInt(it.id, 10)
		if it.name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, map[string]any{"asset_kind": it.kind, "asset_id": it.id, "name": it.name})
	}
	lj, _ := json.Marshal(out)
	_, err = r.pool.Exec(ctx, `
		UPDATE shots SET linked_assets_json=$2::jsonb, updated_at=NOW() WHERE id=$1`,
		shotID, string(lj))
	return err
}

// MarkVideoCardFailedByTask 任务死信归位：card_id 命中的卡置 FAILED_DEAD + 卡内镜头 FAILED；
// legacy shot 模式（params.shot_id）同样把镜头置 FAILED。幂等，失败容错（只记日志由调用方打）。
func (r *Repo) MarkVideoFailedByTask(ctx context.Context, paramsJSON []byte) {
	var p struct {
		CardID int64 `json:"card_id"`
		ShotID int64 `json:"shot_id"`
	}
	if err := json.Unmarshal(paramsJSON, &p); err != nil {
		return
	}
	if p.CardID != 0 {
		if _, err := r.pool.Exec(ctx, `
			UPDATE video_cards SET generation_status='FAILED_DEAD', updated_at=NOW()
			WHERE id=$1 AND generation_status IN ('PENDING','QUEUED','RUNNING')`, p.CardID); err != nil {
			return
		}
		_, _ = r.pool.Exec(ctx, `
			UPDATE shots SET video_generation_status='FAILED_DEAD', updated_at=NOW()
			WHERE video_card_id=$1 AND video_generation_status NOT IN ('SUCCESS','FAILED_DEAD')`, p.CardID)
	}
	if p.ShotID != 0 {
		_, _ = r.pool.Exec(ctx, `
			UPDATE shots SET video_generation_status='FAILED_DEAD', updated_at=NOW()
			WHERE id=$1 AND video_generation_status NOT IN ('SUCCESS','FAILED_DEAD')`, p.ShotID)
	}
}

// RepairStaleVideoCards 存量修复：RUNNING/QUEUED 卡且任务已终态（或无任务关联）→ FAILED_DEAD，
// 卡内未成功镜头 → FAILED_DEAD；僵尸 RUNNING 镜头同理。返回修复的卡数与镜头数。
func (r *Repo) RepairStaleVideoCards(ctx context.Context) (cards int64, shots int64, err error) {
	tag, e := r.pool.Exec(ctx, `
		UPDATE video_cards vc SET generation_status='FAILED_DEAD', updated_at=NOW()
		WHERE vc.deleted_at IS NULL AND vc.generation_status IN ('QUEUED','RUNNING')
		  AND (
		    vc.generation_task_id IS NULL
		    OR EXISTS (SELECT 1 FROM generation_tasks g WHERE g.id = vc.generation_task_id
		               AND g.status IN ('FAILED_RETRYING','FAILED_DEAD'))
		  )`)
	cards = tag.RowsAffected()
	if e != nil {
		return cards, 0, e
	}
	tag2, e2 := r.pool.Exec(ctx, `
		UPDATE shots sh SET video_generation_status='FAILED_DEAD', updated_at=NOW()
		FROM video_cards vc
		WHERE sh.video_card_id = vc.id AND vc.deleted_at IS NULL
		  AND vc.generation_status = 'FAILED_DEAD'
		  AND sh.video_generation_status NOT IN ('SUCCESS','FAILED_DEAD')`)
	shots = tag2.RowsAffected()
	return cards, shots, e2
}

// MarkTaskFailed 标记任务失败（计费冻结失败等场景回滚任务状态）。
func (r *Repo) MarkTaskFailed(ctx context.Context, taskID int64, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE generation_tasks SET status='FAILED', error_message=$2, user_display_error=$2, updated_at=NOW()
		WHERE id=$1`, taskID, reason)
	return err
}

// BeatAssetReadiness 单元关联资产就绪度（F10 门控，18 - 视频制作 阶段一.3）：
// linked=关联资产数；ready=参考图非空数（业务表口径：角色→首个变体、场景/道具→自身 main_image_url，
// 与 LoadShotRefs 一致；assets 表为通用注册表无 main_image_url 列）。
// linked>0 且 ready<linked → 引导回 Step2（对齐 LuxReal code=6002）；无关联资产则放行纯 T2V。
func (r *Repo) BeatAssetReadiness(ctx context.Context, beatID int64) (linked, ready int, err error) {
	var l, rd int64
	err = r.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE t.url != '') FROM (
			SELECT COALESCE(cv.main_image_url, sc.main_image_url, p.main_image_url, '') AS url
			FROM nu_asset_links nal
			LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND c.id=nal.asset_id AND c.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT main_image_url FROM character_variants
			    WHERE character_id=c.id AND deleted_at IS NULL ORDER BY id LIMIT 1
			) cv ON TRUE
			LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
			LEFT JOIN props p ON nal.asset_kind='PROP' AND p.id=nal.asset_id AND p.deleted_at IS NULL
			WHERE nal.narrative_unit_id=$1
		) t`, beatID).Scan(&l, &rd)
	return int(l), int(rd), err
}

// ResetBeatShotVideos 重置 beat 下全部已 SUCCESS 镜头与镜头块（PENDING + 清旧视频 URL）。
// "重新生成"语义（redo）：批量接口默认只补缺失镜头，redo=true 时先重置已成功镜头再全量重建
// （对齐单镜头 RegenerateShot 的重置语义；镜头块状态一并回退避免残留 COMPOSED/SUCCESS）。
func (r *Repo) ResetBeatShotVideos(ctx context.Context, beatID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE shots SET video_generation_status='PENDING', generated_video_url=NULL, updated_at=NOW()
		WHERE narrative_unit_id=$1 AND video_generation_status='SUCCESS' AND deleted_at IS NULL`, beatID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE shot_groups SET video_generation_status='PENDING', updated_at=NOW()
		WHERE narrative_unit_id=$1 AND video_generation_status='SUCCESS' AND deleted_at IS NULL`, beatID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveVideoModelKey 校验模型 key（models.model_key，T2V 能力、启用中）：
// 无效/为空返回空串（走默认路由），避免前端任意字符串阻断 Key 选择。
func (r *Repo) ResolveVideoModelKey(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	var one int
	if err := r.pool.QueryRow(ctx, `
		SELECT 1 FROM models m
		JOIN model_providers p ON p.id=m.provider_id
		WHERE m.model_key=$1 AND m.capability='T2V' AND m.enabled AND m.deleted_at IS NULL
		  AND p.enabled AND p.deleted_at IS NULL`, key).Scan(&one); err != nil {
		return ""
	}
	return key
}

// ListBeats 叙事单元列表（3.1.4）。
func (r *Repo) ListBeats(ctx context.Context, episodeID int64) ([]BeatDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT nu.nu_uuid::text, nu.sort_order, nu.title, nu.preset_duration_sec, nu.actual_duration_sec,
		       nu.status::text, nu.version_num, COALESCE(nu.content, ''), COALESCE(nu.script_original_text, ''),
		       COALESCE(sg.cnt, 0)
		FROM narrative_units nu
		LEFT JOIN (
			SELECT narrative_unit_id, count(*) AS cnt FROM shot_groups WHERE deleted_at IS NULL GROUP BY narrative_unit_id
		) sg ON sg.narrative_unit_id = nu.id
		WHERE nu.episode_id = $1 AND nu.deleted_at IS NULL ORDER BY nu.sort_order`,
		episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BeatDTO, 0)
	for rows.Next() {
		var b BeatDTO
		if err := rows.Scan(&b.BeatID, &b.SortOrder, &b.Title, &b.PresetDurationSec, &b.ActualDurationSec,
			&b.Status, &b.VersionNum, &b.Content, &b.ScriptOriginalText, &b.ShotGroupCount); err != nil {
			return nil, err
		}
		// 前端展示原文：script_original_text 优先，content 兜底
		b.ScriptText = b.ScriptOriginalText
		if b.ScriptText == "" {
			b.ScriptText = b.Content
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetSkillTemplate 按技能码查询启用中的剧本拆分技能提示词（全局 agent_skills 表）。
func (r *Repo) GetSkillTemplate(ctx context.Context, skillCode string) (string, error) {
	var tpl string
	err := r.pool.QueryRow(ctx, `
		SELECT template_content FROM agent_skills
		WHERE skill_code = $1 AND kind = 'SCRIPT_SPLIT' AND enabled = TRUE AND deleted_at IS NULL`,
		skillCode).Scan(&tpl)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return tpl, err
}

// ListSkills 启用中的技能列表（按 kind 过滤，sort_order 升序；供用户端 /skills）。
func (r *Repo) ListSkills(ctx context.Context, kind string) ([]SkillDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT skill_code, name, COALESCE(description, ''), template_content
		FROM agent_skills
		WHERE kind = $1::agent_skill_kind_enum AND enabled = TRUE AND deleted_at IS NULL
		ORDER BY sort_order ASC, id ASC`,
		kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SkillDTO, 0)
	for rows.Next() {
		var s SkillDTO
		if err := rows.Scan(&s.SkillCode, &s.Name, &s.Description, &s.TemplateContent); err != nil {
			return nil, err
		}
		out = append(out, s)
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

// ExportPreflight 合成前预检结果：按 URL 去重后的片段统计。
type ExportPreflight struct {
	Total       int     `json:"total"` // 去重后可参与合成的片段数
	UniqueShots int     `json:"unique_shots"`
	TotalDur    float64 `json:"total_duration_sec"`
	Duplicates  int     `json:"duplicates"` // 被去重掉的重复镜头数
	Beats       int     `json:"beats"`      // 涉及的叙事单元数
}

// FirstEpisodeOf 项目首集（sort_order 最小；供导出缺省目标集使用）。
func (r *Repo) FirstEpisodeOf(ctx context.Context, projectID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM episodes WHERE project_id=$1 AND deleted_at IS NULL ORDER BY sort_order LIMIT 1`, projectID).Scan(&id)
	return id, err
}

// PreflightExportCompose 合成前预检（3.4.1 前置）：指定集按序 SUCCESS 镜头去重统计。
func (r *Repo) PreflightExportCompose(ctx context.Context, episodeID int64) (*ExportPreflight, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (sh.generated_video_url)
		       sh.generated_video_url, COALESCE(sh.duration_sec, 0)::float8,
		       nu.sort_order
		FROM narrative_units nu
		JOIN shots sh ON sh.narrative_unit_id = nu.id AND sh.deleted_at IS NULL
		WHERE nu.episode_id = $1
		  AND nu.deleted_at IS NULL
		  AND sh.video_generation_status = 'SUCCESS' AND COALESCE(sh.generated_video_url, '') <> ''
		ORDER BY sh.generated_video_url`, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pf := &ExportPreflight{}
	seenNU := map[int]bool{}
	for rows.Next() {
		var url string
		var dur float64
		var nuSort int
		if err := rows.Scan(&url, &dur, &nuSort); err != nil {
			return nil, err
		}
		pf.Total++
		pf.UniqueShots++
		pf.TotalDur += dur
		seenNU[nuSort] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pf.Beats = len(seenNU)
	// 重复数：该集全部 SUCCESS 镜头数 − 去重后
	var allN int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM narrative_units nu
		JOIN shots sh ON sh.narrative_unit_id = nu.id AND sh.deleted_at IS NULL
		WHERE nu.episode_id = $1
		  AND nu.deleted_at IS NULL
		  AND sh.video_generation_status = 'SUCCESS' AND COALESCE(sh.generated_video_url, '') <> ''`, episodeID).Scan(&allN); err != nil {
		return nil, err
	}
	pf.Duplicates = allN - pf.Total
	return pf, nil
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
// ResolveNarrativeUnit 叙事单元 uuid → (id, project_id)。
func (r *Repo) ResolveNarrativeUnit(ctx context.Context, nuUUID string) (int64, int64, error) {
	var id, projectID int64
	err := r.pool.QueryRow(ctx, `SELECT id, project_id FROM narrative_units WHERE nu_uuid = $1 AND deleted_at IS NULL`, nuUUID).Scan(&id, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, fmt.Errorf("%w: nu", ErrNotFound)
	}
	return id, projectID, err
}

// CreateLinkedAsset 视频制作页补充缺失资产（Step3「增加关联资产」）：
// 事务内创建 characters/scenes/props 记录 + 写 nu_asset_links 挂到指定叙事单元；
// origin_episode_id 取该单元所在剧集序号（资产归属集，与拆分抽取口径一致）。
// 幂等：项目内同名同类型资产已存在时不重建、直接复用并补建关联。
// 返回 asset_id 与剧集 sort_order（前端据此调 batch-generate 触发参考图生成）。
func (r *Repo) CreateLinkedAsset(ctx context.Context, nuUUID string, assetKind string, name string, description string) (int64, int64, error) {
	nuID, projectID, err := r.ResolveNarrativeUnit(ctx, nuUUID)
	if err != nil {
		return 0, 0, err
	}
	var epID, epSort int64
	if err := r.pool.QueryRow(ctx, `
		SELECT e.id, e.sort_order FROM episodes e
		JOIN narrative_units nu ON nu.episode_id=e.id WHERE nu.id=$1`, nuID).
		Scan(&epID, &epSort); err != nil {
		return 0, 0, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	tableByKind := map[string]string{"CHARACTER": "characters", "SCENE": "scenes", "PROP": "props"}
	nameColByKind := map[string]string{"CHARACTER": "name", "SCENE": "base_name", "PROP": "name"}
	prefixByKind := map[string]string{"CHARACTER": "CHAR", "SCENE": "SCENE", "PROP": "PROP"}
	tbl, okTbl := tableByKind[assetKind]
	nameCol, okCol := nameColByKind[assetKind]
	if !okTbl || !okCol {
		return 0, 0, fmt.Errorf("asset_kind 非法: %s", assetKind)
	}

	// 同名同类型资产复用（幂等：重复提交或资产已在其他集/单元存在时只补关联）
	var assetID int64
	reused := false
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s WHERE project_id=$1 AND %s=$2 AND deleted_at IS NULL ORDER BY id LIMIT 1`, tbl, nameCol),
		projectID, name).Scan(&assetID); err == nil {
		reused = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, err
	}

	if !reused {
		var cnt int
		if err := tx.QueryRow(ctx, fmt.Sprintf(
			`SELECT count(*) FROM %s WHERE project_id=$1 AND deleted_at IS NULL`, tbl), projectID).Scan(&cnt); err != nil {
			return 0, 0, err
		}
		code := fmt.Sprintf("%s_%03d", prefixByKind[assetKind], cnt+1)
		for attempt := 0; attempt < 5; attempt++ {
			var codeTaken bool
			if err := tx.QueryRow(ctx, fmt.Sprintf(
				`SELECT EXISTS(SELECT 1 FROM %s WHERE project_id=$1 AND code=$2 AND deleted_at IS NULL)`, tbl),
				projectID, code).Scan(&codeTaken); err != nil {
				return 0, 0, err
			}
			if !codeTaken {
				break
			}
			code = fmt.Sprintf("%s_%03d_%d", prefixByKind[assetKind], cnt+1, time.Now().UnixMilli()%100000)
		}
		descCol := map[string]string{"CHARACTER": "description_notes", "SCENE": "base_prompt", "PROP": "description_prompt"}[assetKind]
		insertSQL := fmt.Sprintf(
			`INSERT INTO %s (project_id, code, %s, %s, origin_episode_id)
			VALUES ($1,$2,$3,$4,NULLIF($5,0))
			RETURNING id`, tbl, nameCol, descCol)
		if err := tx.QueryRow(ctx, insertSQL, projectID, code, name, description, epSort).Scan(&assetID); err != nil {
			return 0, 0, err
		}
	}
	upsertLink := func() error {
		_, lerr := tx.Exec(ctx, `
			INSERT INTO nu_asset_links (narrative_unit_id, asset_kind, asset_id)
			VALUES ($1, $2::asset_kind_enum, $3) ON CONFLICT DO NOTHING`,
			nuID, assetKind, assetID)
		return lerr
	}
	if err := upsertLink(); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return assetID, epSort, nil
}

// SaveVersionSnapshot 保存叙事单元版本快照（F4：编辑前自动快照，用于回滚）。
func (r *Repo) SaveVersionSnapshot(ctx context.Context, narrativeUnitID, userID int64, script, storyboard string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO narrative_unit_versions (narrative_unit_id, version_num, label, script_snapshot, storyboard_json_snapshot, created_by_user_id, created_from)
		SELECT $1, COALESCE(MAX(version_num),0)+1, '编辑前快照', $3, $4, $2, 'MANUAL_EDIT'
		FROM narrative_unit_versions WHERE narrative_unit_id=$1`,
		narrativeUnitID, userID, script, storyboard)
	return err
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
