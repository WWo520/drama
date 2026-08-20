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

export const splitApi = {
  // 3.1.1 触发拆分（创建项目已自动触发；本接口用于失败重试/强制重拆；scriptText 为 Step1 页输入）
  triggerSplit: (projectId: string, force = false, scriptText?: string) =>
    api.post<{ task_uuid: string; status: string }>(`/projects/${projectId}/split-script`, {
      force,
      script_text: scriptText,
    }),
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