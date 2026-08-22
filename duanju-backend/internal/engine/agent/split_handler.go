// SplitHandler：SCRIPT_SPLIT 任务执行器（双 LLM：拆分 + 润色；坏 JSON 自动重试）。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// 拆分技能 System Prompt（J.2 技能正文规则：角色≤6、场景≤8、景别/运镜/视角枚举、严格 JSON）。
// 资产粒度：角色/场景/道具均按集抽取（origin_episode_id 指向 1-based 集序号），不按叙事单元拆分。
// 剧集拆分规则：集数由 %[3]d 下限硬约束（剧本总字数÷4字/秒 ÷ 每集目标时长），短剧本才允许单集。
// 粒度铁律（对齐 LuxReal 判据 + ToonFlow 估时）：单元=按场景/画面连贯性切分的最小拍摄单元，
// 单单元 ≤30s、台词按 4字/秒估时，单元数由 %[2]d 下限约束。
const splitSystemPrompt = `你是短剧分镜拆解专家。将用户提供的短剧剧本拆解为 JSON 结构：
{"episodes":[{"title":"第1集标题","narrative_units":[{"title":"叙事单元标题","duration_sec":秒数,"script_original_text":"该单元对应剧本原文","shot_groups":[{"title":"镜头块标题","duration_sec":秒数,"shots":[{"duration_sec":秒,"shot_size":"景别","camera_movement":"运镜","camera_angle":"视角","subject_description":"主体描述","background_description":"背景描述","local_light_description":"光影描述","dialogue_text":"台词","ambient_sound":"环境音"}]}]}]}],"characters":[{"code":"角色编码","name":"角色名","tags":"标签逗号分隔","description_notes":"角色描述","origin_episode_id":1}],"scenes":[{"code":"场景编码","base_name":"场景名","base_prompt":"场景描述","origin_episode_id":1}],"props":[{"code":"道具编码","name":"道具名","category":"道具类别","description":"道具描述","origin_episode_id":1}]}
规则：
1. 先分集再分单元：剧本总时长按总字数÷4（4字/秒语速）估算，集数不得少于 %[3]d 集（总时长 ÷ 每集 %[1]d 分钟向上取整）——即使剧情连贯也必须按下刀点分集，每集为一个相对完整、结尾带悬念钩子的剧情段落，集标题格式为“第N集 - 本集看点”；仅当剧本总字数不足 240 字时才允许单集；
2. 叙事单元判据：叙事单元是一集之中按场景与画面连贯性切分出的最小拍摄单元（Beat），它同时是后续资产关联、视频提示词撰写与视频生成的统筹单位——严禁把一整集剧情压缩成一个单元；切分下刀点优先选择：情绪转折点、说话人切换、动作段落结束、场景/时间切换；
3. 每集目标时长约 %[1]d 分钟：叙事单元 duration_sec 加总应接近该目标，时长合计需覆盖该集全部情节；每集叙事单元数量不少于 %[2]d 个（按目标时长折算），通常 4~8 个；
4. 单元时长铁律：单个叙事单元 duration_sec 不得超过 30 秒；单元时长必须与其摘录体量匹配——摘录字数÷4（按 4字/秒语速）≈ duration_sec，不得明显低估；摘录超过约 120 字的段落必然超 30 秒，必须在下刀点继续切分为多个单元；
5. 每个叙事单元 1~3 个镜头块，每个镜头块 2~4 个镜头；镜头块 duration_sec 加总应等于所属单元 duration_sec；
6. script_original_text 必须从剧本原文中逐字摘录该单元对应的真实原文片段，不得改写、缩写或省略为空；全部叙事单元的摘录按剧情顺序拼接后必须完整覆盖剧本原文，不得遗漏任何段落；
7. shot_size 仅限 ECU/CU/MCU/MS/FS/LS/ELS/TWO_SHOT/OTS；
8. camera_movement 仅限 STATIC/SLOW_PUSH_IN/SLOW_PULL_OUT/FAST_PUSH_IN/DOLLY_IN/DOLLY_OUT/PAN_LEFT/PAN_RIGHT/TRACK_LEFT/TRACK_RIGHT/TILT_UP/TILT_DOWN/CRANE_UP/CRANE_DOWN/HANDHELD_SHAKY/WIPE_CLIP/ZOOM_IN_OPTICAL/SLOW_360_ORBIT；
9. camera_angle 仅限 EYE_LEVEL_FRONT/SIDE/BACK/SLIGHT_LOW_ANGLE/LOW_ANGLE/SLIGHT_HIGH/HIGH_BIRDVIEW/OTS_LEFT/OTS_RIGHT/DUTCH_ANGLE；
10. 角色≤6人、场景≤8个；scenes 每项 code/base_name、props 每项 code/name 均不得为空；
11. 角色/场景/道具按集抽取：每项必须携带 origin_episode_id（首次出现集数的 1-based 序号），不区分叙事单元；category 仅限 WEAPON/CLOTHING_ACCESSORY/FOOD_DRINK/STATIONERY_BOOK/JEWELRY_ORNAMENT/FURNITURE_DECOR/ELECTRONIC_APPLIANCE/VEHICLE/TOOL_MUSICAL_INSTRUMENT/TOY/PLANT_FLOWER/ANIMAL_PET/LIGHTING_SOURCE/DOCUMENT_SCROLL_MAP/FLAG_BANNER/OTHER；
12. 只输出合法 JSON，不要任何解释文字。`

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
	// MarkSplitFailed 拆分失败回滚：项目 SCRIPT_SPLITTING → SCRIPT_SPLIT_FAILED（可重新拆分）。
	MarkSplitFailed(ctx context.Context, projectID int64) error
}

