// AdminConfigPage：迁移自 admin-config.html（DOM 1:1）。
// 数据策略：基础设置表单 GET /admin/platform/config 真数据（config_key → 表单字段，失败回退设计稿静态值）；
// 保存逐项 PUT /admin/platform/config（config_value JSON 编码后入库，与后端 value_type='string' 的 ::jsonb 写入对齐）；
// 左侧 8 个垂直 Tab：basic 基础设置（读写）/ membership 会员套餐（套餐编辑+积分包只读）/ models AI 模型配置（复用 AdminModelsTab）/ notify 通知设置（公告+Banner 只读）/ security 安全设置（限流只读）/ appearance 外观定制（主题+风格库只读）/ payment 支付配置（后端无接口，占位）/ legal 协议管理（后端无接口，占位）。
import { useCallback, useEffect, useState } from "react";
import { api } from "@duanju/api-client";
import {
  Crown,
  Cpu,
  Database,
  FileText,
  Globe,
  Mail,
  Palette,
  Save,
  Shield,
  ToggleLeft,
  Wallet,
} from "lucide-react";

import { toast, errMessage } from "../lib";
import AdminAppearancePanel from "./AdminAppearancePanel";
import AdminMembershipPanel from "./AdminMembershipPanel";
import AdminModelsTab from "./AdminModelsTab";
import AdminNotifyPanel from "./AdminNotifyPanel";
import AdminSecurityPanel from "./AdminSecurityPanel";
import "./AdminConfigPage.css";

interface ConfigItem {
  config_key: string;
  config_value: unknown;
  config_group?: string;
  description?: string;
}

interface ConfigResp {
  items: ConfigItem[];
  pagination?: { page: number; page_size: number; total: number };
}

type TabKey = "basic" | "membership" | "models" | "payment" | "notify" | "security" | "legal" | "appearance";

const TABS: { key: TabKey; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { key: "basic", label: "基础设置", icon: Globe },
  { key: "membership", label: "会员套餐", icon: Crown },
  { key: "models", label: "AI 模型配置", icon: Cpu },
  { key: "payment", label: "支付配置", icon: Wallet },
  { key: "notify", label: "通知设置", icon: Mail },
  { key: "security", label: "安全设置", icon: Shield },
  { key: "legal", label: "协议管理", icon: FileText },
  { key: "appearance", label: "外观定制", icon: Palette },
];

// 设计稿静态默认值（无后端配置项时展示；重置默认也回到此值）
const DEFAULTS = {
  platform_name: "Empty Broken",
  platform_subtitle: "AI 驱动的短剧创作平台",
  platform_domain: "emptybroken.com",
  icp_no: "京ICP备2026000000号",
  feature_register: true,
  feature_audit: true,
  feature_comment: true,
  feature_feed: true,
  feature_invite_only: false,
  feature_maintenance: false,
  ai_free_daily_quota: 5,
  ai_max_video_sec: 300,
  ai_concurrent_limit: 2,
  ai_cost_video: 50,
  ai_cost_tts: 10,
  ai_cost_cover: 5,
  storage_provider: "阿里云 OSS",
  storage_region: "oss-cn-beijing",
  storage_bucket: "emptybroken-prod",
  storage_cdn: "cdn.emptybroken.com",
  storage_user_quota_gb: 10,
};

type FormState = typeof DEFAULTS;

// 后端 config_value 为 jsonb：JSON 字符串/数字/布尔需解码回原始值
function decodeValue(v: unknown): unknown {
  if (typeof v !== "string") return v;
  try {
    return JSON.parse(v);
  } catch {
    return v;
  }
}

