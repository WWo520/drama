// S1-F05 Step3 视频页：叙事单元列表 → 生成视频 → 分镜列表（播放器 + 状态轮询）。
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import {
  projectsApi,
  splitApi,
  videoApi,
  VideoStatusMeta,
  type BeatSummary,
  type EpisodeLite,
  type ProjectDetail,
  type ShotVideo,
} from "@duanju/api-client";

import { useStepState } from "../hooks/useStepState";

const toneClass: Record<string, string> = {
  gold: "bg-amber-400/15 text-amber-300 border-amber-400/30",
  green: "bg-emerald-400/15 text-emerald-300 border-emerald-400/30",
  red: "bg-red-400/15 text-red-300 border-red-400/30",
  gray: "bg-slate-400/15 text-slate-300 border-slate-400/30",
};

function StatusBadge({ status }: { status: string }) {
  const meta = VideoStatusMeta[status] ?? { label: status, tone: "gray" };
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] ${toneClass[meta.tone] ?? toneClass.gray}`}
    >
      {meta.label}
    </span>
  );
}

// 单个镜头卡片（状态 + 播放器）
function ShotCard({ shot }: { shot: ShotVideo }) {
  const done = shot.video_generation_status === "SUCCESS";
  return (
    <div
      className={`overflow-hidden rounded-xl border transition ${
        done ? "border-emerald-400/20 bg-emerald-400/[0.03]" : "border-white/5 bg-white/[0.03]"
      }`}
    >
      <div className="relative flex h-28 items-center justify-center bg-black/40">
        {done && shot.generated_video_url ? (
          <video src={shot.generated_video_url} controls className="h-full w-full object-contain" preload="metadata" />
        ) : shot.video_generation_status === "RUNNING" ? (
          <div className="flex flex-col items-center gap-2">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-amber-400/30 border-t-amber-400" />
            <span className="text-[10px] text-amber-300/80">视频渲染中…</span>
          </div>
        ) : (
          <span className="text-2xl text-white/15">🎬</span>
        )}
        <span className="absolute right-2 top-2">
          <StatusBadge status={shot.video_generation_status} />
        </span>
      </div>
      <div className="flex items-center justify-between px-3 py-2">
        <span className="text-xs text-slate-400">镜头 {shot.sort_order}</span>
        <span className="text-[10px] text-slate-600">{shot.duration_sec}s</span>
      </div>
    </div>
  );
}

// 叙事单元卡片：头部（5 件套简化）+ 生成按钮 + 镜头网格
function BeatVideoCard({
  beat,
  shots,
  generating,
  onGenerate,
}: {
  beat: BeatSummary;
  shots: ShotVideo[];
  generating: boolean;
  onGenerate: () => void;
}) {
  const doneCount = shots.filter((s) => s.video_generation_status === "SUCCESS").length;
  const running = shots.some((s) => s.video_generation_status === "RUNNING");
  const allDone = shots.length > 0 && doneCount === shots.length;

  return (
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-5">
      {/* 头部行 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-amber-400/15 text-xs font-bold text-amber-300">
            {beat.sort_order}
          </span>
          <h3 className="text-sm font-semibold text-slate-100">{beat.title}</h3>
          <span className="text-xs text-slate-500">
            规划{beat.preset_duration_sec}秒 · {shots.length} 镜头
          </span>
        </div>
        <button
          onClick={onGenerate}
          disabled={generating || (running && !allDone)}
          className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition disabled:cursor-not-allowed disabled:opacity-40 ${
            allDone
              ? "bg-emerald-400/15 text-emerald-300"
              : "bg-gradient-to-r from-amber-400 to-amber-500 text-black hover:from-amber-300 hover:to-amber-400"
          }`}
        >
          {allDone ? "✅ 已完成" : running ? "⏸ 生成中…" : generating ? "提交中…" : "● 生成视频"}
        </button>
      </div>
      {/* 进度 */}
      {shots.length > 0 && (
        <div className="mt-3 flex items-center gap-3">
          <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-white/5">
            <div
              className="h-full rounded-full bg-gradient-to-r from-amber-400 to-amber-500 transition-all"
              style={{ width: `${shots.length ? (doneCount / shots.length) * 100 : 0}%` }}
            />
          </div>
          <span className="text-[10px] text-slate-500">
            {doneCount}/{shots.length}
          </span>
        </div>
      )}
      {/* 镜头网格 */}
      {shots.length > 0 && (
        <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {shots.map((s) => (
            <ShotCard key={s.shot_id} shot={s} />
          ))}
        </div>
      )}
    </div>
  );
}

