// 空间服务：列表/详情/切换/配额 + 越权拦截（S1-B02 验收：越权访问 403）。
package space

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// Service 空间服务。
type Service struct {
	repo       *Repo
	billingOps BillingOps
}

// BillingOps 团队计费边界（repository.SpaceBillingStore 实现）。
type BillingOps interface {
	GetSpaceAccount(ctx context.Context, spaceID int64) (map[string]any, error)
	ListSpaceTxns(ctx context.Context, spaceID, viewerID int64, isFull bool, page, pageSize int) ([]map[string]any, int, error)
	MemberConsumptionReport(ctx context.Context, spaceID int64) ([]map[string]any, error)
	CreateSpaceOrder(ctx context.Context, userID, spaceID int64, pkgCode, amountYuan string, points int) (string, error)
	PaySpaceOrder(ctx context.Context, orderNo string, points int) error
}

func NewService(repo *Repo, billingOps BillingOps) *Service {
	return &Service{repo: repo, billingOps: billingOps}
}

// Create 创建团队空间（05 2.2）：自动建空间积分账户 + 创建者 OWNER；返回 201。
func (s *Service) Create(c *gin.Context) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=64"`
		Description string `json:"description" binding:"omitempty,max=256"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	r, err := s.repo.CreateTeam(c.Request.Context(), req.Name, req.Description, uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建团队空间失败")
		return
	}
	httpx.OK(c, toCard(*r))
}

// AddMember 添加成员（05 2.7 基础版）：OWNER/ADMIN 可操作；席位配额生效。
func (s *Service) AddMember(c *gin.Context, spaceUUID string) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	space, err := s.repo.GetByUUID(c.Request.Context(), spaceUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeSpaceNotFound, "空间不存在或无权限")
		return
	}
	// RBAC：仅 OWNER/ADMIN 可添加成员（S2-B02 全矩阵；此处先基础校验）
	if space.MyRole != "OWNER" && space.MyRole != "ADMIN" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅空间 OWNER/ADMIN 可添加成员")
		return
	}
	var req struct {
		UserUUID string `json:"user_id" binding:"required"`
		Role     string `json:"role" binding:"omitempty,oneof=ADMIN EDITOR MEMBER VIEWER BILLING_ONLY"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.Role == "" {
		req.Role = "MEMBER"
	}
	// user_id 传 UUID → 转数字主键
	userID, err := s.repo.GetUserIDByUUID(c.Request.Context(), req.UserUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20006, "用户不存在")
		return
	}
	if err := s.repo.AddMember(c.Request.Context(), space.ID, userID, uid, req.Role); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, "添加成员失败："+err.Error())
		return
	}
	httpx.OK(c, nil)
}

// ListMembers 成员列表（2.6）：空间成员可查看（VIEWER+）。
func (s *Service) ListMembers(c *gin.Context, spaceUUID string) {
	_, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if err := Require(role, PermViewResources); err != nil {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限查看成员")
		return
	}
	members, err := s.repo.ListMembers(c.Request.Context(), space.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取成员列表失败")
		return
	}
	httpx.OK(c, gin.H{"items": members, "my_role": role})
}

