// StoryboardPlanHandler：STORYBOARD_PLAN_V1_LUXCHAT 任务执行器（18 - 视频制作 阶段二）。
// 独立分镜脚本生成技能（对齐 LuxReal Step3「重新规划/重生成分镜」Tab2）：
// 叙事单元剧本原文 + 时长预设 + 关联资产清单 → LLM 8 字段分镜 → 润色 → 事务替换 shots + 版本快照。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// storyboardPlanSystemPrompt 分镜脚本生成技能（LuxReal「剧情细腻刻画」模板结构）。
// ref_assets（对齐 LuxReal shots.ref_assets @引用资产列表）：LLM 逐镜头显式声明画面中
// 出现的资产，替代纯文本名称匹配（场景复合名/道具别名文本匹配必然失配）。
const storyboardPlanSystemPrompt = `你是短剧分镜编剧。根据叙事单元剧本原文与时长预设，编写该单元的分镜脚本。
输出 JSON 结构（只输出 JSON，不要任何解释文字）：
{"shot_groups":[{"title":"镜头块标题","duration_sec":秒数,"shots":[{"duration_sec":秒,"shot_size":"景别","camera_movement":"运镜","camera_angle":"视角","subject_description":"主体描述","background_description":"背景描述","local_light_description":"光影描述","dialogue_text":"台词","ambient_sound":"环境音","ref_assets":["资产名"]}]}]}

【规则】
1. 全部镜头 duration_sec 加总须接近单元时长预设 %d 秒（误差 ±20%% 以内）；
2. 每单元 1~3 个镜头块，每块 2~4 个镜头，单镜头 2~9 秒；
3. shot_size 仅限 ECU/CU/MCU/MS/FS/LS/ELS/TWO_SHOT/OTS；
4. camera_movement 仅限 STATIC/SLOW_PUSH_IN/SLOW_PULL_OUT/FAST_PUSH_IN/DOLLY_IN/DOLLY_OUT/PAN_LEFT/PAN_RIGHT/TRACK_LEFT/TRACK_RIGHT/TILT_UP/TILT_DOWN/CRANE_UP/CRANE_DOWN/HANDHELD_SHAKY/WIPE_CLIP/ZOOM_IN_OPTICAL/SLOW_360_ORBIT；
5. camera_angle 仅限 EYE_LEVEL_FRONT/SIDE/BACK/SLIGHT_LOW_ANGLE/LOW_ANGLE/SLIGHT_HIGH/HIGH_BIRDVIEW/OTS_LEFT/OTS_RIGHT/DUTCH_ANGLE；
6. 台词格式：【角色名 情感修饰】：台词（如【陆言 激昂】：各位看官！）；无台词留空字符串；
7. 资产清单：%s。subject_description 必须显式使用清单中的完整资产名（主体角色写进主体描述），background_description 必须写明所在场景的完整场景名，保证后续参考图引用可命中；主体描述包含人物动作/表情/道具互动细节；
8. ref_assets：该镜头画面中实际出现的资产名数组，必须从资产清单原样引用完整名称（清单为"林晚公寓"不得简写为"公寓"）；涵盖主体角色、所在场景、出镜或互动的道具；剧本以别名提及清单资产时（如"玉佩"即清单中的"引魂佩"）须引用清单名；画面未出现任何清单资产时留空数组；
9. 分镜须完整覆盖剧本原文的全部情节，忠于原文不得新增剧情。`

// ErrBeatBusy 单元存在 RUNNING 视频任务，拒绝重规划（防外键/状态残留）。
var ErrBeatBusy = errors.New("storyboard: narrative unit has running video tasks")

// BeatPlanAsset 分镜规划可用资产（Kind: CHARACTER/SCENE/PROP）。
type BeatPlanAsset struct {
	Kind string
	Name string
}

// BeatPlanInput 分镜规划输入。
type BeatPlanInput struct {
	NarrativeUnitID    int64
	ProjectID          int64
	ScriptOriginalText string
	PresetDurationSec  int
	Assets             []BeatPlanAsset // 单元关联资产优先 + 项目其余资产补全（供显式引用与 ref_assets 声明）
}

