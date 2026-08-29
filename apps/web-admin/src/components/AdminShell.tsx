// 管理端公共 shell：240px sidebar + 60px topbar（admin-dashboard.html DOM 1:1）。
import { useEffect, useRef, useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  Users,
  ShieldCheck,
  Film,
  Wallet,
  Cpu,
  Settings,
  Home,
  ChevronRight,
  ChevronDown,
  ChevronUp,
  Search,
  Bell,
  LogOut,
} from "lucide-react";

import { clearSession, getSessionUser } from "../lib";

// 菜单项（设计稿 7 项：无设计稿的页面指向占位页）
const MENU = [
  { to: "/admin/dashboard", label: "工作台", icon: LayoutDashboard },
  { to: "/admin/users", label: "用户管理", icon: Users },
  { to: "/admin/audit", label: "内容审核", icon: ShieldCheck, badge: "12", badgeCls: "bg-red-500/20 text-red-400" },
  { to: "/admin/works", label: "作品管理", icon: Film },
  { to: "/admin/billing", label: "计费财务", icon: Wallet },
  { to: "/admin/tasks", label: "算力任务", icon: Cpu, badge: "34", badgeCls: "bg-amber-gold/20 text-amber-gold" },
  { to: "/admin/settings", label: "平台配置", icon: Settings },
];

// 面包屑标题（topbar 当前页）
const TITLES: Record<string, string> = {
  "/admin/dashboard": "工作台",
  "/admin/users": "用户管理",
  "/admin/audit": "内容审核",
  "/admin/works": "作品管理",
  "/admin/billing": "计费财务",
  "/admin/tasks": "算力任务",
  "/admin/settings": "平台配置",
};

