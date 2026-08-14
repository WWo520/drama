// 内容域（S3-B01~B03）：广场信息流 + 作品页 + 互动（点赞/收藏/评论/关注/举报）+ 审核状态机。
package creation

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// ContentStore 内容域数据边界。
type ContentStore interface {
	// Feed 广场信息流（仅 PUBLISHED，03 E.5 只露已发布作品）。
	Feed(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
	// GetWork 作品详情（统计字段）。
	GetWork(ctx context.Context, workUUID string) (map[string]any, error)
	// SubmitWork 提交发布（创建 work + publish_records PENDING）。
	SubmitWork(ctx context.Context, spaceID, projectID, userID int64, title, desc, mediaURL string, tags []string) (string, error)
	// Review 审核：PENDING → PUBLISHED/REJECTED（写 publish_records）。
	Review(ctx context.Context, workUUID string, approve bool, note string, operatorID int64) error
	// Revoke 下架：PUBLISHED → REVOKED（举报后审核用）。
	Revoke(ctx context.Context, workUUID string, note string, operatorID int64) error
	// Like 点赞（幂等 UNIQUE）。
	Like(ctx context.Context, workUUID string, userID int64) error
	// UnLike 取消点赞。
	UnLike(ctx context.Context, workUUID string, userID int64) error
	// Favorite 收藏。
	Favorite(ctx context.Context, workUUID string, userID int64) error
	// Comment 评论。
	Comment(ctx context.Context, workUUID string, userID int64, content string) error
	// Report 举报（写入 PENDING 报告）。
	Report(ctx context.Context, workUUID string, userID int64, reason string) error
	// Follow 关注用户。
	Follow(ctx context.Context, followeeUUID string, followerID int64) error
	// Unfollow 取消关注。
	Unfollow(ctx context.Context, followeeUUID string, followerID int64) error
	// AuditList 待审列表（审核员用）。
	AuditList(ctx context.Context, page, pageSize int) ([]map[string]any, int, error)
}

// ContentService 内容服务。
type ContentService struct {
	store ContentStore
	repo  *Repo
}

func NewContentService(store ContentStore, repo *Repo) *ContentService {
	return &ContentService{store: store, repo: repo}
}

// Feed 7.1 广场信息流。
func (s *ContentService) Feed(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.Feed(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取信息流失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// Work 7.2 作品详情。
func (s *ContentService) Work(c *gin.Context, workUUID string) {
	w, err := s.store.GetWork(c.Request.Context(), workUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 20001, "作品不存在")
		return
	}
	httpx.OK(c, w)
}

// Submit 5.6 提交发布（进入审核）。
func (s *ContentService) Submit(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		ProjectID string   `json:"project_id" binding:"required"`
		Title     string   `json:"title" binding:"required,min=1,max=128"`
		Desc      string   `json:"description" binding:"omitempty,max=2000"`
		MediaURL  string   `json:"media_url" binding:"required"`
		Tags      []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	proj, err := s.repo.GetProject(c.Request.Context(), req.ProjectID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	// 关键词粗筛（S3-B03：命中敏感词直接 REJECTED）
	for _, kw := range bannedKeywords {
		if strings.Contains(req.Title, kw) || strings.Contains(req.Desc, kw) {
			httpx.Fail(c, http.StatusBadRequest, 20009, "内容包含敏感词，无法发布")
			return
		}
	}
	uuid, err := s.store.SubmitWork(c.Request.Context(), proj.SpaceID, proj.ID, uid, req.Title, req.Desc, req.MediaURL, req.Tags)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "提交失败")
		return
	}
	httpx.OK(c, gin.H{"work_id": uuid, "publish_status": "PENDING"})
}

// Review 11.16 审核通过/拒绝。
func (s *ContentService) Review(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := s.store.Review(c.Request.Context(), workUUID, req.Approve, req.Note, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Revoke 11.17 下架（举报处理）。
func (s *ContentService) Revoke(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Revoke(c.Request.Context(), workUUID, "举报成立，内容下架", uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// AuditList 11.15 待审列表。
func (s *ContentService) AuditList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := s.store.AuditList(c.Request.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取待审列表失败")
		return
	}
	httpx.OK(c, gin.H{"items": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total}})
}

// Like 7.4 点赞。
func (s *ContentService) Like(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Like(c.Request.Context(), workUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// UnLike 7.4b 取消点赞。
func (s *ContentService) UnLike(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.UnLike(c.Request.Context(), workUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Favorite 7.6 收藏。
func (s *ContentService) Favorite(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Favorite(c.Request.Context(), workUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Comment 7.7 评论。
func (s *ContentService) Comment(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Content string `json:"content" binding:"required,min=1,max=1000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.Comment(c.Request.Context(), workUUID, uid, req.Content); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Report 7.10 举报。
func (s *ContentService) Report(c *gin.Context, workUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Reason string `json:"reason" binding:"required,min=1,max=256"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if err := s.store.Report(c.Request.Context(), workUUID, uid, req.Reason); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Follow 关注用户（S3-B02 补齐）。
func (s *ContentService) Follow(c *gin.Context, userUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Follow(c.Request.Context(), userUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// Unfollow 取消关注。
func (s *ContentService) Unfollow(c *gin.Context, userUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.Unfollow(c.Request.Context(), userUUID, uid); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// 敏感词粗筛（S3-B03 粗筛）。
var bannedKeywords = []string{"违禁", "赌博", "诈骗"}