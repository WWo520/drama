// 资产详情弹窗对齐 LuxReal：提示词编辑/重置/AI优化、生成历史、收藏、换图、弹窗初始数据。
package creation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// kind → 资产表映射（CHARACTER_VARIANT 目标为 characters 行，首次生成时 variant 尚不存在）。
func dialogTable(kind string) string {
	switch kind {
	case "CHARACTER_VARIANT":
		return "characters"
	case "SCENE":
		return "scenes"
	case "PROP":
		return "props"
	}
	return ""
}

// resolveDialogTarget 解析弹窗目标 → projectID（权限校验用）。
func (s *AssetDetailService) resolveDialogTarget(ctx context.Context, kind, targetIDStr string) (int64, int64, error) {
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	var projectID int64
	switch kind {
	case "CHARACTER_VARIANT":
		_, projectID, err = s.repo.ResolveCharacter(ctx, targetIDStr)
	case "SCENE":
		_, projectID, err = s.repo.ResolveScene(ctx, targetIDStr)
	case "PROP":
		_, projectID, err = s.repo.ResolveProp(ctx, targetIDStr)
	default:
		err = errors.New("unknown asset kind: " + kind)
	}
	return targetID, projectID, err
}

// DialogData GET /assets/:kind/:target_id/dialog：弹窗初始数据（名称/默认提示词/编辑提示词/主图/变体/单价）。
func (s *AssetDetailService) DialogData(c *gin.Context, kind, targetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	targetID, projectID, err := s.resolveDialogTarget(c.Request.Context(), kind, targetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	table := dialogTable(kind)
	var name, defaultPrompt, editedPrompt, mainImg, status string
	var pointCost int
	var nameCol, promptCol string
	switch kind {
	case "CHARACTER_VARIANT":
		nameCol, promptCol = "name", "description_notes"
	case "SCENE":
		nameCol, promptCol = "base_name", "base_prompt"
	default:
		nameCol, promptCol = "name", "description_prompt"
	}
	// 角色主图/状态在 character_variants 表（characters 无 main_image_url 列），场景/道具在自身表
	var q string
	if kind == "CHARACTER_VARIANT" {
		q = `SELECT COALESCE(c.name,''), COALESCE(c.description_notes,''), COALESCE(c.edited_prompt,''),
		       COALESCE((SELECT cv.main_image_url FROM character_variants cv WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1), ''),
		       COALESCE((SELECT cv.generation_status::text FROM character_variants cv WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1), 'PENDING'),
		       COALESCE((SELECT cv.point_cost FROM character_variants cv WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1), 0)
		   FROM characters c WHERE c.id=$1 AND c.deleted_at IS NULL`
	} else {
		q = "SELECT COALESCE(" + nameCol + ",''), COALESCE(" + promptCol + ",''), COALESCE(edited_prompt,''), COALESCE(main_image_url,''), COALESCE(generation_status::text,''), COALESCE(point_cost,0) FROM " + table + " WHERE id=$1 AND deleted_at IS NULL"
	}
	if err := s.repo.pool.QueryRow(c.Request.Context(), q, targetID).
		Scan(&name, &defaultPrompt, &editedPrompt, &mainImg, &status, &pointCost); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	resp := gin.H{
		"kind": kind, "target_id": targetID, "name": name,
		"default_prompt": defaultPrompt, "edited_prompt": editedPrompt,
		"main_image_url": mainImg, "generation_status": status, "point_cost": pointCost,
	}
	// 角色：变体 tab 列表
	if kind == "CHARACTER_VARIANT" {
		variants, verr := s.repo.pool.Query(c.Request.Context(), `
			SELECT id, COALESCE(variant_name,''), COALESCE(main_image_url,''), COALESCE(generation_status::text,''),
			       CASE WHEN jsonb_typeof(multi_view_images_json)='array' THEN multi_view_images_json::text ELSE '[]' END
			FROM character_variants WHERE character_id=$1 AND deleted_at IS NULL ORDER BY id`, targetID)
		if verr == nil {
			defer variants.Close()
			items := make([]gin.H, 0)
			for variants.Next() {
				var vid int64
				var vname, vimg, vst, vviews string
				_ = variants.Scan(&vid, &vname, &vimg, &vst, &vviews)
				items = append(items, gin.H{
					"id": vid, "name": vname, "main_image_url": vimg,
					"generation_status": vst, "multi_view_images": json.RawMessage(vviews),
				})
			}
			resp["variants"] = items
		}
	}
	httpx.OK(c, resp)
}

// UpdatePrompt PUT /assets/:kind/:target_id/prompt：保存弹窗编辑的提示词。
func (s *AssetDetailService) UpdatePrompt(c *gin.Context, kind, targetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	targetID, projectID, err := s.resolveDialogTarget(c.Request.Context(), kind, targetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(),
		"UPDATE "+dialogTable(kind)+" SET edited_prompt=$1, updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL",
		req.Prompt, targetID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存提示词失败")
		return
	}
	httpx.OK(c, nil)
}

// ResetPrompt POST /assets/:kind/:target_id/prompt/reset：恢复默认提示词（清空 edited_prompt）。
func (s *AssetDetailService) ResetPrompt(c *gin.Context, kind, targetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	targetID, projectID, err := s.resolveDialogTarget(c.Request.Context(), kind, targetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	if _, err := s.repo.pool.Exec(c.Request.Context(),
		"UPDATE "+dialogTable(kind)+" SET edited_prompt=NULL, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL",
		targetID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "恢复默认提示词失败")
		return
	}
	httpx.OK(c, nil)
}

// OptimizePrompt POST /assets/:kind/:target_id/prompt/optimize：AI 优化提示词（LLM 重写精简）。
func (s *AssetDetailService) OptimizePrompt(c *gin.Context, kind, targetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	targetID, projectID, err := s.resolveDialogTarget(c.Request.Context(), kind, targetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	var req struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：prompt 必填")
		return
	}
	// llm 未注入（开发期无模型配置）→ 原样返回（前端 toast 提示）
	if s.llm == nil {
		httpx.OK(c, gin.H{"optimized_prompt": req.Prompt, "is_fallback": true})
		return
	}
	resp, cerr := s.llm.Chat(c.Request.Context(), model.CapLLM, &model.ChatRequest{
		Prompt: req.Prompt,
		System: `你是短剧资产提示词优化专家。请将以下描述重写为适合图像生成模型的中文提示词：

【必须保留的要素】
1. **角色**：身份背景、年龄范围、身材特征、面部特征、发型妆容
2. **服装**：材质（真丝/棉麻/皮革等）、颜色、款式细节、装饰元素、剪裁
3. **场景**：空间结构、主要物体、材质质感、色彩搭配、光照条件、氛围
4. **道具**：物品名称、类别、尺寸、材质成分、颜色、工艺细节、功能属性

【角色铁律】
- 角色描述只描述人物本体（身材/五官/发型/妆容/服装），严禁出现任何背景、场景、环境、房间、街道、室内外空间描述（如“办公室”“客厅”“街道”“天空”等），严禁出现任何环境物件（如桌子、窗户、沙发、电脑）。人物背景由构图阶段统一处理，这里完全不写。

【优化规则】
- 删除无意义的空泛形容词（如"非常"、"特别"、"漂亮"、"豪华"）
- 将抽象概念转化为具体视觉元素
- 使用列举式短语而非完整句子
- 每条信息用分号或句号分隔
- 确保总字数：角色≥80 字，场景≥100 字，道具≥40 字

【输入示例】
"年轻女生，穿红裙子"

【输出示例（角色）】
"中国现代女性，22-25 岁，高中生，身高 160cm，瘦小身材；黑色长直发，齐刘海，清纯淡妆；红色针织 A 字裙，中长款，腰部收腰设计；白色帆布鞋，学生双肩包；正面视角，中等景深，青春校园风格"

注意：输出纯文本，不包含 JSON、Markdown 格式或任何解释性文字。只返回优化后的提示词内容本身。`,
		MaxTokens:   2048,
		Temperature: 0.5,
		NoThinking:  true,
	})
	if cerr != nil || resp.Content == "" {
		httpx.OK(c, gin.H{"optimized_prompt": req.Prompt, "is_fallback": true})
		return
	}
	_ = targetID
	httpx.OK(c, gin.H{"optimized_prompt": resp.Content, "is_fallback": false})
}

// History GET /assets/:kind/:target_id/history：生成历史（?favorite=1 过滤收藏）。
func (s *AssetDetailService) History(c *gin.Context, kind, targetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	targetID, projectID, err := s.resolveDialogTarget(c.Request.Context(), kind, targetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	q := `SELECT id, original_url, COALESCE(prompt,''), COALESCE(model_code,''), COALESCE(style_name,''),
		         COALESCE(aspect_ratio,''), use_default_composition, is_favorite, created_at
		  FROM generated_images
		  WHERE target_kind=$1 AND target_id=$2 AND deleted_at IS NULL AND state='SUCCESS'`
	args := []any{kind, targetID}
	if c.Query("favorite") == "1" {
		q += " AND is_favorite=TRUE"
	}
	q += " ORDER BY id DESC LIMIT 100"
	rows, err := s.repo.pool.Query(c.Request.Context(), q, args...)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取生成历史失败")
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var url, prompt, mc, sn, ar string
		var udc, fav bool
		var createdAt time.Time
		if err := rows.Scan(&id, &url, &prompt, &mc, &sn, &ar, &udc, &fav, &createdAt); err != nil {
			continue
		}
		items = append(items, gin.H{
			"id": id, "url": url, "prompt": prompt, "model_code": mc, "style_name": sn,
			"aspect_ratio": ar, "use_default_composition": udc, "is_favorite": fav, "created_at": createdAt.Format(time.RFC3339),
		})
	}
	httpx.OK(c, gin.H{"items": items})
}

// ToggleFavorite POST /generated-images/:id/favorite：收藏/取消收藏。
func (s *AssetDetailService) ToggleFavorite(c *gin.Context, giIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	giID, err := strconv.ParseInt(giIDStr, 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误")
		return
	}
	var nowFav bool
	if err := s.repo.pool.QueryRow(c.Request.Context(), `
		UPDATE generated_images SET is_favorite = NOT is_favorite, updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL RETURNING is_favorite`, giID).Scan(&nowFav); err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "图片不存在")
		return
	}
	_ = uid
	httpx.OK(c, gin.H{"is_favorite": nowFav})
}

