// WorkbenchShell —— 工作台基准布局（批次 2：workbench-script.html 共用骨架提取）
// 结构：AppShell(72px) + ep-sidebar(240px) + 顶栏(WorkflowPill + 自由画布) + 内容区 + 悬浮按钮
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import {
  ChevronDown, ChevronLeft, ChevronRight, Layout, MessageCircle, Plus, Users,
} from "lucide-react";

import type { BeatSummary, EpisodeLite } from "@duanju/api-client";
import AppShell from "./AppShell";
import "./shell.css";

const WORKFLOW_STEPS = [
  { n: 1, label: "剧本拆分" },
  { n: 2, label: "资产生成" },
  { n: 3, label: "视频制作" },
  { n: 4, label: "成片预览" },
];

export default function WorkbenchShell({
  projectTitle,
  currentStep,
  canClick,
  goStep,
  episodes,
  activeEpisodeId,
  onSelectEpisode,
  activeEpisodeBeats = [],
  children,
}: {
  projectTitle: string;
  currentStep: number;
  canClick: (s: number) => boolean;
  goStep: (s: number) => void;
  episodes: EpisodeLite[];
  activeEpisodeId: string | null;
  onSelectEpisode: (id: string) => void;
  activeEpisodeBeats?: BeatSummary[];
  children: React.ReactNode;
}) {
  const navigate = useNavigate();
  const [activeUnitId, setActiveUnitId] = useState<string | null>(null);

  return (
    <AppShell active="create">
      <div className="flex flex-1 min-h-0 min-w-0">
        {/* Column 2: Episode Sidebar (240px) */}
        <aside className="ep-sidebar flex flex-col flex-shrink-0 scrollbar-thin">
          {/* Top Header */}
          <div className="ep-sidebar-header">
            <a className="ep-sidebar-back" onClick={() => navigate("/projects")}>
              <ChevronLeft size={18} />
            </a>
            <span className="ep-sidebar-project">{projectTitle || "加载中…"}</span>
          </div>

          {/* Episode List (scrollable) */}
          <div className="ep-sidebar-list scrollbar-thin">
            {episodes.map((ep) => {
              const expanded = ep.episode_id === activeEpisodeId;
              return (
                <div
                  key={ep.episode_id}
                  className={`ep-card ${expanded ? "expanded" : "collapsed"}`}
                  onClick={() => onSelectEpisode(ep.episode_id)}
                >
                  {/* Episode Header */}
                  <div className="ep-header">
                    <div className="ep-header-left">
                      {expanded ? (
                        <ChevronDown size={14} className="ep-chevron" />
                      ) : (
                        <ChevronRight size={14} className="ep-chevron" />
                      )}
                      <span className="ep-number">第{ep.sort_order}集</span>
                    </div>
                    {expanded && <div className="ep-status-dot" />}
                  </div>
                  {/* Episode Title */}
                  <div className="ep-title">{ep.title || `第 ${ep.sort_order} 集`}</div>
                  {/* Narrative Units（仅展开卡展示当前集单元） */}
                  {expanded && activeEpisodeBeats.length > 0 && (
                    <div className="ep-units">
                      {activeEpisodeBeats.map((b) => (
                        <button
                          key={b.beat_id}
                          className={`ep-unit ${activeUnitId === b.beat_id ? "active" : ""}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setActiveUnitId(b.beat_id);
                            // 滚动到主内容区对应单元卡片
                            document.getElementById(`unit-card-${b.beat_id}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
                          }}
                        >
                          叙事单元{b.sort_order} | {b.title}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
            {episodes.length === 0 && (
              <div className="text-xs px-2 py-4" style={{ color: "#4A4A5A" }}>
                暂无剧集，拆分剧本后自动生成
              </div>
            )}
          </div>

          {/* Bottom: Add Episode Button */}
          <button className="ep-add-btn" onClick={() => toast.info("手动新增剧集将在后续版本开放")}>
            <Plus size={14} />
            新增剧集
          </button>
        </aside>

        {/* Main Content Area */}
        <main className="flex-1 flex flex-col overflow-hidden" style={{ background: "#030911" }}>
          {/* Top Bar Area */}
          <div className="topbar-container flex items-center justify-between px-6 flex-shrink-0">
            {/* Left: Empty spacer (~200px) */}
            <div className="w-[200px] flex-shrink-0" />

            {/* Center: Workflow Steps Navigation */}
            <div className="workflow-steps">
              <div className="workflow-nav">
                {WORKFLOW_STEPS.map((s) => (
                  <a
                    key={s.n}
                    className={`workflow-nav-item ${currentStep === s.n ? "active" : ""} ${canClick(s.n) ? "" : "disabled"}`}
                    onClick={() => canClick(s.n) && goStep(s.n)}
                  >
                    <span className="workflow-nav-num">{s.n}</span>
                    <span className="workflow-nav-text">{s.label}</span>
                  </a>
                ))}
              </div>
            </div>

            {/* Right: Canvas mode button only */}
            <div className="topbar-right">
              <button className="topbar-btn topbar-btn-primary" onClick={() => navigate("/canvas")}>
                <Layout size={14} />
                自由画布
                <span
                  className="text-[10px] text-white px-1.5 py-px rounded"
                  style={{ background: "linear-gradient(135deg, #4C7FF8, #8B5CF6)" }}
                >
                  Beta
                </span>
              </button>
            </div>
          </div>

          {/* 页面内容区（由各 Step 页面填充） */}
          {children}

          {/* Bottom Right Floating Buttons */}
          <div className="fixed bottom-6 right-6 flex flex-col gap-2 z-20">
            <button
              className="w-10 h-10 rounded-full flex items-center justify-center transition-all hover:text-white hover:border-[#4C7FF8]/50"
              style={{ background: "#1A1A2E", border: "1px solid #2A2A3E", color: "#8B8B9A" }}
              onClick={() => toast.info("协作功能即将上线")}
            >
              <Users size={20} />
            </button>
            <button
              className="w-10 h-10 rounded-full flex items-center justify-center transition-all hover:text-white hover:border-[#4C7FF8]/50"
              style={{ background: "#1A1A2E", border: "1px solid #2A2A3E", color: "#8B8B9A" }}
              onClick={() => toast.info("AI 助手对话将在后续版本开放")}
            >
              <MessageCircle size={20} />
            </button>
          </div>
        </main>
      </div>
    </AppShell>
  );
}
