// 导出域 API：3.4.1 触发导出 / 3.4.2 详情 / 3.4.3 历史。
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
}

export const exportApi = {
  // 3.4.1 触发导出
  trigger: (projectId: string, resolution = "P720", watermark = true) =>
    api.post<{ export_id: string; status: string }>(`/projects/${projectId}/export`, {
      format: "MP4_H264",
      resolution,
      watermark,
    }),
  // 3.4.3 导出历史
  list: (projectId: string) =>
    api.get<Paginated<ExportItem>>("/exports", { params: { project_id: projectId } }),
  // 3.4.2 导出详情
  detail: (exportId: string) => api.get<ExportItem>(`/exports/${exportId}`),
};