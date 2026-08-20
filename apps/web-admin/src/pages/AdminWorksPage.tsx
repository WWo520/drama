// AdminWorksPage：迁移自 admin-works.html（DOM 1:1）。
// 数据策略：统计卡并行 4 个 page_size=1 请求拉各状态总数（失败回退设计稿静态值）；
// 表格 GET /admin/works?status= 真数据（失败回退设计稿 6 行示例，不可操作）；
// 状态=服务端过滤，分类/时长/日期=客户端过滤；审核/下架/删除/编辑/推荐走管理端接口。
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@duanju/api-client";
import {
  ArrowDownToLine,
  Check,
  ChevronLeft,
  ChevronRight,
  Download,
  Edit3,
  Eye,
  Film,
  Heart,
  MoreVertical,
  Play,
  Plus,
  Star,
  Trash2,
  X,
} from "lucide-react";

import { toast, errMessage, fmtNum, exportCSV } from "../lib";
import Modal from "../components/Modal";
import "./AdminWorksPage.css";

type WorkStatus = "" | "PUBLISHED" | "PENDING" | "REJECTED" | "REVOKED" | "UNPUBLISHED";

interface BackendWork {
  work_id: string;
  title: string;
  publish_status: string;
  view_count: number;
  like_count: number;
  author: string;
  tags: string;
  favorite_count: number;
  duration_sec: number;
  published_at: string;
  media_url: string;
  is_recommended: boolean;
}

interface WorksResp {
  items: BackendWork[];
  pagination: { page: number; page_size: number; total: number };
}

interface WorkRow {
  work_id: string;
  title: string;
  durationLabel: string;
  durationSec: number;
  author: string;
  authorInitial: string;
  authorGrad: string;
  category: string;
  views: string;
  likes: string;
  favorites: string;
  status: string;
  publishedAt: string;
  mediaUrl: string;
  recommended: boolean;
  real: boolean;
}

const PAGE_SIZE = 10;

// 状态 → 展示（对齐 publish_status_enum；标签配色与设计稿一致）
const STATUS_META: Record<string, { label: string; cls: string; dot: string }> = {
  PUBLISHED: { label: "已发布", cls: "bg-green-500/15 text-green-400", dot: "bg-green-400" },
  PENDING: { label: "待审核", cls: "bg-yellow-500/15 text-yellow-400", dot: "bg-yellow-400" },
  REJECTED: { label: "已驳回", cls: "bg-red-500/15 text-red-400", dot: "bg-red-400" },
  REVOKED: { label: "已下架", cls: "bg-red-500/15 text-red-400", dot: "bg-red-400" },
  UNPUBLISHED: { label: "草稿", cls: "bg-slate-500/15 text-slate-400", dot: "bg-slate-400" },
};

