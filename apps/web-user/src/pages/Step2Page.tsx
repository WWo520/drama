// S1-F04 Step2 资产生成页 —— 按 luxreal-clone 设计稿 workbench-assets.html 融合（批次 3）
// 布局：WorkbenchShell（AppShell + 剧集侧栏 + WorkflowPill 顶栏），内容区 6 Tab + 统计卡 + 三类资产卡
// 接口：GET /episodes/:id/assets/overview|characters|scenes|props、POST /assets/batch-generate、POST /points/preview
import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ArrowRight, Box, ChevronDown, Film, Gem, Image as ImageIcon, MapPin,
  MoreHorizontal, Mountain, MountainSnow, Package, Pencil, Play, Plus, PlusCircle,
  RefreshCw, Trees, User, Video, Wand2, X,
} from "lucide-react";

import {
  ApiError,
  errorMessage,
  assetsApi,
  previewApi,
  projectsApi,
  splitApi,
  type AssetsOverview,
  type BeatSummary,
  type CharacterItem,
  type EpisodeLite,
  type ProjectDetail,
  type PropItem,
  type SceneItem,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import "./step2.css";

// 错误文案：ApiError 走错误码映射（05 C 章），避免弹出后端原始报文
function errMsg(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return errorMessage(e.code, e.message);
  return e instanceof Error ? e.message : fallback;
}

const isRunning = (s: string) => s === "QUEUED" || s === "RUNNING";

// 后端 tags 为 jsonb 数组（pgx 扫描后为 JSON 字符串），兼容数组/逗号分隔字符串
function parseTags(raw: unknown): string[] {
  if (Array.isArray(raw)) return raw.map(String).filter(Boolean);
  if (typeof raw !== "string" || !raw) return [];
  const trimmed = raw.trim();
  if (trimmed.startsWith("[")) {
    try {
      const arr = JSON.parse(trimmed) as unknown;
      if (Array.isArray(arr)) return arr.map(String).filter(Boolean);
    } catch {
      /* 落入逗号分隔解析 */
    }
  }
  return trimmed.split(",").map((t) => t.trim()).filter(Boolean);
}

type TabKey = "overview" | "characters" | "scenes" | "props";

// ===== 角色卡（三态：已生成 / 生成中 / 未生成）=====
function CharacterCard({
  ch,
  onRegen,
  busy,
}: {
  ch: CharacterItem;
  onRegen: () => void;
  busy: boolean;
}) {
  const running = isRunning(ch.generation_status);
  const done = ch.generation_status === "SUCCESS";
  const tags = parseTags(ch.tags);

  return (
    <div className="bg-[#1E1E24] rounded-xl border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors">
      {/* Card Header */}
      <div className="flex items-center justify-between px-3 py-2.5">
        <span className="text-white font-medium text-sm">{ch.name}</span>
        <div className="flex items-center gap-1">
          <span
            className="text-[10px] text-[#6B7088] border border-[#3A3A4A] px-1.5 py-0.5 rounded cursor-pointer hover:border-[#4C7FF8] hover:text-[#4C7FF8] transition-colors"
            onClick={() => toast.info("角色扩展编辑将在后续版本开放")}
          >
            扩展编辑
          </span>
          <button className="w-6 h-6 rounded flex items-center justify-center text-[#6B7088] hover:bg-white/5">
            <MoreHorizontal size={16} />
          </button>
        </div>
      </div>
      {/* Short Description */}
      {ch.description_notes && (
        <div className="px-3 pb-2 text-xs text-[#6B7088] line-clamp-2">{ch.description_notes}</div>
      )}
      {/* Style Tags */}
      <div className="px-3 pb-2 flex items-center gap-1.5 flex-wrap">
        {tags.map((t) => (
          <span key={t} className="bg-[#2A2A4A] text-[#4C7FF8] text-[10px] px-2 py-0.5 rounded">{t}</span>
        ))}
        <button
          className="w-5 h-5 rounded border border-dashed border-[#3A3A4A] flex items-center justify-center text-[#6B7088] hover:border-[#4C7FF8] hover:text-[#4C7FF8] transition-colors"
          onClick={() => toast.info("添加造型将在后续版本开放")}
        >
          <Plus size={12} />
        </button>
      </div>
      {/* 生成中进度条 */}
      {running && (
        <div className="px-3 pb-2">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-[#4C7FF8]">
              {ch.generation_status === "QUEUED" ? "排队中…" : "生成中…"}
            </span>
          </div>
          <div className="asset-progress">
            <div className="asset-progress-bar" />
          </div>
        </div>
      )}
      {/* 已生成/未生成时的 AI 生图 label */}
      {!running && (
        <div className="px-3 pb-2 text-xs text-[#6B7088] flex items-center gap-1.5">
          <PlusCircle size={16} />
          AI 生图
        </div>
      )}
      {/* Character Images: 4 images (3/4 portrait) */}
      <div className="px-3 pb-3">
        <div className="grid grid-cols-4 gap-1.5">
          {done && ch.main_image_url ? (
            <>
              <div className="aspect-[3/4] rounded-lg overflow-hidden relative cursor-pointer">
                <img src={ch.main_image_url} alt={ch.name} className="absolute inset-0 w-full h-full object-cover" />
                <div className="absolute bottom-1 left-1 right-1 text-[8px] text-white/70 text-center bg-black/40 rounded">正面</div>
              </div>
              {["侧面", "背面", "动作"].map((label) => (
                <div key={label} className="aspect-[3/4] rounded-lg char-silhouette overflow-hidden relative cursor-pointer">
                  <div className="absolute inset-0 flex items-center justify-center">
                    <User size={24} className="text-[#4C7FF8]/30" />
                  </div>
                  <div className="absolute bottom-1 left-1 right-1 text-[8px] text-white/50 text-center">{label}</div>
                </div>
              ))}
            </>
          ) : running ? (
            Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="aspect-[3/4] rounded-lg char-silhouette overflow-hidden relative">
                <div className="absolute inset-0 flex items-center justify-center">
                  <User size={24} className="text-[#8B5CF8]/30" />
                </div>
              </div>
            ))
          ) : (
            Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="aspect-[3/4] rounded-lg bg-[#1A1A24] border border-dashed border-[#2A2A35] flex items-center justify-center cursor-pointer hover:border-[#4C7FF8]/50 transition-colors"
                onClick={onRegen}
              >
                <Plus size={20} className="text-[#3A3A4A]" />
              </div>
            ))
          )}
        </div>
      </div>
      {/* Action Buttons */}
      <div className="px-3 pb-3 flex items-center gap-1.5">
        <button
          onClick={onRegen}
          disabled={busy || running}
          className="flex-1 flex items-center justify-center gap-1 text-xs bg-[#16161C] border border-[#2A2A35] rounded-lg px-2 py-1 text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <RefreshCw size={14} className={running ? "animate-spin" : ""} />
          {running ? "生成中" : done ? "重新生成" : "生成"}
        </button>
        <button
          onClick={() => toast.info("提示词编辑将在后续版本开放")}
          className="flex-1 flex items-center justify-center gap-1 text-xs bg-[#16161C] border border-[#2A2A35] rounded-lg px-2 py-1 text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors"
        >
          <Pencil size={14} />
          编辑提示词
        </button>
        <button className="w-7 h-7 flex items-center justify-center bg-[#16161C] border border-[#2A2A35] rounded-lg text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors">
          <MoreHorizontal size={16} />
        </button>
      </div>
      {/* Description */}
      {ch.description_notes && (
        <div className="px-3 pb-3 text-[11px] text-[#6B7088] leading-relaxed line-clamp-4">{ch.description_notes}</div>
      )}
    </div>
  );
}

