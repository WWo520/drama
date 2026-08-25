// AssetHandler：Worker-Asset（CHAR_IMAGE_GEN/SCENE_IMAGE_GEN/PROP_IMAGE_GEN 执行器）。
// 开发期 mock 图片生成（生成占位图 URL）；生产接入图生模型（B06 ModelEngine.GenerateImage）。
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

// AssetStore 资产生成边界（由 repository.AssetStore 实现）。
type AssetStore interface {
	// LoadAssetTarget 解析任务目标（kind/target_id → 项目/集/积分）。
	LoadAssetTarget(ctx context.Context, targetKind string, targetID int64) (*AssetTarget, error)
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
	Gen       *GenParams // 弹窗生成参数（可为 nil）
}

// AssetHandler 实现 task.Handler（三种资产生成任务）。
type AssetHandler struct {
	store  AssetStore
	engine ImageGenClient // 可为 nil：未注入时降级 mock（开发期）
	env    string         // 08 审查 Major#16：生产环境禁止 mock 降级
}

// ImageGenClient 图生模型能力（由 ModelEngine 实现注入）。
type ImageGenClient interface {
	GenerateImage(ctx context.Context, capability model.Capability, req *model.ImageRequest) (*model.ImageResponse, error)
}

// charThreeViewSuffix 角色默认构图模板（对齐 LuxReal 实测原文）：
// 一张 16:9 图内 = 左侧半身特写 + 右侧正/侧/背三个全身视图，纯白背景无文字。
const charThreeViewSuffix = "生成该主体的全身三视图：正面视图、侧面视图和背面视图。具体布局要求：图纸必须包含角色的三视图，图像左侧为主体上半身特写，图像右侧展示主体的三个视图。纯白色背景，画面仅展示主体上半身特写和三视图，画面中不能出现文字"

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
	target, err := h.store.LoadAssetTarget(ctx, params.TargetKind, params.TargetID)
	if err != nil {
		return nil, err
	}
	// 弹窗参数透传：临时提示词（图改图）优先于资产默认提示词
	target.Gen = params.Gen
	prompt := target.Prompt
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

	// 场景时段/天气（对齐 LuxReal 场景日夜变体）：把枚举翻译为画面描述并入提示词
	if params.TargetKind == "SCENE" && params.Gen != nil {
		if params.Gen.TimeOfDay != "" {
			if desc, ok := timeOfDayCNDesc[params.Gen.TimeOfDay]; ok && !strings.Contains(prompt, desc) {
				prompt += "，" + desc
			}
		}
		if params.Gen.Weather != "" {
			if desc, ok := weatherCNDesc[params.Gen.Weather]; ok && !strings.Contains(prompt, desc) {
				prompt += "，" + desc
			}
		}
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
	log.Printf("asset: %s 生成图片 target=%d url=%s is_mock=%v", params.TargetKind, params.TargetID, imageURL, isMock)

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
