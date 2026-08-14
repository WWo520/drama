// S1-F07 充值页：余额展示 + 积分包 3 档 + 支付宝下单 + 结果轮询（mock 支付）。
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import { pointsApi, POINT_PACKAGES } from "@duanju/api-client";

export default function PointsPage() {
  const nav = useNavigate();
  const [balance, setBalance] = useState<{ current: number; frozen: number }>({ current: 0, frozen: 0 });
  const [loading, setLoading] = useState(true);
  const [orderNo, setOrderNo] = useState<string | null>(null);
  const [orderAmount, setOrderAmount] = useState("");
  const [orderPoints, setOrderPoints] = useState(0);
  const [paying, setPaying] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadBalance = useCallback(async () => {
    try {
      const d = await pointsApi.me();
      setBalance({ current: d.current_balance, frozen: d.frozen_balance });
    } catch (e) {
      showToast(e instanceof Error ? e.message : "获取余额失败");
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    void loadBalance();
  }, [loadBalance]);

  // WS 实时到账（S1-F08）：POINTS_BALANCE_CHANGED → 刷新余额；轮询兜底仍保留
  useEffect(() => {
    const handler = () => {
      void loadBalance();
      setOrderNo(null);
      if (pollRef.current) clearInterval(pollRef.current);
      showToast("充值到账，余额已更新");
    };
    window.addEventListener("points:changed", handler);
    return () => window.removeEventListener("points:changed", handler);
  }, [loadBalance, showToast]);

  // 下单 → 轮询支付状态（9.5 → 9.8）
  const handleBuy = async (code: string, points: number, amount: string) => {
    try {
      setPaying(true);
      const resp = await pointsApi.createOrder(
        { package_code: code, amount_yuan: amount, points },
        `purchase-${Date.now()}`,
      );
      setOrderNo(resp.order_no);
      setOrderAmount(resp.amount_yuan);
      setOrderPoints(points);
      showToast("订单已创建，请扫码支付（开发环境点击模拟支付）");
      startPolling(resp.order_no);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "下单失败");
    } finally {
      setPaying(false);
    }
  };

  const startPolling = useCallback(
    (no: string) => {
      if (pollRef.current) clearInterval(pollRef.current);
      pollRef.current = setInterval(async () => {
        try {
          const s = await pointsApi.orderStatus(no);
          if (s.pay_status === "PAID") {
            if (pollRef.current) clearInterval(pollRef.current);
            showToast(`充值到账 +${s.points} 积分`);
            void loadBalance();
            setOrderNo(null);
          }
        } catch {
          /* 忽略 */
        }
      }, 2000);
    },
    [showToast, loadBalance],
  );

  // 模拟支付：开发环境无真实支付宝 → 触发 mock 回调（7 步验签由测试/脚本完成）
  // 开发提示：真实沙箱扫码后支付宝自动回调；此处提供说明文案
  const simPay = () => {
    showToast("开发环境：请在测试脚本中完成 HMAC 签名回调验证支付");
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      <header className="sticky top-0 z-10 border-b border-white/5 bg-[#0B0B1A]/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-4xl items-center justify-between px-6">
          <div className="flex items-center gap-3">
            <button onClick={() => nav(-1)} className="text-sm text-slate-400 transition hover:text-amber-300">
              ← 返回
            </button>
            <h1 className="text-base font-semibold">积分充值</h1>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-4xl px-6 py-8">
        {/* 余额卡片 */}
        <div className="rounded-2xl border border-amber-400/20 bg-gradient-to-br from-amber-500/10 to-transparent p-6">
          <p className="text-xs text-slate-400">当前可用积分</p>
          <div className="mt-1 flex items-end gap-2">
            <span className="text-4xl font-bold text-amber-300">{loading ? "…" : balance.current}</span>
            <span className="mb-1 text-sm text-slate-500">分</span>
          </div>
          <p className="mt-2 text-xs text-slate-500">
            冻结中：{balance.frozen} 分（生成任务完成后结算）
          </p>
        </div>

        {/* 积分包 3 档 */}
        <h2 className="mb-3 mt-8 text-sm font-semibold text-slate-300">选择积分包</h2>
        <div className="grid grid-cols-3 gap-4">
          {POINT_PACKAGES.map((pkg) => (
            <button
              key={pkg.code}
              onClick={() => void handleBuy(pkg.code, pkg.points, pkg.amount_yuan)}
              disabled={paying || !!orderNo}
              className="group relative rounded-2xl border border-white/10 bg-white/[0.03] p-5 text-left transition hover:border-amber-400/40 hover:bg-white/[0.05] disabled:opacity-50"
            >
              {pkg.badge && (
                <span className="absolute -top-2.5 left-4 rounded-full bg-gradient-to-r from-amber-400 to-amber-500 px-2.5 py-0.5 text-[10px] font-semibold text-black">
                  {pkg.badge}
                </span>
              )}
              <div className="text-xs text-slate-500">{pkg.name}</div>
              <div className="mt-2 text-3xl font-bold text-slate-100">{pkg.points}</div>
              <div className="mt-1 text-xs text-slate-500">积分</div>
              <div className="mt-3 flex items-center justify-between">
                <span className="text-sm font-semibold text-amber-300">¥{pkg.amount_yuan}</span>
                <span className="text-[11px] text-slate-500">{pkg.desc}</span>
              </div>
            </button>
          ))}
        </div>

        {/* 支付中面板 */}
        {orderNo && (
          <div className="mt-8 rounded-2xl border border-white/10 bg-white/[0.03] p-6">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-slate-300">订单支付中</h3>
              <span className="text-xs text-amber-300">等待到账…</span>
            </div>
            <div className="mt-3 flex items-center gap-4">
              {/* mock 二维码 */}
              <div className="flex h-28 w-28 items-center justify-center rounded-xl border border-white/10 bg-white p-2">
                <div className="grid h-full w-full grid-cols-5 grid-rows-5 gap-0.5 opacity-90">
                  {Array.from({ length: 25 }).map((_, i) => (
                    <div key={i} className={i % 3 === 0 || (i > 8 && i < 17 && i % 2 === 0) ? "bg-black" : "bg-white"} />
                  ))}
                </div>
              </div>
              <div className="text-sm text-slate-400">
                <p>订单号：{orderNo}</p>
                <p className="mt-1">
                  金额：<span className="font-semibold text-amber-300">¥{orderAmount}</span> · {orderPoints} 积分
                </p>
                <p className="mt-2 text-xs text-slate-500">使用支付宝扫码支付（沙箱环境）</p>
                <button
                  onClick={simPay}
                  className="mt-3 rounded-lg border border-amber-400/30 px-3 py-1 text-xs text-amber-300 transition hover:bg-amber-400/10"
                >
                  模拟支付（开发）
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 说明 */}
        <div className="mt-8 rounded-xl border border-white/5 bg-white/[0.02] p-4 text-xs leading-relaxed text-slate-500">
          · 首单加赠 50% 积分（限时，自动到账）<br />
          · 支付后积分实时到账，可在「积分流水」中查看明细
        </div>
      </main>

      {toast && (
        <div className="fixed bottom-6 left-1/2 z-30 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200 shadow-xl">
          {toast}
        </div>
      )}
    </div>
  );
}