// S1-F02 项目列表页：卡片列表 + 创建对话框 + 删除确认 + 状态徽章（3.0.1~3.0.7）。
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import {
  projectsApi,
  ProjectStatusMeta,
  type ProjectSummary,
  type ProjectStatusValue,
} from "@duanju/api-client";

// 状态徽章色调
const toneClass: Record<string, string> = {
  gold: "bg-amber-400/15 text-amber-300 border-amber-400/30",
  violet: "bg-violet-400/15 text-violet-300 border-violet-400/30",
  blue: "bg-sky-400/15 text-sky-300 border-sky-400/30",
  green: "bg-emerald-400/15 text-emerald-300 border-emerald-400/30",
  gray: "bg-slate-400/15 text-slate-300 border-slate-400/30",
};

function StatusBadge({ status }: { status: ProjectStatusValue }) {
  const meta = ProjectStatusMeta[status] ?? { label: status, tone: "gray" as const };
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${toneClass[meta.tone]}`}
    >
      {meta.label}
    </span>
  );
}

// 时长格式化：秒 → mm:ss
function fmtDuration(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export default function ProjectListPage() {
  const nav = useNavigate();
  const [items, setItems] = useState<ProjectSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [title, setTitle] = useState("");
  const [script, setScript] = useState("");
  const [toast, setToast] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<ProjectSummary | null>(null);
  const [deleting, setDeleting] = useState(false);

  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 2500);
  }, []);

  const load = useCallback(async () => {
    try {
      setLoading(true);
      const data = await projectsApi.list();
      setItems(data.items);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    void load();
  }, [load]);

  // 创建项目 → 创建即进 Step1（S1-F02 验收）
  const handleCreate = async () => {
    if (!title.trim()) {
      showToast("请输入项目名称");
      return;
    }
    try {
      setCreating(true);
      const data = await projectsApi.create({
        title: title.trim(),
        source_script_text: script.trim() || undefined,
      });
      showToast("项目创建成功");
      setShowCreate(false);
      setTitle("");
      setScript("");
      nav(`/projects/${data.project_id}`);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "创建失败");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async () => {
    if (!confirmDelete) return;
    try {
      setDeleting(true);
      await projectsApi.remove(confirmDelete.project_id);
      showToast("项目已删除");
      setConfirmDelete(null);
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "删除失败");
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      {/* 顶栏 */}
      <header className="sticky top-0 z-10 border-b border-white/5 bg-[#0B0B1A]/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-amber-400 to-amber-600 text-sm font-bold text-black">
              剧
            </div>
            <h1 className="text-lg font-semibold tracking-wide text-slate-100">我的作品</h1>
          </div>
          <button
            onClick={() => setShowCreate(true)}
            className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-5 py-2 text-sm font-semibold text-black shadow-lg shadow-amber-500/20 transition hover:from-amber-300 hover:to-amber-400"
          >
            + 新建项目
          </button>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-8">
        {loading ? (
          <div className="flex h-64 items-center justify-center text-sm text-slate-400">
            加载中…
          </div>
        ) : items.length === 0 ? (
          <div className="flex h-72 flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-white/10">
            <p className="text-sm text-slate-400">还没有作品，创建你的第一部短剧吧</p>
            <button
              onClick={() => setShowCreate(true)}
              className="rounded-lg border border-amber-400/40 px-4 py-2 text-sm text-amber-300 transition hover:bg-amber-400/10"
            >
              开始创作
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {items.map((p) => {
              const meta = ProjectStatusMeta[p.status] ?? { label: p.status, tone: "gray" as const };
              return (
                <div
                  key={p.project_id}
                  className="group cursor-pointer overflow-hidden rounded-2xl border border-white/5 bg-white/[0.03] transition hover:border-amber-400/30 hover:bg-white/[0.05]"
                  onClick={() => {
                    void projectsApi.touch(p.project_id).catch(() => undefined);
                    nav(`/projects/${p.project_id}`);
                  }}
                >
                  {/* 封面占位：状态氛围色 */}
                  <div
                    className={`relative flex h-32 items-center justify-center bg-gradient-to-br ${
                      meta.tone === "gold"
                        ? "from-amber-500/20 to-amber-700/10"
                        : meta.tone === "violet"
                          ? "from-violet-500/20 to-violet-700/10"
                          : "from-sky-500/20 to-sky-700/10"
                    }`}
                  >
                    <span className="text-3xl text-white/20">🎬</span>
                    <span className="absolute right-3 top-3">
                      <StatusBadge status={p.status} />
                    </span>
                  </div>
                  <div className="p-4">
                    <div className="flex items-start justify-between gap-2">
                      <h3 className="truncate text-sm font-semibold text-slate-100">{p.title}</h3>
                      <button
                        className="text-xs text-slate-500 opacity-0 transition group-hover:opacity-100 hover:text-red-400"
                        onClick={(e) => {
                          e.stopPropagation();
                          setConfirmDelete(p);
                        }}
                      >
                        删除
                      </button>
                    </div>
                    <div className="mt-2 flex items-center gap-3 text-xs text-slate-500">
                      <span>{p.total_episode_count} 集</span>
                      <span>·</span>
                      <span>{fmtDuration(p.total_duration_sec)}</span>
                    </div>
                    <div className="mt-1 text-xs text-slate-600">
                      更新于 {new Date(p.updated_at).toLocaleString()}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </main>

      {/* 创建对话框 */}
      {showCreate && (
        <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={() => setShowCreate(false)}>
          <div className="w-full max-w-md rounded-2xl border border-white/10 bg-[#12122A] p-6 shadow-2xl" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-base font-semibold text-slate-100">新建项目</h2>
            <p className="mt-1 text-xs text-slate-500">创建后立即进入剧本拆解（Step1）</p>
            <div className="mt-4 space-y-3">
              <div>
                <label className="mb-1 block text-xs text-slate-400">项目名称 *</label>
                <input
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="例如：我的第一部竖屏短剧"
                  className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-slate-100 outline-none placeholder:text-slate-600 focus:border-amber-400/50"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs text-slate-400">剧本原文（可跳过，稍后在 Step1 输入）</label>
                <textarea
                  value={script}
                  onChange={(e) => setScript(e.target.value)}
                  rows={4}
                  placeholder="粘贴你的剧本…"
                  className="w-full resize-none rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-slate-100 outline-none placeholder:text-slate-600 focus:border-amber-400/50"
                />
              </div>
            </div>
            <div className="mt-5 flex justify-end gap-3">
              <button
                onClick={() => setShowCreate(false)}
                className="rounded-lg px-4 py-2 text-sm text-slate-400 transition hover:text-slate-200"
              >
                取消
              </button>
              <button
                onClick={() => void handleCreate()}
                disabled={creating}
                className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-5 py-2 text-sm font-semibold text-black transition hover:from-amber-300 hover:to-amber-400 disabled:opacity-50"
              >
                {creating ? "创建中…" : "创建并进入 Step1"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 删除确认 */}
      {confirmDelete && (
        <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={() => setConfirmDelete(null)}>
          <div className="w-full max-w-sm rounded-2xl border border-white/10 bg-[#12122A] p-6 shadow-2xl" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-base font-semibold text-slate-100">删除项目</h2>
            <p className="mt-2 text-sm text-slate-400">
              确定删除「{confirmDelete.title}」吗？删除后可在回收站恢复。
            </p>
            <div className="mt-5 flex justify-end gap-3">
              <button
                onClick={() => setConfirmDelete(null)}
                className="rounded-lg px-4 py-2 text-sm text-slate-400 transition hover:text-slate-200"
              >
                取消
              </button>
              <button
                onClick={() => void handleDelete()}
                disabled={deleting}
                className="rounded-lg bg-red-500/90 px-5 py-2 text-sm font-semibold text-white transition hover:bg-red-500 disabled:opacity-50"
              >
                {deleting ? "删除中…" : "确认删除"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Toast */}
      {toast && (
        <div className="fixed bottom-6 left-1/2 z-30 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200 shadow-xl">
          {toast}
        </div>
      )}
    </div>
  );
}