// 表单字段 → 配置元信息（config_key / group / 描述）
const FIELD_META: { key: keyof FormState; group: string; desc: string }[] = [
  { key: "platform_name", group: "basic", desc: "平台名称" },
  { key: "platform_subtitle", group: "basic", desc: "平台副标题" },
  { key: "platform_domain", group: "basic", desc: "主域名" },
  { key: "icp_no", group: "basic", desc: "备案号" },
  { key: "feature_register", group: "feature", desc: "用户注册开关" },
  { key: "feature_audit", group: "feature", desc: "内容审核开关" },
  { key: "feature_comment", group: "feature", desc: "评论功能开关" },
  { key: "feature_feed", group: "feature", desc: "社区发现页开关" },
  { key: "feature_invite_only", group: "feature", desc: "邀请注册开关" },
  { key: "feature_maintenance", group: "feature", desc: "维护模式开关" },
  { key: "ai_free_daily_quota", group: "ai_limits", desc: "每日免费次数" },
  { key: "ai_max_video_sec", group: "ai_limits", desc: "单视频最大时长(秒)" },
  { key: "ai_concurrent_limit", group: "ai_limits", desc: "并发任务数" },
  { key: "ai_cost_video", group: "ai_limits", desc: "单次消耗积分(视频)" },
  { key: "ai_cost_tts", group: "ai_limits", desc: "单次消耗积分(配音)" },
  { key: "ai_cost_cover", group: "ai_limits", desc: "单次消耗积分(封面)" },
  { key: "storage_provider", group: "storage", desc: "存储服务商" },
  { key: "storage_region", group: "storage", desc: "存储区域" },
  { key: "storage_bucket", group: "storage", desc: "Bucket 名称" },
  { key: "storage_cdn", group: "storage", desc: "CDN 域名" },
  { key: "storage_user_quota_gb", group: "storage", desc: "单用户存储空间(GB)" },
];

// 开关列表（设计稿顺序）
const FEATURE_SWITCHES: { key: keyof FormState; label: string; desc: string }[] = [
  { key: "feature_register", label: "用户注册", desc: "允许新用户注册账号" },
  { key: "feature_audit", label: "内容审核", desc: "作品发布前需经过审核" },
  { key: "feature_comment", label: "评论功能", desc: "允许用户对作品发表评论" },
  { key: "feature_feed", label: "社区发现页", desc: "展示公开作品的发现广场" },
  { key: "feature_invite_only", label: "邀请注册", desc: "仅允许通过邀请码注册" },
  { key: "feature_maintenance", label: "维护模式", desc: "开启后前台仅显示维护公告" },
];

// AI 限制字段（设计稿 2 行 6 项）
const AI_LIMIT_FIELDS: { key: keyof FormState; label: string }[] = [
  { key: "ai_free_daily_quota", label: "每日免费次数" },
  { key: "ai_max_video_sec", label: "单视频最大时长(秒)" },
  { key: "ai_concurrent_limit", label: "并发任务数" },
  { key: "ai_cost_video", label: "单次消耗积分(视频)" },
  { key: "ai_cost_tts", label: "单次消耗积分(配音)" },
  { key: "ai_cost_cover", label: "单次消耗积分(封面)" },
];

// 规划中面板：后端未提供该域配置接口时的占位说明（区别于“有接口未做 UI”）
function PlannedPanel({ label, note }: { label: string; note: string }) {
  return (
    <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-16 flex flex-col items-center justify-center text-center">
      <div className="text-sm text-white font-medium mb-1">{label}</div>
      <p className="text-xs text-text-muted">{note}</p>
    </div>
  );
}

