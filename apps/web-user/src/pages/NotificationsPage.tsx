// 消息中心（批次4：设计稿 notifications.html 迁移，DOM 1:1）
// CSS 原样提取 → src/pages/notifications.css（shell 类同源 + notification-item 全套）
// 接口对接：GET /notifications 列表、PUT 单条已读、PUT 全部已读（05 TAG 8）
// 回退策略：接口失败或空列表时回退设计稿 8 条静态消息（视觉保真，注释标注）
// 入口说明：设计稿侧边栏无消息入口 → 头像面板增加「消息中心」项（最小适配，各页同步）
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import {
  Bell, CheckCheck, CheckCircle, Compass, Film, Folder,
  Loader2, Sparkles,
} from "lucide-react";

import { api, notificationsApi, type NotificationItem } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
import "./notifications.css";

// 视图模型
interface NoticeVM {
  id: number | null; // 真数据 notification_id；回退静态项为 null
  tab: "system" | "interaction";
  icon: "system" | "success" | "video";
  title: string;
  preview: string;
  time: string;
  unread: boolean;
}

// 设计稿静态消息（API 无数据时的视觉回退；数据源 notifications.html L92-206）
const FALLBACK_NOTICES: NoticeVM[] = [
  { id: null, tab: "system", icon: "system", title: "系统维护通知", preview: "将于本周六凌晨 2:00-4:00 进行系统升级维护，期间部分功能可能无法使用。", time: "10 分钟前", unread: true },
  { id: null, tab: "system", icon: "success", title: "视频生成完成", preview: "「都市夜归人」第 3 集已生成完毕，快去查看效果吧。", time: "1 小时前", unread: true },
  { id: null, tab: "interaction", icon: "video", title: "新的点赞", preview: "你的作品「迷雾追踪」获得了 28 个新赞，继续加油！", time: "3 小时前", unread: true },
  { id: null, tab: "system", icon: "system", title: "积分充值成功", preview: "你已成功充值 5,000 积分，当前余额 2,580 积分。", time: "昨天", unread: false },
  { id: null, tab: "interaction", icon: "video", title: "新评论", preview: "光影诗人 评论了你的作品：「画面质感很棒，期待更新！」", time: "昨天", unread: false },
  { id: null, tab: "system", icon: "success", title: "审核通过", preview: "你提交的作品「逆光而行」已通过审核，正式发布。", time: "2 天前", unread: false },
  { id: null, tab: "system", icon: "system", title: "新功能上线", preview: "AI 角色一致性功能已上线，快去创作页体验新功能吧。", time: "3 天前", unread: false },
  { id: null, tab: "interaction", icon: "video", title: "新增粉丝", preview: "有 5 位新用户关注了你，感谢支持。", time: "5 天前", unread: false },
];

// 相对时间（对齐设计稿文案风格）
function relTime(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "";
  const diff = Date.now() - t;
  const min = Math.floor(diff / 60000);
  if (min < 1) return "刚刚";
  if (min < 60) return `${min} 分钟前`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour} 小时前`;
  const day = Math.floor(hour / 24);
  if (day === 1) return "昨天";
  if (day < 7) return `${day} 天前`;
  const d = new Date(t);
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日`;
}

// 真接口数据 → 视图模型（tab：INTERACTION→互动，其余→系统；icon：按类型映射设计稿三色图标）
function toVM(n: NotificationItem): NoticeVM {
  return {
    id: n.notification_id,
    tab: n.type === "INTERACTION" ? "interaction" : "system",
    icon: n.type === "INTERACTION" ? "video" : n.type === "CREATION" ? "success" : "system",
    title: n.title,
    preview: n.content,
    time: relTime(n.created_at),
    unread: !n.is_read,
  };
}

