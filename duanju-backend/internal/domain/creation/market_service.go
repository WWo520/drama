// 生态治理（S3-B04~B06）：模板集市 + 任务接单 + 风格库。
package creation

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// MarketStore 生态数据边界。
type MarketStore interface {
	// ListTemplates 模板列表。
	ListTemplates(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
	// CreateTemplate 创建模板。
	CreateTemplate(ctx context.Context, userID int64, name, desc string, data string, price int) error
	// ApplyTemplate 应用模板（used_count+1，返回 data）。
	ApplyTemplate(ctx context.Context, templateUUID string) (string, error)
	// ListStyles 风格库列表（27 风格）。
	ListStyles(ctx context.Context) ([]map[string]any, error)
	// ApplyStyle 应用风格（prompt_suffix 注入 project.config）。
	ApplyStyle(ctx context.Context, projectID int64, styleCode string) error
	// ListOrders 任务列表（按状态）。
	ListOrders(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int, error)
	// CreateOrder 发单。
	CreateOrder(ctx context.Context, spaceID, userID int64, title, desc string, reward int, deadline string) (string, error)
	// CreateIntent 接单意向。
	CreateIntent(ctx context.Context, orderID, userID int64, cover string) error
	// AcceptIntent 发布方接受意向（状态机 OPEN→ACCEPTED）。
	AcceptIntent(ctx context.Context, orderUUID string, intentUserUUID string, publisherID int64) error
	// UpdateOrderStatus 状态推进（ACCEPTED→IN_PROGRESS→SUBMITTED→COMPLETED；完成发放积分）。
	UpdateOrderStatus(ctx context.Context, orderUUID string, userID int64, to string) error
}

// MarketService 生态服务。
type MarketService struct {
	store MarketStore
	repo  *Repo
}

func NewMarketService(store MarketStore, repo *Repo) *MarketService {
	return &MarketService{store: store, repo: repo}
}

// defaultSpaceIDForUser 用户默认空间。
func (s *MarketService) defaultSpaceIDForUser(c *gin.Context) (int64, string, error) {
	uid := userID(c)
	if uid == 0 {
		return 0, "", errors.New("unauthorized")
	}
	return s.repo.DefaultSpaceID(c.Request.Context(), uid)
}

// orderIDByUUID 任务 uuid → id。
func (s *MarketService) orderIDByUUID(c *gin.Context, orderUUID string) (int64, error) {
	var id int64
	err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id FROM task_orders WHERE order_uuid=$1::uuid AND deleted_at IS NULL`, orderUUID).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// applyStyleToProject 应用风格到项目。
func (s *MarketService) applyStyleToProject(c *gin.Context, projectUUID string, uid int64, styleCode string) error {
	proj, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		return err
	}
	return s.store.ApplyStyle(c.Request.Context(), proj.ID, styleCode)
}

// ListTemplates 模板集市列表。
func (s *MarketService) ListTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.ListTemplates(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取模板失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// CreateTemplate 创建模板。
func (s *MarketService) CreateTemplate(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Name  string `json:"name" binding:"required,min=1,max=128"`
		Desc  string `json:"description"`
		Data  string `json:"data" binding:"required"`
		Price int    `json:"price_points"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.CreateTemplate(c.Request.Context(), uid, req.Name, req.Desc, req.Data, req.Price); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建模板失败")
		return
	}
	httpx.OK(c, nil)
}

// ApplyTemplate 应用模板。
func (s *MarketService) ApplyTemplate(c *gin.Context, templateUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	data, err := s.store.ApplyTemplate(c.Request.Context(), templateUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "模板不存在")
		return
	}
	httpx.OK(c, gin.H{"template_data": data})
}

// ListStyles 风格库（S3-B06）。
func (s *MarketService) ListStyles(c *gin.Context) {
	items, err := s.store.ListStyles(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取风格库失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ApplyStyle 应用风格到项目。
func (s *MarketService) ApplyStyle(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		StyleCode string `json:"style_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.applyStyleToProject(c, projectUUID, uid, req.StyleCode); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// ListOrders 任务列表。
func (s *MarketService) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.DefaultQuery("status", "")
	items, total, err := s.store.ListOrders(c.Request.Context(), status, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取任务失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// CreateOrder 发单（6.1）。
func (s *MarketService) CreateOrder(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Title    string `json:"title" binding:"required,min=1,max=200"`
		Desc     string `json:"description"`
		Reward   int    `json:"reward_points" binding:"min=0"`
		Deadline string `json:"deadline_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	spaceID, _, err := s.defaultSpaceIDForUser(c)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取空间失败")
		return
	}
	uuid, err := s.store.CreateOrder(c.Request.Context(), spaceID, uid, req.Title, req.Desc, req.Reward, req.Deadline)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "发单失败")
		return
	}
	httpx.OK(c, gin.H{"order_id": uuid, "status": "OPEN"})
}

// CreateIntent 接单意向（6.4）。
func (s *MarketService) CreateIntent(c *gin.Context, orderUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		CoverLetter string `json:"cover_letter" binding:"omitempty,max=500"`
	}
	_ = c.ShouldBindJSON(&req)
	oid, err := s.orderIDByUUID(c, orderUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "任务不存在")
		return
	}
	if err := s.store.CreateIntent(c.Request.Context(), oid, uid, req.CoverLetter); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// AcceptIntent 发布方接受意向（6.5）。
func (s *MarketService) AcceptIntent(c *gin.Context, orderUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		UserUUID string `json:"user_uuid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.AcceptIntent(c.Request.Context(), orderUUID, req.UserUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// UpdateOrderStatus 状态推进（6.6~6.8：开始/提交/验收完成）。
func (s *MarketService) UpdateOrderStatus(c *gin.Context, orderUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Status string `json:"status" binding:"required,oneof=IN_PROGRESS SUBMITTED COMPLETED"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateOrderStatus(c.Request.Context(), orderUUID, uid, req.Status); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}