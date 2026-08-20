// S1-F05 Step3 视频制作页 —— 按 luxreal-clone 设计稿 workbench-video.html 融合（批次 3）
// 布局：WorkbenchShell + settings bar + 叙事单元卡（展开/收起）+ 镜头网格 + 生成视频弹窗 + 底部悬浮条
// 接口：GET /episodes/:id/beats、GET /beats/:id、GET /beats/:id/shots、POST /beats/:id/generate-videos
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  Box, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Clapperboard, Film,
  GripVertical, History, ImagePlus, Loader2, Mic2, Monitor, MoreHorizontal,
  Pencil, Play, Plus, RefreshCw, Settings2, Sparkles, Subtitles, User, X, Zap,
} from "lucide-react";

import {
  ApiError,
  errorMessage,
  projectsApi,
  splitApi,
  videoApi,
  versionApi,
  type BeatDetail,
  type BeatSummary,
  type EpisodeLite,
  type NuVersionItem,
  type ProjectDetail,
  type ShotVideo,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import "./step3.css";

// 错误文案：ApiError 走错误码映射（05 C 章），避免弹出后端原始报文
function errMsg(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return errorMessage(e.code, e.message);
  return e instanceof Error ? e.message : fallback;
}

// 拆分后镜头初始为 PENDING（待生成），仅 RUNNING 视为活动生成态
const isActive = (s: string) => s === "RUNNING";

// 版本时间格式化：ISO → MM-DD HH:mm
function fmtVersionTime(iso: string): string {
  const m = iso?.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
  if (!m) return iso || "—";
  return `${m[2]}-${m[3]} ${m[4]}:${m[5]}`;
}

// BeatDetail 展平后的单镜头文案
type ShotDetailLite = BeatDetail["shot_groups"][number]["shots"][number];

// 视频模型选项（弹窗三选一；积分系数仅用于预估展示，实际计费以后端为准）
const MODEL_OPTIONS = [
  { key: "2.5", name: "Seedance 2.5", badge: "PRO", desc: "30秒 · 精修可控", factor: 4 },
  { key: "2.0", name: "Seedance 2.0", badge: "", desc: "15秒 · 高一致性", factor: 3 },
  { key: "1.5", name: "Seedance 1.5", badge: "", desc: "10秒 · 高效批量", factor: 2 },
];

// 镜头状态徽章（对齐设计稿配色）
function ShotStatusBadge({ status }: { status: string }) {
  if (status === "SUCCESS") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(16,185,129,0.15)", color: "#10B981" }}>已完成</span>;
  }
  if (status === "RUNNING") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(76,127,248,0.15)", color: "#4C7FF8" }}>生成中</span>;
  }
  if (status === "FAILED") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>失败</span>;
  }
  return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "#1A1A2E", color: "#6B7088" }}>待生成</span>;
}

