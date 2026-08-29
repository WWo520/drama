// 资产详情弹窗（对齐 LuxReal 资产生成弹窗）：变体 tab + 主图操作 + 可编辑提示词
// （恢复默认/AI优化/参考图）+ 模型/风格/比例/默认构图 + 生成历史（收藏/导入/图改图）。
import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import {
  Download, Image as ImageIcon, LayoutGrid, List, Minus, Plus, RefreshCw,
  Sparkles, Star, Wand2, X,
} from "lucide-react";

import { api, assetsApi } from "@duanju/api-client";

import { matchTaskTypes, useWsDrivenPoll } from "../hooks/useWsDrivenPoll";
import { cachedGet } from "../utils/cachedApi";
import { getGlobalStyle, setGlobalStyle, STYLE_CHANGED_EVENT } from "../stores/styleStore";

// 公共风格库（LuxReal 27 项对齐）
const STYLES = [
  "通用写实", "古风写实", "武侠玄幻", "现代写实", "末世废土", "未来科幻", "国风2D", "国风3D",
  "美式卡通", "清新日漫", "韩式网漫", "邵氏武侠", "三渲二", "古风武侠", "王家卫美学", "徐克美学",
  "唯美写实", "幻彩插画", "Q版卡通", "彩铅绘本", "黏土动画", "淡雅水彩", "日系治愈", "双城风格",
  "中式梦核", "水墨写意", "无特定风格",
];
const RATIOS = ["16:9", "9:16", "1:1", "4:3", "3:4"];
const RESOLUTIONS = ["1K", "2K", "4K"];
// 场景时段选项（对齐 time_of_day_enum；仅 SCENE 显示）——场景日夜变体
const SCENE_TOD_OPTIONS = [
  { value: "", label: "默认时段" },
  { value: "DAY_BRIGHT", label: "白天" },
  { value: "MORNING_SUNRISE", label: "清晨" },
  { value: "AFTERNOON_GOLDEN_HOUR", label: "午后" },
  { value: "SUNSET_DUSK", label: "黄昏" },
  { value: "NIGHT_CANDLELIT", label: "夜晚·烛光" },
  { value: "NIGHT_NEON", label: "夜晚·霓虹" },
  { value: "OVERCAST_DAY", label: "阴天" },
  { value: "STORM_NIGHT", label: "暴风雨夜" },
];

export type DialogKind = "CHARACTER_VARIANT" | "SCENE" | "PROP";

interface VariantLite {
  id: number;
  name: string;
  main_image_url: string;
  generation_status: string;
  multi_view_images?: string[]; // 同一形象的多视图（对齐 LuxReal「新增视图」）
}

interface DialogData {
  kind: DialogKind;
  target_id: number;
  name: string;
  default_prompt: string;
  edited_prompt: string;
  main_image_url: string;
  generation_status: string;
  point_cost: number;
  variants?: VariantLite[];
}

interface HistoryItem {
  id: number;
  url: string;
  prompt: string;
  model_code: string;
  style_name: string;
  aspect_ratio: string;
  use_default_composition: boolean;
  is_favorite: boolean;
  created_at: string;
}

interface ModelOption {
  model_key: string;
  name: string;
  cost_points: number;
  enabled: boolean;
}

// 资产库导入项（角色/场景/道具主图 + 角色形象变体图）
interface LibraryItem {
  url: string;
  label: string;
  group: "角色" | "场景" | "道具";
}
// 角色精简项（资产库导入用，含形象变体）
interface CharacterLite {
  name: string;
  main_image_url: string;
  variants?: { name: string; main_image_url: string }[];
}

