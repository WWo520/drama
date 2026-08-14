// 全局 WS 挂载（S1-F08）：登录后自动连接 + 订阅 user 频道 + 消息全局处理（POINTS_BALANCE_CHANGED 等）。
import { useEffect } from "react";

import { useAuthStore } from "../stores/authStore";
import { wsClient } from "../hooks/useWebSocket";

// 消息类型（05 F2）
const TYPE_POINTS_CHANGED = "POINTS_BALANCE_CHANGED";
const TYPE_TASK_PROGRESS = "TASK_PROGRESS";

export default function GlobalWS() {
  const accessToken = useAuthStore((s) => s.accessToken);

  // 连接管理：优先使用 localStorage（api-client refresh 后更新此值），兜底 authStore
  const effectiveToken = localStorage.getItem("access_token") || accessToken;

  useEffect(() => {
    if (!effectiveToken) {
      wsClient.disconnect();
      return;
    }
    wsClient.connect(effectiveToken);
    return () => {
      // 页面卸载不主动断开（单例跨页面）；仅登出时断开
    };
  }, [effectiveToken, accessToken]);

  // 连接成功 → 自动订阅个人频道（05 F1：WELCOME 后订阅）
  useEffect(() => {
    const unsub = wsClient.addStatusListener(() => {
      if (wsClient.getStatus() === "connected") {
        wsClient.subscribe(["user:me"]);
      }
    });
    return unsub;
  }, []);

  // 全局消息处理：积分变化 → 广播事件（充值页/顶栏监听）
  useEffect(() => {
    const unsub = wsClient.addMessageListener((msg) => {
      if (msg.type === TYPE_POINTS_CHANGED) {
        window.dispatchEvent(new CustomEvent("points:changed", { detail: msg.data }));
      }
      if (msg.type === TYPE_TASK_PROGRESS) {
        window.dispatchEvent(new CustomEvent("task:progress", { detail: msg.data }));
      }
    });
    return unsub;
  }, []);

  return null;
}