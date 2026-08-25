// Package agent 智能体引擎（E7）：Step1 剧本拆分 Worker（双 LLM：拆分/润色）+ JSON Schema 结构化输出。
// 对齐 04 J 章（StoryboardAgent 拆分 + PolishAgent 润色 + 技能激活）。
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// FlexInt 容忍 LLM 输出小数时长（如 duration_sec: 2.5）：四舍五入取整。
// GLM 等模型对短镜头常给半秒值，严格 int 反序列化会整轮作废（重试耗尽仍失败）。
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*f = 0
		return nil
	}
	fl, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("非数字时长 %q: %w", s, err)
	}
	*f = FlexInt(int(math.Round(fl)))
	return nil
}

// SplitResult LLM 拆分输出的四层结构 + 集级资产（03 A.2：集 → 叙事单元 → 镜头块 → 单镜头 + 角色/场景/道具抽取）。
// 枚举值对齐 02 A3（景别/运镜/视角）——校验器强制。
// 资产粒度：仅到集级（每集自有角色/场景/道具），不下沉到叙事单元。
type SplitResult struct {
	Episodes   []EpisodeSplit   `json:"episodes"`
	Characters []CharacterSplit `json:"characters,omitempty"`
	Scenes     []SceneSplit     `json:"scenes,omitempty"`
	Props      []PropSplit      `json:"props,omitempty"`
}

// CharacterSplit 角色抽取（Step1 产出，供 Step2 角色卡片；集级粒度）。
type CharacterSplit struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	Tags             string `json:"tags,omitempty"`
	DescriptionNotes string `json:"description_notes,omitempty"`
	OriginEpisodeID  int    `json:"origin_episode_id"` // 1-based 集序号
}

// SceneSplit 场景抽取（Step1 产出，供 Step2 场景卡片；集级粒度）。
type SceneSplit struct {
	Code            string `json:"code"`
	BaseName        string `json:"base_name"`
	BasePrompt      string `json:"base_prompt,omitempty"`
	OriginEpisodeID int    `json:"origin_episode_id"` // 1-based 集序号
}

// PropSplit 道具抽取（Step1 产出，供 Step2 道具卡片；集级粒度）。
type PropSplit struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Category        string `json:"category,omitempty"` // prop_category_enum，非法值落库时归一为 OTHER
	Description     string `json:"description,omitempty"`
	OriginEpisodeID int    `json:"origin_episode_id"` // 1-based 集序号
}

type EpisodeSplit struct {
	Title          string          `json:"title"`
	NarrativeUnits []NarrativeUnit `json:"narrative_units"`
}

type NarrativeUnit struct {
	Title              string      `json:"title"`
	DurationSec        FlexInt     `json:"duration_sec"`
	ScriptOriginalText string      `json:"script_original_text"`
	ShotGroups         []ShotGroup `json:"shot_groups"`
}

type ShotGroup struct {
	Title       string  `json:"title"`
	DurationSec FlexInt `json:"duration_sec"`
	Shots       []Shot  `json:"shots"`
}

type Shot struct {
	DurationSec           FlexInt `json:"duration_sec"`
	ShotSize              string  `json:"shot_size"`
	CameraMovement        string  `json:"camera_movement"`
	CameraAngle           string  `json:"camera_angle"`
	SubjectDescription    string  `json:"subject_description"`
	BackgroundDescription string  `json:"background_description"`
	LocalLightDescription string  `json:"local_light_description"`
	DialogueText          string  `json:"dialogue_text"`
	AmbientSound          string  `json:"ambient_sound"`
	PromptRefined         string  `json:"prompt_refined,omitempty"`
	ReferenceImageURL     string  `json:"reference_image_url,omitempty"` // 镜头级参考图（对齐 LuxReal「镜头N的参考图」）
}

// 镜头枚举 → 中文（对齐 LuxReal：分镜提示词为中文；用于润色 seed 与 LLM 失败回退）。
var shotSizeCnMap = map[string]string{
	"ECU": "极特写", "CU": "特写", "MCU": "中近景", "MS": "中景", "FS": "全景",
	"LS": "远景", "ELS": "大远景", "TWO_SHOT": "双人景", "OTS": "过肩景",
}

