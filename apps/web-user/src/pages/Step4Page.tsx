// S1-F06 Step4 导出页：清晰度选择 → 触发导出 → 任务轮询 → 下载成片。
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { projectsApi, exportApi, type ExportItem, type ProjectDetail } from "@duanju/api-client";

import { useStepState } from "../hooks/useStepState";

// 清晰度选项（05 3.4.1：P480/P720/P1080；S1-B08 三档）
const RESOLUTIONS = [
  { key: "P480", label: "480P", hint: "流畅 · 约 40MB/10min", cost: "0 积分" },
  { key: "P720", label: "720P", hint: "高清 · 约 80MB/10min", cost: "0 积分" },
  { key: "P1080", label: "1080P", hint: "全高清 · 约 160MB/10min", cost: "0 积分" },
];

const statusMeta: Record<string, { label: string; cls: string }> = {
  PENDING: { label: "导出中", cls: "bg-amber-400/15 text-amber-300 border-amber-400/30" },
  SUCCESS: { label: "已完成", cls: "bg-emerald-400/15 text-emerald-300 border-emerald-400/30" },
  FAILED: { label: "失败", cls: "bg-red-400/15 text-red-300 border-red-400/30" },
};

function fmtSize(bytes: number): string {
  if (bytes <= 0) return "-";
  const mb = bytes / 1024 / 1024;
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)}GB` : `${mb.toFixed(0)}MB`;
}

export default function Step4Page() {
  const { projectId = "" } = useParams();
  const nav = useNavigate();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [resolution, setResolution] = useState("P720");
  const [exports, setExports] = useState<ExportItem[]>([]);
  const [exporting, setExporting] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stepState = useStepState(projectId, proj?.status ?? null);
  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadExports = useCallback(async () => {
    try {
      const d = await exportApi.list(projectId);
      setExports(d.items ?? []);
      // 有 PENDING → 轮询
      const pending = (d.items ?? []).some((e) => e.status === "PENDING");
      if (pending && !pollRef.current) {
        pollRef.current = setInterval(async () => {
          try {
            const dd = await exportApi.list(projectId);
            setExports(dd.items ?? []);
            if (!(dd.items ?? []).some((e) => e.status === "PENDING") && pollRef.current) {
              clearInterval(pollRef.current);
              pollRef.current = null;
              showToast("导出完成");
            }
          } catch {
            /* 忽略 */
          }
        }, 3000);
      } else if (!pending && pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载导出历史失败");
    }
  }, [projectId, showToast]);

  useEffect(() => {
    void (async () => {
      try {
        const p = await projectsApi.detail(projectId);
        setProj(p);
        void loadExports();
      } catch (e) {
        showToast(e instanceof Error ? e.message : "加载失败");
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const handleExport = async () => {
    try {
      setExporting(true);
      await exportApi.trigger(projectId, resolution, true);
      showToast(`导出任务已提交（${resolution}）`);
      void loadExports();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "触发导出失败");
    } finally {
      setExporting(false);
    }
  };

  // 状态守卫：未完成视频不能导出（后端 70001）
  const canExport = proj?.status === "ALL_VIDEOS_READY" || proj?.status === "COMPLETED";

  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      <header className="sticky top-0 z-10 border-b border-white/5 bg-[#0B0B1A]/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between px-6">
          <div className="flex items-center gap-3">
            <button onClick={() => nav("/projects")} className="text-sm text-slate-400 transition hover:text-amber-300">
              ← 返回
            </button>
            <h1 className="text-base font-semibold">{proj?.title ?? "加载中…"}</h1>
          </div>
          <div className="flex items-center gap-1">
            {[1, 2, 3, 4].map((n) => (
              <button
                key={n}
                onClick={() => stepState.goStep(n)}
                disabled={!stepState.canClick(n)}
                className={`rounded-full px-3.5 py-1.5 text-xs transition ${
                  stepState.currentStep === n
                    ? "bg-gradient-to-r from-amber-400 to-amber-500 font-semibold text-black"
                    : stepState.canClick(n)
                      ? "text-slate-300 hover:bg-white/5"
                      : "cursor-not-allowed text-slate-600"
                }`}
              >
                Step{n}
              </button>
            ))}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6 py-8">
        {/* 导出配置 */}
        <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-slate-300">成片导出（MP4 H.264）</h2>
            {!canExport && proj && (
              <span className="text-xs text-amber-300/80">完成 Step3 全部视频生成后可导出</span>
            )}
          </div>
          <p className="mt-1 text-xs text-slate-500">自动拼接全部镜头 · 烧录字幕 · AIGC 合规水印</p>

          {/* 清晰度选择 */}
          <div className="mt-4 grid grid-cols-3 gap-3">
            {RESOLUTIONS.map((r) => (
              <button
                key={r.key}
                onClick={() => setResolution(r.key)}
                disabled={!canExport}
                className={`rounded-xl border p-4 text-left transition disabled:cursor-not-allowed disabled:opacity-40 ${
                  resolution === r.key
                    ? "border-amber-400/50 bg-amber-400/10"
                    : "border-white/10 bg-white/[0.02] hover:border-white/20"
                }`}
              >
                <div className="text-sm font-semibold">{r.label}</div>
                <div className="mt-1 text-[11px] text-slate-500">{r.hint}</div>
                <div className="mt-1 text-[11px] text-amber-300/70">{r.cost}</div>
              </button>
            ))}
          </div>

          <div className="mt-5 flex justify-end">
            <button
              onClick={() => void handleExport()}
              disabled={exporting || !canExport}
              className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-7 py-2.5 text-sm font-semibold text-black shadow-lg shadow-amber-500/20 transition hover:from-amber-300 hover:to-amber-400 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {exporting ? "提交中…" : "导出成片"}
            </button>
          </div>
        </div>

        {/* 导出历史 */}
        <div className="mt-6">
          <h3 className="mb-3 text-sm font-semibold text-slate-300">导出历史</h3>
          {exports.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-white/10 p-10 text-center text-sm text-slate-500">
              暂无导出记录
            </div>
          ) : (
            <div className="space-y-3">
              {exports.map((e) => {
                const meta = statusMeta[e.status] ?? { label: e.status, cls: "bg-white/10 text-slate-300" };
                return (
                  <div
                    key={e.export_id}
                    className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-5 py-3.5"
                  >
                    <div className="flex items-center gap-3">
                      <span
                        className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs ${meta.cls}`}
                      >
                        {e.status === "PENDING" ? "⏳ " : e.status === "SUCCESS" ? "✓ " : "✗ "}
                        {meta.label}
                      </span>
                      <span className="text-xs text-slate-400">{e.resolution}</span>
                      <span className="text-xs text-slate-600">{fmtSize(e.file_size)}</span>
                      <span className="text-xs text-slate-600">{new Date(e.created_at).toLocaleString()}</span>
                    </div>
                    {e.status === "SUCCESS" && e.output_url && (
                      <a
                        href={e.output_url}
                        target="_blank"
                        rel="noreferrer"
                        className="rounded-lg bg-emerald-400/15 px-4 py-1.5 text-xs font-semibold text-emerald-300 transition hover:bg-emerald-400/25"
                      >
                        下载成片
                      </a>
                    )}
                  </div>
                );
              })}
            </div>
          )}
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