// AsyncTaskProvider：通用异步生成 Provider（火山引擎即梦/可灵风格）。
// 流程：POST 创建任务（返回 task_id）→ 周期轮询任务状态 → succeeded 后提取结果 URL。
// 与 OpenAI 兼容 LLM Provider 互补：本 Provider 覆盖 T2I/T2V/I2V 异步生成能力；
// 端点路径与响应结构按厂商构造参数配置（兼容火山 visual API 与可灵开放平台两种形态）。
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

// AsyncTaskProvider 异步生成厂商（创建任务 + 轮询查询）。
type AsyncTaskProvider struct {
	name string
	caps []Capability
	http *http.Client
	// 端点模板：{base} 由 KeyCredential.BaseURL 提供；{id} 为任务 ID
	imageCreatePath string // 文生图创建任务路径，如 /api/v1/images/generations
	imageQueryPath  string // 文生图查询路径，如 /api/v1/images/generations/{id}
	videoCreatePath string // 文生视频创建任务路径，如 /api/v1/videos/generations
	videoQueryPath  string // 文生视频查询路径，如 /api/v1/videos/generations/{id}
	pollInterval    time.Duration
	imageTimeout    time.Duration
	videoTimeout    time.Duration
}

// AsyncProviderOption 构造选项。
type AsyncProviderOption struct {
	ImageCreatePath string
	ImageQueryPath  string
	VideoCreatePath string
	VideoQueryPath  string
	// PollInterval 轮询间隔（默认 5s）
	PollInterval time.Duration
	// ImageTimeout 图片任务超时（默认 10min）
	ImageTimeout time.Duration
	// VideoTimeout 视频任务超时（默认 30min）
	VideoTimeout time.Duration
}

// NewAsyncTaskProvider 构造异步厂商 Provider（支持图片/视频能力，按传入路径启用）。
func NewAsyncTaskProvider(name string, caps []Capability, opt AsyncProviderOption) *AsyncTaskProvider {
	p := &AsyncTaskProvider{
		name:            name,
		caps:            caps,
		http:            &http.Client{Timeout: 60 * time.Second},
		imageCreatePath: opt.ImageCreatePath,
		imageQueryPath:  opt.ImageQueryPath,
		videoCreatePath: opt.VideoCreatePath,
		videoQueryPath:  opt.VideoQueryPath,
		pollInterval:    opt.PollInterval,
		imageTimeout:    opt.ImageTimeout,
		videoTimeout:    opt.VideoTimeout,
	}
	if p.pollInterval <= 0 {
		p.pollInterval = 5 * time.Second
	}
	if p.imageTimeout <= 0 {
		p.imageTimeout = 10 * time.Minute
	}
	if p.videoTimeout <= 0 {
		p.videoTimeout = 30 * time.Minute
	}
	return p
}

func (p *AsyncTaskProvider) Name() string               { return p.name }
func (p *AsyncTaskProvider) Capabilities() []Capability { return p.caps }

// normalizeVolcSize 比例格式归一化：前端/弹窗传 "16:9"，火山即梦 API 要求 "R16_9"。
func normalizeVolcSize(ratio string) string {
	if ratio == "" || strings.HasPrefix(ratio, "R") || strings.HasPrefix(ratio, "r") {
		return ratio
	}
	if w, h, ok := strings.Cut(ratio, ":"); ok && w != "" && h != "" {
		return "R" + w + "_" + h
	}
	return ratio
}