var cameraMoveCnMap = map[string]string{
	"STATIC": "固定", "SLOW_PUSH_IN": "缓慢推近", "SLOW_PULL_OUT": "缓慢拉远",
	"FAST_PUSH_IN": "快速推近", "DOLLY_IN": "推轨前移", "DOLLY_OUT": "拉轨后移",
	"PAN_LEFT": "左摇", "PAN_RIGHT": "右摇", "TRACK_LEFT": "左移", "TRACK_RIGHT": "右移",
	"TILT_UP": "上摇", "TILT_DOWN": "下摇", "CRANE_UP": "升臂", "CRANE_DOWN": "降臂",
	"HANDHELD_SHAKY": "手持微晃", "WIPE_CLIP": "转场擦切", "ZOOM_IN_OPTICAL": "光学变焦推近",
	"SLOW_360_ORBIT": "缓慢环绕",
}

var cameraAngleCnMap = map[string]string{
	"EYE_LEVEL_FRONT": "平视", "SIDE": "侧面", "BACK": "背面",
	"SLIGHT_LOW_ANGLE": "微仰拍", "LOW_ANGLE": "仰拍", "SLIGHT_HIGH": "微俯拍",
	"HIGH_BIRDVIEW": "高机位俯拍", "OTS_LEFT": "过肩左", "OTS_RIGHT": "过肩右", "DUTCH_ANGLE": "斜角",
}

func shotSizeCn(v string) string {
	if c, ok := shotSizeCnMap[v]; ok {
		return c
	}
	return v
}

func cameraMoveCn(v string) string {
	if c, ok := cameraMoveCnMap[v]; ok {
		return c
	}
	return v
}

func cameraAngleCn(v string) string {
	if c, ok := cameraAngleCnMap[v]; ok {
		return c
	}
	return v
}

// luxShotPrompt 生成 LuxReal 风格单镜头提示词（结构化【】逐行；无值兜底「无」）。
// 用于 LLM 润色失败时的确定性回退，保证 PromptRefined 始终为 LuxReal 格式。
func luxShotPrompt(s *Shot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "镜头 | %d秒\n", int(s.DurationSec))
	fmt.Fprintf(&b, "【景别】%s\n", shotSizeCn(s.ShotSize))
	fmt.Fprintf(&b, "【运镜】%s\n", cameraMoveCn(s.CameraMovement))
	fmt.Fprintf(&b, "【视角】%s\n", cameraAngleCn(s.CameraAngle))
	if strings.TrimSpace(s.SubjectDescription) != "" {
		fmt.Fprintf(&b, "【主体描述】%s\n", s.SubjectDescription)
	}
	if strings.TrimSpace(s.BackgroundDescription) != "" {
		fmt.Fprintf(&b, "【背景】%s\n", s.BackgroundDescription)
	}
	light := strings.TrimSpace(s.LocalLightDescription)
	if light == "" {
		light = "无"
	}
	fmt.Fprintf(&b, "【光影】%s\n", light)
	if d := strings.TrimSpace(s.DialogueText); d != "" {
		fmt.Fprintf(&b, "【台词】%s\n", d)
	} else {
		fmt.Fprintf(&b, "【台词】无\n")
	}
	if a := strings.TrimSpace(s.AmbientSound); a != "" {
		fmt.Fprintf(&b, "【音效】<%s>\n", a)
	} else {
		fmt.Fprintf(&b, "【音效】无\n")
	}
	return b.String()
}

// 合法枚举集合（02 A3 唯一真源；validator 用）。
var (
	validShotSizes   = setOf("ECU", "CU", "MCU", "MS", "FS", "LS", "ELS", "TWO_SHOT", "OTS")
	validCameraMoves = setOf("STATIC", "SLOW_PUSH_IN", "SLOW_PULL_OUT", "FAST_PUSH_IN", "DOLLY_IN", "DOLLY_OUT",
		"PAN_LEFT", "PAN_RIGHT", "TRACK_LEFT", "TRACK_RIGHT", "TILT_UP", "TILT_DOWN", "CRANE_UP", "CRANE_DOWN",
		"HANDHELD_SHAKY", "WIPE_CLIP", "ZOOM_IN_OPTICAL", "SLOW_360_ORBIT")
	validCameraAngles = setOf("EYE_LEVEL_FRONT", "SIDE", "BACK", "SLIGHT_LOW_ANGLE", "LOW_ANGLE", "SLIGHT_HIGH",
		"HIGH_BIRDVIEW", "OTS_LEFT", "OTS_RIGHT", "DUTCH_ANGLE")
)

