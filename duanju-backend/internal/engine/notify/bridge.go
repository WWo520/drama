// bridge.go 跨进程推送桥：Worker 进程经 Redis Pub/Sub 向 API 进程的 Hub 转发 WS 消息。
// API 进程持有 WS 长连接，Worker 进程无 Hub；任务终态/积分变化经此桥定向推送（01 B.3 ④ 跨节点分发）。
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// PushEnvelope 推送信封（Worker → API Hub）。UserID 必须是 user_uuid（WS 订阅 topic：user:{uuid}）。
type PushEnvelope struct {
	UserID string         `json:"user_id"`
	Type   string         `json:"type"`
	Data   map[string]any `json:"data,omitempty"`
}

// PushChannel 推送 channel 名（按环境隔离）。
func PushChannel(env string) string { return fmt.Sprintf("duanju:%s:ws:push", env) }

// PublishPush 发布推送（Worker 等无 Hub 进程侧调用）。
func PublishPush(ctx context.Context, rdb *redis.Client, env string, ev PushEnvelope) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return rdb.Publish(ctx, PushChannel(env), data).Err()
}

// StartRedisBridge 启动订阅桥（API 进程装配时调用）：订阅 channel → 定向推送 Hub 客户端。
// 断连由 go-redis 自动重连；channel 为空时退出 goroutine（进程退出时 pubsub.Close 触发）。
func StartRedisBridge(rdb *redis.Client, env string, hub *Hub) {
	ctx := context.Background()
	pubsub := rdb.Subscribe(ctx, PushChannel(env))
	go func() {
		defer pubsub.Close()
		for msg := range pubsub.Channel() {
			var ev PushEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
				continue
			}
			if ev.Type == "" || ev.UserID == "" {
				continue
			}
			hub.PublishToUser(ev.UserID, Message{
				Type: ev.Type,
				TS:   time.Now().Format(time.RFC3339),
				Data: ev.Data,
			})
		}
	}()
}
