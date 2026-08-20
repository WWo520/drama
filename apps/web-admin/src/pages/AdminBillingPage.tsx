// AdminBillingPage：迁移自 admin-billing.html（DOM 1:1）。
// 数据策略：套餐卡 GET /admin/packages 真数据映射设计稿卡视觉，失败回退设计稿 8 卡静态数据；
// 订单流水 GET /admin/orders 分页真数据，失败回退静态示例行；提现审核暂无后端，空态占位。
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@duanju/api-client";
import {
  FileText,
  Plus,
  Package as PackageIcon,
  Receipt,
  Banknote,
  Search,
  RotateCcw,
  Coins,
  HardDrive,
  Check,
  Edit3,
  ArrowDownToLine,
  ArrowUpFromLine,
  ChevronLeft,
  ChevronRight,
  BrainCircuit,
} from "lucide-react";

import { toast, errMessage, fmtNum, exportCSV } from "../lib";
import Modal from "../components/Modal";
import AdminModelsTab from "./AdminModelsTab";
import "./AdminBillingPage.css";

type TabKey = "packages" | "orders" | "withdraw" | "models";

// ---------- 套餐卡 ----------

interface PackageCardData {
  key: string;
  name: string;
  tag?: { cls: string; label: string };
  cycleLabel: string;
  price: string;
  unit?: string;
  priceNote?: string;
  priceNoteStrike?: boolean;
  pointsIcon: "coins" | "drive";
  pointsLabel: string;
  pointsNote: string;
  pointsNoteGreen?: boolean;
  benefits: string[];
  offline: boolean;
  real: boolean;
  rawPrice?: string; // 真实数据回传后端用
  rawPoints?: number;
}

interface BackendPackage {
  package_code: string;
  name: string;
  billing_cycle: string;
  price_yuan: string;
  points_quota: number;
  status: string;
}

interface PackagesResp {
  items: BackendPackage[];
}

// billing_cycle → 卡片展示
function cycleLabel(cycle: string): string {
  switch ((cycle || "").toUpperCase()) {
    case "MONTHLY":
      return "月度订阅";
    case "ANNUAL":
      return "年度订阅";
    case "QUARTERLY":
      return "季度订阅";
    default:
      return cycle ? "一次性购买" : "免费套餐";
  }
}

function cycleUnit(cycle: string): string | undefined {
  switch ((cycle || "").toUpperCase()) {
    case "MONTHLY":
      return "/月";
    case "ANNUAL":
      return "/年";
    case "QUARTERLY":
      return "/季";
    default:
      return undefined;
  }
}

