// 作品列表页（批次4：设计稿 works.html 迁移，DOM 1:1）
// CSS 原样提取 → src/pages/works.css（theme tokens + shell 类 + 页面类）
// 接口对接：GET /projects（分页）、PUT /projects/:id（重命名）、DELETE /projects/:id（软删）、
//           POST /projects/:id/last-opened（touch）、GET /points（侧栏积分）
// 左侧 72px 侧边栏照搬设计稿（与 HomePage 同模式，批次后续抽共享布局）
import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  ChevronLeft, ChevronRight, Compass, Copy, Download, Edit3, Folder, FolderOpen,
  Loader2, MoreHorizontal, Play, Plus, RefreshCw,
  Sparkles, Trash2, X,
} from "lucide-react";

import {
  api, ApiError, errorMessage,
  projectsApi, ProjectStatus,
  type Paginated, type ProjectSummary,
} from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import ProfilePanel from "../components/ProfilePanel";
import { useAvatarUrl } from "../hooks/useAvatarUrl";
import "./works.css";

const PAGE_SIZE = 20;

// 已完成态（视频成片 Tab 过滤 + 绿色徽章）
const DONE_STATUSES = new Set<string>([
  ProjectStatus.ALL_VIDEOS_READY, ProjectStatus.EXPORTING, ProjectStatus.COMPLETED,
]);
// 生成中态（蓝色徽章 + 进度条）；进度为前端估值（后端暂无进度字段）
const GEN_PROGRESS: Record<string, number> = {
  [ProjectStatus.SCRIPT_SPLITTING]: 15,
  [ProjectStatus.VIDEO_GENERATING]: 60,
  [ProjectStatus.PARTIAL_VIDEOS]: 80,
};

type BadgeKind = "completed" | "generating" | "failed" | "draft";

function badgeOf(status: string): { kind: BadgeKind; label: string } {
  if (DONE_STATUSES.has(status)) return { kind: "completed", label: "已完成" };
  if (status in GEN_PROGRESS) return { kind: "generating", label: "生成中" };
  if (status === ProjectStatus.SCRIPT_SPLIT_FAILED) return { kind: "failed", label: "失败" };
  return { kind: "draft", label: "草稿" };
}

