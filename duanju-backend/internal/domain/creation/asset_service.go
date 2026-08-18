// 资产服务：预签名上传/回执 + 资产列表/批量生成（S1-B06：3.2.2/3.2.19/3.4.4/3.4.5）。
package creation

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/duanju/backend/internal/engine/asset"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// AssetOps 资产生成与查询边界（由 repository.AssetStore 实现）。
type AssetOps interface {
	// ListCharacters 项目角色列表（3.2.2）。
	ListCharacters(ctx context.Context, projectID int64) ([]map[string]any, error)
	// CreateAssetTask 创建资产生成任务（QUEUED + Outbox 同一事务）。
	CreateAssetTask(ctx context.Context, spaceID, userID, projectID, episodeID int64, kind string, targetID int64, cost int) (int64, error)
	// IsAssetUsedBefore 跨集复用判定：同项目该资产已生成过 → 本次豁免（S2-B06）。
	IsAssetUsedBefore(ctx context.Context, projectID int64, kind string, targetID int64) (bool, error)
	// ListVariants 角色变体列表（3.2.7）。
	ListVariants(ctx context.Context, characterID int64) ([]map[string]any, error)
	// CreateVariant 创建变体（3.2.6）：PENDING 行 + 衍生任务。
	CreateVariant(ctx context.Context, projectID, characterID, userID int64, variantName, variantType string) (int64, error)
	// SaveVersionSnapshot 拆分成功时写版本快照（S2-B05）。
	SaveVersionSnapshot(ctx context.Context, narrativeUnitID, userID int64, script, storyboard string) error
	// ListVersions 版本历史。
	ListVersions(ctx context.Context, narrativeUnitID int64) ([]map[string]any, error)
	// RollbackVersion 回滚到指定版本。
	RollbackVersion(ctx context.Context, narrativeUnitID, versionID int64) error
}

// AssetService 资产相关服务。
type AssetService struct {
	repo    *Repo
	asset   *asset.Client
	ops     AssetOps
	billing *billing.Engine // 计费引擎（批量生成冻结；nil 跳过）
}

func NewAssetService(repo *Repo, ac *asset.Client, ops AssetOps, b *billing.Engine) *AssetService {
	return &AssetService{repo: repo, asset: ac, ops: ops, billing: b}
}

// UploadPresignReq 预签名请求（05 ReqUploadPresign）。
type UploadPresignReq struct {
	BizType     string `json:"biz_type" binding:"required,oneof=script_file reference_image asset_upload work_cover export_rebuild"`
	BizRef      string `json:"biz_ref" binding:"omitempty"`
	FileName    string `json:"file_name" binding:"required,max=256"`
	ContentType string `json:"content_type" binding:"required"`
	Size        int64  `json:"size" binding:"required,min=1,max=1073741824"` // ≤1GB
}

// UploadCompleteReq 回执请求（05 ReqUploadComplete）。
type UploadCompleteReq struct {
	UploadToken    string `json:"upload_token" binding:"required"`
	ObjectKey      string `json:"object_key" binding:"required"`
	ETag           string `json:"etag" binding:"required"`
	ChecksumSHA256 string `json:"checksum_sha256" binding:"omitempty"`
	BizRefID       string `json:"biz_ref_id" binding:"required"`
}

// BatchGenerateReq 批量生成（3.2.19 子集）：items 为 {kind, target_id}。
type BatchGenerateReq struct {
	Items []struct {
		Kind     string `json:"kind" binding:"required,oneof=CHARACTER_VARIANT SCENE PROP"`
		TargetID int64  `json:"target_id" binding:"required"`
	} `json:"items" binding:"required,min=1,max=20"`
	EpisodeID string `json:"episode_id" binding:"required"` // episode_uuid
}

// Presign 预签名上传（3.4.4）。
func (s *AssetService) Presign(c *gin.Context, req *UploadPresignReq) {
	uid := userID(c)
	spaceID := c.GetInt64(middleware.CtxSpaceID)
	if spaceID == 0 {
		id, _, err := s.repo.DefaultSpaceID(c.Request.Context(), uid)
		if err != nil {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "未找到可用空间")
			return
		}
		spaceID = id
	}
	res, err := s.asset.PresignUpload(c.Request.Context(), spaceID, uid, req.BizType, req.FileName, req.ContentType, req.Size)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "生成上传凭证失败")
		return
	}
	httpx.OK(c, res)
}

// Complete 上传完成回执（3.4.5）：HeadObject 校验 + 凭证确认。
func (s *AssetService) Complete(c *gin.Context, req *UploadCompleteReq) {
	uid := userID(c)
	token, err := uuid.Parse(req.UploadToken)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "upload_token 非法")
		return
	}
	if err := s.asset.CompleteUpload(c.Request.Context(), token, req.ObjectKey, req.ETag, uid); err != nil {
		if err == asset.ErrTokenExpired {
			httpx.Fail(c, http.StatusGone, 90005, "上传凭证已过期或不存在")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, 90005, "对象校验失败："+err.Error())
		return
	}
	httpx.OK(c, nil)
}