// UpdateMemberRole 修改成员角色（2.9）：OWNER/ADMIN；OWNER 不可被改。
func (s *Service) UpdateMemberRole(c *gin.Context, spaceUUID, targetUserUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if err := Require(role, PermManageMembers); err != nil {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN 可修改成员角色")
		return
	}
	var req struct {
		Role string `json:"role" binding:"required,oneof=ADMIN EDITOR MEMBER VIEWER BILLING_ONLY"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	targetID, err := s.repo.GetUserIDByUUID(c.Request.Context(), targetUserUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20006, "用户不存在")
		return
	}
	if targetID == uid {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "不能修改自己的角色")
		return
	}
	if err := s.repo.UpdateMemberRole(c.Request.Context(), space.ID, targetID, req.Role); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// RemoveMember 移除成员（2.10）：OWNER/ADMIN；OWNER 不可移除。
func (s *Service) RemoveMember(c *gin.Context, spaceUUID, targetUserUUID string) {
	_, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if err := Require(role, PermManageMembers); err != nil {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN 可移除成员")
		return
	}
	targetID, err := s.repo.GetUserIDByUUID(c.Request.Context(), targetUserUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20006, "用户不存在")
		return
	}
	if err := s.repo.RemoveMember(c.Request.Context(), space.ID, targetID); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// TransferOwner 转让所有权（2.11）：仅 OWNER。
func (s *Service) TransferOwner(c *gin.Context, spaceUUID, targetUserUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER 可转让所有权")
		return
	}
	targetID, err := s.repo.GetUserIDByUUID(c.Request.Context(), targetUserUUID)
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

// CreateInvitation 创建邀请链接（2.8）：OWNER/ADMIN（权限矩阵 invite_members）。
func (s *Service) CreateInvitation(c *gin.Context, spaceUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if err := Require(role, PermInviteMembers); err != nil {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN 可邀请成员")
		return
	}
	var req struct {
		Account string `json:"account" binding:"required"` // 邮箱/手机号（M1 短信；M0 邮箱）
		Role    string `json:"role" binding:"omitempty,oneof=ADMIN EDITOR MEMBER VIEWER BILLING_ONLY"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.Role == "" {
		req.Role = "MEMBER"
	}
	token, err := s.repo.CreateInvitation(c.Request.Context(), space.ID, uid, req.Account, req.Role)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建邀请失败")
		return
	}
	httpx.OK(c, gin.H{"invite_token": token, "expires_in_days": 7, "invite_url": "/invite/" + token})
}

// AcceptInvitation 接受邀请（2.8a）：登录后凭 token 加入。
func (s *Service) AcceptInvitation(c *gin.Context, token string) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.repo.AcceptInvitation(c.Request.Context(), token, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// resolveSpace 解析空间 + 校验成员身份，返回 (uid, space, role, ok)。
func (s *Service) resolveSpace(c *gin.Context, spaceUUID string) (int64, *Row, string, bool) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return 0, nil, "", false
	}
	r, err := s.repo.GetByUUID(c.Request.Context(), spaceUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeSpaceNotFound, "空间不存在或无权限")
		return 0, nil, "", false
	}
	return uid, r, r.MyRole, true
}

// TeamBilling 团队账户（9.3）：OWNER/ADMIN/BILLING_ONLY 可见（03 E.1 共享积分）。
func (s *Service) TeamBilling(c *gin.Context, spaceUUID string) {
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

// TeamBillingTxns 团队流水（9.4）：OWNER/ADMIN/BILLING_ONLY 全员；其他角色仅自己的流水。
func (s *Service) TeamBillingTxns(c *gin.Context, spaceUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if err := Require(role, PermTeamBilling); err != nil {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限查看团队流水")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	isFull := role == "OWNER" || role == "ADMIN" || role == "BILLING_ONLY"
	items, total, err := s.billingOps.ListSpaceTxns(c.Request.Context(), space.ID, uid, isFull, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取团队流水失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// TeamBillingReport 成员消费报表（OWNER/ADMIN）。
func (s *Service) TeamBillingReport(c *gin.Context, spaceUUID string) {
	_, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" && role != "ADMIN" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN 可查看消费报表")
		return
	}
	items, err := s.billingOps.MemberConsumptionReport(c.Request.Context(), space.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取消费报表失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// TeamRecharge 团队充值订单（2.14）：OWNER/ADMIN/BILLING_ONLY；支付回调加 SPACE 账户。
func (s *Service) TeamRecharge(c *gin.Context, spaceUUID string) {
	uid, space, role, ok := s.resolveSpace(c, spaceUUID)
	if !ok {
		return
	}
	if role != "OWNER" && role != "ADMIN" && role != "BILLING_ONLY" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅 OWNER/ADMIN/BILLING_ONLY 可充值团队积分")
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
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建团队充值订单失败")
		return
	}
	httpx.OK(c, gin.H{"order_no": orderNo, "amount_yuan": req.AmountYuan, "points": req.Points})
}

// TeamRechargeCallback 团队订单支付回调（mock HMAC 验签由订单侧完成；此处直接入账）。
func (s *Service) TeamRechargeCallback(c *gin.Context, orderNo string) {
	// 开发期：直接入账（生产由支付宝 7 步验签后调用）
	// 查询积分并入账
	var points int
	if err := s.repo.pool.QueryRow(c.Request.Context(),
		`SELECT points_granted_total FROM orders WHERE order_no=$1`, orderNo).Scan(&points); err != nil {
		httpx.Fail(c, http.StatusNotFound, 80002, "订单不存在")
		return
	}
	if err := s.billingOps.PaySpaceOrder(c.Request.Context(), orderNo, points); err != nil {
		fmt.Printf("[space-billing-debug] PaySpaceOrder(%s) err=%v\n", orderNo, err)
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "入账失败")
		return
	}
	httpx.OK(c, nil)
}

// List 我的空间列表（05 2.1）。
func (s *Service) List(c *gin.Context) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	rows, err := s.repo.ListMine(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取空间列表失败")
		return
	}
	items := make([]SpaceCardDTO, 0, len(rows))
	for _, r := range rows {
		items = append(items, toCard(r))
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": 1, "page_size": len(items), "total": len(items), "total_pages": 1}})
}

// Detail 空间详情（05 2.3）：非成员 404 code=20005。
func (s *Service) Detail(c *gin.Context, spaceUUID string) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	r, err := s.repo.GetByUUID(c.Request.Context(), spaceUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeSpaceNotFound, "空间不存在或无权限")
		return
	}
	quota := s.buildQuota(c, r)
	dto := SpaceDetailDTO{
		SpaceCardDTO: toCard(*r),
		OwnerUserID:  r.OwnerUUID,
		Quota:        quota,
		CreatedAt:    r.CreatedAt,
	}
	if r.Description != nil {
		dto.Description = *r.Description
	}
	httpx.OK(c, dto)
}

