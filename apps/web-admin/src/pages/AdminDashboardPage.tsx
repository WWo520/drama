// AdminDashboardPage：迁移自 admin-dashboard.html（DOM 1:1）。
// 数据策略：8 数据卡中 7 项接 GET /admin/dashboard 真数据（失败回退设计稿静态值）；
// 双趋势图接 GET /admin/trends?days=7 真数据（失败回退设计稿静态 SVG）；
// 排行/队列/日志为设计稿静态数据，运行中任务数 3s 随机刷新（设计稿 JS 行为）。
import { useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import {
  Users,
  Activity,
  FolderOpen,
  Film,
  Cpu,
  Clock,
  DollarSign,
  Percent,
  TrendingUp,
  Flame,
  Play,
  UserCheck,
  BrainCircuit,
  Layers,
  Loader,
  PlayCircle,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ScrollText,
} from "lucide-react";

import { fmtNum } from "../lib";

// Dashboard 真数据（admin_store.go Dashboard）
interface DashboardData {
  total_users: number;
  dau: number;
  total_spaces: number;
  total_works: number;
  total_tasks: number;
  pending_reports: number;
  revenue_yuan: string;
}

// 数据卡定义（值为设计稿静态回退值，接口成功后覆盖前 7 项）
interface StatCard {
  icon: React.ComponentType<{ className?: string }>;
  iconBg: string;
  iconColor: string;
  trend: string;
  trendUp: boolean;
  value: string;
  label: string;
}

const DEFAULT_CARDS: StatCard[] = [
  { icon: Users, iconBg: "bg-amber-gold/10", iconColor: "text-amber-gold", trend: "12.5%", trendUp: true, value: "128,456", label: "累计注册用户" },
  { icon: Activity, iconBg: "bg-blue-500/10", iconColor: "text-blue-400", trend: "8.3%", trendUp: true, value: "24,891", label: "今日活跃 (DAU)" },
  { icon: FolderOpen, iconBg: "bg-purple-500/10", iconColor: "text-purple-400", trend: "5.2%", trendUp: true, value: "8,723", label: "空间总数" },
  { icon: Film, iconBg: "bg-pink-500/10", iconColor: "text-pink-400", trend: "15.7%", trendUp: true, value: "45,672", label: "作品总数" },
  { icon: Cpu, iconBg: "bg-cyan-500/10", iconColor: "text-cyan-400", trend: "22.1%", trendUp: true, value: "3,847", label: "今日生成任务" },
  { icon: Clock, iconBg: "bg-orange-500/10", iconColor: "text-orange-400", trend: "8.0%", trendUp: false, value: "156", label: "待审核作品" },
  { icon: DollarSign, iconBg: "bg-green-500/10", iconColor: "text-green-400", trend: "18.6%", trendUp: true, value: "¥89,432", label: "今日收入" },
  { icon: Percent, iconBg: "bg-rose-500/10", iconColor: "text-rose-400", trend: "2.4%", trendUp: true, value: "6.87%", label: "付费转化率" },
];

// Top10 热门作品（设计稿静态）
const HOT_WORKS = [
  { rank: 1, grad: "from-purple-600 to-pink-500", name: "都市巅峰之逆袭归来", plays: "128.5万次播放", delta: "+12%", deltaCls: "text-amber-gold" },
  { rank: 2, grad: "from-blue-600 to-cyan-500", name: "重生之都市修仙", plays: "98.3万次播放", delta: "+8%", deltaCls: "text-amber-gold" },
  { rank: 3, grad: "from-rose-600 to-orange-500", name: "豪门总裁的契约新娘", plays: "87.1万次播放", delta: "+5%", deltaCls: "text-green-400" },
  { rank: 4, grad: "from-green-600 to-teal-500", name: "穿越古代当王爷", plays: "65.4万次播放", delta: "+3%", deltaCls: "text-green-400" },
  { rank: 5, grad: "from-indigo-600 to-purple-500", name: "战神归来之凤唳九天", plays: "52.8万次播放", delta: "-2%", deltaCls: "text-red-400" },
];

// Top10 活跃用户（设计稿静态）
const ACTIVE_USERS = [
  { rank: 1, grad: "from-amber-400 to-orange-500", initial: "L", name: "李逍遥", sub: "VIP会员 · 在线 32h", score: "328", scoreCls: "text-amber-gold" },
  { rank: 2, grad: "from-pink-400 to-rose-500", initial: "Z", name: "周芷若", sub: "VIP会员 · 在线 28h", score: "256", scoreCls: "text-amber-gold" },
  { rank: 3, grad: "from-cyan-400 to-blue-500", initial: "W", name: "王语嫣", sub: "SVIP会员 · 在线 25h", score: "214", scoreCls: "text-amber-gold" },
  { rank: 4, grad: "from-green-400 to-emerald-500", initial: "C", name: "陈长生", sub: "VIP会员 · 在线 22h", score: "189", scoreCls: "text-text-secondary" },
  { rank: 5, grad: "from-purple-400 to-violet-500", initial: "H", name: "黄蓉", sub: "SVIP会员 · 在线 20h", score: "167", scoreCls: "text-text-secondary" },
];

// 模型使用排行（设计稿静态）
const MODEL_RANKS = [
  { no: "01", name: "Seedance-Video-XL", count: "12,458 次", grad: "from-amber-gold to-amber-dim", width: "92%", noCls: "text-amber-gold" },
  { no: "02", name: "Seedream-Image-Pro", count: "9,823 次", grad: "from-blue-500 to-cyan-400", width: "78%", noCls: "text-text-secondary" },
  { no: "03", name: "Voice-Clone-Pro", count: "7,654 次", grad: "from-purple-500 to-pink-400", width: "61%", noCls: "text-text-secondary" },
  { no: "04", name: "Script-Writer-Max", count: "5,432 次", grad: "from-green-500 to-emerald-400", width: "43%", noCls: "text-text-secondary" },
  { no: "05", name: "Music-Gen-Studio", count: "3,210 次", grad: "from-rose-500 to-orange-400", width: "26%", noCls: "text-text-secondary" },
];

// 任务队列（设计稿静态；运行中数值实时抖动）
const QUEUE_ITEMS = [
  { icon: Loader, iconBg: "bg-yellow-500/10", iconColor: "text-yellow-400", name: "排队中", desc: "等待执行的任务", value: 47, valueCls: "text-yellow-400", barCls: "bg-yellow-400", width: "23%" },
  { icon: PlayCircle, iconBg: "bg-blue-500/10", iconColor: "text-blue-400", name: "运行中", desc: "正在执行的任务", value: 34, valueCls: "text-blue-400", barCls: "bg-blue-400 progress-bar-running", width: "68%", live: true },
  { icon: CheckCircle2, iconBg: "bg-green-500/10", iconColor: "text-green-400", name: "成功", desc: "今日已完成任务", value: 2847, valueCls: "text-green-400", barCls: "bg-green-400", width: "94%" },
  { icon: XCircle, iconBg: "bg-red-500/10", iconColor: "text-red-400", name: "失败", desc: "执行失败的任务", value: 126, valueCls: "text-red-400", barCls: "bg-red-400", width: "4.2%" },
  { icon: AlertTriangle, iconBg: "bg-purple-500/10", iconColor: "text-purple-400", name: "死信", desc: "无法处理的任务", value: 8, valueCls: "text-purple-400", barCls: "bg-purple-400", width: "0.3%" },
];

// 活动日志（设计稿静态 10 条）
const LOGS: { dot: string; content: React.ReactNode; sub: string; time: string; border: boolean }[] = [
  { dot: "bg-green-400", content: <><span className="text-amber-gold">李逍遥</span> 发布了新作品 <span className="text-blue-400">《都市巅峰》</span></>, sub: "作品审核 · 待处理", time: "2 分钟前", border: true },
  { dot: "bg-blue-400", content: <>系统完成 <span className="text-amber-gold">128</span> 个视频生成任务</>, sub: "算力任务 · 自动调度", time: "5 分钟前", border: true },
  { dot: "bg-yellow-400", content: <><span className="text-amber-gold">周芷若</span> 申请升级 SVIP 会员</>, sub: "财务审核 · 待处理", time: "12 分钟前", border: true },
  { dot: "bg-red-400", content: <>作品 <span className="text-red-400">《违规内容示例》</span> 审核未通过</>, sub: "内容审核 · 管理员操作", time: "28 分钟前", border: true },
  { dot: "bg-green-400", content: <>新注册用户 <span className="text-amber-gold">256</span> 人，今日累计 <span className="text-amber-gold">1,847</span> 人</>, sub: "用户增长 · 实时统计", time: "35 分钟前", border: true },
  { dot: "bg-purple-400", content: <>系统自动清理 <span className="text-purple-400">3,421</span> 个过期缓存文件</>, sub: "系统维护 · 自动任务", time: "1 小时前", border: true },
  { dot: "bg-amber-gold", content: <>今日收入突破 <span className="text-amber-gold">¥80,000</span>，同比增长 18.6%</>, sub: "财务数据 · 里程碑", time: "2 小时前", border: true },
  { dot: "bg-cyan-400", content: <>GPU 集群扩容完成，新增 <span className="text-cyan-400">8</span> 张 A100 显卡</>, sub: "算力资源 · 管理员操作", time: "3 小时前", border: true },
  { dot: "bg-green-400", content: <>作品 <span className="text-blue-400">《重生之都市修仙》</span> 播放量突破百万</>, sub: "内容数据 · 热门通知", time: "4 小时前", border: true },
  { dot: "bg-text-muted", content: <>平台配置更新：视频生成单价调整</>, sub: "平台配置 · Admin 操作", time: "6 小时前", border: false },
];

const WEEK_DAYS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];

