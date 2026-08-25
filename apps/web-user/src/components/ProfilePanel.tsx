// ProfilePanel —— 头像资料面板（对齐 LuxReal 官方实测规范：05-前后端对齐/01 清单）
// 形态：头像右侧展开、底边与头像底边对齐；#292D32 底、8px 圆角、无边框无三角、中阴影；
// 内容：用户信息（复制ID + 右上退出）→ 创作空间区（真数据 GET /spaces）→ 菜单项（32px/12px/hover 白6%）。
// 账号设置：昵称内联编辑（点击铅笔按钮后原地输入，Enter/失焦保存、Esc 取消；PUT /auth/me）。
// 触发方式保留点击（比官方 hover 更稳，触屏可用）；面板常驻 DOM，用 .hidden 切换（同官方行为）。
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowLeftRight, Camera, Copy, Droplets, FileText, Flag, Folder, Gem,
  HelpCircle, Languages, Loader2, LogOut, Pencil, Plus,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@duanju/api-client";
import { useAuthStore } from "../stores/authStore";
import { setAvatarUrlCache, useAvatarUrl } from "../hooks/useAvatarUrl";

interface SpaceItem {
  space_id: string;
  name: string;
  type: string;
  my_role: string;
}

export default function ProfilePanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const { user, clearAuth, updateUser } = useAuthStore();
  const [space, setSpace] = useState<SpaceItem | null>(null);
  const [uploading, setUploading] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  // 昵称内联编辑（点击编辑按钮后原地输入，PUT /auth/me）
  const [editingNick, setEditingNick] = useState(false);
  const [nicknameVal, setNicknameVal] = useState("");
  const [savingNickname, setSavingNickname] = useState(false);
  const nickInputRef = useRef<HTMLInputElement>(null);

  const initial = (user?.nickname || "Y").slice(0, 1).toUpperCase();
  const shortId = user?.user_uuid?.slice(0, 8) ?? "—";
  // 头像显示 URL（avatar_url 存 object key → 后端签 24h GET）
  const avatarUrl = useAvatarUrl(user?.avatar_url);

  // 点击面板外关闭（头像自身点击由父级 stopPropagation 后走开关逻辑）
  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener("click", onDocClick);
    return () => document.removeEventListener("click", onDocClick);
  }, [open, onClose]);

  // 面板打开时拉取空间列表（个人空间优先）
  useEffect(() => {
    if (!open) return;
    (async () => {
      try {
        const d = await api.get<{ items: SpaceItem[] }>("/spaces");
        const items = d.items ?? [];
        setSpace(items.find((s) => s.type === "PERSONAL") ?? items[0] ?? null);
      } catch {
        /* 空间加载失败不阻塞面板 */
      }
    })();
  }, [open]);

  // 退出登录：调后端 /auth/logout（拉黑双 jti）后清本地态
  const handleLogout = async () => {
    try {
      await api.post("/auth/logout");
    } catch {
      /* 登出失败也清本地态 */
    }
    clearAuth();
    localStorage.removeItem("access_token");
    sessionStorage.removeItem("access_token");
    navigate("/login");
  };

  const copyId = async () => {
    if (!user?.user_uuid) return;
    try {
      await navigator.clipboard.writeText(user.user_uuid);
      toast.success("用户 ID 已复制");
    } catch {
      toast.error("复制失败，请手动复制");
    }
  };

  const goSpace = () => {
    navigate("/spaces");
  };

  // 开始内联编辑：回填当前昵称并聚焦
  const startEdit = () => {
    setNicknameVal(user?.nickname || "");
    setEditingNick(true);
    setTimeout(() => nickInputRef.current?.focus(), 0);
  };

  // 保存昵称：PUT /auth/me（空字段不更新，后端 UpdateMeReq omitempty）
  const saveNickname = async () => {
    const name = nicknameVal.trim();
    if (!name) {
      toast.error("昵称不能为空");
      return;
    }
    if (name.length < 2 || name.length > 64) {
      toast.error("昵称需为 2-64 个字符");
      return;
    }
    // 未修改则直接退出编辑态，不发请求
    if (name === (user?.nickname || "")) {
      setEditingNick(false);
      return;
    }
    setSavingNickname(true);
    try {
      await api.put("/auth/me", { nickname: name });
      updateUser({ nickname: name });
      setEditingNick(false);
      toast.success("昵称已更新");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "昵称更新失败");
    } finally {
      setSavingNickname(false);
    }
  };

  // 取消编辑：恢复显示态
  const cancelEdit = () => {
    setEditingNick(false);
    setNicknameVal(user?.nickname || "");
  };

  const soon = (feature: string) => () => toast.info(`${feature}将在后续版本开放`);

  // 头像上传：presign(biz_type=avatar) → MinIO 直传 → complete 回执 → PUT /auth/me 落库 object key
  const handleAvatarFile = async (file: File) => {
    if (!file.type.startsWith("image/")) {
      toast.error("请选择图片文件");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      toast.error("头像图片不能超过 5MB");
      return;
    }
    setUploading(true);
    try {
      const presign = await api.post<{
        upload_url: string; upload_token: string; object_key: string; download_url: string;
      }>("/uploads/presign", {
        biz_type: "avatar",
        file_name: file.name,
        content_type: file.type,
        size: file.size,
      });
      const putResp = await fetch(presign.upload_url, {
        method: "PUT", body: file, headers: { "Content-Type": file.type },
      });
      if (!putResp.ok) throw new Error("直传失败 " + putResp.status);
      const etag = (putResp.headers.get("ETag") || "").replace(/"/g, "");
      await api.post("/uploads/complete", {
        upload_token: presign.upload_token,
        object_key: presign.object_key,
        etag: etag || "-",
        biz_ref_id: user?.default_space_id?.toString() || "0",
      });
      await api.put("/auth/me", { avatar_url: presign.object_key });
      // 同步本地态 + 缓存直注入 presign 的 download_url，各侧栏头像立即刷新
      updateUser({ avatar_url: presign.object_key });
      setAvatarUrlCache(presign.object_key, presign.download_url);
      toast.success("头像已更新");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "头像上传失败");
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  return (
    <div ref={panelRef} className={`pp-panel ${open ? "" : "hidden"}`}>
      {/* 用户信息区（头像可点击更换） */}
      <div className="pp-user">
        <div
          className="pp-user-avatar pp-avatar-editable"
          title="更换头像"
          onClick={() => !uploading && fileRef.current?.click()}
        >
          {initial}
          {avatarUrl && (
            <img
              className="pp-avatar-img"
              src={avatarUrl}
              alt=""
              onError={(e) => { e.currentTarget.style.display = "none"; }}
            />
          )}
          <span className="pp-avatar-cam">
            {uploading ? <Loader2 size={12} className="animate-spin" /> : <Camera size={12} />}
          </span>
        </div>
        <input
          ref={fileRef}
          type="file"
          accept="image/*"
          style={{ display: "none" }}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void handleAvatarFile(f);
          }}
        />
        <div className="pp-user-info">
          {editingNick ? (
            <>
              <input
                ref={nickInputRef}
                type="text"
                value={nicknameVal}
                maxLength={64}
                placeholder="2-64 个字符"
                disabled={savingNickname}
                onChange={(e) => setNicknameVal(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void saveNickname();
                  if (e.key === "Escape") cancelEdit();
                }}
                onBlur={() => { if (savingNickname) return; void saveNickname(); }}
                className="pp-user-name-edit"
              />
              <div className="pp-user-edit-tip">
                {savingNickname ? "保存中…" : "Enter 保存 · Esc 取消"}
              </div>
            </>
          ) : (
            <>
              <div className="pp-user-name">{user?.nickname || "未登录用户"}</div>
              <div className="pp-user-id">ID {shortId}</div>
            </>
          )}
        </div>
        <div className="pp-user-actions">
          <button className="pp-icon-btn" title="修改昵称" onClick={startEdit}>
            <Pencil size={14} />
          </button>
          <button className="pp-icon-btn" title="复制用户 ID" onClick={() => void copyId()}>
            <Copy size={14} />
          </button>
          <button className="pp-logout-mini" title="退出登录" onClick={() => void handleLogout()}>
            <LogOut size={12} />
            <span>退出</span>
          </button>
        </div>
      </div>

      <div className="pp-divider" />

      {/* 创作空间区 */}
      <div className="pp-space-head">
        <span className="pp-group-title">创作空间</span>
        <button className="pp-space-new" onClick={goSpace}>
          <Plus size={12} />
          新建空间
        </button>
      </div>
      <div className="pp-space-card" onClick={goSpace} title="管理/切换空间">
        <div className="pp-space-avatar">{(space?.name || "个").slice(0, 1)}</div>
        <span className="pp-space-name">{space?.name || "个人空间"}</span>
        <span className="pp-space-switch">
          <ArrowLeftRight size={12} />
          切换空间
        </span>
      </div>

      <div className="pp-divider" />

      {/* 菜单项 */}
      <div className="pp-menu">
        <a className="pp-menu-item" onClick={() => navigate("/projects")}>
          <Folder size={16} />
          我的作品
        </a>
        <a className="pp-menu-item" onClick={() => navigate("/points")}>
          <Gem size={16} />
          充值中心
        </a>
        <a className="pp-menu-item" onClick={startEdit}>
          <Pencil size={16} />
          修改昵称
        </a>
        <a className="pp-menu-item" onClick={soon("AIGC 水印设置")}>
          <Droplets size={16} />
          AIGC 水印设置
        </a>
        <a className="pp-menu-item" onClick={soon("语言切换")}>
          <Languages size={16} />
          语言切换
        </a>
        <a className="pp-menu-item" onClick={soon("问题反馈")}>
          <Flag size={16} />
          问题反馈
        </a>
        <a className="pp-menu-item" onClick={soon("帮助中心")}>
          <HelpCircle size={16} />
          帮助中心
        </a>
        <a className="pp-menu-item" onClick={soon("平台条款与条件")}>
          <FileText size={16} />
          平台条款与条件
        </a>
      </div>
    </div>
  );
}
