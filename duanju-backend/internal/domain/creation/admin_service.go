// 管理后台（S3-B07/B08）：Dashboard 指标 + 用户管理 + 内容管理 + 数据（审核在 S3-B03 已有）。
package creation

import (
	"context"
	"encoding/json"
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
	// CreateAnnouncement 新增公告。
	CreateAnnouncement(ctx context.Context, title, content, startAt, endAt string, sortOrder int) error
	// UpdateAnnouncement 更新公告。
	UpdateAnnouncement(ctx context.Context, id int64, title, content, startAt, endAt string, sortOrder int) error
	// DeleteAnnouncement 删除公告。
	DeleteAnnouncement(ctx context.Context, id int64) error
	// CreateBanner 新增 Banner。
	CreateBanner(ctx context.Context, title, imageURL, jumpURL, position string, sortOrder int, isActive bool) error
	// UpdateBanner 更新 Banner。
	UpdateBanner(ctx context.Context, id int64, title, imageURL, jumpURL, position string, sortOrder int, isActive bool) error
	// DeleteBanner 删除 Banner。
	DeleteBanner(ctx context.Context, id int64) error
	// UpdatePointPackage 积分包 upsert。
	UpdatePointPackage(ctx context.Context, code, name string, points, bonus int, price, status string) error
	// UpdateRateLimit 更新限流配置。
	UpdateRateLimit(ctx context.Context, id int64, limitPerMin, burst int, status string) error
	// UpdateTheme 更新界面主题。
	UpdateTheme(ctx context.Context, id int64, name, primaryColor, accentColor, status string, isDefault bool) error
	// UpdateStyle 更新风格库条目。
	UpdateStyle(ctx context.Context, id int64, name, description, thumbnailURL string, sortOrder int, enabled bool) error
	// ListWithdrawals 提现申请列表。
	ListWithdrawals(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int, error)
	// ReviewWithdrawal 提现审核（PENDING → APPROVED/REJECTED）。
	ReviewWithdrawal(ctx context.Context, requestUUID string, approve bool, note string, operatorID int64) error
	// UpdatePreferences 更新个人偏好（含模型配置）。
	UpdatePreferences(ctx context.Context, userID int64, theme, lang, tz string, mc ModelPrefs) error
}

// ModelPrefs 模型配置偏好（创作页模型配置弹窗持久化：user_preferences.default_* 列）。
type ModelPrefs struct {
	VideoModel  string // default_video_model
	ImageModel  string // default_image_model（角色/场景/道具共用）
	LLMModel    string // default_llm_model
	StyleCode   string // default_style_code
	Resolution  string // video_resolution_enum：P480/P720/P1080
	AspectRatio string // image_aspect_ratio_enum：R16_9/R1_1/R9_16
	UIJSON      string // ui_preferences_json：{"video_duration":"auto"} 等扩展键
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

// ---- 公告/Banner 写接口（P2 运营模块） ----

// CreateAnnouncement 新增公告（POST /admin/announcements）。
func (s *AdminService) CreateAnnouncement(c *gin.Context) {
	var req struct {
		Title     string `json:"title" binding:"required,min=1,max=128"`
		Content   string `json:"content" binding:"required"`
		StartAt   string `json:"start_at" binding:"required"`
		EndAt     string `json:"end_at" binding:"required"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.CreateAnnouncement(c.Request.Context(), req.Title, req.Content, req.StartAt, req.EndAt, req.SortOrder); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "新增公告失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdateAnnouncement 更新公告（PUT /admin/announcements/:id）。
func (s *AdminService) UpdateAnnouncement(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		Title     string `json:"title" binding:"required,min=1,max=128"`
		Content   string `json:"content" binding:"required"`
		StartAt   string `json:"start_at" binding:"required"`
		EndAt     string `json:"end_at" binding:"required"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateAnnouncement(c.Request.Context(), id, req.Title, req.Content, req.StartAt, req.EndAt, req.SortOrder); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新公告失败")
		return
	}
	httpx.OK(c, nil)
}

// DeleteAnnouncement 删除公告（DELETE /admin/announcements/:id）。
func (s *AdminService) DeleteAnnouncement(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	if err := s.store.DeleteAnnouncement(c.Request.Context(), id); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除公告失败")
		return
	}
	httpx.OK(c, nil)
}

