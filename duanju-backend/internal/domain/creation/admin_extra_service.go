// 管理后台补齐（S5 接口覆盖率）：运营/治理/配置类 admin 端点（TAG 11 全集）。
// 全部路由在 handler 层挂 adminMW（SUPER_ADMIN）鉴权。
package creation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
	"github.com/duanju/backend/internal/pkg/password"
)

// ModelConfigStore 大模型配置数据边界（repository.ModelKeyStore 实现）。
type ModelConfigStore interface {
	ListProviders(ctx context.Context) ([]map[string]any, error)
	CreateProvider(ctx context.Context, name, displayName string, capabilities []string, baseURL string, enabled bool) (int64, error)
	UpdateProvider(ctx context.Context, id int64, displayName string, capabilities []string, baseURL string, enabled bool) error
	CreateModel(ctx context.Context, providerID int64, modelKey, name, capability string, costPoints int, enabled bool) (int64, error)
	UpdateModelByKey(ctx context.Context, modelKey, name, capability string, costPoints int, enabled bool) error
	CreateModelKey(ctx context.Context, providerID int64, name, apiKey string, rpm, qps, concurrent int, enabled bool, priority int) (int64, string, error)
	UpdateModelKey(ctx context.Context, id int64, rpm, qps, concurrent int, enabled bool, priority int) error
	RevokeModelKey(ctx context.Context, id int64) error
	ListModelKeys(ctx context.Context) ([]map[string]any, error)
	ListModelsAdmin(ctx context.Context) ([]map[string]any, error)
}

// AdminExtraService 管理后台补齐服务。
type AdminExtraService struct {
	repo       *Repo
	modelStore ModelConfigStore
}

func NewAdminExtraService(repo *Repo, modelStore ModelConfigStore) *AdminExtraService {
	return &AdminExtraService{repo: repo, modelStore: modelStore}
}

