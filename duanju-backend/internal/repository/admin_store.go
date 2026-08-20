// admin_store 实现 creation.AdminStore：指标/用户/内容/举报。
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/domain/creation"
)

// AdminStore 管理后台数据访问。
type AdminStore struct {
	pool *pgxpool.Pool
}

func NewAdminStore(pool *pgxpool.Pool) *AdminStore {
	return &AdminStore{pool: pool}
}

var _ creation.AdminStore = (*AdminStore)(nil)

func (s *AdminStore) Dashboard(ctx context.Context) (map[string]any, error) {
	var users, spaces, works, tasks, reports int
	var dau int64
	var revenue float64
	s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users)
	s.pool.QueryRow(ctx, `SELECT count(*) FROM spaces`).Scan(&spaces)
	s.pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE is_deleted_recyclebin=FALSE`).Scan(&works)
	s.pool.QueryRow(ctx, `SELECT count(*) FROM task_orders WHERE deleted_at IS NULL`).Scan(&tasks)
	s.pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE publish_status='PENDING' AND is_deleted_recyclebin=FALSE`).Scan(&reports)
	// DAU 近似：近 24h 活跃（无登录表时以近 24h 注册量近似）
	s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE created_at > NOW() - interval '24 hours'`).Scan(&dau)
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(paid_amount_yuan),0) FROM orders WHERE pay_status='PAID'`).Scan(&revenue)
	return map[string]any{
		"total_users": users, "dau": dau, "total_spaces": spaces, "total_works": works,
		"total_tasks": tasks, "pending_reports": reports, "revenue_yuan": revenue,
	}, nil
}

func (s *AdminStore) ListUsers(ctx context.Context, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT user_uuid::text, COALESCE(nickname,''), COALESCE(email,''), status::text,
		       to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM users ORDER BY id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, nick, email, status, created string
		rows.Scan(&uuid, &nick, &email, &status, &created)
		out = append(out, map[string]any{
			"user_uuid": uuid, "nickname": nick, "email": email, "status": status, "created_at": created,
		})
	}
	return out, total, rows.Err()
}