// GenerateImage 文生图：创建任务 → 轮询 → 返回结果 URL（同步阻塞，适配 worker 单任务模型）。
func (p *AsyncTaskProvider) GenerateImage(ctx context.Context, k *KeyCredential, req *ImageRequest) (*ImageResponse, error) {
	if p.imageCreatePath == "" {
		return nil, fmt.Errorf("provider %s 未配置图片生成端点", p.name)
	}
	// 模型取值：调用方指定优先；为空回退 keystore 按 models 表回填的默认模型（避免空 model 被 API 拒绝）。
	useModel := req.ModelKey
	if useModel == "" {
		useModel = k.ModelKey
	}
	body := map[string]any{"prompt": req.Prompt, "model": useModel}
	if req.AspectRatio != "" {
		body["size"] = normalizeVolcSize(req.AspectRatio) // 火山即梦：R16_9/R9_16/R1_1
	}
	if req.Resolution != "" {
		body["resolution"] = req.Resolution
	}
	taskID, err := p.createTask(ctx, k, p.imageCreatePath, body)
	if err != nil {
		return nil, err
	}
	url, err := p.pollTask(ctx, k, p.imageQueryPath, taskID, p.imageTimeout, "image")
	if err != nil {
		return nil, err
	}
	return &ImageResponse{ImageURL: url, ModelUsed: useModel, KeyID: k.ID}, nil
}

// GenerateVideo 文生视频：创建任务 → 轮询 → 返回结果 URL（同步阻塞）。
func (p *AsyncTaskProvider) GenerateVideo(ctx context.Context, k *KeyCredential, req *VideoRequest) (*VideoResponse, error) {
	if p.videoCreatePath == "" {
		return nil, fmt.Errorf("provider %s 未配置视频生成端点", p.name)
	}
	// 模型取值：调用方指定优先；为空回退 keystore 按 models 表回填的默认模型。
	useModel := req.ModelKey
	if useModel == "" {
		useModel = k.ModelKey
	}
	body := map[string]any{"prompt": req.Prompt, "model": useModel}
	if req.DurationSec > 0 {
		body["duration"] = req.DurationSec // 秒；部分厂商用 5/10 枚举
	}
	if req.Resolution != "" {
		body["resolution"] = req.Resolution // P480/P720/P1080
	}
	// F10 参考图：RefImageURLs 多图优先，RefImageURL 单图向后兼容。
	// 火山 ark：image_url 收 string（单图首帧）或 []string（多图参考，1~4 张）。
	refs := dedupeURLs(req.RefImageURLs)
	if len(refs) == 0 && req.RefImageURL != "" {
		refs = []string{req.RefImageURL}
	}
	if len(refs) == 1 {
		body["image_url"] = refs[0]
	} else if len(refs) > 1 {
		body["image_url"] = refs
	}
	taskID, err := p.createTask(ctx, k, p.videoCreatePath, body)
	if err != nil {
		return nil, err
	}
	url, err := p.pollTask(ctx, k, p.videoQueryPath, taskID, p.videoTimeout, "video")
	if err != nil {
		return nil, err
	}
	return &VideoResponse{ExternalTaskID: taskID, VideoURL: url, ModelUsed: useModel, KeyID: k.ID}, nil
}

// Chat 不支持（由 LLM Provider 提供）。
func (p *AsyncTaskProvider) Chat(_ context.Context, _ *KeyCredential, _ *ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持对话", p.name)
}

