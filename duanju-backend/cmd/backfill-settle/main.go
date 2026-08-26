// 回填工具：结算历史卡死的冻结积分（point_transactions.operator_user_id 外键 bug 遗留）。
// 根因：worker Settler 曾漏填 TxnMeta.OperatorUserID → 结算事务整体回滚 → 任务
// is_success_refunded 恒 FALSE、账户 frozen_balance 永久虚高。本工具按引擎同一
// 原子语义（SettleAtomic：CAS + 扣费/解冻同事务）逐任务补结算，幂等可重跑。
// 用法：DATABASE_URL=... go run ./cmd/backfill-settle [-dry]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/repository"
)

func main() {
	dry := flag.Bool("dry", false, "只列出待结算任务，不执行")
	repair := flag.Bool("repair", false, "修复已标记结算但缺扣费流水的历史任务（原子化改造前老 bug 遗留）")
	flag.Parse()

	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Println("pool err:", err)
		os.Exit(1)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if *repair {
		repairLedger(ctx, pool, *dry)
		return
	}

	rows, err := pool.Query(ctx, `
		SELECT id, COALESCE(user_id,0), COALESCE(space_id,0), project_id, frozen_points, status::text
		FROM generation_tasks
		WHERE frozen_points > 0 AND is_success_refunded = FALSE
		  AND status::text IN ('SUCCESS','FAILED_DEAD','CANCELED_USER','CANCELED_TIMEOUT','SKIPPED')
		ORDER BY id`)
	if err != nil {
		fmt.Println("query err:", err)
		os.Exit(1)
	}
	type item struct {
		id                      int64
		userID, spaceID, frozen int64
		status                  string
		projectID               *int64
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.userID, &it.spaceID, &it.projectID, &it.frozen, &it.status); err != nil {
			fmt.Println("scan err:", err)
			os.Exit(1)
		}
		items = append(items, it)
	}
	rows.Close()
	fmt.Printf("待结算任务 %d 个\n", len(items))
	if *dry {
		for _, it := range items {
			fmt.Printf("  task=%d user=%d frozen=%d status=%s\n", it.id, it.userID, it.frozen, it.status)
		}
		return
	}

	engine := billing.NewEngine(repository.NewBillingStore(pool))
	ok, skipped, failed := 0, 0, 0
	for _, it := range items {
		meta := &billing.TxnMeta{SpaceID: it.spaceID, OperatorUserID: it.userID, ProjectID: it.projectID}
		err := engine.Settle(ctx, it.id, it.status == "SUCCESS", 0, meta)
		switch {
		case errors.Is(err, billing.ErrAlreadySettled):
			skipped++
		case err != nil:
			failed++
			fmt.Printf("  task=%d 结算失败: %v\n", it.id, err)
		default:
			ok++
			fmt.Printf("  task=%d user=%d frozen=%d status=%s → 已结算（%s）\n",
				it.id, it.userID, it.frozen, it.status, map[bool]string{true: "实扣", false: "全额解冻"}[it.status == "SUCCESS"])
		}
	}
	fmt.Printf("完成：成功 %d、幂等跳过 %d、失败 %d\n", ok, skipped, failed)
}

