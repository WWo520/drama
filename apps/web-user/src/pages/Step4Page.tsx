// S1-F06 Step4 成片预览页 —— 按 luxreal-clone 设计稿 workbench-preview.html 融合（批次 3）
// 布局：WorkbenchShell + 播放器（镜头视频顺序连播）+ 叙事单元时间线 + 合成/导出 + 导出历史
// 接口：GET /episodes/:id/beats、GET /beats/:id/shots、POST /projects/:id/export、GET /exports
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ChevronDown, Clock, Download, FileText, Film, Loader2, Maximize, Pause, PictureInPicture2, Play,
  RefreshCw, RotateCcw, RotateCw, Volume2, Wand2, X,
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
  type ExportPreflight,
  type ProjectDetail,
  type ShotVideo,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import { matchTaskTypes, useWsDrivenPoll } from "../hooks/useWsDrivenPoll";
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
  preflight,
  preflightLoading,
  onClose,
  onSubmit,
}: {
  totalSec: number;
  submitting: boolean;
  preflight: ExportPreflight | null;
  preflightLoading: boolean;
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

        {/* 合成前预检：片段统计（跳过的重复片段透明化） */}
        <div className="rounded-xl p-3 mb-5" style={{ background: "#1A1A2E", border: "1px solid #2A2A3E" }}>
          {preflightLoading ? (
            <div className="flex items-center gap-2 text-xs text-[#6B7088]">
              <Loader2 size={12} className="animate-spin" />
              正在检查可合成片段…
            </div>
          ) : preflight && preflight.total > 0 ? (
            <div className="text-xs leading-relaxed">
              <span className="text-white/85 font-medium">
                将合成 {preflight.total} 个片段 · 约 {fmtTime(preflight.total_duration_sec)}
              </span>
              <span className="text-[#6B7088]">（{preflight.beats} 个叙事单元）</span>
              {preflight.duplicates > 0 && (
                <div className="text-[#F5B23C] mt-1">
                  {preflight.duplicates} 个重复片段将被自动跳过
                </div>
              )}
            </div>
          ) : (
            <div className="text-xs text-[#EF4444]">未找到可合成的视频片段，请先完成 Step3 视频生成</div>
          )}
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
  // 合成前预检（弹窗打开时拉取片段统计）
  const [preflight, setPreflight] = useState<ExportPreflight | null>(null);
  const [preflightLoading, setPreflightLoading] = useState(false);

  // 播放器状态
  const videoRef = useRef<HTMLVideoElement>(null);
  const playerBoxRef = useRef<HTMLDivElement>(null);
  const pendingSeekRef = useRef<number | null>(null);
  const [curIdx, setCurIdx] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [curTime, setCurTime] = useState(0);
  // 各片段真实时长（key=playlist 索引；元数据加载后校正 DB duration_sec 的漂移）
  const [realDurs, setRealDurs] = useState<Record<number, number>>({});
  const [volume, setVolume] = useState(0.8);
  const [speed, setSpeed] = useState("1");
  const [speedOpen, setSpeedOpen] = useState(false);
  const [qualityOpen, setQualityOpen] = useState(false); // 清晰度菜单（预览档位切换）
  const [controlsVisible, setControlsVisible] = useState(true);

  const stepState = useStepState(projectId, proj?.status ?? null);

  const activeEp = episodes.find((e) => e.episode_id === activeEpisode) ?? null;

  // 连播列表：跨 beat 展平已完成镜头；同 URL 去重（重复镜头块/重规划旧批并存时避免同画面播两次）
  const playlist = useMemo<PlayItem[]>(() => {
    const list: PlayItem[] = [];
    const seenUrls = new Set<string>();
    beats.forEach((b, beatIdx) => {
      (shotsMap[b.beat_id] ?? [])
        .filter((s) => s.video_generation_status === "SUCCESS" && s.generated_video_url)
        .forEach((s) => {
          if (seenUrls.has(s.generated_video_url)) return;
          seenUrls.add(s.generated_video_url);
          list.push({ beatId: b.beat_id, beatIdx, sortOrder: s.sort_order, url: s.generated_video_url, durationSec: s.duration_sec });
        });
    });
    return list;
  }, [beats, shotsMap]);

  const totalDur = useMemo(
    () => playlist.reduce((acc, p, i) => acc + (realDurs[i] || p.durationSec || 0), 0),
    [playlist, realDurs],
  );
  // 各片段在全局时间轴上的起点（优先真实时长）
  const prefix = useMemo(() => {
    const arr: number[] = [];
    let acc = 0;
    playlist.forEach((p, i) => {
      arr.push(acc);
      acc += realDurs[i] || p.durationSec || 0;
    });
    return arr;
  }, [playlist, realDurs]);
  const globalTime = (prefix[curIdx] ?? 0) + curTime;
  const curItem = playlist[curIdx] ?? null;

  // 进度条分段节点：叙事单元边界（亮）+ 单元内镜头块边界（暗），位置=累计秒/总秒（对齐 LuxReal Timeline-section）
  const segTicks = useMemo(() => {
    const ticks: { pct: number; unitBoundary: boolean; label: string }[] = [];
    let lastBeatIdx = -1;
    playlist.forEach((p, i) => {
      if (i === 0) {
        lastBeatIdx = p.beatIdx;
        return;
      }
      const isUnitBoundary = p.beatIdx !== lastBeatIdx;
      lastBeatIdx = p.beatIdx;
      ticks.push({
        pct: totalDur > 0 ? (prefix[i] / totalDur) * 100 : 0,
        unitBoundary: isUnitBoundary,
        label: `${isUnitBoundary ? "【叙事单元】" : ""}${beats[p.beatIdx]?.title ?? ""} · 第${i + 1}段 · ${fmtTime(realDurs[i] || p.durationSec)}`,
      });
    });
    return ticks;
  }, [playlist, prefix, totalDur, beats, realDurs]);

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

  // 切集：重置播放器与数据态，避免旧集镜头残留到新集播放列表
  useEffect(() => {
    setShotsMap({});
    setRealDurs({});
    setCurIdx(0);
    setCurTime(0);
    setPlaying(false);
    videoRef.current?.pause();
  }, [activeEpisode]);

  useEffect(() => {
    void loadStructure();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, activeEpisode]);

  useEffect(() => {
    if (beats.length > 0) void loadShots(beats);
  }, [beats, loadShots]);

  // 导出历史加载（纯加载；刷新节奏由 useWsDrivenPoll 驱动）
  const loadExports = useCallback(async () => {
    try {
      const d = await exportApi.list(projectId);
      setExports(d.items ?? []);
    } catch (e) {
      toast.error(errMsg(e, "加载导出历史失败"));
    }
  }, [projectId]);

  // 有 PENDING 导出：WS 终态推送驱动刷新（导出任务），断线回退 3s 轮询（对齐 LuxReal 推送为主）
  const exportsPending = (exports ?? []).some((e) => e.status === "PENDING");
  const exportDoneRef = useRef(false);
  const refreshExports = useCallback(async () => {
    if (exportDoneRef.current) return;
    try {
      const dd = await exportApi.list(projectId);
      setExports(dd.items ?? []);
      if (!(dd.items ?? []).some((e) => e.status === "PENDING")) {
        exportDoneRef.current = true;
        toast.success("合成导出已完成");
        void projectsApi.detail(projectId).then(setProj).catch(() => undefined);
      }
    } catch {
      /* 忽略刷新抖动 */
    }
  }, [projectId]);

  useWsDrivenPoll(refreshExports, {
    active: exportsPending,
    pollMs: 3000,
    match: matchTaskTypes(["FINAL_VIDEO_COMPOSE", "ENGINEER_FILE_EXPORT"]),
  });

  // 新一轮导出（重新出现 PENDING）→ 重置终态守卫
  useEffect(() => {
    if (exportsPending) exportDoneRef.current = false;
  }, [exportsPending]);

  useEffect(() => {
    void loadExports();
  }, [loadExports]);

  // 键盘控制：ESC 关闭弹窗；空格播放/暂停、←→ 快退/快进（输入框聚焦时不拦截）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setComposeOpen(false);
        setExportFileOpen(false);
        setSpeedOpen(false);
        setQualityOpen(false);
        return;
      }
      const target = e.target as HTMLElement | null;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      if (!videoRef.current) return;
      if (e.code === "Space") {
        e.preventDefault();
        togglePlay();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        skip(-5);
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        skip(5);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [totalDur, curIdx, playing, globalTime]);

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
      void v.play().catch(() => {
        // 播放失败（素材 URL 失效/非视频内容）：自动尝试下一段，末段则停止
        if (curIdx < playlist.length - 1) {
          jumpTo(curIdx + 1, 0);
        } else {
          setPlaying(false);
          toast.error("视频无法播放，素材可能已失效");
        }
      });
      setPlaying(true);
    }
  };

  // 播放中途出错（error 事件）：同样跳下一段，避免卡死在坏段
  const onVideoError = () => {
    if (curIdx < playlist.length - 1) {
      jumpTo(curIdx + 1, 0);
    } else {
      setPlaying(false);
      toast.error("当前片段无法播放");
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
      videoRef.current?.pause();
      setPlaying(false);
    }
  };

  // 段内时间更新：元数据已校正真实时长后按真实时长切段
  const onTimeUpdate = (t: number) => {
    setCurTime(t);
    const seg = playlist[curIdx];
    const segDur = (seg && realDurs[curIdx]) || seg?.durationSec || 0;
    if (seg && segDur > 0 && t >= segDur - 0.03) {
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
    // 校正真实时长：素材实际时长与 DB duration_sec 漂移时以视频元数据为准
    if (v.duration > 0 && Number.isFinite(v.duration)) {
      setRealDurs((prev) => (Math.abs((prev[curIdx] ?? 0) - v.duration) > 0.05 ? { ...prev, [curIdx]: v.duration } : prev));
    }
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

  // 合成前预检：弹窗打开时拉取片段统计（按当前激活集）
  useEffect(() => {
    if (!composeOpen || !projectId) return;
    setPreflightLoading(true);
    setPreflight(null);
    exportApi.preflight(projectId, activeEpisode ?? undefined)
      .then(setPreflight)
      .catch(() => setPreflight(null))
      .finally(() => setPreflightLoading(false));
  }, [composeOpen, projectId, activeEpisode]);

  // 下载 403（签名 URL 过期）→ 引导一键重新合成
  const handleDownloadError = (e: ExportItem) => {
    toast.error("下载链接已过期，正在为您重新合成", {
      id: `retry-${e.export_id}`,
    });
    exportApi.retry(e.export_id)
      .then(() => {
        toast.success("已重新发起合成，完成后可下载");
        void loadExports();
      })
      .catch((err) => toast.error(errMsg(err, "重新合成失败")));
  };

  // 校验下载链接（fetch 探测；403/404 视为过期走重试）
  const handleDownload = (e: ExportItem) => {
    if (!e.output_url) return;
    fetch(e.output_url, { method: "HEAD" })
      .then((resp) => {
        if (resp.status === 403 || resp.status === 404) {
          handleDownloadError(e);
        } else {
          window.open(e.output_url, "_blank", "noopener");
        }
      })
      .catch(() => window.open(e.output_url, "_blank", "noopener"));
  };

  // 合成导出（3.4.1：MP4_H264 + 清晰度 + 合规水印）
  const canExport = proj != null && EXPORTABLE.includes(proj.status);
  const handleCompose = async (resolution: string) => {
    try {
      setSubmitting(true);
      await exportApi.trigger(projectId, resolution, true, activeEpisode ?? undefined);
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

  // 部分完成统计：已就绪单元数 / 整集预估时长（未完成单元按预设时长计入）
  const episodeStats = useMemo(() => {
    const doneBeats = beats.filter((b) => (shotsMap[b.beat_id] ?? []).some((s) => s.video_generation_status === "SUCCESS")).length;
    const fullDur = beats.reduce((acc, b) => {
      const bs = shotsMap[b.beat_id] ?? [];
      const done = bs.filter((s) => s.video_generation_status === "SUCCESS").reduce((a, s) => a + (s.duration_sec || 0), 0);
      const fallback = b.actual_duration_sec > 0 ? b.actual_duration_sec : b.preset_duration_sec;
      return acc + (done > 0 ? done : fallback);
    }, 0);
    return { doneBeats, totalBeats: beats.length, fullDur };
  }, [beats, shotsMap]);
  const partialReady = episodeStats.doneBeats < episodeStats.totalBeats && episodeStats.doneBeats > 0;

  // 项目加载失败（404/无权限/后端异常）整页降级，避免无限“加载中…”
  if (loadError) return <ProjectLoadError />;

  return (
    <WorkbenchShell
      projectTitle={proj?.title ?? ""}
      currentStep={stepState.currentStep}
      canClick={stepState.canClick}
      goStep={stepState.goStep}
      stepDone={stepState.stepDone}
      episodes={episodes}
      activeEpisodeId={activeEpisode}
      onSelectEpisode={setActiveEpisode}
      activeEpisodeBeats={beats}
    >
      {/* Content Scroll Area */}
      <div className="flex-1 overflow-y-auto flex flex-col px-6 pt-10 pb-6 scrollbar-thin" style={{ justifyContent: "flex-start" }}>
        {/* ===== VIDEO PLAYER ===== */}
        <div className="flex-shrink-0 relative w-full flex justify-center mb-8" style={{ height: "76vh" }}>
          <div
            ref={playerBoxRef}
            className="relative h-full rounded-2xl overflow-hidden bg-black shadow-2xl shadow-black/60 group/pv"
            style={{ width: "calc(76vh * 9 / 16)", maxWidth: "520px" }}
            onMouseEnter={() => setControlsVisible(true)}
            onMouseLeave={() => { if (playing) setControlsVisible(false); }}
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
                  onError={onVideoError}
                  onClick={togglePlay}
                  preload="metadata"
                />
                {/* 下一段预加载：缓解连播切源黑屏 */}
                {playlist[curIdx + 1] && (
                  <video src={playlist[curIdx + 1].url} className="hidden" preload="auto" aria-hidden />
                )}
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
              /* 无成片：渐变占位 + 引导 CTA（17 - 工作台对齐，对齐 LuxReal"去生成"） */
              <div className="video-bg absolute inset-0 flex flex-col items-center justify-center gap-3">
                <Film size={36} className="text-white/20" />
                <span className="text-xs text-white/40">{canExport ? "暂无已完成的镜头视频" : "完成 Step3 视频生成后可预览成片"}</span>
                {!canExport && (
                  <button
                    className="h-9 px-5 rounded-xl flex items-center gap-2 text-xs text-white font-medium transition-all hover:opacity-90"
                    style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                    onClick={() => stepState.goStep(3)}
                  >
                    去生成
                  </button>
                )}
              </div>
            )}

            {/* 左上角剧集徽章 + 部分完成提示 */}
            {activeEp && (
              <div className="absolute top-4 left-4 flex flex-col gap-1.5 items-start z-10">
                <div className="bg-black/60 backdrop-blur-sm text-white text-xs px-3 py-1.5 rounded-full flex items-center gap-1.5">
                  <Film size={13} />
                  第{activeEp.sort_order}集 {activeEp.title}
                </div>
                {partialReady && (
                  <div className="bg-[#F5B23C]/20 backdrop-blur-sm text-[#F5B23C] text-[11px] px-3 py-1 rounded-full">
                    部分单元未生成，已跳过播放
                  </div>
                )}
              </div>
            )}

            {/* 右上角清晰度（预览仅源清晰度，导出时可选其他） */}
            <div className="absolute top-4 right-4 z-10">
              <div
                className="bg-black/60 backdrop-blur-sm text-white text-xs px-3 py-1.5 rounded-full flex items-center gap-1 font-medium"
                title="预览使用原始生成清晰度，导出合成时可选择 480P/720P/1080P"
              >
                <span>源 · 1080P</span>
              </div>
            </div>

            {/* 底部控制栏（hover 显隐，播放中自动淡出） */}
            {curItem && (
              <div
                className={`absolute bottom-0 left-0 right-0 px-4 pb-4 pt-10 z-10 transition-opacity duration-200 ${controlsVisible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
                style={{ background: "linear-gradient(to top, rgba(0,0,0,0.85) 0%, rgba(0,0,0,0.5) 50%, transparent 100%)" }}
              >
                {/* 进度条 + 时间（时间置于进度条两端；分段节点对齐 LuxReal Timeline-section） */}
                <div className="flex items-center gap-2 mb-3">
                  <span className="text-white/90 text-[11px] font-mono tabular-nums shrink-0">{fmtTime(globalTime)}</span>
                  <div
                    className="relative flex-1 h-1 bg-white/20 rounded-full cursor-pointer group"
                    onClick={(e) => {
                      const rect = e.currentTarget.getBoundingClientRect();
                      seekGlobal(((e.clientX - rect.left) / rect.width) * totalDur);
                    }}
                  >
                    <div className="h-full rounded-full bg-white relative" style={{ width: `${progressPct}%` }}>
                      <div className="absolute right-0 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-white shadow-md -mr-1.5" />
                    </div>
                    {/* 分段节点：亮=叙事单元边界，暗=单元内镜头块边界；悬停显示所属单元与时长 */}
                    {segTicks.map((t, i) => (
                      <div
                        key={i}
                        title={t.label}
                        className={`absolute top-1/2 -translate-y-1/2 -ml-px pointer-events-auto ${t.unitBoundary ? "w-[2px]" : "w-px"}`}
                        style={{
                          left: `${t.pct}%`,
                          height: t.unitBoundary ? "12px" : "7px",
                          background: t.unitBoundary ? "rgba(255,255,255,0.9)" : "rgba(255,255,255,0.45)",
                          boxShadow: t.unitBoundary ? "0 0 3px rgba(0,0,0,0.6)" : "none",
                        }}
                      />
                    ))}
                  </div>
                  <span className="text-white/60 text-[11px] font-mono tabular-nums shrink-0">{fmtTime(totalDur)}</span>
                </div>

                <div className="flex items-center justify-between">
                  {/* 左：播放/回退/前进/音量 */}
                  <div className="flex items-center gap-2">
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
                        onPointerDown={(e) => {
                          const apply = (clientX: number) => {
                            const rect = e.currentTarget.getBoundingClientRect();
                            const nv = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
                            setVolume(nv);
                            if (videoRef.current) videoRef.current.volume = nv;
                          };
                          apply(e.clientX);
                          const onMove = (ev: PointerEvent) => apply(ev.clientX);
                          const onUp = () => {
                            window.removeEventListener("pointermove", onMove);
                            window.removeEventListener("pointerup", onUp);
                          };
                          window.addEventListener("pointermove", onMove);
                          window.addEventListener("pointerup", onUp);
                        }}
                      >
                        <div className="h-full bg-white rounded-full" style={{ width: `${volume * 100}%` }} />
                      </div>
                    </div>
                  </div>
                  {/* 右：倍速/画中画/全屏 */}
                  <div className="flex items-center gap-2">
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
                    {/* 清晰度（预览档位展示；成片以导出分辨率真源为准） */}
                    <div className="relative">
                      <button
                        className="text-white/80 text-xs hover:text-white transition"
                        onClick={(e) => {
                          e.stopPropagation();
                          setQualityOpen(!qualityOpen);
                          setSpeedOpen(false);
                        }}
                      >
                        1080P
                      </button>
                      {qualityOpen && (
                        <div className="absolute bottom-full right-0 mb-2 bg-[#1E1E24] border border-[#2A2A35] rounded-lg py-1 w-20 z-20 shadow-xl">
                          {["480P", "720P", "1080P"].map((q) => (
                            <button
                              key={q}
                              className={`w-full text-left text-sm py-2 px-3 transition-colors hover:bg-white/5 ${q === "1080P" ? "text-[#4C7FF8] font-medium" : "text-white/70"}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                setQualityOpen(false);
                                toast.info(`${q} 预览档位切换即将上线`);
                              }}
                            >
                              {q}
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
              {beats.map((b, i) => {
                // 镜头完成度占位（17 - 工作台对齐 P2：占位而非静默过滤，对齐 LuxReal 镜头块状态徽标）
                const bs = shotsMap[b.beat_id] ?? [];
                const doneN = bs.filter((s) => s.video_generation_status === "SUCCESS").length;
                const runningN = bs.filter((s) => s.video_generation_status === "RUNNING").length;
                // 单元时长对齐真实拼接结果：已完成片段实际时长之和；无完成片段时回退预设/解析时长
                const doneDur = bs
                  .filter((s) => s.video_generation_status === "SUCCESS")
                  .reduce((acc, s) => acc + (s.duration_sec || 0), 0);
                const shownDur = doneDur > 0 ? doneDur : b.actual_duration_sec > 0 ? b.actual_duration_sec : b.preset_duration_sec;
                return (
                  <div key={b.beat_id} className="flex items-center">
                    {i > 0 && <div className="timeline-connector" />}
                    <div className="timeline-unit">
                      <div
                        className={`timeline-unit-card ${curItem?.beatId === b.beat_id ? "active" : ""}`}
                        onClick={() => jumpToBeat(i)}
                      >
                        <div className="flex items-center justify-between gap-2">
                          <div className="timeline-unit-number">叙事单元{b.sort_order}</div>
                          {bs.length > 0 && (
                            <span
                              className="text-[9px] px-1.5 py-0.5 rounded flex items-center gap-1"
                              style={{
                                background: doneN === bs.length ? "rgba(16,185,129,0.15)" : runningN > 0 ? "rgba(245,178,60,0.15)" : "rgba(107,112,136,0.15)",
                                color: doneN === bs.length ? "#10B981" : runningN > 0 ? "#F5B23C" : "#8B8B9A",
                              }}
                            >
                              {runningN > 0 && <Loader2 size={9} className="animate-spin" />}
                              {doneN === bs.length ? `${doneN}段已完成` : `${doneN}/${bs.length}段`}
                            </span>
                          )}
                        </div>
                        <div className="timeline-unit-title">{b.title}</div>
                        <div className="timeline-unit-duration">
                          <Clock size={12} />
                          <span>{fmtTime(shownDur)}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
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
              {partialReady
                ? `已就绪 ${fmtTime(totalDur)} / 整集约 ${fmtTime(episodeStats.fullDur)} · ${episodeStats.doneBeats}/${episodeStats.totalBeats} 单元完成`
                : `总时长 ${fmtTime(totalDur)} · 1080P · 9:16 竖屏`}
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
            exports.map((e) => {
              const isComposing = e.status === "PENDING";
              const stage = e.progress_stage ?? "";
              const pd = e.progress_detail ?? {};
              // 阶段进度条：下载 0-60%，合成 60-85%，上传 85-99%，DONE 100%
              let pct = 0;
              let stageLabel = "";
              if (stage === "DOWNLOADING") {
                pct = 5 + Math.min(55, (pd.done ?? 0) / Math.max(1, pd.total ?? 1) * 55);
                stageLabel = `下载片段 ${pd.done ?? 0}/${pd.total ?? 0}`;
              } else if (stage === "COMPOSING") {
                pct = 62;
                stageLabel = "FFmpeg 合成中";
              } else if (stage === "UPLOADING") {
                pct = 88;
                stageLabel = "上传成片";
              } else if (stage === "DONE" || e.status === "SUCCESS") {
                pct = 100;
              }
              return (
                <div key={e.export_id} className="export-history-row !items-start">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-3 min-w-0">
                      <span className={`export-badge ${e.status === "SUCCESS" ? "success" : e.status === "FAILED" ? "failed" : "pending"}`}>
                        {e.status === "SUCCESS" ? "已完成" : e.status === "FAILED" ? "失败" : "导出中"}
                      </span>
                      <span className="text-xs text-[#8B8B9A]">{RESOLUTIONS.find((r) => r.key === e.resolution)?.label ?? e.resolution}</span>
                      {e.status === "SUCCESS" && e.file_size > 0 && (
                        <span className="text-xs text-[#4A4A5A]">{fmtSize(e.file_size)}</span>
                      )}
                      {(e.segments_total ?? 0) > 0 && (
                        <span className="text-xs text-[#4A4A5A]">{e.segments_total} 段{((e.segments_failed ?? 0) > 0) ? `（跳过 ${e.segments_failed}）` : ""}</span>
                      )}
                      <span className="text-xs text-[#4A4A5A] truncate">{new Date(e.created_at).toLocaleString()}</span>
                    </div>
                    {/* 合成进度条（PENDING 阶段展示阶段+百分比） */}
                    {isComposing && (
                      <div className="mt-2">
                        <div className="h-1 rounded-full bg-white/10 overflow-hidden">
                          <div
                            className="h-full rounded-full transition-all duration-500"
                            style={{ width: `${pct}%`, background: "linear-gradient(90deg, #4C7FF8, #8B5CF6)" }}
                          />
                        </div>
                        <div className="text-[10px] text-[#6B7088] mt-1">
                          {stageLabel || "排队中…"}
                          {(pd.failed ?? 0) > 0 && <span className="text-[#F5B23C]"> · {pd.failed} 个失效片段已跳过</span>}
                        </div>
                      </div>
                    )}
                  </div>
                  {e.status === "SUCCESS" && e.output_url && (
                    <a
                      className="btn-export-download"
                      href={e.output_url}
                      target="_blank"
                      rel="noreferrer"
                      onClick={(ev) => {
                        // 异步校验签名 URL；过期则拦截默认行为走重试
                        fetch(e.output_url, { method: "HEAD" }).then((resp) => {
                          if (resp.status === 403 || resp.status === 404) {
                            ev.preventDefault();
                            handleDownloadError(e);
                          }
                        }).catch(() => undefined);
                      }}
                    >
                      <Download size={12} />
                      下载
                    </a>
                  )}
                  {e.status === "FAILED" && (
                    <button
                      className="text-[11px] px-2.5 py-1.5 rounded-lg font-medium text-white flex items-center gap-1 hover:opacity-90"
                      style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                      onClick={() => {
                        exportApi.retry(e.export_id)
                          .then(() => {
                            toast.success("已重新发起合成");
                            void loadExports();
                          })
                          .catch((err) => toast.error(errMsg(err, "重新合成失败")));
                      }}
                    >
                      <RefreshCw size={11} />
                      重新合成
                    </button>
                  )}
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* ===== MODALS ===== */}
      {exportFileOpen && <ExportFileModal onClose={() => setExportFileOpen(false)} />}
      {composeOpen && (
        <ComposeModal
          totalSec={totalDur}
          submitting={submitting}
          preflight={preflight}
          preflightLoading={preflightLoading}
          onClose={() => setComposeOpen(false)}
          onSubmit={(r) => void handleCompose(r)}
        />
      )}
    </WorkbenchShell>
  );
}
