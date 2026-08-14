// S1-F04 Step2 资产页：角色/场景/道具卡片 + 批量生成 + 预签名直传 + 生成进度轮询。
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import {
  projectsApi,
  splitApi,
  assetsApi,
  GenStatusMeta,
  type CharacterItem,
  type ProjectDetail,
} from "@duanju/api-client";

import { useStepState } from "../hooks/useStepState";

// 生成状态徽章色调
const toneClass: Record<string, string> = {
  gold: "bg-amber-400/15 text-amber-300 border-amber-400/30",
  green: "bg-emerald-400/15 text-emerald-300 border-emerald-400/30",
  red: "bg-red-400/15 text-red-300 border-red-400/30",
  blue: "bg-sky-400/15 text-sky-300 border-sky-400/30",
  gray: "bg-slate-400/15 text-slate-300 border-slate-400/30",
};

function StatusBadge({ status }: { status: string }) {
  const meta = GenStatusMeta[status] ?? { label: status, tone: "gray" };
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] ${toneClass[meta.tone] ?? toneClass.gray}`}
    >
      {meta.label}
    </span>
  );
}

// 角色卡片（头像 + 名称 + 状态 + 选中生成）
function CharacterCard({
  ch,
  selected,
  onToggle,
}: {
  ch: CharacterItem;
  selected: boolean;
  onToggle: (id: number) => void;
}) {
  const done = ch.generation_status === "SUCCESS";
  return (
    <div
      className={`relative overflow-hidden rounded-2xl border transition ${
        selected ? "border-amber-400/50 bg-amber-400/5" : "border-white/5 bg-white/[0.03] hover:border-white/15"
      }`}
    >
      {/* 头像区 */}
      <div className="relative flex h-36 items-center justify-center bg-gradient-to-br from-violet-500/15 to-violet-800/5">
        {ch.main_image_url ? (
          <img src={ch.main_image_url} alt={ch.name} className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-white/10 text-2xl text-white/30">
            {ch.name.slice(0, 1)}
          </div>
        )}
        <span className="absolute right-2 top-2">
          <StatusBadge status={ch.generation_status} />
        </span>
        {done && (
          <span className="absolute left-2 top-2 rounded-full bg-emerald-500/80 px-2 py-0.5 text-[10px] font-medium text-white">
            可复用
          </span>
        )}
      </div>
      {/* 信息区 */}
      <div className="p-3">
        <div className="flex items-center justify-between gap-2">
          <h4 className="truncate text-sm font-semibold text-slate-100">{ch.name}</h4>
          <button
            onClick={() => onToggle(ch.id)}
            disabled={done}
            className={`rounded-md px-2.5 py-1 text-[11px] font-medium transition disabled:cursor-not-allowed ${
              selected
                ? "bg-amber-400 text-black"
                : done
                  ? "bg-white/5 text-slate-600"
                  : "bg-white/10 text-slate-300 hover:bg-amber-400/20 hover:text-amber-300"
            }`}
          >
            {done ? "✓ 已生成" : selected ? "已选中" : "生成"}
          </button>
        </div>
        {ch.description_notes && (
          <p className="mt-1 line-clamp-2 text-[11px] leading-relaxed text-slate-500">{ch.description_notes}</p>
        )}
      </div>
    </div>
  );
}

export default function Step2Page() {
  const { projectId = "" } = useParams();
  const nav = useNavigate();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodeId, setEpisodeId] = useState<string | null>(null);
  const [chars, setChars] = useState<CharacterItem[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [tab, setTab] = useState<"characters" | "scenes" | "props">("characters");
  const [generating, setGenerating] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stepState = useStepState(projectId, proj?.status ?? null);
  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadChars = useCallback(async () => {
    if (!episodeId) return;
    try {
      const data = await assetsApi.characters(episodeId);
      setChars(data.items ?? []);
      // 有生成中的角色 → 保持轮询
      const running = (data.items ?? []).some(
        (c) => c.generation_status === "QUEUED" || c.generation_status === "RUNNING",
      );
      if (running && !pollRef.current) {
        pollRef.current = setInterval(async () => {
          try {
            const d = await assetsApi.characters(episodeId);
            setChars(d.items ?? []);
            const stillRunning = (d.items ?? []).some(
              (c) => c.generation_status === "QUEUED" || c.generation_status === "RUNNING",
            );
            if (!stillRunning && pollRef.current) {
              clearInterval(pollRef.current);
              pollRef.current = null;
              showToast("资产生成完成");
            }
          } catch {
            /* 忽略 */
          }
        }, 2500);
      } else if (!running && pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载角色失败");
    }
  }, [episodeId, showToast]);

  // 初始化：项目 + 剧集 → 角色
  useEffect(() => {
    void (async () => {
      try {
        const p = await projectsApi.detail(projectId);
        setProj(p);
        const eps = await splitApi.episodes(projectId);
        if (eps.items.length > 0) setEpisodeId(eps.items[0].episode_id);
      } catch (e) {
        showToast(e instanceof Error ? e.message : "加载失败");
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  useEffect(() => {
    if (episodeId) void loadChars();
  }, [episodeId, loadChars]);

  // 批量生成选中的角色
  const handleGenerate = async () => {
    if (!episodeId || selected.size === 0) return;
    try {
      setGenerating(true);
      const data = await assetsApi.batchGenerate({
        episode_id: episodeId,
        items: [...selected].map((id) => ({ kind: "CHARACTER_VARIANT", target_id: id })),
      });
      showToast(`已提交 ${data.created} 个生成任务`);
      setSelected(new Set());
      void loadChars();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "批量生成失败");
    } finally {
      setGenerating(false);
    }
  };

  // 预签名直传（3.4.4 → PUT MinIO → 3.4.5 回执）
  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    try {
      setUploading(true);
      const presign = await assetsApi.presign({
        biz_type: "reference_image",
        file_name: file.name,
        content_type: file.type || "image/png",
        size: file.size,
      });
      // 直连 MinIO PUT（预签名 URL 自带鉴权）
      const resp = await fetch(presign.upload_url, {
        method: "PUT",
        headers: { "Content-Type": file.type || "image/png" },
        body: file,
      });
      if (!resp.ok) throw new Error(`上传失败（HTTP ${resp.status}）`);
      const etag = (resp.headers.get("ETag") ?? "").replace(/"/g, "");
      await assetsApi.complete({
        upload_token: presign.upload_token,
        object_key: presign.object_key,
        etag,
        biz_ref_id: episodeId ?? "",
      });
      showToast("参考图上传成功");
    } catch (err) {
      showToast(err instanceof Error ? err.message : "上传失败");
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      {/* 顶栏 */}
      <header className="sticky top-0 z-10 border-b border-white/5 bg-[#0B0B1A]/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-3">
            <button onClick={() => nav("/projects")} className="text-sm text-slate-400 transition hover:text-amber-300">
              ← 返回
            </button>
            <h1 className="text-base font-semibold">{proj?.title ?? "加载中…"}</h1>
          </div>
          <div className="flex items-center gap-3">
            <span className="rounded-full border border-white/10 bg-white/5 px-2.5 py-0.5 text-xs text-slate-400">
              积分：{(proj as ProjectDetail | null)?.total_episode_count ?? "-"} 集
            </span>
            <button
              onClick={() => nav("/points")}
              className="rounded-lg bg-amber-400/10 px-3 py-1.5 text-xs text-amber-300 transition hover:bg-amber-400/20"
            >
              充值
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-6">
        {/* StepBar 行 */}
        <div className="mb-6 flex items-center justify-between">
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
          {/* 操作区：上传参考图 + 批量生成 */}
          <div className="flex items-center gap-3">
            <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={(e) => void handleFile(e)} />
            <button
              onClick={() => fileRef.current?.click()}
              disabled={uploading}
              className="rounded-lg border border-white/15 px-4 py-2 text-xs text-slate-300 transition hover:bg-white/5 disabled:opacity-50"
            >
              {uploading ? "上传中…" : "上传参考图"}
            </button>
            <button
              onClick={() => void handleGenerate()}
              disabled={generating || selected.size === 0}
              className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-5 py-2 text-xs font-semibold text-black shadow-lg shadow-amber-500/20 transition hover:from-amber-300 hover:to-amber-400 disabled:opacity-40"
            >
              {generating ? "提交中…" : `批量生成（${selected.size}）`}
            </button>
          </div>
        </div>

        {/* 三 Tab */}
        <div className="mb-5 flex gap-2">
          {(
            [
              ["characters", "角色"],
              ["scenes", "场景"],
              ["props", "道具"],
            ] as const
          ).map(([key, label]) => (
            <button
              key={key}
              onClick={() => setTab(key)}
              className={`rounded-lg px-4 py-1.5 text-xs transition ${
                tab === key ? "bg-white/10 font-semibold text-amber-300" : "text-slate-400 hover:bg-white/5"
              }`}
            >
              {label}
            </button>
          ))}
        </div>

        {/* 内容区 */}
        {tab === "characters" && (
          chars.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-white/10 p-14 text-center text-sm text-slate-500">
              暂无角色。完成剧本拆解后角色将出现在这里，点击「生成」创建角色形象。
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
              {chars.map((ch) => (
                <CharacterCard
                  key={ch.id}
                  ch={ch}
                  selected={selected.has(ch.id)}
                  onToggle={(id) => {
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (next.has(id)) next.delete(id);
                      else next.add(id);
                      return next;
                    });
                  }}
                />
              ))}
            </div>
          )
        )}
        {tab === "scenes" && (
          <div className="rounded-2xl border border-dashed border-white/10 p-14 text-center text-sm text-slate-500">
            场景资产：后续批次开放（3.2.10~3.2.14）。
          </div>
        )}
        {tab === "props" && (
          <div className="rounded-2xl border border-dashed border-white/10 p-14 text-center text-sm text-slate-500">
            道具资产：后续批次开放（3.2.15~3.2.18）。
          </div>
        )}
      </main>

      {toast && (
        <div className="fixed bottom-6 left-1/2 z-30 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200 shadow-xl">
          {toast}
        </div>
      )}
    </div>
  );
}