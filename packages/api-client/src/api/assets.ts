// 资产域 API：角色列表 / 批量生成 / 预签名直传（TAG 3 · 3.2.2/3.2.19 + 3.4.4/3.4.5）+ Step2 概览/场景/道具。
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
  // 多形象：该角色全部形象变体（backend ListCharacters 聚合返回；卡片形象名条/切换用）
  variants?: VariantLite[];
}

// 形象变体精简项（对齐 GET /characters/:id/variants 返回结构）
export interface VariantLite {
  id: number;
  name: string;
  main_image_url: string;
  generation_status: string;
}

export interface SceneItem {
  scene_id: number;
  code: string;
  base_name: string;
  main_image_url: string;
  generation_status: string;
  point_cost: number;
  // 场景时段变体（对齐 LuxReal 场景日夜多态）：按 time_of_day 分组的最新图
  tod_variants?: { time_of_day: string; url: string }[];
}

export interface PropItem {
  prop_id: number;
  code: string;
  name: string;
  category: string;
  main_image_url: string;
  generation_status: string;
  point_cost: number;
  description?: string; // F6: 道具描述（PATCH /props/:id 可编辑）
}

export interface AssetsOverview {
  characters: { total: number; generated: number };
  scenes: { total: number; generated: number };
  props: { total: number; generated: number };
}

// 生成状态 → 徽章（对齐 02 A5 资产枚举）
export const GenStatusMeta: Record<string, { label: string; tone: string }> = {
  PENDING: { label: "待生成", tone: "gray" },
  QUEUED: { label: "排队中", tone: "blue" },
  RUNNING: { label: "生成中", tone: "gold" },
  SUCCESS: { label: "已生成", tone: "green" },
  FAILED: { label: "失败", tone: "red" },
};

export interface GenParams {
  prompt?: string;
  model_code?: string;
  style_name?: string;
  aspect_ratio?: string;
  resolution?: string;
  use_default_composition?: boolean;
  ref_image_urls?: string[];
  // 场景时段/天气（场景日夜变体）
  time_of_day?: string;
  weather?: string;
}

export interface BatchGenerateReq {
  episode_id: string;
  items: { kind: "CHARACTER_VARIANT" | "SCENE" | "PROP"; target_id: number; gen?: GenParams }[];
  // 批量弹窗顶层参数：选中的图片模型 / 公共风格（27 项风格库）/ 每个资产生成张数（≥1）
  model_code?: string;
  style_name?: string;
  per_asset_count?: number;
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
  // Step2 资产概览（角色/场景/道具 总数与已生成数）
  overview: (episodeId: string) => api.get<AssetsOverview>(`/episodes/${episodeId}/assets/overview`),
  // 3.2.9 场景列表
  scenes: (episodeId: string) => api.get<{ items: SceneItem[] }>(`/episodes/${episodeId}/scenes`),
  // 3.2.14 道具列表
  props: (episodeId: string) => api.get<{ items: PropItem[] }>(`/episodes/${episodeId}/props`),
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