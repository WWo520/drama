// cmd/reconcile 对账脚本：账户轧差 + 任务计费一致性检查（S1-B12）。
// 每日凌晨 3 点由 cron 执行（05 工程流程 / 01 F.1 积分财务安全）；有差异输出明细并以非零退出码告警。
//
// 用法：DATABASE_URL=... go run ./cmd/reconcile
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/pkg/db"
	"github.com/duanju/backend/internal/pkg/logger"
)

// 对账结果汇总。
type report struct {
	AccountsChecked int
	AccountsDiff    []string
	TasksChecked    int
	TasksDiff       []string
}

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	lg := logger.New(cfg.App.Env)
	defer func() { _ = lg.Sync() }()

	pool, err := db.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		lg.Fatal("数据库连接失败", logger.F("err", err)...)
	}
	defer pool.Close()

	rep := &report{}

	// 检查1：账户轧差（余额 vs 流水）
	reconcileAccounts(ctx, pool, rep)

	// 检查2：任务计费一致性（终态任务应有扣费或解冻流水）
	reconcileTasks(ctx, pool, rep)

	// 输出报告
	fmt.Printf("对账完成：账户 %d 个（差异 %d），任务 %d 个（差异 %d）\n",
		rep.AccountsChecked, len(rep.AccountsDiff), rep.TasksChecked, len(rep.TasksDiff))
	for _, d := range rep.AccountsDiff {
		fmt.Println("[账户差异]", d)
	}
	for _, d := range rep.TasksDiff {
		fmt.Println("[任务差异]", d)
	}
	if len(rep.AccountsDiff) > 0 || len(rep.TasksDiff) > 0 {
		lg.Error("对账发现差异，需人工核查",
			logger.F("accounts_diff", len(rep.AccountsDiff), "tasks_diff", len(rep.TasksDiff))...)
		os.Exit(1)
	}
	fmt.Println("对账通过：无差异")
}

// reconcileAccounts 检查每个积分账户的余额与最近流水快照一致（balance_after 追溯）。
func reconcileAccounts(ctx context.Context, pool *pgxpool.Pool, rep *report) {
	rows, err := pool.Query(ctx, `
		SELECT a.id, a.owner_type::text, a.owner_id, a.current_balance, a.frozen_balance,
		       COALESCE((SELECT balance_after FROM point_transactions t
		                  WHERE t.account_id = a.id ORDER BY t.id DESC LIMIT 1), 0),
		       COALESCE((SELECT frozen_after FROM point_transactions t
		                  WHERE t.account_id = a.id ORDER BY t.id DESC LIMIT 1), 0)
		FROM point_accounts a`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "查询账户失败:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, ownerID int64
		var ownerType string
		var balance, frozen, lastBalance, lastFrozen int
		rows.Scan(&id, &ownerType, &ownerID, &balance, &frozen, &lastBalance, &lastFrozen)
		rep.AccountsChecked++
		// 轧差：账户余额应等于最近流水快照（无流水时余额应=0 或为初始发放后已入流水）
		if balance != lastBalance || frozen != lastFrozen {
			rep.AccountsDiff = append(rep.AccountsDiff,
				fmt.Sprintf("账户 %s:%d 余额(%d/%d) ≠ 流水快照(%d/%d)",
					ownerType, ownerID, balance, frozen, lastBalance, lastFrozen))
		}
	}
}

// reconcileTasks 检查终态任务（SUCCESS/FAILED_DEAD）的计费一致性：
// 成功任务应有 DEBIT 流水（frozen→spent）；失败死信应有 UNFREEZE 流水。
func reconcileTasks(ctx context.Context, pool *pgxpool.Pool, rep *report) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, t.task_uuid::text, t.status::text, t.frozen_points,
		       t.is_success_refunded,
		       EXISTS (SELECT 1 FROM point_transactions pt WHERE pt.related_generation_task_id=t.id AND pt.direction='DEBIT') AS has_debit,
		       EXISTS (SELECT 1 FROM point_transactions pt WHERE pt.related_generation_task_id=t.id AND pt.direction='UNFREEZE') AS has_unfreeze
		FROM generation_tasks t
		WHERE t.status IN ('SUCCESS','FAILED_DEAD','CANCELED_USER','CANCELED_TIMEOUT')
		  AND t.frozen_points > 0`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "查询任务失败:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var taskUUID, status string
		var frozen int
		var settled, hasDebit, hasUnfreeze bool
		rows.Scan(&id, &taskUUID, &status, &frozen, &settled, &hasDebit, &hasUnfreeze)
		rep.TasksChecked++
		if status == "SUCCESS" && !hasDebit {
			rep.TasksDiff = append(rep.TasksDiff,
				fmt.Sprintf("任务 %s SUCCESS 但无扣费流水（冻结 %d）", taskUUID, frozen))
		}
		if status != "SUCCESS" && !hasUnfreeze {
			rep.TasksDiff = append(rep.TasksDiff,
				fmt.Sprintf("任务 %s %s 无解冻流水（冻结 %d）", taskUUID, status, frozen))
		}
	}
}