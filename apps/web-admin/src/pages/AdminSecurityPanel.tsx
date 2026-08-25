// AdminSecurityPanel：平台配置 → 安全设置 Tab。
// 数据源：限流配置 GET /admin/rate-limits + 编辑 PUT /admin/rate-limits/:id。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Shield, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";
import Modal from "../components/Modal";

interface RateLimitRow {
  id: number;
  route_pattern: string;
  limit_per_min: number;
  burst: number;
  scope: string;
  status: string;
}

interface ListResp<T> {
  items: T[];
}

const TH = "px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider whitespace-nowrap";
const TD = "px-3 py-3 text-sm text-text-secondary whitespace-nowrap";

const SCOPE_LABEL: Record<string, string> = {
  IP: "IP 维度",
  USER: "用户维度",
  API_KEY: "API Key 维度",
};

export default function AdminSecurityPanel() {
  const [rows, setRows] = useState<RateLimitRow[]>([]);
  const [loading, setLoading] = useState(false);

  // 限流编辑弹窗
  const [editRow, setEditRow] = useState<RateLimitRow | null>(null);
  const [rlForm, setRlForm] = useState({ limit: "60", burst: "10", status: "ON" });
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.get<ListResp<RateLimitRow>>("/admin/rate-limits", { params: { page_size: 100 } });
      setRows(data.items ?? []);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openEdit = (r: RateLimitRow) => {
    setEditRow(r);
    setRlForm({ limit: String(r.limit_per_min ?? 60), burst: String(r.burst ?? 10), status: r.status || "ON" });
  };

  const save = async () => {
    if (!editRow) return;
    const limit = Number(rlForm.limit);
    const burst = Number(rlForm.burst);
    if (!Number.isInteger(limit) || limit < 0) {
      toast("每分钟上限需为非负整数", "error");
      return;
    }
    if (!Number.isInteger(burst) || burst < 0) {
      toast("突发需为非负整数", "error");
      return;
    }
    setSaving(true);
    try {
      await api.put(`/admin/rate-limits/${editRow.id}`, { limit_per_min: limit, burst, status: rlForm.status });
      toast("限流配置已保存", "success");
      setEditRow(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSaving(false);
    }
  };

  const inputCls = "w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors";

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Shield className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">接口限流配置</h3>
          <span className="text-xs text-text-muted">编辑保存后即时生效（PUT /admin/rate-limits/:id）</span>
        </div>
        <button
          onClick={() => void load()}
          disabled={loading}
          className="px-3 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors flex items-center gap-2 disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} /> 刷新
        </button>
      </div>

      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>路由</th>
              <th className={TH}>每分钟上限</th>
              <th className={TH}>突发</th>
              <th className={TH}>维度</th>
              <th className={TH}>状态</th>
              <th className={TH}>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {rows.map((r) => (
              <tr key={r.id} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-mono text-xs"}>{r.route_pattern}</td>
                <td className={TD}>{r.limit_per_min}</td>
                <td className={TD}>{r.burst}</td>
                <td className={TD}>{SCOPE_LABEL[r.scope] ?? r.scope}</td>
                <td className={TD}>
                  {r.status === "ON" ? (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-green-400 bg-green-400/10 border border-green-400/30">开启</span>
                  ) : (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-text-muted bg-[#2A2A3E]/60 border border-[#2A2A3E]">关闭</span>
                  )}
                </td>
                <td className={TD}>
                  <button onClick={() => openEdit(r)} className="text-amber-gold hover:text-amber-300 text-sm transition-colors">
                    编辑
                  </button>
                </td>
              </tr>
            ))}
            {rows.length === 0 && !loading && (
              <tr>
                <td colSpan={6} className="px-3 py-10 text-center text-text-muted text-sm">暂无限流配置数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 限流编辑弹窗 */}
      <Modal
        title="编辑限流配置"
        subtitle={editRow ? `${editRow.route_pattern} · ${SCOPE_LABEL[editRow.scope] ?? editRow.scope}` : undefined}
        open={editRow !== null}
        onClose={() => setEditRow(null)}
        footer={
          <>
            <button onClick={() => setEditRow(null)} className="px-4 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors">取消</button>
            <button
              onClick={() => void save()}
              disabled={saving}
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
            >
              {saving ? "保存中…" : "保存配置"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">每分钟上限</label>
              <input type="number" className={inputCls} value={rlForm.limit} onChange={(e) => setRlForm((f) => ({ ...f, limit: e.target.value }))} />
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">突发（burst）</label>
              <input type="number" className={inputCls} value={rlForm.burst} onChange={(e) => setRlForm((f) => ({ ...f, burst: e.target.value }))} />
            </div>
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">状态</label>
            <select className={inputCls} value={rlForm.status} onChange={(e) => setRlForm((f) => ({ ...f, status: e.target.value }))}>
              <option value="ON">开启</option>
              <option value="OFF">关闭</option>
            </select>
          </div>
        </div>
      </Modal>
    </div>
  );
}
