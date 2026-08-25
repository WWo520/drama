// AdminNotifyPanel：平台配置 → 通知设置 Tab。
// 数据源：公告 GET /admin/announcements、轮播 GET /admin/banners（真数据）。
// 写接口：公告 PUT/DELETE /admin/announcements[/:id]、Banner POST/PUT/DELETE /admin/banners[/:id]。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import { Mail, Image, Plus, RefreshCw } from "lucide-react";

import { toast, errMessage } from "../lib";
import Modal from "../components/Modal";

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

// datetime-local 值 → 后端 timestamptz
function toTS(v: string): string {
  return v ? `${v}:00Z` : "";
}

export default function AdminNotifyPanel() {
  const [anns, setAnns] = useState<AnnouncementRow[]>([]);
  const [banners, setBanners] = useState<BannerRow[]>([]);
  const [loading, setLoading] = useState(false);

  // 公告表单（null=关闭；editing=编辑）
  const [annForm, setAnnForm] = useState<{ editing: AnnouncementRow | null } | null>(null);
  const [aTitle, setATitle] = useState("");
  const [aContent, setAContent] = useState("");
  const [aStart, setAStart] = useState("");
  const [aEnd, setAEnd] = useState("");
  const [aSort, setASort] = useState("0");
  const [savingAnn, setSavingAnn] = useState(false);

  // Banner 表单
  const [bannerForm, setBannerForm] = useState<{ editing: BannerRow | null } | null>(null);
  const [bTitle, setBTitle] = useState("");
  const [bImage, setBImage] = useState("");
  const [bJump, setBJump] = useState("");
  const [bPos, setBPos] = useState("home");
  const [bSort, setBSort] = useState("0");
  const [bActive, setBActive] = useState(true);
  const [savingBanner, setSavingBanner] = useState(false);

  // 删除确认
  const [delAnn, setDelAnn] = useState<AnnouncementRow | null>(null);
  const [delBanner, setDelBanner] = useState<BannerRow | null>(null);

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

  // ---------- 公告 ----------

  const openAnnForm = (row: AnnouncementRow | null) => {
    setAnnForm({ editing: row });
    setATitle(row?.title ?? "");
    setAContent(row?.content ?? "");
    setAStart(row?.start_at?.slice(0, 16) ?? "");
    setAEnd(row?.end_at?.slice(0, 16) ?? "");
    setASort(String(row?.sort_order ?? 0));
  };

  const submitAnn = async () => {
    if (!aTitle.trim() || !aContent.trim() || !aStart || !aEnd) {
      toast("请完整填写标题、内容与生效时间", "error");
      return;
    }
    setSavingAnn(true);
    try {
      const body = { title: aTitle.trim(), content: aContent.trim(), start_at: toTS(aStart), end_at: toTS(aEnd), sort_order: Number(aSort) || 0 };
      if (annForm?.editing) {
        await api.put(`/admin/announcements/${annForm.editing.id}`, body);
        toast("公告已更新", "success");
      } else {
        await api.put("/admin/announcements", body);
        toast("公告已创建", "success");
      }
      setAnnForm(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingAnn(false);
    }
  };

  const confirmDelAnn = async () => {
    if (!delAnn) return;
    try {
      await api.delete(`/admin/announcements/${delAnn.id}`);
      toast("公告已删除", "success");
      setDelAnn(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // ---------- Banner ----------

  const openBannerForm = (row: BannerRow | null) => {
    setBannerForm({ editing: row });
    setBTitle(row?.title ?? "");
    setBImage(row?.image_url ?? "");
    setBJump(row?.jump_url ?? "");
    setBPos(row?.position ?? "home");
    setBSort(String(row?.sort_order ?? 0));
    setBActive(row?.is_active ?? true);
  };

  const submitBanner = async () => {
    if (!bTitle.trim()) {
      toast("请填写 Banner 标题", "error");
      return;
    }
    setSavingBanner(true);
    try {
      const body = { title: bTitle.trim(), image_url: bImage.trim(), jump_url: bJump.trim(), position: bPos, sort_order: Number(bSort) || 0, is_active: bActive };
      if (bannerForm?.editing) {
        await api.put(`/admin/banners/${bannerForm.editing.id}`, body);
        toast("Banner 已更新", "success");
      } else {
        await api.post("/admin/banners", body);
        toast("Banner 已创建", "success");
      }
      setBannerForm(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingBanner(false);
    }
  };

  const confirmDelBanner = async () => {
    if (!delBanner) return;
    try {
      await api.delete(`/admin/banners/${delBanner.id}`);
      toast("Banner 已删除", "success");
      setDelBanner(null);
      void load();
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  const inputCls = "w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white placeholder-text-muted focus:outline-none focus:border-amber-gold/50 transition-colors";

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Mail className="w-4 h-4 text-amber-gold" />
          <h3 className="text-white font-semibold">通知设置</h3>
          <span className="text-xs text-text-muted">公告与轮播的生效内容（首页公告条/精选推荐轮播实时读取）</span>
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
        <div className="flex items-center justify-between px-4 py-3 border-b border-[#2A2A3E]">
          <div className="flex items-center gap-2">
            <h4 className="text-sm font-medium text-white">平台公告</h4>
            <span className="text-xs text-text-muted">首页公告条读取生效时间窗内的公告（GET /announcements）</span>
          </div>
          <button
            onClick={() => openAnnForm(null)}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-gradient-to-r from-amber-gold to-amber-dim rounded-md text-xs text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Plus className="w-3.5 h-3.5" /> 新增公告
          </button>
        </div>
        <table className="w-full">
          <thead className="bg-[#12122A]/60 border-b border-[#2A2A3E]">
            <tr>
              <th className={TH}>标题</th>
              <th className={TH}>内容</th>
              <th className={TH}>生效开始</th>
              <th className={TH}>生效结束</th>
              <th className={TH}>排序</th>
              <th className={`${TH} text-center`}>操作</th>
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
                <td className={`${TD} text-center`}>
                  <div className="flex items-center justify-center gap-3 text-xs">
                    <a href="#" onClick={(e) => { e.preventDefault(); openAnnForm(a); }} className="action-link">编辑</a>
                    <a href="#" onClick={(e) => { e.preventDefault(); setDelAnn(a); }} className="action-link-danger">删除</a>
                  </div>
                </td>
              </tr>
            ))}
            {anns.length === 0 && !loading && (
              <tr>
                <td colSpan={6} className="px-3 py-8 text-center text-text-muted text-sm">暂无公告（生效时间窗外或未录入）</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 轮播 Banner 列表 */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[#2A2A3E]">
          <div className="flex items-center gap-2">
            <Image className="w-4 h-4 text-amber-gold" />
            <h4 className="text-sm font-medium text-white">轮播 Banner</h4>
            <span className="text-xs text-text-muted">首页精选推荐轮播读取启用中的 Banner（GET /banners）</span>
          </div>
          <button
            onClick={() => openBannerForm(null)}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-gradient-to-r from-amber-gold to-amber-dim rounded-md text-xs text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all"
          >
            <Plus className="w-3.5 h-3.5" /> 新增 Banner
          </button>
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
              <th className={`${TH} text-center`}>操作</th>
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
                <td className={`${TD} text-center`}>
                  <div className="flex items-center justify-center gap-3 text-xs">
                    <a href="#" onClick={(e) => { e.preventDefault(); openBannerForm(b); }} className="action-link">编辑</a>
                    <a href="#" onClick={(e) => { e.preventDefault(); setDelBanner(b); }} className="action-link-danger">删除</a>
                  </div>
                </td>
              </tr>
            ))}
            {banners.length === 0 && !loading && (
              <tr>
                <td colSpan={7} className="px-3 py-8 text-center text-text-muted text-sm">暂无 Banner（未录入或全部停用）</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 公告表单弹窗 */}
      <Modal open={annForm !== null} onClose={() => setAnnForm(null)} title={annForm?.editing ? "编辑公告" : "新增公告"} subtitle="生效时间窗内的公告将展示在用户端首页公告条">
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-text-muted mb-2">标题</label>
            <input type="text" value={aTitle} maxLength={128} placeholder="如 平台升级维护通知" onChange={(e) => setATitle(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">内容</label>
            <textarea value={aContent} rows={3} placeholder="公告正文" onChange={(e) => setAContent(e.target.value)} className={inputCls} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-text-muted mb-2">生效开始</label>
              <input type="datetime-local" value={aStart} onChange={(e) => setAStart(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block text-xs text-text-muted mb-2">生效结束</label>
              <input type="datetime-local" value={aEnd} onChange={(e) => setAEnd(e.target.value)} className={inputCls} />
            </div>
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">排序（小在前）</label>
            <input type="number" value={aSort} onChange={(e) => setASort(e.target.value)} className={inputCls} />
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button onClick={() => setAnnForm(null)} className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors">取消</button>
          <button onClick={() => void submitAnn()} disabled={savingAnn} className="px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50">
            {savingAnn ? "保存中…" : "保存"}
          </button>
        </div>
      </Modal>

      {/* Banner 表单弹窗 */}
      <Modal open={bannerForm !== null} onClose={() => setBannerForm(null)} title={bannerForm?.editing ? "编辑 Banner" : "新增 Banner"} subtitle="启用中的 Banner 将展示在首页精选推荐轮播">
        <div className="space-y-4">
          <div>
            <label className="block text-xs text-text-muted mb-2">标题</label>
            <input type="text" value={bTitle} maxLength={64} placeholder="如 新用户限时福利" onChange={(e) => setBTitle(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">图片 URL（MinIO 预签名/公开地址）</label>
            <input type="text" value={bImage} placeholder="https://..." onChange={(e) => setBImage(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="block text-xs text-text-muted mb-2">跳转地址</label>
            <input type="text" value={bJump} placeholder="/points 或 https://..." onChange={(e) => setBJump(e.target.value)} className={inputCls} />
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs text-text-muted mb-2">位置</label>
              <input type="text" value={bPos} placeholder="home" onChange={(e) => setBPos(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block text-xs text-text-muted mb-2">排序</label>
              <input type="number" value={bSort} onChange={(e) => setBSort(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block text-xs text-text-muted mb-2">状态</label>
              <select value={bActive ? "1" : "0"} onChange={(e) => setBActive(e.target.value === "1")} className={inputCls}>
                <option value="1">启用</option>
                <option value="0">停用</option>
              </select>
            </div>
          </div>
        </div>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button onClick={() => setBannerForm(null)} className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] hover:text-white transition-colors">取消</button>
          <button onClick={() => void submitBanner()} disabled={savingBanner} className="px-4 py-2 bg-gradient-to-r from-amber-gold to-amber-dim rounded-lg text-sm text-[#0B0B1A] font-semibold hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50">
            {savingBanner ? "保存中…" : "保存"}
          </button>
        </div>
      </Modal>

      {/* 删除确认 */}
      <Modal open={delAnn !== null} onClose={() => setDelAnn(null)} title="删除公告" width={400}>
        <p className="text-sm text-white">确定删除公告「{delAnn?.title}」吗？删除后首页不再展示。</p>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button onClick={() => setDelAnn(null)} className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:text-white transition-colors">取消</button>
          <button onClick={() => void confirmDelAnn()} className="px-4 py-2 bg-red-500/90 hover:bg-red-500 rounded-lg text-sm text-white font-semibold transition-colors">确认删除</button>
        </div>
      </Modal>
      <Modal open={delBanner !== null} onClose={() => setDelBanner(null)} title="删除 Banner" width={400}>
        <p className="text-sm text-white">确定删除 Banner「{delBanner?.title}」吗？删除后轮播不再展示。</p>
        <div className="flex items-center justify-end gap-3 mt-6">
          <button onClick={() => setDelBanner(null)} className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:text-white transition-colors">取消</button>
          <button onClick={() => void confirmDelBanner()} className="px-4 py-2 bg-red-500/90 hover:bg-red-500 rounded-lg text-sm text-white font-semibold transition-colors">确认删除</button>
        </div>
      </Modal>
    </div>
  );
}