// ===== 场景卡 =====
const SCENE_ICONS = [MountainSnow, Mountain, Trees, Box];
function SceneCard({
  sc,
  index,
  onRegen,
  busy,
}: {
  sc: SceneItem;
  index: number;
  onRegen: () => void;
  busy: boolean;
}) {
  const running = isRunning(sc.generation_status);
  const done = sc.generation_status === "SUCCESS";
  const Icon = SCENE_ICONS[index % SCENE_ICONS.length];
  return (
    <div className="bg-[#1E1E24] rounded-xl border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors cursor-pointer">
      {/* Scene Image (16:9) */}
      {done && sc.main_image_url ? (
        <div className="aspect-video relative overflow-hidden">
          <img src={sc.main_image_url} alt={sc.base_name} className="absolute inset-0 w-full h-full object-cover" />
          <div className="absolute inset-0 bg-gradient-to-t from-black/40 to-transparent"></div>
        </div>
      ) : (
        <div className="aspect-video scene-placeholder relative overflow-hidden">
          <div className="absolute inset-0 flex items-center justify-center">
            {running ? (
              <RefreshCw size={32} className="text-[#4C7FF8]/40 animate-spin" />
            ) : (
              <Icon size={40} className="text-[#4C7FF8]/20" />
            )}
          </div>
          <div className="absolute inset-0 bg-gradient-to-t from-black/40 to-transparent"></div>
          {running && (
            <div className="absolute bottom-2 left-3 right-3">
              <div className="asset-progress"><div className="asset-progress-bar" /></div>
            </div>
          )}
        </div>
      )}
      <div className="p-3">
        {/* 3D Studio Button */}
        <button
          onClick={() => toast.info("3D 片场为 Beta 功能，将在后续版本开放")}
          className="w-full flex items-center justify-center gap-1.5 text-xs bg-[#4C7FF8]/10 text-[#4C7FF8] rounded-lg px-2 py-1.5 hover:bg-[#4C7FF8]/20 transition-colors mb-2"
        >
          <Box size={14} />
          生成3D片场
        </button>
        {/* Action Buttons */}
        <div className="flex items-center gap-1.5 mb-2">
          <button
            onClick={onRegen}
            disabled={busy || running}
            className="flex-1 flex items-center justify-center gap-1 text-xs bg-[#16161C] border border-[#2A2A35] rounded-lg px-2 py-1 text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <RefreshCw size={14} />
            {running ? "生成中" : done ? "重新生成" : "生成"}
          </button>
          <button
            onClick={() => toast.info("提示词编辑将在后续版本开放")}
            className="flex-1 flex items-center justify-center gap-1 text-xs bg-[#16161C] border border-[#2A2A35] rounded-lg px-2 py-1 text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors"
          >
            <Pencil size={14} />
            编辑提示词
          </button>
          <button className="w-7 h-7 flex items-center justify-center bg-[#16161C] border border-[#2A2A35] rounded-lg text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors">
            <MoreHorizontal size={16} />
          </button>
        </div>
        <div className="flex items-center justify-between mb-2">
          <span className="text-white text-sm font-medium">{sc.base_name}</span>
        </div>
        <div className="text-xs text-[#6B7088] line-clamp-2 mb-2">场景编号 {sc.code} · 生成消耗 {sc.point_cost} 积分</div>
      </div>
    </div>
  );
}

