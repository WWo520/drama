// S1-F06 Step4 成片预览页 —— 按 luxreal-clone 设计稿 workbench-preview.html 融合（批次 3）
// 布局：WorkbenchShell + 播放器（镜头视频顺序连播）+ 叙事单元时间线 + 合成/导出 + 导出历史
// 接口：GET /episodes/:id/beats、GET /beats/:id/shots、POST /projects/:id/export、GET /exports
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ChevronDown, Clock, Download, FileText, Film, Loader2, Maximize, Pause, PictureInPicture2, Play,
  RotateCcw, RotateCw, Volume2, Wand2, X,
} from "lucide-react";

import {
  ApiError,
  errorMessage,
  exportApi,
  projectsApi,
  splitApi,
  videoApi,
  type BeatSummary,
  type EpisodeLite,
  type ExportItem,
  type ProjectDetail,
  type ShotVideo,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import "./step4.css";

// 错误文案：ApiError 走错误码映射（05 C 章），避免弹出后端原始报文
function errMsg(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return errorMessage(e.code, e.message);
  return e instanceof Error ? e.message : fallback;
}

// 可导出的项目状态（对齐后端 export_service 守卫）
const EXPORTABLE = ["ALL_VIDEOS_READY", "PARTIAL_VIDEOS", "COMPLETED", "EXPORTING"];

// 清晰度选项（05 3.4.1：P480/P720/P1080；4K 未支持）
const RESOLUTIONS = [
  { key: "P480", label: "480P" },
  { key: "P720", label: "720P" },
  { key: "P1080", label: "1080P" },
];
const SPEEDS = ["0.5", "0.75", "1", "1.25", "1.5", "2"];

function fmtTime(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

function fmtSize(bytes: number): string {
  if (bytes <= 0) return "-";
  const mb = bytes / 1024 / 1024;
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)}GB` : `${mb.toFixed(0)}MB`;
}

// 连播条目：跨 beat 展平的已完成镜头视频
interface PlayItem {
  beatId: string;
  beatIdx: number;
  sortOrder: number;
  url: string;
  durationSec: number;
}

// ===== 合成视频弹窗（对应后端 3.4.1 触发导出：整集合成 MP4_H264） =====
function ComposeModal({
  totalSec,
  submitting,
  onClose,
  onSubmit,
}: {
  totalSec: number;
  submitting: boolean;
  onClose: () => void;
  onSubmit: (resolution: string) => void;
}) {
  const [mode, setMode] = useState<"full" | "range">("full");
  const [resolution, setResolution] = useState("P1080");
  const [fps, setFps] = useState("30");
  const [subtitles, setSubtitles] = useState(true);
  const [intro, setIntro] = useState(false);
  const [autoDownload, setAutoDownload] = useState(true);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="pv-modal-overlay" onClick={onClose} />
      <div className="relative bg-[#1E1E24] rounded-2xl border border-[#2A2A35] w-[420px] p-6 shadow-2xl">
        <h3 className="text-white text-lg font-medium mb-2">合成视频</h3>
        <p className="text-[#6B7088] text-sm mb-5">将当前集所有叙事单元的视频片段合成为完整剧集</p>

        <div className="space-y-4 mb-5">
          {/* 合成方式 */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">合成方式</span>
            <div className="flex items-center bg-[#252535] rounded-lg p-0.5">
              <button
                className={`px-3 py-1 text-xs rounded-md ${mode === "full" ? "text-white bg-[#1A1A2E] font-medium" : "text-white/60 hover:text-white/80 transition-colors"}`}
                onClick={() => setMode("full")}
              >
                整集合成
              </button>
              <button
                className={`px-3 py-1 text-xs rounded-md ${mode === "range" ? "text-white bg-[#1A1A2E] font-medium" : "text-white/60 hover:text-white/80 transition-colors"}`}
                onClick={() => {
                  setMode("range");
                  toast.info("选片段合成将在后续版本开放，将按整集合成处理");
                }}
              >
                选片段合成
              </button>
            </div>
          </div>
          {/* 导出格式（后端固定 MP4_H264） */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">导出格式</span>
            <span className="text-sm text-white/80 bg-[#252535] rounded-lg px-3 py-1.5">MP4（推荐）</span>
          </div>
          {/* 分辨率 */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">分辨率</span>
            <div className="flex items-center bg-[#252535] rounded-lg p-0.5">
              {RESOLUTIONS.map((r) => (
                <button
                  key={r.key}
                  className={`px-3 py-1 text-xs rounded-md ${resolution === r.key ? "text-white bg-[#1A1A2E] font-medium" : "text-white/60 hover:text-white/80 transition-colors"}`}
                  onClick={() => setResolution(r.key)}
                >
                  {r.label}
                </button>
              ))}
              <button
                className="px-3 py-1 text-xs rounded-md text-white/30 transition-colors"
                onClick={() => toast.info("4K 导出将在后续版本开放")}
              >
                4K
              </button>
            </div>
          </div>
          {/* 帧率（纯展示，后端不区分） */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">帧率</span>
            <div className="flex items-center bg-[#252535] rounded-lg p-0.5">
              {["24", "30", "60"].map((f) => (
                <button
                  key={f}
                  className={`px-3 py-1 text-xs rounded-md ${fps === f ? "text-white bg-[#1A1A2E] font-medium" : "text-white/60 hover:text-white/80 transition-colors"}`}
                  onClick={() => setFps(f)}
                >
                  {f}fps
                </button>
              ))}
            </div>
          </div>
          {/* 开关组 */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">包含字幕</span>
            <div className={`pv-toggle ${subtitles ? "on" : ""}`} onClick={() => setSubtitles(!subtitles)} />
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">包含片头片尾</span>
            <div className={`pv-toggle ${intro ? "on" : ""}`} onClick={() => { setIntro(!intro); toast.info("片头片尾模板将在后续版本开放"); }} />
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm text-white/80">合成后自动下载</span>
            <div className={`pv-toggle ${autoDownload ? "on" : ""}`} onClick={() => setAutoDownload(!autoDownload)} />
          </div>
        </div>

        {/* 费用预估（仅展示，实际计费以后端为准） */}
        <div className="flex items-center justify-center mb-5">
          <span className="text-sm text-[#4C7FF8]">预计消耗 {Math.max(1, Math.ceil(totalSec))} 积分</span>
        </div>

        <div className="flex items-center gap-3">
          <button className="flex-1 h-10 rounded-xl text-sm text-white/70 bg-[#252535] hover:bg-[#2A2A3A] transition-colors" onClick={onClose}>
            取消
          </button>
          <button
            className="flex-1 h-10 rounded-xl text-sm text-white font-medium transition-all hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
            style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
            disabled={submitting}
            onClick={() => onSubmit(resolution)}
          >
            {submitting ? <Loader2 size={15} className="animate-spin" /> : <Wand2 size={15} />}
            {submitting ? "提交中…" : "开始合成"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ===== 导出工程文件弹窗（LXP 工程导出：后端暂未支持，保真占位） =====
function ExportFileModal({ onClose }: { onClose: () => void }) {
  const [opts, setOpts] = useState({ script: true, assets: true, storyboard: true, agent: false });

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="pv-modal-overlay" onClick={onClose} />
      <div className="relative bg-[#1E1E24] rounded-2xl border border-[#2A2A35] w-[420px] p-6 shadow-2xl">
        <h3 className="text-white text-lg font-medium mb-2">导出工程文件</h3>
        <p className="text-[#6B7088] text-sm mb-5">导出包含所有剧本、资产、分镜配置的工程文件，可在其他项目中导入复用</p>

        <div className="space-y-3 mb-5">
          {([
            ["script", "包含剧本内容"],
            ["assets", "包含资产参考图"],
            ["storyboard", "包含分镜设置"],
            ["agent", "包含Agent技能配置"],
          ] as const).map(([k, label]) => (
            <div key={k} className="flex items-center justify-between">
              <span className="text-sm text-white/80">{label}</span>
              <div className={`pv-toggle ${opts[k] ? "on" : ""}`} onClick={() => setOpts({ ...opts, [k]: !opts[k] })} />
            </div>
          ))}
        </div>

        <div className="flex items-center justify-between mb-6">
          <span className="text-sm text-white/80">导出格式</span>
          <span className="text-sm text-white/80 bg-[#252535] rounded-lg px-3 py-1.5">LXP（推荐）</span>
        </div>

        <div className="flex items-center gap-3">
          <button className="flex-1 h-10 rounded-xl text-sm text-white/70 bg-[#252535] hover:bg-[#2A2A3A] transition-colors" onClick={onClose}>
            取消
          </button>
          <button
            className="flex-1 h-10 rounded-xl text-sm text-white font-medium transition-all hover:opacity-90"
            style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
            onClick={() => toast.info("工程文件（LXP）导出将在后续版本开放")}
          >
            导出
          </button>
        </div>
      </div>
    </div>
  );
}

// ===== Step4 主页面 =====
export default function Step4Page() {
  const { projectId = "" } = useParams();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [loadError, setLoadError] = useState(false);
  const [shotsMap, setShotsMap] = useState<Record<string, ShotVideo[]>>({});
  const [exports, setExports] = useState<ExportItem[]>([]);
  const [composeOpen, setComposeOpen] = useState(false);
  const [exportFileOpen, setExportFileOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  // 播放器状态
  const videoRef = useRef<HTMLVideoElement>(null);
  const playerBoxRef = useRef<HTMLDivElement>(null);
  const pendingSeekRef = useRef<number | null>(null);
  const [curIdx, setCurIdx] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [curTime, setCurTime] = useState(0);
  const [volume, setVolume] = useState(0.8);
  const [speed, setSpeed] = useState("1");
  const [speedOpen, setSpeedOpen] = useState(false);
  const [qualityOpen, setQualityOpen] = useState(false);

  const exportPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const stepState = useStepState(projectId, proj?.status ?? null);

  const activeEp = episodes.find((e) => e.episode_id === activeEpisode) ?? null;

  // 连播列表：跨 beat 展平已完成镜头（按 beat / 镜头顺序）
  const playlist = useMemo<PlayItem[]>(() => {
    const list: PlayItem[] = [];
    beats.forEach((b, beatIdx) => {
      (shotsMap[b.beat_id] ?? [])
        .filter((s) => s.video_generation_status === "SUCCESS" && s.generated_video_url)
        .forEach((s) => list.push({ beatId: b.beat_id, beatIdx, sortOrder: s.sort_order, url: s.generated_video_url, durationSec: s.duration_sec }));
    });
    return list;
  }, [beats, shotsMap]);

  const totalDur = useMemo(() => playlist.reduce((acc, p) => acc + (p.durationSec || 0), 0), [playlist]);
  // 各片段在全局时间轴上的起点
  const prefix = useMemo(() => {
    const arr: number[] = [];
    let acc = 0;
    playlist.forEach((p) => {
      arr.push(acc);
      acc += p.durationSec || 0;
    });
    return arr;
  }, [playlist]);
  const globalTime = (prefix[curIdx] ?? 0) + curTime;
  const curItem = playlist[curIdx] ?? null;

  // 加载项目 + 剧集 + beats
  const loadStructure = useCallback(async () => {
    try {
      const p = await projectsApi.detail(projectId);
      setLoadError(false);
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
      setLoadError(true);
      toast.error(errMsg(e, "加载失败"));
    }
  }, [projectId, activeEpisode]);

  // 加载各 beat 的镜头视频
  const loadShots = useCallback(async (beatList: BeatSummary[]) => {
    const sMap: Record<string, ShotVideo[]> = {};
    await Promise.all(
      beatList.map(async (b) => {
        try {
          const d = await videoApi.beatShots(b.beat_id);
          sMap[b.beat_id] = d.items ?? [];
        } catch {
          sMap[b.beat_id] = [];
        }
      }),
    );
    setShotsMap(sMap);
  }, []);

  useEffect(() => {
    void loadStructure();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, activeEpisode]);

  useEffect(() => {
    if (beats.length > 0) void loadShots(beats);
  }, [beats, loadShots]);

  // 导出历史加载 + PENDING 轮询
  const loadExports = useCallback(async () => {
    try {
      const d = await exportApi.list(projectId);
      setExports(d.items ?? []);
      const pending = (d.items ?? []).some((e) => e.status === "PENDING");
      if (pending && !exportPollRef.current) {
        exportPollRef.current = setInterval(async () => {
          try {
            const dd = await exportApi.list(projectId);
            setExports(dd.items ?? []);
            if (!(dd.items ?? []).some((e) => e.status === "PENDING") && exportPollRef.current) {
              clearInterval(exportPollRef.current);
              exportPollRef.current = null;
              toast.success("合成导出已完成");
              void projectsApi.detail(projectId).then(setProj).catch(() => undefined);
            }
          } catch {
            /* 忽略轮询抖动 */
          }
        }, 3000);
      } else if (!pending && exportPollRef.current) {
        clearInterval(exportPollRef.current);
        exportPollRef.current = null;
      }
    } catch (e) {
      toast.error(errMsg(e, "加载导出历史失败"));
    }
  }, [projectId]);

  useEffect(() => {
    void loadExports();
    return () => {
      if (exportPollRef.current) {
        clearInterval(exportPollRef.current);
        exportPollRef.current = null;
      }
    };
  }, [loadExports]);

  // ESC 关闭弹窗与下拉
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setComposeOpen(false);
        setExportFileOpen(false);
        setSpeedOpen(false);
        setQualityOpen(false);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  // 点击空白关闭下拉
  useEffect(() => {
    const onClick = () => {
      setSpeedOpen(false);
      setQualityOpen(false);
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, []);

  // ===== 播放器控制 =====
  const togglePlay = () => {
    const v = videoRef.current;
    if (!v) return;
    if (playing) {
      v.pause();
      setPlaying(false);
    } else {
      // 全片播完后重播：末段截断后 v.ended 恒为 false，改用全局时间判底
      if (totalDur > 0 && globalTime >= totalDur - 0.05) {
        setPlaying(true);
        if (curIdx === 0) {
          // 单片段项目：同元素内重置并续播
          v.currentTime = 0;
          void v.play().catch(() => undefined);
        } else {
          jumpTo(0, 0);
        }
        return;
      }
      void v.play().catch(() => toast.error("视频播放失败，请检查视频资源"));
      setPlaying(true);
    }
  };

  const jumpTo = (idx: number, offset = 0) => {
    if (idx < 0 || idx >= playlist.length) return;
    pendingSeekRef.current = offset;
    setCurTime(0);
    if (idx === curIdx) {
      const v = videoRef.current;
      if (v) v.currentTime = offset;
      pendingSeekRef.current = null;
    } else {
      setCurIdx(idx);
    }
  };

  const onEnded = () => {
    if (curIdx < playlist.length - 1) {
      jumpTo(curIdx + 1, 0);
    } else {
      // 末段截断：mock 素材实际时长可能长于 duration_sec，强制暂停避免时间溢出
      videoRef.current?.pause();
      setPlaying(false);
    }
  };

  // 段内到达片段时长即切段（mock 素材实际时长与 duration_sec 不一致时兼作切段兼容）
  const onTimeUpdate = (t: number) => {
    setCurTime(t);
    const seg = playlist[curIdx];
    if (seg && seg.durationSec > 0 && t >= seg.durationSec) {
      onEnded();
    }
  };

  const seekGlobal = (t: number) => {
    const clamped = Math.min(Math.max(0, t), Math.max(0, totalDur - 0.1));
    let idx = playlist.length - 1;
    for (let i = 0; i < playlist.length; i++) {
      if (clamped < (prefix[i] ?? 0) + (playlist[i].durationSec || 0)) {
        idx = i;
        break;
      }
    }
    jumpTo(idx, clamped - (prefix[idx] ?? 0));
  };

  const skip = (delta: number) => seekGlobal(globalTime + delta);

  const changeSpeed = (s: string) => {
    setSpeed(s);
    if (videoRef.current) videoRef.current.playbackRate = Number(s);
    setSpeedOpen(false);
  };

  const enterFullscreen = () => {
    void playerBoxRef.current?.requestFullscreen().catch(() => toast.info("当前环境不支持全屏"));
  };

  const enterPip = () => {
    const v = videoRef.current as (HTMLVideoElement & { requestPictureInPicture?: () => Promise<unknown> }) | null;
    if (v?.requestPictureInPicture) {
      void v.requestPictureInPicture().catch(() => toast.info("当前环境不支持画中画"));
    } else {
      toast.info("当前环境不支持画中画");
    }
  };

  // 切集/切片段后应用待定位偏移；同 URL 时元素不重建，由本 effect 兑底定位与续播
  const onLoadedMetadata = () => {
    const v = videoRef.current;
    if (!v) return;
    v.volume = volume;
    v.playbackRate = Number(speed);
    if (pendingSeekRef.current !== null) {
      v.currentTime = pendingSeekRef.current;
      pendingSeekRef.current = null;
    }
    if (playing) void v.play().catch(() => undefined);
  };

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    v.volume = volume;
    v.playbackRate = Number(speed);
    if (pendingSeekRef.current !== null) {
      v.currentTime = pendingSeekRef.current;
      pendingSeekRef.current = null;
    }
    if (playing) void v.play().catch(() => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [curIdx]);

  // 合成导出（3.4.1：MP4_H264 + 清晰度 + 合规水印）
  const canExport = proj != null && EXPORTABLE.includes(proj.status);
  const handleCompose = async (resolution: string) => {
    try {
      setSubmitting(true);
      await exportApi.trigger(projectId, resolution, true);
      toast.success(`合成任务已提交（${RESOLUTIONS.find((r) => r.key === resolution)?.label ?? resolution}）`);
      setComposeOpen(false);
      void loadExports();
    } catch (e) {
      toast.error(errMsg(e, "触发合成失败"));
    } finally {
      setSubmitting(false);
    }
  };

  // 时间线：跳转至某叙事单元的首个镜头
  const jumpToBeat = (beatIdx: number) => {
    const first = playlist.findIndex((p) => p.beatIdx === beatIdx);
    if (first >= 0) {
      jumpTo(first, 0);
    } else {
      toast.info("该叙事单元暂无已完成的视频");
    }
  };

  const progressPct = totalDur > 0 ? Math.min(100, (globalTime / totalDur) * 100) : 0;

  // 项目加载失败（404/无权限/后端异常）整页降级，避免无限“加载中…”
  if (loadError) return <ProjectLoadError />;

  return (
    <WorkbenchShell
      projectTitle={proj?.title ?? ""}
      currentStep={stepState.currentStep}
      canClick={stepState.canClick}
      goStep={stepState.goStep}
      episodes={episodes}
      activeEpisodeId={activeEpisode}
      onSelectEpisode={setActiveEpisode}
      activeEpisodeBeats={beats}
    >
      {/* Content Scroll Area */}
      <div className="flex-1 overflow-y-auto flex flex-col px-6 pt-10 pb-6 scrollbar-thin" style={{ justifyContent: "flex-start" }}>
        {/* ===== VIDEO PLAYER ===== */}
        <div className="flex-shrink-0 relative w-full flex justify-center mb-8" style={{ height: "68vh" }}>
          <div
            ref={playerBoxRef}
            className="relative h-full rounded-2xl overflow-hidden bg-black shadow-2xl shadow-black/60"
            style={{ width: "calc(68vh * 9 / 16)", maxWidth: "400px" }}
          >
            {curItem ? (
              <>
                <video
                  key={`${curItem.beatId}-${curItem.sortOrder}-${curIdx}`}
                  ref={videoRef}
                  src={curItem.url}
                  className="absolute inset-0 w-full h-full object-contain"
                  onTimeUpdate={(e) => onTimeUpdate(e.currentTarget.currentTime)}
                  onEnded={onEnded}
                  onLoadedMetadata={onLoadedMetadata}
                  onClick={togglePlay}
                  preload="metadata"
                />
                {/* 中央大播放按钮（暂停态） */}
                {!playing && (
                  <button
                    className="play-btn absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-20 h-20 rounded-full bg-white/10 backdrop-blur-md border border-white/25 flex items-center justify-center hover:bg-white/20 transition-all z-10"
                    onClick={togglePlay}
                  >
                    <Play size={36} className="text-white ml-1 fill-white" />
                  </button>
                )}
              </>
            ) : (
              /* 无成片：渐变占位 */
              <div className="video-bg absolute inset-0 flex flex-col items-center justify-center gap-3">
                <Film size={36} className="text-white/20" />
                <span className="text-xs text-white/40">{canExport ? "暂无已完成的镜头视频" : "完成 Step3 视频生成后可预览成片"}</span>
              </div>
            )}

            {/* 左上角剧集徽章 */}
            {activeEp && (
              <div className="absolute top-4 left-4 bg-black/60 backdrop-blur-sm text-white text-xs px-3 py-1.5 rounded-full flex items-center gap-1.5 z-10">
                <Film size={13} />
                第{activeEp.sort_order}集 {activeEp.title}
              </div>
            )}

            {/* 右上角清晰度下拉 */}
            <div className="absolute top-4 right-4 z-10">
              <button
                className="bg-black/60 backdrop-blur-sm text-white text-xs px-3 py-1.5 rounded-full flex items-center gap-1 font-medium hover:bg-black/70 transition-colors"
                onClick={(e) => {
                  e.stopPropagation();
                  setQualityOpen(!qualityOpen);
                  setSpeedOpen(false);
                }}
              >
                <span>1080P</span>
                <ChevronDown size={12} />
              </button>
              {qualityOpen && (
                <div className="absolute top-full right-0 mt-1.5 bg-[#1E1E24] border border-[#2A2A35] rounded-lg py-1 w-24 z-20 shadow-xl">
                  {["480P", "720P", "1080P", "4K"].map((q) => (
                    <button
                      key={q}
                      className={`w-full text-left text-sm py-2 px-3 transition-colors hover:bg-white/5 ${q === "1080P" ? "text-[#4C7FF8] font-medium" : "text-white/70"}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        setQualityOpen(false);
                        if (q !== "1080P") toast.info("预览暂仅提供 1080P 源，导出时可选择其他清晰度");
                      }}
                    >
                      {q}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* 右下角 LuxReal 水印（对齐设计稿 workbench-preview.html） */}
            <div className="absolute bottom-16 right-4 text-white/30 text-sm flex items-center gap-1 select-none pointer-events-none z-10">
              <span className="font-semibold tracking-wide text-xs">LuxReal</span>
            </div>

            {/* 底部控制栏 */}
            {curItem && (
              <div
                className="absolute bottom-0 left-0 right-0 px-4 pb-4 pt-10 z-10"
                style={{ background: "linear-gradient(to top, rgba(0,0,0,0.85) 0%, rgba(0,0,0,0.5) 50%, transparent 100%)" }}
              >
                {/* 进度条（全局） */}
                <div
                  className="relative h-1 bg-white/20 rounded-full mb-3 cursor-pointer group"
                  onClick={(e) => {
                    const rect = e.currentTarget.getBoundingClientRect();
                    seekGlobal(((e.clientX - rect.left) / rect.width) * totalDur);
                  }}
                >
                  <div className="h-full rounded-full bg-white relative" style={{ width: `${progressPct}%` }}>
                    <div className="absolute right-0 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-white shadow-md -mr-1.5" />
                  </div>
                </div>

                <div className="flex items-center justify-between">
                  {/* 左：播放/回退/前进/音量 */}
                  <div className="flex items-center gap-3">
                    <button className="text-white hover:text-white/80 transition flex items-center justify-center" onClick={togglePlay}>
                      {playing ? <Pause size={20} className="fill-white" /> : <Play size={20} className="fill-white" />}
                    </button>
                    <button className="text-white/80 hover:text-white transition flex items-center justify-center" onClick={() => skip(-5)}>
                      <RotateCcw size={16} />
                    </button>
                    <button className="text-white/80 hover:text-white transition flex items-center justify-center" onClick={() => skip(5)}>
                      <RotateCw size={16} />
                    </button>
                    <div className="flex items-center gap-1.5">
                      <button
                        className="text-white/80 hover:text-white transition"
                        onClick={() => {
                          const nv = volume > 0 ? 0 : 0.8;
                          setVolume(nv);
                          if (videoRef.current) videoRef.current.volume = nv;
                        }}
                      >
                        <Volume2 size={16} />
                      </button>
                      <div
                        className="w-16 h-1 bg-white/20 rounded-full cursor-pointer"
                        onClick={(e) => {
                          const rect = e.currentTarget.getBoundingClientRect();
                          const nv = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
                          setVolume(nv);
                          if (videoRef.current) videoRef.current.volume = nv;
                        }}
                      >
                        <div className="h-full bg-white rounded-full" style={{ width: `${volume * 100}%` }} />
                      </div>
                    </div>
                  </div>
                  {/* 中：时间 */}
                  <div className="absolute left-1/2 -translate-x-1/2">
                    <span className="text-white/90 text-xs font-mono">
                      {fmtTime(globalTime)} / {fmtTime(totalDur)}
                    </span>
                  </div>
                  {/* 右：倍速/画中画/全屏 */}
                  <div className="flex items-center gap-3 relative">
                    <div className="relative">
                      <button
                        className="text-white/80 text-xs hover:text-white transition flex items-center gap-0.5"
                        onClick={(e) => {
                          e.stopPropagation();
                          setSpeedOpen(!speedOpen);
                          setQualityOpen(false);
                        }}
                      >
                        <span>倍速</span>
                        <span className="font-medium">{speed}x</span>
                      </button>
                      {speedOpen && (
                        <div className="absolute bottom-full right-0 mb-2 bg-[#1E1E24] border border-[#2A2A35] rounded-lg py-1 w-20 z-20 shadow-xl">
                          {SPEEDS.map((s) => (
                            <button
                              key={s}
                              className={`w-full text-left text-sm py-2 px-3 transition-colors hover:bg-white/5 ${s === speed ? "text-[#4C7FF8] font-medium" : "text-white/70"}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                changeSpeed(s);
                              }}
                            >
                              {s}x
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    <button className="text-white/80 hover:text-white transition" onClick={enterPip}>
                      <PictureInPicture2 size={16} />
                    </button>
                    <button className="text-white hover:text-white/80 transition" onClick={enterFullscreen}>
                      <Maximize size={18} />
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* ===== NARRATIVE UNIT TIMELINE ===== */}
        {beats.length > 0 && (
          <div className="timeline-container">
            <div className="timeline-track">
              {beats.map((b, i) => (
                <div key={b.beat_id} className="flex items-center">
                  {i > 0 && <div className="timeline-connector" />}
                  <div className="timeline-unit">
                    <div
                      className={`timeline-unit-card ${curItem?.beatId === b.beat_id ? "active" : ""}`}
                      onClick={() => jumpToBeat(i)}
                    >
                      <div className="timeline-unit-number">叙事单元{b.sort_order}</div>
                      <div className="timeline-unit-title">{b.title}</div>
                      <div className="timeline-unit-duration">
                        <Clock size={12} />
                        <span>{fmtTime(b.actual_duration_sec > 0 ? b.actual_duration_sec : b.preset_duration_sec)}</span>
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* ===== INFO + TWO ACTION BUTTONS ===== */}
        <div className="w-full max-w-[420px] mx-auto flex-shrink-0 mt-6 pb-4">
          <div className="text-center mb-5">
            <h2 className="text-white font-medium text-base mb-1">
              {proj?.title ?? "加载中…"} · 第{activeEp?.sort_order ?? 1}集
            </h2>
            <p className="text-xs text-[#6B7088]">
              总时长 {fmtTime(totalDur)} · 1080P · 9:16 竖屏
            </p>
          </div>
          <div className="flex gap-3">
            <button
              className="flex-1 h-11 rounded-xl text-sm font-medium flex items-center justify-center gap-2 transition-all hover:bg-[#252535]"
              style={{ background: "#1A1A2E", border: "1px solid #2A2A3E", color: "white" }}
              onClick={() => setExportFileOpen(true)}
            >
              <FileText size={16} />
              导出工程视频
            </button>
            <button
              className="flex-1 h-11 rounded-xl text-sm font-medium flex items-center justify-center gap-2 transition-all hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
              style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)", color: "white" }}
              disabled={!canExport || playlist.length === 0}
              onClick={() => {
                if (!canExport) {
                  toast.error("视频尚未全部生成，请回到 Step3 完成生成");
                  return;
                }
                setComposeOpen(true);
              }}
            >
              <Wand2 size={16} />
              合成视频
            </button>
          </div>
        </div>

        {/* ===== EXPORT HISTORY（后端实体：导出记录与下载） ===== */}
        <div className="w-full max-w-[420px] mx-auto flex-shrink-0 pb-6">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-medium text-white/70">导出记录</span>
            {exports.some((e) => e.status === "PENDING") && (
              <span className="text-[11px] text-[#F59E0B] flex items-center gap-1">
                <Loader2 size={12} className="animate-spin" />
                合成导出中…
              </span>
            )}
          </div>
          {exports.length === 0 ? (
            <div className="text-center text-xs text-[#4A4A5A] py-4 border border-dashed border-[#252530] rounded-xl">
              暂无导出记录，点击「合成视频」生成成片
            </div>
          ) : (
            exports.map((e) => (
              <div key={e.export_id} className="export-history-row">
                <div className="flex items-center gap-3 min-w-0">
                  <span className={`export-badge ${e.status === "SUCCESS" ? "success" : e.status === "FAILED" ? "failed" : "pending"}`}>
                    {e.status === "SUCCESS" ? "已完成" : e.status === "FAILED" ? "失败" : "导出中"}
                  </span>
                  <span className="text-xs text-[#8B8B9A]">{RESOLUTIONS.find((r) => r.key === e.resolution)?.label ?? e.resolution}</span>
                  <span className="text-xs text-[#4A4A5A]">{fmtSize(e.file_size)}</span>
                  <span className="text-xs text-[#4A4A5A] truncate">{new Date(e.created_at).toLocaleString()}</span>
                </div>
                {e.status === "SUCCESS" && e.output_url && (
                  <a className="btn-export-download" href={e.output_url} target="_blank" rel="noreferrer">
                    <Download size={12} />
                    下载
                  </a>
                )}
                {e.status === "FAILED" && (
                  <button className="text-[11px] text-[#EF4444] hover:underline" onClick={() => toast.error("导出失败，请重新发起合成")}>
                    <X size={12} />
                  </button>
                )}
              </div>
            ))
          )}
        </div>
      </div>

      {/* ===== MODALS ===== */}
      {exportFileOpen && <ExportFileModal onClose={() => setExportFileOpen(false)} />}
      {composeOpen && (
        <ComposeModal
          totalSec={totalDur}
          submitting={submitting}
          onClose={() => setComposeOpen(false)}
          onSubmit={(r) => void handleCompose(r)}
        />
      )}
    </WorkbenchShell>
  );
}
