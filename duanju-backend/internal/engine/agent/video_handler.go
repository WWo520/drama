// VideoHandler：Worker-Video（VIDEO_SEGMENT_GEN / VIDEO_GROUP_CONCAT / FINAL_VIDEO_COMPOSE）。
// 真实/降级视频生成 + 进度回写 + shots 状态 CAS 更新（回调幂等：RUNNING→SUCCESS 仅一次）。
// 18 - 视频制作 阶段一：参考图注入（去重 + 上限 + 尾部全局约束指令）与生成参数透传。
// 成片卡模式（对齐 LuxReal「规划视频任务」）：params.card_id 非空时按卡生成——
// prompt = <Picture N> 声明块 + 资产绑定语句 + 卡内全部镜头的分镜脚本文本 + 完整约束尾缀。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// RefAsset 参考资产配对（名称+类型+图 URL 一体，同一查询同源同序）。
// 供 <Picture N> 显式绑定：H3 Ref2VA 按图片连接顺序编号，提示词须声明每张图控制什么。
type RefAsset struct {
	Kind string // CHARACTER / SCENE / PROP / SHOT（镜头级专属参考图）
	Name string
	URL  string
}

// VideoStore 视频 Worker 数据边界。
type VideoStore interface {
	// LoadShot 按 shot_id 获取镜头信息（项目/关联资源/8 字段分镜结构）。
	LoadShot(ctx context.Context, shotID int64) (*ShotInfo, error)
	// LoadShotRefs 加载镜头参考图 URL 列表（F10：shot_asset_refs 优先，否则继承 nu_asset_links）。
	LoadShotRefs(ctx context.Context, shotID int64, narrativeUnitID int64) ([]string, error)
	// LoadShotRefAssets 加载参考资产配对（kind/name/url 同源同序，仅有图资产；镜头级优先回退单元级）。
	LoadShotRefAssets(ctx context.Context, shotID int64, narrativeUnitID int64) []RefAsset
	// SaveShotVideo 记录视频生成结果：shots.generated_video_url + video_generation_status=SUCCESS + 回写进度。
	// 幂等：status='PENDING' 或已失败才写入，已 SUCCESS 的直接返回（回调幂等）。
	SaveShotVideo(ctx context.Context, shotID int64, videoURL string, progressPct int, taskID int64) error
	// LoadCard 加载成片卡信息（规划视频任务产物；card 模式生成入口）。
	LoadCard(ctx context.Context, cardID int64) (*CardInfo, error)
	// LoadCardShots 按顺序加载卡内镜头（含 8 字段分镜结构；deleted_at 过滤）。
	LoadCardShots(ctx context.Context, cardID int64) ([]*ShotInfo, error)
	// SaveCardVideo 记录成片卡生成结果：video_cards SUCCESS + 卡内 shots 同步置 SUCCESS
	// （保持 CompleteVideoIfAllDone / Step4 展平逻辑兼容）；CAS 幂等同 SaveShotVideo。
	SaveCardVideo(ctx context.Context, cardID int64, videoURL string, progressPct int, taskID int64) error
	// UpdateProgress 更新任务进度（独立调用，不阻塞主流程）。
	UpdateProgress(ctx context.Context, taskID int64, pct int, msg string) error
	// UpdateShotGroupStatus 更新镜头块状态（所有子镜头完成后）。
	UpdateShotGroupStatus(ctx context.Context, shotGroupID int64) error
	// ConcatShotGroup 镜头块拼接（VIDEO_GROUP_CONCAT）：块内全部 SUCCESS 才标记完成，否则报错。
	ConcatShotGroup(ctx context.Context, shotGroupID int64) error
}

// ShotInfo 镜头信息。
type ShotInfo struct {
	ShotID            int64
	ShotGroupID       int64
	ProjectID         int64
	NarrativeUnitID   int64
	DurationSec       int
	PromptRefined     string
	ReferenceImageURL string // 镜头级参考图（对齐 LuxReal「镜头N的参考图」）
	// 8 字段分镜结构（Minimax H3 / 视频模型结构化提示词；prompt_refined 为空时兜底拼接）
	ShotSize       string
	CameraMovement string
	CameraAngle    string
	Subject        string
	Background     string
	Light          string
	Dialogue       string
	AmbientSound   string
}