func setOf(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

// ErrInvalidJSON 坏 JSON（LLM 输出无法解析）→ 触发重试（S1-B05 验收：坏 JSON 自动重试）。
var ErrInvalidJSON = errors.New("split: llm 输出非合法 JSON")

// ErrInvalidStructure 结构校验失败（枚举非法/缺字段）→ 触发重试。
var ErrInvalidStructure = errors.New("split: 结构校验失败")

// ErrProjectNotFound 项目不存在。
var ErrProjectNotFound = errors.New("split: project not found")

// ErrStateConflict 项目状态不允许写入拆分结果（重复回调/状态非法）。
var ErrStateConflict = errors.New("split: project state conflict")

// ParseAndValidate 解析并校验 LLM 输出；坏 JSON/非法枚举返回可重试错误。
func ParseAndValidate(raw string) (*SplitResult, error) {
	// 提取首个 JSON 对象（LLM 可能夹带 ```json 代码块）
	clean := extractJSON(raw)
	var result SplitResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	repairAssets(&result)
	if err := validateStructure(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// repairAssets 集级资产容错补全：模型偶发省略 scenes/props 的 code/name 字段，
// 自动补齐（code 按序生成、name 取描述截断），避免因此触发整次拆分重试。
func repairAssets(r *SplitResult) {
	for i := range r.Scenes {
		sc := &r.Scenes[i]
		if strings.TrimSpace(sc.Code) == "" {
			sc.Code = fmt.Sprintf("SCENE_%02d", i+1)
		}
		if strings.TrimSpace(sc.BaseName) == "" {
			if name := truncateRunes(strings.TrimSpace(sc.BasePrompt), 12); name != "" {
				sc.BaseName = name
			} else {
				sc.BaseName = fmt.Sprintf("场景%d", i+1)
			}
		}
	}
	for i := range r.Props {
		p := &r.Props[i]
		if strings.TrimSpace(p.Code) == "" {
			p.Code = fmt.Sprintf("PROP_%02d", i+1)
		}
		if strings.TrimSpace(p.Name) == "" {
			if name := truncateRunes(strings.TrimSpace(p.Description), 12); name != "" {
				p.Name = name
			} else {
				p.Name = fmt.Sprintf("道具%d", i+1)
			}
		}
	}
}

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}

func validateStructure(r *SplitResult) error {
	if len(r.Episodes) == 0 {
		return fmt.Errorf("%w: 缺少 episodes", ErrInvalidStructure)
	}
	for ei, ep := range r.Episodes {
		if len(ep.NarrativeUnits) == 0 {
			return fmt.Errorf("%w: 第%d集缺少 narrative_units", ErrInvalidStructure, ei+1)
		}
		for bi, beat := range ep.NarrativeUnits {
			if strings.TrimSpace(beat.Title) == "" {
				return fmt.Errorf("%w: 第%d集第%d个叙事单元缺标题", ErrInvalidStructure, ei+1, bi+1)
			}
			// 粒度铁律（对齐 LuxReal/ToonFlow）：单单元 ≤30s，超限视为非法输出触发重试
			if beat.DurationSec <= 0 || beat.DurationSec > 30 {
				return fmt.Errorf("%w: 叙事单元 %q 时长 %d 秒超限（须 1~30 秒，请切分为更多单元）", ErrInvalidStructure, beat.Title, beat.DurationSec)
			}
			if len(beat.ShotGroups) == 0 {
				return fmt.Errorf("%w: 叙事单元 %q 缺少 shot_groups", ErrInvalidStructure, beat.Title)
			}
			for gi, sg := range beat.ShotGroups {
				if len(sg.Shots) == 0 {
					return fmt.Errorf("%w: 镜头块 %q 缺少 shots", ErrInvalidStructure, sg.Title)
				}
				for si, s := range sg.Shots {
					if s.DurationSec <= 0 || s.DurationSec > 30 {
						return fmt.Errorf("%w: 镜头时长非法 %d", ErrInvalidStructure, s.DurationSec)
					}
					if !validShotSizes[s.ShotSize] {
						return fmt.Errorf("%w: 景别 %q 非法（第%d集/单元%d/块%d/镜头%d）", ErrInvalidStructure, s.ShotSize, ei+1, bi+1, gi+1, si+1)
					}
					if !validCameraMoves[s.CameraMovement] {
						return fmt.Errorf("%w: 运镜 %q 非法", ErrInvalidStructure, s.CameraMovement)
					}
					if !validCameraAngles[s.CameraAngle] {
						return fmt.Errorf("%w: 视角 %q 非法", ErrInvalidStructure, s.CameraAngle)
					}
				}
			}
		}
	}
	// 集级资产轻量校验：code/name 非空（缺省字段容忍，落库时兜底）
	for i, sc := range r.Scenes {
		if strings.TrimSpace(sc.Code) == "" || strings.TrimSpace(sc.BaseName) == "" {
			return fmt.Errorf("%w: 第%d个场景缺少 code/base_name", ErrInvalidStructure, i+1)
		}
	}
	for i, p := range r.Props {
		if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("%w: 第%d个道具缺少 code/name", ErrInvalidStructure, i+1)
		}
	}
	return nil
}

// ValidateExcerptCoverage 校验叙事单元原文摘录的忠实性与完整性（防模型静默丢原文）：
// 1) 每个单元摘录非空；2) 摘录开头必须逐字命中剧本原文（防改写/缩写）；
// 3) 全部摘录总长 ≥ 剧本 85%（防遗漏段落）。空白符归一后比较。
func ValidateExcerptCoverage(r *SplitResult, script string) error {
	norm := func(s string) string {
		return strings.Map(func(rt rune) rune {
			if unicode.IsSpace(rt) {
				return -1
			}
			return rt
		}, s)
	}
	srcNorm := norm(script)
	srcLen := len([]rune(srcNorm))
	if srcLen == 0 {
		return nil
	}
	total := 0
	for ei, ep := range r.Episodes {
		for bi, b := range ep.NarrativeUnits {
			ex := strings.TrimSpace(b.ScriptOriginalText)
			if ex == "" {
				return fmt.Errorf("%w: 第%d集第%d个叙事单元缺少原文摘录", ErrInvalidStructure, ei+1, bi+1)
			}
			exNorm := norm(ex)
			total += len([]rune(exNorm))
			// 忠实性抽样：摘录开头 24 字必须在剧本原文中逐字出现（防模型改写/概括）
			head := []rune(exNorm)
			if len(head) > 24 {
				head = head[:24]
			}
			if len(head) >= 8 && !strings.Contains(srcNorm, string(head)) {
				return fmt.Errorf("%w: 叙事单元 %q 的摘录非剧本原文（疑似改写，需逐字摘录）", ErrInvalidStructure, b.Title)
			}
		}
	}
	if float64(total) < float64(srcLen)*0.85 {
		return fmt.Errorf("%w: 原文摘录覆盖率过低（%d/%d 字），疑似遗漏剧本内容", ErrInvalidStructure, total, srcLen)
	}
	return nil
}

// NormalizeEpisodeCount 确定性分集后处理：模型偏好在剧情连贯时把全部单元塞进单集（重试纠正仍顽固），
// 故当输出为单集且要求多集时，按单元边界把叙事单元均衡切分为 minEpisodes 集（按摘录字数贪心配平）；
// 另对尾部/中部过小集（摘录<120字）向前合并，避免孤儿集。集标题取首单元标题。
func NormalizeEpisodeCount(r *SplitResult, minEpisodes int) *SplitResult {
	if len(r.Episodes) == 1 && minEpisodes > 1 {
		r = splitSingleEpisode(r, minEpisodes)
	}
	return mergeTinyEpisodes(r, minEpisodes)
}

func splitSingleEpisode(r *SplitResult, minEpisodes int) *SplitResult {
	if len(r.Episodes) != 1 || minEpisodes <= 1 {
		return r
	}
	units := r.Episodes[0].NarrativeUnits
	if len(units) < minEpisodes {
		minEpisodes = len(units) // 单元数不够时有多少切多少
	}
	if minEpisodes <= 1 {
		return r
	}
	// 按摘录字数贪心均分（保持单元顺序，仅在单元边界下刀）
	weights := make([]int, len(units))
	total := 0
	for i, u := range units {
		w := len([]rune(strings.Map(func(rt rune) rune {
			if unicode.IsSpace(rt) {
				return -1
			}
			return rt
		}, u.ScriptOriginalText)))
		if w <= 0 {
			w = 1
		}
		weights[i] = w
		total += w
	}
	target := (total + minEpisodes - 1) / minEpisodes
	eps := make([]EpisodeSplit, 0, minEpisodes)
	start, acc := 0, 0
	for i, w := range weights {
		acc += w
		left := len(units) - i - 1
		need := minEpisodes - len(eps)
		// 剩余单元数刚好够剩余集数，或累计已达目标且后面还有集要分 → 下刀
		if left == need || (acc >= target && need > 1) {
			eps = append(eps, EpisodeSplit{
				Title:          fmt.Sprintf("第%d集 - %s", len(eps)+1, units[start].Title),
				NarrativeUnits: units[start : i+1],
			})
			start, acc = i+1, 0
		}
	}
	if start < len(units) {
		eps = append(eps, EpisodeSplit{
			Title:          fmt.Sprintf("第%d集 - %s", len(eps)+1, units[start].Title),
			NarrativeUnits: units[start:],
		})
	}
	r.Episodes = eps
	return r
}

// mergeTinyEpisodes 把摘录总量<120字的过小集向前合并（保留 minEpisodes 下限），
// 防止模型切分点不当产生孤儿集（如尾集只有十几个字）。
func mergeTinyEpisodes(r *SplitResult, minEpisodes int) *SplitResult {
	const minEpRunes = 120
	epRunes := func(ep EpisodeSplit) int {
		n := 0
		for _, u := range ep.NarrativeUnits {
			n += len([]rune(strings.Map(func(rt rune) rune {
				if unicode.IsSpace(rt) {
					return -1
				}
				return rt
			}, u.ScriptOriginalText)))
		}
		return n
	}
	eps := r.Episodes
	for i := len(eps) - 1; i >= 1; i-- {
		if len(eps) <= minEpisodes && i <= minEpisodes-1 {
			break // 不合并到低于下限
		}
		if epRunes(eps[i]) >= minEpRunes {
			continue
		}
		// 并入前一集：单元追加、标题保留前集
		eps[i-1].NarrativeUnits = append(eps[i-1].NarrativeUnits, eps[i].NarrativeUnits...)
		eps = append(eps[:i], eps[i+1:]...)
	}
	// 重排集标题序号（模型自分的集标题可能因合并出现跳号）
	for i := range eps {
		if idx := strings.Index(eps[i].Title, " - "); idx > 0 {
			eps[i].Title = fmt.Sprintf("第%d集%s", i+1, eps[i].Title[idx:])
		}
	}
	r.Episodes = eps
	return r
}

// SplitOversizedUnits 确定性超长单元切分：模型对个别长段落反复不切（重试无效且振荡），
// 后处理按句号/问号/感叹号边界把摘录>120字的单元切成 ≤120 字子单元；
// 镜头块按时长比例归属子单元，子单元时长按其镜头块加总回写（满足“块加总=单元”约束）。
func SplitOversizedUnits(r *SplitResult) *SplitResult {
	const maxRunes = 120
	runeLen := func(s string) int {
		return len([]rune(strings.Map(func(rt rune) rune {
			if unicode.IsSpace(rt) {
				return -1
			}
			return rt
		}, s)))
	}
	for ei := range r.Episodes {
		ep := &r.Episodes[ei]
		out := make([]NarrativeUnit, 0, len(ep.NarrativeUnits))
		for _, u := range ep.NarrativeUnits {
			if runeLen(u.ScriptOriginalText) <= maxRunes {
				out = append(out, u)
				continue
			}
			// 按句末标点切句（保留标点）
			rs := []rune(u.ScriptOriginalText)
			var sentences []string
			last := 0
			for i, rt := range rs {
				if rt == '。' || rt == '！' || rt == '？' || rt == '…' || rt == '\n' {
					sentences = append(sentences, string(rs[last:i+1]))
					last = i + 1
				}
			}
			if last < len(rs) {
				sentences = append(sentences, string(rs[last:]))
			}
			// 贪心打包：每块摘录 ≤ maxRunes 字
			type chunk struct {
				text string
				size int
			}
			var chunks []chunk
			cur, curSize := "", 0
			for _, s := range sentences {
				sl := runeLen(s)
				if curSize+sl > maxRunes && curSize > 0 {
					chunks = append(chunks, chunk{cur, curSize})
					cur, curSize = "", 0
				}
				cur += s
				curSize += sl
			}
			if curSize > 0 {
				chunks = append(chunks, chunk{cur, curSize})
			}
			if len(chunks) <= 1 {
				out = append(out, u)
				continue
			}
			// 镜头块按时长比例归属子单元；单块超过首个子单元预算时归入最大预算子单元
			totalDur := 0
			for _, g := range u.ShotGroups {
				totalDur += int(g.DurationSec)
			}
			if totalDur <= 0 {
				totalDur = int(u.DurationSec)
			}
			budgets := make([]int, len(chunks))
			assigned := make([][]ShotGroup, len(chunks))
			accSize, accDur := 0, 0
			for ci, c := range chunks {
				accSize += c.size
				if ci == len(chunks)-1 {
					budgets[ci] = totalDur - accDur
				} else {
					budgets[ci] = totalDur * c.size / runeLen(u.ScriptOriginalText)
				}
				if budgets[ci] < 1 {
					budgets[ci] = 1
				}
				accDur += budgets[ci]
			}
			ci := 0
			for _, g := range u.ShotGroups {
				// 顺序填桶：当前桶时长预算满则进下一桶
				assigned[ci] = append(assigned[ci], g)
				if ci < len(chunks)-1 && groupDur(assigned[ci]) >= budgets[ci] {
					ci++
				}
			}
			// 兄弟单元：空桶合并到相邻桶（保证每子单元有镜头块）
			for i := 0; i < len(chunks); i++ {
				if len(assigned[i]) == 0 {
					j := i - 1
					if j < 0 {
						j = i + 1
					}
					if j >= 0 && j < len(chunks) && len(assigned[j]) > 1 {
						moved := assigned[j][len(assigned[j])-1]
						assigned[j] = assigned[j][:len(assigned[j])-1]
						assigned[i] = append(assigned[i], moved)
					}
				}
			}
			for i, c := range chunks {
				sub := NarrativeUnit{
					Title:              fmt.Sprintf("%s-%d", u.Title, i+1),
					ScriptOriginalText: strings.TrimSpace(c.text),
					ShotGroups:         assigned[i],
				}
				d := 0
				for _, g := range assigned[i] {
					d += int(g.DurationSec)
				}
				if d <= 0 {
					d = budgets[i]
				}
				if d > 30 {
					d = 30
				}
				sub.DurationSec = FlexInt(d)
				out = append(out, sub)
			}
		}
		ep.NarrativeUnits = out
	}
	return r
}

func groupDur(gs []ShotGroup) int {
	d := 0
	for _, g := range gs {
		d += int(g.DurationSec)
	}
	return d
}

// ValidateBeatExcerptSize 校验单元摘录体量与时长匹配（防拆分不精准/时长低估）：
// 单单元摘录超过 120 字（空白归一后）按 4字/秒必然超 30 秒上限，须继续切分。
func ValidateBeatExcerptSize(r *SplitResult) error {
	const maxExcerptRunes = 120
	for ei, ep := range r.Episodes {
		for bi, b := range ep.NarrativeUnits {
			n := len([]rune(strings.Map(func(rt rune) rune {
				if unicode.IsSpace(rt) {
					return -1
				}
				return rt
			}, b.ScriptOriginalText)))
			if n > maxExcerptRunes {
				return fmt.Errorf("%w: 第%d集第%d个叙事单元摘录 %d 字超 %d 字上限，须继续切分", ErrInvalidStructure, ei+1, bi+1, n, maxExcerptRunes)
			}
		}
	}
	return nil
}

// extractJSON 从 LLM 输出中提取第一个 { ... } 对象（容忍 ```json 围栏与前缀文本）。
func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return raw
	}
	end := strings.LastIndex(raw, "}")
	if end <= start {
		return raw
	}
	return raw[start : end+1]
}