export default function AdminShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const user = getSessionUser();
  const displayName = user?.nickname || "Admin";
  const initial = displayName.slice(0, 1).toUpperCase() || "A";

  const logout = () => {
    clearSession();
    navigate("/admin/login", { replace: true });
  };

  // 头像下拉小弹窗：点击头像开关，点击外部关闭
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!menuOpen) return;
    const onDocClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener("click", onDocClick);
    return () => document.removeEventListener("click", onDocClick);
  }, [menuOpen]);

  return (
    <div className="flex min-h-screen">
      {/* ========== SIDEBAR ========== */}
      <aside className="w-[240px] bg-[#0D0D0D] border-r border-[#2A2A3E] flex flex-col flex-shrink-0 fixed h-full z-20">
        {/* Logo */}
        <div className="h-[60px] flex items-center px-5 border-b border-[#2A2A3E]">
          <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-amber-gold to-amber-dim flex items-center justify-center mr-3">
            <svg className="w-5 h-5 text-[#0B0B1A]" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" />
            </svg>
          </div>
          <span className="text-white font-bold text-lg tracking-wide">
            导演台 <span className="text-amber-gold">Admin</span>
          </span>
        </div>

        {/* Menu */}
        <nav className="flex-1 py-4 overflow-y-auto">
          <div className="px-3 mb-2 text-[11px] text-text-muted uppercase tracking-wider font-medium">主菜单</div>
          {MENU.map((item) => {
            const active = location.pathname === item.to;
            const Icon = item.icon;
            return (
              <Link
                key={item.to}
                to={item.to}
                className={`menu-item flex items-center px-5 py-3 text-sm cursor-pointer ${
                  active ? "active" : "text-text-secondary"
                }`}
              >
                <Icon size={18} className="w-[18px] h-[18px] mr-3" />
                <span>{item.label}</span>
                {item.badge && (
                  <span className={`ml-auto text-[10px] px-2 py-0.5 rounded-full ${item.badgeCls}`}>
                    {item.badge}
                  </span>
                )}
              </Link>
            );
          })}
        </nav>

        {/* Bottom user */}
        <div className="p-4 border-t border-[#2A2A3E]">
          <div className="flex items-center">
            <div className="w-9 h-9 rounded-full bg-gradient-to-br from-amber-gold to-amber-dim flex items-center justify-center text-[#0B0B1A] font-bold text-sm">
              {initial}
            </div>
            <div className="ml-3 flex-1 min-w-0">
              <div className="text-sm font-medium text-white truncate">{displayName}</div>
              <div className="text-xs text-text-muted truncate">超级管理员</div>
            </div>
            <button onClick={logout} title="退出登录" className="text-text-muted hover:text-amber-gold transition-colors">
              <ChevronUp size={16} />
            </button>
          </div>
        </div>
      </aside>

      {/* ========== MAIN WRAPPER ========== */}
      <div className="flex-1 ml-[240px] flex flex-col min-w-0">
        {/* ========== TOP BAR ========== */}
        <header className="h-[60px] bg-[#0D0D0D]/80 backdrop-blur-sm border-b border-[#2A2A3E] flex items-center justify-between px-6 sticky top-0 z-10">
          {/* Left: breadcrumb */}
          <div className="flex items-center">
            <Home size={16} className="w-4 h-4 text-text-muted mr-2" />
            <span className="text-text-muted text-sm">管理后台</span>
            <ChevronRight size={16} className="w-4 h-4 text-text-muted mx-2" />
            <span className="text-white text-sm font-medium">{TITLES[location.pathname] ?? "管理后台"}</span>
          </div>

          {/* Right: search + notif + avatar */}
          <div className="flex items-center gap-4">
            {/* Search */}
            <div className="relative">
              <Search size={16} className="w-4 h-4 text-text-muted absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder="搜索..."
                className="bg-bg-card border border-[#2A2A3E] rounded-lg pl-9 pr-4 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 w-56 transition-colors"
              />
            </div>

            {/* Notification */}
            <button className="relative w-9 h-9 rounded-lg bg-bg-card border border-[#2A2A3E] flex items-center justify-center hover:border-[#3A3A4E] transition-colors">
              <Bell size={16} className="w-4 h-4 text-text-secondary" />
              <span className="notif-dot"></span>
            </button>

            {/* Divider */}
            <div className="w-px h-6 bg-[#2A2A3E]"></div>

            {/* Avatar（点击弹出头像附近的小弹窗） */}
            <div className="relative" ref={menuRef}>
              <div className="flex items-center cursor-pointer" onClick={() => setMenuOpen((v) => !v)}>
                <div className="w-8 h-8 rounded-full bg-gradient-to-br from-amber-gold to-amber-dim flex items-center justify-center text-[#0B0B1A] font-bold text-sm">
                  {initial}
                </div>
                <span className="ml-2 text-sm text-white">{displayName}</span>
                <ChevronDown
                  size={16}
                  className={`w-4 h-4 text-text-muted ml-1 transition-transform ${menuOpen ? "rotate-180" : ""}`}
                />
              </div>
              {menuOpen && (
                <div className="absolute right-0 top-[calc(100%+10px)] w-56 bg-bg-card border border-[#2A2A3E] rounded-xl z-50 shadow-[0_12px_40px_rgba(0,0,0,0.6)]">
                  {/* 小三角指向头像圆心（头像在触发区左端，故三角靠弹窗左侧） */}
                  <div className="absolute -top-[5px] right-[106px] w-2.5 h-2.5 bg-bg-card border-l border-t border-[#2A2A3E] rotate-45"></div>
                  <div className="px-4 py-3 border-b border-[#2A2A3E] flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-gradient-to-br from-amber-gold to-amber-dim flex items-center justify-center text-[#0B0B1A] font-bold text-sm flex-shrink-0">
                      {initial}
                    </div>
                    <div className="min-w-0">
                      <div className="text-sm font-medium text-white truncate">{displayName}</div>
                      <div className="text-xs text-text-muted truncate">超级管理员</div>
                    </div>
                  </div>
                  <button
                    onClick={logout}
                    className="w-full flex items-center gap-2 px-4 py-3 text-sm text-text-secondary hover:text-amber-gold hover:bg-white/5 transition-colors rounded-b-xl"
                  >
                    <LogOut size={15} />
                    退出登录
                  </button>
                </div>
              )}
            </div>
          </div>
        </header>

        {/* ========== MAIN CONTENT ========== */}
        <main className="flex-1 p-6 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