// ---- 趋势图真数据（GET /admin/trends，近 7 天按日序列；失败回退设计稿静态 SVG） ----

interface TrendsResp {
  days: string[]; // "MM-DD"
  registrations: number[];
  actives: number[];
  revenue_yuan: number[];
  payers: number[];
}

// 坐标系与设计稿一致：viewBox 500×220，绘图区 x:40~490 / y:20~200，X 轴标签 y=215
const GRID_Y = [20, 70, 120, 170];
const PLOT_TOP = 20;
const PLOT_BOTTOM = 200;

function scaleY(v: number, max: number): number {
  if (max <= 0) return PLOT_BOTTOM;
  return PLOT_BOTTOM - (v / (max * 1.15)) * (PLOT_BOTTOM - PLOT_TOP);
}

function linePath(xs: number[], ys: number[]): string {
  return xs.map((x, i) => `${i === 0 ? "M" : "L"}${x},${ys[i]}`).join(" ");
}

function areaPath(xs: number[], ys: number[]): string {
  if (xs.length === 0) return "";
  return `${linePath(xs, ys)} L${xs[xs.length - 1]},${PLOT_BOTTOM} L${xs[0]},${PLOT_BOTTOM} Z`;
}

function fmtCount(v: number): string {
  return v >= 10000 ? `${Math.round(v / 1000)}k` : `${Math.round(v)}`;
}