// CardInfo 成片卡信息（规划视频任务产物：一次视频生成任务 = 一张卡 = 2~4 个连续分镜镜头）。
type CardInfo struct {
	CardID          int64
	ProjectID       int64
	NarrativeUnitID int64
	DurationSec     int    // 卡内镜头时长加总（生成时按模型上下限截断）
	Title           string // 形如「镜头2~镜头5」
	PromptText      string // 用户自定义提示词；空 = 按分镜脚本三段式自动组装
}

// videoTaskTypes VIDEO_SEGMENT_GEN 等任务类型。
var videoTaskTypes = map[string]bool{
	"VIDEO_SEGMENT_GEN":  true,
	"VIDEO_GROUP_CONCAT": true,
}

// videoTailConstraint 尾部全局约束指令（对齐 LuxReal 成片卡 prompt 实测尾缀：
// 忠于脚本、忠于参考图、禁字幕、音效规则——Agent 自动追加、不可删改的硬约束）。
const videoTailConstraint = "\n严格按照分镜脚本进行生成，参考上传图的人物形象、场景和道具。不要生成字幕。生成关键音效，不要生成背景音乐和BGM。"

// maxShotRefImages 单镜头参考图上限分档：火山 Seedance 多图参考上限 4 张；
// maxH3RefImages 为 MiniMax H3（AutoDL v5 多图参考工作流 ref_image_0~8）上限 9 张。
// 参考资产已按 角色→场景→道具 排序，截断保留主体一致性权重最高的前 N 张。
const (
	maxShotRefImages = 4
	maxH3RefImages   = 9
)

// 成片卡时长送模型上下限（MiniMax H3 支持 4~15 秒；Seedance 系模型内部自行截断）。
const (
	cardMinDurationSec = 4
	cardMaxDurationSec = 15
)

// VideoHandler 视频生成 Worker。
type VideoHandler struct {
	store   VideoStore
	engine  VideoGenClient // 可为 nil：未注入时降级 mock（开发期）
	env     string         // 08 审查 Major#16：生产环境禁止 mock 降级
	persist VideoPersister // 可选：生成视频转存自有对象存储（厂商临时 URL 1~2 天清理 → 视频 404 根因）
}

// VideoGenClient 视频生成能力（由 ModelEngine 实现注入）。
type VideoGenClient interface {
	GenerateVideo(ctx context.Context, capability model.Capability, req *model.VideoRequest) (*model.VideoResponse, error)
}

func NewVideoHandler(store VideoStore, engine VideoGenClient, env string) *VideoHandler {
	return &VideoHandler{store: store, engine: engine, env: env}
}

// SetPersister 注入视频持久化（nil 不转存）。
func (h *VideoHandler) SetPersister(p VideoPersister) { h.persist = p }

func (h *VideoHandler) Name() string { return "VIDEO_SEGMENT_GEN" }

func (h *VideoHandler) Handles(taskType string) bool { return videoTaskTypes[taskType] }

// cardShot 卡内镜头 + 其参考资产配对（绑定语句需要知道"哪个资产出现在哪个镜头"）。
type cardShot struct {
	info *ShotInfo
	refs []RefAsset
}

