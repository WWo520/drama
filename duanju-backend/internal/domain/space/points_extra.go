// 空间域补齐（S5 接口覆盖率）：团队积分账户/消费汇总/消费明细/个人消费/充值订单/扩容/转让。
package space

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// SpacePoints 2.13 团队空间共享积分账户（GET /spaces/:space_id/points）。
func (s *Service) SpacePoints(c *gin.Context, spaceUUID string) {
	_, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if err := Require(role, PermTeamBilling); err != nil {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限查看团队积分")
		return
	}
	acc, err := s.billingOps.GetSpaceAccount(c.Request.Context(), space.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取团队账户失败")
		return
	}
	httpx.OK(c, acc)
}

// SpacePointsConsumption 2.15 团队成员消耗汇总（GET /spaces/:space_id/points/consumption）。
func (s *Service) SpacePointsConsumption(c *gin.Context, spaceUUID string) {
	_, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" && role != "ADMIN" && role != "BILLING_ONLY" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限查看消费汇总")
		return
	}
	report, err := s.billingOps.MemberConsumptionReport(c.Request.Context(), space.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取消费汇总失败")
		return
	}
	httpx.OK(c, gin.H{"items": report})
}

// SpaceMemberConsumption 2.16 团队成员消耗明细（GET /spaces/:space_id/points/consumption/:user_id）。
func (s *Service) SpaceMemberConsumption(c *gin.Context, spaceUUID, userUUID string) {
	_, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" && role != "ADMIN" && role != "BILLING_ONLY" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限查看成员消费")
		return
	}
	targetID, err := s.repo.GetUserIDByUUID(c.Request.Context(), userUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20006, "用户不存在")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT pt.id, pt.txn_type::text, pt.direction::text, pt.amount, pt.balance_after,
		       pt.description, to_char(pt.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM point_transactions pt
		WHERE pt.related_space_id=$1 AND pt.operator_user_id=$2
		ORDER BY pt.id DESC LIMIT $3 OFFSET $4`, space.ID, targetID, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取消费明细失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var txnType, dir, desc, created string
		var amount, balance int
		if err := rows.Scan(&id, &txnType, &dir, &amount, &balance, &desc, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"txn_id": id, "txn_type": txnType, "direction": dir, "amount": amount, "balance_after": balance, "description": desc, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize}})
}

// SpaceMyConsumption 2.17 查看本人团队空间消费明细（GET /spaces/:space_id/points/my-consumption）。
func (s *Service) SpaceMyConsumption(c *gin.Context, spaceUUID string) {
	uid, space, _, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT pt.id, pt.txn_type::text, pt.direction::text, pt.amount, pt.balance_after,
		       pt.description, to_char(pt.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM point_transactions pt
		WHERE pt.related_space_id=$1 AND pt.operator_user_id=$2
		ORDER BY pt.id DESC LIMIT $3 OFFSET $4`, space.ID, uid, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取消费明细失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var txnType, dir, desc, created string
		var amount, balance int
		if err := rows.Scan(&id, &txnType, &dir, &amount, &balance, &desc, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"txn_id": id, "txn_type": txnType, "direction": dir, "amount": amount, "balance_after": balance, "description": desc, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize}})
}

// SpacePointsOrders 2.14 创建团队空间积分充值订单（POST /spaces/:space_id/points/orders）。
func (s *Service) SpacePointsOrders(c *gin.Context, spaceUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" && role != "ADMIN" && role != "BILLING_ONLY" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN/BILLING_ONLY 可充值")
		return
	}
	var req struct {
		PackageCode string `json:"package_code" binding:"required"`
		AmountYuan  string `json:"amount_yuan" binding:"required"`
		Points      int    `json:"points" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	orderNo, err := s.billingOps.CreateSpaceOrder(c.Request.Context(), uid, space.ID, req.PackageCode, req.AmountYuan, req.Points)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "下单失败："+err.Error())
		return
	}
	httpx.OK(c, gin.H{"order_no": orderNo, "amount_yuan": req.AmountYuan, "points": req.Points})
}

// SpaceQuotaPurchase 9.11 空间扩容（POST /spaces/:space_id/quota/purchase，消耗积分购买）。
func (s *Service) SpaceQuotaPurchase(c *gin.Context, spaceUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" && role != "ADMIN" && role != "BILLING_ONLY" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN/BILLING_ONLY 可扩容")
		return
	}
	var req struct {
		MemberSeats int `json:"member_seats" binding:"min=0,max=100"`
		StorageGB   int `json:"storage_gb" binding:"min=0,max=10000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.MemberSeats == 0 && req.StorageGB == 0 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "至少选择一项扩容")
		return
	}
	// 计费：席位 100 积分/个，存储 10 积分/GB
	totalCost := req.MemberSeats*100 + req.StorageGB*10
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "扩容失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	var balance int
	if err := tx.QueryRow(c.Request.Context(), `
		SELECT current_balance FROM point_accounts WHERE owner_type='SPACE' AND owner_id=$1 FOR UPDATE`, space.ID).
		Scan(&balance); err != nil {
		httpx.Fail(c, http.StatusNotFound, 80002, "团队账户不存在")
		return
	}
	if balance < totalCost {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "团队积分不足")
		return
	}
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE point_accounts SET current_balance=current_balance-$2 WHERE owner_type='SPACE' AND owner_id=$1`, space.ID, totalCost); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "扩容失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE spaces SET quota_member_count=quota_member_count+$2, quota_storage_bytes=quota_storage_bytes+$3
		WHERE id=$1`, space.ID, req.MemberSeats, int64(req.StorageGB)*1024*1024*1024); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "扩容失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
			balance_after, related_space_id, operator_user_id, description, idempotency_key)
		SELECT pa.id, 'SPACE', pa.owner_id, 'QUOTA_PURCHASE', 'DEBIT', $1, pa.current_balance, $2, $3,
			'空间扩容（席位/存储）', $4
		FROM point_accounts pa WHERE pa.owner_type='SPACE' AND pa.owner_id=$2`,
		totalCost, space.ID, uid, "quota-"+spaceUUID+"-"+strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "扩容失败")
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "扩容失败")
		return
	}
	httpx.OK(c, gin.H{"total_cost": totalCost, "member_seats_added": req.MemberSeats, "storage_gb_added": req.StorageGB})
}

// SpaceTransfer 2.10 转让空间所有权（POST /spaces/:space_id/transfer）。
func (s *Service) SpaceTransfer(c *gin.Context, spaceUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER 可转让所有权")
		return
	}
	var req struct {
		UserUUID string `json:"user_uuid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	targetID, err := s.repo.GetUserIDByUUID(c.Request.Context(), req.UserUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20006, "用户不存在")
		return
	}
	if err := s.repo.TransferOwner(c.Request.Context(), space.ID, targetID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}
