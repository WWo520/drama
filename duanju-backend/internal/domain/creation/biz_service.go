// 商业域补齐（S5 接口覆盖率）：会员订阅/套餐/模型/积分包公开查询端点。
package creation

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// BizService 商业域公开查询服务。
type BizService struct {
	repo *Repo
}

func NewBizService(repo *Repo) *BizService {
	return &BizService{repo: repo}
}

// MembershipDetail 9.9 我的会员订阅状态（GET /membership）。
func (s *BizService) MembershipDetail(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var planCode, status, renew, endAt string
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT package_code::text, status::text, COALESCE(renew_strategy::text,''),
		       COALESCE(current_period_end::text,'')
		FROM user_memberships WHERE user_id=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1`, uid).
		Scan(&planCode, &status, &renew, &endAt); err != nil {
		httpx.OK(c, gin.H{"plan_code": "BASIC", "status": "ACTIVE", "renew_strategy": "NONE", "expires_at": ""})
		return
	}
	httpx.OK(c, gin.H{"plan_code": planCode, "status": status, "renew_strategy": renew, "expires_at": endAt})
}

// MembershipCancel 9.10 取消自动续费订阅（POST /membership/cancel）。
func (s *BizService) MembershipCancel(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	tag, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE user_memberships SET cancel_at_period_end=TRUE
		WHERE user_id=$1 AND status='ACTIVE' AND renew_strategy='AUTO_RENEW'`, uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "取消失败")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusBadRequest, 20009, "无有效自动续费会员")
		return
	}
	httpx.OK(c, nil)
}

// ListPackages 9.1 套餐列表（GET /packages）。
func (s *BizService) ListPackages(c *gin.Context) {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, package_code, COALESCE(name,''), COALESCE(billing_cycle::text,''), price_yuan::text,
		       COALESCE(status::text,''), COALESCE(points_quota,0)
		FROM packages WHERE deleted_at IS NULL ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取套餐失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, points int
		var code, name, cycle, price, status string
		if err := rows.Scan(&id, &code, &name, &cycle, &price, &status, &points); err != nil {
			continue
		}
		items = append(items, map[string]any{"package_id": id, "package_code": code, "name": name, "billing_cycle": cycle, "price_yuan": price, "status": status, "points_quota": points})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListModels 10.2 模型列表（GET /models，按能力过滤）。
func (s *BizService) ListModels(c *gin.Context) {
	capability := c.Query("capability")
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT m.id, m.model_key, COALESCE(m.name,''), COALESCE(m.capability::text,''),
		       COALESCE(p.name,''), COALESCE(m.cost_points_per_call,0), m.enabled
		FROM models m LEFT JOIN model_providers p ON p.id=m.provider_id
		WHERE m.deleted_at IS NULL AND ($1='' OR m.capability::text=$1) ORDER BY m.sort_order ASC, m.id ASC`, capability)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取模型失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, cost int
		var key, name, cap, provider string
		var enabled bool
		if err := rows.Scan(&id, &key, &name, &cap, &provider, &cost, &enabled); err != nil {
			continue
		}
		items = append(items, map[string]any{"model_id": id, "model_key": key, "name": name, "capability": cap, "provider": provider, "cost_points": cost, "enabled": enabled})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ListPointPackages 9.2 积分包列表（GET /point-packages）。
func (s *BizService) ListPointPackages(c *gin.Context) {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, pkg_code, name, points, COALESCE(bonus_points,0), price_yuan::text,
		       COALESCE(status::text,'')
		FROM point_packages WHERE deleted_at IS NULL ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取积分包失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, points, bonus int
		var code, name, price, status string
		if err := rows.Scan(&id, &code, &name, &points, &bonus, &price, &status); err != nil {
			continue
		}
		items = append(items, map[string]any{"package_code": code, "id": id, "name": name, "points": points, "bonus_points": bonus, "price_yuan": price, "status": status})
	}
	httpx.OK(c, gin.H{"items": items})
}