// adminList 通用表列表查询：SQL 全静态映射（无拼接、无注入面）。
func (s *AdminExtraService) adminList(c *gin.Context, table, orderBy string) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 100 || pageSize < 1 {
		pageSize = 20
	}
	// 表名白名单：静态 SQL 硬编码，拒绝一切动态拼接
	staticSQL := map[string]struct{ count, list string }{
		"marketing_activities": {`SELECT count(*) FROM marketing_activities`, `SELECT to_jsonb(t) FROM marketing_activities t ORDER BY id DESC LIMIT $1 OFFSET $2`},
		"agent_memory":        {`SELECT count(*) FROM agent_memory`, `SELECT to_jsonb(t) FROM agent_memory t ORDER BY id DESC LIMIT $1 OFFSET $2`},
		"categories":          {`SELECT count(*) FROM categories`, `SELECT to_jsonb(t) FROM categories t ORDER BY sort_order ASC, id ASC LIMIT $1 OFFSET $2`},
		"courses":             {`SELECT count(*) FROM courses`, `SELECT to_jsonb(t) FROM courses t ORDER BY sort_order ASC, id ASC LIMIT $1 OFFSET $2`},
		"directive_skills":    {`SELECT count(*) FROM directive_skills`, `SELECT to_jsonb(t) FROM directive_skills t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"languages":           {`SELECT count(*) FROM languages`, `SELECT to_jsonb(t) FROM languages t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"task_queues":         {`SELECT count(*) FROM task_queues`, `SELECT to_jsonb(t) FROM task_queues t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"rate_limits":         {`SELECT count(*) FROM rate_limits`, `SELECT to_jsonb(t) FROM rate_limits t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"themes":              {`SELECT count(*) FROM themes`, `SELECT to_jsonb(t) FROM themes t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"think_levels":        {`SELECT count(*) FROM think_levels`, `SELECT to_jsonb(t) FROM think_levels t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"model_keys":          {`SELECT count(*) FROM model_keys`, `SELECT to_jsonb(t) FROM model_keys t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"model_providers":     {`SELECT count(*) FROM model_providers`, `SELECT to_jsonb(t) FROM model_providers t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"platform_configs":    {`SELECT count(*) FROM platform_configs`, `SELECT to_jsonb(t) FROM platform_configs t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"point_packages":      {`SELECT count(*) FROM point_packages`, `SELECT to_jsonb(t) FROM point_packages t ORDER BY sort_order ASC, id ASC LIMIT $1 OFFSET $2`},
		"style_library":       {`SELECT count(*) FROM style_library`, `SELECT to_jsonb(t) FROM style_library t ORDER BY sort_order ASC, id ASC LIMIT $1 OFFSET $2`},
		"hints":               {`SELECT count(*) FROM hints`, `SELECT to_jsonb(t) FROM hints t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"announcements":       {`SELECT count(*) FROM announcements`, `SELECT to_jsonb(t) FROM announcements t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"banners":             {`SELECT count(*) FROM banners`, `SELECT to_jsonb(t) FROM banners t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"packages":            {`SELECT count(*) FROM packages`, `SELECT to_jsonb(t) FROM packages t ORDER BY id ASC LIMIT $1 OFFSET $2`},
		"models":              {`SELECT count(*) FROM models`, `SELECT to_jsonb(t) FROM models t ORDER BY id ASC LIMIT $1 OFFSET $2`},
	}
	q, ok := staticSQL[table]
	if !ok {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "不支持的列表类型")
		return
	}
	var total int
	_ = s.repo.pool.QueryRow(c.Request.Context(), q.count).Scan(&total)
	rows, err := s.repo.pool.Query(c.Request.Context(), q.list, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取列表失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var item map[string]any
		if json.Unmarshal(raw, &item) == nil {
			items = append(items, item)
		}
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// ---- 11.79 活动管理 ----
func (s *AdminExtraService) Activities(c *gin.Context) { s.adminList(c, "marketing_activities", "id DESC") }

// ---- 11.61 智能体记忆管理 ----
func (s *AdminExtraService) AgentMemory(c *gin.Context) {
	userID := c.DefaultQuery("user_id", "")
	if userID == "" {
		s.adminList(c, "agent_memory", "id DESC")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 100 || pageSize < 1 {
		pageSize = 20
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, user_id, COALESCE(project_id,0), memory_type, content, token_count,
		       to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM agent_memory WHERE user_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`,
		userID, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取记忆失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, uid, pid int64
		var mtype, content, created string
		var tokens int
		if err := rows.Scan(&id, &uid, &pid, &mtype, &content, &tokens, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "user_id": uid, "project_id": pid, "memory_type": mtype, "content": content, "token_count": tokens, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 11.49 智能体技能管理 ----
func (s *AdminExtraService) AgentSkills(c *gin.Context) {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, COALESCE(skill_code,''), COALESCE(name,''), COALESCE(description,''),
		       COALESCE(status::text,'')
		FROM agent_skills ORDER BY id ASC`)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取技能失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var code, name, desc, status string
		if err := rows.Scan(&id, &code, &name, &desc, &status); err != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "skill_code": code, "name": name, "description": desc, "status": status})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 11.0 当前管理员权限 ----
func (s *AdminExtraService) AuthPermissions(c *gin.Context) {
	uid := userID(c)
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT r.code::text, r.name, COALESCE(r.permissions::text,'[]')
		FROM user_roles ur JOIN roles r ON r.id=ur.role_id
		WHERE ur.user_id=$1 AND ur.deleted_at IS NULL`, uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取权限失败")
		return
	}
	defer rows.Close()
	roles := make([]map[string]any, 0)
	perms := map[string]bool{}
	for rows.Next() {
		var code, name, pjson string
		if err := rows.Scan(&code, &name, &pjson); err != nil {
			continue
		}
		var plist []string
		_ = json.Unmarshal([]byte(pjson), &plist)
		for _, p := range plist {
			perms[p] = true
		}
		roles = append(roles, map[string]any{"code": code, "name": name, "permissions": plist})
	}
	permList := make([]string, 0, len(perms))
	for p := range perms {
		permList = append(permList, p)
	}
	httpx.OK(c, gin.H{"roles": roles, "permissions": permList})
}

// ---- 11.18 分类/标签列表 ----
func (s *AdminExtraService) Categories(c *gin.Context) { s.adminList(c, "categories", "sort_order ASC, id ASC") }

// ---- 11.24 评论管理 ----
func (s *AdminExtraService) Comments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 100 || pageSize < 1 {
		pageSize = 20
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT cm.id, cm.user_id, COALESCE(u.nickname,''), cm.work_id, w.title,
		       cm.content, COALESCE(cm.status::text,''),
		       to_char(cm.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM comments cm
		LEFT JOIN users u ON u.id=cm.user_id
		LEFT JOIN works w ON w.id=cm.work_id
		WHERE cm.deleted_at IS NULL ORDER BY cm.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取评论失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, uid, wid int64
		var nickname, title, content, status, created string
		if err := rows.Scan(&id, &uid, &nickname, &wid, &title, &content, &status, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"comment_id": id, "user_id": uid, "nickname": nickname, "work_id": wid, "work_title": title, "content": content, "status": status, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 11.87 课程管理 ----
func (s *AdminExtraService) Courses(c *gin.Context) { s.adminList(c, "courses", "sort_order ASC, id ASC") }

// ---- 11.1 平台概览 Dashboard 指标 ----
func (s *AdminExtraService) DashboardSummary(c *gin.Context) {
	var users, spaces, works, tasks, pendingReports int
	var revenue float64
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM users`).Scan(&users)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM spaces`).Scan(&spaces)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM works WHERE is_deleted_recyclebin=FALSE`).Scan(&works)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM task_orders WHERE deleted_at IS NULL`).Scan(&tasks)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM reports WHERE status='PENDING'`).Scan(&pendingReports)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(paid_amount_yuan),0) FROM orders WHERE pay_status='PAID'`).Scan(&revenue)
	httpx.OK(c, gin.H{
		"total_users": users, "total_spaces": spaces, "total_works": works,
		"total_tasks": tasks, "pending_reports": pendingReports, "revenue_yuan": revenue,
	})
}

// ---- 11.57 导演叙事技能库 ----
func (s *AdminExtraService) DirectiveSkills(c *gin.Context) { s.adminList(c, "directive_skills", "id ASC") }

// ---- 11.69 多语言配置 ----
func (s *AdminExtraService) Languages(c *gin.Context) { s.adminList(c, "languages", "id ASC") }

// ---- 11.43 模型 API Key 管理 ----
func (s *AdminExtraService) ModelKeys(c *gin.Context) {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT id, COALESCE(provider_id,0), COALESCE(model_key,''), COALESCE(api_key_prefix,''),
		       COALESCE(status::text,''), COALESCE(priority,0), COALESCE(quota_per_min,60)
		FROM model_keys WHERE deleted_at IS NULL ORDER BY priority DESC, id ASC`)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取密钥失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, providerID, priority, quota int64
		var key, prefix, status string
		if err := rows.Scan(&id, &providerID, &key, &prefix, &status, &priority, &quota); err != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "provider_id": providerID, "model_key": key, "api_key_prefix": prefix, "status": status, "priority": priority, "quota_per_min": quota})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 11.75 平台配置列表 ----