// ===== 道具卡 =====
const PROP_ICONS = [ImageIcon, Gem, Package, MountainSnow];
function PropCard({
  prop,
  index,
  onRegen,
  busy,
}: {
  prop: PropItem;
  index: number;
  onRegen: () => void;
  busy: boolean;
}) {
  const running = isRunning(prop.generation_status);
  const done = prop.generation_status === "SUCCESS";
  const Icon = PROP_ICONS[index % PROP_ICONS.length];
  return (
    <div className="bg-[#1E1E24] rounded-xl border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors cursor-pointer">
      {done && prop.main_image_url ? (
        <div className="aspect-square relative overflow-hidden">
          <img src={prop.main_image_url} alt={prop.name} className="absolute inset-0 w-full h-full object-cover" />
        </div>
      ) : (
        <div className="aspect-square prop-placeholder relative overflow-hidden">
          <div className="absolute inset-0 flex items-center justify-center">
            {running ? (
              <RefreshCw size={32} className="text-[#4C7FF8]/40 animate-spin" />
            ) : (
              <Icon size={40} className="text-[#4C7FF8]/25" />
            )}
          </div>
          {running && (
            <div className="absolute bottom-2 left-3 right-3">
              <div className="asset-progress"><div className="asset-progress-bar" /></div>
            </div>
          )}
        </div>
      )}
      <div className="p-3">
        {/* Action Buttons */}
        <div className="flex items-center gap-1.5 mb-2">
          <button
            onClick={onRegen}
            disabled={busy || running}
            className="flex-1 flex items-center justify-center gap-1 text-xs bg-[#16161C] border border-[#2A2A35] rounded-lg px-2 py-1 text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <RefreshCw size={14} />
            {running ? "生成中" : done ? "重新生成" : "生成"}
          </button>
          <button
            onClick={() => toast.info("提示词编辑将在后续版本开放")}
            className="flex-1 flex items-center justify-center gap-1 text-xs bg-[#16161C] border border-[#2A2A35] rounded-lg px-2 py-1 text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors"
          >
            <Pencil size={14} />
            编辑提示词
          </button>
          <button className="w-7 h-7 flex items-center justify-center bg-[#16161C] border border-[#2A2A35] rounded-lg text-white/70 hover:text-white hover:border-[#3A3A4A] transition-colors">
            <MoreHorizontal size={14} />
          </button>
        </div>
        <div className="flex items-center justify-between mb-1">
          <span className="text-white text-sm font-medium">{prop.name}</span>
        </div>
        <div className="text-xs text-[#6B7088] line-clamp-2">
          {prop.category || "道具"} · 生成消耗 {prop.point_cost} 积分
        </div>
      </div>
    </div>
  );
}

