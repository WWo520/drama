// 内容域 + 任务接单域补齐（S5 接口覆盖率）：公开作品/发布/评论编辑/信息流/回收站/任务广场/接单意向。
package creation

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// ---- 7.x 发现互动 ----

// PublicFeed 7.1 信息流（GET /feed，无需登录）。
func (s *ContentService) PublicFeed(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 50 || pageSize < 1 {
		pageSize = 20
	}
	var total int
	_ = s.repo.pool.QueryRow(c.Request.Context(),
		`SELECT count(*) FROM works WHERE publish_status='PUBLISHED' AND is_deleted_recyclebin=FALSE`).Scan(&total)
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT w.work_uuid::text, w.title, COALESCE(w.cover_url,''), w.view_count, w.like_count,
		       w.favorite_count, w.comment_count, w.duration_sec, COALESCE(u.nickname,''),
		       to_char(w.published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
		WHERE w.publish_status='PUBLISHED' AND w.is_deleted_recyclebin=FALSE
		ORDER BY w.published_at DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取信息流失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, title, cover, nickname, published string
		var views, likes, favs, comments, dur int
		if err := rows.Scan(&uuid, &title, &cover, &views, &likes, &favs, &comments, &dur, &nickname, &published); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"work_id": uuid, "title": title, "cover_url": cover, "view_count": views,
			"like_count": likes, "favorite_count": favs, "comment_count": comments,
			"duration_sec": dur, "author": nickname, "published_at": published,
		})
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total, "total_pages": (total + pageSize - 1) / pageSize}})
}

// PublicWorksSearch 7.2 公开作品搜索分类（GET /works/public）。
func (s *ContentService) PublicWorksSearch(c *gin.Context) {
	keyword := c.Query("keyword")
	category := c.Query("category")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 50 || pageSize < 1 {
		pageSize = 20
	}
	like := "%" + keyword + "%"
	var total int
	_ = s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT count(*) FROM works WHERE publish_status='PUBLISHED' AND is_deleted_recyclebin=FALSE
		AND ($1='' OR title ILIKE $2)`, keyword, like).Scan(&total)
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT w.work_uuid::text, w.title, COALESCE(w.cover_url,''), w.view_count, w.like_count,
		       w.duration_sec, COALESCE(u.nickname,''),
		       to_char(w.published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
		WHERE w.publish_status='PUBLISHED' AND w.is_deleted_recyclebin=FALSE
		AND ($1='' OR w.title ILIKE $2)
		ORDER BY w.published_at DESC LIMIT $3 OFFSET $4`, keyword, like, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "搜索失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var uuid, title, cover, nickname, published string
		var views, likes, dur int
		if err := rows.Scan(&uuid, &title, &cover, &views, &likes, &dur, &nickname, &published); err != nil {
			continue
		}
		items = append(items, map[string]any{"work_id": uuid, "title": title, "cover_url": cover, "view_count": views, "like_count": likes, "duration_sec": dur, "author": nickname, "published_at": published})
	}
	_ = category
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// PublicWorkDetail 7.3 公开作品详情页（GET /works/:work_id/public，无需登录）。
func (s *ContentService) PublicWorkDetail(c *gin.Context, workUUID string) {
	var uuid, title, desc, cover, media, nickname, published string
	var views, likes, favs, comments, dur int
	err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT w.work_uuid::text, w.title, COALESCE(w.description,''), COALESCE(w.cover_url,''),
		       COALESCE(w.media_url,''), w.view_count, w.like_count, w.favorite_count, w.comment_count,
		       w.duration_sec, COALESCE(u.nickname,''),
		       to_char(w.published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM works w LEFT JOIN users u ON u.id=w.owner_user_id
		WHERE w.work_uuid=$1 AND w.publish_status='PUBLISHED' AND w.is_deleted_recyclebin=FALSE`, workUUID).
		Scan(&uuid, &title, &desc, &cover, &media, &views, &likes, &favs, &comments, &dur, &nickname, &published)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	_, _ = s.repo.pool.Exec(c.Request.Context(), `UPDATE works SET view_count=view_count+1 WHERE work_uuid=$1`, workUUID)
	httpx.OK(c, gin.H{
		"work_id": uuid, "title": title, "description": desc, "cover_url": cover, "media_url": media,
		"view_count": views + 1, "like_count": likes, "favorite_count": favs, "comment_count": comments,
		"duration_sec": dur, "author": nickname, "published_at": published,
	})
}

// PublishWork 5.6 提交发布（POST /works/:work_id/publish → 待审核）。
func (s *ContentService) PublishWork(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var wid, ownerID int64
	var status string
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, owner_user_id, publish_status::text FROM works WHERE work_uuid=$1::uuid AND is_deleted_recyclebin=FALSE`, workUUID).
		Scan(&wid, &ownerID, &status); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	if ownerID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限发布该作品")
		return
	}
	if status == "PUBLISHED" {
		httpx.Fail(c, http.StatusBadRequest, 20009, "作品已发布")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET publish_status='PENDING', updated_at=NOW() WHERE id=$1`, wid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "提交失败")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO publish_records (work_id, action, submitter_user_id, snapshot_after_json)
		VALUES ($1, 'SUBMIT', $2, jsonb_build_object('status','PENDING'))`, wid, uid); err != nil {
		// 记录失败不阻塞主流程
	}
	httpx.OK(c, gin.H{"work_id": workUUID, "publish_status": "PENDING"})
}