// ===== 镜头卡 =====
function ShotCard({ shot, detail }: { shot: ShotVideo; detail?: ShotDetailLite }) {
  const done = shot.video_generation_status === "SUCCESS";
  const running = shot.video_generation_status === "RUNNING";
  // 参考图 chips：无数据源时按镜头序号轮转渐变色（视觉保真占位）
  const grads = [`lens-grad-${(shot.sort_order % 6) + 1}`, `lens-grad-${((shot.sort_order + 2) % 6) + 1}`, `lens-grad-${((shot.sort_order + 4) % 6) + 1}`];

  return (
    <div className="shot-card">
      <div className="shot-header">
        <span className="shot-ref-badge">{done ? "视频" : "参考"}</span>
        <div className="shot-header-actions">
          <div className="shot-duration" onClick={() => toast.info("镜头时长调整将在后续版本开放")}>
            <span>{shot.duration_sec}s</span>
            <ChevronDown size={12} />
          </div>
          <button className="icon-btn icon-btn-sm" onClick={() => toast.info("单镜头重新生成将在后续版本开放")}>
            <RefreshCw size={14} />
          </button>
          <button className="icon-btn icon-btn-sm" onClick={() => toast.info("更多操作即将上线")}>
            <MoreHorizontal size={14} />
          </button>
          <button className="icon-btn icon-btn-sm" onClick={() => toast.info("全屏预览即将上线")}>
            <MaximizeIcon />
          </button>
        </div>
      </div>
      <div className="shot-content">
        {/* 画面区：成功 → 视频；生成中 → 进度条；待生成 → 剪影 */}
        <div className="shot-image">
          {done && shot.generated_video_url ? (
            <video src={shot.generated_video_url} className="absolute inset-0 w-full h-full object-cover" controls preload="metadata" />
          ) : running ? (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 px-6">
              <Loader2 size={24} className="animate-spin text-[#4C7FF8]" />
              <span className="text-[11px] text-[#4C7FF8]">视频渲染中…</span>
              <div className="shot-progress w-full">
                <div className="shot-progress-bar" />
              </div>
            </div>
          ) : (
            <div className="shot-silhouette">
              <User size={40} className="text-white" />
            </div>
          )}
          {!done && (
            <>
              <button className="shot-image-nav left" onClick={() => toast.info("参考图切换将在后续版本开放")}>
                <ChevronLeft size={12} />
              </button>
              <button className="shot-image-nav right" onClick={() => toast.info("参考图切换将在后续版本开放")}>
                <ChevronRight size={12} />
              </button>
            </>
          )}
        </div>
        <div className="shot-details">
          {/* 镜头号 + 状态 + 模型 */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-white">镜头 {String(shot.sort_order).padStart(2, "0")}</span>
              <ShotStatusBadge status={shot.video_generation_status} />
            </div>
            <span className="text-[10px] px-2 py-0.5 rounded" style={{ background: "#1A1A2E", color: "#6B7088" }}>Seedance 2.5 · 1080P</span>
          </div>
          {/* 参考图 chips */}
          <div className="shot-ref-chips">
            {grads.map((g) => (
              <div key={g} className={`shot-ref-chip ${g}`} onClick={() => toast.info("参考图管理将在后续版本开放")} />
            ))}
            <div className="shot-ref-chip add" onClick={() => toast.info("添加参考图将在后续版本开放")}>
              <Plus size={12} />
            </div>
          </div>
          {/* 提示词描述（景别/运镜/主体动作/台词） */}
          <div className="shot-desc">
            {(detail?.shot_scale || detail?.camera_movement || detail?.camera_angle) && (
              <div className="flex flex-wrap gap-1.5 mb-1.5">
                {detail.shot_scale && (
                  <span className="text-[10px]"><span style={{ color: "#4C7FF8" }}>【景别】</span><span className="text-white/80">{detail.shot_scale}</span></span>
                )}
                {detail.camera_movement && (
                  <span className="text-[10px]"><span style={{ color: "#4C7FF8" }}>【运镜】</span><span className="text-white/80">{detail.camera_movement}</span></span>
                )}
                {detail.camera_angle && (
                  <span className="text-[10px]"><span style={{ color: "#4C7FF8" }}>【画面氛围】</span><span className="text-white/80">{detail.camera_angle}</span></span>
                )}
              </div>
            )}
            {detail?.subject_description && (
              <div className="text-[11px] text-white/70 leading-relaxed">
                <span style={{ color: "#4C7FF8" }}>【主体动作】</span>{detail.subject_description}
              </div>
            )}
            {detail?.dialogue_text && (
              <div className="text-[11px] text-white/70 leading-relaxed mt-1">
                <span style={{ color: "#4C7FF8" }}>【台词】</span>"{detail.dialogue_text}"
              </div>
            )}
          </div>
          {/* 操作行 */}
          <div className="flex items-center justify-between mt-auto pt-1">
            <div className="flex items-center gap-1.5">
              <button
                className="flex items-center gap-1 text-xs px-2.5 py-1 rounded-lg border text-white/70 hover:text-white transition-colors"
                style={{ background: "#16161C", borderColor: "#2A2A35" }}
                onClick={() => toast.info("提示词编辑将在后续版本开放")}
              >
                <Pencil size={12} />
                <span>编辑提示词</span>
              </button>
              <button
                className="flex items-center gap-1 text-xs px-2.5 py-1 rounded-lg border text-white/70 hover:text-white transition-colors"
                style={{ background: "#16161C", borderColor: "#2A2A35" }}
                onClick={() => toast.info("添加参考图将在后续版本开放")}
              >
                <ImagePlus size={12} />
                <span>添加参考图</span>
              </button>
            </div>
            <div className="flex items-center gap-1.5">
              <button
                className="flex items-center gap-1 text-xs px-3 py-1 rounded-lg text-white font-medium disabled:opacity-50"
                style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                disabled={isActive(shot.video_generation_status)}
                onClick={() => toast.info("单镜头视频生成将在后续版本开放，请使用单元级「生成视频」")}
              >
                <Play size={12} className="fill-white" />
                <span>生成视频</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// maximize-2 小图标（lucide Maximize2 名称避免与 CSS 冲突直接内联）
function MaximizeIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="15 3 21 3 21 9" /><polyline points="9 21 3 21 3 15" />
      <line x1="21" x2="14" y1="3" y2="10" /><line x1="3" x2="10" y1="21" y2="14" />
    </svg>
  );
}

// ===== 生成视频弹窗 =====
function GenVideoModal({
  beat,
  shots,
  submitting,
  onClose,
  onConfirm,
}: {
  beat: BeatSummary;
  shots: ShotVideo[];
  submitting: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const [model, setModel] = useState("2.5");
  const [resolution, setResolution] = useState("1080P");
  const [fps, setFps] = useState("24fps");
  const [advOpen, setAdvOpen] = useState(true);
  const [adv, setAdv] = useState({ lens: true, box3d: false, subtitle: false, voice: false });
  const advCount = Object.values(adv).filter(Boolean).length;

  // 预计消耗：总时长 × 模型系数（仅预估展示，实际计费以后端为准）
  const totalSec = shots.reduce((acc, s) => acc + (s.duration_sec || 0), 0);
  const factor = MODEL_OPTIONS.find((m) => m.key === model)?.factor ?? 4;
  const estCost = Math.max(totalSec * factor, shots.length * 10);

  return (
    <div className="gen-modal-overlay" onClick={onClose}>
      <div className="gen-video-modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">生成视频</div>
          <div className="modal-desc">{beat.title} · {shots.length || beat.shot_group_count} 个镜头 · 选择模型与参数生成</div>
        </div>
        <div className="gen-modal-body">
          {/* 生成模型 */}
          <div className="gen-model-section">
            <div className="gen-section-header">
              <span className="gen-section-title">生成模型</span>
              <span className="gen-section-hint">不同模型消耗积分不同</span>
            </div>
            <div className="gen-model-tabs">
              {MODEL_OPTIONS.map((m) => (
                <div key={m.key} className={`gen-model-tab ${model === m.key ? "active" : ""}`} onClick={() => setModel(m.key)}>
                  <div className="gen-model-tab-name">
                    {m.name}
                    {m.badge && <span className="gen-model-tab-badge">{m.badge}</span>}
                  </div>
                  <div className="gen-model-tab-desc">{m.desc}</div>
                </div>
              ))}
            </div>
          </div>

          {/* 分辨率 / 帧率 */}
          <div className="gen-quality-row">
            <div className="gen-quality-item">
              <div className="gen-quality-label">
                <Monitor />
                分辨率
              </div>
              <div className="gen-segmented">
                {["480P", "1080P", "4K"].map((r) => (
                  <button key={r} className={`gen-segmented-btn ${resolution === r ? "active" : ""}`} onClick={() => setResolution(r)}>{r}</button>
                ))}
              </div>
            </div>
            <div className="gen-quality-item">
              <div className="gen-quality-label">
                <Film />
                帧率
              </div>
              <div className="gen-segmented">
                {["24fps", "30fps"].map((f) => (
                  <button key={f} className={`gen-segmented-btn ${fps === f ? "active" : ""}`} onClick={() => setFps(f)}>{f}</button>
                ))}
              </div>
            </div>
          </div>

          {/* 高级选项 */}
          <div className="gen-advanced-section">
            <div className={`gen-advanced-header ${advOpen ? "open" : ""}`} onClick={() => setAdvOpen(!advOpen)}>
              <span className="gen-advanced-title">
                <ChevronDown />
                高级选项
              </span>
              <span className="gen-advanced-count">已开启 {advCount} 项</span>
            </div>
            {advOpen && (
              <div>
                <div className="gen-toggle-row">
                  <div className="gen-toggle-info">
                    <div className="gen-toggle-icon"><Sparkles /></div>
                    <div className="gen-toggle-text">
                      <span className="gen-toggle-name">智能镜头优化</span>
                      <span className="gen-toggle-desc">使用关键帧提升画面一致性</span>
                    </div>
                  </div>
                  <div className={`toggle-switch ${adv.lens ? "on" : ""}`} onClick={() => setAdv({ ...adv, lens: !adv.lens })} />
                </div>
                <div className="gen-toggle-row">
                  <div className="gen-toggle-info">
                    <div className="gen-toggle-icon" style={{ background: "rgba(139, 92, 246, 0.1)", color: "#8B5CF6" }}><Box /></div>
                    <div className="gen-toggle-text">
                      <span className="gen-toggle-name">3D 空间约束</span>
                      <span className="gen-toggle-desc">保持场景空间关系稳定</span>
                    </div>
                  </div>
                  <div className={`toggle-switch ${adv.box3d ? "on" : ""}`} onClick={() => setAdv({ ...adv, box3d: !adv.box3d })} />
                </div>
                <div className="gen-toggle-row">
                  <div className="gen-toggle-info">
                    <div className="gen-toggle-icon" style={{ background: "rgba(245, 158, 11, 0.1)", color: "#F59E0B" }}><Subtitles /></div>
                    <div className="gen-toggle-text">
                      <span className="gen-toggle-name">自动生成字幕</span>
                      <span className="gen-toggle-desc">根据剧本台词自动生成</span>
                    </div>
                  </div>
                  <div className={`toggle-switch ${adv.subtitle ? "on" : ""}`} onClick={() => setAdv({ ...adv, subtitle: !adv.subtitle })} />
                </div>
                <div className="gen-toggle-row">
                  <div className="gen-toggle-info">
                    <div className="gen-toggle-icon" style={{ background: "rgba(239, 68, 68, 0.1)", color: "#EF4444" }}><Mic2 /></div>
                    <div className="gen-toggle-text">
                      <span className="gen-toggle-name">AI 智能配音</span>
                      <span className="gen-toggle-desc">多音色 AI 配音角色台词</span>
                    </div>
                  </div>
                  <div className={`toggle-switch ${adv.voice ? "on" : ""}`} onClick={() => setAdv({ ...adv, voice: !adv.voice })} />
                </div>
              </div>
            )}
          </div>

          {/* 预计消耗 */}
          <div className="gen-cost-bar">
            <div className="gen-cost-left">
              <div className="gen-cost-icon"><Zap /></div>
              <div className="gen-cost-text">
                <span className="gen-cost-label">预计消耗</span>
                <span className="gen-cost-hint">以实际生成为准</span>
              </div>
            </div>
            <div className="gen-cost-right">
              <span className="gen-cost-value">{estCost}</span>
              <span className="gen-cost-unit">积分</span>
            </div>
          </div>
        </div>
        <div className="modal-footer">
          <button className="btn-modal-cancel" onClick={onClose}>取消</button>
          <button className="btn-modal-primary btn-gen-start" disabled={submitting} onClick={onConfirm}>
            {submitting ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
            {submitting ? "提交中…" : "开始生成"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ===== Step3 主页面 =====
export default function Step3Page() {
  const { projectId = "" } = useParams();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [loadError, setLoadError] = useState(false);
  const [shotsMap, setShotsMap] = useState<Record<string, ShotVideo[]>>({});
  const [detailsMap, setDetailsMap] = useState<Record<string, BeatDetail>>({});
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [scriptOpen, setScriptOpen] = useState<Record<string, boolean>>({});
  const [genTarget, setGenTarget] = useState<BeatSummary | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // 历史版本弹窗（S2-B05）：historyTarget 非空即挂载，historyActive 控制入场动画
  const [historyTarget, setHistoryTarget] = useState<BeatSummary | null>(null);
  const [historyActive, setHistoryActive] = useState(false);
  const [versions, setVersions] = useState<NuVersionItem[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [restoringId, setRestoringId] = useState<number | null>(null);
  // 重新规划弹窗（3.3.7c）：两个开关为纯 UI 视觉态，后端 replan 目前仅接收 planning_mode
  const [replanTarget, setReplanTarget] = useState<BeatSummary | null>(null);
  const [replanActive, setReplanActive] = useState(false);
  const [replanKeepRefs, setReplanKeepRefs] = useState(true);
  const [replanApplySkill, setReplanApplySkill] = useState(false);
  const [replanSubmitting, setReplanSubmitting] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stepState = useStepState(projectId, proj?.status ?? null);

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
        // 默认展开第一个叙事单元
        if (b.items.length > 0) {
          setExpandedIds((prev) => (prev.size > 0 ? prev : new Set([b.items[0].beat_id])));
        }
      }
    } catch (e) {
      setLoadError(true);
      toast.error(errMsg(e, "加载失败"));
    }
  }, [projectId, activeEpisode]);

  // 加载各 beat 的 shots（状态 + 视频 URL）与详情（镜头文案）
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
    return sMap;
  }, []);

  const loadDetails = useCallback(async (beatList: BeatSummary[]) => {
    const dMap: Record<string, BeatDetail> = {};
    await Promise.all(
      beatList.map(async (b) => {
        try {
          dMap[b.beat_id] = await splitApi.beatDetail(b.beat_id);
        } catch {
          /* 文案缺失不阻断 */
        }
      }),
    );
    setDetailsMap(dMap);
  }, []);

  useEffect(() => {
    void loadStructure();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, activeEpisode]);

  useEffect(() => {
    if (beats.length > 0) {
      void loadShots(beats);
      void loadDetails(beats);
    }
  }, [beats, loadShots, loadDetails]);

  const anyActive = useMemo(
    () => beats.some((b) => (shotsMap[b.beat_id] ?? []).some((s) => isActive(s.video_generation_status))),
    [beats, shotsMap],
  );

  // 有活动任务 → 3s 轮询；全部终态 → 停止并刷新项目状态（解锁 Step4）
  useEffect(() => {
    if (anyActive && !pollRef.current) {
      pollRef.current = setInterval(async () => {
        try {
          const map = await loadShots(beats);
          const still = beats.some((b) => (map[b.beat_id] ?? []).some((s) => isActive(s.video_generation_status)));
          if (!still && pollRef.current) {
            clearInterval(pollRef.current);
            pollRef.current = null;
            toast.success("视频生成完成");
            // 刷新项目状态：VIDEO_GENERATING → ALL_VIDEOS_READY（Step4 解锁）
            try {
              setProj(await projectsApi.detail(projectId));
            } catch { /* 忽略 */ }
          }
        } catch { /* 轮询容错 */ }
      }, 3000);
    } else if (!anyActive && pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, [anyActive, beats, loadShots, projectId]);

  // WS 实时进度：TASK_PROGRESS/TASK_SUCCESS → 主动刷新 shots
  useEffect(() => {
    const handler = (ev: Event) => {
      const data = (ev as CustomEvent).detail as { task_uuid?: string } | undefined;
      if (data?.task_uuid) void loadShots(beats);
    };
    window.addEventListener("task:progress", handler);
    return () => window.removeEventListener("task:progress", handler);
  }, [beats, loadShots]);

  const toggleExpand = (beatId: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(beatId)) next.delete(beatId);
      else next.add(beatId);
      return next;
    });
  };

  // 提交生成视频（弹窗确认后）
  const handleGenerate = async () => {
    if (!genTarget) return;
    try {
      setSubmitting(true);
      const r = await videoApi.generateBeat(genTarget.beat_id);
      toast.success(`已提交 ${r.created}/${r.total} 个视频生成任务`);
      setGenTarget(null);
      setExpandedIds((prev) => new Set(prev).add(genTarget.beat_id));
      // 立即刷新状态与项目状态（VIDEO_GENERATING）
      void loadShots(beats);
      try {
        setProj(await projectsApi.detail(projectId));
      } catch { /* 忽略 */ }
    } catch (e) {
      toast.error(errMsg(e, "生成视频失败"));
    } finally {
      setSubmitting(false);
    }
  };

  // BeatDetail shots 展平：sort_order → 镜头文案
  const detailOf = (beatId: string, sortOrder: number): ShotDetailLite | undefined => {
    const d = detailsMap[beatId];
    if (!d) return undefined;
    for (const g of d.shot_groups) {
      const hit = g.shots.find((s) => s.sort_order === sortOrder);
      if (hit) return hit;
    }
    return undefined;
  };

  // 弹窗入场动画：挂载后短暂延迟加 active，触发 opacity/scale transition（setTimeout 兼容后台标签页 rAF 节流）
  useEffect(() => {
    if (!historyTarget) {
      setHistoryActive(false);
      return;
    }
    const t = setTimeout(() => setHistoryActive(true), 30);
    return () => clearTimeout(t);
  }, [historyTarget]);

  useEffect(() => {
    if (!replanTarget) {
      setReplanActive(false);
      return;
    }
    const t = setTimeout(() => setReplanActive(true), 30);
    return () => clearTimeout(t);
  }, [replanTarget]);

  // 打开历史版本弹窗并加载版本列表（S2-B05，后端按 version_num 降序）
  const openHistory = async (beat: BeatSummary) => {
    setHistoryTarget(beat);
    setVersionsLoading(true);
    try {
      const r = await versionApi.list(beat.beat_id);
      setVersions(r.items ?? []);
    } catch (e) {
      toast.error(errMsg(e, "加载版本历史失败"));
    } finally {
      setVersionsLoading(false);
    }
  };

  // 恢复到指定版本：成功后重载结构/镜头（项目状态可能回退至 PARTIAL_ASSETS）
  const restoreVersion = async (v: NuVersionItem) => {
    if (!historyTarget) return;
    try {
      setRestoringId(v.version_id);
      await versionApi.rollback(historyTarget.beat_id, v.version_id);
      toast.success(`已恢复到 v${v.version_num}`);
      setHistoryTarget(null);
      void loadStructure();
      void loadShots(beats);
      void loadDetails(beats);
    } catch (e) {
      toast.error(errMsg(e, "恢复失败"));
    } finally {
      setRestoringId(null);
    }
  };

  // 确认重新规划（3.3.7c）：后端入队 BEAT_REPLAN 任务（开发期 mock）
  const confirmReplan = async () => {
    if (!replanTarget) return;
    try {
      setReplanSubmitting(true);
      await splitApi.replan(replanTarget.beat_id);
      toast.success("重新规划任务已提交，请稍候");
      setReplanTarget(null);
    } catch (e) {
      toast.error(errMsg(e, "提交重新规划失败"));
    } finally {
      setReplanSubmitting(false);
    }
  };

  // 待生成镜头总数（底部悬浮条计数）
  const pendingCount = useMemo(
    () => beats.reduce((acc, b) => acc + (shotsMap[b.beat_id] ?? []).filter((s) => s.video_generation_status !== "SUCCESS").length, 0),
    [beats, shotsMap],
  );

  // 项目加载失败（404/无权限/后端异常）整页降级，避免无限“加载中…”
  if (loadError) return <ProjectLoadError />;

  return (
    <WorkbenchShell
      projectTitle={proj?.title ?? ""}
      currentStep={3}
      canClick={stepState.canClick}
      goStep={stepState.goStep}
      episodes={episodes}
      activeEpisodeId={activeEpisode}
      onSelectEpisode={setActiveEpisode}
      activeEpisodeBeats={beats}
    >
      {/* Settings Bar（视频模型/分辨率/比例/风格 + 高级设置） */}
      <div className="flex items-center justify-between px-6 py-2 flex-shrink-0">
        <div className="flex items-center gap-3 mx-auto" style={{ maxWidth: "85%", width: "100%" }}>
          {[
            { label: "视频模型", value: "Seedance 2.5" },
            { label: "分辨率", value: "1080P" },
            { label: "比例", value: "16:9" },
            { label: "风格", value: proj?.tags?.[0] || "武侠古风" },
          ].map((it) => (
            <div key={it.label} className="flex items-center gap-2">
              <span className="text-xs text-[#6B7088]">{it.label}</span>
              <div
                className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg border cursor-pointer hover:border-[#4C7FF8] transition-colors"
                style={{ background: "#16161C", borderColor: "#2A2A35" }}
                onClick={() => toast.info(`${it.label}切换将在后续版本开放`)}
              >
                <span className="text-xs text-white">{it.value}</span>
                <ChevronDown size={12} className="text-[#6B7088]" />
              </div>
            </div>
          ))}
          <div className="ml-auto flex items-center gap-2">
            <button
              className="flex items-center gap-1.5 text-xs text-[#6B7088] hover:text-white transition-colors px-2.5 py-1 rounded-lg border"
              style={{ background: "#16161C", borderColor: "#2A2A35" }}
              onClick={() => toast.info("高级设置将在后续版本开放")}
            >
              <Settings2 size={14} />
              <span>高级设置</span>
            </button>
          </div>
        </div>
      </div>

      {/* Content Scroll Area */}
      <div className="flex-1 overflow-y-auto px-6 pb-24 scrollbar-thin">
        <div className="flex flex-col gap-3 mx-auto" style={{ maxWidth: "85%" }}>
          {beats.length === 0 && (
            <div className="rounded-2xl border border-dashed border-white/10 p-14 text-center text-sm text-slate-500">
              暂无叙事单元。请先在 Step1 完成剧本拆分。
            </div>
          )}

          {beats.map((beat) => {
            const shots = shotsMap[beat.beat_id] ?? [];
            const expanded = expandedIds.has(beat.beat_id);
            const running = shots.some((s) => isActive(s.video_generation_status));
            const doneCount = shots.filter((s) => s.video_generation_status === "SUCCESS").length;
            const allDone = shots.length > 0 && doneCount === shots.length;
            const showScript = scriptOpen[beat.beat_id] ?? true;

            return (
              <div key={beat.beat_id} id={`unit-card-${beat.beat_id}`} className="video-unit-card">
                {/* Header */}
                <div className="unit-header" onClick={() => toggleExpand(beat.beat_id)} style={{ cursor: "pointer" }}>
                  <GripVertical size={16} className="unit-drag" onClick={(e) => e.stopPropagation()} />
                  <div className="unit-title-group">
                    <span className="unit-label">叙事单元{beat.sort_order}</span>
                    <span className="unit-sep">|</span>
                    <span className="unit-name">{beat.title}</span>
                  </div>
                  {/* 状态横幅：规划完成 / 生成中 / 全部完成 */}
                  {expanded && shots.length > 0 && (
                    running ? (
                      <div className="unit-status-banner running">
                        <Loader2 size={13} className="animate-spin" />
                        <span>视频生成中（{doneCount}/{shots.length}），请稍候…</span>
                      </div>
                    ) : allDone ? (
                      <div className="unit-status-banner">
                        <CheckCircle2 size={13} />
                        <span>{shots.length} 个镜头视频已全部生成完成</span>
                      </div>
                    ) : (
                      <div className="unit-status-banner">
                        <CheckCircle2 size={13} />
                        <span>已规划为 {shots.length} 个视频生成任务，请检查后生成视频</span>
                      </div>
                    )
                  )}
                  <div className="unit-actions" onClick={(e) => e.stopPropagation()}>
                    <span className="unit-duration-label">时长</span>
                    <span className="unit-duration-value">{beat.preset_duration_sec}秒</span>
                    <button className="icon-btn icon-btn-sm" onClick={() => toast.info("更多操作即将上线")}>
                      <MoreHorizontal size={16} />
                    </button>
                    {expanded ? (
                      <>
                        <div className="unit-history" onClick={() => void openHistory(beat)}>
                          <History size={12} />
                          <span>历史版本</span>
                        </div>
                        <button className="btn-replan" onClick={() => setReplanTarget(beat)}>
                          <RefreshCw size={12} />
                          <span>重新规划</span>
                        </button>
                        <button
                          className="btn-generate-video"
                          disabled={running || submitting}
                          onClick={() => setGenTarget(beat)}
                        >
                          <Clapperboard size={14} />
                          <span>{allDone ? "重新生成" : "生成视频"}</span>
                        </button>
                      </>
                    ) : (
                      <button className="btn-gen-prompts" onClick={() => toggleExpand(beat.beat_id)}>
                        <Sparkles size={13} />
                        <span>展开镜头</span>
                        <ChevronDown size={12} />
                      </button>
                    )}
                  </div>
                </div>

                {expanded ? (
                  <>
                    {/* 剧本原文 toggle */}
                    <div className="unit-subheader">
                      <span className="unit-sub-label">剧本原文</span>
                      <div
                        className="unit-toggle"
                        onClick={() => setScriptOpen((prev) => ({ ...prev, [beat.beat_id]: !showScript }))}
                      >
                        <ChevronDown size={12} style={{ transform: showScript ? "rotate(180deg)" : undefined, transition: "transform 0.2s" }} />
                        <span>{showScript ? "收起" : "展开"}</span>
                      </div>
                    </div>
                    {showScript && <div className="script-preview">{beat.script_text}</div>}
                    {/* 镜头网格 */}
                    {shots.length > 0 ? (
                      <div className="shots-grid">
                        {shots.map((s) => (
                          <ShotCard key={s.shot_uuid} shot={s} detail={detailOf(beat.beat_id, s.sort_order)} />
                        ))}
                      </div>
                    ) : (
                      <div className="px-5 pb-5 text-xs text-[#6B7088]">该单元暂无镜头数据，可尝试重新规划。</div>
                    )}
                  </>
                ) : (
                  <>
                    <div className="script-preview-collapsed">{beat.script_text}</div>
                    {/* 关联资产占位（头像渐变位，真实关联数据待后续接口） */}
                    <div className="assets-row">
                      <span className="assets-label">关联资产</span>
                      <div className="assets-avatars">
                        <div className="asset-avatar asset-avatar-1" />
                        <div className="asset-avatar asset-avatar-2" />
                        <div className="asset-avatar asset-avatar-3" />
                        <div className="asset-more">+{beat.shot_group_count > 3 ? beat.shot_group_count - 3 : 0}</div>
                      </div>
                    </div>
                  </>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Bottom Floating Bar */}
      <div className="bottom-bar">
        <button className="btn-agent-config" onClick={() => toast.info("Agent 技能配置将在后续版本开放")}>
          <Settings2 size={16} />
          <span>Agent 技能配置</span>
        </button>
        <button
          className="btn-gen-video-float"
          onClick={() => {
            const first = beats.find((b) => !(shotsMap[b.beat_id] ?? []).every((s) => s.video_generation_status === "SUCCESS"));
            if (first) setGenTarget(first);
            else if (beats.length > 0) setGenTarget(beats[0]);
            else toast.info("暂无可生成的叙事单元");
          }}
        >
          <span>生成视频</span>
          <span className="gen-video-count">({pendingCount})</span>
          <ChevronDown size={16} />
        </button>
      </div>

      {/* 生成视频弹窗 */}
      {genTarget && (
        <GenVideoModal
          beat={genTarget}
          shots={shotsMap[genTarget.beat_id] ?? []}
          submitting={submitting}
          onClose={() => !submitting && setGenTarget(null)}
          onConfirm={() => void handleGenerate()}
        />
      )}

      {/* 历史版本弹窗（S2-B05，DOM 对齐设计稿 modalHistory） */}
      {historyTarget && (
        <div className={`history-modal-overlay ${historyActive ? "active" : ""}`} onClick={() => setHistoryTarget(null)}>
          <div className="modal-box" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <div className="modal-title">历史版本</div>
            </div>
            <div className="modal-body">
              {versionsLoading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 size={20} className="animate-spin text-[#4C7FF8]" />
                </div>
              ) : versions.length === 0 ? (
                <div className="text-xs text-[#6B7088] text-center py-8">暂无版本记录</div>
              ) : (
                <div className="version-list">
                  {versions.map((v, i) => (
                    <div key={v.version_id} className="version-row">
                      <div className={`version-dot ${i === 0 ? "current" : ""}`} />
                      <div className="version-info">
                        <div className="version-name">
                          v{v.version_num}
                          {i === 0 && <span className="version-badge">当前版本</span>}
                        </div>
                        <div className="version-meta">
                          <span>{fmtVersionTime(v.created_at)}</span>
                          {v.label && <span>{v.label}</span>}
                        </div>
                      </div>
                      <button
                        className="btn-restore"
                        disabled={restoringId === v.version_id}
                        onClick={() => void restoreVersion(v)}
                      >
                        {restoringId === v.version_id ? "恢复中…" : "恢复"}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className="modal-footer">
              <button className="btn-modal-cancel" onClick={() => setHistoryTarget(null)}>关闭</button>
            </div>
          </div>
        </div>
      )}

      {/* 重新规划确认弹窗（3.3.7c，DOM 对齐设计稿 modalReplan） */}
      {replanTarget && (
        <div
          className={`replan-modal-overlay ${replanActive ? "active" : ""}`}
          onClick={() => !replanSubmitting && setReplanTarget(null)}
        >
          <div className="modal-box" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <div className="modal-title">重新规划分镜</div>
            </div>
            <div className="modal-body">
              <div className="confirm-message">重新规划将覆盖当前分镜方案，确定要继续吗？</div>
              <div className="setting-row">
                <div className="setting-label-group">
                  <span className="setting-label">保留已生成的参考图</span>
                </div>
                <div className={`toggle-switch ${replanKeepRefs ? "on" : ""}`} onClick={() => setReplanKeepRefs(!replanKeepRefs)} />
              </div>
              <div className="setting-row">
                <div className="setting-label-group">
                  <span className="setting-label">应用新的Agent技能</span>
                </div>
                <div className={`toggle-switch ${replanApplySkill ? "on" : ""}`} onClick={() => setReplanApplySkill(!replanApplySkill)} />
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn-modal-cancel" onClick={() => !replanSubmitting && setReplanTarget(null)}>取消</button>
              <button className="btn-modal-warning" disabled={replanSubmitting} onClick={() => void confirmReplan()}>
                {replanSubmitting ? "提交中…" : "重新规划"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 未解锁提示（直接访问未达状态的 step 时温和提示） */}
      {proj && !stepState.canClick(3) && (
        <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="bg-[#1A1A2E] border border-[#2A2A3E] rounded-2xl p-6 w-[360px] text-center">
            <X size={20} className="mx-auto mb-3 text-[#6B7088]" />
            <div className="text-white text-sm font-medium mb-2">视频制作尚未解锁</div>
            <div className="text-xs text-[#6B7088] mb-4">请先在 Step2 完成全部资产生成</div>
            <button
              className="btn-modal-primary w-full justify-center"
              onClick={() => stepState.goStep(2)}
            >
              返回资产生成
            </button>
          </div>
        </div>
      )}
    </WorkbenchShell>
  );
}
