// 创作入口页 —— 按 luxreal-clone 设计稿 create-video.html 融合（批次 1）
// CSS 原样提取（全局 token 585 行 + 页面 2590 行）→ src/pages/home.css（铁律①）
// 接口对接：GET /projects、POST /projects（01 文档 4.2）
// 左侧 72px 侧边栏按设计稿照搬（AppShell 雏形，批次 2 抽为共享布局）
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  AlertCircle, Check, ChevronDown, ChevronLeft, ChevronRight, Clapperboard, Coins, Compass, Copy,
  Download, Edit3, FileText, FileUp, Folder, FolderOpen, HelpCircle, Image,
  Megaphone, MoreHorizontal, Palette, Play, Settings, Sparkles, ThumbsUp, Trash2, Wand2, X,
} from "lucide-react";

import { api, ApiError, errorMessage } from "@duanju/api-client";
import AgentSkillModal from "../components/AgentSkillModal";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
import { cachedGet, invalidateCachedGet } from "../utils/cachedApi";
import { useAuthStore } from "../stores/authStore";
import "./home.css";

// 封面素材（ImageGen 生成，替代设计稿渐变占位）
import coverImg1 from "../assets/covers/project-cover-1.jpg";
import coverImg2 from "../assets/covers/project-cover-2.jpg";
import coverImg3 from "../assets/covers/project-cover-3.jpg";
import coverImg4 from "../assets/covers/project-cover-4.jpg";
import bannerMember from "../assets/covers/banner-member.jpg";
import bannerCanvas from "../assets/covers/banner-canvas.jpg";
import bannerTutorial from "../assets/covers/banner-tutorial.jpg";
import bannerDiscover from "../assets/covers/banner-discover.jpg";
import bannerEvent from "../assets/covers/banner-event.jpg";

const PROJECT_COVERS = [coverImg1, coverImg2, coverImg3, coverImg4];

// 风格库回退项（API 不可用时对齐设计稿固定 10 项）
const FALLBACK_STYLES = [
  { code: "REALISTIC", name: "通用写实" },
  { code: "CINEMATIC", name: "电影感" },
  { code: "ANCIENT", name: "古风写实" },
  { code: "JAPANESE", name: "日系清新" },
  { code: "HOLLYWOOD", name: "欧美大片" },
  { code: "CYBERPUNK", name: "赛博朋克" },
  { code: "ANIME", name: "动漫风格" },
  { code: "WATERCOLOR", name: "水彩风格" },
  { code: "OIL", name: "油画风格" },
  { code: "BW_FILM", name: "黑白胶片" },
];

// 风格预览渐变类（home.css style-preview-*，按索引循环）
const STYLE_PREVIEW_CLASSES = [
  "style-preview-realistic", "style-preview-cinematic", "style-preview-ancient",
  "style-preview-japanese", "style-preview-hollywood", "style-preview-cyberpunk",
  "style-preview-anime", "style-preview-watercolor", "style-preview-oil", "style-preview-bw",
];

// 图像模型画廊（角色/场景/道具三分类，渐变编号对齐设计稿 thumb-grad-*）
const IMAGE_MODEL_ROWS: Record<"character" | "scene" | "prop", { label: string; prefix: string; grads: [number, number, number] }[]> = {
  character: [
    { label: "香蕉2", prefix: "banana2", grads: [1, 2, 3] },
    { label: "Seedream 5.0", prefix: "seedream50", grads: [4, 5, 6] },
    { label: "MJ V7", prefix: "mjv7", grads: [7, 8, 9] },
    { label: "Image2", prefix: "image2", grads: [10, 11, 12] },
  ],
  scene: [
    { label: "香蕉2场景", prefix: "banana2s", grads: [13, 14, 15] },
    { label: "Seedream 5.0", prefix: "seedream50s", grads: [16, 17, 18] },
    { label: "MJ V7", prefix: "mjv7s", grads: [19, 20, 21] },
    { label: "Image2", prefix: "image2s", grads: [22, 23, 24] },
  ],
  prop: [
    { label: "香蕉2道具", prefix: "banana2p", grads: [25, 26, 27] },
    { label: "Seedream 5.0", prefix: "seedream50p", grads: [28, 29, 30] },
    { label: "MJ V7", prefix: "mjv7p", grads: [31, 32, 33] },
    { label: "Image2", prefix: "image2p", grads: [31, 32, 33] },
  ],
};

// ===== 模型配置对接（GET /models 渲染 + GET/PUT /me/preferences 持久化） =====
interface ModelOption {
  model_key: string;
  name: string;
  provider: string;
  cost_points: number;
  enabled: boolean;
}

// 前端显示值 → 后端枚举（user_preferences 列）
const RES_TO_ENUM: Record<string, string> = { "480P": "P480", "720P": "P720", "1080P": "P1080" };
const ENUM_TO_RES: Record<string, string> = { P480: "480P", P720: "720P", P1080: "1080P", P2K_2560x1440: "2K", P4K_UHD: "4K" };
const RATIO_TO_ENUM: Record<string, string> = { "16:9": "R16_9", "1:1": "R1_1", "9:16": "R9_16" };
const ENUM_TO_RATIO: Record<string, string> = { R16_9: "16:9", R1_1: "1:1", R9_16: "9:16", R4_3: "4:3", R3_4: "3:4" };

// 视频模型硬编码回退（GET /models 不可用时）
const FALLBACK_VIDEO_MODELS: ModelOption[] = [
  { model_key: "seedance25", name: "Seedance 2.5", provider: "", cost_points: 0, enabled: true },
  { model_key: "seedance20", name: "Seedance 2.0", provider: "", cost_points: 0, enabled: true },
  { model_key: "seedance20fast", name: "Seedance 2.0 Fast", provider: "", cost_points: 0, enabled: true },
  { model_key: "klingo3", name: "KlingO3", provider: "", cost_points: 0, enabled: true },
];

// 图片模型硬编码回退（展开 IMAGE_MODEL_ROWS）
const FALLBACK_IMAGE_MODELS: ModelOption[] = Object.values(IMAGE_MODEL_ROWS)
  .flatMap((rows) => rows)
  .map((row, i) => ({ model_key: `${row.prefix}-${i + 1}`, name: `${row.label} ${i + 1}`, provider: "", cost_points: 0, enabled: true }));

interface ProjectItem {
  project_id: string;
  title: string;
  status: string;
  current_step: string;
  cover_image_url: string | null;
  created_at: string;
}

