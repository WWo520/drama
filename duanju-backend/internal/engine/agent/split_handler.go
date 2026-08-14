// SplitHandler：SCRIPT_SPLIT 任务执行器（双 LLM：拆分 + 润色；坏 JSON 自动重试）。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// 拆分技能 System Prompt（J.2 技能正文规则：角色≤6、场景≤8、景别/运镜/视角枚举、严格 JSON）。
const splitSystemPrompt = `你是短剧分镜拆解专家。将用户提供的短剧剧本拆解为四层 JSON 结构：
{"episodes":[{"title":"第1集标题","narrative_units":[{"title":"叙事单元标题","duration_sec":秒数,"script_original_text":"该单元对应剧本原文","shot_groups":[{"title":"镜头块标题","duration_sec":秒数,"shots":[{"duration_sec":秒,"shot_size":"景别","camera_movement":"运镜","camera_angle":"视角","subject_description":"主体描述","background_description":"背景描述","local_light_description":"光影描述","dialogue_text":"台词","ambient_sound":"环境音"}]}]}]}]}
规则：
1. 每个叙事单元 2~3 个镜头块，每个镜头块 2~4 个镜头；
2. shot_size 仅限 ECU/CU/MCU/MS/FS/LS/ELS/TWO_SHOT/OTS；
3. camera_movement 仅限 STATIC/SLOW_PUSH_IN/SLOW_PULL_OUT/FAST_PUSH_IN/DOLLY_IN/DOLLY_OUT/PAN_LEFT/PAN_RIGHT/TRACK_LEFT/TRACK_RIGHT/TILT_UP/TILT_DOWN/CRANE_UP/CRANE_DOWN/HANDHELD_SHAKY/WIPE_CLIP/ZOOM_IN_OPTICAL/SLOW_360_ORBIT；
4. camera_angle 仅限 EYE_LEVEL_FRONT/SIDE/BACK/SLIGHT_LOW_ANGLE/LOW_ANGLE/SLIGHT_HIGH/HIGH_BIRDVIEW/OTS_LEFT/OTS_RIGHT/DUTCH_ANGLE；
5. 角色≤6人、场景≤8个；时长合计需覆盖剧本情节；
6. 只输出合法 JSON，不要任何解释文字。`

const polishSystemPrompt = `你是视频提示词润色专家。将镜头要素（主体/背景/光影/台词/环境音/景别/运镜/视角）润色为可直接送视频生成模型的中文提示词，输出一行文本，不要 JSON。`

// LLMClient 拆分/润色所需的模型能力（由 ModelEngine 实现注入）。
type LLMClient interface {
	Chat(ctx context.Context, capability model.Capability, req *model.ChatRequest) (*model.ChatResponse, error)
}

// SplitStore 拆分结果写库边界（四层结构 + 项目状态转移，事务内）。
type SplitStore interface {
	// SaveSplitResult 事务写入：集/单元/块/镜头 + 项目状态 SCRIPT_SPLITTING→PARTIAL_ASSETS。
	// 返回写入统计。
	SaveSplitResult(ctx context.Context, projectID int64, result *SplitResult) (*SplitStats, error)
	// GetProjectInfo 项目归属（space_id/owner_id/script_text）。
	GetProjectInfo(ctx context.Context, projectID int64) (*ProjectInfo, error)
}

// ProjectInfo 项目最小信息。
type ProjectInfo struct {
	SpaceID   int64
	OwnerID   int64
	ScriptText string
}

// SplitStats 拆分写入统计。
type SplitStats struct {
	Episodes int `json:"episodes"`
	Beats    int `json:"beats"`
	Groups   int `json:"shot_groups"`
	Shots    int `json:"shots"`
}

// SplitHandler 实现 task.Handler（task_type=SCRIPT_SPLIT）。
type SplitHandler struct {
	llm    LLMClient
	store  SplitStore
	maxTry int // 坏 JSON 重试次数（含首次）
}

func NewSplitHandler(llm LLMClient, store SplitStore) *SplitHandler {
	return &SplitHandler{llm: llm, store: store, maxTry: 3}
}

// Name 任务类型（task_type_enum）。
func (h *SplitHandler) Name() string { return "SCRIPT_SPLIT" }

// Execute 执行拆分：读剧本 → LLM 拆分（坏 JSON 自动重试 ≤3）→ LLM 润色 → 四层入库 → 状态转移。
func (h *SplitHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ScriptText string `json:"script_text"`
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	if params.ScriptText == "" {
		// 参数为空时回退到项目剧本
		info, err := h.store.GetProjectInfo(ctx, derefProjectID(t))
		if err != nil {
			return nil, err
		}
		params.ScriptText = info.ScriptText
	}
	if params.ScriptText == "" {
		return nil, errors.New("剧本内容为空")
	}

	// ① 拆分 LLM（坏 JSON 自动重试 ≤3 次）
	var result *SplitResult
	var lastErr error
	for attempt := 1; attempt <= h.maxTry; attempt++ {
		resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
			Prompt:      params.ScriptText,
			System:      splitSystemPrompt,
			MaxTokens:   4096,
			Temperature: 0.3,
			JSONMode:    true,
		})
		if err != nil {
			lastErr = fmt.Errorf("拆分 LLM 调用失败(第%d次): %w", attempt, err)
			continue
		}
		result, lastErr = ParseAndValidate(resp.Content)
		if lastErr == nil {
			break
		}
		log.Printf("split: 第%d次输出非法 JSON/结构（%v），重试", attempt, lastErr)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("拆分失败（重试%d次）: %w", h.maxTry, lastErr)
	}

	// ② 润色 LLM（对每个镜头生成 prompt_refined；失败不阻塞整体，回退原文）
	h.polishAll(ctx, result)

	// ③ 四层入库 + 状态转移（事务）
	stats, err := h.store.SaveSplitResult(ctx, derefProjectID(t), result)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(stats)
	return out, nil
}

// polishAll 逐镜头润色（PolishAgent；单镜头失败不中断）。
func (h *SplitHandler) polishAll(ctx context.Context, r *SplitResult) {
	for ei := range r.Episodes {
		for bi := range r.Episodes[ei].NarrativeUnits {
			beat := &r.Episodes[ei].NarrativeUnits[bi]
			for gi := range beat.ShotGroups {
				sg := &beat.ShotGroups[gi]
				for si := range sg.Shots {
					s := &sg.Shots[si]
					prompt := fmt.Sprintf("景别%s，运镜%s，视角%s。主体：%s。背景：%s。光影：%s。台词：%s。环境音：%s。",
						s.ShotSize, s.CameraMovement, s.CameraAngle, s.SubjectDescription,
						s.BackgroundDescription, s.LocalLightDescription, s.DialogueText, s.AmbientSound)
					resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
						Prompt:      prompt,
						System:      polishSystemPrompt,
						MaxTokens:   512,
						Temperature: 0.5,
					})
					if err != nil {
						s.PromptRefined = prompt // 回退：用结构化描述作为 refined
						continue
					}
					s.PromptRefined = resp.Content
				}
			}
		}
	}
}

func derefProjectID(t *task.Task) int64 {
	if t.ProjectID == nil {
		return 0
	}
	return *t.ProjectID
}
