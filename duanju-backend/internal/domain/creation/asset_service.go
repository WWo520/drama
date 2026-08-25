// 资产服务：预签名上传/回执 + 资产列表/批量生成（S1-B06：3.2.2/3.2.19/3.4.4/3.4.5）。
package creation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/duanju/backend/internal/engine/agent"
	"github.com/duanju/backend/internal/engine/asset"
	"github.com/duanju/backend/internal/engine/billing"
	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
)

// AssetOps 资产生成与查询边界（由 repository.AssetStore 实现）。
type AssetOps interface {
	// ListCharacters 项目角色列表（3.2.2）。
	ListCharacters(ctx context.Context, projectID int64) ([]map[string]any, error)
	// CreateAssetTask 创建资产生成任务（QUEUED + Outbox 同一事务）。
	CreateAssetTask(ctx context.Context, spaceID, userID, projectID, episodeID int64, kind string, targetID int64, cost int) (int64, error)
	// CreateAssetTaskWithGen 建资产生成任务（携带弹窗生成参数）。
	CreateAssetTaskWithGen(ctx context.Context, spaceID, userID, projectID, episodeID int64, kind string, targetID int64, cost int, gen *agent.GenParams) (int64, error)
	// IsAssetUsedBefore 跨集复用判定：同项目该资产已生成过 → 本次豁免（S2-B06）。
	IsAssetUsedBefore(ctx context.Context, projectID int64, kind string, targetID int64) (bool, error)
	// ListVariants 角色变体列表（3.2.7）。
	ListVariants(ctx context.Context, characterID int64) ([]map[string]any, error)
	// CreateVariant 创建变体（3.2.6）：PENDING 行 + 衍生任务。
	CreateVariant(ctx context.Context, projectID, characterID, userID int64, variantName, variantType, clothingDesc, extraAppearancePrompt string) (int64, error)
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
	llm     agent.LLMClient // LLM（从剧本自动生成形象；nil 降级为忽略）
}

func NewAssetService(repo *Repo, ac *asset.Client, ops AssetOps, b *billing.Engine, llm agent.LLMClient) *AssetService {
	return &AssetService{repo: repo, asset: ac, ops: ops, billing: b, llm: llm}
}

