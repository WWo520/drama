// 资产域 API：角色列表 / 批量生成 / 预签名直传（TAG 3 · 3.2.2/3.2.19 + 3.4.4/3.4.5）。
import { api } from "../index";

export interface CharacterItem {
  id: number;
  code: string;
  name: string;
  tags: string;
  description_notes: string;
  main_image_url: string;
  generation_status: string; // PENDING / QUEUED / RUNNING / SUCCESS / FAILED
  origin_episode_id: number | null;
}

// 生成状态 → 徽章（对齐 02 A5 资产枚举）
export const GenStatusMeta: Record<string, { label: string; tone: string }> = {
  PENDING: { label: "待生成", tone: "gray" },
  QUEUED: { label: "排队中", tone: "blue" },
  RUNNING: { label: "生成中", tone: "gold" },
  SUCCESS: { label: "已生成", tone: "green" },
  FAILED: { label: "失败", tone: "red" },
};

export interface BatchGenerateReq {
  episode_id: string;
  items: { kind: "CHARACTER_VARIANT" | "SCENE" | "PROP"; target_id: number }[];
}

export interface PresignResp {
  upload_url: string;
  object_key: string;
  download_url: string;
  expires_at: string;
  upload_token: string;
}

export interface CompleteUploadReq {
  upload_token: string;
  object_key: string;
  etag: string;
  biz_ref_id: string;
}

export const assetsApi = {
  // 3.2.2 角色列表
  characters: (episodeId: string) =>
    api.get<{ items: CharacterItem[] }>(`/episodes/${episodeId}/characters`),
  // 3.2.19 批量生成资产图
  batchGenerate: (req: BatchGenerateReq) =>
    api.post<{ task_uuids: string[]; created: number }>("/assets/batch-generate", req),
  // 3.4.4 预签名上传
  presign: (data: { biz_type: string; file_name: string; content_type: string; size: number }) =>
    api.post<PresignResp>("/uploads/presign", data),
  // 3.4.5 上传回执
  complete: (data: CompleteUploadReq) =>
    api.post<null>("/uploads/complete", data),
};