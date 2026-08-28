// AutodlComfyProvider：AutoDL.Art 托管 ComfyUI API Provider（免开机 GPU 实例、按秒计费）。
// 文档：https://autodl.art/docs/comfyui_api/ 工作流广场：https://autodl.art/large-model/comfyui
//   - 第一步 POST {base_url} 提交任务（返回 task_id）
//   - 第二步 GET  {base_url}/result/{task_id} 异步轮询（QUEUED/RUNNING/SUCCESS/FAILED）
//   - 鉴权：Authorization: <Token>（autodl.art/large-model/tokens 创建令牌，分组选 ComfyUI）
//   - 结果：data.results[] 内资源 URL（⚠️ 短时效，官方要求获取后尽快下载保存）
//
// 同族双工作流智能路由（H3 系）：
//   - 有参考图 → minimax_h3_lightx2v_v5（H3多图参考生视频）：ref_image_0 必填、最多 9 张、
//     duration 1-10s、分辨率含 768p竖（限时与 480p 同价）
//   - 无参考图 → minimax_h3_lightx2v_no_pic（H3文生视频）：无图入参、duration 4-15s、
//     分辨率仅 480p竖/480p横
//     v5 因 ref_image_0 必填无法承接纯文生视频，故按参考图有无在同族两个工作流间自动切换。
//
// 管理端配置约定：
//   - model_providers.name = autodl-comfy（与 Worker 注册名一致）
//   - model_providers.base_url = https://autodl.art/api/v1/comfyui/comfyui_workflow/{workflow_id}
//     （workflow_id 填同族任一工作流 ID 均可，Provider 按参考图有无自动路由到对端；
//     填其他自定义工作流 ID 时保持原样不切换，ref_image_N 不注入）
//   - models.model_key = MiniMax-H3（前端提交值，命中路由用；workflow 在 base_url 里）
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AutodlComfyProvider AutoDL.Art 托管 ComfyUI 厂商（工作流包装 API）。
type AutodlComfyProvider struct {
	name     string
	caps     []Capability
	http     *http.Client
	interval time.Duration
	timeout  time.Duration
}

// NewAutodlComfyProvider 构造（base_url 由 KeyCredential 携带，含 workflow_id）。
func NewAutodlComfyProvider(name string, caps []Capability) *AutodlComfyProvider {
	return &AutodlComfyProvider{
		name:     name,
		caps:     caps,
		http:     &http.Client{Timeout: 60 * time.Second},
		interval: 5 * time.Second,
		timeout:  30 * time.Minute,
	}
}

func (p *AutodlComfyProvider) Name() string               { return p.name }
func (p *AutodlComfyProvider) Capabilities() []Capability { return p.caps }

// GenerateVideo 提交 ComfyUI 工作流任务 → 轮询 → 返回成片 URL。
func (p *AutodlComfyProvider) GenerateVideo(ctx context.Context, k *KeyCredential, req *VideoRequest) (*VideoResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("autodl comfy: prompt 为空")
	}
	if strings.TrimSpace(k.BaseURL) == "" {
		return nil, fmt.Errorf("autodl comfy: base_url 未配置（应含工作流 ID）")
	}
	// Token 鉴权（裸 Token；剥掉录入时可能带的 Bearer 前缀）
	token := strings.TrimPrefix(strings.TrimSpace(k.APIKey), "Bearer ")

	prompt := req.Prompt
	if len(prompt) > maxH3PromptChars {
		prompt = prompt[:maxH3PromptChars]
	}

	// 参考图清洗 + 截断（v5 工作流最多 9 张；Handler 侧已按角色→场景→道具序去重）。
	// 私网地址（127.0.0.1/localhost/192.168.x 等）云端 GPU 拉不到 → 转纯文生，
	// 避免整卡失败（ref_image_0 必填的工作流收到不可达 URL 直接拒单）。
	refs := make([]string, 0, len(req.RefImageURLs))
	for _, u := range req.RefImageURLs {
		if u = strings.TrimSpace(u); u != "" && !isPrivateURL(u) {
			refs = append(refs, u)
		}
	}
	if len(refs) > maxAutodlRefImages {
		refs = refs[:maxAutodlRefImages]
	}

	// 同族双工作流智能路由：有参考图 → v5（多图参考），无参考图 → no_pic（文生）。
	// v5 的 ref_image_0 必填，纯文生必须回退 no_pic，否则参数校验直接失败。
	baseURL := strings.TrimRight(strings.TrimSpace(k.BaseURL), "/")
	endpoint := routeAutodlWorkflow(baseURL, len(refs) > 0)
	r2vVariant := autodlR2VVariant(endpoint) // ""=非 R2V 工作流；否则 "v5" / "v5_15s"
	useR2V := r2vVariant != ""

	// duration 工作流各自规范：v5 为 1~10s、v5_15s 为 1~15s（默认 5），no_pic 为 4~15s
	duration := req.DurationSec
	if duration <= 0 {
		duration = 5
	}
	switch {
	case r2vVariant == workflowAutodlH3R2V15s:
		if duration < 1 {
			duration = 1
		}
		if duration > 15 {
			duration = 15
		}
	case useR2V:
		if duration < 1 {
			duration = 1
		}
		if duration > 10 {
			duration = 10
		}
	default:
		if duration < 4 {
			duration = 4
		}
		if duration > 15 {
			duration = 15
		}
	}

	// resolution：no_pic 枚举仅 480p竖/480p横（已实测 720p/1080p 均被拒）；
	// v5 支持 480p/768p/1080p 多档，短剧竖屏取 768p竖（限时与 480p 同价 1 分/秒）。
	resolution := "480p竖"
	if useR2V {
		resolution = "768p竖"
	}
	body := map[string]any{
		"prompt":     prompt,
		"duration":   duration,
		"resolution": resolution,
	}
	// 参考图注入：仅 v5 工作流认 ref_image_N 字段（ref_image_0 ~ ref_image_8）
	if useR2V {
		for i, u := range refs {
			body[fmt.Sprintf("ref_image_%d", i)] = u
		}
	}

	taskID, err := p.createTask(ctx, endpoint, token, body)
	if err != nil {
		return nil, err
	}
	url, err := p.pollTask(ctx, endpoint, token, taskID)
	if err != nil {
		return nil, err
	}
	return &VideoResponse{ExternalTaskID: taskID, VideoURL: url, ModelUsed: "autodl-comfy", KeyID: k.ID}, nil
}