// dedupeURLs URL 列表去重保序（跳过空值；参考图来源可能重复命中单元级+镜头级）。
func dedupeURLs(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// createTask POST 创建异步任务，解析 task_id（兼容火山 {id} 与可灵 {data:{task_id}}）。
func (p *AsyncTaskProvider) createTask(ctx context.Context, k *KeyCredential, path string, body map[string]any) (string, error) {
	payload, _ := json.Marshal(body)
	endpoint := strings.TrimRight(k.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.APIKey)
	resp, err := p.http.Do(req)
	if err != nil {
		return "", NewRetryableError(0, "http: "+err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return "", NewRetryableError(resp.StatusCode, "provider http "+resp.Status)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("provider error %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
	}
	// 火山：{"id": "..."}；可灵：{"data":{"task_id":"..."}}
	if id, ok := parsed["id"].(string); ok && id != "" {
		return id, nil
	}
	if data, ok := parsed["data"].(map[string]any); ok {
		if id, ok := data["task_id"].(string); ok && id != "" {
			return id, nil
		}
	}
	return "", NewRetryableError(0, "provider 未返回 task_id: "+truncate(string(raw), 200))
}

// pollTask 轮询任务状态直至 succeeded/failed/超时；提取结果 URL。
func (p *AsyncTaskProvider) pollTask(ctx context.Context, k *KeyCredential, queryPath, taskID string, timeout time.Duration, kind string) (string, error) {
	endpoint := strings.TrimRight(k.BaseURL, "/") + strings.ReplaceAll(queryPath, "{id}", taskID)
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return "", NewRetryableError(0, fmt.Sprintf("%s 任务超时(%s)", kind, timeout.String()))
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+k.APIKey)
		resp, err := p.http.Do(req)
		if err != nil {
			// 查询瞬断：短暂等待后重试（不切换 Key）
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.pollInterval):
				continue
			}
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			// 08 审查 Major#17：轮询阶段瞬时 429/5xx 等待后继续（不终止整个异步任务，避免重试重建厂商任务重复计费）
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.pollInterval):
				continue
			}
		}
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("provider error %d: %s", resp.StatusCode, truncate(string(raw), 200))
		}
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
		}
		status := extractTaskStatus(parsed)
		switch status {
		case "succeeded", "success", "succeed":
			url := extractResultURL(parsed)
			if url == "" {
				return "", NewRetryableError(0, fmt.Sprintf("%s 任务成功但无结果 URL: %s", kind, truncate(string(raw), 200)))
			}
			return url, nil
		case "failed", "failure":
			return "", NewRetryableError(0, fmt.Sprintf("%s 任务失败: %s", kind, truncate(string(raw), 200)))
		case "canceled", "cancelled":
			return "", NewRetryableError(0, fmt.Sprintf("%s 任务被取消", kind))
		}
		// queued/running/processing → 继续轮询
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.pollInterval):
		}
	}
}

// extractTaskStatus 兼容多种状态字段形态（status / task_status / data.status）。
func extractTaskStatus(parsed map[string]any) string {
	for _, key := range []string{"status", "task_status"} {
		if v, ok := parsed[key].(string); ok && v != "" {
			return strings.ToLower(v)
		}
	}
	if data, ok := parsed["data"].(map[string]any); ok {
		if v, ok := data["status"].(string); ok && v != "" {
			return strings.ToLower(v)
		}
		if v, ok := data["task_status"].(string); ok && v != "" {
			return strings.ToLower(v)
		}
	}
	return ""
}

// extractResultURL 兼容多种结果形态：
// 火山：{"content":[{"url":"..."}]}；可灵：{"data":{"task_result":{"videos":[{"url":"..."}]}}}
// 兼容：{"data":{"image":{"url":"..."}}} / {"data":{"video_url":"..."}}
func extractResultURL(parsed map[string]any) string {
	if content, ok := parsed["content"].([]any); ok {
		for _, c := range content {
			if m, ok := c.(map[string]any); ok {
				if u, ok := m["url"].(string); ok && u != "" {
					return u
				}
			}
		}
	}
	data, _ := parsed["data"].(map[string]any)
	if data == nil {
		return ""
	}
	if tr, ok := data["task_result"].(map[string]any); ok {
		for _, listKey := range []string{"videos", "images"} {
			if list, ok := tr[listKey].([]any); ok {
				for _, c := range list {
					if m, ok := c.(map[string]any); ok {
						if u, ok := m["url"].(string); ok && u != "" {
							return u
						}
					}
				}
			}
		}
		if u, ok := tr["url"].(string); ok && u != "" {
			return u
		}
	}
	if img, ok := data["image"].(map[string]any); ok {
		if u, ok := img["url"].(string); ok && u != "" {
			return u
		}
	}
	if u, ok := data["video_url"].(string); ok && u != "" {
		return u
	}
	if u, ok := data["url"].(string); ok && u != "" {
		return u
	}
	return ""
}
