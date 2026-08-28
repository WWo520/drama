// SplitHandler：SCRIPT_SPLIT 任务执行器（双 LLM：拆分 + 润色；坏 JSON 自动重试）。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// outlineSystemPrompt 段A（结构拆分）：只切 集/叙事单元/逐字摘录 + 集级资产清单，
// 不含镜头——镜头由段B按集设计。单次输出体量 ≈ 剧本长度，恒定可控不被 max_tokens 截断。
// 规则与单次全量版完全一致（集数下限/单元粒度铁律/逐字摘录全覆盖/资产详尽描述）。
func outlineSystemPrompt(episodeDurationMin, minUnits, minEpisodes int) string {
	return fmt.Sprintf(`你是短剧分镜拆解专家。将用户提供的短剧剧本拆解为 JSON 结构：
{"episodes":[{"title":"第 N 集标题","narrative_units":[{"title":"叙事单元标题","duration_sec":秒数,"script_original_text":"该单元对应剧本原文"}]}],"characters":[{"code":"角色编码","name":"角色名","tags":"标签逗号分隔","description_notes":"【身份】+【年龄范围】+【性别】+【身材特征】+【面部特征】+【发型】+【妆容】+【服装材质】+【服装款式】+【配色】+【配饰】。必须包含具体细节，字数不低于 80 字。禁止空泛形容词，必须转化为具体视觉元素。使用列举式短语而非完整句子","origin_episode_id":1}],"scenes":[{"code":"场景编码","base_name":"场景名","base_prompt":"【空间结构】+【主要物体】+【材质细节】+【色彩搭配】+【光照条件】+【氛围描述】。必须包含环境光效、材质质感、色彩配置，字数不低于 100 字。禁止空泛形容词，必须包含具体视觉元素。使用列举式短语而非完整句子","origin_episode_id":1}],"props":[{"code":"道具编码","name":"道具名","category":"道具类别","description":"【物品名称】+【类别】+【尺寸】+【材质】+【颜色】+【工艺细节】+【功能属性】。必须包含外观特征、材质工艺，字数不低于 40 字。禁止空泛形容词，必须包含具体视觉元素。使用列举式短语而非完整句子","origin_episode_id":1}]}

【注意】本阶段不要输出任何 shot_groups / 镜头内容，只做结构切分与资产抽取；镜头组将由后续流程按集补齐。

【强制要求——提示词详细程度】
1. **角色描述 (description_notes)**: 必须包含以下内容
   - 身份背景：职业/社会地位/性格特点
   - 年龄范围：25-28 岁、30 岁左右等
   - 身材特征：身高、体型（如"身高 168cm，苗条身材"）
   - 面部特征：脸型、五官特点
   - 发型：发色、长度、样式；妆容：浓淡、风格
   - 服装材质与款式、配色、配饰：具体视觉元素
   - **字数要求**：≥80 个中文字符

2. **场景描述 (base_prompt)**: 空间结构/主要物体/材质细节/色彩搭配/光照条件/氛围描述，须包含环境光效与材质质感；**字数要求**：≥100 个中文字符

3. **道具描述 (description)**: 物品名称/类别/尺寸/材质成分/颜色/工艺细节/功能属性；**字数要求**：≥40 个中文字符

【其他规则（保持不变）】
1. 先分集再分单元：剧本总时长按总字数÷4（4 字/秒语速）估算，集数不得少于 %[3]d 集（总时长 ÷ 每集 %[1]d 分钟向上取整）——即使剧情连贯也必须按下刀点分集，每集为一个相对完整、结尾带悬念钩子的剧情段落，集标题格式为"第 N 集 - 本集看点"；仅当剧本总字数不足 240 字时才允许单集；
2. 叙事单元判据：叙事单元是一集之中按场景与画面连贯性切分出的最小拍摄单元——严禁把一整集剧情压缩成一个单元；切分下刀点优先选择：情绪转折点、说话人切换、动作段落结束、场景/时间切换；
3. 每集目标时长约 %[1]d 分钟：每集叙事单元数量不少于 %[2]d 个（按目标时长折算），通常 4~8 个；
4. 单元时长铁律：单个叙事单元 duration_sec 不得超过 30 秒；摘录字数÷4 ≈ duration_sec，摘录超过约 120 字的段落必然超 30 秒，必须继续切分为多个单元；
5. script_original_text 必须从剧本原文中逐字摘录该单元对应的真实原文片段，不得改写、缩写或省略为空；全部叙事单元的摘录按剧情顺序拼接后必须完整覆盖剧本原文，不得遗漏任何段落；
6. 角色≤6 人、场景≤8 个；scenes 每项 code/base_name、props 每项 code/name 均不得为空；scenes 的 base_name 必须为 2~6 字场景简称（如"公寓"、"办公室"、"古玩街"，须与剧本中的地点称呼一致），禁止写成短语或长句描述（详细环境描述只写进 base_prompt）；
7. 角色/场景/道具按集抽取：每项必须携带 origin_episode_id（首次出现集数的 1-based 序号），不区分叙事单元；category 仅限 WEAPON/CLOTHING_ACCESSORY/FOOD_DRINK/STATIONERY_BOOK/JEWELRY_ORNAMENT/FURNITURE_DECOR/ELECTRONIC_APPLIANCE/VEHICLE/TOOL_MUSICAL_INSTRUMENT/TOY/PLANT_FLOWER/ANIMAL_PET/LIGHTING_SOURCE/DOCUMENT_SCROLL_MAP/FLAG_BANNER/OTHER；
8. 只输出合法 JSON，不要任何解释文字。`, episodeDurationMin, minUnits, minEpisodes)
}