// UploadPresignReq 预签名请求（05 ReqUploadPresign）。
type UploadPresignReq struct {
	BizType     string `json:"biz_type" binding:"required,oneof=script_file reference_image asset_upload work_cover export_rebuild avatar"`
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

// GenParamsReq 弹窗生成参数（对齐 LuxReal 弹窗底部工具栏）。
type GenParamsReq struct {
	Prompt                string   `json:"prompt"`
	ModelCode             string   `json:"model_code"`
	StyleName             string   `json:"style_name"`
	AspectRatio           string   `json:"aspect_ratio"`
	Resolution            string   `json:"resolution"`
	UseDefaultComposition bool     `json:"use_default_composition"`
	RefImageURLs          []string `json:"ref_image_urls"`
	TimeOfDay             string   `json:"time_of_day"` // 场景时段（time_of_day_enum），场景日夜变体
	Weather               string   `json:"weather"`     // 天气（weather_enum）
}

func (g *GenParamsReq) toAgent() *agent.GenParams {
	if g == nil {
		return nil
	}
	return &agent.GenParams{
		Prompt: g.Prompt, ModelCode: g.ModelCode, StyleName: g.StyleName,
		AspectRatio: g.AspectRatio, Resolution: g.Resolution,
		UseDefaultComposition: g.UseDefaultComposition, RefImageURLs: g.RefImageURLs,
		TimeOfDay: g.TimeOfDay, Weather: g.Weather,
	}
}

// BatchGenerateReq 批量生成（3.2.19 子集）：items 为 {kind, target_id, gen?}。
type BatchGenerateReq struct {
	Items []struct {
		Kind     string        `json:"kind" binding:"required,oneof=CHARACTER_VARIANT SCENE PROP"`
		TargetID int64         `json:"target_id" binding:"required"`
		Gen      *GenParamsReq `json:"gen"`
	} `json:"items" binding:"required,min=1,max=20"`
	EpisodeID     string `json:"episode_id" binding:"required"` // episode_uuid
	ModelCode     string `json:"model_code"`                    // 批量弹窗选中的图片模型（透传到每个生成任务）
	PerAssetCount int    `json:"per_asset_count"`               // 每个资产生成的张数（≥1；默认 1）
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
		VariantName           string `json:"variant_name" binding:"required,min=1,max=128"`
		VariantType           string `json:"variant_type" binding:"omitempty,oneof=COSTUME HAIRSTYLE MAKEUP AGE_PROGRESSION INJURY_DIRTY SPECIAL_OUTFIT_UNIFORM CASUAL FORMAL"`
		ClothingDesc          string `json:"clothing_desc"`
		ExtraAppearancePrompt string `json:"extra_appearance_prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.VariantType == "" {
		req.VariantType = "COSTUME"
	}
	vid, err := s.ops.CreateVariant(c.Request.Context(), projectID, charID, uid, req.VariantName, req.VariantType, req.ClothingDesc, req.ExtraAppearancePrompt)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建变体失败")
		return
	}
	httpx.OK(c, gin.H{"variant_id": vid, "status": "PENDING"})
}

// GenerateVariantsFromScript POST /characters/:character_id/generate-variants：
// 需求2——根据剧本内容自动生成角色在不同场景/状态下的形象变体（对齐 LuxReal 剧本整合）。
// 流程：读角色设定 + 相关剧本片段 → LLM 提取形象列表 → 批量创建变体并触发生成。
func (s *AssetService) GenerateVariantsFromScript(c *gin.Context, characterUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	charID, projectID, err := s.repo.ResolveCharacter(c.Request.Context(), characterUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "角色不存在")
		return
	}
	if s.llm == nil {
		httpx.OK(c, gin.H{"created": 0, "message": "未配置模型，无法自动生成形象，请手动创建"})
		return
	}
	// 角色设定
	var charName, notes, appearance string
	_ = s.repo.pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(name,''), COALESCE(description_notes,''), COALESCE(appearance_base_prompt,'')
		FROM characters WHERE id=$1 AND deleted_at IS NULL`, charID).Scan(&charName, &notes, &appearance)
	// 相关剧本片段（该角色名字出现的叙事单元）
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT COALESCE(nu.title,''), COALESCE(nu.script_original_text,'')
		FROM narrative_units nu
		WHERE nu.project_id=$1 AND nu.deleted_at IS NULL
		  AND (COALESCE(nu.script_original_text,'') ILIKE '%'||$2||'%' OR COALESCE(nu.title,'') ILIKE '%'||$2||'%')
		ORDER BY nu.id ASC LIMIT 8`, projectID, charName)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "读取剧本失败")
		return
	}
	var sb strings.Builder
	seen := 0
	for rows.Next() {
		var title, text string
		if err := rows.Scan(&title, &text); err != nil {
			continue
		}
		if seen > 0 {
			sb.WriteString("\n---\n")
		}
		sb.WriteString("【" + title + "】\n" + text)
		seen++
	}
	rows.Close()
	if seen == 0 {
		sb.WriteString("（剧本中未检索到该角色的独立叙事片段，请依据角色设定设计）")
	}
	// LLM 提取形象列表
	userPrompt := fmt.Sprintf(`角色名称：%s
角色设定：%s
外观基线：%s
相关剧本片段：
%s

请根据以上剧本内容，提取该角色在【不同场景 / 不同状态】下应穿着的不同服装形象（如：白天市井装、夜间便装、战斗/受伤形态等）。为每个形象给出：
- variant_name：形象名称（简洁，如"干练老板娘装"）
- variant_type：类型，取值 COSTUME/HAIRSTYLE/MAKEUP/AGE_PROGRESSION/INJURY_DIRTY/SPECIAL_OUTFIT_UNIFORM/CASUAL/FORMAL
- clothing_desc：服装描述（材质/颜色/款式/配饰）
- extra_appearance_prompt：额外外观提示词（发型/妆容/神态等）

只输出 JSON 数组，不要任何解释文字或 Markdown，形如：
[{"variant_name":"...","variant_type":"COSTUME","clothing_desc":"...","extra_appearance_prompt":"..."}]`,
		charName, notes, appearance, sb.String())
	resp, cerr := s.llm.Chat(c.Request.Context(), model.CapLLM, &model.ChatRequest{
		Prompt:      userPrompt,
		System:      `你是短剧角色形象设计专家。你根据剧本原文为同一角色设计在不同场景下穿着的多个形象，保证人物面部/身材特征一致，仅变化服装与状态。请严格输出 JSON 数组。`,
		MaxTokens:   2048,
		Temperature: 0.6,
		NoThinking:  true,
	})
	if cerr != nil || resp == nil || resp.Content == "" {
		httpx.Fail(c, http.StatusBadGateway, httpx.CodeInternal, "AI 形象提取失败，请稍后重试")
		return
	}
	// 解析 JSON（容错：剥离 ```json 包裹）
	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var variants []struct {
		VariantName           string `json:"variant_name"`
		VariantType           string `json:"variant_type"`
		ClothingDesc          string `json:"clothing_desc"`
		ExtraAppearancePrompt string `json:"extra_appearance_prompt"`
	}
	if err := json.Unmarshal([]byte(content), &variants); err != nil {
		httpx.OK(c, gin.H{"created": 0, "message": "AI 返回格式异常，未创建形象", "raw": content})
		return
	}
	if len(variants) == 0 {
		httpx.OK(c, gin.H{"created": 0, "message": "未提取到可用形象"})
		return
	}
	// 批量创建变体 + 触发生成（复用 CreateVariant 链路）
	created := 0
	createdItems := make([]gin.H, 0, len(variants))
	for _, v := range variants {
		vtype := v.VariantType
		if vtype == "" {
			vtype = "COSTUME"
		}
		vid, verr := s.ops.CreateVariant(c.Request.Context(), projectID, charID, uid, v.VariantName, vtype, v.ClothingDesc, v.ExtraAppearancePrompt)
		if verr != nil {
			continue
		}
		created++
		createdItems = append(createdItems, gin.H{"variant_id": vid, "name": v.VariantName, "status": "PENDING"})
	}
	httpx.OK(c, gin.H{"created": created, "variants": createdItems})
}