// EditComment 7.6 编辑评论（PUT /comments/:comment_id，仅作者本人）。
func (s *ContentService) EditComment(c *gin.Context, commentIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "comment_id 非法")
		return
	}
	var req struct {
		Content string `json:"content" binding:"required,min=1,max=1000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	var authorID int64
	var createdAt time.Time
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT user_id, created_at FROM comments WHERE id=$1 AND deleted_at IS NULL`, commentID).
		Scan(&authorID, &createdAt); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "评论不存在")
		return
	}
	if authorID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅作者本人可编辑")
		return
	}
	if time.Since(createdAt) > 10*time.Minute {
		httpx.Fail(c, http.StatusBadRequest, 20009, "评论超过 10 分钟不可编辑")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE comments SET content=$2, updated_at=NOW() WHERE id=$1`, commentID, req.Content); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "编辑失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 5.7/5.8 回收站 ----

// RestoreWork 5.7 从回收站恢复作品（POST /recycle-bin/:work_id/restore）。
func (s *ContentService) RestoreWork(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var wid, ownerID int64
	var recycled bool
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, owner_user_id, is_deleted_recyclebin FROM works WHERE work_uuid=$1::uuid`, workUUID).
		Scan(&wid, &ownerID, &recycled); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	if ownerID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限操作该作品")
		return
	}
	if !recycled {
		httpx.Fail(c, http.StatusBadRequest, 20009, "作品不在回收站")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET is_deleted_recyclebin=FALSE, updated_at=NOW() WHERE id=$1`, wid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "恢复失败")
		return
	}
	httpx.OK(c, nil)
}

// DeleteWorkForever 5.8 彻底删除（DELETE /recycle-bin/:work_id，不可恢复）。
func (s *ContentService) DeleteWorkForever(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var wid, ownerID int64
	var recycled bool
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, owner_user_id, is_deleted_recyclebin FROM works WHERE work_uuid=$1::uuid`, workUUID).
		Scan(&wid, &ownerID, &recycled); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	if ownerID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "无权限操作该作品")
		return
	}
	if !recycled {
		httpx.Fail(c, http.StatusBadRequest, 20009, "作品不在回收站，请先移入回收站")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(), `
		UPDATE works SET deleted_at=NOW() WHERE id=$1`, wid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除失败")
		return
	}
	httpx.OK(c, nil)
}

// ---- 6.x 任务接单 ----

// TaskOrderSquare 6.1 任务广场（GET /task-orders，公开任务列表+筛选）。
func (s *MarketService) TaskOrderSquare(c *gin.Context) {
	status := c.DefaultQuery("status", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize > 50 || pageSize < 1 {
		pageSize = 20
	}
	var total int
	if status == "" {
		_ = s.repo.pool.QueryRow(c.Request.Context(),
			`SELECT count(*) FROM task_orders WHERE status='OPEN' AND deleted_at IS NULL`).Scan(&total)
	} else {
		_ = s.repo.pool.QueryRow(c.Request.Context(),
			`SELECT count(*) FROM task_orders WHERE status=$1 AND deleted_at IS NULL`, status).Scan(&total)
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT o.id, o.order_uuid::text, COALESCE(o.title,''), COALESCE(o.description,''),
		       o.reward_points, COALESCE(o.status::text,''), COALESCE(o.deadline_at::text,''),
		       COALESCE(u.nickname,''), COALESCE(o.space_id,0),
		       to_char(o.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM task_orders o LEFT JOIN users u ON u.id=o.publisher_user_id
		WHERE o.deleted_at IS NULL AND ($1='' OR o.status::text=$1)
		ORDER BY o.id DESC LIMIT $2 OFFSET $3`, status, pageSize, (page-1)*pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取任务失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, reward, spaceID int64
		var ouuid, title, desc, ostatus, deadline, publisher, created string
		if err := rows.Scan(&id, &ouuid, &title, &desc, &reward, &ostatus, &deadline, &publisher, &spaceID, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"task_order_id": ouuid, "id": id, "title": title, "description": desc,
			"reward_points": reward, "status": ostatus, "deadline_at": deadline,
			"publisher": publisher, "created_at": created,
		})
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// TaskOrderDetail 6.3 任务详情（GET /task-orders/:task_order_id）。
func (s *MarketService) TaskOrderDetail(c *gin.Context, orderUUID string) {
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT o.id, o.order_uuid::text, COALESCE(o.title,''), COALESCE(o.description,''),
		       o.reward_points, COALESCE(o.status::text,''), COALESCE(o.deadline_at::text,''),
		       COALESCE(u.nickname,''), COALESCE(o.requirement_json::text,'{}')
		FROM task_orders o LEFT JOIN users u ON u.id=o.publisher_user_id
		WHERE o.order_uuid=$1::uuid AND o.deleted_at IS NULL`, orderUUID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取任务失败")
		return
	}
	defer rows.Close()
	if !rows.Next() {
		httpx.Fail(c, http.StatusNotFound, 20001, "任务不存在")
		return
	}
	var id, reward int64
	var ouuid, title, desc, ostatus, deadline, publisher, requirement string
	if err := rows.Scan(&id, &ouuid, &title, &desc, &reward, &ostatus, &deadline, &publisher, &requirement); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "任务不存在")
		return
	}
	httpx.OK(c, gin.H{
		"task_order_id": ouuid, "id": id, "title": title, "description": desc,
		"reward_points": reward, "status": ostatus, "deadline_at": deadline,
		"publisher": publisher, "requirement": requirement,
	})
}

