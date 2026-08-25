// VolcImageProvider：火山方舟 Seedream 同步图片生成。
// 端点 POST {base}/api/v3/images/generations（同步返回图片 URL，约 5-15s）。
// 注意：Seedream 系列不支持 /api/v3/contents/generations/tasks（报 not support content generation）。
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

// volcMinPixels Seedream 4.5 最小像素数（3686400 = 1920x1920）。
const volcMinPixels = 3686400

// volcPixelSize 比例/分辨率 → Seedream 像素尺寸（保证 ≥ 最小像素）。
// 输入兼容 "16:9"/"R16_9" 比例格式；分辨率 1K/2K/4K 档位（4K 提升一档）。
func volcPixelSize(ratio, resolution string) string {
	r := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(ratio), "R"))
	w, h := 1920, 1920
	switch r {
	case "16:9":
		w, h = 2560, 1440
	case "9:16":
		w, h = 1440, 2560
	case "1:1":
		w, h = 1920, 1920
	case "4:3":
		w, h = 2560, 1920
	case "3:4":
		w, h = 1920, 2560
	case "3:2":
		w, h = 2688, 1792
	case "2:3":
		w, h = 1792, 2688
	}
	if strings.EqualFold(resolution, "4K") {
		// 4K 档：长边 3840（保持比例，像素充足）
		if w >= h {
			h = h * 3840 / w
			w = 3840
		} else {
			w = w * 3840 / h
			h = 3840
		}
	}
	return fmt.Sprintf("%dx%d", w, h)
}

// VolcImageProvider 同步图片 Provider（火山方舟 Seedream / Ark images API）。
type VolcImageProvider struct {
	name       string
	caps       []Capability
	http       *http.Client
	createPath string
}

// NewVolcImageProvider 创建同步图片 Provider；createPath 空时默认 /api/v3/images/generations。
func NewVolcImageProvider(name string, caps []Capability, createPath string) *VolcImageProvider {
	if createPath == "" {
		createPath = "/api/v3/images/generations"
	}
	return &VolcImageProvider{
		name:       name,
		caps:       caps,
		http:       &http.Client{Timeout: 300 * time.Second},
		createPath: createPath,
	}
}

func (p *VolcImageProvider) Name() string               { return p.name }
func (p *VolcImageProvider) Capabilities() []Capability { return p.caps }

func (p *VolcImageProvider) Chat(_ context.Context, _ *KeyCredential, _ *ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持对话", p.name)
}

func (p *VolcImageProvider) GenerateVideo(_ context.Context, _ *KeyCredential, _ *VideoRequest) (*VideoResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持视频生成", p.name)
}

// GenerateImage 文生图/图生图（同步）：POST images/generations → data[0].url。
func (p *VolcImageProvider) GenerateImage(ctx context.Context, k *KeyCredential, req *ImageRequest) (*ImageResponse, error) {
	useModel := req.ModelKey
	if useModel == "" {
		useModel = k.ModelKey
	}
	body := map[string]any{
		"model":           useModel,
		"prompt":          req.Prompt,
		"size":            volcPixelSize(req.AspectRatio, req.Resolution),
		"response_format": "url",
		"watermark":       false,
	}
	if req.RefImageURL != "" {
		body["image"] = []string{req.RefImageURL} // Seedream 图生图参考图
	}
	payload, _ := json.Marshal(body)
	endpoint := strings.TrimRight(k.BaseURL, "/") + p.createPath
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
	var parsed struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("provider error: %s", parsed.Error.Message)
	}
	if len(parsed.Data) == 0 || parsed.Data[0].URL == "" {
		return nil, NewRetryableError(0, "provider 未返回图片 URL: "+truncate(string(raw), 200))
	}
	return &ImageResponse{ImageURL: parsed.Data[0].URL, ModelUsed: useModel, KeyID: k.ID}, nil
}