// reextractPropCategories prop_category_enum 合法集（与 split_store 一致，避免枚举插入报错）。
var reextractPropCategories = map[string]bool{
	"WEAPON": true, "CLOTHING_ACCESSORY": true, "FOOD_DRINK": true, "STATIONERY_BOOK": true,
	"JEWELRY_ORNAMENT": true, "FURNITURE_DECOR": true, "ELECTRONIC_APPLIANCE": true, "VEHICLE": true,
	"TOOL_MUSICAL_INSTRUMENT": true, "TOY": true, "PLANT_FLOWER": true, "ANIMAL_PET": true,
	"LIGHTING_SOURCE": true, "DOCUMENT_SCROLL_MAP": true, "FLAG_BANNER": true, "OTHER": true,
}

func reextractSanitizeCategory(cat string) string {
	if cat != "" && reextractPropCategories[cat] {
		return cat
	}
	return "OTHER"
}

// ReextractEpisodeAssets POST /episodes/:episode_id/reextract-assets：
// 未对齐点2——剧本修改后一键重跑本集资产提取（对齐 LuxReal「重新提取本集资产」，不必重走全流程）。
// 流程：读本集剧本原文 → LLM 提取角色/场景/道具 → 事务软删旧资产重建新资产。
func (s *AssetService) ReextractEpisodeAssets(c *gin.Context, epUUID string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	epID, projectID, err := s.repo.ResolveEpisode(c.Request.Context(), epUUID)
	if err != nil || s.repo.CheckProjectAccess(c.Request.Context(), projectID, uid) != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeProjectNotFound, "剧集不存在")
		return
	}
	if s.llm == nil {
		httpx.OK(c, gin.H{"characters": 0, "scenes": 0, "props": 0, "message": "未配置模型，无法重新提取资产"})
		return
	}
	// 读本集全部叙事单元原文（保持顺序，作为 LLM 输入）
	rows, err := s.repo.pool.Query(c.Request.Context(), `
		SELECT COALESCE(script_original_text,'') FROM narrative_units
		WHERE episode_id=$1 AND deleted_at IS NULL ORDER BY sort_order`, epID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "读取剧本失败")
		return
	}
	var sb strings.Builder
	seen := 0
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		if seen > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(text)
		seen++
	}
	rows.Close()
	if seen == 0 {
		httpx.OK(c, gin.H{"characters": 0, "scenes": 0, "props": 0, "message": "本集无剧本原文，无法提取"})
		return
	}
	// LLM 提取资产（角色≤6、场景≤8，category 限枚举）
	resp, cerr := s.llm.Chat(c.Request.Context(), model.CapLLM, &model.ChatRequest{
		Prompt: fmt.Sprintf(`【剧本原文】
%s

请提取本集中的角色、场景、道具三类资产，严格输出 JSON 对象（不要解释/Markdown）：
{
  "characters": [{"code":"角色代码","name":"角色名","tags":"标签,逗号分隔","description_notes":"角色外貌/服装/性格描述"}],
  "scenes":     [{"code":"场景代码","base_name":"场景名","base_prompt":"场景视觉描述"}],
  "props":      [{"code":"道具代码","name":"道具名","category":"枚举类别","description":"道具描述"}]
}
约束：角色≤6、场景≤8；道具 category 仅限 WEAPON/CLOTHING_ACCESSORY/FOOD_DRINK/STATIONERY_BOOK/JEWELRY_ORNAMENT/FURNITURE_DECOR/ELECTRONIC_APPLIANCE/VEHICLE/TOOL_MUSICAL_INSTRUMENT/TOY/PLANT_FLOWER/ANIMAL_PET/LIGHTING_SOURCE/DOCUMENT_SCROLL_MAP/FLAG_BANNER/OTHER；code/name 不得为空。`, sb.String()),
		System:      `你是短剧资产提取专家。只提取剧本中明确出现的资产，描述需具体可生成，严格输出 JSON 对象。`,
		MaxTokens:   2048,
		Temperature: 0.4,
		NoThinking:  true,
	})
	if cerr != nil || resp == nil || resp.Content == "" {
		httpx.Fail(c, http.StatusBadGateway, httpx.CodeInternal, "AI 资产提取失败，请稍后重试")
		return
	}
	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var extracted struct {
		Characters []struct {
			Code             string `json:"code"`
			Name             string `json:"name"`
			Tags             string `json:"tags"`
			DescriptionNotes string `json:"description_notes"`
		} `json:"characters"`
		Scenes []struct {
			Code       string `json:"code"`
			BaseName   string `json:"base_name"`
			BasePrompt string `json:"base_prompt"`
		} `json:"scenes"`
		Props []struct {
			Code        string `json:"code"`
			Name        string `json:"name"`
			Category    string `json:"category"`
			Description string `json:"description"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(content), &extracted); err != nil {
		httpx.OK(c, gin.H{"characters": 0, "scenes": 0, "props": 0, "message": "AI 返回格式异常，未重建资产", "raw": content})
		return
	}
	// 事务：软删本集旧资产 → 插入新资产
	tx, err := s.repo.pool.Begin(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "开启事务失败")
		return
	}
	defer tx.Rollback(c.Request.Context())
	if _, err := tx.Exec(c.Request.Context(),
		`UPDATE characters SET deleted_at=NOW(), updated_at=NOW() WHERE origin_episode_id=$1 AND deleted_at IS NULL`, epID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置角色失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(),
		`UPDATE scenes SET deleted_at=NOW(), updated_at=NOW() WHERE origin_episode_id=$1 AND deleted_at IS NULL`, epID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置场景失败")
		return
	}
	if _, err := tx.Exec(c.Request.Context(),
		`UPDATE props SET deleted_at=NOW(), updated_at=NOW() WHERE origin_episode_id=$1 AND deleted_at IS NULL`, epID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重置道具失败")
		return
	}
	charCnt, sceneCnt, propCnt := 0, 0, 0
	for _, ch := range extracted.Characters {
		if strings.TrimSpace(ch.Code) == "" || strings.TrimSpace(ch.Name) == "" {
			continue
		}
		tagsJSON, _ := json.Marshal(splitCommaTags(ch.Tags))
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO characters (project_id, code, name, tags, description_notes, origin_episode_id)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6)`,
			projectID, ch.Code, ch.Name, string(tagsJSON), ch.DescriptionNotes, epID); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重建角色失败")
			return
		}
		charCnt++
	}
	for _, sc := range extracted.Scenes {
		if strings.TrimSpace(sc.Code) == "" || strings.TrimSpace(sc.BaseName) == "" {
			continue
		}
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO scenes (project_id, code, base_name, base_prompt, origin_episode_id)
			VALUES ($1, $2, $3, $4, $5)`,
			projectID, sc.Code, sc.BaseName, sc.BasePrompt, epID); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重建场景失败")
			return
		}
		sceneCnt++
	}
	for _, p := range extracted.Props {
		if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
			continue
		}
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO props (project_id, code, name, category, description_prompt, origin_episode_id)
			VALUES ($1, $2, $3, $4::prop_category_enum, $5, $6)`,
			projectID, p.Code, p.Name, reextractSanitizeCategory(p.Category), p.Description, epID); err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重建道具失败")
			return
		}
		propCnt++
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "提交重建失败")
		return
	}
	httpx.OK(c, gin.H{"characters": charCnt, "scenes": sceneCnt, "props": propCnt})
}

