// 充值中心（批次4：设计稿 recharge.html 迁移，DOM 1:1）
// CSS 原样提取 → src/pages/recharge.css（modal 体系重命名 .pay-modal-* 避免与 shell.css 冲突）
// 布局：可滚动落地页模式（flex min-h-screen + sticky 侧边栏，区别于 works/discover 的 app-shell 固定模式）
// 接口对接：
// - 积分包购买 → pointsApi.createOrder + 轮询 + "points:changed" WS 监听（档位以 GET /point-packages 为唯一真源，失败回退常量）
// - 订阅套餐 → GET /packages 真数据渲染（管理端配置即用户端展示）；后端暂无订阅下单 API，确认支付为 toast 占位
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import {
  Check, ChevronDown, Coins, Compass, CreditCard, Folder, Gem, MessageCircle,
  Minus, Sparkles, X,
} from "lucide-react";

import { api, pointsApi, POINT_PACKAGES, type OrderResp } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
import "./recharge.css";

// ===== 订阅套餐数据字典（原样摘自设计稿 planData / DOM data-* 属性）=====
interface PlanFeature { text: string; muted?: boolean }
interface PlanDef {
  id: string;
  name: string;
  desc: string;
  badge?: { cls: string; text: string };
  featured?: boolean;
  priceMonthly: number;
  priceYearly: number; // 按年=月均价×12（与设计稿 selectPlan 逻辑一致）
  creditMonthly: number;
  creditYearly: number;
  creditBonus?: string;
  features: PlanFeature[];
  cta: string;
}

const PLANS: PlanDef[] = [
  {
    id: "basic", name: "基础版", desc: "个人创作者入门之选",
    priceMonthly: 70, priceYearly: 58 * 12, creditMonthly: 500, creditYearly: 6000,
    features: [
      { text: "Seedance 2.5 全功能" },
      { text: "高清 1080P 导出" },
      { text: "智能镜头优化" },
      { text: "标准技术支持" },
      { text: "4K 超清导出", muted: true },
    ],
    cta: "立即订阅",
  },
  {
    id: "pro", name: "进阶版", desc: "高频创作的性价比之选",
    badge: { cls: "popular", text: "最受欢迎" }, featured: true,
    priceMonthly: 700, priceYearly: 583 * 12, creditMonthly: 7000, creditYearly: 84000,
    creditBonus: "首月 +2100",
    features: [
      { text: "全模型解锁" },
      { text: "4K 超清导出" },
      { text: "优先生成队列" },
      { text: "3D 空间约束" },
      { text: "AI 智能配音" },
    ],
    cta: "立即订阅",
  },
  {
    id: "premium", name: "高级版", desc: "专业团队的创作利器",
    badge: { cls: "value", text: "超值之选" },
    priceMonthly: 1960, priceYearly: 1633 * 12, creditMonthly: 19600, creditYearly: 235200,
    creditBonus: "首月 +9800",
    features: [
      { text: "专属客户经理" },
      { text: "API 接入权限" },
      { text: "企业级安全保障" },
      { text: "多人协作 (5人)" },
      { text: "优先功能体验" },
    ],
    cta: "立即订阅",
  },
  {
    id: "flagship", name: "旗舰版", desc: "海量产出的旗舰方案",
    badge: { cls: "best", text: "企业旗舰" },
    priceMonthly: 4900, priceYearly: 4083 * 12, creditMonthly: 49000, creditYearly: 588000,
    creditBonus: "首月 +49000",
    features: [
      { text: "定制模型训练" },
      { text: "SLA 服务保障" },
      { text: "专属部署方案" },
      { text: "无限团队席位" },
      { text: "7x24 技术支持" },
    ],
    cta: "联系销售",
  },
];

// 积分包徽章映射（后端 3 档真源 × 设计稿徽章样式）
const PACK_BADGE: Record<string, { cls: string; text: string } | null> = {
  POINTS_PKG_SMALL: null,
  POINTS_PKG_MEDIUM: { cls: "new", text: "新人特惠" },
  POINTS_PKG_LARGE: { cls: "hot", text: "超值" },
};

