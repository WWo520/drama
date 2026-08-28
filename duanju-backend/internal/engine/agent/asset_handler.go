// AssetHandler：Worker-Asset（CHAR_IMAGE_GEN/SCENE_IMAGE_GEN/PROP_IMAGE_GEN 执行器）。
// 开发期 mock 图片生成（生成占位图 URL）；生产接入图生模型（B06 ModelEngine.GenerateImage）。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/engine/task"
)

// AssetStore 资产生成边界（由 repository.AssetStore 实现）。
type AssetStore interface {
	// LoadAssetTarget 解析任务目标（kind/target_id → 项目/集/积分）。
	LoadAssetTarget(ctx context.Context, targetKind string, targetID int64) (*AssetTarget, error)
	// MarkAssetRunning 资产表置 RUNNING（执行开始；前端进度条"生成中"）。
	MarkAssetRunning(ctx context.Context, targetKind string, targetID int64) error
	// SaveGeneratedImage 记录生成图 + 更新业务表 + usage log（事务）。
	SaveGeneratedImage(ctx context.Context, t *AssetTarget, imageURL string, taskID int64) error
}

// GenParams 弹窗生成参数（对齐 LuxReal：模型/风格/比例/默认构图/参考图/临时提示词/场景时段）。
type GenParams struct {
	Prompt                string   `json:"prompt,omitempty"` // 临时提示词（图改图指令），优先于资产默认提示词
	ModelCode             string   `json:"model_code,omitempty"`
	StyleName             string   `json:"style_name,omitempty"`
	AspectRatio           string   `json:"aspect_ratio,omitempty"`
	Resolution            string   `json:"resolution,omitempty"`
	UseDefaultComposition bool     `json:"use_default_composition"`
	RefImageURLs          []string `json:"ref_image_urls,omitempty"` // 参考图 URL 列表（图生图）
	TimeOfDay             string   `json:"time_of_day,omitempty"`    // 场景时段（time_of_day_enum：DAY_BRIGHT/NIGHT_CANDLELIT…），场景日夜变体
	Weather               string   `json:"weather,omitempty"`        // 天气（weather_enum）
}

// AssetTarget 资产生成目标（与 repository.AssetTarget 对齐，接口隔离避免依赖方向反转）。
type AssetTarget struct {
	Kind      string
	TargetID  int64
	ProjectID int64
	EpisodeID int64
	AssetID   *int64
	PointCost int
	Prompt    string     // 生成提示词（edited_prompt 优先，回退拆分默认描述）
	TimeOfDay string     // 场景时段原始枚举（SCENE；Go 侧翻译，避免 SQL 枚举字面量 22P02）
	Weather   string     // 天气原始枚举（SCENE）
	Gen       *GenParams // 弹窗生成参数（可为 nil）
}

// AssetHandler 实现 task.Handler（三种资产生成任务）。
type AssetHandler struct {
	store   AssetStore
	engine  ImageGenClient // 可为 nil：未注入时降级 mock（开发期）
	env     string         // 08 审查 Major#16：生产环境禁止 mock 降级
	persist ImagePersister // 可选：生成图转存自有 MinIO（TOS 签名 24h 过期根因）
}

// SetPersister 注入图片持久化（nil 不转存）。
func (h *AssetHandler) SetPersister(p ImagePersister) { h.persist = p }

// ImageGenClient 图生模型能力（由 ModelEngine 实现注入）。
type ImageGenClient interface {
	GenerateImage(ctx context.Context, capability model.Capability, req *model.ImageRequest) (*model.ImageResponse, error)
}

// charThreeViewSuffix 角色默认构图模板（对齐 LuxReal 实测原文）：
// 一张 16:9 图内 = 左侧半身特写 + 右侧正/侧/背三个全身视图，纯白背景无文字。
// 注意措辞禁用"图纸"——该词曾诱导模型按"角色设定图纸"作画，把提示词中的
// 身高/体重数字渲染成画面上的标注线与文字（用户实测图内出现"身高175"）。
const charThreeViewSuffix = "生成该主体的全身三视图：正面视图、侧面视图和背面视图。画面布局：图像左侧为主体上半身特写，图像右侧并排展示主体的正面、侧面和背面三个全身视图。纯白色背景，画面仅展示主体上半身特写和三视图。这不是角色设定图纸，禁止绘制身高标注线、尺寸标注线、网格线和参考线，画面中不能出现任何文字、字母、数字和水印"

