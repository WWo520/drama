// AdminNotifyPanel：平台配置 → 通知设置 Tab。
// 数据源：公告 GET /admin/announcements、轮播 GET /admin/banners（后端当前仅提供查询，新增/编辑规划中，只读展示）。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Mail, Image, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";

interface AnnouncementRow {
  id: number;
  title: string;
  content: string;
  start_at: string;
  end_at: string;
  sort_order: number;
}

interface BannerRow {
  id: number;
  title: string;
  image_url: string;
  jump_url: string;
  position: string;
  sort_order: number;
  is_active: boolean;
}

interface ListResp<T> {
  items: T[];
}

const TH = "px-3 py-3 text-left text-xs font-medium text-text-muted uppercase tracking-wider whitespace-nowrap";
const TD = "px-3 py-3 text-sm text-text-secondary whitespace-nowrap";

export default function AdminNotifyPanel() {
  const [anns, setAnns] = useState<AnnouncementRow[]>([]);
  const [banners, setBanners] = useState<BannerRow[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [a, b] = await Promise.all([
        api.get<ListResp<AnnouncementRow>>("/admin/announcements", { params: { page_size: 50 } }),
        api.get<ListResp<BannerRow>>("/admin/banners", { params: { page_size: 50 } }),
      ]);
      setAnns(a.items ?? []);
      setBanners(b.items ?? []);
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
          <Mail className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">通知设置</h3>
          <span className="text-xs text-text-muted">公告与轮播的生效内容（仅查询展示，新增/编辑接口规划中）</span>
        </div>
        <button
          onClick={() => void load()}
          disabled={loading}
          className="px-3 py-2 rounded-lg text-sm text-text-secondary bg-bg-card border border-[#2A2A3E] hover:border-[#3A3A4E] transition-colors flex items-center gap-2 disabled:opacity-50"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} /> 刷新
        </button>
      </div>

      {/* 公告列表 */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-[#2A2A3E]">
          <h4 className="text-sm font-medium text-white">平台公告</h4>
          <span className="text-xs text-text-muted">首页公告条读取生效时间窗内的公告（GET /announcements）</span>
        </div>
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>标题</th>
              <th className={TH}>内容</th>
              <th className={TH}>生效开始</th>
              <th className={TH}>生效结束</th>
              <th className={TH}>排序</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {anns.map((a) => (
              <tr key={a.id} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-medium"}>{a.title}</td>
                <td className={TD + " max-w-[280px] truncate"} title={a.content}>{a.content}</td>
                <td className={TD}>{a.start_at?.slice(0, 16).replace("T", " ") ?? "—"}</td>
                <td className={TD}>{a.end_at?.slice(0, 16).replace("T", " ") ?? "—"}</td>
                <td className={TD}>{a.sort_order ?? 0}</td>
              </tr>
            ))}
            {anns.length === 0 && !loading && (
              <tr>
                <td colSpan={5} className="px-3 py-8 text-center text-text-muted text-sm">暂无公告（生效时间窗外或未录入）</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 轮播 Banner 列表 */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-[#2A2A3E]">
          <Image className="w-4 h-4 text-amber-gold" />
          <h4 className="text-sm font-medium text-white">轮播 Banner</h4>
          <span className="text-xs text-text-muted">首页精选推荐轮播读取启用中的 Banner（GET /banners）</span>
        </div>
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>标题</th>
              <th className={TH}>图片</th>
              <th className={TH}>跳转地址</th>
              <th className={TH}>位置</th>
              <th className={TH}>排序</th>
              <th className={TH}>状态</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#2A2A3E]/60">
            {banners.map((b) => (
              <tr key={b.id} className="hover:bg-white/[0.02]">
                <td className={TD + " text-white font-medium"}>{b.title}</td>
                <td className={TD + " max-w-[200px] truncate font-mono text-xs"} title={b.image_url}>
                  {b.image_url || "—"}
                </td>
                <td className={TD + " max-w-[180px] truncate font-mono text-xs"} title={b.jump_url}>
                  {b.jump_url || "—"}
                </td>
                <td className={TD}>{b.position || "—"}</td>
                <td className={TD}>{b.sort_order ?? 0}</td>
                <td className={TD}>
                  {b.is_active ? (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-green-400 bg-green-400/10 border border-green-400/30">启用</span>
                  ) : (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium text-text-muted bg-[#2A2A3E]/60 border border-[#2A2A3E]">停用</span>
                  )}
                </td>
              </tr>
            ))}
            {banners.length === 0 && !loading && (
              <tr>
                <td colSpan={6} className="px-3 py-8 text-center text-text-muted text-sm">暂无 Banner（未录入或全部停用）</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