// shotDesignSystemPrompt 段B（镜头设计）：为单集叙事单元设计 shot_groups。
func shotDesignSystemPrompt() string {
	return fmt.Sprintf(`你是短剧分镜专家。为给定一集的每个叙事单元设计镜头块（shot_groups），只输出合法 JSON：
{"units":[{"index":单元序号,"shot_groups":[{"title":"镜头块标题","duration_sec":秒,"shots":[{"duration_sec":秒,"shot_size":"景别枚举","camera_movement":"运镜枚举","camera_angle":"视角枚举","subject_description":"主体描述","background_description":"背景描述","local_light_description":"光影描述","dialogue_text":"台词","ambient_sound":"环境音"}]}]}]}

【规则】
1. 每个叙事单元 1~3 个镜头块、每块 2~4 个镜头；块与镜头 duration_sec 必须为正整数；
2. 块时长加总必须恰好等于所属单元的 duration_sec；
3. %s；
4. subject_description 40~120 字：人物肢体动作/表情微表情/服饰外观/与他人或道具的空间关系；background_description 写环境层次（近景陈设/中景动线/远景氛围）；local_light_description 写光源方向、明暗对比与面部落点；台词照抄摘录原文并以"角色名："开头（无台词时以纯画面动作撑满时长）；ambient_sound 写短文本（如"键盘敲击声渐弱"）；
5. 镜头顺序遵循摘录的动作时间线；units 数组必须覆盖输入的每一个叙事单元（index 从 1 开始对应），不得遗漏；
6. 不要任何解释文字，不要 markdown。`, enumRuleSentence())
}

// enumRuleSentence 枚举约束句（与 splitter.go 合法值集合同源生成，避免双份维护漂移）。
func enumRuleSentence() string {
	return fmt.Sprintf("shot_size 仅限 %s；camera_movement 仅限 %s；camera_angle 仅限 %s",
		enumList(validShotSizes), enumList(validCameraMoves), enumList(validCameraAngles))
}

func enumList(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "/")
}

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
	SpaceID    int64
	OwnerID    int64
	ScriptText string
}

// SplitStats 拆分写入统计（含集级资产）。
type SplitStats struct {
	Episodes       int `json:"episodes"`
	Beats          int `json:"beats"`
	Groups         int `json:"shot_groups"`
	Shots          int `json:"shots"`
	Characters     int `json:"characters"`
	Scenes         int `json:"scenes"`
	Props          int `json:"props"`
	AssetUnitLinks int `json:"asset_unit_links"` // U5：单元级资产建链数（nu_asset_links）
	AssetShotLinks int `json:"asset_shot_links"` // U5：镜头级资产建链数（shot_asset_refs + linked_assets_json）
}

// SplitHandler 实现 task.Handler（task_type=SCRIPT_SPLIT）。
type SplitHandler struct {
	llm    LLMClient
	store  SplitStore
	maxTry int // 坏 JSON 重试次数（含首次）
	// progress 可选进度上报（Execute 注入的闭包；worker main 里写 DB + WS 桥）。
	// 阶段化真实文案取代前端假动画轮换（用户实测反馈"等待 27 分钟像卡死"）。
	progress func(pct int, msg string)
}

