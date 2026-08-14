// S1-F03 Step1 拆分页：剧本输入 → 触发拆分 → 轮询进度 → 叙事单元 5 件套渲染（03 E.2.2）。
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import {
  projectsApi,
  splitApi,
  ProjectStatusMeta,
  type BeatSummary,
  type EpisodeLite,
  type ProjectDetail,
  type ProjectStatusValue,
} from "@duanju/api-client";

import { useStepState } from "../hooks/useStepState";

// StepBar（A.5：Step 可点击规则）
function StepBar({
  current,
  canClick,
  goStep,
}: {
  current: number;
  canClick: (s: number) => boolean;
  goStep: (s: number) => void;
}) {
  const steps = [
    { n: 1, label: "拆解" },
    { n: 2, label: "资产" },
    { n: 3, label: "视频" },
    { n: 4, label: "导出" },
  ];
  return (
    <div className="flex items-center gap-1">
      {steps.map((s, i) => (
        <div key={s.n} className="flex items-center">
          <button
            onClick={() => goStep(s.n)}
            disabled={!canClick(s.n)}
            className={`flex items-center gap-1.5 rounded-full px-3.5 py-1.5 text-xs transition ${
              current === s.n
                ? "bg-gradient-to-r from-amber-400 to-amber-500 font-semibold text-black"
                : canClick(s.n)
                  ? "text-slate-300 hover:bg-white/5"
                  : "cursor-not-allowed text-slate-600"
            }`}
          >
            <span
              className={`flex h-4 w-4 items-center justify-center rounded-full text-[10px] ${
                current === s.n ? "bg-black/20" : "bg-white/10"
              }`}
            >
              {s.n}
            </span>
            {s.label}
          </button>
          {i < steps.length - 1 && <span className="mx-0.5 h-px w-4 bg-white/10" />}
        </div>
      ))}
    </div>
  );
}

// 叙事单元头部 5 件套（03 E.2.2）：序号+名称+时长 / [历史版本][重新规划][●生成视频] / 剧本原文展开
function BeatCard({ beat, episodeTitle }: { beat: BeatSummary; episodeTitle: string }) {
  const [expanded, setExpanded] = useState(false);
  const duration =
    beat.actual_duration_sec > 0
      ? `规划${beat.preset_duration_sec}秒·实际${beat.actual_duration_sec}秒`
      : `${beat.preset_duration_sec}秒`;

  return (
    <div className="overflow-hidden rounded-2xl border border-white/5 bg-white/[0.03] transition hover:border-white/10">
      {/* ① 头部行 */}
      <div className="flex items-center justify-between px-5 pt-4">
        <div className="flex items-center gap-2.5">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-amber-400/15 text-xs font-bold text-amber-300">
            {beat.sort_order}
          </span>
          <h3 className="text-sm font-semibold text-slate-100">{beat.title}</h3>
          <span className="text-xs text-slate-500">{duration}</span>
        </div>
        <span className="text-xs text-slate-600">{episodeTitle}</span>
      </div>
      {/* ② 按钮行（M0：历史版本/重新规划灰化+Tooltip；生成视频按状态变化） */}
      <div className="mt-3 flex items-center gap-2 px-5">
        <button
          disabled
          title="M1 上线"
          className="cursor-not-allowed rounded-md border border-white/10 px-3 py-1 text-xs text-slate-600"
        >
          历史版本
        </button>
        <button
          disabled
          title="M1 上线，不满意请全项目删除重建"
          className="cursor-not-allowed rounded-md border border-white/10 px-3 py-1 text-xs text-slate-600"
        >
          🔄 重新规划
        </button>
        <button
          disabled
          title="Step3 开放"
          className="cursor-not-allowed rounded-md bg-slate-700/40 px-3 py-1 text-xs text-slate-500"
        >
          ● 生成视频
        </button>
        <span className="ml-auto text-xs text-slate-600">{beat.shot_group_count} 个镜头块</span>
      </div>
      {/* ③ 剧本原文 展开/收起（默认 2 行省略，展开最大 400px 滚动） */}
      <div className="mt-3 px-5 pb-4">
        <button
          onClick={() => setExpanded((v) => !v)}
          className="text-xs text-amber-300/80 transition hover:text-amber-200"
        >
          {expanded ? "剧本原文 [收起]" : "剧本原文 [展开]"}
        </button>
        {expanded && (
          <div className="mt-2 max-h-[400px] overflow-y-auto rounded-lg border border-white/5 bg-black/20 p-3 text-xs leading-relaxed text-slate-400">
            {beat.script_text || "（无剧本原文）"}
          </div>
        )}
      </div>
    </div>
  );
}