// 设计稿静态回退行（接口失败时展示，示例数据不可操作）
const FALLBACK_ROWS: WorkRow[] = [
  {
    work_id: "wk_2a8f3d1e", title: "霸总追妻：重生后我成了千亿继承人", durationLabel: "3分24秒", durationSec: 204, author: "林小雨", authorInitial: "林",
    authorGrad: "from-pink-500 to-purple-500", category: "都市情感", views: "128.5万", likes: "8.2万", favorites: "3.1万",
    status: "PUBLISHED", publishedAt: "2026-08-15 14:32", mediaUrl: "", recommended: false, real: false,
  },
  {
    work_id: "wk_9c7e2b4a", title: "穿越古代：我带系统当皇帝", durationLabel: "2分58秒", durationSec: 178, author: "墨尘居士", authorInitial: "墨",
    authorGrad: "from-blue-500 to-cyan-500", category: "古装穿越", views: "256.8万", likes: "15.6万", favorites: "6.2万",
    status: "PENDING", publishedAt: "2026-08-16 09:15", mediaUrl: "", recommended: false, real: false,
  },
  {
    work_id: "wk_4d1a8f9c", title: "迷雾追踪：连环杀人案调查实录", durationLabel: "4分12秒", durationSec: 252, author: "陈推理", authorInitial: "陈",
    authorGrad: "from-slate-500 to-slate-700", category: "悬疑推理", views: "89.3万", likes: "4.7万", favorites: "2.8万",
    status: "REVOKED", publishedAt: "2026-08-10 16:48", mediaUrl: "", recommended: false, real: false,
  },
  {
    work_id: "wk_7b3e5c2d", title: "废柴逆袭：我靠系统打脸全场", durationLabel: "1分45秒", durationSec: 105, author: "爽文达人", authorInitial: "爽",
    authorGrad: "from-orange-500 to-red-500", category: "爽文逆袭", views: "312.4万", likes: "22.3万", favorites: "9.5万",
    status: "PUBLISHED", publishedAt: "2026-08-14 20:03", mediaUrl: "", recommended: false, real: false,
  },
  {
    work_id: "wk_1e6d9a4b", title: "职场囧途：我的奇葩同事们", durationLabel: "2分30秒", durationSec: 150, author: "笑料百出", authorInitial: "笑",
    authorGrad: "from-green-500 to-emerald-500", category: "搞笑喜剧", views: "67.2万", likes: "3.8万", favorites: "1.2万",
    status: "PENDING", publishedAt: "2026-08-16 18:22", mediaUrl: "", recommended: false, real: false,
  },
  {
    work_id: "wk_5f8c3e7a", title: "替嫁新娘：冷面总裁的契约妻", durationLabel: "3分05秒", durationSec: 185, author: "苏暖暖", authorInitial: "苏",
    authorGrad: "from-violet-500 to-purple-500", category: "都市情感", views: "198.7万", likes: "12.1万", favorites: "5.4万",
    status: "PUBLISHED", publishedAt: "2026-08-13 11:20", mediaUrl: "", recommended: false, real: false,
  },
];

// 封面渐变池（按行序循环）
const COVER_GRADS = [
  "from-purple-600 to-indigo-900",
  "from-amber-900 to-orange-900",
  "from-slate-800 to-slate-900",
  "from-rose-900 to-pink-900",
  "from-emerald-900 to-teal-900",
  "from-indigo-900 to-purple-900",
];

const CATEGORY_OPTIONS = ["全部分类", "都市情感", "古装穿越", "悬疑推理", "爽文逆袭", "搞笑喜剧"];
const DURATION_OPTIONS = ["全部时长", "1分钟以内", "1-3分钟", "3-5分钟", "5分钟以上"];

// tags jsonb（pgx 扫描为 JSON 字符串）→ 首个分类标签
function firstTag(raw: string): string {
  if (!raw) return "—";
  const t = raw.trim();
  if (t.startsWith("[")) {
    try {
      const arr = JSON.parse(t) as unknown;
      if (Array.isArray(arr) && arr.length > 0) return String(arr[0]);
    } catch {
      /* 落入逗号解析 */
    }
  }
  const first = t.split(",")[0]?.trim();
  return first || "—";
}

function fmtDuration(sec: number): string {
  if (sec <= 0) return "—";
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return m > 0 ? `${m}分${String(s).padStart(2, "0")}秒` : `${s}秒`;
}

function fmtWan(v: number): string {
  if (v >= 10000) return `${(v / 10000).toFixed(1)}万`;
  return `${v}`;
}

// 时长区间客户端过滤
function durationMatch(sec: number, opt: string): boolean {
  switch (opt) {
    case "1分钟以内":
      return sec > 0 && sec < 60;
    case "1-3分钟":
      return sec >= 60 && sec < 180;
    case "3-5分钟":
      return sec >= 180 && sec < 300;
    case "5分钟以上":
      return sec >= 300;
    default:
      return true;
  }
}

