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

// 叙事单元 ↔ 资产关联（17 - 工作台对齐：Step3 关联资产名称占位 + 单元级就绪度）
export interface NuAssetLinkItem {
  beat_id: string;
  asset_kind: "CHARACTER" | "SCENE" | "PROP";
  asset_id: number;
  name: string;
  main_image_url: string;
  generation_status: string; // PENDING / QUEUED / RUNNING / SUCCESS / FAILED
}

// 视频状态 → 徽章
export const VideoStatusMeta: Record<string, { label: string; tone: string }> = {
  PENDING: { label: "待生成", tone: "gray" },
  RUNNING: { label: "生成中", tone: "gold" },
  SUCCESS: { label: "已生成", tone: "green" },
  FAILED: { label: "失败", tone: "red" },
};

export const videoApi = {
  // 3.3.7a 批量生成 beat 下全部镜头视频（18 - 视频制作：弹窗参数随请求提交落任务表）
  generateBeat: (
    beatId: string,
    opts?: { resolution?: string; video_model?: string; model_key?: string },
  ) =>
    api.post<{ created: number; total: number }>(`/beats/${beatId}/generate-videos`, {
      resolution: opts?.resolution,
      video_model: opts?.video_model,
      model_key: opts?.model_key,
    }),
  // 单镜头视频状态轮询（Step3）
  beatShots: (beatId: string) => api.get<{ items: ShotVideo[] }>(`/beats/${beatId}/shots`),
  // 叙事单元 ↔ 资产关联清单（Step3 关联资产占位 + 就绪度）
  nuAssetLinks: (episodeId: string) =>
    api.get<{ items: NuAssetLinkItem[] }>(`/episodes/${episodeId}/asset-links`),
  // F11 单镜头重新生成（18 - 视频制作 阶段三：真实链路——计费冻结 + Outbox 派发）
  regenerateShot: (shotId: string) =>
    api.post<{ task_id: string; shot_id: number }>(`/shots/${shotId}/regenerate`),
  // 生成镜头级参考图（18 - 视频制作 阶段三：对齐 LuxReal「镜头N的参考图」，SHOT_REF_IMAGE_GEN）
  generateShotRefImage: (shotId: string) =>
    api.post<{ task_id: string; shot_id: number }>(`/shots/${shotId}/generate-reference-image`),
};