// repairLedger 补记"已标记结算但缺扣费/解冻流水"的历史任务（SettleAtomic 原子化
// 改造前的老 bug：CAS 先提交、扣费失败不回滚 → is_success_refunded=TRUE 但资金未动）。
// 仅处理 frozen_balance>0 的账户（资金已人工修平的账户跳过，避免重复扣成负数）。
// 幂等：流水 idempotency_key 与引擎结算一致（deduct-{id}/unfreeze-{id}）。
func repairLedger(ctx context.Context, pool *pgxpool.Pool, dry bool) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, t.charged_account_id, COALESCE(t.user_id,0), COALESCE(t.space_id,0),
		       COALESCE(t.project_id,0), t.frozen_points, t.status::text
		FROM generation_tasks t
		JOIN point_accounts pa ON pa.id = t.charged_account_id AND pa.frozen_balance > 0
		WHERE t.frozen_points > 0 AND t.is_success_refunded = TRUE
		  AND NOT EXISTS (SELECT 1 FROM point_transactions pt
		                  WHERE pt.related_generation_task_id = t.id
		                    AND pt.direction IN ('DEBIT','UNFREEZE'))
		ORDER BY t.charged_account_id, t.id`)
	if err != nil {
		fmt.Println("repair query err:", err)
		os.Exit(1)
	}
	type ritem struct {
		id, acc, user, space, frozen int64
		project                      int64
		status                       string
	}
	var items []ritem
	for rows.Next() {
		var it ritem
		if err := rows.Scan(&it.id, &it.acc, &it.user, &it.space, &it.project, &it.frozen, &it.status); err != nil {
			fmt.Println("repair scan err:", err)
			os.Exit(1)
		}
		items = append(items, it)
	}
	rows.Close()
	fmt.Printf("补流水候选任务 %d 个\n", len(items))
	if dry {
		for _, it := range items {
			fmt.Printf("  task=%d acc=%d user=%d frozen=%d status=%s\n", it.id, it.acc, it.user, it.frozen, it.status)
		}
		return
	}
	ok, failed := 0, 0
	for _, it := range items {
		if err := repairOne(ctx, pool, it); err != nil {
			failed++
			fmt.Printf("  task=%d 补流水失败: %v\n", it.id, err)
			continue
		}
		ok++
		fmt.Printf("  task=%d acc=%d frozen=%d status=%s → 已补记（%s）\n",
			it.id, it.acc, it.frozen, it.status, map[bool]string{true: "实扣", false: "全额解冻"}[it.status == "SUCCESS"])
	}
	fmt.Printf("补流水完成：成功 %d、失败 %d\n", ok, failed)
}

// repairOne 单任务补记（一事务）：资金变动 + DEBIT/UNFREEZE 流水 + billing_records。
func repairOne(ctx context.Context, pool *pgxpool.Pool, it struct {
	id, acc, user, space, frozen int64
	project                      int64
	status                       string
}) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var ownerType string
	var ownerID, cur, froz int64
	if err := tx.QueryRow(ctx, `
		SELECT owner_type::text, owner_id, current_balance, frozen_balance
		FROM point_accounts WHERE id=$1 FOR UPDATE`, it.acc).
		Scan(&ownerType, &ownerID, &cur, &froz); err != nil {
		return err
	}
	if froz < it.frozen {
		return fmt.Errorf("账户 %d 冻结余额不足（%d < %d），疑似已人工修平，跳过", it.acc, froz, it.frozen)
	}

	var idemKey, txnType, dir string
	if it.status == "SUCCESS" { // 冻结转实扣（对齐 SettleAtomic ④a）
		if _, err := tx.Exec(ctx, `
			UPDATE point_accounts SET frozen_balance=frozen_balance-$2,
			       lifetime_spent_total=lifetime_spent_total+$2,
			       total_generation_count=total_generation_count+1, last_txn_at=NOW()
			WHERE id=$1 AND frozen_balance>=$2`, it.acc, it.frozen); err != nil {
			return err
		}
		idemKey, txnType, dir = fmt.Sprintf("deduct-%d", it.id), "UNFREEZE_SUCCESS_DEDUCT", "DEBIT"
	} else { // 失败全额解冻（对齐 ④b）
		if _, err := tx.Exec(ctx, `
			UPDATE point_accounts SET frozen_balance=frozen_balance-$2, current_balance=current_balance+$2,
			       last_txn_at=NOW() WHERE id=$1 AND frozen_balance>=$2`, it.acc, it.frozen); err != nil {
			return err
		}
		idemKey, txnType, dir = fmt.Sprintf("unfreeze-%d", it.id), "UNFREEZE_PARTIAL_FAIL", "UNFREEZE"
	}

	// 流水（balance_after/frozen_after 按变动后快照；operator_user_id 必填有效用户）
	var balAfter, frozAfter int64
	if err := tx.QueryRow(ctx, `SELECT current_balance, frozen_balance FROM point_accounts WHERE id=$1`, it.acc).
		Scan(&balAfter, &frozAfter); err != nil {
		return err
	}
	var projectID *int64
	if it.project > 0 {
		p := it.project
		projectID = &p
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
			balance_after, frozen_after, related_generation_task_id, related_space_id, operator_user_id,
			related_project_id, description, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		it.acc, ownerType, ownerID, txnType, dir, it.frozen,
		balAfter, frozAfter, it.id, it.space, it.user, projectID,
		"历史结算补记（backfill-settle）", idemKey); err != nil {
		return err
	}
	if it.status == "SUCCESS" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing_records (billing_account_id, billed_owner_type, billed_owner_id, operator_user_id,
				user_id, space_id, task_id, cost_points, point_txn_id)
			SELECT $1,$2,$3,$4,$4,$5,$6,$7,id FROM point_transactions WHERE idempotency_key=$8
			ON CONFLICT (task_id) DO NOTHING`,
			it.acc, ownerType, ownerID, it.user, it.space, it.id, it.frozen, idemKey); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