func (s *AdminExtraService) PlatformConfig(c *gin.Context) { s.adminList(c, "platform_configs", "id ASC") }

// ---- 11.30 积分包管理 ----
func (s *AdminExtraService) PointPackages(c *gin.Context) { s.adminList(c, "point_packages", "sort_order ASC, id ASC") }

// ---- 11.35 模型厂商管理 ----
func (s *AdminExtraService) Providers(c *gin.Context) { s.adminList(c, "model_providers", "id ASC") }

// ---- 11.73 任务并发/队列管理 ----
func (s *AdminExtraService) Queues(c *gin.Context) { s.adminList(c, "task_queues", "id ASC") }

// ---- 11.47 接口限流配置 ----
func (s *AdminExtraService) RateLimits(c *gin.Context) { s.adminList(c, "rate_limits", "id ASC") }

// ---- 11.12 空间列表（全局治理） ----
func (s *AdminExtraService) Spaces(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 100 || pageSize < 1 {
		pageSize = 20
	}
	var total int
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM spaces WHERE deleted_at IS NULL`).Scan(&total)
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT s.id, s.space_uuid::text, s.name, COALESCE(s.type::text,''),
		       COALESCE(s.status::text,''), COALESCE(u.nickname,''),
		       to_char(s.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM spaces s LEFT JOIN users u ON u.id=s.owner_user_id
		WHERE s.deleted_at IS NULL ORDER BY s.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取空间失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var suuid, name, stype, status, owner, created string
		if err := rows.Scan(&id, &suuid, &name, &stype, &status, &owner, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"space_id": suuid, "id": id, "name": name, "type": stype, "status": status, "owner": owner, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// ---- 11.13 空间治理（关闭/降配/恢复） ----
func (s *AdminExtraService) SpaceStatus(c *gin.Context, spaceUUID string) {
	var req struct {
		Status string `json:"status" binding:"required,oneof=ACTIVE CLOSED DOWNGRADED"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE spaces SET status=$2, updated_at=NOW() WHERE space_uuid=$1::uuid`, spaceUUID, req.Status); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新空间状态失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 11.91 数据报表 ----
func (s *AdminExtraService) Statistics(c *gin.Context) {
	var regTotal, regToday, gmv, taskTotal, taskSucc int
	var succRate float64
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM users`).Scan(&regTotal)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM users WHERE created_at > NOW() - interval '24 hours'`).Scan(&regToday)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT COALESCE(SUM(points_granted_total),0) FROM orders WHERE pay_status='PAID'`).Scan(&gmv)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM generation_tasks WHERE deleted_at IS NULL`).Scan(&taskTotal)
	_ = s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM generation_tasks WHERE status='SUCCESS' AND deleted_at IS NULL`).Scan(&taskSucc)
	if taskTotal > 0 {
		succRate = float64(taskSucc) / float64(taskTotal)
	}
	httpx.OK(c, gin.H{
		"registered_total": regTotal, "registered_today": regToday,
		"gmv_points": gmv, "task_total": taskTotal, "task_success_rate": succRate,
	})
}

