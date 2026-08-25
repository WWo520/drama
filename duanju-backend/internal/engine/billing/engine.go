// Package billing 计费引擎（E5）：三阶段扣费（冻结→扣费/解冻）全原子。
// 对齐 01 C.2.1 / 04 H.3 / 03 E.6.4（扣费铁律 6 条）。balance_after 每行快照，UNIQUE 防重复。
package billing

import (
	"context"
	"errors"
	"fmt"
)

// ErrInsufficient 积分不足（40003）。
var ErrInsufficient = errors.New("insufficient points")

// ErrAlreadySettled 任务已结算（重复回调/乱序，幂等拒绝）。
var ErrAlreadySettled = errors.New("task already settled")

// Account 账户快照。
type Account struct {
	ID            int64
	OwnerType     string // USER / SPACE
	OwnerID       int64
	CurrentBalance int
	FrozenBalance  int
}

// TaskBill 任务计费快照（generation_tasks 冻结时固化）。
type TaskBill struct {
	TaskID           int64
	AccountID        int64
	OwnerType        string
	OwnerID          int64
	SpaceID          int64
	OperatorUserID   int64
	ProjectID        *int64
	FrozenPoints     int
	CrossEpisodeExempt int
	IdempotencyKey   string
}

// BillStore 计费数据边界（repository 实现）。
type BillStore interface {
	// GetAccountForUpdate 锁定账户（FOR UPDATE）。
	GetAccountForUpdate(ctx context.Context, ownerType string, ownerID int64) (*Account, error)
	// GetAccount 无锁查询账户（只读场景，避免行锁阻塞扣费）。
	GetAccount(ctx context.Context, ownerType string, ownerID int64) (*Account, error)
	// Freeze 冻结：current−=N, frozen+=N + FROZEN 流水（事务内）。
	Freeze(ctx context.Context, a *Account, amount int, taskID int64, meta *TxnMeta) error
	// Deduct 扣费：frozen−=N + DEBIT 流水 + billing_records（事务内；UNIQUE 防重）。
	Deduct(ctx context.Context, a *Account, amount int, taskID int64, meta *TxnMeta) error
	// Unfreeze 解冻：frozen−=N, current+=N + UNFREEZE 流水（事务内）。
	Unfreeze(ctx context.Context, a *Account, amount int, taskID int64, meta *TxnMeta) error
	// SaveTaskBill 固化任务计费快照（generation_tasks 三字段 + charged_*）。
	SaveTaskBill(ctx context.Context, taskID int64, bill *TaskBill) error
	// LoadTaskBill 读取任务计费快照。
	LoadTaskBill(ctx context.Context, taskID int64) (*TaskBill, error)
	// MarkTaskSettled 标记任务已结算（幂等 CAS：is_success_refunded）。
	MarkTaskSettled(ctx context.Context, taskID int64) (bool, error)
	// IsTaskSettled 检查是否已结算。
	IsTaskSettled(ctx context.Context, taskID int64) (bool, error)
	// FreezeAtomic 单事务冻结：任务幂等检查 + 账户行锁 + 余额守卫 + 快照固化 + 冻结流水（08 审查 Major#10）。
	// alreadyFrozen=true 表示任务已冻结（幂等跳过）。
	FreezeAtomic(ctx context.Context, ownerType string, ownerID, taskID int64, amount int, meta *TxnMeta) (alreadyFrozen bool, err error)
	// SettleAtomic 单事务结算：MarkTaskSettled CAS + 扣费/解冻同事务（08 审查 Major#11）。
	// alreadySettled=true 表示已结算（幂等拒绝）；actualCost>0 且小于冻结额时按实际成本扣费并解冻差额。
	SettleAtomic(ctx context.Context, taskID int64, deliverable bool, actualCost int, meta *TxnMeta) (alreadySettled bool, err error)
	// ListTransactions 积分流水（分页）。
	ListTransactions(ctx context.Context, accountID int64, page, pageSize int) ([]map[string]any, int, error)
}

// TxnMeta 流水元数据（operator/space/project 快照）。
type TxnMeta struct {
	SpaceID        int64
	OperatorUserID int64
	ProjectID      *int64
	Description    string
}

// Engine 计费引擎。
type Engine struct {
	store BillStore
}

func NewEngine(store BillStore) *Engine {
	return &Engine{store: store}
}

// Freeze 阶段1 冻结（API 侧提交任务时）：单事务 任务幂等 → 账户行锁 → 余额守卫 → 双余额 → FROZEN 流水（08 审查 Major#10）。
// 幂等：任务已有冻结则跳过不重复冻结（04 H.3 提交冻结）。
func (e *Engine) Freeze(ctx context.Context, ownerType string, ownerID int64, taskID int64, amount int, meta *TxnMeta) error {
	if amount <= 0 {
		return fmt.Errorf("冻结金额必须 > 0")
	}
	_, err := e.store.FreezeAtomic(ctx, ownerType, ownerID, taskID, amount, meta)
	return err
}

// Settle 终态结算：任务结果可交付 → 冻结转实扣；不可交付 → 全额解冻（03 A.3 黄金规则①）。
// 幂等：MarkTaskSettled CAS 与资金变动同一事务（08 审查 Major#11），并发/重复回调安全。
func (e *Engine) Settle(ctx context.Context, taskID int64, deliverable bool, actualCost int, meta *TxnMeta) error {
	already, err := e.store.SettleAtomic(ctx, taskID, deliverable, actualCost, meta)
	if err != nil {
		return err
	}
	if already {
		return ErrAlreadySettled
	}
	return nil
}