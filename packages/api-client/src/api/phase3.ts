// 阶段 3 API（S3-F01~F05）：广场/互动/审核/任务/模板/风格/管理端/开发者。
import { api, type Paginated } from "../index";

// ---- 内容域（F01）----
export interface WorkItem {
  work_id: string;
  title: string;
  cover_url: string;
  view_count: number;
  like_count: number;
  favorite_count: number;
  comment_count: number;
  duration_sec: number;
  author: string;
  published_at: string;
}

export const workApi = {
  feed: () => api.get<Paginated<WorkItem>>("/works"),
  detail: (workId: string) => api.get<Record<string, unknown>>(`/works/${workId}`),
  submit: (p: { project_id: string; title: string; description?: string; media_url: string; tags?: string[] }) =>
    api.post<{ work_id: string; publish_status: string }>("/works", p),
  like: (workId: string) => api.post<null>(`/works/${workId}/like`),
  unlike: (workId: string) => api.delete<null>(`/works/${workId}/like`),
  favorite: (workId: string) => api.post<null>(`/works/${workId}/favorite`),
  comment: (workId: string, content: string) => api.post<null>(`/works/${workId}/comments`, { content }),
  report: (workId: string, reason: string) => api.post<null>(`/works/${workId}/report`, { reason }),
};

// ---- 审核（F04）----
export const auditApi = {
  list: () => api.get<Paginated<Record<string, unknown>>>("/audit/works"),
  review: (workId: string, approve: boolean, note?: string) =>
    api.post<null>(`/audit/works/${workId}/review`, { approve, note }),
  revoke: (workId: string) => api.post<null>(`/audit/works/${workId}/revoke`),
};

// ---- 任务（F02）----
export const taskApi = {
  list: (status?: string) => api.get<Paginated<Record<string, unknown>>>("/tasks", { params: { status } }),
  create: (p: { title: string; description?: string; reward_points?: number }) =>
    api.post<{ order_id: string; status: string }>("/tasks", p),
  intent: (orderId: string, coverLetter?: string) =>
    api.post<null>(`/tasks/${orderId}/intents`, { cover_letter: coverLetter }),
  accept: (orderId: string, userUuid: string) => api.post<null>(`/tasks/${orderId}/accept`, { user_uuid: userUuid }),
  status: (orderId: string, status: "IN_PROGRESS" | "SUBMITTED" | "COMPLETED") =>
    api.post<null>(`/tasks/${orderId}/status`, { status }),
};

// ---- 模板/风格（F02 集市）----
export const marketApi = {
  templates: () => api.get<Paginated<Record<string, unknown>>>("/templates"),
  createTemplate: (p: { name: string; description?: string; data: string; price_points?: number }) =>
    api.post<null>("/templates", p),
  applyTemplate: (templateId: string) => api.post<{ template_data: string }>(`/templates/${templateId}/apply`),
  styles: () => api.get<{ items: Record<string, unknown>[] }>("/style-library"),
  applyStyle: (projectId: string, styleCode: string) =>
    api.post<null>(`/projects/${projectId}/apply-style`, { style_code: styleCode }),
};

// ---- 管理端（F03/F04）----
export const adminApi = {
  dashboard: () => api.get<Record<string, unknown>>("/admin/dashboard"),
  users: () => api.get<Paginated<Record<string, unknown>>>("/admin/users"),
  banUser: (userUuid: string, banned: boolean) => api.patch<null>(`/admin/users/${userUuid}/ban`, { banned }),
  works: (status?: string) => api.get<Paginated<Record<string, unknown>>>("/admin/works", { params: { status } }),
  deleteWork: (workId: string) => api.delete<null>(`/admin/works/${workId}`),
  reports: () => api.get<Paginated<Record<string, unknown>>>("/admin/reports"),
};

// ---- 开发者（F05）----
export const devApi = {
  keys: () => api.get<{ items: Record<string, unknown>[] }>("/api-keys"),
  createKey: (name: string) => api.post<{ api_key: string }>("/api-keys", { name }),
  revokeKey: (prefix: string) => api.post<null>(`/api-keys/${prefix}/revoke`),
  registerWebhook: (p: { url: string; secret: string; events?: string[] }) => api.post<null>("/webhooks", p),
};