// 设计稿静态回退卡（接口失败时展示）
const FALLBACK_CARDS: PackageCardData[] = [
  {
    key: "flagship",
    name: "旗舰会员",
    tag: { cls: "tag-hot", label: "热门" },
    cycleLabel: "年度订阅",
    price: "999",
    unit: "/年",
    priceNote: "原价 ¥1,299",
    priceNoteStrike: true,
    pointsIcon: "coins",
    pointsLabel: "100,000 积分",
    pointsNote: "额外赠送 20,000 积分",
    benefits: ["无限视频生成", "高清 4K 画质输出", "专属客服优先处理", "商业授权许可"],
    offline: false,
    real: false,
  },
  {
    key: "premium",
    name: "高级会员",
    tag: { cls: "tag-recommend", label: "推荐" },
    cycleLabel: "月度订阅",
    price: "99",
    unit: "/月",
    priceNote: "连续包月 ¥79/月",
    pointsIcon: "coins",
    pointsLabel: "5,000 积分",
    pointsNote: "每月自动续期",
    benefits: ["每日 50 次生成", "1080P 高清画质", "标准客服支持", "去除水印"],
    offline: false,
    real: false,
  },
  {
    key: "basic",
    name: "基础会员",
    tag: { cls: "tag-new", label: "新品" },
    cycleLabel: "月度订阅",
    price: "29",
    unit: "/月",
    priceNote: "入门首选",
    pointsIcon: "coins",
    pointsLabel: "1,000 积分",
    pointsNote: "每月自动续期",
    benefits: ["每日 10 次生成", "720P 画质输出", "基础模板库", "社区支持"],
    offline: false,
    real: false,
  },
  {
    key: "quarter",
    name: "季度会员",
    cycleLabel: "季度订阅",
    price: "249",
    unit: "/季",
    priceNote: "省 ¥48，约 ¥83/月",
    pointsIcon: "coins",
    pointsLabel: "18,000 积分",
    pointsNote: "季度一次性发放",
    benefits: ["每日 50 次生成", "1080P 高清画质", "标准客服支持", "去除水印"],
    offline: false,
    real: false,
  },
  {
    key: "points-100k",
    name: "10万积分包",
    tag: { cls: "tag-hot", label: "热门" },
    cycleLabel: "积分包 · 永久有效",
    price: "599",
    priceNote: "约 ¥0.006/积分",
    pointsIcon: "coins",
    pointsLabel: "100,000 积分",
    pointsNote: "限时赠送 10,000 积分",
    pointsNoteGreen: true,
    benefits: ["永久有效，无过期", "可用于所有生成功能", "支持批量购买优惠", "可开发票"],
    offline: false,
    real: false,
  },
  {
    key: "points-10k",
    name: "1万积分包",
    cycleLabel: "积分包 · 永久有效",
    price: "69",
    priceNote: "约 ¥0.0069/积分",
    pointsIcon: "coins",
    pointsLabel: "10,000 积分",
    pointsNote: "即买即用",
    benefits: ["永久有效，无过期", "可用于所有生成功能", "小额充值灵活使用", "可开发票"],
    offline: false,
    real: false,
  },
  {
    key: "storage-100g",
    name: "存储扩容 100G",
    tag: { cls: "tag-new", label: "新品" },
    cycleLabel: "扩容包 · 年付",
    price: "199",
    unit: "/年",
    priceNote: "100GB 云存储空间",
    pointsIcon: "drive",
    pointsLabel: "100 GB 存储空间",
    pointsNote: "叠加在基础容量上",
    benefits: ["额外 100GB 云存储", "作品文件长期保存", "可多次购买叠加", "数据安全加密"],
    offline: false,
    real: false,
  },
  {
    key: "enterprise",
    name: "企业定制版",
    cycleLabel: "企业订阅 · 按年",
    price: "4,999",
    unit: "/年",
    priceNote: "企业级解决方案",
    pointsIcon: "coins",
    pointsLabel: "500,000 积分",
    pointsNote: "多席位团队账号",
    benefits: ["无限视频生成", "4K 高清 + 批量渲染", "专属客户经理", "API 接口调用"],
    offline: true,
    real: false,
  },
];

// ---------- 订单流水 ----------

interface OrderRow {
  order_no: string;
  user: string;
  plan: string;
  amount: string;
  payMethod: string;
  statusLabel: string;
  statusCls: string;
  created_at: string;
  real: boolean;
}

interface BackendOrder {
  order_no: string;
  user_id: number;
  order_type: string;
  plan_code: string;
  amount_yuan: string;
  pay_status: string;
  created_at: string;
}

interface OrdersResp {
  items: BackendOrder[];
  pagination: { page: number; page_size: number; total: number };
}

const ORDER_PAGE_SIZE = 10;

const ORDER_TYPE_LABEL: Record<string, string> = {
  SUBSCRIPTION: "订阅支付",
  POINTS: "积分充值",
  STORAGE: "扩容购买",
};

