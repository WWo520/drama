// 管理后台（S3-B07/B08）：Dashboard 指标 + 用户管理 + 内容管理 + 数据（审核在 S3-B03 已有）。
package creation

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// AdminStore 管理后台数据边界。
type AdminStore interface {
	// Dashboard 核心指标（11.0）：注册用户/DAU/空间/作品/任务/收入。
	Dashboard(ctx context.Context) (map[string]any, error)
	// ListUsers 用户列表（11.2）。
	ListUsers(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
	// BanUser 封禁/解封（11.5）。
	BanUser(ctx context.Context, userUUID string, banned bool, operatorID int64) error
	// ListWorks 内容列表（11.15 全量含状态）。
	ListWorks(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int, error)
	// DeleteWork 强制删除内容（11.73）。
	DeleteWork(ctx context.Context, workUUID string, operatorID int64) error
	// ListReports 举报列表（11.74）。
	ListReports(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
	// ListOrders 订单列表（P2）。
	ListOrders(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
	// ListPackages 套餐列表（P2）。
	ListPackages(ctx context.Context) ([]map[string]any, error)
	// ListModels 模型列表（P2）。
	ListModels(ctx context.Context) ([]map[string]any, error)
	// ListAuditLogs 审计日志（P2）。
	ListAuditLogs(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
	// ListRoles 角色列表（P2）。
	ListRoles(ctx context.Context) ([]map[string]any, error)
	// ListPlatformConfigs 平台配置（P2）。
	ListPlatformConfigs(ctx context.Context) ([]map[string]any, error)
	// GetPreferences 个人偏好。
	GetPreferences(ctx context.Context, userID int64) (map[string]any, error)
	// ListAnnouncements 公告列表（P2）。
	ListAnnouncements(ctx context.Context) ([]map[string]any, error)
	// ListBanners banner 列表（P2）。
	ListBanners(ctx context.Context) ([]map[string]any, error)
	// ListHints 提示列表（P2）。
	ListHints(ctx context.Context, category string, step int) ([]map[string]any, error)
	// UpdatePlatformConfig 更新平台配置。
	UpdatePlatformConfig(ctx context.Context, key, value, group, desc string, operatorID int64) error
	// UpdatePackage 更新套餐。
	UpdatePackage(ctx context.Context, code, name string, price string, points int, status string) error
	// UpdateModel 更新模型。
	UpdateModel(ctx context.Context, key, name, capability string, cost int, enabled bool) error
	// UpdatePreferences 更新个人偏好。
	UpdatePreferences(ctx context.Context, userID int64, theme, lang, tz string) error
}

// AdminService 管理后台服务。
type AdminService struct {
	store AdminStore
}

func NewAdminService(store AdminStore) *AdminService {
	return &AdminService{store: store}
}

// Dashboard 指标（S3-F03 卡片数据源）。
func (s *AdminService) Dashboard(c *gin.Context) {
	d, err := s.store.Dashboard(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取指标失败")
		return
	}
	httpx.OK(c, d)
}

// ListUsers 用户列表。
func (s *AdminService) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.ListUsers(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取用户失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// BanUser 封禁。
func (s *AdminService) BanUser(c *gin.Context, userUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Banned bool `json:"banned"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := s.store.BanUser(c.Request.Context(), userUUID, req.Banned, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// ListWorks 内容管理列表。
func (s *AdminService) ListWorks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.DefaultQuery("status", "")
	items, total, err := s.store.ListWorks(c.Request.Context(), status, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取内容失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// DeleteWork 强制删除。
func (s *AdminService) DeleteWork(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.DeleteWork(c.Request.Context(), workUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// ListReports 举报列表。
func (s *AdminService) ListReports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.ListReports(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取举报失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// ListOrders 订单列表（P2）。
func (s *AdminService) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.ListOrders(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取订单失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// ListPackages 套餐列表（P2）。
func (s *AdminService) ListPackages(c *gin.Context) {
	items, err := s.store.ListPackages(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取套餐失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListModels 模型列表（P2）。
func (s *AdminService) ListModels(c *gin.Context) {
	items, err := s.store.ListModels(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取模型失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListAuditLogs 审计日志（P2）。
func (s *AdminService) ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.ListAuditLogs(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取审计日志失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// ListRoles 角色列表（P2）。
func (s *AdminService) ListRoles(c *gin.Context) {
	items, err := s.store.ListRoles(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取角色失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListPlatformConfigs 平台配置（P2）。
func (s *AdminService) ListPlatformConfigs(c *gin.Context) {
	items, err := s.store.ListPlatformConfigs(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取配置失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// MyPreferences 个人偏好（/me/preferences）。
func (s *AdminService) MyPreferences(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	prefs, err := s.store.GetPreferences(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取偏好失败")
		return
	}
	httpx.OK(c, prefs)
}

// ListAnnouncements 公告列表（P2 运营模块）。
func (s *AdminService) ListAnnouncements(c *gin.Context) {
	items, err := s.store.ListAnnouncements(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取公告失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListBanners banner 列表（P2 运营模块）。
func (s *AdminService) ListBanners(c *gin.Context) {
	items, err := s.store.ListBanners(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取 Banner 失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListHints 提示列表（P2 运营模块）。
func (s *AdminService) ListHints(c *gin.Context) {
	category := c.Query("category")
	step, _ := strconv.Atoi(c.DefaultQuery("step", "0"))
	items, err := s.store.ListHints(c.Request.Context(), category, step)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取提示失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// UpdatePreferences 更新个人偏好（PUT /me/preferences）。
func (s *AdminService) UpdatePreferences(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Theme    string `json:"theme"`
		Language string `json:"language"`
		Timezone string `json:"timezone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdatePreferences(c.Request.Context(), uid, req.Theme, req.Language, req.Timezone); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新偏好失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdatePlatformConfig 更新平台配置（PUT /admin/platform-config）。
func (s *AdminService) UpdatePlatformConfig(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		ConfigKey   string `json:"config_key" binding:"required"`
		ConfigValue string `json:"config_value" binding:"required"`
		ConfigGroup string `json:"config_group"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdatePlatformConfig(c.Request.Context(), req.ConfigKey, req.ConfigValue, req.ConfigGroup, req.Description, uid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新配置失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdatePackage 更新套餐（PUT /admin/packages）。
func (s *AdminService) UpdatePackage(c *gin.Context) {
	var req struct {
		PackageCode   string `json:"package_code" binding:"required"`
		Name          string `json:"name" binding:"required"`
		PriceYuan     string `json:"price_yuan" binding:"required"`
		PointsQuota   int    `json:"points_quota"`
		Status        string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdatePackage(c.Request.Context(), req.PackageCode, req.Name, req.PriceYuan, req.PointsQuota, req.Status); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新套餐失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdateModel 更新模型（PUT /admin/models）。
func (s *AdminService) UpdateModel(c *gin.Context) {
	var req struct {
		ModelKey    string `json:"model_key" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Capability  string `json:"capability"`
		Provider    string `json:"provider_name"`
		CostPoints  int    `json:"cost_points"`
		Enabled     bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateModel(c.Request.Context(), req.ModelKey, req.Name, req.Capability, req.CostPoints, req.Enabled); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新模型失败")
		return
	}
	httpx.OK(c, nil)
}