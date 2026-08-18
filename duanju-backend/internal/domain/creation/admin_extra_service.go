// 管理后台补齐（S5 接口覆盖率）：运营/治理/配置类 admin 端点（TAG 11 全集）。
// 全部路由在 handler 层挂 adminMW（SUPER_ADMIN）鉴权。
package creation

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
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
		SELECT COALESCE(plan_code::text,'BASIC') FROM user_memberships WHERE user_id=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1`, id).Scan(&planCode)
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
