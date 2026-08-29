// S1-F03 Step1 剧本拆分页 —— 按 luxreal-clone 设计稿 workbench-script.html 融合（批次 2 工作台基准页）
// 布局：WorkbenchShell（AppShell + 剧集侧栏 + WorkflowPill 顶栏），内容区 65% 宽
// 接口：GET /projects/:id、POST /projects/:id/split-script（force 重拆 + episode_duration_min + split_skill_code）、
// GET /projects/:id/episodes、GET /episodes/:id/beats、GET /skills?kind=SCRIPT_SPLIT（拆分技能库）
import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  AlertTriangle, ArrowRight, ChevronDown, ChevronUp, GripVertical, Loader2, Pencil, Plus, RefreshCw, ScrollText, Settings, Trash2, Wand2,
} from "lucide-react";

import {
  ApiError,
  api,
  errorMessage,
  projectsApi,
  splitApi,
  ProjectStatusMeta,
  type BeatSummary,
  type EpisodeLite,
  type ProjectDetail,
  type SplitSkill,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import AgentSkillModal from "../components/AgentSkillModal";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import { matchTaskTypes, useWsDrivenPoll } from "../hooks/useWsDrivenPoll";
import { wsClient } from "../hooks/useWebSocket";
import "../layouts/shell.css";

// 单元状态 → 徽标（后端无专用枚举，按通用规则映射）
function beatStatusBadge(status: string): { label: string; cls: string; running?: boolean } {
  const s = (status || "").toUpperCase();
  if (s.includes("FAIL")) return { label: "解析失败", cls: "unit-status-running", running: true };
  if (s.includes("ING") || s.includes("GENERAT") || s.includes("RUNNING")) return { label: "解析中", cls: "unit-status-running", running: true };
  return { label: "解析完成", cls: "unit-status-done" };
}

const DURATION_OPTIONS = ["自动", "5秒", "10秒", "15秒", "20秒", "自定义"];

// 视频模型 key → 显示名（首页快照可能存首页 key（seedance25/MiniMax-H3 等）或 Step3 回写的弹窗 key（2.5 等））
const VIDEO_MODEL_DISPLAY: Record<string, string> = {
  seedance25: "Seedance 2.5",
  "2.5": "Seedance 2.5",
  seedance20: "Seedance 2.0",
  "2.0": "Seedance 2.0",
  seedance20fast: "Seedance 2.0 Fast",
  seedance15: "Seedance 1.5",
  "1.5": "Seedance 1.5",
  klingo3: "KlingO3",
  "MiniMax-H3": "MiniMax H3",
};
// 后端分辨率枚举 → 显示值（快照可能存 P720 等枚举）
const displayRes = (v: string) => (v === "P480" ? "480P" : v === "P720" ? "720P" : v === "P1080" ? "1080P" : v);

// 台词说话人着色（按首次出现顺序分配，对齐 luxreal 设计稿剧本原文区台词配色）
const SPEAKER_COLORS = ["#4C7FF8", "#8B5CF6", "#10B981", "#F59E0B", "#EC4899", "#22D3EE"];

// renderScriptText 剧本原文分行渲染：台词行（“角色：内容”）speaker 着色，其余按段落展示。
function renderScriptText(text: string) {
  const lines = text.split(/\n+/).map((l) => l.trim()).filter(Boolean);
  const colorMap = new Map<string, string>();
  return lines.map((line, i) => {
    const m = line.match(/^([^\s:：,，!！?？]{1,10})[:：]\s*(.+)$/);
    if (m) {
      if (!colorMap.has(m[1])) colorMap.set(m[1], SPEAKER_COLORS[colorMap.size % SPEAKER_COLORS.length]);
      return (
        <p key={i} className="flex items-start gap-2">
          <span className="font-medium flex-shrink-0" style={{ color: colorMap.get(m[1]) }}>{m[1]}：</span>
          <span>{m[2]}</span>
        </p>
      );
    }
    return <p key={i}>{line}</p>;
  });
}

// 错误文案：ApiError 走错误码映射（05 C 章），避免弹出后端原始报文
function errMsg(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return errorMessage(e.code, e.message);
  return e instanceof Error ? e.message : fallback;
}

export default function Step1Page() {
  const { projectId = "" } = useParams();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [scriptInput, setScriptInput] = useState("");
  const [splitting, setSplitting] = useState(false);
  const [loadError, setLoadError] = useState(false);

  // 弹窗/下拉状态
  const [splitModalOpen, setSplitModalOpen] = useState(false);
  const [agentSkillOpen, setAgentSkillOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<BeatSummary | null>(null);
  const [addUnitOpen, setAddUnitOpen] = useState(false);
  // F4: 叙事单元编辑弹窗
  const [editTarget, setEditTarget] = useState<BeatSummary | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const [editScript, setEditScript] = useState("");
  const [editDuration, setEditDuration] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [durationDrop, setDurationDrop] = useState<{ beatId: string; left: number; top: number } | null>(null);
  const [durationMap, setDurationMap] = useState<Record<string, string>>({});
  // 拆分设置（自动拆分弹窗，视觉态；后端暂未接收参数）
  const [splitByScene, setSplitByScene] = useState(true);
  const [splitByDialogue, setSplitByDialogue] = useState(true);
  const [splitPrompt, setSplitPrompt] = useState(false);
  const [epDuration, setEpDuration] = useState(2);
  // 拆分技能（全局 agent_skills 库；默认选中 sort_order 最小的标准拆分）
  const [splitSkills, setSplitSkills] = useState<SplitSkill[]>([]);
  const [splitSkillCode, setSplitSkillCode] = useState("");

  const stepState = useStepState(projectId, proj?.status ?? null);
  const meta = proj ? ProjectStatusMeta[proj.status] : null;

  // 加载项目 + 剧集
  const loadProject = useCallback(async () => {
    try {
      const p = await projectsApi.detail(projectId);
      setLoadError(false);
      setProj(p);
      if (p.source_script_text) setScriptInput(p.source_script_text);
      const eps = await splitApi.episodes(projectId);
      setEpisodes(eps.items ?? []);
      if (eps.items.length > 0 && !activeEpisode) {
        setActiveEpisode(eps.items[0].episode_id);
      }
    } catch (e) {
      setLoadError(true);
      toast.error(errMsg(e, "加载失败"));
    }
  }, [projectId, activeEpisode]);

  // 加载单元列表
  const loadBeats = useCallback(async () => {
    if (!activeEpisode) return;
    try {
      const data = await splitApi.beats(activeEpisode);
      setBeats(data.items ?? []);
    } catch (e) {
      toast.error(errMsg(e, "加载叙事单元失败"));
    }
  }, [activeEpisode]);

  // F4: 打开编辑弹窗
  const openEditUnit = (b: BeatSummary) => {
    setEditTarget(b);
    setEditTitle(b.title);
    setEditScript(b.script_text || "");
    setEditDuration(b.preset_duration_sec > 0 ? String(b.preset_duration_sec) : "");
  };

  // F4: 保存编辑（PATCH /narrative-units/:nu_id）
  const saveEditUnit = async () => {
    if (!editTarget) return;
    if (!editTitle.trim()) {
      toast.error("标题不能为空");
      return;
    }
    setSavingEdit(true);
    try {
      await api.patch(`/narrative-units/${editTarget.beat_id}`, {
        title: editTitle.trim(),
        content: editScript.trim(),
        script_original_text: editScript.trim(),
        ...(editDuration ? { preset_duration_sec: Number(editDuration) } : {}),
      });
      toast.success("叙事单元已更新");
      setEditTarget(null);
      void loadBeats();
    } catch (e) {
      toast.error(errMsg(e, "更新失败"));
    } finally {
      setSavingEdit(false);
    }
  };

  // F4: 确认删除（DELETE /narrative-units/:nu_id）
  const confirmDeleteUnit = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.delete(`/narrative-units/${deleteTarget.beat_id}`);
      toast.success(`已删除「${deleteTarget.title}」`);
      setDeleteTarget(null);
      void loadBeats();
    } catch (e) {
      toast.error(errMsg(e, "删除失败"));
    } finally {
      setDeleting(false);
    }
  };

  // 拆分中状态刷新：WS 终态推送驱动（SCRIPT_SPLIT 任务），断线回退 5s 轮询（对齐 LuxReal 推送为主）
  const splitDoneRef = useRef(false);
  const refreshSplit = useCallback(async () => {
    if (splitDoneRef.current) return;
    try {
      const p = await projectsApi.detail(projectId);
      setProj(p);
      if (p.status !== "SCRIPT_SPLITTING") {
        splitDoneRef.current = true;
        if (p.status === "SCRIPT_SPLIT_FAILED") {
          toast.error("剧本拆解失败，请修改剧本后重试");
        } else {
          toast.success("剧本拆解完成");
          const eps = await splitApi.episodes(projectId);
          setEpisodes(eps.items ?? []);
          if (eps.items.length > 0) setActiveEpisode(eps.items[0].episode_id);
        }
      }
    } catch {
      /* 刷新失败忽略 */
    }
  }, [projectId]);

  useWsDrivenPoll(refreshSplit, {
    active: proj?.status === "SCRIPT_SPLITTING",
    pollMs: 5000,
    match: matchTaskTypes(["SCRIPT_SPLIT"]),
  });

  // 再次触发拆分时重置终态守卫
  useEffect(() => {
    if (proj?.status === "SCRIPT_SPLITTING") splitDoneRef.current = false;
  }, [proj?.status]);

  // 初始加载 + 单元列表联动
  useEffect(() => {
    void loadProject();
    // 拆分技能库（失败静默：弹窗回落无技能可选，后端默认标准拆分）
    splitApi.splitSkills()
      .then((d) => {
        const items = d.items ?? [];
        setSplitSkills(items);
        if (items.length > 0) setSplitSkillCode(items[0].skill_code);
      })
      .catch(() => { /* 技能库不可用不阻断拆分 */ });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  useEffect(() => {
    void loadBeats();
  }, [loadBeats]);

  // 点击下拉外关闭
  useEffect(() => {
    if (!durationDrop) return;
    const onDocClick = () => setDurationDrop(null);
    document.addEventListener("click", onDocClick);
    return () => document.removeEventListener("click", onDocClick);
  }, [durationDrop]);

  // 触发拆分（force=true 为重新拆分：覆盖已有剧集/单元/镜头结果）
  const handleSplit = async (force: boolean) => {
    if (!scriptInput.trim()) {
      toast.error("请先输入剧本原文");
      return;
    }
    try {
      setSplitting(true);
      await splitApi.triggerSplit(projectId, force, scriptInput, epDuration, splitSkillCode || undefined);
      toast.success(force ? "重新拆分任务已提交，旧结果将被覆盖…" : "拆分任务已提交，AI 处理中…");
      setSplitModalOpen(false);
      setBeats([]);           // 清空旧单元展示（重拆后 UUID 全变）
      setDurationMap({});
      setProj({ ...(proj as ProjectDetail), status: "SCRIPT_SPLITTING" });
    } catch (e) {
      toast.error(errMsg(e, "触发拆分失败"));
    } finally {
      setSplitting(false);
    }
  };

  const splittingNow = proj?.status === "SCRIPT_SPLITTING";

  // 拆分等待进度：优先消费后端真实上报（Worker 阶段化文案：读剧本→逐集镜头设计→
  // 资产扩写→润色→入库，WS TASK_PROGRESS 推送 + 断线时项目轮询同步）；无真实进度时
  // 回退确定性缓动动画。拆分结束（splittingNow 变 false）组件卸载。
  const [splitPct, setSplitPct] = useState(5);
  const [splitPhase, setSplitPhase] = useState("读取剧本原文");
  const [realProgress, setRealProgress] = useState(false); // 收到过后端真实进度标记
  useEffect(() => {
    if (!splittingNow) return;
    setRealProgress(false);
    setSplitPct(5);
    setSplitPhase("读取剧本原文");
    let cur = 5;
    const timer = setInterval(() => {
      if (realProgressRef.current) return; // 真实进度接管后停用假动画
      const delta = Math.max(0.3, (95 - cur) * 0.04);
      cur = Math.min(95, cur + delta);
      setSplitPct(Math.round(cur));
    }, 400);
    return () => clearInterval(timer);
  }, [splittingNow]);
  const realProgressRef = useRef(realProgress);
  realProgressRef.current = realProgress;

  // WS 实时进度（TASK_PROGRESS）：拆分任务推送 → 更新百分比与阶段文案
  useEffect(() => {
    return wsClient.addMessageListener((msg) => {
      if (msg.type !== "TASK_PROGRESS") return;
      const tt = msg.data?.task_type;
      if (tt && tt !== "SCRIPT_SPLIT") return;
      const pct = Number(msg.data?.progress_pct);
      const message = typeof msg.data?.progress_message === "string" ? msg.data.progress_message : "";
      if (!Number.isFinite(pct) || pct <= 0) return;
      setRealProgress(true);
      setSplitPct(Math.min(99, Math.round(pct)));
      if (message) setSplitPhase(message);
    });
  }, []);
  const isDraft = proj?.status === "DRAFT";
  const splitFailed = proj?.status === "SCRIPT_SPLIT_FAILED";
  // 可重新拆分阶段：拆分成功后的资产阶段（后端状态机限定；视频阶段后不可重拆）
  const canResplit = proj?.status === "PARTIAL_ASSETS" || proj?.status === "ALL_ASSETS_READY";

  // 项目配置快照 → 设置条显示值（首页所选模型/分辨率/画幅/风格贯穿全流程）
  const cfg = (proj?.config ?? {}) as Record<string, unknown>;
  const cfgStr = (k: string) => (typeof cfg[k] === "string" ? (cfg[k] as string) : "");
  const cfgVideoModel = VIDEO_MODEL_DISPLAY[cfgStr("video_model")] || cfgStr("video_model") || "Seedance 2.5";
  const cfgResolution = displayRes(cfgStr("resolution")) || "1080P";
  const cfgRatio = cfgStr("aspect_ratio") || "9:16";
  const cfgStyleName = cfgStr("style_name") || "默认风格";
  const cfgDuration = cfgStr("video_duration") === "auto" || !cfgStr("video_duration") ? "自动时长" : cfgStr("video_duration");

  // 项目加载失败（404/无权限/后端异常）整页降级，避免无限“加载中…”
  if (loadError) return <ProjectLoadError />;

  // 时长徽标点击：定位下拉
  const openDurationDrop = (e: React.MouseEvent<HTMLButtonElement>, beatId: string) => {
    e.stopPropagation();
    if (durationDrop?.beatId === beatId) {
      setDurationDrop(null);
      return;
    }
    const rect = e.currentTarget.getBoundingClientRect();
    setDurationDrop({ beatId, left: rect.left, top: rect.bottom + 4 });
  };

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
      {/* Duration Preset Row */}
      <div className="flex justify-end px-6 pb-2 flex-shrink-0">
        <div className="flex items-center">
          <span className="text-xs" style={{ color: "#6B7088" }}>本集预设总时长</span>
          <span className="text-xs px-2 py-0.5 rounded ml-2" style={{ color: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }}>自动</span>
        </div>
      </div>

      {/* Content Scroll Area */}
      <div className="flex-1 overflow-y-auto px-6 pb-24 scrollbar-thin">
        <div className="flex flex-col gap-3 mx-auto w-full" style={{ maxWidth: "65%" }}>

          {/* Project Settings Bar */}
          <div className="settings-bar flex items-center justify-between h-9 px-4 rounded-xl">
            <div className="flex items-center gap-2 min-w-0">
              <span className="text-white font-medium text-sm truncate">{proj?.title ?? "加载中…"}</span>
              {meta && (
                <span className="text-[10px] px-1.5 py-0.5 rounded flex-shrink-0 unit-status-done">{meta.label}</span>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button
                className="flex items-center gap-1 text-xs transition-colors hover:text-white"
                style={{ color: "#6B7088" }}
                onClick={() => setAgentSkillOpen(true)}
              >
                <Settings size={14} />
                Agent技能
                <span className="underline">修改</span>
              </button>
              <button
                className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white"
                onClick={() => toast.info("模型配置请从创作首页进入")}
              >
                {cfgVideoModel}
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("分辨率切换将在后续版本开放")}>
                {cfgResolution}
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("画幅切换将在后续版本开放")}>
                {cfgRatio}
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("风格切换将在后续版本开放")}>
                {cfgStyleName}
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("时长策略将在后续版本开放")}>
                {cfgDuration}
                <ChevronDown size={12} />
              </button>
            </div>
          </div>

          {/* 剧本原文（DRAFT/拆分失败/可重拆阶段可编辑；尚未拆出剧集/单元时也展示输入区） */}
          {(isDraft || splitFailed || canResplit || (episodes.length === 0 && beats.length === 0)) && proj && !splittingNow && (
            <div className="rounded-2xl overflow-hidden unit-card" style={{ animationDelay: "0ms" }}>
              <div className="flex items-center justify-between px-5 py-3.5">
                <div className="flex items-center gap-2">
                  <ScrollText size={16} style={{ color: "#4C7FF8" }} />
                  <span className="text-white font-medium text-sm">剧本原文</span>
                  <span className="text-xs" style={{ color: "#6B7088" }}>{scriptInput.length} 字</span>
                </div>
                {canResplit && !splittingNow ? (
                  <button
                    onClick={() => setSplitModalOpen(true)}
                    className="text-xs flex items-center gap-1.5 px-3 py-1.5 rounded-lg transition-colors hover:bg-[#4C7FF8]/20"
                    style={{ color: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }}
                  >
                    <RefreshCw size={13} />
                    重新拆分
                  </button>
                ) : !splittingNow ? (
                  <button
                    onClick={() => void handleSplit(false)}
                    disabled={splitting}
                    className="text-xs flex items-center gap-1.5 px-3 py-1.5 rounded-lg transition-colors hover:bg-[#4C7FF8]/20 disabled:opacity-50"
                    style={{ color: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }}
                  >
                    <Wand2 size={14} />
                    {splitting ? "提交中…" : "开始 AI 拆解"}
                  </button>
                ) : null}
              </div>
              <div className="px-5 pb-4">
                <textarea
                  value={scriptInput}
                  onChange={(e) => setScriptInput(e.target.value)}
                  rows={8}
                  placeholder="粘贴你的剧本，AI 将拆解为剧集、叙事单元与分镜脚本…"
                  className="wb-textarea resize-none"
                  style={{ minHeight: 160 }}
                />
              </div>
            </div>
          )}

          {/* 拆分中状态卡：动态百分比进度条（SCRIPT_SPLIT 为 LLM 单次调用无真实进度，
              采用确定性缓动动画：5% → 95% 逼近，完成跳 100%；阶段文案随进度轮换） */}
          {splittingNow && (
            <div className="unit-card rounded-2xl overflow-hidden">
              <div className="px-5 py-4">
                <div className="flex items-center justify-between text-xs" style={{ color: "#4C7FF8" }}>
                  <div className="flex items-center gap-2">
                    <Loader2 size={14} className="animate-spin" />
                    正在拆解剧本《{proj?.title ?? ""}》 · {splitPhase}…
                  </div>
                  <span className="tabular-nums" style={{ color: "#8B8B9A" }}>{splitPct}%</span>
                </div>
                <div className="asset-progress" style={{ marginTop: 12 }}>
                  <div
                    className="asset-progress-bar"
                    style={{ width: `${splitPct}%`, right: "auto", animation: "none", transition: "width 0.35s cubic-bezier(0.4, 0, 0.2, 1)" }}
                  />
                </div>
                <div className="mt-4 space-y-2">
                  {[0, 1, 2].map((i) => (
                    <div key={i} className="h-12 animate-pulse rounded-lg" style={{ background: "#16161C", animationDelay: `${i * 150}ms` }} />
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* 叙事单元卡片列表（入场错峰动画：animation-delay 按序递增） */}
          {!splittingNow && beats.map((b, i) => {
            const badge = beatStatusBadge(b.status);
            const dur = durationMap[b.beat_id] ?? (b.preset_duration_sec > 0 ? `${b.preset_duration_sec}秒` : "自动");
            return (
              <div key={b.beat_id} id={`unit-card-${b.beat_id}`} className="unit-card rounded-2xl overflow-hidden transition-all" style={{ animationDelay: `${i * 70}ms` }}>
                <div className="flex items-center justify-between px-5 py-3.5">
                  <div className="flex items-center min-w-0">
                    <GripVertical size={16} className="mr-2 cursor-grab flex-shrink-0" style={{ color: "#4A4A5A" }} />
                    <span className="text-sm flex-shrink-0" style={{ color: "#8B8B9A" }}>叙事单元{b.sort_order}</span>
                    <span className="mx-2 flex-shrink-0" style={{ color: "#4A4A5A" }}>|</span>
                    <span className="text-white font-medium text-base truncate">{b.title}</span>
                    <span className={`ml-2 text-[10px] px-1.5 py-0.5 rounded flex-shrink-0 ${badge.cls} ${badge.running ? "inline-flex items-center gap-1" : ""}`}>
                      {badge.running && <span className="w-1.5 h-1.5 rounded-full animate-pulse" style={{ background: "currentColor" }} />}
                      {badge.label}
                    </span>
                  </div>
                  <div className="flex items-center gap-3 flex-shrink-0">
                    <button className="w-7 h-7 rounded flex items-center justify-center text-[#6B7088] hover:text-white hover:bg-white/5 transition-colors" onClick={() => toast.info("单元排序将在后续版本开放")}>
                      <ChevronUp size={16} />
                    </button>
                    <button className="w-7 h-7 rounded flex items-center justify-center text-[#6B7088] hover:text-white hover:bg-white/5 transition-colors" onClick={() => toast.info("单元排序将在后续版本开放")}>
                      <ChevronDown size={16} />
                    </button>
                    <div className="w-px h-4" style={{ background: "#2A2A35" }} />
                    <span className="text-xs" style={{ color: "#6B7088" }}>时长预设</span>
                    <button
                      className="duration-badge text-xs px-2 py-0.5 rounded"
                      style={{ color: dur === "自动" ? "#6B7088" : "#4C7FF8", border: "1px solid #3A3A4A" }}
                      onClick={(e) => openDurationDrop(e, b.beat_id)}
                    >
                      {dur}
                    </button>
                    <div className="w-px h-4" style={{ background: "#2A2A35" }} />
                    <button
                      className="text-xs flex items-center gap-1 cursor-pointer transition-colors hover:!text-red-400"
                      style={{ color: "#6B7088" }}
                      onClick={() => setDeleteTarget(b)}
                    >
                      <Trash2 size={14} />
                      删除
                    </button>
                  </div>
                </div>
                {/* 剧本原文 */}
                <div className="px-5 pb-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs" style={{ color: "#6B7088" }}>剧本原文</span>
                    <div className="flex items-center gap-2">
                      <span className="text-[10px]" style={{ color: "#4A4A5A" }}>{b.shot_group_count} 个镜头块</span>
                      <button className="w-6 h-6 rounded flex items-center justify-center transition-colors hover:bg-white/5" style={{ color: "#6B7088" }} onClick={() => openEditUnit(b)}>
                        <Pencil size={14} />
                      </button>
                    </div>
                  </div>
                  <div className="unit-script-box rounded-lg p-3 text-sm leading-relaxed max-h-[280px] overflow-y-auto scrollbar-thin">
                    {b.script_text ? renderScriptText(b.script_text) : "（无剧本原文）"}
                  </div>
                </div>
              </div>
            );
          })}

          {/* 完成态空提示（有剧集但无单元） */}
          {!splittingNow && proj && !isDraft && episodes.length > 0 && beats.length === 0 && (
            <div className="rounded-2xl p-10 text-center text-sm" style={{ border: "1px dashed #2A2A35", color: "#6B7088" }}>
              该剧集暂无叙事单元数据
            </div>
          )}

          {/* Add Narrative Unit Button */}
          {!splittingNow && beats.length > 0 && (
            <button
              id="addUnitBtn"
              className="w-full h-11 rounded-xl border border-dashed flex items-center justify-center gap-1.5 text-sm transition-colors hover:border-[#4C7FF8] hover:text-[#4C7FF8]"
              style={{ borderColor: "#2A2A35", color: "#6B7088" }}
              onClick={() => setAddUnitOpen(true)}
            >
              <Plus size={16} />
              新增叙事单元
            </button>
          )}

          {/* Next Step CTA Button */}
          {!splittingNow && beats.length > 0 && (
            <a
              className="w-full h-11 rounded-xl flex items-center justify-center gap-2 text-white font-medium transition-all hover:opacity-90 mt-4 cursor-pointer"
              style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
              onClick={() => stepState.canClick(2) ? stepState.goStep(2) : toast.info("请先完成剧本拆分")}
            >
              下一步：资产生成
              <ArrowRight size={16} />
            </a>
          )}

        </div>
      </div>

      {/* Duration Preset Dropdown */}
      {durationDrop && (
        <div className="duration-dropdown" style={{ left: durationDrop.left, top: durationDrop.top }}>
          {DURATION_OPTIONS.map((v) => {
            const current = durationMap[durationDrop.beatId] ?? "自动";
            return (
              <button
                key={v}
                style={{ color: v === current ? "#4C7FF8" : "#C0C0CC" }}
                onClick={(e) => {
                  e.stopPropagation();
                  setDurationMap((m) => ({ ...m, [durationDrop.beatId]: v }));
                  setDurationDrop(null);
                }}
              >
                {v}
              </button>
            );
          })}
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deleteTarget && (
        <div className="modal-overlay">
          <div className="modal-backdrop" onClick={() => setDeleteTarget(null)} />
          <div className="modal-content" style={{ width: 360 }}>
            <h3 className="text-white text-lg font-medium mb-2">删除叙事单元</h3>
            <p className="text-sm mb-6" style={{ color: "#8B8B9A" }}>确定要删除「{deleteTarget.title}」吗？删除后无法恢复。</p>
            <div className="flex gap-3 justify-end">
              <button className="px-4 py-2 text-sm rounded-lg transition-colors hover:bg-white/5" style={{ color: "#8B8B9A" }} onClick={() => setDeleteTarget(null)}>取消</button>
              <button
                className="px-4 py-2 text-sm rounded-lg text-white transition-colors hover:opacity-90"
                style={{ background: "#EF4444" }}
                onClick={() => {
                  void confirmDeleteUnit();
                }}
                disabled={deleting}
              >
                {deleting ? "删除中…" : "删除"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* F4: 叙事单元编辑弹窗（PATCH /narrative-units/:nu_id） */}
      {editTarget && (
        <div className="modal-overlay">
          <div className="modal-backdrop" onClick={() => setEditTarget(null)} />
          <div className="modal-content" style={{ width: 560 }}>
            <h3 className="text-white text-lg font-medium mb-1">编辑叙事单元</h3>
            <p className="text-xs mb-4" style={{ color: "#8B8B9A" }}>修改后自动保存版本快照，可在「历史版本」中回滚</p>
            <div className="space-y-4">
              <div>
                <label className="block text-xs mb-1.5" style={{ color: "#8B8B9A" }}>单元标题</label>
                <input
                  type="text"
                  value={editTitle}
                  onChange={(e) => setEditTitle(e.target.value)}
                  maxLength={256}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs mb-1.5" style={{ color: "#8B8B9A" }}>剧本原文</label>
                <textarea
                  value={editScript}
                  onChange={(e) => setEditScript(e.target.value)}
                  rows={6}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white leading-relaxed resize-y focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div className="w-40">
                <label className="block text-xs mb-1.5" style={{ color: "#8B8B9A" }}>时长预设（秒）</label>
                <input
                  type="number"
                  min={1}
                  max={600}
                  value={editDuration}
                  onChange={(e) => setEditDuration(e.target.value)}
                  placeholder="自动"
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
            </div>
            <div className="flex gap-3 justify-end mt-6">
              <button className="px-4 py-2 text-sm rounded-lg transition-colors hover:bg-white/5" style={{ color: "#8B8B9A" }} onClick={() => setEditTarget(null)}>取消</button>
              <button
                className="px-4 py-2 text-sm rounded-lg text-white transition-colors"
                style={{ background: "linear-gradient(135deg, #4C7FF8, #3B6CE0)" }}
                onClick={() => void saveEditUnit()}
                disabled={savingEdit}
              >
                {savingEdit ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Auto Split Modal（重新拆分设置：覆盖警示 + 拆分参数生效透传） */}
      {splitModalOpen && (
        <div className="modal-overlay">
          <div className="modal-backdrop" onClick={() => setSplitModalOpen(false)} />
          <div className="modal-content" style={{ width: 420 }}>
            <h3 className="text-white text-lg font-medium mb-1">{canResplit ? "AI 重新拆分" : "AI 自动拆分设置"}</h3>
            <p className="text-sm mb-4" style={{ color: "#8B8B9A" }}>{canResplit ? "调整拆分参数后重新拆解剧本" : "配置剧本自动拆分的规则和参数"}</p>

            {canResplit && (
              <div className="flex items-start gap-2 rounded-lg px-3 py-2.5 mb-4 text-xs leading-relaxed" style={{ background: "rgba(245, 158, 11, 0.08)", border: "1px solid rgba(245, 158, 11, 0.25)", color: "#F59E0B" }}>
                <AlertTriangle size={14} className="flex-shrink-0 mt-0.5" />
                <span>重新拆分会清空现有剧集、叙事单元与镜头结果并重新生成，此操作不可恢复。</span>
              </div>
            )}

            <div className="mb-4">
              <label className="text-sm text-white mb-2 block">拆分技能（全局技能库，管理端维护）</label>
              <div className="wb-select-wrapper">
                <select className="wb-select" value={splitSkillCode} onChange={(e) => setSplitSkillCode(e.target.value)}>
                  {splitSkills.map((s) => (
                    <option key={s.skill_code} value={s.skill_code}>{s.name}</option>
                  ))}
                </select>
                <ChevronDown size={16} />
              </div>
              {(() => {
                const cur = splitSkills.find((s) => s.skill_code === splitSkillCode);
                return cur?.description ? (
                  <p className="text-xs mt-2 leading-relaxed" style={{ color: "#6B7088" }}>{cur.description}</p>
                ) : null;
              })()}
            </div>

            <div className="space-y-3 mb-5">
              <div className="flex items-center justify-between">
                <span className="text-sm text-white">按场景拆分</span>
                <div className={`toggle ${splitByScene ? "active" : ""}`} onClick={() => setSplitByScene((v) => !v)}>
                  <div className="toggle-knob" />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-white">按角色对话拆分</span>
                <div className={`toggle ${splitByDialogue ? "active" : ""}`} onClick={() => setSplitByDialogue((v) => !v)}>
                  <div className="toggle-knob" />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-white">智能生成分镜提示词</span>
                <div className={`toggle ${splitPrompt ? "active" : ""}`} onClick={() => setSplitPrompt((v) => !v)}>
                  <div className="toggle-knob" />
                </div>
              </div>
            </div>

            <div className="mb-6">
              <label className="text-sm text-white mb-2 block">单集目标时长（影响分集粒度）</label>
              <div className="segmented">
                {[1, 2, 3].map((n) => (
                  <button key={n} className={epDuration === n ? "active" : ""} onClick={() => setEpDuration(n)}>{n}分钟</button>
                ))}
              </div>
            </div>

            <div className="flex gap-3 justify-end">
              <button className="px-4 py-2 text-sm rounded-lg transition-colors hover:bg-white/5" style={{ color: "#8B8B9A" }} onClick={() => setSplitModalOpen(false)}>取消</button>
              <button
                className="px-4 py-2 text-sm rounded-lg text-white transition-all hover:opacity-90 disabled:opacity-50"
                style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                disabled={splitting}
                onClick={() => void handleSplit(true)}
              >
                {splitting ? "提交中…" : canResplit ? "确认重新拆分" : "开始拆分"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Add Unit Modal */}
      {addUnitOpen && (
        <div className="modal-overlay">
          <div className="modal-backdrop" onClick={() => setAddUnitOpen(false)} />
          <div className="modal-content" style={{ width: 420 }}>
            <h3 className="text-white text-lg font-medium mb-5">新增叙事单元</h3>

            <div className="mb-4">
              <label className="text-sm text-white mb-2 block">单元名称</label>
              <input type="text" className="wb-input" placeholder="请输入单元名称" />
            </div>

            <div className="mb-4">
              <label className="text-sm text-white mb-2 block">剧本内容</label>
              <textarea rows={4} className="wb-textarea resize-none" placeholder="请输入剧本内容..." />
            </div>

            <div className="mb-6">
              <label className="text-sm text-white mb-2 block">插入位置</label>
              <div className="wb-select-wrapper">
                <select className="wb-select" defaultValue="当前选中单元之后">
                  <option>当前选中单元之后</option>
                  <option>所有单元之前</option>
                  <option>所有单元之后</option>
                </select>
                <ChevronDown size={16} />
              </div>
            </div>

            <div className="flex gap-3 justify-end">
              <button className="px-4 py-2 text-sm rounded-lg transition-colors hover:bg-white/5" style={{ color: "#8B8B9A" }} onClick={() => setAddUnitOpen(false)}>取消</button>
              <button
                className="px-4 py-2 text-sm rounded-lg text-white transition-all hover:opacity-90"
                style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                onClick={() => {
                  toast.info("手动新增叙事单元将在后续版本开放");
                  setAddUnitOpen(false);
                }}
              >
                添加
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Agent 技能配置弹窗（共享组件，对齐设计稿「Agent技能 修改」入口） */}
      <AgentSkillModal open={agentSkillOpen} onClose={() => setAgentSkillOpen(false)} />
    </WorkbenchShell>
  );
}
