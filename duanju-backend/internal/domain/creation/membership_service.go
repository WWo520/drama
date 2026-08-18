// 月卡会员（S2-B09）：9.9 购买/查询 + 9.10 取消 + 到期自动降级。
package creation

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// MembershipStore 会员数据边界。
type MembershipStore interface {
	// GetMembership 当前会员（过期自动降级 BASIC）。
	GetMembership(ctx context.Context, userID int64) (map[string]any, error)
	// Purchase 购买：创建订单（subscription_days_granted + related_membership_id 预留）。
	Purchase(ctx context.Context, userID int64, planCode, billingCycle, amountYuan string) (orderNo string, err error)
	// Activate 支付回调后激活/续期会员（归属校验：仅订单所有者）。
	Activate(ctx context.Context, orderNo string, userID int64) error
	// Cancel 取消续订（cancel_at_period_end=true）。
	Cancel(ctx context.Context, userID int64) error
}

// MembershipService 会员服务。
type MembershipService struct {
	store MembershipStore
}

func NewMembershipService(store MembershipStore) *MembershipService {
	return &MembershipService{store: store}
}

// Purchase 9.9 购买会员：POST /memberships/purchase。
func (s *MembershipService) Purchase(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		PlanCode     string `json:"plan_code" binding:"required,oneof=PRO ENTERPRISE ULTIMATE"`
		BillingCycle string `json:"billing_cycle" binding:"required,oneof=MONTHLY ANNUAL"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	// 价格表（03 E.6.1 默认）
	amount := map[string]string{"PRO": "29.00", "ENTERPRISE": "99.00", "ULTIMATE": "299.00"}[req.PlanCode]
	orderNo, err := s.store.Purchase(c.Request.Context(), uid, req.PlanCode, req.BillingCycle, amount)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建订单失败")
		return
	}
	httpx.OK(c, gin.H{"order_no": orderNo, "amount_yuan": amount, "plan_code": req.PlanCode, "billing_cycle": req.BillingCycle})
}

// Activate 支付回调（开发期端点）→ 激活会员（仅订单所有者可触发）。
func (s *MembershipService) Activate(c *gin.Context, orderNo string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Activate(c.Request.Context(), orderNo, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, "激活失败："+err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Detail 9.9 查询当前会员。
func (s *MembershipService) Detail(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	m, err := s.store.GetMembership(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询会员失败")
		return
	}
	httpx.OK(c, m)
}

// Cancel 9.10 取消续订。
func (s *MembershipService) Cancel(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Cancel(c.Request.Context(), uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// SSE 4.5 AI 对话流式输出（S2-B10）：开发期 mock 分段输出，带 is_mock 标记（生产接入真实 LLM 后移除）。
func (s *MembershipService) AIDialogSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, _ := c.Writer.(http.Flusher)
	// meta 事件告知前端当前为开发期 mock 回复
	c.SSEvent("meta", map[string]any{"is_mock": true})
	flusher.Flush()

	segments := []string{"好的，我来帮你分析这段剧本…", "已提取 2 个核心角色与 3 个关键场景。", "建议在场景 2 增加冲突升级，戏剧张力提升 30%。"}
	for _, seg := range segments {
		c.SSEvent("message", map[string]string{"text": seg})
		flusher.Flush()
		time.Sleep(300 * time.Millisecond)
	}
	c.SSEvent("done", map[string]any{"status": "ok", "is_mock": true})
	flusher.Flush()
}