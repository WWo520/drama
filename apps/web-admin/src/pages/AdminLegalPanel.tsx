// AdminLegalPanel：平台配置 → 协议管理 Tab。
// 数据源：平台配置 GET/PUT /admin/platform/config（group=legal，legal_* 键）。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { FileText, RefreshCw } from "lucide-react";

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

const FIELDS: { key: string; label: string; desc: string }[] = [
  { key: "legal_terms_of_service", label: "用户协议", desc: "注册/登录时展示的服务条款（支持换行）" },
  { key: "legal_privacy_policy", label: "隐私政策", desc: "个人信息收集与使用说明" },
  { key: "legal_copyright", label: "版权声明", desc: "平台内容版权与侵权投诉说明" },
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

export default function AdminLegalPanel() {
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
            config_value: form[f.key] ?? "",
            config_group: "legal",
            description: f.desc,
          }),
        ),
      );
      toast("协议内容已保存", "success");
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
          <FileText className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">协议管理</h3>
          <span className="text-xs text-text-muted">用户协议 / 隐私政策 / 版权声明（持久化到平台配置）</span>
        </div>
        <button
          onClick={() => void load()}
          disabled={loading}
          className="px-3 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors flex items-center gap-2 disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} /> 刷新
        </button>
      </div>

      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 space-y-5">
        {FIELDS.map((f) => (
          <div key={f.key}>
            <label className="block text-sm text-text-secondary mb-1.5">{f.label}</label>
            <textarea rows={6} className={inputCls} value={form[f.key] ?? ""} placeholder={f.desc} onChange={(e) => setForm((prev) => ({ ...prev, [f.key]: e.target.value }))} />
            <p className="text-xs text-text-muted mt-1">{f.desc}</p>
          </div>
        ))}
        <div className="flex justify-end pt-2">
          <button
            onClick={() => void save()}
            disabled={saving}
            className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
          >
            {saving ? "保存中…" : "保存协议"}
          </button>
        </div>
      </div>
    </div>
  );
}
