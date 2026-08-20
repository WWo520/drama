// Agent 技能配置弹窗（对齐设计稿 create-video.html #modal-agent-config）
// 左侧导航 + 技能列表/详情双视图；Home/Step1 共用（CSS 位于 home.css，Step1 已引）
import { useState } from "react";
import { toast } from "sonner";
import { ArrowLeft, ChevronDown, Clapperboard, ListVideo, Settings, Sparkles, X } from "lucide-react";
import "./AgentSkillModal.css";

// Agent 技能数据（对齐设计稿 skillData：模板 + 开关）
const AGENT_SKILLS: Record<"basic" | "storyboard" | "planning" | "polish", {
  title: string; subtitle: string; navDesc: string;
  Icon: typeof Settings;
  templates: { value: string; prompt: string }[];
  toggles: { label: string; on: boolean }[];
}> = {
  basic: {
    title: "基础参数配置", subtitle: "全局技能参数设置，影响所有技能的基础行为", navDesc: "全局技能参数设置", Icon: Settings,
    templates: [{ value: "默认配置", prompt: "全局技能基础配置参数，包括默认时长、输出比例、生成质量等基础设置。所有技能将以此为基础运行。" }],
    toggles: [{ label: "自动保存草稿", on: true }, { label: "启用智能推荐", on: true }, { label: "显示技能执行日志", on: false }],
  },
  storyboard: {
    title: "镜头提示词生成技能", subtitle: "分镜编剧角色 · 将剧本按照时长拆分为多个镜头", navDesc: "分镜编剧角色", Icon: Clapperboard,
    templates: [
      { value: "通用叙事拆解", prompt: "将剧本按照指定时长自动拆解为结构化镜头序列，为每个镜头标注景别、运镜、主体动作、台词等信息。确保叙事节奏紧凑，镜头语言丰富，符合短剧观看习惯。每个镜头时长控制在3-8秒，重点突出情绪转折点和关键剧情节点。" },
      { value: "深度情绪刻画", prompt: "以人物情绪变化为主线拆解镜头，重点刻画角色内心世界和情感张力。每个镜头都围绕情绪递进设计，通过景别变化、光影对比、镜头语言强化情绪表达。适合情感戏、内心戏丰富的剧本。" },
      { value: "详细动态描述", prompt: "侧重镜头内的动态细节描述，对人物动作、场景变化、物体运动进行精细化拆解。每个镜头包含详细的动作设计、运动轨迹、节奏变化，确保生成视频具有丰富的动态效果。" },
      { value: "按剧情连贯拆分", prompt: "按照剧情发展的自然段落进行镜头拆分，保持每场戏的完整性和连贯性。优先考虑叙事逻辑和场景转换的流畅性，镜头数量适中，单镜头时长略长，适合剧情片风格。" },
    ],
    toggles: [
      { label: "自动估算时长", on: true }, { label: "标注景别信息", on: true }, { label: "标注运镜方式", on: true },
      { label: "丰富画面氛围描述", on: true }, { label: "添加台词提示", on: true },
    ],
  },
  planning: {
    title: "视频任务规划技能", subtitle: "制片调度角色 · 智能规划视频生成任务序列", navDesc: "制片调度角色", Icon: ListVideo,
    templates: [
      { value: "标准调度策略", prompt: "根据分镜脚本智能规划视频生成任务，按照叙事单元逻辑排序，合理分配参考图使用策略，确保相邻镜头画风统一，关键镜头保持角色一致性。自动识别需要优先生成的关键帧，优化任务并行度，平衡生成质量与效率。" },
      { value: "连贯性优先", prompt: "以镜头间的视觉连贯性为首要目标规划任务序列。相邻镜头共享参考图，确保角色形象、场景风格高度一致。适当降低并行度以换取连贯性，适合对画风一致性要求高的项目。" },
      { value: "效率优先", prompt: "以最快生成速度为目标规划任务，最大化并行生成数量，减少依赖链长度。适合时间紧迫、对连贯性要求适中的项目，在保证基本质量的前提下优先完成速度。" },
    ],
    toggles: [{ label: "自动分配参考图", on: true }, { label: "关键帧优先生成", on: true }, { label: "批量任务调度", on: true }],
  },
  polish: {
    title: "单视频提示词润色技能", subtitle: "提示词优化 · 优化提示词使其更适合视频模型", navDesc: "提示词优化", Icon: Sparkles,
    templates: [
      { value: "标准润色", prompt: "将镜头描述优化为适合视频生成模型的专业提示词，补充镜头运动方式、动作节奏、光影氛围、画面细节等，减少模糊表达，提升生成质量。确保提示词结构清晰，关键词权重合理，符合视频模型的理解逻辑。" },
      { value: "高能戏剧化润色", prompt: "采用高强度戏剧化风格润色提示词，强化画面冲击力和情绪张力。增加动态模糊、强烈光影对比、戏剧性构图等高能量元素，适合动作戏、高潮戏份、悬念场景。" },
      { value: "慢节奏细腻润色", prompt: "以细腻、舒缓的风格润色提示词，注重画面质感和氛围营造。增加细腻的光影层次、丰富的环境细节、柔和的运动节奏，适合文艺片、情感戏、慢镜头场景。" },
    ],
    toggles: [
      { label: "增强光影描述", on: true }, { label: "补充画面细节", on: true },
      { label: "优化镜头运动描述", on: true }, { label: "强化情绪氛围", on: false },
    ],
  },
};
const SKILL_KEYS = ["basic", "storyboard", "planning", "polish"] as const;