// bodyMetricRe 身体数据模式（身高/体重/体脂/三围/年龄）：纯数字度量对图像生成无视觉
// 价值，反而诱导模型把数字渲染进画面。剥离后再组装提示词。
var bodyMetricRe = regexp.MustCompile(
	`(身高|体重|体脂率|体脂|三围)\s*约?\s*[0-9]+(\.[0-9]+)?\s*(cm|CM|厘米|毫米|米|m|kg|KG|公斤|斤|%|)` +
		`|[0-9]{1,3}\s*[-–~到至]\s*[0-9]{0,3}\s*岁(左右|上下)?` +
		`|[0-9]{1,3}\s*岁(左右|上下)?`)

// punctSquashRe 连续分隔标点压缩（剥离数据后残留"，,"等）。
var punctSquashRe = regexp.MustCompile(`[，。；,;]\s*[，。；,;]+`)

// stripBodyMetrics 剥离提示词中的身体数据描述并整理标点。
func stripBodyMetrics(s string) string {
	if s == "" {
		return s
	}
	out := bodyMetricRe.ReplaceAllString(s, "")
	out = punctSquashRe.ReplaceAllString(out, "，")
	out = strings.TrimLeft(out, "，。；、 ")
	return out
}

// noTextSuffix 全局无文字约束（画面防文字兜底；提示词已含同类约束时跳过）。
const noTextSuffix = "。纯摄影画面，画面中禁止出现任何文字、字母、数字、水印、商标、标签、贴纸、字幕和图形标注"

// bracketLabelRe 资产描述中的「【字段名】」结构化标签（拆分阶段 LLM 输出的模板格式）。
var bracketLabelRe = regexp.MustCompile(`【[^】]*】`)

// flattenBracketLabels 剥离资产描述里的【字段名】标签并把内容压成自然语言短句。
// 根因：「【物品名称】咖啡【尺寸】约200ml」这类带方括号字段的模板文本是文生图模型
// 公认的"画面内标注文字"强诱导源（Seedream 会把字段名/数值直接画成杯身印刷、包装
// 标签等），结尾一句无文字软约束对抗不了该硬诱导（用户实测图内出现整段中文标签）。
func flattenBracketLabels(s string) string {
	if s == "" {
		return s
	}
	out := bracketLabelRe.ReplaceAllString(s, "，")
	out = punctSquashRe.ReplaceAllString(out, "，")
	out = strings.TrimLeft(out, "，。；、 ")
	return out
}

// timeOfDayCNDesc 时段枚举 → 画面描述（场景日夜变体，对齐 LuxReal）。
var timeOfDayCNDesc = map[string]string{
	"DAWN_BLUE_HOUR":        "黎明蓝调时刻，天色微亮",
	"MORNING_SUNRISE":       "清晨日出，晨光柔和",
	"NOON_HIGH_SUN":         "正午烈日，阳光强烈",
	"DAY_BRIGHT":            "白天明亮",
	"AFTERNOON_GOLDEN_HOUR": "午后黄金时刻，暖阳斜照",
	"SUNSET_DUSK":           "黄昏日落，橙红霞光，暮色",
	"NIGHT_CANDLELIT":       "夜晚烛光照明，暖黄光线",
	"NIGHT_NEON":            "夜晚霓虹灯光，城市夜色",
	"OVERCAST_DAY":          "阴天，光线柔和漫射",
	"STORM_NIGHT":           "暴风雨夜，闪电雷雨",
}

// weatherCNDesc 天气枚举 → 画面描述。
var weatherCNDesc = map[string]string{
	"CLEAR_SUNNY":       "晴朗无云",
	"CLOUDY_OVERCAST":   "多云阴天",
	"LIGHT_RAIN":        "小雨",
	"HEAVY_RAIN":        "大雨",
	"SNOW":              "下雪",
	"FOG_MIST":          "雾霭弥漫",
	"WINDY_DUST":        "风沙扬尘",
	"THUNDERSTORM":      "雷暴",
	"INDOOR_CONTROLLED": "室内环境可控光线",
}

