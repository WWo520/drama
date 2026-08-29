// S1-F05 Step3 视频制作页 —— 成片卡主线（对齐 LuxReal 两级结构：分镜脚本 → 规划视频任务 → 成片卡生成）
// 布局：WorkbenchShell + settings bar + 叙事单元卡（剧本原文|分镜脚本 Tab）+ 成片卡网格 + 生成弹窗
// 接口：GET /episodes/:id/beats、POST /beats/:id/plan-video-tasks、GET /beats/:id/cards、
//       POST /beats/:id/generate-videos、POST /video-cards/:uuid/regenerate、PATCH /video-cards/:uuid/prompt
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  Box, CheckCircle2, ChevronDown, Clapperboard, Film, GripVertical, History,
  Loader2, Mic2, Monitor, MoreHorizontal, Pencil, Play, Plus, RefreshCw,
  Settings2, Sparkles, Subtitles, Zap,
} from "lucide-react";

import {
  ApiError,
  errorMessage,
  assetsApi,
  projectsApi,
  splitApi,
  tasksApi,
  videoApi,
  versionApi,
  type BeatDetail,
  type BeatSummary,
  type EpisodeLite,
  type NuAssetLinkItem,
  type NuVersionItem,
  type ProjectDetail,
  type ShotLinkedAsset,
  type VideoCard,
} from "@duanju/api-client";

import WorkbenchShell from "../layouts/WorkbenchShell";
import ProjectLoadError from "../components/ProjectLoadError";
import { useStepState } from "../hooks/useStepState";
import { matchTaskTypes, useWsDrivenPoll } from "../hooks/useWsDrivenPoll";
import "./step3.css";

// 错误文案：ApiError 走错误码映射（05 C 章），避免弹出后端原始报文
function errMsg(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return errorMessage(e.code, e.message);
  return e instanceof Error ? e.message : fallback;
}

// 卡片活动生成态（QUEUED/RUNNING）
const isCardActive = (s: string) => s === "RUNNING" || s === "QUEUED";

// 版本时间格式化：ISO → MM-DD HH:mm
function fmtVersionTime(iso: string): string {
  const m = iso?.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
  if (!m) return iso || "—";
  return `${m[2]}-${m[3]} ${m[4]}:${m[5]}`;
}

// 关联资产就绪判定（对齐后端 LoadShotRefs：参考图 URL 非空即可用）
const assetReady = (l: { main_image_url?: string }) => !!l.main_image_url;

// 资产类型 → 中文名（chips 提示用）
const ASSET_KIND_LABEL: Record<string, string> = { CHARACTER: "角色", SCENE: "场景", PROP: "道具" };

// 提示词文本中命中资产名 → @资产 引用高亮（对齐 LuxReal「@资产」引用展示）。
// 长名优先匹配，避免"陆言-市井"被"陆言"截断命中；资产名 <2 字不参与（与后端建链口径一致）。
function RefText({ text, assets }: { text?: string; assets?: ShotLinkedAsset[] }) {
  const names = (assets ?? [])
    .map((a) => a.name?.trim())
    .filter((n): n is string => !!n && [...n].length >= 2)
    .sort((a, b) => b.length - a.length);
  if (!text || names.length === 0) return <>{text}</>;
  const escaped = names.map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  const parts = text.split(new RegExp(`(${escaped.join("|")})`, "g"));
  const nameSet = new Set(names);
  return (
    <>
      {parts.map((p, i) =>
        nameSet.has(p) ? (
          <span key={i} className="asset-ref-tag" title="已关联资产，生成视频时作为参考图注入">@{p}</span>
        ) : (
          p
        ),
      )}
    </>
  );
}

// ===== 关联资产 chip（17 - 工作台对齐：名称引用占位，对齐 LuxReal「关联资产」） =====
function AssetChip({ link, onGoAssets }: { link: NuAssetLinkItem; onGoAssets: () => void }) {
  const ready = assetReady(link);
  const label = link.name || ASSET_KIND_LABEL[link.asset_kind] || "资产";
  return (
    <div
      className="asset-avatar"
      style={ready ? undefined : { background: "#2A2A35" }}
      title={`${ASSET_KIND_LABEL[link.asset_kind] ?? ""} · ${label}${ready ? "" : "（待生成）"}`}
      onClick={onGoAssets}
    >
      {ready ? (
        <img src={link.main_image_url} alt={label} className="w-full h-full object-cover" />
      ) : (
        <span className="w-full h-full flex items-center justify-center text-[9px] text-[#8B8B9A] select-none">
          {label.slice(0, 1)}
        </span>
      )}
      {!ready && <span className="absolute bottom-0 right-0 w-1.5 h-1.5 rounded-full" style={{ background: "#F5B23C" }} />}
    </div>
  );
}

// BeatDetail 展平后的单镜头文案
type ShotDetailLite = BeatDetail["shot_groups"][number]["shots"][number];

// 视频模型选项（弹窗选择；积分系数仅用于预估展示，实际计费以后端为准）
// key 同时是提交后端的 model_key：Seedance 系列走默认路由（key 不命中 models 表），
// MiniMax H3 须命中管理端配置的 models.model_key=MiniMax-H3 才参与路由。
const MODEL_OPTIONS = [
  { key: "2.5", name: "Seedance 2.5", badge: "PRO", desc: "30秒 · 精修可控", factor: 4 },
  { key: "2.0", name: "Seedance 2.0", badge: "", desc: "15秒 · 高一致性", factor: 3 },
  { key: "1.5", name: "Seedance 1.5", badge: "", desc: "10秒 · 高效批量", factor: 2 },
  { key: "MiniMax-H3", name: "MiniMax H3", badge: "NEW", desc: "15秒 · 多图参考 · 原生音画", factor: 4 },
];
// MiniMax H3 分辨率枚举（V2 接口仅 768P/2K；自部署仅 768P，2K 需官方 API）
const H3_RESOLUTIONS = ["768P", "2K"];
// 后端枚举 → 前端显示值（项目配置快照可能存 P720 等枚举）
const normRes = (v?: string) =>
  v === "P480" ? "480P" : v === "P720" ? "720P" : v === "P1080" ? "1080P" : v;
// 首页/管理端模型 model_key → 弹窗选项 key（Seedance 系命名差异；MiniMax-H3 两边一致）
const VIDEO_MODEL_KEY_ALIAS: Record<string, string> = {
  seedance25: "2.5",
  seedance20: "2.0",
  seedance15: "1.5",
};
const resolveModelKey = (v?: string) => (v ? VIDEO_MODEL_KEY_ALIAS[v] ?? v : "");

// 卡片状态徽章（对齐设计稿配色）
function CardStatusBadge({ status }: { status: string }) {
  if (status === "SUCCESS") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(16,185,129,0.15)", color: "#10B981" }}>已完成</span>;
  }
  if (status === "RUNNING") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(76,127,248,0.15)", color: "#4C7FF8" }}>生成中</span>;
  }
  if (status === "QUEUED") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(245,178,60,0.15)", color: "#F5B23C" }}>排队中</span>;
  }
  if (status === "FAILED_DEAD") {
    return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>失败</span>;
  }
  return <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ background: "#1A1A2E", color: "#6B7088" }}>待生成</span>;
}

// 景别/运镜/视角 枚举 → 中文（对齐 LuxReal 分镜脚本正文呈现；DB 存英文枚举值）
const SHOT_SIZE_LABEL: Record<string, string> = {
  ECU: "大特写", CU: "特写", MCU: "中特写", MS: "中景", FS: "全景",
  LS: "远景", ELS: "大远景", TWO_SHOT: "双人镜头", OTS: "过肩",
};
const CAMERA_MOVE_LABEL: Record<string, string> = {
  STATIC: "固定", SLOW_PUSH_IN: "慢慢推进", SLOW_PULL_OUT: "慢慢拉远", FAST_PUSH_IN: "快速推进",
  DOLLY_IN: "推车前进", DOLLY_OUT: "推车后退", PAN_LEFT: "左摇", PAN_RIGHT: "右摇",
  TRACK_LEFT: "左横移跟拍", TRACK_RIGHT: "右横移跟拍", TILT_UP: "仰摇", TILT_DOWN: "俯摇",
  CRANE_UP: "升镜头", CRANE_DOWN: "降镜头", HANDHELD_SHAKY: "手持晃动", WIPE_CLIP: "甩镜切换",
  ZOOM_IN_OPTICAL: "光学变焦推近", SLOW_360_ORBIT: "缓慢环绕",
};
const CAMERA_ANGLE_LABEL: Record<string, string> = {
  EYE_LEVEL_FRONT: "平视", SIDE: "侧面", BACK: "背面", SLIGHT_LOW_ANGLE: "微仰拍",
  LOW_ANGLE: "仰拍", SLIGHT_HIGH: "微俯拍", HIGH_BIRDVIEW: "鸟瞰俯拍",
  OTS_LEFT: "左侧过肩", OTS_RIGHT: "右侧过肩", DUTCH_ANGLE: "荷兰角倾斜",
};
const shotSizeLabel = (v: string) => SHOT_SIZE_LABEL[v] ?? v;
const cameraMoveLabel = (v: string) => CAMERA_MOVE_LABEL[v] ?? v;
const cameraAngleLabel = (v: string) => CAMERA_ANGLE_LABEL[v] ?? v;

// ===== 拖拽源资产调色板（对齐 LuxReal：拖资产 chip 到成片卡绑定参考图） =====
function AssetPalette({ links, onGoAssets }: { links: NuAssetLinkItem[]; onGoAssets: () => void }) {
  if (links.length === 0) return null;
  return (
    <div className="flex items-center gap-2 px-5 pt-3 pb-1 flex-wrap">
      <span className="text-[10px] text-[#6B7088] select-none">关联资产（拖到成片卡绑定参考图）：</span>
      {links.map((l) => {
        const ready = assetReady(l);
        return (
          <div
            key={`${l.asset_kind}-${l.asset_id}`}
            draggable={ready}
            onDragStart={(e) => {
              e.dataTransfer.setData(ASSET_DND_MIME, JSON.stringify({ kind: l.asset_kind, id: l.asset_id }));
              e.dataTransfer.effectAllowed = "copy";
            }}
            title={`${ASSET_KIND_LABEL[l.asset_kind] ?? ""} · ${l.name}${ready ? "" : "（待生成，去 Step2 补齐）"}${ready ? " · 拖拽绑定" : ""}`}
            onClick={!ready ? onGoAssets : undefined}
            className={`flex items-center gap-1.5 pl-0.5 pr-2 py-0.5 rounded-full border ${ready ? "cursor-grab active:cursor-grabbing" : "cursor-pointer opacity-60"}`}
            style={{ background: "#16161C", borderColor: "#2A2A35" }}
          >
            {ready ? (
              <img src={l.main_image_url} alt={l.name} className="w-5 h-5 rounded-full object-cover" />
            ) : (
              <span className="w-5 h-5 rounded-full flex items-center justify-center text-[9px] text-[#8B8B9A] select-none"
                style={{ background: "#2A2A35" }}>{(l.name || "?").slice(0, 1)}</span>
            )}
            <span className="text-[10px] text-white/80 whitespace-nowrap">{l.name}</span>
          </div>
        );
      })}
    </div>
  );
}