func (s *AdminStore) BanUser(ctx context.Context, userUUID string, banned bool, operatorID int64) error {
	status := "ACTIVE"
	if banned {
		status = "BANNED"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE users SET status=$2 WHERE user_uuid=$1::uuid`, userUUID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("用户不存在")
	}
	// 审计日志（audit_logs 表结构见迁移 000011：user_id/operator_role/detail_json）
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (user_id, operator_role, action, target_type, target_id, detail_json)
		VALUES ($1, 'SUPER_ADMIN', 'ADMIN_BAN', 'USER', (SELECT id FROM users WHERE user_uuid=$2::uuid), jsonb_build_object('note', $3::text))`,
		operatorID, userUUID, "封禁/解封"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *AdminStore) ListWorks(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	var total int
	if status == "" {
		s.pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE is_deleted_recyclebin=FALSE`).Scan(&total)
	} else {
		s.pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE publish_status=$1 AND is_deleted_recyclebin=FALSE`, status).Scan(&total)
	}
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = s.pool.Query(ctx, `
			SELECT w.work_uuid::text, w.title, w.publish_status::text, COALESCE(w.view_count,0), COALESCE(w.like_count,0),
			       COALESCE(u.nickname,''), COALESCE(w.tags::text,'[]'), COALESCE(w.favorite_count,0), COALESCE(w.duration_sec,0),
			       to_char(COALESCE(w.published_at, w.created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI'),
			       COALESCE(w.media_url,''), COALESCE(w.is_recommended,FALSE)
			FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
			WHERE w.is_deleted_recyclebin=FALSE ORDER BY w.id DESC LIMIT $1 OFFSET $2`,
			pageSize, (page-1)*pageSize)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT w.work_uuid::text, w.title, w.publish_status::text, COALESCE(w.view_count,0), COALESCE(w.like_count,0),
			       COALESCE(u.nickname,''), COALESCE(w.tags::text,'[]'), COALESCE(w.favorite_count,0), COALESCE(w.duration_sec,0),
			       to_char(COALESCE(w.published_at, w.created_at) AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI'),
			       COALESCE(w.media_url,''), COALESCE(w.is_recommended,FALSE)
			FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
			WHERE w.publish_status=$1 AND w.is_deleted_recyclebin=FALSE ORDER BY w.id DESC LIMIT $2 OFFSET $3`,
			status, pageSize, (page-1)*pageSize)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, title, st, author, tags, publishedAt, mediaURL string
		var views, likes, favorites, duration int
		var recommended bool
		rows.Scan(&uuid, &title, &st, &views, &likes, &author, &tags, &favorites, &duration, &publishedAt, &mediaURL, &recommended)
		out = append(out, map[string]any{
			"work_id": uuid, "title": title, "publish_status": st, "view_count": views, "like_count": likes,
			"author": author, "tags": tags, "favorite_count": favorites, "duration_sec": duration, "published_at": publishedAt,
			"media_url": mediaURL, "is_recommended": recommended,
		})
	}
	return out, total, rows.Err()
}

func (s *AdminStore) DeleteWork(ctx context.Context, workUUID string, operatorID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE works SET is_deleted_recyclebin=TRUE, publish_status='REVOKED'
		WHERE work_uuid=$1::uuid AND is_deleted_recyclebin=FALSE`, workUUID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("内容不存在")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO publish_records (work_id, action, operator_user_id, note, snapshot_after_json)
		VALUES ((SELECT id FROM works WHERE work_uuid=$1::uuid), 'REVOKE', $2, '管理员强制删除', jsonb_build_object('status','REVOKED'))`,
		workUUID, operatorID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *AdminStore) ListReports(ctx context.Context, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM reports WHERE status='PENDING'`).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, w.work_uuid::text, COALESCE(w.title,''), r.reason, r.status::text,
		       to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM reports r LEFT JOIN works w ON w.id=r.work_id
		WHERE r.status='PENDING' ORDER BY r.id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var uuid, title, reason, status, created string
		rows.Scan(&id, &uuid, &title, &reason, &status, &created)
		out = append(out, map[string]any{
			"report_id": id, "work_id": uuid, "title": title, "reason": reason, "status": status, "created_at": created,
		})
	}
	return out, total, rows.Err()
}
// ListOrders 订单列表（P2：11.7 订单管理）。
func (s *AdminStore) ListOrders(ctx context.Context, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM orders`).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT order_no, user_id, order_type::text, COALESCE(plan_code,''), paid_amount_yuan::text, pay_status::text,
		       to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM orders ORDER BY id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var no, otype, plan, amount, status, created string
		var uid int64
		rows.Scan(&no, &uid, &otype, &plan, &amount, &status, &created)
		out = append(out, map[string]any{
			"order_no": no, "user_id": uid, "order_type": otype, "plan_code": plan,
			"amount_yuan": amount, "pay_status": status, "created_at": created,
		})
	}
	return out, total, rows.Err()
}

// ListPackages 套餐/积分包列表（P2：11.8）。
func (s *AdminStore) ListPackages(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT package_code, name, COALESCE(billing_cycle,''), price_yuan::text, points_quota, COALESCE(status::text,'')
		FROM packages ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var code, name, cycle, price, status string
		var points int
		rows.Scan(&code, &name, &cycle, &price, &points, &status)
		out = append(out, map[string]any{
			"package_code": code, "name": name, "billing_cycle": cycle, "price_yuan": price,
			"points_quota": points, "status": status,
		})
	}
	return out, rows.Err()
}

// ListModels 模型列表（P2：11.9）。
func (s *AdminStore) ListModels(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.model_key, m.name, m.capability::text, COALESCE(p.name,''), COALESCE(m.cost_points_per_call,0),
		       COALESCE(m.is_default_for_capability,FALSE), m.enabled
		FROM models m LEFT JOIN model_providers p ON p.id=m.provider_id ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var key, name, cap, provider string
		var cost int
		var def, enabled bool
		rows.Scan(&key, &name, &cap, &provider, &cost, &def, &enabled)
		out = append(out, map[string]any{
			"model_key": key, "name": name, "capability": cap, "provider_name": provider,
			"cost_points": cost, "is_default": def, "enabled": enabled,
		})
	}
	return out, rows.Err()
}

// ListAuditLogs 审计日志列表（P2：11.4）。
func (s *AdminStore) ListAuditLogs(ctx context.Context, page, pageSize int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	var total int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&total)
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(operator_role,''), action, COALESCE(target_type,''), COALESCE(target_id,0), detail_json::text,
		       to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM audit_logs ORDER BY id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var role, action, ttype, detail, created string
		var tid int64
		rows.Scan(&role, &action, &ttype, &tid, &detail, &created)
		out = append(out, map[string]any{
			"operator_role": role, "action": action, "target_type": ttype, "target_id": tid,
			"detail": detail, "created_at": created,
		})
	}
	return out, total, rows.Err()
}
// ListRoles 角色列表（P2）。
func (s *AdminStore) ListRoles(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT code::text, name, COALESCE(description,''), permissions::text FROM roles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var code, name, desc, perms string
		rows.Scan(&code, &name, &desc, &perms)
		out = append(out, map[string]any{"role_code": code, "name": name, "description": desc, "permissions": perms})
	}
	return out, rows.Err()
}

// ListPlatformConfigs 平台配置列表（P2）。
func (s *AdminStore) ListPlatformConfigs(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT config_key, config_value::text, config_group, COALESCE(description,''), is_public FROM platform_configs ORDER BY config_group, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var key, val, group, desc string
		var pub bool
		rows.Scan(&key, &val, &group, &desc, &pub)
		out = append(out, map[string]any{"config_key": key, "config_value": val, "config_group": group, "description": desc, "is_public": pub})
	}
	return out, rows.Err()
}

// GetPreferences 个人偏好（/me/preferences）。
func (s *AdminStore) GetPreferences(ctx context.Context, userID int64) (map[string]any, error) {
	var theme, lang, tz string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(theme,''), COALESCE(language,''), COALESCE(timezone,'') FROM user_preferences WHERE user_id=$1`, userID).Scan(&theme, &lang, &tz)
	if err != nil {
		return map[string]any{"theme": "dark", "language": "zh-CN", "timezone": "Asia/Shanghai"}, nil
	}
	return map[string]any{"theme": theme, "language": lang, "timezone": tz}, nil
}
// UpdatePlatformConfig 更新平台配置（upsert）。
func (s *AdminStore) UpdatePlatformConfig(ctx context.Context, key, value, group, desc string, operatorID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO platform_configs (config_key, config_value, config_group, value_type, description, updated_by_user_id)
		VALUES ($1, $2::jsonb, $3, 'string', $4, $5)
		ON CONFLICT (config_key) DO UPDATE SET config_value=$2::jsonb, config_group=$3, description=$4, updated_by_user_id=$5, updated_at=NOW()`,
		key, value, group, desc, operatorID)
	return err
}

// UpdatePackage 更新套餐（upsert，对齐 packages 真实列）。
func (s *AdminStore) UpdatePackage(ctx context.Context, code, name string, price string, points int, status string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO packages (package_code, name, price_yuan, points_quota, status)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (package_code) DO UPDATE SET name=$2, price_yuan=$3, points_quota=$4, status=$5, updated_at=NOW()`,
		code, name, price, points, status)
	return err
}

