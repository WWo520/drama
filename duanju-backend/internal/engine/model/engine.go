// ModelEngine 路由核心：按 priority 取 Key → 单 Key 信号量并发控制 → 429/5xx 自动切下一个 Key → 全失败友好报错。
package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ModelEngine 模型路由引擎。
type ModelEngine struct {
	providers  map[string]Provider
	store      KeyStore
	mu         sync.Mutex
	semaphores map[int64]*keySemaphore // keyID → 信号量
	coolDown   map[int64]time.Time     // keyID → 冷却截止
}

// 信号量全满时的排队等待参数：300ms 轮询、总上限 15s（超载"慢一点"而非"直接死"）。
const (
	waitForSlotStep  = 300 * time.Millisecond
	waitForSlotTotal = 15 * time.Second
)

type keySemaphore struct {
	ch chan struct{}
}

func NewEngine(providers []Provider, store KeyStore) *ModelEngine {
	e := &ModelEngine{
		providers:  make(map[string]Provider),
		store:      store,
		semaphores: make(map[int64]*keySemaphore),
		coolDown:   make(map[int64]time.Time),
	}
	for _, p := range providers {
		e.providers[p.Name()] = p
	}
	return e
}

// Chat 对话路由（429 自动切 Key，4xx 用户侧错误直接报错）。
func (e *ModelEngine) Chat(ctx context.Context, capability Capability, req *ChatRequest) (*ChatResponse, error) {
	keys, err := e.store.AvailableKeys(ctx, capability, req.ModelKey)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrNoAvailableKey
	}
	var lastErr error
	// 所有 key 信号量满时不立即失败：排队等待重试（多用户/多任务挤兑时"慢一点"而非"直接死"）
	for deadline := time.Now().Add(waitForSlotTotal); ; time.Sleep(waitForSlotStep) {
		lastErr = nil
		for _, k := range keys {
			if !e.keyUsable(k.ID) {
				continue
			}
			p, ok := e.providers[k.ProviderName]
			if !ok {
				// 厂商名未注册（DB 配置厂商名多样，如 deepseek）：回退 OpenAI 兼容通道
				p, ok = e.providers["openai-compat"]
			}
			if !ok {
				continue
			}
			if !e.acquire(k.ID, k.Concurrency) {
				continue // 信号量满，试下一个 Key
			}
			resp, callErr := p.Chat(ctx, k, req)
			e.release(k.ID)
			if callErr == nil {
				return resp, nil
			}
			lastErr = callErr
			if isRetryable(callErr) {
				// 429/5xx/超时 → 标记冷却，自动切下一个 Key（01 C.2.2 ④）
				e.markCooldown(k.ID)
				e.store.MarkKeyUnavailable(ctx, k.ID, callErr.Error())
				continue
			}
			// 4xx 用户侧错误（prompt 违规等）→ 直接报错不切换
			return nil, callErr
		}
		if lastErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderAllFailed, lastErr)
		}
		if time.Now().After(deadline) {
			return nil, ErrNoAvailableKey
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

// GenerateImage 图片生成路由。
func (e *ModelEngine) GenerateImage(ctx context.Context, capability Capability, req *ImageRequest) (*ImageResponse, error) {
	keys, err := e.store.AvailableKeys(ctx, capability, req.ModelKey)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrNoAvailableKey
	}
	var lastErr error
	// 所有 key 信号量满时排队等待重试（上限 waitForSlotTotal）
	for deadline := time.Now().Add(waitForSlotTotal); ; time.Sleep(waitForSlotStep) {
		lastErr = nil
		for _, k := range keys {
			if !e.keyUsable(k.ID) {
				continue
			}
			p, ok := e.providers[k.ProviderName]
			if !ok {
				p, ok = e.providers["openai-compat"]
			}
			if !ok {
				continue
			}
			if !e.acquire(k.ID, k.Concurrency) {
				continue
			}
			resp, callErr := p.GenerateImage(ctx, k, req)
			e.release(k.ID)
			if callErr == nil {
				return resp, nil
			}
			lastErr = callErr
			if isRetryable(callErr) {
				e.markCooldown(k.ID)
				e.store.MarkKeyUnavailable(ctx, k.ID, callErr.Error())
				continue
			}
			return nil, callErr
		}
		if lastErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderAllFailed, lastErr)
		}
		if time.Now().After(deadline) {
			return nil, ErrNoAvailableKey
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

// GenerateVideo 视频生成路由（异步提交，返回外部任务 ID）。
func (e *ModelEngine) GenerateVideo(ctx context.Context, capability Capability, req *VideoRequest) (*VideoResponse, error) {
	keys, err := e.store.AvailableKeys(ctx, capability, req.ModelKey)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrNoAvailableKey
	}
	var lastErr error
	// 所有 key 信号量满时排队等待重试（上限 waitForSlotTotal）
	for deadline := time.Now().Add(waitForSlotTotal); ; time.Sleep(waitForSlotStep) {
		lastErr = nil
		for _, k := range keys {
			if !e.keyUsable(k.ID) {
				continue
			}
			p, ok := e.providers[k.ProviderName]
			if !ok {
				p, ok = e.providers["openai-compat"]
			}
			if !ok {
				continue
			}
			if !e.acquire(k.ID, k.Concurrency) {
				continue
			}
			resp, callErr := p.GenerateVideo(ctx, k, req)
			e.release(k.ID)
			if callErr == nil {
				return resp, nil
			}
			lastErr = callErr
			if isRetryable(callErr) {
				e.markCooldown(k.ID)
				e.store.MarkKeyUnavailable(ctx, k.ID, callErr.Error())
				continue
			}
			return nil, callErr
		}
		if lastErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderAllFailed, lastErr)
		}
		if time.Now().After(deadline) {
			return nil, ErrNoAvailableKey
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

// acquire 单 Key 信号量并发控制（非阻塞，满则 false）。
func (e *ModelEngine) acquire(keyID int64, limit int) bool {
	e.mu.Lock()
	sem, ok := e.semaphores[keyID]
	if !ok {
		if limit <= 0 {
			limit = 1
		}
		sem = &keySemaphore{ch: make(chan struct{}, limit)}
		e.semaphores[keyID] = sem
	}
	e.mu.Unlock()
	select {
	case sem.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (e *ModelEngine) release(keyID int64) {
	e.mu.Lock()
	sem := e.semaphores[keyID]
	e.mu.Unlock()
	if sem != nil {
		<-sem.ch
	}
}

// keyUsable 冷却期内的 Key 跳过。
func (e *ModelEngine) keyUsable(keyID int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if until, ok := e.coolDown[keyID]; ok {
		if time.Now().Before(until) {
			return false
		}
		delete(e.coolDown, keyID)
	}
	return true
}

// markCooldown 429/5xx 后冷却 30s（对齐 01 C.2.2 ④）。
func (e *ModelEngine) markCooldown(keyID int64) {
	e.mu.Lock()
	e.coolDown[keyID] = time.Now().Add(30 * time.Second)
	e.mu.Unlock()
}

// isRetryable 429 限流 / 5xx 服务端错误 / 超时 → 可切换 Key；4xx 用户侧错误不切换。
func isRetryable(err error) bool {
	var re *RetryableError
	if errors.As(err, &re) {
		return re.Retryable
	}
	return false
}

// RetryableError 可重试错误包装（Provider 返回 429/5xx 时使用）。
type RetryableError struct {
	Code      int
	Message   string
	Retryable bool
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("provider error %d: %s", e.Code, e.Message)
}

// NewRetryableError 构造可切换 Key 的厂商错误。
func NewRetryableError(code int, msg string) *RetryableError {
	return &RetryableError{Code: code, Message: msg, Retryable: true}
}