export default function NotificationsPage() {
  const { user } = useAuthStore();
  const avatarUrl = useAvatarUrl(user?.avatar_url);

  const [notices, setNotices] = useState<NoticeVM[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<"all" | "system" | "interaction">("all");
  const [points, setPoints] = useState<number | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);

  // 消息列表：GET /notifications；失败或空时回退设计稿静态项（视觉保真）
  useEffect(() => {
    (async () => {
      try {
        const data = await notificationsApi.list(1, 50);
        const items = data.items ?? [];
        setNotices(items.length > 0 ? items.map(toVM) : FALLBACK_NOTICES);
      } catch {
        setNotices(FALLBACK_NOTICES);
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

  // 单条已读（点击消息）
  const handleRead = async (n: NoticeVM) => {
    if (!n.unread) return;
    setNotices((list) => list.map((x) => (x === n ? { ...x, unread: false } : x)));
    if (n.id !== null) {
      try {
        await notificationsApi.markRead(n.id);
      } catch {
        /* 已读标记失败不回滚 UI */
      }
    }
  };

  // 全部已读
  const handleMarkAll = async () => {
    setNotices((list) => list.map((x) => ({ ...x, unread: false })));
    try {
      await notificationsApi.markAllRead();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "操作失败");
    }
  };

  // Tab 客户端过滤（对齐设计稿 JS）
  const visible = useMemo(
    () => (tab === "all" ? notices : notices.filter((n) => n.tab === tab)),
    [notices, tab],
  );

  const hasUnread = notices.some((n) => n.unread);

  return (
    <div data-viewport-mode="app-shell" className="flex flex-col h-screen w-screen overflow-hidden">
      {/* 主行：侧边栏 + 内容 */}
      <div className="flex flex-1 min-h-0 min-w-0">
        {/* ===== 左侧 72px 侧边栏（照搬设计稿，三导航均无 active） ===== */}
        <aside
          id="sidebar-nav"
          className="flex flex-col items-center py-4 gap-2 flex-shrink-0 z-10"
          style={{ width: 72, background: "var(--luxreal-sidebar)", borderRight: "1px solid var(--luxreal-border)" }}
        >
          {/* Logo */}
          <div className="flex items-center justify-center flex-shrink-0" style={{ height: 56, marginBottom: 4 }}>
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M6 4H22V8H10V24H6V4Z" fill="white" />
              <path d="M14 12H22V16H18V24H14V12Z" fill="white" opacity="0.7" />
            </svg>
          </div>

          <div className="flex-1" style={{ minHeight: 16 }} />

          {/* 中心导航（消息页无 active，对齐设计稿） */}
          <nav className="flex flex-col items-center gap-2 flex-shrink-0">
            <Link to="/" className="luxreal-nav-item" title="创作">
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

          {/* Divider（设计稿本段独有） */}
          <div className="w-8 h-px" style={{ background: "var(--luxreal-border)", margin: "8px 0" }} />

          <div className="flex-1" style={{ minHeight: 16 }} />

          {/* 头像（点击展开面板，LuxReal 对齐版 ProfilePanel） */}
          <div className="pp-anchor" style={{ marginBottom: 6 }}>
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
        </aside>

        {/* ===== 主内容区 ===== */}
        <main className="flex-1 flex flex-col h-full overflow-hidden min-w-0">
          {/* Page Header (sticky) */}
          <header
            className="flex items-center justify-between flex-shrink-0 px-8"
            style={{ height: 60, borderBottom: "1px solid var(--luxreal-border)", background: "var(--luxreal-background)" }}
          >
            <h1 className="text-[18px] font-semibold m-0" style={{ color: "var(--luxreal-foreground)" }}>
              消息中心
            </h1>
            <button id="markAllReadBtn" className="luxreal-btn-ghost" disabled={!hasUnread} onClick={() => void handleMarkAll()}>
              <CheckCheck size={16} />
              <span>全部已读</span>
            </button>
          </header>

          {/* Tabs */}
          <div className="flex-shrink-0 px-8 pt-5 pb-4">
            <div className="segmented-group" id="notificationTabs">
              <button className={`segmented-btn${tab === "all" ? " active" : ""}`} onClick={() => setTab("all")}>全部</button>
              <button className={`segmented-btn${tab === "system" ? " active" : ""}`} onClick={() => setTab("system")}>系统</button>
              <button className={`segmented-btn${tab === "interaction" ? " active" : ""}`} onClick={() => setTab("interaction")}>互动</button>
            </div>
          </div>

          {/* Message List */}
          <div className="flex-1 overflow-y-auto px-8 pb-8">
            {loading ? (
              <div className="flex items-center justify-center py-24 gap-2" style={{ color: "var(--luxreal-muted-foreground)" }}>
                <Loader2 size={16} className="animate-spin" />
                <span className="text-sm">加载中...</span>
              </div>
            ) : (
              <div id="messageList" className="flex flex-col">
                {visible.map((n, idx) => (
                  <div
                    key={n.id ?? `fb-${idx}`}
                    className="notification-item"
                    data-type={n.tab}
                    data-unread={n.unread}
                    onClick={() => void handleRead(n)}
                  >
                    <div className={`notification-icon icon-${n.icon}`}>
                      {n.icon === "system" ? <Bell size={20} /> : n.icon === "success" ? <CheckCircle size={20} /> : <Film size={20} />}
                    </div>
                    <div className="notification-content">
                      <div className="notification-title-row">
                        <span className="notification-title">{n.title}</span>
                        {n.unread && <span className="unread-dot" />}
                      </div>
                      <p className="notification-preview">{n.preview}</p>
                      <span className="notification-time">{n.time}</span>
                    </div>
                  </div>
                ))}
                {visible.length === 0 && (
                  <div className="flex items-center justify-center py-24 text-sm" style={{ color: "var(--luxreal-muted-foreground)" }}>
                    暂无消息
                  </div>
                )}
              </div>
            )}
          </div>
        </main>
      </div>
    </div>
  );
}