// ---- Dashboard 趋势图（近 N 天按日序列，默认 7 天，供 admin-dashboard 双趋势图） ----
func (s *AdminExtraService) Trends(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days < 1 || days > 30 {
		days = 7
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT to_char(d.day,'MM-DD') AS day,
		       COALESCE(u.reg,0), COALESCE(a.act,0),
		       COALESCE(o.amt,0)::float8, COALESCE(o.payers,0)
		FROM generate_series(CURRENT_DATE - make_interval(days => $1 - 1), CURRENT_DATE, INTERVAL '1 day') AS d(day)
		LEFT JOIN (
		  SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS day, count(*) AS reg
		  FROM users GROUP BY 1
		) u ON u.day = d.day::date
		LEFT JOIN (
		  SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS day, count(DISTINCT user_id) AS act
		  FROM generation_tasks WHERE deleted_at IS NULL GROUP BY 1
		) a ON a.day = d.day::date
		LEFT JOIN (
		  SELECT (paid_at AT TIME ZONE 'Asia/Shanghai')::date AS day,
		         COALESCE(SUM(paid_amount_yuan),0) AS amt, count(DISTINCT user_id) AS payers
		  FROM orders WHERE pay_status='PAID' AND paid_at IS NOT NULL GROUP BY 1
		) o ON o.day = d.day::date
		ORDER BY d.day`, days)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取趋势失败")
		return
	}
	defer rows.Close()
	labels := make([]string, 0)
	regs := make([]int, 0)
	acts := make([]int, 0)
	amts := make([]float64, 0)
	payers := make([]int, 0)
	for rows.Next() {
		var label string
		var reg, act, payer int
		var amt float64
		if err := rows.Scan(&label, &reg, &act, &amt, &payer); err != nil {
			continue
		}
		labels = append(labels, label)
		regs = append(regs, reg)
		acts = append(acts, act)
		amts = append(amts, amt)
		payers = append(payers, payer)
	}
	httpx.OK(c, gin.H{
		"days": labels, "registrations": regs, "actives": acts,
		"revenue_yuan": amts, "payers": payers,
	})
}

// ---- 11.53 风格库管理 ----
func (s *AdminExtraService) StyleLibrary(c *gin.Context) { s.adminList(c, "style_library", "sort_order ASC, id ASC") }

// ---- 11.65 界面主题配置 ----
func (s *AdminExtraService) Themes(c *gin.Context) { s.adminList(c, "themes", "id ASC") }

// ---- 11.63 思考等级配置 ----
func (s *AdminExtraService) ThinkLevels(c *gin.Context) { s.adminList(c, "think_levels", "id ASC") }

// ---- 11.3 用户详情 ----
func (s *AdminExtraService) UserDetail(c *gin.Context, userUUID string) {
	var id int64
	var nickname, email, phone, status, createdAt string
	var points, frozen int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT u.id, COALESCE(u.nickname,''), COALESCE(u.email,''), COALESCE(u.phone,''),
		       COALESCE(u.status::text,''),
		       to_char(u.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM users u WHERE u.user_uuid=$1::uuid AND u.deleted_at IS NULL`, userUUID).
		Scan(&id, &nickname, &email, &phone, &status, &createdAt); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "用户不存在")
		return
	}
	_ = s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(current_balance,0), COALESCE(frozen_balance,0) FROM point_accounts
		WHERE owner_type='USER' AND owner_id=$1`, id).Scan(&points, &frozen)
	var planCode string
	_ = s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(package_code::text,'BASIC') FROM user_memberships WHERE user_id=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1`, id).Scan(&planCode)
	httpx.OK(c, gin.H{
		"user_uuid": userUUID, "nickname": nickname, "email": email, "phone": phone,
		"status": status, "created_at": createdAt,
		"points": gin.H{"current_balance": points, "frozen_balance": frozen},
		"membership_plan": planCode,
	})
}