// ===== 分镜脚本视图（对齐 LuxReal：8 字段结构化正文 + 尾部全局约束指令） =====
// 支持镜头级资产绑定：拖 AssetPalette chip 到单个镜头行（精准 @ 引用，替换单元级继承）；
// 每个引用小图可 × 解绑（仅移除镜头级显式行；继承项解绑无效会有提示）。
function StoryboardScriptView({
  detail,
  generating,
  bindEnabled,
  onBindAsset,
  onUnbindAsset,
}: {
  detail?: BeatDetail;
  generating?: boolean;
  bindEnabled?: boolean; // 是否允许拖拽/解绑（分镜生成中禁用）
  onBindAsset?: (shotId: string, kind: string, assetId: number) => void;
  onUnbindAsset?: (shotId: string, kind: string, assetId: number) => void;
}) {
  const [dropHotFor, setDropHotFor] = useState<string | null>(null);
  // 展平为全局序号（跨镜头块连续：flatMap 保序 + 全局编号，杜绝组内 sort_order 重复）
  const shots = useMemo(() => {
    const list: { no: number; shot: NonNullable<BeatDetail["shot_groups"][number]["shots"][number]> }[] = [];
    (detail?.shot_groups ?? []).forEach((g) => g.shots.forEach((s) => list.push({ no: list.length + 1, shot: s })));
    return list;
  }, [detail]);

  const dndProps = (shotId: string) =>
    bindEnabled && onBindAsset
      ? {
          onDragOver: (e: React.DragEvent) => {
            if (!e.dataTransfer.types.includes(ASSET_DND_MIME)) return;
            e.preventDefault();
            e.stopPropagation();
            if (dropHotFor !== shotId) setDropHotFor(shotId);
          },
          onDragLeave: () => {
            if (dropHotFor === shotId) setDropHotFor(null);
          },
          onDrop: (e: React.DragEvent) => {
            e.preventDefault();
            e.stopPropagation();
            setDropHotFor(null);
            try {
              const payload = JSON.parse(e.dataTransfer.getData(ASSET_DND_MIME));
              if (payload?.kind && payload?.id) onBindAsset(shotId, String(payload.kind), Number(payload.id));
            } catch { /* 忽略非法负载 */ }
          },
        }
      : {};

  if (generating) {
    return (
      <div className="storyboard-script generating">
        <Loader2 size={16} className="animate-spin text-[#4C7FF8]" />
        <span>分镜脚本生成中，请稍候…</span>
      </div>
    );
  }
  if (!detail) {
    return <div className="storyboard-script empty">分镜脚本加载中…</div>;
  }
  if (shots.length === 0) {
    return <div className="storyboard-script empty">尚未生成分镜脚本，请先点击上方「生成分镜脚本」。</div>;
  }

  return (
    <div className="storyboard-script">
      {shots.map(({ no, shot: s }) => (
        <div key={s.shot_id} className={`sb-shot${dropHotFor === s.shot_id ? " drop-hot" : ""}`} {...dndProps(s.shot_id)}>
          <div className="sb-shot-head">
            <span className="sb-shot-no">镜头{no}</span>
            <span className="sb-tag">【时长{s.duration_sec}s】</span>
            {s.shot_size && <span className="sb-tag">【景别{shotSizeLabel(s.shot_size)}】</span>}
            {s.camera_movement && <span className="sb-tag">【运镜{cameraMoveLabel(s.camera_movement)}】</span>}
            {s.camera_angle && <span className="sb-tag">【视角{cameraAngleLabel(s.camera_angle)}】</span>}
            {bindEnabled && (
              <span className="sb-bind-hint" title="从上方关联资产列表拖拽到本镜头可精准绑定参考图">
                可拖入关联资产
              </span>
            )}
          </div>
          {/* 该镜头引用的资产（对齐 LuxReal：小图 + @引用标签；生成视频时作为参考图注入；× 镜头级解绑） */}
          {(s.linked_assets ?? []).map((a) => (
            <div key={`${a.asset_kind}-${a.asset_id}`} className="sb-asset-row">
              <span
                className={`sb-asset${assetReady(a) ? "" : " pending"}`}
                title={`${ASSET_KIND_LABEL[a.asset_kind] ?? ""} · ${a.name}${assetReady(a) ? "" : "（待生成）"}`}
              >
                {a.main_image_url ? <img src={a.main_image_url} alt={a.name} /> : <span className="sb-asset-letter">{(a.name || "?").slice(0, 1)}</span>}
                <span className="sb-asset-name">@{a.name}</span>
              </span>
              {bindEnabled && onUnbindAsset && (
                <button
                  className="sb-asset-x"
                  title="移除该镜头的此参考资产"
                  onClick={(e) => {
                    e.stopPropagation();
                    onUnbindAsset(s.shot_id, a.asset_kind, a.asset_id);
                  }}
                >
                  ×
                </button>
              )}
            </div>
          ))}
          {s.subject_description && (
            <div className="sb-line"><span className="sb-field">【主体】</span><span className="sb-text"><RefText text={s.subject_description} assets={s.linked_assets} /></span></div>
          )}
          {s.background_description && (
            <div className="sb-line"><span className="sb-field">【背景】</span><span className="sb-text"><RefText text={s.background_description} assets={s.linked_assets} /></span></div>
          )}
          {s.local_light_description && (
            <div className="sb-line"><span className="sb-field">【光影】</span><span className="sb-text">{s.local_light_description}</span></div>
          )}
          {s.dialogue_text && (
            <div className="sb-line"><span className="sb-field">【台词】</span><span className="sb-text">{s.dialogue_text}</span></div>
          )}
          {s.ambient_sound && (
            <div className="sb-line"><span className="sb-field">【环境音】</span><span className="sb-text">{s.ambient_sound}</span></div>
          )}
        </div>
      ))}
      {/* 尾部全局约束指令（对齐 LuxReal：Agent 自动追加，生成视频时注入 prompt） */}
      <div className="sb-constraint">严格按照分镜脚本进行生成，参考上传图的人物形象、场景和道具。不要生成字幕。</div>
    </div>
  );
}

// ===== 成片卡 =====
// 对齐 LuxReal 成片卡：一次视频生成任务 = 连续 2~4 个分镜镜头的合并段；
// prompt（后端自动组装三段式：Picture 声明 + 资产绑定语句 + 分镜脚本文本 + 完整约束尾缀）
// 卡头时长下拉（4~15s）与参考图拖拽绑定均对齐 LuxReal 交互。
const CARD_DURATIONS = [4, 5, 6, 7, 8, 9, 10, 12, 15];
const ASSET_DND_MIME = "application/x-duanju-asset";

