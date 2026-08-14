// BillingEngine 测试：三阶段扣费时序 + 幂等防重 + 失败全额解冻（S1-B09 验收）。
package billing

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBillStore 内存计费存储。
type fakeBillStore struct {
	accounts map[string]*Account // ownerType:ownerID → account
	bills    map[int64]*TaskBill
	settled  map[int64]bool
	txns     []string // 流水记录（freeze/deduct/unfreeze）
}

func newFakeStore(balance int) *fakeBillStore {
	return &fakeBillStore{
		accounts: map[string]*Account{"USER:1": {ID: 1, OwnerType: "USER", OwnerID: 1, CurrentBalance: balance}},
		bills:    map[int64]*TaskBill{},
		settled:  map[int64]bool{},
	}
}

func (f *fakeBillStore) GetAccountForUpdate(_ context.Context, ownerType string, ownerID int64) (*Account, error) {
	a, ok := f.accounts[ownerType+":"+fmt.Sprint(ownerID)]
	if !ok {
		return nil, errors.New("no account")
	}
	return a, nil // 返回共享指针（模拟 FOR UPDATE 行锁语义）
}

func (f *fakeBillStore) Freeze(_ context.Context, a *Account, amount int, _ int64, _ *TxnMeta) error {
	a.CurrentBalance -= amount
	a.FrozenBalance += amount
	f.txns = append(f.txns, "freeze")
	return nil
}
func (f *fakeBillStore) Deduct(_ context.Context, a *Account, amount int, _ int64, _ *TxnMeta) error {
	a.FrozenBalance -= amount
	f.txns = append(f.txns, "deduct")
	return nil
}
func (f *fakeBillStore) Unfreeze(_ context.Context, a *Account, amount int, _ int64, _ *TxnMeta) error {
	a.FrozenBalance -= amount
	a.CurrentBalance += amount
	f.txns = append(f.txns, "unfreeze")
	return nil
}
func (f *fakeBillStore) SaveTaskBill(_ context.Context, taskID int64, b *TaskBill) error {
	f.bills[taskID] = b
	return nil
}
func (f *fakeBillStore) LoadTaskBill(_ context.Context, taskID int64) (*TaskBill, error) {
	b, ok := f.bills[taskID]
	if !ok {
		return nil, errors.New("no bill")
	}
	return b, nil
}
func (f *fakeBillStore) MarkTaskSettled(_ context.Context, taskID int64) (bool, error) {
	if f.settled[taskID] {
		return false, nil
	}
	f.settled[taskID] = true
	return true, nil
}
func (f *fakeBillStore) IsTaskSettled(_ context.Context, taskID int64) (bool, error) {
	return f.settled[taskID], nil
}
func (f *fakeBillStore) ListTransactions(context.Context, int64, int, int) ([]map[string]any, int, error) {
	return nil, 0, nil
}

func meta() *TxnMeta { return &TxnMeta{SpaceID: 10, OperatorUserID: 1} }

func TestThreeStageSuccess(t *testing.T) {
	store := newFakeStore(100)
	eng := NewEngine(store)

	// 阶段1 冻结
	require.NoError(t, eng.Freeze(context.Background(), "USER", 1, 1, 50, meta()))
	acc := store.accounts["USER:1"]
	assert.Equal(t, 50, acc.CurrentBalance, "冻结后可用余额 50")
	assert.Equal(t, 50, acc.FrozenBalance, "冻结余额 50")
	assert.Contains(t, store.txns, "freeze")

	// 阶段2 成功扣费
	require.NoError(t, eng.Settle(context.Background(), 1, true, 50, meta()))
	assert.Equal(t, 50, acc.CurrentBalance)
	assert.Equal(t, 0, acc.FrozenBalance, "扣费后冻结归零")
	assert.Contains(t, store.txns, "deduct")
}

func TestThreeStageFailureFullUnfreeze(t *testing.T) {
	store := newFakeStore(100)
	eng := NewEngine(store)

	require.NoError(t, eng.Freeze(context.Background(), "USER", 1, 1, 80, meta()))
	// 失败 → 全额解冻（永不倒扣，03 A.3 黄金规则①）
	require.NoError(t, eng.Settle(context.Background(), 1, false, 0, meta()))
	acc := store.accounts["USER:1"]
	assert.Equal(t, 100, acc.CurrentBalance, "失败后全额退回")
	assert.Equal(t, 0, acc.FrozenBalance)
	assert.Contains(t, store.txns, "unfreeze")
	assert.NotContains(t, store.txns, "deduct")
}

func TestSettleIdempotent(t *testing.T) {
	store := newFakeStore(100)
	eng := NewEngine(store)
	require.NoError(t, eng.Freeze(context.Background(), "USER", 1, 1, 50, meta()))

	// 成功结算
	require.NoError(t, eng.Settle(context.Background(), 1, true, 50, meta()))
	// 重复结算（乱序回调）→ 幂等拒绝，不重复扣
	err := eng.Settle(context.Background(), 1, true, 50, meta())
	assert.ErrorIs(t, err, ErrAlreadySettled)
	acc := store.accounts["USER:1"]
	assert.Equal(t, 50, acc.CurrentBalance, "重复结算不二次扣费")
	// 只记一次 deduct
	deductCount := 0
	for _, txn := range store.txns {
		if txn == "deduct" {
			deductCount++
		}
	}
	assert.Equal(t, 1, deductCount, "扣费仅一次")
}

func TestFreezeInsufficient(t *testing.T) {
	store := newFakeStore(30)
	eng := NewEngine(store)
	err := eng.Freeze(context.Background(), "USER", 1, 1, 50, meta())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInsufficient)
	acc := store.accounts["USER:1"]
	assert.Equal(t, 30, acc.CurrentBalance, "冻结失败余额不变")
}

func TestFreezeIdempotent(t *testing.T) {
	store := newFakeStore(100)
	eng := NewEngine(store)
	require.NoError(t, eng.Freeze(context.Background(), "USER", 1, 1, 50, meta()))
	// 重复冻结 → 幂等跳过（不二次冻结）
	require.NoError(t, eng.Freeze(context.Background(), "USER", 1, 1, 50, meta()))
	acc := store.accounts["USER:1"]
	assert.Equal(t, 50, acc.CurrentBalance, "重复冻结不二次扣减")
}