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
  api,
  errorMessage,
  assetsApi,
  previewApi,
  projectsApi,
  splitApi,
  videoApi,
  type AssetsOverview,
  type BeatSummary,
  type CharacterItem,
  type EpisodeLite,
  type ProjectDetail,
  type PropItem,
  type SceneItem,
  type ShotVideo,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import ProjectLoadError from "../components/ProjectLoadError";
import AssetDetailDialog, { type DialogKind } from "../components/AssetDetailDialog";
import AssetProgress from "../components/AssetProgress";
import { useStepState } from "../hooks/useStepState";
import { matchTaskTypes, useWsDrivenPoll } from "../hooks/useWsDrivenPoll";
import { cachedGet, invalidateCachedGet } from "../utils/cachedApi";
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

type TabKey = "overview" | "characters" | "scenes" | "props" | "shots" | "materials";

// 项目通用素材（GET /projects/:project_id/assets）
interface ProjectAsset {
  asset_id: string;
  type: string;
  name: string;
  status: string;
  files_json: string;
  origin_episode_id: number;
}

// 模板（GET /templates）
interface TemplateItem {
  template_id: string;
  name: string;
  description?: string;
  price_points?: number;
}

const ASSET_TYPE_LABEL: Record<string, string> = {
  CHARACTER_VARIANT: "角色变体",
  SCENE: "场景",
  PROP: "道具",
  CHARACTER: "角色",
  GENERIC: "通用素材",
};

// 场景时段枚举 → 中文标签（对齐 time_of_day_enum）
const SCENE_TOD_LABEL: Record<string, string> = {
  DAWN_BLUE_HOUR: "黎明",
  MORNING_SUNRISE: "清晨",
  NOON_HIGH_SUN: "正午",
  DAY_BRIGHT: "白天",
  AFTERNOON_GOLDEN_HOUR: "午后",
  SUNSET_DUSK: "黄昏",
  NIGHT_CANDLELIT: "夜晚·烛光",
  NIGHT_NEON: "夜晚·霓虹",
  OVERCAST_DAY: "阴天",
  STORM_NIGHT: "暴风雨夜",
};