// ===== 统计卡 =====
function StatCard({ icon, value, label }: { icon: React.ReactNode; value: number | string; label: string }) {
  return (
    <div className="bg-[#1E1E24] border border-[#2A2A35] rounded-xl p-4 flex-1 relative">
      <div className="absolute top-3 right-3 text-[#4C7FF8]">{icon}</div>
      <div className="text-2xl text-white font-bold">{value}</div>
      <div className="text-xs text-[#6B7088] mt-1">{label}</div>
    </div>
  );
}

// 批量生成范围
type BatchRange = "all" | "characters" | "scenes" | "props";
const RANGE_OPTIONS: { key: BatchRange; label: string }[] = [
  { key: "all", label: "本集全部" },
  { key: "characters", label: "仅角色" },
  { key: "scenes", label: "仅场景" },
  { key: "props", label: "仅道具" },
];

export default function Step2Page() {
  const { projectId = "" } = useParams();
  const [proj, setProj] = useState<ProjectDetail | null>(null);
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [loadError, setLoadError] = useState(false);

  const [tab, setTab] = useState<TabKey>("overview");
  const [overview, setOverview] = useState<AssetsOverview | null>(null);
  const [chars, setChars] = useState<CharacterItem[]>([]);
  const [scenes, setScenes] = useState<SceneItem[]>([]);
  const [props, setProps] = useState<PropItem[]>([]);

  const [batchOpen, setBatchOpen] = useState(false);
  const [batchRange, setBatchRange] = useState<BatchRange>("all");
  // 风格/每张数量（对齐设计稿视觉态；后端暂不接收参数，按项目风格与默认张数生成）
  const [batchStyle, setBatchStyle] = useState("武侠仙侠");
  const [batchCount, setBatchCount] = useState(4);
  const [batchCost, setBatchCost] = useState<{ items: number; cost: number } | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stepState = useStepState(projectId, proj?.status ?? null);

  // 加载项目 + 剧集
  useEffect(() => {
    void (async () => {
      try {
        const p = await projectsApi.detail(projectId);
        setLoadError(false);
        setProj(p);
        const eps = await splitApi.episodes(projectId);
        setEpisodes(eps.items ?? []);
        if (eps.items.length > 0) setActiveEpisode(eps.items[0].episode_id);
      } catch (e) {
        setLoadError(true);
        toast.error(errMsg(e, "加载失败"));
      }
    })();
  }, [projectId]);

  // 加载叙事单元（内容区统计卡与单元卡用；侧栏仅展示剧集，不展示单元）
  useEffect(() => {
    if (!activeEpisode) return;
    void (async () => {
      try {
        const data = await splitApi.beats(activeEpisode);
        setBeats(data.items ?? []);
      } catch {
        /* 忽略 */
      }
    })();
  }, [activeEpisode]);

  // 拉取全部资产数据
  const loadAssets = useCallback(async () => {
    if (!activeEpisode) return null;
    try {
      const [ov, ch, sc, pr] = await Promise.all([
        assetsApi.overview(activeEpisode),
        assetsApi.characters(activeEpisode),
        assetsApi.scenes(activeEpisode),
        assetsApi.props(activeEpisode),
      ]);
      setOverview(ov);
      setChars(ch.items ?? []);
      setScenes(sc.items ?? []);
      setProps(pr.items ?? []);
      return { chars: ch.items ?? [], scenes: sc.items ?? [], props: pr.items ?? [] };
    } catch (e) {
      toast.error(errMsg(e, "加载资产失败"));
      return null;
    }
  }, [activeEpisode]);

  useEffect(() => {
    void loadAssets();
  }, [loadAssets]);

  // 有任务在跑 → 2.5s 轮询直至全部终态
  const anyRunning =
    chars.some((c) => isRunning(c.generation_status)) ||
    scenes.some((s) => isRunning(s.generation_status)) ||
    props.some((p) => isRunning(p.generation_status));

  useEffect(() => {
    if (anyRunning && !pollRef.current) {
      pollRef.current = setInterval(async () => {
        const data = await loadAssets();
        if (!data) return;
        const still =
          data.chars.some((c) => isRunning(c.generation_status)) ||
          data.scenes.some((s) => isRunning(s.generation_status)) ||
          data.props.some((p) => isRunning(p.generation_status));
        if (!still && pollRef.current) {
          clearInterval(pollRef.current);
          pollRef.current = null;
          toast.success("资产生成完成");
        }
      }, 2500);
    } else if (!anyRunning && pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [anyRunning, loadAssets]);

  // 按范围构造待生成 items（跳过已成功）
  const buildItems = useCallback(
    (range: BatchRange) => {
      const items: { kind: "CHARACTER_VARIANT" | "SCENE" | "PROP"; target_id: number }[] = [];
      if (range === "all" || range === "characters") {
        chars.filter((c) => c.generation_status !== "SUCCESS").forEach((c) => items.push({ kind: "CHARACTER_VARIANT", target_id: c.id }));
      }
      if (range === "all" || range === "scenes") {
        scenes.filter((s) => s.generation_status !== "SUCCESS").forEach((s) => items.push({ kind: "SCENE", target_id: s.scene_id }));
      }
      if (range === "all" || range === "props") {
        props.filter((p) => p.generation_status !== "SUCCESS").forEach((p) => items.push({ kind: "PROP", target_id: p.prop_id }));
      }
      return items;
    },
    [chars, scenes, props],
  );

  // 打开批量生成弹窗 → 预览积分
  const openBatchModal = async () => {
    setBatchRange("all");
    setBatchOpen(true);
    const items = buildItems("all");
    if (items.length === 0) {
      setBatchCost({ items: 0, cost: 0 });
      return;
    }
    try {
      const pv = await previewApi.preview(projectId, items);
      setBatchCost({ items: pv.total_items, cost: pv.actual_cost });
    } catch {
      setBatchCost(null);
    }
  };

  // 切换范围 → 重新预览
  const switchRange = async (range: BatchRange) => {
    setBatchRange(range);
    const items = buildItems(range);
    if (items.length === 0) {
      setBatchCost({ items: 0, cost: 0 });
      return;
    }
    try {
      setBatchCost(null);
      const pv = await previewApi.preview(projectId, items);
      setBatchCost({ items: pv.total_items, cost: pv.actual_cost });
    } catch {
      setBatchCost(null);
    }
  };

  // 开始批量生成
  const handleBatchGenerate = async () => {
    if (!activeEpisode) return;
    const items = buildItems(batchRange);
    if (items.length === 0) {
      toast.info("当前范围内没有待生成的资产");
      return;
    }
    try {
      setSubmitting(true);
      const data = await assetsApi.batchGenerate({ episode_id: activeEpisode, items });
      toast.success(`已提交 ${data.created} 个生成任务`);
      setBatchOpen(false);
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "批量生成失败"));
    } finally {
      setSubmitting(false);
    }
  };

  // 单资产重新生成
  const regenOne = async (kind: "CHARACTER_VARIANT" | "SCENE" | "PROP", targetId: number) => {
    if (!activeEpisode) return;
    try {
      await assetsApi.batchGenerate({ episode_id: activeEpisode, items: [{ kind, target_id: targetId }] });
      toast.success("生成任务已提交");
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "提交生成失败"));
    }
  };

  // ESC 关闭弹窗
  useEffect(() => {
    if (!batchOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setBatchOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [batchOpen]);

  const showChars = tab === "overview" || tab === "characters";
  const showScenes = tab === "overview" || tab === "scenes";
  const showProps = tab === "overview" || tab === "props";

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
    >
      {/* Project Settings Bar */}
      <div className="mx-6 mt-2 flex items-center justify-between h-9 px-4 rounded-xl flex-shrink-0" style={{ background: "#16161C", border: "1px solid #2A2A35" }}>
        <span className="text-white font-medium text-sm truncate">{proj?.title ?? "加载中…"}</span>
        <div className="flex items-center gap-2">
          <button className="flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" style={{ background: "#1E1E24", border: "1px solid #2A2A35" }}>
            Lux Image 2
            <ChevronDown size={12} />
          </button>
          <button className="flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" style={{ background: "#1E1E24", border: "1px solid #2A2A35" }}>
            {proj?.tags?.[0] || "武侠仙侠"}
            <ChevronDown size={12} />
          </button>
          <button
            className="flex items-center gap-1 text-xs text-[#6B7088] hover:text-white transition-colors"
            onClick={() => toast.info("模板管理将在后续版本开放")}
          >
            管理模板
          </button>
        </div>
      </div>

      {/* Sub Tabs */}
      <div className="flex items-center gap-0 px-6 border-b border-[#1E1E2E] flex-shrink-0">
        {([
          ["overview", "概览", false],
          ["characters", "角色", false],
          ["scenes", "场景", false],
          ["props", "道具", false],
          ["shots", "智能镜头", true],
          ["materials", "通用素材", true],
        ] as const).map(([key, label, disabled]) => (
          <button
            key={key}
            disabled={disabled}
            onClick={() => !disabled && setTab(key as TabKey)}
            title={disabled ? "后续版本开放" : undefined}
            className={`px-4 py-3 text-sm border-b-2 transition-colors ${
              disabled
                ? "text-[#3A3A4A] border-transparent cursor-not-allowed"
                : tab === key
                  ? "text-white border-[#4C7FF8] font-medium"
                  : "text-[#6B7088] border-transparent hover:text-white cursor-pointer"
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Content Scroll Area */}
      <div className="flex-1 overflow-y-auto scrollbar-thin px-6 py-4 pb-24">
        {/* Asset Stats Row（仅概览 Tab） */}
        {tab === "overview" && (
          <div className="flex gap-3 mb-6">
            <StatCard icon={<User size={16} />} value={overview?.characters.total ?? "-"} label={`角色总数（已生成 ${overview?.characters.generated ?? 0}）`} />
            <StatCard icon={<MapPin size={16} />} value={overview?.scenes.total ?? "-"} label={`场景总数（已生成 ${overview?.scenes.generated ?? 0}）`} />
            <StatCard icon={<Package size={16} />} value={overview?.props.total ?? "-"} label={`道具总数（已生成 ${overview?.props.generated ?? 0}）`} />
            <StatCard icon={<Video size={16} />} value={beats.length} label="叙事单元" />
          </div>
        )}

        {/* Section: 角色 */}
        {showChars && (
          <div className="mb-6">
            <h3 className="text-sm text-[#8B8B9A] font-medium mb-3">角色</h3>
            {chars.length === 0 ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">
                暂无角色。完成剧本拆解后角色将出现在这里。
              </div>
            ) : (
              <div className="grid grid-cols-3 gap-4">
                {chars.map((ch) => (
                  <CharacterCard
                    key={ch.id}
                    ch={ch}
                    busy={submitting}
                    onRegen={() => void regenOne("CHARACTER_VARIANT", ch.id)}
                  />
                ))}
              </div>
            )}
          </div>
        )}

        {/* Section: 场景 */}
        {showScenes && (
          <div className="mb-6">
            <h3 className="text-sm text-[#8B8B9A] font-medium mb-3">场景</h3>
            {scenes.length === 0 ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">
                暂无场景。完成剧本拆解后场景将出现在这里。
              </div>
            ) : (
              <div className="grid grid-cols-4 gap-4">
                {scenes.map((sc, i) => (
                  <SceneCard key={sc.scene_id} sc={sc} index={i} busy={submitting} onRegen={() => void regenOne("SCENE", sc.scene_id)} />
                ))}
              </div>
            )}
          </div>
        )}

        {/* Section: 道具 */}
        {showProps && (
          <div className="mb-6">
            <h3 className="text-sm text-[#8B8B9A] font-medium mb-3">道具</h3>
            {props.length === 0 ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">
                暂无道具。完成剧本拆解后道具将出现在这里。
              </div>
            ) : (
              <div className="grid grid-cols-4 gap-4">
                {props.map((p, i) => (
                  <PropCard key={p.prop_id} prop={p} index={i} busy={submitting} onRegen={() => void regenOne("PROP", p.prop_id)} />
                ))}
              </div>
            )}
          </div>
        )}

        {/* Section: 智能镜头（概览 Tab 视觉态，功能随 Step3 落地） */}
        {tab === "overview" && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm text-[#8B8B9A] font-medium">智能镜头</h3>
              <button
                className="text-xs text-[#4C7FF8] hover:text-[#6B8FFF] transition-colors"
                onClick={() => stepState.goStep(3)}
              >
                前往视频制作
              </button>
            </div>
            <div className="flex gap-3 overflow-x-auto scrollbar-thin pb-2">
              {beats.slice(0, 4).map((b) => (
                <div
                  key={b.beat_id}
                  className="w-56 flex-shrink-0 rounded-xl bg-[#1E1E24] border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors cursor-pointer"
                  onClick={() => stepState.goStep(3)}
                >
                  <div className="aspect-video relative scene-placeholder">
                    <div className="absolute inset-0 flex items-center justify-center">
                      <Film size={40} className="text-[#4C7FF8]/20" />
                    </div>
                    <div className="absolute inset-0 flex items-center justify-center">
                      <div className="w-10 h-10 rounded-full bg-black/50 flex items-center justify-center">
                        <Play size={20} className="text-white ml-0.5" />
                      </div>
                    </div>
                    <div className="absolute inset-0 bg-gradient-to-t from-black/40 to-transparent"></div>
                  </div>
                  <div className="p-3">
                    <div className="flex items-center justify-between">
                      <div className="text-sm text-white font-medium truncate">{b.title || `叙事单元${b.sort_order}`}</div>
                    </div>
                    <div className="text-xs text-[#6B7088] mt-1">待生成视频</div>
                  </div>
                </div>
              ))}
              <div
                className="w-56 flex-shrink-0 rounded-xl bg-[#1E1E24] border border-[#2A2A35] border-dashed overflow-hidden hover:border-[#4C7FF8]/50 transition-colors cursor-pointer flex flex-col items-center justify-center"
                style={{ minHeight: "200px" }}
                onClick={() => stepState.goStep(3)}
              >
                <div className="w-10 h-10 rounded-full bg-[#1A1A24] flex items-center justify-center mb-2">
                  <Plus size={20} className="text-[#6B7088]" />
                </div>
                <div className="text-xs text-[#6B7088]">前往视频制作</div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Bottom Floating Action Bar */}
      <div className="fixed bottom-6 left-1/2 -translate-x-1/2 flex items-center gap-3 z-40">
        <button
          className="bg-[#1E1E24] border border-[#2A2A35] text-[#C0C0CC] text-sm px-5 py-2.5 rounded-full hover:bg-[#252535] transition-colors flex items-center gap-2"
          onClick={() => toast.info("资产重新提取将在后续版本开放")}
        >
          <RefreshCw size={16} />
          重新提取本集资产
        </button>
        <button
          onClick={() => void openBatchModal()}
          className="bg-gradient-to-r from-[#4C7FF8] to-[#8B5CF8] text-white text-sm px-6 py-2.5 rounded-full font-medium shadow-lg shadow-[#4C7FF8]/20 hover:shadow-[#4C7FF8]/30 transition-shadow flex items-center gap-2"
        >
          <Wand2 size={16} />
          批量生成资产图
        </button>
        <button
          onClick={() => stepState.goStep(3)}
          className="bg-gradient-to-r from-[#4C7FF8] to-[#6B8FFF] text-white text-sm px-6 py-2.5 rounded-full font-medium shadow-lg shadow-[#4C7FF8]/20 hover:shadow-[#4C7FF8]/30 transition-shadow flex items-center gap-2"
        >
          下一步：视频制作
          <ArrowRight size={16} />
        </button>
      </div>

      {/* Modal Overlay */}
      {batchOpen && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50" onClick={() => setBatchOpen(false)} />
      )}

      {/* Batch Generate Modal */}
      {batchOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center pointer-events-none">
          <div className="bg-[#1A1A2E] border border-[#2A2A3E] rounded-2xl w-[480px] pointer-events-auto shadow-2xl">
            <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A3E]">
              <h3 className="text-white font-medium text-base">批量生成设置</h3>
              <button
                className="w-7 h-7 rounded-lg flex items-center justify-center text-[#6B7088] hover:bg-white/5 transition-colors"
                onClick={() => setBatchOpen(false)}
              >
                <X size={16} />
              </button>
            </div>
            <div className="p-5 space-y-4">
              {/* Generate Range */}
              <div>
                <label className="text-xs text-[#8B8B9A] mb-2 block">生成范围</label>
                <div className="flex rounded-lg p-0.5 bg-[#0D0D14] border border-[#2A2A3E]">
                  {RANGE_OPTIONS.map((opt) => (
                    <button
                      key={opt.key}
                      onClick={() => void switchRange(opt.key)}
                      className={`flex-1 py-1.5 text-xs rounded-md transition-colors ${
                        batchRange === opt.key
                          ? "bg-gradient-to-r from-[#4C7FF8] to-[#8B5CF6] text-white font-medium"
                          : "text-[#6B7088] hover:text-white"
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </div>
              {/* Model（视觉态，后端按模型路由自动选择） */}
              <div>
                <label className="text-xs text-[#8B8B9A] mb-1.5 block">模型</label>
                <div className="relative">
                  <select className="w-full h-9 px-3 rounded-lg bg-[#0D0D14] border border-[#2A2A3E] text-white text-sm focus:border-[#4C7FF8] focus:outline-none transition-colors appearance-none cursor-pointer">
                    <option>自动路由（推荐）</option>
                    <option>Lux Image 2</option>
                    <option>Lux Realistic</option>
                  </select>
                  <ChevronDown size={16} className="text-[#6B7088] absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
                </div>
              </div>
              {/* Style（视觉态；默认跟随项目风格） */}
              <div>
                <label className="text-xs text-[#8B8B9A] mb-1.5 block">风格</label>
                <div className="relative">
                  <select
                    value={batchStyle}
                    onChange={(e) => setBatchStyle(e.target.value)}
                    className="w-full h-9 px-3 rounded-lg bg-[#0D0D14] border border-[#2A2A3E] text-white text-sm focus:border-[#4C7FF8] focus:outline-none transition-colors appearance-none cursor-pointer"
                  >
                    <option>武侠仙侠</option>
                    <option>都市现代</option>
                    <option>古风宫廷</option>
                    <option>科幻未来</option>
                  </select>
                  <ChevronDown size={16} className="text-[#6B7088] absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
                </div>
              </div>
              {/* Per Asset Count（视觉态） */}
              <div>
                <label className="text-xs text-[#8B8B9A] mb-2 block">每张生成数量</label>
                <div className="flex rounded-lg p-0.5 bg-[#0D0D14] border border-[#2A2A3E] w-48">
                  {[2, 4, 6].map((n) => (
                    <button
                      key={n}
                      onClick={() => setBatchCount(n)}
                      className={`flex-1 py-1.5 text-xs rounded-md transition-colors ${
                        batchCount === n
                          ? "bg-gradient-to-r from-[#4C7FF8] to-[#8B5CF6] text-white font-medium"
                          : "text-[#6B7088] hover:text-white"
                      }`}
                    >
                      {n}张
                    </button>
                  ))}
                </div>
              </div>
              {/* Cost Info */}
              <div className="flex items-center justify-between pt-2">
                <span className="text-sm text-[#8B8B9A]">预计消耗（{batchCost ? `${batchCost.items} 项资产` : "计算中…"}）</span>
                <span className="text-base font-bold text-[#4C7FF8]">
                  {batchCost ? `${batchCost.cost} 积分` : "…"}
                </span>
              </div>
              {batchCost && batchCost.items === 0 && (
                <div className="text-xs text-[#6B7088]">当前范围内资产均已生成，无需重复消耗积分。</div>
              )}
            </div>
            {/* Footer */}
            <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[#2A2A3E]">
              <button
                className="px-4 py-2 rounded-lg text-sm text-[#C0C0CC] hover:bg-white/5 transition-colors"
                onClick={() => setBatchOpen(false)}
              >
                取消
              </button>
              <button
                onClick={() => void handleBatchGenerate()}
                disabled={submitting || (batchCost !== null && batchCost.items === 0)}
                className="px-5 py-2 rounded-lg bg-gradient-to-r from-[#4C7FF8] to-[#8B5CF6] text-white text-sm font-medium hover:shadow-lg hover:shadow-[#4C7FF8]/20 transition-shadow disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {submitting ? "提交中…" : "开始生成"}
              </button>
            </div>
          </div>
        </div>
      )}

    </WorkbenchShell>
  );
}