// 同族双工作流 ID（H3 系）与参考图上限（v5 工作流 ref_image_0 ~ ref_image_8）。
const (
	workflowAutodlH3R2V    = "minimax_h3_lightx2v_v5"     // H3多图参考生视频（ref_image_0 必填，1-10s）
	workflowAutodlH3R2V15s = "minimax_h3_lightx2v_v5_15s" // H3多图生视频15秒（ref_image_0 必填，1-15s）
	workflowAutodlH3T2V    = "minimax_h3_lightx2v_no_pic" // H3文生视频（无图入参）
	maxAutodlRefImages     = 9
	comfyWorkflowPrefix    = "/comfyui_workflow" // 查询 URL 定位前缀（pollTask 剥 workflow_id 段用）
)

// routeAutodlWorkflow 按参考图有无在同族双工作流间切换 base_url 尾段：
// base_url 指向 v5 且无参考图 → 切 no_pic；指向 no_pic 且有参考图 → 切 v5；
// 指向其他工作流 ID（含 v5_15s、首尾帧、音频系等）时原样返回（管理员显式指定优先）。
func routeAutodlWorkflow(baseURL string, hasRefs bool) string {
	want := workflowAutodlH3T2V
	other := workflowAutodlH3R2V
	if hasRefs {
		want, other = workflowAutodlH3R2V, workflowAutodlH3T2V
	}
	switch {
	case strings.HasSuffix(baseURL, "/"+want):
		return baseURL
	case strings.HasSuffix(baseURL, "/"+other):
		return strings.TrimSuffix(baseURL, other) + want
	default:
		return baseURL
	}
}

// autodlR2VVariant 判断 endpoint 尾段是否 R2V 多图参考工作流（v5 / v5_15s），
// 命中返回对应工作流 ID，否则返回空串（不注入 ref_image_N）。
func autodlR2VVariant(endpoint string) string {
	for _, id := range []string{workflowAutodlH3R2V, workflowAutodlH3R2V15s} {
		if strings.HasSuffix(endpoint, "/"+id) {
			return id
		}
	}
	return ""
}

// isPrivateURL 判定云 GPU 不可达的私网地址（127.0.0.1/localhost/192.168/10./172.16-31）。
// AutoDL 等公网实例拉不到此类 URL，提交前过滤转纯文生。
func isPrivateURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	switch host {
	case "127.0.0.1", "localhost", "0.0.0.0", "::1":
		return true
	}
	return strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.16.") || strings.HasPrefix(host, "172.17.") ||
		strings.HasPrefix(host, "172.18.") || strings.HasPrefix(host, "172.19.") ||
		strings.HasPrefix(host, "172.2") || strings.HasPrefix(host, "172.30.") ||
		strings.HasPrefix(host, "172.31.")
}

// GenerateImage 本 Provider 仅接入视频工作流（图像工作流后续按需扩展）。
func (p *AutodlComfyProvider) GenerateImage(_ context.Context, _ *KeyCredential, _ *ImageRequest) (*ImageResponse, error) {
	return nil, fmt.Errorf("provider %s 仅支持视频生成", p.name)
}

