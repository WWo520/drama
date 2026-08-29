// @duanju/api-client 共享层：axios 封装 + ApiResponse 统一解包 + 401 自动 refresh 重放（06 共享层规范）。
import axios, { AxiosError, type AxiosRequestConfig } from "axios";

export * from "./types/enums";
export * from "./api/projects";
export * from "./api/split";
export * from "./api/assets";
export * from "./api/video";
export * from "./api/export";
export * from "./api/tasks";
export * from "./api/points";
export * from "./api/phase2";
export * from "./api/phase3";
export * from "./api/notifications";

// ApiResponse<T> 对齐 05 B2 章统一响应体
export interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data: T | null;
  request_id: string;
  timestamp: string;
}

export interface Pagination {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface Paginated<T> {
  items: T[];
  pagination: Pagination;
}

// 业务错误码（05 C 章错误码字典，9 大类 40+ 细分码）
export const ErrorCode = {
  OK: 0,
  UNAUTHORIZED: 10001, // 未登录 / Token 缺失/格式错误 → 跳登录页
  TOKEN_EXPIRED: 10002, // access 过期 → 静默 refresh 重放
  TOKEN_INVALID: 10003, // 签名无效 / 被篡改 → 强制踢登录页
  FORBIDDEN: 10004, // 无权限
  REFRESH_EXPIRED: 10005, // refresh 过期/拉黑 → 强制登录
  LOGIN_FAILED: 10006, // 账号或密码错误
  BANNED: 10007, // 账号被封禁
  DUPLICATE: 10008, // 邮箱/手机号已被注册
  PROJECT_NOT_FOUND: 20001,
  SPACE_NOT_FOUND: 20005,
  INSUFFICIENT_POINTS: 40003,
  RATE_LIMITED: 90001,
  CONFLICT: 90006,
  INTERNAL: 99999,
} as const;

// 错误码 → 用户提示文案映射（05 C 章：前端禁止硬编码 toast 文案，一律拦截器映射）
const ERROR_MESSAGES: Record<number, string> = {
  [ErrorCode.UNAUTHORIZED]: "请先登录",
  [ErrorCode.TOKEN_EXPIRED]: "登录已过期，正在刷新…",
  [ErrorCode.TOKEN_INVALID]: "登录状态无效，请重新登录",
  [ErrorCode.FORBIDDEN]: "没有操作权限",
  [ErrorCode.REFRESH_EXPIRED]: "登录已过期，请重新登录",
  [ErrorCode.LOGIN_FAILED]: "账号或密码错误",
  [ErrorCode.BANNED]: "账号已被封禁，请联系客服",
  [ErrorCode.DUPLICATE]: "该邮箱/手机号已被注册",
  [ErrorCode.PROJECT_NOT_FOUND]: "项目不存在或无权限",
  [ErrorCode.SPACE_NOT_FOUND]: "空间不存在或已关闭",
  [ErrorCode.INSUFFICIENT_POINTS]: "积分余额不足",
  [ErrorCode.RATE_LIMITED]: "操作过于频繁，请稍后再试",
  [ErrorCode.CONFLICT]: "名称已存在，请换一个",
  [ErrorCode.INTERNAL]: "服务器开小差了，请稍后重试",
};

export function errorMessage(code: number, fallback?: string): string {
  return ERROR_MESSAGES[code] ?? fallback ?? "操作失败，请稍后重试";
}

const http = axios.create({
  baseURL: "/api/v1",
  timeout: 15000,
  withCredentials: true, // refresh_token HttpOnly Cookie
});

// 应用域标记：用户端/管理端同 host 部署时（浏览器 Cookie 按 host 隔离、不区分端口），
// 后端按 X-App-Realm 头将 refresh Cookie 隔离为不同名字，防止两端登录互相覆盖导致串号。
let appRealm = "user";
export function setAppRealm(realm: "user" | "admin") {
  appRealm = realm;
}

// 请求拦截：注入 access_token + X-App-Realm
http.interceptors.request.use((config) => {
  config.headers["X-App-Realm"] = appRealm;
  const token = localStorage.getItem("access_token");
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// 响应拦截：ApiResponse 解包 + 401 自动 refresh 重放（05 B3 章流程）
let refreshing: Promise<string | null> | null = null;

async function refreshAccessToken(): Promise<string | null> {
  try {
    const resp = await axios.post<ApiResponse<{ access_token: string }>>(
      "/api/v1/auth/refresh",
      {},
      { withCredentials: true, headers: { "X-App-Realm": appRealm } },
    );
    if (resp.data.code === 0 && resp.data.data) {
      localStorage.setItem("access_token", resp.data.data.access_token);
      return resp.data.data.access_token;
    }
  } catch {
    // refresh 失败 → 踢登录页
  }
  localStorage.removeItem("access_token");
  window.location.href = "/login";
  return null;
}

http.interceptors.response.use(
  (resp) => resp,
  async (error: AxiosError<ApiResponse>) => {
    const original = error.config as AxiosRequestConfig & { _retried?: boolean };
    const code = error.response?.data?.code;

    if (error.response?.status === 401 && code === ErrorCode.TOKEN_EXPIRED && !original._retried) {
      original._retried = true;
      refreshing = refreshing ?? refreshAccessToken();
      const token = await refreshing;
      refreshing = null;
      if (token) {
        original.headers = { ...original.headers, Authorization: `Bearer ${token}` };
        return http(original);
      }
    }
    // HTTP 错误但携带业务码（如 400 + code 40003）→ 转为 ApiError，供页面错误码映射使用
    const data = error.response?.data;
    if (data && typeof data === "object" && typeof data.code === "number" && data.code !== 0) {
      return Promise.reject(new ApiError(data.code, data.message || errorMessage(data.code)));
    }
    return Promise.reject(error);
  },
);

// 类型化请求：自动解包 data，业务错误抛出带 code 的 ApiError
export class ApiError extends Error {
  code: number;
  constructor(code: number, message: string) {
    super(message);
    this.code = code;
  }
}

export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const resp = await http.request<ApiResponse<T>>(config);
  if (resp.data.code !== 0) {
    throw new ApiError(resp.data.code, resp.data.message || errorMessage(resp.data.code));
  }
  return resp.data.data as T;
}

export const api = {
  get: <T>(url: string, config?: AxiosRequestConfig) => request<T>({ method: "GET", url, ...config }),
  post: <T>(url: string, data?: unknown, config?: AxiosRequestConfig) =>
    request<T>({ method: "POST", url, data, ...config }),
  put: <T>(url: string, data?: unknown, config?: AxiosRequestConfig) =>
    request<T>({ method: "PUT", url, data, ...config }),
  patch: <T>(url: string, data?: unknown, config?: AxiosRequestConfig) =>
    request<T>({ method: "PATCH", url, data, ...config }),
  delete: <T>(url: string, config?: AxiosRequestConfig) => request<T>({ method: "DELETE", url, ...config }),
};

export default http;
