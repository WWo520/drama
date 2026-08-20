// 发现页（批次4：设计稿 discover.html 迁移，DOM 1:1）
// CSS 原样提取 → src/pages/feed.css（theme tokens + shell 类 + 瀑布流卡片类）
// 接口对接：GET /works（workApi.feed，S3-F01 广场信息流）；失败或空时回退设计稿 30 张静态卡片（视觉保真）
// 搜索：后端暂无搜索接口，客户端按标题/作者过滤（state 保存）
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  Compass, Eye, Folder, Heart, Image, Loader2,
  Play, Search, Sparkles, Video,
} from "lucide-react";

import { api, workApi, type WorkItem } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
import "./feed.css";

// 卡片视觉元数据（瀑布流高度/缩略渐变/头像渐变/点赞数/播放数 —— 原样摘自设计稿 30 张卡）
interface FeedCard {
  title: string;
  author: string;
  kind: "video" | "image";
  duration?: string;
  height: number;
  thumb: number;
  avatar: string;
  likes: string;
  views: string;
}

// 设计稿静态卡片（API 无数据时的视觉回退；数据源 discover.html L1217-1954）
const FALLBACK_CARDS: FeedCard[] = [
  { title: "星际迷航：深空觉醒", author: "导演阿泽", kind: "video", duration: "00:32", height: 220, thumb: 1, avatar: "linear-gradient(135deg, #3B82F6, #8B5CF6)", likes: "12.4k", views: "89.2k" },
  { title: "末日黄昏", author: "落日画师", kind: "image", height: 180, thumb: 2, avatar: "linear-gradient(135deg, #EF4444, #F59E0B)", likes: "8.3k", views: "45.6k" },
  { title: "赛博雨林：数字绿洲", author: "青空影像", kind: "video", duration: "01:15", height: 260, thumb: 3, avatar: "linear-gradient(135deg, #10B981, #06B6D4)", likes: "23.1k", views: "156k" },
  { title: "紫水晶传说", author: "幻梦工坊", kind: "video", duration: "00:45", height: 200, thumb: 4, avatar: "linear-gradient(135deg, #8B5CF6, #EC4899)", likes: "18.7k", views: "102k" },
  { title: "玫瑰战争：血色婚礼", author: "绯红视觉", kind: "image", height: 240, thumb: 5, avatar: "linear-gradient(135deg, #EC4899, #EF4444)", likes: "31.2k", views: "203k" },
  { title: "深海巨兽：深渊回响", author: "深蓝叙事", kind: "video", duration: "00:28", height: 180, thumb: 6, avatar: "linear-gradient(135deg, #0369A1, #0284C7)", likes: "9.6k", views: "67.8k" },
  { title: "黄沙古道：丝路遗梦", author: "金色卷轴", kind: "image", height: 220, thumb: 7, avatar: "linear-gradient(135deg, #D97706, #F59E0B)", likes: "14.5k", views: "88.3k" },
  { title: "翡翠森林：精灵守护", author: "绿野仙踪", kind: "video", duration: "00:52", height: 200, thumb: 8, avatar: "linear-gradient(135deg, #16A34A, #10B981)", likes: "16.8k", views: "112k" },
  { title: "霓虹都市：夜行者", author: "霓虹制片厂", kind: "video", duration: "01:08", height: 260, thumb: 9, avatar: "linear-gradient(135deg, #7C3AED, #A855F7)", likes: "42.3k", views: "287k" },
  { title: "黑白叙事：无声之城", author: "灰调影像", kind: "image", height: 180, thumb: 10, avatar: "linear-gradient(135deg, #475569, #64748B)", likes: "7.2k", views: "38.9k" },
  { title: "苍穹之上：云端之战", author: "天行者工作室", kind: "video", duration: "00:38", height: 240, thumb: 11, avatar: "linear-gradient(135deg, #2563EB, #3B82F6)", likes: "27.5k", views: "198k" },
  { title: "旧城记忆：时光碎片", author: "老胶片", kind: "image", height: 200, thumb: 12, avatar: "linear-gradient(135deg, #78716C, #A8A29E)", likes: "11.3k", views: "72.4k" },
  { title: "青璃：水光之城", author: "碧水创作", kind: "video", duration: "00:41", height: 220, thumb: 13, avatar: "linear-gradient(135deg, #0D9488, #14B8A6)", likes: "19.2k", views: "134k" },
  { title: "赤焰：龙息降临", author: "烈焰影像", kind: "video", duration: "01:22", height: 260, thumb: 14, avatar: "linear-gradient(135deg, #DC2626, #F97316)", likes: "35.6k", views: "245k" },
  { title: "幻境之门：紫霄诀", author: "云梦泽", kind: "video", duration: "00:25", height: 180, thumb: 15, avatar: "linear-gradient(135deg, #6D28D9, #8B5CF6)", likes: "21.7k", views: "156k" },
  { title: "星轨：银河信差", author: "星尘画社", kind: "image", height: 240, thumb: 1, avatar: "linear-gradient(135deg, #1E40AF, #6366F1)", likes: "13.8k", views: "95.2k" },
  { title: "烬土：荒原求生", author: "荒原导演", kind: "video", duration: "00:58", height: 200, thumb: 2, avatar: "linear-gradient(135deg, #9A3412, #EA580C)", likes: "18.1k", views: "127k" },
  { title: "碧潭幽境", author: "山青水秀", kind: "image", height: 180, thumb: 3, avatar: "linear-gradient(135deg, #0F766E, #2DD4BF)", likes: "6.5k", views: "34.7k" },
  { title: "紫罗兰之夜", author: "暮光制片厂", kind: "video", duration: "00:35", height: 220, thumb: 4, avatar: "linear-gradient(135deg, #5B21B6, #7C3AED)", likes: "24.9k", views: "178k" },
  { title: "绯樱：落华抄", author: "樱花物语", kind: "image", height: 200, thumb: 5, avatar: "linear-gradient(135deg, #BE185D, #EC4899)", likes: "29.4k", views: "201k" },
  { title: "蓝鲸之歌：海洋之心", author: "潮汐映画", kind: "video", duration: "01:12", height: 260, thumb: 6, avatar: "linear-gradient(135deg, #075985, #0EA5E9)", likes: "17.6k", views: "119k" },
  { title: "麦田守望者", author: "秋日叙", kind: "video", duration: "00:22", height: 180, thumb: 7, avatar: "linear-gradient(135deg, #B45309, #FBBF24)", likes: "8.9k", views: "52.1k" },
  { title: "竹影清风：隐士录", author: "竹林七贤", kind: "image", height: 240, thumb: 8, avatar: "linear-gradient(135deg, #15803D, #4ADE80)", likes: "15.3k", views: "96.7k" },
  { title: "暗紫迷宫：意识边界", author: "迷墙视觉", kind: "video", duration: "00:48", height: 200, thumb: 9, avatar: "linear-gradient(135deg, #6B21A8, #C084FC)", likes: "22.8k", views: "163k" },
  { title: "雾都孤儿：工业时代", author: "蒸汽故事", kind: "image", height: 220, thumb: 10, avatar: "linear-gradient(135deg, #334155, #94A3B8)", likes: "10.7k", views: "63.5k" },
  { title: "天蓝色的彼岸", author: "晴空万里", kind: "video", duration: "00:30", height: 180, thumb: 11, avatar: "linear-gradient(135deg, #1D4ED8, #60A5FA)", likes: "13.2k", views: "84.6k" },
  { title: "墨色山河：长卷", author: "水墨斋", kind: "video", duration: "00:55", height: 260, thumb: 12, avatar: "linear-gradient(135deg, #57534E, #A8A29E)", likes: "26.1k", views: "189k" },
  { title: "冰川之下：秘境", author: "极地映像", kind: "image", height: 200, thumb: 13, avatar: "linear-gradient(135deg, #0E7490, #22D3EE)", likes: "9.1k", views: "58.3k" },
  { title: "火焰山：红孩儿传", author: "西游新说", kind: "video", duration: "00:42", height: 220, thumb: 14, avatar: "linear-gradient(135deg, #B91C1C, #F87171)", likes: "33.7k", views: "231k" },
  { title: "紫电：雷神之锤", author: "雷霆制作", kind: "video", duration: "01:05", height: 240, thumb: 15, avatar: "linear-gradient(135deg, #7C3AED, #A78BFA)", likes: "28.5k", views: "195k" },
];