func NewSplitHandler(llm LLMClient, store SplitStore) *SplitHandler {
	// maxTry=4：校验维度多（JSON/结构/集数/摘录体量/覆盖率），纠正提示累积后模型常需多轮收敛
	return &SplitHandler{llm: llm, store: store, maxTry: 4}
}

// SetProgressReporter 注入拆分进度上报（nil 不上报）。
func (h *SplitHandler) SetProgressReporter(fn func(pct int, msg string)) { h.progress = fn }

// report 进度容错上报。
func (h *SplitHandler) report(pct int, msg string) {
	if h.progress != nil {
		h.progress(pct, msg)
	}
}

// Name 任务类型（task_type_enum）。
func (h *SplitHandler) Name() string { return "SCRIPT_SPLIT" }

// Execute 执行拆分：读剧本 → LLM 拆分（坏 JSON 自动重试 maxTry=4 含首次）→ LLM 润色 → 四层入库 → 状态转移。
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
	outlinePrompt := outlineSystemPrompt(params.EpisodeDurationMin, minUnits, minEpisodes)
	// 拆分技能风格段（全局 agent_skills 库，随任务参数透传；与基础规则冲突时以技能要求为准）
	if params.SplitSkillPrompt != "" {
		outlinePrompt += "\n拆分风格技能（与上述规则冲突时以技能要求为准）：" + params.SplitSkillPrompt
	}

	// ① 拆分 LLM（两段式：先结构后镜头——单次输出全片分镜 JSON 在长剧本下必然超出
	// max_tokens 被截断（unexpected end of JSON input），6445 字剧本实测 4 次重试全部截断）。
	//   - 段A 结构拆分：集/叙事单元/逐字摘录/资产清单，单次输出体量 ≈ 剧本长度，恒定可控；
	//   - 段B 镜头设计：按集逐次小请求补 shot_groups（每集 ≤8 单元 × ≤3 块 × ≤4 镜头），
	//     单次输出上限不随总剧本体量增长。
	//
	// 坏 JSON/摘录不全自动重试 maxTry=4 含首次；纠正提示累积避免振荡。
	var result *SplitResult
	var lastErr error
	extraHint := ""
	h.report(8, "AI 正在通读剧本、划分剧集与叙事单元…")
	for attempt := 1; attempt <= h.maxTry; attempt++ {
		resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
			Prompt:      params.ScriptText,
			System:      outlineSystemPrompt(params.EpisodeDurationMin, minUnits, minEpisodes) + extraHint,
			MaxTokens:   16384,
			Temperature: 0.3,
			JSONMode:    true,
			// deepseek-v4-* 默认开推理：推理烧光 token 预算导致截断/空内容，显式关闭。
			NoThinking: true,
		})
		if err != nil {
			lastErr = fmt.Errorf("拆分 LLM 调用失败(第%d次): %w", attempt, err)
			log.Printf("split: 第%d次 LLM 调用失败: %v", attempt, err)
			continue
		}
		result, lastErr = parseAndValidateOutline(resp.Content)
		if lastErr != nil && isTruncatedJSON(lastErr) {
			// JSON 截断兜底（长剧本输出顶到 MaxTokens）：先尝试确定性补尾修复，
			// 不行则下一轮携带"精简输出"纠正提示降档。
			if repaired, rerr := repairTruncatedJSON(resp.Content); rerr == nil {
				if rep2, perr := parseAndValidateOutline(repaired); perr == nil {
					result = rep2
					lastErr = nil
				}
			}
		}
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
		_ = h.store.MarkSplitFailed(ctx, derefProjectID(t))
		return nil, fmt.Errorf("拆分失败（重试%d次）: %w", h.maxTry, lastErr)
	}

	// ①.5 确定性后处理：超长单元切分 + 分集均衡 + 单元时长按摘录字数归一
	result = SplitOversizedUnits(result)
	result = NormalizeEpisodeCount(result, minEpisodes)
	normalizeUnitDurations(result)

	// ①.7 段B：逐集镜头设计（每集独立小请求；单集失败整体失败回滚状态）
	totalEps := len(result.Episodes)
	for ei := range result.Episodes {
		h.report(15+ei*55/max(totalEps, 1),
			fmt.Sprintf("正在设计第 %d/%d 集的分镜镜头…", ei+1, totalEps))
		if err := h.designEpisodeShots(ctx, ei, &result.Episodes[ei]); err != nil {
			_ = h.store.MarkSplitFailed(ctx, derefProjectID(t))
			return nil, err
		}
	}

	h.report(72, "AI 正在扩写资产视觉描述…")
	// ②.5 AI 优化资产提示词（核心功能：将简要描述扩展为 LuxReal 风格的详细视觉提示词）
	// 注意：不阻塞主流程，失败也不影响拆分结果写入
	h.optimizeAllAssets(ctx, result)

	h.report(78, "AI 正在润色分镜提示词…")
	// ③ 润色 LLM（对每个镜头生成 prompt_refined；失败不阻塞整体，回退原文）
	h.polishAll(ctx, result)

	h.report(96, "分镜正在入库…")
	// ③ 四层入库 + 状态转移（事务）
	stats, err := h.store.SaveSplitResult(ctx, derefProjectID(t), result)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(stats)
	return out, nil
}

