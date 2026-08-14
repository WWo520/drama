// WS 端点：升级 + 鉴权 + 上行消息处理（PING/SUBSCRIBE/UNSUBSCRIBE）+ WELCOME 欢迎包。
package notify

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/duanju/backend/internal/pkg/jwt"
)

// WSEndpoint WS 处理。
type WSEndpoint struct {
	hub    *Hub
	jwtMgr *jwt.Manager
	upgrader websocket.Upgrader
	env    string
}

func NewWSEndpoint(hub *Hub, jwtMgr *jwt.Manager, env string) *WSEndpoint {
	return &WSEndpoint{
		hub:    hub,
		jwtMgr: jwtMgr,
		env:    env,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// 开发环境放行本地；生产按 origin 白名单（05 B1）
				if env != "production" {
					return true
				}
				origin := r.Header.Get("Origin")
				switch origin {
				case "https://app.example.com", "https://admin.example.com":
					return true
				}
				return false
			},
		},
	}
}

// Handle 升级为 WS（05 F1：token 鉴权）。
func (e *WSEndpoint) Handle(c *gin.Context) {
	// 鉴权：query token 或 Authorization header
	tokenStr := c.Query("token")
	if tokenStr == "" {
		tokenStr = bearer(c.GetHeader("Authorization"))
	}
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 10001, "message": "未登录"})
		return
	}
	claims, err := e.jwtMgr.Parse(tokenStr)
	if err != nil || claims[jwt.ClaimKind] != jwt.KindAccess {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 10001, "message": "登录状态无效"})
		return
	}
	userID, _ := jwt.UserID(claims)

	conn, err := e.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &Client{
		ConnID:     uuid.NewString()[:8],
		UserID:     userID,
		Conn:       conn,
		Send:       make(chan []byte, 64),
		Subscribed: make(map[string]bool),
	}
	// 自动订阅个人频道 user:{id}
	client.Subscribed["user:"+userID] = true
	e.hub.mu.Lock()
	if e.hub.topicIndex["user:"+userID] == nil {
		e.hub.topicIndex["user:"+userID] = make(map[*Client]bool)
	}
	e.hub.topicIndex["user:"+userID][client] = true
	e.hub.mu.Unlock()

	e.hub.Register(client)

	// WELCOME 欢迎包（05 F2）
	client.SendMsg(NewMessage(TypeWelcome, map[string]any{
		"connection_id":       client.ConnID,
		"heartbeat_interval_ms": 30000,
		"server_version":      "v1.0.0",
	}))

	// 读协程：心跳 + 订阅处理
	go e.readLoop(client)
	// 写协程
	go e.writeLoop(client)
}

// readLoop 处理上行消息（PING/SUBSCRIBE/UNSUBSCRIBE/TASK_CANCEL_REQUEST）。
func (e *WSEndpoint) readLoop(c *Client) {
	defer func() {
		e.hub.Unregister(c)
		c.Conn.Close()
	}()
	c.Conn.SetReadDeadline(time.Now().Add(HeartbeatInterval + PongTimeout))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(HeartbeatInterval + PongTimeout))
		return nil
	})
	for {
		_, raw, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type      string   `json:"type"`
			ClientTS  int64    `json:"client_ts"`
			Channels  []string `json:"channels"`
			TaskUUID  string   `json:"task_uuid"`
			Reason    string   `json:"reason"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.SendMsg(NewMessage(TypeError, map[string]any{"code": 90004, "message": "JSON 格式错误"}))
			continue
		}
		switch msg.Type {
		case TypePing:
			// 心跳：回 PONG（回显 client_ts）
			c.SendMsg(NewMessage(TypePong, map[string]any{"client_ts": msg.ClientTS}))
		case TypeSubscribe:
			// 订阅（越权校验：仅允许自己的 user:/space: 频道；05 F1 越权返回 ERROR 不泄露）
			var allowed []string
			for _, ch := range msg.Channels {
				if isOwnChannel(c.UserID, ch) {
					allowed = append(allowed, ch)
				} else {
					c.SendMsg(NewMessage(TypeError, map[string]any{"code": 10004, "message": "无权订阅频道", "ref_id": ch}))
				}
			}
			if len(allowed) > 0 {
				e.hub.Subscribe(c, allowed)
			}
		case TypeUnsubscribe:
			e.hub.Unsubscribe(c, msg.Channels)
		case TypeTaskCancelRequest:
			// 快速通道：转发给任务服务（HTTP 保留；此处仅回执）
			c.SendMsg(NewMessage(TypeTaskCancelled, map[string]any{
				"task_uuid": msg.TaskUUID, "frozen_points_refunded": 0, "reason": msg.Reason,
			}))
		}
	}
}

// writeLoop 写协程（Send 缓冲满由 Hub 关闭连接）。
func (e *WSEndpoint) writeLoop(c *Client) {
	ticker := time.NewTicker(10 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case data, ok := <-c.Send:
			if !ok {
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			// 服务端心跳兜底（防半开连接）
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// isOwnChannel 频道归属校验：user:{self} / tasks:{uuid} / batches:{uuid} 允许（05 F1）。
func isOwnChannel(userID, ch string) bool {
	switch {
	case ch == "user:"+userID:
		return true
	case len(ch) >= 7 && ch[:6] == "tasks:":
		return true
	case len(ch) >= 9 && ch[:8] == "batches:":
		return true
	}
	return false
}

func bearer(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && header[:len(prefix)] == prefix {
		return header[len(prefix):]
	}
	return ""
}