// OpenAI 兼容 Provider：通过 /chat/completions 调用真实 LLM（DeepSeek/Qwen/GLM 等）。
// KeyCredential.APIKey 为已解密明文；BaseURL 来自 model_providers.base_url。
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatProvider OpenAI 兼容厂商（Chat 能力；图/视频由其他 Provider 实现）。
type OpenAICompatProvider struct {
	name string
	caps []Capability
	http *http.Client
}

func NewOpenAICompatProvider(name string, caps ...Capability) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		name: name,
		caps: caps,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAICompatProvider) Name() string { return p.name }

func (p *OpenAICompatProvider) Capabilities() []Capability { return p.caps }

// Chat 调用 /chat/completions（支持 JSON mode：response_format=json_object）。
func (p *OpenAICompatProvider) Chat(ctx context.Context, k *KeyCredential, req *ChatRequest) (*ChatResponse, error) {
	messages := []map[string]string{}
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})

	body := map[string]any{
		"model":       k.ModelKey,
		"messages":    messages,
		"temperature": req.Temperature,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.JSONMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	payload, _ := json.Marshal(body)

	endpoint := strings.TrimRight(k.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+k.APIKey)

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, NewRetryableError(0, "http: "+err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return nil, NewRetryableError(resp.StatusCode, "provider http "+resp.Status)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("provider error %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return nil, NewRetryableError(0, "empty provider content")
	}
	return &ChatResponse{Content: out.Choices[0].Message.Content, ModelUsed: k.ModelKey, KeyID: k.ID}, nil
}

// GenerateImage / GenerateVideo 未实现（需接入对应厂商 SDK，暂返回不可重试错误）。
func (p *OpenAICompatProvider) GenerateImage(_ context.Context, _ *KeyCredential, _ *ImageRequest) (*ImageResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持图片生成", p.name)
}

func (p *OpenAICompatProvider) GenerateVideo(_ context.Context, _ *KeyCredential, _ *VideoRequest) (*VideoResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持视频生成", p.name)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