// polishConcurrency 润色并发数（提速核心：长剧本数百镜头此前串行跑 15+ 分钟；
// LLM 客户端线程安全，4 并发把耗时压到 ~1/4 且不会触发厂商限流）。
const polishConcurrency = 4

// polishAll 并发按卡聚合润色（提示词优化方案A）：整卡镜头一次 LLM 调用统一润色，
// 携带前后相邻卡的末/首镜头摘要作为承接上下文，每镜头生成「开场承接+连续动作+
// 收尾定格」与【转场】标注。卡级调用失败时该卡全部镜头回退 luxShotPrompt，不中断整体。
func (h *SplitHandler) polishAll(ctx context.Context, r *SplitResult) {
	// 全项目镜头卡保序收集：跨单元/集的相邻关系即叙事时间线顺序
	var groups []*ShotGroup
	for ei := range r.Episodes {
		for bi := range r.Episodes[ei].NarrativeUnits {
			for gi := range r.Episodes[ei].NarrativeUnits[bi].ShotGroups {
				groups = append(groups, &r.Episodes[ei].NarrativeUnits[bi].ShotGroups[gi])
			}
		}
	}
	if len(groups) == 0 {
		return
	}
	done := make(chan struct{}, len(groups))
	sem := make(chan struct{}, polishConcurrency)
	total := len(groups)
	var doneCount int32
	for gi := 0; gi < len(groups); gi++ {
		g := groups[gi]
		var prevBrief, nextBrief string
		if gi > 0 && len(groups[gi-1].Shots) > 0 {
			prevBrief = neighborBrief(&groups[gi-1].Shots[len(groups[gi-1].Shots)-1])
		}
		if gi+1 < len(groups) && len(groups[gi+1].Shots) > 0 {
			nextBrief = neighborBrief(&groups[gi+1].Shots[0])
		}
		// 主流程先持有令牌再启动（并发闸门；goroutine 内 defer 释放）
		sem <- struct{}{}
		go func(g *ShotGroup, pb, nb string) {
			defer func() { <-sem; done <- struct{}{} }()
			if !PolishShotGroupAsCard(ctx, h.llm, g, pb, nb) {
				for si := range g.Shots { // 整卡回退：确定性 LuxReal 结构化格式
					g.Shots[si].PromptRefined = luxShotPrompt(&g.Shots[si])
				}
			}
			n := atomic.AddInt32(&doneCount, 1)
			// 阶段内进度：78 → 94 随润色完成度推进（每 ~5% 上报一次防消息风暴）
			ni := int(n)
			if ni%max(total/8, 1) == 0 || ni == total {
				h.report(78+16*ni/total, fmt.Sprintf("正在润色分镜提示词…(%d/%d 卡)", ni, total))
			}
		}(g, prevBrief, nextBrief)
	}
	for range groups {
		<-done
	}
}