// stylePromptSuffix 公共风格库 → 提示词后缀（对齐 LuxReal 27 项风格库，prompt_suffix_static 机制）。
// 根因修复：此前 StyleName 只落库不进 prompt，导致选"国风3D"等风格出图与"通用写实"完全一致；
// 且默认无任何风格约束词时模型自由发挥（写实剧本也画出动漫脸）。
var stylePromptSuffix = map[string]string{
	"通用写实":  "写实摄影风格，真实人物质感，自然光影，电影级真实感，照片级细节，禁止动漫/插画/卡通化",
	"古风写实":  "中国古代写实风格，传统服饰质感真实，古典光影氛围，写实摄影质感",
	"武侠玄幻":  "武侠玄幻风格，飘逸衣袂，仙侠气场，戏剧化光影，气势恢宏",
	"现代写实":  "现代都市写实风格，当代服装与场景，自然光摄影质感，真实生活气息",
	"末世废土":  "末世废土风格，破败荒凉，锈蚀质感，灰暗色调，荒芜苍凉氛围",
	"未来科幻":  "未来科幻风格，赛博科技感，冷色调霓虹，机械质感，高级感光效",
	"国风2D":  "国风二维动画风格，中国风配色，扁平化插画线条，东方美学构图",
	"国风3D":  "国风三维动画风格，中国古典元素三维建模渲染，皮克斯级3D质感，东方角色设计，精致3D材质光影",
	"美式卡通":  "美式卡通风格，夸张比例，鲜明色彩，迪士尼/卡通频道式动画质感",
	"清新日漫":  "日系动漫风格，清新色彩，赛璐璐上色，日式动漫画风，大眼睛角色",
	"韩式网漫":  "韩国网络漫画风格，精致五官，柔和渐变上色，网漫式渲染",
	"邵氏武侠":  "邵氏武侠电影风格，七十年代港式武侠质感，胶片颗粒，经典武打片色调",
	"三渲二":   "三渲二风格（3D建模+2D渲染），卡通渲染质感，动画番剧画面",
	"古风武侠":  "古风武侠风格，中国古代武侠气质，飘逸写意，江湖氛围",
	"王家卫美学": "王家卫电影美学，浓郁色彩，抽帧慢镜情绪感，霓虹夜色，暧昧光影",
	"徐克美学":  "徐克电影美学，奇诡武侠视觉，夸张色彩构图，天马行空的东方奇幻",
	"唯美写实":  "唯美写实风格，柔和光线，精致细节，梦幻唯美氛围，高质感人像",
	"幻彩插画":  "幻彩插画风格，绚丽色彩，梦幻光效，艺术化插画表现",
	"Q版卡通":  "Q版卡通风格，二头身可爱比例，圆润造型，明亮色彩",
	"彩铅绘本":  "彩铅手绘绘本风格，彩色铅笔质感，温暖笔触，童书插画感",
	"黏土动画":  "黏土动画风格，粘土质感角色，定格动画质感，柔和材质",
	"淡雅水彩":  "淡雅水彩风格，水彩晕染，透明色调，清新雅致",
	"日系治愈":  "日系治愈风格，温暖柔和色调，治愈系氛围，新海诚式光影",
	"双城风格":  "双城对比风格，城市光影反差，现代与传统交融视觉",
	"中式梦核":  "中式梦核风格，千禧年怀旧中国场景，梦境般超现实氛围，旧照片色调",
	"水墨写意":  "中国水墨写意风格，留白意境，墨色晕染，文人画气质",
	"无特定风格": "",
}

// defaultRealisticSuffix 默认写实约束（无显式风格时的兜底，防模型自由发挥画出动漫脸）。
const defaultRealisticSuffix = "写实风格，真实质感，自然光影"

// truncateStr UTF-8 安全截断（日志摘要用）。
func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func NewAssetHandler(store AssetStore, engine ImageGenClient, env string) *AssetHandler {
	return &AssetHandler{store: store, engine: engine, env: env}
}

// Name 支持的任务类型。
func (h *AssetHandler) Name() string { return "CHAR_IMAGE_GEN" }

