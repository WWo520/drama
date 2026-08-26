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
{"episodes":[{"title":"第 1 集标题","narrative_units":[{"title":"叙事单元标题","duration_sec":秒数,"script_original_text":"该单元对应剧本原文","shot_groups":[{"title":"镜头块标题","duration_sec":秒数,"shots":[{"duration_sec":秒,"shot_size":"景别","camera_movement":"运镜","camera_angle":"视角","subject_description":"主体描述","background_description":"背景描述","local_light_description":"光影描述","dialogue_text":"台词","ambient_sound":"环境音"}]}]}]},"characters":[{"code":"角色编码","name":"角色名","tags":"标签逗号分隔","description_notes":"【身份】+【年龄范围】+【性别】+【身材特征】+【面部特征】+【发型】+【妆容】+【服装材质】+【服装款式】+【配色】+【配饰】。必须包含具体细节，字数不低于 80 字。禁止空泛形容词（如'漂亮'、'豪华'），必须转化为具体视觉元素。使用列举式短语而非完整句子","origin_episode_id":1}],"scenes":[{"code":"场景编码","base_name":"场景名","base_prompt":"【空间结构】+【主要物体】+【材质细节】+【色彩搭配】+【光照条件】+【氛围描述】。必须包含环境光效、材质质感、色彩配置，字数不低于 100 字。禁止空泛形容词，必须包含具体视觉元素。使用列举式短语而非完整句子","origin_episode_id":1}],"props":[{"code":"道具编码","name":"道具名","category":"道具类别","description":"【物品名称】+【类别】+【尺寸】+【材质】+【颜色】+【工艺细节】+【功能属性】。必须包含外观特征、材质工艺，字数不低于 40 字。禁止空泛形容词，必须包含具体视觉元素。使用列举式短语而非完整句子","origin_episode_id":1}]}

【强制要求——提示词详细程度】
1. **角色描述 (description_notes)**: 必须包含以下内容
   - 身份背景：职业/社会地位/性格特点（如"职场精英"、"富家千金"）
   - 年龄范围：25-28 岁、30 岁左右等
   - 身材特征：身高、体型（如"身高 168cm，苗条身材"、"高大健硕"）
   - 面部特征：脸型、五官特点（如"瓜子脸，五官立体"、"圆脸，大眼睛"）
   - 发型：发色、长度、样式（如"黑色长直发及肩"、"棕色卷发盘起"）
   - 妆容：浓淡、风格（如"淡妆"、"浓艳红唇"）
   - 服装材质：真丝/棉麻/羊毛/皮革/丝绸等（如"红色真丝连衣裙"）
   - 服装款式：V 领/A 字裙/修身西装等（如"V 领设计，腰部收腰"）
   - 配色：主色 + 辅色（如"深蓝色外套，白色内搭"）
   - 配饰：鞋履/首饰/包袋等（如"金色细带高跟鞋，珍珠耳环"）
   - **字数要求**：≥80 个中文字符

2. **场景描述 (base_prompt)**: 必须包含以下内容
   - 空间结构：房间大小/挑高/布局（如"挑高 8 米的大厅"、"紧凑的卧室"）
   - 主要物体：关键家具/装饰/设备（如"施华洛世奇水晶吊灯"、"大理石地面"）
   - 材质细节：墙面/地面/天花的材质（如"意大利进口米白色大理石"）
   - 色彩搭配：主色调 + 辅助色（如"暖金色与乳白色为主基调"）
   - 光照条件：光源类型/方向/强度（如"水晶吊灯光芒流泻"、"自然光从窗外射入"）
   - 氛围描述：整体感觉（如"高级奢华"、"温馨舒适"）
   - **字数要求**：≥100 个中文字符

3. **道具描述 (description)**: 必须包含以下内容
   - 物品名称：具体型号/品牌（如"iPhone 15 Pro Max"）
   - 类别：电子设备/服饰/家具等
   - 尺寸：长×宽×高或直径/厚度（如"6.7 英寸"、"厚 7.25mm"）
   - 材质成分：外壳/屏幕/按键材质（如"钛金属边框，玻璃面板"）
   - 颜色：主色 + 表面处理（如"哑光黑色"）
   - 工艺细节：表面纹理/接口/按钮（如"三摄像头模组垂直排列"）
   - 功能属性：用途/特色功能（如"USB-C 接口"）
   - **字数要求**：≥40 个中文字符

【输出格式示例】
❌ 错误示范（过于简略）：
{"description_notes": "年轻女性，红色裙子"}
{"base_prompt": "豪华宴会厅"}
{"description": "智能手机"}

✅ 正确示范（符合要求）：
{"description_notes": "中国现代都市女性，25-28 岁，职场精英，身高 168cm，苗条身材；黑色直发及肩，齐刘海，淡妆；深蓝色羊毛 V 领修身西装套装，银色金属纽扣，内搭白色真丝衬衫；脚踩黑色真皮高跟鞋；办公室环境，书架电脑；正面视角，电影质感"}
{"base_prompt": "高档宴会厅，挑高 8 米，穹顶悬挂施华洛世奇水晶吊灯群，折射暖黄色光晕；地面铺设意大利进口米白色大理石，倒映金碧辉煌；两侧香槟金雕花扶梯通至二楼；中央设多层香槟塔，反射纸醉金迷的光影；圆桌铺洁白丝绸桌布，摆放鎏金餐具与大簇白玫瑰花艺；整体色调暖金与乳白为主，充满高级奢华氛围"}
{"description": "iPhone 15 Pro Max，6.7 英寸屏幕，钛金属边框，哑光黑色玻璃面板；背面三摄像头模组垂直排列，LED 闪光灯；厚度 7.25mm，重量 221g；侧边 USB-C 接口"}

【其他规则（保持不变）】
1. 先分集再分单元：剧本总时长按总字数÷4（4 字/秒语速）估算，集数不得少于 %[3]d 集（总时长 ÷ 每集 %[1]d 分钟向上取整）——即使剧情连贯也必须按下刀点分集，每集为一个相对完整、结尾带悬念钩子的剧情段落，集标题格式为“第 N 集 - 本集看点”；仅当剧本总字数不足 240 字时才允许单集；
2. 叙事单元判据：叙事单元是一集之中按场景与画面连贯性切分出的最小拍摄单元（Beat），它同时是后续资产关联、视频提示词撰写与视频生成的统筹单位——严禁把一整集剧情压缩成一个单元；切分下刀点优先选择：情绪转折点、说话人切换、动作段落结束、场景/时间切换；
3. 每集目标时长约 %[1]d 分钟：叙事单元 duration_sec 加总应接近该目标，时长合计需覆盖该集全部情节；每集叙事单元数量不少于 %[2]d 个（按目标时长折算），通常 4~8 个；
4. 单元时长铁律：单个叙事单元 duration_sec 不得超过 30 秒；单元时长必须与其摘录体量匹配——摘录字数÷4（按 4 字/秒语速）≈ duration_sec，不得明显低估；摘录超过约 120 字的段落必然超 30 秒，必须在下刀点继续切分为多个单元；
5. 每个叙事单元 1~3 个镜头块，每个镜头块 2~4 个镜头；镜头块 duration_sec 加总应等于所属单元 duration_sec；
6. script_original_text 必须从剧本原文中逐字摘录该单元对应的真实原文片段，不得改写、缩写或省略为空；全部叙事单元的摘录按剧情顺序拼接后必须完整覆盖剧本原文，不得遗漏任何段落；
7. shot_size 仅限 ECU/CU/MCU/MS/FS/LS/ELS/TWO_SHOT/OTS；
8. camera_movement 仅限 STATIC/SLOW_PUSH_IN/SLOW_PULL_OUT/FAST_PUSH_IN/DOLLY_IN/DOLLY_OUT/PAN_LEFT/PAN_RIGHT/TRACK_LEFT/TRACK_RIGHT/TILT_UP/TILT_DOWN/CRANE_UP/CRANE_DOWN/HANDHELD_SHAKY/WIPE_CLIP/ZOOM_IN_OPTICAL/SLOW_360_ORBIT；
9. camera_angle 仅限 EYE_LEVEL_FRONT/SIDE/BACK/SLIGHT_LOW_ANGLE/LOW_ANGLE/SLIGHT_HIGH/HIGH_BIRDVIEW/OTS_LEFT/OTS_RIGHT/DUTCH_ANGLE；
10. 角色≤6 人、场景≤8 个；scenes 每项 code/base_name、props 每项 code/name 均不得为空；scenes 的 base_name 必须为 2~6 字场景简称（如"公寓"、"办公室"、"古玩街"，须与剧本中的地点称呼一致），禁止写成短语或长句描述（详细环境描述只写进 base_prompt）；
11. 角色/场景/道具按集抽取：每项必须携带 origin_episode_id（首次出现集数的 1-based 序号），不区分叙事单元；category 仅限 WEAPON/CLOTHING_ACCESSORY/FOOD_DRINK/STATIONERY_BOOK/JEWELRY_ORNAMENT/FURNITURE_DECOR/ELECTRONIC_APPLIANCE/VEHICLE/TOOL_MUSICAL_INSTRUMENT/TOY/PLANT_FLOWER/ANIMAL_PET/LIGHTING_SOURCE/DOCUMENT_SCROLL_MAP/FLAG_BANNER/OTHER；
12. 只输出合法 JSON，不要任何解释文字。`

// polishSystemPrompt 润色镜头要素为 LuxReal 风格提示词（结构化【】逐行，直接送视频模型）。
// 对齐 LuxReal 实测分镜脚本格式：镜头 | 时长 + 【景别】【运镜】【视角】【主体描述】【光影】【台词】【音效】。
const polishSystemPrompt = `你是资深短剧分镜与视频提示词专家。将镜头要素润色为 LuxReal 风格分镜提示词，直接输出给视频生成模型。
必须严格按以下格式逐行输出（不要 JSON、不要多余解释、不要 markdown）：
镜头 | %d秒
【景别】...
【运镜】...
【视角】...
【主体描述】...
【光影】...
【台词】...（无台词填"无"）
【音效】<...>（无音效填"无"）
要求：主体描述包含人物动作/表情/道具互动细节；光影细化光线方向与明暗对比；忠于给定景别/运镜/视角；台词保留角色名；主体/背景中出现的角色名、场景名、道具名保持原文，不得改写或省略（供参考图引用匹配）。`

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
}

func NewSplitHandler(llm LLMClient, store SplitStore) *SplitHandler {
	// maxTry=4：校验维度多（JSON/结构/集数/摘录体量/覆盖率），纠正提示累积后模型常需多轮收敛
	return &SplitHandler{llm: llm, store: store, maxTry: 4}
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
	systemPrompt := fmt.Sprintf(splitSystemPrompt, params.EpisodeDurationMin, minUnits, minEpisodes)
	// 拆分技能风格段（全局 agent_skills 库，随任务参数透传；与基础规则冲突时以技能要求为准）
	if params.SplitSkillPrompt != "" {
		systemPrompt += "\n拆分风格技能（与上述规则冲突时以技能要求为准）：" + params.SplitSkillPrompt
	}

	// ① 拆分 LLM（坏 JSON/集数不足/摘录超长/摘录不完整 自动重试 maxTry=4 含首次；纠正提示累积避免振荡）
	var result *SplitResult
	var lastErr error
	extraHint := ""
	for attempt := 1; attempt <= h.maxTry; attempt++ {
		resp, err := h.llm.Chat(ctx, model.CapLLM, &model.ChatRequest{
			Prompt:      params.ScriptText,
			System:      systemPrompt + extraHint,
			MaxTokens:   16384, // 多集结构 + 逐字摘录体量大，8192 会截断 JSON（unexpected end of JSON input）
			Temperature: 0.3,
			JSONMode:    true,
			// deepseek-v4-* 默认开推理：拆分实测推理耗 98~105s 且 reasoning 烧光 16384 token 预算
			// 导致 content 为空（provider 报 empty provider content）。拆分不需要推理过程，显式关闭。
			NoThinking: true,
		})
		if err != nil {
			lastErr = fmt.Errorf("拆分 LLM 调用失败(第%d次): %w", attempt, err)
			// 每次尝试的真实错误都落日志（此前仅最后一次进 error_message，超时/限流等根因被冷却跳过掩盖）
			log.Printf("split: 第%d次 LLM 调用失败: %v", attempt, err)
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

	// ②.5 AI 优化资产提示词（核心功能：将简要描述扩展为 LuxReal 风格的详细视觉提示词）
	// 注意：不阻塞主流程，失败也不影响拆分结果写入
	h.optimizeAllAssets(ctx, result)

	// ③ 润色 LLM（对每个镜头生成 prompt_refined；失败不阻塞整体，回退原文）
	h.polishAll(ctx, result)

	// ③ 四层入库 + 状态转移（事务）
	stats, err := h.store.SaveSplitResult(ctx, derefProjectID(t), result)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(stats)
	return out, nil
}

// polishAll 逐镜头润色 (PolishAgent;单镜头失败不中断)。
func (h *SplitHandler) polishAll(ctx context.Context, r *SplitResult) {
	for ei := range r.Episodes {
		for bi := range r.Episodes[ei].NarrativeUnits {
			beat := &r.Episodes[ei].NarrativeUnits[bi]
			for gi := range beat.ShotGroups {
				sg := &beat.ShotGroups[gi]
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
						NoThinking:  true, // 推理模式会烧光 512 预算致 content 空，润色无需推理
					})
					if err != nil || resp.Content == "" {
						s.PromptRefined = luxShotPrompt(s) // 回退：LuxReal 结构化格式（枚举转中文）
						continue
					}
					s.PromptRefined = resp.Content
				}
			}
		}
	}
}

// optimizeAllAssets AI 优化所有资产提示词 (角色/场景/道具;将其从简要描述升级为 LuxReal 风格详细提示词)。
// 注意：每个资产独立优化，单个失败不影响其他资产；失败只记录日志不返回错误。
func (h *SplitHandler) optimizeAllAssets(ctx context.Context, r *SplitResult) {
	log.Printf("[AssetOptimization] 开始优化资产...")

	// 1. 优化角色提示词
	for ci := range r.Characters {
		char := &r.Characters[ci]
		if char.DescriptionNotes == "" || len([]rune(char.DescriptionNotes)) < 50 {
			optimized, err := h.expandAssetPrompt(ctx, "character", char.Name, char.DescriptionNotes)
			if err != nil {
				log.Printf("[AssetOptimization] 角色%s优化失败：%v，使用原描述", char.Name, err)
				continue
			}
			if optimized != "" && optimized != char.DescriptionNotes {
				char.DescriptionNotes = optimized
				log.Printf("[AssetOptimization] 角色%s已优化: %d→%d字", char.Name, len([]rune(char.DescriptionNotes)), len([]rune(optimized)))
			}
		}
	}

	// 2. 优化场景提示词
	for si := range r.Scenes {
		scene := &r.Scenes[si]
		if scene.BasePrompt == "" || len([]rune(scene.BasePrompt)) < 60 {
			optimized, err := h.expandAssetPrompt(ctx, "scene", scene.BaseName, scene.BasePrompt)
			if err != nil {
				log.Printf("[AssetOptimization] 场景%s优化失败：%v，使用原描述", scene.BaseName, err)
				continue
			}
			if optimized != "" && optimized != scene.BasePrompt {
				scene.BasePrompt = optimized
				log.Printf("[AssetOptimization] 场景%s已优化: %d→%d字", scene.BaseName, len([]rune(scene.BasePrompt)), len([]rune(optimized)))
			}
		}
	}

	// 3. 优化道具提示词
	for pi := range r.Props {
		prop := &r.Props[pi]
		if prop.Description == "" || len([]rune(prop.Description)) < 30 {
			optimized, err := h.expandAssetPrompt(ctx, "prop", prop.Name, prop.Description)
			if err != nil {
				log.Printf("[AssetOptimization] 道具%s优化失败：%v，使用原描述", prop.Name, err)
				continue
			}
			if optimized != "" && optimized != prop.Description {
				prop.Description = optimized
				log.Printf("[AssetOptimization] 道具%s已优化: %d→%d字", prop.Name, len([]rune(prop.Description)), len([]rune(optimized)))
			}
		}
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