// SetImage POST /assets/:kind/:target_id/set-image：替换/导入主图（前端 presign 直传后回传 URL）。
func (s *AssetDetailService) SetImage(c *gin.Context, kind, targetIDStr string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	targetID, projectID, err := s.resolveDialogTarget(c.Request.Context(), kind, targetIDStr)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "资产不存在")
		return
	}
	var req struct {
		ImageURL string `json:"image_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误：image_url 必填")
		return
	}
	// 角色主图在 character_variants 表（无行则创建），场景/道具在自身表
	if kind == "CHARACTER_VARIANT" {
		// 对齐 LuxReal「历史图切换为主图」：只更新该角色默认形象（最小 id 变体）的主图，
		// 避免无差别更新全部变体导致多形象被同一张图覆盖。
		if _, err := s.repo.pool.Exec(c.Request.Context(), `
			UPDATE character_variants SET main_image_url=$1, generation_status='SUCCESS', updated_at=NOW()
			WHERE id = (SELECT cv.id FROM character_variants cv WHERE cv.character_id=$2 AND cv.deleted_at IS NULL ORDER BY cv.id LIMIT 1)`,
			req.ImageURL, targetID); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "替换主图失败")
			return
		}
		if _, err := s.repo.pool.Exec(c.Request.Context(), `
			INSERT INTO character_variants (character_id, variant_name, generation_status, main_image_url)
			SELECT $2, COALESCE((SELECT name FROM characters WHERE id=$2), '默认形象'), 'SUCCESS', $1
			WHERE NOT EXISTS (SELECT 1 FROM character_variants WHERE character_id=$2 AND deleted_at IS NULL)`,
			req.ImageURL, targetID); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "替换主图失败")
			return
		}
	} else {
		if _, err := s.repo.pool.Exec(c.Request.Context(),
			"UPDATE "+dialogTable(kind)+" SET main_image_url=$1, generation_status='SUCCESS', updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL",
			req.ImageURL, targetID); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "替换主图失败")
			return
		}
	}
	// 写入生成历史（导入来源标记 model_code=UPLOAD）
	_, _ = s.repo.pool.Exec(c.Request.Context(), `
		INSERT INTO generated_images (state, original_url, target_kind, target_id, model_code)
		VALUES ('SUCCESS', $1, $2, $3, 'UPLOAD')`, req.ImageURL, kind, targetID)
	httpx.OK(c, nil)
}