// designEpisodeShots 段B：为单集全部叙事单元设计 shot_groups（独立小请求，maxTry 次重试）。
// 成功时块时长已确定性均衡到单元时长；失败返回错误由上层回滚项目状态。
func (h *SplitHandler) designEpisodeShots(ctx context.Context, epIndex int, ep *EpisodeSplit) error {
	if len(ep.NarrativeUnits) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "本集标题：%s\n叙事单元共 %d 个（index 从 1 开始）：\n", ep.Title, len(ep.NarrativeUnits))
	for i, u := range ep.NarrativeUnits {
		fmt.Fprintf(&b, "[单元%d] 标题：%s｜时长：%d 秒｜原文摘录：%s\n",
			i+1, u.Title, int(u.DurationSec), u.ScriptOriginalText)
	}
	prompt := b.String()
	system := shotDesignSystemPrompt()
	var lastErr error
	for attempt := 1; attempt <= h.maxTry; attempt++ {
		resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
			Prompt:      prompt,
			System:      system,
			MaxTokens:   16384, // 单集体量上限；段B实际输出远小于该值
			Temperature: 0.3,
			JSONMode:    true,
			NoThinking:  true,
		})
		if err != nil {
			lastErr = fmt.Errorf("第%d集镜头设计 LLM 调用失败(第%d次): %w", epIndex+1, attempt, err)
			log.Printf("split: 第%d集段B第%d次调用失败: %v", epIndex+1, attempt, err)
			continue
		}
		out, perr := parseShotDesign(resp.Content)
		if perr == nil {
			if aerr := applyShotDesign(ep, out); aerr == nil {
				return nil
			} else {
				lastErr = fmt.Errorf("第%d集镜头设计校验失败(第%d次): %w", epIndex+1, attempt, aerr)
				log.Printf("split: 第%d集段B非法结构（%v），重试", epIndex+1, aerr)
				continue
			}
		}
		lastErr = fmt.Errorf("第%d集镜头设计解析失败(第%d次): %w", epIndex+1, attempt, perr)
		log.Printf("split: 第%d集段B输出非法 JSON/结构（%v），重试", epIndex+1, perr)
	}
	return lastErr
}

// optimizeAllAssets AI 优化所有资产提示词（角色/场景/道具；并发执行，单个失败不影响其他）。
func (h *SplitHandler) optimizeAllAssets(ctx context.Context, r *SplitResult) {
	log.Printf("[AssetOptimization] 开始优化资产...")

	type assetJob struct {
		kind, name string
		apply      func(string)
	}
	var jobs []assetJob
	for ci := range r.Characters {
		char := &r.Characters[ci]
		if char.DescriptionNotes == "" || len([]rune(char.DescriptionNotes)) < 50 {
			c := char
			jobs = append(jobs, assetJob{"character", c.Name, func(opt string) { c.DescriptionNotes = opt }})
		}
	}
	for si := range r.Scenes {
		scene := &r.Scenes[si]
		if scene.BasePrompt == "" || len([]rune(scene.BasePrompt)) < 60 {
			s := scene
			jobs = append(jobs, assetJob{"scene", s.BaseName, func(opt string) { s.BasePrompt = opt }})
		}
	}
	for pi := range r.Props {
		prop := &r.Props[pi]
		if prop.Description == "" || len([]rune(prop.Description)) < 30 {
			p := prop
			jobs = append(jobs, assetJob{"prop", p.Name, func(opt string) { p.Description = opt }})
		}
	}
	if len(jobs) == 0 {
		log.Printf("[AssetOptimization] 无需优化")
		return
	}
	sem := make(chan struct{}, polishConcurrency)
	done := make(chan struct{}, len(jobs))
	var doneCount int32
	total := len(jobs)
	for _, j := range jobs {
		sem <- struct{}{} // 主流程先持令牌再启动（并发闸门）
		go func(kind, name string, apply func(string)) {
			defer func() { <-sem; done <- struct{}{} }()
			brief := ""
			switch kind {
			case "character":
				for _, c := range r.Characters {
					if c.Name == name {
						brief = c.DescriptionNotes
						break
					}
				}
			case "scene":
				for _, s := range r.Scenes {
					if s.BaseName == name {
						brief = s.BasePrompt
						break
					}
				}
			default:
				for _, p := range r.Props {
					if p.Name == name {
						brief = p.Description
						break
					}
				}
			}
			optimized, err := h.expandAssetPrompt(ctx, kind, name, brief)
			if err != nil {
				log.Printf("[AssetOptimization] %s(%s)优化失败：%v，使用原描述", kind, name, err)
			} else if optimized != "" && optimized != brief {
				apply(optimized)
			}
			n := atomic.AddInt32(&doneCount, 1)
			ni := int(n)
			if ni%max(total/4, 1) == 0 || ni == total {
				h.report(72+5*ni/total, fmt.Sprintf("正在扩写资产视觉描述…(%d/%d)", ni, total))
			}
		}(j.kind, j.name, j.apply)
	}
	for range jobs {
		<-done
	}

	log.Printf("[AssetOptimization] 资产优化完成")
}

