// MinimaxH3Provider：MiniMax H3 视频生成 Provider（对齐官方 V2 API 规范）。
// 规范要点（platform.minimaxi.com/docs/api-reference/video-generation-v2-create）：
//   - POST /v2/video_generation（创建，返回 task_id）+ GET /v2/query/video_generation（轮询）
//   - 多模态 content[]：text（必填非空）+ image_url（role 为顶层字段，与 type 平级）
//   - 多模态参考 R2V：参考图 role=reference_image ≤9 张；reference_* 与 first/last_frame 互斥
//   - duration 4~15 整数必填；resolution 枚举 768P/2K；纯文生 ratio 必填（不可 adaptive）
//   - 轮询响应嵌套 task 对象：task.status=succeeded → 取 task.content.url
//   - V2 错误体：{"error":{"message":"..."},"request_id":"..."}
//
// 与 AsyncTaskProvider（火山/可灵形态）互补：DB 配置厂商 name=minimax 时按名匹配注册。
// 官方 API 与 AutoDL SGLang 自部署共用 V2 契约，仅 BaseURL/Key 不同（管理端切换即可）。
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

// MinimaxH3Provider MiniMax H3 视频厂商。
type MinimaxH3Provider struct {
	name     string
	caps     []Capability
	http     *http.Client
	create   string // 创建任务路径（默认 /v2/video_generation）
	query    string // 查询任务路径（默认 /v2/query/video_generation）
	interval time.Duration
	timeout  time.Duration
}

// NewMinimaxH3Provider 构造（路径留空用官方默认）。
func NewMinimaxH3Provider(name string, caps []Capability, createPath, queryPath string) *MinimaxH3Provider {
	p := &MinimaxH3Provider{
		name:     name,
		caps:     caps,
		http:     &http.Client{Timeout: 60 * time.Second},
		create:   createPath,
		query:    queryPath,
		interval: 5 * time.Second,
		timeout:  30 * time.Minute,
	}
	if p.create == "" {
		p.create = "/v2/video_generation"
	}
	if p.query == "" {
		p.query = "/v2/query/video_generation"
	}
	return p
}

func (p *MinimaxH3Provider) Name() string               { return p.name }
func (p *MinimaxH3Provider) Capabilities() []Capability { return p.caps }

// maxH3RefImages 全能参考图上限（官方规范 ≤9 张，混合输入 ≤12 文件）。
const maxH3RefImages = 9

// maxH3PromptChars 提示词上限（官方 7000 字符，预留尾部约束余量）。
const maxH3PromptChars = 6500

// GenerateVideo 创建 H3 任务 → 轮询 → 返回成片 URL。
func (p *MinimaxH3Provider) GenerateVideo(ctx context.Context, k *KeyCredential, req *VideoRequest) (*VideoResponse, error) {
	useModel := req.ModelKey
	if useModel == "" {
		useModel = k.ModelKey
	}
	if useModel == "" {
		useModel = "MiniMax-H3"
	}

	// content[] 多模态结构：text（V2 必填非空）在前，参考图依次排列。
	// role 为顶层字段（与 type 平级，不在 image_url 内）；多图参考统一 reference_image
	// ——V2 强校验 reference_* 与 first/last_frame 互斥，本 Provider 全走参考模式。
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("minimax h3: prompt 为空")
	}
	prompt := req.Prompt
	if len(prompt) > maxH3PromptChars {
		prompt = prompt[:maxH3PromptChars]
	}
	content := make([]map[string]any, 0, 1+maxH3RefImages)
	content = append(content, map[string]any{"type": "text", "text": prompt})
	refs := dedupeURLs(req.RefImageURLs)
	if len(refs) == 0 && req.RefImageURL != "" {
		refs = []string{req.RefImageURL}
	}
	if len(refs) > maxH3RefImages {
		refs = refs[:maxH3RefImages]
	}
	for _, u := range refs {
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": u},
			"role":      "reference_image",
		})
	}

	// duration V2 必填（4~15 整数）：0 → 默认 5s；越界 clamp（镜头时长来自分镜，可能为 3s/20s）。
	duration := req.DurationSec
	if duration <= 0 {
		duration = 5
	}
	if duration < 4 {
		duration = 4
	}
	if duration > 15 {
		duration = 15
	}

	// resolution 归一化为 V2 枚举：仅显式 2K（大小写不敏感）为 2K，
	// 其余（P480/P720/P1080/空等历史值）统一回落 768P（自部署 SGLang 也仅支持 768P）。
	resolution := "768P"
	if strings.EqualFold(strings.TrimSpace(req.Resolution), "2K") {
		resolution = "2K"
	}

	body := map[string]any{
		"model":          useModel,
		"content":        content,
		"duration":       duration,
		"resolution":     resolution,
		"aigc_watermark": false,
	}
	// ratio 规则（V2）：纯文生（无参考图）必填且不可 adaptive → 短剧默认 9:16 竖屏；
	// 带参考图（R2V）由参考输入自适应（默认 adaptive），不传。
	if len(refs) == 0 {
		body["ratio"] = "9:16"
	}

	taskID, err := p.createTask(ctx, k, body)
	if err != nil {
		return nil, err
	}
	url, err := p.pollTask(ctx, k, taskID)
	if err != nil {
		return nil, err
	}
	return &VideoResponse{ExternalTaskID: taskID, VideoURL: url, ModelUsed: useModel, KeyID: k.ID}, nil
}