// ProjectInfo 项目最小信息。
type ProjectInfo struct {
	SpaceID   int64
	OwnerID   int64
	ScriptText string
}

// SplitStats 拆分写入统计（含集级资产）。
type SplitStats struct {
	Episodes   int `json:"episodes"`
	Beats      int `json:"beats"`
	Groups     int `json:"shot_groups"`
	Shots      int `json:"shots"`
	Characters int `json:"characters"`
	Scenes     int `json:"scenes"`
	Props      int `json:"props"`
}

// SplitHandler 实现 task.Handler（task_type=SCRIPT_SPLIT）。
type SplitHandler struct {
	llm    LLMClient
	store  SplitStore
	maxTry int // 坏 JSON 重试次数（含首次）
}

func NewSplitHandler(llm LLMClient, store SplitStore) *SplitHandler {
	// maxTry=4：校验维度多（JSON/结构/集数/摘录体量/覆盖率），纠正提示累积后模型常需多轮收敛
	return &SplitHandler{llm: llm, store: store, maxTry: 4}
}

// Name 任务类型（task_type_enum）。
func (h *SplitHandler) Name() string { return "SCRIPT_SPLIT" }

// Execute 执行拆分：读剧本 → LLM 拆分（坏 JSON 自动重试 ≤3）→ LLM 润色 → 四层入库 → 状态转移。
func (h *SplitHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ScriptText         string `json:"script_text"`
		EpisodeDurationMin int    `json:"episode_duration_min"` // 单集目标时长（分钟；0=默认 2 分钟）
		SplitSkillPrompt   string `json:"split_skill_prompt"`   // 拆分技能风格提示词（agent_skills.template_content；空=默认标准拆分）
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
	if params.EpisodeDurationMin <= 0 || params.EpisodeDurationMin > 10 {
		params.EpisodeDurationMin = 2
	}
	// 单元数下限：ceil(集时长秒数/25)，即默认 2 分钟 → 5 个（对齐 LuxReal 实测 4~6 单元/集）
	minUnits := (params.EpisodeDurationMin*60 + 24) / 25
	// 集数下限：剧本正文字数÷4（4字/秒）÷ 每集目标秒数，向上取整（防止模型偷懒全部塞进单集）
	scriptRunes := len([]rune(strings.Map(func(rt rune) rune {
		if unicode.IsSpace(rt) {
			return -1
		}
		return rt
	}, params.ScriptText)))
	epSec := params.EpisodeDurationMin * 60
	minEpisodes := (scriptRunes/4 + epSec - 1) / epSec
	// 结构性上限：每集至少 minUnits 个单元、每单元摘录≤120字，倒推每集最少需 minUnits*100 字；
	// 超过该集数的要求与单元粒度要求矛盾，会导致模型在重试中振荡
	if maxByContent := scriptRunes / (minUnits * 100); minEpisodes > maxByContent {
		minEpisodes = maxByContent
	}
	if minEpisodes < 1 {
		minEpisodes = 1
	}
	systemPrompt := fmt.Sprintf(splitSystemPrompt, params.EpisodeDurationMin, minUnits, minEpisodes)
	// 拆分技能风格段（全局 agent_skills 库，随任务参数透传；与基础规则冲突时以技能要求为准）
	if params.SplitSkillPrompt != "" {
		systemPrompt += "\n拆分风格技能（与上述规则冲突时以技能要求为准）：" + params.SplitSkillPrompt
	}

	// ① 拆分 LLM（坏 JSON/集数不足/摘录超长/摘录不完整 自动重试 ≤3；纠正提示累积避免振荡）
	var result *SplitResult
	var lastErr error
	extraHint := ""
	for attempt := 1; attempt <= h.maxTry; attempt++ {
		resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
			Prompt:      params.ScriptText,
			System:      systemPrompt + extraHint,
			MaxTokens:   16384, // 多集结构+逐字摘录体量大，8192 会截断 JSON（unexpected end of JSON input）
			Temperature: 0.3,
			JSONMode:    true,
			NoThinking:  true, // deepseek-v4-* 默认思考模式，拆分不需要推理过程
		})
		if err != nil {
			lastErr = fmt.Errorf("拆分 LLM 调用失败(第%d次): %w", attempt, err)
			continue
		}
		result, lastErr = ParseAndValidate(resp.Content)
		if lastErr == nil {
			// 原文摘录覆盖率校验：防止模型缩写/遗漏 script_original_text（静默丢原文）
			if cerr := ValidateExcerptCoverage(result, params.ScriptText); cerr != nil {
				lastErr = cerr
				extraHint += "\n重要纠正：上一次输出中叙事单元的 script_original_text 摘录不完整。请逐字摘录剧本原文，确保所有单元摘录拼接后完整覆盖剧本全文，不得缩写或遗漏。"
				log.Printf("split: 第%d次原文摘录不完整（%v），重试", attempt, cerr)
				continue
			}
			break
		}
		log.Printf("split: 第%d次输出非法 JSON/结构（%v），重试", attempt, lastErr)
	}
	if lastErr != nil {
		// 失败回滚项目状态（可重新拆分），不阻塞任务失败上报
		_ = h.store.MarkSplitFailed(ctx, derefProjectID(t))
		return nil, fmt.Errorf("拆分失败（重试%d次）: %w", h.maxTry, lastErr)
	}

	// ①.5 确定性后处理：模型对分集/超长段落纠正无效时，按单元/句子边界确定性切分
	result = SplitOversizedUnits(result)
	result = NormalizeEpisodeCount(result, minEpisodes)

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