// splitCommaTags 逗号分隔标签 → 字符串数组（供 JSONB 序列化）。
func splitCommaTags(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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
	freezeNeeded := 0
	count := req.PerAssetCount
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}
	for _, item := range req.Items {
		// 跨集复用豁免：同项目已用过的资产本次 cost=0（03 E.6.4 不重复扣费）
		cost := defaultCost(item.Kind)
		if reused, _ := s.ops.IsAssetUsedBefore(c.Request.Context(), projectID, item.Kind, item.TargetID); reused {
			cost = 0
		}
		// 批量弹窗顶层模型透传：item 无独立 gen 时套用全批量模型
		gen := item.Gen.toAgent()
		if req.ModelCode != "" {
			if gen == nil {
				gen = &agent.GenParams{}
			}
			if gen.ModelCode == "" {
				gen.ModelCode = req.ModelCode
			}
		}
		// 每张数量：同一目标创建 count 个任务（各自计费），实现「每资产多张」
		for i := 0; i < count; i++ {
			taskID, err := s.ops.CreateAssetTaskWithGen(c.Request.Context(), spaceID, uid, projectID, epID,
				item.Kind, item.TargetID, cost, gen)
			if err != nil {
				fmt.Printf("[asset] CreateAssetTask kind=%s target=%d: %v\n", item.Kind, item.TargetID, err)
				continue
			}
			// 计费冻结（cost>0 时；不足则任务标记失败）
			if cost > 0 && s.billing != nil {
				freezeNeeded++
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
	}
	if created == 0 {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "批量生成任务创建失败")
		return
	}
	if s.billing != nil && freezeNeeded > 0 && frozen == 0 {
		// 全部因积分不足失败（复用豁免 cost=0 不计入 freezeNeeded，不误报）
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInsufficientPoints, "积分不足，无法生成资产")
		return
	}
	httpx.OK(c, gin.H{"created": created, "total": len(req.Items)})
}
