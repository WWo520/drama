// market_store 实现 creation.MarketStore：模板/风格/任务单。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/domain/creation"
)

// MarketStore 生态数据访问。
type MarketStore struct {
	pool *pgxpool.Pool
}

func NewMarketStore(pool *pgxpool.Pool) *MarketStore {
	return &MarketStore{pool: pool}
}

var _ creation.MarketStore = (*MarketStore)(nil)

func (s *MarketStore) ListTemplates(ctx context.Context, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM templates WHERE deleted_at IS NULL`).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT template_uuid::text, name, COALESCE(description,''), price_points, used_count, is_system
		FROM templates WHERE deleted_at IS NULL ORDER BY is_system DESC, used_count DESC LIMIT $1 OFFSET $2`,
		pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, name, desc string
		var price, used int
		var sys bool
		rows.Scan(&uuid, &name, &desc, &price, &used, &sys)
		out = append(out, map[string]any{
			"template_id": uuid, "name": name, "description": desc,
			"price_points": price, "used_count": used, "is_system": sys,
		})
	}
	return out, total, rows.Err()
}

func (s *MarketStore) CreateTemplate(ctx context.Context, userID int64, name, desc, data string, price int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO templates (name, description, data, price_points, created_by)
		VALUES ($1, $2, $3::jsonb, $4, $5)`, name, desc, data, price, userID)
	return err
}

func (s *MarketStore) ApplyTemplate(ctx context.Context, templateUUID string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var data string
	if err := tx.QueryRow(ctx, `
		SELECT data::text FROM templates WHERE template_uuid=$1::uuid AND deleted_at IS NULL`, templateUUID).Scan(&data); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE templates SET used_count=used_count+1 WHERE template_uuid=$1::uuid`, templateUUID); err != nil {
		return "", err
	}
	return data, tx.Commit(ctx)
}

func (s *MarketStore) ListStyles(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT style_code, name, COALESCE(prompt_suffix_static,''), used_count, is_default
		FROM style_library WHERE enabled=TRUE ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var code, name, suffix string
		var used int
		var def bool
		rows.Scan(&code, &name, &suffix, &used, &def)
		out = append(out, map[string]any{
			"style_code": code, "name": name, "prompt_suffix": suffix, "used_count": used, "is_default": def,
		})
	}
	return out, rows.Err()
}

func (s *MarketStore) ApplyStyle(ctx context.Context, projectID int64, styleCode string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var suffix string
	if err := tx.QueryRow(ctx, `
		SELECT prompt_suffix_static FROM style_library WHERE style_code=$1 AND enabled=TRUE`, styleCode).Scan(&suffix); err != nil {
		return errors.New("风格不存在")
	}
	var cfgJSON string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(config::text,'{}') FROM projects WHERE id=$1`, projectID).Scan(&cfgJSON); err != nil {
		return errors.New("项目不存在")
	}
	var cfg map[string]any
	json.Unmarshal([]byte(cfgJSON), &cfg)
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfg["style_code"] = styleCode
	cfg["prompt_suffix"] = suffix
	b, _ := json.Marshal(cfg)
	if _, err := tx.Exec(ctx, `UPDATE projects SET config=$2::jsonb WHERE id=$1`, projectID, string(b)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE style_library SET used_count=used_count+1 WHERE style_code=$1`, styleCode); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *MarketStore) ListOrders(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	// 状态白名单（防注入）
	allowedStatus := map[string]bool{"OPEN": true, "ACCEPTED": true, "IN_PROGRESS": true, "SUBMITTED": true, "COMPLETED": true, "CANCELLED": true}
	if status != "" && !allowedStatus[status] {
		status = ""
	}
	if status == "" {
		var total int
		s.pool.QueryRow(ctx, `SELECT count(*) FROM task_orders WHERE deleted_at IS NULL`).Scan(&total)
		rows, err := s.pool.Query(ctx, `
			SELECT order_uuid::text, order_no, title, COALESCE(description,''), reward_points, status::text,
			       COALESCE(assignee_user_id,0)
			FROM task_orders WHERE deleted_at IS NULL ORDER BY id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		out := make([]map[string]any, 0)
		for rows.Next() {
			var uuid, no, title, desc, st string
			var reward int
			var assignee int64
			rows.Scan(&uuid, &no, &title, &desc, &reward, &st, &assignee)
			out = append(out, map[string]any{
				"order_id": uuid, "order_no": no, "title": title, "description": desc,
				"reward_points": reward, "status": st, "assignee_user_id": assignee,
			})
		}
		return out, total, rows.Err()
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM task_orders WHERE deleted_at IS NULL AND status=$1`, status).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT order_uuid::text, order_no, title, COALESCE(description,''), reward_points, status::text,
		       COALESCE(assignee_user_id,0)
		FROM task_orders WHERE deleted_at IS NULL AND status=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`,
		status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, no, title, desc, st string
		var reward int
		var assignee int64
		rows.Scan(&uuid, &no, &title, &desc, &reward, &st, &assignee)
		out = append(out, map[string]any{
			"order_id": uuid, "order_no": no, "title": title, "description": desc,
			"reward_points": reward, "status": st, "assignee_user_id": assignee,
		})
	}
	return out, total, rows.Err()
}

func (s *MarketStore) CreateOrder(ctx context.Context, spaceID, userID int64, title, desc string, reward int, deadline string) (string, error) {
	orderUUID := uuid.NewString()
	var deadlineAt any
	if deadline != "" {
		deadlineAt = deadline
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO task_orders (order_uuid, order_no, space_id, publisher_user_id, title, description, reward_points, deadline_at)
		VALUES ($1, 'TO'||to_char(now(),'YYYYMMDDHH24MISS')||substr(gen_random_uuid()::text,1,6), $2, $3, $4, $5, $6, $7::timestamptz)`,
		orderUUID, spaceID, userID, title, desc, reward, deadlineAt)
	if err != nil {
		return "", err
	}
	return orderUUID, nil
}