// ---- 11.5 开通/变更用户会员套餐 ----
func (s *AdminExtraService) UserMembership(c *gin.Context, userUUID string) {
	var req struct {
		PlanCode   string `json:"plan_code" binding:"required"`
		Days       int    `json:"days" binding:"required,min=1,max=3650"`
		RenewStrategy string `json:"renew_strategy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.RenewStrategy == "" {
		req.RenewStrategy = "NONE"
	}
	var uid int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id FROM users WHERE user_uuid=$1::uuid`, userUUID).Scan(&uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "用户不存在")
		return
	}
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "操作失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	if _, err := tx.Exec(c.Request.Context(), `UPDATE user_memberships SET status='EXPIRED' WHERE user_id=$1 AND status='ACTIVE'`, uid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "操作失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO user_memberships (user_id, plan_code, status, renew_strategy, start_at, current_period_end, total_cycles_paid)
		VALUES ($1, $2, 'ACTIVE', $3, NOW(), NOW() + ($4 || ' days')::interval, 1)`,
		uid, req.PlanCode, req.RenewStrategy, req.Days); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "操作失败")
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "操作失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 11.6 积分调整 ----
func (s *AdminExtraService) UserPoints(c *gin.Context, userUUID string) {
	uid := userID(c)
	var req struct {
		Amount  int    `json:"amount" binding:"required"`
		Reason  string `json:"reason" binding:"required,min=1,max=200"`
		Freeze  bool   `json:"freeze"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	var targetID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id FROM users WHERE user_uuid=$1::uuid`, userUUID).Scan(&targetID); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "用户不存在")
		return
	}
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "调整失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	direction := "CREDIT"
	amount := req.Amount
	if req.Amount < 0 {
		direction = "DEBIT"
		amount = -req.Amount
	}
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE point_accounts SET current_balance=current_balance+$2, updated_at=NOW()
		WHERE owner_type='USER' AND owner_id=$1`, targetID, req.Amount); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "调整失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
			balance_after, operator_user_id, description, idempotency_key)
		SELECT pa.id, 'USER', $1, 'ADMIN_COMP', $4, $5, pa.current_balance, $2, $3, 'admin-points-'||$1||'-'||extract(epoch from now())::text
		FROM point_accounts pa WHERE pa.owner_type='USER' AND pa.owner_id=$1`,
		targetID, uid, req.Reason, direction, amount); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "调整失败")
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "调整失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 11.11 分配平台角色 ----
func (s *AdminExtraService) UserRoles(c *gin.Context, userUUID string) {
	uid := userID(c)
	var req struct {
		RoleCodes []string `json:"role_codes" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	var targetID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id FROM users WHERE user_uuid=$1::uuid`, userUUID).Scan(&targetID); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "用户不存在")
		return
	}
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "分配失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	// 清旧角色 + 授新角色
	if _, err := tx.Exec(c.Request.Context(), `DELETE FROM user_roles WHERE user_id=$1`, targetID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "分配失败")
		return
	}
	for _, code := range req.RoleCodes {
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO user_roles (user_id, role_id, granted_by_user_id)
			SELECT $1, id, $2 FROM roles WHERE code=$3`, targetID, uid, code); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "分配失败")
			return
		}
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "分配失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 11.4 启用/禁用/封禁用户 ----
func (s *AdminExtraService) UserStatus(c *gin.Context, userUUID string) {
	var req struct {
		Status string `json:"status" binding:"required,oneof=ACTIVE DISABLED BANNED"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE users SET status=$2, updated_at=NOW() WHERE user_uuid=$1::uuid`, userUUID, req.Status); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新状态失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 11.16 作品审核 ----
func (s *AdminExtraService) WorkAudit(c *gin.Context, workUUID string) {
	uid := userID(c)
	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	status := "PUBLISHED"
	action := "APPROVE"
	if !req.Approve {
		status = "REJECTED"
		action = "REJECT"
	}
	var wid int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id FROM works WHERE work_uuid=$1::uuid`, workUUID).Scan(&wid); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET publish_status=$2, audit_note=$3, published_at=NOW()
		WHERE id=$1 AND publish_status='PENDING'`, wid, status, req.Note); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "审核失败")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO publish_records (work_id, action, submitter_user_id, snapshot_after_json)
		VALUES ($1, $2, $3, jsonb_build_object('status',$4))`, wid, action, uid, status); err != nil {
		// 记录失败不阻塞
	}
	httpx.OK(c, nil)
}

// ---- 11.17 下架已发布作品 ----
func (s *AdminExtraService) WorkUnlist(c *gin.Context, workUUID string) {
	uid := userID(c)
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	var wid int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT id FROM works WHERE work_uuid=$1::uuid`, workUUID).Scan(&wid); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET publish_status='REVOKED', audit_note=$2, updated_at=NOW()
		WHERE id=$1 AND publish_status='PUBLISHED'`, wid, req.Note); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "下架失败")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO publish_records (work_id, action, submitter_user_id, snapshot_after_json)
		VALUES ($1, 'REVOKE', $2, jsonb_build_object('status','REVOKED'))`, wid, uid); err != nil {
		// 记录失败不阻塞
	}
	httpx.OK(c, nil)
}

// ==================== 大模型配置管理（11.35/11.39/11.43：厂商/模型/Key 三级 CRUD） ====================
// 配置真源：model_providers / models / model_keys 表；worker 侧 DB 驱动 KeyStore 实时生效。

// ModelProvidersList 厂商列表（管理端，含能力）。
func (s *AdminExtraService) ModelProvidersList(c *gin.Context) {
	if s.modelStore == nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "模型配置服务未初始化")
		return
	}
	items, err := s.modelStore.ListProviders(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取厂商失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ModelProviderCreate 新建厂商（POST /admin/providers）。
func (s *AdminExtraService) ModelProviderCreate(c *gin.Context) {
	var req struct {
		Name         string   `json:"name" binding:"required,min=1,max=64"`
		DisplayName  string   `json:"display_name" binding:"required,min=1,max=64"`
		Capabilities []string `json:"capabilities" binding:"required,min=1"`
		BaseURL      string   `json:"base_url" binding:"required,url"`
		Enabled      *bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id, err := s.modelStore.CreateProvider(c.Request.Context(), req.Name, req.DisplayName, req.Capabilities, req.BaseURL, enabled)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建厂商失败")
		return
	}
	httpx.OK(c, gin.H{"id": id})
}

// ModelProviderUpdate 更新厂商（PUT /admin/providers/:id）。
func (s *AdminExtraService) ModelProviderUpdate(c *gin.Context, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		DisplayName  string   `json:"display_name"`
		Capabilities []string `json:"capabilities"`
		BaseURL      string   `json:"base_url"`
		Enabled      *bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := s.modelStore.UpdateProvider(c.Request.Context(), id, req.DisplayName, req.Capabilities, req.BaseURL, enabled); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新厂商失败")
		return
	}
	httpx.OK(c, nil)
}

