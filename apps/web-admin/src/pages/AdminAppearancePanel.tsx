// AdminAppearancePanel：平台配置 → 外观定制 Tab。
// 数据源：主题 GET /admin/themes、风格库 GET /admin/style-library（后端当前仅提供查询，编辑规划中，只读展示）。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Palette, Sparkles, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";

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

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Palette className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">外观定制</h3>
          <span className="text-xs text-text-muted">主题与画面风格库（仅查询展示，编辑接口规划中）</span>
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
              </tr>
            ))}
            {themes.length === 0 && !loading && (
              <tr>
                <td colSpan={6} className="px-3 py-8 text-center text-text-muted text-sm">暂无主题数据</td>
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
              </tr>
            ))}
            {styles.length === 0 && !loading && (
              <tr>
                <td colSpan={6} className="px-3 py-8 text-center text-text-muted text-sm">暂无风格数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
