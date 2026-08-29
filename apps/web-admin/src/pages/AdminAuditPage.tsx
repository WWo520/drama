// AdminAuditPage：迁移自 admin-audit.html（DOM 1:1）。
// 数据策略：4 个 tab 统一走 GET /audit/works?status= 真数据（待审 PENDING/已通过 PUBLISHED/
// 已驳回 REJECTED/草稿 UNPUBLISHED），失败时待审 tab 回退设计稿静态 10 行，其余 tab 空态；
// 审核 POST /audit/works/:id/review，已通过 tab 下架 POST /audit/works/:id/revoke。
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@duanju/api-client";
import {
  CheckCircle2,
  XCircle,
  Clock3,
  Search,
  List,
  LayoutGrid,
  Settings2,
  Download,
  Play,
  Check,
  X,
  AlertTriangle,
  Eye,
  ChevronLeft,
  ChevronRight,
  Ban,
} from "lucide-react";

import { toast, errMessage, fmtNum, exportCSV } from "../lib";
import "./AdminAuditPage.css";

type TabKey = "pending" | "approved" | "rejected" | "draft" | "reports" | "logs"; // A2+A3: 举报 + 审计日志
type ComplianceState = "pass" | "fail" | "warn";

interface AuditRow {
  work_id: string;
  title: string;
  meta: string;
  author: string;
  authorInitial: string;
  authorGrad: string;
  submitted: string;
  compliance: { label: string; state: ComplianceState }[];
  media_url: string;
  status: string; // publish_status：PENDING/PUBLISHED/REJECTED/UNPUBLISHED/REVOKED
  real: boolean;
}

interface BackendWork {
  work_id: string;
  title: string;
  description: string;
  media_url: string;
  publish_status: string;
  updated_at: string;
  view_count: number;
  like_count: number;
}

interface AuditResp {
  items: BackendWork[];
  pagination: { page: number; page_size: number; total: number };
}

const PAGE_SIZE = 10;

// tab → 后端 publish_status 过滤值
const TAB_STATUS: Record<TabKey, string> = {
  pending: "PENDING",
  approved: "PUBLISHED",
  rejected: "REJECTED",
  draft: "UNPUBLISHED",
  reports: "", // A2: 举报无状态映射（前端仅展示 PENDING 列表）
  logs: "", // 审计日志无状态映射，仅作占位
};

const TAB_LABEL: Record<TabKey, string> = {
  pending: "待审核",
  approved: "已通过",
  rejected: "已驳回",
  draft: "草稿",
  reports: "举报", // A2: 举报处理 tab
  logs: "审计日志", // A3: 审计日志 tab
};

// 行内状态标签（复用设计稿 tag 样式）
const STATUS_TAGS: Record<string, { label: string; cls: string }> = {
  PENDING: { label: "待审核", cls: "tag-pending" },
  PUBLISHED: { label: "已通过", cls: "tag-pass" },
  REJECTED: { label: "已驳回", cls: "tag-reject" },
  UNPUBLISHED: { label: "草稿", cls: "tag-draft" },
  REVOKED: { label: "已下架", cls: "tag-draft" },
};