// expandAssetPrompt 单个资产提示词扩展 (LLM 重写为详细视觉描述)。
func (h *SplitHandler) expandAssetPrompt(ctx context.Context, assetType, name, brief string) (string, error) {
	var systemPrompt string
	switch assetType {
	case "character":
		systemPrompt = `你是 AI 绘画提示词专家。请将以下简短的角色描述重写为适合图像生成模型的详细中文提示词。

【必须包含】
- 身份背景、年龄范围、身材特征、面部特征
- 发型、妆容
- 服装材质、款式、配色、配饰
【格式】使用列举式短语，以分号分隔，总字数≥100 字

注意：不要 JSON、Markdown 或任何解释，只返回优化后的提示词本身。`
	case "scene":
		systemPrompt = `你是 AI 绘画提示词专家。请将以下简短的场景描述重写为适合图像生成模型的详细中文提示词。

【必须包含】
- 空间结构、主要物体、材质细节
- 色彩搭配、光照条件、氛围描述
【格式】使用列举式短语，以分号分隔，总字数≥120 字

注意：不要 JSON、Markdown 或任何解释，只返回优化后的提示词本身。`
	default: // prop
		systemPrompt = `你是 AI 绘画提示词专家。请将以下简短的道具描述重写为适合图像生成模型的详细中文提示词。

【必须包含】
- 物品名称、类别、尺寸
- 材质成分、颜色、工艺细节
- 功能属性
【格式】使用列举式短语，以分号分隔，总字数≥50 字

注意：不要 JSON、Markdown 或任何解释，只返回优化后的提示词本身。`
	}

	resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
		Prompt:      name + ": " + brief,
		System:      systemPrompt,
		MaxTokens:   1024,
		Temperature: 0.6,
		NoThinking:  true, // 推理模式会烧光 1024 预算致 content 空，扩写无需推理
	})
	if err != nil {
		// LLM 调用失败：静默返回原始简要描述
		return brief, nil
	}
	if resp.Content == "" {
		// 响应为空：静默返回原始简要描述
		return brief, nil
	}
	return resp.Content, nil
}

func derefProjectID(t *task.Task) int64 {
	if t.ProjectID == nil {
		return 0
	}
	return *t.ProjectID
}

// isTruncatedJSON 判定错误是否为 JSON 截断（输出顶到 MaxTokens：unexpected end of JSON）。
func isTruncatedJSON(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unexpected end of JSON") || strings.Contains(msg, "unexpected EOF")
}

// repairTruncatedJSON 确定性修复被截断的 JSON 数组：
// 原则——丢弃最后一个不完整对象，回退闭合已完成的全部元素（丢尾不丢主体，
// 保住拆分主结构后覆盖率校验兜底；远好于整轮作废重跑 5 分钟）。
// 处理路径：定位到首个顶层 '[' → 逐字符扫描字符串/转义态 → 在数组上下文中
// 尝试逐个补齐 '}]' 闭合；成功闭合且 json.Valid 即返回。
func repairTruncatedJSON(raw string) (string, error) {
	s := extractJSON(raw)
	start := strings.IndexByte(s, '[')
	if start < 0 {
		return "", errors.New("no array")
	}
	// 从最深处往后找最后一个"对象级"安全点：逐步截短并尝试闭合
	for cut := len(s); cut > start+1; cut-- {
		// 快速剪枝：trial 结尾处于字符串内部时跳过（引号计数为奇）
		inStr, esc := false, false
		lastBrace := -1 // 最后一个 '}' 的位置（串外）
		for i := start; i < cut; i++ {
			c := s[i]
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				if inStr {
					esc = true
				}
			case '"':
				inStr = !inStr
				if !inStr && i > lastBrace {
					// 字符串刚结束，无操作（lastBrace 只在串外更新）
				}
			case '}':
				if !inStr {
					lastBrace = i
				}
			}
		}
		if inStr || esc {
			continue // 截断点在字符串中间，跳过
		}
		if lastBrace <= start {
			continue // 没有完整对象可保留
		}
		// 以最后完整对象为终点，尝试两种闭合（对象在对象数组内 / 对象在数组的数组内）
		head := s[:lastBrace+1]
		for _, candidate := range []string{head + "]", head + "}]"} {
			if json.Valid([]byte(candidate)) {
				return candidate, nil
			}
			// 多层嵌套（episodes→narrative_units 链上每层都是数组）：从内向外补 N 层
			padded := head
			for depth := 0; depth < 4; depth++ {
				padded += "}"
				if json.Valid([]byte(padded + "]")) {
					return padded + "]", nil
				}
			}
		}
	}
	return "", errors.New("repair failed")
}