function payStatusTag(status: string): { label: string; cls: string } {
  switch ((status || "").toUpperCase()) {
    case "PAID":
      return { label: "已完成", cls: "pay-paid" };
    case "PENDING":
      return { label: "待支付", cls: "pay-pending" };
    case "REFUNDED":
      return { label: "已退款", cls: "pay-refunded" };
    case "FAILED":
      return { label: "支付失败", cls: "pay-canceled" };
    case "EXPIRED":
    case "CANCELLED":
      return { label: "已取消", cls: "pay-canceled" };
    default:
      return { label: status || "—", cls: "pay-refunded" };
  }
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso || "—";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

const FALLBACK_ORDERS: OrderRow[] = [
  { order_no: "ORD202501200915001", user: "李逍遥", plan: "旗舰会员", amount: "999.00", payMethod: "支付宝", statusLabel: "已完成", statusCls: "pay-paid", created_at: "2025-01-20 09:15", real: false },
  { order_no: "ORD202501200842002", user: "周芷若", plan: "高级会员", amount: "99.00", payMethod: "支付宝", statusLabel: "已完成", statusCls: "pay-paid", created_at: "2025-01-20 08:42", real: false },
  { order_no: "ORD202501192210003", user: "王语嫣", plan: "1万积分包", amount: "69.00", payMethod: "支付宝", statusLabel: "待支付", statusCls: "pay-pending", created_at: "2025-01-19 22:10", real: false },
  { order_no: "ORD202501191805004", user: "郭靖", plan: "基础会员", amount: "29.00", payMethod: "支付宝", statusLabel: "已退款", statusCls: "pay-refunded", created_at: "2025-01-19 18:05", real: false },
  { order_no: "ORD202501181520005", user: "黄蓉", plan: "存储扩容 100G", amount: "199.00", payMethod: "支付宝", statusLabel: "已取消", statusCls: "pay-canceled", created_at: "2025-01-18 15:20", real: false },
];

// ---------- 组件 ----------

export default function AdminBillingPage() {
  const [tab, setTab] = useState<TabKey>("packages");

  // 套餐数据
  const [cards, setCards] = useState<PackageCardData[]>(FALLBACK_CARDS);
  const [cardsReal, setCardsReal] = useState(false);

  // 套餐过滤（当前数据上客户端过滤）
  const [pkgKeywordInput, setPkgKeywordInput] = useState("");
  const [pkgStatus, setPkgStatus] = useState("全部状态");
  const [pkgType, setPkgType] = useState("全部类型");
  const [pkgKeyword, setPkgKeyword] = useState("");
  const [appliedPkgStatus, setAppliedPkgStatus] = useState("全部状态");

  // 订单数据
  const [orders, setOrders] = useState<OrderRow[]>(FALLBACK_ORDERS);
  const [ordersReal, setOrdersReal] = useState(false);
  const [orderTotal, setOrderTotal] = useState(FALLBACK_ORDERS.length);
  const [orderPage, setOrderPage] = useState(1);
  const [ordKeywordInput, setOrdKeywordInput] = useState("");
  const [ordKeyword, setOrdKeyword] = useState("");

  const loadPackages = useCallback(async () => {
    try {
      const resp = await api.get<PackagesResp>("/admin/packages");
      const mapped: PackageCardData[] = resp.items.map((p) => {
        const online = p.status.toUpperCase() === "ACTIVE" || p.status.toUpperCase() === "ON";
        return {
          key: p.package_code,
          name: p.name || p.package_code,
          cycleLabel: cycleLabel(p.billing_cycle),
          price: fmtNum(Number(p.price_yuan || 0)),
          unit: cycleUnit(p.billing_cycle),
          pointsIcon: "coins" as const,
          pointsLabel: `${fmtNum(p.points_quota || 0)} 积分`,
          pointsNote: p.billing_cycle ? "按期自动续期" : "即买即用",
          benefits: [
            "积分可用于所有生成功能",
            p.billing_cycle ? `${cycleLabel(p.billing_cycle)}，按期生效` : "一次购买，立即生效",
            `套餐编码 ${p.package_code}`,
            "支持开具发票",
          ],
          offline: !online,
          real: true,
          rawPrice: p.price_yuan,
          rawPoints: p.points_quota || 0,
        };
      });
      setCards(mapped);
      setCardsReal(true);
    } catch {
      setCards(FALLBACK_CARDS);
      setCardsReal(false);
    }
  }, []);

  const loadOrders = useCallback(async (p: number) => {
    try {
      const resp = await api.get<OrdersResp>(`/admin/orders?page=${p}&page_size=${ORDER_PAGE_SIZE}`);
      const mapped: OrderRow[] = resp.items.map((o) => {
        const tag = payStatusTag(o.pay_status);
        return {
          order_no: o.order_no,
          user: `用户 #${o.user_id}`,
          plan: o.plan_code || ORDER_TYPE_LABEL[o.order_type] || "—",
          amount: Number(o.amount_yuan || 0).toFixed(2),
          payMethod: "—",
          statusLabel: tag.label,
          statusCls: tag.cls,
          created_at: fmtTime(o.created_at),
          real: true,
        };
      });
      setOrders(mapped);
      setOrderTotal(resp.pagination.total);
      setOrdersReal(true);
    } catch {
      setOrders(FALLBACK_ORDERS);
      setOrderTotal(FALLBACK_ORDERS.length);
      setOrdersReal(false);
    }
  }, []);

  useEffect(() => {
    loadPackages();
  }, [loadPackages]);

  useEffect(() => {
    if (tab === "orders") loadOrders(orderPage);
  }, [tab, orderPage, loadOrders]);

  const filteredCards = useMemo(
    () =>
      cards.filter((c) => {
        if (pkgKeyword && !c.name.includes(pkgKeyword)) return false;
        if (appliedPkgStatus === "上架中" && c.offline) return false;
        if (appliedPkgStatus === "已下架" && !c.offline) return false;
        // 类型过滤仅在回退静态数据上有意义（真实数据无类型标签）
        if (pkgType !== "全部类型" && cardsReal) return true;
        if (pkgType === "积分包" && !c.key.startsWith("points-")) return false;
        if (pkgType === "扩容包" && !c.key.startsWith("storage-")) return false;
        if (pkgType === "订阅" && (c.key.startsWith("points-") || c.key.startsWith("storage-"))) return false;
        return true;
      }),
    [cards, pkgKeyword, appliedPkgStatus, pkgType, cardsReal],
  );

  const filteredOrders = useMemo(
    () => orders.filter((o) => !ordKeyword || o.order_no.toLowerCase().includes(ordKeyword.toLowerCase())),
    [orders, ordKeyword],
  );

  const orderTotalPages = Math.max(1, Math.ceil(orderTotal / ORDER_PAGE_SIZE));

  const gotoOrderPage = (p: number) => {
    if (p < 1 || p > orderTotalPages || !ordersReal) return;
    setOrderPage(p);
  };

  const pkgEdit = (card: PackageCardData) => {
    setPkgEditMode(true);
    setPkgForm({
      code: card.key,
      name: card.name,
      price: card.rawPrice ?? "0.00",
      points: String(card.rawPoints ?? 0),
      status: card.offline ? "OFF" : "ACTIVE",
    });
    setPkgModalOpen(true);
  };

  // 套餐编辑/新增弹窗（PUT /admin/packages upsert；mode=true 编辑，false 新增）
  const [pkgModalOpen, setPkgModalOpen] = useState(false);
  const [pkgEditMode, setPkgEditMode] = useState(false);
  const [pkgForm, setPkgForm] = useState({ code: "", name: "", price: "0.00", points: "0", status: "ACTIVE" });
  const [savingPkg, setSavingPkg] = useState(false);

  const savePackage = async () => {
    if (!pkgForm.code.trim() || !pkgForm.name.trim()) {
      toast("套餐编码与名称不能为空", "error");
      return;
    }
    setSavingPkg(true);
    try {
      await api.put("/admin/packages", {
        package_code: pkgForm.code.trim(),
        name: pkgForm.name.trim(),
        price_yuan: pkgForm.price || "0.00",
        points_quota: Number(pkgForm.points) || 0,
        status: pkgForm.status,
      });
      setPkgModalOpen(false);
      toast(`已保存套餐 ${pkgForm.name.trim()}`, "success");
      void loadPackages();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingPkg(false);
    }
  };
  const pkgToggle = async (card: PackageCardData) => {
    if (!card.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.put("/admin/packages", {
        package_code: card.key,
        name: card.name,
        price_yuan: card.rawPrice ?? "0.00",
        points_quota: card.rawPoints ?? 0,
        status: card.offline ? "ACTIVE" : "OFF",
      });
      setCards((prev) => prev.map((c) => (c.key === card.key ? { ...c, offline: !card.offline } : c)));
      toast(card.offline ? `已上架 ${card.name}` : `已下架 ${card.name}`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  const orderPageNums = useMemo(() => {
    const nums: number[] = [];
    const max = Math.min(orderTotalPages, 5);
    for (let i = 1; i <= max; i++) nums.push(i);
    return nums;
  }, [orderTotalPages]);

  return (
    <>
      {/* Page title */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white mb-1">计费财务</h1>
          <p className="text-sm text-text-muted">管理平台套餐配置、订单流水与提现审核</p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() =>
              exportCSV(
                "订单流水",
                ["订单号", "用户", "套餐", "金额(元)", "状态", "创建时间"],
                orders.map((o) => [o.order_no, o.user, o.plan, o.amount, o.statusLabel, o.created_at]),
              )
            }
            className="flex items-center px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-amber-gold/50 hover:text-white transition-colors"
          >
            <FileText className="w-4 h-4 mr-2" />
            导出对账报表
          </button>
          <button
            onClick={() => {
              setPkgEditMode(false);
              setPkgForm({ code: "", name: "", price: "0.00", points: "0", status: "ACTIVE" });
              setPkgModalOpen(true);
            }}
            className="flex items-center px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Plus className="w-4 h-4 mr-2" />
            新增套餐
          </button>
        </div>
      </div>

      {/* TABS */}
      <div className="border-b border-[#2A2A3E] mb-6">
        <div className="flex items-center gap-8">
          <div className={`tab-btn ${tab === "packages" ? "active" : ""}`} onClick={() => setTab("packages")}>
            <span className="flex items-center gap-2">
              <PackageIcon className="w-4 h-4" />
              套餐管理
            </span>
          </div>
          <div className={`tab-btn ${tab === "orders" ? "active" : ""}`} onClick={() => setTab("orders")}>
            <span className="flex items-center gap-2">
              <Receipt className="w-4 h-4" />
              订单流水
            </span>
          </div>
          <div className={`tab-btn ${tab === "withdraw" ? "active" : ""}`} onClick={() => setTab("withdraw")}>
            <span className="flex items-center gap-2">
              <Banknote className="w-4 h-4" />
              提现审核
            </span>
          </div>
          {/* 融合清单批次 5：大模型配置 CRUD（设计稿无此 Tab，按清单要求增设） */}
          <div className={`tab-btn ${tab === "models" ? "active" : ""}`} onClick={() => setTab("models")}>
            <span className="flex items-center gap-2">
              <BrainCircuit className="w-4 h-4" />
              模型配置
            </span>
          </div>
        </div>
      </div>

      {/* TAB 1: 套餐管理 */}
      {tab === "packages" && (
        <div>
          {/* Filter bar */}
          <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-4 mb-6">
            <div className="flex items-center gap-3 flex-wrap">
              <div className="relative flex-1 min-w-[240px] max-w-[320px]">
                <Search className="w-4 h-4 text-text-muted absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  placeholder="搜索套餐名称..."
                  value={pkgKeywordInput}
                  onChange={(e) => setPkgKeywordInput(e.target.value)}
                  className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg pl-9 pr-4 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
                />
              </div>

              <div className="relative">
                <select
                  value={pkgStatus}
                  onChange={(e) => setPkgStatus(e.target.value)}
                  className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors w-[120px]"
                >
                  <option>全部状态</option>
                  <option>上架中</option>
                  <option>已下架</option>
                </select>
              </div>

              <div className="relative">
                <select
                  value={pkgType}
                  onChange={(e) => setPkgType(e.target.value)}
                  className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors w-[130px]"
                >
                  <option>全部类型</option>
                  <option>订阅</option>
                  <option>积分包</option>
                  <option>扩容包</option>
                </select>
              </div>

              <button
                onClick={() => {
                  setPkgKeyword(pkgKeywordInput.trim());
                  setAppliedPkgStatus(pkgStatus);
                }}
                className="flex items-center px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
              >
                <Search className="w-4 h-4 mr-2" />
                搜索
              </button>

              <button
                onClick={() => {
                  setPkgKeywordInput("");
                  setPkgStatus("全部状态");
                  setPkgType("全部类型");
                  setPkgKeyword("");
                  setAppliedPkgStatus("全部状态");
                }}
                className="flex items-center px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
              >
                <RotateCcw className="w-4 h-4 mr-2" />
                重置
              </button>
            </div>
          </div>

          {/* Package Grid */}
          <div className="grid grid-cols-4 gap-5">
            {filteredCards.map((c) => (
              <div key={c.key} className="package-card bg-bg-card rounded-xl p-5 flex flex-col">
                <div className="flex items-start justify-between mb-3">
                  <div>
                    <div className="flex items-center gap-2 mb-1">
                      <h3 className="text-white font-semibold text-base">{c.name}</h3>
                      {c.tag && <span className={`${c.tag.cls} text-[10px] font-bold px-2 py-0.5 rounded-md`}>{c.tag.label}</span>}
                    </div>
                    <div className="text-xs text-text-muted">{c.cycleLabel}</div>
                  </div>
                  <span className={`${c.offline ? "status-offline" : "status-online"} text-[11px] font-medium px-2 py-0.5 rounded-md`}>
                    {c.offline ? "已下架" : "上架中"}
                  </span>
                </div>

                <div className="mb-4">
                  <div className="flex items-baseline gap-1">
                    <span className={`text-sm ${c.offline ? "text-text-muted" : "text-amber-gold"}`}>¥</span>
                    <span className={`text-3xl font-bold ${c.offline ? "text-text-muted" : "text-amber-gold price-gold"} font-mono`}>{c.price}</span>
                    {c.unit && <span className="text-xs text-text-muted">{c.unit}</span>}
                  </div>
                  {c.priceNote && (
                    <div className={`text-xs ${c.priceNoteStrike ? "text-text-muted mt-1 line-through" : "text-text-muted mt-1"}`}>{c.priceNote}</div>
                  )}
                </div>

                <div className="mb-4 pb-4 border-b border-[#2A2A3E]">
                  <div className="flex items-center gap-2 mb-2">
                    {c.pointsIcon === "drive" ? (
                      <HardDrive className={`w-4 h-4 ${c.offline ? "text-text-muted" : "text-amber-gold"}`} />
                    ) : (
                      <Coins className={`w-4 h-4 ${c.offline ? "text-text-muted" : "text-amber-gold"}`} />
                    )}
                    <span className={`text-sm ${c.offline ? "text-text-secondary" : "text-white"} font-medium`}>{c.pointsLabel}</span>
                  </div>
                  <div className={`text-xs ${c.pointsNoteGreen ? "text-green-400" : "text-text-muted"}`}>{c.pointsNote}</div>
                </div>

                <div className="flex-1 space-y-2 mb-5">
                  {c.benefits.map((b) => (
                    <div key={b} className={`flex items-center gap-2 text-xs ${c.offline ? "text-text-muted" : "text-text-secondary"}`}>
                      <Check className={`w-3.5 h-3.5 ${c.offline ? "text-text-muted" : "benefit-check"} flex-shrink-0`} />
                      <span>{b}</span>
                    </div>
                  ))}
                </div>

                <div className="flex items-center gap-2">
                  <button
                    onClick={() => pkgEdit(c)}
                    className="flex-1 flex items-center justify-center px-3 py-2 bg-bg-card-light border border-[#2A2A3E] rounded-lg text-xs text-text-secondary hover:border-amber-gold/50 hover:text-amber-gold transition-colors"
                  >
                    <Edit3 className="w-3.5 h-3.5 mr-1.5" />
                    编辑
                  </button>
                  {c.offline ? (
                    <button
                      onClick={() => pkgToggle(c)}
                      className="flex-1 flex items-center justify-center px-3 py-2 bg-green-500/10 border border-green-500/30 rounded-lg text-xs text-green-400 hover:bg-green-500/20 transition-colors"
                    >
                      <ArrowUpFromLine className="w-3.5 h-3.5 mr-1.5" />
                      上架
                    </button>
                  ) : (
                    <button
                      onClick={() => pkgToggle(c)}
                      className="flex-1 flex items-center justify-center px-3 py-2 bg-red-500/10 border border-red-500/30 rounded-lg text-xs text-red-400 hover:bg-red-500/20 transition-colors"
                    >
                      <ArrowDownToLine className="w-3.5 h-3.5 mr-1.5" />
                      下架
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* TAB 2: 订单流水 */}
      {tab === "orders" && (
        <div>
          {/* Filter bar */}
          <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-4 mb-6">
            <div className="flex items-center gap-3 flex-wrap">
              <div className="relative flex-1 min-w-[240px] max-w-[320px]">
                <Search className="w-4 h-4 text-text-muted absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  placeholder="输入订单号搜索..."
                  value={ordKeywordInput}
                  onChange={(e) => setOrdKeywordInput(e.target.value)}
                  className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg pl-9 pr-4 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
                />
              </div>

              <button
                onClick={() => setOrdKeyword(ordKeywordInput.trim())}
                className="flex items-center px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
              >
                <Search className="w-4 h-4 mr-2" />
                搜索
              </button>
            </div>
          </div>

          {/* Table */}
          <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="bg-bg-card-light border-b border-[#2A2A3E]">
                  <th className="px-4 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">订单号</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">用户</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">套餐</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-text-muted uppercase tracking-wider">金额</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">支付方式</th>
                  <th className="px-4 py-3 text-center text-xs font-medium text-text-muted uppercase tracking-wider">状态</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider">创建时间</th>
                  <th className="px-4 py-3 text-center text-xs font-medium text-text-muted uppercase tracking-wider">操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredOrders.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="px-4 py-10 text-center text-sm text-text-muted">
                      暂无订单数据
                    </td>
                  </tr>
                ) : (
                  filteredOrders.map((o) => (
                    <tr key={o.order_no} className="table-row border-b border-[#2A2A3E] last:border-b-0">
                      <td className="px-4 py-3 text-sm text-white font-mono">{o.order_no}</td>
                      <td className="px-4 py-3 text-sm text-text-secondary">{o.user}</td>
                      <td className="px-4 py-3 text-sm text-text-secondary">{o.plan}</td>
                      <td className="px-4 py-3 text-sm text-amber-gold font-mono text-right">¥{o.amount}</td>
                      <td className="px-4 py-3 text-sm text-text-secondary">{o.payMethod}</td>
                      <td className="px-4 py-3 text-center">
                        <span className={`pay-tag ${o.statusCls}`}>{o.statusLabel}</span>
                      </td>
                      <td className="px-4 py-3 text-sm text-text-muted">{o.created_at}</td>
                      <td className="px-4 py-3 text-center">
                        <button
                          onClick={() => toast(o.real ? "订单详情功能规划中" : "示例数据不可操作", o.real ? "info" : "error")}
                          className="text-xs text-text-secondary hover:text-amber-gold transition-colors"
                        >
                          详情
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {ordersReal && (
            <div className="flex items-center justify-between mt-4">
              <div className="text-xs text-text-muted">
                共 {fmtNum(orderTotal)} 条 · 第 {orderPage}/{orderTotalPages} 页
              </div>
              <div className="flex items-center gap-2">
                <button className="page-btn" disabled={orderPage <= 1} onClick={() => gotoOrderPage(orderPage - 1)}>
                  <ChevronLeft className="w-4 h-4" />
                </button>
                {orderPageNums.map((n) => (
                  <button key={n} className={`page-btn ${n === orderPage ? "active" : ""}`} onClick={() => gotoOrderPage(n)}>
                    {n}
                  </button>
                ))}
                {orderTotalPages > 5 && <span className="text-text-muted text-sm">...</span>}
                <button className="page-btn" disabled={orderPage >= orderTotalPages} onClick={() => gotoOrderPage(orderPage + 1)}>
                  <ChevronRight className="w-4 h-4" />
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* TAB 3: 提现审核（后端未实现，占位） */}
      {tab === "withdraw" && (
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl py-20 flex flex-col items-center justify-center">
          <Banknote className="w-10 h-10 text-text-muted mb-4" />
          <div className="text-sm text-white font-medium mb-1">提现审核</div>
          <p className="text-xs text-text-muted">创作者提现审核功能规划中，敬请期待</p>
        </div>
      )}

      {/* TAB 4: 模型配置（大模型厂商 / 模型 / API 密钥 CRUD） */}
      {tab === "models" && <AdminModelsTab />}

      {/* 套餐编辑/新增弹窗（PUT /admin/packages upsert） */}
      <Modal
        title={pkgEditMode ? "编辑套餐" : "新增套餐"}
        subtitle="保存后全局生效（编码已存在则更新，不存在则新建）"
        open={pkgModalOpen}
        onClose={() => setPkgModalOpen(false)}
        footer={
          <>
            <button
              className="px-4 py-2 rounded-lg text-sm text-text-secondary hover:text-white transition-colors"
              onClick={() => setPkgModalOpen(false)}
            >
              取消
            </button>
            <button
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              disabled={savingPkg}
              onClick={() => void savePackage()}
            >
              {savingPkg ? "保存中…" : "保存套餐"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">套餐编码</label>
            <input
              type="text"
              value={pkgForm.code}
              onChange={(e) => setPkgForm((f) => ({ ...f, code: e.target.value }))}
              placeholder="如 PACK_PRO / POINTS_10K"
              disabled={pkgEditMode}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 disabled:opacity-50"
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">套餐名称</label>
            <input
              type="text"
              value={pkgForm.name}
              onChange={(e) => setPkgForm((f) => ({ ...f, name: e.target.value }))}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">价格（元）</label>
              <input
                type="number"
                step="0.01"
                value={pkgForm.price}
                onChange={(e) => setPkgForm((f) => ({ ...f, price: e.target.value }))}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50"
              />
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">积分额度</label>
              <input
                type="number"
                value={pkgForm.points}
                onChange={(e) => setPkgForm((f) => ({ ...f, points: e.target.value }))}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50"
              />
            </div>
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">状态</label>
            <select
              value={pkgForm.status}
              onChange={(e) => setPkgForm((f) => ({ ...f, status: e.target.value }))}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50"
            >
              <option value="ACTIVE">上架中</option>
              <option value="OFF">已下架</option>
            </select>
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