// ModelListAdmin 模型列表（含厂商名，11.39）。
func (s *AdminExtraService) ModelListAdmin(c *gin.Context) {
	items, err := s.modelStore.ListModelsAdmin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取模型失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ModelCreate 新建模型（POST /admin/models）。
func (s *AdminExtraService) ModelCreate(c *gin.Context) {
	var req struct {
		ProviderID int64  `json:"provider_id" binding:"required"`
		ModelKey   string `json:"model_key" binding:"required,min=1,max=64"`
		Name       string `json:"name" binding:"required,min=1,max=64"`
		Capability string `json:"capability" binding:"required,oneof=LLM T2I I2I T2V I2V AUDIO"`
		CostPoints int    `json:"cost_points_per_call" binding:"min=0"`
		Enabled    *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	id, err := s.modelStore.CreateModel(c.Request.Context(), req.ProviderID, req.ModelKey, req.Name, req.Capability, req.CostPoints, enabled)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建模型失败")
		return
	}
	httpx.OK(c, gin.H{"id": id})
}

// ModelKeyList 模型 API Key 列表（隐藏密文，11.43）。
func (s *AdminExtraService) ModelKeyList(c *gin.Context) {
	items, err := s.modelStore.ListModelKeys(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取密钥失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ModelKeyCreate 新建 API Key（密文入库，明文仅返回一次，POST /admin/model-keys）。
func (s *AdminExtraService) ModelKeyCreate(c *gin.Context) {
	var req struct {
		ProviderID int64  `json:"provider_id" binding:"required"`
		Name       string `json:"name" binding:"required,min=1,max=64"`
		APIKey     string `json:"api_key" binding:"required,min=8,max=256"`
		RPM        int    `json:"rate_limit_rpm" binding:"min=0,max=100000"`
		QPS        int    `json:"rate_limit_qps" binding:"min=0,max=100"`
		Concurrent int    `json:"concurrent_limit" binding:"min=1,max=64"`
		Priority   int    `json:"priority" binding:"min=0,max=100"`
		Enabled    *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.Concurrent == 0 {
		req.Concurrent = 4
	}
	id, prefix, err := s.modelStore.CreateModelKey(c.Request.Context(), req.ProviderID, req.Name, req.APIKey,
		req.RPM, req.QPS, req.Concurrent, enabled, req.Priority)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建密钥失败")
		return
	}
	httpx.OK(c, gin.H{"id": id, "key_prefix": prefix, "tip": "明文仅展示一次，请妥善保存"})
}

// ModelKeyUpdate 更新 API Key 配置（PUT /admin/model-keys/:id；不更新密文）。
func (s *AdminExtraService) ModelKeyUpdate(c *gin.Context, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	var req struct {
		RPM        int   `json:"rate_limit_rpm" binding:"min=0,max=100000"`
		QPS        int   `json:"rate_limit_qps" binding:"min=0,max=100"`
		Concurrent int   `json:"concurrent_limit" binding:"min=0,max=64"`
		Priority   int   `json:"priority" binding:"min=0,max=100"`
		Enabled    *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := s.modelStore.UpdateModelKey(c.Request.Context(), id, req.RPM, req.QPS, req.Concurrent, enabled, req.Priority); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新密钥失败")
		return
	}
	httpx.OK(c, nil)
}

// ModelKeyRevoke 吊销 API Key（POST /admin/model-keys/:id/revoke）。
func (s *AdminExtraService) ModelKeyRevoke(c *gin.Context, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "id 非法")
		return
	}
	if err := s.modelStore.RevokeModelKey(c.Request.Context(), id); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// ---- 算力任务管理（11.57：generation_tasks 列表/统计/管理端重试与取消） ----

// Tasks 任务列表（status/task_type 可选过滤）。
func (s *AdminExtraService) Tasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := strings.ToUpper(c.DefaultQuery("status", ""))
	taskType := strings.ToUpper(c.DefaultQuery("task_type", ""))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int
	where := `t.deleted_at IS NULL`
	args := make([]any, 0, 3)
	if status != "" {
		// 支持逗号分隔多值（如 status=QUEUED 或 status=PENDING,QUEUED）
		parts := strings.Split(status, ",")
		in := make([]string, 0, len(parts))
		for _, p := range parts {
			args = append(args, p)
			in = append(in, fmt.Sprintf("$%d", len(args)))
		}
		where += " AND t.status IN (" + strings.Join(in, ",") + ")"
	}
	if taskType != "" {
		args = append(args, taskType)
		where += fmt.Sprintf(" AND t.task_type=$%d", len(args))
	}
	if err := s.repo.pool.QueryRow(c.Request.Context(), `SELECT count(*) FROM generation_tasks t WHERE `+where, args...).Scan(&total); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取任务失败")
		return
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT t.task_uuid::text, t.request_id, t.task_type::text, t.status::text, COALESCE(t.progress_pct,0),
		       COALESCE(p.title,''), COALESCE(u.nickname,''), COALESCE(t.gpu_node_id,''),
		       COALESCE(t.total_execution_ms,0),
		       to_char(t.created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI')
		FROM generation_tasks t
		LEFT JOIN projects p ON p.id=t.project_id
		LEFT JOIN users u ON u.id=t.user_id
		WHERE `+where+` ORDER BY t.id DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)),
		args...)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取任务失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var taskUUID, requestID, taskTypeV, st, projectTitle, author, gpuNode, createdAt string
		var progress, execMs int
		if err := rows.Scan(&taskUUID, &requestID, &taskTypeV, &st, &progress, &projectTitle, &author, &gpuNode, &execMs, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"task_uuid": taskUUID, "request_id": requestID, "task_type": taskTypeV, "status": st,
			"progress_pct": progress, "project_title": projectTitle, "author": author,
			"gpu_node_id": gpuNode, "execution_ms": execMs, "created_at": createdAt,
		})
	}
	httpx.OK(c, gin.H{"items": out, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// TaskStats 任务统计（队列计数 + 平均耗时）。
func (s *AdminExtraService) TaskStats(c *gin.Context) {
	var queued, running, doneToday, failed, avgMs int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT
		  (SELECT count(*) FROM generation_tasks WHERE deleted_at IS NULL AND status IN ('PENDING','QUEUED')),
		  (SELECT count(*) FROM generation_tasks WHERE deleted_at IS NULL AND status='RUNNING'),
		  (SELECT count(*) FROM generation_tasks WHERE deleted_at IS NULL AND status='SUCCESS' AND completed_at > NOW()-interval '24 hours'),
		  (SELECT count(*) FROM generation_tasks WHERE deleted_at IS NULL AND status IN ('FAILED_RETRYING','FAILED_DEAD')),
		  (SELECT COALESCE(AVG(total_execution_ms),0)::int FROM generation_tasks WHERE deleted_at IS NULL AND status='SUCCESS' AND total_execution_ms > 0)`).
		Scan(&queued, &running, &doneToday, &failed, &avgMs); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取任务统计失败")
		return
	}
	httpx.OK(c, gin.H{
		"queued": queued, "running": running, "done_today": doneToday, "failed": failed,
		"avg_execution_ms": avgMs,
	})
}

// TaskRetryAdmin 管理端强制重试失败任务（POST /admin/tasks/:task_id/retry）。
func (s *AdminExtraService) TaskRetryAdmin(c *gin.Context, taskUUID string) {
	tag, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE generation_tasks SET status='QUEUED', enqueued_at=NOW(), error_message=NULL, retry_count=0, updated_at=NOW()
		WHERE task_uuid=$1 AND deleted_at IS NULL AND status IN ('FAILED_RETRYING','FAILED_DEAD')`, taskUUID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重试失败")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusBadRequest, 20009, "任务不存在或不可重试")
		return
	}
	httpx.OK(c, nil)
}

// TaskCancelAdmin 管理端强制取消任务（POST /admin/tasks/:task_id/cancel）。
func (s *AdminExtraService) TaskCancelAdmin(c *gin.Context, taskUUID string) {
	tag, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE generation_tasks SET status='CANCELED_USER', updated_at=NOW()
		WHERE task_uuid=$1 AND deleted_at IS NULL AND status IN ('PENDING','QUEUED','RUNNING')`, taskUUID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "取消失败")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusBadRequest, 20009, "任务不存在或不可取消")
		return
	}
	httpx.OK(c, nil)
}

// TaskDetailAdmin 任务详情（GET /admin/tasks/:task_id；含参数/结果/错误/进度）。
func (s *AdminExtraService) TaskDetailAdmin(c *gin.Context, taskUUID string) {
	var taskID, progress, priority, retryCount int
	var requestID, taskType, st, gpuNode, externalID, aiModel string
	var paramsJSON, resultJSON string
	var errorMsg, userError *string
	var enqueuedAt, startedAt, completedAt, createdAt string
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, request_id, task_type::text, status::text, COALESCE(progress_pct,0),
		       priority, retry_count, COALESCE(gpu_node_id,''), COALESCE(external_task_id,''),
		       COALESCE(ai_model_actual_used,''), COALESCE(request_params_json::text,'{}'),
		       COALESCE(result_output_json::text,'{}'), error_message, user_display_error,
		       COALESCE(to_char(enqueued_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI'),''),
		       COALESCE(to_char(started_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI'),''),
		       COALESCE(to_char(completed_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI'),''),
		       to_char(created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI')
		FROM generation_tasks WHERE task_uuid=$1 AND deleted_at IS NULL`, taskUUID).
		Scan(&taskID, &requestID, &taskType, &st, &progress, &priority, &retryCount, &gpuNode,
			&externalID, &aiModel, &paramsJSON, &resultJSON, &errorMsg, &userError,
			&enqueuedAt, &startedAt, &completedAt, &createdAt); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "任务不存在")
		return
	}
	var params, result any
	_ = json.Unmarshal([]byte(paramsJSON), &params)
	_ = json.Unmarshal([]byte(resultJSON), &result)
	httpx.OK(c, gin.H{
		"task_id": taskID, "task_uuid": taskUUID, "request_id": requestID, "task_type": taskType,
		"status": st, "progress_pct": progress, "priority": priority, "retry_count": retryCount,
		"gpu_node_id": gpuNode, "external_task_id": externalID, "ai_model_actual_used": aiModel,
		"request_params": params, "result_output": result,
		"error_message": errorMsg, "user_display_error": userError,
		"enqueued_at": enqueuedAt, "started_at": startedAt, "completed_at": completedAt, "created_at": createdAt,
	})
}

