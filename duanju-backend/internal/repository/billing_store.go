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

// FreezeAtomic 单事务冻结（08 审查 Major#10）：任务幂等检查 + 账户 FOR UPDATE + 余额守卫 + 快照固化 + 冻结流水，全部同事务。
func (s *BillingStore) FreezeAtomic(ctx context.Context, ownerType string, ownerID, taskID int64, amount int, meta *billing.TxnMeta) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// ① 任务幂等：已冻结直接跳过（FOR UPDATE 锁任务行，防并发双冻结 TOCTOU）
	var frozen int
	if err := tx.QueryRow(ctx, `SELECT frozen_points FROM generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&frozen); err != nil {
		return false, fmt.Errorf("任务不存在 %d", taskID)
	}
	if frozen > 0 {
		return true, tx.Commit(ctx)
	}
	// ② 锁账户 + 余额守卫
	var acc billing.Account
	if err := tx.QueryRow(ctx, `
		SELECT id, owner_type::text, owner_id, current_balance, frozen_balance
		FROM point_accounts WHERE owner_type=$1 AND owner_id=$2 FOR UPDATE`,
		ownerType, ownerID).Scan(&acc.ID, &acc.OwnerType, &acc.OwnerID, &acc.CurrentBalance, &acc.FrozenBalance); err != nil {
		return false, fmt.Errorf("积分账户不存在 owner=%s/%d", ownerType, ownerID)
	}
	if acc.CurrentBalance < amount {
		return false, fmt.Errorf("%w: 需 %d 积分，当前 %d", billing.ErrInsufficient, amount, acc.CurrentBalance)
	}
	// ③ 冻结
	if _, err := tx.Exec(ctx, `
		UPDATE point_accounts SET current_balance=current_balance-$2, frozen_balance=frozen_balance+$2,
		       last_txn_at=NOW() WHERE id=$1 AND current_balance>=$2`, acc.ID, amount); err != nil {
		return false, err
	}
	acc.CurrentBalance -= amount
	acc.FrozenBalance += amount
	// ④ 固化任务计费快照
	if _, err := tx.Exec(ctx, `
		UPDATE generation_tasks SET charged_account_id=$2, charged_owner_type=$3, charged_owner_id=$4,
		       frozen_points=$5, thawed_points=0, is_success_refunded=FALSE
		WHERE id=$1`, taskID, acc.ID, ownerType, ownerID, amount); err != nil {
		return false, err
	}
	// ⑤ 冻结流水
	if err := s.writeTxn(ctx, tx, &acc, "BATCH_GEN_FREEZE", "FROZEN", amount, taskID, meta,
		fmt.Sprintf("freeze-%d", taskID)); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

// SettleAtomic 单事务结算（08 审查 Major#11）：MarkTaskSettled CAS 与扣费/解冻同事务；actualCost<冻结额时解冻差额。
func (s *BillingStore) SettleAtomic(ctx context.Context, taskID int64, deliverable bool, actualCost int, meta *billing.TxnMeta) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// ① CAS 标记已结算（同事务，失败即整体回滚，资金不动）
	tag, err := tx.Exec(ctx, `
		UPDATE generation_tasks SET is_success_refunded=TRUE, thawed_points=frozen_points
		WHERE id=$1 AND is_success_refunded=FALSE`, taskID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		_ = tx.Rollback(ctx)
		return true, nil // 已结算，幂等
	}
	// ② 读冻结快照
	var accountID, ownerID, frozenPoints int64
	var ownerType string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(charged_account_id,0), COALESCE(charged_owner_type::text,'USER'),
		       COALESCE(charged_owner_id,0), frozen_points
		FROM generation_tasks WHERE id=$1`, taskID).Scan(&accountID, &ownerType, &ownerID, &frozenPoints); err != nil {
		return false, err
	}
	// 不计费任务（无冻结）：仅标记结算
	if accountID == 0 || frozenPoints == 0 {
		return false, tx.Commit(ctx)
	}
	// ③ 锁账户
	var acc billing.Account
	if err := tx.QueryRow(ctx, `
		SELECT id, owner_type::text, owner_id, current_balance, frozen_balance
		FROM point_accounts WHERE id=$1 FOR UPDATE`, accountID).
		Scan(&acc.ID, &acc.OwnerType, &acc.OwnerID, &acc.CurrentBalance, &acc.FrozenBalance); err != nil {
		return false, fmt.Errorf("积分账户不存在 id=%d", accountID)
	}
	if deliverable {
		cost := int(frozenPoints)
		if actualCost > 0 && actualCost < cost {
			cost = actualCost
		}
		// ④a 冻结转实扣
		if _, err := tx.Exec(ctx, `
			UPDATE point_accounts SET frozen_balance=frozen_balance-$2, lifetime_spent_total=lifetime_spent_total+$2,
			       total_generation_count=total_generation_count+1, last_txn_at=NOW()
			WHERE id=$1 AND frozen_balance>=$2`, acc.ID, cost); err != nil {
			return false, err
		}
		acc.FrozenBalance -= cost
		// 差额解冻（actualCost<冻结额）
		if remainder := int(frozenPoints) - cost; remainder > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE point_accounts SET frozen_balance=frozen_balance-$2, current_balance=current_balance+$2,
				       last_txn_at=NOW() WHERE id=$1 AND frozen_balance>=$2`, acc.ID, remainder); err != nil {
				return false, err
			}
			acc.FrozenBalance -= remainder
			acc.CurrentBalance += remainder
			if err := s.writeTxn(ctx, tx, &acc, "UNFREEZE_PARTIAL_FAIL", "UNFREEZE", remainder, taskID, meta,
				fmt.Sprintf("unfreeze-remainder-%d", taskID)); err != nil {
				return false, err
			}
		}
		if err := s.writeTxn(ctx, tx, &acc, "UNFREEZE_SUCCESS_DEDUCT", "DEBIT", cost, taskID, meta,
			fmt.Sprintf("deduct-%d", taskID)); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing_records (billing_account_id, billed_owner_type, billed_owner_id, operator_user_id,
				user_id, space_id, task_id, cost_points, point_txn_id)
			SELECT $1, $2, $3, $4, $4, $5, $6, $7, id FROM point_transactions WHERE idempotency_key=$8
			ON CONFLICT (task_id) DO NOTHING`,
			acc.ID, acc.OwnerType, acc.OwnerID, meta.OperatorUserID, meta.SpaceID, taskID, cost,
			fmt.Sprintf("deduct-%d", taskID)); err != nil {
			return false, err
		}
	} else {
		// ④b 失败/取消：全额解冻（永不倒扣）
		if _, err := tx.Exec(ctx, `
			UPDATE point_accounts SET frozen_balance=frozen_balance-$2, current_balance=current_balance+$2,
			       last_txn_at=NOW() WHERE id=$1 AND frozen_balance>=$2`, acc.ID, frozenPoints); err != nil {
			return false, err
		}
		acc.FrozenBalance -= int(frozenPoints)
		acc.CurrentBalance += int(frozenPoints)
		if err := s.writeTxn(ctx, tx, &acc, "UNFREEZE_PARTIAL_FAIL", "UNFREEZE", int(frozenPoints), taskID, meta,
			fmt.Sprintf("unfreeze-%d", taskID)); err != nil {
			return false, err
		}
	}
	return false, tx.Commit(ctx)
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
	return s.GetAccount(ctx, "USER", userID)
}

// GetAccount 无锁读取账户（只读场景，避免行锁阻塞扣费）。
func (s *BillingStore) GetAccount(ctx context.Context, ownerType string, ownerID int64) (*billing.Account, error) {
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