export default function AdminConfigPage() {
  const [tab, setTab] = useState<TabKey>("basic");
  const [form, setForm] = useState<FormState>(DEFAULTS);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);

  // 加载后端配置：config_key → 覆盖默认值
  const load = useCallback(async () => {
    try {
      const resp = await api.get<ConfigResp>("/admin/platform/config");
      const items = resp.items ?? [];
      if (items.length === 0) return;
      setForm((prev) => {
        const next = { ...prev };
        const map = new Map(items.map((i) => [i.config_key, i]));
        for (const meta of FIELD_META) {
          const item = map.get(meta.key);
          if (!item) continue;
          const v = decodeValue(item.config_value);
          if (v !== undefined && v !== null && v !== "") {
            (next as Record<string, unknown>)[meta.key] = v as never;
          }
        }
        return next;
      });
      setLoaded(true);
    } catch {
      /* 回退设计稿静态值 */
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const setField = (key: keyof FormState, value: string | boolean | number) => {
    setForm((prev) => ({ ...prev, [key]: value as never }));
  };

  // 保存：逐项 PUT /admin/platform/config（config_value JSON 编码，对齐后端 ::jsonb 写入）
  const saveAll = async () => {
    setSaving(true);
    try {
      await Promise.all(
        FIELD_META.map((meta) =>
          api.put("/admin/platform/config", {
            config_key: meta.key,
            config_value: JSON.stringify(form[meta.key]),
            config_group: meta.group,
            description: meta.desc,
          }),
        ),
      );
      setLoaded(true);
      toast("配置已保存", "success");
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSaving(false);
    }
  };

  const resetDefault = () => {
    setForm(DEFAULTS);
    toast("已恢复默认值，点击“保存配置”生效", "info");
  };

  const storageProviders = ["阿里云 OSS", "腾讯云 COS", "七牛云 Kodo", "AWS S3"];

  return (
    <>
      {/* Page title */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white mb-1">平台配置</h1>
          <p className="text-sm text-text-muted">管理平台基础设置、会员套餐、AI 模型参数等全局配置</p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={resetDefault}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] transition-colors"
          >
            重置默认
          </button>
          <button
            onClick={() => void saveAll()}
            disabled={saving}
            className="px-4 py-2 bg-amber-gold text-[#0B0B1A] rounded-lg text-sm font-semibold hover:bg-amber-400 transition-colors flex items-center gap-2 disabled:opacity-50"
          >
            <Save className="w-4 h-4" />
            {saving ? "保存中…" : "保存配置"}
          </button>
        </div>
      </div>

      <div className="flex gap-6">
        {/* Left: Vertical tabs */}
        <div className="w-48 flex-shrink-0">
          <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-2 sticky top-20">
            {TABS.map((t) => {
              const Icon = t.icon;
              return (
                <button
                  key={t.key}
                  className={`w-full flex items-center px-3 py-2.5 rounded-lg text-sm transition-colors ${
                    tab === t.key ? "bg-amber-gold/10 text-amber-gold font-medium" : "text-text-secondary hover:bg-white/5"
                  }`}
                  onClick={() => setTab(t.key)}
                >
                  <Icon className="w-4 h-4 mr-3" />
                  {t.label}
                </button>
              );
            })}
          </div>
        </div>

        {/* Right: Config panels */}
        <div className="flex-1 space-y-5">
          {tab === "basic" && (
            <>
              {/* 基础信息 */}
              <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 config-section transition-colors">
                <div className="flex items-center gap-3 mb-5">
                  <div className="w-10 h-10 rounded-lg bg-amber-gold/10 flex items-center justify-center">
                    <Globe className="w-5 h-5 text-amber-gold" />
                  </div>
                  <div>
                    <h3 className="text-white font-semibold">基础信息</h3>
                    <p className="text-text-muted text-xs">平台名称、Logo、域名等基础配置</p>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-5">
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">平台名称</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.platform_name}
                      onChange={(e) => setField("platform_name", e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">平台副标题</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.platform_subtitle}
                      onChange={(e) => setField("platform_subtitle", e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">主域名</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.platform_domain}
                      onChange={(e) => setField("platform_domain", e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">备案号</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.icp_no}
                      onChange={(e) => setField("icp_no", e.target.value)}
                    />
                  </div>
                  <div className="col-span-2">
                    <label className="block text-sm text-text-secondary mb-1.5">平台 Logo</label>
                    <div className="flex items-center gap-4">
                      <div className="w-16 h-16 rounded-xl bg-gradient-to-br from-amber-gold to-amber-dim flex items-center justify-center">
                        <svg className="w-8 h-8 text-[#0B0B1A]" fill="currentColor" viewBox="0 0 24 24">
                          <path d="M8 5v14l11-7z" />
                        </svg>
                      </div>
                      <div>
                        <button
                          className="px-4 py-2 bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg text-sm text-white hover:border-[#3A3A4E] transition-colors"
                          onClick={() => toast("Logo 上传规划中", "info")}
                        >
                          上传 Logo
                        </button>
                        <p className="text-xs text-text-muted mt-1">推荐尺寸 512x512 PNG，透明背景</p>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              {/* 功能开关 */}
              <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 config-section transition-colors">
                <div className="flex items-center gap-3 mb-5">
                  <div className="w-10 h-10 rounded-lg bg-green-500/10 flex items-center justify-center">
                    <ToggleLeft className="w-5 h-5 text-green-400" />
                  </div>
                  <div>
                    <h3 className="text-white font-semibold">功能开关</h3>
                    <p className="text-text-muted text-xs">开启或关闭平台各项功能模块</p>
                  </div>
                </div>
                <div className="space-y-4">
                  {FEATURE_SWITCHES.map((sw, idx) => (
                    <div
                      key={sw.key}
                      className={`flex items-center justify-between py-2 ${idx < FEATURE_SWITCHES.length - 1 ? "border-b border-[#2A2A3E]/50" : ""}`}
                    >
                      <div>
                        <div className="text-sm text-white">{sw.label}</div>
                        <div className="text-xs text-text-muted">{sw.desc}</div>
                      </div>
                      <label className="switch">
                        <input
                          type="checkbox"
                          checked={Boolean(form[sw.key])}
                          onChange={(e) => setField(sw.key, e.target.checked)}
                        />
                        <span className="slider" />
                      </label>
                    </div>
                  ))}
                </div>
              </div>

              {/* AI 生成限制 */}
              <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 config-section transition-colors">
                <div className="flex items-center gap-3 mb-5">
                  <div className="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center">
                    <Cpu className="w-5 h-5 text-purple-400" />
                  </div>
                  <div>
                    <h3 className="text-white font-semibold">AI 生成限制</h3>
                    <p className="text-text-muted text-xs">免费用户每日生成额度与速率限制</p>
                  </div>
                </div>
                <div className="grid grid-cols-3 gap-5">
                  {AI_LIMIT_FIELDS.map((f) => (
                    <div key={f.key}>
                      <label className="block text-sm text-text-secondary mb-1.5">{f.label}</label>
                      <input
                        type="number"
                        className="input-field"
                        value={Number(form[f.key])}
                        onChange={(e) => setField(f.key, Number(e.target.value) || 0)}
                      />
                    </div>
                  ))}
                </div>
              </div>

              {/* 存储配置 */}
              <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 config-section transition-colors">
                <div className="flex items-center gap-3 mb-5">
                  <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                    <Database className="w-5 h-5 text-blue-400" />
                  </div>
                  <div>
                    <h3 className="text-white font-semibold">存储配置</h3>
                    <p className="text-text-muted text-xs">OSS 对象存储与 CDN 设置</p>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-5">
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">存储服务商</label>
                    <select
                      className="input-field"
                      value={form.storage_provider}
                      onChange={(e) => setField("storage_provider", e.target.value)}
                    >
                      {storageProviders.map((p) => (
                        <option key={p}>{p}</option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">存储区域</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.storage_region}
                      onChange={(e) => setField("storage_region", e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">Bucket 名称</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.storage_bucket}
                      onChange={(e) => setField("storage_bucket", e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-secondary mb-1.5">CDN 域名</label>
                    <input
                      type="text"
                      className="input-field"
                      value={form.storage_cdn}
                      onChange={(e) => setField("storage_cdn", e.target.value)}
                    />
                  </div>
                  <div className="col-span-2">
                    <label className="block text-sm text-text-secondary mb-1.5">单用户存储空间(GB)</label>
                    <input
                      type="number"
                      className="input-field"
                      value={Number(form.storage_user_quota_gb)}
                      onChange={(e) => setField("storage_user_quota_gb", Number(e.target.value) || 0)}
                    />
                  </div>
                </div>
              </div>

              {/* 操作按钮 */}
              <div className="flex justify-end gap-3 pt-2">
                <button
                  onClick={resetDefault}
                  className="px-6 py-2.5 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] transition-colors"
                >
                  取消
                </button>
                <button
                  onClick={() => void saveAll()}
                  disabled={saving}
                  className="px-6 py-2.5 bg-amber-gold text-[#0B0B1A] rounded-lg text-sm font-semibold hover:bg-amber-400 transition-colors flex items-center gap-2 disabled:opacity-50"
                >
                  <Save className="w-4 h-4" />
                  {saving ? "保存中…" : "保存所有配置"}
                </button>
              </div>
            </>
          )}
          {tab === "membership" && <AdminMembershipPanel />}
          {tab === "models" && <AdminModelsTab />}
          {tab === "notify" && <AdminNotifyPanel />}
          {tab === "security" && <AdminSecurityPanel />}
          {tab === "appearance" && <AdminAppearancePanel />}
          {tab === "payment" && (
            <PlannedPanel
              label="支付配置"
              note="后端未提供支付配置接口（支付宝正式验签已接入，渠道参数走环境变量；面板规划中）"
            />
          )}
          {tab === "legal" && (
            <PlannedPanel label="协议管理" note="后端未提供协议管理接口（协议内容维护规划中，设计稿亦未提供面板）" />
          )}
          {loaded && tab === "basic" && (
            <p className="text-xs text-text-muted pl-1">已加载后端持久化配置，保存后全局生效</p>
          )}
        </div>
      </div>
    </>
  );
}