function VideoCardView({
  card,
  index,
  onRegenerate,
  onEditPrompt,
  onDurationChange,
  onBindAsset,
  onUnbindAsset,
  onGoAssets,
}: {
  card: VideoCard;
  index: number;
  onRegenerate: (card: VideoCard) => void;
  onEditPrompt: (card: VideoCard) => void;
  onDurationChange: (card: VideoCard, dur: number) => void;
  onBindAsset: (card: VideoCard, kind: string, id: number) => void;
  onUnbindAsset: (card: VideoCard, kind: string, id: number) => void;
  onGoAssets?: () => void;
}) {
  const done = card.generation_status === "SUCCESS";
  const running = isCardActive(card.generation_status);
  // 卡内镜头引用资产的并集（跨镜头去重，key 为 kind-id）
  const assets = useMemo(() => {
    const seen = new Set<string>();
    const out: ShotLinkedAsset[] = [];
    (card.shots ?? []).forEach((s) =>
      (s.linked_assets ?? []).forEach((a) => {
        const k = `${a.asset_kind}-${a.asset_id}`;
        if (!seen.has(k)) { seen.add(k); out.push(a); }
      }),
    );
    return out;
  }, [card]);
  const hasCustomPrompt = !!card.prompt_text.trim();
  const [durOpen, setDurOpen] = useState(false);
  const [dropHot, setDropHot] = useState(false);

  return (
    <div className="shot-card">
      <div className="shot-header">
        <span className="shot-ref-badge">{done ? "成片" : "视频"}</span>
        <div className="flex items-center gap-2 ml-1 min-w-0">
          <span className="text-sm font-medium text-white whitespace-nowrap">成片卡 {String(index).padStart(2, "0")}</span>
          <span className="text-[11px] text-white/50 truncate">{card.title} · {card.shot_count} 镜头</span>
        </div>
        <div className="shot-header-actions">
          {/* 时长下拉（对齐 LuxReal 卡头 4s 下拉；送模型时长，4~15s） */}
          <div className="shot-duration-wrap">
            <div className="shot-duration" onClick={() => !running && setDurOpen((v) => !v)} title="调整本卡送模型时长">
              <span>{card.duration_sec}s</span>
              <ChevronDown size={12} />
            </div>
            {durOpen && (
              <>
                <div className="shot-duration-mask" onClick={() => setDurOpen(false)} />
                <div className="shot-duration-menu">
                  {CARD_DURATIONS.map((d) => (
                    <button
                      key={d}
                      className={`shot-duration-opt${d === card.duration_sec ? " active" : ""}`}
                      onClick={() => {
                        setDurOpen(false);
                        if (d !== card.duration_sec) onDurationChange(card, d);
                      }}
                    >
                      {d}s
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
          <CardStatusBadge status={card.generation_status} />
          <button className="icon-btn icon-btn-sm" disabled={running} onClick={() => onEditPrompt(card)}
            title={hasCustomPrompt ? "编辑自定义提示词（替换自动组装内容）" : "自定义提示词（默认按分镜脚本三段式组装）"}>
            <Pencil size={14} />
          </button>
          <button className="icon-btn icon-btn-sm" disabled={running} onClick={() => onRegenerate(card)}
            title={done ? "按当前提示词重新生成本卡（计费）" : "生成本卡视频（计费）"}>
            <RefreshCw size={14} />
          </button>
          <button className="icon-btn icon-btn-sm" onClick={() => toast.info("更多操作即将上线")}>
            <MoreHorizontal size={14} />
          </button>
        </div>
      </div>
      <div className="shot-content">
        {/* 画面区（拖拽放置目标）：成功 → 视频；排队/生成中 → 进度；待生成 → 剪影 */}
        <div
          className="shot-image"
          style={dropHot ? { outline: "2px dashed #4C7FF8", outlineOffset: "-4px" } : undefined}
          onDragOver={(e) => {
            if (running || done || !e.dataTransfer.types.includes(ASSET_DND_MIME)) return;
            e.preventDefault();
            setDropHot(true);
          }}
          onDragLeave={() => setDropHot(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDropHot(false);
            try {
              const payload = JSON.parse(e.dataTransfer.getData(ASSET_DND_MIME));
              if (payload?.kind && payload?.id) onBindAsset(card, payload.kind, payload.id);
            } catch { /* 非资产拖拽忽略 */ }
          }}
        >
          {done && card.generated_video_url ? (
            <video src={card.generated_video_url} className="absolute inset-0 w-full h-full object-cover" controls preload="metadata" />
          ) : running ? (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 px-6">
              <Loader2 size={24} className="animate-spin text-[#4C7FF8]" />
              <span className="text-[11px] text-[#4C7FF8]">{card.generation_status === "QUEUED" ? "排队等待中…" : "视频渲染中…"}</span>
              <div className="shot-progress w-full"><div className="shot-progress-bar" /></div>
            </div>
          ) : (
            <div className="shot-silhouette">
              <Clapperboard size={36} className="text-white opacity-70" />
              <span className="text-[10px] mt-2 select-none" style={{ color: "#6B7088" }}>拖拽资产图到此绑定参考</span>
            </div>
          )}
        </div>
        <div className="shot-details">
          {/* 参考图 chips（可拖拽移出=解绑；× 直接解绑）：生成时按 Picture 注入 */}
          <div className="shot-ref-chips flex-wrap">
            {assets.length > 0 ? (
              assets.map((a) => {
                const ready = assetReady(a);
                return (
                  <div
                    key={`${a.asset_kind}-${a.asset_id}`}
                    className={`shot-ref-chip ${ready ? "" : "pending"}`}
                    style={ready ? { position: "relative" } : { background: "#2A2A35", position: "relative" }}
                    title={`${ASSET_KIND_LABEL[a.asset_kind] ?? ""} · ${a.name}${ready ? "" : "（待生成）"} · 点击×解绑`}
                  >
                    {ready ? (
                      <img src={a.main_image_url} alt={a.name} className="w-full h-full object-cover" />
                    ) : (
                      <span className="w-full h-full flex items-center justify-center text-[10px] text-[#8B8B9A] select-none"
                        onClick={onGoAssets}>
                        {(a.name || "?").slice(0, 1)}
                      </span>
                    )}
                    {!running && !done && (
                      <span
                        className="absolute -top-1 -right-1 w-3.5 h-3.5 rounded-full flex items-center justify-center text-white select-none"
                        style={{ background: "#EF4444", fontSize: 9, cursor: "pointer", lineHeight: 1 }}
                        onClick={(e) => { e.stopPropagation(); onUnbindAsset(card, a.asset_kind, a.asset_id); }}
                      >×</span>
                    )}
                  </div>
                );
              })
            ) : (
              <div className="shot-ref-chip empty" title="本卡未关联资产（纯文生视频）；可从上方资产条拖拽绑定">
                <span className="w-full h-full flex items-center justify-center text-[9px] text-[#6B7088] select-none px-0.5 text-center">无关联资产</span>
              </div>
            )}
          </div>
          {/* 卡内镜头脚本摘要（与分镜脚本同源一体；主体/台词 @ 资产高亮） */}
          <div className="shot-desc space-y-1.5 mt-1">
            {(card.shots ?? []).map((s, i) => (
              <div key={s.shot_uuid || s.shot_id} className="rounded-lg px-2 py-1.5" style={{ background: "#16161C", border: "1px solid #2A2A35" }}>
                <div className="flex items-center justify-between mb-0.5">
                  <span className="text-[11px] font-medium text-white/90">镜头{i + 1}
                    <span className="text-white/40 font-normal"> · {s.duration_sec}s</span>
                  </span>
                  {s.dialogue_text && <span className="text-[10px]" style={{ color: "#8B8B9A" }}>{s.dialogue_text.slice(0, 18)}</span>}
                </div>
                {s.subject_description && (
                  <div className="text-[11px] text-white/70 leading-relaxed line-clamp-2">
                    <RefText text={s.subject_description} assets={assets} />
                  </div>
                )}
              </div>
            ))}
          </div>
          {/* 自定义提示词状态提示 */}
          {hasCustomPrompt && (
            <details className="mt-1.5 group">
              <summary className="text-[10px] cursor-pointer select-none" style={{ color: "#F5B23C" }}>
                已自定义提示词<span className="group-open:hidden">（点击查看）</span><span className="hidden group-open:inline">收起</span>
              </summary>
              <div className="text-[11px] text-white/60 leading-relaxed mt-1 p-2 rounded bg-[#16161C] border border-[#2A2A35] whitespace-pre-wrap">
                {card.prompt_text}
              </div>
            </details>
          )}
          {/* 操作行 */}
          <div className="flex items-center justify-end mt-auto pt-1">
            <button
              className="flex items-center gap-1 text-xs px-3 py-1 rounded-lg text-white font-medium disabled:opacity-50"
              style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
              disabled={running}
              title={done ? "按当前提示词重新生成本卡（计费）" : "生成本卡视频（计费）"}
              onClick={() => onRegenerate(card)}
            >
              <Play size={12} className="fill-white" />
              <span>{done ? "重新生成" : "生成视频"}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ===== 生成视频弹窗（单元级批量：非 SUCCESS 成片卡全量提交）=====
function GenVideoModal({
  beat,
  cards,
  links,
  submitting,
  onClose,
  onConfirm,
  defaultModel,
  defaultResolution,
}: {
  beat: BeatSummary;
  cards: VideoCard[];
  links: NuAssetLinkItem[];
  submitting: boolean;
  onClose: () => void;
  onConfirm: (opts: { resolution?: string; video_model?: string; model_key?: string; redo?: boolean }) => void;
  /** 项目配置快照默认值（video_model/resolution） */
  defaultModel?: string;
  defaultResolution?: string;
}) {
  // 初始值：项目配置快照 → 未配置/不合法时回退原默认（Seedance 2.5 / 1080P）
  const initModel =
    resolveModelKey(defaultModel) && MODEL_OPTIONS.some((m) => m.key === resolveModelKey(defaultModel))
      ? resolveModelKey(defaultModel)
      : "2.5";
  const initResolution = (() => {
    const r = normRes(defaultResolution);
    const list = initModel === "MiniMax-H3" ? H3_RESOLUTIONS : ["480P", "720P", "1080P"];
    return r && list.includes(r) ? r : initModel === "MiniMax-H3" ? "768P" : "1080P";
  })();
  const [model, setModel] = useState(initModel);
  const [resolution, setResolution] = useState(initResolution);
  const [fps, setFps] = useState("24fps");
  const [advOpen, setAdvOpen] = useState(true);
  const [adv, setAdv] = useState({ lens: true, box3d: false, subtitle: false, voice: false });
  const advCount = Object.values(adv).filter(Boolean).length;

  // 待生成卡片：预计消耗 = 待生成卡时长 × 模型系数（仅预估展示，实际计费以后端为准）
  const pendingCards = cards.filter((cd) => cd.generation_status !== "SUCCESS");
  const totalSec = pendingCards.reduce((acc, cd) => acc + (cd.duration_sec || 0), 0);
  const factor = MODEL_OPTIONS.find((m) => m.key === model)?.factor ?? 4;
  const estCost = Math.max(totalSec * factor, pendingCards.length * 10);
  // 全部成片卡已成功 = "重新生成"语义：redo 让后端重置 SUCCESS 卡片后全量重建
  const allDone = cards.length > 0 && cards.every((cd) => cd.generation_status === "SUCCESS");
  // 资产就绪度（17 - 工作台对齐）：未全就绪时提示纯文生视频降级
  const readyCount = links.filter((l) => !!l.main_image_url).length;
  const notReady = links.length > 0 && readyCount < links.length;

  return (
    <div className="gen-modal-overlay" onClick={onClose}>
      <div className="gen-video-modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">生成视频</div>
          <div className="modal-desc">
            {beat.title} · {pendingCards.length}/{cards.length || beat.shot_group_count} 张成片卡待生成 · 选择模型与参数
          </div>
          {notReady && (
            <div
              className="mt-2 rounded-lg px-3 py-2 text-xs leading-relaxed"
              style={{ background: "rgba(245,178,60,0.12)", color: "#F5B23C" }}
            >
              本单元有 {links.length - readyCount} 个关联资产未生成参考图，相关成片卡将使用纯文生视频模式生成；可先返回 Step2
              补齐资产以获得更好的一致性。
            </div>
          )}
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
                <div
                  key={m.key}
                  className={`gen-model-tab ${model === m.key ? "active" : ""}`}
                  onClick={() => {
                    setModel(m.key);
                    setResolution(m.key === "MiniMax-H3" ? "768P" : "1080P");
                  }}
                >
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
                {(model === "MiniMax-H3" ? H3_RESOLUTIONS : ["480P", "720P", "1080P"]).map((r) => (
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
          <button
            className="btn-modal-primary btn-gen-start"
            disabled={submitting}
            onClick={() =>
              onConfirm({
                resolution:
                  model === "MiniMax-H3"
                    ? resolution
                    : resolution === "480P"
                      ? "P480"
                      : resolution === "720P"
                        ? "P720"
                        : "P1080",
                model_key: model,
                video_model: MODEL_OPTIONS.find((m) => m.key === model)?.name ?? "Seedance 2.5",
                redo: allDone,
              })
            }
          >
            {submitting ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
            {submitting ? "提交中…" : "开始生成"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ===== 成片卡提示词编辑弹窗（PATCH /video-cards/:id/prompt · prompt_text）=====
function CardPromptEditModal({
  cardTitle,
  init,
  saving,
  onClose,
  onSave,
}: {
  cardTitle: string;
  init: string;
  saving: boolean;
  onClose: () => void;
  onSave: (value: string) => void;
}) {
  const [value, setValue] = useState(init);
  return (
    <div className="gen-modal-overlay" onClick={onClose}>
      <div className="gen-video-modal" style={{ maxWidth: 560 }} onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">编辑成片卡提示词</div>
          <div className="modal-desc">
            {cardTitle} · 自定义提示词将替换自动组装内容（资产绑定语句 + 分镜脚本文本）；清空恢复自动组装，重新生成后生效
          </div>
        </div>
        <div className="gen-modal-body">
          <textarea
            className="prompt-edit-textarea"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            rows={12}
            maxLength={7000}
            placeholder="输入成片卡提示词（可覆盖自动组装内容）。留空则按分镜脚本自动组装…"
            autoFocus
            onKeyDown={(e) => {
              if ((e.ctrlKey || e.metaKey) && e.key === "Enter") onSave(value.trim());
            }}
          />
          <div className="prompt-edit-count">{[...value].length} / 7000</div>
        </div>
        <div className="modal-footer">
          <button className="btn-modal-cancel" onClick={onClose}>取消</button>
          <button
            className="btn-modal-primary"
            disabled={value.trim() === init.trim() || saving}
            onClick={() => onSave(value.trim())}
          >
            {saving ? <Loader2 size={16} className="animate-spin" /> : <Pencil size={14} />}
            {saving ? "保存中…" : "保存"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ===== 批量生成视频弹窗（多选分镜已就绪的叙事单元 → 逐单元提交：无成片卡先自动规划，有卡直接生成） =====
function BatchVideoModal({
  beats,
  cardsMap,
  defaultSelected,
  submitting,
  onClose,
  onConfirm,
  defaultModel,
  defaultResolution,
}: {
  beats: BeatSummary[];
  cardsMap: Record<string, VideoCard[]>;
  defaultSelected: string[];
  submitting: boolean;
  onClose: () => void;
  onConfirm: (ids: string[], opts: { resolution?: string; model_key?: string; video_model?: string }) => void;
  /** 项目配置快照默认值（video_model/resolution） */
  defaultModel?: string;
  defaultResolution?: string;
}) {
  // 初始值：项目配置快照 → 未配置/不合法时回退原默认（Seedance 2.5 / 1080P）
  const initModel =
    resolveModelKey(defaultModel) && MODEL_OPTIONS.some((m) => m.key === resolveModelKey(defaultModel))
      ? resolveModelKey(defaultModel)
      : MODEL_OPTIONS[0].key;
  const initResolution = (() => {
    const r = normRes(defaultResolution);
    const list = initModel === "MiniMax-H3" ? H3_RESOLUTIONS : ["480P", "720P", "1080P"];
    return r && list.includes(r) ? r : initModel === "MiniMax-H3" ? "768P" : "1080P";
  })();
  const [selected, setSelected] = useState<Set<string>>(new Set(defaultSelected));
  const [model, setModel] = useState(initModel);
  const [resolution, setResolution] = useState(initResolution);

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const allSelected = selected.size === beats.length && beats.length > 0;
  const toggleAll = () =>
    setSelected(allSelected ? new Set() : new Set(beats.map((b) => b.beat_id)));

  // 待生成时长预估（仅统计已有成片卡的单元；未规划单元在自动规划后才会消耗）
  const pendingSec = beats.reduce(
    (acc, b) => acc + (cardsMap[b.beat_id] ?? []).filter((cd) => cd.generation_status !== "SUCCESS").reduce((s, cd) => s + (cd.duration_sec || 0), 0),
    0,
  );
  const factor = MODEL_OPTIONS.find((m) => m.key === model)?.factor ?? 4;

  return (
    <div className="replan-modal-overlay active" onClick={() => !submitting && onClose()}>
      <div className="modal-box" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <div className="modal-title">批量生成视频</div>
        </div>
        <div className="modal-body">
          <div className="confirm-message">
            选择分镜脚本已就绪的叙事单元批量生成视频：尚无成片卡的单元将先自动「规划视频任务」，已有成片卡的单元直接提交生成（全部完成的单元按「重新生成」处理）。
          </div>
          {/* 生成模型（应用至全部所选单元） */}
          <div className="mt-3 flex items-center gap-1.5 flex-wrap">
            {MODEL_OPTIONS.map((m) => (
              <button
                key={m.key}
                className={`px-3 py-1.5 rounded-lg text-xs transition-colors ${model === m.key ? "" : "opacity-70"}`}
                style={{
                  background: model === m.key ? "rgba(76,127,248,0.15)" : "#16161C",
                  border: `1px solid ${model === m.key ? "#4C7FF8" : "#2A2A35"}`,
                  color: "#fff",
                }}
                onClick={() => {
                  setModel(m.key);
                  setResolution(m.key === "MiniMax-H3" ? "768P" : "1080P");
                }}
              >
                {m.name}
              </button>
            ))}
            <div className="ml-auto flex items-center gap-1">
              {(model === "MiniMax-H3" ? H3_RESOLUTIONS : ["480P", "720P", "1080P"]).map((r) => (
                <button
                  key={r}
                  className={`px-2.5 py-1.5 rounded-lg text-xs transition-colors`}
                  style={{
                    background: resolution === r ? "rgba(76,127,248,0.15)" : "#16161C",
                    border: `1px solid ${resolution === r ? "#4C7FF8" : "#2A2A35"}`,
                    color: "#fff",
                  }}
                  onClick={() => setResolution(r)}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>
          {/* 单元多选列表 */}
          <div className="flex items-center justify-between px-3 py-2 rounded-lg mt-3 text-xs" style={{ background: "#16161C", border: "1px solid #2A2A35" }}>
            <label className="flex items-center gap-2 cursor-pointer select-none" style={{ color: "#C9CADC" }} onClick={toggleAll}>
              <input type="checkbox" checked={allSelected} readOnly />
              全选（共 {beats.length} 个）
            </label>
            <span style={{ color: "#8B8B9A" }}>已选 {selected.size} 个</span>
          </div>
          <div className="mt-3 flex flex-col gap-1.5 max-h-72 overflow-y-auto scrollbar-thin pr-1">
            {beats.map((b) => {
              const cards = cardsMap[b.beat_id] ?? [];
              const running = cards.some((cd) => isCardActive(cd.generation_status));
              const doneAll = cards.length > 0 && cards.every((cd) => cd.generation_status === "SUCCESS");
              return (
                <label
                  key={b.beat_id}
                  className={`flex items-center gap-3 px-3 py-2 rounded-lg text-xs cursor-pointer transition-colors`}
                  style={{ background: selected.has(b.beat_id) ? "rgba(76,127,248,0.08)" : "#16161C", border: `1px solid ${selected.has(b.beat_id) ? "#4C7FF8" : "#2A2A35"}` }}
                >
                  <input type="checkbox" checked={selected.has(b.beat_id)} onChange={() => toggle(b.beat_id)} disabled={running} />
                  <span className="flex-1 truncate text-white/85">叙事单元{b.sort_order} · {b.title}</span>
                  {running ? (
                    <span className="flex items-center gap-1 shrink-0" style={{ color: "#4C7FF8" }}>
                      <Loader2 size={11} className="animate-spin" />视频生成中
                    </span>
                  ) : cards.length === 0 ? (
                    <span className="shrink-0" style={{ color: "#F5B23C" }}>未规划（将自动规划）</span>
                  ) : doneAll ? (
                    <span className="shrink-0" style={{ color: "#10B981" }}>已完成（重新生成）</span>
                  ) : (
                    <span className="shrink-0" style={{ color: "#F5B23C" }}>{cards.length - cards.filter((cd) => cd.generation_status === "SUCCESS").length}/{cards.length} 张待生成</span>
                  )}
                </label>
              );
            })}
          </div>
          {/* 预计消耗（已有卡部分） */}
          <div className="mt-4 flex items-center justify-between px-3 py-2 rounded-lg text-xs" style={{ background: "#16161C", border: "1px solid #2A2A35", color: "#8B8B9A" }}>
            <span>预计消耗（以实际生成为准{pendingSec === 0 ? "，未规划单元将在规划后计入" : ""}）</span>
            <span style={{ color: "#F5B23C", fontWeight: 600 }}>{Math.max(Math.round(pendingSec * factor), 0)} 积分</span>
          </div>
        </div>
        <div className="modal-footer">
          <button className="btn-modal-cancel" onClick={() => !submitting && onClose()}>取消</button>
          <button
            className="btn-modal-primary"
            disabled={submitting || selected.size === 0}
            onClick={() =>
              onConfirm([...selected], {
                resolution:
                  model === "MiniMax-H3"
                    ? resolution
                    : resolution === "480P"
                      ? "P480"
                      : resolution === "720P"
                        ? "P720"
                        : "P1080",
                model_key: model,
                video_model: MODEL_OPTIONS.find((m) => m.key === model)?.name ?? "Seedance 2.5",
              })
            }
          >
            {submitting ? (
              <>
                <Loader2 size={14} className="animate-spin" /> 提交中…
              </>
            ) : (
              <>开始生成 ({selected.size})</>
            )}
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
  // 项目配置快照 → 视频生成弹窗默认模型/分辨率
  const cfgVideoModel = (() => {
    const v = (proj?.config as Record<string, unknown> | undefined)?.video_model;
    return typeof v === "string" ? v : "";
  })();
  const cfgResolution = (() => {
    const v = (proj?.config as Record<string, unknown> | undefined)?.resolution;
    return typeof v === "string" ? v : "";
  })();
  const [episodes, setEpisodes] = useState<EpisodeLite[]>([]);
  const [activeEpisode, setActiveEpisode] = useState<string | null>(null);
  const [beats, setBeats] = useState<BeatSummary[]>([]);
  const [loadError, setLoadError] = useState(false);
  // 成片卡数据源：beat_id → VideoCard[]
  const [cardsMap, setCardsMap] = useState<Record<string, VideoCard[]>>({});
  const [detailsMap, setDetailsMap] = useState<Record<string, BeatDetail>>({});
  // 关联资产（17 - 工作台对齐）：beat_id → 关联资产清单；加载失败保持空（回退视觉占位）
  const [linksMap, setLinksMap] = useState<Record<string, NuAssetLinkItem[]>>({});
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [scriptOpen, setScriptOpen] = useState<Record<string, boolean>>({});
  const [genTarget, setGenTarget] = useState<BeatSummary | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // 规划视频任务确认弹窗：firstTime=false 时为重新规划（force 清空旧卡及视频）
  const [planTarget, setPlanTarget] = useState<{ beat: BeatSummary; firstTime: boolean } | null>(null);
  const [planSubmitting, setPlanSubmitting] = useState(false);
  // 成片卡提示词编辑弹窗
  const [cardPromptEdit, setCardPromptEdit] = useState<{ card: VideoCard; beatId: string } | null>(null);
  const [promptSaving, setPromptSaving] = useState(false);
  // 历史版本弹窗（S2-B05）
  const [historyTarget, setHistoryTarget] = useState<BeatSummary | null>(null);
  const [historyActive, setHistoryActive] = useState(false);
  const [versions, setVersions] = useState<NuVersionItem[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [restoringId, setRestoringId] = useState<number | null>(null);
  // 分镜生成弹窗（3.3.7c）
  const [replanTarget, setReplanTarget] = useState<BeatSummary | null>(null);
  const [replanActive, setReplanActive] = useState(false);
  const [replanApplySkill, setReplanApplySkill] = useState(false);
  const [replanSubmitting, setReplanSubmitting] = useState(false);
  // 分镜脚本生成中的单元 beat_id 集合：驱动 STORYBOARD_PLAN_V1_LUXCHAT 终态后的刷新（批量支持多单元并行）
  const [replanningIds, setReplanningIds] = useState<Set<string>>(new Set());
  // 任务级对账表：task_id → beat_id（轮询兜底时按真实任务状态判断是否完成，防止提前误判"已生成"后不再刷新）
  const replanTasksRef = useRef(new Map<string, string>());
  // 批量生成分镜脚本：多选弹窗（Set 为当前勾选的 beat_id；null=弹窗关闭）
  const [batchTarget, setBatchTarget] = useState<Set<string> | null>(null);
  const [batchSubmitting, setBatchSubmitting] = useState(false);
  // 批量生成视频：多选弹窗（仅分镜已就绪的单元；确认后逐单元 提交规划+视频生成）
  const [batchVideoTarget, setBatchVideoTarget] = useState<string[] | null>(null);
  const [batchVideoSubmitting, setBatchVideoSubmitting] = useState(false);
  // 补充缺失资产（Step2 拆分时 LLM 漏抽取的，如「银行短信」类瞬时道具）：弹窗表单
  const [addAssetTarget, setAddAssetTarget] = useState<BeatSummary | null>(null);
  const [addAssetKind, setAddAssetKind] = useState<"CHARACTER" | "SCENE" | "PROP">("PROP");
  const [addAssetName, setAddAssetName] = useState("");
  const [addAssetDesc, setAddAssetDesc] = useState("");
  const [addAssetSubmitting, setAddAssetSubmitting] = useState(false);
  // 单元内容 Tab：script=剧本原文 | storyboard=分镜脚本
  const [scriptTab, setScriptTab] = useState<Record<string, "script" | "storyboard">>({});

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
        if (b.items.length > 0) {
          setExpandedIds((prev) => (prev.size > 0 ? prev : new Set([b.items[0].beat_id])));
        }
      }
    } catch (e) {
      setLoadError(true);
      toast.error(errMsg(e, "加载失败"));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, activeEpisode]);

  // 加载各 beat 的成片卡（状态 + 视频 URL + 卡内镜头摘要）
  const loadCards = useCallback(async (beatList: BeatSummary[]) => {
    const cMap: Record<string, VideoCard[]> = {};
    await Promise.all(
      beatList.map(async (b) => {
        try {
          const d = await videoApi.beatCards(b.beat_id);
          cMap[b.beat_id] = d.items ?? [];
        } catch {
          cMap[b.beat_id] = [];
        }
      }),
    );
    setCardsMap(cMap);
    return cMap;
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
  }, [projectId, activeEpisode, loadStructure]);

  // 加载本剧集的叙事单元↔资产关联（失败静默，收起态回退视觉占位）
  const loadAssetLinks = useCallback(async (episodeId: string | null) => {
    if (!episodeId) return;
    try {
      const d = await videoApi.nuAssetLinks(episodeId);
      const map: Record<string, NuAssetLinkItem[]> = {};
      (d.items ?? []).forEach((l) => {
        (map[l.beat_id] ??= []).push(l);
      });
      setLinksMap(map);
    } catch {
      setLinksMap({});
    }
  }, []);

  useEffect(() => {
    void loadAssetLinks(activeEpisode);
  }, [activeEpisode, loadAssetLinks]);

  // 资产联动回写（17 - 工作台对齐 P2）：资产生成任务 WS 终态 → 刷新关联资产
  useWsDrivenPoll(() => void loadAssetLinks(activeEpisode), {
    active: !!activeEpisode,
    pollMs: 60000,
    slowPollMs: 0,
    match: matchTaskTypes(["CHAR_IMAGE_GEN", "SCENE_IMAGE_GEN", "PROP_IMAGE_GEN"]),
  });

  useEffect(() => {
    if (beats.length > 0) {
      void loadCards(beats);
      void loadDetails(beats);
    }
  }, [beats, loadCards, loadDetails]);

  const anyActive = useMemo(
    () => beats.some((b) => (cardsMap[b.beat_id] ?? []).some((cd) => isCardActive(cd.generation_status))),
    [beats, cardsMap],
  );

  // 定向刷新（限流治理）：只拉取有 RUNNING/QUEUED 卡片的 beat（N → 活跃数）
  const refreshRunningCards = useCallback(async (beatList: BeatSummary[], prev: Record<string, VideoCard[]>) => {
    const targets = beatList.filter((b) => (prev[b.beat_id] ?? []).some((cd) => isCardActive(cd.generation_status)));
    const next = { ...prev };
    await Promise.all(
      targets.map(async (b) => {
        try {
          const d = await videoApi.beatCards(b.beat_id);
          next[b.beat_id] = d.items ?? [];
        } catch { /* 忽略单 beat 刷新失败 */ }
      }),
    );
    setCardsMap(next);
    return next;
  }, []);

  // 有活动任务：WS 终态推送驱动刷新（视频任务），断线回退 5s 轮询；全部终态 → 刷新项目状态（解锁 Step4）
  const videoDoneRef = useRef(false);
  const refreshVideos = useCallback(async () => {
    if (videoDoneRef.current) return;
    try {
      const map = await refreshRunningCards(beats, cardsMap);
      const still = beats.some((b) => (map[b.beat_id] ?? []).some((cd) => isCardActive(cd.generation_status)));
      if (!still) {
        videoDoneRef.current = true;
        toast.success("视频生成完成");
        try {
          setProj(await projectsApi.detail(projectId));
        } catch { /* 忽略 */ }
      }
    } catch { /* 刷新容错 */ }
  }, [beats, cardsMap, refreshRunningCards, projectId]);

  useWsDrivenPoll(refreshVideos, {
    active: anyActive,
    pollMs: 5000,
    match: matchTaskTypes(["VIDEO_SEGMENT_GEN", "VIDEO_GROUP_CONCAT"]),
  });

  // 分镜脚本生成（STORYBOARD_PLAN_V1_LUXCHAT）终态对账：
  // WS 终态消息或轮询兜底触发时，按 tasksApi 真实状态核对每个任务——
  // 只有 SUCCESS 才刷新分镜+切 Tab+清守卫；FAILED 报错移除；仍在跑则保留守卫继续等
  // （旧实现不校验状态，WS 断线 5s 轮询一触发就误报"已生成"并清空守卫 → 内容永远不显示）。
  const reconcileReplanning = useCallback(async () => {
    const tasks = replanTasksRef.current;
    if (tasks.size === 0) return;
    const entries = [...tasks.entries()];
    const checked = await Promise.all(
      entries.map(async ([taskId, beatId]) => {
        try {
          const d = await tasksApi.detail(taskId);
          return { taskId, beatId, status: d.status };
        } catch {
          return { taskId, beatId, status: "" }; // 查询失败：保留守卫下次再试
        }
      }),
    );
    const doneBeats = checked.filter((r) => r.status === "SUCCESS");
    const dead = checked.filter((r) => r.status === "FAILED" || r.status === "FAILED_DEAD");
    if (doneBeats.length > 0) {
      const doneIds = new Set(doneBeats.map((r) => r.beatId));
      await loadDetails(beats.filter((b) => doneIds.has(b.beat_id)));
      setScriptTab((prev) => {
        const next = { ...prev };
        doneBeats.forEach((r) => { next[r.beatId] = "storyboard"; });
        return next;
      });
      toast.success(doneBeats.length > 1 ? `已批量生成 ${doneBeats.length} 个单元的分镜脚本` : "分镜脚本已生成");
    }
    if (dead.length > 0) {
      toast.error(`${dead.length} 个单元的分镜脚本生成失败，可重试`);
    }
    if (doneBeats.length > 0 || dead.length > 0) {
      const settled = new Set([...doneBeats, ...dead].map((r) => r.taskId));
      replanTasksRef.current = new Map(entries.filter(([tid]) => !settled.has(tid)));
      const settledBeatIds = new Set([...doneBeats, ...dead].map((r) => r.beatId));
      setReplanningIds((prev) => {
        const next = new Set(prev);
        settledBeatIds.forEach((id) => next.delete(id));
        return next;
      });
    }
  }, [beats, loadDetails]);

  useWsDrivenPoll(() => void reconcileReplanning(), {
    active: replanningIds.size > 0,
    pollMs: 5000,
    match: matchTaskTypes(["STORYBOARD_PLAN_V1_LUXCHAT"]),
  });

  // 有活动任务重新出现（新一轮生成）→ 重置终态守卫
  useEffect(() => {
    if (anyActive) videoDoneRef.current = false;
  }, [anyActive]);

  // WS 实时进度：TASK_PROGRESS → 2.5s 尾随防抖后定向刷新（突发进度合并为一次，避免打爆后端限流）
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    const handler = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        void refreshRunningCards(beats, cardsMap);
      }, 2500);
    };
    window.addEventListener("task:progress", handler);
    return () => {
      if (timer) clearTimeout(timer);
      window.removeEventListener("task:progress", handler);
    };
  }, [beats, cardsMap, refreshRunningCards]);

  const toggleExpand = (beatId: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(beatId)) next.delete(beatId);
      else next.add(beatId);
      return next;
    });
  };

  // 提交批量生成视频（单元内全部非 SUCCESS 成片卡）
  const handleGenerate = async (opts?: { resolution?: string; video_model?: string; model_key?: string; redo?: boolean }) => {
    if (!genTarget) return;
    try {
      setSubmitting(true);
      const r = await videoApi.generateBeat(genTarget.beat_id, opts);
      toast.success(`已提交 ${r.created}/${r.total} 个成片卡视频任务`);
      setGenTarget(null);
      // 回写项目配置快照：弹窗所选模型/分辨率成为本项目后续默认值
      if (opts?.model_key || opts?.resolution) {
        void projectsApi
          .updateConfig(projectId, {
            ...(opts.model_key ? { video_model: opts.model_key } : {}),
            ...(opts.resolution ? { resolution: normRes(opts.resolution) } : {}),
          })
          .catch(() => {});
      }
      setExpandedIds((prev) => new Set(prev).add(genTarget.beat_id));
      void loadCards(beats);
      try {
        setProj(await projectsApi.detail(projectId));
      } catch { /* 忽略 */ }
    } catch (e) {
      toast.error(errMsg(e, "生成视频失败"));
    } finally {
      setSubmitting(false);
    }
  };

  // 规划视频任务（成片卡编组）：已有卡片时二次确认 force 清空旧卡及视频
  const confirmPlan = async () => {
    if (!planTarget) return;
    try {
      setPlanSubmitting(true);
      const r = await videoApi.planVideoTasks(planTarget.beat.beat_id, !planTarget.firstTime);
      toast.success(`已规划 ${r.total_cards} 张成片卡（${r.total_shots} 个分镜镜头）`);
      setPlanTarget(null);
      void loadCards(beats);
    } catch (e) {
      toast.error(errMsg(e, "规划视频任务失败"));
    } finally {
      setPlanSubmitting(false);
    }
  };

  // 单成片卡重新生成（计费冻结 + Outbox 派发）
  const handleRegenerateCard = async (card: VideoCard) => {
    try {
      await videoApi.regenerateCard(String(card.card_uuid));
      toast.success(`${card.title} 视频生成任务已提交`);
      void loadCards(beats);
    } catch (e) {
      toast.error(errMsg(e, "提交成片卡生成失败"));
    }
  };

  // 保存成片卡提示词（PATCH /video-cards/:uuid/prompt）：空串恢复自动组装
  const handleSaveCardPrompt = async (value: string) => {
    if (!cardPromptEdit) return;
    setPromptSaving(true);
    try {
      await videoApi.updateCardPrompt(String(cardPromptEdit.card.card_uuid), value);
      toast.success(value ? "成片卡提示词已更新，重新生成后生效" : "已恢复自动组装提示词");
      setCardPromptEdit(null);
      void loadCards(beats);
    } catch (e) {
      toast.error(errMsg(e, "保存提示词失败"));
    } finally {
      setPromptSaving(false);
    }
  };

  // 调整成片卡送模型时长（PATCH /video-cards/:uuid/duration）
  const handleCardDurationChange = async (beatId: string, card: VideoCard, dur: number) => {
    try {
      await videoApi.updateCardDuration(String(card.card_uuid), dur);
      toast.success(`${card.title} 时长已调整为 ${dur}s`);
      setCardsMap((prev) => ({
        ...prev,
        [beatId]: (prev[beatId] ?? []).map((c) => (c.card_id === card.card_id ? { ...c, duration_sec: dur } : c)),
      }));
    } catch (e) {
      toast.error(errMsg(e, "调整卡时长失败"));
    }
  };

  // 拖拽绑定资产到成片卡（POST bind；作用于卡内全部镜头；成功后刷新该 beat 卡片）
  const handleBindAsset = async (beatId: string, card: VideoCard, kind: string, id: number) => {
    try {
      const r = await videoApi.bindCardAsset(String(card.card_uuid), kind, id);
      toast.success(`已绑定 ${r.bound_shots} 个镜头的参考图`);
      void refreshBeatCards(beatId);
    } catch (e) {
      toast.error(errMsg(e, "绑定资产失败"));
    }
  };

  // 解绑卡参考资产（× 按钮）
  const handleUnbindAsset = async (beatId: string, card: VideoCard, kind: string, id: number) => {
    try {
      await videoApi.unbindCardAsset(String(card.card_uuid), kind, id);
      toast.success("已解绑参考图");
      void refreshBeatCards(beatId);
    } catch (e) {
      toast.error(errMsg(e, "解绑失败"));
    }
  };

  // 单 beat 卡片定向刷新（绑定/解绑后局部更新）
  const refreshBeatCards = useCallback(async (beatId: string) => {
    try {
      const d = await videoApi.beatCards(beatId);
      setCardsMap((prev) => ({ ...prev, [beatId]: d.items ?? [] }));
    } catch { /* 刷新容错 */ }
  }, []);

  // 单 beat 分镜详情定向刷新（镜头级绑定/解绑后局部更新 linked_assets）
  const refreshBeatDetail = useCallback(async (beatId: string) => {
    try {
      const d = await splitApi.beatDetail(beatId);
      setDetailsMap((prev) => ({ ...prev, [beatId]: d }));
    } catch { /* 刷新容错 */ }
  }, []);

  // 镜头级绑定参考资产（拖资产 chip 到分镜脚本单个镜头行；POST /shots/:id/assets/bind）
  const handleBindShotAsset = (beatId: string, shotId: string, kind: string, assetId: number) => {
    void (async () => {
      try {
        const r = await videoApi.bindShotAsset(shotId, kind, assetId);
        toast.success(r.inserted ? "已精准绑定到该镜头" : "该镜头已包含此参考资产");
        void refreshBeatDetail(beatId);
      } catch (e) {
        toast.error(errMsg(e, "绑定资产失败"));
      }
    })();
  };

  // 镜头级解绑参考资产（× 按钮；removed=false 表示引用来自叙事单元级继承，无法按镜头单独移除）
  const handleUnbindShotAsset = (beatId: string, shotId: string, kind: string, assetId: number) => {
    void (async () => {
      try {
        const r = await videoApi.unbindShotAsset(shotId, kind, assetId);
        if (!r.removed) toast.info("该引用来自叙事单元级关联，如需调整请在重新生成分镜脚本时处理");
        else toast.success("已从该镜头移除参考资产");
        void refreshBeatDetail(beatId);
      } catch (e) {
        toast.error(errMsg(e, "解绑失败"));
      }
    })();
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
  void detailOf; // 预留给镜头级编辑入口

  // 弹窗入场动画：挂载后短暂延迟加 active，触发 opacity/scale transition
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

  // 恢复到指定版本：成功后重载结构/成片卡/详情（项目状态可能回退至 PARTIAL_ASSETS）
  const restoreVersion = async (v: NuVersionItem) => {
    if (!historyTarget) return;
    try {
      setRestoringId(v.version_id);
      await versionApi.rollback(historyTarget.beat_id, v.version_id);
      toast.success(`已恢复到 v${v.version_num}`);
      setHistoryTarget(null);
      setCardsMap((prev) => ({ ...prev, [historyTarget.beat_id]: [] })); // 回滚作废旧卡
      void loadStructure();
      void loadDetails(beats);
    } catch (e) {
      toast.error(errMsg(e, "恢复失败"));
    } finally {
      setRestoringId(null);
    }
  };

  // 确认生成分镜脚本（3.3.7c）：后端入队 STORYBOARD_PLAN_V1_LUXCHAT 真实任务
  const confirmReplan = async () => {
    if (!replanTarget) return;
    try {
      setReplanSubmitting(true);
      const resp = await splitApi.replan(replanTarget.beat_id);
      if (resp.task_id) replanTasksRef.current.set(resp.task_id, replanTarget.beat_id);
      toast.success("分镜脚本生成任务已提交，请稍候");
      setScriptTab((prev) => ({ ...prev, [replanTarget.beat_id]: "storyboard" }));
      setReplanningIds((prev) => new Set(prev).add(replanTarget.beat_id));
      setReplanTarget(null);
    } catch (e) {
      toast.error(errMsg(e, "提交分镜生成失败"));
    } finally {
      setReplanSubmitting(false);
    }
  };

  // 批量生成分镜脚本：多选弹窗确认 → 复用单单元 replan 接口串行提交（后端按 Outbox 队列自然排队）
  const confirmBatchStoryboard = async (beatIds: string[]) => {
    if (beatIds.length === 0) return;
    try {
      setBatchSubmitting(true);
      let ok = 0;
      const failed: string[] = [];
      for (const id of beatIds) {
        try {
          const resp = await splitApi.replan(id);
          if (resp.task_id) replanTasksRef.current.set(resp.task_id, id);
          ok++;
          setReplanningIds((prev) => new Set(prev).add(id));
        } catch {
          failed.push(beats.find((b) => b.beat_id === id)?.title ?? id);
        }
      }
      if (failed.length > 0) toast.error(`以下单元提交失败：${failed.join("、")}`);
      if (ok > 0) toast.success(`已提交 ${ok} 个叙事单元的分镜脚本生成，完成后将自动刷新`);
      setBatchTarget(null);
    } finally {
      setBatchSubmitting(false);
    }
  };

  // 「批量生成视频」入口：有单元缺分镜 → 先引导批量分镜；否则打开批量视频多选弹窗（仅分镜就绪的单元）
  const openGenerateVideoBatch = () => {
    const missing = beats.filter((b) => !storyboardReady(b.beat_id));
    if (missing.length > 0) {
      toast.warning(`${missing.length} 个叙事单元尚未生成分镜脚本，请先批量生成分镜`);
      setBatchTarget(new Set(missing.map((b) => b.beat_id)));
      return;
    }
    const ready = beats.filter((b) => storyboardReady(b.beat_id));
    if (ready.length === 0) {
      toast.info("暂无可生成的叙事单元");
      return;
    }
    setBatchVideoTarget(ready.map((b) => b.beat_id)); // 默认全选分镜已就绪的单元
  };

  // 批量生成视频提交：全部单元并行提交（无成片卡先自动规划，再提交 generateBeat）。
  // 并行而非串行：哪个单元先提交成功，其视频任务就先进入 AutoDL 队列先渲染；避免单个慢请求阻塞后续单元。
  const confirmBatchVideos = async (ids: string[], opts: { resolution?: string; model_key?: string; video_model?: string }) => {
    if (ids.length === 0) return;
    try {
      setBatchVideoSubmitting(true);
      let plannedCount = 0;
      let createdTotal = 0;
      let unitCount = 0;
      const failedTitles: string[] = [];
      await Promise.all(
        ids.map(async (id) => {
          const title = beats.find((b) => b.beat_id === id)?.title ?? id;
          try {
            const cards = cardsMap[id] ?? [];
            if (cards.length === 0) {
              await videoApi.planVideoTasks(id, false);
              plannedCount++;
            }
            const allDone = cards.length > 0 && cards.every((cd) => cd.generation_status === "SUCCESS");
            const r = await videoApi.generateBeat(id, { ...opts, redo: allDone });
            createdTotal += r.created;
            unitCount++;
          } catch {
            failedTitles.push(title);
          }
        }),
      );
      if (failedTitles.length > 0) toast.error(`以下单元提交失败：${failedTitles.join("、")}`);
      if (unitCount > 0) {
        toast.success(
          `已为 ${unitCount} 个单元提交视频生成${plannedCount > 0 ? `（自动规划 ${plannedCount} 个）` : ""}，共 ${createdTotal} 个任务，每张完成即实时上屏`,
        );
        // 回写项目配置快照：弹窗所选模型/分辨率成为本项目后续默认值
        if (opts?.model_key || opts?.resolution) {
          void projectsApi
            .updateConfig(projectId, {
              ...(opts.model_key ? { video_model: opts.model_key } : {}),
              ...(opts.resolution ? { resolution: normRes(opts.resolution) } : {}),
            })
            .catch(() => {});
        }
      }
      setBatchVideoTarget(null);
      void loadCards(beats);
      try {
        setProj(await projectsApi.detail(projectId));
      } catch { /* 忽略 */ }
    } finally {
      setBatchVideoSubmitting(false);
    }
  };

  // 分镜脚本是否已就绪（有镜头块，或正在生成中视为进行中而非就绪）
  const storyboardReady = (beatId: string): boolean =>
    (detailsMap[beatId]?.shot_groups ?? []).some((g) => g.shots.length > 0);

  // 补充缺失资产：创建（同名幂等复用）→ 挂当前单元 → 触发参考图生成 → 刷新关联资产
  const submitAddAsset = async () => {
    if (!addAssetTarget || !addAssetName.trim()) return;
    try {
      setAddAssetSubmitting(true);
      const r = await assetsApi.createLinkedAsset(addAssetTarget.beat_id, {
        asset_kind: addAssetKind,
        name: addAssetName.trim(),
        description: addAssetDesc.trim() || undefined,
      });
      // 复用 Step2 batch-generate 链路生成参考图（计费/回写/WS 全部走既有链路）
      const genKind = addAssetKind === "CHARACTER" ? "CHARACTER_VARIANT" : addAssetKind;
      try {
        await assetsApi.batchGenerate({
          episode_id: activeEpisode ?? "",
          items: [{ kind: genKind as "CHARACTER_VARIANT" | "SCENE" | "PROP", target_id: r.asset_id }],
        });
        toast.success(`已创建「${addAssetName.trim()}」并开始生成参考图，完成后自动出现在关联资产中`);
      } catch {
        toast.info("资产已创建并关联到本单元；参考图生成提交失败，可稍后在 Step2 手动生成");
      }
      setAddAssetTarget(null);
      setAddAssetName("");
      setAddAssetDesc("");
      void loadAssetLinks(activeEpisode);
    } catch (e) {
      toast.error(errMsg(e, "创建资产失败"));
    } finally {
      setAddAssetSubmitting(false);
    }
  };

  // 待生成成片卡总数（底部悬浮条计数）
  const pendingCount = useMemo(
    () => beats.reduce((acc, b) => acc + (cardsMap[b.beat_id] ?? []).filter((cd) => !["SUCCESS"].includes(cd.generation_status)).length, 0),
    [beats, cardsMap],
  );

  // 项目加载失败（404/无权限/后端异常）整页降级，避免无限"加载中…"
  if (loadError) return <ProjectLoadError />;

  return (
    <WorkbenchShell
      projectTitle={proj?.title ?? ""}
      currentStep={3}
      canClick={stepState.canClick}
      goStep={stepState.goStep}
      stepDone={stepState.stepDone}
      episodes={episodes}
      activeEpisodeId={activeEpisode}
      onSelectEpisode={setActiveEpisode}
      activeEpisodeBeats={beats}
    >
      {/* Settings Bar（视频模型/分辨率/比例/风格 + 高级设置） */}
      <div className="flex items-center justify-between px-6 py-2 flex-shrink-0">
        <div className="flex items-center gap-3 mx-auto" style={{ maxWidth: "85%", width: "100%" }}>
          {(() => {
            // 项目配置快照 → 设置条显示值（与生成弹窗默认值同源）
            const cfgStr = (k: string) => {
              const v = (proj?.config as Record<string, unknown> | undefined)?.[k];
              return typeof v === "string" ? v : "";
            };
            const videoModelName = MODEL_OPTIONS.find((m) => m.key === resolveModelKey(cfgVideoModel))?.name ?? (cfgVideoModel || "Seedance 2.5");
            return [
              { label: "视频模型", value: videoModelName },
              { label: "分辨率", value: normRes(cfgResolution) || "1080P" },
              { label: "比例", value: cfgStr("aspect_ratio") || "16:9" },
              { label: "风格", value: cfgStr("style_name") || proj?.tags?.[0] || "默认风格" },
            ];
          })().map((it) => (
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
          {beats.length === 0 && episodes.length === 0 && (
            /* 未拆分空态引导 */
            <div className="rounded-2xl border border-dashed border-white/10 p-14 flex flex-col items-center gap-4 text-center">
              <Clapperboard size={32} className="text-[#6B7088]" />
              <div className="text-sm text-white/70">完成剧本拆分后，可在此按叙事单元生成分镜与视频</div>
              <button
                className="h-10 px-6 rounded-xl flex items-center gap-2 text-sm text-white font-medium transition-all hover:opacity-90"
                style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                onClick={() => stepState.goStep(1)}
              >
                前往剧本拆分
              </button>
            </div>
          )}

          {beats.length === 0 && episodes.length > 0 && (
            <div className="rounded-2xl border border-dashed border-white/10 p-14 text-center text-sm text-slate-500">
              本集暂无叙事单元，可在 Step1 补充或新增。
            </div>
          )}

          {beats.map((beat) => {
            const cards = cardsMap[beat.beat_id] ?? [];
            const expanded = expandedIds.has(beat.beat_id);
            const running = cards.some((cd) => isCardActive(cd.generation_status));
            const doneCount = cards.filter((cd) => cd.generation_status === "SUCCESS").length;
            const allDone = cards.length > 0 && doneCount === cards.length;
            const shotsExist = (detailsMap[beat.beat_id]?.shot_groups ?? []).some((g) => g.shots.length > 0)
              || replanningIds.has(beat.beat_id)
              || cards.length > 0;
            const showScript = scriptOpen[beat.beat_id] ?? true;
            const tab = scriptTab[beat.beat_id] ?? "script";
            const generating = replanningIds.has(beat.beat_id);
            const links = linksMap[beat.beat_id] ?? [];
            const readyCount = links.filter(assetReady).length;

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
                  {/* 状态横幅：分镜生成中 → 未生成分镜 → 未规划 → 视频生成中 / 全部完成 / 待生成视频 */}
                  {expanded && (
                    generating ? (
                      <div className="unit-status-banner running">
                        <Loader2 size={13} className="animate-spin" />
                        <span>分镜脚本生成中，请稍候…</span>
                      </div>
                    ) : !shotsExist ? (
                      <div className="unit-status-banner">
                        <Sparkles size={13} />
                        <span>尚未生成分镜脚本，请先「生成分镜脚本」</span>
                      </div>
                    ) : cards.length === 0 ? (
                      <div className="unit-status-banner">
                        <Sparkles size={13} />
                        <span>分镜脚本已就绪，请「规划视频任务」编组成片卡</span>
                      </div>
                    ) : running ? (
                      <div className="unit-status-banner running">
                        <Loader2 size={13} className="animate-spin" />
                        <span>视频生成中（{doneCount}/{cards.length} 张成片卡完成），请稍候…</span>
                      </div>
                    ) : allDone ? (
                      <div className="unit-status-banner">
                        <CheckCircle2 size={13} />
                        <span>{cards.length} 张成片卡视频已全部生成完成</span>
                      </div>
                    ) : (
                      <div className="unit-status-banner">
                        <CheckCircle2 size={13} />
                        <span>已规划为 {cards.length} 张成片卡（{doneCount} 完成），请检查后生成视频</span>
                      </div>
                    )
                  )}
                  <div className="unit-actions" onClick={(e) => e.stopPropagation()}>
                    <span className="unit-duration-label">时长</span>
                    <span className="unit-duration-value">{beat.preset_duration_sec}秒</span>
                    {links.length > 0 && (
                      /* 单元级资产就绪度（17 - 工作台对齐）：n/m 已就绪，点击回 Step2 补齐 */
                      <span
                        className="text-[10px] px-2 py-0.5 rounded cursor-pointer transition-opacity hover:opacity-80"
                        style={{
                          background: readyCount === links.length ? "rgba(16,185,129,0.15)" : "rgba(245,178,60,0.15)",
                          color: readyCount === links.length ? "#10B981" : "#F5B23C",
                        }}
                        title={`关联资产 ${readyCount}/${links.length} 已就绪${readyCount < links.length ? "，未就绪资产生成视频时将走纯文生视频" : ""}`}
                        onClick={() => stepState.goStep(2)}
                      >
                        资产 {readyCount}/{links.length}
                      </span>
                    )}
                    <button className="icon-btn icon-btn-sm" onClick={() => toast.info("更多操作即将上线")}>
                      <MoreHorizontal size={16} />
                    </button>
                    {expanded ? (
                      <>
                        <div className="unit-history" onClick={() => void openHistory(beat)}>
                          <History size={12} />
                          <span>历史版本</span>
                        </div>
                        {!shotsExist || generating ? (
                          /* 先生成分镜脚本；生成中禁用防重复提交 */
                          <button
                            className="btn-replan btn-gen-storyboard"
                            disabled={generating}
                            onClick={() => setReplanTarget(beat)}
                          >
                            <Sparkles size={12} />
                            <span>{generating ? "分镜生成中…" : "生成分镜脚本"}</span>
                          </button>
                        ) : cards.length === 0 ? (
                          /* 分镜已就绪 → 规划视频任务（成片卡编组） */
                          <button
                            className="btn-generate-video"
                            onClick={() => setPlanTarget({ beat, firstTime: true })}
                          >
                            <Sparkles size={14} />
                            <span>规划视频任务</span>
                          </button>
                        ) : (
                          <>
                            <button className="btn-replan" onClick={() => setPlanTarget({ beat, firstTime: false })}>
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
                        )}
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
                    {/* 剧本原文 / 分镜脚本 Tab（对齐 LuxReal：分镜脚本正文为 Step3 核心可视物） */}
                    <div className="unit-subheader">
                      <div className="unit-tab-group">
                        <span
                          className={`unit-tab ${tab === "script" ? "active" : ""}`}
                          onClick={() => setScriptTab((prev) => ({ ...prev, [beat.beat_id]: "script" }))}
                        >
                          剧本原文
                        </span>
                        <span
                          className={`unit-tab ${tab === "storyboard" ? "active" : ""}`}
                          onClick={() => setScriptTab((prev) => ({ ...prev, [beat.beat_id]: "storyboard" }))}
                        >
                          分镜脚本{shotsExist && cards.length === 0 ? ` · 已就绪` : ""}
                        </span>
                      </div>
                      <div
                        className="unit-toggle"
                        onClick={() => setScriptOpen((prev) => ({ ...prev, [beat.beat_id]: !showScript }))}
                      >
                        <ChevronDown size={12} style={{ transform: showScript ? "rotate(180deg)" : undefined, transition: "transform 0.2s" }} />
                        <span>{showScript ? "收起" : "展开"}</span>
                      </div>
                    </div>
                    {showScript && tab === "script" && <div className="script-preview">{beat.script_text}</div>}
                    {showScript && tab === "storyboard" && (
                      <>
                        {/* 拖拽源：资产条 + 补充缺失资产入口 */}
                        <div className="flex items-start gap-2">
                          <AssetPalette links={links} onGoAssets={() => stepState.goStep(2)} />
                          <button
                            className="flex items-center gap-1 px-2.5 py-1 rounded-lg text-[11px] shrink-0 transition-colors"
                            style={{ background: "#16161C", border: "1px dashed #3A3A4A", color: "#8B8B9A" }}
                            title="Step2 拆分时遗漏的资产可在此补充创建并生成参考图，自动归属本集"
                            onClick={() => {
                              setAddAssetKind("PROP");
                              setAddAssetName("");
                              setAddAssetDesc("");
                              setAddAssetTarget(beat);
                            }}
                          >
                            <Plus size={12} /> 补充资产
                          </button>
                        </div>
                        <StoryboardScriptView
                          detail={detailsMap[beat.beat_id]}
                          generating={generating}
                          bindEnabled={!generating}
                          onBindAsset={(shotId, kind, assetId) => handleBindShotAsset(beat.beat_id, shotId, kind, assetId)}
                          onUnbindAsset={(shotId, kind, assetId) => handleUnbindShotAsset(beat.beat_id, shotId, kind, assetId)}
                        />
                      </>
                    )}
                    {/* 成片卡网格（规划视频任务产物） */}
                    {cards.length > 0 ? (
                      <>
                        <AssetPalette links={links} onGoAssets={() => stepState.goStep(2)} />
                        <div className="shots-grid">
                          {cards.map((cd, ci) => (
                            <VideoCardView
                              key={cd.card_uuid || cd.card_id}
                              card={cd}
                              index={ci + 1}
                              onRegenerate={(c) => void handleRegenerateCard(c)}
                              onEditPrompt={(c) => setCardPromptEdit({ card: c, beatId: beat.beat_id })}
                              onDurationChange={(c, dur) => void handleCardDurationChange(beat.beat_id, c, dur)}
                              onBindAsset={(c, kind, id) => void handleBindAsset(beat.beat_id, c, kind, id)}
                              onUnbindAsset={(c, kind, id) => void handleUnbindAsset(beat.beat_id, c, kind, id)}
                              onGoAssets={() => stepState.goStep(2)}
                            />
                          ))}
                        </div>
                      </>
                    ) : (
                      shotsExist && !generating && tab !== "script" ? null : (
                        <div className="px-5 pb-5 text-xs text-[#6B7088]">
                          {shotsExist
                            ? "分镜脚本已就绪 — 点击上方「规划视频任务」，把连续分镜镜头编组成片卡后再生成视频。"
                            : "尚未生成分镜脚本 — 点击上方「生成分镜脚本」生成该单元的分镜（2~3 秒细粒度镜头）。"}
                        </div>
                      )
                    )}
                  </>
                ) : (
                  <>
                    <div className="script-preview-collapsed">{beat.script_text}</div>
                    {/* 关联资产（17 - 工作台对齐）：真实关联数据 chips（就绪缩略图 / 未就绪名称占位） */}
                    <div className="assets-row">
                      <span className="assets-label">关联资产</span>
                      <div className="assets-avatars">
                        {links.length > 0 ? (
                          <>
                            {links.slice(0, 5).map((l) => (
                              <AssetChip key={`${l.asset_kind}-${l.asset_id}`} link={l} onGoAssets={() => stepState.goStep(2)} />
                            ))}
                            {links.length > 5 && (
                              <div className="asset-more" title={`共 ${links.length} 个关联资产`} onClick={() => stepState.goStep(2)}>
                                +{links.length - 5}
                              </div>
                            )}
                          </>
                        ) : (
                          <>
                            <div className="asset-avatar asset-avatar-1" />
                            <div className="asset-avatar asset-avatar-2" />
                            <div className="asset-avatar asset-avatar-3" />
                            <div className="asset-more">+{beat.shot_group_count > 3 ? beat.shot_group_count - 3 : 0}</div>
                          </>
                        )}
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
        {/* 批量生成分镜脚本（生成视频前置步骤）：多选叙事单元 → 串行提交 STORYBOARD_PLAN */}
        <button
          className="btn-agent-config"
          disabled={batchSubmitting || replanningIds.size > 0}
          onClick={() => {
            const pending = beats.filter((b) => !storyboardReady(b.beat_id) && !replanningIds.has(b.beat_id));
            setBatchTarget(new Set(pending.map((b) => b.beat_id))); // 默认勾选缺失分镜的单元
          }}
        >
          {replanningIds.size > 0 ? (
            <>
              <Loader2 size={16} className="animate-spin" />
              <span>分镜生成中({replanningIds.size})</span>
            </>
          ) : (
            <>
              <Sparkles size={16} />
              <span>批量生成分镜</span>
            </>
          )}
        </button>
        <button
          className="btn-gen-video-float"
          onClick={openGenerateVideoBatch}
        >
          <span>批量生成视频</span>
          <span className="gen-video-count">({pendingCount})</span>
          <ChevronDown size={16} />
        </button>
      </div>

      {/* 批量生成分镜脚本弹窗：多选叙事单元 */}
      {batchTarget && (() => {
        const selected = batchTarget;
        const toggle = (id: string) => {
          const next = new Set(selected);
          if (next.has(id)) next.delete(id);
          else next.add(id);
          setBatchTarget(next);
        };
        const allSelected = selected.size === beats.length;
        const toggleAll = () => setBatchTarget(allSelected ? new Set() : new Set(beats.map((b) => b.beat_id)));
        return (
          <div className={`replan-modal-overlay active`} onClick={() => !batchSubmitting && setBatchTarget(null)}>
            <div className="modal-box" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <div className="modal-title">批量生成分镜脚本</div>
              </div>
              <div className="modal-body">
                <div className="confirm-message">
                  勾选需要生成分镜脚本的叙事单元，将按顺序逐个提交 AI 生成分镜任务；已就绪的单元可跳过。生成视频前，所选单元必须已有分镜脚本。
                </div>
                <div
                  className="flex items-center justify-between px-3 py-2 rounded-lg mt-3 text-xs"
                  style={{ background: "#16161C", border: "1px solid #2A2A35" }}
                >
                  <label className="flex items-center gap-2 cursor-pointer select-none" style={{ color: "#C9CADC" }} onClick={toggleAll}>
                    <input type="checkbox" checked={allSelected} readOnly />
                    全选（共 {beats.length} 个）
                  </label>
                  <span style={{ color: "#8B8B9A" }}>已选 {selected.size} 个</span>
                </div>
                <div className="mt-3 flex flex-col gap-1.5 max-h-72 overflow-y-auto scrollbar-thin pr-1">
                  {beats.map((b) => {
                    const ready = storyboardReady(b.beat_id);
                    const gen = replanningIds.has(b.beat_id);
                    return (
                      <label
                        key={b.beat_id}
                        className={`flex items-center gap-3 px-3 py-2 rounded-lg text-xs cursor-pointer transition-colors ${selected.has(b.beat_id) ? "border-[#4C7FF8]" : ""}`}
                        style={{ background: selected.has(b.beat_id) ? "rgba(76,127,248,0.08)" : "#16161C", border: `1px solid ${selected.has(b.beat_id) ? "#4C7FF8" : "#2A2A35"}` }}
                      >
                        <input type="checkbox" checked={selected.has(b.beat_id)} onChange={() => toggle(b.beat_id)} />
                        <span className="flex-1 truncate text-white/85">叙事单元{b.sort_order} · {b.title}</span>
                        {gen ? (
                          <span className="flex items-center gap-1 shrink-0" style={{ color: "#4C7FF8" }}>
                            <Loader2 size={11} className="animate-spin" />生成中
                          </span>
                        ) : ready ? (
                          <span className="shrink-0" style={{ color: "#10B981" }}>分镜已就绪</span>
                        ) : (
                          <span className="shrink-0" style={{ color: "#F5B23C" }}>未生成</span>
                        )}
                      </label>
                    );
                  })}
                </div>
              </div>
              <div className="modal-footer">
                <button className="btn-modal-cancel" onClick={() => !batchSubmitting && setBatchTarget(null)}>取消</button>
                <button
                  className="btn-modal-primary"
                  disabled={batchSubmitting || selected.size === 0}
                  onClick={() => void confirmBatchStoryboard([...selected])}
                >
                  {batchSubmitting ? (
                    <>
                      <Loader2 size={14} className="animate-spin" /> 提交中…
                    </>
                  ) : (
                    <>生成所选 ({selected.size})</>
                  )}
                </button>
              </div>
            </div>
          </div>
        );
      })()}

      {/* 生成视频弹窗 */}
      {genTarget && (
        <GenVideoModal
          beat={genTarget}
          cards={cardsMap[genTarget.beat_id] ?? []}
          links={linksMap[genTarget.beat_id] ?? []}
          submitting={submitting}
          onClose={() => !submitting && setGenTarget(null)}
          onConfirm={(opts) => void handleGenerate(opts)}
        />
      )}

      {/* 批量生成视频弹窗：多选分镜已就绪的叙事单元流水线提交 */}
      {batchVideoTarget && (
        <BatchVideoModal
          beats={beats.filter((b) => batchVideoTarget.includes(b.beat_id))}
          cardsMap={cardsMap}
          defaultSelected={batchVideoTarget}
          submitting={batchVideoSubmitting}
          defaultModel={cfgVideoModel}
          defaultResolution={cfgResolution}
          onClose={() => !batchVideoSubmitting && setBatchVideoTarget(null)}
          onConfirm={(ids, opts) => void confirmBatchVideos(ids, opts)}
        />
      )}

      {/* 成片卡提示词编辑弹窗（PATCH /video-cards/:uuid/prompt） */}
      {cardPromptEdit && (
        <CardPromptEditModal
          cardTitle={cardPromptEdit.card.title || `成片卡${cardPromptEdit.card.card_id}`}
          init={cardPromptEdit.card.prompt_text}
          saving={promptSaving}
          onClose={() => !promptSaving && setCardPromptEdit(null)}
          onSave={(value) => void handleSaveCardPrompt(value)}
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

      {/* 分镜脚本生成弹窗（语义区分：首次生成 / 重新生成可回滚） */}
      {replanTarget && (() => {
        const firstTime = (detailsMap[replanTarget.beat_id]?.shot_groups ?? []).every((g) => g.shots.length === 0);
        return (
          <div
            className={`replan-modal-overlay ${replanActive ? "active" : ""}`}
            onClick={() => !replanSubmitting && setReplanTarget(null)}
          >
            <div className="modal-box" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <div className="modal-title">{firstTime ? "生成分镜脚本" : "重新生成分镜脚本"}</div>
              </div>
              <div className="modal-body">
                <div className="confirm-message">
                  {firstTime
                    ? "将根据剧本原文与时长预设，为本单元生成 2~3 秒细粒度的 8 字段分镜脚本（景别/运镜/视角/主体/背景/光影/台词/环境音）。"
                    : "将重新生成本单元的分镜脚本并保存为新版本（旧版可回滚）；已规划的成片卡和已生成的视频将被清空。"}
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
                <button
                  className={firstTime ? "btn-modal-primary" : "btn-modal-warning"}
                  disabled={replanSubmitting}
                  onClick={() => void confirmReplan()}
                >
                  {replanSubmitting ? "提交中…" : firstTime ? "生成分镜脚本" : "重新生成"}
                </button>
              </div>
            </div>
          </div>
        );
      })()}

      {/* 补充缺失资产弹窗（Step2 拆分遗漏 → 此处补建并生成参考图，归属本集） */}
      {addAssetTarget && (
        <div className="replan-modal-overlay active" onClick={() => !addAssetSubmitting && setAddAssetTarget(null)}>
          <div className="modal-box" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <div className="modal-title">补充关联资产</div>
            </div>
            <div className="modal-body">
              <div className="confirm-message">
                分镜中引用但 Step2 资产阶段未提取到的资产（如瞬时道具），在此补建后自动归属当前剧集并生成参考图；
                同名资产已存在时会直接复用。创建后可拖拽绑定到具体镜头。
              </div>
              <div className="mt-3 flex items-center gap-1.5">
                {([["PROP", "道具"], ["CHARACTER", "角色"], ["SCENE", "场景"]] as const).map(([k, label]) => (
                  <button
                    key={k}
                    className={`px-3 py-1.5 rounded-lg text-xs transition-colors`}
                    style={{
                      background: addAssetKind === k ? "rgba(76,127,248,0.15)" : "#16161C",
                      border: `1px solid ${addAssetKind === k ? "#4C7FF8" : "#2A2A35"}`,
                      color: "#fff",
                    }}
                    onClick={() => setAddAssetKind(k)}
                  >
                    {label}
                  </button>
                ))}
              </div>
              <input
                className="luxreal-form-input mt-3"
                value={addAssetName}
                onChange={(e) => setAddAssetName(e.target.value)}
                placeholder={addAssetKind === "SCENE" ? "场景名称（如：银行大厅）" : "资产名称（如：银行短信）"}
                maxLength={64}
                autoFocus
              />
              <textarea
                className="luxreal-form-input mt-2"
                style={{ minHeight: 72 }}
                value={addAssetDesc}
                onChange={(e) => setAddAssetDesc(e.target.value)}
                placeholder="外观/内容描述（可选，越具体生成的参考图越准确）"
                maxLength={2000}
              />
            </div>
            <div className="modal-footer">
              <button className="btn-modal-cancel" onClick={() => !addAssetSubmitting && setAddAssetTarget(null)}>取消</button>
              <button
                className="btn-modal-primary"
                disabled={addAssetSubmitting || !addAssetName.trim()}
                onClick={() => void submitAddAsset()}
              >
                {addAssetSubmitting ? (
                  <>
                    <Loader2 size={14} className="animate-spin" /> 创建中…
                  </>
                ) : (
                  <>创建并生成参考图</>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 规划视频任务确认弹窗（成片卡编组） */}
      {planTarget && (
        <div className="replan-modal-overlay active" onClick={() => !planSubmitting && setPlanTarget(null)}>
          <div className="modal-box" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <div className="modal-title">{planTarget.firstTime ? "规划视频任务" : "重新规划视频任务"}</div>
            </div>
            <div className="modal-body">
              <div className="confirm-message">
                {planTarget.firstTime
                  ? "AI 将把该单元的连续分镜镜头编组为若干张成片卡（每张 2~4 个镜头、总时长约 4~10 秒），每张成片卡为一个视频生成任务。"
                  : "将按当前分镜脚本重新编组成片卡；已有的成片卡及其生成的视频将被清空。"}
              </div>
              <div className="mt-2 text-xs leading-relaxed" style={{ color: "#8B8B9A" }}>
                提示：如需调整镜头划分，请先在「分镜脚本」中使用重新生成分镜脚本，再回来规划。
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn-modal-cancel" onClick={() => !planSubmitting && setPlanTarget(null)}>取消</button>
              <button
                className="btn-modal-primary"
                disabled={planSubmitting}
                onClick={() => void confirmPlan()}
              >
                {planSubmitting ? "规划中…" : planTarget.firstTime ? "开始规划" : "重新规划"}
              </button>
            </div>
          </div>
        </div>
      )}
    </WorkbenchShell>
  );
}
