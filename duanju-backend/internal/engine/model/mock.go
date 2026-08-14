// mock provider：开发期无真实 Key 时的模拟实现（05 工程流程四章：开发期用 mock provider / 沙箱 Key）。
package model

import (
	"context"
	"fmt"
	"sync/atomic"
)

// MockProvider 模拟厂商，可配置失败行为（用于 429 切换测试）。
type MockProvider struct {
	name         string
	caps         []Capability
	failNextN    atomic.Int64 // 剩余可失败次数（每次调用减 1，>0 时返回 429）
}

func NewMockProvider(name string, caps ...Capability) *MockProvider {
	return &MockProvider{name: name, caps: caps}
}

// FailNext 使接下来的 n 次调用返回 429（可重试），用于测试自动切 Key。
func (m *MockProvider) FailNext(n int) {
	m.failNextN.Store(int64(n))
}

func (m *MockProvider) Name() string { return m.name }

func (m *MockProvider) Capabilities() []Capability { return m.caps }

func (m *MockProvider) maybeFail() error {
	if m.failNextN.Load() > 0 {
		m.failNextN.Add(-1)
		return NewRetryableError(429, "mock rate limited")
	}
	return nil
}

func (m *MockProvider) Chat(_ context.Context, k *KeyCredential, req *ChatRequest) (*ChatResponse, error) {
	if err := m.maybeFail(); err != nil {
		return nil, err
	}
	content := fmt.Sprintf("[%s] echo: %s", m.name, req.Prompt)
	if req.JSONMode {
		// JSON 模式：返回预设的四层拆分结构（S1-B05 开发期 mock，字段对齐 02 A3 枚举）
		content = mockSplitJSON
	}
	return &ChatResponse{
		Content:   content,
		ModelUsed: k.ModelKey,
		KeyID:     k.ID,
	}, nil
}

func (m *MockProvider) GenerateImage(_ context.Context, k *KeyCredential, req *ImageRequest) (*ImageResponse, error) {
	if err := m.maybeFail(); err != nil {
		return nil, err
	}
	return &ImageResponse{
		ImageURL:  fmt.Sprintf("https://mock.example.com/img/%d.png", k.ID),
		ModelUsed: k.ModelKey,
		KeyID:     k.ID,
	}, nil
}

func (m *MockProvider) GenerateVideo(_ context.Context, k *KeyCredential, req *VideoRequest) (*VideoResponse, error) {
	if err := m.maybeFail(); err != nil {
		return nil, err
	}
	return &VideoResponse{
		ExternalTaskID: fmt.Sprintf("mock-task-%d-%d", k.ID, req.DurationSec),
		ModelUsed:      k.ModelKey,
		KeyID:          k.ID,
	}, nil
}

// mockSplitJSON 开发期拆分的 mock 输出（1 集 2 单元 4 块 8 镜头；枚举对齐 02 A3）。
const mockSplitJSON = `{
  "episodes": [
    {
      "title": "第1集 - 雨夜追击",
      "narrative_units": [
        {
          "title": "陆言深夜布防",
          "duration_sec": 12,
          "script_original_text": "雨夜，陆言在巷口埋伏，等待目标出现。",
          "shot_groups": [
            {
              "title": "巷口埋伏",
              "duration_sec": 6,
              "shots": [
                {"duration_sec": 3, "shot_size": "CU", "camera_movement": "SLOW_PUSH_IN", "camera_angle": "EYE_LEVEL_FRONT", "subject_description": "陆言眼神锐利地盯着巷口", "background_description": "昏暗的雨巷，霓虹灯闪烁", "local_light_description": "侧光勾勒面部轮廓", "dialogue_text": "", "ambient_sound": "雨声、远处警笛"},
                {"duration_sec": 3, "shot_size": "MS", "camera_movement": "PAN_RIGHT", "camera_angle": "SIDE", "subject_description": "目标身影出现在巷口", "background_description": "雨幕中路灯昏黄", "local_light_description": "背光剪影", "dialogue_text": "", "ambient_sound": "雨声渐大"}
              ]
            },
            {
              "title": "短兵相接",
              "duration_sec": 6,
              "shots": [
                {"duration_sec": 3, "shot_size": "FS", "camera_movement": "DOLLY_IN", "camera_angle": "EYE_LEVEL_FRONT", "subject_description": "两人雨中对峙", "background_description": "积水倒映霓虹", "local_light_description": "顶光雨丝可见", "dialogue_text": "你终于来了。", "ambient_sound": "雨声、脚步踏水"},
                {"duration_sec": 3, "shot_size": "ECU", "camera_movement": "STATIC", "camera_angle": "SLIGHT_LOW_ANGLE", "subject_description": "目标手伸向腰间", "background_description": "模糊的雨巷", "local_light_description": "低角度逆光", "dialogue_text": "", "ambient_sound": "金属摩擦声"}
              ]
            }
          ]
        },
        {
          "title": "巷口追捕",
          "duration_sec": 10,
          "script_original_text": "目标逃窜，陆言紧追不舍，翻过矮墙。",
          "shot_groups": [
            {
              "title": "追逐",
              "duration_sec": 5,
              "shots": [
                {"duration_sec": 3, "shot_size": "LS", "camera_movement": "TRACK_RIGHT", "camera_angle": "SIDE", "subject_description": "两人一前一后奔跑", "background_description": "湿滑街道，水花四溅", "local_light_description": "路灯连续闪过", "dialogue_text": "", "ambient_sound": "急促脚步、喘息"},
                {"duration_sec": 2, "shot_size": "MS", "camera_movement": "HANDHELD_SHAKY", "camera_angle": "DUTCH_ANGLE", "subject_description": "目标回头张望", "background_description": "雨幕模糊", "local_light_description": "脸侧闪电光", "dialogue_text": "", "ambient_sound": "雷声"}
              ]
            },
            {
              "title": "翻墙",
              "duration_sec": 5,
              "shots": [
                {"duration_sec": 3, "shot_size": "FS", "camera_movement": "CRANE_UP", "camera_angle": "SLIGHT_HIGH", "subject_description": "目标翻上矮墙", "background_description": "墙后是一片荒地", "local_light_description": "月光破云", "dialogue_text": "", "ambient_sound": "布料摩擦、落地闷响"},
                {"duration_sec": 2, "shot_size": "CU", "camera_movement": "FAST_PUSH_IN", "camera_angle": "EYE_LEVEL_FRONT", "subject_description": "陆言咬牙纵身跃起", "background_description": "雨幕中的矮墙", "local_light_description": "侧逆光", "dialogue_text": "站住！", "ambient_sound": "喊声、雷雨"}
              ]
            }
          ]
        }
      ]
    }
  ],
  "characters": [
    {"code": "LU_YAN", "name": "陆言", "tags": "主角,刑警", "description_notes": "雨夜追击案的执行者，身手矫健，意志坚定。", "origin_episode_id": 1},
    {"code": "TARGET", "name": "神秘目标", "tags": "反派,悬念", "description_notes": "身份不明，行为诡异，与陆言在雨夜对峙后逃窜。", "origin_episode_id": 1}
  ]
}`
