// Package notify 通知引擎（E6）：WS Hub + 10 种下行消息 + 心跳 + 节流（对齐 04 M 章 / 05 F 章）。
package notify

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 下行消息类型（05 F2：服务端 10 种）。
const (
	TypeWelcome          = "WELCOME"
	TypeError            = "ERROR"
	TypePong             = "PONG"
	TypeTaskProgress     = "TASK_PROGRESS"
	TypeTaskSuccess      = "TASK_SUCCESS"
	TypeTaskFailed       = "TASK_FAILED"
	TypeTaskCancelled    = "TASK_CANCELLED"
	TypeBatchProgress    = "BATCH_PROGRESS"
	TypeNotify           = "NOTIFY"
	TypePointsChanged    = "POINTS_BALANCE_CHANGED"
	TypeSpacePointsChanged = "SPACE_POINTS_BALANCE_CHANGED"
)

// 客户端上行消息（05 F3：4 种）。
const (
	TypePing               = "PING"
	TypeSubscribe          = "SUBSCRIBE"
	TypeUnsubscribe        = "UNSUBSCRIBE"
	TypeTaskCancelRequest  = "TASK_CANCEL_REQUEST"
)

// 心跳参数（05 F1：30s PING / 10s PONG 超时）。
const (
	HeartbeatInterval = 30 * time.Second
	PongTimeout       = 10 * time.Second
)

// Message 统一 WS 消息（type + ts 必填，05 F2）。
type Message struct {
	Type string         `json:"type"`
	TS   string         `json:"ts"`
	Data map[string]any `json:"data,omitempty"`
}

// Client 单个 WS 连接。
type Client struct {
	ConnID    string
	UserID    string
	Conn      *websocket.Conn
	Send      chan []byte
	Subscribed map[string]bool // topic → 订阅
	mu        sync.Mutex
}

// Hub 连接管理。
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	topicIndex map[string]map[*Client]bool // topic → clients
	register   chan *Client
	unregister chan *Client
	// 进度节流（250ms 合并，04 M.4）
	throttleMu    sync.Mutex
	throttleTimer *time.Timer
	pending       map[string][]byte // topic → 待推送消息
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		topicIndex: make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		pending:    make(map[string][]byte),
	}
}

// Run 主事件循环（单 goroutine 处理注册/注销/广播）。
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.Send)
				// 从所有 topic 反注册
				h.mu.Lock()
				for topic := range c.Subscribed {
					if clients, ok := h.topicIndex[topic]; ok {
						delete(clients, c)
						if len(clients) == 0 {
							delete(h.topicIndex, topic)
						}
					}
				}
				h.mu.Unlock()
			}
		}
	}
}

// Register 注册连接。
func (h *Hub) Register(c *Client) { h.register <- c }

// Unregister 注销连接。
func (h *Hub) Unregister(c *Client) { h.unregister <- c }

// Subscribe 订阅频道（越权校验由调用方完成；05 F1：拒绝时返回 ERROR 不泄露存在性）。
func (h *Hub) Subscribe(c *Client, topics []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range topics {
		if c.Subscribed[t] {
			continue
		}
		c.Subscribed[t] = true
		if h.topicIndex[t] == nil {
			h.topicIndex[t] = make(map[*Client]bool)
		}
		h.topicIndex[t][c] = true
	}
}

// Unsubscribe 取消订阅。
func (h *Hub) Unsubscribe(c *Client, topics []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range topics {
		if c.Subscribed[t] {
			delete(c.Subscribed, t)
			if clients, ok := h.topicIndex[t]; ok {
				delete(clients, c)
				if len(clients) == 0 {
					delete(h.topicIndex, t)
				}
			}
		}
	}
}

// Send 直接给客户端发送（缓冲满踢下线，防慢消费者泄漏——04 M.1）。
func (c *Client) SendMsg(msg Message) {
	data, _ := json.Marshal(msg)
	select {
	case c.Send <- data:
	default:
		// 缓冲满 → 踢下线
		go func(cc *Client) { cc.Conn.Close() }(c)
	}
}

// PublishToUser 定向推送（多端在线逐连接推）。
func (h *Hub) PublishToUser(userID string, msg Message) {
	topic := "user:" + userID
	h.PublishToTopic(topic, msg)
}

// PublishToTopic 按 topic 广播。
func (h *Hub) PublishToTopic(topic string, msg Message) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	clients := h.topicIndex[topic]
	var targets []*Client
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		select {
		case c.Send <- data:
		default:
			go func(cc *Client) { cc.Conn.Close() }(c)
		}
	}
}

// PublishProgressThrottled 进度消息 250ms 节流合并（04 M.4：同 topic 后来者覆盖）。
func (h *Hub) PublishProgressThrottled(topic string, msg Message) {
	data, _ := json.Marshal(msg)
	h.throttleMu.Lock()
	h.pending[topic] = data
	if h.throttleTimer == nil {
		h.throttleTimer = time.AfterFunc(250*time.Millisecond, func() {
			h.flushPending()
		})
	}
	h.throttleMu.Unlock()
}

func (h *Hub) flushPending() {
	h.throttleMu.Lock()
	pending := h.pending
	h.pending = make(map[string][]byte)
	h.throttleTimer = nil
	h.throttleMu.Unlock()

	for topic, data := range pending {
		h.mu.RLock()
		clients := h.topicIndex[topic]
		var targets []*Client
		for c := range clients {
			targets = append(targets, c)
		}
		h.mu.RUnlock()
		for _, c := range targets {
			select {
			case c.Send <- data:
			default:
				go func(cc *Client) { cc.Conn.Close() }(c)
			}
		}
	}
}

// NewMessage 构造消息（自动带 ts）。
func NewMessage(msgType string, data map[string]any) Message {
	return Message{
		Type: msgType,
		TS:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Data: data,
	}
}

var _ = fmt.Sprintf
