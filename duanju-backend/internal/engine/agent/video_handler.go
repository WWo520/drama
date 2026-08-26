// VideoHandler：Worker-Video（VIDEO_SEGMENT_GEN / VIDEO_GROUP_CONCAT / FINAL_VIDEO_COMPOSE）。
// 真实/降级视频生成 + 进度回写 + shots 状态 CAS 更新（回调幂等：RUNNING→SUCCESS 仅一次）。
// 18 - 视频制作 阶段一：参考图注入（去重 + 上限 + 尾部全局约束指令）与生成参数透传。
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

// videoTaskTypes VIDEO_SEGMENT_GEN 等任务类型。
var videoTaskTypes = map[string]bool{
	"VIDEO_SEGMENT_GEN":  true,
	"VIDEO_GROUP_CONCAT": true,
}

// videoTailConstraint 尾部全局约束指令（对齐 LuxReal：Agent 自动追加、不可删改的硬约束，
// 钉住三件事——忠于脚本、忠于参考图、禁字幕）。
const videoTailConstraint = "\n严格按照分镜脚本进行生成，参考上传图的人物形象、场景和道具。不要生成字幕。"

// maxShotRefImages 单镜头参考图上限分档：火山 Seedance 多图参考上限 4 张；
// maxH3RefImages 为 MiniMax H3（AutoDL v5 多图参考工作流 ref_image_0~8）上限 9 张。
// 参考资产已按 角色→场景→道具 排序，截断保留主体一致性权重最高的前 N 张。
const (
	maxShotRefImages = 4
	maxH3RefImages   = 9
)

// VideoHandler 视频生成 Worker。
type VideoHandler struct {
	store  VideoStore
	engine VideoGenClient // 可为 nil：未注入时降级 mock（开发期）
	env    string         // 08 审查 Major#16：生产环境禁止 mock 降级
}

// VideoGenClient 视频生成能力（由 ModelEngine 实现注入）。
type VideoGenClient interface {
	GenerateVideo(ctx context.Context, capability model.Capability, req *model.VideoRequest) (*model.VideoResponse, error)
}

func NewVideoHandler(store VideoStore, engine VideoGenClient, env string) *VideoHandler {
	return &VideoHandler{store: store, engine: engine, env: env}
}

func (h *VideoHandler) Name() string { return "VIDEO_SEGMENT_GEN" }

func (h *VideoHandler) Handles(taskType string) bool { return videoTaskTypes[taskType] }

