// AppShell —— 72px 主导航侧边栏（批次 2 从 HomePage 提取的共用布局，设计稿 sidebar-nav 1:1）
// 样式：shell.css（luxreal-nav-item / sidebar-avatar / pp-panel 等）
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Compass, Folder, Sparkles } from "lucide-react";

import { api } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
import "./shell.css";

export type NavKey = "create" | "discover" | "works";

export default function AppShell({
  active,
  children,
}: {
  active: NavKey;
  children: React.ReactNode;
}) {
  const { user } = useAuthStore();
  const avatarUrl = useAvatarUrl(user?.avatar_url);
  const [profileOpen, setProfileOpen] = useState(false);
  const [points, setPoints] = useState<number | null>(null);

  // 积分余额（侧边栏真实数据）
  useEffect(() => {
    (async () => {
      try {
        const acc = await api.get<{ current_balance: number }>("/points");
        setPoints(acc.current_balance);
      } catch {
        /* 积分失败不阻塞布局 */
      }
    })();
  }, []);

  const initial = (user?.nickname || "Y").slice(0, 1).toUpperCase();

  return (
    <div className="flex h-screen w-screen overflow-hidden">
      {/* Column 1: Far Left Main Nav (72px) */}
      <aside
        id="sidebar-nav"
        className="flex flex-col items-center py-3 gap-1 flex-shrink-0 z-10"
        style={{ width: 72, background: "var(--luxreal-sidebar)", borderRight: "1px solid var(--luxreal-border)" }}
      >
        {/* Top: Logo mark */}
        <div className="flex items-center justify-center flex-shrink-0" style={{ height: 56, marginBottom: 4 }}>
          <svg width="28" height="28" viewBox="0 0 28 28" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M6 4H22V8H10V24H6V4Z" fill="white" />
            <path d="M14 12H22V16H18V24H14V12Z" fill="white" opacity="0.7" />
          </svg>
        </div>

        {/* Spacer to push main nav down */}
        <div className="flex-1" style={{ minHeight: 20 }} />

        {/* Center nav - vertically centered */}
        <nav className="flex flex-col items-center gap-2 flex-shrink-0">
          <Link to="/" className={`luxreal-nav-item ${active === "create" ? "active" : ""}`} title="创作">
            <Sparkles size={22} />
            <span>创作</span>
          </Link>
          <Link to="/feed" className={`luxreal-nav-item ${active === "discover" ? "active" : ""}`} title="发现">
            <Compass size={22} />
            <span>发现</span>
          </Link>
          <Link to="/projects" className={`luxreal-nav-item ${active === "works" ? "active" : ""}`} title="作品">
            <Folder size={22} />
            <span>作品</span>
          </Link>
        </nav>

        {/* Spacer to push lower nav down */}
        <div className="flex-1" style={{ minHeight: 20 }} />

        {/* Avatar（点击开关头像资料面板，LuxReal 对齐版 pp-panel） */}
        <div className="pp-anchor" style={{ marginBottom: 4 }}>
          <div
            id="sidebarAvatar"
            className="sidebar-avatar"
            onClick={(e) => {
              e.stopPropagation();
              setProfileOpen((v) => !v);
            }}
          >
            {initial}
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

        {/* Credits number */}
        <span className="sidebar-credits">{points === null ? "…" : points.toLocaleString()}</span>
        {/* Recharge link */}
        <Link to="/points" className="sidebar-recharge" title="充值">充值</Link>
      </aside>

      {/* 主内容区 */}
      <div className="flex-1 flex flex-col min-w-0 min-h-0">{children}</div>
    </div>
  );
}
