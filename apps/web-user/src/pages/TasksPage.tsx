// S3-F02 任务接单 + 模板集市页。
import { useCallback, useEffect, useState } from "react";

import { marketApi, taskApi } from "@duanju/api-client";

export default function TasksPage() {
  const [orders, setOrders] = useState<Record<string, any>[]>([]);
  const [styles, setStyles] = useState<Record<string, any>[]>([]);
  const [templates, setTemplates] = useState<Record<string, any>[]>([]);
  const [toast, setToast] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [reward, setReward] = useState("100");
  const [myUuid, setMyUuid] = useState("");
  const [tab, setTab] = useState<"tasks" | "market">("tasks");

  const showToast = useCallback((m: string) => {
    setToast(m);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const load = useCallback(async () => {
    try {
      const [o, s, t] = await Promise.all([taskApi.list(), marketApi.styles(), marketApi.templates()]);
      setOrders(o.items);
      setStyles(s.items);
      setTemplates(t.items);
      const me = await fetch("/api/v1/auth/me", { headers: { Authorization: `Bearer ${localStorage.getItem("duanju_token")}` } }).then((r) => r.json());
      setMyUuid(me.data?.user?.user_uuid ?? "");
    } catch {
      /* 忽略 */
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const createOrder = async () => {
    if (!title.trim()) return;
    try {
      await taskApi.create({ title: title.trim(), reward_points: Number(reward) || 0 });
      showToast("任务已发布");
      setTitle("");
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "发单失败");
    }
  };

  const push = async (id: string, status: "IN_PROGRESS" | "SUBMITTED" | "COMPLETED") => {
    try {
      await taskApi.status(id, status);
      showToast(`状态 → ${status}`);
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "操作失败");
    }
  };

  const act = async (fn: () => Promise<unknown>, msg: string) => {
    try {
      await fn();
      showToast(msg);
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "操作失败");
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] p-6 text-slate-100">
      <div className="flex items-center gap-3">
        <h1 className="text-lg font-semibold">生态 · 任务与集市</h1>
        <button onClick={() => setTab("tasks")} className={`rounded-full px-3 py-1 text-xs ${tab === "tasks" ? "bg-amber-400/20 text-amber-300" : "text-slate-400"}`}>
          任务接单
        </button>
        <button onClick={() => setTab("market")} className={`rounded-full px-3 py-1 text-xs ${tab === "market" ? "bg-amber-400/20 text-amber-300" : "text-slate-400"}`}>
          风格/模板集市
        </button>
      </div>

      {tab === "tasks" ? (
        <>
          <div className="mt-4 flex items-center gap-3 rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="任务标题（如：30s 切片剪辑）"
              className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm"
            />
            <input
              value={reward}
              onChange={(e) => setReward(e.target.value)}
              placeholder="赏金积分"
              className="w-28 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm"
            />
            <button onClick={() => void createOrder()} className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-4 py-2 text-sm font-semibold text-black">
              发布任务
            </button>
          </div>
          <div className="mt-4 space-y-2">
            {orders.length === 0 && <div className="text-sm text-slate-600">暂无任务</div>}
            {orders.map((o) => (
              <div key={String(o.order_id)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-3">
                <div>
                  <div className="text-sm font-medium">{String(o.title)}</div>
                  <div className="text-xs text-slate-500">
                    {String(o.status)} · 赏金 {String(o.reward_points)} 分
                  </div>
                </div>
                <div className="flex gap-2">
                  {String(o.status) === "OPEN" && (
                    <button onClick={() => void act(() => taskApi.intent(String(o.order_id), "我可以"), "已投递意向")} className="rounded-lg bg-white/10 px-3 py-1.5 text-xs">
                      投递意向
                    </button>
                  )}
                  {String(o.status) === "ACCEPTED" && myUuid === String(o.assignee_user_id) && (
                    <button onClick={() => void push(String(o.order_id), "IN_PROGRESS")} className="rounded-lg bg-sky-500/20 px-3 py-1.5 text-xs text-sky-300">
                      开始
                    </button>
                  )}
                  {String(o.status) === "IN_PROGRESS" && myUuid === String(o.assignee_user_id) && (
                    <button onClick={() => void push(String(o.order_id), "SUBMITTED")} className="rounded-lg bg-violet-500/20 px-3 py-1.5 text-xs text-violet-300">
                      提交
                    </button>
                  )}
                  {String(o.status) === "SUBMITTED" && (
                    <button onClick={() => void push(String(o.order_id), "COMPLETED")} className="rounded-lg bg-emerald-500/20 px-3 py-1.5 text-xs text-emerald-300">
                      验收完成
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </>
      ) : (
        <>
          <div className="mt-4 rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">风格库（27 种官方风格）</h2>
            <div className="mt-3 flex flex-wrap gap-2">
              {styles.map((s) => (
                <span key={String(s.style_code)} className="rounded-full border border-white/10 bg-white/[0.03] px-3 py-1 text-xs text-slate-300">
                  {String(s.name)}
                </span>
              ))}
            </div>
          </div>
          <div className="mt-4 rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">模板集市</h2>
            <div className="mt-3 grid grid-cols-2 gap-3">
              {templates.length === 0 && <div className="col-span-2 text-xs text-slate-600">暂无模板，可创建第一个</div>}
              {templates.map((t) => (
                <div key={String(t.template_id)} className="rounded-xl border border-white/10 bg-white/[0.03] p-3">
                  <div className="text-sm font-medium">{String(t.name)}</div>
                  <div className="text-xs text-slate-500">
                    {String(t.description)} · 使用 {String(t.used_count)} 次
                  </div>
                  <button
                    onClick={() => void act(() => marketApi.applyTemplate(String(t.template_id)), "模板已应用")}
                    className="mt-2 rounded-lg bg-amber-400/15 px-3 py-1 text-xs font-semibold text-amber-300"
                  >
                    应用
                  </button>
                </div>
              ))}
            </div>
          </div>
        </>
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200">
          {toast}
        </div>
      )}
    </div>
  );
}