// 通知 API：8.1 列表 / 8.2 单条已读 / 8.3 全部已读（05 TAG 8，后端 notify/api.go）。
import { api } from "../index";

export interface NotificationItem {
  notification_id: number;
  type: string; // SYSTEM / CREATION / INTERACTION / AUDIT / SPACE / BILLING
  title: string;
  content: string;
  link: string;
  is_read: boolean;
  created_at: string;
}

// 后端分页仅返回 page/page_size/total（无 total_pages），单独定义
export interface NotificationListResp {
  items: NotificationItem[] | null;
  pagination: { page: number; page_size: number; total: number };
}

export const notificationsApi = {
  // 8.1 通知列表（分页）
  list: (page = 1, pageSize = 20) =>
    api.get<NotificationListResp>(`/notifications?page=${page}&page_size=${pageSize}`),
  // 8.2 单条已读
  markRead: (notificationId: number) => api.put<null>(`/notifications/${notificationId}/read`),
  // 8.3 全部已读
  markAllRead: () => api.put<null>("/notifications/read-all"),
};