// 设计稿静态回退行（接口失败时展示）
const FALLBACK_ROWS: AuditRow[] = [
  {
    work_id: "W100001", title: "都市巅峰之逆袭归来", meta: "80集 · 总时长 120分钟", author: "李逍遥", authorInitial: "L", authorGrad: "from-amber-400 to-orange-500", submitted: "2025-01-20 14:32",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "pass" }, { label: "政治", state: "pass" }, { label: "版权", state: "warn" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100002", title: "重生之都市修仙", meta: "100集 · 总时长 150分钟", author: "周芷若", authorInitial: "Z", authorGrad: "from-pink-400 to-rose-500", submitted: "2025-01-20 13:18",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "fail" }, { label: "政治", state: "pass" }, { label: "版权", state: "pass" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100003", title: "豪门总裁的契约新娘", meta: "60集 · 总时长 90分钟", author: "王语嫣", authorInitial: "W", authorGrad: "from-cyan-400 to-blue-500", submitted: "2025-01-20 12:45",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "pass" }, { label: "政治", state: "pass" }, { label: "版权", state: "pass" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100004", title: "穿越古代当王爷", meta: "120集 · 总时长 180分钟", author: "陈长生", authorInitial: "C", authorGrad: "from-green-400 to-emerald-500", submitted: "2025-01-20 11:22",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "warn" }, { label: "政治", state: "pass" }, { label: "版权", state: "warn" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100005", title: "战神归来之凤唳九天", meta: "90集 · 总时长 135分钟", author: "黄蓉", authorInitial: "H", authorGrad: "from-purple-400 to-violet-500", submitted: "2025-01-20 10:56",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "fail" }, { label: "政治", state: "fail" }, { label: "版权", state: "pass" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100006", title: "暗夜追凶之无间道", meta: "75集 · 总时长 112分钟", author: "郭靖", authorInitial: "G", authorGrad: "from-red-400 to-orange-500", submitted: "2025-01-20 09:40",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "pass" }, { label: "政治", state: "warn" }, { label: "版权", state: "pass" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100007", title: "神医嫡女风华录", meta: "85集 · 总时长 128分钟", author: "小龙女", authorInitial: "X", authorGrad: "from-teal-400 to-cyan-500", submitted: "2025-01-20 09:15",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "pass" }, { label: "政治", state: "pass" }, { label: "版权", state: "pass" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100008", title: "豪门千金的秘密情人", meta: "50集 · 总时长 75分钟", author: "乔峰", authorInitial: "Q", authorGrad: "from-rose-400 to-pink-500", submitted: "2025-01-20 08:28",
    compliance: [
      { label: "色情", state: "fail" }, { label: "暴力", state: "pass" }, { label: "政治", state: "pass" }, { label: "版权", state: "warn" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100009", title: "仙帝归来之纵横都市", meta: "110集 · 总时长 165分钟", author: "段誉", authorInitial: "D", authorGrad: "from-yellow-400 to-amber-500", submitted: "2025-01-20 07:50",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "warn" }, { label: "政治", state: "pass" }, { label: "版权", state: "pass" },
    ], media_url: "", status: "PENDING", real: false,
  },
  {
    work_id: "W100010", title: "错嫁新娘：总裁的替身妻", meta: "65集 · 总时长 98分钟", author: "杨过", authorInitial: "Y", authorGrad: "from-indigo-400 to-purple-500", submitted: "2025-01-20 07:12",
    compliance: [
      { label: "色情", state: "pass" }, { label: "暴力", state: "pass" }, { label: "政治", state: "pass" }, { label: "版权", state: "fail" },
    ], media_url: "", status: "PENDING", real: false,
  },
];

// 审计日志回退行（接口失败时展示）
const FALLBACK_AUDIT_ROWS: AuditRow[] = [
  { work_id: "LOG-001", title: "管理员张三将《都市巅峰之逆袭归来》状态改为已下架", meta: "操作：下架", author: "张三", authorInitial: "Z", authorGrad: "from-amber-400 to-orange-500", submitted: "2026-08-22 14:32", compliance: [{ label: "系统", state: "pass" }], media_url: "", status: "LOG", real: false },
  { work_id: "LOG-002", title: "管理员李四审核通过《战神归来之凤唳九天》", meta: "操作：审核通过", author: "李四", authorInitial: "L", authorGrad: "from-green-400 to-emerald-500", submitted: "2026-08-22 13:18", compliance: [{ label: "系统", state: "pass" }], media_url: "", status: "LOG", real: false },
  { work_id: "LOG-003", title: "管理员王五创建新套餐《Pro 年费版》", meta: "操作：新增套餐", author: "王五", authorInitial: "W", authorGrad: "from-blue-400 to-cyan-500", submitted: "2026-08-22 12:45", compliance: [{ label: "系统", state: "pass" }], media_url: "", status: "LOG", real: false },
];

// 举报回退行（接口失败时展示）
const FALLBACK_REPORT_ROWS: AuditRow[] = [
  { work_id: "RPT-001", title: "《战神归来之凤唳九天》涉嫌血腥暴力", meta: "举报人：用户小王", author: "小王", authorInitial: "王", authorGrad: "from-red-400 to-orange-500", submitted: "2026-08-22 15:30", compliance: [{ label: "暴力", state: "fail" }], media_url: "", status: "PENDING", real: false },
  { work_id: "RPT-002", title: "《都市巅峰之逆袭归来》涉嫌抄袭", meta: "举报人：用户小李", author: "小李", authorInitial: "李", authorGrad: "from-blue-400 to-cyan-500", submitted: "2026-08-22 14:20", compliance: [{ label: "版权", state: "warn" }], media_url: "", status: "PENDING", real: false },
];

// 封面渐变池（按行序循环）
const COVER_GRADS = [
  "from-purple-600 to-pink-500",
  "from-blue-600 to-cyan-500",
  "from-rose-600 to-orange-500",
  "from-green-600 to-teal-500",
  "from-indigo-600 to-purple-500",
  "from-red-600 to-rose-500",
  "from-teal-600 to-cyan-500",
  "from-amber-600 to-yellow-500",
  "from-violet-600 to-purple-500",
  "from-pink-600 to-rose-500",
];

function ComplianceIcon({ state }: { state: ComplianceState }) {
  if (state === "pass") return <Check className="w-3 h-3 mr-0.5" />;
  if (state === "fail") return <X className="w-3 h-3 mr-0.5" />;
  return <AlertTriangle className="w-3 h-3 mr-0.5" />;
}

export default function AdminAuditPage() {
  const [tab, setTab] = useState<TabKey>("pending");
  const [rows, setRows] = useState<AuditRow[]>(FALLBACK_ROWS);
  const [isReal, setIsReal] = useState(false);
  const [total, setTotal] = useState(128);
  // 4 个 tab 的角标计数（真数据加载后覆盖，回退时保持设计稿静态值）
  const [tabTotals, setTabTotals] = useState<Record<TabKey, number>>({
    pending: 128,
    approved: 1247,
    rejected: 186,
    draft: 32,
    reports: 0, // A2: 举报初始计数
    logs: 0, // A3: 审计日志初始计数
  });
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [keyword, setKeyword] = useState("");
  const [view, setView] = useState<"list" | "grid">("list");
  const [jumpInput, setJumpInput] = useState("1");

  const load = useCallback(async (p: number, t: TabKey) => {
    try {
      if (t === "reports") {
        // A2: 举报页（GET /admin/reports）
        const resp = await api.get<{ items: { report_id: number; work_id: string; title: string; reason: string; status: string; created_at: string }[]; pagination: { total: number } }>(`/admin/reports?page=${p}&page_size=${PAGE_SIZE}`);
        const mapped: AuditRow[] = (resp.items || []).map((r: { report_id: number; work_id: string; title: string; reason: string; status: string; created_at: string }) => ({
          work_id: String(r.report_id), // 用 report_id 作为操作主键
          title: r.title || "未知作品",
          meta: r.reason,
          author: "举报用户",
          authorInitial: "举报",
          authorGrad: "",
          submitted: r.created_at,
          compliance: [{ label: r.reason, state: "fail" as ComplianceState }],
          media_url: "",
          status: r.status,
          real: true,
        }));
        setRows(mapped);
        setTotal(resp.pagination?.total || 0);
        setTabTotals((prev) => ({ ...prev, reports: resp.pagination?.total || 0 }));
        setIsReal(true);
        return;
      }
      if (t === "logs") {
        // A3: 审计日志页（GET /admin/audit-logs）
        const resp = await api.get<{ items: AuditRow[]; pagination: { total: number } }>(`/admin/audit-logs?page=${p}&page_size=${PAGE_SIZE}`);
        setRows(resp.items || []);
        setTotal(resp.pagination?.total || 0);
        setTabTotals((prev) => ({ ...prev, logs: resp.pagination?.total || 0 }));
        setIsReal(true);
        return;
      }
      const resp = await api.get<AuditResp>(`/audit/works?status=${TAB_STATUS[t]}&page=${p}&page_size=${PAGE_SIZE}`);
      const mapped: AuditRow[] = resp.items.map((w) => ({
        work_id: w.work_id,
        title: w.title || "未命名作品",
        meta: w.description || `播放 ${fmtNum(w.view_count || 0)} · 点赞 ${fmtNum(w.like_count || 0)}`,
        author: "—",
        authorInitial: "",
        authorGrad: "",
        submitted: w.updated_at || "—",
        compliance: [{ label: "无自动检测数据", state: "warn" as ComplianceState }],
        media_url: w.media_url || "",
        status: w.publish_status || TAB_STATUS[t],
        real: true,
      }));
      setRows(mapped);
      setTotal(resp.pagination.total);
      setTabTotals((prev) => ({ ...prev, [t]: resp.pagination.total }));
      setIsReal(true);
    } catch {
      if (t === "reports") {
        setRows(FALLBACK_REPORT_ROWS);
        setTotal(2);
      } else if (t === "logs") {
        setRows(FALLBACK_AUDIT_ROWS);
        setTotal(3);
      } else if (t === "pending") {
        setRows(FALLBACK_ROWS);
        setTotal(128);
      } else {
        setRows([]);
        setTotal(0);
      }
      setIsReal(false);
    }
  }, []);

  useEffect(() => {
    load(page, tab);
  }, [tab, page, load]);

  // 首次进入时并行拉 4 个 tab 的计数（page_size=1 轻量请求，避免未访问 tab 的角标停留在设计稿静态值）
  useEffect(() => {
    (Object.keys(TAB_STATUS) as TabKey[]).forEach((t) => {
      if (t === "logs") return; // 审计日志 tab 无需预取计数
      api
        .get<AuditResp>(`/audit/works?status=${TAB_STATUS[t]}&page=1&page_size=1`)
        .then((resp) => setTabTotals((prev) => ({ ...prev, [t]: resp.pagination.total })))
        .catch(() => {
          /* 保持设计稿静态值 */
        });
    });
  }, []);

  const switchTab = (t: TabKey) => {
    if (t === tab) return;
    setTab(t);
    setPage(1);
    setJumpInput("1");
    setSelected(new Set());
    setKeyword("");
    if (t === "logs") {
      setRows(FALLBACK_AUDIT_ROWS);
      setTotal(3);
    }
    if (t === "reports") {
      setRows(FALLBACK_REPORT_ROWS);
      setTotal(2);
    }
  };

  const filtered = useMemo(
    () => rows.filter((r) => !keyword || r.title.includes(keyword) || r.author.includes(keyword)),
    [rows, keyword],
  );

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const allChecked = filtered.length > 0 && filtered.every((r) => selected.has(r.work_id));

  const toggleAll = () => {
    setSelected(allChecked ? new Set() : new Set(filtered.map((r) => r.work_id)));
  };

  const toggleRow = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // 单条审核
  const review = async (row: AuditRow, approve: boolean) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.post(`/audit/works/${row.work_id}/review`, { approve, note: "" });
      setRows((prev) => prev.filter((r) => r.work_id !== row.work_id));
      setTabTotals((prev) => ({ ...prev, pending: Math.max(0, prev.pending - 1) }));
      setSelected((prev) => {
        const next = new Set(prev);
        next.delete(row.work_id);
        return next;
      });
      toast(approve ? `已通过《${row.title}》` : `已驳回《${row.title}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 批量审核
  const batchReview = async (approve: boolean) => {
    const targets = filtered.filter((r) => selected.has(r.work_id));
    if (targets.length === 0) {
      toast("请先选择作品", "error");
      return;
    }
    if (targets.some((t) => !t.real)) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await Promise.all(targets.map((t) => api.post(`/audit/works/${t.work_id}/review`, { approve, note: "" })));
      const ids = new Set(targets.map((t) => t.work_id));
      setRows((prev) => prev.filter((r) => !ids.has(r.work_id)));
      setTabTotals((prev) => ({ ...prev, pending: Math.max(0, prev.pending - targets.length) }));
      setSelected(new Set());
      toast(approve ? `已批量通过 ${targets.length} 部作品` : `已批量驳回 ${targets.length} 部作品`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  const preview = (row: AuditRow) => {
    if (row.media_url) {
      window.open(row.media_url, "_blank");
      return;
    }
    toast("预览功能规划中", "info");
  };

  // 已通过 tab：下架（PUBLISHED → REVOKED）
  const revokeWork = async (row: AuditRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.post(`/audit/works/${row.work_id}/revoke`, { note: "管理员下架" });
      setRows((prev) => prev.filter((r) => r.work_id !== row.work_id));
      setTabTotals((prev) => ({ ...prev, approved: Math.max(0, prev.approved - 1) }));
      toast(`已下架《${row.title}》`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  const gotoPage = (p: number) => {
    if (p < 1 || p > totalPages || !isReal) return;
    setPage(p);
    setSelected(new Set());
    setJumpInput(String(p));
  };

  const jumpTo = () => {
    const p = parseInt(jumpInput, 10);
    if (!Number.isNaN(p)) gotoPage(p);
  };

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

  return (
    <>
      {/* PAGE TITLE + STATS */}
      <div className="flex items-center justify-between mb-5">
        <div>
          <h1 className="text-2xl font-bold text-white mb-1">内容审核</h1>
          <p className="text-sm text-text-muted">审核平台提交的作品，确保内容合规与质量</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="stat-mini-card flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-green-500/15 flex items-center justify-center flex-shrink-0">
              <CheckCircle2 className="w-4 h-4 text-green-400" />
            </div>
            <div>
              <div className="text-lg font-bold text-green-400 font-mono leading-tight">86</div>
              <div className="text-[11px] text-text-muted">今日已通过</div>
            </div>
          </div>
          <div
            className="stat-mini-card flex items-center gap-3"
            style={{ background: "linear-gradient(135deg, rgba(239, 68, 68, 0.08) 0%, rgba(239, 68, 68, 0.02) 100%)", borderColor: "rgba(239, 68, 68, 0.15)" }}
          >
            <div className="w-8 h-8 rounded-lg bg-red-500/15 flex items-center justify-center flex-shrink-0">
              <XCircle className="w-4 h-4 text-red-400" />
            </div>
            <div>
              <div className="text-lg font-bold text-red-400 font-mono leading-tight">23</div>
              <div className="text-[11px] text-text-muted">今日已驳回</div>
            </div>
          </div>
          <div
            className="stat-mini-card flex items-center gap-3"
            style={{ background: "linear-gradient(135deg, rgba(59, 130, 246, 0.08) 0%, rgba(59, 130, 246, 0.02) 100%)", borderColor: "rgba(59, 130, 246, 0.15)" }}
          >
            <div className="w-8 h-8 rounded-lg bg-blue-500/15 flex items-center justify-center flex-shrink-0">
              <Clock3 className="w-4 h-4 text-blue-400" />
            </div>
            <div>
              <div className="text-lg font-bold text-blue-400 font-mono leading-tight">
                4.2<span className="text-xs font-normal">min</span>
              </div>
              <div className="text-[11px] text-text-muted">平均耗时</div>
            </div>
          </div>
        </div>
      </div>

      {/* TAB BAR CARD */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl mb-4 overflow-hidden">
        <div className="flex items-center border-b border-[#2A2A3E] px-2">
          <div className={`audit-tab ${tab === "pending" ? "active" : ""}`} onClick={() => switchTab("pending")}>
            待审核<span className="tab-badge">{fmtNum(tabTotals.pending)}</span>
          </div>
          <div className={`audit-tab ${tab === "approved" ? "active" : ""}`} onClick={() => switchTab("approved")}>
            已通过<span className="tab-badge">{fmtNum(tabTotals.approved)}</span>
          </div>
          <div className={`audit-tab ${tab === "rejected" ? "active" : ""}`} onClick={() => switchTab("rejected")}>
            已驳回<span className="tab-badge">{fmtNum(tabTotals.rejected)}</span>
          </div>
          <div className={`audit-tab ${tab === "draft" ? "active" : ""}`} onClick={() => switchTab("draft")}>
            草稿<span className="tab-badge">{fmtNum(tabTotals.draft)}</span>
          </div>
          <div className={`audit-tab ${tab === "reports" ? "active" : ""}`} onClick={() => switchTab("reports")}>
            举报<span className="tab-badge">{fmtNum(tabTotals.reports)}</span>
          </div>
          <div className={`audit-tab ${tab === "logs" ? "active" : ""}`} onClick={() => switchTab("logs")}>
            审计日志<span className="tab-badge">{fmtNum(tabTotals.logs)}</span>
          </div>
        </div>

        <>
            {/* BATCH TOOLBAR */}
            <div className="sticky top-[60px] z-[5] bg-bg-card border-b border-[#2A2A3E] px-4 py-2.5 flex items-center justify-between">
              <div className="flex items-center gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" className="custom-checkbox" checked={allChecked} onChange={toggleAll} />
                  <span className="text-sm text-text-secondary">全选</span>
                </label>
                <span className="text-sm text-text-secondary">
                  已选 <span className="text-amber-gold font-semibold">{selected.size}</span> 项
                </span>
              </div>

              <div className="flex items-center gap-2">
                {tab === "pending" && (
                  <>
                    <button
                      onClick={() => batchReview(true)}
                      className="flex items-center px-3 py-1.5 bg-green-500/10 border border-green-500/30 rounded-md text-xs text-green-400 hover:bg-green-500/20 hover:border-green-500/50 transition-colors"
                    >
                      <CheckCircle2 className="w-3.5 h-3.5 mr-1.5" />
                      批量通过
                      <kbd className="ml-2 px-1.5 py-0.5 text-[10px] bg-green-500/10 rounded text-green-400/70">Shift+A</kbd>
                    </button>
                    <button
                      onClick={() => batchReview(false)}
                      className="flex items-center px-3 py-1.5 bg-red-500/10 border border-red-500/30 rounded-md text-xs text-red-400 hover:bg-red-500/20 hover:border-red-500/50 transition-colors"
                    >
                      <XCircle className="w-3.5 h-3.5 mr-1.5" />
                      批量驳回
                      <kbd className="ml-2 px-1.5 py-0.5 text-[10px] bg-red-500/10 rounded text-red-400/70">Shift+R</kbd>
                    </button>
                  </>
                )}
                <button
                  onClick={() =>
                    exportCSV(
                      `审核-${TAB_LABEL[tab]}`,
                      ["作品ID", "标题", "作者", "状态", "提交时间"],
                      filtered.map((r) => [r.work_id, r.title, r.author, STATUS_TAGS[r.status]?.label ?? r.status, r.submitted]),
                    )
                  }
                  className="flex items-center px-3 py-1.5 bg-bg-card-light border border-[#2A2A3E] rounded-md text-xs text-text-secondary hover:border-amber-gold/50 hover:text-amber-gold transition-colors"
                >
                  <Download className="w-3.5 h-3.5 mr-1.5" />
                  批量导出
                </button>
              </div>

              <div className="flex items-center gap-3">
                <div className="relative">
                  <Search className="w-4 h-4 text-text-muted absolute left-3 top-1/2 -translate-y-1/2" />
                  <input
                    type="text"
                    placeholder="搜索作品/作者..."
                    value={keyword}
                    onChange={(e) => setKeyword(e.target.value)}
                    className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg pl-9 pr-4 py-1.5 text-xs text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 w-48 transition-colors"
                  />
                </div>
                <div className="flex items-center">
                  <button className={`view-btn ${view === "list" ? "active" : ""}`} title="列表视图" onClick={() => setView("list")}>
                    <List className="w-4 h-4" />
                  </button>
                  <button
                    className={`view-btn ${view === "grid" ? "active" : ""}`}
                    title="卡片视图"
                    onClick={() => {
                      setView("grid");
                      toast("卡片视图规划中", "info");
                    }}
                  >
                    <LayoutGrid className="w-4 h-4" />
                  </button>
                </div>
                <button
                  onClick={() => toast("列设置功能规划中", "info")}
                  className="w-8 h-8 flex items-center justify-center border border-[#2A2A3E] rounded-lg bg-bg-card-light text-text-secondary hover:border-amber-gold/50 hover:text-amber-gold transition-colors"
                  title="列设置"
                >
                  <Settings2 className="w-4 h-4" />
                </button>
              </div>
            </div>

            {/* AUDIT TABLE */}
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="bg-bg-card-light border-b border-[#2A2A3E]">
                    <th className="px-4 py-3 text-left w-12">
                      <input type="checkbox" className="custom-checkbox" checked={allChecked} onChange={toggleAll} />
                    </th>
                    <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider w-16">封面</th>
                    <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">作品名称</th>
                    <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">作者</th>
                    <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">提交时间</th>
                    <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">自动合规检测</th>
                    <th className="px-3 py-3 text-center text-xs font-medium text-text-muted uppercase tracking-wider">审核状态</th>
                    <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">审核员</th>
                    <th className="px-3 py-3 text-center text-xs font-medium text-text-muted uppercase tracking-wider">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#2A2A3E]">
                  {filtered.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="px-4 py-10 text-center text-sm text-text-muted">
                        暂无{TAB_LABEL[tab]}作品
                      </td>
                    </tr>
                  ) : (
                    filtered.map((r, i) => (
                      <tr key={r.work_id} className="table-row transition-colors h-16">
                        <td className="px-4 py-0">
                          <input
                            type="checkbox"
                            className="custom-checkbox"
                            checked={selected.has(r.work_id)}
                            onChange={() => toggleRow(r.work_id)}
                          />
                        </td>
                        <td className="px-3 py-0">
                          <div className={`w-10 h-14 rounded bg-gradient-to-br ${COVER_GRADS[i % COVER_GRADS.length]} flex items-center justify-center flex-shrink-0 overflow-hidden`}>
                            <Play className="w-4 h-4 text-white/80" />
                          </div>
                        </td>
                        <td className="px-3 py-0">
                          <div className="text-sm text-white font-medium truncate max-w-[200px]">{r.title}</div>
                          <div className="text-xs text-text-muted mt-0.5">{r.meta}</div>
                        </td>
                        <td className="px-3 py-0">
                          {r.author === "—" ? (
                            <span className="text-sm text-text-muted">—</span>
                          ) : (
                            <div className="flex items-center">
                              <div className={`w-7 h-7 rounded-full bg-gradient-to-br ${r.authorGrad} flex-shrink-0 flex items-center justify-center text-white text-xs font-bold`}>
                                {r.authorInitial}
                              </div>
                              <span className="ml-2 text-sm text-text-secondary">{r.author}</span>
                            </div>
                          )}
                        </td>
                        <td className="px-3 py-0 text-sm text-text-secondary">{r.submitted}</td>
                        <td className="px-3 py-0">
                          <div className="flex items-center gap-1.5 flex-wrap">
                            {r.compliance.map((c) => (
                              <span
                                key={c.label}
                                className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium compliance-${c.state === "pass" ? "pass" : c.state === "fail" ? "fail" : "warn"}`}
                              >
                                <ComplianceIcon state={c.state} />
                                {c.label}
                              </span>
                            ))}
                          </div>
                        </td>
                        <td className="px-3 py-0 text-center">
                          <span
                            className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                              STATUS_TAGS[r.status]?.cls || "tag-pending"
                            }`}
                          >
                            {STATUS_TAGS[r.status]?.label || r.status}
                          </span>
                        </td>
                        <td className="px-3 py-0 text-sm text-text-muted">—</td>
                        <td className="px-3 py-0">
                          <div className="flex items-center justify-center gap-2">
                            <button className="btn-preview text-xs flex items-center" onClick={() => preview(r)}>
                              <Eye className="w-3.5 h-3.5 mr-1" />
                              预览
                            </button>
                            {tab === "pending" && (
                              <>
                                <button className="btn-pass px-2 py-1 rounded text-xs font-medium flex items-center" onClick={() => review(r, true)}>
                                  <Check className="w-3.5 h-3.5 mr-0.5" />
                                  通过
                                </button>
                                <button className="btn-reject px-2 py-1 rounded text-xs font-medium flex items-center" onClick={() => review(r, false)}>
                                  <X className="w-3.5 h-3.5 mr-0.5" />
                                  驳回
                                </button>
                              </>
                            )}
                            {tab === "approved" && r.status === "PUBLISHED" && (
                              <button className="btn-reject px-2 py-1 rounded text-xs font-medium flex items-center" onClick={() => revokeWork(r)}>
                                <Ban className="w-3.5 h-3.5 mr-0.5" />
                                下架
                              </button>
                            )}
                          </div>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {/* PAGINATION */}
            <div className="flex items-center justify-between px-4 py-3 border-t border-[#2A2A3E]">
              <div className="text-sm text-text-muted">
                显示第 <span className="text-white">{isReal ? `${rangeStart}-${rangeEnd}` : "1-10"}</span> 条，共 <span className="text-white">{fmtNum(total)}</span> 条
                {TAB_LABEL[tab]}
              </div>
              <div className="flex items-center gap-2">
                <button className="page-btn" disabled={page <= 1 || !isReal} onClick={() => gotoPage(page - 1)}>
                  <ChevronLeft className="w-4 h-4" />
                </button>
                {pageBtns.map((n, idx) =>
                  n === "..." ? (
                    <span key={`dot-${idx}`} className="text-text-muted text-sm px-1">
                      ...
                    </span>
                  ) : (
                    <button key={n} className={`page-btn ${n === page ? "active" : ""}`} onClick={() => gotoPage(n)}>
                      {n}
                    </button>
                  ),
                )}
                <button className="page-btn" disabled={page >= totalPages || !isReal} onClick={() => gotoPage(page + 1)}>
                  <ChevronRight className="w-4 h-4" />
                </button>
                <div className="flex items-center gap-2 ml-3">
                  <span className="text-xs text-text-muted">跳至</span>
                  <input
                    type="number"
                    value={jumpInput}
                    onChange={(e) => setJumpInput(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && jumpTo()}
                    className="w-12 h-8 bg-bg-card border border-[#2A2A3E] rounded-md text-center text-sm text-white focus:outline-none focus:border-amber-gold/50"
                  />
                  <span className="text-xs text-text-muted">页</span>
                </div>
              </div>
            </div>
          </>
      </div>

      {/* Footer */}
      <div className="mt-8 pt-4 border-t border-[#2A2A3E] text-center">
        <p className="text-xs text-text-muted">导演台 Admin v1.0 · © 2025 导演台创作平台</p>
      </div>
    </>
  );
}