// Chat 不支持。
func (p *AutodlComfyProvider) Chat(_ context.Context, _ *KeyCredential, _ *ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("provider %s 不支持对话", p.name)
}

// createTask POST 提交工作流任务（响应 {"code":"Success","data":{"task_id":"..."}}）。
func (p *AutodlComfyProvider) createTask(ctx context.Context, baseURL, token string, body map[string]any) (string, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	resp, err := p.http.Do(req)
	if err != nil {
		return "", NewRetryableError(0, "http: "+err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return "", NewRetryableError(resp.StatusCode, "provider http "+resp.Status)
	}
	var parsed struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			TaskID  string `json:"task_id"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
	}
	if resp.StatusCode >= 400 || (parsed.Code != "" && parsed.Code != "Success") {
		msg := parsed.Msg
		if parsed.Data != nil && parsed.Data.Message != "" {
			msg = parsed.Data.Message
		}
		return "", fmt.Errorf("autodl error %d: %s", resp.StatusCode, firstNonEmpty(msg, truncate(string(raw), 200)))
	}
	if parsed.Data == nil || parsed.Data.TaskID == "" {
		return "", NewRetryableError(0, "provider 未返回 task_id: "+truncate(string(raw), 200))
	}
	return parsed.Data.TaskID, nil
}

// pollTask 轮询任务直至 SUCCESS/FAILED/超时（status ∈ QUEUED/RUNNING/SUCCESS/FAILED）。
// 成功后从 data.results[] 提取资源 URL（元素结构兼容 {url} 对象与纯字符串两种形态）。
// ⚠️ 查询 URL 与提交 URL 不同构：文档为 GET {prefix}/comfyui_workflow/result/{task_id}——
// 不含 workflow_id 段。曾直接在提交 URL 后拼 /result/{task_id} 导致永久 404 → Provider
// 报错 → worker 降级 mock（任务在 AutoDL 侧已真实提交并计费，结果无人认领）。
func (p *AutodlComfyProvider) pollTask(ctx context.Context, baseURL, token, taskID string) (string, error) {
	queryBase := strings.TrimRight(baseURL, "/")
	if idx := strings.Index(queryBase, comfyWorkflowPrefix); idx >= 0 {
		queryBase = queryBase[:idx+len(comfyWorkflowPrefix)] // 截到 /comfyui_workflow（剥掉 workflow_id 段）
	}
	endpoint := queryBase + "/result/" + taskID
	deadline := time.Now().Add(p.timeout)
	for {
		if time.Now().After(deadline) {
			return "", NewRetryableError(0, fmt.Sprintf("video 任务超时(%s)", p.timeout.String()))
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", token)
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
			Code string `json:"code"`
			Msg  string `json:"msg"`
			Data *struct {
				Status   string          `json:"status"`
				Results  json.RawMessage `json:"results"`
				Duration int             `json:"duration"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", NewRetryableError(0, "bad provider json: "+truncate(string(raw), 200))
		}
		if parsed.Data == nil {
			return "", NewRetryableError(0, "查询响应缺少 data: "+truncate(string(raw), 200))
		}
		// 状态归一大写：文档页示例为 SUCCESS/QUEUED/RUNNING/FAILED，
		// 工作流详情页输出示例为小写 completed——两处口径不一致，统一兼容。
		switch strings.ToUpper(strings.TrimSpace(parsed.Data.Status)) {
		case "SUCCESS", "COMPLETED":
			if url := extractAutodlResultURL(parsed.Data.Results); url != "" {
				return url, nil
			}
			// 状态已 SUCCESS 但 results 仍为空（AutoDL 侧状态与资源 URL 落库存在瞬时竞态，
			// 曾致整任务误判失败降级 mock）：按间隔重试直至超时，而非立即报错。
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.interval):
				continue
			}
		case "FAILED":
			return "", NewRetryableError(0, "video 任务失败: "+firstNonEmpty(parsed.Msg, truncate(string(raw), 200)))
		default: // QUEUED / RUNNING / 其他中间态：按间隔轮询（防紧循环打爆限流）
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(p.interval):
				continue
			}
		}
	}
}

// extractAutodlResultURL 从 results 数组提取第一个资源 URL（兼容对象 {url} 与纯字符串元素）。
func extractAutodlResultURL(results json.RawMessage) string {
	if len(results) == 0 {
		return ""
	}
	var arr []any
	if err := json.Unmarshal(results, &arr); err != nil {
		return ""
	}
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case map[string]any:
			for _, key := range []string{"url", "video_url", "file_url", "download_url"} {
				if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
