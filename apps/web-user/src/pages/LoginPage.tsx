// 登录/注册页（S1-F01）：影视感暗夜基调 + 玻璃卡片 + 聚光灯金 CTA
// 设计系统：design-system/duanju/MASTER.md（Theater/Cinema + Liquid Glass）
import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Clapperboard } from "lucide-react";

import { api, ApiError, errorMessage } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";

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
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [nickname, setNickname] = useState("");
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const setAuth = useAuthStore((s) => s.setAuth);

  const account = email || phone;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!account || !password) return;
    setLoading(true);
    try {
      const resp =
        mode === "login"
          ? await api.post<AuthResp>("/auth/login", {
              email: email || undefined,
              phone: phone || undefined,
              password,
            })
          : await api.post<AuthResp>("/auth/register", {
              email: email || undefined,
              phone: phone || undefined,
              password,
              nickname: nickname || "创作者",
            });
      localStorage.setItem("access_token", resp.access_token);
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
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-cinema-bg px-4">
      {/* 氛围光斑（克制：静态渐变，避免花哨动画） */}
      <div
        aria-hidden
        className="pointer-events-none absolute -top-32 left-1/4 h-96 w-96 rounded-full opacity-20 blur-3xl"
        style={{ background: "radial-gradient(circle, #6366F1 0%, transparent 70%)" }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute -bottom-40 right-1/4 h-96 w-96 rounded-full opacity-15 blur-3xl"
        style={{ background: "radial-gradient(circle, #CA8A04 0%, transparent 70%)" }}
      />

      <div className="glass-card relative z-10 w-full max-w-md rounded-2xl p-8 shadow-2xl">
        {/* 品牌区 */}
        <div className="mb-8 flex flex-col items-center">
          <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-primary-deep">
            <Clapperboard className="h-6 w-6 text-white" aria-hidden />
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-foreground">短剧 AI 创作平台</h1>
          <p className="mt-1 text-sm text-cinema-muted-fg">文字剧本 → 可商用短剧 MP4</p>
        </div>

        {/* 模式切换 */}
        <div className="mb-6 flex rounded-lg bg-cinema-muted p-1" role="tablist" aria-label="登录或注册">
          {(["login", "register"] as const).map((m) => (
            <button
              key={m}
              type="button"
              role="tab"
              aria-selected={mode === m}
              onClick={() => setMode(m)}
              className={`flex-1 cursor-pointer rounded-md py-2 text-sm transition-colors duration-200 ${
                mode === m ? "bg-cinema-elevated font-medium text-foreground" : "text-cinema-muted-fg hover:text-foreground"
              }`}
            >
              {m === "login" ? "登录" : "注册"}
            </button>
          ))}
        </div>

        <form onSubmit={submit} className="space-y-4" noValidate>
          {mode === "register" && (
            <label className="block">
              <span className="mb-1.5 block text-sm text-cinema-muted-fg">昵称</span>
              <input
                value={nickname}
                onChange={(e) => setNickname(e.target.value)}
                placeholder="你的显示名"
                minLength={2}
                maxLength={64}
                className="w-full rounded-lg border border-cinema-border bg-cinema-card px-4 py-3 text-foreground placeholder-cinema-muted-fg/60 outline-none transition-colors duration-200 focus:border-primary"
              />
            </label>
          )}
          <label className="block">
            <span className="mb-1.5 block text-sm text-cinema-muted-fg">邮箱</span>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              type="email"
              autoComplete="email"
              className="w-full rounded-lg border border-cinema-border bg-cinema-card px-4 py-3 text-foreground placeholder-cinema-muted-fg/60 outline-none transition-colors duration-200 focus:border-primary"
            />
          </label>
          <label className="block">
            <span className="mb-1.5 block text-sm text-cinema-muted-fg">手机号（选填）</span>
            <input
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              placeholder="13800000000"
              autoComplete="tel"
              className="w-full rounded-lg border border-cinema-border bg-cinema-card px-4 py-3 text-foreground placeholder-cinema-muted-fg/60 outline-none transition-colors duration-200 focus:border-primary"
            />
          </label>
          <label className="block">
            <span className="mb-1.5 block text-sm text-cinema-muted-fg">密码</span>
            <input
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="至少 8 位"
              type="password"
              minLength={8}
              maxLength={64}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              className="w-full rounded-lg border border-cinema-border bg-cinema-card px-4 py-3 text-foreground placeholder-cinema-muted-fg/60 outline-none transition-colors duration-200 focus:border-primary"
            />
          </label>

          <button
            type="submit"
            disabled={loading || !account || password.length < 8}
            className="mt-2 w-full cursor-pointer rounded-lg bg-cinema-gold py-3 font-medium text-cinema-bg transition-colors duration-200 hover:bg-cinema-gold-soft disabled:cursor-not-allowed disabled:opacity-50"
          >
            {loading ? "处理中…" : mode === "login" ? "进入工作台" : "创建账号"}
          </button>
        </form>

        <p className="mt-6 text-center text-xs text-cinema-muted-fg/70">
          未点「生成」不扣费 · 注册即赠初始积分
        </p>
      </div>
    </div>
  );
}
