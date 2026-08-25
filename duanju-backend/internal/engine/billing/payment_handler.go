// 支付 HTTP 端点：9.5 下单 / 9.7 回调 / 9.8 状态轮询。
package billing

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// PaymentHandler 支付路由处理器。
type PaymentHandler struct {
	svc *AlipayService
}

func NewPaymentHandler(svc *AlipayService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

// CreateOrderReq 下单请求（9.5）。
// biz_type=package 订阅套餐（package_code + billing_cycle）；biz_type=point_package 积分包（兼容旧字段）。
type CreateOrderReq struct {
	BizType        string `json:"biz_type"`                    // package | point_package（默认 point_package）
	PackageCode    string `json:"package_code" binding:"required"` // 套餐编码（packages 或 point_packages）
	AmountYuan     string `json:"amount_yuan" binding:"required"`  // 金额字符串（精确比较）
	Points         int    `json:"points"`                      // 积分包必填
	BillingCycle   string `json:"billing_cycle"`               // 订阅：monthly | yearly
	IdempotencyKey string `json:"idempotency_key" binding:"required"` // 幂等键（H.1）
}

// CreateOrder 9.5 创建订单（订阅套餐/积分包分派）。
func (h *PaymentHandler) CreateOrder(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req CreateOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	var resp *CreateOrderResp
	var err error
	if req.BizType == "package" {
		// 订阅套餐：服务端校验 packages 表价目/周期
		resp, err = h.svc.CreateSubscriptionOrder(c.Request.Context(), uid, req.PackageCode, req.BillingCycle, req.IdempotencyKey)
	} else {
		// 积分包（默认）：校验 point_packages 表价目
		if req.Points <= 0 {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：积分包缺少 points")
			return
		}
		resp, err = h.svc.CreateOrder(c.Request.Context(), uid, "POINTS_PACKAGE", req.PackageCode, req.AmountYuan, req.Points, req.IdempotencyKey)
	}
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "下单失败："+err.Error())
		return
	}
	httpx.OK(c, resp)
}

// MockPay 开发环境模拟支付到账（仅 dev 注册；生产不注册该路由）。
// 内部生成 mock 回调参数并走 HandleCallback——验签/金额/幂等全链路与真实回调一致。
func (h *PaymentHandler) MockPay(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	orderNo := c.Param("order_no")
	if orderNo == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "order_no 为空")
		return
	}
	// 归属校验：仅订单所有者可模拟支付自己的订单
	order, err := h.svc.store.GetOrderByNoForUser(c.Request.Context(), orderNo, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "订单不存在")
		return
	}
	if order.PayStatus == "PAID" {
		httpx.OK(c, gin.H{"order_no": orderNo, "paid": true, "already": true})
		return
	}
	cb := h.svc.BuildMockCallback(order.OrderNo, order.AmountYuan, "MOCK-"+fmt.Sprint(time.Now().UnixMilli()))
	result := h.svc.HandleCallback(c.Request.Context(), cb)
	if result != "success" {
		httpx.Fail(c, http.StatusBadRequest, 20009, "模拟支付失败："+result)
		return
	}
	httpx.OK(c, gin.H{"order_no": orderNo, "paid": true})
}

// PayCallback 9.7 支付宝回调（返回纯文本 success/fail，不套 ApiResponse——支付宝协议）。
func (h *PaymentHandler) PayCallback(c *gin.Context) {
	// 显式解析 form（支付宝 POST application/x-www-form-urlencoded）
	_ = c.Request.ParseForm()
	params := map[string]string{}
	for k, vs := range c.Request.Form {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	if len(params) == 0 {
		// 兼容 JSON body
		var raw map[string]any
		if err := c.ShouldBindJSON(&raw); err != nil {
			c.String(http.StatusOK, "fail")
			return
		}
		for k, v := range raw {
			params[k] = fmt.Sprint(v)
		}
	}
	result := h.svc.HandleCallback(c.Request.Context(), params)
	c.String(http.StatusOK, result)
}

// Status 9.8 订单状态轮询（仅订单所有者可见）。
func (h *PaymentHandler) Status(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	orderNo := c.Param("order_no")
	if orderNo == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 order_no")
		return
	}
	order, err := h.svc.store.GetOrderByNoForUser(c.Request.Context(), orderNo, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 80002, "订单不存在")
		return
	}
	resp := gin.H{
		"order_no":   order.OrderNo,
		"pay_status": order.PayStatus,
		"points":     order.Points,
	}
	// 订阅订单：附带会员状态（05 9.8 OrderStatusDTO.subscription）
	if strings.HasPrefix(order.OrderType, "SUBSCRIPTION_") {
		var planCode, status, renew, endAt string
		if err := h.svc.QueryMembership(c.Request.Context(), uid, &planCode, &status, &renew, &endAt); err == nil {
			resp["subscription"] = gin.H{
				"plan_code":      planCode,
				"status":        status,
				"renew_strategy": renew,
				"expires_at":     endAt,
			}
		}
	}
	httpx.OK(c, resp)
}