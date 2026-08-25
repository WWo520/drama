// WS 驱动刷新 + 轮询兜底（对齐 LuxReal 请求模式：推送为主、轮询兜底，降低请求密度防限流误伤）。
// - WS 在线：匹配的下行消息（任务终态等）立即触发 refresh；另保留慢轮询（默认 30s）防丢消息
// - WS 断线：按 pollMs 轮询兜底（保持页面原有频率），WS 恢复后自动切回慢轮询
import { useEffect, useRef } from "react";

import { wsClient, WSMessage } from "./useWebSocket";

const TASK_TERMINAL_TYPES = new Set(["TASK_SUCCESS", "TASK_FAILED"]);

// 按任务类型匹配任务终态消息（消息缺 task_type 时视为匹配，宽松兜底）
export function matchTaskTypes(types: string[]) {
  const set = new Set(types);
  return (msg: WSMessage) => {
    if (!TASK_TERMINAL_TYPES.has(msg.type)) return false;
    const tt = msg.data?.task_type;
    return typeof tt !== "string" || set.size === 0 || set.has(tt);
  };
}

export function useWsDrivenPoll(
  refresh: () => unknown,
  opts: {
    active: boolean; // 是否启用（如：拆分中/有任务在跑）
    pollMs: number; // WS 断线时轮询间隔
    match?: (msg: WSMessage) => boolean; // 匹配的 WS 消息触发刷新（缺省：任意任务终态）
    slowPollMs?: number; // WS 在线时慢轮询兜底（默认 30s；0 关闭）
  },
) {
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  const matchRef = useRef(opts.match);
  const activeRef = useRef(opts.active);
  activeRef.current = opts.active;

  // ① WS 事件驱动：匹配消息 → 立即刷新
  useEffect(() => {
    return wsClient.addMessageListener((msg) => {
      if (!activeRef.current) return;
      const match = matchRef.current;
      if (!match || match(msg)) void refreshRef.current();
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ② 轮询兜底：WS 断线全速轮询；在线时慢轮询防丢
  useEffect(() => {
    if (!opts.active) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const tick = () => {
      if (stopped) return;
      const connected = wsClient.getStatus() === "connected";
      const interval = connected ? (opts.slowPollMs ?? 30000) : opts.pollMs;
      if (interval <= 0) return;
      timer = setTimeout(() => {
        void refreshRef.current();
        tick();
      }, interval);
    };
    tick();
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
  }, [opts.active, opts.pollMs, opts.slowPollMs]);
}