export default function AgentSkillModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [agentNav, setAgentNav] = useState<"basic" | "storyboard" | "planning" | "polish">("basic");
  const [view, setView] = useState<"list" | "detail">("list");
  const [detail, setDetail] = useState<string | null>(null);
  const [skillTemplate, setSkillTemplate] = useState<Record<string, string>>({ basic: "默认配置", storyboard: "通用叙事拆解", planning: "标准调度策略", polish: "标准润色" });
  const [tplMenuOpen, setTplMenuOpen] = useState(false);
  const [skillToggles, setSkillToggles] = useState<Record<string, boolean[]>>(() =>
    Object.fromEntries(SKILL_KEYS.map((k) => [k, AGENT_SKILLS[k].toggles.map((t) => t.on)]))
  );

  return (
    <div
      id="modal-agent-config"
      className={`luxreal-modal-backdrop ${open ? "visible" : ""}`}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="luxreal-modal agent-skill-modal">
        <div className="luxreal-modal-header">
          <div className="luxreal-modal-title">Agent 技能配置</div>
          <button className="luxreal-modal-close" onClick={onClose} aria-label="关闭">
            <X size={16} />
          </button>
        </div>

        <div className="agent-modal-body">
          {/* 左侧导航 */}
          <div className="agent-modal-sidebar">
            {SKILL_KEYS.map((k) => {
              const sk = AGENT_SKILLS[k];
              const NavIcon = sk.Icon;
              return (
                <button
                  key={k}
                  className={`agent-nav-item ${agentNav === k ? "active" : ""}`}
                  onClick={() => {
                    setAgentNav(k);
                    setView("list");
                    // 定位并高亮对应技能项（对齐设计稿交互）
                    requestAnimationFrame(() => {
                      const li = document.querySelector(`[data-skill-list-item="${k}"]`);
                      if (!li) return;
                      li.classList.add("highlight");
                      li.scrollIntoView({ behavior: "smooth", block: "center" });
                      setTimeout(() => li.classList.remove("highlight"), 1500);
                    });
                  }}
                >
                  <div className="agent-nav-icon">
                    <NavIcon size={16} />
                  </div>
                  <div className="agent-nav-text">
                    <div className="agent-nav-title">{sk.title}</div>
                    <div className="agent-nav-desc">{sk.navDesc}</div>
                  </div>
                </button>
              );
            })}
          </div>

          {/* 右侧内容区 */}
          <div className="agent-modal-content">
            {/* 列表视图 */}
            <div className="agent-skill-view" style={{ display: view === "list" ? undefined : "none" }}>
              <div className="agent-skill-list-header">
                <div className="text-[18px] font-medium text-white">技能列表</div>
                <div className="text-[12px] text-[var(--luxreal-muted-foreground)] mt-1">点击右侧"修改"可自定义每个技能的参数</div>
              </div>
              <div className="agent-skill-list">
                {SKILL_KEYS.map((k) => {
                  const sk = AGENT_SKILLS[k];
                  const SkillIcon = sk.Icon;
                  return (
                    <div key={k} className="agent-skill-list-item" data-skill-list-item={k}>
                      <div className="flex items-start gap-3">
                        <div className="skill-icon-wrap">
                          <SkillIcon size={20} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="skill-title">{sk.title}</div>
                          <div className="skill-subtitle">{sk.subtitle}</div>
                        </div>
                      </div>
                      <div className="flex items-center gap-3 flex-shrink-0">
                        <span className="skill-status-label">{skillTemplate[k]}</span>
                        <button
                          className="skill-modify-link"
                          onClick={() => {
                            setDetail(k);
                            setView("detail");
                            setTplMenuOpen(false);
                          }}
                        >
                          修改
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* 详情视图 */}
            {view === "detail" && detail && (() => {
              const dk = detail as keyof typeof AGENT_SKILLS;
              const sk = AGENT_SKILLS[dk];
              const curTpl = skillTemplate[dk] ?? sk.templates[0].value;
              const curPrompt = sk.templates.find((t) => t.value === curTpl)?.prompt ?? "";
              return (
                <div className="agent-skill-view">
                  <button className="agent-detail-back" onClick={() => setView("list")}>
                    <ArrowLeft size={14} />
                    <span>返回列表</span>
                  </button>
                  <div className="agent-detail-title">{sk.title}</div>

                  {/* 模板选择器 */}
                  <div className="agent-detail-section">
                    <div className="flex items-center gap-2">
                      <span className="text-[13px] text-[var(--luxreal-muted-foreground)]">当前模板:</span>
                      <div className="skill-template-dropdown">
                        <button className="skill-template-selector" onClick={() => setTplMenuOpen((v) => !v)}>
                          <span className="skill-template-value">{curTpl}</span>
                          <ChevronDown size={12} />
                        </button>
                        <div className={`skill-template-menu ${tplMenuOpen ? "open" : ""}`}>
                          {sk.templates.map((t) => (
                            <button
                              key={t.value}
                              className={`skill-template-option ${t.value === curTpl ? "active" : ""}`}
                              onClick={() => {
                                setSkillTemplate((prev) => ({ ...prev, [dk]: t.value }));
                                setTplMenuOpen(false);
                              }}
                            >
                              {t.value}
                            </button>
                          ))}
                        </div>
                      </div>
                      <button className="skill-manage-btn" onClick={() => toast.info("模板管理功能将在后续版本提供")}>管理</button>
                    </div>
                  </div>

                  {/* 配置开关 */}
                  <div className="agent-detail-config">
                    {sk.toggles.map((t, ti) => {
                      const on = skillToggles[dk]?.[ti] ?? t.on;
                      return (
                        <div key={t.label} className="agent-config-toggle-row">
                          <span className="agent-config-toggle-label">{t.label}</span>
                          <button
                            className={`luxreal-toggle ${on ? "on" : ""}`}
                            onClick={() =>
                              setSkillToggles((prev) => ({
                                ...prev,
                                [dk]: (prev[dk] ?? sk.toggles.map((x) => x.on)).map((v, vi) => (vi === ti ? !v : v)),
                              }))
                            }
                          >
                            <span className="luxreal-toggle-thumb" />
                          </button>
                        </div>
                      );
                    })}
                  </div>

                  {/* 提示词模板预览 */}
                  <div className="agent-detail-section">
                    <div className="skill-preview-label">技能提示词模板</div>
                    <textarea className="skill-preview-textarea" readOnly value={curPrompt} />
                  </div>

                  <div className="flex justify-end mt-4">
                    <button
                      className="btn-gradient-primary"
                      onClick={() => {
                        toast.success("技能配置已保存");
                        setView("list");
                      }}
                    >
                      <span>确认修改</span>
                    </button>
                  </div>
                </div>
              );
            })()}
          </div>
        </div>

        <div className="luxreal-modal-footer">
          <button className="btn-ghost" onClick={onClose}>取消</button>
          <button className="btn-gradient-primary" onClick={onClose}>
            <span>确认配置</span>
          </button>
        </div>
      </div>
    </div>
  );
}