export default function Step3Page() {
  const { projectId = "" } = useParams();
  const nav = useNavigate();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [shotsMap, setShotsMap] = useState<Record<string, ShotVideo[]>>({});
  const [generatingBeat, setGeneratingBeat] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stepState = useStepState(projectId, proj?.status ?? null);
  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
  }, []);

  // 加载项目 + 剧集 + beats
  const loadAll = useCallback(async () => {
    try {
      const p = await projectsApi.detail(projectId);
      setProj(p);
      const eps = await splitApi.episodes(projectId);
      setEpisodes(eps.items ?? []);
      if (eps.items.length > 0) {
        const ep = activeEpisode ?? eps.items[0].episode_id;
        setActiveEpisode(ep);
        const b = await splitApi.beats(ep);
        setBeats(b.items ?? []);
      }
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载失败");
    }
  }, [projectId, activeEpisode, showToast]);

  // 加载各 beat 的 shots
  const loadShots = useCallback(async (beatList: BeatSummary[]) => {
    const map: Record<string, ShotVideo[]> = {};
    for (const b of beatList) {
      try {
        const d = await videoApi.beatShots(b.beat_id);
        map[b.beat_id] = d.items ?? [];
      } catch {
        map[b.beat_id] = [];
      }
    }
    setShotsMap(map);
    return map;
  }, []);

  useEffect(() => {
    void loadAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, activeEpisode]);

  useEffect(() => {
    if (beats.length > 0) void loadShots(beats);
  }, [beats, loadShots]);

  // 有 RUNNING → 3s 轮询 shots
  useEffect(() => {
    const anyRunning = beats.some((b) =>
      (shotsMap[b.beat_id] ?? []).some((s) => s.video_generation_status === "RUNNING"),
    );
    if (anyRunning && !pollRef.current) {
      pollRef.current = setInterval(async () => {
        try {
          const map = await loadShots(beats);
          const still = beats.some((b) =>
            (map[b.beat_id] ?? []).some((s) => s.video_generation_status === "RUNNING"),
          );
          if (!still && pollRef.current) {
            clearInterval(pollRef.current);
            pollRef.current = null;
            showToast("视频生成完成");
          }
        } catch {
          /* 忽略 */
        }
      }, 3000);
    } else if (!anyRunning && pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, [shotsMap, beats, loadShots, showToast]);

  // WS 实时进度（S1-F08）：TASK_PROGRESS/TASK_SUCCESS → 主动刷新 shots；断线时轮询兜底无缝衔接
  useEffect(() => {
    const handler = (ev: Event) => {
      const data = (ev as CustomEvent).detail as { task_uuid?: string } | undefined;
      if (data?.task_uuid) {
        void loadShots(beats);
      }
    };
    window.addEventListener("task:progress", handler);
    return () => window.removeEventListener("task:progress", handler);
  }, [beats, loadShots]);

  // 生成某 beat 的视频
  const handleGenerate = async (beatId: string) => {
    try {
      setGeneratingBeat(beatId);
      const r = await videoApi.generateBeat(beatId);
      showToast(`已提交 ${r.created}/${r.total} 个视频生成任务`);
      // 立即刷新状态
      const d = await videoApi.beatShots(beatId);
      setShotsMap((prev) => ({ ...prev, [beatId]: d.items ?? [] }));
    } catch (e) {
      showToast(e instanceof Error ? e.message : "触发失败");
    } finally {
      setGeneratingBeat(null);
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      <header className="sticky top-0 z-10 border-b border-white/5 bg-[#0B0B1A]/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
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

      <main className="mx-auto max-w-6xl px-6 py-6">
        {/* 剧集切换 */}
        {episodes.length > 1 && (
          <div className="mb-4 flex items-center gap-2">
            <span className="text-xs text-slate-500">剧集：</span>
            {episodes.map((ep) => (
              <button
                key={ep.episode_id}
                onClick={() => setActiveEpisode(ep.episode_id)}
                className={`rounded-full px-3.5 py-1 text-xs transition ${
                  activeEpisode === ep.episode_id ? "bg-amber-400/20 text-amber-300" : "text-slate-400 hover:bg-white/5"
                }`}
              >
                第 {ep.episode_number} 集
              </button>
            ))}
          </div>
        )}

        {/* 叙事单元 → 生成视频 → 分镜列表 */}
        {beats.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-white/10 p-14 text-center text-sm text-slate-500">
            暂无叙事单元。完成剧本拆解（Step1）后，这里将显示分镜视频列表。
          </div>
        ) : (
          <div className="space-y-4">
            {beats.map((b) => (
              <BeatVideoCard
                key={b.beat_id}
                beat={b}
                shots={shotsMap[b.beat_id] ?? []}
                generating={generatingBeat === b.beat_id}
                onGenerate={() => void handleGenerate(b.beat_id)}
              />
            ))}
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