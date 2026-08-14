// Package model 模型治理引擎（E2 ModelEngine）：Provider 接口 + Key 三级路由 + 单 Key 信号量 + 429 自动切换 + 降级链。
// 对齐 01 C.2.2 / 04 E 章。配置真源：model_providers / models / model_keys 表。
package model

import (
	"context"
	"errors"
)

// Capability 模型能力（对齐 model_capability_enum）。
type Capability string

const (
	CapLLM   Capability = "LLM"
	CapT2I   Capability = "T2I"
	CapI2I   Capability = "I2I"
	CapT2V   Capability = "T2V"
	CapI2V   Capability = "I2V"
	CapAudio Capability = "AUDIO"
)

// ErrNoAvailableKey 全部 Key 不可用（模型全部维护中）。
var ErrNoAvailableKey = errors.New("model: no available key")

// ErrProviderAllFailed 全部厂商调用失败。
var ErrProviderAllFailed = errors.New("model: all providers failed")

// ChatRequest LLM 对话请求。
type ChatRequest struct {
	Prompt      string
	System      string
	ModelKey    string // 期望模型 key（可为空，走默认路由）
	MaxTokens   int
	Temperature float64
	JSONMode    bool // 结构化输出
}

// ChatResponse LLM 响应。
type ChatResponse struct {
	Content    string
	ModelUsed  string // 实际使用模型 key
	KeyID      int64
}

// ImageRequest 文生图/图生图请求。
type ImageRequest struct {
	Prompt         string
	RefImageURL    string // I2I 参考图
	AspectRatio    string // R16_9 / R9_16 / R1_1 ...
	Resolution     string // 1K / 2K / 4K
	Seed           int64
	ModelKey       string
}

// ImageResponse 图片生成响应。
type ImageResponse struct {
	ImageURL  string
	ModelUsed string
	KeyID     int64
}

// VideoRequest 文生视频请求。
type VideoRequest struct {
	Prompt      string
	RefImageURL string
	DurationSec int
	Resolution  string // P480/P720/P1080
	ModelKey    string
}

// VideoResponse 视频生成响应（异步任务提交后回调，返回外部任务 ID）。
type VideoResponse struct {
	ExternalTaskID string
	ModelUsed      string
	KeyID          int64
}

// KeyCredential 单 Key 运行时快照（从 model_keys 表加载，Key 已解密）。
type KeyCredential struct {
	ID              int64
	ProviderName    string
	ModelKey        string
	APIKey          string
	BaseURL         string
	Concurrency     int
	Priority        int
	Enabled         bool
}

// Provider 厂商统一接口（E2 输入 → 输出）。
type Provider interface {
	Name() string
	Capabilities() []Capability
	Chat(ctx context.Context, k *KeyCredential, req *ChatRequest) (*ChatResponse, error)
	GenerateImage(ctx context.Context, k *KeyCredential, req *ImageRequest) (*ImageResponse, error)
	GenerateVideo(ctx context.Context, k *KeyCredential, req *VideoRequest) (*VideoResponse, error)
}

// KeyStore 从 DB 读取 Key 配置（表驱动路由）。
type KeyStore interface {
	// AvailableKeys 返回指定能力 + 模型 key 的可启用 Key 列表（按 priority 升序）。
	AvailableKeys(ctx context.Context, capability Capability, modelKey string) ([]*KeyCredential, error)
	// MarkKeyUnavailable 429/5xx 连续失败后标记 Key 临时不可用（30s 冷却）。
	MarkKeyUnavailable(ctx context.Context, keyID int64, reason string) error
}