// Execute 执行视频生成：解析目标 → 加载参考图 → 真实/降级生成 → 状态 CAS（回调幂等）。
// 目标二选一：card_id（成片卡模式，主线）/ shot_id（旧单镜头兼容路径）。
func (h *VideoHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ShotID      int64  `json:"shot_id"`
		ShotGroupID int64  `json:"shot_group_id"`
		CardID      int64  `json:"card_id"`    // 成片卡模式（规划视频任务后的主线链路）
		Resolution  string `json:"resolution"` // 弹窗提交（P480/P720/P1080）；空=默认 P720
		ModelKey    string `json:"model_key"`  // 弹窗提交；空=keystore 默认路由
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	// 镜头块拼接（3.3.8）：块内全部 SUCCESS → 标记 COMPOSED。
	if t.TaskType == "VIDEO_GROUP_CONCAT" {
		return h.executeGroupConcat(ctx, params.ShotGroupID)
	}
	if params.CardID != 0 {
		return h.executeCard(ctx, t, params.CardID, params.Resolution, params.ModelKey)
	}
	if params.ShotID == 0 {
		return nil, fmt.Errorf("缺少 card_id 或 shot_id")
	}

	// ===== 旧单镜头路径（兼容历史任务；新任务一律走成片卡）=====
	shot, err := h.store.LoadShot(ctx, params.ShotID)
	if err != nil {
		return nil, err
	}
	refAssets := h.store.LoadShotRefAssets(ctx, shot.ShotID, shot.NarrativeUnitID)
	if strings.TrimSpace(shot.ReferenceImageURL) != "" && !refHasURL(refAssets, shot.ReferenceImageURL) {
		refAssets = append([]RefAsset{{Kind: "SHOT", Name: "本镜头画面参考", URL: shot.ReferenceImageURL}}, refAssets...)
	}
	refAssets, expired := filterExpiredRefs(dedupeRefAssets(refAssets))
	if len(expired) > 0 {
		log.Printf("video: shot %d 剔除 %d 张已过期 TOS 签名参考图（请对该镜头重生成参考图）: %s",
			shot.ShotID, len(expired), strings.Join(expired, "、"))
	}
	refAssets, refURLs := capRefAssets(refAssets, params.ModelKey)
	if len(refAssets) > 0 {
		log.Printf("video: shot %d 参考资产 %d 个 target=%s", shot.ShotID, len(refAssets), refAssets[0].Name)
	}
	return h.generateAndSave(ctx, t, genTarget{
		Kind: "shot", ID: shot.ShotID, GroupID: shot.ShotGroupID, ProjectID: shot.ProjectID,
		DurationSec: shot.DurationSec,
		Prompt:      buildLegacyPrompt(shot, refAssets), RefURLs: refURLs,
	}, params.Resolution, params.ModelKey)
}

// executeCard 成片卡模式：加载卡 + 全部镜头 → 聚合参考图 → LuxReal 三段式 prompt → 生成 → 回写卡片与镜头。
func (h *VideoHandler) executeCard(ctx context.Context, t *task.Task, cardID int64, resolution, modelKey string) ([]byte, error) {
	card, err := h.store.LoadCard(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("加载成片卡失败: %w", err)
	}
	shots, err := h.store.LoadCardShots(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("加载卡内镜头失败: %w", err)
	}
	if len(shots) == 0 {
		return nil, fmt.Errorf("成片卡 %d 内无有效镜头（分镜脚本可能已重排，请重新规划视频任务）", cardID)
	}

	// 每镜头参考资产配对（镜头级专属参考图前置为 Picture 最高优先级；同 URL 去重留首次出现位置）
	cards := make([]cardShot, 0, len(shots))
	for _, s := range shots {
		ra := h.store.LoadShotRefAssets(ctx, s.ShotID, card.NarrativeUnitID)
		if strings.TrimSpace(s.ReferenceImageURL) != "" && !refHasURL(ra, s.ReferenceImageURL) {
			ra = append([]RefAsset{{Kind: "SHOT", Name: "本镜头画面参考", URL: s.ReferenceImageURL}}, ra...)
		}
		cards = append(cards, cardShot{info: s, refs: ra})
	}
	flatRefs, expired := filterExpiredRefs(dedupeRefAssets(flattenCardRefs(cards)))
	if len(expired) > 0 {
		log.Printf("video: card %d 剔除 %d 张已过期 TOS 签名参考图（请对相关镜头重生成参考图）: %s",
			card.CardID, len(expired), strings.Join(expired, "、"))
	}
	flatRefs, refURLs := capRefAssets(flatRefs, modelKey)
	log.Printf("video: card %d (%s) shots=%d 参考图 %d 张 dur=%ds",
		card.CardID, card.Title, len(cards), len(refURLs), card.DurationSec)

	duration := clampCardDuration(card.DurationSec)

	// 提示词三段式（对齐 LuxReal 成片卡 prompt 实测结构）：
	//   [1] <Picture N> 声明块（H3 Ref2VA 按图序编号逐张声明职责）
	//   [2] 资产绑定语句：「X」是镜头N的参考图 / 多镜头资产列全部镜头号（LuxReal 绑定语法）
	//   [3] 分镜脚本文本：以下内容是具体的分镜脚本：镜头1，【时长】2s…（卡内全镜头逐个拼入）
	// 用户自定义 prompt_text 替换 [2][3]；尾缀恒定追加（幂等防重复）。
	basePrompt := strings.TrimSpace(card.PromptText)
	if basePrompt == "" {
		basePrompt = buildLuxRealCardPrompt(cards, flatRefs)
	}
	prompt := basePrompt
	if len(flatRefs) > 0 {
		prompt = buildPictureDeclarations(flatRefs) + prompt
	}
	if !strings.Contains(prompt, "严格按照分镜脚本进行生成") {
		prompt += videoTailConstraint
	}

	return h.generateAndSave(ctx, t, genTarget{
		Kind: "card", ID: card.CardID, ProjectID: card.ProjectID,
		DurationSec: duration, Prompt: prompt, RefURLs: refURLs,
	}, resolution, modelKey)
}