export default function HomePage() {
  const [tab, setTab] = useState<"agent" | "canvas">("agent");
  const [script, setScript] = useState("");
  const [modelPanelOpen, setModelPanelOpen] = useState(false);
  const [modelTab, setModelTab] = useState<"video" | "character" | "scene" | "prop">("video");
  const [videoModel, setVideoModel] = useState("seedance25");
  const [resolution, setResolution] = useState("480P");
  const [ratio, setRatio] = useState("9:16");
  const [videoDuration, setVideoDuration] = useState("auto");
  const [imageModel, setImageModel] = useState<Record<string, string>>({ character: "luxbanana2", scene: "luxbanana2", prop: "luxbanana2" });
  // 模型选项（GET /models 真源；失败回退硬编码）
  const [t2vModels, setT2vModels] = useState<ModelOption[]>(FALLBACK_VIDEO_MODELS);
  const [t2iModels, setT2iModels] = useState<ModelOption[]>(FALLBACK_IMAGE_MODELS);
  const [profileOpen, setProfileOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [projects, setProjects] = useState<ProjectItem[]>([]);
  const [points, setPoints] = useState<number | null>(null);
  // 风格选择弹层（GET /style-library 真数据；失败回退设计稿固定 10 项）
  const [styleOpen, setStyleOpen] = useState(false);
  const [stylePos, setStylePos] = useState<{ top: number; left: number } | null>(null);
  const [styleName, setStyleName] = useState("通用写实");
  const [styleCode, setStyleCode] = useState<string>("");
  const [styleLib, setStyleLib] = useState<{ code: string; name: string; thumb?: string }[]>([]);
  // 项目卡 … 下拉菜单（打开/重命名/复制/导出/删除）
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [menuPos, setMenuPos] = useState<{ top: number; left: number } | null>(null);
  const [renameTarget, setRenameTarget] = useState<ProjectItem | null>(null);
  const [renameVal, setRenameVal] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ProjectItem | null>(null);
  // Agent 技能配置弹窗（对齐设计稿 modal-agent-config：左导航 + 技能列表/详情双视图）
  const [agentOpen, setAgentOpen] = useState(false);
  const navigate = useNavigate();
  const { user } = useAuthStore();
  const avatarUrl = useAvatarUrl(user?.avatar_url);

  // 页面标题
  useEffect(() => {
    document.title = "AI创作 - EmptyBroken";
  }, []);

  // 模型配置：GET /models 加载可用模型（T2V 视频 / T2I 图片），GET /me/preferences 恢复已保存选择
  useEffect(() => {
    (async () => {
      try {
        const [v, i, prefs] = await Promise.all([
          cachedGet<{ items: ModelOption[] }>("/models?capability=T2V", 60000),
          cachedGet<{ items: ModelOption[] }>("/models?capability=T2I", 60000),
          cachedGet<Record<string, unknown>>("/me/preferences", 30000),
        ]);
        if (v.items?.length) setT2vModels(v.items);
        if (i.items?.length) setT2iModels(i.items);
        const mc = (prefs.model_config ?? {}) as Record<string, unknown>;
        if (mc.video_model) setVideoModel(String(mc.video_model));
        if (mc.image_model) {
          const im = String(mc.image_model);
          setImageModel({ character: im, scene: im, prop: im });
        }
        if (mc.resolution && ENUM_TO_RES[String(mc.resolution)]) setResolution(ENUM_TO_RES[String(mc.resolution)]);
        if (mc.aspect_ratio && ENUM_TO_RATIO[String(mc.aspect_ratio)]) setRatio(ENUM_TO_RATIO[String(mc.aspect_ratio)]);
        if (mc.video_duration) setVideoDuration(String(mc.video_duration));
      } catch {
        /* 模型/偏好加载失败：使用默认值与硬编码回退 */
      }
    })();
  }, []);

  // 保存模型配置 → PUT /me/preferences（持久化，下次打开自动恢复）
  const saveModelConfig = async () => {
    try {
      await api.put("/me/preferences", {
        model_config: {
          video_model: videoModel,
          image_model: imageModel.character,
          resolution: RES_TO_ENUM[resolution] ?? resolution,
          aspect_ratio: RATIO_TO_ENUM[ratio] ?? ratio,
          video_duration: videoDuration,
        },
      });
      toast.success("模型配置已保存");
      invalidateCachedGet("/me/preferences"); // 写后失效，其他页面重进时读到新值
      setModelPanelOpen(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "保存失败");
    }
  };

  // 舞台灯光：左上/右上双光源，20s 循环扫描
  // 阶段：品牌名逐字扫描 → 品牌名飞到输入框 → 飞到下方游走 → 飞回原位
  // 自然光质感：暖白调、微抖动、每侧三束光、长度随距离丝滑变化
  useEffect(() => {
    const stage = document.querySelector<HTMLElement>(".home-stage");
    const beamLeft = document.querySelector<HTMLElement>(".flashlight.is-left");
    const beamRight = document.querySelector<HTMLElement>(".flashlight.is-right");
    const brandTitle = document.querySelector<HTMLElement>(".brand-title");
    const spot = document.querySelector<HTMLElement>(".flashlight-spot");
    if (!stage || !beamLeft || !beamRight) return;

    let vw = window.innerWidth;
    let letters: { el: HTMLElement; cx: number }[] = [];
    let words: HTMLElement[] = [];
    let brandX = 0, brandY = 100, brandW = 300;
    let inputX = 0, inputY = 200;
    let projX = 0, projY = 400, projW = 600, projH = 200;

    const sync = () => {
      const sr = stage.getBoundingClientRect();
      vw = window.innerWidth;

      const brandEl = document.getElementById("brand-header");
      if (brandEl) {
        const r = brandEl.getBoundingClientRect();
        brandX = r.left + r.width / 2 - sr.left;
        brandY = r.top + r.height / 2 - sr.top;
        brandW = r.width;
      }
      const inputEl = document.getElementById("script-upload");
      if (inputEl) {
        const r = inputEl.getBoundingClientRect();
        inputX = r.left + r.width / 2 - sr.left;
        inputY = r.top + r.height / 2 - sr.top;
      }
      const projEl = document.getElementById("example-projects");
      if (projEl) {
        const r = projEl.getBoundingClientRect();
        projX = r.left + r.width / 2 - sr.left;
        projY = r.top + r.height / 2 - sr.top;
        projW = r.width;
        projH = r.height;
      }

      if (brandTitle) {
        brandTitle.style.setProperty("--title-w", `${brandTitle.offsetWidth}px`);
        const tr = brandTitle.getBoundingClientRect();
        letters = Array.from(brandTitle.querySelectorAll<HTMLElement>(".brand-letter")).map((l) => {
          const lr = l.getBoundingClientRect();
          l.style.backgroundPosition = `${-(lr.left - tr.left)}px 0`;
          return { el: l, cx: lr.left + lr.width / 2 - sr.left };
        });
      }
      words = Array.from(document.querySelectorAll<HTMLElement>(".brand-word"));
    };
    sync();
    const raf1 = requestAnimationFrame(sync);
    let alive = true;
    const resync = () => { if (alive) sync(); };
    document.fonts?.ready.then(resync).catch(() => {});
    const ro = brandTitle && typeof ResizeObserver !== "undefined" ? new ResizeObserver(resync) : null;
    if (brandTitle) ro?.observe(brandTitle);
    window.addEventListener("resize", sync);
    window.addEventListener("scroll", sync, true);

    // —— 尘埃粒子：光束中漂浮的微尘，增强真实感 ——
    // 每侧生成一批粒子，沿光束方向缓慢飘动，大小/速度/亮度随机
    const DUST_COUNT = 18; // 每侧粒子数
    const dustLayers = document.querySelector<HTMLElement>(".flashlight-layer");
    interface DustParticle {
      el: HTMLElement;
      side: "left" | "right";
      progress: number; // 0~1，沿光束方向的进度
      speed: number; // 飘动速度
      size: number; // 粒子大小
      offset: number; // 横向偏移（-1~1，相对于光束宽度）
      brightness: number; // 亮度系数
      drift: number; // 横向漂移速度
      phase: number; // 闪烁相位
    }
    const dustParticles: DustParticle[] = [];

    if (dustLayers) {
      for (let i = 0; i < DUST_COUNT; i++) {
        for (const side of ["left", "right"] as const) {
          const el = document.createElement("div");
          el.className = "dust-particle";
          dustLayers.appendChild(el);
          dustParticles.push({
            el,
            side,
            progress: Math.random(), // 随机初始位置
            speed: 0.015 + Math.random() * 0.025, // 随机速度
            size: 1.5 + Math.random() * 3, // 1.5~4.5px
            offset: (Math.random() - 0.5) * 1.2, // 横向偏移
            brightness: 0.3 + Math.random() * 0.7,
            drift: (Math.random() - 0.5) * 0.003,
            phase: Math.random() * Math.PI * 2,
          });
        }
      }
    }

    // —— 20s 扫描时间线（节奏更舒缓，品牌名会飞行） ——
    const DURATION = 20000;
    const clamp01 = (v: number) => (v < 0 ? 0 : v > 1 ? 1 : v);
    const easeInOut = (t: number) => t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
    const ramp = (t: number, a: number, b: number) => clamp01((t - a) / (b - a));
    const start = performance.now();
    let raf2 = 0;
    const tick = (now: number) => {
      if (!alive) return;
      const t = ((now - start) % DURATION) / 1000;
      const timeSec = now / 1000; // 用于微抖动的连续时间

      // —— 光束焦点位置（弧线过渡，不生硬） ——
      let fx = brandX, fy = brandY;

      if (t < 5.5) {
        // 阶段1：品牌名从左到右缓慢扫描
        const p = easeInOut(clamp01(t / 5.5));
        fx = brandX - brandW / 2 + p * brandW;
        fy = brandY;
      } else if (t < 7.2) {
        // 过渡：品牌名 → 输入框（弧线扫过去）
        const k = easeInOut(ramp(t, 5.5, 7.2));
        const sx = brandX + brandW / 2;
        const sy = brandY;
        const arcY = Math.sin(k * Math.PI) * 50;
        fx = sx + (inputX - sx) * k;
        fy = sy + (inputY - sy) * k - arcY;
      } else if (t < 10.5) {
        // 阶段2：输入框，轻微呼吸
        const breatheX = Math.sin((t - 7.2) * 1.3) * 8;
        const breatheY = Math.sin((t - 7.2) * 1.1 + 0.5) * 5;
        fx = inputX + breatheX;
        fy = inputY + breatheY;
      } else if (t < 11.5) {
        // 过渡：输入框 → 下方
        const k = easeInOut(ramp(t, 10.5, 11.5));
        fx = inputX + (projX - inputX) * k;
        fy = inputY + (projY - inputY) * k;
      } else if (t < 16.0) {
        // 阶段3：下方区域缓慢游走（多频正弦叠加，自然随机感）
        // 焦点往下偏移 projH*0.15，更多照在卡片图片上
        const phase = (t - 11.5) / 4.5;
        fx = projX
          + Math.sin(phase * Math.PI * 1.0) * projW * 0.3
          + Math.sin(phase * Math.PI * 2.2 + 1.2) * projW * 0.07;
        fy = projY + projH * 0.45
          + Math.sin(phase * Math.PI * 0.75 + 0.6) * projH * 0.3
          + Math.sin(phase * Math.PI * 1.8 + 1.8) * projH * 0.08;
      } else if (t < 18.0) {
        // 收回：弧线回到品牌名起点
        const k = easeInOut(ramp(t, 16.0, 18.0));
        const arcY = Math.sin(k * Math.PI) * 60;
        fx = projX + (brandX - projX) * k;
        fy = projY + (brandY - projY) * k - arcY;
      } else {
        fx = brandX;
        fy = brandY;
      }

      // —— 光束微抖动：模拟真实光线的不稳定（非常细微，不晃眼） ——
      const flickerA = Math.sin(timeSec * 7.3) * 0.15 + Math.sin(timeSec * 3.1 + 1) * 0.1;
      const flickerL = 1 + Math.sin(timeSec * 5.7 + 0.5) * 0.015 + Math.sin(timeSec * 11.3) * 0.008;
      const flickerR = 1 + Math.sin(timeSec * 6.2 + 1.2) * 0.015 + Math.sin(timeSec * 9.8 + 0.3) * 0.008;
      const flickerOp = 1 + Math.sin(timeSec * 4.5 + 0.8) * 0.03 + Math.sin(timeSec * 8.1) * 0.02;

      // —— 辅助光束独立微运动：模拟多镜片散射的有机感 ——
      // 内侧辅助光：角度轻微独立摆动 + 长度独立呼吸
      const subInnerA_L = Math.sin(timeSec * 2.8 + 0.3) * 0.7 + Math.sin(timeSec * 5.2 + 1.1) * 0.35;
      const subInnerA_R = Math.sin(timeSec * 2.6 + 0.9) * 0.7 + Math.sin(timeSec * 4.8 + 0.2) * 0.35;
      const subInnerL_L = 1 + Math.sin(timeSec * 1.9 + 0.5) * 0.04 + Math.sin(timeSec * 4.1 + 0.8) * 0.02;
      const subInnerL_R = 1 + Math.sin(timeSec * 2.1 + 1.2) * 0.04 + Math.sin(timeSec * 3.9 + 0.3) * 0.02;
      // 外侧辅助光：摆动幅度稍大，频率稍慢，与内侧不同步
      const subOuterA_L = Math.sin(timeSec * 1.6 + 0.7) * 1.0 + Math.sin(timeSec * 3.7 + 1.5) * 0.4;
      const subOuterA_R = Math.sin(timeSec * 1.8 + 0.2) * 1.0 + Math.sin(timeSec * 3.5 + 0.9) * 0.4;
      const subOuterL_L = 1 + Math.sin(timeSec * 1.2 + 0.8) * 0.055 + Math.sin(timeSec * 2.8 + 0.2) * 0.025;
      const subOuterL_R = 1 + Math.sin(timeSec * 1.4 + 0.3) * 0.055 + Math.sin(timeSec * 2.6 + 0.7) * 0.025;

      // —— 光束角度实时追光 + 微抖动 ——
      const angleL = (Math.atan2(fy - 40, fx - 72) * 180) / Math.PI - 90 + flickerA;
      const angleR = (Math.atan2(fy - 40, fx - vw) * 180) / Math.PI - 90 - flickerA;
      beamLeft.style.setProperty("--beam-a-l", angleL.toFixed(2) + "deg");
      beamRight.style.setProperty("--beam-a-r", angleR.toFixed(2) + "deg");
      // 辅助光束独立角度偏移
      beamLeft.style.setProperty("--sub-inner-a-l", subInnerA_L.toFixed(2) + "deg");
      beamLeft.style.setProperty("--sub-outer-a-l", subOuterA_L.toFixed(2) + "deg");
      beamRight.style.setProperty("--sub-inner-a-r", subInnerA_R.toFixed(2) + "deg");
      beamRight.style.setProperty("--sub-outer-a-r", subOuterA_R.toFixed(2) + "deg");

      // —— 光束长度：根据距离动态计算 + 微抖动 ——
      const distL = Math.sqrt((fx - 72) ** 2 + (fy - 40) ** 2);
      const distR = Math.sqrt((fx - vw) ** 2 + (fy - 40) ** 2);
      const vh = window.innerHeight;
      let lenL = (distL / vh) * 0.9 * flickerL;
      let lenR = (distR / vh) * 0.9 * flickerR;

      // 品牌名扫描阶段：左光束明显变长，右光束明显变短
      // 模拟手电筒照近物的效果：靠近光源的光束更长更亮
      if (t < 5.5) {
        const p = easeInOut(clamp01(t / 5.5));
        // 左光束：从 1.0 增长到 1.35（明显变长）
        lenL *= 1 + 0.35 * p;
        // 右光束：从 1.0 缩短到 0.55（明显变短）
        lenR *= Math.max(0.55, 1 - 0.45 * p);
      } else if (t < 7.2) {
        // 飞行1阶段：光束长度逐渐过渡到等长
        const k = easeInOut(ramp(t, 5.5, 7.2));
        lenL *= 1.35 - 0.35 * k;
        lenR *= 0.55 + 0.45 * k;
      }

      // 收尾阶段缩短
      if (t >= 16.0) {
        const k = easeInOut(ramp(t, 16.0, 18.0));
        lenL *= 1 - 0.85 * k;
        lenR *= 1 - 0.85 * k;
      }
      const finalLenL = Math.max(0.12, lenL);
      const finalLenR = Math.max(0.1, lenR);
      beamLeft.style.setProperty("--beam-l", finalLenL.toFixed(3));
      beamRight.style.setProperty("--beam-r", finalLenR.toFixed(3));
      // 辅助光束独立长度系数
      beamLeft.style.setProperty("--sub-inner-l-l", (finalLenL * subInnerL_L).toFixed(3));
      beamLeft.style.setProperty("--sub-outer-l-l", (finalLenL * subOuterL_L).toFixed(3));
      beamRight.style.setProperty("--sub-inner-l-r", (finalLenR * subInnerL_R).toFixed(3));
      beamRight.style.setProperty("--sub-outer-l-r", (finalLenR * subOuterL_R).toFixed(3));

      // —— 光束整体亮度 + 微抖动 ——
      let baseOp = 0.38;
      if (t < 0.5) baseOp = 0.1 + 0.28 * easeInOut(ramp(t, 0, 0.5));
      else if (t >= 16.0) baseOp = 0.38 - 0.32 * easeInOut(ramp(t, 16.0, 17.8));
      const beamOp = Math.max(0.05, baseOp * flickerOp);
      beamLeft.style.opacity = beamOp.toFixed(3);
      beamRight.style.opacity = beamOp.toFixed(3);

      // —— 品牌名逐字母点亮（跟随扫描位置） ——
      if (t < 5.5) {
        const x = brandX - brandW / 2 + easeInOut(clamp01(t / 5.5)) * brandW;
        for (const l of letters) if (!l.el.classList.contains("lit") && x >= l.cx) l.el.classList.add("lit");
      }
      // 副标题逐词点亮
      if (t >= 4.5 && t < 17.8) words.forEach((w, i) => { if (t >= 4.5 + i * 0.3) w.classList.add("lit"); });

      // —— 收尾复位 ——
      if (t >= 18.0 && t < 18.3) {
        letters.forEach((l) => l.el.classList.remove("lit"));
        words.forEach((w) => w.classList.remove("lit"));
      }

      // —— 聚焦光斑：跟随光束焦点，模拟光线打在物体表面 ——
      if (spot) {
        // 光斑大小随距离变化：越远越大（越散）
        const distAvg = (distL + distR) / 2;
        const spotSize = 180 + (distAvg / vh) * 80;
        spot.style.left = fx.toFixed(1) + "px";
        spot.style.top = fy.toFixed(1) + "px";
        spot.style.width = spotSize.toFixed(0) + "px";
        spot.style.height = spotSize.toFixed(0) + "px";
        // 光斑微抖动
        const spotFlicker = 1 + Math.sin(timeSec * 5.5) * 0.05 + Math.sin(timeSec * 9.2) * 0.03;
        spot.style.opacity = (0.65 * spotFlicker * (t >= 16.0 ? 1 - easeInOut(ramp(t, 16.0, 17.8)) : 1)).toFixed(3);
      }

      // —— 卡片照亮：光束照到精选推荐卡片时，卡片发光 ——
      const cards = document.querySelectorAll<HTMLElement>(".carousel-card");
      if (cards.length > 0) {
        // 找到离焦点最近的卡片
        let nearestIdx = -1;
        let nearestDist = Infinity;
        cards.forEach((card, i) => {
          const r = card.getBoundingClientRect();
          const sr = stage.getBoundingClientRect();
          const cx = r.left + r.width / 2 - sr.left;
          const cy = r.top + r.height / 2 - sr.top;
          const d = Math.sqrt((fx - cx) ** 2 + (fy - cy) ** 2);
          if (d < nearestDist) { nearestDist = d; nearestIdx = i; }
        });
        // 只有在下方区域阶段 + 距离足够近才点亮
        const inProjPhase = t >= 10.5 && t < 16.5;
        const litRadius = 160;
        cards.forEach((card, i) => {
          if (inProjPhase && i === nearestIdx && nearestDist < litRadius) {
            card.classList.add("is-lit");
          } else {
            card.classList.remove("is-lit");
          }
        });
      }

      // —— 尘埃粒子更新：沿光束方向飘动 ——
      if (dustParticles.length > 0) {
        const beamLenL = parseFloat(beamLeft.style.getPropertyValue("--beam-l") || "1");
        const beamLenR = parseFloat(beamRight.style.getPropertyValue("--beam-r") || "1");
        const angleL = parseFloat(beamLeft.style.getPropertyValue("--beam-a-l") || "7");
        const angleR = parseFloat(beamRight.style.getPropertyValue("--beam-a-r") || "-7");
        const beamLength = vh * 0.9; // 基础光束长度（与 CSS 一致）
        const beamWidth = beamLength * 0.12; // 光束宽度的一半（用于横向偏移）

        for (const p of dustParticles) {
          // 更新进度：沿光束向前飘动
          p.progress += p.speed * 0.016 * 60; // 基于 60fps 归一化
          p.offset += p.drift;
          // 越界后重置到起点
          if (p.progress > 1) {
            p.progress = 0;
            p.offset = (Math.random() - 0.5) * 1.2;
          }
          // 横向漂移限制
          if (p.offset > 0.9) p.offset = 0.9;
          if (p.offset < -0.9) p.offset = -0.9;

          const isLeft = p.side === "left";
          const angle = isLeft ? angleL : angleR;
          const beamLen = isLeft ? beamLenL : beamLenR;
          const srcX = isLeft ? 72 : vw;
          const srcY = 40;

          // 粒子在光束中的位置：沿光束方向 * 进度
          const distAlong = beamLength * beamLen * p.progress;
          const distSide = beamWidth * p.offset * (1 - p.progress * 0.3); // 越远越宽
          const rad = (angle + 90) * Math.PI / 180; // 转成标准弧度

          const px = srcX + Math.cos(rad) * distAlong + Math.cos(rad + Math.PI / 2) * distSide;
          const py = srcY + Math.sin(rad) * distAlong + Math.sin(rad + Math.PI / 2) * distSide;

          // 透明度：两端淡，中间亮；加上闪烁
          const fadeIn = Math.min(1, p.progress * 4);
          const fadeOut = Math.min(1, (1 - p.progress) * 2.5);
          const twinkle = 0.6 + 0.4 * Math.sin(timeSec * 3 + p.phase);
          const opacity = fadeIn * fadeOut * p.brightness * twinkle * 0.7;

          p.el.style.transform = `translate(${px.toFixed(1)}px, ${py.toFixed(1)}px)`;
          p.el.style.width = p.size.toFixed(1) + "px";
          p.el.style.height = p.size.toFixed(1) + "px";
          p.el.style.opacity = opacity.toFixed(3);
        }
      }

      raf2 = requestAnimationFrame(tick);
    };
    raf2 = requestAnimationFrame(tick);

    return () => {
      alive = false;
      cancelAnimationFrame(raf1);
      cancelAnimationFrame(raf2);
      // 清理尘埃粒子 DOM
      for (const p of dustParticles) p.el.remove();
      dustParticles.length = 0;
      ro?.disconnect();
      window.removeEventListener("resize", sync);
      window.removeEventListener("scroll", sync, true);
    };
  }, []);

  // 项目列表 + 积分余额（GET /points 真实数据，替代设计稿硬编码）
  const loadProjects = async () => {
    try {
      const data = await api.get<{ items: ProjectItem[] }>("/projects", { params: { page_size: 8 } });
      setProjects(data.items ?? []);
    } catch {
      /* 列表失败不阻塞页面 */
    }
  };
  useEffect(() => {
    void loadProjects();
    (async () => {
      try {
        const acc = await api.get<{ current_balance: number }>("/points");
        setPoints(acc.current_balance);
      } catch {
        /* 积分失败不阻塞页面 */
      }
    })();
  }, []);

  // 精选推荐轮播（GET /banners 真数据：启用中且按 sort_order；失败/空回退设计稿静态横幅）
  useEffect(() => {
    (async () => {
      try {
        const data = await api.get<{ items: { title: string; image_url: string; jump_url: string }[] }>("/banners");
        const items = (data.items ?? [])
          .filter((b) => b.image_url)
          .map((b) => ({
            img: b.image_url,
            title: b.title,
            subtitle: "",
            action: "立即查看",
            to: b.jump_url || undefined,
          }));
        if (items.length > 0) setCarouselSlides(items);
      } catch {
        /* 回退设计稿静态横幅 */
      }
    })();
  }, []);

  // 顶部公告条（GET /announcements 首条；无生效公告不展示）
  useEffect(() => {
    (async () => {
      try {
        const data = await api.get<{ items: { title: string; content: string }[] }>("/announcements");
        const first = (data.items ?? [])[0];
        if (first) setNotice(first);
      } catch {
        /* 无公告不展示 */
      }
    })();
  }, []);

  // 风格库（GET /style-library）：前 5 项归「风格库」，其余归「公共库」（设计稿分组）
  // thumbnail_url 为火山生成的真实风格预览图（无图回退 CSS 渐变占位）
  useEffect(() => {
    (async () => {
      try {
        const data = await api.get<{ items: { style_code: string; name: string; thumbnail_url?: string }[] }>("/style-library");
        const items = (data.items ?? []).map((s) => ({ code: s.style_code, name: s.name, thumb: s.thumbnail_url || "" }));
        setStyleLib(items.length > 0 ? items : FALLBACK_STYLES);
      } catch {
        setStyleLib(FALLBACK_STYLES);
      }
    })();
  }, []);

  // 风格弹层/卡片菜单：点击外部关闭
  useEffect(() => {
    if (!styleOpen && !menuFor) return;
    const onDocDown = (e: MouseEvent) => {
      const el = e.target as HTMLElement;
      if (styleOpen && !el.closest("#popover-style-selector") && !el.closest("#btn-style-selector")) setStyleOpen(false);
      if (menuFor && !el.closest("#card-dropdown-menu") && !el.closest(".card-menu")) {
        setMenuFor(null);
        setMenuPos(null);
      }
    };
    document.addEventListener("mousedown", onDocDown);
    return () => document.removeEventListener("mousedown", onDocDown);
  }, [styleOpen, menuFor]);

  // 打开风格弹层：定位在 pill 正下方
  const openStylePopover = (e: React.MouseEvent<HTMLButtonElement>) => {
    if (styleOpen) {
      setStyleOpen(false);
      return;
    }
    const rect = e.currentTarget.getBoundingClientRect();
    setStylePos({ top: rect.bottom + 10, left: Math.max(16, rect.left - 24) });
    setStyleOpen(true);
  };

  // 卡片 … 菜单：仅真实项目可操作（示例卡不展示菜单项）
  const openCardMenu = (e: React.MouseEvent<HTMLButtonElement>, projectId?: string) => {
    e.stopPropagation();
    if (!projectId) {
      toast.info("示例剧本无项目操作，点击卡片可填入剧本");
      return;
    }
    const rect = e.currentTarget.getBoundingClientRect();
    setMenuFor(projectId);
    setMenuPos({ top: rect.bottom + 6, left: Math.max(16, rect.right - 140) });
  };

  // 重命名：PUT /projects/:id
  const submitRename = async () => {
    if (!renameTarget || !renameVal.trim()) return;
    try {
      await api.put(`/projects/${renameTarget.project_id}`, { title: renameVal.trim() });
      toast.success("已重命名");
      setRenameTarget(null);
      void loadProjects();
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "重命名失败");
    }
  };

  // 删除：DELETE /projects/:id（软删）
  const submitDelete = async () => {
    if (!deleteTarget) return;
    try {
      await api.delete(`/projects/${deleteTarget.project_id}`);
      toast.success("项目已删除");
      setDeleteTarget(null);
      void loadProjects();
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "删除失败");
    }
  };

  // 进入工作台：创建项目（标题取剧本前 20 字，无剧本用默认名）
  const createProject = async (e: FormEvent) => {
    e.preventDefault();
    if (!script.trim()) {
      toast.error("请先上传或粘贴剧本内容");
      return;
    }
    setCreating(true);
    try {
      const title = script.trim().replace(/\s+/g, " ").slice(0, 20) || "未命名项目";
      const data = await api.post<{ project_id: string }>("/projects", {
        title,
        source_script_text: script.trim(),
      });
      // 选中非默认风格时同步应用到项目（POST /projects/:id/apply-style；失败不阻断进入）
      if (styleCode) {
        try {
          await api.post(`/projects/${data.project_id}/apply-style`, { style_code: styleCode });
        } catch {
          /* 风格应用失败不阻断工作台进入 */
        }
      }
      toast.success("项目创建成功，正在进入工作台");
      navigate(`/projects/${data.project_id}/step/1`);
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "创建失败，请稍后重试");
    } finally {
      setCreating(false);
    }
  };

  const covers = PROJECT_COVERS;

  // 精选推荐轮播默认数据（设计稿静态横幅；GET /banners 有数据时替换）
  const DEFAULT_SLIDES: {
    img: string;
    title: string;
    subtitle: string;
    action: string;
    to?: string;
    toastMsg?: string;
  }[] = [
    { img: bannerMember, title: "会员月卡，首购5折！", subtitle: "Seedance2.0 低至 0.49元/秒", action: "立即开通", to: "/points" },
    { img: bannerCanvas, title: "自由画布 Beta 上线", subtitle: "流水线一键转画布，关键片段自由精修", action: "立即查看", to: "/canvas" },
    { img: bannerTutorial, title: "新手教程，快速创作！", subtitle: "跟随引导，轻松掌握创作流程", action: "立即查看", toastMsg: "新手教程即将上线" },
    { img: bannerDiscover, title: "发现好剧，灵感不停", subtitle: "浏览社区精选作品，找到你的下一个爆款", action: "去看看", to: "/feed" },
    { img: bannerEvent, title: "限时创作挑战赛", subtitle: "投稿 AI 短剧作品，赢取万元创作基金", action: "立即报名", toastMsg: "创作挑战赛即将开启" },
  ];

  // 示例剧本（API 无数据时展示,点击填入示例剧本)——设计稿 cover-1~4 的四个示例
  const sampleProjects = [
    { title: "危险关系法则", time: "2026-08-07 20:38:54", script: "深夜，林晚回到公寓，发现门锁被人撬开。屋内一片狼藉，而电视屏幕上，正播放着她自己睡觉的画面……" },
    { title: "一语成谶：市井说书人的谎言局", time: "2026-08-07 20:33:52", script: "城西老槐树下，说书人苏无恙一柄折扇讲尽天下奇谈。凡他开口的命数，三日内必应验。直到那个雨夜，一个戴着兜帽的人递上一锭金子，求他说一个『死局』……" },
    { title: "合租上司是心动对象", time: "2026-08-07 20:33:52", script: "陆眠为了省钱与人合租，搬进去才发现，合租室友竟是白天冷面毒舌、专克她的顶头上司顾北辰。白天被骂，晚上抬头不见低头见，这场合租生活，彻底失控……" },
    { title: "疼痛银行", time: "2026-08-07 20:33:52", script: "城市角落有一家只做夜晚生意的『疼痛银行』，人类可以用痛苦换钱，也可以存下快乐。柜台后的女孩简央，无意中发现自己的记忆被频繁取走，而每一次取走，都牵出一桩悬案……" },
  ];

