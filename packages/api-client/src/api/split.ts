// 拆分域 API：触发拆分 / 剧集列表 / 叙事单元列表（TAG 3 · 3.1.1/3.1.2/3.1.4/3.1.5）。
import { api } from "../index";

export interface EpisodeLite {
  episode_id: string;
  sort_order: number; // 后端 EpisodeDTO 字段（集序号）
  title: string;
  status: string;
  current_step: string;
  narrative_unit_count: number;
  actual_duration_sec: number;
}

export interface BeatSummary {
  beat_id: string;
  title: string;
  sort_order: number;
  status: string;
  preset_duration_sec: number;
  actual_duration_sec: number;
  script_text: string;
  shot_group_count: number;
  video_generation_status: string | null;
}

export interface BeatDetail extends BeatSummary {
  shot_groups: {
    shot_group_id: string;
    sort_order: number;
    video_generation_status: string | null;
    shots: {
      shot_id: string;
      sort_order: number;
      duration_sec: number;
      shot_scale: string;
      camera_movement: string;
      camera_angle: string;
      subject_description: string;
      dialogue_text: string;
      video_generation_status: string | null;
    }[];
  }[];
}

// 剧本拆分技能（全局 agent_skills 库，kind=SCRIPT_SPLIT；GET /skills）
export interface SplitSkill {
  skill_code: string;
  name: string;
  description: string;
  template_content: string;
}

export const splitApi = {
  // 3.1.1 触发拆分（创建项目已自动触发；本接口用于失败重试/强制重拆；scriptText 为 Step1 页输入；
  // force=true 表示重新拆分（覆盖已有拆分结果，后端要求 PARTIAL_ASSETS/ALL_ASSETS_READY 阶段显式确认）；
  // splitSkillCode 为拆分技能（空=默认标准拆分））
  triggerSplit: (projectId: string, force = false, scriptText?: string, episodeDurationMin?: number, splitSkillCode?: string) =>
    api.post<{ task_uuid: string; status: string }>(`/projects/${projectId}/split-script`, {
      force,
      script_text: scriptText,
      episode_duration_min: episodeDurationMin,
      split_skill_code: splitSkillCode,
    }),
  // 剧本拆分技能列表（Step1 拆分弹窗 / Agent 技能弹窗）
  splitSkills: () => api.get<{ items: SplitSkill[] }>("/skills", { params: { kind: "SCRIPT_SPLIT" } }),
  // 3.1.2 项目剧集列表
  episodes: (projectId: string) =>
    api.get<{ items: EpisodeLite[] }>(`/projects/${projectId}/episodes`),
  // 3.1.4 叙事单元列表（Step1 拆分结果树）
  beats: (episodeId: string) =>
    api.get<{ items: BeatSummary[] }>(`/episodes/${episodeId}/beats`),
  // 3.1.5 叙事单元详情（含剧本原文）
  beatDetail: (beatId: string) => api.get<BeatDetail>(`/beats/${beatId}`),
  // 3.3.7c 叙事单元重新规划（开发期 mock：入队 BEAT_REPLAN 任务）
  replan: (beatId: string, planningMode?: string) =>
    api.post<{ task_id: string; status: string; is_mock?: boolean }>(`/beats/${beatId}/replan`, {
      planning_mode: planningMode,
    }),
};