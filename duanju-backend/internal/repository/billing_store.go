// billing_store 实现 billing.BillStore：三阶段扣费事务（FOR UPDATE + balance_after 快照 + UNIQUE 幂等）。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/billing"
)

// BillingStore 计费数据访问。
type BillingStore struct {
	pool *pgxpool.Pool
}

func NewBillingStore(pool *pgxpool.Pool) *BillingStore {
	return &BillingStore{pool: pool}
}

var _ billing.BillStore = (*BillingStore)(nil)

func (s *BillingStore) GetAccountForUpdate(ctx context.Context, ownerType string, ownerID int64) (*billing.Account, error) {
	var a billing.Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, owner_type::text, owner_id, current_balance, frozen_balance
		FROM point_accounts WHERE owner_type=$1 AND owner_id=$2 FOR UPDATE`,
		ownerType, ownerID).Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.CurrentBalance, &a.FrozenBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("积分账户不存在 owner=%s/%d", ownerType, ownerID)
	}
	return &a, err
}

func (s *BillingStore) SaveTaskBill(ctx context.Context, taskID int64, bill *billing.TaskBill) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE generation_tasks SET charged_account_id=$2, charged_owner_type=$3, charged_owner_id=$4,
		       frozen_points=$5, thawed_points=0, is_success_refunded=FALSE
		WHERE id=$1`, taskID, bill.AccountID, bill.OwnerType, bill.OwnerID, bill.FrozenPoints)
	return err
}

func (s *BillingStore) LoadTaskBill(ctx context.Context, taskID int64) (*billing.TaskBill, error) {
	var b billing.TaskBill
	err := s.pool.QueryRow(ctx, `
		SELECT t.id, COALESCE(t.charged_account_id,0), COALESCE(t.charged_owner_type::text,'USER'),
		       COALESCE(t.charged_owner_id,0), t.space_id, t.user_id, t.project_id, t.frozen_points
		FROM generation_tasks t WHERE t.id=$1`, taskID).
		Scan(&b.TaskID, &b.AccountID, &b.OwnerType, &b.OwnerID, &b.SpaceID, &b.OperatorUserID, &b.ProjectID, &b.FrozenPoints)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("任务不存在 %d", taskID)
	}
	return &b, err
}

func (s *BillingStore) IsTaskSettled(ctx context.Context, taskID int64) (bool, error) {
	var settled bool
	err := s.pool.QueryRow(ctx, `SELECT is_success_refunded FROM generation_tasks WHERE id=$1`, taskID).Scan(&settled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("任务不存在 %d", taskID)
	}
	return settled, err
}

func (s *BillingStore) MarkTaskSettled(ctx context.Context, taskID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE generation_tasks SET is_success_refunded=TRUE, thawed_points=frozen_points
		WHERE id=$1 AND is_success_refunded=FALSE`, taskID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// writeTxn 通用流水写入（balance_after 快照 + idempotency_key UNIQUE 防重）。
func (s *BillingStore) writeTxn(ctx context.Context, tx pgx.Tx, a *billing.Account, txnType, direction string, amount int, taskID int64, meta *billing.TxnMeta, idemKey string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
			balance_after, frozen_after, related_generation_task_id, related_space_id, operator_user_id,
			related_project_id, description, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		a.ID, a.OwnerType, a.OwnerID, txnType, direction, amount,
		a.CurrentBalance, a.FrozenBalance, taskID, meta.SpaceID, meta.OperatorUserID, meta.ProjectID,
		meta.Description, idemKey)
	return err
}

func (s *BillingStore) Freeze(ctx context.Context, a *billing.Account, amount int, taskID int64, meta *billing.TxnMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE point_accounts SET current_balance=current_balance-$2, frozen_balance=frozen_balance+$2,
		       last_txn_at=NOW() WHERE id=$1`, a.ID, amount); err != nil {
		return err
	}
	a.CurrentBalance -= amount
	a.FrozenBalance += amount
	if err := s.writeTxn(ctx, tx, a, "BATCH_GEN_FREEZE", "FROZEN", amount, taskID, meta,
		fmt.Sprintf("freeze-%d", taskID)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *BillingStore) Deduct(ctx context.Context, a *billing.Account, amount int, taskID int64, meta *billing.TxnMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE point_accounts SET frozen_balance=frozen_balance-$2, lifetime_spent_total=lifetime_spent_total+$2,
		       total_generation_count=total_generation_count+1, last_txn_at=NOW() WHERE id=$1`, a.ID, amount); err != nil {
		return err
	}
	a.FrozenBalance -= amount
	if err := s.writeTxn(ctx, tx, a, "UNFREEZE_SUCCESS_DEDUCT", "DEBIT", amount, taskID, meta,
		fmt.Sprintf("deduct-%d", taskID)); err != nil {
		return err
	}
	// billing_records（任务唯一）
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing_records (billing_account_id, billed_owner_type, billed_owner_id, operator_user_id,
			user_id, space_id, task_id, cost_points, point_txn_id)
		SELECT $1, $2, $3, $4, $4, $5, $6, $7, id FROM point_transactions WHERE idempotency_key=$8
		ON CONFLICT (task_id) DO NOTHING`,
		a.ID, a.OwnerType, a.OwnerID, meta.OperatorUserID, meta.SpaceID, taskID, amount,
		fmt.Sprintf("deduct-%d", taskID)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *BillingStore) Unfreeze(ctx context.Context, a *billing.Account, amount int, taskID int64, meta *billing.TxnMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE point_accounts SET frozen_balance=frozen_balance-$2, current_balance=current_balance+$2,
		       last_txn_at=NOW() WHERE id=$1`, a.ID, amount); err != nil {
		return err
	}
	a.FrozenBalance -= amount
	a.CurrentBalance += amount
	if err := s.writeTxn(ctx, tx, a, "UNFREEZE_PARTIAL_FAIL", "UNFREEZE", amount, taskID, meta,
		fmt.Sprintf("unfreeze-%d", taskID)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---- 积分 API ----

// GetPoints 用户积分账户（9.3）。
func (s *BillingStore) GetPoints(ctx context.Context, userID int64) (*billing.Account, error) {
	return s.GetAccountForUpdateNoLock(ctx, "USER", userID)
}

// GetAccountForUpdateNoLock 无锁读取（查询场景）。
func (s *BillingStore) GetAccountForUpdateNoLock(ctx context.Context, ownerType string, ownerID int64) (*billing.Account, error) {
	var a billing.Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, owner_type::text, owner_id, current_balance, frozen_balance
		FROM point_accounts WHERE owner_type=$1 AND owner_id=$2`,
		ownerType, ownerID).Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.CurrentBalance, &a.FrozenBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("积分账户不存在")
	}
	return &a, err
}

// ListTransactions 积分流水（9.4）。
func (s *BillingStore) ListTransactions(ctx context.Context, accountID int64, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM point_transactions WHERE account_id=$1`, accountID).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT txn_type::text, direction::text, amount, balance_after, frozen_after, description, created_at
		FROM point_transactions WHERE account_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		accountID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var txnType, dir, desc string
		var amount, balance, frozen int
		var createdAt time.Time
		rows.Scan(&txnType, &dir, &amount, &balance, &frozen, &desc, &createdAt)
		out = append(out, map[string]any{
			"txn_type": txnType, "direction": dir, "amount": amount,
			"balance_after": balance, "frozen_after": frozen, "description": desc,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}
	return out, total, rows.Err()
}