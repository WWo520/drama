// S1-F03 Step1 剧本拆分页 —— 按 luxreal-clone 设计稿 workbench-script.html 融合（批次 2 工作台基准页）
// 布局：WorkbenchShell（AppShell + 剧集侧栏 + WorkflowPill 顶栏），内容区 65% 宽
// 接口：GET /projects/:id、POST /projects/:id/split-script、GET /projects/:id/episodes、GET /episodes/:id/beats
import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ArrowRight, ChevronDown, ChevronUp, GripVertical, Loader2, Pencil, Plus, ScrollText, Settings, Trash2, Wand2,
} from "lucide-react";

import {
  ApiError,
  errorMessage,
  projectsApi,
  splitApi,
  ProjectStatusMeta,
  type BeatSummary,
  type EpisodeLite,
  type ProjectDetail,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import AgentSkillModal from "../components/AgentSkillModal";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import "../layouts/shell.css";

// 单元状态 → 徽标（后端无专用枚举，按通用规则映射）
function beatStatusBadge(status: string): { label: string; cls: string; running?: boolean } {
  const s = (status || "").toUpperCase();
  if (s.includes("FAIL")) return { label: "解析失败", cls: "unit-status-running", running: true };
  if (s.includes("ING") || s.includes("GENERAT") || s.includes("RUNNING")) return { label: "解析中", cls: "unit-status-running", running: true };
  return { label: "解析完成", cls: "unit-status-done" };
}

const DURATION_OPTIONS = ["自动", "5秒", "10秒", "15秒", "20秒", "自定义"];

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
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [loadError, setLoadError] = useState(false);

  // 弹窗/下拉状态
  const [splitModalOpen, setSplitModalOpen] = useState(false);
  const [agentSkillOpen, setAgentSkillOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<BeatSummary | null>(null);
  const [addUnitOpen, setAddUnitOpen] = useState(false);
  const [durationDrop, setDurationDrop] = useState<{ beatId: string; left: number; top: number } | null>(null);
  const [durationMap, setDurationMap] = useState<Record<string, string>>({});
  // 拆分设置（自动拆分弹窗，视觉态；后端暂未接收参数）
  const [splitByScene, setSplitByScene] = useState(true);
  const [splitByDialogue, setSplitByDialogue] = useState(true);
  const [splitPrompt, setSplitPrompt] = useState(false);
  const [epDuration, setEpDuration] = useState(2);

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
          /* 轮询失败忽略 */
        }
      }, 5000);
      return () => {
        if (pollRef.current) clearInterval(pollRef.current);
      };
    }
  }, [proj?.status, projectId]);

  // 初始加载 + 单元列表联动
  useEffect(() => {
    void loadProject();
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

  // 触发拆分
  const handleSplit = async () => {
    if (!scriptInput.trim()) {
      toast.error("请先输入剧本原文");
      return;
    }
    try {
      setSplitting(true);
      await splitApi.triggerSplit(projectId, false, scriptInput);
      toast.success("拆分任务已提交，AI 处理中…");
      setSplitModalOpen(false);
      setProj({ ...(proj as ProjectDetail), status: "SCRIPT_SPLITTING" });
    } catch (e) {
      toast.error(errMsg(e, "触发拆分失败"));
    } finally {
      setSplitting(false);
    }
  };

  const splittingNow = proj?.status === "SCRIPT_SPLITTING";
  const isDraft = proj?.status === "DRAFT";
  const splitFailed = proj?.status === "SCRIPT_SPLIT_FAILED";

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
                Seedance 2.5
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("分辨率切换将在后续版本开放")}>
                1080P
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("画幅切换将在后续版本开放")}>
                9:16
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("风格切换将在后续版本开放")}>
                武侠仙侠
                <ChevronDown size={12} />
              </button>
              <button className="settings-chip flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" onClick={() => toast.info("时长策略将在后续版本开放")}>
                自动时长
                <ChevronDown size={12} />
              </button>
            </div>
          </div>

          {/* 剧本原文（DRAFT/拆分失败可编辑；尚未拆出剧集/单元时也展示输入区） */}
          {(isDraft || splitFailed || (episodes.length === 0 && beats.length === 0)) && proj && !splittingNow && (
            <div className="rounded-2xl overflow-hidden" style={{ background: "#1E1E24", border: "1px solid #2A2A35" }}>
              <div className="flex items-center justify-between px-5 py-3.5">
                <div className="flex items-center gap-2">
                  <ScrollText size={16} style={{ color: "#4C7FF8" }} />
                  <span className="text-white font-medium text-sm">剧本原文</span>
                  <span className="text-xs" style={{ color: "#6B7088" }}>{scriptInput.length} 字</span>
                </div>
                <button
                  onClick={() => void handleSplit()}
                  disabled={splitting}
                  className="text-xs flex items-center gap-1.5 px-3 py-1.5 rounded-lg transition-colors hover:bg-[#4C7FF8]/20 disabled:opacity-50"
                  style={{ color: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }}
                >
                  <Wand2 size={14} />
                  {splitting ? "提交中…" : "开始 AI 拆解"}
                </button>
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

          {/* AI Split Button（已有拆分结果时的重拆入口） */}
          {!isDraft && beats.length > 0 && !splittingNow && (
            <div className="flex justify-end">
              <button
                id="autoSplitBtn"
                className="text-xs flex items-center gap-1.5 px-3 py-1.5 rounded-lg transition-colors hover:bg-[#4C7FF8]/20"
                style={{ color: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }}
                onClick={() => setSplitModalOpen(true)}
              >
                <Wand2 size={14} />
                自动拆分
              </button>
            </div>
          )}

          {/* 拆分中状态卡 */}
          {splittingNow && (
            <div className="unit-card rounded-2xl overflow-hidden">
              <div className="px-5 py-4">
                <div className="flex items-center gap-2 text-xs" style={{ color: "#4C7FF8" }}>
                  <Loader2 size={14} className="animate-spin" />
                  正在拆解剧本《{proj?.title ?? ""}》，AI 分析剧情结构中，预计 30s…
                </div>
                <div className="mt-4 space-y-2">
                  {[0, 1, 2].map((i) => (
                    <div key={i} className="h-12 animate-pulse rounded-lg" style={{ background: "#16161C", animationDelay: `${i * 150}ms` }} />
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* 叙事单元卡片列表 */}
          {!splittingNow && beats.map((b) => {
            const badge = beatStatusBadge(b.status);
            const dur = durationMap[b.beat_id] ?? (b.preset_duration_sec > 0 ? `${b.preset_duration_sec}秒` : "自动");
            return (
              <div key={b.beat_id} id={`unit-card-${b.beat_id}`} className="unit-card rounded-2xl overflow-hidden transition-all">
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
                      <button className="w-6 h-6 rounded flex items-center justify-center transition-colors hover:bg-white/5" style={{ color: "#6B7088" }} onClick={() => toast.info("单元编辑将在后续版本开放")}>
                        <Pencil size={14} />
                      </button>
                    </div>
                  </div>
                  <div className="unit-script-box rounded-lg p-3 text-sm leading-relaxed max-h-[280px] overflow-y-auto scrollbar-thin whitespace-pre-wrap">
                    {b.script_text || "（无剧本原文）"}
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
                  toast.info("手动删除叙事单元将在后续版本开放");
                  setDeleteTarget(null);
                }}
              >
                删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Auto Split Modal */}
      {splitModalOpen && (
        <div className="modal-overlay">
          <div className="modal-backdrop" onClick={() => setSplitModalOpen(false)} />
          <div className="modal-content" style={{ width: 420 }}>
            <h3 className="text-white text-lg font-medium mb-1">AI 自动拆分设置</h3>
            <p className="text-sm mb-5" style={{ color: "#8B8B9A" }}>配置剧本自动拆分的规则和参数</p>

            <div className="mb-4">
              <label className="text-sm text-white mb-2 block">拆分粒度</label>
              <div className="wb-select-wrapper">
                <select className="wb-select" defaultValue="标准">
                  <option>细粒度</option>
                  <option>标准</option>
                  <option>粗粒度</option>
                </select>
                <ChevronDown size={16} />
              </div>
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
              <label className="text-sm text-white mb-2 block">单集默认时长</label>
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
                onClick={() => void handleSplit()}
              >
                {splitting ? "提交中…" : "开始拆分"}
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
