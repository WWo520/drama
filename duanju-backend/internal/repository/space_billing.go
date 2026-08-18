// 团队共享积分（S2-B03）：团队账户读取/流水/成员报表 + 团队充值订单（SPACE 账户）。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SpaceBillingStore 团队计费数据访问。
type SpaceBillingStore struct {
	pool *pgxpool.Pool
}

func NewSpaceBillingStore(pool *pgxpool.Pool) *SpaceBillingStore {
	return &SpaceBillingStore{pool: pool}
}

// GetSpaceAccount 团队账户（9.3 团队视角）。
func (s *SpaceBillingStore) GetSpaceAccount(ctx context.Context, spaceID int64) (map[string]any, error) {
	var id int64
	var balance, frozen int
	err := s.pool.QueryRow(ctx, `
		SELECT id, current_balance, frozen_balance FROM point_accounts
		WHERE owner_type='SPACE' AND owner_id=$1`, spaceID).Scan(&id, &balance, &frozen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("团队账户不存在")
	}
	return map[string]any{
		"space_id":       spaceID,
		"current_balance": balance,
		"frozen_balance": frozen,
	}, err
}

// ListSpaceTxns 团队流水（9.4）：角色过滤——OWNER/ADMIN/BILLING_ONLY 全员；其他仅 operator_user_id=自己。
func (s *SpaceBillingStore) ListSpaceTxns(ctx context.Context, spaceID, viewerID int64, isFull bool, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int
	var rows pgx.Rows
	var err error
	if isFull {
		s.pool.QueryRow(ctx, `SELECT count(*) FROM point_transactions pt WHERE pt.related_space_id=$1`, spaceID).Scan(&total)
		rows, err = s.pool.Query(ctx, `
			SELECT pt.txn_type::text, pt.direction::text, pt.amount, pt.balance_after, pt.operator_user_id,
			       COALESCE(u.nickname,''), pt.description, to_char(pt.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM point_transactions pt LEFT JOIN users u ON u.id=pt.operator_user_id
			WHERE pt.related_space_id=$1 ORDER BY pt.id DESC LIMIT $2 OFFSET $3`,
			spaceID, pageSize, (page-1)*pageSize)
	} else {
		s.pool.QueryRow(ctx, `SELECT count(*) FROM point_transactions pt WHERE pt.related_space_id=$1 AND pt.operator_user_id=$2`, spaceID, viewerID).Scan(&total)
		rows, err = s.pool.Query(ctx, `
			SELECT pt.txn_type::text, pt.direction::text, pt.amount, pt.balance_after, pt.operator_user_id,
			       COALESCE(u.nickname,''), pt.description, to_char(pt.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM point_transactions pt LEFT JOIN users u ON u.id=pt.operator_user_id
			WHERE pt.related_space_id=$1 AND pt.operator_user_id=$2 ORDER BY pt.id DESC LIMIT $3 OFFSET $4`,
			spaceID, viewerID, pageSize, (page-1)*pageSize)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var txnType, dir, desc, nickname, created string
		var amount, balance int
		var operator int64
		rows.Scan(&txnType, &dir, &amount, &balance, &operator, &nickname, &desc, &created)
		out = append(out, map[string]any{
			"txn_type": txnType, "direction": dir, "amount": amount, "balance_after": balance,
			"operator_user_id": operator, "operator_nickname": nickname,
			"description": desc, "created_at": created,
		})
	}
	return out, total, rows.Err()
}

