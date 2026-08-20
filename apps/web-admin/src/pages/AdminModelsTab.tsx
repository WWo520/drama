// AdminModelsTab：计费财务页「模型配置」Tab（融合清单批次 5：大模型配置 CRUD）。
// 设计稿无此区块，按管理端视觉自建；对应后端（admin_extra_service / admin_service）：
//   厂商 GET/POST /admin/providers、PUT /admin/providers/:id
//   模型 GET /admin/model-list、POST /admin/model-create、PUT /admin/models（按 model_key 更新）
//   密钥 GET /admin/model-key-list、POST /admin/model-key-create、PUT /admin/model-key-update/:id、
//        POST /admin/model-key-revoke/:id（密文入库，明文仅创建时返回 key_prefix）
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Plus, KeyRound, Cpu, Boxes } from "lucide-react";

import { toast, errMessage, fmtNum } from "../lib";
import Modal from "../components/Modal";

// ---------- 类型（与 model_keystore.go 返回字段对齐） ----------

interface ProviderRow {
  id: number;
  name: string;
  display_name: string;
  capabilities: string; // JSON 字符串数组
  base_url: string;
  enabled: boolean;
  sort_order: number;
}

interface ModelRow {
  id: number;
  provider_id: number;
  provider_name: string;
  model_key: string;
  name: string;
  capability: string;
  cost_points_per_call: number;
  enabled: boolean;
  sort_order: number;
}

interface KeyRow {
  id: number;
  provider_id: number;
  provider_name: string;
  name: string;
  key_prefix: string;
  rate_limit_rpm: number;
  rate_limit_qps: number;
  concurrent_limit: number;
  daily_quota_limit: number;
  daily_quota_used: number;
  enabled: boolean;
  priority: number;
  last_used_at: string;
}

// 能力枚举（与后端 capability oneof 一致；厂商 capabilities 从同一集合多选）
const CAPABILITY_OPTIONS = [
  { code: "LLM", label: "LLM 文本" },
  { code: "T2I", label: "T2I 文生图" },
  { code: "I2I", label: "I2I 图生图" },
  { code: "T2V", label: "T2V 文生视频" },
  { code: "I2V", label: "I2V 图生视频" },
  { code: "AUDIO", label: "AUDIO 音频" },
] as const;

const CAPABILITY_LABEL: Record<string, string> = Object.fromEntries(
  CAPABILITY_OPTIONS.map((c) => [c.code, c.label]),
);

function parseCaps(raw: string): string[] {
  try {
    const v = JSON.parse(raw);
    return Array.isArray(v) ? v.map(String) : [];
  } catch {
    return [];
  }
}

