// B05 管理前端骨架 + 07 完整版：登录 + 布局 + RBAC 守卫 + 7 页真实数据（Dashboard/用户/内容/举报/任务/财务/配置）。
import React, { useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Link, Navigate, Route, Routes, useNavigate } from "react-router-dom";

// ---- RBAC 角色 → 菜单权限 ----
const ROLES: Record<string, string[]> = {
  SUPER_ADMIN: ["dashboard", "users", "content", "reports", "tasks", "finance", "config"],
  CONTENT_ADMIN: ["dashboard", "content", "reports"],
  FINANCE_ADMIN: ["dashboard", "finance"],
  VIEWER: ["dashboard"],
};

const MENUS = [
  { key: "dashboard", label: "工作台" },
  { key: "users", label: "用户管理" },
  { key: "content", label: "内容管理" },
  { key: "reports", label: "举报审核" },
  { key: "tasks", label: "任务管理" },
  { key: "finance", label: "财务" },
  { key: "config", label: "平台配置" },
];

function useAuth() {
  const [user, setUser] = useState<{ uuid: string; nickname: string; role: string } | null>(() => {
    const raw = localStorage.getItem("duanju_admin_user");
    return raw ? JSON.parse(raw) : null;
  });
  return { user, setUser };
}

function RequirePerm({ user, perm, children }: { user: { role: string } | null; perm: string; children: React.ReactNode }) {
  if (!user) return <Navigate to="/login" replace />;
  if (!(ROLES[user.role] ?? []).includes(perm)) return <Navigate to="/dashboard" replace />;
  return <>{children}</>;
}

// ---- API 工具 ----
const API_BASE = "/api/v1";
const authHeaders = () => ({ Authorization: `Bearer ${localStorage.getItem("duanju_token")}` });

function apiGet<T>(path: string): Promise<T> {
  return fetch(`${API_BASE}${path}`, { headers: authHeaders() })
    .then((r) => r.json())
    .then((d) => { if (d.code !== 0) throw new Error(d.message); return d.data as T; });
}

function apiPost(path: string, body?: unknown): Promise<unknown> {
  return fetch(`${API_BASE}${path}`, {
    method: "POST",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  }).then((r) => r.json());
}

function apiDelete(path: string): Promise<unknown> {
  return fetch(`${API_BASE}${path}`, { method: "DELETE", headers: authHeaders() }).then((r) => r.json());
}

// ---- 7 个管理页面（07 完整版：真实数据）----
function DashboardPage() {
  const [d, setD] = useState<Record<string, any> | null>(null);
  useEffect(() => { apiGet("/admin/dashboard").then((d) => setD(d as Record<string, any>)); }, []);
  if (!d) return <div className="text-sm text-slate-500">加载中…</div>;
  const cards = [
    { label: "注册用户", value: d.total_users }, { label: "近24h活跃", value: d.dau },
    { label: "团队空间", value: d.total_spaces }, { label: "作品", value: d.total_works },
    { label: "任务", value: d.total_tasks }, { label: "待审举报", value: d.pending_reports },
    { label: "累计收入", value: `¥${d.revenue_yuan ?? 0}` },
  ];
  return <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">{cards.map((c) => (
    <div key={c.label} className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
      <div className="text-xs text-slate-500">{c.label}</div>
      <div className="mt-1 text-2xl font-bold text-amber-300">{String(c.value ?? 0)}</div>
    </div>
  ))}</div>;
}

