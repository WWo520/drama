// order_store 实现 billing.OrderStore：订单幂等创建 + 支付回调事务（PAID+积分+流水原子）。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
// 订阅订单：packageID>0 且 days>0（写 package_id + subscription_days_granted）；积分包传 0,0。
func (s *OrderStore) CreateOrder(ctx context.Context, userID int64, orderType, planCode, amountYuan string, points int, idemKey string, packageID int64, days int) (int64, string, error) {
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
		INSERT INTO orders (order_no, user_id, order_type, plan_code, package_id, original_amount_yuan, discount_amount_yuan,
			paid_amount_yuan, pay_status, points_granted_total, subscription_days_granted, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $6, 'PENDING', $7, $8, $9)`,
		orderNo, userID, orderType, planCode, nullableID(packageID), paid, points, days, idemKey)
	if ierr != nil {
		return 0, "", ierr
	}
	var oid int64
	s.pool.QueryRow(ctx, `SELECT id FROM orders WHERE order_no=$1`, orderNo).Scan(&oid)
	return oid, orderNo, nil
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func (s *OrderStore) GetOrderByNo(ctx context.Context, orderNo string) (*billing.Order, error) {
	var o billing.Order
	err := s.pool.QueryRow(ctx, `
		SELECT id, order_no, user_id, order_type, plan_code, paid_amount_yuan::text, points_granted_total,
		       pay_status::text, COALESCE(pay_transaction_id,''), COALESCE(package_id,0),
		       COALESCE(subscription_days_granted,0)
		FROM orders WHERE order_no=$1`, orderNo).
		Scan(&o.ID, &o.OrderNo, &o.UserID, &o.OrderType, &o.PlanCode, &o.AmountYuan, &o.Points,
			&o.PayStatus, &o.PayTransactionID, &o.PackageID, &o.DaysGranted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("订单不存在")
	}
	return &o, err
}

// GetOrderByNoForUser 查订单（归属校验：仅订单所有者可见，防水平越权）。
func (s *OrderStore) GetOrderByNoForUser(ctx context.Context, orderNo string, userID int64) (*billing.Order, error) {
	var o billing.Order
	err := s.pool.QueryRow(ctx, `
		SELECT id, order_no, user_id, order_type, plan_code, paid_amount_yuan::text, points_granted_total,
		       pay_status::text, COALESCE(pay_transaction_id,'')
		FROM orders WHERE order_no=$1 AND user_id=$2`, orderNo, userID).
		Scan(&o.ID, &o.OrderNo, &o.UserID, &o.OrderType, &o.PlanCode, &o.AmountYuan, &o.Points,
			&o.PayStatus, &o.PayTransactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("订单不存在")
	}
	return &o, err
}

// GetPackage 查积分套餐（档位校验）。
func (s *OrderStore) GetPackage(ctx context.Context, pkgCode string) (*billing.Package, error) {
	var p billing.Package
	err := s.pool.QueryRow(ctx, `
		SELECT pkg_code, points, COALESCE(bonus_points,0), price_yuan::text, status::text
		FROM point_packages WHERE pkg_code=$1 AND deleted_at IS NULL`, pkgCode).
		Scan(&p.PkgCode, &p.Points, &p.BonusPoints, &p.PriceYuan, &p.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("套餐不存在")
	}
	return &p, err
}

// GetPlanPackage 查订阅套餐（packages 表，档位校验用）。
func (s *OrderStore) GetPlanPackage(ctx context.Context, code string) (*billing.PlanPackage, error) {
	var p billing.PlanPackage
	err := s.pool.QueryRow(ctx, `
		SELECT id, package_code, COALESCE(name,''), COALESCE(billing_cycle,'monthly'),
		       price_yuan::text, COALESCE(points_quota,0), COALESCE(status,'OFF')
		FROM packages WHERE package_code=$1 AND deleted_at IS NULL`, code).
		Scan(&p.ID, &p.Code, &p.Name, &p.Cycle, &p.PriceYuan, &p.PointsQuota, &p.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("套餐不存在")
	}
	return &p, err
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
	var oid, userID, packageID int64
	var payStatus, orderType, planCode string
	var daysGranted int
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, pay_status::text, order_type, COALESCE(plan_code,''),
		       COALESCE(package_id,0), COALESCE(subscription_days_granted,0)
		FROM orders WHERE order_no=$1 FOR UPDATE`, orderNo).
		Scan(&oid, &userID, &payStatus, &orderType, &planCode, &packageID, &daysGranted)
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

	// 订阅订单：同一事务内开通/续期会员
	isSubscription := strings.HasPrefix(orderType, "SUBSCRIPTION_")
	if isSubscription {
		if err := activateMembership(ctx, tx, userID, packageID, planCode, daysGranted, oid); err != nil {
			return false, err
		}
	}

	// 首购判定（03 E.6.2：历史 PAID 订单数=0；加赠 50% 为 LIMITED_TIME 30 天；订阅订单不参与加赠）
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
	if firstPurchase && !isSubscription {
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

// QueryMembership 查询用户当前 ACTIVE 会员（订阅订单状态轮询附带）。
func (s *OrderStore) QueryMembership(ctx context.Context, userID int64, planCode, status, renew, endAt *string) error {
	return s.pool.QueryRow(ctx, `
		SELECT package_code::text, status::text, COALESCE(renew_strategy::text,''),
		       COALESCE(to_char(current_period_end AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),'')
		FROM user_memberships WHERE user_id=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1`, userID).
		Scan(planCode, status, renew, endAt)
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

// activateMembership 订阅开通/续期（回调事务内）：
// 有 ACTIVE 会员 → 顺延 current_period_end；无 → 新建 ACTIVE 记录；回填订单 related_membership_id。
func activateMembership(ctx context.Context, tx pgx.Tx, userID, packageID int64, planCode string, days int, orderID int64) error {
	if packageID == 0 || days <= 0 {
		return fmt.Errorf("订阅订单缺少套餐或天数")
	}
	var mid int64
	var endAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT id, current_period_end FROM user_memberships
		WHERE user_id=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1 FOR UPDATE`, userID).Scan(&mid, &endAt)
	if err == nil {
		// 续期：从当前到期日顺延（已过期则从现在起算）
		base := endAt
		if base.Before(time.Now()) {
			base = time.Now()
		}
		if _, uerr := tx.Exec(ctx, `
			UPDATE user_memberships SET current_period_end=$2, total_cycles_paid=total_cycles_paid+1,
			       last_renew_order_id=$3, updated_at=NOW()
			WHERE id=$1 AND status='ACTIVE'`, mid, base.AddDate(0, 0, days), orderID); uerr != nil {
			return uerr
		}
		if _, uerr := tx.Exec(ctx, `UPDATE orders SET related_membership_id=$2 WHERE id=$1`, orderID, mid); uerr != nil {
			return uerr
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// 新建 ACTIVE 会员
	if err := tx.QueryRow(ctx, `
		INSERT INTO user_memberships (user_id, package_id, package_code, status, renew_strategy,
			start_at, current_period_end, total_cycles_paid, last_renew_order_id)
		VALUES ($1, $2, $3, 'ACTIVE', 'MANUAL_ONE_TIME', NOW(), NOW() + make_interval(days => $4), 1, $5)
		RETURNING id`,
		userID, packageID, planCode, days, orderID).Scan(&mid); err != nil {
		return err
	}
	if _, uerr := tx.Exec(ctx, `UPDATE orders SET related_membership_id=$2 WHERE id=$1`, orderID, mid); uerr != nil {
		return uerr
	}
	return nil
}