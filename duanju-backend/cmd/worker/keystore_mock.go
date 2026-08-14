// memoryKeyStore：开发期内存 KeyStore（B06 验收用 mock provider；生产换 DB 表驱动实现）。
package main

import (
	"context"

	"github.com/duanju/backend/internal/engine/model"
)

type memoryKeyStore struct{}

func (memoryKeyStore) AvailableKeys(_ context.Context, capability model.Capability, modelKey string) ([]*model.KeyCredential, error) {
	keys := []*model.KeyCredential{
		{ID: 1, ProviderName: "mock-llm", ModelKey: "luxchat-mock", APIKey: "mock-key-1", BaseURL: "mock://llm", Concurrency: 8, Priority: 0, Enabled: true},
	}
	if capability == model.CapLLM {
		return keys, nil
	}
	return nil, nil
}

func (memoryKeyStore) MarkKeyUnavailable(_ context.Context, _ int64, _ string) error { return nil }