// genTarget 生成目标归一（card 主线 / shot 兼容）。
type genTarget struct {
	Kind        string // "card" / "shot"
	ID          int64  // card_id / shot_id
	GroupID     int64  // shot 模式的镜头块 id
	ProjectID   int64  // 转存自有存储的 objectKey 归属
	DurationSec int
	Prompt      string
	RefURLs     []string
}

// generateAndSave 共用执行段：mock 进度推进 → 引擎生成（生产禁降级）→ CAS 回写 → 输出摘要。
func (h *VideoHandler) generateAndSave(ctx context.Context, t *task.Task, g genTarget, resolution, modelKey string) ([]byte, error) {
	// 阶段性进度更新（模拟分步处理：提交→排队→渲染→完成）
	h.store.UpdateProgress(ctx, t.ID, 10, "模型排队中…")
	time.Sleep(50 * time.Millisecond)
	h.store.UpdateProgress(ctx, t.ID, 40, "场景渲染中…")
	time.Sleep(100 * time.Millisecond)
	h.store.UpdateProgress(ctx, t.ID, 70, "镜头合成中…")
	time.Sleep(100 * time.Millisecond)

	res := resolution
	if res == "" {
		res = "P720"
	}
	var videoURL string
	isMock := false
	switch {
	case h.engine != nil && strings.TrimSpace(g.Prompt) != "":
		resp, gerr := h.engine.GenerateVideo(ctx, model.CapT2V, &model.VideoRequest{
			Prompt:       g.Prompt,
			RefImageURLs: g.RefURLs,
			DurationSec:  g.DurationSec,
			Resolution:   res,
			ModelKey:     modelKey,
		})
		if gerr != nil || resp.VideoURL == "" {
			// 真实调用失败一律返回错误交由任务层重试（reaper 到上限后任务 FAILED 可见）；
			// 不再降级 mock——伪造"已生成"的占位视频会误导用户（生产禁用逻辑合并于此）。
			return nil, fmt.Errorf("真实视频生成失败: %w", gerr)
		}
		videoURL = resp.VideoURL
	case h.engine == nil && h.env != "production":
		// 未配置任何厂商的开发环境才允许链路自测 mock
		isMock = true
	case h.env == "production":
		return nil, fmt.Errorf("生产环境未配置 T2V 模型或提示词为空")
	default:
		return nil, fmt.Errorf("提示词为空，无法生成视频")
	}
	if videoURL == "" {
		videoURL = fmt.Sprintf("%s/sample.mp4?%s=%d&t=%d", mockCDNBase(), g.Kind, g.ID, time.Now().UnixMilli())
	}
	// 真实结果转存自有对象存储（厂商临时 URL 保留期 1~2 天 → 历史视频 404 根因）；
	// 转存失败保留原 URL 不阻断；mock 占位地址自动跳过。
	videoURL = persistGeneratedVideo(ctx, h.persist, g.ProjectID, g.Kind, g.ID, videoURL)
	log.Printf("video: %s %d 视频 url=%s is_mock=%v refs=%d dur=%ds",
		g.Kind, g.ID, videoURL, isMock, len(g.RefURLs), g.DurationSec)

	switch g.Kind {
	case "card":
		if err := h.store.SaveCardVideo(ctx, g.ID, videoURL, 100, t.ID); err != nil {
			return nil, err
		}
	default:
		if err := h.store.SaveShotVideo(ctx, g.ID, videoURL, 100, t.ID); err != nil {
			return nil, err
		}
		if g.GroupID > 0 {
			if err := h.store.UpdateShotGroupStatus(ctx, g.GroupID); err != nil {
				log.Printf("video: 更新镜头块状态 shot_group=%d: %v", g.GroupID, err)
			}
		}
	}

	out, _ := json.Marshal(map[string]any{
		"video_url":    videoURL,
		"target_kind":  g.Kind,
		"target_id":    g.ID,
		"progress_pct": 100,
		"is_mock":      isMock,
		"ref_images":   len(g.RefURLs),
	})
	return out, nil
}

