// AdminUsersPage：迁移自 admin-users.html（DOM 1:1）。
// 数据策略：GET /admin/users 真数据（缺字段显示 "—"），封禁/解封走 PATCH /admin/users/:uuid/ban；
// 接口失败回退设计稿静态 10 行（示例数据不可操作）。
// 管理弹窗（设计稿静态链接，无 DOM 结构，按管理端视觉自建）：
//   查看详情 GET /admin/users/:uuid（含积分/会员）；会员设置 PUT /users/:uuid/membership；
//   分配角色 PUT /users/:uuid/roles（全量替换语义）；调整积分 PUT /users/:uuid/points（单个/批量）。
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@duanju/api-client";
import {
  Download,
  Coins,
  Plus,
  Search,
  RotateCcw,
  Ban,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";

import { toast, errMessage, fmtNum, exportCSV } from "../lib";
import Modal from "../components/Modal";
import "./AdminUsersPage.css";

type UserStatus = "normal" | "banned" | "pending";

interface UserRow {
  user_uuid: string;
  nickname: string;
  email: string;
  phone: string;
  created_at: string;
  last_login: string;
  vipLabel: string;
  vipCls: string; // vip-* 渐变条类名，空则显示 "—"
  points: string;
  status: UserStatus;
  grad: string; // 头像渐变
  real: boolean; // 是否真实数据（回退静态行不可操作）
}

interface BackendUser {
  user_uuid: string;
  nickname: string;
  email: string;
  status: string;
  created_at: string;
}

interface UsersResp {
  items: BackendUser[];
  pagination: { page: number; page_size: number; total: number };
}

// 用户详情（GET /admin/users/:uuid）
interface UserDetail {
  user_uuid: string;
  nickname: string;
  email: string;
  phone: string;
  status: string;
  created_at: string;
  points: { current_balance: number; frozen_balance: number };
  membership_plan: string; // BASIC/PRO/ENTERPRISE/ULTIMATE，空 = 无套餐
}

// 角色列表项（GET /admin/roles）
interface RoleItem {
  role_code: string;
  name: string;
  description: string;
}

// 会员套餐可选值（与后端 user_memberships.plan_code 约定一致）
const PLAN_OPTIONS = [
  { code: "BASIC", label: "基础会员 BASIC" },
  { code: "PRO", label: "进阶会员 PRO" },
  { code: "ENTERPRISE", label: "高级会员 ENTERPRISE" },
  { code: "ULTIMATE", label: "旗舰会员 ULTIMATE" },
] as const;
const PLAN_LABEL: Record<string, string> = {
  BASIC: "基础会员",
  PRO: "进阶会员",
  ENTERPRISE: "高级会员",
  ULTIMATE: "旗舰会员",
};

const PAGE_SIZE = 10;

// 头像渐变色池（按行序循环）
const GRADS = [
  "from-amber-400 to-orange-500",
  "from-pink-400 to-rose-500",
  "from-cyan-400 to-blue-500",
  "from-green-400 to-emerald-500",
  "from-purple-400 to-violet-500",
  "from-red-400 to-orange-500",
  "from-indigo-400 to-purple-500",
  "from-teal-400 to-cyan-500",
  "from-rose-400 to-pink-500",
  "from-yellow-400 to-amber-500",
];

// 设计稿静态回退行（接口失败时展示）
const FALLBACK_ROWS: UserRow[] = [
  { user_uuid: "U100001", nickname: "李逍遥", email: "lixiaoyao@email.com", phone: "138****8888", created_at: "2024-03-15 14:32", last_login: "2025-01-20 09:15", vipLabel: "旗舰会员", vipCls: "vip-flagship", points: "128,500", status: "normal", grad: GRADS[0], real: false },
  { user_uuid: "U100002", nickname: "周芷若", email: "zhouzhiruo@email.com", phone: "139****6666", created_at: "2024-04-02 10:18", last_login: "2025-01-20 08:42", vipLabel: "高级会员", vipCls: "vip-premium", points: "56,200", status: "normal", grad: GRADS[1], real: false },
  { user_uuid: "U100003", nickname: "王语嫣", email: "wangyuyan@email.com", phone: "137****5555", created_at: "2024-05-20 16:45", last_login: "2025-01-19 22:10", vipLabel: "高级会员", vipCls: "vip-premium", points: "42,800", status: "normal", grad: GRADS[2], real: false },
  { user_uuid: "U100004", nickname: "陈长生", email: "chenchangsheng@email.com", phone: "136****4444", created_at: "2024-06-10 09:22", last_login: "2025-01-20 07:30", vipLabel: "进阶会员", vipCls: "vip-advanced", points: "18,600", status: "normal", grad: GRADS[3], real: false },
  { user_uuid: "U100005", nickname: "黄蓉", email: "huangrong@email.com", phone: "135****3333", created_at: "2024-07-08 11:56", last_login: "2025-01-18 15:20", vipLabel: "进阶会员", vipCls: "vip-advanced", points: "15,400", status: "pending", grad: GRADS[4], real: false },
  { user_uuid: "U100006", nickname: "郭靖", email: "guojing@email.com", phone: "134****2222", created_at: "2024-08-15 13:40", last_login: "2025-01-19 18:05", vipLabel: "基础会员", vipCls: "vip-basic", points: "6,800", status: "normal", grad: GRADS[5], real: false },
  { user_uuid: "U100007", nickname: "杨过", email: "yangguo@email.com", phone: "133****1111", created_at: "2024-09-03 08:15", last_login: "2025-01-17 20:45", vipLabel: "基础会员", vipCls: "vip-basic", points: "4,200", status: "banned", grad: GRADS[6], real: false },
  { user_uuid: "U100008", nickname: "小龙女", email: "xiaolongnv@email.com", phone: "132****9999", created_at: "2024-10-22 17:28", last_login: "2025-01-20 06:50", vipLabel: "免费用户", vipCls: "vip-free", points: "500", status: "normal", grad: GRADS[7], real: false },
  { user_uuid: "U100009", nickname: "乔峰", email: "qiaofeng@email.com", phone: "131****7777", created_at: "2024-11-30 20:10", last_login: "2025-01-16 11:30", vipLabel: "免费用户", vipCls: "vip-free", points: "200", status: "pending", grad: GRADS[8], real: false },
  { user_uuid: "U100010", nickname: "段誉", email: "duanyu@email.com", phone: "130****6666", created_at: "2024-12-18 12:05", last_login: "2025-01-19 09:55", vipLabel: "旗舰会员", vipCls: "vip-flagship", points: "99,999", status: "normal", grad: GRADS[9], real: false },
];

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso || "—";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

const STATUS_TAG: Record<UserStatus, { cls: string; label: string }> = {
  normal: { cls: "tag-normal", label: "正常" },
  banned: { cls: "tag-banned", label: "封禁" },
  pending: { cls: "tag-pending", label: "待实名" },
};

export default function AdminUsersPage() {
  const [rows, setRows] = useState<UserRow[]>(FALLBACK_ROWS);
  const [total, setTotal] = useState(12847);
  const [page, setPage] = useState(1);
  const [isReal, setIsReal] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // 过滤条件（后端仅支持分页，过滤在当前页数据上执行）
  const [keywordInput, setKeywordInput] = useState("");
  const [statusFilter, setStatusFilter] = useState("全部状态");
  const [vipFilter, setVipFilter] = useState("全部等级");
  const [keyword, setKeyword] = useState("");
  const [appliedStatus, setAppliedStatus] = useState("全部状态");
  const [appliedVip, setAppliedVip] = useState("全部等级");

  const load = useCallback(async (p: number) => {
    try {
      const resp = await api.get<UsersResp>(`/admin/users?page=${p}&page_size=${PAGE_SIZE}`);
      const mapped: UserRow[] = resp.items.map((u, i) => ({
        user_uuid: u.user_uuid,
        nickname: u.nickname || "—",
        email: u.email || "—",
        phone: "—",
        created_at: fmtTime(u.created_at),
        last_login: "—",
        vipLabel: "",
        vipCls: "",
        points: "—",
        status: u.status === "banned" ? "banned" : "normal",
        grad: GRADS[i % GRADS.length],
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
  }, []);

  useEffect(() => {
    load(page);
  }, [page, load]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const filtered = useMemo(
    () =>
      rows.filter((r) => {
        if (keyword && !r.nickname.includes(keyword) && !r.email.includes(keyword)) return false;
        if (appliedStatus !== "全部状态" && STATUS_TAG[r.status].label !== appliedStatus) return false;
        if (appliedVip !== "全部等级" && r.vipLabel !== appliedVip) return false;
        return true;
      }),
    [rows, keyword, appliedStatus, appliedVip],
  );

  const allChecked = filtered.length > 0 && filtered.every((r) => selected.has(r.user_uuid));

  const toggleAll = () => {
    setSelected(allChecked ? new Set() : new Set(filtered.map((r) => r.user_uuid)));
  };

  const toggleRow = (uuid: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(uuid)) next.delete(uuid);
      else next.add(uuid);
      return next;
    });
  };

  // 封禁 / 解封
  const setBan = async (row: UserRow, banned: boolean) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.patch(`/admin/users/${row.user_uuid}/ban`, { banned });
      setRows((prev) => prev.map((r) => (r.user_uuid === row.user_uuid ? { ...r, status: banned ? "banned" : "normal" } : r)));
      toast(banned ? `已封禁 ${row.nickname}` : `已解封 ${row.nickname}`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 批量封禁（仅真实数据）
  const batchBan = async () => {
    const targets = filtered.filter((r) => selected.has(r.user_uuid));
    if (targets.length === 0) {
      toast("请先选择用户", "error");
      return;
    }
    if (targets.some((t) => !t.real)) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await Promise.all(targets.map((t) => api.patch(`/admin/users/${t.user_uuid}/ban`, { banned: true })));
      setRows((prev) => prev.map((r) => (selected.has(r.user_uuid) ? { ...r, status: "banned" as UserStatus } : r)));
      setSelected(new Set());
      toast(`已批量封禁 ${targets.length} 个用户`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // ---------- 管理弹窗 ----------

  // 详情弹窗：目标行 + 拉取的详情 + 角色可选项
  const [detailRow, setDetailRow] = useState<UserRow | null>(null);
  const [detailData, setDetailData] = useState<UserDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [roles, setRoles] = useState<RoleItem[]>([]);
  const [rolesLoaded, setRolesLoaded] = useState(false);
  const [checkedRoles, setCheckedRoles] = useState<Set<string>>(new Set());

  // 新增用户表单（POST /admin/users）
  const [createOpen, setCreateOpen] = useState(false);
  const [createEmail, setCreateEmail] = useState("");
  const [createPwd, setCreatePwd] = useState("");
  const [createNick, setCreateNick] = useState("");
  const [createPoints, setCreatePoints] = useState("0");
  const [createRoles, setCreateRoles] = useState<Set<string>>(new Set());
  const [creating, setCreating] = useState(false);

  const submitCreate = async () => {
    if (!createEmail.trim() || !createPwd || !createNick.trim()) {
      toast("请填写邮箱、密码与昵称", "error");
      return;
    }
    if (createPwd.length < 8) {
      toast("密码至少 8 位", "error");
      return;
    }
    setCreating(true);
    try {
      await api.post("/admin/users", {
        email: createEmail.trim(),
        password: createPwd,
        nickname: createNick.trim(),
        points: Number(createPoints) || 0,
        role_codes: Array.from(createRoles),
      });
      toast(`已创建用户 ${createNick.trim()}`, "success");
      setCreateOpen(false);
      setCreateEmail("");
      setCreatePwd("");
      setCreateNick("");
      setCreatePoints("0");
      setCreateRoles(new Set());
      setPage(1);
      void load(1);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setCreating(false);
    }
  };

  // 会员设置表单
  const [planCode, setPlanCode] = useState("BASIC");
  const [planDays, setPlanDays] = useState("30");
  const [savingPlan, setSavingPlan] = useState(false);

  // 积分调整弹窗（单个 / 批量共用）
  const [pointsTargets, setPointsTargets] = useState<UserRow[] | null>(null);
  const [pointsAmount, setPointsAmount] = useState("");
  const [pointsReason, setPointsReason] = useState("");
  const [savingPoints, setSavingPoints] = useState(false);

  const loadDetail = useCallback(async (row: UserRow) => {
    setDetailRow(row);
    setDetailData(null);
    setDetailLoading(true);
    try {
      const d = await api.get<UserDetail>(`/admin/users/${row.user_uuid}`);
      setDetailData(d);
    } catch (e) {
      toast(errMessage(e), "error");
      setDetailRow(null);
    } finally {
      setDetailLoading(false);
    }
  }, []);

  // 角色列表懒加载：详情弹窗或新增用户弹窗打开时拉取（供角色勾选）
  useEffect(() => {
    if ((!detailRow && !createOpen) || rolesLoaded) return;
    api
      .get<{ items: RoleItem[] }>("/admin/roles")
      .then((r) => {
        setRoles(r.items ?? []);
        setRolesLoaded(true);
      })
      .catch(() => {
        /* 角色拉取失败不阻塞展示，提交时再报错 */
      });
  }, [detailRow, createOpen, rolesLoaded]);

  const openDetail = (row: UserRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    setCheckedRoles(new Set());
    setPlanCode("BASIC");
    setPlanDays("30");
    void loadDetail(row);
  };

  // 设置会员：PUT /admin/users/:uuid/membership（后端为替换语义：旧 ACTIVE 套餐置 EXPIRED）
  const submitMembership = async () => {
    if (!detailRow || !detailData) return;
    const days = Number(planDays);
    if (!Number.isInteger(days) || days < 1 || days > 3650) {
      toast("有效天数需为 1-3650 的整数", "error");
      return;
    }
    setSavingPlan(true);
    try {
      await api.put(`/admin/users/${detailRow.user_uuid}/membership`, {
        plan_code: planCode,
        days,
        renew_strategy: "NONE",
      });
      toast(`已为 ${detailData.nickname} 开通 ${PLAN_LABEL[planCode] ?? planCode}（${days} 天）`, "success");
      void loadDetail(detailRow); // 刷新详情确认生效
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingPlan(false);
    }
  };

  // 分配角色：PUT /admin/users/:uuid/roles（全量替换语义：未勾选的现有角色会被移除）
  const submitRoles = async () => {
    if (!detailRow || !detailData) return;
    if (checkedRoles.size === 0) {
      toast("请至少勾选一个角色", "error");
      return;
    }
    try {
      await api.put(`/admin/users/${detailRow.user_uuid}/roles`, {
        role_codes: Array.from(checkedRoles),
      });
      toast(`已更新 ${detailData.nickname} 的平台角色`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 调整积分：单个/批量共用一个弹窗，正数增加、负数扣减
  const openPoints = (targets: UserRow[]) => {
    const realTargets = targets.filter((t) => t.real);
    if (realTargets.length === 0) {
      toast("示例数据不可操作", "error");
      return;
    }
    if (realTargets.length !== targets.length) {
      toast("已忽略示例数据行", "info");
    }
    setPointsTargets(realTargets);
    setPointsAmount("");
    setPointsReason("");
  };

  // 顶部「调整积分」：无选中时提示，有选中时对选中行批量调整
  const openPointsForSelection = () => {
    const targets = filtered.filter((r) => selected.has(r.user_uuid));
    if (targets.length === 0) {
      toast("请先勾选要调整积分的用户", "error");
      return;
    }
    openPoints(targets);
  };

  const submitPoints = async () => {
    if (!pointsTargets) return;
    const amount = Number(pointsAmount);
    if (!Number.isInteger(amount) || amount === 0) {
      toast("调整值需为非零整数（负数为扣减）", "error");
      return;
    }
    const reason = pointsReason.trim();
    if (reason.length < 1 || reason.length > 200) {
      toast("请填写 1-200 字的调整原因", "error");
      return;
    }
    setSavingPoints(true);
    try {
      await Promise.all(
        pointsTargets.map((t) =>
          api.put(`/admin/users/${t.user_uuid}/points`, { amount, reason }),
        ),
      );
      toast(
        pointsTargets.length === 1
          ? `已调整 ${pointsTargets[0].nickname} 的积分（${amount > 0 ? "+" : ""}${fmtNum(amount)}）`
          : `已批量调整 ${pointsTargets.length} 个用户的积分`,
        "success",
      );
      setPointsTargets(null);
      // 详情弹窗若正展示同一用户，刷新其积分展示
      if (detailRow && pointsTargets.some((t) => t.user_uuid === detailRow.user_uuid)) {
        void loadDetail(detailRow);
      }
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingPoints(false);
    }
  };

  const applyFilter = () => {
    setKeyword(keywordInput.trim());
    setAppliedStatus(statusFilter);
    setAppliedVip(vipFilter);
  };

  const resetFilter = () => {
    setKeywordInput("");
    setStatusFilter("全部状态");
    setVipFilter("全部等级");
    setKeyword("");
    setAppliedStatus("全部状态");
    setAppliedVip("全部等级");
  };

  const gotoPage = (p: number) => {
    if (p < 1 || p > totalPages) return;
    if (!isReal) return; // 示例数据无分页
    setPage(p);
    setSelected(new Set());
  };

  const rangeStart = (page - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(page * PAGE_SIZE, total);

  return (
    <>
      {/* Page title */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white mb-1">用户管理</h1>
          <p className="text-sm text-text-muted">管理平台所有注册用户，查看用户详情，调整用户权限与积分</p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() =>
              exportCSV(
                "用户管理",
                ["用户ID", "昵称", "邮箱", "状态", "注册时间"],
                rows.map((r) => [r.user_uuid, r.nickname, r.email, String(r.status), r.created_at]),
              )
            }
            className="flex items-center px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-amber-gold/50 hover:text-white transition-colors"
          >
            <Download className="w-4 h-4 mr-2" />
            导出 Excel
          </button>
          <button
            onClick={openPointsForSelection}
            className="flex items-center px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-amber-gold/50 hover:text-white transition-colors"
          >
            <Coins className="w-4 h-4 mr-2" />
            调整积分
          </button>
          <button
            onClick={() => setCreateOpen(true)}
            className="flex items-center px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Plus className="w-4 h-4 mr-2" />
            新增用户
          </button>
        </div>
      </div>

      {/* ====== FILTER SECTION ====== */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-4 mb-4">
        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative flex-1 min-w-[200px]">
            <Search className="w-4 h-4 text-text-muted absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="输入昵称 / 邮箱 / 手机号搜索"
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg pl-9 pr-4 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
            />
          </div>

          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors w-[120px]"
          >
            <option>全部状态</option>
            <option>正常</option>
            <option>封禁</option>
            <option>待实名</option>
          </select>

          <select
            value={vipFilter}
            onChange={(e) => setVipFilter(e.target.value)}
            className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors w-[130px]"
          >
            <option>全部等级</option>
            <option>免费用户</option>
            <option>基础会员</option>
            <option>进阶会员</option>
            <option>高级会员</option>
            <option>旗舰会员</option>
          </select>

          <div className="flex items-center gap-2">
            <input type="date" className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors" />
            <span className="text-text-muted text-sm">至</span>
            <input type="date" className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors" />
          </div>

          <button
            onClick={applyFilter}
            className="flex items-center px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Search className="w-4 h-4 mr-2" />
            搜索
          </button>

          <button
            onClick={resetFilter}
            className="flex items-center px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
          >
            <RotateCcw className="w-4 h-4 mr-2" />
            重置
          </button>
        </div>
      </div>

      {/* ====== TABLE TOOLBAR ====== */}
      <div className="flex items-center justify-between mb-3 px-1">
        <div className="flex items-center gap-4">
          <span className="text-sm text-text-secondary">
            已选 <span className="text-amber-gold font-semibold">{selected.size}</span> 项
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={batchBan}
              className="flex items-center px-3 py-1.5 bg-bg-card border border-[#2A2A3E] rounded-md text-xs text-text-secondary hover:border-red-500/50 hover:text-red-400 transition-colors"
            >
              <Ban className="w-3.5 h-3.5 mr-1.5" />
              批量封禁
            </button>
            <button
              onClick={openPointsForSelection}
              className="flex items-center px-3 py-1.5 bg-bg-card border border-[#2A2A3E] rounded-md text-xs text-text-secondary hover:border-amber-gold/50 hover:text-amber-gold transition-colors"
            >
              <Coins className="w-3.5 h-3.5 mr-1.5" />
              批量调整积分
            </button>
            <button
              onClick={() => toast("批量导出功能规划中", "info")}
              className="flex items-center px-3 py-1.5 bg-bg-card border border-[#2A2A3E] rounded-md text-xs text-text-secondary hover:border-green-500/50 hover:text-green-400 transition-colors"
            >
              <Download className="w-3.5 h-3.5 mr-1.5" />
              批量导出
            </button>
          </div>
        </div>
        <div className="text-sm text-text-muted">
          共 <span className="text-white font-medium">{fmtNum(total)}</span> 条记录
        </div>
      </div>

      {/* ====== USER TABLE ====== */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="bg-bg-card-light border-b border-[#2A2A3E]">
                <th className="px-4 py-3 text-left w-12">
                  <input type="checkbox" className="custom-checkbox" checked={allChecked} onChange={toggleAll} />
                </th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">用户</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">用户ID</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">邮箱</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">手机号</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">注册时间</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">最后登录</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">VIP等级</th>
                <th className="px-3 py-3 text-right text-xs font-medium text-text-muted uppercase tracking-wider">积分余额</th>
                <th className="px-3 py-3 text-center text-xs font-medium text-text-muted uppercase tracking-wider">状态</th>
                <th className="px-3 py-3 text-center text-xs font-medium text-text-muted uppercase tracking-wider">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#2A2A3E]">
              {filtered.map((r) => {
                const tag = STATUS_TAG[r.status];
                const initial = r.nickname.slice(0, 1).toUpperCase();
                const hasVip = r.vipCls !== "";
                return (
                  <tr key={r.user_uuid} className="table-row transition-colors h-12">
                    <td className="px-4 py-0">
                      <input
                        type="checkbox"
                        className="custom-checkbox"
                        checked={selected.has(r.user_uuid)}
                        onChange={() => toggleRow(r.user_uuid)}
                      />
                    </td>
                    <td className="px-3 py-0">
                      <div className="flex items-center">
                        <div className={`w-8 h-8 rounded-full bg-gradient-to-br ${r.grad} flex-shrink-0 flex items-center justify-center text-white text-xs font-bold`}>
                          {initial}
                        </div>
                        <span className="ml-3 text-sm text-white">{r.nickname}</span>
                      </div>
                    </td>
                    <td className="px-3 py-0 text-sm text-text-secondary font-mono">{r.user_uuid.slice(0, 8)}</td>
                    <td className="px-3 py-0 text-sm text-text-secondary">{r.email}</td>
                    <td className="px-3 py-0 text-sm text-text-secondary font-mono">{r.phone}</td>
                    <td className="px-3 py-0 text-sm text-text-secondary">{r.created_at}</td>
                    <td className="px-3 py-0 text-sm text-text-secondary">{r.last_login}</td>
                    <td className="px-3 py-0">
                      {hasVip ? (
                        <div className="flex items-center">
                          <div className={`w-1.5 h-5 rounded-full ${r.vipCls} mr-2`}></div>
                          <span className={`text-xs ${r.vipCls === "vip-free" ? "text-text-secondary" : "text-white"}`}>{r.vipLabel}</span>
                        </div>
                      ) : (
                        <span className="text-sm text-text-secondary">—</span>
                      )}
                    </td>
                    <td className={`px-3 py-0 text-sm text-right font-mono ${r.points === "—" ? "text-text-secondary" : "text-amber-gold font-semibold"}`}>
                      {r.points}
                    </td>
                    <td className="px-3 py-0 text-center">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${tag.cls}`}>{tag.label}</span>
                    </td>
                    <td className="px-3 py-0">
                      <div className="flex items-center justify-center gap-3 text-xs">
                        <a href="#" onClick={(e) => { e.preventDefault(); openDetail(r); }} className="action-link">
                          查看详情
                        </a>
                        <a href="#" onClick={(e) => { e.preventDefault(); openPoints([r]); }} className="action-link">
                          调整积分
                        </a>
                        {r.status === "banned" ? (
                          <a href="#" onClick={(e) => { e.preventDefault(); setBan(r, false); }} className="action-link" style={{ color: "#22c55e" }}>
                            解封
                          </a>
                        ) : (
                          <a href="#" onClick={(e) => { e.preventDefault(); setBan(r, true); }} className="action-link-danger">
                            封禁
                          </a>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        {/* ====== PAGINATION ====== */}
        <div className="flex items-center justify-between px-4 py-3 border-t border-[#2A2A3E]">
          <div className="text-sm text-text-muted">
            显示第 <span className="text-white">{isReal ? `${rangeStart}-${rangeEnd}` : "1-10"}</span> 条，共{" "}
            <span className="text-white">{fmtNum(total)}</span> 条
          </div>
          <div className="flex items-center gap-2">
            <button className="page-btn" disabled={page <= 1} onClick={() => gotoPage(page - 1)}>
              <ChevronLeft className="w-4 h-4" />
            </button>
            {Array.from({ length: Math.min(5, totalPages) }, (_, i) => i + 1).map((p) => (
              <button key={p} className={`page-btn${p === page ? " active" : ""}`} onClick={() => gotoPage(p)}>
                {p}
              </button>
            ))}
            {totalPages > 5 && (
              <>
                <span className="text-text-muted text-sm px-1">...</span>
                <button className={`page-btn${totalPages === page ? " active" : ""}`} onClick={() => gotoPage(totalPages)}>
                  {totalPages}
                </button>
              </>
            )}
            <button className="page-btn" disabled={page >= totalPages} onClick={() => gotoPage(page + 1)}>
              <ChevronRight className="w-4 h-4" />
            </button>
            <div className="flex items-center gap-2 ml-3">
              <span className="text-xs text-text-muted">跳至</span>
              <input
                type="number"
                value={page}
                min={1}
                max={totalPages}
                onChange={(e) => {
                  const v = Number(e.target.value);
                  if (Number.isInteger(v)) gotoPage(v);
                }}
                className="w-12 h-8 bg-bg-card border border-[#2A2A3E] rounded-md text-center text-sm text-white focus:outline-none focus:border-amber-gold/50"
              />
              <span className="text-xs text-text-muted">页</span>
            </div>
          </div>
        </div>
      </div>

      {/* ====== 用户详情弹窗（含会员设置 / 角色分配） ====== */}
      <Modal
        open={detailRow !== null}
        onClose={() => setDetailRow(null)}
        title="用户详情"
        subtitle={detailRow ? `${detailRow.nickname} · ${detailRow.user_uuid.slice(0, 8)}` : undefined}
        width={560}
      >
        {detailLoading || !detailData ? (
          <div className="py-10 text-center text-sm text-text-muted">加载中…</div>
        ) : (
          <>
            {/* 基础信息 */}
            <div className="grid grid-cols-2 gap-x-6 gap-y-3 mb-6">
              {([
                ["昵称", detailData.nickname || "—"],
                ["邮箱", detailData.email || "—"],
                ["手机号", detailData.phone || "—"],
                ["注册时间", fmtTime(detailData.created_at)],
                ["账号状态", detailData.status || "—"],
                ["当前会员", detailData.membership_plan ? `${PLAN_LABEL[detailData.membership_plan] ?? detailData.membership_plan}（${detailData.membership_plan}）` : "无套餐"],
              ] as const).map(([k, v]) => (
                <div key={k}>
                  <div className="text-xs text-text-muted mb-1">{k}</div>
                  <div className="text-sm text-white break-all">{v}</div>
                </div>
              ))}
              <div>
                <div className="text-xs text-text-muted mb-1">积分余额</div>
                <div className="text-sm text-white">
                  <span className="text-amber-gold font-semibold font-mono">{fmtNum(detailData.points?.current_balance ?? 0)}</span>
                  <span className="text-text-muted text-xs ml-2">冻结 {fmtNum(detailData.points?.frozen_balance ?? 0)}</span>
                </div>
              </div>
            </div>

            {/* 会员设置 */}
            <div className="border-t border-[#2A2A3E] pt-4 mb-6">
              <div className="text-sm font-semibold text-white mb-3">设置会员套餐</div>
              <div className="text-xs text-text-muted mb-3">开通后旧套餐自动失效，按天数一次性生效</div>
              <div className="flex items-center gap-3">
                <select
                  value={planCode}
                  onChange={(e) => setPlanCode(e.target.value)}
                  className="flex-1 bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
                >
                  {PLAN_OPTIONS.map((p) => (
                    <option key={p.code} value={p.code}>{p.label}</option>
                  ))}
                </select>
                <input
                  type="number"
                  min={1}
                  max={3650}
                  value={planDays}
                  onChange={(e) => setPlanDays(e.target.value)}
                  className="w-28 bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
                />
                <span className="text-xs text-text-muted">天</span>
                <button
                  onClick={submitMembership}
                  disabled={savingPlan}
                  className="flex-shrink-0 px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {savingPlan ? "开通中…" : "开通"}
                </button>
              </div>
            </div>

            {/* 分配角色 */}
            <div className="border-t border-[#2A2A3E] pt-4">
              <div className="text-sm font-semibold text-white mb-1">分配平台角色</div>
              <div className="text-xs text-text-muted mb-3">保存后替换该用户现有全部角色</div>
              {roles.length === 0 ? (
                <div className="text-xs text-text-muted py-2">{rolesLoaded ? "暂无可分配角色" : "角色列表加载中…"}</div>
              ) : (
                <div className="grid grid-cols-2 gap-2 mb-3">
                  {roles.map((role) => (
                    <label
                      key={role.role_code}
                      className="flex items-start gap-2 bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 cursor-pointer hover:border-amber-gold/40 transition-colors"
                    >
                      <input
                        type="checkbox"
                        className="custom-checkbox mt-0.5"
                        checked={checkedRoles.has(role.role_code)}
                        onChange={() =>
                          setCheckedRoles((prev) => {
                            const next = new Set(prev);
                            if (next.has(role.role_code)) next.delete(role.role_code);
                            else next.add(role.role_code);
                            return next;
                          })
                        }
                      />
                      <span>
                        <span className="block text-sm text-white">{role.name}</span>
                        <span className="block text-xs text-text-muted mt-0.5 break-all">{role.description || role.role_code}</span>
                      </span>
                    </label>
                  ))}
                </div>
              )}
              <button
                onClick={submitRoles}
                disabled={roles.length === 0}
                className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-amber-gold/50 hover:text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                保存角色分配
              </button>
            </div>
          </>
        )}
      </Modal>

      {/* ====== 积分调整弹窗（单个 / 批量共用） ====== */}
      <Modal
        open={pointsTargets !== null}
        onClose={() => setPointsTargets(null)}
        title="调整积分"
        subtitle={
          pointsTargets
            ? pointsTargets.length === 1
              ? `${pointsTargets[0].nickname} · ${pointsTargets[0].user_uuid.slice(0, 8)}`
              : `已选 ${pointsTargets.length} 个用户，将应用同一调整`
            : undefined
        }
      >
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-text-muted mb-2">调整值（正数增加 / 负数扣减）</label>
            <input
              type="number"
              autoFocus
              placeholder="如 500 或 -200"
              value={pointsAmount}
              onChange={(e) => setPointsAmount(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
            />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">调整原因（必填，1-200 字）</label>
            <textarea
              rows={3}
              placeholder="如：活动奖励补发 / 违规扣减"
              value={pointsReason}
              onChange={(e) => setPointsReason(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors resize-none"
            />
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button
            onClick={() => setPointsTargets(null)}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
          >
            取消
          </button>
          <button
            onClick={submitPoints}
            disabled={savingPoints}
            className="px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {savingPoints ? "提交中…" : "确认调整"}
          </button>
        </div>
      </Modal>

      {/* 新增用户弹窗（POST /admin/users） */}
      <Modal
        title="新增用户"
        subtitle="创建用户账号并初始化积分与角色"
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        footer={
          <>
            <button
              className="px-4 py-2 rounded-lg text-sm text-text-secondary hover:text-white transition-colors"
              onClick={() => setCreateOpen(false)}
            >
              取消
            </button>
            <button
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              disabled={creating}
              onClick={() => void submitCreate()}
            >
              {creating ? "创建中…" : "创建用户"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">邮箱</label>
            <input
              type="email"
              value={createEmail}
              onChange={(e) => setCreateEmail(e.target.value)}
              placeholder="user@example.com"
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">密码（至少 8 位）</label>
            <input
              type="password"
              value={createPwd}
              onChange={(e) => setCreatePwd(e.target.value)}
              autoComplete="new-password"
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">昵称</label>
            <input
              type="text"
              value={createNick}
              onChange={(e) => setCreateNick(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">初始积分</label>
            <input
              type="number"
              value={createPoints}
              onChange={(e) => setCreatePoints(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">角色（可选）</label>
            <div className="space-y-1.5">
              {roles.map((role) => (
                <label key={role.role_code} className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    className="accent-amber-gold"
                    checked={createRoles.has(role.role_code)}
                    onChange={(e) =>
                      setCreateRoles((prev) => {
                        const next = new Set(prev);
                        if (e.target.checked) next.add(role.role_code);
                        else next.delete(role.role_code);
                        return next;
                      })
                    }
                  />
                  <span className="text-sm text-white">{role.name}</span>
                  <span className="text-xs text-text-muted">({role.role_code})</span>
                </label>
              ))}
              {roles.length === 0 && <span className="text-xs text-text-muted">角色加载中…</span>}
            </div>
          </div>
        </div>
      </Modal>

      {/* Footer */}
      <div className="mt-8 pt-4 border-t border-[#2A2A3E] text-center">
        <p className="text-xs text-text-muted">Empty Broken Admin v1.0 · © 2025 Empty Broken 创作平台</p>
      </div>
    </>
  );
}
