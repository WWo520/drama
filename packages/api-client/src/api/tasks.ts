// 任务域 API：统一任务轮询（后端 GET /tasks/:task_id，generation_tasks.task_uuid）。
import { api } from "../index";

export interface TaskDetail {
  task_id: string;
  task_type: string;
  status: string; // QUEUED / RUNNING / SUCCESS / FAILED / FAILED_RETRYING / FAILED_DEAD
  progress_pct: number;
  progress_message: string;
  retry_count: number;
  max_retry_count: number;
  error_code: number;
  error_message: string;
}

// 任务状态 → 徽章
export const TaskStatusMeta: Record<string, { label: string; tone: string }> = {
  QUEUED: { label: "排队中", tone: "blue" },
  RUNNING: { label: "生成中", tone: "gold" },
  SUCCESS: { label: "已完成", tone: "green" },
  FAILED: { label: "失败", tone: "red" },
  FAILED_RETRYING: { label: "重试中", tone: "gold" },
  FAILED_DEAD: { label: "失败", tone: "red" },
};

export const tasksApi = {
  // 统一任务详情轮询（Step2/3 生成进度）
  detail: (taskId: string) => api.get<TaskDetail>(`/tasks/${taskId}`),
  // 失败重试
  retry: (taskId: string) => api.post<null>(`/tasks/${taskId}/retry`),
};