// Switch 切换当前工作空间（05 2.5）：返回空间上下文。
func (s *Service) Switch(c *gin.Context, spaceUUID string) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	r, err := s.repo.GetByUUID(c.Request.Context(), spaceUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeSpaceNotFound, "空间不存在或无权限")
		return
	}
	// 切换后注入 ctx（后续创作接口读取 X-Space-Id 或 ctx.space_id）
	c.Set(middleware.CtxSpaceID, r.ID)
	httpx.OK(c, SpaceContextDTO{
		SpaceID: r.SpaceUUID,
		Name:    r.Name,
		MyRole:  r.MyRole,
		Quota:   s.buildQuota(c, r),
	})
}

// Quota 空间配额与用量（05 2.12）。
func (s *Service) Quota(c *gin.Context, spaceUUID string) {
	uid, err := currentUserID(c)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	r, err := s.repo.GetByUUID(c.Request.Context(), spaceUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeSpaceNotFound, "空间不存在或无权限")
		return
	}
	httpx.OK(c, s.buildQuota(c, r))
}

// buildQuota 组装配额（存储用量从 s3_object_metadata 统计）。
func (s *Service) buildQuota(c *gin.Context, r *Row) SpaceQuotaDTO {
	var q SpaceQuotaDTO
	q.Storage.LimitBytes = r.QuotaStorageBytes
	q.Storage.UsedBytes, _ = s.repo.UsedStorage(c.Request.Context(), r.ID)
	q.Member.LimitCount = r.QuotaMemberCount
	q.Member.UsedCount = r.MemberCount
	return q
}

func toCard(r Row) SpaceCardDTO {
	avatar := ""
	if r.AvatarURL != nil {
		avatar = *r.AvatarURL
	}
	return SpaceCardDTO{
		SpaceID:     r.SpaceUUID,
		Name:        r.Name,
		AvatarURL:   avatar,
		Type:        r.Type,
		MyRole:      r.MyRole,
		MemberCount: r.MemberCount,
		UpdatedAt:   r.UpdatedAt,
	}
}

// currentUserID 从 JWT 中间件 ctx 取数字主键（LoadUserID 中间件注入）。
func currentUserID(c *gin.Context) (int64, error) {
	if id := c.GetInt64(middleware.CtxUserID); id > 0 {
		return id, nil
	}
	return 0, errors.New("user id not resolved")
}
