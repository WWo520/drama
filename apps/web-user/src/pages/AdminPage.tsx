// S3-F03/F04 管理端页（Dashboard + 用户 + 内容 + 举报 + 审核）+ S3-F05 开发者页。
import { useCallback, useEffect, useState } from "react";

import { adminApi, auditApi, devApi } from "@duanju/api-client";

type Tab = "dashboard" | "users" | "works" | "reports" | "dev";

export default function AdminPage() {
  const [tab, setTab] = useState<Tab>("dashboard");
  const [dash, setDash] = useState<Record<string, any>>({});
  const [users, setUsers] = useState<Record<string, any>[]>([]);
  const [works, setWorks] = useState<Record<string, any>[]>([]);
  const [audit, setAudit] = useState<Record<string, any>[]>([]);
  const [reports, setReports] = useState<Record<string, any>[]>([]);
  const [keys, setKeys] = useState<Record<string, any>[]>([]);
  const [toast, setToast] = useState<string | null>(null);
  const [keyName, setKeyName] = useState("");
  const [newKey, setNewKey] = useState("");

  const showToast = useCallback((m: string) => {
    setToast(m);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const load = useCallback(async () => {
    try {
      const [d, u, w, a, r, k] = await Promise.all([
        adminApi.dashboard(),
        adminApi.users(),
        adminApi.works(),
        auditApi.list(),
        adminApi.reports(),
        devApi.keys(),
      ]);
      setDash(d);
      setUsers(u.items);
      setWorks(w.items);
      setAudit(a.items);
      setReports(r.items);
      setKeys(k.items);
    } catch {
      /* 忽略 */
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const act = async (fn: () => Promise<unknown>, msg: string) => {
    try {
      await fn();
      showToast(msg);
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "操作失败");
    }
  };

  const createKey = async () => {
    if (!keyName.trim()) return;
    try {
      const r = await devApi.createKey(keyName.trim());
      setNewKey(r.api_key);
      setKeyName("");
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "签发失败");
    }
  };

  const tabs: { id: Tab; label: string }[] = [
    { id: "dashboard", label: "指标" },
    { id: "users", label: "用户" },
    { id: "works", label: "内容" },
    { id: "reports", label: "举报/审核" },
    { id: "dev", label: "开发者" },
  ];

  const cards = [
    { label: "注册用户", value: dash.total_users },
    { label: "近24h活跃", value: dash.dau },
    { label: "团队空间", value: dash.total_spaces },
    { label: "作品", value: dash.total_works },
    { label: "任务", value: dash.total_tasks },
    { label: "待审举报", value: dash.pending_reports },
    { label: "累计收入", value: `¥${dash.revenue_yuan ?? 0}` },
  ];

  return (
    <div className="min-h-screen bg-[#0B0B1A] p-6 text-slate-100">
      <h1 className="text-lg font-semibold">管理后台</h1>
      <div className="mt-3 flex gap-2">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`rounded-full px-3 py-1 text-xs ${tab === t.id ? "bg-amber-400/20 text-amber-300" : "text-slate-400"}`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "dashboard" && (
        <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {cards.map((c) => (
            <div key={c.label} className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
              <div className="text-xs text-slate-500">{c.label}</div>
              <div className="mt-1 text-2xl font-bold text-amber-300">{String(c.value ?? 0)}</div>
            </div>
          ))}
        </div>
      )}

      {tab === "users" && (
        <div className="mt-4 space-y-2">
          {users.map((u) => (
            <div key={String(u.user_uuid)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-2.5">
              <div>
                <div className="text-sm">{String(u.nickname)}</div>
                <div className="text-xs text-slate-500">{String(u.email)} · {String(u.status)}</div>
              </div>
              <button
                onClick={() =>
                  void act(() => adminApi.banUser(String(u.user_uuid), String(u.status) !== "BANNED"), String(u.status) !== "BANNED" ? "已封禁" : "已解封")
                }
                className={`rounded-lg px-3 py-1.5 text-xs ${String(u.status) === "BANNED" ? "bg-emerald-500/20 text-emerald-300" : "bg-red-500/20 text-red-300"}`}
              >
                {String(u.status) === "BANNED" ? "解封" : "封禁"}
              </button>
            </div>
          ))}
        </div>
      )}

      {tab === "works" && (
        <div className="mt-4 space-y-2">
          {works.map((w) => (
            <div key={String(w.work_id)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-2.5">
              <div>
                <div className="text-sm">{String(w.title)}</div>
                <div className="text-xs text-slate-500">
                  {String(w.publish_status)} · 播放 {String(w.view_count)} · 点赞 {String(w.like_count)}
                </div>
              </div>
              <button
                onClick={() => void act(() => adminApi.deleteWork(String(w.work_id)), "已强制删除")}
                className="rounded-lg bg-red-500/20 px-3 py-1.5 text-xs text-red-300"
              >
                强制删除
              </button>
            </div>
          ))}
          {works.length === 0 && <div className="text-xs text-slate-600">暂无内容</div>}
        </div>
      )}

      {tab === "reports" && (
        <div className="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2">
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">待审作品</h2>
            {audit.map((w) => (
              <div key={String(w.work_id)} className="mt-2 flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2">
                <div>
                  <div className="text-xs">{String(w.title)}</div>
                  <div className="text-[10px] text-slate-500">{String(w.work_id).slice(0, 8)}…</div>
                </div>
                <div className="flex gap-2">
                  <button onClick={() => void act(() => auditApi.review(String(w.work_id), true), "已通过")} className="rounded-lg bg-emerald-500/20 px-2 py-1 text-xs text-emerald-300">
                    通过
                  </button>
                  <button onClick={() => void act(() => auditApi.review(String(w.work_id), false, "违规"), "已拒绝")} className="rounded-lg bg-red-500/20 px-2 py-1 text-xs text-red-300">
                    拒绝
                  </button>
                </div>
              </div>
            ))}
            {audit.length === 0 && <div className="mt-2 text-xs text-slate-600">无待审内容</div>}
          </div>
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">举报列表</h2>
            {reports.map((r) => (
              <div key={String(r.report_id)} className="mt-2 flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2">
                <div>
                  <div className="text-xs">{String(r.title)}</div>
                  <div className="text-[10px] text-slate-500">{String(r.reason)}</div>
                </div>
                <button
                  onClick={() => void act(() => auditApi.revoke(String(r.work_id)), "已下架")}
                  className="rounded-lg bg-orange-500/20 px-2 py-1 text-xs text-orange-300"
                >
                  下架
                </button>
              </div>
            ))}
            {reports.length === 0 && <div className="mt-2 text-xs text-slate-600">无待处理举报</div>}
          </div>
        </div>
      )}

      {tab === "dev" && (
        <div className="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2">
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">API Key</h2>
            <div className="mt-3 flex gap-2">
              <input
                value={keyName}
                onChange={(e) => setKeyName(e.target.value)}
                placeholder="Key 名称"
                className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-xs"
              />
              <button onClick={() => void createKey()} className="rounded-lg bg-amber-400/15 px-3 py-2 text-xs font-semibold text-amber-300">
                签发
              </button>
            </div>
            {newKey && (
              <div className="mt-2 rounded-lg border border-amber-400/30 bg-amber-400/5 p-3 text-xs break-all text-amber-200">
                新 Key（仅显示一次）：{newKey}
              </div>
            )}
            <div className="mt-3 space-y-1">
              {keys.map((k) => (
                <div key={String(k.api_key_prefix)} className="flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2 text-xs">
                  <span>{String(k.name)}（{String(k.api_key_prefix)}…）· {String(k.status)}</span>
                  <button
                    onClick={() => void act(() => devApi.revokeKey(String(k.api_key_prefix)), "已吊销")}
                    className="text-red-400"
                  >
                    吊销
                  </button>
                </div>
              ))}
            </div>
          </div>
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <h2 className="text-sm font-semibold">Webhook 回调</h2>
            <p className="mt-1 text-xs text-slate-500">HMAC-SHA256 签名校验（X-Signature 头）</p>
            <button
              onClick={() => void act(() => devApi.registerWebhook({ url: "https://example.com/hook", secret: "dev-secret-123456" }), "回调已注册")}
              className="mt-3 rounded-lg bg-violet-500/20 px-3 py-2 text-xs font-semibold text-violet-300"
            >
              注册示例回调
            </button>
          </div>
        </div>
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200">
          {toast}
        </div>
      )}
    </div>
  );
}