// ListVariants 变体列表（3.2.7）。
func (s *AssetService) ListVariants(c *gin.Context, characterUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	charID, projectID, err := s.repo.ResolveCharacter(c.Request.Context(), characterUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "角色不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	items, err := s.ops.ListVariants(c.Request.Context(), charID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取变体失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// CreateVariant 创建变体（3.2.6）：插入 PENDING + 衍生生成任务。
func (s *AssetService) CreateVariant(c *gin.Context, characterUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	charID, projectID, err := s.repo.ResolveCharacter(c.Request.Context(), characterUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "角色不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	var req struct {
		VariantName string `json:"variant_name" binding:"required,min=1,max=64"`
		VariantType string `json:"variant_type" binding:"omitempty,oneof=OUTFIT_ONE OUTFIT_TWO HAIR_ONE"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.VariantType == "" {
		req.VariantType = "OUTFIT_ONE"
	}
	vid, err := s.ops.CreateVariant(c.Request.Context(), projectID, charID, uid, req.VariantName, req.VariantType)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建变体失败")
		return
	}
	httpx.OK(c, gin.H{"variant_id": vid, "status": "PENDING"})
}

// ListVersions 版本历史（S2-B05）。
func (s *AssetService) ListVersions(c *gin.Context, nuUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	nuID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), nuUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	items, err := s.ops.ListVersions(c.Request.Context(), nuID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取版本历史失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// RollbackVersion 回滚（S2-B05）：恢复脚本快照 + 状态机回退可重新规划。
func (s *AssetService) RollbackVersion(c *gin.Context, nuUUID, versionUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	nuID, projectID, err := s.repo.ResolveNarrativeUnit(c.Request.Context(), nuUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "叙事单元不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	versionID, err := s.repo.ResolveVersionID(c.Request.Context(), versionUUID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "版本不存在")
		return
	}
	if err := s.ops.RollbackVersion(c.Request.Context(), nuID, versionID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "回滚失败")
		return
	}
	httpx.OK(c, nil)
}

// ListCharacters 角色列表（3.2.2）。
func (s *AssetService) ListCharacters(c *gin.Context, projectUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	p, err := s.repo.GetProject(c.Request.Context(), projectUUID, uid)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	items, err := s.ops.ListCharacters(c.Request.Context(), p.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取角色列表失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// PreviewPoints 9.3.1 生成消耗预览（估算 + 豁免 + 余额校验）。
func (s *AssetService) PreviewPoints(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
		EpisodeID string `json:"episode_id"`
		Items     []struct {
			Kind     string `json:"kind" binding:"required,oneof=CHARACTER_VARIANT SCENE PROP"`
			TargetID int64  `json:"target_id" binding:"required"`
		} `json:"items" binding:"required,min=1,max=20"`
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
	// 豁免判定（S2-B06）：asset_usage_logs 同项目已生成过 → 免扣费
	totalCost, exemptCount, exemptAmount := 0, 0, 0
	for _, item := range req.Items {
		cost := defaultCost(item.Kind)
		exempt, _ := s.ops.IsAssetUsedBefore(c.Request.Context(), proj.ID, item.Kind, item.TargetID)
		if exempt {
			exemptCount++
			exemptAmount += cost // 按实际 kind 单价计算豁免金额（修复：不再用 CHARACTER_VARIANT 单价近似）
		} else {
			totalCost += cost
		}
	}
	httpx.OK(c, gin.H{
		"total_items":   len(req.Items),
		"total_cost":    totalCost,
		"exempt_count":  exemptCount,
		"exempt_amount": exemptAmount, // 公示豁免金额（按实际 kind 单价）
		"actual_cost":   totalCost,
		"billing_scope": "USER", // TEAM 空间时为 TEAM_SPACE（B03 已支持 SPACE 账户）
	})
}

// defaultCost 单资产默认积分成本（03 E.6.3 默认值）。
func defaultCost(kind string) int {
	switch kind {
	case "CHARACTER_VARIANT":
		return 10
	case "SCENE":
		return 8
	case "PROP":
		return 5
	}
	return 10
}

// BatchGenerate 批量生成资产图（3.2.19）：逐条建任务入队（跨集复用豁免接入 S2-B06）。
func (s *AssetService) BatchGenerate(c *gin.Context, req *BatchGenerateReq) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	epID, projectID, err := s.repo.ResolveEpisode(c.Request.Context(), req.EpisodeID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在")
		return
	}
	if err := s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "项目不存在或无权限")
		return
	}
	spaceID, err := s.repo.SpaceIDByProject(c.Request.Context(), projectID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "查询空间失败")
		return
	}
	created := 0
	frozen := 0
	for _, item := range req.Items {
		// 跨集复用豁免：同项目已用过的资产本次 cost=0（03 E.6.4 不重复扣费）
		cost := defaultCost(item.Kind)
		if reused, _ := s.ops.IsAssetUsedBefore(c.Request.Context(), projectID, item.Kind, item.TargetID); reused {
			cost = 0
		}
		taskID, err := s.ops.CreateAssetTask(c.Request.Context(), spaceID, uid, projectID, epID,
			item.Kind, item.TargetID, cost)
		if err != nil {
			fmt.Printf("[asset] CreateAssetTask kind=%s target=%d: %v\n", item.Kind, item.TargetID, err)
			continue
		}
		// 计费冻结（cost>0 时；不足则任务标记失败）
		if cost > 0 && s.billing != nil {
			if ferr := s.billing.Freeze(c.Request.Context(), "USER", uid, taskID, cost, &billing.TxnMeta{
				SpaceID: spaceID, OperatorUserID: uid, ProjectID: &projectID, Description: "资产生成",
			}); ferr != nil {
				_ = s.repo.MarkTaskFailed(c.Request.Context(), taskID, "积分不足或冻结失败："+ferr.Error())
				continue
			}
			frozen++
		}
		created++
	}
	if created == 0 {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "批量生成任务创建失败")
		return
	}
	if s.billing != nil && frozen == 0 && created > 0 {
		// 全部因积分不足失败
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "积分不足，无法生成资产")
		return
	}
	httpx.OK(c, gin.H{"created": created, "total": len(req.Items)})
}