// MemberConsumptionReport 成员消费报表（OWNER/ADMIN）：按成员汇总扣费。
func (s *SpaceBillingStore) MemberConsumptionReport(ctx context.Context, spaceID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pt.operator_user_id, COALESCE(u.nickname,''), COUNT(*) AS txn_count, SUM(pt.amount) AS total_spent
		FROM point_transactions pt LEFT JOIN users u ON u.id=pt.operator_user_id
		WHERE pt.related_space_id=$1 AND pt.direction='DEBIT'
		GROUP BY pt.operator_user_id, u.nickname ORDER BY total_spent DESC`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uid int64
		var nickname string
		var count, total int
		rows.Scan(&uid, &nickname, &count, &total)
		out = append(out, map[string]any{
			"user_id": uid, "nickname": nickname, "txn_count": count, "total_spent": total,
		})
	}
	return out, rows.Err()
}

// CreateSpaceOrder 团队充值订单（2.14）：档位校验（point_packages 为唯一真源）→ orders.space_id 标记。
func (s *SpaceBillingStore) CreateSpaceOrder(ctx context.Context, userID, spaceID int64, pkgCode, amountYuan string, points int) (string, error) {
	// 档位校验（P0 修复：拒绝任意金额/积分组合）
	var pkgPoints, bonusPoints int
	var priceYuan, status string
	if err := s.pool.QueryRow(ctx, `
		SELECT points, COALESCE(bonus_points,0), price_yuan::text, status::text
		FROM point_packages WHERE pkg_code=$1 AND deleted_at IS NULL`, pkgCode).
		Scan(&pkgPoints, &bonusPoints, &priceYuan, &status); err != nil {
		return "", fmt.Errorf("套餐不存在")
	}
	if status != "ON" {
		return "", fmt.Errorf("套餐已下架")
	}
	if amountYuan != priceYuan {
		return "", fmt.Errorf("金额与套餐价目不符")
	}
	if points != pkgPoints+bonusPoints {
		return "", fmt.Errorf("积分与套餐价目不符")
	}
	orderNo := fmt.Sprintf("P%s%s", time.Now().Format("20060102150405"), uuid.NewString()[:6])
	_, err := s.pool.Exec(ctx, `
		INSERT INTO orders (order_no, user_id, space_id, order_type, plan_code, original_amount_yuan,
			discount_amount_yuan, paid_amount_yuan, pay_status, points_granted_total, idempotency_key)
		VALUES ($1, $2, $3, 'SPACE_POINTS', $4, $5, 0, $5, 'PENDING', $6, $7)`,
		orderNo, userID, spaceID, pkgCode, amountYuan, points,
		fmt.Sprintf("space-order-%s", uuid.NewString()))
	if err != nil {
		return "", err
	}
	return orderNo, nil
}

// PaySpaceOrder 团队订单支付回调：仅订单创建者可触发；SPACE 账户加积分 + 流水（幂等）。
func (s *SpaceBillingStore) PaySpaceOrder(ctx context.Context, orderNo string, points int, requesterID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var oid, spaceID, operatorID int64
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT id, space_id, user_id, pay_status::text FROM orders WHERE order_no=$1::text FOR UPDATE`, orderNo).
		Scan(&oid, &spaceID, &operatorID, &status); err != nil {
		return err
	}
	// 归属校验（P0 修复：仅订单创建者可触发入账）
	if operatorID != requesterID {
		return fmt.Errorf("无权操作该订单")
	}
	if status == "PAID" {
		return nil // 幂等
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET pay_status='PAID', paid_at=NOW() WHERE id=$1 AND pay_status='PENDING'`, oid); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE point_accounts SET current_balance=current_balance+$2, lifetime_earned_total=lifetime_earned_total+$2
		WHERE owner_type='SPACE' AND owner_id=$1`, spaceID, points); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
			balance_after, related_space_id, operator_user_id, description, idempotency_key)
		SELECT pa.id, 'SPACE', pa.owner_id, 'PURCHASE_POINTS_PACKAGE', 'CREDIT', $1, pa.current_balance, $2, $3,
		       '团队积分充值', $4
		FROM point_accounts pa WHERE pa.owner_type='SPACE' AND pa.owner_id=$2`,
		points, spaceID, operatorID, fmt.Sprintf("space-recharge-%d", oid)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}