export default function AdminWorksPage() {
  const [rows, setRows] = useState<WorkRow[]>(FALLBACK_ROWS);
  const [isReal, setIsReal] = useState(false);
  const [total, setTotal] = useState(12847);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // 统计卡：4 个状态计数（并行 page_size=1 轻量请求；失败保持设计稿静态值）
  const [stats, setStats] = useState({ all: 12847, published: 10532, pending: 156, revoked: 2159 });
  const [statsReal, setStatsReal] = useState(false);

  // 筛选（状态=服务端；其余客户端）
  const [statusFilter, setStatusFilter] = useState<WorkStatus>("");
  const [appliedStatus, setAppliedStatus] = useState<WorkStatus>("");
  const [category, setCategory] = useState("全部分类");
  const [duration, setDuration] = useState("全部时长");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [applied, setApplied] = useState({ category: "全部分类", duration: "全部时长", dateFrom: "", dateTo: "" });

  const loadStats = useCallback(async () => {
    try {
      const req = async (st: WorkStatus): Promise<number> => {
        const r = await api.get<WorksResp>(`/admin/works?status=${st}&page=1&page_size=1`);
        return r.pagination.total;
      };
      const [all, published, pending, revoked] = await Promise.all([
        req(""),
        req("PUBLISHED"),
        req("PENDING"),
        req("REVOKED"),
      ]);
      setStats({ all, published, pending, revoked });
      setStatsReal(true);
    } catch {
      /* 回退设计稿静态值 */
    }
  }, []);

  const load = useCallback(
    async (p: number, st: WorkStatus) => {
      try {
        const resp = await api.get<WorksResp>(
          `/admin/works?status=${st}&page=${p}&page_size=${PAGE_SIZE}`,
        );
        const mapped: WorkRow[] = resp.items.map((w, i) => ({
          work_id: w.work_id,
          title: w.title || "未命名作品",
          durationLabel: fmtDuration(w.duration_sec || 0),
          durationSec: w.duration_sec || 0,
          author: w.author || "—",
          authorInitial: w.author ? w.author.charAt(0) : "?",
          authorGrad: COVER_GRADS[i % COVER_GRADS.length],
          category: firstTag(w.tags),
          views: fmtWan(w.view_count || 0),
          likes: fmtWan(w.like_count || 0),
          favorites: fmtWan(w.favorite_count || 0),
          status: w.publish_status || "UNPUBLISHED",
          publishedAt: w.published_at || "—",
          mediaUrl: w.media_url || "",
          recommended: Boolean(w.is_recommended),
          real: true,
        }));
        setRows(mapped);
        setTotal(resp.pagination.total);
        setIsReal(true);
      } catch {
        setRows(FALLBACK_ROWS);
        setTotal(12847);
        setIsReal(false);
      }
    },
    [],
  );

  useEffect(() => {
    loadStats();
    void load(1, "");
  }, [load, loadStats]);

  // 客户端过滤：分类/时长/日期
  const filtered = useMemo(() => {
    if (!isReal) return rows;
    return rows.filter((r) => {
      if (applied.category !== "全部分类" && r.category !== applied.category) return false;
      if (!durationMatch(r.durationSec, applied.duration)) return false;
      if (applied.dateFrom && r.publishedAt.slice(0, 10) < applied.dateFrom) return false;
      if (applied.dateTo && r.publishedAt.slice(0, 10) > applied.dateTo) return false;
      return true;
    });
  }, [rows, applied, isReal]);

  const allChecked = filtered.length > 0 && filtered.every((r) => selected.has(r.work_id));
  const toggleAll = () => setSelected(allChecked ? new Set() : new Set(filtered.map((r) => r.work_id)));
  const toggleRow = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const rangeStart = (page - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(page * PAGE_SIZE, total);

  const pageBtns = useMemo(() => {
    const arr: (number | "...")[] = [];
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) arr.push(i);
    } else {
      for (let i = 1; i <= 5; i++) arr.push(i);
      arr.push("...");
      arr.push(totalPages);
    }
    return arr;
  }, [totalPages]);

  const gotoPage = (p: number) => {
    if (p < 1 || p > totalPages || !isReal) return;
    setPage(p);
    setSelected(new Set());
  };

  // 应用筛选：状态服务端重拉，分类/时长/日期客户端生效
  const applyFilter = () => {
    if (appliedStatus !== statusFilter) {
      setAppliedStatus(statusFilter);
      setPage(1);
      void load(1, statusFilter);
    }
    setApplied({ category, duration, dateFrom, dateTo });
  };

  const resetFilter = () => {
    setStatusFilter("");
    setCategory("全部分类");
    setDuration("全部时长");
    setDateFrom("");
    setDateTo("");
    setAppliedStatus("");
    setApplied({ category: "全部分类", duration: "全部时长", dateFrom: "", dateTo: "" });
    setPage(1);
    void load(1, "");
  };

  // 审核（PENDING → PUBLISHED/REJECTED）
  const audit = async (row: WorkRow, approve: boolean) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.post(`/admin/works/${row.work_id}/audit`, { approve, note: "" });
      setRows((prev) => prev.filter((r) => r.work_id !== row.work_id));
      setSelected((prev) => {
        const next = new Set(prev);
        next.delete(row.work_id);
        return next;
      });
      setStats((prev) => ({
        ...prev,
        pending: Math.max(0, prev.pending - 1),
        published: approve ? prev.published + 1 : prev.published,
      }));
      toast(approve ? `已通过《${row.title}》` : `已驳回《${row.title}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 下架（PUBLISHED → REVOKED）
  const unlist = async (row: WorkRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.put(`/admin/works/${row.work_id}/unlist`, { note: "管理员下架" });
      setRows((prev) => prev.filter((r) => r.work_id !== row.work_id));
      setSelected((prev) => {
        const next = new Set(prev);
        next.delete(row.work_id);
        return next;
      });
      setStats((prev) => ({
        ...prev,
        published: Math.max(0, prev.published - 1),
        revoked: prev.revoked + 1,
      }));
      toast(`已下架《${row.title}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 强制删除（置回收站）
  const remove = async (row: WorkRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.delete(`/admin/works/${row.work_id}`);
      setRows((prev) => prev.filter((r) => r.work_id !== row.work_id));
      setSelected((prev) => {
        const next = new Set(prev);
        next.delete(row.work_id);
        return next;
      });
      setStats((prev) => ({ ...prev, all: Math.max(0, prev.all - 1) }));
      toast(`已删除《${row.title}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 预览：有 media_url 直接新窗口打开（审核页同款行为）
  const preview = (row: WorkRow) => {
    if (row.mediaUrl) {
      window.open(row.mediaUrl, "_blank");
      return;
    }
    toast("该作品暂无媒体地址", "info");
  };

  // 编辑弹窗（PUT /admin/works/:work_id：title/description/tags）
  const [editTarget, setEditTarget] = useState<WorkRow | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const [editDesc, setEditDesc] = useState("");
  const [editTags, setEditTags] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);

  const openEdit = (row: WorkRow) => {
    setEditTarget(row);
    setEditTitle(row.title);
    setEditDesc("");
    setEditTags(row.category === "—" ? "" : row.category);
  };

  const saveEdit = async () => {
    if (!editTarget) return;
    if (!editTitle.trim()) {
      toast("标题不能为空", "error");
      return;
    }
    setSavingEdit(true);
    try {
      const tags = editTags
        .split(/[,，]/)
        .map((t) => t.trim())
        .filter(Boolean);
      await api.put(`/admin/works/${editTarget.work_id}`, {
        title: editTitle.trim(),
        description: editDesc.trim(),
        tags,
      });
      setRows((prev) =>
        prev.map((r) =>
          r.work_id === editTarget.work_id
            ? { ...r, title: editTitle.trim(), category: tags[0] || "—" }
            : r,
        ),
      );
      setEditTarget(null);
      toast(`已更新《${editTitle.trim()}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingEdit(false);
    }
  };

  // 推荐位切换（PUT /admin/works/:work_id/recommend，仅 PUBLISHED）
  const toggleRecommend = async (row: WorkRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.put(`/admin/works/${row.work_id}/recommend`, { recommended: !row.recommended });
      setRows((prev) => prev.map((r) => (r.work_id === row.work_id ? { ...r, recommended: !row.recommended } : r)));
      toast(row.recommended ? `已取消推荐《${row.title}》` : `已推荐《${row.title}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  const statCards = [
    { label: "作品总数", icon: Film, iconBg: "bg-amber-gold/10", iconColor: "text-amber-gold", value: fmtNum(stats.all), note: statsReal ? "实时统计" : "较昨日 +128", noteCls: statsReal ? "text-text-muted" : "text-green-400" },
    { label: "已发布", icon: Check, iconBg: "bg-green-500/10", iconColor: "text-green-400", value: fmtNum(stats.published), note: statsReal ? "占比 " + (stats.all ? ((stats.published / stats.all) * 100).toFixed(1) : "0") + "%" : "占比 82.0%", noteCls: "text-text-muted" },
    { label: "待审核", icon: X, iconBg: "bg-yellow-500/10", iconColor: "text-yellow-400", value: fmtNum(stats.pending), note: statsReal ? "需及时处理" : "需及时处理", noteCls: "text-yellow-400" },
    { label: "已下架", icon: ArrowDownToLine, iconBg: "bg-red-500/10", iconColor: "text-red-400", value: fmtNum(stats.revoked), note: statsReal ? "占比 " + (stats.all ? ((stats.revoked / stats.all) * 100).toFixed(1) : "0") + "%" : "占比 16.8%", noteCls: "text-text-muted" },
  ];

  return (
    <>
      {/* Page title */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white mb-1">作品管理</h1>
          <p className="text-sm text-text-muted">管理平台所有短剧作品，支持上下架、删除、推荐等操作</p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={() =>
              exportCSV(
                "作品管理",
                ["作品ID", "标题", "作者", "分类", "播放量", "点赞", "收藏", "状态", "发布时间"],
                rows.map((r) => [
                  r.work_id,
                  r.title,
                  r.author,
                  r.category,
                  r.views,
                  r.likes,
                  r.favorites,
                  STATUS_META[r.status]?.label ?? r.status,
                  r.publishedAt,
                ]),
              )
            }
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] transition-colors flex items-center gap-2"
          >
            <Download className="w-4 h-4" />
            导出数据
          </button>
          <button
            onClick={() => toast("推荐作品功能规划中", "info")}
            className="px-4 py-2 bg-amber-gold text-[#0B0B1A] rounded-lg text-sm font-semibold hover:bg-amber-400 transition-colors flex items-center gap-2"
          >
            <Plus className="w-4 h-4" />
            推荐作品
          </button>
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        {statCards.map((c) => {
          const Icon = c.icon;
          return (
            <div key={c.label} className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
              <div className="flex items-center justify-between mb-3">
                <span className="text-text-muted text-sm">{c.label}</span>
                <div className={`w-9 h-9 rounded-lg ${c.iconBg} flex items-center justify-center`}>
                  <Icon className={`w-4 h-4 ${c.iconColor}`} />
                </div>
              </div>
              <div className="text-2xl font-bold text-white font-mono">{c.value}</div>
              <div className={`text-xs mt-1 ${c.noteCls}`}>{c.note}</div>
            </div>
          );
        })}
      </div>

      {/* Filters */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-4 mb-4">
        <div className="flex items-center gap-4 flex-wrap">
          <div className="flex items-center gap-2">
            <span className="text-text-muted text-sm">状态：</span>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as WorkStatus)}
              className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            >
              <option value="">全部状态</option>
              <option value="PUBLISHED">已发布</option>
              <option value="PENDING">待审核</option>
              <option value="REJECTED">已驳回</option>
              <option value="REVOKED">已下架</option>
              <option value="UNPUBLISHED">草稿</option>
            </select>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-text-muted text-sm">分类：</span>
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            >
              {CATEGORY_OPTIONS.map((c) => (
                <option key={c}>{c}</option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-text-muted text-sm">时长：</span>
            <select
              value={duration}
              onChange={(e) => setDuration(e.target.value)}
              className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            >
              {DURATION_OPTIONS.map((d) => (
                <option key={d}>{d}</option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-text-muted text-sm">日期：</span>
            <input
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
              className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            />
            <span className="text-text-muted">至</span>
            <input
              type="date"
              value={dateTo}
              onChange={(e) => setDateTo(e.target.value)}
              className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div className="ml-auto flex gap-2">
            <button
              onClick={resetFilter}
              className="px-3 py-1.5 bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] transition-colors"
            >
              重置
            </button>
            <button
              onClick={applyFilter}
              className="px-3 py-1.5 bg-amber-gold text-[#0B0B1A] rounded-lg text-sm font-medium hover:bg-amber-400 transition-colors"
            >
              筛选
            </button>
          </div>
        </div>
      </div>

      {/* Table */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-[#2A2A3E]">
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">
                  <input type="checkbox" className="mr-2 accent-amber-gold" checked={allChecked} onChange={toggleAll} />
                  作品信息
                </th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">作者</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">分类</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">播放量</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">点赞/收藏</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">状态</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">发布时间</th>
                <th className="text-right px-4 py-3 text-text-muted text-xs font-medium uppercase tracking-wider">操作</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={8} className="px-4 py-10 text-center text-sm text-text-muted">
                    暂无符合条件的作品
                  </td>
                </tr>
              ) : (
                filtered.map((r, i) => {
                  const meta = STATUS_META[r.status] ?? STATUS_META.UNPUBLISHED;
                  return (
                    <tr key={r.work_id} className="border-b border-[#2A2A3E]/50 table-row">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-3">
                          <input
                            type="checkbox"
                            className="accent-amber-gold"
                            checked={selected.has(r.work_id)}
                            onChange={() => toggleRow(r.work_id)}
                          />
                          <div
                            className={`w-16 h-10 rounded bg-gradient-to-br ${COVER_GRADS[i % COVER_GRADS.length]} flex items-center justify-center flex-shrink-0`}
                          >
                            <Play className="w-4 h-4 text-white/60" />
                          </div>
                          <div className="min-w-0">
                            <div className="text-sm text-white font-medium truncate max-w-[240px]">{r.title}</div>
                            <div className="text-xs text-text-muted">
                              ID: {r.work_id.slice(0, 8)} · {r.durationLabel}
                            </div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div
                            className={`w-7 h-7 rounded-full bg-gradient-to-br ${r.authorGrad} flex items-center justify-center text-white text-xs font-bold`}
                          >
                            {r.authorInitial}
                          </div>
                          <div className="text-sm text-white">{r.author}</div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="tag bg-purple-500/15 text-purple-400">{r.category}</span>
                      </td>
                      <td className="px-4 py-3 text-sm text-white">{r.views}</td>
                      <td className="px-4 py-3 text-sm">
                        <div className="flex items-center gap-3 text-text-secondary text-xs">
                          <span>
                            <Heart className="w-3 h-3 inline mr-1" />
                            {r.likes}
                          </span>
                          <span>
                            <Star className="w-3 h-3 inline mr-1" />
                            {r.favorites}
                          </span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`tag ${meta.cls}`}>
                          <span className={`status-dot ${meta.dot}`} />
                          {meta.label}
                        </span>
                        {r.recommended && (
                          <span className="tag ml-1 bg-amber-gold/15 text-amber-gold">
                            <Star className="w-3 h-3 mr-0.5" />
                            推荐
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm text-text-secondary">{r.publishedAt}</td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex justify-end gap-1">
                          <button className="action-btn text-text-secondary" title="预览" onClick={() => preview(r)}>
                            <Eye className="w-3.5 h-3.5" />
                          </button>
                          {r.status === "PUBLISHED" && (
                            <button className="action-btn text-amber-gold" title="编辑" onClick={() => openEdit(r)}>
                              <Edit3 className="w-3.5 h-3.5" />
                            </button>
                          )}
                          {r.status === "PUBLISHED" && (
                            <button
                              className={`action-btn ${r.recommended ? "text-amber-gold" : "text-text-secondary"}`}
                              title={r.recommended ? "取消推荐" : "推荐作品"}
                              onClick={() => toggleRecommend(r)}
                            >
                              <Star className={`w-3.5 h-3.5 ${r.recommended ? "fill-amber-gold" : ""}`} />
                            </button>
                          )}
                          {r.status === "PUBLISHED" && (
                            <button className="action-btn text-red-400" title="下架" onClick={() => unlist(r)}>
                              <ArrowDownToLine className="w-3.5 h-3.5" />
                            </button>
                          )}
                          {r.status === "PENDING" && (
                            <button className="action-btn text-green-400" title="通过" onClick={() => audit(r, true)}>
                              <Check className="w-3.5 h-3.5" />
                            </button>
                          )}
                          {r.status === "PENDING" && (
                            <button className="action-btn text-red-400" title="驳回" onClick={() => audit(r, false)}>
                              <X className="w-3.5 h-3.5" />
                            </button>
                          )}
                          {(r.status === "REVOKED" || r.status === "REJECTED" || r.status === "UNPUBLISHED") && (
                            <button className="action-btn text-red-400" title="删除" onClick={() => remove(r)}>
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                          <button className="action-btn text-text-muted" title="更多" onClick={() => toast("更多操作规划中", "info")}>
                            <MoreVertical className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        <div className="flex items-center justify-between px-4 py-3 border-t border-[#2A2A3E]">
          <div className="text-sm text-text-muted">
            共 <span className="text-white font-medium">{fmtNum(total)}</span> 条，每页显示 {PAGE_SIZE} 条
            {isReal && (
              <span className="ml-2">
                （当前第 {rangeStart}-{rangeEnd} 条）
              </span>
            )}
          </div>
          <div className="flex items-center gap-1">
            <button
              className="w-8 h-8 rounded-lg border border-[#2A2A3E] flex items-center justify-center text-text-muted hover:border-[#3A3A4E] disabled:opacity-40"
              disabled={page <= 1 || !isReal}
              onClick={() => gotoPage(page - 1)}
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            {pageBtns.map((n, idx) =>
              n === "..." ? (
                <span key={`dot-${idx}`} className="text-text-muted px-1 text-sm">
                  ...
                </span>
              ) : (
                <button
                  key={n}
                  className={`w-8 h-8 rounded-lg border text-sm ${
                    n === page
                      ? "bg-amber-gold text-[#0B0B1A] font-medium border-amber-gold"
                      : "border-[#2A2A3E] text-text-secondary hover:border-[#3A3A4E]"
                  }`}
                  onClick={() => gotoPage(n)}
                >
                  {n}
                </button>
              ),
            )}
            <button
              className="w-8 h-8 rounded-lg border border-[#2A2A3E] flex items-center justify-center text-text-secondary hover:border-[#3A3A4E] disabled:opacity-40"
              disabled={page >= totalPages || !isReal}
              onClick={() => gotoPage(page + 1)}
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      {/* 编辑作品弹窗（PUT /admin/works/:work_id） */}
      <Modal
        title="编辑作品"
        subtitle={editTarget ? `ID: ${editTarget.work_id.slice(0, 8)}` : ""}
        open={Boolean(editTarget)}
        onClose={() => setEditTarget(null)}
        footer={
          <>
            <button
              className="px-4 py-2 rounded-lg text-sm text-text-secondary hover:text-white transition-colors"
              onClick={() => setEditTarget(null)}
            >
              取消
            </button>
            <button
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              disabled={savingEdit}
              onClick={() => void saveEdit()}
            >
              {savingEdit ? "保存中…" : "保存修改"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">作品标题</label>
            <input
              type="text"
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">作品描述</label>
            <textarea
              value={editDesc}
              onChange={(e) => setEditDesc(e.target.value)}
              rows={3}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 resize-none"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">分类标签（逗号分隔）</label>
            <input
              type="text"
              value={editTags}
              onChange={(e) => setEditTags(e.target.value)}
              placeholder="如：都市情感, 悬疑推理"
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50"
            />
          </div>
        </div>
      </Modal>
    </>
  );
}