// GenerateImage H3 为视频模型，不支持图片生成。
func (p *MinimaxH3Provider) GenerateImage(_ context.Context, _ *KeyCredential, _ *ImageRequest) (*ImageResponse, error) {
	return nil, fmt.Errorf("provider %s 仅支持视频生成", p.name)
}

// Chat 不支持。
func (p *MinimaxH3Provider) Chat(_ context.Context, _ *KeyCredential, _ *ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持对话", p.name)
}

// createTask POST 创建任务（官方返回 {"task_id": "..."}）。
func (p *MinimaxH3Provider) createTask(ctx context.Context, k *KeyCredential, body map[string]any) (string, error) {
	payload, _ := json.Marshal(body)
	endpoint := strings.TrimRight(k.BaseURL, "/") + p.create
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
	var parsed struct {
		TaskID   string `json:"task_id"`
		BaseResp *struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
	}
	// V2 错误体：{"type":"error","error":{"message":"..."}}（200/4xx 均可能出现）
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("minimax error: %s", parsed.Error.Message)
	}
	if parsed.BaseResp != nil && parsed.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("minimax error %d: %s", parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}
	if parsed.TaskID == "" {
		return "", NewRetryableError(0, "provider 未返回 task_id: "+truncate(string(raw), 200))
	}
	return parsed.TaskID, nil
}

// pollTask 轮询任务直至成功/失败/超时（官方 v2 成功态 status=Succeed，成片在 content.url）。
func (p *MinimaxH3Provider) pollTask(ctx context.Context, k *KeyCredential, taskID string) (string, error) {
	endpoint := strings.TrimRight(k.BaseURL, "/") + p.query + "?task_id=" + taskID
	deadline := time.Now().Add(p.timeout)
	for {
		if time.Now().After(deadline) {
			return "", NewRetryableError(0, fmt.Sprintf("video 任务超时(%s)", p.timeout.String()))
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+k.APIKey)
		resp, err := p.http.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.interval):
				continue
			}
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.interval):
				continue
			}
		}
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("provider error %d: %s", resp.StatusCode, truncate(string(raw), 200))
		}
		var parsed struct {
			Status  string `json:"status"`
			Content *struct {
				URL string `json:"url"`
			} `json:"content"`
			Task *struct {
				Status  string `json:"status"`
				Content *struct {
					URL string `json:"url"`
				} `json:"content"`
			} `json:"task"`
			BaseResp *struct {
				StatusCode int    `json:"status_code"`
				StatusMsg  string `json:"status_msg"`
			} `json:"base_resp"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
		}
		// V2 查询响应：{"task":{"status":"succeeded","content":{"url":"..."}}}
		// 兼容扁平结构（status/content 顶层）与 V1 base_resp 错误体。
		status := parsed.Status
		resultURL := ""
		if parsed.Content != nil {
			resultURL = parsed.Content.URL
		}
		if parsed.Task != nil {
			if parsed.Task.Status != "" {
				status = parsed.Task.Status
			}
			if parsed.Task.Content != nil && parsed.Task.Content.URL != "" {
				resultURL = parsed.Task.Content.URL
			}
		}
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", NewRetryableError(0, "查询失败: "+parsed.Error.Message)
		}
		switch strings.ToLower(status) {
		case "succeed", "success", "succeeded":
			if resultURL == "" {
				return "", NewRetryableError(0, "任务成功但无结果 URL: "+truncate(string(raw), 200))
			}
			return resultURL, nil
		case "failed", "failure":
			msg := ""
			if parsed.BaseResp != nil {
				msg = parsed.BaseResp.StatusMsg
			}
			return "", NewRetryableError(0, "video 任务失败: "+msg)
		case "canceled", "cancelled":
			return "", NewRetryableError(0, "任务被取消")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(p.interval):
		}
	}
}
