// 通知 API：8.1 列表 / 8.2 标记已读 / 8.3 全部已读（05 TAG 8）。
package notify

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// NotifyStore 通知数据边界。
type NotifyStore interface {
	// List 通知列表（分页）。
	List(ctx context.Context, userID int64, page, pageSize int) ([]map[string]any, int, error)
	// MarkRead 单条已读。
	MarkRead(ctx context.Context, userID, notificationID int64) error
	// MarkAllRead 全部已读。
	MarkAllRead(ctx context.Context, userID int64) error
	// Create 创建通知（站内信异步，03 D3）。
	Create(ctx context.Context, userID int64, ntype, title, content, link string) (int64, error)
}

// NotifyAPI 通知 HTTP 服务。
type NotifyAPI struct {
	store NotifyStore
}

func NewNotifyAPI(store NotifyStore) *NotifyAPI {
	return &NotifyAPI{store: store}
}

// List 8.1 通知列表。
func (a *NotifyAPI) List(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := a.store.List(c.Request.Context(), uid, page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取通知失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// MarkRead 8.2 单条已读。
func (a *NotifyAPI) MarkRead(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	nid, err := strconv.ParseInt(c.Param("notification_id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "notification_id 非法")
		return
	}
	if err := a.store.MarkRead(c.Request.Context(), uid, nid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "标记失败")
		return
	}
	httpx.OK(c, nil)
}

// MarkAllRead 8.3 全部已读。
func (a *NotifyAPI) MarkAllRead(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := a.store.MarkAllRead(c.Request.Context(), uid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "标记失败")
		return
	}
	httpx.OK(c, nil)
}