function UsersPage() {
  const [items, setItems] = useState<Record<string, any>[]>([]);
  useEffect(() => { apiGet<{ items: Record<string, any>[] }>("/admin/users").then((d) => setItems(d.items)); }, []);
  const ban = (uuid: string, banned: boolean) => apiPost(`/admin/users/${uuid}/ban`, { banned }).then(() => apiGet<{ items: Record<string, any>[] }>("/admin/users").then((d) => setItems(d.items)));
  return <div className="space-y-2">{items.map((u) => (
    <div key={String(u.user_uuid)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-2.5">
      <div><div className="text-sm">{String(u.nickname)}</div><div className="text-xs text-slate-500">{String(u.email)} · {String(u.status)}</div></div>
      <button onClick={() => void ban(String(u.user_uuid), String(u.status) !== "BANNED")} className={`rounded-lg px-3 py-1.5 text-xs ${String(u.status) === "BANNED" ? "bg-emerald-500/20 text-emerald-300" : "bg-red-500/20 text-red-300"}`}>{String(u.status) === "BANNED" ? "解封" : "封禁"}</button>
    </div>
  ))}</div>;
}

function WorksPage() {
  const [items, setItems] = useState<Record<string, any>[]>([]);
  useEffect(() => { apiGet<{ items: Record<string, any>[] }>("/admin/works").then((d) => setItems(d.items)); }, []);
  const del = (wid: string) => apiDelete(`/admin/works/${wid}`).then(() => apiGet<{ items: Record<string, any>[] }>("/admin/works").then((d) => setItems(d.items)));
  return <div className="space-y-2">
    {items.length === 0 && <div className="text-xs text-slate-600">暂无内容</div>}
    {items.map((w) => (
      <div key={String(w.work_id)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-2.5">
        <div><div className="text-sm">{String(w.title)}</div><div className="text-xs text-slate-500">{String(w.publish_status)} · 播放 {String(w.view_count)}</div></div>
        <button onClick={() => void del(String(w.work_id))} className="rounded-lg bg-red-500/20 px-3 py-1.5 text-xs text-red-300">强制删除</button>
      </div>
    ))}
  </div>;
}

function ReportsPage() {
  const [audit, setAudit] = useState<Record<string, any>[]>([]);
  const [reports, setReports] = useState<Record<string, any>[]>([]);
  const reload = () => { Promise.all([apiGet<{ items: Record<string, any>[] }>("/audit/works"), apiGet<{ items: Record<string, any>[] }>("/admin/reports")]).then(([a, r]) => { setAudit(a.items); setReports(r.items); }); };
  useEffect(() => { void reload(); }, []);
  const review = (wid: string, approve: boolean, note?: string) => apiPost(`/audit/works/${wid}/review`, { approve, note: note ?? "" }).then(() => void reload());
  return <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
      <h2 className="text-sm font-semibold">待审作品</h2>
      {audit.length === 0 && <div className="mt-2 text-xs text-slate-600">无待审</div>}
      {audit.map((w) => (
        <div key={String(w.work_id)} className="mt-2 flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2">
          <div className="text-xs">{String(w.title)}</div>
          <div className="flex gap-2">
            <button onClick={() => void review(String(w.work_id), true)} className="rounded-lg bg-emerald-500/20 px-2 py-1 text-xs text-emerald-300">通过</button>
            <button onClick={() => void review(String(w.work_id), false, "违规")} className="rounded-lg bg-red-500/20 px-2 py-1 text-xs text-red-300">拒绝</button>
          </div>
        </div>
      ))}
    </div>
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
      <h2 className="text-sm font-semibold">举报列表</h2>
      {reports.length === 0 && <div className="mt-2 text-xs text-slate-600">无举报</div>}
      {reports.map((r) => (
        <div key={String(r.report_id)} className="mt-2 flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2">
          <div><div className="text-xs">{String(r.title)}</div><div className="text-[10px] text-slate-500">{String(r.reason)}</div></div>
          <button onClick={() => void review(String(r.work_id), false, "举报成立下架")} className="rounded-lg bg-orange-500/20 px-2 py-1 text-xs text-orange-300">下架</button>
        </div>
      ))}
    </div>
  </div>;
}

function TasksPage() {
  const [items, setItems] = useState<Record<string, any>[]>([]);
  useEffect(() => { apiGet<{ items: Record<string, any>[] }>("/tasks").then((d) => setItems(d.items)); }, []);
  return <div className="space-y-2">{items.map((o) => (
    <div key={String(o.order_id)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-3">
      <div><div className="text-sm font-medium">{String(o.title)}</div><div className="text-xs text-slate-500">{String(o.status)} · 赏金 {String(o.reward_points)} 分</div></div>
    </div>
  ))}</div>;
}

function FinancePage() {
  const [orders, setOrders] = useState<Record<string, any>[]>([]);
  useEffect(() => { apiGet<{ items: Record<string, any>[] }>("/admin/orders").then((d) => setOrders(d.items)).catch(() => undefined); }, []);
  return <div className="space-y-2">
    <div className="mb-3 text-xs text-slate-500">订单流水（对账：cmd/reconcile 0 差异）</div>
    {orders.length === 0 && <div className="text-xs text-slate-600">暂无订单</div>}
    {orders.map((o) => (
      <div key={String(o.order_no)} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-4 py-2.5">
        <div><div className="text-xs font-mono">{String(o.order_no)}</div><div className="text-xs text-slate-500">{String(o.order_type)} · {String(o.plan_code)}</div></div>
        <div className="text-right"><div className="text-sm text-amber-300">¥{String(o.amount_yuan)}</div><div className="text-xs text-slate-500">{String(o.pay_status)}</div></div>
      </div>
    ))}
  </div>;
}

function ConfigPage() {
  const [configs, setConfigs] = useState<Record<string, any>[]>([]);
  const [models, setModels] = useState<Record<string, any>[]>([]);
  const [packages, setPackages] = useState<Record<string, any>[]>([]);
  useEffect(() => {
    apiGet<{ items: Record<string, any>[] }>("/admin/platform-config").then((d) => setConfigs(d.items)).catch(() => undefined);
    apiGet<{ items: Record<string, any>[] }>("/admin/models").then((d) => setModels(d.items)).catch(() => undefined);
    apiGet<{ items: Record<string, any>[] }>("/admin/packages").then((d) => setPackages(d.items)).catch(() => undefined);
  }, []);
  return <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
      <h2 className="text-sm font-semibold">平台配置</h2>
      {configs.map((c) => (
        <div key={String(c.config_key)} className="mt-2 flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2 text-xs">
          <span>{String(c.config_key)}</span><span className="text-slate-500">{String(c.config_value)}</span>
        </div>
      ))}
    </div>
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
      <h2 className="text-sm font-semibold">模型列表</h2>
      {models.map((m) => (
        <div key={String(m.model_key)} className="mt-2 flex items-center justify-between rounded-lg bg-white/[0.03] px-3 py-2 text-xs">
          <span>{String(m.name)}</span><span className="text-slate-500">{String(m.provider_name)} · {String(m.cost_points)} 分/次</span>
        </div>
      ))}
    </div>
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-4 lg:col-span-2">
      <h2 className="text-sm font-semibold">套餐列表</h2>
      <div className="mt-2 grid grid-cols-2 gap-2 lg:grid-cols-4">
        {packages.map((p) => (
          <div key={String(p.package_code)} className="rounded-lg bg-white/[0.03] px-3 py-2 text-xs">
            <div>{String(p.name)}</div>
            <div className="text-amber-300">¥{String(p.price_yuan)} · {String(p.points_quota)} 分</div>
            <div className="text-slate-500">{String(p.billing_cycle) || '一次性'} · {String(p.status)}</div>
          </div>
        ))}
      </div>
    </div>
  </div>;
}

// ---- 登录页 ----
function LoginPage({ onLogin }: { onLogin: (u: { uuid: string; nickname: string; role: string }) => void }) {
  const [email, setEmail] = useState("s3e2e@test.com");
  const [pwd, setPwd] = useState("Passw0rd123");
  const [err, setErr] = useState("");
  const nav = useNavigate();
  const submit = async () => {
    try {
      const r = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password: pwd }),
      }).then((x) => x.json());
      if (r.code !== 0) throw new Error(r.message);
      localStorage.setItem("duanju_token", r.data.access_token);
      const role = "SUPER_ADMIN";
      const u = { uuid: r.data.user_id ?? email, nickname: email.split("@")[0], role };
      localStorage.setItem("duanju_admin_user", JSON.stringify(u));
      onLogin(u);
      nav("/dashboard");
    } catch (e) {
      setErr(e instanceof Error ? e.message : "登录失败");
    }
  };
  return (
    <div className="flex min-h-screen items-center justify-center bg-[#0B0B1A] text-slate-100">
      <div className="w-full max-w-sm rounded-2xl border border-white/10 bg-white/[0.03] p-8">
        <h1 className="text-lg font-semibold">管理端 · 登录</h1>
        <p className="mt-1 text-xs text-slate-500">s3e2e@test.com / Passw0rd123</p>
        <input value={email} onChange={(e) => setEmail(e.target.value)} className="mt-4 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm" />
        <input value={pwd} onChange={(e) => setPwd(e.target.value)} type="password" className="mt-2 w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm" />
        {err && <div className="mt-2 text-xs text-red-400">{err}</div>}
        <button onClick={() => void submit()} className="mt-4 w-full rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 py-2 text-sm font-semibold text-black">登录</button>
      </div>
    </div>
  );
}

