// notify_store 实现 notify.NotifyStore：notifications 表读写。
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NotifyStore 通知数据访问。
type NotifyStore struct {
	pool *pgxpool.Pool
}

func NewNotifyStore(pool *pgxpool.Pool) *NotifyStore {
	return &NotifyStore{pool: pool}
}

func (s *NotifyStore) List(ctx context.Context, userID int64, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1 AND deleted_at IS NULL`, userID).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT id, type::text, title, COALESCE(content,''), COALESCE(link_url,''), is_read, created_at
		FROM notifications WHERE user_id=$1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var ntype, title, content, link string
		var read bool
		var createdAt time.Time
		rows.Scan(&id, &ntype, &title, &content, &link, &read, &createdAt)
		out = append(out, map[string]any{
			"notification_id": id, "type": ntype, "title": title, "content": content,
			"link": link, "is_read": read, "created_at": createdAt.Format(time.RFC3339),
		})
	}
	return out, total, rows.Err()
}

func (s *NotifyStore) MarkRead(ctx context.Context, userID, notificationID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET is_read=TRUE, read_at=NOW()
		WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, notificationID, userID)
	return err
}

func (s *NotifyStore) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET is_read=TRUE, read_at=NOW()
		WHERE user_id=$1 AND is_read=FALSE AND deleted_at IS NULL`, userID)
	return err
}

func (s *NotifyStore) Create(ctx context.Context, userID int64, ntype, title, content, link string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, type, title, content, link_url)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, ntype, title, content, link).Scan(&id)
	return id, err
}