// 支持的目标资产类型（asset_kind_enum 真源：CHARACTER_VARIANT/SCENE/PROP）。
var assetTaskTypes = map[string]bool{
	"CHARACTER_VARIANT": true,
	"SCENE":             true,
	"PROP":              true,
}

// Handles 判断是否处理该任务类型（task_type_enum：三种图生成任务）。
func (h *AssetHandler) Handles(taskType string) bool {
	switch taskType {
	case "CHAR_IMAGE_GEN", "SCENE_IMAGE_GEN", "PROP_IMAGE_GEN":
		return true
	}
	return false
}

// Execute 执行：解析目标 → mock 生成图 URL → 事务入库。
func (h *AssetHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		TargetKind string     `json:"target_kind"`
		TargetID   int64      `json:"target_id"`
		Gen        *GenParams `json:"gen,omitempty"`
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil {
		return nil, fmt.Errorf("解析任务参数失败: %w", err)
	}
	if !assetTaskTypes[params.TargetKind] || params.TargetID == 0 {
		return nil, fmt.Errorf("非法资产生成参数: kind=%s id=%d", params.TargetKind, params.TargetID)
	}
	// 执行即置 RUNNING（前端进度条"生成中"；非致命失败不影响生成）
	if err := h.store.MarkAssetRunning(ctx, params.TargetKind, params.TargetID); err != nil {
		log.Printf("asset: 置 RUNNING 失败 kind=%s target=%d: %v", params.TargetKind, params.TargetID, err)
	}
	target, err := h.store.LoadAssetTarget(ctx, params.TargetKind, params.TargetID)
	if err != nil {
		return nil, err
	}
	// 弹窗参数透传：临时提示词（图改图）优先于资产默认提示词
	target.Gen = params.Gen
	// 基线提示词清洗：剥离身体数据（身高/体重/年龄）+ 剥离【字段名】结构化标签
	// （标签模板是"画面内文字"的最强诱导源，必须转自然语言短句）
	prompt := flattenBracketLabels(stripBodyMetrics(target.Prompt))
	aspect, resolution := "R9_16", "1K"
	if params.Gen != nil {
		if params.Gen.Prompt != "" {
			prompt = params.Gen.Prompt
		}
		if params.Gen.AspectRatio != "" {
			aspect = params.Gen.AspectRatio
		}
		if params.Gen.Resolution != "" {
			resolution = params.Gen.Resolution
		}
	}

	// 场景时段/天气（对齐 LuxReal 场景日夜变体）：弹窗参数优先，批量生成回退场景自身枚举；
	// 把枚举翻译为画面描述并入提示词（翻译收口在 Go 侧，SQL 层旧字面量已致 22P02 故移除）
	if params.TargetKind == "SCENE" {
		tod, weather := "", ""
		if params.Gen != nil {
			tod, weather = params.Gen.TimeOfDay, params.Gen.Weather
		}
		if tod == "" {
			tod = target.TimeOfDay
		}
		if weather == "" {
			weather = target.Weather
		}
		if desc, ok := timeOfDayCNDesc[tod]; ok && desc != "" && !strings.Contains(prompt, desc) {
			prompt += "，" + desc
		}
		if desc, ok := weatherCNDesc[weather]; ok && desc != "" && !strings.Contains(prompt, desc) {
			prompt += "，" + desc
		}
	}

	// 风格注入（LuxReal prompt_suffix_static 机制）：风格后缀追加进提示词。
	// 批量顶层 style_name 由 asset_service 建任务时填入 gen.StyleName；
	// 未指定风格时注入默认写实约束（修复"通用写实出动漫脸"：此前无任何
	// 风格约束词，模型自由解读画风）。
	styleName := ""
	if params.Gen != nil {
		styleName = params.Gen.StyleName
	}
	if suffix, ok := stylePromptSuffix[styleName]; ok && suffix != "" {
		prompt += "。" + suffix
	} else if _, has := stylePromptSuffix[styleName]; !has {
		// 未知风格名（含空）：透传原词 + 默认写实兜底
		if styleName != "" {
			prompt += "。" + styleName + "风格"
		}
		prompt += "。" + defaultRealisticSuffix
	}

	// 角色默认构图（对齐 LuxReal）：默认开启，追加三视图构图模板（半身特写+正/侧/背），
	// 且未显式指定比例时用 16:9 横幅（竖图放不下 1 特写 + 3 视图）。场景/道具不追加。
	useDefaultComp := params.Gen != nil && params.Gen.UseDefaultComposition
	if params.TargetKind == "CHARACTER_VARIANT" {
		if params.Gen == nil {
			useDefaultComp = true // 批量生成（无弹窗参数）时按官方默认开启
		}
		if useDefaultComp {
			if !strings.HasSuffix(prompt, "。") && !strings.HasSuffix(prompt, ".") {
				prompt += "。"
			}
			prompt += charThreeViewSuffix
			if params.Gen == nil || params.Gen.AspectRatio == "" {
				aspect = "16:9"
			}
		}
	}

	// 全局无文字兜底（三视图模板已含同类约束则跳过）：防止模型把提示词中的
	// 文字性内容（身高/台词/标签）渲染进画面。
	if !strings.Contains(prompt, "不能出现") || !strings.Contains(prompt, "文字") {
		prompt += noTextSuffix
	}

	// 真实图生模型（B06 ModelEngine.GenerateImage）：配置了 T2I 厂商/Key 后走真实调用；
	// 无配置或调用失败降级 mock 占位图（开发期链路保持可用）。
	imageURL := ""
	isMock := true
	if h.engine != nil && prompt != "" {
		reqModel := ""
		refImage := ""
		if params.Gen != nil {
			reqModel = params.Gen.ModelCode // 弹窗选中的模型（路由到对应厂商/模型，勿丢弃）
			if len(params.Gen.RefImageURLs) > 0 {
				refImage = params.Gen.RefImageURLs[0] // 图改图：首参考图作为 I2I 输入
			}
		}
		resp, gerr := h.engine.GenerateImage(ctx, model.CapT2I, &model.ImageRequest{
			Prompt:      prompt,
			RefImageURL: refImage,
			AspectRatio: aspect,
			Resolution:  resolution,
			ModelKey:    reqModel,
		})
		if gerr == nil && resp.ImageURL != "" {
			imageURL = resp.ImageURL
			isMock = false
		} else {
			// 08 审查 Major#16：生产环境禁止降级 mock（避免按成功扣费给用户占位图）
			if h.env == "production" {
				return nil, fmt.Errorf("真实图生失败（生产禁用 mock 降级）: %w", gerr)
			}
			log.Printf("asset: 真实图生调用失败(%v)，降级 mock kind=%s target=%d", gerr, params.TargetKind, params.TargetID)
		}
	} else if h.env == "production" && (h.engine == nil || prompt == "") {
		// 生产环境未配置模型引擎/提示词为空：直接失败，不出占位图
		return nil, fmt.Errorf("生产环境未配置 T2I 模型或资产提示词为空")
	}
	if imageURL == "" {
		imageURL = fmt.Sprintf("%s/asset-placeholder.png?kind=%s&id=%d&t=%d",
			mockCDNBase(), params.TargetKind, params.TargetID, time.Now().UnixMilli())
	}
	// 真实生成图转存自有 MinIO（TOS 私有签名 24h 过期 → 图裂/视频拉不到参考图的根因）；
	// 转存失败保留原 URL 不阻断。mock 占位图不转存。
	if !isMock {
		imageURL = persistGeneratedImage(ctx, h.persist, target.ProjectID, params.TargetKind, params.TargetID, imageURL)
	}
	// 最终提示词摘要入日志（prompt 组装链路可观测：基线+变体描述+时段/天气+风格后缀+构图模板）
	log.Printf("asset: %s 生成图片 target=%d url=%s is_mock=%v prompt=<%s>", params.TargetKind, params.TargetID, imageURL, isMock, truncateStr(prompt, 160))

	// 事务入库（generated_images + 业务表 + usage log）
	if err := h.store.SaveGeneratedImage(ctx, target, imageURL, t.ID); err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]any{
		"image_url":   imageURL,
		"target_kind": params.TargetKind,
		"target_id":   params.TargetID,
		"is_mock":     isMock,
	})
	return out, nil
}
