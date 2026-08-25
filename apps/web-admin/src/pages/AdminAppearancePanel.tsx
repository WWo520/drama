// AdminAppearancePanel：平台配置 → 外观定制 Tab。
// 数据源：主题 GET /admin/themes、风格库 GET /admin/style-library；编辑 PUT /admin/themes/:id、PUT /admin/style-library/:id。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Palette, Sparkles, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";
import Modal from "../components/Modal";

interface ThemeRow {
  id: number;
  theme_code: string;
  name: string;
  primary_color: string;
  accent_color: string;
  is_default: boolean;
  status: string;
}

interface StyleRow {
  id: number;
  style_code: string;
  name: string;
  category: string;
  scope: string;
  thumbnail_url: string;
  prompt_suffix_static: string;
  is_default: boolean;
}

interface ListResp<T> {
  items: T[];
}

const TH = "px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider whitespace-nowrap";
const TD = "px-3 py-3 text-sm text-text-secondary whitespace-nowrap";

function ColorDot({ color }: { color: string }) {
  return (
    <span
      className="inline-block w-4 h-4 rounded border border-white/20 align-middle mr-1.5"
      style={{ backgroundColor: color || "#0B0B1A" }}
    />
  );
}

export default function AdminAppearancePanel() {
  const [themes, setThemes] = useState<ThemeRow[]>([]);
  const [styles, setStyles] = useState<StyleRow[]>([]);
  const [loading, setLoading] = useState(false);

  // 主题编辑弹窗
  const [themeEdit, setThemeEdit] = useState<ThemeRow | null>(null);
  const [tForm, setTForm] = useState({ name: "", primary: "#0B0B1A", accent: "#F5B23C", status: "ON", isDefault: false });
  const [savingT, setSavingT] = useState(false);

  // 风格编辑弹窗
  const [styleEdit, setStyleEdit] = useState<StyleRow | null>(null);
  const [sForm, setSForm] = useState({ name: "", description: "", thumbnail: "", sortOrder: "0", enabled: true });
  const [savingS, setSavingS] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [t, s] = await Promise.all([
        api.get<ListResp<ThemeRow>>("/admin/themes", { params: { page_size: 50 } }),
        api.get<ListResp<StyleRow>>("/admin/style-library", { params: { page_size: 100 } }),
      ]);
      setThemes(t.items ?? []);
      setStyles(s.items ?? []);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // ---------- 主题编辑 ----------

  const openThemeEdit = (t: ThemeRow) => {
    setThemeEdit(t);
    setTForm({ name: t.name, primary: t.primary_color || "#0B0B1A", accent: t.accent_color || "#F5B23C", status: t.status || "ON", isDefault: t.is_default });
  };

  const saveTheme = async () => {
    if (!themeEdit) return;
    if (!tForm.name.trim()) {
      toast("主题名称不能为空", "error");
      return;
    }
    setSavingT(true);
    try {
      await api.put(`/admin/themes/${themeEdit.id}`, {
        name: tForm.name.trim(),
        primary_color: tForm.primary,
        accent_color: tForm.accent,
        status: tForm.status,
        is_default: tForm.isDefault,
      });
      toast("主题已保存", "success");
      setThemeEdit(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingT(false);
    }
  };

  // ---------- 风格编辑 ----------

  const openStyleEdit = (s: StyleRow) => {
    setStyleEdit(s);
    setSForm({ name: s.name, description: "", thumbnail: s.thumbnail_url || "", sortOrder: "0", enabled: true });
  };

  const saveStyle = async () => {
    if (!styleEdit) return;
    if (!sForm.name.trim()) {
      toast("风格名称不能为空", "error");
      return;
    }
    setSavingS(true);
    try {
      await api.put(`/admin/style-library/${styleEdit.id}`, {
        name: sForm.name.trim(),
        description: sForm.description,
        thumbnail_url: sForm.thumbnail.trim(),
        sort_order: Number(sForm.sortOrder) || 0,
        enabled: sForm.enabled,
      });
      toast("风格已保存", "success");
      setStyleEdit(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingS(false);
    }
  };

  const inputCls = "w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors";

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Palette className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">外观定制</h3>
          <span className="text-xs text-text-muted">主题与画面风格库（编辑保存后全局生效）</span>
        </div>
        <button
          onClick={() => void load()}
          disabled={loading}
          className="px-3 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors flex items-center gap-2 disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} /> 刷新
        </button>
      </div>

      {/* 界面主题 */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-[#2A2A3E]">
          <h4 className="text-sm font-medium text-white">界面主题</h4>
          <span className="text-xs text-text-muted">暗色电影为默认主题（themes 表）</span>
        </div>
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>编码</th>
              <th className={TH}>名称</th>
              <th className={TH}>主色</th>
              <th className={TH}>强调色</th>
              <th className={TH}>默认</th>
              <th className={TH}>状态</th>
              <th className={TH}>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {themes.map((t) => (
              <tr key={t.theme_code} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-mono text-xs"}>{t.theme_code}</td>
                <td className={TD + " text-white"}>{t.name}</td>
                <td className={TD}><ColorDot color={t.primary_color} />{t.primary_color}</td>
                <td className={TD}><ColorDot color={t.accent_color} />{t.accent_color}</td>
                <td className={TD}>{t.is_default ? "✓" : "—"}</td>
                <td className={TD}>
                  {t.status === "ON" ? (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-green-400 bg-green-400/10 border border-green-400/30">启用</span>
                  ) : (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-text-muted bg-[#2A2A3E]/60 border border-[#2A2A3E]">停用</span>
                  )}
                </td>
                <td className={TD}>
                  <button onClick={() => openThemeEdit(t)} className="text-amber-gold hover:text-amber-300 text-sm transition-colors">编辑</button>
                </td>
              </tr>
            ))}
            {themes.length === 0 && !loading && (
              <tr>
                <td colSpan={7} className="px-3 py-8 text-center text-text-muted text-sm">暂无主题数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 画面风格库 */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-[#2A2A3E]">
          <Sparkles className="w-4 h-4 text-amber-gold" />
          <h4 className="text-sm font-medium text-white">画面风格库</h4>
          <span className="text-xs text-text-muted">创作页风格选择器数据源（style_library 表）</span>
        </div>
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>编码</th>
              <th className={TH}>名称</th>
              <th className={TH}>分类</th>
              <th className={TH}>作用域</th>
              <th className={TH}>Prompt 后缀</th>
              <th className={TH}>默认</th>
              <th className={TH}>操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {styles.map((s) => (
              <tr key={s.style_code} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-mono text-xs"}>{s.style_code}</td>
                <td className={TD + " text-white"}>{s.name}</td>
                <td className={TD}>{s.category || "—"}</td>
                <td className={TD}>{s.scope || "—"}</td>
                <td className={TD + " max-w-[240px] truncate font-mono text-xs"} title={s.prompt_suffix_static || ""}>
                  {s.prompt_suffix_static || "—"}
                </td>
                <td className={TD}>{s.is_default ? "✓" : "—"}</td>
                <td className={TD}>
                  <button onClick={() => openStyleEdit(s)} className="text-amber-gold hover:text-amber-300 text-sm transition-colors">编辑</button>
                </td>
              </tr>
            ))}
            {styles.length === 0 && !loading && (
              <tr>
                <td colSpan={7} className="px-3 py-8 text-center text-text-muted text-sm">暂无风格数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 主题编辑弹窗 */}
      <Modal
        title="编辑主题"
        subtitle={themeEdit ? `${themeEdit.theme_code}` : undefined}
        open={themeEdit !== null}
        onClose={() => setThemeEdit(null)}
        footer={
          <>
            <button onClick={() => setThemeEdit(null)} className="px-4 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors">取消</button>
            <button
              onClick={() => void saveTheme()}
              disabled={savingT}
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
            >
              {savingT ? "保存中…" : "保存主题"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">名称</label>
            <input type="text" className={inputCls} value={tForm.name} onChange={(e) => setTForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">主色</label>
              <div className="flex items-center gap-2">
                <input type="color" value={tForm.primary} onChange={(e) => setTForm((f) => ({ ...f, primary: e.target.value }))} className="w-10 h-9 rounded cursor-pointer bg-transparent border border-[#2A2A3E]" />
                <input type="text" className={inputCls} value={tForm.primary} onChange={(e) => setTForm((f) => ({ ...f, primary: e.target.value }))} />
              </div>
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">强调色</label>
              <div className="flex items-center gap-2">
                <input type="color" value={tForm.accent} onChange={(e) => setTForm((f) => ({ ...f, accent: e.target.value }))} className="w-10 h-9 rounded cursor-pointer bg-transparent border border-[#2A2A3E]" />
                <input type="text" className={inputCls} value={tForm.accent} onChange={(e) => setTForm((f) => ({ ...f, accent: e.target.value }))} />
              </div>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">状态</label>
              <select className={inputCls} value={tForm.status} onChange={(e) => setTForm((f) => ({ ...f, status: e.target.value }))}>
                <option value="ON">启用</option>
                <option value="OFF">停用</option>
              </select>
            </div>
            <div className="flex items-end pb-2">
              <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer">
                <input type="checkbox" checked={tForm.isDefault} onChange={(e) => setTForm((f) => ({ ...f, isDefault: e.target.checked }))} className="custom-checkbox" />
                设为默认主题
              </label>
            </div>
          </div>
        </div>
      </Modal>

      {/* 风格编辑弹窗 */}
      <Modal
        title="编辑风格"
        subtitle={styleEdit ? `${styleEdit.style_code} · ${styleEdit.category || "—"}` : undefined}
        open={styleEdit !== null}
        onClose={() => setStyleEdit(null)}
        footer={
          <>
            <button onClick={() => setStyleEdit(null)} className="px-4 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors">取消</button>
            <button
              onClick={() => void saveStyle()}
              disabled={savingS}
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
            >
              {savingS ? "保存中…" : "保存风格"}
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">名称</label>
            <input type="text" className={inputCls} value={sForm.name} onChange={(e) => setSForm((f) => ({ ...f, name: e.target.value }))} />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">描述</label>
            <textarea rows={2} className={inputCls} value={sForm.description} onChange={(e) => setSForm((f) => ({ ...f, description: e.target.value }))} />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1.5">缩略图 URL</label>
            <input type="text" className={inputCls} value={sForm.thumbnail} placeholder="https://..." onChange={(e) => setSForm((f) => ({ ...f, thumbnail: e.target.value }))} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1.5">排序</label>
              <input type="number" className={inputCls} value={sForm.sortOrder} onChange={(e) => setSForm((f) => ({ ...f, sortOrder: e.target.value }))} />
            </div>
            <div className="flex items-end pb-2">
              <label className="flex items-center gap-2 text-sm text-text-secondary cursor-pointer">
                <input type="checkbox" checked={sForm.enabled} onChange={(e) => setSForm((f) => ({ ...f, enabled: e.target.checked }))} className="custom-checkbox" />
                启用（创作页可选）
              </label>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
}