function fmtYuan(v: number): string {
  return v >= 1000 ? `¥${(v / 1000).toFixed(1)}k` : `¥${Math.round(v)}`;
}

// 左图：用户增长趋势（注册金线 + 活跃蓝线，同轴缩放）
function UserTrendChart({ trends }: { trends: TrendsResp }) {
  const n = trends.days.length;
  const xs = trends.days.map((_, i) => 70 + i * (420 / Math.max(1, n - 1)));
  const maxV = Math.max(1, ...trends.registrations, ...trends.actives);
  const regY = trends.registrations.map((v) => scaleY(v, maxV));
  const actY = trends.actives.map((v) => scaleY(v, maxV));
  const tickVals = GRID_Y.map((_, i) => maxV * 1.15 * (1 - i / 4));
  return (
    <svg className="w-full h-full" viewBox="0 0 500 220" preserveAspectRatio="none">
      <g className="chart-grid">
        {GRID_Y.map((y) => (
          <line key={y} x1="40" y1={y} x2="490" y2={y} />
        ))}
      </g>
      {GRID_Y.map((y, i) => (
        <text key={y} x="35" y={y + 4} textAnchor="end" fill="#6b7280" fontSize="10">
          {fmtCount(tickVals[i])}
        </text>
      ))}
      <defs>
        <linearGradient id="goldGradientT" x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" style={{ stopColor: "#fbbf24", stopOpacity: 0.3 }} />
          <stop offset="100%" style={{ stopColor: "#fbbf24", stopOpacity: 0 }} />
        </linearGradient>
        <linearGradient id="blueGradientT" x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" style={{ stopColor: "#60a5fa", stopOpacity: 0.2 }} />
          <stop offset="100%" style={{ stopColor: "#60a5fa", stopOpacity: 0 }} />
        </linearGradient>
      </defs>
      <path d={areaPath(xs, regY)} fill="url(#goldGradientT)" />
      <path d={linePath(xs, regY)} fill="none" stroke="#fbbf24" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      {xs.map((x, i) => (
        <circle key={i} cx={x} cy={regY[i]} r="4" fill="#fbbf24" />
      ))}
      <path d={areaPath(xs, actY)} fill="url(#blueGradientT)" />
      <path d={linePath(xs, actY)} fill="none" stroke="#60a5fa" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      {xs.map((x, i) => (
        <circle key={i} cx={x} cy={actY[i]} r="3.5" fill="#60a5fa" />
      ))}
      {trends.days.map((d, i) => (
        <text key={d + i} x={xs[i]} y="215" textAnchor="middle" fill="#6b7280" fontSize="10">
          {d}
        </text>
      ))}
    </svg>
  );
}

