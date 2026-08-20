// AdminSecurityPanel：平台配置 → 安全设置 Tab。
// 数据源：限流配置 GET /admin/rate-limits（后端当前仅提供查询，编辑规划中，只读展示）。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Shield, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";

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

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Shield className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">接口限流配置</h3>
          <span className="text-xs text-text-muted">后端当前仅提供查询接口，编辑走数据库或后续迭代（只读展示）</span>
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
              </tr>
            ))}
            {rows.length === 0 && !loading && (
              <tr>
                <td colSpan={5} className="px-3 py-10 text-center text-text-muted text-sm">暂无限流配置数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