// 拆分中骨架
function SplittingSkeleton({ title }: { title: string }) {
  return (
    <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-6">
      <div className="flex items-center gap-3">
        <div className="h-3 w-3 animate-pulse rounded-full bg-amber-400" />
        <h2 className="text-sm font-semibold text-slate-300">正在拆解剧本：{title}</h2>
      </div>
      <p className="mt-2 text-xs text-slate-500">AI 正在分析剧情结构，生成叙事单元与分镜脚本…</p>
      <div className="mt-4 space-y-2">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-16 animate-pulse rounded-lg bg-white/5" style={{ animationDelay: `${i * 150}ms` }} />
        ))}
      </div>
    </div>
  );
}

export default function Step1Page() {
  const { projectId = "" } = useParams();
  const nav = useNavigate();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [scriptInput, setScriptInput] = useState("");
  const [splitting, setSplitting] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stepState = useStepState(projectId, proj?.status ?? null);
  const meta = proj ? ProjectStatusMeta[proj.status] : null;

  const showToast = useCallback((msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 3000);
  }, []);

  // 加载项目 + 剧集
  const loadProject = useCallback(async () => {
    try {
      const p = await projectsApi.detail(projectId);
      setProj(p);
      if (p.source_script_text) setScriptInput(p.source_script_text);
      const eps = await splitApi.episodes(projectId);
      setEpisodes(eps.items ?? []);
      if (eps.items.length > 0 && !activeEpisode) {
        setActiveEpisode(eps.items[0].episode_id);
      }
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载失败");
    }
  }, [projectId, activeEpisode, showToast]);

  // 加载单元列表
  const loadBeats = useCallback(async () => {
    if (!activeEpisode) return;
    try {
      const data = await splitApi.beats(activeEpisode);
      setBeats(data.items ?? []);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载叙事单元失败");
    }
  }, [activeEpisode, showToast]);

  // 拆分中轮询（SCRIPT_SPLITTING → 终态后停止）
  useEffect(() => {
    if (!proj) return;
    if (proj.status === "SCRIPT_SPLITTING") {
      pollRef.current = setInterval(async () => {
        try {
          const p = await projectsApi.detail(projectId);
          setProj(p);
          if (p.status !== "SCRIPT_SPLITTING") {
            if (pollRef.current) clearInterval(pollRef.current);
            showToast("剧本拆解完成");
            const eps = await splitApi.episodes(projectId);
            setEpisodes(eps.items ?? []);
            if (eps.items.length > 0) setActiveEpisode(eps.items[0].episode_id);
          }
        } catch {
          /* 轮询失败忽略 */
        }
      }, 4000);
      return () => {
        if (pollRef.current) clearInterval(pollRef.current);
      };
    }
  }, [proj?.status, projectId, showToast]);

  // 初始加载 + 单元列表联动
  useEffect(() => {
    void loadProject();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  useEffect(() => {
    void loadBeats();
  }, [loadBeats]);

  // 触发拆分
  const handleSplit = async () => {
    if (!scriptInput.trim()) {
      showToast("请输入剧本原文");
      return;
    }
    try {
      setSplitting(true);
      await splitApi.triggerSplit(projectId, false, scriptInput);
      showToast("拆分任务已提交，AI 处理中…");
      setSplitting(false);
      setProj({ ...(proj as ProjectDetail), status: "SCRIPT_SPLITTING" as ProjectStatusValue });
    } catch (e) {
      setSplitting(false);
      showToast(e instanceof Error ? e.message : "触发拆分失败");
    }
  };

  const splittingNow = proj?.status === "SCRIPT_SPLITTING";

  return (
    <div className="min-h-screen bg-[#0B0B1A] text-slate-100">
      {/* 顶栏：返回 + 标题 + StepBar */}
      <header className="sticky top-0 z-10 border-b border-white/5 bg-[#0B0B1A]/80 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between px-6">
          <div className="flex items-center gap-3">
            <button
              onClick={() => nav("/projects")}
              className="text-sm text-slate-400 transition hover:text-amber-300"
            >
              ← 返回
            </button>
            <h1 className="text-base font-semibold">{proj?.title ?? "加载中…"}</h1>
            {meta && (
              <span className="rounded-full border border-amber-400/30 bg-amber-400/10 px-2.5 py-0.5 text-xs text-amber-300">
                {meta.label}
              </span>
            )}
          </div>
          <StepBar
            current={stepState.currentStep}
            canClick={stepState.canClick}
            goStep={stepState.goStep}
          />
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-6 py-8">
        {/* 剧本输入（DRAFT 时可编辑/触发） */}
        {(proj?.status === "DRAFT" || proj?.status === "SCRIPT_SPLITTING") && (
          <div className="rounded-2xl border border-white/5 bg-white/[0.03] p-6">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-slate-300">剧本原文</h2>
              <span className="text-xs text-slate-600">{scriptInput.length} 字</span>
            </div>
            <textarea
              value={scriptInput}
              onChange={(e) => setScriptInput(e.target.value)}
              disabled={splittingNow}
              rows={6}
              placeholder="粘贴你的剧本，AI 将拆解为叙事单元（场景）与分镜脚本…"
              className="mt-3 w-full resize-none rounded-lg border border-white/10 bg-black/20 px-4 py-3 text-sm leading-relaxed text-slate-200 outline-none placeholder:text-slate-600 focus:border-amber-400/50 disabled:opacity-50"
            />
            <div className="mt-3 flex justify-end">
              <button
                onClick={() => void handleSplit()}
                disabled={splitting || splittingNow}
                className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-6 py-2 text-sm font-semibold text-black shadow-lg shadow-amber-500/20 transition hover:from-amber-300 hover:to-amber-400 disabled:opacity-50"
              >
                {splittingNow ? "拆解中…" : splitting ? "提交中…" : "开始 AI 拆解"}
              </button>
            </div>
          </div>
        )}

        {/* 拆分中骨架 */}
        {splittingNow && <div className="mt-6"><SplittingSkeleton title={proj?.title ?? ""} /></div>}

        {/* 拆分结果：剧集切换 + 叙事单元 5 件套 */}
        {!splittingNow && beats.length > 0 && (
          <div className="mt-6 space-y-4">
            {/* 剧集切换（多集） */}
            {episodes.length > 1 && (
              <div className="flex items-center gap-2">
                <span className="text-xs text-slate-500">剧集：</span>
                {episodes.map((ep) => (
                  <button
                    key={ep.episode_id}
                    onClick={() => setActiveEpisode(ep.episode_id)}
                    className={`rounded-full px-3.5 py-1 text-xs transition ${
                      activeEpisode === ep.episode_id
                        ? "bg-amber-400/20 text-amber-300"
                        : "text-slate-400 hover:bg-white/5"
                    }`}
                  >
                    第 {ep.episode_number} 集
                  </button>
                ))}
              </div>
            )}
            {beats.map((b) => (
              <BeatCard
                key={b.beat_id}
                beat={b}
                episodeTitle={episodes.find((e) => e.episode_id === activeEpisode)?.title ?? ""}
              />
            ))}
          </div>
        )}

        {/* 完成态空提示 */}
        {!splittingNow && proj && proj.status !== "DRAFT" && proj.status !== "SCRIPT_SPLITTING" && beats.length === 0 && (
          <div className="mt-6 rounded-2xl border border-dashed border-white/10 p-10 text-center text-sm text-slate-500">
            该剧集暂无叙事单元数据
          </div>
        )}
      </main>

      {/* Toast */}
      {toast && (
        <div className="fixed bottom-6 left-1/2 z-30 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200 shadow-xl">
          {toast}
        </div>
      )}
    </div>
  );
}