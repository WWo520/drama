// cachedApi —— GET 请求模块级 TTL 缓存 + in-flight 去重（限流治理）。
// 用于页面挂载时的高频只读请求（模型列表/偏好/积分等）：StrictMode 双挂载、HMR 重挂载、
// 多标签页/多次导航时复用缓存，避免重复请求打满后端限流（120/min）。
// 注意：仅适用于可容忍短暂陈旧的展示型数据；写操作后请调用 invalidateCachedGet 使缓存失效。
import { api } from "@duanju/api-client";

interface CacheEntry {
  at: number;
  promise: Promise<unknown>;
}

const cache = new Map<string, CacheEntry>();

/** 带 TTL 缓存与并发去重的 GET（同 key 并发共享同一 Promise） */
export function cachedGet<T>(url: string, ttlMs = 30000): Promise<T> {
  const hit = cache.get(url);
  if (hit && Date.now() - hit.at < ttlMs) return hit.promise as Promise<T>;
  const promise = api.get<T>(url).catch((e) => {
    cache.delete(url); // 失败不缓存，下次重试
    throw e;
  });
  cache.set(url, { at: Date.now(), promise });
  return promise;
}

/** 写操作后失效指定缓存（如保存偏好后清除 /me/preferences） */
export function invalidateCachedGet(url: string) {
  cache.delete(url);
}

/** 登录/登出时清空全部缓存（缓存 key 无用户维度，切换账号后防止读到上一账号数据） */
export function clearCachedGet() {
  cache.clear();
}
