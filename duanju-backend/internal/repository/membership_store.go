// membership_store 实现 creation.MembershipStore：会员购买/激活/取消/到期降级。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/domain/creation"
)

// MembershipStore 会员数据访问。
type MembershipStore struct {
	pool *pgxpool.Pool
}

func NewMembershipStore(pool *pgxpool.Pool) *MembershipStore {
	return &MembershipStore{pool: pool}
}

var _ creation.MembershipStore = (*MembershipStore)(nil)

func (s *MembershipStore) GetMembership(ctx context.Context, userID int64) (map[string]any, error) {
	var planCode, status, renew, endAt string
	err := s.pool.QueryRow(ctx, `
		SELECT package_code, status::text, COALESCE(renew_strategy::text,''), COALESCE(current_period_end::text,'')
		FROM user_memberships WHERE user_id=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1`, userID).
		Scan(&planCode, &status, &renew, &endAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{"plan_code": "BASIC", "status": "ACTIVE", "renew_strategy": "NONE"}, nil
	}
	if err != nil {
		return nil, err
	}
	// 到期自动降级（gocron 简化：查询时校验）
	if endAt != "" && endAt < time.Now().UTC().Format(time.RFC3339) {
		_, _ = s.pool.Exec(ctx, `
			UPDATE user_memberships SET status='EXPIRED' WHERE user_id=$1 AND status='ACTIVE'`, userID)
		return map[string]any{"plan_code": "BASIC", "status": "ACTIVE", "renew_strategy": "NONE"}, nil
	}
	return map[string]any{
		"plan_code": planCode, "status": status, "renew_strategy": renew, "expires_at": endAt,
	}, nil
}

func (s *MembershipStore) Purchase(ctx context.Context, userID int64, planCode, billingCycle, amountYuan string) (string, error) {
	days := 30
	if billingCycle == "ANNUAL" {
		days = 365
	}
	orderNo := fmt.Sprintf("M%s%s", time.Now().Format("20060102150405"), uuid.NewString()[:6])
	_, err := s.pool.Exec(ctx, `
		INSERT INTO orders (order_no, user_id, order_type, plan_code, original_amount_yuan,
			discount_amount_yuan, paid_amount_yuan, pay_status, subscription_days_granted, idempotency_key)
		VALUES ($1, $2, 'MEMBERSHIP', $3, $4, 0, $4, 'PENDING', $5, $6)`,
		orderNo, userID, planCode, amountYuan, days, "membership-"+uuid.NewString())
	if err != nil {
		return "", err
	}
	return orderNo, nil
}

func (s *MembershipStore) Activate(ctx context.Context, orderNo string, requesterID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var oid, userID int64
	var planCode string
	var days int
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT id, user_id, plan_code, subscription_days_granted, pay_status::text
		FROM orders WHERE order_no=$1::text FOR UPDATE`, orderNo).
		Scan(&oid, &userID, &planCode, &days, &status); err != nil {
		return err
	}
	// 归属校验（P0 修复：仅订单所有者可激活会员）
	if userID != requesterID {
		return errors.New("无权操作该订单")
	}
	if status == "PAID" {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET pay_status='PAID', paid_at=NOW() WHERE id=$1 AND pay_status='PENDING'`, oid); err != nil {
		return err
	}
	// 旧会员置 EXPIRED（升级场景）
	if _, err := tx.Exec(ctx, `
		UPDATE user_memberships SET status='EXPIRED' WHERE user_id=$1 AND status='ACTIVE'`, userID); err != nil {
		return err
	}
	// package_code 反查 package_id（表结构要求 package_id NOT NULL FK）；
	// 套餐码存 PACK_ 前缀（如 PACK_PRO），未命中时降级 PACK_FREE 避免激活失败
	var pkgID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM packages WHERE package_code='PACK_' || $1 AND status='ACTIVE' LIMIT 1`, planCode).
		Scan(&pkgID); errors.Is(err, pgx.ErrNoRows) {
		if err2 := tx.QueryRow(ctx, `
			SELECT id FROM packages WHERE package_code='PACK_FREE' LIMIT 1`).Scan(&pkgID); err2 != nil {
			return fmt.Errorf("未找到会员套餐 %s 且无免费兑底: %w", planCode, err2)
		}
	} else if err != nil {
		return fmt.Errorf("未找到会员套餐 %s: %w", planCode, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_memberships (user_id, package_id, package_code, status, renew_strategy, start_at, current_period_end, total_cycles_paid, last_renew_order_id)
		VALUES ($1, $2, $3, 'ACTIVE', 'AUTO_RENEW', NOW(), NOW() + ($4 || ' days')::interval, 1, $5)`,
		userID, pkgID, planCode, days, oid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *MembershipStore) Cancel(ctx context.Context, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE user_memberships SET cancel_at_period_end=TRUE
		WHERE user_id=$1 AND status='ACTIVE' AND renew_strategy='AUTO_RENEW'`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("无有效自动续费会员")
	}
	return nil
}