// 积分包展示项（真源 GET /point-packages，失败回退常量）
interface PackItem {
  code: string;
  name: string;
  points: number;
  amount_yuan: string;
  desc: string;
  badge: string | null;
}

// 支付弹窗订单视图模型
interface PayOrder {
  kind: "plan" | "pack";
  name: string;
  cycle: string;
  credits: string;
  amount: string;
  // 积分包购买时携带后端档位（订阅为 null → toast 占位）
  pack?: { code: string; points: number; amountYuan: string };
  // 订阅购买时携带套餐编码与计费周期（biz_type=package）
  plan?: { code: string; cycle: string };
}

export default function PointsPage() {
  const { user } = useAuthStore();
  const avatarUrl = useAvatarUrl(user?.avatar_url);

  // 页面状态
  const [billing, setBilling] = useState<"monthly" | "yearly">("monthly");
  const [view, setView] = useState<"subscription" | "packs">("packs");
  const [profileOpen, setProfileOpen] = useState(false);
  const [points, setPoints] = useState<number | null>(null);

  // 支付弹窗状态
  const [order, setOrder] = useState<PayOrder | null>(null);
  const [payMethod, setPayMethod] = useState<"wechat" | "alipay">("wechat");
  const [paying, setPaying] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // 订阅套餐真源（GET /packages：管理端 packages 表；失败回退设计稿静态 PLANS）
  const [plans, setPlans] = useState<PlanDef[]>(PLANS);
  // 积分包真源（GET /point-packages：管理端 point_packages 表；失败回退常量）
  const [packs, setPacks] = useState<PackItem[]>(
    POINT_PACKAGES.map((p) => ({ code: p.code, name: p.name, points: p.points, amount_yuan: p.amount_yuan, desc: p.desc, badge: p.badge })),
  );

  // 加载套餐与积分包（管理端配置即用户端展示）
  useEffect(() => {
    void api
      .get<{ items: { package_code: string; name: string; billing_cycle: string; price_yuan: string; status: string; points_quota: number }[] }>("/packages")
      .then((r) => {
        const active = r.items.filter((p) => p.status.toUpperCase() === "ACTIVE");
        if (active.length === 0) return;
        setPlans(
          active.map((p, i) => {
            const price = Number(p.price_yuan) || 0;
            const annual = (p.billing_cycle || "").toUpperCase() === "ANNUAL";
            return {
              id: p.package_code,
              name: p.name || p.package_code,
              desc: `${annual ? "年度" : "月度"}订阅 · ${(p.points_quota || 0).toLocaleString()} 积分`,
              badge: i === 0 ? { cls: "popular", text: "推荐" } : undefined,
              featured: i === 0,
              priceMonthly: annual ? Math.round(price / 12) : price,
              priceYearly: annual ? price : price * 12,
              creditMonthly: annual ? Math.round((p.points_quota || 0) / 12) : p.points_quota || 0,
              creditYearly: annual ? p.points_quota || 0 : (p.points_quota || 0) * 12,
              features: [
                { text: "积分可用于所有生成功能" },
                { text: `套餐编码 ${p.package_code}` },
                { text: annual ? "按年生效，到期自动续费" : "按月生效，到期自动续费" },
              ],
              cta: "立即订阅",
            };
          }),
        );
      })
      .catch(() => {
        /* 回退设计稿静态套餐 */
      });
    void api
      .get<{ items: { package_code: string; name: string; points: number; bonus_points: number; price_yuan: string; status: string }[] }>("/point-packages")
      .then((r) => {
        const active = r.items.filter((p) => p.status.toUpperCase() === "ON" || p.status.toUpperCase() === "ACTIVE");
        if (active.length === 0) return;
        setPacks(
          active.map((p) => ({
            code: p.package_code,
            name: p.name || "积分包",
            points: p.points || 0,
            amount_yuan: p.price_yuan || "0",
            desc: p.bonus_points ? `含 ${p.bonus_points.toLocaleString()} 加赠积分` : "即买即用",
            badge: (PACK_BADGE[p.package_code] ?? null)?.text ?? null,
          })),
        );
      })
      .catch(() => {
        /* 回退常量积分包 */
      });
  }, []);

  // 侧栏积分余额（真实接口）
  const loadPoints = useCallback(async () => {
    try {
      const acc = await pointsApi.me();
      setPoints(acc.current_balance);
    } catch {
      /* 积分失败不阻塞页面 */
    }
  }, []);

  useEffect(() => {
    void loadPoints();
  }, [loadPoints]);

  // WS 实时到账：POINTS_BALANCE_CHANGED → 刷新余额；source=order_paid（支付到账）才结束轮询并提示
  useEffect(() => {
    const handler = (ev: Event) => {
      const detail = (ev as CustomEvent).detail as { source?: string } | undefined;
      void loadPoints();
      if (detail?.source !== "order_paid") return; // 任务结算等其他积分变动仅刷新余额
      if (pollRef.current) clearInterval(pollRef.current);
      setOrder(null);
      toast.success("充值到账，余额已更新");
    };
    window.addEventListener("points:changed", handler);
    return () => window.removeEventListener("points:changed", handler);
  }, [loadPoints]);

  // 轮询兜底清理
  useEffect(() => () => { if (pollRef.current) clearInterval(pollRef.current); }, []);

  // 积分明细弹窗（GET /points/transactions）
  const [txnOpen, setTxnOpen] = useState(false);
  const [txns, setTxns] = useState<{ txn_type: string; direction: string; amount: number; balance_after: number; description: string; created_at: string }[]>([]);
  const [txnsLoading, setTxnsLoading] = useState(false);

  const openTxns = async () => {
    setTxnOpen(true);
    setTxnsLoading(true);
    try {
      const d = await api.get<{ items: { txn_type: string; direction: string; amount: number; balance_after: number; description: string; created_at: string }[] }>("/points/transactions", { params: { page_size: 50 } });
      setTxns(d.items ?? []);
    } catch {
      setTxns([]);
    } finally {
      setTxnsLoading(false);
    }
  };

  // 流水类型 → 文案/颜色
  const txnMeta = (t: { txn_type: string; direction: string; amount: number }): { label: string; cls: string; sign: string } => {
    const map: Record<string, { label: string; cls: string }> = {
      RECHARGE: { label: "充值", cls: "text-green-400" },
      PACKAGE_ORDER: { label: "购买积分包", cls: "text-green-400" },
      SUBSCRIPTION: { label: "订阅开通", cls: "text-green-400" },
      ASSET_GENERATION: { label: "资产生成", cls: "text-red-400" },
      VIDEO_GENERATION: { label: "视频生成", cls: "text-red-400" },
      EXPORT: { label: "导出", cls: "text-red-400" },
      FREEZE: { label: "冻结", cls: "text-amber-gold" },
      UNFREEZE: { label: "解冻", cls: "text-amber-gold" },
      REFUND: { label: "退款", cls: "text-green-400" },
      ADMIN_ADJUST: { label: "管理员调整", cls: "text-blue-400" },
    };
    const m = map[t.txn_type] ?? { label: t.txn_type || "流水", cls: "text-text-secondary" };
    const sign = t.direction === "CREDIT" || t.amount > 0 ? "+" : "";
    return { label: m.label, cls: m.cls, sign };
  };

  // 弹窗开关（对齐设计稿：body overflow 锁定 + ESC 关闭）
  const openPayModal = (o: PayOrder) => {
    setPayMethod("wechat");
    setOrder(o);
    document.body.style.overflow = "hidden";
  };
  const closePayModal = () => {
    setOrder(null);
    document.body.style.overflow = "";
  };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && order) closePayModal();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  // 订阅卡 → 弹窗（biz_type=package：管理端 packages 表真源，确认支付走真接口）
  const selectPlan = (p: PlanDef) => {
    const yearly = billing === "yearly";
    const price = yearly ? p.priceYearly : p.priceMonthly;
    const credits = `${(yearly ? p.creditYearly : p.creditMonthly).toLocaleString()} 积分/${yearly ? "年" : "月"}`;
    openPayModal({
      kind: "plan",
      name: p.name,
      cycle: yearly ? "按年自动续费" : "按月自动续费",
      credits,
      amount: `¥${price.toFixed(2)}`,
      plan: { code: p.id, cycle: yearly ? "yearly" : "monthly" },
    });
  };

  // 积分包卡 → 弹窗（携带后端档位，确认支付走真接口）
  const selectPack = (pkg: { code: string; points: number; amount_yuan: string }) => {
    openPayModal({
      kind: "pack",
      name: `${pkg.points.toLocaleString()} 积分包`,
      cycle: "一次性购买",
      credits: `${pkg.points.toLocaleString()} 积分`,
      amount: `¥${Number(pkg.amount_yuan).toFixed(2)}`,
      pack: { code: pkg.code, points: pkg.points, amountYuan: pkg.amount_yuan },
    });
  };

  // 确认支付：订阅/积分包 → createOrder + 轮询
  const confirmPayment = async () => {
    if (!order) return;
    if (order.kind === "plan") {
      const plan = order.plan;
      if (!plan) return;
      try {
        setPaying(true);
        const resp = await api.post<OrderResp>("/orders", {
          biz_type: "package",
          package_code: plan.code,
          billing_cycle: plan.cycle,
          amount_yuan: "0.00", // 订阅金额以服务端 packages 表为准（忽略前端金额）
          points: 0,
          idempotency_key: `subscribe-${Date.now()}`,
        });
        toast.success("订阅订单已创建，等待支付回调（沙箱环境）");
        // 开发环境：自动触发 mock 支付到账（生产无此端点，静默失败回退轮询）
        try {
          await api.post(`/orders/${resp.order_no}/mock-pay`);
          toast.success("订阅已开通（沙箱支付）");
          void loadPoints();
          setOrder(null);
          closePayModal();
          return;
        } catch { /* 生产环境无 mock-pay，走轮询 */ }
        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = setInterval(async () => {
          try {
            const s = await pointsApi.orderStatus(resp.order_no);
            if (s.pay_status === "PAID") {
              if (pollRef.current) clearInterval(pollRef.current);
              const sub = (s as unknown as { subscription?: { plan_code?: string; expires_at?: string } }).subscription;
              toast.success(sub?.plan_code ? `会员已开通：${sub.plan_code}，到期 ${(sub.expires_at || "").slice(0, 10)}` : "会员已开通");
              void loadPoints();
              setOrder(null);
            }
          } catch {
            /* 忽略单次轮询失败 */
          }
        }, 2000);
        closePayModal();
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "订阅下单失败");
      } finally {
        setPaying(false);
      }
      return;
    }
    const pack = order.pack;
    if (!pack) return;
    try {
      setPaying(true);
      const resp = await pointsApi.createOrder(
        { package_code: pack.code, amount_yuan: pack.amountYuan, points: pack.points },
        `purchase-${Date.now()}`,
      );
      toast.success("订单已创建，等待支付回调（沙箱环境）");
      // 开发环境：自动触发 mock 支付到账（生产无此端点，静默失败回退轮询）
      try {
        await api.post(`/orders/${resp.order_no}/mock-pay`);
        toast.success(`充值到账 +${pack.points} 积分`);
        void loadPoints();
        setOrder(null);
        closePayModal();
        return;
      } catch { /* 生产环境无 mock-pay，走轮询 */ }
      // 轮询支付状态兜底（WS 到账事件会提前结束）
      if (pollRef.current) clearInterval(pollRef.current);
      pollRef.current = setInterval(async () => {
        try {
          const s = await pointsApi.orderStatus(resp.order_no);
          if (s.pay_status === "PAID") {
            if (pollRef.current) clearInterval(pollRef.current);
            toast.success(`充值到账 +${s.points} 积分`);
            void loadPoints();
            setOrder(null);
          }
        } catch {
          /* 忽略单次轮询失败 */
        }
      }, 2000);
      closePayModal();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "下单失败");
    } finally {
      setPaying(false);
    }
  };

  const yearly = billing === "yearly";

  return (
    <div className="pricing-page">
      <div className="flex min-h-screen">
        {/* ===== 左侧 72px 侧边栏（照搬设计稿，三导航均无 active；sticky 保持可见）===== */}
        <aside
          id="sidebar-nav"
          className="flex flex-col items-center py-3 gap-1 flex-shrink-0 z-10"
          style={{
            width: 72, background: "var(--luxreal-sidebar)", borderRight: "1px solid var(--luxreal-border)",
            position: "sticky", top: 0, height: "100vh",
          }}
        >
          {/* Logo */}
          <div className="flex items-center justify-center flex-shrink-0" style={{ height: 56, marginBottom: 4 }}>
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M6 4H22V8H10V24H6V4Z" fill="white" />
              <path d="M14 12H22V16H18V24H14V12Z" fill="white" opacity="0.7" />
            </svg>
          </div>

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 中心导航（充值页无 active，对齐设计稿） */}
          <nav className="flex flex-col items-center gap-2 flex-shrink-0">
            <Link to="/" className="luxreal-nav-item" title="创作">
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

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 头像（点击展开面板，LuxReal 对齐版 ProfilePanel） */}
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

          {/* 积分（真实余额，点击查看明细，金色徽章 Coins 图标 + 数字） */}
          <button className="sidebar-credits" style={{ cursor: "pointer" }} title="查看积分明细" onClick={() => void openTxns()}>
            <span className="sidebar-credits-badge">
              <Coins size={16} className="sidebar-credits-icon" />
            </span>
            {points === null ? "…" : points.toLocaleString()}
          </button>
          {/* 充值（当前页） */}
          <Link to="/points" className="sidebar-recharge" title="充值">
            充值
          </Link>
        </aside>

        {/* ===== 主内容区（自然滚动）===== */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Hero Section */}
          <section className="pricing-hero">
            <div className="hero-badge">
              <Sparkles size={14} />
              新用户首月享 5 折优惠
            </div>
            <h1 className="hero-title">选择适合你的创作方案</h1>
            <p className="hero-subtitle">
              从个人创作者到专业团队，LuxReal 提供多种订阅方案，
              让每一次创作都物超所值
            </p>

            {/* Billing Toggle（始终显示） */}
            <div className="billing-toggle-wrap">
              <span
                className={`billing-label${billing === "monthly" ? " active" : ""}`}
                onClick={() => setBilling("monthly")}
              >
                按月付费
              </span>
              <div
                className={`billing-switch${yearly ? " yearly" : ""}`}
                onClick={() => setBilling(yearly ? "monthly" : "yearly")}
              />
              <span
                className={`billing-label${yearly ? " active" : ""}`}
                onClick={() => setBilling("yearly")}
              >
                按年付费
                {view === "subscription" && <span className="billing-save">省 17%</span>}
              </span>
            </div>

            {/* 订阅套餐/积分包切换按钮（始终显示） */}
            <div style={{ display: "flex", justifyContent: "center", marginTop: 24, gap: 8 }} id="packsToggle">
              <button
                className="plan-cta-btn"
                style={view === "subscription"
                  ? { width: "auto", padding: "10px 24px", borderColor: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }
                  : { width: "auto", padding: "10px 24px" }}
                onClick={() => setView("subscription")}
              >
                订阅套餐
              </button>
              <button
                className="plan-cta-btn"
                style={view === "packs"
                  ? { width: "auto", padding: "10px 24px", borderColor: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)" }
                  : { width: "auto", padding: "10px 24px" }}
                onClick={() => setView("packs")}
              >
                积分包
              </button>
            </div>
          </section>

          {/* Pricing Container */}
          <div className="pricing-container">
            {/* Monthly/Yearly Plans */}
            {view === "subscription" && (
              <div className="plans-section active">
                <div className="plans-grid">
                  {plans.map((p) => (
                    <div key={p.id} className={`plan-card${p.featured ? " featured" : ""}`} data-plan-id={p.id}>
                      {p.badge && <div className={`plan-badge ${p.badge.cls}`}>{p.badge.text}</div>}
                      <div className="plan-name">{p.name}</div>
                      <div className="plan-desc">{p.desc}</div>
                      <div className="plan-price-wrap">
                        <div className="plan-price">
                          <span className="plan-price-symbol">¥</span>
                          <span className="plan-price-value">
                            {(yearly ? p.priceYearly : p.priceMonthly).toLocaleString()}
                          </span>
                        </div>
                        <div className="plan-price-unit">{yearly ? "/ 年" : "/ 月"}</div>
                      </div>
                      <div className="plan-credit-info">
                        <Gem size={16} className="plan-credit-icon" />
                        <span className="plan-credit-text">
                          {(yearly ? p.creditYearly : p.creditMonthly).toLocaleString()} 积分 / {yearly ? "年" : "月"}
                        </span>
                        {p.creditBonus && <span className="plan-credit-bonus">{p.creditBonus}</span>}
                      </div>
                      <div className="plan-features">
                        {p.features.map((f) => (
                          <div key={f.text} className={`plan-feature${f.muted ? " muted" : ""}`}>
                            {f.muted ? <Minus size={16} /> : <Check size={16} />}
                            <span>{f.text}</span>
                          </div>
                        ))}
                      </div>
                      <button className="plan-cta-btn" onClick={() => selectPlan(p)}>{p.cta}</button>
                    </div>
                  ))}
                </div>

                {/* Comparison Table */}
                <div className="comparison-section">
                  <div className="section-header">
                    <h2 className="section-title">功能对比</h2>
                    <p className="section-subtitle">详细了解各版本功能差异，选择最适合你的方案</p>
                  </div>
                  <div className="comparison-table">
                    <div className="comp-row header">
                      <div className="comp-cell label">功能</div>
                      <div className="comp-cell">基础版</div>
                      <div className="comp-cell highlight">进阶版</div>
                      <div className="comp-cell">高级版</div>
                      <div className="comp-cell">旗舰版</div>
                    </div>
                    <div className="comp-row category">
                      <div className="comp-cell">基础功能</div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">月度积分</div>
                      <div className="comp-cell">500</div>
                      <div className="comp-cell highlight">7,000</div>
                      <div className="comp-cell">19,600</div>
                      <div className="comp-cell">49,000</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">视频模型</div>
                      <div className="comp-cell">2.5 标准</div>
                      <div className="comp-cell yes">全部解锁</div>
                      <div className="comp-cell yes">全部解锁</div>
                      <div className="comp-cell yes">全部解锁</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">最高分辨率</div>
                      <div className="comp-cell">1080P</div>
                      <div className="comp-cell">4K</div>
                      <div className="comp-cell">4K</div>
                      <div className="comp-cell">4K</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">生成队列优先级</div>
                      <div className="comp-cell">标准</div>
                      <div className="comp-cell">优先</div>
                      <div className="comp-cell">极速</div>
                      <div className="comp-cell">专属</div>
                    </div>
                    <div className="comp-row category">
                      <div className="comp-cell">高级功能</div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">智能镜头优化</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">3D 空间约束</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">AI 智能配音</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">API 接入</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell yes">✓</div>
                      <div className="comp-cell yes">✓</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">定制模型训练</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell no">—</div>
                      <div className="comp-cell yes">✓</div>
                    </div>
                    <div className="comp-row category">
                      <div className="comp-cell">服务支持</div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                      <div className="comp-cell"></div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">技术支持</div>
                      <div className="comp-cell">工单</div>
                      <div className="comp-cell">优先工单</div>
                      <div className="comp-cell">专属经理</div>
                      <div className="comp-cell">SLA 保障</div>
                    </div>
                    <div className="comp-row">
                      <div className="comp-cell label">团队协作人数</div>
                      <div className="comp-cell">1 人</div>
                      <div className="comp-cell">3 人</div>
                      <div className="comp-cell">10 人</div>
                      <div className="comp-cell">不限</div>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Credits Packs（档位以后端 POINT_PACKAGES 为唯一真源，视觉类照搬设计稿） */}
            {view === "packs" && (
              <div id="plans-packs" className="packs-section active">
                <div className="section-header">
                  <h2 className="section-title">积分包</h2>
                  <p className="section-subtitle">一次性购买，积分永久有效，随用随充</p>
                </div>
                <div className="packs-grid">
                  {packs.map((pkg) => {
                    const badge = PACK_BADGE[pkg.code] ?? null;
                    return (
                      <div key={pkg.code} className="pack-card" data-plan-id={pkg.code} onClick={() => selectPack(pkg)}>
                        {badge && <div className={`pack-badge ${badge.cls}`}>{badge.text}</div>}
                        <div className="pack-credit-icon-wrap">
                          <Gem size={24} />
                        </div>
                        <div className="pack-credits">{pkg.points.toLocaleString()}</div>
                        <div className="pack-credits-label">积分</div>
                        <div className="pack-price">¥{Number(pkg.amount_yuan).toFixed(2)}</div>
                        {pkg.badge ? (
                          <div className="pack-discount">{pkg.badge}</div>
                        ) : (
                          <div style={{ height: 14 }} />
                        )}
                      </div>
                    );
                  })}
                </div>

                {/* 积分包 FAQ */}
                <div className="faq-section">
                  <div className="section-header">
                    <h2 className="section-title">常见问题</h2>
                    <p className="section-subtitle">关于积分包的常见疑问</p>
                  </div>
                  <div className="faq-grid">
                    {[
                      { q: "积分会过期吗？", a: "购买的一次性积分包积分永久有效，不会过期。订阅套餐的月度积分当月有效，次月重置。消耗时系统会优先使用即将过期的积分。" },
                      { q: "积分包可以与其他功能搭配使用吗？", a: "可以。积分可用于所有生成功能（角色、场景、道具、视频生成等），无限制地应用于任何需要积分的创作操作。" },
                      { q: "如何充值积分？", a: "您可以随时在个人中心或当前页面选择积分包进行购买。购买后积分将立即到账并可用于创作。" },
                      { q: "支持退款吗？", a: "未使用的积分包支持 7 天内无理由退款。如有退款需求，请联系客服处理。" },
                      { q: "企业用户有优惠吗？", a: "大批量采购可享受专属报价和服务方案。如有 20 人以上团队采购需求，请联系我们的商务团队获取定制化优惠方案。" },
                    ].map((f) => (
                      <div key={f.q} className="faq-item">
                        <div className="faq-question">
                          <span>{f.q}</span>
                          <ChevronDown size={20} />
                        </div>
                        <div className="faq-answer">{f.a}</div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}

          </div>

          {/* Footer */}
          <footer className="site-footer">
            <div className="footer-links">
              <a href="#" className="footer-link">关于我们</a>
              <a href="#" className="footer-link">使用条款</a>
              <a href="#" className="footer-link">隐私政策</a>
              <a href="#" className="footer-link">帮助中心</a>
              <a href="#" className="footer-link">联系我们</a>
            </div>
            <div className="footer-copy">© 2026 Empty Broken. All rights reserved.</div>
          </footer>
        </div>
      </div>

      {/* Payment Modal（类名重命名 .pay-modal-*，交互对齐设计稿） */}
      <div
        className={`pay-modal-overlay${order ? " active" : ""}`}
        onClick={(e) => { if (e.target === e.currentTarget) closePayModal(); }}
      >
        <div className="pay-modal-box" onClick={(e) => e.stopPropagation()}>
          <div className="pay-modal-header">
            <div className="pay-modal-title">确认订单</div>
            <div className="pay-modal-desc">请确认您的购买信息</div>
            <button className="pay-modal-close-btn" onClick={closePayModal}>
              <X size={20} />
            </button>
          </div>
          <div className="pay-modal-body">
            {/* Order Summary */}
            <div className="order-summary">
              <div className="order-row">
                <span className="order-label">套餐名称</span>
                <span className="order-value">{order?.name ?? ""}</span>
              </div>
              <div className="order-row">
                <span className="order-label">计费方式</span>
                <span className="order-value">{order?.cycle ?? ""}</span>
              </div>
              <div className="order-row">
                <span className="order-label">包含积分</span>
                <span className="order-value">{order?.credits ?? ""}</span>
              </div>
              <div className="order-row total">
                <span className="order-label">应付金额</span>
                <span className="order-value">{order?.amount ?? ""}</span>
              </div>
            </div>

            {/* Payment Methods */}
            <div className="pay-section-title">选择支付方式</div>
            <div className="pay-method-list">
              <div
                className={`pay-method-item${payMethod === "wechat" ? " selected" : ""}`}
                onClick={() => setPayMethod("wechat")}
              >
                <div className="pay-method-icon wechat">
                  <MessageCircle size={20} />
                </div>
                <div className="pay-method-text">
                  <div className="pay-method-name">微信支付</div>
                  <div className="pay-method-desc">推荐使用，安全便捷</div>
                </div>
                <div className="pay-method-radio" />
              </div>
              <div
                className={`pay-method-item${payMethod === "alipay" ? " selected" : ""}`}
                onClick={() => setPayMethod("alipay")}
              >
                <div className="pay-method-icon alipay">
                  <CreditCard size={20} />
                </div>
                <div className="pay-method-text">
                  <div className="pay-method-name">支付宝</div>
                  <div className="pay-method-desc">支持花呗分期</div>
                </div>
                <div className="pay-method-radio" />
              </div>
            </div>
          </div>
          <div className="pay-modal-footer">
            <button className="pay-btn-cancel" onClick={closePayModal}>取消</button>
            <button className="pay-btn-confirm" disabled={paying} onClick={() => void confirmPayment()}>
              {paying ? "提交中..." : "确认支付"}
            </button>
          </div>
        </div>
      </div>

      {/* ===== 积分明细弹窗（GET /points/transactions） ===== */}
      {txnOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          style={{ background: "rgba(5,5,15,0.72)", backdropFilter: "blur(6px)" }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setTxnOpen(false);
          }}
        >
          <div
            className="w-[560px] max-h-[75vh] flex flex-col rounded-2xl border border-[var(--luxreal-border)]"
            style={{ background: "var(--luxreal-panel-bg, #15151D)", boxShadow: "0 24px 80px rgba(0,0,0,0.6)" }}
          >
            <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--luxreal-border)]">
              <div>
                <div className="text-[15px] font-semibold text-[var(--luxreal-foreground)]">积分明细</div>
                <div className="text-xs mt-0.5" style={{ color: "var(--luxreal-muted-foreground)" }}>
                  当前余额 {points === null ? "…" : points.toLocaleString()} · 最近 50 条流水
                </div>
              </div>
              <button onClick={() => setTxnOpen(false)} className="text-[var(--luxreal-muted-foreground)] hover:text-white transition-colors" aria-label="关闭">
                <X size={18} />
              </button>
            </div>
            <div className="flex-1 min-h-0 overflow-y-auto px-5 py-4 space-y-2.5">
              {txnsLoading ? (
                <div className="py-16 text-center text-sm" style={{ color: "var(--luxreal-muted-foreground)" }}>加载流水…</div>
              ) : txns.length === 0 ? (
                <div className="py-16 text-center text-sm" style={{ color: "var(--luxreal-muted-foreground)" }}>暂无积分流水</div>
              ) : (
                txns.map((t, i) => {
                  const m = txnMeta(t);
                  return (
                    <div key={i} className="flex items-center gap-3">
                      <div className={`text-sm font-medium w-24 flex-shrink-0 ${m.cls}`}>{m.label}</div>
                      <div className="flex-1 min-w-0">
                        <div className="text-[13px] text-white/70 truncate">{t.description || "—"}</div>
                        <div className="text-[10px]" style={{ color: "var(--luxreal-muted-foreground)" }}>
                          {t.created_at ? new Date(t.created_at).toLocaleString("zh-CN", { hour12: false }) : ""}
                        </div>
                      </div>
                      <div className={`text-sm font-mono flex-shrink-0 ${m.cls}`}>
                        {m.sign}{t.amount.toLocaleString()}
                      </div>
                      <div className="text-xs text-text-secondary font-mono flex-shrink-0 w-20 text-right">
                        {t.balance_after.toLocaleString()}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