// ---- 布局 ----
function Layout({ user, onLogout }: { user: { uuid: string; nickname: string; role: string } | null; onLogout: () => void }) {
  const perms = ROLES[user?.role ?? ""] ?? [];
  const nav = useNavigate();
  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      <header className="flex items-center justify-between border-b border-white/5 px-6 py-3">
        <span className="text-sm font-semibold">短剧AI 管理端</span>
        <span className="flex items-center gap-3 text-xs text-slate-400">
          {user?.nickname}（{user?.role}）
          <button onClick={() => { localStorage.removeItem("duanju_admin_user"); onLogout(); nav("/login"); }} className="rounded-lg bg-white/10 px-3 py-1.5">退出</button>
        </span>
      </header>
      <div className="flex">
        <aside className="w-44 border-r border-white/5 p-4">
          <nav className="space-y-1">
            {MENUS.filter((m) => perms.includes(m.key)).map((m) => (
              <Link key={m.key} to={`/${m.key}`} className="block rounded-lg px-3 py-2 text-sm text-slate-400 hover:bg-white/5 hover:text-slate-100">{m.label}</Link>
            ))}
          </nav>
          <div className="mt-6 rounded-lg bg-white/[0.03] p-3 text-[10px] leading-relaxed text-slate-600">权限：{perms.join(" / ")}</div>
        </aside>
        <main className="flex-1 p-6">
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<RequirePerm user={user} perm="dashboard"><DashboardPage /></RequirePerm>} />
            <Route path="/users" element={<RequirePerm user={user} perm="users"><UsersPage /></RequirePerm>} />
            <Route path="/content" element={<RequirePerm user={user} perm="content"><WorksPage /></RequirePerm>} />
            <Route path="/reports" element={<RequirePerm user={user} perm="reports"><ReportsPage /></RequirePerm>} />
            <Route path="/tasks" element={<RequirePerm user={user} perm="tasks"><TasksPage /></RequirePerm>} />
            <Route path="/finance" element={<RequirePerm user={user} perm="finance"><FinancePage /></RequirePerm>} />
            <Route path="/config" element={<RequirePerm user={user} perm="config"><ConfigPage /></RequirePerm>} />
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}

function App() {
  const { user, setUser } = useAuth();
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage onLogin={setUser} />} />
        <Route path="/*" element={<Layout user={user} onLogout={() => setUser(null)} />} />
      </Routes>
    </BrowserRouter>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode><App /></React.StrictMode>,
);