// 右图：收入趋势（柱 = 当日收入，折线 = 当日付费用户数，双轴）
function RevenueChart({ trends }: { trends: TrendsResp }) {
  const n = trends.days.length;
  const slot = 450 / Math.max(1, n);
  const centers = trends.days.map((_, i) => 40 + slot * (i + 0.5));
  const barW = Math.min(35, slot * 0.55);
  const maxAmt = Math.max(1, ...trends.revenue_yuan);
  const maxPayer = Math.max(1, ...trends.payers);
  const tickVals = GRID_Y.map((_, i) => maxAmt * 1.15 * (1 - i / 4));
  return (
    <svg className="w-full h-full" viewBox="0 0 500 220" preserveAspectRatio="none">
      <g className="chart-grid">
        {GRID_Y.map((y) => (
          <line key={y} x1="40" y1={y} x2="490" y2={y} />
        ))}
      </g>
      {GRID_Y.map((y, i) => (
        <text key={y} x="35" y={y + 4} textAnchor="end" fill="#6b7280" fontSize="10">
          {fmtYuan(tickVals[i])}
        </text>
      ))}
      <defs>
        <linearGradient id="barGradientT" x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" style={{ stopColor: "#fbbf24", stopOpacity: 0.9 }} />
          <stop offset="100%" style={{ stopColor: "#d97706", stopOpacity: 0.6 }} />
        </linearGradient>
      </defs>
      {trends.revenue_yuan.map((v, i) => {
        const y = scaleY(v, maxAmt);
        return <rect key={i} x={centers[i] - barW / 2} y={y} width={barW} height={PLOT_BOTTOM - y} rx="3" fill="url(#barGradientT)" />;
      })}
      {(() => {
        const ys = trends.payers.map((v) => scaleY(v, maxPayer));
        return (
          <>
            <path d={linePath(centers, ys)} fill="none" stroke="#34d399" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            {centers.map((x, i) => (
              <circle key={i} cx={x} cy={ys[i]} r="3.5" fill="#34d399" />
            ))}
          </>
        );
      })()}
      {trends.days.map((d, i) => (
        <text key={d + i} x={centers[i]} y="215" textAnchor="middle" fill="#6b7280" fontSize="10">
          {d}
        </text>
      ))}
    </svg>
  );
}

function rankBadge(rank: number) {
  if (rank <= 3) return `rank-${rank} w-5 h-5 rounded text-xs font-bold flex items-center justify-center flex-shrink-0`;
  return "w-5 h-5 rounded text-xs font-bold flex items-center justify-center flex-shrink-0 bg-[#2A2A3E] text-text-secondary";
}