// Execute 执行视频生成：解析目标 → 加载参考图 → 真实/降级生成 → shots 状态 CAS（回调幂等）。
func (h *VideoHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ShotID      int64  `json:"shot_id"`
		ShotGroupID int64  `json:"shot_group_id"`
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
	if params.ShotID == 0 {
		return nil, fmt.Errorf("缺少 shot_id")
	}

	// 加载镜头信息
	shot, err := h.store.LoadShot(ctx, params.ShotID)
	if err != nil {
		return nil, err
	}

	// F10: 加载镜头参考资产配对（kind/name/url 同源同序；shot_asset_refs 优先，否则继承 nu_asset_links）。
	// 配对而非两条独立查询：旧链路 URL 列表过滤无图资产、名称列表不过滤 → 索引错位，
	// 且 <Picture N> 绑定需要知道"第 i 张图是哪个资产"。
	refAssets := h.store.LoadShotRefAssets(ctx, params.ShotID, shot.NarrativeUnitID)
	// 镜头级专属参考图（对齐 LuxReal「镜头N的参考图」）：作为最高优先级（Picture 1）注入。
	if strings.TrimSpace(shot.ReferenceImageURL) != "" {
		refAssets = append([]RefAsset{{Kind: "SHOT", Name: "本镜头画面参考", URL: shot.ReferenceImageURL}}, refAssets...)
	}
	refAssets = dedupeRefAssets(refAssets)
	// 上限按模型分档：MiniMax H3（AutoDL v5 多图参考）支持 9 张；火山 Seedance 上限 4 张。
	refCap := maxShotRefImages
	if params.ModelKey == "MiniMax-H3" {
		refCap = maxH3RefImages
	}
	if len(refAssets) > refCap {
		refAssets = refAssets[:refCap]
	}
	refURLs := make([]string, 0, len(refAssets))
	for _, ra := range refAssets {
		refURLs = append(refURLs, ra.URL)
	}
	if len(refAssets) > 0 {
		log.Printf("video: shot %d 参考图 %d 张 target=%s", shot.ShotID, len(refAssets), refAssets[0].URL)
	}

	// 阶段性进度更新（模拟分步处理：提交→排队→渲染→完成）
	h.store.UpdateProgress(ctx, t.ID, 10, "模型排队中…")
	time.Sleep(50 * time.Millisecond) // mock 处理延迟

	h.store.UpdateProgress(ctx, t.ID, 40, "场景渲染中…")
	time.Sleep(100 * time.Millisecond)

	h.store.UpdateProgress(ctx, t.ID, 70, "镜头合成中…")
	time.Sleep(100 * time.Millisecond)

	// 提示词组装（对齐 LuxReal + MiniMax H3 官方规范）：
	// 优先润色提示词（Lux Gem 润色后自然语言，含全部镜头信息）；
	// 为空时按 8 字段结构化拼接（镜头参数头 + 主体/背景/光影/台词/环境音）兜底。
	// 参考绑定双保险：
	//   1) <Picture N> 声明块（H3 Ref2VA 官方机制——按参考图传入顺序编号，逐张声明
	//      控制对象；官方指南 "Assign each reference a job"，人物/场景/道具各锁其维度）
	//   2) @资产名标记（LuxReal 约定保留）+ 「资产名→Picture N」正文映射行
	assetNames := make([]string, 0, len(refAssets))
	for _, ra := range refAssets {
		if ra.Kind != "SHOT" {
			assetNames = append(assetNames, ra.Name)
		}
	}
	basePrompt := shot.PromptRefined
	if strings.TrimSpace(basePrompt) == "" {
		basePrompt = buildStructuredShotPrompt(shot)
	}
	basePrompt = TagAssetsInPrompt(basePrompt, assetNames)
	var promptB strings.Builder
	if len(refAssets) > 0 {
		promptB.WriteString(buildPictureDeclarations(refAssets))
		bindings := make([]string, 0, len(assetNames))
		for i, ra := range refAssets {
			if ra.Kind != "SHOT" {
				bindings = append(bindings, fmt.Sprintf("%s→<Picture %d>", ra.Name, i+1))
			}
		}
		if len(bindings) > 0 {
			promptB.WriteString("资产参考绑定：" + strings.Join(bindings, "、") +
				"（画面须与对应参考图保持一致）\n\n")
		}
	}
	promptB.WriteString(basePrompt)
	promptB.WriteString(videoTailConstraint)
	prompt := promptB.String()
	resolution := params.Resolution
	if resolution == "" {
		resolution = "P720"
	}

	// 真实视频生成（B06 ModelEngine.GenerateVideo）：配置了 T2V 厂商/Key 后走真实调用；
	// 无配置或调用失败降级 mock 样例视频（开发期链路保持可用）。
	videoURL := ""
	isMock := true
	if h.engine != nil && strings.TrimSpace(prompt) != "" {
		resp, gerr := h.engine.GenerateVideo(ctx, model.CapT2V, &model.VideoRequest{
			Prompt:       prompt,
			RefImageURLs: refURLs, // F10: 多参考图注入
			DurationSec:  shot.DurationSec,
			Resolution:   resolution,
			ModelKey:     params.ModelKey,
		})
		if gerr == nil && resp.VideoURL != "" {
			videoURL = resp.VideoURL
			isMock = false
		} else {
			// 08 审查 Major#16：生产环境禁止降级 mock（避免按成功扣费给用户样例视频）
			if h.env == "production" {
				return nil, fmt.Errorf("真实视频生成失败（生产禁用 mock 降级）: %w", gerr)
			}
			log.Printf("video: 真实视频生成失败(%v)，降级 mock shot=%d", gerr, shot.ShotID)
		}
	} else if h.env == "production" && (h.engine == nil || strings.TrimSpace(prompt) == "") {
		return nil, fmt.Errorf("生产环境未配置 T2V 模型或镜头提示词为空")
	}
	if videoURL == "" {
		videoURL = fmt.Sprintf("%s/sample.mp4?shot=%d&t=%d",
			mockCDNBase(), shot.ShotID, time.Now().UnixMilli())
	}
	log.Printf("video: shot %d 视频 url=%s is_mock=%v refs=%d", shot.ShotID, videoURL, isMock, len(refURLs))

	// 回调幂等：SaveShotVideo 内 CAS（仅 PENDING/FAILED 状态写入，已 SUCCESS 跳过）
	if err := h.store.SaveShotVideo(ctx, shot.ShotID, videoURL, 100, t.ID); err != nil {
		return nil, err
	}

	// 更新镜头块状态（检查同块内所有镜头是否全部完成）
	if err := h.store.UpdateShotGroupStatus(ctx, shot.ShotGroupID); err != nil {
		log.Printf("video: 更新镜头块状态 shot_group=%d: %v", shot.ShotGroupID, err)
	}

	out, _ := json.Marshal(map[string]any{
		"video_url":     videoURL,
		"shot_id":       shot.ShotID,
		"shot_group_id": shot.ShotGroupID,
		"progress_pct":  100,
		"is_mock":       isMock,
		"ref_images":    len(refURLs),
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
		default: // SHOT：镜头级专属参考（构图示意，不强制逐帧一致）
			b.WriteString(fmt.Sprintf("<Picture %d> 是本镜头的画面参考（构图与内容示意）。\n", i+1))
		}
	}
	b.WriteString("\n")
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
