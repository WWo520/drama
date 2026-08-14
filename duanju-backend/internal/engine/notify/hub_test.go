// Hub 测试：订阅/广播/定向推送/节流合并（S1-B11 验收：心跳/消息格式）。
package notify

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memClient 内存客户端（不连真实 WS）。
type memClient struct {
	*Client
	received []Message
}

func newMemClient(userID string) *memClient {
	c := &memClient{
		Client: &Client{
			ConnID:     "c1",
			UserID:     userID,
			Send:       make(chan []byte, 64),
			Subscribed: make(map[string]bool),
		},
	}
	return c
}

// drain 读取 Send 通道到 received（供断言）。
func (m *memClient) drain(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case data := <-m.Send:
			var msg Message
			require.NoError(t, json.Unmarshal(data, &msg))
			m.received = append(m.received, msg)
		case <-deadline:
			return
		}
	}
}

func TestHubSubscribeAndPublish(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c := newMemClient("user-1")
	hub.Register(c.Client)
	// 直接订阅（模拟 WSEndpoint 自动订阅 user:）
	hub.mu.Lock()
	hub.topicIndex["user:user-1"] = map[*Client]bool{c.Client: true}
	c.Subscribed["user:user-1"] = true
	hub.mu.Unlock()

	// 定向推送（PublishToUser）
	hub.PublishToUser("user-1", NewMessage(TypeNotify, map[string]any{
		"notification_id": 42, "type": "SYSTEM", "title": "测试", "content": "hi",
	}))
	c.drain(t, 100*time.Millisecond)
	require.Len(t, c.received, 1)
	assert.Equal(t, TypeNotify, c.received[0].Type)
	assert.NotEmpty(t, c.received[0].TS, "消息必须带 ts（05 F2）")
}

func TestHubPublishTopic(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newMemClient("u1")
	c2 := newMemClient("u2")
	hub.Register(c1.Client)
	hub.Register(c2.Client)
	// 两个客户端订阅 tasks:abc
	hub.mu.Lock()
	hub.topicIndex["tasks:abc"] = map[*Client]bool{c1.Client: true, c2.Client: true}
	c1.Subscribed["tasks:abc"] = true
	c2.Subscribed["tasks:abc"] = true
	hub.mu.Unlock()

	hub.PublishToTopic("tasks:abc", NewMessage(TypeTaskProgress, map[string]any{
		"task_uuid": "abc", "status": "RUNNING", "progress_pct": 50,
	}))
	c1.drain(t, 100*time.Millisecond)
	c2.drain(t, 100*time.Millisecond)
	assert.Len(t, c1.received, 1)
	assert.Len(t, c2.received, 1)
	assert.Equal(t, TypeTaskProgress, c1.received[0].Type)
}

func TestProgressThrottle(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c := newMemClient("u1")
	hub.Register(c.Client)
	hub.mu.Lock()
	hub.topicIndex["tasks:abc"] = map[*Client]bool{c.Client: true}
	c.Subscribed["tasks:abc"] = true
	hub.mu.Unlock()

	// 250ms 内连发 5 条进度 → 节流后只推最后一条（04 M.4）
	for i := 1; i <= 5; i++ {
		hub.PublishProgressThrottled("tasks:abc", NewMessage(TypeTaskProgress, map[string]any{
			"task_uuid": "abc", "progress_pct": i * 10,
		}))
		time.Sleep(20 * time.Millisecond)
	}
	c.drain(t, 500*time.Millisecond)
	require.NotEmpty(t, c.received)
	// 最后一条的进度应为 50（后来者覆盖）
	last := c.received[len(c.received)-1]
	assert.Equal(t, float64(50), last.Data["progress_pct"])
}

func TestChannelOwnership(t *testing.T) {
	assert.True(t, isOwnChannel("u1", "user:u1"), "自己的 user 频道允许")
	assert.False(t, isOwnChannel("u1", "user:u2"), "他人 user 频道拒绝")
	assert.True(t, isOwnChannel("u1", "tasks:abc"), "tasks 频道允许")
	assert.True(t, isOwnChannel("u1", "batches:xyz"), "batches 频道允许")
	assert.False(t, isOwnChannel("u1", "space:1:billing"), "space billing 频道需 TEAM 角色（此处拒绝）")
}