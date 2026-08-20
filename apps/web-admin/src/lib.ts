// 管理端公共工具：会话（与 api-client 统一 access_token）+ 轻提示 + 数字格式化。
import { ApiError, errorMessage } from "@duanju/api-client";

const TOKEN_KEY = "access_token"; // 与 @duanju/api-client 请求拦截器保持一致
const USER_KEY = "admin_user";

export interface AdminUser {
  user_id: string;
  nickname: string;
  email: string;
  avatar_url?: string;
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setSession(token: string, user: AdminUser): void {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function getSessionUser(): AdminUser | null {
  const raw = localStorage.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as AdminUser;
  } catch {
    return null;
  }
}

export function clearSession(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

// errMessage：ApiError 走错误码映射，其它错误取 message。
export function errMessage(e: unknown, fallback?: string): string {
  if (e instanceof ApiError) return errorMessage(e.code, e.message);
  if (e instanceof Error && e.message) return e.message;
  return fallback ?? "操作失败，请稍后重试";
}

// toast：DOM 插入式轻提示（管理端不引入 toast 库）。
export function toast(message: string, type: "success" | "error" | "info" = "info"): void {
  let holder = document.querySelector<HTMLDivElement>(".admin-toast");
  if (!holder) {
    holder = document.createElement("div");
    holder.className = "admin-toast";
    document.body.appendChild(holder);
  }
  const item = document.createElement("div");
  item.className = `admin-toast-item ${type}`;
  item.textContent = message;
  holder.appendChild(item);
  window.setTimeout(() => item.remove(), 2600);
}

// fmtNum：千分位格式化（dashboard 数字卡）。
export function fmtNum(n: number): string {
  return n.toLocaleString("en-US");
}

// exportCSV：前端生成 CSV 并触发下载（管理端列表导出，无后端接口场景）。
// headers: 列标题；rows: 每行字符串数组（与 headers 等长）。
export function exportCSV(filename: string, headers: string[], rows: string[][]): void {
  const escape = (v: string) => {
    const s = v ?? "";
    return /[,"\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const lines = [headers.map(escape).join(","), ...rows.map((r) => r.map(escape).join(","))];
  // 前置 BOM 保证 Excel 中文不乱码
  const blob = new Blob(["\uFEFF" + lines.join("\r\n")], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${filename}-${new Date().toISOString().slice(0, 10)}.csv`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
