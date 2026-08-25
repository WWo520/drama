// 资产生成百分比进度条（对齐 LuxReal）：基于任务状态的前端确定性动画。
// 底层火山图片生成为同步短任务，无真实上报百分比，故按状态推进：
//   QUEUED=10（排队）→ RUNNING 内 10→90（递减增量逼近）→ SUCCESS 定格 100。
import { useEffect, useState } from "react";

const isRunning = (s: string) => s === "QUEUED" || s === "RUNNING";

export default function AssetProgress({ status }: { status: string }) {
  const [pct, setPct] = useState(0);

  // 状态驱动基准值（QUEUED/SUCCESS/终态）
  useEffect(() => {
    if (status === "QUEUED") setPct(10);
    else if (status === "SUCCESS") setPct(100);
    else if (status === "PENDING" || status === "FAILED" || status === "FAILED_DEAD") setPct(0);
  }, [status]);

  // RUNNING：从 10 向 90 渐进（增量随余量递减，避免过早触顶形成停留感）
  useEffect(() => {
    if (status !== "RUNNING") return;
    let cur = 10;
    setPct(cur);
    const timer = setInterval(() => {
      const delta = Math.max(0.6, (90 - cur) * 0.05);
      cur = Math.min(90, cur + delta);
      setPct(Math.round(cur));
    }, 500);
    return () => clearInterval(timer);
  }, [status]);

  if (!isRunning(status) && status !== "SUCCESS") return null;

  const label = status === "QUEUED" ? "排队中" : status === "SUCCESS" ? "完成" : "生成中";

  return (
    <div className="w-full">
      <div className="flex items-center justify-between mb-1">
        <span className="text-[10px] text-[#4C7FF8]">{label}…</span>
        <span className="text-[10px] text-[#8B8B9A]">{pct}%</span>
      </div>
      <div className="asset-progress">
        <div className="asset-progress-bar" style={{ width: `${pct}%`, right: "auto", animation: "none" }} />
      </div>
    </div>
  );
}
