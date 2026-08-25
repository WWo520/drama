// 空间 HTTP 入口：2.1 列表 / 2.3 详情 / 2.5 切换 / 2.12 配额（S1-B02 个人空间子集）。
package space

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// Handler 空间路由处理器。
type Handler struct {
	svc *Service
	env string // 08 审查 Critical#2：开发期入账端点生产不注册
}

func NewHandler(svc *Service, env string) *Handler {
	return &Handler{svc: svc, env: env}
}

// RegisterRoutes 注册 /spaces/* 路由（05 TAG 2 子集；JWT + LoadUserID 已由 router 层组挂载）。
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	spaces := rg.Group("/spaces")
	{
		spaces.GET("", h.list)
		spaces.POST("", h.create) // 2.2 创建团队空间（S2-B01）
		spaces.GET("/:space_id", h.detail)
		spaces.POST("/:space_id/switch", h.switchSpace)
		spaces.GET("/:space_id/quota", h.quota)
		spaces.POST("/:space_id/members", h.addMember) // 2.7 添加成员（S2-B01）
		spaces.GET("/:space_id/members", h.listMembers)       // 2.6 成员列表（S2-B02）
		spaces.PATCH("/:space_id/members/:user_id", h.updateMemberRole) // 2.9 改角色
		spaces.PUT("/:space_id/members/:user_id/role", h.updateMemberRole) // 2.8 变更成员角色（文档路径）
		spaces.DELETE("/:space_id/members/:user_id", h.removeMember)    // 2.10 移除
		spaces.POST("/:space_id/transfer/:user_id", h.transferOwner)    // 2.11 转让
		spaces.POST("/:space_id/invitations", h.createInvitation)       // 2.8 邀请链接
		spaces.POST("/invitations/:token/accept", h.acceptInvitation)   // 2.8a 接受邀请
		spaces.GET("/:space_id/billing", h.teamBilling)                 // 9.3 团队账户（S2-B03）
		spaces.GET("/:space_id/billing/transactions", h.teamBillingTxns) // 9.4 团队流水
		spaces.GET("/:space_id/billing/report", h.teamBillingReport)     // 成员消费报表
		spaces.POST("/:space_id/billing/recharge", h.teamRecharge)       // 2.14 团队充值订单
		// 08 审查 Critical#2：开发期直接入账端点，生产环境不注册（真实支付回调接入前彻底移除攻击面）
		if h.env != "production" {
			spaces.POST("/billing/orders/:order_no/pay", h.teamRechargeCallback) // 团队订单入账（仅开发期）
		}
		// S5 补齐：团队积分/消费/扩容/转让
		spaces.GET("/:space_id/points", h.spacePoints)                 // 2.13 团队共享积分账户
		spaces.GET("/:space_id/points/consumption", h.spacePointsConsumption)      // 2.15 成员消耗汇总
		spaces.GET("/:space_id/points/consumption/:user_id", h.spaceMemberConsumption) // 2.16 成员消耗明细
		spaces.GET("/:space_id/points/my-consumption", h.spaceMyConsumption)  // 2.17 本人消费明细
		spaces.POST("/:space_id/points/orders", h.spacePointsOrders)   // 2.14 团队充值订单
		spaces.POST("/:space_id/quota/purchase", h.spaceQuotaPurchase) // 9.11 空间扩容
		spaces.POST("/:space_id/transfer", h.spaceTransfer)            // 2.10 转让所有权
	}
}

func (h *Handler) spacePoints(c *gin.Context)              { h.svc.SpacePoints(c, c.Param("space_id")) }
func (h *Handler) spacePointsConsumption(c *gin.Context)   { h.svc.SpacePointsConsumption(c, c.Param("space_id")) }
func (h *Handler) spaceMemberConsumption(c *gin.Context)   { h.svc.SpaceMemberConsumption(c, c.Param("space_id"), c.Param("user_id")) }
func (h *Handler) spaceMyConsumption(c *gin.Context)       { h.svc.SpaceMyConsumption(c, c.Param("space_id")) }
func (h *Handler) spacePointsOrders(c *gin.Context)        { h.svc.SpacePointsOrders(c, c.Param("space_id")) }
func (h *Handler) spaceQuotaPurchase(c *gin.Context)       { h.svc.SpaceQuotaPurchase(c, c.Param("space_id")) }
func (h *Handler) spaceTransfer(c *gin.Context)            { h.svc.SpaceTransfer(c, c.Param("space_id")) }

func (h *Handler) teamBilling(c *gin.Context) {
	h.svc.TeamBilling(c, c.Param("space_id"))
}

func (h *Handler) teamBillingTxns(c *gin.Context) {
	h.svc.TeamBillingTxns(c, c.Param("space_id"))
}

func (h *Handler) teamBillingReport(c *gin.Context) {
	h.svc.TeamBillingReport(c, c.Param("space_id"))
}

func (h *Handler) teamRecharge(c *gin.Context) {
	h.svc.TeamRecharge(c, c.Param("space_id"))
}

func (h *Handler) teamRechargeCallback(c *gin.Context) {
	h.svc.TeamRechargeCallback(c, c.Param("order_no"))
}

func (h *Handler) listMembers(c *gin.Context) {
	h.svc.ListMembers(c, c.Param("space_id"))
}

func (h *Handler) updateMemberRole(c *gin.Context) {
	h.svc.UpdateMemberRole(c, c.Param("space_id"), c.Param("user_id"))
}

func (h *Handler) removeMember(c *gin.Context) {
	h.svc.RemoveMember(c, c.Param("space_id"), c.Param("user_id"))
}

func (h *Handler) transferOwner(c *gin.Context) {
	h.svc.TransferOwner(c, c.Param("space_id"), c.Param("user_id"))
}

func (h *Handler) createInvitation(c *gin.Context) {
	h.svc.CreateInvitation(c, c.Param("space_id"))
}

func (h *Handler) acceptInvitation(c *gin.Context) {
	h.svc.AcceptInvitation(c, c.Param("token"))
}

func (h *Handler) create(c *gin.Context) {
	h.svc.Create(c)
}

func (h *Handler) addMember(c *gin.Context) {
	h.svc.AddMember(c, c.Param("space_id"))
}

func (h *Handler) list(c *gin.Context) {
	h.svc.List(c)
}

func (h *Handler) detail(c *gin.Context) {
	spaceUUID := c.Param("space_id")
	if spaceUUID == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 space_id")
		return
	}
	h.svc.Detail(c, spaceUUID)
}

func (h *Handler) switchSpace(c *gin.Context) {
	spaceUUID := c.Param("space_id")
	if spaceUUID == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 space_id")
		return
	}
	h.svc.Switch(c, spaceUUID)
}

func (h *Handler) quota(c *gin.Context) {
	spaceUUID := c.Param("space_id")
	if spaceUUID == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 space_id")
		return
	}
	h.svc.Quota(c, spaceUUID)
}