// 真数据头像渐变轮转（API 作品无头像字段，按索引取色）
const AVATAR_GRADIENTS = [
  "linear-gradient(135deg, #3B82F6, #8B5CF6)",
  "linear-gradient(135deg, #EF4444, #F59E0B)",
  "linear-gradient(135deg, #10B981, #06B6D4)",
  "linear-gradient(135deg, #8B5CF6, #EC4899)",
  "linear-gradient(135deg, #7C3AED, #A855F7)",
  "linear-gradient(135deg, #2563EB, #3B82F6)",
  "linear-gradient(135deg, #D97706, #F59E0B)",
  "linear-gradient(135deg, #16A34A, #10B981)",
];
// 瀑布流高度轮转（对齐设计稿节奏）
const HEIGHT_CYCLE = [220, 180, 260, 200, 240];

// 时长秒 → mm:ss
function fmtDuration(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

// 计数 → 12.4k 形式（对齐设计稿）
function fmtCount(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

// 真接口数据 → 卡片视图模型
function toCard(w: WorkItem, idx: number): FeedCard {
  return {
    title: w.title,
    author: w.author,
    kind: "video",
    duration: fmtDuration(w.duration_sec),
    height: HEIGHT_CYCLE[idx % HEIGHT_CYCLE.length],
    thumb: (idx % 15) + 1,
    avatar: AVATAR_GRADIENTS[idx % AVATAR_GRADIENTS.length],
    likes: fmtCount(w.like_count),
    views: fmtCount(w.view_count),
  };
}

export default function FeedPage() {
  const { user } = useAuthStore();
  const avatarUrl = useAvatarUrl(user?.avatar_url);

  const [cards, setCards] = useState<FeedCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [keyword, setKeyword] = useState("");
  const [points, setPoints] = useState<number | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);

  // 广场信息流：GET /works；失败或空时回退设计稿静态卡（视觉保真，标注占位）
  useEffect(() => {
    (async () => {
      try {
        const data = await workApi.feed();
        setCards(data.items.length > 0 ? data.items.map(toCard) : FALLBACK_CARDS);
      } catch {
        setCards(FALLBACK_CARDS);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  // 侧栏积分余额（与 HomePage 同源）
  useEffect(() => {
    (async () => {
      try {
        const acc = await api.get<{ current_balance: number }>("/points");
        setPoints(acc.current_balance);
      } catch {
        /* 积分失败不阻塞页面 */
      }
    })();
  }, []);

  // 客户端搜索过滤（后端暂无搜索接口）
  const visibleCards = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    if (!kw) return cards;
    return cards.filter((c) => c.title.toLowerCase().includes(kw) || c.author.toLowerCase().includes(kw));
  }, [cards, keyword]);

  return (
    <div data-viewport-mode="app-shell" className="flex flex-col h-screen w-screen overflow-hidden">
      {/* 聚光背景 */}
      <div className="spotlight-overlay" />

      {/* 主行：侧边栏 + 内容 */}
      <div className="flex flex-1 min-h-0 min-w-0">
        {/* ===== 左侧 72px 侧边栏（照搬设计稿，发现 active） ===== */}
        <aside
          id="sidebar-nav"
          className="flex flex-col items-center py-3 gap-1 flex-shrink-0 z-10"
          style={{ width: 72, background: "var(--luxreal-sidebar)", borderRight: "1px solid var(--luxreal-border)" }}
        >
          {/* Logo */}
          <div className="flex items-center justify-center flex-shrink-0" style={{ height: 56, marginBottom: 4 }}>
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M6 4H22V8H10V24H6V4Z" fill="white" />
              <path d="M14 12H22V16H18V24H14V12Z" fill="white" opacity="0.7" />
            </svg>
          </div>

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 中心导航（发现 active） */}
          <nav className="flex flex-col items-center gap-2 flex-shrink-0">
            <Link to="/" className="luxreal-nav-item" title="创作">
              <Sparkles size={22} />
              <span>创作</span>
            </Link>
            <Link to="/feed" className="luxreal-nav-item active" title="发现">
              <Compass size={22} />
              <span>发现</span>
            </Link>
            <Link to="/projects" className="luxreal-nav-item" title="作品">
              <Folder size={22} />
              <span>作品</span>
            </Link>
          </nav>

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 头像（点击开关头像资料面板，LuxReal 对齐版 pp-panel） */}
          <div className="pp-anchor" style={{ marginBottom: 4 }}>
            <div
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

          {/* 头像资料面板已内置于 ProfilePanel（上方 pp-anchor 内） */}
        </aside>

        {/* ===== 主内容区 ===== */}
        <div className="flex-1 flex flex-col min-w-0 min-h-0">
          {/* 顶栏（h-14）：标题 + 搜索框 */}
          <header className="h-14 flex items-center px-6 flex-shrink-0 z-10" style={{ background: "transparent" }}>
            <h1 className="text-xl font-semibold text-white mr-8">发现</h1>
            <div className="search-wrapper" style={{ width: "24rem" }}>
              <Search size={16} className="search-icon" />
              <input
                type="text"
                className="search-input"
                placeholder="搜索作品、创作者、风格..."
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
              />
            </div>
          </header>

          {/* 滚动内容区 */}
          <main data-scroll-region="primary" className="overflow-y-auto flex-1 p-6">
            {loading ? (
              <div className="flex items-center justify-center py-24 gap-2" style={{ color: "var(--luxreal-muted-foreground)" }}>
                <Loader2 size={16} className="animate-spin" />
                <span className="text-sm">加载中...</span>
              </div>
            ) : (
              <>
                {/* 瀑布流网格 */}
                <div className="masonry-grid">
                  {visibleCards.map((c, idx) => (
                    <div key={`${c.title}-${idx}`} className="discover-card">
                      <div className="card-thumb">
                        <div className={`card-thumb-gradient thumb-${c.thumb}`} style={{ height: c.height }} />
                        <div className="card-overlay" />
                        <div className="card-badge-video">
                          {c.kind === "video" ? <Video size={12} /> : <Image size={12} />}
                          <span>{c.kind === "video" ? "AI视频" : "AI图片"}</span>
                        </div>
                        {c.kind === "video" && c.duration && <div className="card-duration">{c.duration}</div>}
                        <div className="card-play-btn">
                          <Play size={20} />
                        </div>
                      </div>
                      <div className="card-info">
                        <div className="card-title">{c.title}</div>
                        <div className="card-meta">
                          <div className="card-avatar" style={{ background: c.avatar }} />
                          <span className="card-author">{c.author}</span>
                          <span className="card-stat"><Heart size={12} />{c.likes}</span>
                          <span className="card-stat"><Eye size={12} />{c.views}</span>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>

                {/* 底部加载指示（对齐设计稿） */}
                {visibleCards.length > 0 && (
                  <div className="flex items-center justify-center py-8 gap-2" style={{ color: "var(--luxreal-muted-foreground)" }}>
                    <Loader2 size={16} className="animate-spin" />
                    <span className="text-sm">加载更多精彩内容...</span>
                  </div>
                )}
                {visibleCards.length === 0 && (
                  <div className="flex items-center justify-center py-24 text-sm" style={{ color: "var(--luxreal-muted-foreground)" }}>
                    没有找到相关内容
                  </div>
                )}
              </>
            )}
          </main>
        </div>
      </div>
    </div>
  );
}
