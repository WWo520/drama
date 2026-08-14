// 计费 API：9.3 积分账户 / 9.3.1 预览消耗 / 9.4 积分流水。
package billing

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// PointsAPI 计费 HTTP 服务。
type PointsAPI struct {
	store BillStore
}

func NewPointsAPI(store BillStore) *PointsAPI {
	return &PointsAPI{store: store}
}

// Me 9.3 我的积分账户。
func (a *PointsAPI) Me(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	acc, err := a.store.GetAccountForUpdate(c.Request.Context(), "USER", uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取积分账户失败")
		return
	}
	httpx.OK(c, gin.H{
		"current_balance": acc.CurrentBalance,
		"frozen_balance":  acc.FrozenBalance,
	})
}

// Transactions 9.4 积分流水。
func (a *PointsAPI) Transactions(c *gin.Context) {
	uid := c.GetInt64(middleware.CtxUserID)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	acc, err := a.store.GetAccountForUpdate(c.Request.Context(), "USER", uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取积分账户失败")
		return
	}
	items, total, err := a.store.ListTransactions(c.Request.Context(), acc.ID, 1, 20)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取积分流水失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": 1, "page_size": 20, "total": total, "total_pages": (total + 19) / 20}})
}