// 剧本文件上传约束（方案 07-01 D.5）
const SCRIPT_FILE_MAX_SIZE = 20 * 1024 * 1024; // 单文件 ≤ 20MB
const SCRIPT_FILE_EXTS = ["txt", "md", "doc", "docx", "pdf"] as const;
const MAX_SCRIPT_CHARS = 50000;

type ScriptUploadState = "EMPTY" | "UPLOADING" | "PARSING" | "FILE_READY" | "PARSE_FAILED";

// docx 解析（mammoth 动态导入，仅上传 docx 时加载，避免主包膨胀）
async function parseDocxToText(arrayBuffer: ArrayBuffer): Promise<string> {
  const mammoth = (await import("mammoth")).default;
  const result = await mammoth.extractRawText({ arrayBuffer });
  return result.value.trim();
}

// pdf 解析（pdfjs-dist 动态导入；worker 走 Vite ?url 资源，按需加载）
async function parsePdfToText(arrayBuffer: ArrayBuffer): Promise<string> {
  const pdfjs = await import("pdfjs-dist");
  pdfjs.GlobalWorkerOptions.workerSrc = (await import("pdfjs-dist/build/pdf.worker.min.mjs?url")).default;
  const task = pdfjs.getDocument({ data: new Uint8Array(arrayBuffer) });
  const doc = await task.promise;
  try {
    let text = "";
    for (let i = 1; i <= doc.numPages; i++) {
      const page = await doc.getPage(i);
      const content = await page.getTextContent();
      text += content.items.map((it) => ("str" in it ? it.str : "")).join(" ") + "\n";
    }
    return text.trim();
  } finally {
    await task.destroy();
  }
}

  const carouselRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [carouselSlides, setCarouselSlides] = useState(DEFAULT_SLIDES);
  const [carouselIndex, setCarouselIndex] = useState(0);
  const [carouselPaused, setCarouselPaused] = useState(false);
  // 剧本文件上传状态机（方案 07-01 D.2：EMPTY→UPLOADING→PARSING→FILE_READY / PARSE_FAILED）
  const [uploadState, setUploadState] = useState<ScriptUploadState>("EMPTY");
  const [uploadedFile, setUploadedFile] = useState<{ name: string; size: number } | null>(null);
  const [parseError, setParseError] = useState("");
  // 顶部公告条（GET /announcements 首条，生效时间窗内；无数据不展示）
  const [notice, setNotice] = useState<{ title: string; content: string } | null>(null);

  const SLIDE_COUNT = carouselSlides.length;

  // 我的项目固定 4 卡：真实项目（最新）优先，不足用示例剧本补齐（用户要求固定四张）
  const displayProjects = useMemo(() => {
    const real = projects.slice(0, 4).map((p, i) => ({
      key: p.project_id,
      title: p.title,
      time: p.created_at?.slice(0, 19).replace("T", " ") ?? "—",
      cover: p.cover_image_url || covers[i % 4],
      badge: "我的项目" as const,
      projectId: p.project_id as string | undefined,
      script: undefined as string | undefined,
    }));
    const fill = sampleProjects.slice(0, Math.max(0, 4 - real.length)).map((s, i) => ({
      key: `sample-${s.title}`,
      title: s.title,
      time: s.time,
      cover: covers[(real.length + i) % 4],
      badge: "示例剧本" as const,
      projectId: undefined as string | undefined,
      script: s.script as string | undefined,
    }));
    return [...real, ...fill];
  }, [projects, covers, sampleProjects]);

  // 自动轮播：每 4s 切换激活卡片（一屏展示 4 张，共 5 张横向滚动轮播，仿 luxreal banner）；
  // 鼠标悬停暂停；状态更新与副作用分离，避免 StrictMode 双调用
  useEffect(() => {
    if (carouselPaused) return;
    const timer = setInterval(() => {
      setCarouselIndex((i) => (i + 1) % SLIDE_COUNT);
    }, 4000);
    return () => clearInterval(timer);
  }, [carouselPaused]);

  // 激活卡切换时滚动到可见（一屏 4 张，第 5 张靠横向滚动露出；限幅到最大可滚距离）
  useEffect(() => {
    const track = carouselRef.current;
    const slide = track?.children[carouselIndex] as HTMLElement | undefined;
    if (!track || !slide) return;
    const maxScroll = track.scrollWidth - track.clientWidth;
    track.scrollTo({ left: Math.min(slide.offsetLeft, maxScroll), behavior: "smooth" });
  }, [carouselIndex]);

  // 头像面板：点击外部关闭逻辑已内置于 ProfilePanel 组件

  // 剧本文件上传（方案 07-01：txt/md 直读；docx/pdf 前端解析 + MinIO 入库；.doc 明确提示不支持）
  const handleFile = async (file: File) => {
    if (!file) return;
    const ext = (file.name.split(".").pop() || "").toLowerCase();
    if (!(SCRIPT_FILE_EXTS as readonly string[]).includes(ext)) {
      toast.error("仅支持 txt / md / doc / docx / pdf 格式");
      return;
    }
    if (file.size > SCRIPT_FILE_MAX_SIZE) {
      toast.error("文件过大，请压缩后上传（≤20MB）");
      return;
    }
    setUploadState("UPLOADING");
    setParseError("");
    try {
      // .doc 旧格式：M0 不解析，明确提示转化建议（方案 07-01 D.5）
      if (ext === "doc") {
        setUploadState("PARSE_FAILED");
        setParseError("旧版 .doc 暂不支持解析，请另存为 .docx 或使用 txt");
        toast.error("旧版 .doc 暂不支持解析，请另存为 .docx 或使用 txt");
        return;
      }
      // 1. doc/docx/pdf 走预签名上传入库（保留 MinIO 链路，M1 服务端解析复用；txt/md 直读无需入库）
      if (ext !== "txt" && ext !== "md") {
        const presign = await api.post<{ upload_url: string; upload_token: string; object_key: string }>("/uploads/presign", {
          biz_type: "script_file",
          file_name: file.name,
          content_type: file.type || "application/octet-stream",
          size: file.size,
        });
        const putResp = await fetch(presign.upload_url, { method: "PUT", body: file, headers: { "Content-Type": file.type || "application/octet-stream" } });
        if (!putResp.ok) throw new Error("直传失败 " + putResp.status);
        const etag = (putResp.headers.get("ETag") || "").replace(/"/g, "");
        await api.post("/uploads/complete", {
          upload_token: presign.upload_token,
          object_key: presign.object_key,
          etag: etag || "-",
          biz_ref_id: user?.default_space_id?.toString() || "0",
        });
      }
      // 2. 解析文档为纯文本（docx=mammoth / pdf=pdfjs-dist，动态导入按需加载）
      setUploadState("PARSING");
      let text: string;
      if (ext === "txt" || ext === "md") {
        text = await file.text();
      } else if (ext === "docx") {
        text = await parseDocxToText(await file.arrayBuffer());
      } else {
        text = await parsePdfToText(await file.arrayBuffer());
      }
      text = text.trim();
      if (!text) {
        setUploadState("PARSE_FAILED");
        setParseError(ext === "pdf" ? "扫描件暂不支持文字提取，请使用 txt/docx" : "文件内容为空，无法提取剧本文本");
        toast.error("未能从文件中提取文字，请改用 txt/docx 或直接粘贴");
        return;
      }
      if (text.length > MAX_SCRIPT_CHARS) {
        setUploadState("PARSE_FAILED");
        setParseError(`解析文本超过 ${MAX_SCRIPT_CHARS.toLocaleString()} 字上限，请拆分文件后重试`);
        toast.error("解析文本超过 5 万字上限");
        return;
      }
      // 3. 文件卡片态（格式互斥：文件存在期间禁止手动编辑）
      // U1：上传后输入框不再回填小说全文预览（script 仍持有文本供创建项目）
      setScript(text);
      setUploadedFile({ name: file.name, size: file.size });
      setUploadState("FILE_READY");
      toast.success(`剧本「${file.name}」已解析完成，可直接进入工作台`);
    } catch (err) {
      setUploadState("PARSE_FAILED");
      setParseError(err instanceof ApiError ? errorMessage(err.code, err.message) : "文档解析失败，请重试或改用粘贴文本");
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "文档解析失败，请重试或改用粘贴文本");
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  // 移除已上传文件：回到拖拽区，恢复手动编辑（解析文本保留在输入框）
  const removeUploadedFile = () => {
    setUploadedFile(null);
    setUploadState("EMPTY");
    setParseError("");
    if (fileInputRef.current) fileInputRef.current.value = "";
    toast.info("已移除已上传文件，可直接编辑下方剧本内容");
  };

  // 填入示例剧本（对齐官网「使用示例剧本」；与文件源互斥，先移除已上传文件）
  const fillSampleScript = (scriptText: string) => {
    if (uploadedFile) {
      setUploadedFile(null);
      setUploadState("EMPTY");
      setParseError("");
    }
    setScript(scriptText);
    toast.success("示例剧本已填入输入框，点击「进入工作台」开始创作");
  };
  const scrollToSlide = (i: number) => {
    setCarouselIndex(i);
  };

  // 风格弹层选中态：未选择时默认首项（对齐设计稿「通用写实」默认选中）
  const selectedStyleCode = styleCode || styleLib[0]?.code || "";
  const renderStyleCard = (s: { code: string; name: string; thumb?: string }, idx: number) => (
    <div
      key={s.code}
      className={`style-card ${s.code === selectedStyleCode ? "selected" : ""}`}
      onClick={() => {
        setStyleCode(s.code);
        setStyleName(s.name);
        setStyleOpen(false);
      }}
    >
      <div className={`style-card-preview ${STYLE_PREVIEW_CLASSES[idx % STYLE_PREVIEW_CLASSES.length]}`}>
        {s.thumb ? (
          <img src={s.thumb} alt={s.name} loading="lazy" className="style-card-thumb" />
        ) : null}
        <div className="style-card-check">
          <Check size={12} />
        </div>
      </div>
      <div className="style-card-name">{s.name}</div>
    </div>
  );

  // 卡片菜单动作后关闭
  const closeMenu = () => {
    setMenuFor(null);
    setMenuPos(null);
  };
  const menuProject = menuFor ? projects.find((p) => p.project_id === menuFor) ?? null : null;

  return (
    <div data-viewport-mode="app-shell" className="home-stage flex flex-col h-screen w-screen overflow-hidden">
      {/* 品牌区光晕（对齐 LuxReal 官网实测：884×884 静态径向蓝紫光晕，见 home.css） */}
      <div className="home-brand-glow">
        <div className="home-brand-glow-core" />
      </div>

      {/* 双手电筒舞台灯光：左上/右上光束缓慢对称扫向中央，循环照亮品牌名 ⇄ 输入卡（样式见 home.css）
          三层光束系统：外层辉光（可见光柱散射）+ 主光束 + 两侧辅助光，更有层次感 */}
      <div className="flashlight-layer" aria-hidden>
        <div className="flashlight is-left">
          <div className="flashlight-beam-ambient" />
          <div className="flashlight-beam-glow" />
          <div className="flashlight-beam-sub flashlight-beam-sub-outer" />
          <div className="flashlight-beam" />
          <div className="flashlight-beam-sub flashlight-beam-sub-inner" />
          <div className="flashlight-beam-core" />
          <div className="flashlight-src" />
        </div>
        <div className="flashlight is-right">
          <div className="flashlight-beam-ambient" />
          <div className="flashlight-beam-glow" />
          <div className="flashlight-beam-sub flashlight-beam-sub-outer" />
          <div className="flashlight-beam" />
          <div className="flashlight-beam-sub flashlight-beam-sub-inner" />
          <div className="flashlight-beam-core" />
          <div className="flashlight-src" />
        </div>
        {/* 聚焦光斑：光束末端打在物体上的亮光斑 */}
        <div className="flashlight-spot" />
      </div>

      {/* 主行:侧边栏 + 内容 */}
      <div className="flex flex-1 min-h-0 min-w-0">
        {/* ===== 左侧 72px 侧边栏（照搬设计稿） ===== */}
        <aside id="sidebar-nav" className="flex flex-col items-center py-3 gap-1 flex-shrink-0 z-10" style={{ width: 72, background: "var(--luxreal-sidebar)", borderRight: "1px solid var(--luxreal-border)" }}>
          {/* Logo */}
          <div className="flex items-center justify-center flex-shrink-0" style={{ height: 56, marginBottom: 4 }}>
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M6 4H22V8H10V24H6V4Z" fill="white" />
              <path d="M14 12H22V16H18V24H14V12Z" fill="white" opacity="0.7" />
            </svg>
          </div>

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 中心导航 */}
          <nav className="flex flex-col items-center gap-2 flex-shrink-0">
            <Link to="/" className="luxreal-nav-item active" title="创作">
              <Sparkles size={24} />
              <span>创作</span>
            </Link>
            <Link to="/feed" className="luxreal-nav-item" title="发现">
              <Compass size={24} />
              <span>发现</span>
            </Link>
            <Link to="/projects" className="luxreal-nav-item" title="作品">
              <Folder size={24} />
              <span>作品</span>
            </Link>
          </nav>

          <div className="flex-1" style={{ minHeight: 40 }} />

          {/* 头像(点击展开面板，LuxReal 对齐版 pp-panel) */}
          <div className="pp-anchor" style={{ marginBottom: 4 }}>
            <div
              id="sidebarAvatar"
              className="sidebar-avatar"
              onClick={(e) => {
                e.stopPropagation();
                setProfileOpen((v) => !v);
              }}
            >
              {(user?.nickname || "Y").slice(0, 1).toUpperCase()}
              {avatarUrl && (
                <img
                  className="sidebar-avatar-img"
                  src={avatarUrl}
                  alt=""
                  onError={(e) => { e.currentTarget.style.display = "none"; }}
                />
              )}
            </div>
            <ProfilePanel open={profileOpen} onClose={() => setProfileOpen(false)} />
          </div>

          {/* 积分（真实余额，金色徽章 Coins 图标 + 数字） */}
          <span className="sidebar-credits">
            <span className="sidebar-credits-badge">
              <Coins size={16} className="sidebar-credits-icon" />
            </span>
            {points === null ? "…" : points.toLocaleString()}
          </span>
          {/* 充值 */}
          <Link to="/points" className="sidebar-recharge" title="充值">
            充值
          </Link>
        </aside>

        {/* ===== 主内容区 ===== */}
        <div className="flex-1 flex flex-col min-w-0 min-h-0">
          <main data-scroll-region="primary" className="overflow-y-auto flex-1">
            <div id="agent-view">
              {/* 窄居中区:标题 + Tab + 输入 */}
              <div className="w-[88%] max-w-[1400px] mx-auto px-6">
                {/* 品牌标题（逐字拆分：光束扫过时逐字缓慢照亮，见 home.css fl-letter） */}
                <div id="brand-header" style={{ paddingTop: 64, paddingBottom: 32, textAlign: "center" }}>
                  <div className="flex flex-col items-center justify-center">
                    {/* 顶部公告条（运营内容：GET /announcements） */}
                    {notice && (
                      <div className="announcement-bar flex items-center gap-2 px-4 py-2 mb-4 rounded-lg bg-amber-gold/10 border border-amber-gold/20 text-sm max-w-3xl">
                        <Megaphone size={14} className="text-amber-gold flex-shrink-0" />
                        <span className="text-white font-medium flex-shrink-0">{notice.title}</span>
                        <span className="text-text-secondary truncate">{notice.content}</span>
                      </div>
                    )}
                    <h1 className="brand-title select-none" aria-label="EmptyBroken">
                      {"EmptyBroken".split("").map((ch, i) => (
                        <span key={i} className="brand-letter" aria-hidden style={{ animationDelay: `${i * 0.12}s` }}>
                          {ch}
                        </span>
                      ))}
                    </h1>
                    <p className="brand-sub text-[var(--luxreal-muted-foreground)] text-sm mt-3 tracking-widest">
                      {["AI", "驱动", "·", "短剧创作平台"].map((w, i) => (
                        <span key={i} className="brand-word" style={{ animationDelay: `${0.5 + i * 0.22}s` }}>
                          {w + "\u00A0"}
                        </span>
                      ))}
                    </p>
                  </div>
                </div>

                {/* 创作双 Tab */}
                <div id="creation-tabs" className="flex items-center justify-center gap-2 pb-6">
                  <button id="tab-agent" className={`creation-tab ${tab === "agent" ? "active" : ""}`} onClick={() => setTab("agent")}>
                    <Clapperboard size={16} className="relative z-[1]" />
                    <span className="relative z-[1]">短剧 Agent</span>
                  </button>
                  <button
                    type="button"
                    id="tab-canvas"
                    className={`creation-tab ${tab === "canvas" ? "active" : ""}`}
                    onClick={() => {
                      // 自由画布开发中：不跳转，仅提示（U3）
                      toast.info("自由画布开发中，敬请期待");
                    }}
                  >
                    <Palette size={16} />
                    <span>自由画布</span>
                    <span className="luxreal-badge luxreal-badge-beta" style={{ padding: "1px 7px", fontSize: 10 }}>
                      开发中
                    </span>
                  </button>
                </div>

                {/* 主输入卡 */}
                <form onSubmit={createProject}>
                  <section id="script-upload" className="luxreal-glass rounded-xl overflow-hidden" style={{ borderRadius: 20 }}>
                    <div className="flex" style={{ minHeight: 200 }}>
                      {/* 左:上传区(300px)——文件选择 + 拖拽 + 文件卡片态（方案 07-01 D.2） */}
                      <div
                        id="upload-dropzone"
                        className="flex flex-col items-center justify-center py-6 px-8 cursor-pointer"
                        style={{ flex: "0 0 300px", borderRight: "1px solid var(--border-default)", transition: "all 0.3s ease" }}
                        onClick={() => {
                          // 文件已就绪/处理中禁止重复选择（先移除或等待完成）
                          if (uploadState === "EMPTY") fileInputRef.current?.click();
                        }}
                        onDragOver={(e) => e.preventDefault()}
                        onDrop={(e) => {
                          e.preventDefault();
                          if (uploadState !== "EMPTY") return;
                          const f = e.dataTransfer.files?.[0];
                          if (f) handleFile(f);
                        }}
                      >
                        <input ref={fileInputRef} type="file" accept=".txt,.md,.doc,.docx,.pdf" style={{ display: "none" }} onChange={(e) => {
                          const f = e.target.files?.[0];
                          if (f) handleFile(f);
                        }} />
                        {uploadState === "FILE_READY" && uploadedFile ? (
                          /* 文件卡片态（对齐官网：文件名 + 绿勾 + 移除） */
                          <div className="upload-file-card">
                            <div className="upload-file-card-icon">
                              <FileText size={20} />
                            </div>
                            <div className="upload-file-card-info">
                              <div className="upload-file-card-name" title={uploadedFile.name}>{uploadedFile.name}</div>
                              <div className="flex items-center gap-1 mt-0.5">
                                <Check size={12} className="text-emerald-400 flex-shrink-0" />
                                <span className="text-[11px] text-emerald-400 leading-snug">已解析为剧本，如需手动输入请先移除文件</span>
                              </div>
                            </div>
                            <button type="button" className="upload-file-card-remove" title="移除文件" onClick={(e) => { e.stopPropagation(); removeUploadedFile(); }}>
                              <X size={14} />
                            </button>
                          </div>
                        ) : uploadState === "PARSE_FAILED" ? (
                          /* 解析失败卡片：错误文案 + 可移除重试 */
                          <div className="upload-file-card failed">
                            <div className="upload-file-card-icon">
                              <AlertCircle size={20} className="text-red-400" />
                            </div>
                            <div className="upload-file-card-info">
                              <div className="upload-file-card-name" style={{ color: "var(--luxreal-muted-foreground)" }}>解析失败</div>
                              <div className="text-[11px] text-red-400 leading-snug mt-0.5">{parseError}</div>
                            </div>
                            <button type="button" className="upload-file-card-remove" title="关闭" onClick={(e) => { e.stopPropagation(); removeUploadedFile(); }}>
                              <X size={14} />
                            </button>
                          </div>
                        ) : (
                          <>
                            <div className="upload-icon-stack mb-5">
                              <div className="upload-tile back">
                                <FileText size={26} />
                              </div>
                              <div className="upload-tile front">
                                <FileUp size={26} />
                              </div>
                            </div>
                            <div className="text-center">
                              <div className="flex items-center justify-center gap-1.5 mb-2">
                                <span className="text-[14px] font-medium text-white">
                                  {uploadState === "UPLOADING" ? "上传中…" : uploadState === "PARSING" ? "正在解析文档内容…" : "上传或拖放您的剧本文件"}
                                </span>
                                <span className="upload-tip-wrap">
                                  <HelpCircle size={14} className="text-[var(--luxreal-muted-foreground)] cursor-help" />
                                  <span className="upload-tip">支持 txt / md / doc / docx / pdf，单文件 ≤ 20MB，解析文本 ≤ 5万字；docx/pdf 自动解析为剧本文本</span>
                                </span>
                              </div>
                              {(uploadState === "UPLOADING" || uploadState === "PARSING") && (
                                <div className="flex items-center justify-center gap-2 mt-1 mb-1">
                                  <span className="upload-spinner" />
                                  <span className="text-[11px] text-[var(--luxreal-muted-foreground)]">
                                    {uploadState === "UPLOADING" ? "正在上传文件…" : "正在提取文字，请稍候…"}
                                  </span>
                                </div>
                              )}
                              <p className="text-[12px] text-[var(--luxreal-muted-foreground)]">
                                支持 txt / md / doc / docx / pdf；没有剧本？
                                <a
                                  href="#"
                                  className="info-link"
                                  onClick={(e) => {
                                    e.preventDefault();
                                    fillSampleScript(sampleProjects[0].script);
                                  }}
                                >
                                  使用示例剧本
                                </a>
                                开始创作体验
                              </p>
                            </div>
                          </>
                        )}
                      </div>

                      {/* 右:textarea + 工具栏 */}
                      <div className="flex flex-col flex-1 min-w-0 py-4 px-5">
                        <div className="flex-1">
                          <textarea
                            id="script-textarea"
                            className="chat-textarea"
                            rows={1}
                            value={uploadedFile ? "" : script}
                            disabled={uploadState === "FILE_READY" || uploadState === "UPLOADING" || uploadState === "PARSING"}
                            onChange={(e) => {
                              setScript(e.target.value);
                              // 格式互斥（方案 07-01 D.2）：手动输入即移除文件源
                              if (uploadedFile) {
                                setUploadedFile(null);
                                setUploadState("EMPTY");
                                setParseError("");
                              }
                            }}
                            placeholder={
                              uploadedFile
                                ? `已上传「${uploadedFile.name}」，剧本内容已就绪（${script.length} 字）；如需手动编辑请先移除文件`
                                : "通过左侧按钮上传剧本文件，或在此粘贴剧本文字（不超过5万字）"
                            }
                            maxLength={MAX_SCRIPT_CHARS}
                          />
                        </div>

                        {/* 快捷配置条 */}
                        <div id="quick-config" className="flex flex-wrap items-center gap-2.5 mt-3 pt-3" style={{ borderTop: "1px solid var(--luxreal-border)" }}>
                          <button type="button" id="btn-style-selector" className="toolbar-pill" onClick={openStylePopover}>
                            <span className="style-circle">
                              <Image size={14} className="text-white" />
                            </span>
                            <span id="current-style-name">{styleName}</span>
                            <ChevronDown size={13} className="ml-0.5 opacity-60" />
                          </button>

                          <button type="button" id="btn-model-config-toggle" className="toolbar-pill" style={{ gap: 6 }} onClick={() => setModelPanelOpen(true)}>
                            <span>模型配置</span>
                            <span className="pill-divider" />
                            <span id="config-resolution-label">{resolution}</span>
                            <span className="pill-divider" />
                            <span id="config-ratio-label">{ratio}</span>
                            <span className="pill-divider" />
                            <span id="config-duration-label">{videoDuration === "auto" ? "时长自动" : `时长 ${videoDuration}`}</span>
                            <Settings size={14} className="ml-1" />
                          </button>

                          <button type="button" id="btn-agent-modal-open" className="toolbar-pill" style={{ gap: 6 }} onClick={() => setAgentOpen(true)}>
                            <span>Agent技能</span>
                            <span className="pill-divider" />
                            <span id="agent-config-summary">标准模板</span>
                            <ChevronDown size={13} className="ml-0.5 opacity-60" />
                          </button>

                          <div className="flex-1" />

                          <span id="char-count" className="text-[12px] text-[var(--luxreal-muted-foreground)] mr-1">
                            {script.length} / {MAX_SCRIPT_CHARS}
                          </span>

                          <button type="submit" id="btn-enter-workbench" className={`btn-enter shrink-0 ${script.trim() ? "" : "disabled"}`} disabled={creating}>
                            <Wand2 size={14} />
                            <span>{creating ? "创建中…" : "进入工作台"}</span>
                          </button>
                        </div>
                      </div>
                    </div>
                  </section>
                </form>

                {/* 模型配置弹窗（luxreal-modal 体系，CSS 提取自设计稿） */}
                <div
                  id="model-config-modal"
                  className={`luxreal-modal-backdrop ${modelPanelOpen ? "visible" : ""}`}
                  onClick={(e) => {
                    if (e.target === e.currentTarget) setModelPanelOpen(false);
                  }}
                >
                  <div className="luxreal-modal" style={{ width: 640 }}>
                    <div className="luxreal-modal-header">
                      <div className="luxreal-modal-title">模型配置</div>
                      <button className="luxreal-modal-close" onClick={() => setModelPanelOpen(false)} aria-label="关闭">
                        <X size={16} />
                      </button>
                    </div>

                    <div className="flex items-center px-5 border-b border-[var(--luxreal-border)]">
                      {(["video", "character", "scene", "prop"] as const).map((mt) => (
                        <button key={mt} className={`model-subtab ${modelTab === mt ? "active" : ""}`} onClick={() => setModelTab(mt)}>
                          {mt === "video" ? "视频" : mt === "character" ? "角色图片" : mt === "scene" ? "场景图片" : "道具图片"}
                        </button>
                      ))}
                    </div>

                    <div className="luxreal-modal-body">
                      {modelTab === "video" && (
                        <div id="modeltab-video" className="model-tab-panel">
                          <div className="mb-6">
                            <p className="text-[12px] text-[var(--luxreal-muted-foreground)] mb-3">视频生成模型</p>
                            <div className="flex flex-wrap gap-2">
                              {t2vModels.map((m) => (
                                <button
                                  key={m.model_key}
                                  className={`video-model-btn ${videoModel === m.model_key ? "active" : ""}`}
                                  data-video-model={m.model_key}
                                  disabled={!m.enabled}
                                  onClick={() => setVideoModel(m.model_key)}
                                  style={!m.enabled ? { opacity: 0.4, cursor: "not-allowed" } : undefined}
                                >
                                  <span>{m.name || m.model_key}</span>
                                  {m.cost_points > 0 && (
                                    <span className="luxreal-badge luxreal-badge-vip ml-1.5" style={{ fontSize: 10, padding: "1px 6px" }}>
                                      {m.cost_points} 积分
                                    </span>
                                  )}
                                </button>
                              ))}
                            </div>
                          </div>

                          <div className="mb-6">
                            <p className="text-[12px] text-[var(--luxreal-muted-foreground)] mb-3">分辨率</p>
                            <div className="flex flex-wrap gap-2">
                              <div className="segmented-group">
                                {["480P", "720P", "1080P"].map((r) => (
                                  <button key={r} className={`segmented-btn ${resolution === r ? "active" : ""}`} onClick={() => setResolution(r)}>
                                    {r}
                                  </button>
                                ))}
                              </div>
                            </div>
                          </div>

                          <div className="mb-6">
                            <p className="text-[12px] text-[var(--luxreal-muted-foreground)] mb-3">画面比例</p>
                            <div className="flex flex-wrap gap-2">
                              <div className="segmented-group">
                                {["16:9", "1:1", "9:16"].map((r) => (
                                  <button key={r} className={`segmented-btn ${ratio === r ? "active" : ""}`} onClick={() => setRatio(r)}>
                                    {r}
                                  </button>
                                ))}
                              </div>
                            </div>
                          </div>

                          <div>
                            <p className="text-[12px] text-[var(--luxreal-muted-foreground)] mb-3">视频时长</p>
                            <div className="flex flex-wrap gap-2">
                              <div className="segmented-group">
                                {[
                                  { value: "auto", label: "自动" },
                                  { value: "1分钟", label: "1分钟" },
                                  { value: "1-2分钟", label: "1-2分钟" },
                                  { value: "2-3分钟", label: "2-3分钟" },
                                  { value: "3-5分钟", label: "3-5分钟" },
                                  { value: "5-10分钟", label: "5-10分钟" },
                                ].map((d) => (
                                  <button key={d.value} className={`segmented-btn ${videoDuration === d.value ? "active" : ""}`} onClick={() => setVideoDuration(d.value)}>
                                    {d.label}
                                  </button>
                                ))}
                              </div>
                            </div>
                          </div>
                        </div>
                      )}
                      {modelTab !== "video" && (
                        <div className="model-tab-panel">
                          <div className="mb-6">
                            <p className="text-[12px] text-[var(--luxreal-muted-foreground)] mb-3">
                              {modelTab === "character" ? "角色" : modelTab === "scene" ? "场景" : "道具"}图形生成模型
                            </p>
                            <div className="flex flex-wrap gap-2">
                              {t2iModels.map((m) => (
                                <button
                                  key={m.model_key}
                                  className={`video-model-btn ${imageModel[modelTab] === m.model_key ? "active" : ""}`}
                                  disabled={!m.enabled}
                                  onClick={() => setImageModel((prev) => ({ ...prev, [modelTab]: m.model_key }))}
                                  style={!m.enabled ? { opacity: 0.4, cursor: "not-allowed" } : undefined}
                                >
                                  <span>{m.name || m.model_key}</span>
                                  {m.cost_points > 0 && (
                                    <span className="luxreal-badge luxreal-badge-vip ml-1.5" style={{ fontSize: 10, padding: "1px 6px" }}>
                                      {m.cost_points} 积分
                                    </span>
                                  )}
                                </button>
                              ))}
                            </div>
                          </div>
                        </div>
                      )}
                    </div>

                    <div className="luxreal-modal-footer">
                      <button className="btn-ghost" onClick={() => setModelPanelOpen(false)}>取消</button>
                      <button className="btn-gradient-primary" onClick={() => void saveModelConfig()}>
                        <span>确认配置</span>
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              {/* 项目卡网格(固定 4 卡:真实项目 + 示例补齐;封面用真实图片) */}
              <div className="w-[88%] max-w-[1400px] mx-auto px-6">
                <section id="example-projects" className="mt-6 mb-6">
                  <div className="grid grid-cols-4 gap-4">
                    {displayProjects.map((p) => (
                      <div
                        key={p.key}
                        className="project-card cursor-pointer group"
                        style={{ backgroundImage: `url(${p.cover})`, backgroundSize: "cover", backgroundPosition: "center" }}
                        onClick={() => {
                          if (p.projectId) {
                            navigate(`/projects/${p.projectId}/step/1`);
                          } else if (p.script) {
                            fillSampleScript(p.script);
                          }
                        }}
                      >
                        <div className="card-gradient-overlay" />
                        {/* hover 播放遮罩（设计稿 play-overlay） */}
                        <div className="play-overlay">
                          <div className="play-btn-circle">
                            <Play size={18} className="text-white" fill="currentColor" />
                          </div>
                        </div>
                        <span
                          className={`card-badge text-[10px] px-2 py-0.5 rounded text-white font-medium ${
                            p.projectId ? "bg-[var(--luxreal-primary)]/80" : "bg-white/15 backdrop-blur"
                          }`}
                        >
                          {p.badge}
                        </span>
                        <button className="card-menu" onClick={(e) => openCardMenu(e, p.projectId)}>
                          <MoreHorizontal size={16} />
                        </button>
                        <div className="card-info">
                          <p className="text-[13px] font-bold text-white truncate leading-tight mb-1">{p.title}</p>
                          <p className="text-[11px] text-white/60">创建时间:{p.time}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </section>
              </div>

              {/* ===== 宽区:精选推荐轮播(照搬设计稿) ===== */}
              <div className="w-[80%] mx-auto px-4 pb-8 mt-2">
                <section id="recommendation-banners" className="mb-6 mt-6">
                  <div className="flex items-center gap-2 mb-4">
                    <ThumbsUp size={16} className="text-[var(--luxreal-primary)]" />
                    <h3 className="text-[16px] font-semibold text-white">精选推荐</h3>
                  </div>
                  <div
                    className="carousel-wrapper relative"
                    id="carousel-wrapper"
                    onMouseEnter={() => setCarouselPaused(true)}
                    onMouseLeave={() => setCarouselPaused(false)}
                  >
                    <div className="carousel-track flex gap-4 overflow-x-auto no-scrollbar scroll-smooth pb-1" id="carousel-track" ref={carouselRef}>
                      {carouselSlides.map((s, idx) => (
                        <div
                          key={s.title}
                          className={`carousel-slide flex-shrink-0 cursor-pointer group ${carouselIndex === idx ? "is-active" : ""}`}
                          data-index={idx}
                          onClick={() => {
                            if (carouselIndex !== idx) {
                              setCarouselIndex(idx);
                              return;
                            }
                            if (s.to) {
                              if (/^https?:\/\//.test(s.to)) {
                                window.open(s.to, "_blank", "noopener");
                                return;
                              }
                              navigate(s.to);
                            } else if (s.toastMsg) toast.info(s.toastMsg);
                          }}
                        >
                          <div className="carousel-card">
                            <div
                              className="carousel-cover"
                              style={{ backgroundImage: `url(${s.img})`, backgroundSize: "cover", backgroundPosition: "center" }}
                            >
                              <div className="carousel-title-overlay">
                                <p className="carousel-title">{s.title}</p>
                                <p className="carousel-subtitle">{s.subtitle}</p>
                              </div>
                              <div className="carousel-action">
                                <span>{s.action}</span>
                                <ChevronRight size={14} />
                              </div>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                    <button className="carousel-arrow carousel-prev" id="carousel-prev" aria-label="上一张" onClick={() => scrollToSlide((carouselIndex - 1 + SLIDE_COUNT) % SLIDE_COUNT)}>
                      <ChevronLeft size={20} />
                    </button>
                    <button className="carousel-arrow carousel-next" id="carousel-next" aria-label="下一张" onClick={() => scrollToSlide((carouselIndex + 1) % SLIDE_COUNT)}>
                      <ChevronRight size={20} />
                    </button>
                    <div className="carousel-dots" id="carousel-dots">
                      {carouselSlides.map((_, d) => (
                        <button key={d} className={`carousel-dot ${carouselIndex === d ? "active" : ""}`} data-dot={d} onClick={() => scrollToSlide(d)} />
                      ))}
                    </div>
                  </div>
                </section>

                {/* 底部页脚(照搬设计稿) */}
                <footer className="site-footer">
                  <span>专业短剧模式：新增 Seedance2.0 模型！</span>
                  <a href="#">加入 EmptyBroken 官方社群</a>
                  <div className="flex items-center gap-2">
                    <a href="#" className="text-white">中文</a>
                    <span className="text-[var(--luxreal-border)]">|</span>
                    <a href="#">EN</a>
                  </div>
                  <span className="text-[11px] opacity-60">关注微信公众号获取最新动态</span>
                </footer>
              </div>
            </div>
          </main>
        </div>
      </div>

      {/* ===== 风格选择弹层（对齐设计稿 #popover-style-selector） ===== */}
      <div
        id="popover-style-selector"
        className={`luxreal-popover ${styleOpen ? "visible" : ""}`}
        style={stylePos ? { top: stylePos.top, left: stylePos.left } : undefined}
      >
        <div className="luxreal-popover-arrow" style={{ left: 50 }} />
        <div className="popover-body" style={{ padding: 16 }}>
          <div className="style-section-title">风格库</div>
          <div className="style-grid" id="style-grid-library">
            {styleLib.slice(0, 5).map((s, i) => renderStyleCard(s, i))}
          </div>
          <div className="style-section-title">公共库</div>
          <div className="style-grid" id="style-grid-public">
            {styleLib.slice(5).map((s, i) => renderStyleCard(s, i + 5))}
          </div>
        </div>
      </div>

      {/* ===== 项目卡下拉菜单（对齐设计稿 #card-dropdown-menu） ===== */}
      <div
        id="card-dropdown-menu"
        className={`card-dropdown-menu ${menuFor ? "visible" : ""}`}
        style={menuPos ? { top: menuPos.top, left: menuPos.left } : undefined}
      >
        <button className="card-dropdown-item" onClick={() => { if (menuFor) navigate(`/projects/${menuFor}/step/1`); }}>
          <FolderOpen size={14} />
          <span>打开项目</span>
        </button>
        <button
          className="card-dropdown-item"
          onClick={() => {
            if (menuProject) {
              setRenameTarget(menuProject);
              setRenameVal(menuProject.title);
            }
            closeMenu();
          }}
        >
          <Edit3 size={14} />
          <span>重命名</span>
        </button>
        <button className="card-dropdown-item" onClick={() => { toast.info("复制项目功能将在后续版本提供"); closeMenu(); }}>
          <Copy size={14} />
          <span>复制项目</span>
        </button>
        <button className="card-dropdown-item" onClick={() => { toast.info("导出功能将在后续版本提供"); closeMenu(); }}>
          <Download size={14} />
          <span>导出</span>
        </button>
        <div style={{ height: 1, background: "rgba(255,255,255,0.06)", margin: "4px 0" }} />
        <button
          className="card-dropdown-item danger"
          onClick={() => {
            if (menuProject) setDeleteTarget(menuProject);
            closeMenu();
          }}
        >
          <Trash2 size={14} />
          <span>删除</span>
        </button>
      </div>

      {/* ===== Agent 技能配置弹窗（共享组件，对齐设计稿 #modal-agent-config） ===== */}
      <AgentSkillModal open={agentOpen} onClose={() => setAgentOpen(false)} />

      {/* ===== 重命名弹窗（PUT /projects/:id） ===== */}
      <div
        className={`luxreal-modal-backdrop ${renameTarget ? "visible" : ""}`}
        onClick={(e) => {
          if (e.target === e.currentTarget) setRenameTarget(null);
        }}
      >
        <div className="luxreal-modal" style={{ width: 420 }}>
          <div className="luxreal-modal-header">
            <div className="luxreal-modal-title">重命名项目</div>
            <button className="luxreal-modal-close" onClick={() => setRenameTarget(null)} aria-label="关闭">
              <X size={16} />
            </button>
          </div>
          <div className="luxreal-modal-body">
            <input
              value={renameVal}
              onChange={(e) => setRenameVal(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void submitRename();
              }}
              maxLength={50}
              placeholder="请输入项目名称"
              autoFocus
              style={{
                width: "100%", background: "rgba(255,255,255,0.04)", border: "1px solid rgba(255,255,255,0.1)",
                borderRadius: 10, padding: "10px 14px", fontSize: 13, color: "var(--luxreal-foreground)", outline: "none",
              }}
            />
          </div>
          <div className="luxreal-modal-footer">
            <button className="btn-ghost" onClick={() => setRenameTarget(null)}>取消</button>
            <button className="btn-gradient-primary" onClick={() => void submitRename()}>
              <span>确认</span>
            </button>
          </div>
        </div>
      </div>

      {/* ===== 删除确认弹窗（DELETE /projects/:id 软删） ===== */}
      <div
        className={`luxreal-modal-backdrop ${deleteTarget ? "visible" : ""}`}
        onClick={(e) => {
          if (e.target === e.currentTarget) setDeleteTarget(null);
        }}
      >
        <div className="luxreal-modal" style={{ width: 420 }}>
          <div className="luxreal-modal-header">
            <div className="luxreal-modal-title">删除项目</div>
            <button className="luxreal-modal-close" onClick={() => setDeleteTarget(null)} aria-label="关闭">
              <X size={16} />
            </button>
          </div>
          <div className="luxreal-modal-body">
            <p className="text-[13px] text-[var(--luxreal-foreground)]">
              确定要删除项目「{deleteTarget?.title}」吗？删除后可在回收站恢复。
            </p>
          </div>
          <div className="luxreal-modal-footer">
            <button className="btn-ghost" onClick={() => setDeleteTarget(null)}>取消</button>
            <button
              className="btn-gradient-primary"
              style={{ background: "linear-gradient(135deg, #EF4444, #B91C1C)" }}
              onClick={() => void submitDelete()}
            >
              <span>确认删除</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
