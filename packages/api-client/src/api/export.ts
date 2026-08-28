// 导出域 API：3.4.1 触发导出 / 3.4.2 详情 / 3.4.3 历史 / 预检 / 重试。
import { api, type Paginated } from "../index";

export interface ExportItem {
  export_id: string;
  status: string; // PENDING / SUCCESS / FAILED
  export_type: string;
  resolution: string;
  watermark: boolean;
  output_url: string;
  file_size: number;
  expire_at: string;
  created_at: string;
  // 合成进度（真实 ffmpeg 链路回写）
  progress_stage?: string; // "" | DOWNLOADING | COMPOSING | UPLOADING | DONE
  progress_detail?: { done?: number; total?: number; failed?: number };
  segments_total?: number;
  segments_failed?: number;
  duration_sec?: number;
}

// 合成前预检结果：去重后片段统计
export interface ExportPreflight {
  total: number; // 去重后可参与合成的片段数
  unique_shots: number;
  total_duration_sec: number;
  duplicates: number; // 被去重掉的重复镜头数
  beats: number; // 涉及的叙事单元数
}

export const exportApi = {
  // 3.4.1 触发导出（episodeId = 当前集 UUID；缺省由后端回退首集）
  trigger: (projectId: string, resolution = "P720", watermark = true, episodeId?: string) =>
    api.post<{ export_id: string; status: string }>(`/projects/${projectId}/export`, {
      format: "MP4_H264",
      resolution,
      watermark,
      episode_id: episodeId,
    }),
  // 3.4.3 导出历史
  list: (projectId: string) =>
    api.get<Paginated<ExportItem>>("/exports", { params: { project_id: projectId } }),
  // 3.4.2 导出详情
  detail: (exportId: string) => api.get<ExportItem>(`/exports/${exportId}`),
  // 合成前预检：片段可用性统计（弹窗展示"将合成 N 个片段"）
  preflight: (projectId: string, episodeId?: string) =>
    api.get<ExportPreflight>(`/projects/${projectId}/export/preflight`, {
      params: { episode_id: episodeId },
    }),
  // 重新合成：签名 URL 过期（403）或 FAILED 记录一键重发起
  retry: (exportId: string) =>
    api.post<{ export_id: string; status: string }>(`/exports/${exportId}/retry`),
};