func (s *MarketStore) CreateIntent(ctx context.Context, orderID, userID int64, cover string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO task_intents (order_id, user_id, cover_letter)
		VALUES ($1, $2, $3)
		ON CONFLICT (order_id, user_id) DO NOTHING`, orderID, userID, cover)
	return err
}

func (s *MarketStore) AcceptIntent(ctx context.Context, orderUUID string, intentUserUUID string, publisherID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var intentUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE user_uuid=$1::uuid`, intentUserUUID).Scan(&intentUserID); err != nil {
		return errors.New("接单人不存在")
	}
	var oid int64
	var pub int64
	var st string
	if err := tx.QueryRow(ctx, `
		SELECT id, publisher_user_id, status::text FROM task_orders WHERE order_uuid=$1::uuid AND deleted_at IS NULL`,
		orderUUID).Scan(&oid, &pub, &st); err != nil {
		return errors.New("任务不存在")
	}
	if pub != publisherID {
		return errors.New("仅发布方可接受意向")
	}
	if st != "OPEN" {
		return errors.New("任务已非 OPEN 状态")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE task_orders SET status='ACCEPTED', assignee_user_id=$2 WHERE id=$1`, oid, intentUserID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE task_intents SET status='ACCEPTED' WHERE order_id=$1 AND user_id=$2`, oid, intentUserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *MarketStore) UpdateOrderStatus(ctx context.Context, orderUUID string, userID int64, to string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var oid, reward int64
	var st string
	var assignee int64
	if err := tx.QueryRow(ctx, `
		SELECT id, status::text, COALESCE(assignee_user_id,0), reward_points FROM task_orders
		WHERE order_uuid=$1::uuid AND deleted_at IS NULL`, orderUUID).Scan(&oid, &st, &assignee, &reward); err != nil {
		return errors.New("任务不存在")
	}
	// 状态机校验：ACCEPTED→IN_PROGRESS（接单人）→SUBMITTED（接单人）→COMPLETED（发布方）
	allowed := map[string]string{"ACCEPTED": "IN_PROGRESS", "IN_PROGRESS": "SUBMITTED", "SUBMITTED": "COMPLETED"}
	if allowed[st] != to {
		return fmt.Errorf("非法状态迁移: %s -> %s", st, to)
	}
	if (to == "IN_PROGRESS" || to == "SUBMITTED") && assignee != userID {
		return errors.New("仅接单人可操作")
	}
	if to == "COMPLETED" {
		var pub int64
		tx.QueryRow(ctx, `SELECT publisher_user_id FROM task_orders WHERE id=$1`, oid).Scan(&pub)
		if pub != userID {
			return errors.New("仅发布方可验收")
		}
		// 发放积分给接单人（03 E.7 结算）
		if _, err := tx.Exec(ctx, `
			UPDATE point_accounts SET current_balance=current_balance+$2, lifetime_earned_total=lifetime_earned_total+$2
			WHERE owner_type='USER' AND owner_id=$1`, assignee, reward); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
				balance_after, operator_user_id, description, idempotency_key)
			SELECT pa.id, 'USER', pa.owner_id, 'ADMIN_COMP', 'CREDIT', $2, pa.current_balance, $1, '任务完成奖励', $3
			FROM point_accounts pa WHERE pa.owner_type='USER' AND pa.owner_id=$1`,
			assignee, reward, fmt.Sprintf("task-reward-%d", oid)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE task_orders SET status=$2, updated_at=NOW() WHERE id=$1`, oid, to); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var _ = time.Now