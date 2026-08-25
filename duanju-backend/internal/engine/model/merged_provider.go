// MergedProvider 同厂多端点 Provider 合并（如火山：图片走同步 images API、视频走异步 tasks API）。
// 按能力分派：Chat/GenerateImage/GenerateVideo 各转发给声明了对应能力的子 Provider。
package model

import (
	"context"
	"fmt"
)

type mergedProvider struct {
	name    string
	chat    Provider
	image   Provider
	video   Provider
	allCaps []Capability
}

// MergeProviders 将多个 Provider 合并为一个（name 以第一个为准）；子 Provider 需能力互不重叠。
func MergeProviders(ps ...Provider) Provider {
	if len(ps) == 0 {
		return nil
	}
	m := &mergedProvider{name: ps[0].Name()}
	for _, p := range ps {
		for _, c := range p.Capabilities() {
			m.allCaps = append(m.allCaps, c)
			switch c {
			case CapLLM:
				m.chat = p
			case CapT2I, CapI2I:
				m.image = p
			case CapT2V, CapI2V:
				m.video = p
			}
		}
	}
	return m
}

func (m *mergedProvider) Name() string               { return m.name }
func (m *mergedProvider) Capabilities() []Capability { return m.allCaps }

func (m *mergedProvider) Chat(ctx context.Context, k *KeyCredential, req *ChatRequest) (*ChatResponse, error) {
	if m.chat == nil {
		return nil, fmt.Errorf("provider %s 不支持对话", m.name)
	}
	return m.chat.Chat(ctx, k, req)
}

func (m *mergedProvider) GenerateImage(ctx context.Context, k *KeyCredential, req *ImageRequest) (*ImageResponse, error) {
	if m.image == nil {
		return nil, fmt.Errorf("provider %s 不支持图片生成", m.name)
	}
	return m.image.GenerateImage(ctx, k, req)
}

func (m *mergedProvider) GenerateVideo(ctx context.Context, k *KeyCredential, req *VideoRequest) (*VideoResponse, error) {
	if m.video == nil {
		return nil, fmt.Errorf("provider %s 不支持视频生成", m.name)
	}
	return m.video.GenerateVideo(ctx, k, req)
}