// UpdateModel 更新模型费率/启用（upsert）。
func (s *AdminStore) UpdateModel(ctx context.Context, key, name, capability string, cost int, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE models SET name=$2, capability=$3::text::model_capability_enum, cost_points_per_call=$4, enabled=$5, updated_at=NOW()
		WHERE model_key=$1 AND deleted_at IS NULL`,
		key, name, capability, cost, enabled)
	return err
}

// UpdatePreferences 更新个人偏好（upsert）。
func (s *AdminStore) UpdatePreferences(ctx context.Context, userID int64, theme, lang, tz string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_preferences (user_id, theme, language, timezone)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) WHERE (deleted_at IS NULL) DO UPDATE SET theme=$2, language=$3, timezone=$4, updated_at=NOW()`,
		userID, theme, lang, tz)
	return err
}
// ListAnnouncements 公告列表。
func (s *AdminStore) ListAnnouncements(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, content, start_at::text, end_at::text FROM announcements ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var title, content, start, end string
		rows.Scan(&id, &title, &content, &start, &end)
		out = append(out, map[string]any{"id": id, "title": title, "content": content, "start_at": start, "end_at": end})
	}
	return out, rows.Err()
}

// ListPublicAnnouncements 公开公告（生效时间窗内，首页公告条）。
func (s *AdminStore) ListPublicAnnouncements(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT title, content FROM announcements WHERE start_at<=NOW() AND end_at>=NOW() ORDER BY sort_order LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var title, content string
		if err := rows.Scan(&title, &content); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"title": title, "content": content})
	}
	return out, rows.Err()
}

// ListPublicBanners 公开轮播（启用中，首页精选推荐）。
func (s *AdminStore) ListPublicBanners(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT title, image_url, jump_url FROM banners WHERE is_active=TRUE ORDER BY sort_order LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var title, img, jump string
		if err := rows.Scan(&title, &img, &jump); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"title": title, "image_url": img, "jump_url": jump})
	}
	return out, rows.Err()
}

// ListBanners banner 列表（P2 运营模块）。
func (s *AdminStore) ListBanners(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, image_url, jump_url, position, sort_order FROM banners WHERE is_active=TRUE ORDER BY sort_order LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var title, img, jump, pos string
		var order int
		rows.Scan(&id, &title, &img, &jump, &pos, &order)
		out = append(out, map[string]any{
			"banner_id": id, "title": title, "image_url": img, "jump_url": jump, "position": pos, "sort_order": order,
		})
	}
	return out, rows.Err()
}

// ListHints 提示列表（P2 运营模块）。
func (s *AdminStore) ListHints(ctx context.Context, category string, step int) ([]map[string]any, error) {
	if category == "" && step == 0 {
		rows, err := s.pool.Query(ctx, `SELECT id, category, title, description FROM hints ORDER BY step, position LIMIT 50`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]map[string]any, 0)
		for rows.Next() {
			var id int
			var cat, title, desc string
			rows.Scan(&id, &cat, &title, &desc)
			out = append(out, map[string]any{"hint_id": id, "category": cat, "title": title, "description": desc})
		}
		return out, rows.Err()
	}
	rows, err := s.pool.Query(ctx, `SELECT id, title, description FROM hints WHERE category=$1 AND step=$2 ORDER BY position`, category, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int
		var title, desc string
		rows.Scan(&id, &title, &desc)
		out = append(out, map[string]any{"hint_id": id, "title": title, "description": desc})
	}
	return out, rows.Err()
}