// TaskPriorityAdmin 调整任务优先级（PUT /admin/tasks/:task_id/priority；仅排队/生成中）。
func (s *AdminExtraService) TaskPriorityAdmin(c *gin.Context, taskUUID string) {
	var req struct {
		Priority int `json:"priority" binding:"required,min=1,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	tag, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE generation_tasks SET priority=$2, updated_at=NOW()
		WHERE task_uuid=$1 AND deleted_at IS NULL AND status IN ('PENDING','QUEUED')`, taskUUID, req.Priority)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "调整失败")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusBadRequest, 20009, "任务不存在或已开始执行，无法调整优先级")
		return
	}
	httpx.OK(c, nil)
}

// ---- 作品管理补充（编辑/推荐位） ----

// WorkUpdate 编辑作品（PUT /admin/works/:work_id；title/description/tags）。
func (s *AdminExtraService) WorkUpdate(c *gin.Context, workUUID string) {
	var req struct {
		Title       string   `json:"title" binding:"required,min=1,max=256"`
		Description string   `json:"description" binding:"max=2000"`
		Tags        []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	tagsJSON := "[]"
	if len(req.Tags) > 0 {
		b, _ := json.Marshal(req.Tags)
		tagsJSON = string(b)
	}
	tag, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET title=$2, description=$3, tags=$4::jsonb, updated_at=NOW()
		WHERE work_uuid=$1::uuid AND is_deleted_recyclebin=FALSE`, workUUID, req.Title, req.Description, tagsJSON)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新失败")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	httpx.OK(c, nil)
}

// WorkRecommend 设置/取消推荐位（PUT /admin/works/:work_id/recommend）。
func (s *AdminExtraService) WorkRecommend(c *gin.Context, workUUID string) {
	var req struct {
		Recommended bool `json:"recommended"`
	}
	_ = c.ShouldBindJSON(&req)
	tag, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET is_recommended=$2, updated_at=NOW()
		WHERE work_uuid=$1::uuid AND is_deleted_recyclebin=FALSE AND publish_status='PUBLISHED'`, workUUID, req.Recommended)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "设置失败")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusBadRequest, 20009, "作品不存在或非已发布状态")
		return
	}
	httpx.OK(c, nil)
}

