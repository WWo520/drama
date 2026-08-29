// 视频域 API：成片卡主线（规划视频任务 / 批量生成 / 单卡重生成 / 提示词编辑）+ 兼容端点。
import { api } from "../index";

// 镜头关联资产（shot_asset_refs JOIN 业务表；Step3 镜头卡资产小图与 @ 引用展示）
export interface ShotLinkedAsset {
  asset_kind: "CHARACTER" | "SCENE" | "PROP";
  asset_id: number;
  name: string;
  main_image_url: string;
  generation_status: string;
}

// 成片卡内嵌镜头摘要（subject/dialogue 仅卡视图返回）
export interface VideoCardShot {
  shot_id: number;
  shot_uuid: string;
  sort_order: number;
  duration_sec: number;
  video_generation_status: string;
  generated_video_url?: string;
  subject_description?: string;
  dialogue_text?: string;
  linked_assets?: ShotLinkedAsset[];
}

// 成片卡（对齐 LuxReal「成片卡」：一次视频生成任务 = 2~4 个连续分镜镜头的合并段）
export interface VideoCard {
  card_id: number;
  card_uuid: string;
  title: string;
  sort_order: number;
  duration_sec: number;
  shot_count: number;
  generation_status: string; // PENDING / RUNNING / SUCCESS / FAILED_DEAD
  generated_video_url: string;
  prompt_text: string; // 自定义提示词；空 = 按分镜脚本三段式自动组装
  shots?: VideoCardShot[];
}

export interface ShotVideo {
  shot_id: number;
  shot_uuid: string;
  sort_order: number;
  duration_sec: number;
  video_generation_status: string; // PENDING / RUNNING / SUCCESS / FAILED
  generated_video_url: string;
  linked_assets?: ShotLinkedAsset[];
  subject_description?: string;
  dialogue_text?: string;
}

// 叙事单元 ↔ 资产关联（17 - 工作台对齐：Step3 关联资产名称占位 + 单元级就绪度）
export interface NuAssetLinkItem {
  beat_id: string;
  asset_kind: "CHARACTER" | "SCENE" | "PROP";
  asset_id: number;
  name: string;
  main_image_url: string;
  generation_status: string; // PENDING / QUEUED / RUNNING / SUCCESS / FAILED
  /** 资产原属集（episode_uuid）：未生成时跳转 Step2 定位生成入口 */
  origin_episode_id?: string;
}

// 视频状态 → 徽章
export const VideoStatusMeta: Record<string, { label: string; tone: string }> = {
  PENDING: { label: "待生成", tone: "gray" },
  QUEUED: { label: "排队中", tone: "gold" },
  RUNNING: { label: "生成中", tone: "gold" },
  SUCCESS: { label: "已生成", tone: "green" },
  FAILED: { label: "失败", tone: "red" },
};

export const videoApi = {
  // ===== 成片卡主线（对齐 LuxReal 两级结构）=====
  // 规划视频任务：连续分镜镜头编组成片卡（4~10s、2~4 镜头）；force=true 确认清空旧卡
  planVideoTasks: (beatId: string, force?: boolean) =>
    api.post<{ created: number; total_cards: number; total_shots: number }>(
      `/beats/${beatId}/plan-video-tasks`,
      { force: !!force },
    ),
  // 成片卡列表轮询（Step3 网格数据源）
  beatCards: (beatId: string) => api.get<{ items: VideoCard[] }>(`/beats/${beatId}/cards`),
  // 单成片卡重新生成（计费冻结 + Outbox）
  regenerateCard: (cardUuid: string) =>
    api.post<{ task_id: string; card_id: number }>(`/video-cards/${cardUuid}/regenerate`),
  // 编辑成片卡提示词（空串恢复按分镜脚本自动组装）
  updateCardPrompt: (cardUuid: string, prompt_text: string) =>
    api.patch<{ card_id: number }>(`/video-cards/${cardUuid}/prompt`, { prompt_text }),
  // 调整成片卡送模型时长（4~15s；对齐 LuxReal 卡头时长下拉）
  updateCardDuration: (cardUuid: string, duration_sec: number) =>
    api.patch<{ card_id: number; duration_sec: number }>(`/video-cards/${cardUuid}/duration`, { duration_sec }),
  // 参考图拖拽绑定：把资产绑定进卡作为参考图（作用于卡内全部镜头）
  bindCardAsset: (cardUuid: string, asset_kind: string, asset_id: number) =>
    api.post<{ card_id: number; bound_shots: number }>(`/video-cards/${cardUuid}/assets/bind`, {
      asset_kind,
      asset_id,
    }),
  // 解绑卡参考资产（无镜头级覆盖后自动回落单元级链，不会断参考）
  unbindCardAsset: (cardUuid: string, asset_kind: string, asset_id: number) =>
    api.delete<{ card_id: number }>(
      `/video-cards/${cardUuid}/assets/bind?asset_kind=${asset_kind}&asset_id=${asset_id}`,
    ),
  // 镜头级参考资产绑定（精准到单个镜头；镜头级显式引用生效后不再回落单元级继承）
  bindShotAsset: (shotId: string, asset_kind: string, asset_id: number) =>
    api.post<{ shot_id: string; inserted: boolean }>(`/shots/${shotId}/assets/bind`, {
      asset_kind,
      asset_id,
    }),
  // 镜头级解绑参考资产（removed=false 表示该引用来自叙事单元级继承，无法按镜头单独摘除）
  unbindShotAsset: (shotId: string, asset_kind: string, asset_id: number) =>
    api.delete<{ shot_id: string; removed: boolean }>(
      `/shots/${shotId}/assets/bind?asset_kind=${asset_kind}&asset_id=${asset_id}`,
    ),

  // ===== 兼容旧链路 =====
  // 批量生成单元下全部成片卡视频（弹窗参数随请求落任务表）
  // redo=true：重置已成功卡片后全量重建（默认只补缺失卡片）
  generateBeat: (
    beatId: string,
    opts?: { resolution?: string; video_model?: string; model_key?: string; redo?: boolean },
  ) =>
    api.post<{ created: number; total: number }>(`/beats/${beatId}/generate-videos`, {
      resolution: opts?.resolution,
      video_model: opts?.video_model,
      model_key: opts?.model_key,
      redo: opts?.redo,
    }),
  // 单镜头视频状态轮询（旧链路兼容；Step3 主视图已切成片卡）
  beatShots: (beatId: string) => api.get<{ items: ShotVideo[] }>(`/beats/${beatId}/shots`),
  // 叙事单元 ↔ 资产关联清单（Step3 关联资产占位 + 就绪度）
  nuAssetLinks: (episodeId: string) =>
    api.get<{ items: NuAssetLinkItem[] }>(`/episodes/${episodeId}/asset-links`),
  // F11 单镜头重新生成（旧链路兼容）
  regenerateShot: (shotId: string) =>
    api.post<{ task_id: string; shot_id: number }>(`/shots/${shotId}/regenerate`),
  // 生成镜头级参考图（SHOT_REF_IMAGE_GEN）
  generateShotRefImage: (shotId: string) =>
    api.post<{ task_id: string; shot_id: number }>(`/shots/${shotId}/generate-reference-image`),
  // 镜头编辑（提示词/时长/分镜字段微调；nil 字段不更新）
  updateShot: (
    shotId: string,
    patch: Partial<{
      prompt_refined: string;
      duration_sec: number;
      subject_description: string;
      background_description: string;
      dialogue_text: string;
      ambient_sound: string;
    }>,
  ) => api.patch<{ shot_id: string; duration_sec: number; prompt_refined: string }>(`/shots/${shotId}`, patch),
};
