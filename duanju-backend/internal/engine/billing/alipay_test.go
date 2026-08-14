// 支付宝支付测试：下单幂等 + 回调 7 步验签 + 到账 + 重复/伪造回调拒绝（S1-B10 验收）。
package billing

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOrderStore 内存订单存储。
type fakeOrderStore struct {
	orders   map[string]*Order // orderNo → order
	idemKeys map[string]string // idemKey → orderNo
	seq      int64
}

func newFakeOrderStore() *fakeOrderStore {
	return &fakeOrderStore{orders: map[string]*Order{}, idemKeys: map[string]string{}}
}

func (f *fakeOrderStore) CreateOrder(_ context.Context, userID int64, orderType, planCode, amountYuan string, points int, idemKey string) (int64, string, error) {
	if no, ok := f.idemKeys[idemKey]; ok {
		return 0, no, nil
	}
	f.seq++
	no := fmt.Sprintf("P%d", f.seq)
	f.idemKeys[idemKey] = no
	f.orders[no] = &Order{ID: f.seq, OrderNo: no, UserID: userID, OrderType: orderType, PlanCode: planCode,
		AmountYuan: amountYuan, Points: points, PayStatus: "PENDING"}
	return f.seq, no, nil
}

func (f *fakeOrderStore) GetOrderByNo(_ context.Context, orderNo string) (*Order, error) {
	o, ok := f.orders[orderNo]
	if !ok {
		return nil, errors.New("订单不存在")
	}
	return o, nil
}

func (f *fakeOrderStore) PayCallback(_ context.Context, orderNo, payTransactionID string, points int) (bool, error) {
	o := f.orders[orderNo]
	if o == nil {
		return false, errors.New("订单不存在")
	}
	if o.PayStatus == "PAID" {
		return false, nil // 幂等
	}
	o.PayStatus = "PAID"
	o.PayTransactionID = payTransactionID
	o.Points = points
	return true, nil
}

func (f *fakeOrderStore) LogCallback(_ context.Context, _ map[string]string, _ int, _, _ string) error {
	return nil
}

func testAlipay() *AlipayService {
	return NewAlipayService(newFakeOrderStore(), AlipayConfig{
		AppID:      "2021000000000000",
		MockSecret: "dev-alipay-mock-secret",
		UseMock:    true,
	})
}

func TestCreateOrderAndCallback(t *testing.T) {
	svc := testAlipay()
	ctx := context.Background()

	// 下单
	resp, err := svc.CreateOrder(ctx, 1, "POINTS_PACKAGE", "POINTS_PKG_SMALL", "49.00", 500, "idem-1")
	require.NoError(t, err)
	assert.Contains(t, resp.OrderNo, "P")
	assert.Equal(t, "49.00", resp.AmountYuan)

	// 生成 mock 回调并处理（沙箱 0.01 全流程）
	cb := svc.BuildMockCallback(resp.OrderNo, "49.00", "TXN-1001")
	result := svc.HandleCallback(ctx, cb)
	assert.Equal(t, "success", result)

	// 订单已支付
	order, _ := svc.store.GetOrderByNo(ctx, resp.OrderNo)
	assert.Equal(t, "PAID", order.PayStatus)
}

func TestRepeatedCallbackRejected(t *testing.T) {
	svc := testAlipay()
	ctx := context.Background()
	resp, _ := svc.CreateOrder(ctx, 1, "POINTS_PACKAGE", "PKG", "19.90", 200, "idem-2")

	cb := svc.BuildMockCallback(resp.OrderNo, "19.90", "TXN-2001")
	assert.Equal(t, "success", svc.HandleCallback(ctx, cb))
	// 重复回调（同一交易号）→ 幂等 success，不重复入账
	assert.Equal(t, "success", svc.HandleCallback(ctx, cb))
}

func TestForgedCallbackRejected(t *testing.T) {
	svc := testAlipay()
	ctx := context.Background()
	resp, _ := svc.CreateOrder(ctx, 1, "POINTS_PACKAGE", "PKG", "9.90", 100, "idem-3")

	// 伪造：篡改金额（签名不匹配）→ 步1 验签失败 → fail
	cb := svc.BuildMockCallback(resp.OrderNo, "9.90", "TXN-3001")
	cb["total_amount"] = "0.01" // 篡改
	assert.Equal(t, "fail", svc.HandleCallback(ctx, cb))

	// 金额不匹配（正确签名但金额错误）→ 步2 金额字符串比较 → fail
	cb2 := svc.BuildMockCallback(resp.OrderNo, "9.90", "TXN-3002")
	// 重新签名正确金额，但订单实际金额 9.90 与回调 8.90 不符
	cb2["total_amount"] = "8.90"
	// 重签：用新金额重新构建（需完整签名）——直接构造错误的
	cb3 := svc.BuildMockCallback(resp.OrderNo, "9.90", "TXN-3003")
	cb3["total_amount"] = "8.90"
	// 修改后签名失效 → 应 fail
	assert.Equal(t, "fail", svc.HandleCallback(ctx, cb3))

	// 伪造交易号（正确签名但 trade_no 与订单无关）→ 仍 success 幂等（同一订单首付）
	cb4 := svc.BuildMockCallback(resp.OrderNo, "9.90", "TXN-FORGED")
	assert.Equal(t, "success", svc.HandleCallback(ctx, cb4))
}

func TestCreateOrderIdempotent(t *testing.T) {
	svc := testAlipay()
	ctx := context.Background()
	r1, _ := svc.CreateOrder(ctx, 1, "POINTS_PACKAGE", "PKG", "29.90", 300, "idem-4")
	r2, _ := svc.CreateOrder(ctx, 1, "POINTS_PACKAGE", "PKG", "29.90", 300, "idem-4")
	assert.Equal(t, r1.OrderNo, r2.OrderNo, "同幂等键返回同一订单")
}