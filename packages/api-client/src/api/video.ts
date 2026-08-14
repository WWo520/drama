// 视频域 API：3.3.7a 批量生成镜头视频 / 单镜头列表轮询。
import { api } from "../index";

export interface ShotVideo {
  shot_id: number;
  shot_uuid: string;
  sort_order: number;
  duration_sec: number;
  video_generation_status: string; // PENDING / RUNNING / SUCCESS / FAILED
  generated_video_url: string;
}

// 视频状态 → 徽章
export const VideoStatusMeta: Record<string, { label: string; tone: string }> = {
  PENDING: { label: "待生成", tone: "gray" },
  RUNNING: { label: "生成中", tone: "gold" },
  SUCCESS: { label: "已生成", tone: "green" },
  FAILED: { label: "失败", tone: "red" },
};

export const videoApi = {
  // 3.3.7a 批量生成 beat 下全部镜头视频
  generateBeat: (beatId: string) =>
    api.post<{ created: number; total: number }>(`/beats/${beatId}/generate-videos`),
  // 单镜头视频状态轮询（Step3）
  beatShots: (beatId: string) => api.get<{ items: ShotVideo[] }>(`/beats/${beatId}/shots`),
};