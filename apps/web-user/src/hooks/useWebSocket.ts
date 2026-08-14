// WS 客户端单例（S1-F08）：05 F.1~F.3 协议——心跳 30s / 指数退避重连（1s→2s→4s→8s→16s→32s 上限，连 10 次失败停止）/ 自动订阅 user 频道。
// 断线时各页面保留 3s 轮询兜底（06 共享层：WS 恢复后无缝）。
import { useSyncExternalStore } from "react";

export type WSStatus = "connecting" | "connected" | "disconnected";

export interface WSMessage {
  type: string;
  ts: string;
  data?: Record<string, unknown>;
}

type Listener = (msg: WSMessage) => void;

const MAX_RECONNECT = 10;
const HEARTBEAT_MS = 30000;
const RETRY_BASE_MS = 1000;

class WSClient {
  private ws: WebSocket | null = null;
  private status: WSStatus = "disconnected";
  private retry = 0;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private listeners = new Set<Listener>();
  private statusListeners = new Set<() => void>();

  connect(token: string) {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    this.retry = 0; // 手动连接（token 刷新后）重置退避计数
    this.setStatus("connecting");
    const proto = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${window.location.host}/ws/v1?token=${encodeURIComponent(token)}`);
    this.ws = ws;

    ws.onopen = () => {
      this.retry = 0;
      this.setStatus("connected");
      this.startHeartbeat();
    };

    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data as string) as WSMessage;
        // 心跳响应（PONG）无需分发
        if (msg.type !== "PONG") {
          this.listeners.forEach((fn) => fn(msg));
        }
      } catch {
        /* 忽略非法消息 */
      }
    };

    ws.onclose = () => {
      this.stopHeartbeat();
      this.setStatus("disconnected");
      this.scheduleReconnect(token);
    };

    ws.onerror = () => {
      ws.close();
    };
  }

  private scheduleReconnect(token: string) {
    if (this.reconnectTimer) return;
    if (this.retry >= MAX_RECONNECT) {
      this.setStatus("disconnected"); // 连 10 次失败停止（05 F1）
      return;
    }
    const delay = Math.min(RETRY_BASE_MS * 2 ** this.retry, 32000); // 指数退避 1s→…→32s 上限
    this.retry++;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect(token);
    }, delay);
  }

  private startHeartbeat() {
    this.stopHeartbeat();
    // 客户端每 30s 发 PING（05 F1）
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: "PING", client_ts: Date.now() }));
      }
    }, HEARTBEAT_MS);
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  // 订阅频道（05 F3：SUBSCRIBE，可一次多个）
  subscribe(channels: string[]) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: "SUBSCRIBE", channels }));
    }
  }

  send(data: unknown) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    }
  }

  disconnect() {
    this.stopHeartbeat();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.ws?.close();
    this.ws = null;
    this.retry = 0;
    this.setStatus("disconnected");
  }

  getStatus(): WSStatus {
    return this.status;
  }

  addMessageListener(fn: Listener): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  addStatusListener(fn: () => void): () => void {
    this.statusListeners.add(fn);
    return () => this.statusListeners.delete(fn);
  }

  private setStatus(s: WSStatus) {
    if (this.status !== s) {
      this.status = s;
      this.statusListeners.forEach((fn) => fn());
    }
  }
}

// 模块级单例（多页面共享连接）
export const wsClient = new WSClient();

// 页面 Hook：status 订阅 + 消息订阅
export function useWebSocket(): { status: WSStatus; onMessage: (fn: Listener) => () => void } {
  const status = useSyncExternalStore(
    (cb) => wsClient.addStatusListener(cb),
    () => wsClient.getStatus(),
  );
  return { status, onMessage: (fn) => wsClient.addMessageListener(fn) };
}