// presign 直传 → 回执 → 返回可访问 URL
async function uploadFile(file: File): Promise<string> {
  const p = await assetsApi.presign({
    biz_type: "asset_image", file_name: file.name, content_type: file.type || "image/png", size: file.size,
  });
  const resp = await fetch(p.upload_url, { method: "PUT", body: file, headers: { "Content-Type": file.type || "image/png" } });
  if (!resp.ok) throw new Error("上传失败");
  const etag = (resp.headers.get("ETag") || "").replace(/"/g, "");
  await assetsApi.complete({ upload_token: p.upload_token, object_key: p.object_key, etag, biz_ref_id: `asset-dialog-${Date.now()}` });
  return p.download_url;
}

export default function AssetDetailDialog({
  kind, targetId, episodeId, onClose, onChanged,
}: {
  kind: DialogKind;
  targetId: number;
  episodeId: string;
  onClose: () => void;
  onChanged: () => void;
}) {
  // 提示词质量评估函数
  const evaluatePrompt = (content: string): PromptQuality => {
    const result: PromptQuality = {
      score: 0,
      visualElements: 0,
      clarityScore: 1,
      specificty: 5,
      suggestions: [],
    };

    // 检测视觉元素
    const hasMaterial = /真丝 | 棉麻 | 羊毛 | 皮革 | 丝绸| 玻璃 | 金属| 钛金/.test(content);
    const hasColor = /红 | 蓝 | 绿 | 黄 | 黑 | 白 | 金 | 银/.test(content);
    const hasLighting = /光 | 影 | 亮 | 暗 | 明/.test(content);
    const hasSize = /厘米 | cm|m|毫米/.test(content);
    let elementsCount = 0;
    if (hasMaterial) elementsCount++;
    if (hasColor) elementsCount++;
    if (hasLighting) elementsCount++;
    if (hasSize) elementsCount++;
    result.visualElements = elementsCount;

    // 长度评分
    const length = content.length;
    const minLength = kind === "CHARACTER_VARIANT" ? 80 : kind === "SCENE" ? 100 : 40;
    if (length >= minLength) {
      result.score += 40;
    } else {
      result.score += (length / minLength) * 40;
    }

    // 清晰度：避免模糊词
    const vagueWords = ["漂亮", "豪华", "好看", "很", "非常", "特别"];
    let vagueCount = 0;
    vagueWords.forEach(word => {
      if (content.includes(word)) vagueCount++;
    });
    result.clarityScore = Math.max(0, 1 - vagueCount * 0.1);

    // 生成建议
    if (result.visualElements < 3) {
      result.suggestions.push("增加材质、色彩、光影等视觉元素描述");
    }
    if (result.clarityScore < 0.8) {
      result.suggestions.push("替换空泛形容词为具体视觉元素");
    }
    if (length < minLength) {
      result.suggestions.push(`扩展描述至${minLength}字以上`);
    }

    result.score = result.score * result.clarityScore * Math.min(result.visualElements / 5, 1);
    if (result.score > 100) result.score = 100;
    result.score = Math.round(result.score);

    return result;
  };

  const [data, setData] = useState<DialogData | null>(null);
  const [prompt, setPrompt] = useState("");
  const [useDefaultComp, setUseDefaultComp] = useState(true);
  const [modelCode, setModelCode] = useState("");
  const [styleName, setStyleName] = useState(() => getGlobalStyle().name);
  const [ratio, setRatio] = useState(kind === "PROP" ? "1:1" : "16:9");
  const [resolution, setResolution] = useState("1K");
  const [sceneTod, setSceneTod] = useState(""); // 场景时段（SCENE）
  const [models, setModels] = useState<ModelOption[]>([]);
  const [refImages, setRefImages] = useState<string[]>([]);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [historyMode, setHistoryMode] = useState<"card" | "list">("card");
  const [favOnly, setFavOnly] = useState(false);
  // 历史图选择模式（对齐 LuxReal 交互）：先点击选中卡片（高亮），底部出现「设为主图」操作条
  const [selectedHistoryId, setSelectedHistoryId] = useState<number | null>(null);
  const [activeVariant, setActiveVariant] = useState<number | null>(null);
  const [optimizing, setOptimizing] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [styleOpen, setStyleOpen] = useState(false);
  const [modelOpen, setModelOpen] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const refFileRef = useRef<HTMLInputElement>(null);

  // 参考图来源菜单（加号）：本地上传 / 资产库导入
  const [refMenuOpen, setRefMenuOpen] = useState(false);
  const [libraryOpen, setLibraryOpen] = useState(false);
  const [libraryLoading, setLibraryLoading] = useState(false);
  const [libraryItems, setLibraryItems] = useState<LibraryItem[]>([]);

  // 加载资产库图片（角色含形象变体 / 场景 / 道具主图），供「通过资产库导入」选择
  const openLibrary = async () => {
    setLibraryOpen(true);
    if (libraryItems.length) return;
    setLibraryLoading(true);
    try {
      const [chars, scenes, props] = await Promise.all([
        assetsApi.characters(episodeId).catch(() => ({ items: [] as CharacterLite[] })),
        assetsApi.scenes(episodeId).catch(() => ({ items: [] })),
        assetsApi.props(episodeId).catch(() => ({ items: [] })),
      ]);
      const items: LibraryItem[] = [];
      for (const c of (chars as { items: CharacterLite[] }).items || []) {
        if (c.main_image_url) items.push({ url: c.main_image_url, label: c.name, group: "角色" });
        for (const v of c.variants || []) {
          if (v.main_image_url) items.push({ url: v.main_image_url, label: `${c.name}·${v.name}`, group: "角色" });
        }
      }
      for (const s of (scenes as { items: { base_name: string; main_image_url: string }[] }).items || []) {
        if (s.main_image_url) items.push({ url: s.main_image_url, label: s.base_name, group: "场景" });
      }
      for (const p of (props as { items: { name: string; main_image_url: string }[] }).items || []) {
        if (p.main_image_url) items.push({ url: p.main_image_url, label: p.name, group: "道具" });
      }
      setLibraryItems(items);
    } finally {
      setLibraryLoading(false);
    }
  };

  // 从资产库导入参考图（去重）
  const importFromLibrary = (item: LibraryItem) => {
    setRefImages((prev) => (prev.includes(item.url) ? prev : [...prev, item.url]));
    toast.success("已从资产库导入参考图");
    setLibraryOpen(false);
  };

  const isChar = kind === "CHARACTER_VARIANT";

  // 提示词质量评估
  type PromptQuality = {
    score: number;
    visualElements: number;
    clarityScore: number;
    specificty: number;
    suggestions: string[];
  };
  const [quality, setQuality] = useState<PromptQuality | null>(null);
  const qualityRef = useRef<HTMLDivElement>(null);

  // 加载弹窗初始数据（silent=true 用于轮询刷新：失败静默，避免 429 反复弹 toast）
  const loadData = useCallback(async (silent = false) => {
    try {
      const d = await api.get<DialogData>(`/asset-dialog/${kind}/${targetId}/dialog`);
      setData(d);
      setPrompt(d.edited_prompt || d.default_prompt || "");
    } catch {
      if (!silent) toast.error("加载资产详情失败");
    }
  }, [kind, targetId]);

  // 加载生成历史
  const loadHistory = useCallback(async () => {
    try {
      const r = await api.get<{ items: HistoryItem[] }>(`/asset-dialog/${kind}/${targetId}/history`, {
        params: favOnly ? { favorite: "1" } : undefined,
      });
      setHistory(r.items || []);
    } catch {
      /* 历史加载失败不阻塞 */
    }
  }, [kind, targetId, favOnly]);

  useEffect(() => { void loadData(); }, [loadData]);
  useEffect(() => { void loadHistory(); }, [loadHistory]);

  // 模型真源（T2I；TTL 缓存与 Step2/首页共享，限流治理）
  useEffect(() => {
    void cachedGet<{ items: ModelOption[] }>("/models?capability=T2I", 60000)
      .then((r) => setModels((r.items || []).filter((m) => m.enabled)))
      .catch(() => { /* 回退空列表 */ });
  }, []);

  // 风格库真源（GET /style-library，与首页同源；失败回退公共风格库常量）
  const [styleOptions, setStyleOptions] = useState<string[]>(STYLES);
  useEffect(() => {
    void cachedGet<{ items: { style_code: string; name: string }[] }>("/style-library", 60000)
      .then((r) => {
        const names = (r.items || []).map((s) => s.name).filter(Boolean);
        if (names.length) setStyleOptions(names);
      })
      .catch(() => { /* 回退 STYLES */ });
  }, []);

  // 全局风格同步：首页/其他页面切换风格时实时跟随（双向：本页切换也写回全局）
  useEffect(() => {
    const onStyleChanged = (e: Event) => {
      const detail = (e as CustomEvent<{ name: string }>).detail;
      if (detail?.name) setStyleName(detail.name);
    };
    window.addEventListener(STYLE_CHANGED_EVENT, onStyleChanged);
    return () => window.removeEventListener(STYLE_CHANGED_EVENT, onStyleChanged);
  }, []);

  const changeStyle = (name: string) => {
    setStyleName(name);
    setStyleOpen(false);
    setGlobalStyle({ code: "", name });
  };

  const showReset = !!data?.edited_prompt || (data ? prompt !== data.default_prompt : false);

  // 保存提示词
  const savePrompt = useCallback(async (value: string) => {
    try {
      await api.put(`/asset-dialog/${kind}/${targetId}/prompt`, { prompt: value });
      // 评估质量
      setQuality(evaluatePrompt(value));
    } catch {
      toast.error("提示词保存失败");
    }
  }, [kind, targetId]);

  // 恢复默认提示词
  const resetPrompt = async () => {
    try {
      await api.post(`/asset-dialog/${kind}/${targetId}/prompt/reset`, {});
      if (data) setPrompt(data.default_prompt || "");
      toast.success("已恢复默认提示词");
      void loadData();
    } catch {
      toast.error("恢复默认失败");
    }
  };

  // AI 优化提示词（魔法棒）
  const optimizePrompt = async () => {
    if (!prompt.trim()) { toast.info("提示词为空，无法优化"); return; }
    setOptimizing(true);
    try {
      const r = await api.post<{ optimized_prompt: string; is_fallback: boolean }>(
        `/asset-dialog/${kind}/${targetId}/prompt/optimize`, { prompt });
      setPrompt(r.optimized_prompt);
      await savePrompt(r.optimized_prompt);
      toast.success(r.is_fallback ? "未配置模型，保持原提示词" : "AI 优化完成");
    } catch {
      toast.error("AI 优化失败");
    } finally {
      setOptimizing(false);
    }
  };

  // 生成观察窗口（替换原 10×3s 轮询）：WS 终态推送驱动刷新（图生成任务），
  // 断线回退 3s 轮询；新图落地（历史条数增长）自动结束，75s 硬超时兜底
  const [watching, setWatching] = useState(false);
  const watchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const historyCountRef = useRef(0);

  const refreshGen = useCallback(async () => {
    await Promise.all([loadHistory(), loadData(true)]);
    onChanged();
  }, [loadHistory, loadData, onChanged]);

  useWsDrivenPoll(refreshGen, {
    active: watching,
    pollMs: 3000,
    slowPollMs: 15000,
    match: matchTaskTypes(["CHAR_IMAGE_GEN", "SCENE_IMAGE_GEN", "PROP_IMAGE_GEN"]),
  });

  // 新图落地（历史条数增长）→ 结束观察
  useEffect(() => {
    if (watching && history.length > historyCountRef.current) setWatching(false);
  }, [watching, history]);

  // 卸载清理硬超时
  useEffect(() => () => {
    if (watchTimeoutRef.current) clearTimeout(watchTimeoutRef.current);
  }, []);

  // 生成（携带全参数）
  const generate = async () => {
    setGenerating(true);
    try {
      await savePrompt(prompt);
      // 角色选中具体形象时传 variant id：只生成/更新该形象（后端按 variant id 精确回写主图）；
      // 未选中形象时传 character id（角色级，更新默认形象）。
      const genTargetId = isChar && activeVariant ? activeVariant : targetId;
      await assetsApi.batchGenerate({
        episode_id: episodeId,
        items: [{
          kind, target_id: genTargetId,
          gen: {
            prompt, model_code: modelCode, style_name: styleName,
            aspect_ratio: ratio, resolution,
            use_default_composition: isChar && useDefaultComp,
            ref_image_urls: refImages.length ? refImages : undefined,
            time_of_day: isChar || !sceneTod ? undefined : sceneTod,
          },
        }],
      });
      toast.success("生成任务已提交");
      // 开启观察窗口：WS 终态推送/轮询兜底刷新，新图落地自动结束
      historyCountRef.current = history.length;
      setWatching(true);
      if (watchTimeoutRef.current) clearTimeout(watchTimeoutRef.current);
      watchTimeoutRef.current = setTimeout(() => setWatching(false), 75000);
      void refreshGen();
    } catch {
      toast.error("生成任务提交失败");
    } finally {
      setGenerating(false);
    }
  };

  // 替换主图 / 导入历史（presign 直传）
  const onPickFile = async (file: File, asMain: boolean) => {
    try {
      const url = await uploadFile(file);
      if (asMain) {
        await api.post(`/asset-dialog/${kind}/${targetId}/set-image`, { image_url: url });
        toast.success("主图已替换");
        void loadData();
        onChanged();
      } else {
        setRefImages((prev) => [...prev, url]);
        toast.success("已添加参考图");
      }
      void loadHistory();
    } catch {
      toast.error("上传失败");
    }
  };

  // 修改图片（图改图）：主图设为参考图 + 清空提示词
  const modifyImage = () => {
    if (!data?.main_image_url) { toast.info("暂无主图可修改"); return; }
    setRefImages((prev) => [data.main_image_url, ...prev.filter((u) => u !== data.main_image_url)]);
    setPrompt("");
    toast.info("已进入图改图模式：主图已设为参考图，请输入修改指令");
  };

  // 收藏切换
  const toggleFav = async (id: number) => {
    try {
      await api.post(`/generated-images/${id}/favorite`, {});
      void loadHistory();
    } catch {
      toast.error("收藏操作失败");
    }
  };

  // 历史图切换为主图（对齐 LuxReal）：复用 set-image，将选中的历史图设为主形象/主封面
  const setAsMain = async (h: HistoryItem) => {
    try {
      await api.post(`/asset-dialog/${kind}/${targetId}/set-image`, { image_url: h.url });
      toast.success("已切换为主图");
      void loadData();
      onChanged();
    } catch {
      toast.error("切换主图失败");
    }
  };

  // 创建新形象（角色）：创建后切到新形象的空白图片模板，并清空提示词输入框（从零开始设计）
  const createVariant = async () => {
    if (!isChar) return;
    try {
      const r = await api.post<{ variant_id: number }>(`/characters/${targetId}/variants`, {
        variant_name: `未命名形象${(data?.variants?.length || 0) + 1}`, variant_type: "COSTUME",
      });
      toast.success("已创建空白形象，请输入提示词后生成");
      setPrompt(""); // 清空提示词：新形象从零设计，避免沿用旧形象提示词
      setRefImages([]); // 参考图同步清空
      if (r?.variant_id) setActiveVariant(r.variant_id); // 切到新形象（空白图片模板）
      void loadData();
      onChanged();
    } catch {
      toast.error("创建新形象失败");
    }
  };

  // 从剧本自动生成形象（需求2：对齐 LuxReal，按剧本场景/状态提取服装并批量创建+生成）
  const [genVariantsLoading, setGenVariantsLoading] = useState(false);
  const generateVariantsFromScript = async () => {
    if (!isChar) return;
    setGenVariantsLoading(true);
    try {
      const r = await api.post<{ created: number; message?: string }>(
        `/characters/${targetId}/generate-variants`, {},
      );
      if (r.created > 0) toast.success(`已按剧本提取并创建 ${r.created} 个形象，正在生成`);
      else toast.info(r.message || "未提取到可用形象");
      void loadData();
      onChanged();
    } catch {
      toast.error("自动生成形象失败");
    } finally {
      setGenVariantsLoading(false);
    }
  };

  const cost = data?.point_cost || 6;
   // 主图跟随选中形象：选中形象有图显示其图；无图显示空白模板（不兜底角色主图，
   // 否则形象切换看起来"没变化"）；未选形象显示角色默认主图。
   const mainUrl = isChar && activeVariant
     ? (data?.variants?.find((v) => v.id === activeVariant)?.main_image_url || "")
     : (data?.main_image_url || "");

  return (
    <div className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-6" onClick={onClose}>
      <div
        className="bg-[#16161C] border border-[#2A2A35] rounded-2xl w-full max-w-6xl max-h-[90vh] flex flex-col shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 顶部：名称 + 变体 tab + 关闭 */}
        <div className="flex items-center gap-3 px-5 py-3 border-b border-[#2A2A35]">
          <span className="text-white font-semibold text-base">{data?.name || "…"}</span>
          {isChar && (
            <div className="flex items-center gap-2 overflow-x-auto">
              {(data?.variants || []).map((v) => (
                <button
                  key={v.id}
                  onClick={() => setActiveVariant(v.id === activeVariant ? null : v.id)}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs whitespace-nowrap border transition-colors ${
                    activeVariant === v.id || (!activeVariant && v.id === (data?.variants?.[0]?.id ?? -1))
                      ? "border-[#4C7FF8] bg-[#4C7FF8]/10 text-white"
                      : "border-[#2A2A35] bg-[#1E1E24] text-[#8B8B9A] hover:border-[#3A3A4A]"
                  }`}
                >
                  {v.main_image_url
                    ? <img src={v.main_image_url} alt="" className="w-5 h-5 rounded object-cover" />
                    : <span className="w-5 h-5 rounded bg-[#2A2A35]" />}
                  {v.name}
                </button>
              ))}
              <button
                onClick={() => void createVariant()}
                className="flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs border border-dashed border-[#3A3A4A] text-[#8B8B9A] hover:border-[#4C7FF8] hover:text-[#4C7FF8] whitespace-nowrap"
              >
                <Plus size={12} /> 创建新形象
              </button>
              <button
                onClick={() => void generateVariantsFromScript()}
                disabled={genVariantsLoading}
                className="flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs border border-dashed border-[#3A3A4A] text-[#8B8B9A] hover:border-[#8B5CF6] hover:text-[#8B5CF6] whitespace-nowrap disabled:opacity-40"
                title="按剧本文本自动提取该角色在不同场景/状态下的服装形象并批量生成"
              >
                <Wand2 size={12} className={genVariantsLoading ? "animate-pulse" : ""} /> 自动生成形象
              </button>
            </div>
          )}
          <button onClick={onClose} className="ml-auto w-8 h-8 rounded-lg flex items-center justify-center text-[#8B8B9A] hover:bg-white/5 hover:text-white">
            <X size={18} />
          </button>
        </div>

        {/* 主体：左主图+提示词 / 右历史 */}
        <div className="flex flex-1 min-h-0 gap-4 p-5">
          {/* 左列 */}
          <div className="flex-1 flex flex-col gap-4 min-w-0">
            {/* 主图区 */}
            <div className="relative rounded-xl overflow-hidden bg-[#1E1E24] border border-[#2A2A35] aspect-video flex items-center justify-center">
              {mainUrl ? (
                <img src={mainUrl} alt={data?.name} className="max-h-full max-w-full object-contain" />
              ) : (
                <div className="flex flex-col items-center gap-2 text-[#3A3A4A]">
                  <ImageIcon size={48} />
                  <span className="text-xs">暂无主形象图</span>
                </div>
              )}
              {mainUrl && (
                <>
                  <div className="absolute top-2 left-2 flex items-center gap-1.5">
                    <span className="px-2 py-0.5 rounded bg-black/50 text-white text-xs">主图</span>
                    <button className="w-7 h-7 rounded bg-black/50 flex items-center justify-center text-white/80 hover:text-yellow-300" title="收藏主图">
                      <Star size={14} />
                    </button>
                  </div>
                  <a
                    href={mainUrl} download target="_blank" rel="noreferrer"
                    className="absolute top-2 right-2 w-7 h-7 rounded bg-black/50 flex items-center justify-center text-white/80 hover:text-white"
                    title="下载"
                  >
                    <Download size={14} />
                  </a>
                </>
              )}
              <div className="absolute bottom-2 right-2 flex items-center gap-1.5">
                <button
                  onClick={() => fileRef.current?.click()}
                  className="px-2.5 py-1 rounded bg-black/60 text-white/80 text-xs hover:text-white"
                >
                  替换
                </button>
                <button
                  onClick={() => { if (data) setPrompt(data.edited_prompt || data.default_prompt || ""); toast.info("已回填当前主图生成参数"); }}
                  className="px-2.5 py-1 rounded bg-black/60 text-white/80 text-xs hover:text-white"
                >
                  重新编辑
                </button>
                <button
                  onClick={modifyImage}
                  className="px-2.5 py-1 rounded bg-[#4C7FF8] text-white text-xs hover:bg-[#3A6FE8]"
                >
                  修改图片
                </button>
              </div>
            </div>

            {/* 提示词区 */}
            <div className="rounded-xl bg-[#1E1E24] border border-[#2A2A35] p-3 flex flex-col gap-2">
              <div className="flex items-center gap-2 flex-wrap">
                {/* 加号菜单：本地上传参考图 / 通过资产库导入 */}
                <div className="relative">
                  <button
                    onClick={() => setRefMenuOpen((v) => !v)}
                    className="w-7 h-7 rounded border border-dashed border-[#3A3A4A] flex items-center justify-center text-[#8B8B9A] hover:border-[#4C7FF8] hover:text-[#4C7FF8]"
                    title="添加参考图"
                  >
                    <Plus size={14} />
                  </button>
                  {refMenuOpen && (
                    <>
                      <div className="fixed inset-0 z-10" onClick={() => setRefMenuOpen(false)} />
                      <div className="absolute top-full mt-1 left-0 w-44 rounded-lg bg-[#1E1E24] border border-[#2A2A35] shadow-xl z-20 py-1">
                        <button
                          onClick={() => { setRefMenuOpen(false); refFileRef.current?.click(); }}
                          className="block w-full text-left px-3 py-2 text-xs text-white/80 hover:bg-white/5"
                        >
                          本地上传参考图
                        </button>
                        <button
                          onClick={() => { setRefMenuOpen(false); void openLibrary(); }}
                          className="block w-full text-left px-3 py-2 text-xs text-white/80 hover:bg-white/5"
                        >
                          通过资产库导入
                        </button>
                      </div>
                    </>
                  )}
                </div>
                {refImages.map((u, i) => (
                  <span key={u} className="flex items-center gap-1 px-1.5 py-1 rounded bg-[#2A2A4A] text-[#4C7FF8] text-xs">
                    <img src={u} alt="" className="w-5 h-5 rounded object-cover" />
                    图片{i}
                    <button onClick={() => setRefImages((prev) => prev.filter((x) => x !== u))} className="hover:text-white"><X size={12} /></button>
                  </span>
                ))}
              </div>
              <textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                onBlur={() => void savePrompt(prompt)}
                rows={5}
                placeholder="输入图像生成提示词…"
                className="w-full bg-transparent text-sm text-white/90 leading-relaxed resize-none outline-none"
              />
              {showReset && (
                <button onClick={() => void resetPrompt()} className="self-start text-xs text-[#4C7FF8] hover:underline">
                  恢复默认提示词
                </button>
              )}
              {/* 提示词质量评分 */}
              <div ref={qualityRef} className="mt-2 text-xs" style={{ color: "var(--luxreal-muted-foreground)" }}>
                提示词质量：<span className="text-[#4C7FF8] font-medium">{quality ? quality.score : "--"}/100</span>
                {quality && quality.suggestions.length > 0 && (
                  <div className="mt-1 flex items-start gap-1">
                    <Minus size={10} className="flex-shrink-0 mt-0.5 text-[#4C7FF8]" />
                    <div className="flex flex-wrap gap-x-2 gap-y-1">
                      {quality.suggestions.map((s, i) => (
                        <span key={i} className="text-[#6B7088]">• {s}</span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </div>

            {/* 底部工具栏 */}
            <div className="flex items-center gap-2 flex-wrap">
              {isChar && (
                <label className="flex items-center gap-1.5 text-xs text-white/80 cursor-pointer" title="勾选后，会基于提示词生成同时含有“人物半身图 + 人物全身三视图（正/侧/背）”的图片">
                  <input type="checkbox" checked={useDefaultComp} onChange={(e) => setUseDefaultComp(e.target.checked)} className="accent-[#4C7FF8]" />
                  使用默认构图
                </label>
              )}
              {/* 模型下拉 */}
              <div className="relative">
                <button
                  onClick={() => { setModelOpen(!modelOpen); setStyleOpen(false); }}
                  className="px-2.5 py-1.5 rounded-lg bg-[#1E1E24] border border-[#2A2A35] text-xs text-white/80 hover:border-[#3A3A4A]"
                >
                  {models.find((m) => m.model_key === modelCode)?.name || "默认模型"}
                </button>
                {modelOpen && (
                  <div className="absolute bottom-full mb-1 left-0 w-48 max-h-56 overflow-auto rounded-lg bg-[#1E1E24] border border-[#2A2A35] shadow-xl z-10">
                    <button onClick={() => { setModelCode(""); setModelOpen(false); }} className="block w-full text-left px-3 py-2 text-xs text-white/80 hover:bg-white/5">默认模型</button>
                    {models.map((m) => (
                      <button key={m.model_key} onClick={() => { setModelCode(m.model_key); setModelOpen(false); }} className="block w-full text-left px-3 py-2 text-xs text-white/80 hover:bg-white/5">
                        {m.name}（{m.cost_points} 分）
                      </button>
                    ))}
                  </div>
                )}
              </div>
              {/* 风格下拉 */}
              <div className="relative">
                <button
                  onClick={() => { setStyleOpen(!styleOpen); setModelOpen(false); }}
                  className="px-2.5 py-1.5 rounded-lg bg-[#1E1E24] border border-[#2A2A35] text-xs text-white/80 hover:border-[#3A3A4A]"
                >
                  {styleName}
                </button>
                {styleOpen && (
                  <div className="absolute bottom-full mb-1 left-0 w-40 max-h-56 overflow-auto rounded-lg bg-[#1E1E24] border border-[#2A2A35] shadow-xl z-10">
                    {styleOptions.map((s) => (
                      <button key={s} onClick={() => changeStyle(s)} className="block w-full text-left px-3 py-1.5 text-xs text-white/80 hover:bg-white/5">
                        {s}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              {/* 比例/分辨率 */}
              <select value={ratio} onChange={(e) => setRatio(e.target.value)} className="px-2 py-1.5 rounded-lg bg-[#1E1E24] border border-[#2A2A35] text-xs text-white/80 outline-none">
                {RATIOS.map((r) => <option key={r} value={r}>{r}</option>)}
              </select>
              <select value={resolution} onChange={(e) => setResolution(e.target.value)} className="px-2 py-1.5 rounded-lg bg-[#1E1E24] border border-[#2A2A35] text-xs text-white/80 outline-none">
                {RESOLUTIONS.map((r) => <option key={r} value={r}>{r}</option>)}
              </select>
              {/* 场景时段（日夜变体，对齐 LuxReal）；仅场景显示 */}
              {kind === "SCENE" && (
                <select value={sceneTod} onChange={(e) => setSceneTod(e.target.value)} className="px-2 py-1.5 rounded-lg bg-[#1E1E24] border border-[#2A2A35] text-xs text-white/80 outline-none">
                  {SCENE_TOD_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
                </select>
              )}
              <button
                onClick={() => void optimizePrompt()}
                disabled={optimizing}
                className="w-8 h-8 rounded-lg bg-[#1E1E24] border border-[#2A2A35] flex items-center justify-center text-[#8B5CF8] hover:border-[#8B5CF8] disabled:opacity-40"
                title="AI 优化提示词"
              >
                <Wand2 size={14} className={optimizing ? "animate-pulse" : ""} />
              </button>
              <button
                onClick={() => void generate()}
                disabled={generating}
                className="ml-auto flex items-center gap-1.5 px-4 py-2 rounded-lg bg-[#4C7FF8] text-white text-sm font-medium hover:bg-[#3A6FE8] disabled:opacity-40"
              >
                <Sparkles size={14} />
                {cost} 生成
              </button>
            </div>
          </div>

          {/* 右列：生成历史 */}
          <div className="w-72 shrink-0 flex flex-col rounded-xl bg-[#1E1E24] border border-[#2A2A35] p-3 min-h-0">
            <div className="flex items-center gap-2 mb-2">
              <span className="text-white text-sm font-medium">生成历史</span>
              <button
                onClick={() => fileRef.current?.click()}
                className="w-6 h-6 rounded border border-dashed border-[#3A3A4A] flex items-center justify-center text-[#8B8B9A] hover:border-[#4C7FF8] hover:text-[#4C7FF8]"
                title="导入图片"
              >
                <Plus size={12} />
              </button>
              <div className="ml-auto flex items-center rounded-lg overflow-hidden border border-[#2A2A35]">
                <button
                  onClick={() => setHistoryMode("card")}
                  className={`px-2 py-1 text-xs flex items-center gap-1 ${historyMode === "card" ? "bg-[#2A2A4A] text-[#4C7FF8]" : "text-[#8B8B9A]"}`}
                >
                  <LayoutGrid size={12} /> 卡片
                </button>
                <button
                  onClick={() => setHistoryMode("list")}
                  className={`px-2 py-1 text-xs flex items-center gap-1 ${historyMode === "list" ? "bg-[#2A2A4A] text-[#4C7FF8]" : "text-[#8B8B9A]"}`}
                >
                  <List size={12} /> 列表
                </button>
              </div>
            </div>
            <label className="flex items-center gap-1.5 text-xs text-[#8B8B9A] mb-2 cursor-pointer">
              <input type="checkbox" checked={favOnly} onChange={(e) => setFavOnly(e.target.checked)} className="accent-[#4C7FF8]" />
              我的收藏
            </label>
            <div className="flex-1 overflow-y-auto min-h-0">
              {history.length === 0 ? (
                <div className="text-center text-xs text-[#3A3A4A] py-8">暂无生成内容</div>
              ) : historyMode === "card" ? (
                <div className="grid grid-cols-2 gap-2 pb-1">
                  {history.map((h) => {
                    const selected = selectedHistoryId === h.id;
                    return (
                      <div
                        key={h.id}
                        onClick={() => setSelectedHistoryId(selected ? null : h.id)}
                        className="relative rounded-lg overflow-hidden bg-[#16161C] cursor-pointer transition-all"
                        style={{
                          border: selected ? "2px solid #4C7FF8" : "1px solid #2A2A35",
                          boxShadow: selected ? "0 0 0 2px rgba(76,127,248,0.25)" : undefined,
                        }}
                        title="点击选中后可设为主图"
                      >
                        <img src={h.url} alt="" className="w-full aspect-video object-cover" />
                        {/* 选中角标 */}
                        {selected && (
                          <div className="absolute top-1 left-1 w-5 h-5 rounded-full bg-[#4C7FF8] flex items-center justify-center text-white">
                            <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="3">
                              <path d="M5 13l4 4L19 7" />
                            </svg>
                          </div>
                        )}
                        <button
                          onClick={(e) => { e.stopPropagation(); void toggleFav(h.id); }}
                          className={`absolute top-1 right-1 w-6 h-6 rounded flex items-center justify-center bg-black/50 ${h.is_favorite ? "text-yellow-300" : "text-white/60"}`}
                        >
                          <Star size={12} fill={h.is_favorite ? "currentColor" : "none"} />
                        </button>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="flex flex-col gap-2">
                  {history.map((h) => (
                    <div key={h.id} className="rounded-lg bg-[#16161C] border border-[#2A2A35] p-2">
                      <div className="flex items-center gap-2">
                        <img src={h.url} alt="" className="w-10 h-10 rounded object-cover" />
                        <div className="flex-1 min-w-0">
                          <p className="text-[10px] text-white/70 line-clamp-2">{h.prompt || "（无提示词）"}</p>
                          <p className="text-[9px] text-[#6B7088]">{h.model_code || "默认"} · {h.style_name || "通用"} · {h.aspect_ratio || "16:9"}</p>
                        </div>
                        <button onClick={() => void toggleFav(h.id)} className={h.is_favorite ? "text-yellow-300" : "text-white/40"}>
                          <Star size={12} fill={h.is_favorite ? "currentColor" : "none"} />
                        </button>
                      </div>
                      <div className="flex items-center gap-1.5 mt-1.5 flex-wrap">
                        <button
                          onClick={() => void setAsMain(h)}
                          className="px-2 py-0.5 rounded bg-[#4C7FF8]/15 text-[10px] text-[#4C7FF8] hover:bg-[#4C7FF8]/25"
                        >
                          设为主图
                        </button>
                        <button
                          onClick={() => { setPrompt(h.prompt || ""); toast.info("已回填该历史提示词"); }}
                          className="px-2 py-0.5 rounded bg-[#2A2A35] text-[10px] text-white/70 hover:text-white"
                        >
                          重新编辑
                        </button>
                        <button
                          onClick={() => void generate()}
                          className="px-2 py-0.5 rounded bg-[#2A2A35] text-[10px] text-white/70 hover:text-white flex items-center gap-1"
                        >
                          <RefreshCw size={10} /> 重新生成
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
            {/* 选中历史图后的底部操作条（先选图 → 显式确认设为主图） */}
            {selectedHistoryId != null && historyMode === "card" && (
              <div className="mt-2 pt-2 border-t border-[#2A2A35] flex items-center gap-2">
                <span className="text-[10px] text-[#6B7088] flex-1 truncate">已选中 1 张图片</span>
                <button
                  onClick={() => setSelectedHistoryId(null)}
                  className="px-2.5 py-1 rounded-lg text-xs text-[#8B8B9A] hover:text-white bg-[#2A2A35]"
                >
                  取消
                </button>
                <button
                  onClick={() => {
                    const h = history.find((x) => x.id === selectedHistoryId);
                    if (h) {
                      void setAsMain(h);
                      setSelectedHistoryId(null);
                    }
                  }}
                  className="px-3 py-1 rounded-lg text-xs text-white font-medium"
                  style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                >
                  设为主图
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* 隐藏文件输入 */}
      <input
        ref={fileRef} type="file" accept="image/*" className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void onPickFile(f, true);
          e.target.value = "";
        }}
      />
      <input
        ref={refFileRef} type="file" accept="image/*" className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void onPickFile(f, false);
          e.target.value = "";
        }}
      />

      {/* 资产库导入弹层（选择角色/场景/道具图片作为参考图） */}
      {libraryOpen && (
        <div className="fixed inset-0 z-[60] bg-black/60 flex items-center justify-center p-6" onClick={() => setLibraryOpen(false)}>
          <div
            className="bg-[#16161C] border border-[#2A2A35] rounded-2xl w-full max-w-2xl max-h-[70vh] flex flex-col shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-5 py-3 border-b border-[#2A2A35]">
              <span className="text-white font-semibold text-sm">从资产库导入参考图</span>
              <button onClick={() => setLibraryOpen(false)} className="w-8 h-8 rounded-lg flex items-center justify-center text-[#8B8B9A] hover:bg-white/5 hover:text-white">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto min-h-0 p-4">
              {libraryLoading ? (
                <div className="text-center text-xs text-[#6B7088] py-10">加载资产库中…</div>
              ) : libraryItems.length === 0 ? (
                <div className="text-center text-xs text-[#3A3A4A] py-10">资产库暂无可用图片</div>
              ) : (
                (["角色", "场景", "道具"] as const).map((group) => {
                  const items = libraryItems.filter((it) => it.group === group);
                  if (!items.length) return null;
                  return (
                    <div key={group} className="mb-4 last:mb-0">
                      <div className="text-xs text-[#8B8B9A] mb-2">{group}</div>
                      <div className="grid grid-cols-5 gap-2">
                        {items.map((it) => (
                          <button
                            key={`${group}-${it.url}`}
                            onClick={() => importFromLibrary(it)}
                            className="relative aspect-square rounded-lg overflow-hidden border border-[#2A2A35] hover:border-[#4C7FF8] transition-colors group"
                            title={it.label}
                          >
                            <img src={it.url} alt={it.label} className="w-full h-full object-cover" />
                            <span className="absolute bottom-0 inset-x-0 px-1 py-0.5 bg-black/60 text-[10px] text-white/80 truncate opacity-0 group-hover:opacity-100 transition-opacity">
                              {it.label}
                            </span>
                          </button>
                        ))}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
