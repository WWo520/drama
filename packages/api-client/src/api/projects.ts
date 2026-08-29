// 创作域 API：项目 CRUD（TAG 3 · 3.0.1~3.0.7）。
import { api, type Paginated } from "../index";

// 项目状态（03 A.1 状态机，对齐后端 project_status_enum）
export const ProjectStatus = {
  DRAFT: "DRAFT",
  SCRIPT_SPLITTING: "SCRIPT_SPLITTING",
  SCRIPT_SPLIT_FAILED: "SCRIPT_SPLIT_FAILED",
  PARTIAL_ASSETS: "PARTIAL_ASSETS",
  ALL_ASSETS_READY: "ALL_ASSETS_READY",
  VIDEO_GENERATING: "VIDEO_GENERATING",
  PARTIAL_VIDEOS: "PARTIAL_VIDEOS",
  ALL_VIDEOS_READY: "ALL_VIDEOS_READY",
  EXPORTING: "EXPORTING",
  COMPLETED: "COMPLETED",
  ARCHIVED: "ARCHIVED",
  CANCELLED: "CANCELLED",
} as const;

export type ProjectStatusValue = (typeof ProjectStatus)[keyof typeof ProjectStatus];

export interface ProjectSummary {
  project_id: string;
  title: string;
  status: ProjectStatusValue;
  current_step: string;
  total_episode_count: number;
  total_duration_sec: number;
  cover_image_url: string | null;
  updated_at: string;
}

export interface ProjectDetail extends ProjectSummary {
  project_type: string;
  source_script_text: string;
  config: Record<string, unknown>;
  tags: string[];
}

export interface CreateProjectReq {
  title: string;
  source_script_text?: string;
  tags?: string[];
  /** 项目配置快照（模型/分辨率/时长/风格），贯穿 Step2/Step3 默认值 */
  config?: Record<string, unknown>;
}

// 状态 → 徽章文案/颜色（前端映射，03 A.1）
export const ProjectStatusMeta: Record<
  ProjectStatusValue,
  { label: string; tone: "gold" | "violet" | "blue" | "green" | "gray" }
> = {
  DRAFT: { label: "草稿", tone: "gray" },
  SCRIPT_SPLITTING: { label: "剧本拆解中", tone: "blue" },
  SCRIPT_SPLIT_FAILED: { label: "拆分失败", tone: "gray" },
  PARTIAL_ASSETS: { label: "资产生成中", tone: "violet" },
  ALL_ASSETS_READY: { label: "资产就绪", tone: "green" },
  VIDEO_GENERATING: { label: "视频生成中", tone: "blue" },
  PARTIAL_VIDEOS: { label: "视频生成中", tone: "blue" },
  ALL_VIDEOS_READY: { label: "视频就绪", tone: "gold" },
  EXPORTING: { label: "导出中", tone: "gold" },
  COMPLETED: { label: "已完成", tone: "green" },
  ARCHIVED: { label: "已归档", tone: "gray" },
  CANCELLED: { label: "已取消", tone: "gray" },
};

// 项目 API（3.0.1~3.0.7）
export const projectsApi = {
  // 3.0.1 项目列表
  list: (page = 1, pageSize = 20) =>
    api.get<Paginated<ProjectSummary>>("/projects", { params: { page, page_size: pageSize } }),
  // 3.0.2 创建项目
  create: (data: CreateProjectReq) => api.post<{ project_id: string; status: string }>("/projects", data),
  // 3.0.3 项目详情
  detail: (projectId: string) => api.get<ProjectDetail>(`/projects/${projectId}`),
  // 3.0.4 更新项目
  update: (projectId: string, data: Partial<CreateProjectReq>) =>
    api.put<ProjectDetail>(`/projects/${projectId}`, data),
  // 3.0.5 删除项目（软删）
  remove: (projectId: string) => api.delete<null>(`/projects/${projectId}`),
  // 3.0.6 恢复项目
  restore: (projectId: string) => api.post<null>(`/projects/${projectId}`),
  // 3.0.7 最近打开
  touch: (projectId: string) => api.post<null>(`/projects/${projectId}/last-opened`),
  // 项目配置快照合并更新（PATCH /projects/:id/config）
  updateConfig: (projectId: string, patch: Record<string, unknown>) =>
    api.patch<{ updated: boolean }>(`/projects/${projectId}/config`, patch),
};