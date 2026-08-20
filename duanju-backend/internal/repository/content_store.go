// content_store 实现 creation.ContentStore：信息流/作品/互动/审核。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/domain/creation"
)

// ContentStore 内容域数据访问。
type ContentStore struct {
	pool *pgxpool.Pool
}

func NewContentStore(pool *pgxpool.Pool) *ContentStore {
	return &ContentStore{pool: pool}
}

var _ creation.ContentStore = (*ContentStore)(nil)

func (s *ContentStore) Feed(ctx context.Context, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE publish_status='PUBLISHED' AND is_deleted_recyclebin=FALSE`).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT w.work_uuid::text, w.title, COALESCE(w.cover_url,''), w.view_count, w.like_count,
		       w.favorite_count, w.comment_count, w.duration_sec,
		       COALESCE(u.nickname,''), to_char(w.published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
		WHERE w.publish_status='PUBLISHED' AND w.is_deleted_recyclebin=FALSE
		ORDER BY w.is_recommended DESC, w.published_at DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, title, cover, nickname, published string
		var views, likes, favs, comments, dur int
		rows.Scan(&uuid, &title, &cover, &views, &likes, &favs, &comments, &dur, &nickname, &published)
		out = append(out, map[string]any{
			"work_id": uuid, "title": title, "cover_url": cover,
			"view_count": views, "like_count": likes, "favorite_count": favs,
			"comment_count": comments, "duration_sec": dur, "author": nickname, "published_at": published,
		})
	}
	return out, total, rows.Err()
}

func (s *ContentStore) GetWork(ctx context.Context, workUUID string) (map[string]any, error) {
	var uuid, title, desc, cover, media, nickname, published string
	var views, likes, favs, comments int
	var dur int
	err := s.pool.QueryRow(ctx, `
		SELECT w.work_uuid::text, w.title, COALESCE(w.description,''), COALESCE(w.cover_url,''),
		       COALESCE(w.media_url,''), w.view_count, w.like_count, w.favorite_count, w.comment_count,
		       w.duration_sec, COALESCE(u.nickname,''),
		       to_char(w.published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
		WHERE w.work_uuid=$1 AND w.publish_status='PUBLISHED' AND w.is_deleted_recyclebin=FALSE`, workUUID).
		Scan(&uuid, &title, &desc, &cover, &media, &views, &likes, &favs, &comments, &dur, &nickname, &published)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("作品不存在")
	}
	if err != nil {
		return nil, err
	}
	// 浏览量 +1
	_, _ = s.pool.Exec(ctx, `UPDATE works SET view_count=view_count+1 WHERE work_uuid=$1`, workUUID)
	views++
	return map[string]any{
		"work_id": uuid, "title": title, "description": desc, "cover_url": cover, "media_url": media,
		"view_count": views, "like_count": likes, "favorite_count": favs, "comment_count": comments,
		"duration_sec": dur, "author": nickname, "published_at": published,
	}, nil
}

func (s *ContentStore) SubmitWork(ctx context.Context, spaceID, projectID, userID int64, title, desc, mediaURL string, tags []string) (string, error) {
	workUUID := uuid.NewString()
	tagsJSON, _ := json.Marshal(tags)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO works (work_uuid, space_id, owner_user_id, project_id, kind, title, description,
			media_url, publish_status, tags)
		VALUES ($1, $2, $3, $4, 'SHORT_DRAMA', $5, $6, $7, 'PENDING', $8::jsonb)
		ON CONFLICT (project_id) WHERE is_deleted_recyclebin=FALSE
		DO UPDATE SET title=$5, description=$6, media_url=$7, publish_status='PENDING', tags=$8::jsonb`,
		workUUID, spaceID, userID, projectID, title, desc, mediaURL, string(tagsJSON))
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO publish_records (work_id, action, submitter_user_id, snapshot_after_json)
		VALUES ((SELECT id FROM works WHERE work_uuid=$1), 'SUBMIT', $2, jsonb_build_object('status','PENDING'))`,
		workUUID, userID)
	return workUUID, err
}

func (s *ContentStore) Review(ctx context.Context, workUUID string, approve bool, note string, operatorID int64) error {
	status := "PUBLISHED"
	action := "APPROVE"
	if !approve {
		status = "REJECTED"
		action = "REJECT"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var wid int64
	if err := tx.QueryRow(ctx, `SELECT id FROM works WHERE work_uuid=$1::uuid`, workUUID).Scan(&wid); err != nil {
		return errors.New("作品不存在")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE works SET publish_status=$2, audit_note=$3, published_at=NOW()
		WHERE id=$1 AND publish_status='PENDING'`, wid, status, note); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO publish_records (work_id, action, operator_user_id, note, snapshot_after_json)
		VALUES ($1, $2, $3, $4, jsonb_build_object('status',$5))`,
		wid, action, operatorID, note, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *ContentStore) Revoke(ctx context.Context, workUUID string, note string, operatorID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var wid int64
	if err := tx.QueryRow(ctx, `SELECT id FROM works WHERE work_uuid=$1::uuid`, workUUID).Scan(&wid); err != nil {
		return errors.New("作品不存在")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE works SET publish_status='REVOKED', audit_note=$2 WHERE id=$1 AND publish_status='PUBLISHED'`,
		wid, note); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO publish_records (work_id, action, operator_user_id, note, snapshot_after_json)
		VALUES ($1, 'REVOKE', $2, $3, jsonb_build_object('status','REVOKED'))`,
		wid, operatorID, note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *ContentStore) AuditList(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if status == "" {
		status = "PENDING"
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE publish_status=$1`, status).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT work_uuid::text, title, COALESCE(description,''), COALESCE(media_url,''), publish_status::text,
		       to_char(updated_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI'),
		       COALESCE(view_count,0), COALESCE(like_count,0)
		FROM works WHERE publish_status=$1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`,
		status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, title, desc, media, st, updated string
		var views, likes int
		rows.Scan(&uuid, &title, &desc, &media, &st, &updated, &views, &likes)
		out = append(out, map[string]any{
			"work_id": uuid, "title": title, "description": desc, "media_url": media,
			"publish_status": st, "updated_at": updated, "view_count": views, "like_count": likes,
		})
	}
	return out, total, rows.Err()
}

func (s *ContentStore) Like(ctx context.Context, workUUID string, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO likes (work_id, user_id)
		SELECT id, $2 FROM works WHERE work_uuid=$1::uuid
		ON CONFLICT (work_id, user_id) DO NOTHING`, workUUID, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE works SET like_count=(SELECT count(*) FROM likes WHERE work_id=works.id) WHERE work_uuid=$1::uuid`, workUUID)
	return err
}

func (s *ContentStore) UnLike(ctx context.Context, workUUID string, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM likes WHERE work_id=(SELECT id FROM works WHERE work_uuid=$1::uuid) AND user_id=$2`,
		workUUID, userID)
	return err
}