// executeGroupConcat 镜头块拼接（VIDEO_GROUP_CONCAT）。
func (h *VideoHandler) executeGroupConcat(ctx context.Context, shotGroupID int64) ([]byte, error) {
	if shotGroupID == 0 {
		return nil, fmt.Errorf("缺少 shot_group_id")
	}
	if err := h.store.ConcatShotGroup(ctx, shotGroupID); err != nil {
		return nil, err
	}
	log.Printf("video: shot_group %d 拼接完成（COMPOSED）", shotGroupID)
	out, _ := json.Marshal(map[string]any{
		"shot_group_id": shotGroupID,
		"status":        "COMPOSED",
	})
	return out, nil
}

// ---- 纯函数辅助（可单测）----

// dedupeRefAssets 参考资产按 URL 去重（角色→场景→道具序保持）。
func dedupeRefAssets(refs []RefAsset) []RefAsset {
	seen := make(map[string]bool, len(refs))
	out := make([]RefAsset, 0, len(refs))
	for _, ra := range refs {
		if ra.URL == "" || seen[ra.URL] {
			continue
		}
		seen[ra.URL] = true
		out = append(out, ra)
	}
	return out
}

// flattenCardRefs 展平卡内各镜头参考资产（保镜头顺序）。
func flattenCardRefs(cards []cardShot) []RefAsset {
	all := make([]RefAsset, 0, len(cards)*3)
	for _, cs := range cards {
		all = append(all, cs.refs...)
	}
	return all
}

// refHasURL 判断参考列表中是否已含指定 URL。
func refHasURL(refs []RefAsset, url string) bool {
	for _, ra := range refs {
		if ra.URL == url {
			return true
		}
	}
	return false
}

// capRefAssets 按模型分档截断参考列表（声明块与 URL 列表索引保持对齐）并返回两者。
func capRefAssets(refs []RefAsset, modelKey string) ([]RefAsset, []string) {
	limit := maxShotRefImages
	if modelKey == "MiniMax-H3" {
		limit = maxH3RefImages
	}
	if len(refs) > limit {
		refs = refs[:limit]
	}
	urls := make([]string, 0, len(refs))
	for _, ra := range refs {
		urls = append(urls, ra.URL)
	}
	return refs, urls
}

// clampCardDuration 成片卡送模型时长上下限（MiniMax H3 支持 4~15 秒）。
func clampCardDuration(sec int) int {
	if sec < cardMinDurationSec {
		return cardMinDurationSec
	}
	if sec > cardMaxDurationSec {
		return cardMaxDurationSec
	}
	return sec
}

// legacyPrompt 构建旧单镜头提示词：Picture 声明 + @ 资产标记润色稿/结构化兜底 + 尾缀。
func buildLegacyPrompt(s *ShotInfo, refs []RefAsset) string {
	names := make([]string, 0, len(refs))
	for _, ra := range refs {
		if ra.Kind != "SHOT" {
			names = append(names, ra.Name)
		}
	}
	base := s.PromptRefined
	if strings.TrimSpace(base) == "" {
		base = buildStructuredShotPrompt(s)
	}
	base = TagAssetsInPrompt(base, names)
	var b strings.Builder
	if len(refs) > 0 {
		b.WriteString(buildPictureDeclarations(refs))
		bindings := make([]string, 0, len(names))
		for i, ra := range refs {
			if ra.Kind != "SHOT" {
				bindings = append(bindings, fmt.Sprintf("%s→<Picture %d>", ra.Name, i+1))
			}
		}
		if len(bindings) > 0 {
			b.WriteString("资产参考绑定：" + strings.Join(bindings, "、") +
				"（画面须与对应参考图保持一致）\n\n")
		}
		// 场景锁定（与 buildLuxRealCardPrompt 铁律段同源，防对话/特写镜头场景漂移）
		var sceneNames []string
		for _, ra := range refs {
			if ra.Kind == "SCENE" && ra.Name != "" {
				sceneNames = append(sceneNames, ra.Name)
			}
		}
		if len(sceneNames) > 0 {
			b.WriteString("场景锁定：本段视频所有镜头必须发生在「" + strings.Join(sceneNames, "」「") +
				"」场景内，不得切换到其他地点。\n\n")
		}
	}
	b.WriteString(base)
	b.WriteString(videoTailConstraint)
	return b.String()
}

