// 资产生成百分比进度条（对齐 LuxReal）：基于任务状态的前端确定性动画。
// 底层火山图片生成为同步短任务，无真实上报百分比，故按状态推进：
//   QUEUED=10（排队）→ RUNNING 内 10→90（指数缓动逼近，200ms 步进更流畅）→ SUCCESS 100 淡出。
// 视觉约定：图片未生成（QUEUED/RUNNING）时进度条独立占位显示；SUCCESS 后资产卡
// 刷新为主图，进度条完成 100% 即不再渲染（被图片自然覆盖）。
import { useEffect, useState } from "react";

const isRunning = (s: string) => s === "QUEUED" || s === "RUNNING";

export default function AssetProgress({ status }: { status: string }) {
  const [pct, setPct] = useState(0);
  const [fading, setFading] = useState(false);

  // 状态驱动基准值（QUEUED/SUCCESS/终态）
  useEffect(() => {
    if (status === "QUEUED") setPct(10);
    else if (status === "SUCCESS") {
      setPct(100);
      // 完成淡出：100% 短暂定格后淡出，由资产卡主图接管展示
      const t = setTimeout(() => setFading(true), 450);
      return () => clearTimeout(t);
    } else if (status === "PENDING" || status === "FAILED" || status === "FAILED_DEAD") setPct(0);
  }, [status]);

  // RUNNING：10 → 90 指数缓动（200ms 步进 + CSS width 过渡，避免 500ms 大跳步的卡顿感）
  useEffect(() => {
    if (status !== "RUNNING") return;
    let cur = 10;
    setPct(cur);
    const timer = setInterval(() => {
      // 缓动因子随剩余空间衰减：前期快（用户感知"动起来了"）、后期慢（逼近不触顶）
      const delta = Math.max(0.4, (90 - cur) * 0.08);
      cur = Math.min(90, cur + delta);
      setPct(Math.round(cur));
    }, 200);
    return () => clearInterval(timer);
  }, [status]);

  if (!isRunning(status) && status !== "SUCCESS") return null;

  const label = status === "QUEUED" ? "排队中" : status === "SUCCESS" ? "完成" : "生成中";

  return (
    <div
      className="w-full transition-opacity duration-500"
      style={{ opacity: fading ? 0 : 1, pointerEvents: "none" }}
    >
      <div className="flex items-center justify-between mb-1">
        <span className="text-[10px] text-[#4C7FF8]">{label}…</span>
        <span className="text-[10px] text-[#8B8B9A] tabular-nums">{pct}%</span>
      </div>
      <div className="asset-progress">
        <div
          className="asset-progress-bar"
          style={{ width: `${pct}%`, right: "auto", animation: "none", transition: "width 0.35s cubic-bezier(0.4, 0, 0.2, 1)" }}
        />
      </div>
    </div>
  );
}