// ===== 角色卡（三态：已生成 / 生成中 / 未生成）=====
function CharacterCard({
  ch,
  onRegen,
  onEdit,
  onAddVariant,
  onOpenDialog,
  busy,
}: {
  ch: CharacterItem;
  onRegen: () => void;
  onEdit: (ch: CharacterItem) => void;
  onAddVariant: (ch: CharacterItem) => void;
  onOpenDialog: (ch: CharacterItem) => void;
  busy: boolean;
}) {
  const running = isRunning(ch.generation_status);
  const tags = parseTags(ch.tags);
  // 多形象（需求1）：形象变体列表 + 当前选中形象；主图优先取选中形象
  const variants = ch.variants ?? [];
  const [activeVariantId, setActiveVariantId] = useState<number | null>(null);
  const currentVariant = activeVariantId != null ? variants.find((v) => v.id === activeVariantId) ?? null : variants[0] ?? null;
  const currentImg = currentVariant?.main_image_url || ch.main_image_url || "";
  const done = !!currentImg || ch.generation_status === "SUCCESS";
  const currentName = currentVariant?.name || ch.name;

  return (
    <div className="bg-[#1E1E24] rounded-xl border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors">
      {/* Card Header */}
      <div className="flex items-center justify-between px-3 py-2.5">
        <span className="text-white font-medium text-sm">{ch.name}</span>
        <div className="flex items-center gap-1">
          <span
            className="text-[10px] text-[#6B7088] border border-[#3A3A4A] px-1.5 py-0.5 rounded cursor-pointer hover:border-[#4C7FF8] hover:text-[#4C7FF8] transition-colors"
            onClick={() => onEdit(ch)}
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
          onClick={() => onAddVariant(ch)}
          title="添加造型"
        >
          <Plus size={12} />
        </button>
      </div>
      {/* 生成中进度条（百分比） */}
      {running && (
        <div className="px-3 pb-2">
          <AssetProgress status={ch.generation_status} />
        </div>
      )}
      {/* 已生成/未生成时的 AI 生图 label */}
      {!running && (
        <div className="px-3 pb-2 text-xs text-[#6B7088] flex items-center gap-1.5">
          <PlusCircle size={16} />
          AI 生图
        </div>
      )}
      {/* 形象名条（图片上方，点击切换不同形象，对齐 LuxReal） */}
      {variants.length > 0 && (
        <div className="px-3 pb-2 flex items-center gap-1.5 flex-wrap">
          {variants.map((v) => (
            <button
              key={v.id}
              onClick={() => setActiveVariantId(v.id === activeVariantId ? null : v.id)}
              className={`flex items-center gap-1 px-2 py-1 rounded-md text-[10px] whitespace-nowrap border transition-colors ${
                activeVariantId === v.id
                  ? "border-[#4C7FF8] bg-[#4C7FF8]/10 text-white"
                  : "border-[#2A2A35] bg-[#16161C] text-[#8B8B9A] hover:border-[#3A3A4A]"
              }`}
            >
              {v.main_image_url && <img src={v.main_image_url} alt="" className="w-4 h-4 rounded object-cover" />}
              {v.name || `形象${v.id}`}
            </button>
          ))}
        </div>
      )}
      {/* 主图：16:9 完整显示三视图（对齐 LuxReal 一人一张图），点击进入详情看多形象 */}
      <div className="px-3 pb-3">
        {done && currentImg ? (
          <div className="aspect-video rounded-lg overflow-hidden relative cursor-pointer bg-[#16161C]" onClick={() => onOpenDialog(ch)}>
            <img src={currentImg} alt={currentName} className="absolute inset-0 w-full h-full object-contain" />
            {/* 当前形象名 */}
            <div className="absolute top-1 left-1 right-1 text-[10px] text-white/90 text-center bg-black/45 rounded px-1 py-0.5">
              {currentName}
            </div>
          </div>
        ) : running ? (
          <div className="aspect-video rounded-lg char-silhouette overflow-hidden relative">
            <div className="absolute inset-0 flex items-center justify-center">
              <User size={32} className="text-[#8B5CF8]/30" />
            </div>
          </div>
        ) : (
          <div
            className="aspect-video rounded-lg bg-[#1A1A24] border border-dashed border-[#2A2A35] flex items-center justify-center cursor-pointer hover:border-[#4C7FF8]/50 transition-colors"
            onClick={onRegen}
          >
            <Plus size={20} className="text-[#3A3A4A]" />
          </div>
        )}
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
          onClick={() => onEdit(ch)}
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
  onEdit,
  onOpenDialog,
  busy,
}: {
  sc: SceneItem;
  index: number;
  onRegen: () => void;
  onEdit: (sc: SceneItem) => void;
  onOpenDialog: (sc: SceneItem) => void;
  busy: boolean;
}) {
  const running = isRunning(sc.generation_status);
  const done = sc.generation_status === "SUCCESS";
  const Icon = SCENE_ICONS[index % SCENE_ICONS.length];
  // 场景时段变体（对齐 LuxReal 场景日夜多态）：可切换不同时段主图
  const todVariants = sc.tod_variants ?? [];
  const [activeTod, setActiveTod] = useState<number | null>(null);
  const todVariant = activeTod != null ? todVariants[activeTod] ?? null : null;
  const curImg = todVariant?.url || sc.main_image_url || "";
  const curTod = todVariant ? (SCENE_TOD_LABEL[todVariant.time_of_day] || todVariant.time_of_day) : "主图";
  return (
    <div className="bg-[#1E1E24] rounded-xl border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors cursor-pointer">
      {/* Scene Image (16:9) */}
      {done && curImg ? (
        <div className="aspect-video relative overflow-hidden" onClick={() => onOpenDialog(sc)}>
          <img src={curImg} alt={sc.base_name} className="absolute inset-0 w-full h-full object-cover" />
          <div className="absolute inset-0 bg-gradient-to-t from-black/40 to-transparent"></div>
          {/* 当前时段标签 */}
          <div className="absolute top-2 left-2 px-1.5 py-0.5 rounded bg-black/50 text-white text-[10px]">{curTod}</div>
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
              <AssetProgress status={sc.generation_status} />
            </div>
          )}
        </div>
      )}
      <div className="p-3">
        {/* 场景时段变体条：点击切换不同时段主图 */}
        {todVariants.length > 0 && (
          <div className="flex items-center gap-1.5 flex-wrap mb-2">
            {todVariants.map((v, i) => (
              <button
                key={v.time_of_day}
                onClick={() => setActiveTod(i === activeTod ? null : i)}
                className={`px-2 py-1 rounded-md text-[10px] border transition-colors ${
                  activeTod === i
                    ? "border-[#4C7FF8] bg-[#4C7FF8]/10 text-white"
                    : "border-[#2A2A35] bg-[#16161C] text-[#8B8B9A] hover:border-[#3A3A4A]"
                }`}
              >
                {SCENE_TOD_LABEL[v.time_of_day] || v.time_of_day}
              </button>
            ))}
          </div>
        )}
        {/* 3D Studio Button —— 已隐藏（Beta 功能暂缓，需求：暂时移除该入口） */}
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
            onClick={() => onEdit(sc)}
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
  onEdit,
  onOpenDialog,
  busy,
}: {
  prop: PropItem;
  index: number;
  onRegen: () => void;
  onEdit: (prop: PropItem) => void;
  onOpenDialog: (prop: PropItem) => void;
  busy: boolean;
}) {
  const running = isRunning(prop.generation_status);
  const done = prop.generation_status === "SUCCESS";
  const Icon = PROP_ICONS[index % PROP_ICONS.length];
  return (
    <div className="bg-[#1E1E24] rounded-xl border border-[#2A2A35] overflow-hidden hover:border-[#3A3A4A] transition-colors cursor-pointer">
      {done && prop.main_image_url ? (
        <div className="aspect-square relative overflow-hidden" onClick={() => onOpenDialog(prop)}>
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
              <AssetProgress status={prop.generation_status} />
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
            onClick={() => onEdit(prop)}
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
  // 批量弹窗选中的图片模型（""=自动路由），真实透传到生成任务
  const [batchModel, setBatchModel] = useState("");
  // 重新提取本集资产（剧本修改后重跑资产提取）
  const [reextracting, setReextracting] = useState(false);
  const [batchCost, setBatchCost] = useState<{ items: number; cost: number } | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // 顶部模型选择（GET /models?capability=T2I 真源 + /me/preferences 持久化）
  const [imgModelOpen, setImgModelOpen] = useState(false);
  const [imgModels, setImgModels] = useState<{ model_key: string; name: string; cost_points: number; enabled: boolean }[]>([]);
  const [imgModel, setImgModel] = useState("");

  // 智能镜头（汇总当前剧集全部叙事单元的镜头）
  const [shotMap, setShotMap] = useState<Record<string, ShotVideo[]>>({});
  const [shotsLoading, setShotsLoading] = useState(false);

  // 通用素材（GET /projects/:project_id/assets）
  const [materials, setMaterials] = useState<ProjectAsset[]>([]);
  const [materialsLoading, setMaterialsLoading] = useState(false);

  // 模板管理弹窗（GET /templates + POST /templates/:id/apply）
  const [templateOpen, setTemplateOpen] = useState(false);
  const [templates, setTemplates] = useState<TemplateItem[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);

  // 加载图片模型选项与已保存偏好（TTL 缓存：StrictMode 双挂载/HMR 重挂载不重复请求，限流治理）
  useEffect(() => {
    void (async () => {
      try {
        const [m, prefs] = await Promise.all([
          cachedGet<{ items: { model_key: string; name: string; cost_points: number; enabled: boolean }[] }>("/models?capability=T2I", 60000),
          cachedGet<Record<string, unknown>>("/me/preferences", 30000),
        ]);
        if (m.items?.length) setImgModels(m.items);
        const mc = (prefs.model_config ?? {}) as Record<string, unknown>;
        if (mc.image_model) setImgModel(String(mc.image_model));
      } catch {
        /* 模型加载失败：按钮保持默认展示 */
      }
    })();
  }, []);

  // 选择模型 → 保存到 /me/preferences（创作页模型配置共用同一偏好）
  const pickImageModel = async (key: string) => {
    setImgModel(key);
    setImgModelOpen(false);
    try {
      await api.put("/me/preferences", { model_config: { image_model: key } });
      invalidateCachedGet("/me/preferences"); // 写后失效，其他页面重进时读到新值
      toast.success("默认图片模型已更新");
    } catch {
      toast.error("模型偏好保存失败");
    }
  };

  // 智能镜头：拉取当前剧集全部叙事单元的镜头
  const loadShots = useCallback(async () => {
    if (!activeEpisode || beats.length === 0) return;
    setShotsLoading(true);
    try {
      const map: Record<string, ShotVideo[]> = {};
      await Promise.all(
        beats.map(async (b) => {
          try {
            const d = await videoApi.beatShots(b.beat_id);
            map[b.beat_id] = d.items ?? [];
          } catch {
            map[b.beat_id] = [];
          }
        }),
      );
      setShotMap(map);
    } finally {
      setShotsLoading(false);
    }
  }, [activeEpisode, beats]);

  // 通用素材：项目资产列表
  const loadMaterials = useCallback(async () => {
    if (!projectId) return;
    setMaterialsLoading(true);
    try {
      const d = await api.get<{ items: ProjectAsset[] }>(`/projects/${projectId}/assets`);
      setMaterials(d.items ?? []);
    } catch {
      setMaterials([]);
    } finally {
      setMaterialsLoading(false);
    }
  }, [projectId]);

  // Tab 切换时按需加载（智能镜头 / 通用素材）
  useEffect(() => {
    if (tab === "shots") void loadShots();
    if (tab === "materials") void loadMaterials();
  }, [tab, loadShots, loadMaterials]);

  // 模板管理：打开弹窗时加载模板列表
  const openTemplates = async () => {
    setTemplateOpen(true);
    setTemplatesLoading(true);
    try {
      const d = await api.get<{ items: TemplateItem[] }>("/templates", { params: { page_size: 50 } });
      setTemplates(d.items ?? []);
    } catch (e) {
      toast.error(errMsg(e, "模板列表加载失败"));
    } finally {
      setTemplatesLoading(false);
    }
  };

  const applyTemplate = async (t: TemplateItem) => {
    try {
      const d = await api.post<{ template_data: string }>(`/templates/${t.template_id}/apply`);
      toast.success(`模板「${t.name}」已应用${d.template_data ? "（数据已返回）" : ""}`);
      setTemplateOpen(false);
    } catch (e) {
      toast.error(errMsg(e, "模板应用失败"));
    }
  };

  const stepState = useStepState(projectId, proj?.status ?? null);

  // 未拆分状态（17 - 工作台对齐：Step2 平行可达，未拆分时空态引导而非封锁）
  const notSplit =
    !!proj && ["DRAFT", "SCRIPT_SPLITTING", "SCRIPT_SPLIT_FAILED", "CANCELLED"].includes(proj.status);

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

  // 拉取全部资产数据（silent=true 用于轮询刷新：失败静默）
  const loadAssets = useCallback(async (silent = false) => {
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
      // 限流治理：轮询刷新失败静默（避免 429 反复弹 toast），仅初始加载提示
      if (!silent) toast.error(errMsg(e, "加载资产失败"));
      return null;
    }
  }, [activeEpisode]);

  useEffect(() => {
    void loadAssets();
  }, [loadAssets]);

  // 有任务在跑：WS 终态推送驱动刷新（三类图生成任务），断线回退 5s 轮询（对齐 LuxReal 推送为主）
  const anyRunning =
    chars.some((c) => isRunning(c.generation_status)) ||
    scenes.some((s) => isRunning(s.generation_status)) ||
    props.some((p) => isRunning(p.generation_status));

  const assetsDoneRef = useRef(false);
  const refreshAssets = useCallback(async () => {
    const data = await loadAssets(true);
    if (!data) return;
    const still =
      data.chars.some((c) => isRunning(c.generation_status)) ||
      data.scenes.some((s) => isRunning(s.generation_status)) ||
      data.props.some((p) => isRunning(p.generation_status));
    if (!still && !assetsDoneRef.current) {
      assetsDoneRef.current = true;
      toast.success("资产生成完成");
    }
  }, [loadAssets]);

  useWsDrivenPoll(refreshAssets, {
    active: anyRunning,
    pollMs: 5000,
    match: matchTaskTypes(["CHAR_IMAGE_GEN", "SCENE_IMAGE_GEN", "PROP_IMAGE_GEN"]),
  });

  // 有任务重新在跑（新一轮批量生成）→ 重置终态守卫
  useEffect(() => {
    if (anyRunning) assetsDoneRef.current = false;
  }, [anyRunning]);

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
      const data = await assetsApi.batchGenerate({
        episode_id: activeEpisode,
        items,
        model_code: batchModel || undefined,
        style_name: batchStyle || undefined,
        per_asset_count: batchCount,
      });
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

  // 重新提取本集资产（剧本修改后一键重跑资产提取，对齐 LuxReal）
  const reextractAssets = async () => {
    if (!activeEpisode) return;
    setReextracting(true);
    try {
      const d = await api.post<{ characters: number; scenes: number; props: number; message?: string }>(
        `/episodes/${activeEpisode}/reextract-assets`, {},
      );
      toast.success(d.message || `已重新提取：角色 ${d.characters}、场景 ${d.scenes}、道具 ${d.props}`);
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "资产重新提取失败"));
    } finally {
      setReextracting(false);
    }
  };

  // 角色编辑弹窗（PATCH /characters/:character_id：name/description/appearance_prompt/tags）
  const [charEdit, setCharEdit] = useState<CharacterItem | null>(null);
  const [ceName, setCeName] = useState("");
  const [ceDesc, setCeDesc] = useState("");
  const [ceAppearance, setCeAppearance] = useState("");
  const [ceTags, setCeTags] = useState("");
  const [savingChar, setSavingChar] = useState(false);

  const openCharEdit = (c: CharacterItem) => {
    setCharEdit(c);
    setCeName(c.name);
    setCeDesc(c.description_notes || "");
    setCeAppearance("");
    setCeTags(parseTags(c.tags).join(", "));
  };

  const saveCharEdit = async () => {
    if (!charEdit) return;
    if (!ceName.trim()) {
      toast.error("角色名称不能为空");
      return;
    }
    setSavingChar(true);
    try {
      await api.patch(`/characters/${charEdit.id}`, {
        name: ceName.trim(),
        description: ceDesc.trim(),
        appearance_prompt: ceAppearance.trim(),
        tags: ceTags.split(/[,，]/).map((t) => t.trim()).filter(Boolean),
      });
      toast.success("角色资料已更新");
      setCharEdit(null);
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "角色更新失败"));
    } finally {
      setSavingChar(false);
    }
  };

  // 场景编辑弹窗（PATCH /scenes/:scene_id：base_name/base_prompt/time_of_day/weather）
  const [sceneEdit, setSceneEdit] = useState<SceneItem | null>(null);
  const [seName, setSeName] = useState("");
  const [sePrompt, setSePrompt] = useState("");
  const [savingScene, setSavingScene] = useState(false);

  const openSceneEdit = (s: SceneItem) => {
    setSceneEdit(s);
    setSeName(s.base_name);
    setSePrompt(""); // 场景无独立 prompt 字段，仅名称+基础提示词
  };

  const saveSceneEdit = async () => {
    if (!sceneEdit) return;
    if (!seName.trim()) {
      toast.error("场景名称不能为空");
      return;
    }
    setSavingScene(true);
    try {
      await api.patch(`/scenes/${sceneEdit.scene_id}`, {
        base_name: seName.trim(),
        base_prompt: sePrompt.trim(),
      });
      toast.success("场景资料已更新");
      setSceneEdit(null);
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "场景更新失败"));
    } finally {
      setSavingScene(false);
    }
  };

  // 道具编辑弹窗（PATCH /props/:prop_id：name/description/category）
  const [propEdit, setPropEdit] = useState<PropItem | null>(null);
  const [peName, setPeName] = useState("");
  const [peDesc, setPeDesc] = useState("");
  const [peCategory, setPeCategory] = useState("");
  const [savingProp, setSavingProp] = useState(false);

  const openPropEdit = (p: PropItem) => {
    setPropEdit(p);
    setPeName(p.name);
    setPeDesc(p.description || "");
    setPeCategory(p.category || "");
  };

  const savePropEdit = async () => {
    if (!propEdit) return;
    if (!peName.trim()) {
      toast.error("道具名称不能为空");
      return;
    }
    setSavingProp(true);
    try {
      await api.patch(`/props/${propEdit.prop_id}`, {
        name: peName.trim(),
        description: peDesc.trim(),
        category: peCategory.trim(),
      });
      toast.success("道具资料已更新");
      setPropEdit(null);
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "道具更新失败"));
    } finally {
      setSavingProp(false);
    }
  };

  // 角色变体弹窗（POST /characters/:id/variants）
  const [variantChar, setVariantChar] = useState<CharacterItem | null>(null);
  // 资产详情弹窗（点击主图打开，对齐 LuxReal）
  const [assetDialog, setAssetDialog] = useState<{ kind: DialogKind; targetId: number } | null>(null);
  const [variantName, setVariantName] = useState("");
  const [variantType, setVariantType] = useState("COSTUME");
  const [variantClothing, setVariantClothing] = useState("");
  const [variantExtraPrompt, setVariantExtraPrompt] = useState("");
  const [savingVariant, setSavingVariant] = useState(false);

  const openVariantModal = (ch: CharacterItem) => {
    setVariantChar(ch);
    setVariantName("");
    setVariantType("COSTUME");
    setVariantClothing("");
    setVariantExtraPrompt("");
  };

  const saveVariant = async () => {
    if (!variantChar) return;
    if (!variantName.trim()) {
      toast.error("造型名称不能为空");
      return;
    }
    setSavingVariant(true);
    try {
      await api.post(`/characters/${variantChar.id}/variants`, {
        variant_name: variantName.trim(),
        variant_type: variantType,
        clothing_desc: variantClothing.trim(),
        extra_appearance_prompt: variantExtraPrompt.trim(),
      });
      toast.success(`已为「${variantChar.name}」创建造型「${variantName}」`);
      setVariantChar(null);
      void loadAssets();
    } catch (e) {
      toast.error(errMsg(e, "创建造型失败"));
    } finally {
      setSavingVariant(false);
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
      stepDone={stepState.stepDone}
      episodes={episodes}
      activeEpisodeId={activeEpisode}
      onSelectEpisode={setActiveEpisode}
    >
      {/* Project Settings Bar */}
      <div className="mx-6 mt-2 flex items-center justify-between h-9 px-4 rounded-xl flex-shrink-0" style={{ background: "#16161C", border: "1px solid #2A2A35" }}>
        <span className="text-white font-medium text-sm truncate">{proj?.title ?? "加载中…"}</span>
        <div className="flex items-center gap-2">
          <div className="relative">
            <button
              className="flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white"
              style={{ background: "#1E1E24", border: "1px solid #2A2A35" }}
              onClick={() => setImgModelOpen((v) => !v)}
            >
              {imgModels.find((m) => m.model_key === imgModel)?.name || imgModel || "Lux Image 2"}
              <ChevronDown size={12} />
            </button>
            {imgModelOpen && (
              <div
                className="absolute right-0 top-full mt-1 z-50 rounded-lg border border-[#2A2A35] bg-[#1B1B22] shadow-xl p-1 min-w-[180px]"
                onMouseLeave={() => setImgModelOpen(false)}
              >
                {imgModels.length === 0 && (
                  <div className="px-3 py-2 text-xs text-[#8B8B9A]">暂无可用图片模型</div>
                )}
                {imgModels.map((m) => (
                  <button
                    key={m.model_key}
                    className="flex items-center justify-between w-full gap-3 px-3 py-1.5 rounded-md text-xs text-left text-white/80 hover:bg-white/5 disabled:opacity-40 disabled:cursor-not-allowed"
                    disabled={!m.enabled}
                    onClick={() => void pickImageModel(m.model_key)}
                  >
                    <span>{m.name || m.model_key}</span>
                    {m.cost_points > 0 && <span className="text-[10px] text-[#F5B23C]">{m.cost_points} 积分</span>}
                  </button>
                ))}
              </div>
            )}
          </div>
          <button className="flex items-center gap-1 text-xs rounded-lg px-2.5 py-1 text-white/80 transition-colors hover:text-white" style={{ background: "#1E1E24", border: "1px solid #2A2A35" }}>
            {proj?.tags?.[0] || "武侠仙侠"}
            <ChevronDown size={12} />
          </button>
          <button
            className="flex items-center gap-1 text-xs text-[#6B7088] hover:text-white transition-colors"
            onClick={() => void openTemplates()}
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
          ["shots", "智能镜头", false],
          ["materials", "通用素材", false],
        ] as const).map(([key, label, disabled]) => (
          <button
            key={key}
            disabled={disabled}
            onClick={() => !disabled && setTab(key as TabKey)}
            title={disabled ? "后续版本开放" : key === "shots" ? "智能镜头（HeroShotGroup 预制镜头方案）待开发，当前为镜头状态只读汇总" : undefined}
            className={`px-4 py-3 text-sm border-b-2 transition-colors ${
              disabled
                ? "text-[#3A3A4A] border-transparent cursor-not-allowed"
                : tab === key
                  ? "text-white border-[#4C7FF8] font-medium"
                  : "text-[#6B7088] border-transparent hover:text-white cursor-pointer"
            }`}
          >
            {label}
            {key === "shots" && (
              <span className="ml-1.5 text-[9px] px-1 py-0.5 rounded align-middle" style={{ background: "rgba(245,178,60,0.15)", color: "#F5B23C" }}>
                待开发
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Content Scroll Area */}
      <div className="flex-1 overflow-y-auto scrollbar-thin px-6 py-4 pb-24">
        {notSplit ? (
          /* 未拆分空态引导（17 - 工作台对齐：可浏览 + 引导，不封锁） */
          <div className="rounded-2xl border border-dashed border-[#2A2A35] p-16 flex flex-col items-center gap-4 text-center">
            <Wand2 size={32} className="text-[#6B7088]" />
            <div className="text-sm text-white/70">剧本拆分完成后，将自动提取角色 / 场景 / 道具资产</div>
            <button
              className="h-10 px-6 rounded-xl flex items-center gap-2 text-sm text-white font-medium transition-all hover:opacity-90"
              style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
              onClick={() => stepState.goStep(1)}
            >
              前往剧本拆分
            </button>
          </div>
        ) : (
        <>
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
                    onEdit={(c) => openCharEdit(c)}
                    onAddVariant={openVariantModal}
                    onOpenDialog={(c) => setAssetDialog({ kind: "CHARACTER_VARIANT", targetId: c.id })}
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
                  <SceneCard key={sc.scene_id} sc={sc} index={i} busy={submitting} onRegen={() => void regenOne("SCENE", sc.scene_id)} onEdit={openSceneEdit} onOpenDialog={(s) => setAssetDialog({ kind: "SCENE", targetId: s.scene_id })} />
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
                  <PropCard key={p.prop_id} prop={p} index={i} busy={submitting} onRegen={() => void regenOne("PROP", p.prop_id)} onEdit={openPropEdit} onOpenDialog={(pp) => setAssetDialog({ kind: "PROP", targetId: pp.prop_id })} />
                ))}
              </div>
            )}
          </div>
        )}

        {/* Section: 智能镜头（汇总当前剧集全部叙事单元的镜头，GET /beats/:id/shots） */}
        {tab === "shots" && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm text-[#8B8B9A] font-medium">智能镜头</h3>
              <span className="text-xs text-[#6B7088]">
                {Object.values(shotMap).reduce((n, s) => n + s.length, 0)} 个镜头 · 按叙事单元分组
              </span>
            </div>
            {shotsLoading ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">加载镜头列表…</div>
            ) : beats.length === 0 ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">
                暂无叙事单元。完成剧本拆解后镜头将出现在这里。
              </div>
            ) : (
              <div className="space-y-4">
                {beats.map((b) => {
                  const shots = shotMap[b.beat_id] ?? [];
                  return (
                    <div key={b.beat_id} className="rounded-xl bg-[#1E1E24] border border-[#2A2A35] overflow-hidden">
                      <div className="flex items-center justify-between px-4 py-2.5 border-b border-[#2A2A35]">
                        <span className="text-sm text-white font-medium truncate">
                          {b.sort_order}. {b.title}
                        </span>
                        <span className="text-xs text-[#6B7088] flex-shrink-0 ml-3">{shots.length} 镜头</span>
                      </div>
                      {shots.length === 0 ? (
                        <div className="px-4 py-6 text-center text-xs text-[#6B7088]">该叙事单元暂无镜头（可在 Step3 生成视频后回看）</div>
                      ) : (
                        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3 p-4">
                          {shots.map((s) => (
                            <div key={s.shot_uuid} className="rounded-lg bg-[#16161C] border border-[#2A2A35] p-3">
                              <div className="flex items-center justify-between mb-2">
                                <span className="text-xs text-white font-medium">镜头 #{s.sort_order}</span>
                                <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(76,127,248,0.12)", color: "#4C7FF8" }}>
                                  {s.duration_sec}s
                                </span>
                              </div>
                              <div className="text-[11px] text-[#6B7088]">
                                {s.video_generation_status === "SUCCESS" ? (
                                  <span className="text-green-400">视频已生成</span>
                                ) : s.video_generation_status === "RUNNING" || s.video_generation_status === "QUEUED" ? (
                                  <span className="text-amber-gold">生成中…</span>
                                ) : (
                                  <span>待生成</span>
                                )}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

        {/* Section: 通用素材（项目资产库，GET /projects/:project_id/assets） */}
        {tab === "materials" && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm text-[#8B8B9A] font-medium">通用素材</h3>
              <span className="text-xs text-[#6B7088]">{materials.length} 项 · 项目维度共享</span>
            </div>
            {materialsLoading ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">加载素材库…</div>
            ) : materials.length === 0 ? (
              <div className="rounded-xl border border-dashed border-[#2A2A35] p-10 text-center text-sm text-[#6B7088]">
                暂无通用素材。角色/场景/道具生成后资产将沉淀到这里，可跨集复用。
              </div>
            ) : (
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
                {materials.map((m) => {
                  let preview = "";
                  try {
                    const files = JSON.parse(m.files_json || "{}") as Record<string, unknown>;
                    const url = (files.preview_url ?? files.url ?? files.main_image_url ?? "") as string;
                    if (url) preview = url;
                  } catch {
                    /* 忽略解析失败 */
                  }
                  return (
                    <div key={m.asset_id} className="rounded-xl bg-[#1E1E24] border border-[#2A2A35] overflow-hidden group">
                      {preview ? (
                        <img src={preview} alt={m.name} className="w-full h-24 object-cover" onError={(e) => { e.currentTarget.style.display = "none"; }} />
                      ) : (
                        <div className="w-full h-24 flex items-center justify-center bg-[#16161C]">
                          <ImageIcon size={28} className="text-[#4C7FF8]/20" />
                        </div>
                      )}
                      <div className="p-3">
                        <div className="text-sm text-white font-medium truncate">{m.name}</div>
                        <div className="flex items-center justify-between mt-1.5">
                          <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(245,178,60,0.12)", color: "#F5B23C" }}>
                            {ASSET_TYPE_LABEL[m.type] ?? m.type}
                          </span>
                          <span className={`text-[10px] ${m.status === "SUCCESS" ? "text-green-400" : m.status === "FAILED" ? "text-red-400" : "text-amber-gold"}`}>
                            {m.status === "SUCCESS" ? "已生成" : m.status === "FAILED" ? "失败" : "生成中"}
                          </span>
                        </div>
                      </div>
                    </div>
                  );
                })}
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
        </>
        )}
      </div>

      {/* Bottom Floating Action Bar（未拆分时隐藏，避免空态误导操作） */}
      {!notSplit && (
      <div className="fixed bottom-6 left-1/2 -translate-x-1/2 flex items-center gap-3 z-40">
        <button
          className="bg-[#1E1E24] border border-[#2A2A35] text-[#C0C0CC] text-sm px-5 py-2.5 rounded-full hover:bg-[#252535] transition-colors flex items-center gap-2 disabled:opacity-40 disabled:cursor-not-allowed"
          onClick={() => void reextractAssets()}
          disabled={reextracting || anyRunning}
        >
          <RefreshCw size={16} className={reextracting ? "animate-spin" : ""} />
          {reextracting ? "提取中…" : "重新提取本集资产"}
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
      )}

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
              {/* Model（真实透传：选中模型作为生成任务 model_code） */}
              <div>
                <label className="text-xs text-[#8B8B9A] mb-1.5 block">模型</label>
                <div className="relative">
                  <select
                    value={batchModel}
                    onChange={(e) => setBatchModel(e.target.value)}
                    className="w-full h-9 px-3 rounded-lg bg-[#0D0D14] border border-[#2A2A3E] text-white text-sm focus:border-[#4C7FF8] focus:outline-none transition-colors appearance-none cursor-pointer"
                  >
                    <option value="">自动路由（推荐）</option>
                    {imgModels.map((m) => (
                      <option key={m.model_key} value={m.model_key} disabled={!m.enabled}>
                        {m.name}
                      </option>
                    ))}
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

      {/* ===== 角色编辑弹窗（PATCH /characters/:id） ===== */}
      {charEdit && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: "rgba(5,5,15,0.72)", backdropFilter: "blur(6px)" }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setCharEdit(null);
          }}
        >
          <div
            className="w-[520px] max-h-[80vh] flex flex-col rounded-2xl border border-[#2A2A35]"
            style={{ background: "#15151D", boxShadow: "0 24px 80px rgba(0,0,0,0.6)" }}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A35]">
              <div>
                <div className="text-[15px] font-semibold text-white">角色编辑 - {charEdit.name}</div>
                <div className="text-xs text-[#8B8B9A] mt-0.5">修改角色设定，保存后影响后续生成（PATCH /characters/:id）</div>
              </div>
              <button onClick={() => setCharEdit(null)} className="text-[#8B8B9A] hover:text-white transition-colors" aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 min-h-0 overflow-y-auto p-5 space-y-4">
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">角色名称</label>
                <input
                  type="text"
                  value={ceName}
                  maxLength={64}
                  onChange={(e) => setCeName(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">角色描述</label>
                <textarea
                  rows={2}
                  value={ceDesc}
                  onChange={(e) => setCeDesc(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors resize-none"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">外观描述（生成 prompt 前缀）</label>
                <textarea
                  rows={3}
                  value={ceAppearance}
                  placeholder="如：干练老板娘装，发髻高盘，眉眼锐利…"
                  onChange={(e) => setCeAppearance(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors resize-none"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">标签（逗号分隔）</label>
                <input
                  type="text"
                  value={ceTags}
                  placeholder="如：老板娘, 古装, 反派"
                  onChange={(e) => setCeTags(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-3 px-5 py-4 border-t border-[#2A2A35]">
              <button onClick={() => setCharEdit(null)} className="px-4 py-2 rounded-lg text-sm text-white/70 hover:text-white transition-colors">取消</button>
              <button
                onClick={() => void saveCharEdit()}
                disabled={savingChar}
                className="px-4 py-2 rounded-lg text-sm font-semibold text-[#0B0B1A] bg-gradient-to-r from-amber-gold to-amber-dim hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              >
                {savingChar ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== 场景编辑弹窗（PATCH /scenes/:id） ===== */}
      {sceneEdit && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: "rgba(5,5,15,0.72)", backdropFilter: "blur(6px)" }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setSceneEdit(null);
          }}
        >
          <div
            className="w-[520px] max-h-[80vh] flex flex-col rounded-2xl border border-[#2A2A35]"
            style={{ background: "#15151D", boxShadow: "0 24px 80px rgba(0,0,0,0.6)" }}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A35]">
              <div>
                <div className="text-[15px] font-semibold text-white">场景编辑 - {sceneEdit.base_name}</div>
                <div className="text-xs text-[#8B8B9A] mt-0.5">修改场景设定，保存后影响后续生成（PATCH /scenes/:id）</div>
              </div>
              <button onClick={() => setSceneEdit(null)} className="text-[#8B8B9A] hover:text-white transition-colors" aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 min-h-0 overflow-y-auto p-5 space-y-4">
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">场景名称</label>
                <input
                  type="text"
                  value={seName}
                  maxLength={128}
                  onChange={(e) => setSeName(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">基础提示词（生成 prompt 前缀）</label>
                <textarea
                  rows={3}
                  value={sePrompt}
                  placeholder="如：现代都市夜景，霓虹灯闪烁，雨后街道…"
                  onChange={(e) => setSePrompt(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors resize-none"
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-3 px-5 py-4 border-t border-[#2A2A35]">
              <button onClick={() => setSceneEdit(null)} className="px-4 py-2 rounded-lg text-sm text-white/70 hover:text-white transition-colors">取消</button>
              <button
                onClick={() => void saveSceneEdit()}
                disabled={savingScene}
                className="px-4 py-2 rounded-lg text-sm font-semibold text-[#0B0B1A] bg-gradient-to-r from-amber-gold to-amber-dim hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              >
                {savingScene ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== 道具编辑弹窗（PATCH /props/:id） ===== */}
      {propEdit && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: "rgba(5,5,15,0.72)", backdropFilter: "blur(6px)" }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setPropEdit(null);
          }}
        >
          <div
            className="w-[520px] max-h-[80vh] flex flex-col rounded-2xl border border-[#2A2A35]"
            style={{ background: "#15151D", boxShadow: "0 24px 80px rgba(0,0,0,0.6)" }}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A35]">
              <div>
                <div className="text-[15px] font-semibold text-white">道具编辑 - {propEdit.name}</div>
                <div className="text-xs text-[#8B8B9A] mt-0.5">修改道具设定，保存后影响后续生成（PATCH /props/:id）</div>
              </div>
              <button onClick={() => setPropEdit(null)} className="text-[#8B8B9A] hover:text-white transition-colors" aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 min-h-0 overflow-y-auto p-5 space-y-4">
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">道具名称</label>
                <input
                  type="text"
                  value={peName}
                  maxLength={128}
                  onChange={(e) => setPeName(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">道具描述</label>
                <textarea
                  rows={2}
                  value={peDesc}
                  onChange={(e) => setPeDesc(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors resize-none"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">分类</label>
                <input
                  type="text"
                  value={peCategory}
                  placeholder="如：武器, 服饰, 道具"
                  onChange={(e) => setPeCategory(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-3 px-5 py-4 border-t border-[#2A2A35]">
              <button onClick={() => setPropEdit(null)} className="px-4 py-2 rounded-lg text-sm text-white/70 hover:text-white transition-colors">取消</button>
              <button
                onClick={() => void savePropEdit()}
                disabled={savingProp}
                className="px-4 py-2 rounded-lg text-sm font-semibold text-[#0B0B1A] bg-gradient-to-r from-amber-gold to-amber-dim hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              >
                {savingProp ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== 角色变体弹窗（POST /characters/:id/variants） ===== */}
      {variantChar && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: "rgba(5,5,15,0.72)", backdropFilter: "blur(6px)" }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setVariantChar(null);
          }}
        >
          <div
            className="w-[480px] flex flex-col rounded-2xl border border-[#2A2A35]"
            style={{ background: "#15151D", boxShadow: "0 24px 80px rgba(0,0,0,0.6)" }}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A35]">
              <div>
                <div className="text-[15px] font-semibold text-white">添加造型 - {variantChar.name}</div>
                <div className="text-xs text-[#8B8B9A] mt-0.5">为角色创建新的造型/服装变体（POST /characters/:id/variants）</div>
              </div>
              <button onClick={() => setVariantChar(null)} className="text-[#8B8B9A] hover:text-white transition-colors" aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="p-5 space-y-4">
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">造型名称</label>
                <input
                  type="text"
                  value={variantName}
                  maxLength={128}
                  placeholder="如：古装造型、现代休闲装、战斗形态…"
                  onChange={(e) => setVariantName(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">造型类型</label>
                <select
                  value={variantType}
                  onChange={(e) => setVariantType(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                >
                  <option value="COSTUME">服装（换装）</option>
                  <option value="HAIRSTYLE">发型</option>
                  <option value="MAKEUP">妆容</option>
                  <option value="AGE_PROGRESSION">年龄变化</option>
                  <option value="INJURY_DIRTY">受伤/脏污</option>
                  <option value="SPECIAL_OUTFIT_UNIFORM">特殊制服</option>
                  <option value="CASUAL">休闲装</option>
                  <option value="FORMAL">正装</option>
                </select>
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">服装描述（clothing_desc）</label>
                <input
                  type="text"
                  value={variantClothing}
                  maxLength={256}
                  placeholder="如：浅蓝色衬衫+深灰西装+干练知性"
                  onChange={(e) => setVariantClothing(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-[#8B8B9A] mb-1.5">额外外观提示词（在基础提示词上追加）</label>
                <textarea
                  rows={2}
                  value={variantExtraPrompt}
                  maxLength={512}
                  placeholder="如：头发扎起高马尾，眼神锐利，佩戴银色耳环…"
                  onChange={(e) => setVariantExtraPrompt(e.target.value)}
                  className="w-full bg-white/5 border border-[#2A2A35] rounded-lg px-3 py-2 text-sm text-white placeholder-[#8B8B9A] focus:outline-none focus:border-[#4C7FF8]/50 transition-colors resize-none"
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-3 px-5 py-4 border-t border-[#2A2A35]">
              <button onClick={() => setVariantChar(null)} className="px-4 py-2 rounded-lg text-sm text-white/70 hover:text-white transition-colors">取消</button>
              <button
                onClick={() => void saveVariant()}
                disabled={savingVariant}
                className="px-4 py-2 rounded-lg text-sm font-semibold text-[#0B0B1A] bg-gradient-to-r from-amber-gold to-amber-dim hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              >
                {savingVariant ? "创建中…" : "创建造型"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== 模板管理弹窗（GET /templates + POST /templates/:id/apply） ===== */}
      {templateOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: "rgba(5,5,15,0.72)", backdropFilter: "blur(6px)" }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setTemplateOpen(false);
          }}
        >
          <div
            className="w-[520px] max-h-[75vh] flex flex-col rounded-2xl border border-[#2A2A35]"
            style={{ background: "#15151D", boxShadow: "0 24px 80px rgba(0,0,0,0.6)" }}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A35]">
              <div>
                <div className="text-[15px] font-semibold text-white">模板管理</div>
                <div className="text-xs text-[#8B8B9A] mt-0.5">应用模板数据到当前项目（生态模板库）</div>
              </div>
              <button onClick={() => setTemplateOpen(false)} className="text-[#8B8B9A] hover:text-white transition-colors" aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 min-h-0 overflow-y-auto p-5 space-y-3">
              {templatesLoading ? (
                <div className="py-12 text-center text-sm text-[#8B8B9A]">加载模板…</div>
              ) : templates.length === 0 ? (
                <div className="py-12 text-center text-sm text-[#8B8B9A]">暂无可用模板（生态集市建设中）</div>
              ) : (
                templates.map((t) => (
                  <div key={t.template_id} className="flex items-center justify-between gap-4 rounded-xl bg-[#1E1E24] border border-[#2A2A35] px-4 py-3 hover:border-[#3A3A4A] transition-colors">
                    <div className="min-w-0">
                      <div className="text-sm text-white font-medium truncate">{t.name}</div>
                      {t.description && <div className="text-xs text-[#8B8B9A] mt-0.5 truncate">{t.description}</div>}
                    </div>
                    <button
                      onClick={() => void applyTemplate(t)}
                      className="flex-shrink-0 px-3 py-1.5 rounded-lg text-xs font-semibold text-[#0B0B1A] bg-gradient-to-r from-amber-gold to-amber-dim hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
                    >
                      应用
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      )}

      {/* 资产详情弹窗（点击主图打开，对齐 LuxReal） */}
      {assetDialog && activeEpisode && (
        <AssetDetailDialog
          kind={assetDialog.kind}
          targetId={assetDialog.targetId}
          episodeId={activeEpisode}
          onClose={() => setAssetDialog(null)}
          onChanged={() => void loadAssets()}
        />
      )}

    </WorkbenchShell>
  );
}
