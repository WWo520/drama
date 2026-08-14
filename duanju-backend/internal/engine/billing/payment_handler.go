// 支付 HTTP 端点：9.5 下单 / 9.7 回调 / 9.8 状态轮询。
package billing

import (
	"fmt"
	"net/http"

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
type CreateOrderReq struct {
	PackageCode   string `json:"package_code" binding:"required"` // POINTS_PKG_SMALL 等
	AmountYuan    string `json:"amount_yuan" binding:"required"`  // 金额字符串（精确比较）
	Points        int    `json:"points" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"` // 幂等键（H.1）
}

// CreateOrder 9.5 创建订单。
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
	resp, err := h.svc.CreateOrder(c.Request.Context(), uid, "POINTS_PACKAGE", req.PackageCode, req.AmountYuan, req.Points, req.IdempotencyKey)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建订单失败")
		return
	}
	httpx.OK(c, resp)
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

// Status 9.8 订单状态轮询。
func (h *PaymentHandler) Status(c *gin.Context) {
	orderNo := c.Param("order_no")
	if orderNo == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 order_no")
		return
	}
	order, err := h.svc.store.GetOrderByNo(c.Request.Context(), orderNo)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 80002, "订单不存在")
		return
	}
	httpx.OK(c, gin.H{
		"order_no":   order.OrderNo,
		"pay_status": order.PayStatus,
		"points":     order.Points,
	})
}