// ModelEngine 路由测试：mock provider 一次调用成功 + 429 自动切 Key（B06 验收）。
package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore 内存 KeyStore：返回固定 Key 列表，记录冷却调用。
type fakeStore struct {
	keys      []*KeyCredential
	cooldownCalls int
}

func (f *fakeStore) AvailableKeys(_ context.Context, _ Capability, modelKey string) ([]*KeyCredential, error) {
	if modelKey == "" {
		return f.keys, nil
	}
	var out []*KeyCredential
	for _, k := range f.keys {
		if k.ModelKey == modelKey {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkKeyUnavailable(_ context.Context, _ int64, _ string) error {
	f.cooldownCalls++
	return nil
}

func TestEngineChatSuccess(t *testing.T) {
	p1 := NewMockProvider("mock-a", CapLLM)
	store := &fakeStore{keys: []*KeyCredential{
		{ID: 1, ProviderName: "mock-a", ModelKey: "m1", APIKey: "k1", Concurrency: 2, Priority: 0, Enabled: true},
	}}
	e := NewEngine([]Provider{p1}, store)

	resp, err := e.Chat(context.Background(), CapLLM, &ChatRequest{Prompt: "你好"})
	require.NoError(t, err)
	assert.Contains(t, resp.Content, "你好")
	assert.Equal(t, "m1", resp.ModelUsed)
	assert.Equal(t, int64(1), resp.KeyID)
}

func TestEngineChat429SwitchKey(t *testing.T) {
	// Key1 首次调用返回 429 → 自动切 Key2（01 C.2.2 ④）
	p1 := NewMockProvider("mock-a", CapLLM)
	p2 := NewMockProvider("mock-b", CapLLM)
	p1.FailNext(1)
	store := &fakeStore{keys: []*KeyCredential{
		{ID: 1, ProviderName: "mock-a", ModelKey: "m1", APIKey: "k1", Concurrency: 2, Priority: 0, Enabled: true},
		{ID: 2, ProviderName: "mock-b", ModelKey: "m1", APIKey: "k2", Concurrency: 2, Priority: 1, Enabled: true},
	}}
	e := NewEngine([]Provider{p1, p2}, store)

	resp, err := e.Chat(context.Background(), CapLLM, &ChatRequest{Prompt: "测试切换"})
	require.NoError(t, err)
	assert.Contains(t, resp.Content, "[mock-b]")
	assert.Equal(t, int64(2), resp.KeyID)
	assert.GreaterOrEqual(t, store.cooldownCalls, 1, "429 后应标记 Key 冷却")
}

func TestEngineChatAllFailed(t *testing.T) {
	p1 := NewMockProvider("mock-a", CapLLM)
	p1.FailNext(3)
	store := &fakeStore{keys: []*KeyCredential{
		{ID: 1, ProviderName: "mock-a", ModelKey: "m1", APIKey: "k1", Concurrency: 2, Priority: 0, Enabled: true},
	}}
	e := NewEngine([]Provider{p1}, store)

	_, err := e.Chat(context.Background(), CapLLM, &ChatRequest{Prompt: "x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderAllFailed)
}

func TestEngineImageAndVideo(t *testing.T) {
	p1 := NewMockProvider("mock-img", CapT2I, CapT2V)
	store := &fakeStore{keys: []*KeyCredential{
		{ID: 3, ProviderName: "mock-img", ModelKey: "img-m1", APIKey: "k", Concurrency: 2, Priority: 0, Enabled: true},
		{ID: 4, ProviderName: "mock-img", ModelKey: "vid-m1", APIKey: "k", Concurrency: 2, Priority: 0, Enabled: true},
	}}
	e := NewEngine([]Provider{p1}, store)

	img, err := e.GenerateImage(context.Background(), CapT2I, &ImageRequest{Prompt: "古装侠客", AspectRatio: "R9_16"})
	require.NoError(t, err)
	assert.Contains(t, img.ImageURL, "/img/3.png")

	vid, err := e.GenerateVideo(context.Background(), CapT2V, &VideoRequest{Prompt: "雨夜", DurationSec: 5, ModelKey: "vid-m1"})
	require.NoError(t, err)
	assert.Contains(t, vid.ExternalTaskID, "mock-task-4-5")
}