// StoryboardPlanStore 分镜规划数据边界。
type StoryboardPlanStore interface {
	// LoadBeatForPlan 加载单元剧本/时长预设/关联资产名。
	LoadBeatForPlan(ctx context.Context, narrativeUnitID int64) (*BeatPlanInput, error)
	// SaveStoryboardPlan 事务写入：软删旧 shots/groups → 插入新分镜 → 新版本快照 → 单元时长回写；
	// 并按新镜头文本重建单元资产链。存在 RUNNING 视频任务时返回 ErrBeatBusy。
	SaveStoryboardPlan(ctx context.Context, narrativeUnitID int64, groups []ShotGroup) (*SplitStats, error)
}

// StoryboardPlanHandler 实现 task.Handler（task_type=STORYBOARD_PLAN_V1_LUXCHAT）。
type StoryboardPlanHandler struct {
	llm    LLMClient
	store  StoryboardPlanStore
	maxTry int
}

func NewStoryboardPlanHandler(llm LLMClient, store StoryboardPlanStore) *StoryboardPlanHandler {
	return &StoryboardPlanHandler{llm: llm, store: store, maxTry: 3}
}

func (h *StoryboardPlanHandler) Name() string { return "STORYBOARD_PLAN_V1_LUXCHAT" }

// Execute 执行分镜重生成：读单元 → LLM 生成分镜（坏 JSON 重试）→ 逐镜头润色 → 事务替换入库。
func (h *StoryboardPlanHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		BeatID       int64  `json:"beat_id"` // narrative_units.id
		PlanningMode string `json:"planning_mode"`
		SkillPrompt  string `json:"skill_prompt"` // 分镜技能风格提示词（可选）
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	if params.BeatID == 0 {
		return nil, fmt.Errorf("缺少 beat_id")
	}
	beat, err := h.store.LoadBeatForPlan(ctx, params.BeatID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(beat.ScriptOriginalText) == "" {
		return nil, errors.New("叙事单元剧本原文为空，无法生成分镜")
	}

	assetList := formatPlanAssetList(beat.Assets)
	systemPrompt := fmt.Sprintf(storyboardPlanSystemPrompt, beat.PresetDurationSec, assetList)
	if params.SkillPrompt != "" {
		systemPrompt += "\n分镜风格技能（与上述规则冲突时以技能要求为准）：" + params.SkillPrompt
	}

	// LLM 生成分镜（坏 JSON/结构非法自动重试）
	var groups []ShotGroup
	var lastErr error
	extraHint := ""
	for attempt := 1; attempt <= h.maxTry; attempt++ {
		resp, cerr := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
			Prompt:      beat.ScriptOriginalText,
			System:      systemPrompt + extraHint,
			MaxTokens:   8192,
			Temperature: 0.4,
			JSONMode:    true,
			NoThinking:  true, // 分镜规划无需推理，避免烧光 token 预算
		})
		if cerr != nil {
			lastErr = fmt.Errorf("分镜 LLM 调用失败(第%d次): %w", attempt, cerr)
			log.Printf("storyboard-plan: 第%d次 LLM 调用失败: %v", attempt, cerr)
			continue
		}
		groups, lastErr = parseStoryboardGroups(resp.Content)
		if lastErr == nil {
			break
		}
		log.Printf("storyboard-plan: 第%d次输出非法（%v），重试", attempt, lastErr)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("分镜生成失败（重试%d次）: %w", h.maxTry, lastErr)
	}

	// 逐镜头润色出 prompt_refined（复用拆分润色技能；失败回退结构化描述，不阻塞）
	h.polishGroups(ctx, groups)

	stats, err := h.store.SaveStoryboardPlan(ctx, params.BeatID, groups)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(stats)
	return out, nil
}

// formatPlanAssetList 资产清单格式化（按 角色/场景/道具 分组，注入 prompt 供 LLM 原样引用）。
func formatPlanAssetList(assets []BeatPlanAsset) string {
	byKind := map[string][]string{"CHARACTER": {}, "SCENE": {}, "PROP": {}}
	for _, a := range assets {
		if name := strings.TrimSpace(a.Name); name != "" {
			byKind[a.Kind] = append(byKind[a.Kind], name)
		}
	}
	var parts []string
	if len(byKind["CHARACTER"]) > 0 {
		parts = append(parts, "角色："+strings.Join(byKind["CHARACTER"], "、"))
	}
	if len(byKind["SCENE"]) > 0 {
		parts = append(parts, "场景："+strings.Join(byKind["SCENE"], "、"))
	}
	if len(byKind["PROP"]) > 0 {
		parts = append(parts, "道具："+strings.Join(byKind["PROP"], "、"))
	}
	if len(parts) == 0 {
		return "无"
	}
	return strings.Join(parts, "；")
}