// ---- 用户管理补充：管理端新增用户（POST /admin/users） ----

// AdminCreateUser 管理端创建用户（邮箱+密码+昵称；可选初始积分与角色）。
func (s *AdminExtraService) AdminCreateUser(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Email    string   `json:"email" binding:"required,email,max=128"`
		Password string   `json:"password" binding:"required,min=8,max=64"`
		Nickname string   `json:"nickname" binding:"required,min=2,max=64"`
		Points   int      `json:"points"`
		RoleCodes []string `json:"role_codes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	hash, err := password.Hash(req.Password)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "密码加密失败")
		return
	}
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	var newUserID int64
	if err := tx.QueryRow(c.Request.Context(), `
		INSERT INTO users (email, phone, password_hash, nickname, status)
		VALUES ($1, '', $2, $3, 'ACTIVE') RETURNING id`,
		req.Email, hash, req.Nickname).Scan(&newUserID); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "23505") {
			httpx.Fail(c, http.StatusBadRequest, 20009, "邮箱已被注册")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建用户失败")
		return
	}
	// 默认空间（与注册链路一致：个人空间）
	if _, err := tx.Exec(c.Request.Context(), `
		INSERT INTO spaces (owner_user_id, name, type, status)
		VALUES ($1, $2, 'PERSONAL', 'ACTIVE')`,
		newUserID, req.Nickname+" 的个人空间"); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建空间失败")
		return
	}
	// 初始积分
	if req.Points != 0 {
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO point_accounts (owner_type, owner_id, current_balance, frozen_balance)
			VALUES ('USER', $1, $2, 0)
			ON CONFLICT (owner_type, owner_id) DO UPDATE SET current_balance=point_accounts.current_balance+$2, updated_at=NOW()`,
			newUserID, req.Points); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "初始化积分失败")
			return
		}
	}
	// 角色绑定
	for _, code := range req.RoleCodes {
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO user_roles (user_id, role_id, granted_by_user_id)
			SELECT $1, id, $2 FROM roles WHERE code=$3`,
			newUserID, uid, code); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "角色绑定失败")
			return
		}
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建失败")
		return
	}
	httpx.OK(c, gin.H{"user_id": newUserID})
}