// 日期 → YYYY-MM-DD（对齐设计稿卡片日期格式）
function fmtDate(iso: string): string {
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// 时长 → mm:ss（对齐设计稿 duration-badge）
function fmtDuration(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

export default function ProjectListPage() {
  const navigate = useNavigate();
  const { user } = useAuthStore();
  const avatarUrl = useAvatarUrl(user?.avatar_url);

  const [tab, setTab] = useState<"project" | "canvas" | "video">("project");
  const [items, setItems] = useState<ProjectSummary[]>([]);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [loading, setLoading] = useState(true);
  const [points, setPoints] = useState<number | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);

  // 卡片 … 下拉菜单 / 重命名 / 删除
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [renameTarget, setRenameTarget] = useState<ProjectSummary | null>(null);
  const [renameVal, setRenameVal] = useState("");
  const [renaming, setRenaming] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ProjectSummary | null>(null);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async (p: number) => {
    try {
      setLoading(true);
      const data: Paginated<ProjectSummary> = await projectsApi.list(p, PAGE_SIZE);
      setItems(data.items);
      setTotalPages(Math.max(1, data.pagination.total_pages));
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(page);
  }, [load, page]);

  // 侧栏积分余额（与 HomePage 同源）
  useEffect(() => {
    (async () => {
      try {
        const acc = await api.get<{ current_balance: number }>("/points");
        setPoints(acc.current_balance);
      } catch {
        /* 积分失败不阻塞页面 */
      }
    })();
  }, []);

  // 点击菜单外区域关闭下拉菜单
  useEffect(() => {
    if (!menuFor) return;
    const onDocDown = (e: MouseEvent) => {
      const el = e.target as HTMLElement;
      if (!el.closest(".card-dropdown-menu") && !el.closest(".card-menu-btn")) {
        setMenuFor(null);
      }
    };
    document.addEventListener("mousedown", onDocDown);
    return () => document.removeEventListener("mousedown", onDocDown);
  }, [menuFor]);

  const openProject = (p: ProjectSummary) => {
    void projectsApi.touch(p.project_id).catch(() => undefined);
    navigate(`/projects/${p.project_id}/step/1`);
  };

  // 重命名：PUT /projects/:id
  const submitRename = async () => {
    if (!renameTarget || !renameVal.trim()) return;
    try {
      setRenaming(true);
      await projectsApi.update(renameTarget.project_id, { title: renameVal.trim() });
      toast.success("已重命名");
      setRenameTarget(null);
      void load(page);
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "重命名失败");
    } finally {
      setRenaming(false);
    }
  };

  // 删除：DELETE /projects/:id（软删）
  const submitDelete = async () => {
    if (!deleteTarget) return;
    try {
      setDeleting(true);
      await projectsApi.remove(deleteTarget.project_id);
      toast.success("项目已删除");
      setDeleteTarget(null);
      void load(page);
    } catch (err) {
      toast.error(err instanceof ApiError ? errorMessage(err.code, err.message) : "删除失败");
    } finally {
      setDeleting(false);
    }
  };

  // 视频成片 Tab：客户端按已完成状态过滤当前页数据
  const visibleItems = tab === "video" ? items.filter((p) => DONE_STATUSES.has(p.status)) : items;

  // 分页页码序列（对齐设计稿：1 2 3 ... 末页）
  const pageSeq: (number | "…")[] = (() => {
    if (totalPages <= 7) return Array.from({ length: totalPages }, (_, i) => i + 1);
    if (page <= 4) return [1, 2, 3, 4, 5, "…", totalPages];
    if (page >= totalPages - 3) return [1, "…", totalPages - 4, totalPages - 3, totalPages - 2, totalPages - 1, totalPages];
    return [1, "…", page - 1, page, page + 1, "…", totalPages];
  })();

  const renderCard = (p: ProjectSummary, idx: number) => {
    const badge = badgeOf(p.status);
    const progress = GEN_PROGRESS[p.status];
    return (
      <div key={p.project_id} className="project-card" onClick={() => openProject(p)}>
        <div className="project-thumb">
          <div className={`project-thumb-gradient thumb-${(idx % 16) + 1}`} />
          <div className="card-highlight" />
          <span className={`status-badge status-${badge.kind}`}>{badge.label}</span>
          {badge.kind === "completed" && p.total_duration_sec > 0 && (
            <span className="duration-badge">{fmtDuration(p.total_duration_sec)}</span>
          )}
          <button
            className="card-menu-btn"
            onClick={(e) => {
              e.stopPropagation();
              setMenuFor(menuFor === p.project_id ? null : p.project_id);
            }}
            aria-label="更多操作"
          >
            <MoreHorizontal size={16} />
          </button>
          <div className="play-overlay">
            {badge.kind === "generating" ? (
              <div className="play-btn" style={{ opacity: 0.5 }}>
                <Loader2 size={20} className="animate-spin text-white" />
              </div>
            ) : badge.kind === "failed" ? (
              <div className="play-btn" style={{ borderColor: "rgba(239,68,68,0.4)", background: "rgba(239,68,68,0.15)" }}>
                <RefreshCw size={20} className="text-white" />
              </div>
            ) : (
              <div className="play-btn">
                <Play size={20} className="ml-0.5 text-white" />
              </div>
            )}
          </div>
          {/* 卡片 … 下拉菜单（works.css 锚定卡片内右上角） */}
          {menuFor === p.project_id && (
            <div className="card-dropdown-menu" onClick={(e) => e.stopPropagation()}>
              <button className="card-menu-item" onClick={() => openProject(p)}>
                <FolderOpen size={14} />
                <span>打开项目</span>
              </button>
              <button
                className="card-menu-item"
                onClick={() => {
                  setRenameTarget(p);
                  setRenameVal(p.title);
                  setMenuFor(null);
                }}
              >
                <Edit3 size={14} />
                <span>重命名</span>
              </button>
              <button className="card-menu-item" onClick={() => { setMenuFor(null); toast.info("复制功能即将上线"); }}>
                <Copy size={14} />
                <span>复制</span>
              </button>
              <button className="card-menu-item" onClick={() => { setMenuFor(null); toast.info("导出功能即将上线"); }}>
                <Download size={14} />
                <span>导出</span>
              </button>
              <button
                className="card-menu-item danger"
                onClick={() => {
                  setDeleteTarget(p);
                  setMenuFor(null);
                }}
              >
                <Trash2 size={14} />
                <span>删除</span>
              </button>
            </div>
          )}
        </div>
        <div className="project-info">
          <div className="project-title">{p.title}</div>
          <div className="project-meta">
            <span className="project-type-tag">{tab === "video" ? "视频成片" : "项目"}</span>
            <span>{fmtDate(p.updated_at)}</span>
          </div>
          {badge.kind === "generating" && (
            <div className="progress-bar">
              <div className="progress-fill" style={{ width: `${progress}%` }} />
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <div data-viewport-mode="app-shell" className="flex flex-col h-screen w-screen overflow-hidden">
      {/* 聚光背景 */}
      <div className="spotlight-overlay" />

      {/* 主行：侧边栏 + 内容 */}
      <div className="flex flex-1 min-h-0 min-w-0">
        {/* ===== 左侧 72px 侧边栏（照搬设计稿，作品 active） ===== */}
        <aside
          id="sidebar-nav"
          className="flex flex-col items-center py-3 gap-1 flex-shrink-0 z-10"
          style={{ width: 72, background: "var(--luxreal-sidebar)", borderRight: "1px solid var(--luxreal-border)" }}
        >
          {/* Logo */}
          <div className="flex items-center justify-center flex-shrink-0" style={{ height: 56, marginBottom: 4 }}>
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M6 4H22V8H10V24H6V4Z" fill="white" />
              <path d="M14 12H22V16H18V24H14V12Z" fill="white" opacity="0.7" />
            </svg>
          </div>

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 中心导航（作品 active） */}
          <nav className="flex flex-col items-center gap-2 flex-shrink-0">
            <Link to="/" className="luxreal-nav-item" title="创作">
              <Sparkles size={22} />
              <span>创作</span>
            </Link>
            <Link to="/feed" className="luxreal-nav-item" title="发现">
              <Compass size={22} />
              <span>发现</span>
            </Link>
            <Link to="/projects" className="luxreal-nav-item active" title="作品">
              <Folder size={22} />
              <span>作品</span>
            </Link>
          </nav>

          <div className="flex-1" style={{ minHeight: 20 }} />

          {/* 头像（点击开关头像资料面板，LuxReal 对齐版 pp-panel） */}
          <div className="pp-anchor" style={{ marginBottom: 4 }}>
            <div
              className="sidebar-avatar"
              onClick={(e) => {
                e.stopPropagation();
                setProfileOpen((v) => !v);
              }}
            >
              {(user?.nickname || "Y").slice(0, 1).toUpperCase()}
              {avatarUrl && (
                <img
                  className="sidebar-avatar-img"
                  src={avatarUrl}
                  alt=""
                  onError={(e) => { e.currentTarget.style.display = "none"; }}
                />
              )}
            </div>
            <ProfilePanel open={profileOpen} onClose={() => setProfileOpen(false)} />
          </div>

          {/* 积分（真实余额） */}
          <span className="sidebar-credits">{points === null ? "…" : points.toLocaleString()}</span>
          {/* 充值 */}
          <Link to="/points" className="sidebar-recharge" title="充值">
            充值
          </Link>

          {/* 头像资料面板已内置于 ProfilePanel（上方 pp-anchor 内） */}
        </aside>

        {/* ===== 主内容区 ===== */}
        <div className="flex-1 flex flex-col min-w-0 min-h-0">
          {/* 顶栏（h-56px）：标题 + 新建项目 */}
          <header className="flex items-center justify-between px-6 flex-shrink-0 z-10" style={{ height: 56, background: "transparent" }}>
            <h1 className="text-lg font-semibold text-white">我的作品</h1>
            <div className="flex items-center">
              <button className="btn-new-project" onClick={() => navigate("/")}>
                <Plus size={16} />
                <span>新建项目</span>
              </button>
            </div>
          </header>

          {/* 滚动内容区 */}
          <main data-scroll-region="primary" className="overflow-y-auto flex-1 px-6">
            {/* 类型 Tabs：项目 / 画布 / 视频成片 */}
            <div>
              <nav className="flex items-center border-b border-[var(--luxreal-border)]">
                <button className={`type-tab ${tab === "project" ? "active" : ""}`} onClick={() => setTab("project")}>项目</button>
                <button className={`type-tab ${tab === "canvas" ? "active" : ""}`} onClick={() => setTab("canvas")}>画布</button>
                <button className={`type-tab ${tab === "video" ? "active" : ""}`} onClick={() => setTab("video")}>视频成片</button>
              </nav>
            </div>

            {tab === "canvas" ? (
              /* 画布：后端暂无画布数据，空态占位 */
              <div className="flex flex-col items-center justify-center gap-2 py-24">
                <p className="text-sm text-[var(--luxreal-muted-foreground)]">画布功能即将上线</p>
              </div>
            ) : loading ? (
              <div className="flex items-center justify-center py-24 text-sm text-[var(--luxreal-muted-foreground)]">
                加载中…
              </div>
            ) : visibleItems.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-3 py-24">
                <p className="text-sm text-[var(--luxreal-muted-foreground)]">
                  {tab === "video" ? "还没有已完成的视频成片" : "还没有作品，创建你的第一部短剧吧"}
                </p>
                {tab === "project" && (
                  <button className="btn-new-project" onClick={() => navigate("/")}>
                    <Plus size={16} />
                    <span>开始创作</span>
                  </button>
                )}
              </div>
            ) : (
              <>
                {/* 项目网格（4 列） */}
                <div className="grid grid-cols-4 gap-5 mt-6">
                  {visibleItems.map((p, idx) => renderCard(p, idx))}
                </div>

                {/* 分页（仅项目 Tab，服务端分页） */}
                {tab === "project" && totalPages > 1 && (
                  <div className="flex items-center justify-center gap-1 mt-8 pb-8">
                    <button className="page-btn" title="上一页" disabled={page <= 1} onClick={() => setPage((v) => Math.max(1, v - 1))}>
                      <ChevronLeft size={16} />
                    </button>
                    {pageSeq.map((n, i) =>
                      n === "…" ? (
                        <span key={`e${i}`} className="page-ellipsis">...</span>
                      ) : (
                        <button key={n} className={`page-btn ${n === page ? "active" : ""}`} onClick={() => setPage(n)}>
                          {n}
                        </button>
                      ),
                    )}
                    <button className="page-btn" title="下一页" disabled={page >= totalPages} onClick={() => setPage((v) => Math.min(totalPages, v + 1))}>
                      <ChevronRight size={16} />
                    </button>
                  </div>
                )}
              </>
            )}
          </main>
        </div>
      </div>

      {/* ===== 重命名弹窗（PUT /projects/:id） ===== */}
      <div
        className={`luxreal-modal-backdrop ${renameTarget ? "visible" : ""}`}
        onClick={(e) => {
          if (e.target === e.currentTarget) setRenameTarget(null);
        }}
      >
        <div className="luxreal-modal" style={{ width: 420 }}>
          <div className="luxreal-modal-header">
            <div className="luxreal-modal-title">重命名项目</div>
            <button className="luxreal-modal-close" onClick={() => setRenameTarget(null)} aria-label="关闭">
              <X size={16} />
            </button>
          </div>
          <div className="luxreal-modal-body">
            <div className="luxreal-form-group" style={{ marginBottom: 0 }}>
              <label className="luxreal-form-label">项目名称</label>
              <input
                className="luxreal-form-input"
                value={renameVal}
                onChange={(e) => setRenameVal(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void submitRename();
                }}
                placeholder="请输入项目名称"
                autoFocus
              />
            </div>
          </div>
          <div className="luxreal-modal-footer">
            <button className="btn-ghost" onClick={() => setRenameTarget(null)}>取消</button>
            <button
              className="btn-gradient-primary"
              disabled={renaming || !renameVal.trim()}
              onClick={() => void submitRename()}
            >
              <span>{renaming ? "保存中…" : "保存"}</span>
            </button>
          </div>
        </div>
      </div>

      {/* ===== 删除确认弹窗（DELETE /projects/:id 软删） ===== */}
      <div
        className={`luxreal-modal-backdrop ${deleteTarget ? "visible" : ""}`}
        onClick={(e) => {
          if (e.target === e.currentTarget) setDeleteTarget(null);
        }}
      >
        <div className="luxreal-modal" style={{ width: 420 }}>
          <div className="luxreal-modal-header">
            <div className="luxreal-modal-title">删除项目</div>
            <button className="luxreal-modal-close" onClick={() => setDeleteTarget(null)} aria-label="关闭">
              <X size={16} />
            </button>
          </div>
          <div className="luxreal-modal-body">
            <p className="text-[13px] text-[var(--luxreal-foreground)]">
              确定要删除项目「{deleteTarget?.title}」吗？删除后可在回收站恢复。
            </p>
          </div>
          <div className="luxreal-modal-footer">
            <button className="btn-ghost" onClick={() => setDeleteTarget(null)}>取消</button>
            <button
              className="btn-gradient-primary"
              style={{ background: "linear-gradient(135deg, #EF4444, #B91C1C)" }}
              disabled={deleting}
              onClick={() => void submitDelete()}
            >
              <span>{deleting ? "删除中…" : "确认删除"}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
