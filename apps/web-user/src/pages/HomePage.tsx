// 创作入口页 —— 按 luxreal-clone 设计稿 create-video.html 融合（批次 1）
// CSS 原样提取（全局 token 585 行 + 页面 2590 行）→ src/pages/home.css（铁律①）
// 接口对接：GET /projects、POST /projects（01 文档 4.2）
// 左侧 72px 侧边栏按设计稿照搬（AppShell 雏形，批次 2 抽为共享布局）
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  AlertCircle, Check, ChevronDown, ChevronLeft, ChevronRight, Clapperboard, Compass, Copy,
  Download, Edit3, FileText, FileUp, Folder, FolderOpen, HelpCircle, Image,
  Megaphone, MoreHorizontal, Palette, Play, Settings, Sparkles, ThumbsUp, Trash2, Wand2, X,
} from "lucide-react";

import { api, ApiError, errorMessage } from "@duanju/api-client";
import AgentSkillModal from "../components/AgentSkillModal";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
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
  const [profileOpen, setProfileOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [projects, setProjects] = useState<ProjectItem[]>([]);
  const [points, setPoints] = useState<number | null>(null);
  // 风格选择弹层（GET /style-library 真数据；失败回退设计稿固定 10 项）
  const [styleOpen, setStyleOpen] = useState(false);
  const [stylePos, setStylePos] = useState<{ top: number; left: number } | null>(null);
  const [styleName, setStyleName] = useState("通用写实");
  const [styleCode, setStyleCode] = useState<string>("");
  const [styleLib, setStyleLib] = useState<{ code: string; name: string }[]>([]);
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

  // 舞台灯光：左上/右上双光源，16s 循环扫描时间线（类名与 home.css flashlight 系契约对齐）
  // 每个目标区域（品牌名 → 聊天框 → 我的项目 → 精选推荐）光斑自左向右滑动扫描；
  // 光束角度每帧按 atan2 实时追光斑（手电筒追光感）；扫品牌名时左光束变长、右光束变短，
  // 其余阶段等长；收光复位首尾帧一致，无缝循环。光斑锚定目标元素，随滚动/resize 跟随。
  useEffect(() => {
    const stage = document.querySelector<HTMLElement>(".home-stage");
    const spotBrand = document.querySelector<HTMLElement>(".flashlight-spot.spot-brand");
    const spotInput = document.querySelector<HTMLElement>(".flashlight-spot.spot-input");
    const spotProjects = document.querySelector<HTMLElement>(".flashlight-spot.spot-projects");
    const spotCarousel = document.querySelector<HTMLElement>(".flashlight-spot.spot-carousel");
    const beamLeft = document.querySelector<HTMLElement>(".flashlight.is-left");
    const beamRight = document.querySelector<HTMLElement>(".flashlight.is-right");
    const brandTitle = document.querySelector<HTMLElement>(".brand-title");
    if (!stage || !spotBrand || !spotInput || !spotProjects || !spotCarousel || !beamLeft || !beamRight) return;

    // 目标区域（光斑锚定中心，stage 相对坐标）
    const targets = [
      { el: () => document.getElementById("brand-header"), spot: spotBrand },
      { el: () => document.getElementById("script-upload"), spot: spotInput },
      { el: () => document.getElementById("example-projects"), spot: spotProjects },
      { el: () => document.getElementById("recommendation-banners"), spot: spotCarousel },
    ];
    const centers = new Map<HTMLElement, { x: number; y: number }>();
    const rects = new Map<HTMLElement, { left: number; top: number; width: number; height: number }>();
    let vw = window.innerWidth; // 右光源 x 坐标（右上角）
    let letters: { el: HTMLElement; cx: number }[] = [];
    let words: HTMLElement[] = [];

    const sync = () => {
      const sr = stage.getBoundingClientRect();
      vw = window.innerWidth;
      for (const t of targets) {
        const el = t.el();
        if (!el) continue;
        const r = el.getBoundingClientRect();
        centers.set(t.spot, { x: r.left + r.width / 2 - sr.left, y: r.top + r.height / 2 - sr.top });
        rects.set(t.spot, { left: r.left - sr.left, top: r.top - sr.top, width: r.width, height: r.height });
      }
      // 逐字渐变分配：每个字符自带渐变裁切，按实测偏移分配 background-position，
      // 保持整词渐变连续（父级 clip:text 无法穿透带 filter 的子元素——实测坑）
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
    const raf1 = requestAnimationFrame(sync); // 首屏布局/字体稳定后再锚一次
    // 字体加载完成后字符宽度会变化（webfont 替换回退字体），必须重新分配渐变位置，
    // 否则后段字母错位露出透明区；ResizeObserver 兜底品牌名尺寸变化
    let alive = true;
    const resync = () => { if (alive) sync(); };
    document.fonts?.ready.then(resync).catch(() => {});
    const ro = brandTitle && typeof ResizeObserver !== "undefined" ? new ResizeObserver(resync) : null;
    if (brandTitle) ro?.observe(brandTitle);
    window.addEventListener("resize", sync);
    window.addEventListener("scroll", sync, true); // 捕获内部内容容器滚动

    // —— 16s 扫描时间线（rAF 驱动，只写 transform/opacity 等合成层属性） ——
    const DURATION = 16000;
    const clamp01 = (v: number) => (v < 0 ? 0 : v > 1 ? 1 : v);
    const ease = (t: number) => t * t * (3 - 2 * t); // smoothstep：移动缓入缓出
    const ramp = (t: number, a: number, b: number) => clamp01((t - a) / (b - a));
    const setSpotAt = (spot: HTMLElement, alpha: number, x: number, y: number) => {
      spot.style.opacity = alpha.toFixed(3);
      spot.style.transform = `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px) translate(-50%, -50%)`;
    };
    const start = performance.now();
    let raf2 = 0;
    const tick = (now: number) => {
      if (!alive) return;
      const t = ((now - start) % DURATION) / 1000; // 0–16s 循环相位

      // —— 各区域滑动进度（smoothstep，自左向右） ——
      const brandP = ease(clamp01((t - 0.3) / 3.4)); // 品牌名：0.3–3.7s
      const inputP = ease(clamp01((t - 4.6) / 2.2)); // 输入框：4.6–6.8s
      const projP = ease(clamp01((t - 7.0) / 2.2));  // 我的项目：7.0–9.2s
      const carP = ease(clamp01((t - 9.4) / 2.2));   // 精选推荐：9.4–11.6s

      // —— 焦点光斑位置（光束追光目标）：逐阶段滑动 + 阶段间 0.4s 平滑过渡 ——
      const brandR = rects.get(spotBrand);
      const inputR = rects.get(spotInput);
      const projR = rects.get(spotProjects);
      const carR = rects.get(spotCarousel);
      const brandC = centers.get(spotBrand);
      let focusX = brandC?.x ?? vw / 2;
      let focusY = brandC?.y ?? 100;
      if (t < 4.0 && brandR && brandC) {
        focusX = brandR.left + brandP * brandR.width;
        focusY = brandC.y;
      } else if (t < 4.6) {
        // 品牌名末端 → 输入框起点
        const k = ease(ramp(t, 4.0, 4.6));
        const sx = brandR ? brandR.left + brandR.width : focusX;
        const sy = brandC ? brandC.y : focusY;
        const ex = inputR ? inputR.left : sx;
        const ey = inputR ? inputR.top + inputR.height / 2 : sy;
        focusX = sx + (ex - sx) * k;
        focusY = sy + (ey - sy) * k;
      } else if (t < 7.0 && inputR) {
        focusX = inputR.left + inputP * inputR.width;
        focusY = inputR.top + inputR.height / 2;
      } else if (t < 7.4) {
        // 输入框末端 → 项目起点
        const k = ease(ramp(t, 7.0, 7.4));
        const sx = inputR ? inputR.left + inputR.width : focusX;
        const sy = inputR ? inputR.top + inputR.height / 2 : focusY;
        const ex = projR ? projR.left : sx;
        const ey = projR ? projR.top + projR.height / 2 : sy;
        focusX = sx + (ex - sx) * k;
        focusY = sy + (ey - sy) * k;
      } else if (t < 9.4 && projR) {
        focusX = projR.left + projP * projR.width;
        focusY = projR.top + projR.height / 2;
      } else if (t < 9.8) {
        // 项目末端 → 精选推荐起点
        const k = ease(ramp(t, 9.4, 9.8));
        const sx = projR ? projR.left + projR.width : focusX;
        const sy = projR ? projR.top + projR.height / 2 : focusY;
        const ex = carR ? carR.left : sx;
        const ey = carR ? carR.top + carR.height / 2 : sy;
        focusX = sx + (ex - sx) * k;
        focusY = sy + (ey - sy) * k;
      } else if (t < 11.8 && carR) {
        focusX = carR.left + carP * carR.width;
        focusY = carR.top + carR.height / 2;
      } else if (t < 13.6) {
        // 收回：精选推荐末端 → 品牌名中心（下一轮起点）
        const k = ease(ramp(t, 11.8, 13.6));
        const sx = carR ? carR.left + carR.width : focusX;
        const sy = carR ? carR.top + carR.height / 2 : focusY;
        focusX = sx + ((brandC?.x ?? sx) - sx) * k;
        focusY = sy + ((brandC?.y ?? sy) - sy) * k;
      } else if (brandC) {
        focusX = brandC.x;
        focusY = brandC.y;
      }

      // —— 光束角度实时追光（左光源 72,40；右光源 vw,40）
      // conic-gradient(from -90deg) = 锥心在 90deg（正下），需减 90° 使锥心指向 atan2 方向 ——
      const angleL = (Math.atan2(focusY - 40, focusX - 72) * 180) / Math.PI - 90;
      const angleR = (Math.atan2(focusY - 40, focusX - vw) * 180) / Math.PI - 90;
      beamLeft.style.setProperty("--beam-a-l", angleL.toFixed(2) + "deg");
      beamRight.style.setProperty("--beam-a-r", angleR.toFixed(2) + "deg");

      // —— 光束长度：按光源到目标的实际距离计算，确保光束末端到达光斑
      const vh2 = window.innerHeight;
      const baseBeamH2 = 2 * vh2; // 主光束 200vh ≈ 2vh
      const distL = Math.sqrt((focusX - 72) ** 2 + (focusY - 40) ** 2);
      const distR = Math.sqrt((focusX - vw) ** 2 + (focusY - 40) ** 2);
      let baseLenL = (distL / baseBeamH2) * 1.15;
      let baseLenR = (distR / baseBeamH2) * 1.15;
      // 动画节奏：品牌名阶段左长右短，其余等长，收尾同步收回
      let modL = 1, modR = 1;
      if (t < 4.0) { modL = 1 + 0.25 * brandP; modR = 1 - 0.2 * brandP; }
      else if (t < 4.6) { const k = ramp(t, 4.0, 4.6); modL = 1.25 - 0.25 * k; modR = 0.8 + 0.2 * k; }
      else if (t >= 11.8) { const k = ease(ramp(t, 11.8, 14.2)); modL = 1 - 0.7 * k; modR = 1 - 0.7 * k; }
      const lenL = Math.max(0.2, baseLenL * modL);
      const lenR = Math.max(0.2, baseLenR * modR);
      beamLeft.style.setProperty("--beam-l", lenL.toFixed(3));
      beamRight.style.setProperty("--beam-r", lenR.toFixed(3));
      // 光束亮度：苏醒 → 全亮 → 收回（首尾一致 0.3）
      const beamOp = t < 0.5 ? 0.3 + 0.6 * ramp(t, 0, 0.5) : t < 11.8 ? 0.9 : 0.9 - 0.6 * ease(ramp(t, 11.8, 13.4));
      beamLeft.style.opacity = beamOp.toFixed(3);
      beamRight.style.opacity = beamOp.toFixed(3);
      for (const b of [beamLeft, beamRight]) {
        const fog = b.querySelector<HTMLElement>(".flashlight-fog");
        if (fog) fog.style.opacity = (beamOp * 0.66).toFixed(3);
      }

      // —— 光斑在各区域内自左向右滑动（0.4s 淡入/淡出交叉衔接） ——
      const fade = (t0: number, t1: number) => Math.min(ramp(t, t0, t0 + 0.4), 1 - ramp(t, t1, t1 + 0.4));
      if (brandR && brandC) {
        setSpotAt(spotBrand, 0.95 * fade(0.15, 3.7), brandR.left + brandP * brandR.width, brandC.y);
      } else {
        spotBrand.style.opacity = "0";
      }
      if (inputR) {
        setSpotAt(spotInput, 0.9 * fade(4.3, 6.7), inputR.left + inputP * inputR.width, inputR.top + inputR.height / 2);
      } else {
        spotInput.style.opacity = "0";
      }
      if (projR) {
        setSpotAt(spotProjects, 0.9 * fade(6.7, 9.1), projR.left + projP * projR.width, projR.top + projR.height / 2);
      } else {
        spotProjects.style.opacity = "0";
      }
      if (carR) {
        setSpotAt(spotCarousel, 0.9 * fade(9.1, 11.5), carR.left + carP * carR.width, carR.top + carR.height / 2);
      } else {
        spotCarousel.style.opacity = "0";
      }

      // —— 品牌名扫描：光斑滑过字母逐个点亮并保持 ——
      if (brandR && t < 4.0) {
        const x = brandR.left + brandP * brandR.width;
        for (const l of letters) if (!l.el.classList.contains("lit") && x >= l.cx) l.el.classList.add("lit");
      }
      // 副标题逐词点亮（字母扫完后，0.22s/词）
      if (t >= 3.3 && t < 14.4) words.forEach((w, i) => { if (t >= 3.3 + i * 0.22) w.classList.add("lit"); });

      // —— 阶段 C 收尾：字母复位变暗（CSS transition 平滑），准备下一轮 ——
      if (t >= 14.4 && t < 14.7) {
        letters.forEach((l) => l.el.classList.remove("lit"));
        words.forEach((w) => w.classList.remove("lit"));
      }
      raf2 = requestAnimationFrame(tick);
    };
    raf2 = requestAnimationFrame(tick);

    return () => {
      alive = false;
      cancelAnimationFrame(raf1);
      cancelAnimationFrame(raf2);
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
  useEffect(() => {
    (async () => {
      try {
        const data = await api.get<{ items: { style_code: string; name: string }[] }>("/style-library");
        const items = (data.items ?? []).map((s) => ({ code: s.style_code, name: s.name }));
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
      // 3. 回填输入框 + 文件卡片态（格式互斥：文件存在期间禁止手动编辑）
      setScript(text);
      setUploadedFile({ name: file.name, size: file.size });
      setUploadState("FILE_READY");
      toast.success(`剧本「${file.name}」已解析并填入输入框`);
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
  const renderStyleCard = (s: { code: string; name: string }, idx: number) => (
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
          三层光束系统：外层辉光（可见光柱散射）+ 主光束（锥形聚光）+ 内层核心（白热灯丝） */}
      <div className="flashlight-layer" aria-hidden>
        <div className="flashlight is-left">
          <div className="flashlight-fog" />
          <div className="flashlight-beam-glow" />
          <div className="flashlight-beam" />
          <div className="flashlight-beam-core" />
          <div className="flashlight-src" />
        </div>
        <div className="flashlight is-right">
          <div className="flashlight-fog" />
          <div className="flashlight-beam-glow" />
          <div className="flashlight-beam" />
          <div className="flashlight-beam-core" />
          <div className="flashlight-src" />
        </div>
        <div className="flashlight-spot spot-brand" />
        <div className="flashlight-spot spot-input" />
        <div className="flashlight-spot spot-projects" />
        <div className="flashlight-spot spot-carousel" />
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
              <Sparkles size={22} />
              <span>创作</span>
            </Link>
            <Link to="/feed" className="luxreal-nav-item" title="发现">
              <Compass size={22} />
              <span>发现</span>
            </Link>
            <Link to="/projects" className="luxreal-nav-item" title="作品">
              <Folder size={22} />
              <span>作品</span>
            </Link>
          </nav>

          <div className="flex-1" style={{ minHeight: 20 }} />

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

          {/* 积分（真实余额） */}
          <span className="sidebar-credits">{points === null ? "…" : points.toLocaleString()}</span>
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
                      setTab("canvas");
                      navigate("/canvas");
                    }}
                  >
                    <Palette size={16} />
                    <span>自由画布</span>
                    <span className="luxreal-badge luxreal-badge-beta" style={{ padding: "1px 7px", fontSize: 10 }}>
                      Beta
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
                            value={script}
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
                            placeholder="通过左侧按钮上传剧本文件，或在此粘贴剧本文字（不超过5万字）"
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
                              {[
                                { key: "seedance25", label: "Seedance 2.5", vip: true },
                                { key: "seedance20", label: "Seedance 2.0" },
                                { key: "seedance20fast", label: "Seedance 2.0 Fast" },
                                { key: "klingo3", label: "KlingO3" },
                              ].map((m) => (
                                <button
                                  key={m.key}
                                  className={`video-model-btn ${videoModel === m.key ? "active" : ""}`}
                                  data-video-model={m.key}
                                  onClick={() => setVideoModel(m.key)}
                                >
                                  <span>{m.label}</span>
                                  {m.vip && (
                                    <span className="luxreal-badge luxreal-badge-vip ml-1.5" style={{ fontSize: 10, padding: "1px 6px" }}>
                                      VIP
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
                            <div className="space-y-3">
                              {IMAGE_MODEL_ROWS[modelTab].map((row) => (
                                <div key={row.prefix} className="model-gallery-row">
                                  <span className="model-gallery-label">{row.label}</span>
                                  <div className="model-gallery-thumbs">
                                    {row.grads.map((g, gi) => {
                                      const modelKey = `${row.prefix}-${gi + 1}`;
                                      return (
                                        <div
                                          key={modelKey}
                                          className={`model-thumb ${imageModel[modelTab] === modelKey ? "selected" : ""}`}
                                          onClick={() => setImageModel((prev) => ({ ...prev, [modelTab]: modelKey }))}
                                        >
                                          <div className={`thumb-preview thumb-grad-${g}`} />
                                        </div>
                                      );
                                    })}
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        </div>
                      )}
                    </div>

                    <div className="luxreal-modal-footer">
                      <button className="btn-ghost" onClick={() => setModelPanelOpen(false)}>取消</button>
                      <button className="btn-gradient-primary" onClick={() => setModelPanelOpen(false)}>
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
