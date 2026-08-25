// AdminPaymentPanel：平台配置 → 支付配置 Tab。
// 数据源：平台配置 GET/PUT /admin/platform/config（group=payment，alipay_* 键）。
// 说明：正式验签/密钥走环境变量（ALIPAY_*），面板仅维护渠道展示类参数。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Wallet, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";

interface ConfigItem {
  config_key: string;
  config_value: string;
  config_group: string;
  description: string;
  is_public: boolean;
}

interface ConfigResp {
  items: ConfigItem[];
}

const FIELDS: { key: string; label: string; desc: string; type: "text" | "switch" }[] = [
  { key: "alipay_app_id", label: "支付宝应用 ID", desc: "开放平台创建应用后获取", type: "text" },
  { key: "alipay_gateway", label: "网关地址", desc: "默认 https://openapi.alipay.com/gateway.do", type: "text" },
  { key: "alipay_notify_url", label: "异步回调地址", desc: "接收支付宝服务端通知的 URL", type: "text" },
  { key: "pay_mock_enabled", label: "沙箱支付开关", desc: "开发环境使用 mock 支付，生产强制关闭", type: "switch" },
];

const inputCls = "w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors";

function decodeValue(v: unknown): unknown {
  if (typeof v !== "string") return v;
  try {
    return JSON.parse(v);
  } catch {
    return v;
  }
}

export default function AdminPaymentPanel() {
  const [form, setForm] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await api.get<ConfigResp>("/admin/platform/config");
      const next: Record<string, string> = {};
      for (const it of resp.items ?? []) {
        if (FIELDS.some((f) => f.key === it.config_key)) {
          next[it.config_key] = String(decodeValue(it.config_value) ?? "");
        }
      }
      setForm(next);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    setSaving(true);
    try {
      await Promise.all(
        FIELDS.map((f) =>
          api.put("/admin/platform/config", {
            config_key: f.key,
            config_value: f.type === "switch" ? String(form[f.key] === "true") : (form[f.key] ?? ""),
            config_group: "payment",
            description: f.desc,
          }),
        ),
      );
      toast("支付配置已保存", "success");
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Wallet className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">支付配置</h3>
          <span className="text-xs text-text-muted">渠道展示参数持久化到平台配置（验签密钥仍走环境变量 ALIPAY_*）</span>
        </div>
        <button
          onClick={() => void load()}
          disabled={loading}
          className="px-3 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors flex items-center gap-2 disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} /> 刷新
        </button>
      </div>

      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 space-y-4">
        {FIELDS.map((f) => (
          <div key={f.key}>
            <label className="block text-sm text-text-secondary mb-1.5">{f.label}</label>
            {f.type === "switch" ? (
              <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer">
                <input
                  type="checkbox"
                  checked={form[f.key] === "true"}
                  onChange={(e) => setForm((prev) => ({ ...prev, [f.key]: String(e.target.checked) }))}
                  className="custom-checkbox"
                />
                {form[f.key] === "true" ? "已开启（沙箱）" : "已关闭（正式）"}
              </label>
            ) : (
              <input
                type="text"
                className={inputCls}
                value={form[f.key] ?? ""}
                placeholder={f.desc}
                onChange={(e) => setForm((prev) => ({ ...prev, [f.key]: e.target.value }))}
              />
            )}
            <p className="text-xs text-text-muted mt-1">{f.desc}</p>
          </div>
        ))}
        <div className="flex justify-end pt-2">
          <button
            onClick={() => void save()}
            disabled={saving}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
          >
            {saving ? "保存中…" : "保存配置"}
          </button>
        </div>
      </div>
    </div>
  );
}
