// order_store 实现 billing.OrderStore：订单幂等创建 + 支付回调事务（PAID+积分+流水原子）。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/billing"
)

// OrderStore 订单数据访问。
type OrderStore struct {
	pool *pgxpool.Pool
}

func NewOrderStore(pool *pgxpool.Pool) *OrderStore {
	return &OrderStore{pool: pool}
}

var _ billing.OrderStore = (*OrderStore)(nil)

// CreateOrder 幂等创建订单（idempotency_key UNIQUE 防并发双写；05 9.5）。
func (s *OrderStore) CreateOrder(ctx context.Context, userID int64, orderType, planCode, amountYuan string, points int, idemKey string) (int64, string, error) {
	// 幂等查重：同 key 返回既有订单
	var existingNo string
	err := s.pool.QueryRow(ctx, `SELECT order_no FROM orders WHERE idempotency_key=$1`, idemKey).Scan(&existingNo)
	if err == nil {
		var oid int64
		s.pool.QueryRow(ctx, `SELECT id FROM orders WHERE idempotency_key=$1`, idemKey).Scan(&oid)
		return oid, existingNo, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", err
	}

	orderNo := fmt.Sprintf("P%s%s", time.Now().Format("20060102150405"), uuid.NewString()[:6])
	paid := fmt.Sprintf("%.2f", mustParseFloat(amountYuan))
	_, ierr := s.pool.Exec(ctx, `
		INSERT INTO orders (order_no, user_id, order_type, plan_code, original_amount_yuan, discount_amount_yuan,
			paid_amount_yuan, pay_status, points_granted_total, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, 0, $5, 'PENDING', $6, $7)`,
		orderNo, userID, orderType, planCode, paid, points, idemKey)
	if ierr != nil {
		return 0, "", ierr
	}
	var oid int64
	s.pool.QueryRow(ctx, `SELECT id FROM orders WHERE order_no=$1`, orderNo).Scan(&oid)
	return oid, orderNo, nil
}

func (s *OrderStore) GetOrderByNo(ctx context.Context, orderNo string) (*billing.Order, error) {
	var o billing.Order
	err := s.pool.QueryRow(ctx, `
		SELECT id, order_no, user_id, order_type, plan_code, paid_amount_yuan::text, points_granted_total,
		       pay_status::text, COALESCE(pay_transaction_id,'')
		FROM orders WHERE order_no=$1`, orderNo).
		Scan(&o.ID, &o.OrderNo, &o.UserID, &o.OrderType, &o.PlanCode, &o.AmountYuan, &o.Points,
			&o.PayStatus, &o.PayTransactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("订单不存在")
	}
	return &o, err
}

// PayCallback 回调事务（04 H.2 步4~6）：订单 PAID + 积分加余额（含首购加赠）+ 流水。
// pay_transaction_id UNIQUE 幂等：重复回调返回 (false, nil)。
// 首购判定：历史 PAID 订单数=0 → 加赠（03 E.6.2：中包/大包 +50%）。
func (s *OrderStore) PayCallback(ctx context.Context, orderNo, payTransactionID string, points int) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	// 锁订单行
	var oid, userID int64
	var payStatus string
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, pay_status::text FROM orders WHERE order_no=$1 FOR UPDATE`, orderNo).
		Scan(&oid, &userID, &payStatus)
	if err != nil {
		return false, err
	}
	// 已支付且同交易号 → 幂等返回
	if payStatus == "PAID" {
		var existing string
		tx.QueryRow(ctx, `SELECT COALESCE(pay_transaction_id,'') FROM orders WHERE id=$1`, oid).Scan(&existing)
		if existing == payTransactionID || payTransactionID == "" {
			return false, nil
		}
		return false, fmt.Errorf("订单已支付且交易号不一致")
	}

	// 步4：订单置 PAID
	tag, err := tx.Exec(ctx, `
		UPDATE orders SET pay_status='PAID', pay_transaction_id=$2, paid_at=NOW()
		WHERE id=$1 AND pay_status='PENDING'`, oid, payTransactionID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("订单状态冲突")
	}

	// 首购判定（03 E.6.2：历史 PAID 订单数=0；加赠 50% 为 LIMITED_TIME 30 天）
	firstPurchase := false
	var paidCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM orders WHERE user_id=$1 AND pay_status='PAID'`, userID).Scan(&paidCount); err != nil {
		return false, err
	}
	if paidCount == 1 { // 当前订单已 PAID，历史=1 → 首购
		firstPurchase = true
	}
	bonus := 0
	if firstPurchase {
		bonus = points / 2 // +50% 加赠（03 E.6.2 默认；S-01 定值后替换）
		if _, err := tx.Exec(ctx, `
			UPDATE orders SET first_purchase_bonus_points=$2, points_granted_total=points_granted_total+$2 WHERE id=$1`,
			oid, bonus); err != nil {
			// 忽略（仅统计）
		}
	}

	// 步5：同一事务内加积分余额（含首购加赠）
	if _, err := tx.Exec(ctx, `
		UPDATE point_accounts SET current_balance=current_balance+$2, lifetime_earned_total=lifetime_earned_total+$2,
		       last_txn_at=NOW() WHERE owner_type='USER' AND owner_id=$1`, userID, points+bonus); err != nil {
		return false, err
	}

	// 步6：积分流水（CREDIT + PURCHASE_POINTS_PACKAGE + 首购加赠独立流水）
	if _, err := tx.Exec(ctx, `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
			balance_after, related_order_id, operator_user_id, description, idempotency_key)
		SELECT pa.id, 'USER', $1, 'PURCHASE_POINTS_PACKAGE', 'CREDIT', $2, pa.current_balance, $3, $1,
		       '支付宝充值', $4
		FROM point_accounts pa WHERE pa.owner_type='USER' AND pa.owner_id=$1`,
		userID, points, oid, fmt.Sprintf("alipay-%d", oid)); err != nil {
		return false, err
	}
	if firstPurchase && bonus > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
				balance_after, related_order_id, operator_user_id, description, idempotency_key)
			SELECT pa.id, 'USER', $1, 'FIRST_PURCHASE_LIMITED_BONUS', 'CREDIT', $2, pa.current_balance, $3, $1,
			       '首购限时加赠', $4
			FROM point_accounts pa WHERE pa.owner_type='USER' AND pa.owner_id=$1`,
			userID, bonus, oid, fmt.Sprintf("first-bonus-%d", oid)); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// LogCallback 记录回调日志（H01 生产加固）。
func (s *OrderStore) LogCallback(ctx context.Context, params map[string]string, step int, result, failReason string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pay_callback_logs (params_snapshot, order_no, pay_transaction_id, verification_step, result, fail_reason)
		VALUES ($1::jsonb, $2, $3, $4, $5, $6)`,
		mapToJSON(params), params["out_trade_no"], params["trade_no"], step, result, failReason)
	return err
}

func mapToJSON(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func mustParseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}