// buildPictureDeclarations 生成 <Picture N> 声明块（H3 Ref2VA 官方参考绑定机制：
// 按参考图传入顺序编号，逐张声明控制对象。官方指南 "Assign each reference a job"——
// 人物锁面部/发型/体型/服饰，场景锁布局/色调/氛围，道具锁外观细节）。
func buildPictureDeclarations(refs []RefAsset) string {
	var b strings.Builder
	for i, ra := range refs {
		switch ra.Kind {
		case "CHARACTER":
			b.WriteString(fmt.Sprintf("<Picture %d> 是角色「%s」的形象参考：画面中%s的面部、发型、体型与服饰须与该参考图保持一致。\n", i+1, ra.Name, ra.Name))
		case "SCENE":
			b.WriteString(fmt.Sprintf("<Picture %d> 是场景「%s」的参考：画面中的环境布局、色调与氛围须与该参考图保持一致。\n", i+1, ra.Name))
		case "PROP":
			b.WriteString(fmt.Sprintf("<Picture %d> 是道具「%s」的参考：其外观细节须与该参考图保持一致。\n", i+1, ra.Name))
		default: // SHOT：镜头级专属参考（本卡首帧画面基准——LuxReal 智能镜头的降级替代）
			b.WriteString(fmt.Sprintf("<Picture %d> 是本段视频开场第一镜的画面基准（人物站位、环境布局、光线氛围必须从该画面自然延续展开，不得凭空另起构图）。\n", i+1))
		}
	}
	b.WriteString("\n")
	return b.String()
}

// buildAssetBindingLine LuxReal 成片卡实测的资产绑定语句段：
// 「香槟塔金色酒液流动」是镜头1的参考图，「豪华宴会厅」是镜头2、镜头3的参考图。
// 对每个聚合参考资产拼装其出现的卡内镜头号（refs 序即 Picture 序，双保险互证）。
func buildAssetBindingLine(cards []cardShot, refs []RefAsset) string {
	hitShots := make(map[string][]int, len(refs)) // url → 出现的镜头号（1-based 卡内序号）
	for no, cs := range cards {
		for _, ra := range cs.refs {
			hitShots[ra.URL] = append(hitShots[ra.URL], no+1)
		}
	}
	parts := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ra := range refs {
		nums := hitShots[ra.URL]
		if len(nums) == 0 || seen[ra.Name] {
			continue
		}
		seen[ra.Name] = true
		numStr := make([]string, 0, len(nums))
		for _, n := range nums {
			numStr = append(numStr, fmt.Sprintf("镜头%d", n))
		}
		parts = append(parts, fmt.Sprintf("「%s」是%s的参考图", ra.Name, strings.Join(numStr, "、")))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "，") + "。"
}