// CreateBanner 新增 Banner（POST /admin/banners）。
func (s *AdminService) CreateBanner(c *gin.Context) {
	var req struct {
		Title     string `json:"title" binding:"required,min=1,max=64"`
		ImageURL  string `json:"image_url"`
		JumpURL   string `json:"jump_url"`
		Position  string `json:"position"`
		SortOrder int    `json:"sort_order"`
		IsActive  bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.CreateBanner(c.Request.Context(), req.Title, req.ImageURL, req.JumpURL, req.Position, req.SortOrder, req.IsActive); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "新增 Banner 失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdateBanner 更新 Banner（PUT /admin/banners/:id）。
func (s *AdminService) UpdateBanner(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		Title     string `json:"title" binding:"required,min=1,max=64"`
		ImageURL  string `json:"image_url"`
		JumpURL   string `json:"jump_url"`
		Position  string `json:"position"`
		SortOrder int    `json:"sort_order"`
		IsActive  bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateBanner(c.Request.Context(), id, req.Title, req.ImageURL, req.JumpURL, req.Position, req.SortOrder, req.IsActive); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新 Banner 失败")
		return
	}
	httpx.OK(c, nil)
}

// DeleteBanner 删除 Banner（DELETE /admin/banners/:id）。
func (s *AdminService) DeleteBanner(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	if err := s.store.DeleteBanner(c.Request.Context(), id); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除 Banner 失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 积分包/限流/主题/风格库写接口 ----

// UpdatePointPackage 积分包 upsert（PUT /admin/point-packages）。
func (s *AdminService) UpdatePointPackage(c *gin.Context) {
	var req struct {
		PackageCode string `json:"package_code" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Points      int    `json:"points"`
		BonusPoints int    `json:"bonus_points"`
		PriceYuan   string `json:"price_yuan"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdatePointPackage(c.Request.Context(), req.PackageCode, req.Name, req.Points, req.BonusPoints, req.PriceYuan, req.Status); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新积分包失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdateRateLimit 更新限流配置（PUT /admin/rate-limits/:id）。
func (s *AdminService) UpdateRateLimit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		LimitPerMin int    `json:"limit_per_min"`
		Burst       int    `json:"burst"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateRateLimit(c.Request.Context(), id, req.LimitPerMin, req.Burst, req.Status); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新限流配置失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdateTheme 更新界面主题（PUT /admin/themes/:id）。
func (s *AdminService) UpdateTheme(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		Name         string `json:"name"`
		PrimaryColor string `json:"primary_color"`
		AccentColor  string `json:"accent_color"`
		Status       string `json:"status"`
		IsDefault    bool   `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateTheme(c.Request.Context(), id, req.Name, req.PrimaryColor, req.AccentColor, req.Status, req.IsDefault); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新主题失败")
		return
	}
	httpx.OK(c, nil)
}

// UpdateStyle 更新风格库条目（PUT /admin/style-library/:id）。
func (s *AdminService) UpdateStyle(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		ThumbnailURL string `json:"thumbnail_url"`
		SortOrder    int    `json:"sort_order"`
		Enabled      bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdateStyle(c.Request.Context(), id, req.Name, req.Description, req.ThumbnailURL, req.SortOrder, req.Enabled); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新风格失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 提现审核 ----

// ListWithdrawals 提现申请列表（GET /admin/withdrawals）。
func (s *AdminService) ListWithdrawals(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 100 || pageSize < 1 {
		pageSize = 20
	}
	status := c.Query("status")
	items, total, err := s.store.ListWithdrawals(c.Request.Context(), status, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取提现申请失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// ReviewWithdrawal 提现审核（POST /admin/withdrawals/:id/review）。
func (s *AdminService) ReviewWithdrawal(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.ReviewWithdrawal(c.Request.Context(), c.Param("id"), req.Approve, req.Note, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// UpdatePreferences 更新个人偏好（PUT /me/preferences）。
// model_config 子对象：video_model/image_model/llm_model/style_code/resolution/aspect_ratio
// 为固定列；其余键（如 video_duration）归入 ui_preferences_json。
func (s *AdminService) UpdatePreferences(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Theme       string         `json:"theme"`
		Language    string         `json:"language"`
		Timezone    string         `json:"timezone"`
		ModelConfig map[string]any `json:"model_config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.UpdatePreferences(c.Request.Context(), uid, req.Theme, req.Language, req.Timezone, buildModelPrefs(req.ModelConfig)); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新偏好失败")
		return
	}
	httpx.OK(c, nil)
}

// buildModelPrefs 从 model_config 对象提取模型偏好固定字段；未知键归入 UIJSON。
func buildModelPrefs(mc map[string]any) ModelPrefs {
	out := ModelPrefs{}
	if len(mc) == 0 {
		return out
	}
	str := func(k string) string {
		if v, ok := mc[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	out.VideoModel = str("video_model")
	out.ImageModel = str("image_model")
	out.LLMModel = str("llm_model")
	out.StyleCode = str("style_code")
	out.Resolution = str("resolution")
	out.AspectRatio = str("aspect_ratio")
	known := map[string]bool{"video_model": true, "image_model": true, "llm_model": true, "style_code": true, "resolution": true, "aspect_ratio": true}
	rest := map[string]any{}
	for k, v := range mc {
		if !known[k] {
			rest[k] = v
		}
	}
	if len(rest) > 0 {
		if b, err := json.Marshal(rest); err == nil {
			out.UIJSON = string(b)
		}
	}
	return out
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