export default function AdminDashboardPage() {
  const [cards, setCards] = useState<StatCard[]>(DEFAULT_CARDS);
  const [running, setRunning] = useState(34);
  const [trends, setTrends] = useState<TrendsResp | null>(null);

  // 真数据：GET /admin/dashboard（失败保持设计稿静态值）
  useEffect(() => {
    api
      .get<DashboardData>("/admin/dashboard")
      .then((d) => {
        setCards((prev) => {
          const next = [...prev];
          next[0] = { ...next[0], value: fmtNum(d.total_users) };
          next[1] = { ...next[1], value: fmtNum(d.dau) };
          next[2] = { ...next[2], value: fmtNum(d.total_spaces) };
          next[3] = { ...next[3], value: fmtNum(d.total_works) };
          next[4] = { ...next[4], value: fmtNum(d.total_tasks) };
          next[5] = { ...next[5], value: fmtNum(d.pending_reports) };
          next[6] = { ...next[6], value: `¥${fmtNum(Math.round(Number(d.revenue_yuan) || 0))}` };
          return next;
        });
      })
      .catch(() => {
        /* 回退设计稿静态值 */
      });
  }, []);

  // 趋势图真数据：GET /admin/trends（近 7 天按日序列；失败保持设计稿静态 SVG）
  useEffect(() => {
    api
      .get<TrendsResp>("/admin/trends?days=7")
      .then((t) => {
        if (t && Array.isArray(t.days) && t.days.length > 0) setTrends(t);
      })
      .catch(() => {
        /* 回退设计稿静态 SVG */
      });
  }, []);

  // 运行中任务数 3s 抖动（设计稿 JS 行为：±2，钳制 20~50）
  useEffect(() => {
    const timer = window.setInterval(() => {
      setRunning((cur) => {
        const delta = Math.floor(Math.random() * 5) - 2;
        return Math.max(20, Math.min(50, cur + delta));
      });
    }, 3000);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <>
      {/* Page title */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-white mb-1">数据概览</h1>
        <p className="text-sm text-text-muted">欢迎回来，以下是平台今日运营数据概览</p>
      </div>

      {/* ====== SECTION 1: DATA CARDS ====== */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        {cards.map((card) => {
          const Icon = card.icon;
          return (
            <div key={card.label} className="data-card bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
              <div className="flex items-start justify-between mb-4">
                <div className={`w-10 h-10 rounded-lg ${card.iconBg} flex items-center justify-center`}>
                  <Icon className={`w-5 h-5 ${card.iconColor}`} />
                </div>
                <span
                  className={`text-xs px-2 py-1 rounded-md flex items-center ${
                    card.trendUp ? "text-green-400 bg-green-400/10" : "text-red-400 bg-red-400/10"
                  }`}
                >
                  <TrendingUp className="w-3 h-3 mr-1" />
                  {card.trend}
                </span>
              </div>
              <div className="text-2xl font-bold text-amber-gold glow-gold font-mono">{card.value}</div>
              <div className="text-xs text-text-muted mt-1">{card.label}</div>
            </div>
          );
        })}
      </div>

      {/* ====== SECTION 2: TREND CHARTS ====== */}
      <div className="grid grid-cols-2 gap-4 mb-6">
        {/* Left: 用户增长趋势图 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-5">
            <div>
              <h3 className="text-white font-semibold">用户增长趋势</h3>
              <p className="text-xs text-text-muted mt-0.5">近 7 天注册与活跃数据</p>
            </div>
            <div className="flex items-center gap-4 text-xs">
              <span className="flex items-center text-text-secondary">
                <span className="w-3 h-3 rounded-full bg-amber-gold mr-2"></span>注册用户
              </span>
              <span className="flex items-center text-text-secondary">
                <span className="w-3 h-3 rounded-full bg-blue-400 mr-2"></span>活跃用户
              </span>
            </div>
          </div>

          <div className="relative h-[220px]">
            {trends && trends.days.length > 0 ? (
              <UserTrendChart trends={trends} />
            ) : (
            <svg className="w-full h-full" viewBox="0 0 500 220" preserveAspectRatio="none">
              <g className="chart-grid">
                <line x1="40" y1="20" x2="490" y2="20" />
                <line x1="40" y1="70" x2="490" y2="70" />
                <line x1="40" y1="120" x2="490" y2="120" />
                <line x1="40" y1="170" x2="490" y2="170" />
              </g>
              <text x="35" y="24" textAnchor="end" fill="#6b7280" fontSize="10">30k</text>
              <text x="35" y="74" textAnchor="end" fill="#6b7280" fontSize="10">22.5k</text>
              <text x="35" y="124" textAnchor="end" fill="#6b7280" fontSize="10">15k</text>
              <text x="35" y="174" textAnchor="end" fill="#6b7280" fontSize="10">7.5k</text>

              <defs>
                <linearGradient id="goldGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" style={{ stopColor: "#fbbf24", stopOpacity: 0.3 }} />
                  <stop offset="100%" style={{ stopColor: "#fbbf24", stopOpacity: 0 }} />
                </linearGradient>
                <linearGradient id="blueGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" style={{ stopColor: "#60a5fa", stopOpacity: 0.2 }} />
                  <stop offset="100%" style={{ stopColor: "#60a5fa", stopOpacity: 0 }} />
                </linearGradient>
              </defs>

              <path d="M70,150 L140,130 L210,110 L280,95 L350,80 L420,60 L490,45 L490,200 L70,200 Z" fill="url(#goldGradient)" />
              <path d="M70,150 L140,130 L210,110 L280,95 L350,80 L420,60 L490,45" fill="none" stroke="#fbbf24" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
              <circle cx="70" cy="150" r="4" fill="#fbbf24" />
              <circle cx="140" cy="130" r="4" fill="#fbbf24" />
              <circle cx="210" cy="110" r="4" fill="#fbbf24" />
              <circle cx="280" cy="95" r="4" fill="#fbbf24" />
              <circle cx="350" cy="80" r="4" fill="#fbbf24" />
              <circle cx="420" cy="60" r="4" fill="#fbbf24" />
              <circle cx="490" cy="45" r="4" fill="#fbbf24" />

              <path d="M70,170 L140,160 L210,145 L280,135 L350,125 L420,110 L490,95 L490,200 L70,200 Z" fill="url(#blueGradient)" />
              <path d="M70,170 L140,160 L210,145 L280,135 L350,125 L420,110 L490,95" fill="none" stroke="#60a5fa" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
              <circle cx="70" cy="170" r="3.5" fill="#60a5fa" />
              <circle cx="140" cy="160" r="3.5" fill="#60a5fa" />
              <circle cx="210" cy="145" r="3.5" fill="#60a5fa" />
              <circle cx="280" cy="135" r="3.5" fill="#60a5fa" />
              <circle cx="350" cy="125" r="3.5" fill="#60a5fa" />
              <circle cx="420" cy="110" r="3.5" fill="#60a5fa" />
              <circle cx="490" cy="95" r="3.5" fill="#60a5fa" />

              {WEEK_DAYS.map((d, i) => (
                <text key={d} x={70 + i * 70} y="215" textAnchor="middle" fill="#6b7280" fontSize="10">
                  {d}
                </text>
              ))}
            </svg>
            )}
          </div>
        </div>

        {/* Right: 收入趋势图 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-5">
            <div>
              <h3 className="text-white font-semibold">收入趋势</h3>
              <p className="text-xs text-text-muted mt-0.5">近 7 天收入与付费用户</p>
            </div>
            <div className="flex items-center gap-4 text-xs">
              <span className="flex items-center text-text-secondary">
                <span className="w-3 h-3 rounded-sm bg-amber-gold mr-2"></span>收入
              </span>
              <span className="flex items-center text-text-secondary">
                <span className="w-3 h-0.5 bg-green-400 mr-2"></span>付费用户
              </span>
            </div>
          </div>

          <div className="relative h-[220px]">
            {trends && trends.days.length > 0 ? (
              <RevenueChart trends={trends} />
            ) : (
            <svg className="w-full h-full" viewBox="0 0 500 220" preserveAspectRatio="none">
              <g className="chart-grid">
                <line x1="40" y1="20" x2="490" y2="20" />
                <line x1="40" y1="70" x2="490" y2="70" />
                <line x1="40" y1="120" x2="490" y2="120" />
                <line x1="40" y1="170" x2="490" y2="170" />
              </g>
              <text x="35" y="24" textAnchor="end" fill="#6b7280" fontSize="10">¥100k</text>
              <text x="35" y="74" textAnchor="end" fill="#6b7280" fontSize="10">¥75k</text>
              <text x="35" y="124" textAnchor="end" fill="#6b7280" fontSize="10">¥50k</text>
              <text x="35" y="174" textAnchor="end" fill="#6b7280" fontSize="10">¥25k</text>

              <defs>
                <linearGradient id="barGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" style={{ stopColor: "#fbbf24", stopOpacity: 0.9 }} />
                  <stop offset="100%" style={{ stopColor: "#d97706", stopOpacity: 0.6 }} />
                </linearGradient>
              </defs>

              <rect x="55" y="100" width="35" height="100" rx="3" fill="url(#barGradient)" />
              <rect x="120" y="85" width="35" height="115" rx="3" fill="url(#barGradient)" />
              <rect x="185" y="70" width="35" height="130" rx="3" fill="url(#barGradient)" />
              <rect x="250" y="55" width="35" height="145" rx="3" fill="url(#barGradient)" />
              <rect x="315" y="60" width="35" height="140" rx="3" fill="url(#barGradient)" />
              <rect x="380" y="35" width="35" height="165" rx="3" fill="url(#barGradient)" />
              <rect x="445" y="30" width="35" height="170" rx="3" fill="url(#barGradient)" />

              <path d="M72,140 L137,130 L202,115 L267,100 L332,105 L397,80 L462,70" fill="none" stroke="#34d399" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
              <circle cx="72" cy="140" r="3.5" fill="#34d399" />
              <circle cx="137" cy="130" r="3.5" fill="#34d399" />
              <circle cx="202" cy="115" r="3.5" fill="#34d399" />
              <circle cx="267" cy="100" r="3.5" fill="#34d399" />
              <circle cx="332" cy="105" r="3.5" fill="#34d399" />
              <circle cx="397" cy="80" r="3.5" fill="#34d399" />
              <circle cx="462" cy="70" r="3.5" fill="#34d399" />

              {WEEK_DAYS.map((d, i) => (
                <text key={d} x={72 + i * 65} y="215" textAnchor="middle" fill="#6b7280" fontSize="10">
                  {d}
                </text>
              ))}
            </svg>
            )}
          </div>
        </div>
      </div>

      {/* ====== SECTION 3: RANKINGS ====== */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        {/* Top10 热门作品 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-white font-semibold flex items-center">
              <Flame className="w-4 h-4 text-amber-gold mr-2" />
              Top10 热门作品
            </h3>
            <a href="#" className="text-xs text-text-muted hover:text-amber-gold transition-colors">查看全部</a>
          </div>
          <div className="space-y-3">
            {HOT_WORKS.map((w) => (
              <div key={w.rank} className="flex items-center gap-3 py-1">
                <span className={rankBadge(w.rank)}>{w.rank}</span>
                <div className={`w-10 h-10 rounded bg-gradient-to-br ${w.grad} flex-shrink-0 flex items-center justify-center`}>
                  <Play className="w-4 h-4 text-white" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="text-sm text-white truncate">{w.name}</div>
                  <div className="text-xs text-text-muted">{w.plays}</div>
                </div>
                <span className={`text-xs ${w.deltaCls} font-mono`}>{w.delta}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Top10 活跃用户 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-white font-semibold flex items-center">
              <UserCheck className="w-4 h-4 text-blue-400 mr-2" />
              Top10 活跃用户
            </h3>
            <a href="#" className="text-xs text-text-muted hover:text-amber-gold transition-colors">查看全部</a>
          </div>
          <div className="space-y-3">
            {ACTIVE_USERS.map((u) => (
              <div key={u.rank} className="flex items-center gap-3 py-1">
                <span className={rankBadge(u.rank)}>{u.rank}</span>
                <div className={`w-8 h-8 rounded-full bg-gradient-to-br ${u.grad} flex-shrink-0 flex items-center justify-center text-white text-xs font-bold`}>
                  {u.initial}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="text-sm text-white truncate">{u.name}</div>
                  <div className="text-xs text-text-muted">{u.sub}</div>
                </div>
                <span className={`text-xs ${u.scoreCls} font-mono`}>{u.score}</span>
              </div>
            ))}
          </div>
        </div>

        {/* 模型使用排行 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-white font-semibold flex items-center">
              <BrainCircuit className="w-4 h-4 text-purple-400 mr-2" />
              模型使用排行
            </h3>
            <a href="#" className="text-xs text-text-muted hover:text-amber-gold transition-colors">查看全部</a>
          </div>
          <div className="space-y-4">
            {MODEL_RANKS.map((m) => (
              <div key={m.no}>
                <div className="flex items-center justify-between mb-1.5">
                  <div className="flex items-center gap-2">
                    <span className={`text-xs font-bold ${m.noCls}`}>{m.no}</span>
                    <span className="text-sm text-white">{m.name}</span>
                  </div>
                  <span className="text-xs text-text-secondary font-mono">{m.count}</span>
                </div>
                <div className="w-full h-1.5 bg-[#2A2A3E] rounded-full overflow-hidden">
                  <div className={`h-full bg-gradient-to-r ${m.grad} rounded-full`} style={{ width: m.width }}></div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* ====== SECTION 4: REAL-TIME MONITORING ====== */}
      <div className="grid grid-cols-2 gap-4">
        {/* Left: 任务队列状态 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-5">
            <h3 className="text-white font-semibold flex items-center">
              <Layers className="w-4 h-4 text-cyan-400 mr-2" />
              任务队列状态
            </h3>
            <span className="text-xs text-green-400 bg-green-400/10 px-2 py-1 rounded-md flex items-center">
              <span className="w-1.5 h-1.5 rounded-full bg-green-400 mr-1.5 animate-pulse"></span>
              实时更新中
            </span>
          </div>

          <div className="space-y-4">
            {QUEUE_ITEMS.map((q) => {
              const Icon = q.icon;
              return (
                <div key={q.name}>
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center">
                      <div className={`w-8 h-8 rounded-lg ${q.iconBg} flex items-center justify-center mr-3`}>
                        <Icon className={`w-4 h-4 ${q.iconColor}`} />
                      </div>
                      <div>
                        <div className="text-sm text-white">{q.name}</div>
                        <div className="text-xs text-text-muted">{q.desc}</div>
                      </div>
                    </div>
                    <div className="text-right">
                      <div className={`text-lg font-bold ${q.valueCls} font-mono`}>
                        {fmtNum(q.live ? running : q.value)}
                      </div>
                    </div>
                  </div>
                  <div className="w-full h-2 bg-[#2A2A3E] rounded-full overflow-hidden">
                    <div className={`h-full ${q.barCls} rounded-full`} style={{ width: q.width }}></div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Right: 最近活动日志 */}
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-white font-semibold flex items-center">
              <ScrollText className="w-4 h-4 text-amber-gold mr-2" />
              最近活动日志
            </h3>
            <a href="#" className="text-xs text-text-muted hover:text-amber-gold transition-colors">查看全部</a>
          </div>

          <div className="space-y-0 max-h-[320px] overflow-y-auto -mx-2 px-2">
            {LOGS.map((log, i) => (
              <div
                key={i}
                className={`log-item flex items-start gap-3 py-3 px-2 rounded${
                  log.border ? " border-b border-[#2A2A3E]/50" : ""
                }`}
              >
                <div className={`w-2 h-2 rounded-full ${log.dot} mt-1.5 flex-shrink-0`}></div>
                <div className="flex-1 min-w-0">
                  <div className="text-sm text-white">{log.content}</div>
                  <div className="text-xs text-text-muted mt-0.5">{log.sub}</div>
                </div>
                <div className="text-xs text-text-muted flex-shrink-0">{log.time}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Footer */}
      <div className="mt-8 pt-4 border-t border-[#2A2A3E] text-center">
        <p className="text-xs text-text-muted">导演台 Admin v1.0 · © 2025 导演台创作平台</p>
      </div>
    </>
  );
}
