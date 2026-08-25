// AdminMembershipPanel：平台配置 → 会员套餐 Tab。
// 数据源：套餐 GET /admin/packages（全字段 to_jsonb）+ 编辑 PUT /admin/packages（upsert，与计费财务页共用同一写接口）；
//        积分包 GET /admin/point-packages + 编辑 PUT /admin/point-packages（upsert）。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Plus, RefreshCw, Coins, Crown } from "lucide-react";

import { toast, errMessage } from "../lib";
import Modal from "../components/Modal";

interface PackageRow {
  id: number;
  package_code: string;
  name: string;
  version: string;
  billing_cycle: string;
  price_yuan: string;
  points_quota: number;
  points_monthly_grant: number;
  is_recommended: boolean;
  status: string;
}

interface PointPackageRow {
  id: number;
  pkg_code: string;
  name: string;
  points: number;
  bonus_points: number;
  price_yuan: string;
  effective_days: number;
  is_limited_offer: boolean;
  status: string;
}

interface ListResp<T> {
  items: T[];
  pagination?: { total: number };
}

const TH = "px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider whitespace-nowrap";
const TD = "px-3 py-3 text-sm text-text-secondary whitespace-nowrap";

function StatusTag({ status }: { status: string }) {
  const on = status === "ON" || status === "ACTIVE";
  return on ? (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-green-400 bg-green-400/10 border border-green-400/30">启用</span>
  ) : (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-text-muted bg-[#2A2A3E]/60 border border-[#2A2A3E]">停用</span>
  );
}

function cycleLabel(cycle: string): string {
  switch ((cycle || "").toUpperCase()) {
    case "MONTHLY": return "月度";
    case "ANNUAL": return "年度";
    case "QUARTERLY": return "季度";
    case "ONETIME": return "一次性";
    default: return cycle || "—";
  }
}

export default function AdminMembershipPanel() {
  const [packages, setPackages] = useState<PackageRow[]>([]);
  const [pointPkgs, setPointPkgs] = useState<PointPackageRow[]>([]);
  const [loading, setLoading] = useState(false);

  // 套餐编辑弹窗（PUT /admin/packages upsert：package_code 已存在则更新）
  const [editOpen, setEditOpen] = useState(false);
  const [editPkg, setEditPkg] = useState<PackageRow | null>(null);
  const [form, setForm] = useState({ code: "", name: "", price: "0.00", points: "0", status: "ON" });
  const [saving, setSaving] = useState(false);

  // 积分包编辑弹窗（PUT /admin/point-packages upsert：pkg_code 已存在则更新）
  const [ppEditOpen, setPpEditOpen] = useState(false);
  const [ppEdit, setPpEdit] = useState<PointPackageRow | null>(null);
  const [ppForm, setPpForm] = useState({ code: "", name: "", points: "0", bonus: "0", price: "0.00", status: "ON" });
  const [savingPp, setSavingPp] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [p, pp] = await Promise.all([
        api.get<ListResp<PackageRow>>("/admin/packages", { params: { page_size: 100 } }),
        api.get<ListResp<PointPackageRow>>("/admin/point-packages", { params: { page_size: 100 } }),
      ]);
      setPackages(p.items ?? []);
      setPointPkgs(pp.items ?? []);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openEdit = (pkg: PackageRow) => {
    setEditPkg(pkg);
    setForm({ code: pkg.package_code, name: pkg.name, price: pkg.price_yuan ?? "0.00", points: String(pkg.points_quota ?? 0), status: pkg.status });
    setEditOpen(true);
  };

  const openCreate = () => {
    setEditPkg(null);
    setForm({ code: "", name: "", price: "0.00", points: "0", status: "ON" });
    setEditOpen(true);
  };

  const save = async () => {
    if (!editPkg && !form.code.trim()) {
      toast("新增套餐需填写唯一编码", "error");
      return;
    }
    if (!form.name.trim()) {
      toast("套餐名称不能为空", "error");
      return;
    }
    setSaving(true);
    try {
      await api.put("/admin/packages", {
        package_code: editPkg?.package_code ?? form.code.trim(),
        name: form.name.trim(),
        price_yuan: form.price || "0.00",
        points_quota: Number(form.points) || 0,
        status: form.status,
      });
      toast("套餐已保存", "success");
      setEditOpen(false);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSaving(false);
    }
  };

  // ---------- 积分包编辑 ----------

  const openPpEdit = (pkg: PointPackageRow | null) => {
    setPpEdit(pkg);
    setPpForm({
      code: pkg?.pkg_code ?? "",
      name: pkg?.name ?? "",
      points: String(pkg?.points ?? 0),
      bonus: String(pkg?.bonus_points ?? 0),
      price: pkg?.price_yuan ?? "0.00",
      status: pkg?.status ?? "ON",
    });
    setPpEditOpen(true);
  };

  const savePp = async () => {
    if (!ppEdit && !ppForm.code.trim()) {
      toast("新增积分包需填写唯一编码", "error");
      return;
    }
    if (!ppForm.name.trim()) {
      toast("积分包名称不能为空", "error");
      return;
    }
    setSavingPp(true);
    try {
      await api.put("/admin/point-packages", {
        package_code: ppEdit?.pkg_code ?? ppForm.code.trim(),
        name: ppForm.name.trim(),
        points: Number(ppForm.points) || 0,
        bonus_points: Number(ppForm.bonus) || 0,
        price_yuan: ppForm.price || "0.00",
        status: ppForm.status,
      });
      toast("积分包已保存", "success");
      setPpEditOpen(false);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingPp(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Crown className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">会员套餐</h3>
          <span className="text-xs text-text-muted">套餐编辑保存后全局生效；新增套餐需填写唯一编码</span>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => void load()}
            disabled={loading}
            className="px-3 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors flex items-center gap-2 disabled:opacity-50"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} /> 刷新
          </button>
          <button
            onClick={openCreate}
            className="px-3 py-2 rounded-lg text-sm font-semibold bg-amber-gold text-[#0B0B1A] hover:bg-amber-400 transition-colors flex items-center gap-2"
          >
            <Plus className="w-4 h-4" /> 新增套餐
          </button>
        </div>
      </div>

      {/* 套餐表格 */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>编码</th>
              <th className={TH}>名称</th>
              <th className={TH}>周期</th>
              <th className={TH}>价格(¥)</th>
              <th className={TH}>积分配额</th>
              <th className={TH}>月赠积分</th>
              <th className={TH}>推荐</th>
              <th className={TH}>状态</th>
              <th className={TH}>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {packages.map((p) => (
              <tr key={p.package_code} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-mono text-xs"}>{p.package_code}</td>
                <td className={TD + " text-white"}>{p.name}</td>
                <td className={TD}>{cycleLabel(p.billing_cycle)}</td>
                <td className={TD}>¥{p.price_yuan ?? "0.00"}</td>
                <td className={TD}>{p.points_quota?.toLocaleString() ?? 0}</td>
                <td className={TD}>{p.points_monthly_grant ?? 0}</td>
                <td className={TD}>{p.is_recommended ? "✓" : "—"}</td>
                <td className={TD}><StatusTag status={p.status} /></td>
                <td className={TD}>
                  <button onClick={() => openEdit(p)} className="text-amber-gold hover:text-amber-300 text-sm transition-colors">
                    编辑
                  </button>
                </td>
              </tr>
            ))}
            {packages.length === 0 && !loading && (
              <tr>
                <td colSpan={9} className="px-3 py-10 text-center text-text-muted text-sm">暂无套餐数据（接口无记录或加载失败）</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 积分包（可编辑） */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[#2A2A3E]">
          <div className="flex items-center gap-2">
            <Coins className="w-4 h-4 text-amber-gold" />
            <h4 className="text-sm font-medium text-white">积分包</h4>
            <span className="text-xs text-text-muted">用户端充值页档位实时读取（PUT /admin/point-packages upsert）</span>
          </div>
          <button
            onClick={() => openPpEdit(null)}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-gradient-to-r from-amber-gold to-amber-dim rounded-md text-xs text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Plus className="w-3.5 h-3.5" /> 新增积分包
          </button>
        </div>
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>编码</th>
              <th className={TH}>名称</th>
              <th className={TH}>积分</th>
              <th className={TH}>赠送</th>
              <th className={TH}>价格(¥)</th>
              <th className={TH}>有效期(天)</th>
              <th className={TH}>限时</th>
              <th className={TH}>状态</th>
              <th className={TH}>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {pointPkgs.map((p) => (
              <tr key={p.pkg_code} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-mono text-xs"}>{p.pkg_code}</td>
                <td className={TD + " text-white"}>{p.name}</td>
                <td className={TD}>{p.points?.toLocaleString() ?? 0}</td>
                <td className={TD}>{p.bonus_points ?? 0}</td>
                <td className={TD}>¥{p.price_yuan ?? "0.00"}</td>
                <td className={TD}>{p.effective_days ?? 365}</td>
                <td className={TD}>{p.is_limited_offer ? "✓" : "—"}</td>
                <td className={TD}><StatusTag status={p.status} /></td>
                <td className={TD}>
                  <button onClick={() => openPpEdit(p)} className="text-amber-gold hover:text-amber-300 text-sm transition-colors">
                    编辑
                  </button>
                </td>
              </tr>
            ))}
            {pointPkgs.length === 0 && !loading && (
              <tr>
                <td colSpan={9} className="px-3 py-8 text-center text-text-muted text-sm">暂无积分包数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 套餐编辑/新增弹窗 */}
      <Modal
        title={editPkg ? "编辑套餐" : "新增套餐"}
        subtitle={editPkg ? `编码 ${editPkg.package_code}（保存按编码更新）` : "保存后创建新套餐；编码需唯一"}
        open={editOpen}
        onClose={() => setEditOpen(false)}
        footer={
          <>
            <button onClick={() => setEditOpen(false)} className="px-4 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors">
              取消
            </button>
            <button
              onClick={() => void save()}
              disabled={saving}
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
            >
              {saving ? "保存中…" : "保存套餐"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          {!editPkg && (
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">套餐编码（创建后不可改）</label>
              <input
                type="text"
                className="input-field"
                placeholder="如 pro_monthly"
                value={form.code}
                onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.trim() }))}
              />
            </div>
          )}
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">套餐名称</label>
            <input type="text" className="input-field" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">价格(¥)</label>
              <input type="number" step="0.01" className="input-field" value={form.price} onChange={(e) => setForm((f) => ({ ...f, price: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">积分配额</label>
              <input type="number" className="input-field" value={form.points} onChange={(e) => setForm((f) => ({ ...f, points: e.target.value }))} />
            </div>
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">状态</label>
            <select className="input-field" value={form.status} onChange={(e) => setForm((f) => ({ ...f, status: e.target.value }))}>
              <option value="ON">启用（ON）</option>
              <option value="OFF">停用（OFF）</option>
              <option value="ACTIVE">启用（ACTIVE）</option>
            </select>
          </div>
        </div>
      </Modal>

      {/* 积分包编辑/新增弹窗 */}
      <Modal
        title={ppEdit ? "编辑积分包" : "新增积分包"}
        subtitle={ppEdit ? `编码 ${ppEdit.pkg_code}（保存按编码更新）` : "保存后创建新档位；编码需唯一"}
        open={ppEditOpen}
        onClose={() => setPpEditOpen(false)}
        footer={
          <>
            <button onClick={() => setPpEditOpen(false)} className="px-4 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors">
              取消
            </button>
            <button
              onClick={() => void savePp()}
              disabled={savingPp}
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
            >
              {savingPp ? "保存中…" : "保存积分包"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          {!ppEdit && (
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">积分包编码（创建后不可改）</label>
              <input
                type="text"
                className="input-field"
                placeholder="如 POINTS_PKG_MEDIUM"
                value={ppForm.code}
                onChange={(e) => setPpForm((f) => ({ ...f, code: e.target.value.trim() }))}
              />
            </div>
          )}
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">名称</label>
            <input type="text" className="input-field" value={ppForm.name} onChange={(e) => setPpForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">积分</label>
              <input type="number" className="input-field" value={ppForm.points} onChange={(e) => setPpForm((f) => ({ ...f, points: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">加赠积分</label>
              <input type="number" className="input-field" value={ppForm.bonus} onChange={(e) => setPpForm((f) => ({ ...f, bonus: e.target.value }))} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">价格(¥)</label>
              <input type="number" step="0.01" className="input-field" value={ppForm.price} onChange={(e) => setPpForm((f) => ({ ...f, price: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">状态</label>
              <select className="input-field" value={ppForm.status} onChange={(e) => setPpForm((f) => ({ ...f, status: e.target.value }))}>
                <option value="ON">启用（ON）</option>
                <option value="OFF">停用（OFF）</option>
              </select>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
}