// 状态徽章（避免跨页 css 依赖，内联 Tailwind）
function EnabledTag({ enabled }: { enabled: boolean }) {
  return enabled ? (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-green-400 bg-green-400/10 border border-green-400/30">启用</span>
  ) : (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-text-muted bg-[#2A2A3E]/60 border border-[#2A2A3E]">停用</span>
  );
}

const TH = "px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider whitespace-nowrap";
const TD = "px-3 py-0 text-sm text-text-secondary whitespace-nowrap";

// ---------- 组件 ----------

export default function AdminModelsTab() {
  const [providers, setProviders] = useState<ProviderRow[]>([]);
  const [models, setModels] = useState<ModelRow[]>([]);
  const [keys, setKeys] = useState<KeyRow[]>([]);
  const [loaded, setLoaded] = useState(false);

  // 厂商表单（新建 / 编辑）
  const [providerForm, setProviderForm] = useState<{ editing: ProviderRow | null } | null>(null);
  const [pName, setPName] = useState("");
  const [pDisplay, setPDisplay] = useState("");
  const [pCaps, setPCaps] = useState<Set<string>>(new Set(["LLM"]));
  const [pBaseURL, setPBaseURL] = useState("");
  const [savingProvider, setSavingProvider] = useState(false);

  // 模型表单（仅新建；启停走 PUT /admin/models 行内）
  const [modelFormOpen, setModelFormOpen] = useState(false);
  const [mProvider, setMProvider] = useState("");
  const [mKey, setMKey] = useState("");
  const [mName, setMName] = useState("");
  const [mCap, setMCap] = useState("LLM");
  const [mCost, setMCost] = useState("1");
  const [savingModel, setSavingModel] = useState(false);

  // 密钥表单（新建 / 编辑限额）
  const [keyForm, setKeyForm] = useState<{ editing: KeyRow | null } | null>(null);
  const [kProvider, setKProvider] = useState("");
  const [kName, setKName] = useState("");
  const [kSecret, setKSecret] = useState("");
  const [kRpm, setKRpm] = useState("60");
  const [kQps, setKQps] = useState("5");
  const [kConc, setKConc] = useState("4");
  const [kPrio, setKPrio] = useState("50");
  const [savingKey, setSavingKey] = useState(false);

  // 密钥吊销确认
  const [revokeTarget, setRevokeTarget] = useState<KeyRow | null>(null);
  const [revoking, setRevoking] = useState(false);

  const loadAll = useCallback(async () => {
    try {
      const [pv, md, ky] = await Promise.all([
        api.get<{ items: ProviderRow[] }>("/admin/providers"),
        api.get<{ items: ModelRow[] }>("/admin/model-list"),
        api.get<{ items: KeyRow[] }>("/admin/model-key-list"),
      ]);
      setProviders(pv.items ?? []);
      setModels(md.items ?? []);
      setKeys(ky.items ?? []);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  // ---------- 厂商 ----------

  const openProviderForm = (row: ProviderRow | null) => {
    setProviderForm({ editing: row });
    if (row) {
      setPName(row.name);
      setPDisplay(row.display_name);
      setPCaps(new Set(parseCaps(row.capabilities)));
      setPBaseURL(row.base_url);
    } else {
      setPName("");
      setPDisplay("");
      setPCaps(new Set(["LLM"]));
      setPBaseURL("");
    }
  };

  const submitProvider = async () => {
    const editing = providerForm?.editing ?? null;
    const name = pName.trim();
    const displayName = pDisplay.trim();
    const baseURL = pBaseURL.trim();
    if (!editing && !name) {
      toast("请填写厂商标识名", "error");
      return;
    }
    if (!displayName) {
      toast("请填写显示名", "error");
      return;
    }
    if (pCaps.size === 0) {
      toast("请至少勾选一项能力", "error");
      return;
    }
    if (!/^https?:\/\//.test(baseURL)) {
      toast("Base URL 需以 http(s):// 开头", "error");
      return;
    }
    setSavingProvider(true);
    try {
      if (editing) {
        await api.put(`/admin/providers/${editing.id}`, {
          display_name: displayName,
          capabilities: Array.from(pCaps),
          base_url: baseURL,
        });
        toast(`已更新厂商 ${displayName}`, "success");
      } else {
        await api.post("/admin/providers", {
          name,
          display_name: displayName,
          capabilities: Array.from(pCaps),
          base_url: baseURL,
        });
        toast(`已新增厂商 ${displayName}`, "success");
      }
      setProviderForm(null);
      void loadAll();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingProvider(false);
    }
  };

  const toggleProvider = async (row: ProviderRow) => {
    try {
      await api.put(`/admin/providers/${row.id}`, { enabled: !row.enabled });
      setProviders((prev) => prev.map((p) => (p.id === row.id ? { ...p, enabled: !row.enabled } : p)));
      toast(row.enabled ? `已停用厂商 ${row.display_name}` : `已启用厂商 ${row.display_name}`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // ---------- 模型 ----------

  const submitModel = async () => {
    const providerId = Number(mProvider);
    const modelKey = mKey.trim();
    const name = mName.trim();
    const cost = Number(mCost);
    if (!providerId) {
      toast("请选择所属厂商", "error");
      return;
    }
    if (!modelKey || !name) {
      toast("请填写模型 Key 与名称", "error");
      return;
    }
    if (!Number.isInteger(cost) || cost < 0) {
      toast("单次消耗积分需为非负整数", "error");
      return;
    }
    setSavingModel(true);
    try {
      await api.post("/admin/model-create", {
        provider_id: providerId,
        model_key: modelKey,
        name,
        capability: mCap,
        cost_points_per_call: cost,
      });
      toast(`已新增模型 ${name}`, "success");
      setModelFormOpen(false);
      void loadAll();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingModel(false);
    }
  };

  // 启停模型：PUT /admin/models 按 model_key 全量更新
  const toggleModel = async (row: ModelRow) => {
    try {
      await api.put("/admin/models", {
        model_key: row.model_key,
        name: row.name,
        capability: row.capability,
        cost_points: row.cost_points_per_call,
        enabled: !row.enabled,
      });
      setModels((prev) => prev.map((m) => (m.id === row.id ? { ...m, enabled: !row.enabled } : m)));
      toast(row.enabled ? `已停用模型 ${row.name}` : `已启用模型 ${row.name}`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // ---------- 密钥 ----------

  const openKeyForm = (row: KeyRow | null) => {
    setKeyForm({ editing: row });
    if (row) {
      setKProvider(String(row.provider_id));
      setKName(row.name);
      setKSecret("");
      setKRpm(String(row.rate_limit_rpm));
      setKQps(String(row.rate_limit_qps));
      setKConc(String(row.concurrent_limit));
      setKPrio(String(row.priority));
    } else {
      setKProvider(providers[0] ? String(providers[0].id) : "");
      setKName("");
      setKSecret("");
      setKRpm("60");
      setKQps("5");
      setKConc("4");
      setKPrio("50");
    }
  };

  const submitKey = async () => {
    const editing = keyForm?.editing ?? null;
    const name = kName.trim();
    const rpm = Number(kRpm);
    const qps = Number(kQps);
    const conc = Number(kConc);
    const prio = Number(kPrio);
    if (!editing) {
      if (!Number(kProvider)) {
        toast("请选择所属厂商", "error");
        return;
      }
      if (!name) {
        toast("请填写密钥名称", "error");
        return;
      }
      if (kSecret.trim().length < 8) {
        toast("API Key 至少 8 位", "error");
        return;
      }
    }
    if (!Number.isInteger(rpm) || rpm < 0 || rpm > 100000) {
      toast("RPM 需为 0-100000 的整数", "error");
      return;
    }
    if (!Number.isInteger(qps) || qps < 0 || qps > 100) {
      toast("QPS 需为 0-100 的整数", "error");
      return;
    }
    if (!Number.isInteger(conc) || conc < 1 || conc > 64) {
      toast("并发上限需为 1-64 的整数", "error");
      return;
    }
    if (!Number.isInteger(prio) || prio < 0 || prio > 100) {
      toast("优先级需为 0-100 的整数", "error");
      return;
    }
    setSavingKey(true);
    try {
      if (editing) {
        await api.put(`/admin/model-key-update/${editing.id}`, {
          rate_limit_rpm: rpm,
          rate_limit_qps: qps,
          concurrent_limit: conc,
          priority: prio,
        });
        toast(`已更新密钥 ${editing.name} 的限额`, "success");
      } else {
        const resp = await api.post<{ key_prefix: string }>("/admin/model-key-create", {
          provider_id: Number(kProvider),
          name,
          api_key: kSecret.trim(),
          rate_limit_rpm: rpm,
          rate_limit_qps: qps,
          concurrent_limit: conc,
          priority: prio,
        });
        toast(`密钥已创建（前缀 ${resp.key_prefix}），明文仅展示一次，请妥善保存`, "success");
      }
      setKeyForm(null);
      void loadAll();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingKey(false);
    }
  };

  const submitRevoke = async () => {
    if (!revokeTarget) return;
    setRevoking(true);
    try {
      await api.post(`/admin/model-key-revoke/${revokeTarget.id}`);
      toast(`已吊销密钥 ${revokeTarget.name}`, "success");
      setRevokeTarget(null);
      void loadAll();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setRevoking(false);
    }
  };

  // ---------- 渲染 ----------

  if (!loaded) {
    return <div className="py-16 text-center text-sm text-text-muted">加载中…</div>;
  }

  return (
    <div className="space-y-6">
      {/* ====== 厂商管理 ====== */}
      <section>
        <div className="flex items-center justify-between mb-3 px-1">
          <h3 className="text-sm font-semibold text-white flex items-center">
            <Boxes className="w-4 h-4 text-amber-gold mr-2" />
            大模型厂商
          </h3>
          <button
            onClick={() => openProviderForm(null)}
            className="flex items-center px-3 py-1.5 bg-gradient-to-r from-amber-gold to-amber-dim rounded-md text-xs text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Plus className="w-3.5 h-3.5 mr-1.5" />
            新增厂商
          </button>
        </div>
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-bg-card-light border-b border-[#2A2A3E]">
                  <th className={TH}>ID</th>
                  <th className={TH}>标识名</th>
                  <th className={TH}>显示名</th>
                  <th className={TH}>Base URL</th>
                  <th className={TH}>能力</th>
                  <th className={`${TH} text-center`}>状态</th>
                  <th className={`${TH} text-center`}>操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#2A2A3E]">
                {providers.map((p) => (
                  <tr key={p.id} className="table-row transition-colors h-12">
                    <td className={`${TD} font-mono`}>{p.id}</td>
                    <td className={`${TD} text-white`}>{p.name}</td>
                    <td className={TD}>{p.display_name}</td>
                    <td className={`${TD} max-w-[240px] truncate`} title={p.base_url}>{p.base_url}</td>
                    <td className={TD}>
                      <div className="flex items-center gap-1 flex-wrap">
                        {parseCaps(p.capabilities).map((c) => (
                          <span key={c} className="px-1.5 py-0.5 rounded text-[10px] text-amber-gold bg-amber-gold/10 border border-amber-gold/30">
                            {c}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className={`${TD} text-center`}><EnabledTag enabled={p.enabled} /></td>
                    <td className={`${TD} text-center`}>
                      <div className="flex items-center justify-center gap-3 text-xs">
                        <a href="#" onClick={(e) => { e.preventDefault(); openProviderForm(p); }} className="action-link">编辑</a>
                        <a
                          href="#"
                          onClick={(e) => { e.preventDefault(); void toggleProvider(p); }}
                          className="action-link"
                          style={{ color: p.enabled ? "#f59e0b" : "#22c55e" }}
                        >
                          {p.enabled ? "停用" : "启用"}
                        </a>
                      </div>
                    </td>
                  </tr>
                ))}
                {providers.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-10 text-center text-sm text-text-muted">暂无厂商，点击右上角新增</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ====== 模型管理 ====== */}
      <section>
        <div className="flex items-center justify-between mb-3 px-1">
          <h3 className="text-sm font-semibold text-white flex items-center">
            <Cpu className="w-4 h-4 text-cyan-400 mr-2" />
            模型列表
          </h3>
          <button
            onClick={() => {
              setMProvider(providers[0] ? String(providers[0].id) : "");
              setMKey("");
              setMName("");
              setMCap("LLM");
              setMCost("1");
              setModelFormOpen(true);
            }}
            disabled={providers.length === 0}
            className="flex items-center px-3 py-1.5 bg-gradient-to-r from-amber-gold to-amber-dim rounded-md text-xs text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Plus className="w-3.5 h-3.5 mr-1.5" />
            新增模型
          </button>
        </div>
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-bg-card-light border-b border-[#2A2A3E]">
                  <th className={TH}>ID</th>
                  <th className={TH}>厂商</th>
                  <th className={TH}>模型 Key</th>
                  <th className={TH}>名称</th>
                  <th className={TH}>能力</th>
                  <th className={`${TH} text-right`}>单次积分</th>
                  <th className={`${TH} text-center`}>状态</th>
                  <th className={`${TH} text-center`}>操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#2A2A3E]">
                {models.map((m) => (
                  <tr key={m.id} className="table-row transition-colors h-12">
                    <td className={`${TD} font-mono`}>{m.id}</td>
                    <td className={TD}>{m.provider_name || "—"}</td>
                    <td className={`${TD} font-mono text-white`}>{m.model_key}</td>
                    <td className={TD}>{m.name}</td>
                    <td className={TD}>{CAPABILITY_LABEL[m.capability] ?? m.capability}</td>
                    <td className={`${TD} text-right font-mono text-amber-gold`}>{fmtNum(m.cost_points_per_call)}</td>
                    <td className={`${TD} text-center`}><EnabledTag enabled={m.enabled} /></td>
                    <td className={`${TD} text-center`}>
                      <a
                        href="#"
                        onClick={(e) => { e.preventDefault(); void toggleModel(m); }}
                        className="action-link"
                        style={{ color: m.enabled ? "#f59e0b" : "#22c55e" }}
                      >
                        {m.enabled ? "停用" : "启用"}
                      </a>
                    </td>
                  </tr>
                ))}
                {models.length === 0 && (
                  <tr>
                    <td colSpan={8} className="py-10 text-center text-sm text-text-muted">暂无模型</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ====== API 密钥管理 ====== */}
      <section>
        <div className="flex items-center justify-between mb-3 px-1">
          <h3 className="text-sm font-semibold text-white flex items-center">
            <KeyRound className="w-4 h-4 text-purple-400 mr-2" />
            API 密钥
            <span className="ml-2 text-xs text-text-muted font-normal">密文入库，明文仅创建时返回一次</span>
          </h3>
          <button
            onClick={() => openKeyForm(null)}
            disabled={providers.length === 0}
            className="flex items-center px-3 py-1.5 bg-gradient-to-r from-amber-gold to-amber-dim rounded-md text-xs text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Plus className="w-3.5 h-3.5 mr-1.5" />
            新增密钥
          </button>
        </div>
        <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-bg-card-light border-b border-[#2A2A3E]">
                  <th className={TH}>ID</th>
                  <th className={TH}>厂商</th>
                  <th className={TH}>名称</th>
                  <th className={TH}>前缀</th>
                  <th className={`${TH} text-right`}>RPM</th>
                  <th className={`${TH} text-right`}>QPS</th>
                  <th className={`${TH} text-right`}>并发</th>
                  <th className={`${TH} text-right`}>优先级</th>
                  <th className={`${TH} text-center`}>状态</th>
                  <th className={`${TH} text-center`}>操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#2A2A3E]">
                {keys.map((k) => (
                  <tr key={k.id} className="table-row transition-colors h-12">
                    <td className={`${TD} font-mono`}>{k.id}</td>
                    <td className={TD}>{k.provider_name}</td>
                    <td className={`${TD} text-white`}>{k.name}</td>
                    <td className={`${TD} font-mono`}>{k.key_prefix}…</td>
                    <td className={`${TD} text-right font-mono`}>{fmtNum(k.rate_limit_rpm)}</td>
                    <td className={`${TD} text-right font-mono`}>{fmtNum(k.rate_limit_qps)}</td>
                    <td className={`${TD} text-right font-mono`}>{k.concurrent_limit}</td>
                    <td className={`${TD} text-right font-mono`}>{k.priority}</td>
                    <td className={`${TD} text-center`}><EnabledTag enabled={k.enabled} /></td>
                    <td className={`${TD} text-center`}>
                      <div className="flex items-center justify-center gap-3 text-xs">
                        <a href="#" onClick={(e) => { e.preventDefault(); openKeyForm(k); }} className="action-link">限额</a>
                        <a href="#" onClick={(e) => { e.preventDefault(); setRevokeTarget(k); }} className="action-link-danger">吊销</a>
                      </div>
                    </td>
                  </tr>
                ))}
                {keys.length === 0 && (
                  <tr>
                    <td colSpan={10} className="py-10 text-center text-sm text-text-muted">暂无密钥</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      {/* ====== 厂商表单弹窗 ====== */}
      <Modal
        open={providerForm !== null}
        onClose={() => setProviderForm(null)}
        title={providerForm?.editing ? "编辑厂商" : "新增厂商"}
        subtitle="厂商为模型与密钥的归属主体"
      >
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-text-muted mb-2">标识名（英文短名，创建后不可改）</label>
            <input
              type="text"
              value={pName}
              disabled={providerForm?.editing != null}
              placeholder="如 openai / volcengine"
              onChange={(e) => setPName(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors disabled:opacity-50"
            />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">显示名</label>
            <input
              type="text"
              value={pDisplay}
              placeholder="如 OpenAI"
              onChange={(e) => setPDisplay(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
            />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">能力（多选）</label>
            <div className="flex items-center gap-2 flex-wrap">
              {CAPABILITY_OPTIONS.map((c) => (
                <label
                  key={c.code}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs cursor-pointer transition-colors ${
                    pCaps.has(c.code)
                      ? "border-amber-gold/60 text-amber-gold bg-amber-gold/10"
                      : "border-[#2A2A3E] text-text-secondary hover:border-[#3A3A4E]"
                  }`}
                >
                  <input
                    type="checkbox"
                    className="custom-checkbox"
                    checked={pCaps.has(c.code)}
                    onChange={() =>
                      setPCaps((prev) => {
                        const next = new Set(prev);
                        if (next.has(c.code)) next.delete(c.code);
                        else next.add(c.code);
                        return next;
                      })
                    }
                  />
                  {c.label}
                </label>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">Base URL</label>
            <input
              type="text"
              value={pBaseURL}
              placeholder="https://api.example.com/v1"
              onChange={(e) => setPBaseURL(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
            />
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button
            onClick={() => setProviderForm(null)}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
          >
            取消
          </button>
          <button
            onClick={submitProvider}
            disabled={savingProvider}
            className="px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {savingProvider ? "保存中…" : "保存"}
          </button>
        </div>
      </Modal>

      {/* ====== 模型表单弹窗 ====== */}
      <Modal open={modelFormOpen} onClose={() => setModelFormOpen(false)} title="新增模型">
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-text-muted mb-2">所属厂商</label>
            <select
              value={mProvider}
              onChange={(e) => setMProvider(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
            >
              {providers.map((p) => (
                <option key={p.id} value={p.id}>{p.display_name}（{p.name}）</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">模型 Key（调用标识）</label>
            <input
              type="text"
              value={mKey}
              placeholder="如 gpt-4o / doubao-pro-32k"
              onChange={(e) => setMKey(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
            />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">显示名称</label>
            <input
              type="text"
              value={mName}
              onChange={(e) => setMName(e.target.value)}
              className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
            />
          </div>
          <div className="flex items-center gap-3">
            <div className="flex-1">
              <label className="block text-xs text-text-muted mb-2">能力</label>
              <select
                value={mCap}
                onChange={(e) => setMCap(e.target.value)}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
              >
                {CAPABILITY_OPTIONS.map((c) => (
                  <option key={c.code} value={c.code}>{c.label}（{c.code}）</option>
                ))}
              </select>
            </div>
            <div className="w-40">
              <label className="block text-xs text-text-muted mb-2">单次积分</label>
              <input
                type="number"
                min={0}
                value={mCost}
                onChange={(e) => setMCost(e.target.value)}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
              />
            </div>
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button
            onClick={() => setModelFormOpen(false)}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
          >
            取消
          </button>
          <button
            onClick={submitModel}
            disabled={savingModel}
            className="px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {savingModel ? "创建中…" : "创建"}
          </button>
        </div>
      </Modal>

      {/* ====== 密钥表单弹窗（新建含密文 / 编辑仅限额） ====== */}
      <Modal
        open={keyForm !== null}
        onClose={() => setKeyForm(null)}
        title={keyForm?.editing ? "调整密钥限额" : "新增 API 密钥"}
        subtitle={keyForm?.editing ? `${keyForm.editing.name} · ${keyForm.editing.key_prefix}…（密文不可查看）` : undefined}
      >
        <div className="space-y-4">
          {!keyForm?.editing && (
            <>
              <div>
                <label className="block text-xs text-text-muted mb-2">所属厂商</label>
                <select
                  value={kProvider}
                  onChange={(e) => setKProvider(e.target.value)}
                  className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
                >
                  {providers.map((p) => (
                    <option key={p.id} value={p.id}>{p.display_name}（{p.name}）</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs text-text-muted mb-2">密钥名称</label>
                <input
                  type="text"
                  value={kName}
                  placeholder="如 生产环境主密钥"
                  onChange={(e) => setKName(e.target.value)}
                  className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
                />
              </div>
              <div>
                <label className="block text-xs text-text-muted mb-2">API Key（密文入库，保存后不可再查看）</label>
                <input
                  type="password"
                  autoComplete="new-password"
                  value={kSecret}
                  placeholder="sk-..."
                  onChange={(e) => setKSecret(e.target.value)}
                  className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors"
                />
              </div>
            </>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-text-muted mb-2">RPM（每分钟请求数）</label>
              <input
                type="number"
                min={0}
                max={100000}
                value={kRpm}
                onChange={(e) => setKRpm(e.target.value)}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
              />
            </div>
            <div>
              <label className="block text-xs text-text-muted mb-2">QPS（每秒请求数）</label>
              <input
                type="number"
                min={0}
                max={100}
                value={kQps}
                onChange={(e) => setKQps(e.target.value)}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
              />
            </div>
            <div>
              <label className="block text-xs text-text-muted mb-2">并发上限（1-64）</label>
              <input
                type="number"
                min={1}
                max={64}
                value={kConc}
                onChange={(e) => setKConc(e.target.value)}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
              />
            </div>
            <div>
              <label className="block text-xs text-text-muted mb-2">优先级（0-100，高优先）</label>
              <input
                type="number"
                min={0}
                max={100}
                value={kPrio}
                onChange={(e) => setKPrio(e.target.value)}
                className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50 transition-colors"
              />
            </div>
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button
            onClick={() => setKeyForm(null)}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
          >
            取消
          </button>
          <button
            onClick={submitKey}
            disabled={savingKey}
            className="px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {savingKey ? "保存中…" : keyForm?.editing ? "保存限额" : "创建密钥"}
          </button>
        </div>
      </Modal>

      {/* ====== 吊销确认弹窗 ====== */}
      <Modal
        open={revokeTarget !== null}
        onClose={() => setRevokeTarget(null)}
        title="吊销密钥"
        width={400}
      >
        <p className="text-sm text-white">
          确定吊销 <span className="text-amber-gold">{revokeTarget?.name}</span>
          （{revokeTarget?.provider_name} · {revokeTarget?.key_prefix}…）吗？
        </p>
        <p className="text-xs text-text-muted mt-2">吊销后使用该密钥的调用将立即失败，此操作不可恢复。</p>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button
            onClick={() => setRevokeTarget(null)}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors"
          >
            取消
          </button>
          <button
            onClick={submitRevoke}
            disabled={revoking}
            className="px-4 py-2 bg-red-500/90 hover:bg-red-500 rounded-lg text-sm text-white font-semibold transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {revoking ? "吊销中…" : "确认吊销"}
          </button>
        </div>
      </Modal>
    </div>
  );
}