// buildShotScriptLine 单镜头 → LuxReal 分镜脚本文本块。
// 精度优先级（对齐 LuxReal「润色后的镜头描述直接作为视频 prompt 主体」）：
// 有润色稿（prompt_refined，含景别/运镜/动作/表情/承接/转场等融合细化语言）整段作为画面主体；
// 无润色稿回退 8 字段结构化拼接。头行恒带 镜号+时长+时间戳（对齐 LuxReal 实测规范：
// 镜号必须写清楚、时间戳明确逐镜节奏），供模型对齐分镜时序。
func buildShotScriptLine(no int, s *ShotInfo, startSec int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "镜头%02d【时间戳 %d-%ds】【时长】%ds", no, startSec, startSec+s.DurationSec, s.DurationSec)
	if refined := strings.TrimSpace(s.PromptRefined); refined != "" {
		// 润色稿自带「镜头 | N秒」头行，与外层编号/时长重复，剥掉首行再嵌入
		if i := strings.IndexByte(refined, '\n'); i > 0 && strings.Contains(refined[:i], "秒") {
			refined = strings.TrimSpace(refined[i+1:])
		}
		b.WriteString("\n" + refined)
		return b.String()
	}
	if v, ok := shotSizeCN[s.ShotSize]; ok && v != "" {
		b.WriteString("\n【景别】" + v)
	}
	if v, ok := cameraMoveCN[s.CameraMovement]; ok && v != "" {
		b.WriteString("【运镜】" + v)
	}
	if v, ok := cameraAngleCN[s.CameraAngle]; ok && v != "" {
		b.WriteString("【视角】" + v)
	}
	if s.Subject != "" {
		b.WriteString("\n【主体】" + s.Subject)
	}
	if s.Background != "" {
		b.WriteString("\n【背景】" + s.Background)
	}
	if s.Light != "" {
		b.WriteString("\n【光影】" + s.Light)
	}
	if s.Dialogue != "" {
		b.WriteString("\n【台词】" + s.Dialogue)
	}
	if s.AmbientSound != "" {
		b.WriteString("\n【音效】" + s.AmbientSound)
	}
	return b.String()
}

// buildLuxRealCardPrompt 三段式主体（对齐 LuxReal 单视频提示词润色规范——
// 开头先锁「主体人物+场景+音效要求」，中段多镜号分镜脚本带时间戳，末段一致性铁律）：
//
//	[锚定段] 主体人物 / 场景 清单 + 「不要生成任何BGM」声明
//	[脚本段] 资产绑定语句 + 卡内全镜头逐块文本（镜头01【时间戳 0-3s】…）
//	[铁律段] 逐镜衔接与参考图一致性约束
//
// 不含 <Picture> 声明头与尾部全局硬约束（调用方拼接）。
func buildLuxRealCardPrompt(cards []cardShot, refs []RefAsset) string {
	var b strings.Builder
	// [锚定段] 从聚合参考资产抽取角色/场景名锁定主语（名字贯穿全片提示词）
	var chars, scenes []string
	for _, ra := range refs {
		switch ra.Kind {
		case "CHARACTER":
			chars = append(chars, ra.Name)
		case "SCENE":
			scenes = append(scenes, ra.Name)
		}
	}
	b.WriteString("【画面主体】")
	if len(chars) > 0 {
		b.WriteString("角色：" + strings.Join(chars, "、") + "。")
	} else {
		b.WriteString("以分镜脚本描述的主体为准。")
	}
	if len(scenes) > 0 {
		b.WriteString("场景：" + strings.Join(scenes, "、") + "。")
	}
	b.WriteString("\n【音效要求】只生成画面与环境音效，不要生成任何 BGM 或背景音乐。\n\n")
	// [脚本段] 资产绑定语句（LuxReal 绑定语法：「X」是镜头1、镜头2的参考图）+ 逐镜号块
	if line := buildAssetBindingLine(cards, refs); line != "" {
		b.WriteString(line + "\n\n")
	}
	b.WriteString("以下内容是具体的分镜脚本：\n")
	offset := 0
	for no, cs := range cards {
		b.WriteString(buildShotScriptLine(no+1, cs.info, offset) + "\n")
		offset += cs.info.DurationSec
	}
	// [铁律段] 连贯性与一致性硬约束
	b.WriteString("\n各镜头为同一时空连续叙事：人物的位置、朝向、动作状态必须逐镜衔接，" +
		"前后镜头的起止画面不能跳变；人物形象、服饰与场景布局严格与对应参考图保持一致。")
	// 场景锁定（防场景漂移）：卡内已关联场景时显式禁止跨卡/跨镜切换地点
	// （用户实测"餐厅对话答话人不在餐厅"——对话特写镜头缺场景约束时模型自由发挥地点）。
	if len(scenes) > 0 {
		b.WriteString("\n场景锁定：本段视频所有镜头必须发生在「" + strings.Join(scenes, "」「") +
			"」场景内，环境布局、陈设、色调与光线保持连续，任何镜头（含人物特写）不得切换到其他地点或凭空出现新场景。")
	}
	b.WriteString("\n严格按照每个镜头的时间戳顺序推进画面。")
	return b.String()
}