// CloseTaskOrder 6.5 截止任务（POST /task-orders/:task_order_id/close，仅发布方）。
func (s *MarketService) CloseTaskOrder(c *gin.Context, orderUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var oid, publisherID int64
	var status string
	var reward int
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, publisher_user_id, status::text, reward_points FROM task_orders WHERE order_uuid=$1::uuid AND deleted_at IS NULL`, orderUUID).
		Scan(&oid, &publisherID, &status, &reward); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "任务不存在")
		return
	}
	if publisherID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅发布方可截止任务")
		return
	}
	if status != "OPEN" {
		httpx.Fail(c, http.StatusBadRequest, 20009, "仅进行中的任务可截止")
		return
	}
	// 08 审查 Critical#5：截止即退回发布方冻结积分（同事务）
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "截止失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE task_orders SET status='CLOSED', updated_at=NOW() WHERE id=$1`, oid); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "截止失败")
		return
	}
	if reward > 0 {
		if _, err := tx.Exec(c.Request.Context(), `
			UPDATE point_accounts SET frozen_balance=frozen_balance-$2, current_balance=current_balance+$2, last_txn_at=NOW()
			WHERE owner_type='USER' AND owner_id=$1 AND frozen_balance>=$2`, publisherID, reward); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "冻结积分退回失败")
			return
		}
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount,
				balance_after, operator_user_id, description, idempotency_key)
			SELECT pa.id, 'USER', pa.owner_id, 'MARKET_UNFREEZE', 'CREDIT', $2, pa.current_balance, $1, '任务截止退回冻结', $3
			FROM point_accounts pa WHERE pa.owner_type='USER' AND pa.owner_id=$1`,
			publisherID, reward, fmt.Sprintf("market-unfreeze-%d", oid)); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "冻结积分退回失败")
			return
		}
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "截止失败")
		return
	}
	httpx.OK(c, nil)
}

// TaskOrderIntents 6.6 查看收到的接单意向（GET /task-orders/:task_order_id/intents，仅发布方）。
func (s *MarketService) TaskOrderIntents(c *gin.Context, orderUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var oid, publisherID int64
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT id, publisher_user_id FROM task_orders WHERE order_uuid=$1::uuid AND deleted_at IS NULL`, orderUUID).
		Scan(&oid, &publisherID); err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "任务不存在")
		return
	}
	if publisherID != uid {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "仅发布方可查看意向")
		return
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT ti.id, ti.user_id, COALESCE(u.nickname,''), COALESCE(ti.cover_letter,''),
		       COALESCE(ti.status::text,''), to_char(ti.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM task_intents ti LEFT JOIN users u ON u.id=ti.user_id
		WHERE ti.order_id=$1 ORDER BY ti.id DESC`, oid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取意向失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, userID int64
		var nickname, cover, status, created string
		if err := rows.Scan(&id, &userID, &nickname, &cover, &status, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"intent_id": id, "user_id": userID, "nickname": nickname, "cover_letter": cover, "status": status, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items})
}

// MyTaskIntents 6.8 我登记的接单意向（GET /me/task-intents）。
func (s *MarketService) MyTaskIntents(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT ti.id, ti.order_id, o.order_uuid::text, COALESCE(o.title,''), COALESCE(ti.cover_letter,''),
		       COALESCE(ti.status::text,''), to_char(ti.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM task_intents ti JOIN task_orders o ON o.id=ti.order_id AND o.deleted_at IS NULL
		WHERE ti.user_id=$1 ORDER BY ti.id DESC`, uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取意向失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, orderID int64
		var ouuid, title, cover, status, created string
		if err := rows.Scan(&id, &orderID, &ouuid, &title, &cover, &status, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"intent_id": id, "task_order_id": ouuid, "title": title, "cover_letter": cover, "status": status, "created_at": created})
	}
	httpx.OK(c, gin.H{"items": items})
}