// parseStoryboardGroups 解析并校验 LLM 分镜输出（复用拆分枚举真源）。
func parseStoryboardGroups(raw string) ([]ShotGroup, error) {
	clean := extractJSON(raw)
	var wrapped struct {
		ShotGroups []ShotGroup `json:"shot_groups"`
	}
	if err := json.Unmarshal([]byte(clean), &wrapped); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if len(wrapped.ShotGroups) == 0 {
		return nil, fmt.Errorf("%w: 缺少 shot_groups", ErrInvalidStructure)
	}
	for gi, sg := range wrapped.ShotGroups {
		if len(sg.Shots) == 0 {
			return nil, fmt.Errorf("%w: 镜头块 %d 缺少 shots", ErrInvalidStructure, gi+1)
		}
		for si, s := range sg.Shots {
			if s.DurationSec <= 0 || s.DurationSec > 30 {
				return nil, fmt.Errorf("%w: 镜头时长非法 %d（块%d/镜头%d）", ErrInvalidStructure, s.DurationSec, gi+1, si+1)
			}
			if !validShotSizes[s.ShotSize] {
				return nil, fmt.Errorf("%w: 景别 %q 非法", ErrInvalidStructure, s.ShotSize)
			}
			if !validCameraMoves[s.CameraMovement] {
				return nil, fmt.Errorf("%w: 运镜 %q 非法", ErrInvalidStructure, s.CameraMovement)
			}
			if !validCameraAngles[s.CameraAngle] {
				return nil, fmt.Errorf("%w: 视角 %q 非法", ErrInvalidStructure, s.CameraAngle)
			}
		}
	}
	sanitizeRefAssets(wrapped.ShotGroups)
	return wrapped.ShotGroups, nil
}

// sanitizeRefAssets ref_assets 清洗：去空白/空名、截断上限、去重。
// 非法引用不致命（失配项在建链时被忽略，仍有文本匹配兜底），不触发整轮重试。
func sanitizeRefAssets(groups []ShotGroup) {
	for gi := range groups {
		for si := range groups[gi].Shots {
			s := &groups[gi].Shots[si]
			seen := make(map[string]bool, len(s.RefAssetNames))
			clean := make([]string, 0, len(s.RefAssetNames))
			for _, n := range s.RefAssetNames {
				if n = strings.TrimSpace(n); n != "" && !seen[n] && len(clean) < 8 {
					seen[n] = true
					clean = append(clean, n)
				}
			}
			s.RefAssetNames = clean
		}
	}
}

// polishGroups 逐镜头润色（与拆分 polishAll 同款技能，单镜头失败不中断）。
func (h *StoryboardPlanHandler) polishGroups(ctx context.Context, groups []ShotGroup) {
	for gi := range groups {
		sg := &groups[gi]
		for si := range sg.Shots {
			s := &sg.Shots[si]
			prompt := fmt.Sprintf("镜头时长%d秒。景别:%s。运镜:%s。视角:%s。主体:%s。背景:%s。光影:%s。台词:%s。环境音:%s。",
				int(s.DurationSec), shotSizeCn(s.ShotSize), cameraMoveCn(s.CameraMovement), cameraAngleCn(s.CameraAngle),
				s.SubjectDescription, s.BackgroundDescription, s.LocalLightDescription, s.DialogueText, s.AmbientSound)
			resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
				Prompt:      prompt,
				System:      fmt.Sprintf(polishSystemPrompt, int(s.DurationSec)),
				MaxTokens:   512,
				Temperature: 0.5,
				NoThinking:  true,
			})
			if err != nil || resp.Content == "" {
				s.PromptRefined = luxShotPrompt(s)
				continue
			}
			s.PromptRefined = resp.Content
		}
	}
}