func (s *ContentStore) Favorite(ctx context.Context, workUUID string, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO favorites (work_id, user_id)
		SELECT id, $2 FROM works WHERE work_uuid=$1::uuid
		ON CONFLICT (work_id, user_id) DO NOTHING`, workUUID, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE works SET favorite_count=(SELECT count(*) FROM favorites WHERE work_id=works.id) WHERE work_uuid=$1::uuid`, workUUID)
	return err
}

func (s *ContentStore) Comment(ctx context.Context, workUUID string, userID int64, content string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var wid int64
	if err := tx.QueryRow(ctx, `SELECT id FROM works WHERE work_uuid=$1::uuid`, workUUID).Scan(&wid); err != nil {
		return errors.New("作品不存在")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO comments (work_id, user_id, content) VALUES ($1, $2, $3)`, wid, userID, content); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE works SET comment_count=comment_count+1 WHERE id=$1`, wid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *ContentStore) Report(ctx context.Context, workUUID string, userID int64, reason string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO reports (work_id, reporter_user_id, reason)
		SELECT id, $2, $3 FROM works WHERE work_uuid=$1::uuid`, workUUID, userID, reason)
	return err
}

// Follow 关注用户（S3-B02 补齐：follows 表端点）。
func (s *ContentStore) Follow(ctx context.Context, followeeUUID string, followerID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO follows (follower_user_id, followee_user_id)
		SELECT $2, id FROM users WHERE user_uuid=$1::uuid
		ON CONFLICT (follower_user_id, followee_user_id) DO NOTHING`, followeeUUID, followerID)
	return err
}

// Unfollow 取消关注。
func (s *ContentStore) Unfollow(ctx context.Context, followeeUUID string, followerID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM follows WHERE followee_user_id=(SELECT id FROM users WHERE user_uuid=$1::uuid) AND follower_user_id=$2`,
		followeeUUID, followerID)
	return err
}