// shotParamHeadCN 枚举 → 中文镜头术语（结构化提示词头）。
var (
	shotSizeCN = map[string]string{
		"ECU": "大特写", "CU": "特写", "MCU": "中特写", "MS": "中景", "FS": "全景",
		"LS": "远景", "ELS": "大远景", "TWO_SHOT": "双人镜头", "OTS": "过肩镜头",
	}
	cameraMoveCN = map[string]string{
		"STATIC": "固定机位", "SLOW_PUSH_IN": "缓慢推近", "SLOW_PULL_OUT": "缓慢拉远", "FAST_PUSH_IN": "快速推近",
		"DOLLY_IN": "推轨前进", "DOLLY_OUT": "推轨后退", "PAN_LEFT": "左摇", "PAN_RIGHT": "右摇",
		"TRACK_LEFT": "左横移跟拍", "TRACK_RIGHT": "右横移跟拍", "TILT_UP": "仰摇上摇", "TILT_DOWN": "俯摇下摇",
		"CRANE_UP": "升镜头", "CRANE_DOWN": "降镜头", "HANDHELD_SHAKY": "手持晃动", "WIPE_CLIP": "甩镜切换",
		"ZOOM_IN_OPTICAL": "光学变焦推近", "SLOW_360_ORBIT": "缓慢环绕",
	}
	cameraAngleCN = map[string]string{
		"EYE_LEVEL_FRONT": "平视正面", "SIDE": "侧面", "BACK": "背面", "SLIGHT_LOW_ANGLE": "微仰拍",
		"LOW_ANGLE": "仰拍", "SLIGHT_HIGH": "微俯拍", "HIGH_BIRDVIEW": "鸟瞰俯拍",
		"OTS_LEFT": "左侧过肩", "OTS_RIGHT": "右侧过肩", "DUTCH_ANGLE": "倾斜构图",
	}
)

// buildStructuredShotPrompt 8 字段结构化提示词（prompt_refined 为空时兜底；
// 结构对齐 LuxReal prompt_original 与 Minimax H3 提示词最佳实践：镜头参数头 → 叙述主体）。
func buildStructuredShotPrompt(s *ShotInfo) string {
	var b strings.Builder
	head := make([]string, 0, 3)
	if v, ok := shotSizeCN[s.ShotSize]; ok && v != "" {
		head = append(head, v)
	}
	if v, ok := cameraAngleCN[s.CameraAngle]; ok && v != "" {
		head = append(head, v)
	}
	if v, ok := cameraMoveCN[s.CameraMovement]; ok && v != "" {
		head = append(head, v)
	}
	if len(head) > 0 {
		b.WriteString("[" + strings.Join(head, "+") + "] ")
	}
	if s.Subject != "" {
		b.WriteString("主体：" + s.Subject + "。")
	}
	if s.Background != "" {
		b.WriteString("背景：" + s.Background + "。")
	}
	if s.Light != "" {
		b.WriteString("光影：" + s.Light + "。")
	}
	if s.Dialogue != "" {
		b.WriteString("台词：" + s.Dialogue + "。")
	}
	if s.AmbientSound != "" {
		b.WriteString("环境音：" + s.AmbientSound + "。")
	}
	return strings.TrimSpace(b.String())
}

// TagAssetsInPrompt 提示词中出现的关联资产名加 @ 前缀（LuxReal @精准引用；
// 仅替换未被 @ 标记的裸资产名，避免重复标记）。导出供 repository 分镜落库时复用
// （prompt_refined 持久化即含 @引用，对齐 LuxReal 分镜脚本文本）。
func TagAssetsInPrompt(prompt string, assetNames []string) string {
	for _, name := range assetNames {
		name = strings.TrimSpace(name)
		if len([]rune(name)) < 2 {
			continue // 单字名误替换率高，跳过
		}
		if strings.Contains(prompt, "@"+name) {
			continue // 已显式 @ 标记
		}
		prompt = strings.ReplaceAll(prompt, name, "@"+name)
	}
	return prompt
}
