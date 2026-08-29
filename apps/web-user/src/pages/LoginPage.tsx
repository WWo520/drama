// 登录/注册页 —— 按 luxreal-clone 用户端设计稿 login-register.html 重构（左右分栏 + 动画光斑 + 粒子 + 滑动 Tab + 密码强度条）
// CSS 原样提取自设计稿 <style> 块（648 行）→ src/pages/login.css（铁律①：零修改）
// 接口对接：POST /auth/login、/auth/register（05 - 01 文档 4.1）
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Eye, EyeOff, Lock, ShieldCheck, Smile, User } from "lucide-react";

import { api, ApiError, errorMessage } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import { clearCachedGet } from "../utils/cachedApi";
import "./login.css";

interface AuthResp {
  access_token: string;
  expires_in: number;
  token_type?: string;
  user: {
    user_id: string;
    email: string | null;
    phone: string | null;
    nickname: string;
    avatar_url: string | null;
    vip_level: number;
    default_space_id: string | null;
  };
}

export default function LoginPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [nickname, setNickname] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [agreed, setAgreed] = useState(true);
  const [showLoginPwd, setShowLoginPwd] = useState(false);
  const [showRegPwd, setShowRegPwd] = useState(false);
  const [showConfirmPwd, setShowConfirmPwd] = useState(false);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const setAuth = useAuthStore((s) => s.setAuth);
  const particlesRef = useRef<HTMLDivElement>(null);

  // 页面标题
  useEffect(() => {
    document.title = "登录注册 - 导演台";
  }, []);

  // 浮动粒子（照搬设计稿：35 个，含 p-large/p-blue 变体、随机尺寸 2~7px、透明度 0.3~1.0）
  useEffect(() => {
    const container = particlesRef.current;
    if (!container) return;
    container.innerHTML = "";
    const count = 35;
    const frag = document.createDocumentFragment();
    for (let i = 0; i < count; i++) {
      const p = document.createElement("div");
      p.className = "anim-particle";
      p.style.left = Math.random() * 100 + "%";
      p.style.top = Math.random() * 100 + "%";
      p.style.animationDelay = Math.random() * 8 + "s";
      p.style.animationDuration = 6 + Math.random() * 8 + "s";
      const size = 2 + Math.random() * 5; // 2~7px
      p.style.width = size + "px";
      p.style.height = size + "px";
      p.style.opacity = (0.3 + Math.random() * 0.7).toFixed(2); // 0.3~1.0
      const r = Math.random();
      if (r < 0.15) p.classList.add("p-large");
      else if (r < 0.35) p.classList.add("p-blue");
      frag.appendChild(p);
    }
    container.appendChild(frag);
  }, []);

  // 密码强度（照搬设计稿：长度≥8 + 大小写 + 数字 + 特殊字符，四色四标签）
  const strength = useMemo(() => {
    let score = 0;
    if (password.length >= 8) score++;
    if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score++;
    if (/\d/.test(password)) score++;
    if (/[^a-zA-Z0-9]/.test(password)) score++;
    return score;
  }, [password]);
  const strengthColors = ["#ef4444", "#f59e0b", "#eab308", "#22c55e"];
  const strengthLabels = ["很弱", "较弱", "中等", "很强"];

  // 账号 → 邮箱或手机号拆分（后端契约：email 必须符合邮箱格式；不支持纯用户名）
  const parseAccount = (raw: string): { email?: string; phone?: string } | null => {
    if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw)) return { email: raw };
    if (/^1\d{10}$/.test(raw)) return { phone: raw };
    return null;
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!account || !password) return;
    const parsed = parseAccount(account.trim());
    if (!parsed) {
      toast.error("请输入有效的邮箱或手机号");
      return;
    }
    if (mode === "register") {
      if (nickname.trim().length < 2) {
        toast.error("昵称至少 2 个字符");
        return;
      }
      if (password !== confirmPassword) {
        toast.error("两次输入的密码不一致");
        return;
      }
      if (!agreed) {
        toast.error("请先阅读并同意用户协议");
        return;
      }
    }
    setLoading(true);
    try {
      const resp =
        mode === "login"
          ? await api.post<AuthResp>("/auth/login", { ...parsed, password })
          : await api.post<AuthResp>("/auth/register", { ...parsed, password, nickname: nickname.trim() || "创作者" });
      if (remember) localStorage.setItem("access_token", resp.access_token);
      else sessionStorage.setItem("access_token", resp.access_token);
      clearCachedGet(); // 缓存无用户维度，切换账号登录后防止读到上一账号数据
      setAuth(resp.access_token, {
        user_uuid: resp.user.user_id,
        email: resp.user.email ?? "",
        nickname: resp.user.nickname,
        avatar_url: resp.user.avatar_url ?? "",
        vip_level: resp.user.vip_level,
        default_space_id: Number(resp.user.default_space_id ?? 0),
      });
      toast.success(mode === "login" ? "欢迎回来" : "注册成功，已自动登录");
      navigate("/");
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "网络异常，请稍后重试");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-page">
      {/* 动画背景（视频缺失时用光斑/粒子/网格兜底，设计稿 animated fallback） */}
      <div className="anim-orbs">
        <div className="anim-orb orb-1" />
        <div className="anim-orb orb-2" />
        <div className="anim-orb orb-3" />
        <div className="anim-orb orb-4" />
      </div>
      <div className="anim-particles" ref={particlesRef} />
      <div className="anim-grid" />

      {/* 虚影视频背景（铺满整个界面：blur 柔化 + 低透明 + 渐变遮罩） */}
      <div className="login-video-bg" aria-hidden>
        <video autoPlay muted loop playsInline preload="auto">
          <source src="/media/login-anim.mp4" type="video/mp4" />
        </video>
      </div>

      <div className="video-overlay" />

      {/* ========== 左品牌区 ========== */}
      <div className="brand-section">
        <div className="brand-content">
        <div className="logo">
          <div className="logo-icon">
            <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <polygon points="5 3 19 12 5 21 5 3" />
            </svg>
          </div>
          <span className="logo-text">导演台</span>
        </div>
        <h1 className="hero-title">
          用 AI 重塑
          <br />
          <span className="highlight">短剧创作</span>的未来
        </h1>
        <p className="hero-desc">
          从剧本拆分到视频生成，一站式 AI 短剧生产平台。
          让创意不再受限于技术，每个人都能成为导演。
        </p>
        </div>
      </div>

      {/* ========== 右认证卡 ========== */}
      <div className="auth-section">
        <div className="auth-card">
          {/* Tab 切换（滑动指示器） */}
          <div className="tab-switcher">
            <div className="tab-indicator" style={{ transform: mode === "login" ? "translateX(0)" : "translateX(100%)" }} />
            <button className={`tab-btn ${mode === "login" ? "active" : ""}`} onClick={() => setMode("login")}>
              登录
            </button>
            <button className={`tab-btn ${mode === "register" ? "active" : ""}`} onClick={() => setMode("register")}>
              注册
            </button>
          </div>

          {mode === "login" ? (
            <form className="auth-form active" onSubmit={submit} noValidate>
              <h2 className="form-title">欢迎回来 👋</h2>
              <p className="form-subtitle">登录你的账号继续创作之旅</p>
              <div className="form-group">
                <label className="form-label">账号</label>
                <div className="input-wrapper">
                  <User className="input-icon" size={18} aria-hidden />
                  <input
                    type="text"
                    className="form-input"
                    value={account}
                    onChange={(e) => setAccount(e.target.value)}
                    placeholder="请输入邮箱 / 手机号"
                    autoComplete="username"
                  />
                </div>
              </div>
              <div className="form-group">
                <label className="form-label">密码</label>
                <div className="input-wrapper">
                  <Lock className="input-icon" size={18} aria-hidden />
                  <input
                    type={showLoginPwd ? "text" : "password"}
                    className="form-input"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="请输入密码"
                    autoComplete="current-password"
                  />
                  <button type="button" className="password-toggle" onClick={() => setShowLoginPwd((v) => !v)} aria-label="切换密码可见性">
                    {showLoginPwd ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </div>
              </div>
              <div className="options-row">
                <label className="remember-me">
                  <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} /> 记住我
                </label>
                <a href="#" className="forgot-link">
                  忘记密码？
                </a>
              </div>
              <button type="submit" className="submit-btn" disabled={loading}>
                <span>{loading ? "处理中…" : "登录"}</span>
              </button>
            </form>
          ) : (
            <form className="auth-form active" onSubmit={submit} noValidate>
              <h2 className="form-title">创建账号 ✨</h2>
              <p className="form-subtitle">加入我们，开启你的 AI 创作之旅</p>
              <div className="form-group">
                <label className="form-label">昵称</label>
                <div className="input-wrapper">
                  <Smile className="input-icon" size={18} aria-hidden />
                  <input
                    type="text"
                    className="form-input"
                    value={nickname}
                    onChange={(e) => setNickname(e.target.value)}
                    placeholder="给自己起个名字吧"
                    minLength={2}
                    maxLength={64}
                    autoComplete="nickname"
                  />
                </div>
              </div>
              <div className="form-group">
                <label className="form-label">账号</label>
                <div className="input-wrapper">
                  <User className="input-icon" size={18} aria-hidden />
                  <input
                    type="text"
                    className="form-input"
                    value={account}
                    onChange={(e) => setAccount(e.target.value)}
                    placeholder="邮箱 / 手机号"
                    autoComplete="email"
                  />
                </div>
              </div>
              <div className="form-group">
                <label className="form-label">密码</label>
                <div className="input-wrapper">
                  <Lock className="input-icon" size={18} aria-hidden />
                  <input
                    type={showRegPwd ? "text" : "password"}
                    className="form-input"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="设置 8-20 位密码"
                    minLength={8}
                    maxLength={20}
                    autoComplete="new-password"
                  />
                  <button type="button" className="password-toggle" onClick={() => setShowRegPwd((v) => !v)} aria-label="切换密码可见性">
                    {showRegPwd ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </div>
                <div className="password-strength">
                  <div className="strength-bar">
                    {[1, 2, 3, 4].map((seg) => (
                      <div
                        key={seg}
                        className="strength-segment"
                        style={{ background: seg <= strength ? strengthColors[strength - 1] : "rgba(255, 255, 255, 0.08)" }}
                      />
                    ))}
                  </div>
                  <div className="strength-text">
                    密码强度：{password.length === 0 ? "请输入密码" : strength > 0 ? strengthLabels[strength - 1] : "很弱"}
                  </div>
                </div>
              </div>
              <div className="form-group">
                <label className="form-label">确认密码</label>
                <div className="input-wrapper">
                  <ShieldCheck className="input-icon" size={18} aria-hidden />
                  <input
                    type={showConfirmPwd ? "text" : "password"}
                    className="form-input"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    placeholder="再次输入密码"
                    autoComplete="new-password"
                  />
                  <button type="button" className="password-toggle" onClick={() => setShowConfirmPwd((v) => !v)} aria-label="切换密码可见性">
                    {showConfirmPwd ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </div>
              </div>
              <label className="agreement">
                <input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} />
                <span>
                  我已阅读并同意<a href="#">《用户协议》</a>和<a href="#">《隐私政策》</a>
                </span>
              </label>
              <button type="submit" className="submit-btn" disabled={loading}>
                <span>{loading ? "处理中…" : "创建账号"}</span>
              </button>
            </form>
          )}

          {/* 底部切换链接（照搬设计稿 auth-switch） */}
          <div className="auth-switch">
            {mode === "login" ? (
              <>
                还没有账号？{" "}
                <a href="#" onClick={(e) => { e.preventDefault(); setMode("register"); }}>
                  立即注册
                </a>
              </>
            ) : (
              <>
                已有账号？{" "}
                <a href="#" onClick={(e) => { e.preventDefault(); setMode("login"); }}>
                  立即登录
                </a>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
