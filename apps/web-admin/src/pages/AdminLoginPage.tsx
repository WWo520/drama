// AdminLoginPage：迁移自 admin-login.html（DOM 1:1），真接口 POST /auth/login。
import { useMemo, useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { api } from "@duanju/api-client";
import {
  User,
  Lock,
  Eye,
  EyeOff,
  ArrowRight,
  ShieldAlert,
  LayoutDashboard,
  Film,
  Users,
  ShieldCheck,
  Video,
  Loader2,
} from "lucide-react";

import { getToken, setSession, toast, errMessage, type AdminUser } from "../lib";
import "./AdminLoginPage.css";

// LoginResp 对齐 auth/dto.go（api-client 未含 auth 模块，本地声明）
interface LoginResp {
  access_token: string;
  expires_in: number;
  token_type: string;
  user: {
    user_id: string;
    email: string;
    phone: string;
    nickname: string;
    avatar_url: string;
  };
}

// 左侧特性卡（设计稿 4 项）
const FEATURES = [
  { icon: LayoutDashboard, name: "数据看板", desc: "实时监控核心指标" },
  { icon: Film, name: "内容管理", desc: "全生命周期管理" },
  { icon: Users, name: "用户运营", desc: "精细化用户管理" },
  { icon: ShieldCheck, name: "权限控制", desc: "多级角色权限体系" },
];

export default function AdminLoginPage() {
  const navigate = useNavigate();
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [showPwd, setShowPwd] = useState(false);
  const [loading, setLoading] = useState(false);
  const [shakeTarget, setShakeTarget] = useState<"" | "account" | "password">("");

  // 粒子星点（设计稿 createParticles：40 个随机粒子）
  const particles = useMemo(
    () =>
      Array.from({ length: 40 }, (_, i) => ({
        key: i,
        left: `${Math.random() * 100}%`,
        top: `${Math.random() * 100}%`,
        animationDelay: `${Math.random() * 4}s`,
        animationDuration: `${3 + Math.random() * 3}s`,
        width: `${1 + Math.random() * 2}px`,
        height: `${1 + Math.random() * 2}px`,
      })),
    [],
  );

  if (getToken()) {
    return <Navigate to="/admin/dashboard" replace />;
  }

  const shake = (target: "account" | "password") => {
    setShakeTarget(target);
    window.setTimeout(() => setShakeTarget(""), 450);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const acct = account.trim();
    const pwd = password.trim();
    if (!acct) {
      shake("account");
      return;
    }
    if (!pwd) {
      shake("password");
      return;
    }
    setLoading(true);
    try {
      // 后端 LoginReq：email 或 phone 二选一（含 @ 视为邮箱）
      const body = acct.includes("@") ? { email: acct, password: pwd } : { phone: acct, password: pwd };
      const resp = await api.post<LoginResp>("/auth/login", body);
      const u = resp.user;
      const sessionUser: AdminUser = {
        user_id: u.user_id,
        nickname: u.nickname,
        email: u.email,
        avatar_url: u.avatar_url,
      };
      setSession(resp.access_token, sessionUser);
      toast("登录成功", "success");
      navigate("/admin/dashboard", { replace: true });
    } catch (err) {
      toast(errMessage(err, "登录失败，请检查账号密码"), "error");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-wrapper">
      {/* Left Panel: Brand Showcase */}
      <div className="brand-panel">
        {/* Background Effects */}
        <div className="grid-pattern"></div>
        <div className="orb orb-amber-1"></div>
        <div className="orb orb-amber-2"></div>
        <div className="orb orb-deep"></div>
        <div className="particles">
          {particles.map((p) => (
            <div key={p.key} className="particle" style={p} />
          ))}
        </div>

        {/* Geometric Elements */}
        <div className="geo-element geo-ring"></div>
        <div className="geo-element geo-hexagon">
          <svg viewBox="0 0 100 115">
            <polygon points="50,2 95,28 95,87 50,113 5,87 5,28" />
            <polygon points="50,18 82,37 82,78 50,97 18,78 18,37" opacity="0.5" />
          </svg>
        </div>
        <div className="geo-lines">
          <svg viewBox="0 0 180 120">
            <line x1="0" y1="30" x2="120" y2="30" />
            <line x1="0" y1="60" x2="180" y2="60" />
            <line x1="0" y1="90" x2="150" y2="90" />
            <line x1="0" y1="120" x2="100" y2="120" />
            <circle cx="120" cy="30" r="3" fill="#fbbf24" />
            <circle cx="180" cy="60" r="3" fill="#fbbf24" />
            <circle cx="150" cy="90" r="3" fill="#fbbf24" />
            <circle cx="100" cy="120" r="3" fill="#fbbf24" />
          </svg>
        </div>

        {/* Brand Header */}
        <div className="brand-header">
          <div className="brand-logo-mark">
            <Video />
          </div>
          <span className="brand-name">Empty Broken</span>
          <span className="brand-badge">Admin</span>
        </div>

        {/* Brand Hero */}
        <div className="brand-hero">
          <div className="hero-label">
            <span className="hero-label-dot"></span>
            Empty Broken · 管理系统
          </div>
          <h1 className="hero-title">
            智能创作
            <br />
            <span className="highlight">高效运营</span>
          </h1>
          <p className="hero-subtitle">
            一站式 Empty Broken 内容管理后台，支持剧本审核、作品管理、数据分析、用户运营等全链路功能，助力平台高效运转。
          </p>

          <div className="feature-grid">
            {FEATURES.map((f) => {
              const Icon = f.icon;
              return (
                <div key={f.name} className="feature-card">
                  <div className="feature-icon">
                    <Icon />
                  </div>
                  <div className="feature-name">{f.name}</div>
                  <div className="feature-desc">{f.desc}</div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Brand Footer */}
        <div className="brand-footer">
          <span>&copy; 2024 Empty Broken. All rights reserved.</span>
          <div className="brand-footer-links">
            <a href="#">服务条款</a>
            <a href="#">隐私政策</a>
            <a href="#">帮助中心</a>
          </div>
        </div>
      </div>

      {/* Right Panel: Login Form */}
      <div className="login-panel">
        <div className="login-panel-bg"></div>

        <div className="login-card">
          <div className="login-header">
            <h2 className="login-title">管理后台</h2>
            <p className="login-subtitle">Empty Broken 管理后台</p>
          </div>

          <form onSubmit={submit} noValidate>
            <div className="form-group">
              <label className="form-label">账号 / 邮箱</label>
              <div className={`input-wrapper${shakeTarget === "account" ? " shake" : ""}`}>
                <User className="input-icon" />
                <input
                  type="text"
                  className="admin-input"
                  placeholder="请输入管理员账号或邮箱"
                  autoComplete="username"
                  value={account}
                  onChange={(e) => setAccount(e.target.value)}
                />
                <div className="input-focus-line"></div>
              </div>
            </div>

            <div className="form-group">
              <label className="form-label">密码</label>
              <div className={`input-wrapper${shakeTarget === "password" ? " shake" : ""}`}>
                <Lock className="input-icon" />
                <input
                  type={showPwd ? "text" : "password"}
                  className="admin-input"
                  placeholder="请输入登录密码"
                  autoComplete="current-password"
                  style={{ paddingRight: 48 }}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                <div className="input-suffix">
                  <button
                    type="button"
                    className="toggle-password"
                    aria-label="切换密码可见性"
                    onClick={() => setShowPwd((v) => !v)}
                  >
                    {showPwd ? <EyeOff /> : <Eye />}
                  </button>
                </div>
                <div className="input-focus-line"></div>
              </div>
            </div>

            <div className="form-options">
              <a href="#" className="forgot-password">
                忘记密码？
              </a>
            </div>

            <button type="submit" className="login-btn" disabled={loading}>
              {loading ? (
                <>
                  <Loader2 className="spin" /> 登录中...
                </>
              ) : (
                <>
                  <span>登 录</span>
                  <ArrowRight />
                </>
              )}
            </button>
          </form>

          <div className="login-divider">
            <div className="login-divider-line"></div>
            <span className="login-divider-text">安全提示</span>
            <div className="login-divider-line"></div>
          </div>

          <div className="admin-notice">
            <div className="admin-notice-icon">
              <ShieldAlert />
            </div>
            <div className="admin-notice-text">
              本系统仅限授权管理员访问，所有操作均被记录。 如遇登录问题，请联系 <strong>技术支持团队</strong>。
            </div>
          </div>

          <div className="card-footer">
            返回 <a href="http://localhost:5173">用户端首页</a>
          </div>
        </div>
      </div>
    </div>
  );
}
