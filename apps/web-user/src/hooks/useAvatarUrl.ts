// useAvatarUrl —— 头像显示 URL 解析（users.avatar_url 存 MinIO object key）。
// 链路：GET /users/me/avatar（鉴权）→ 后端现签 24h GET 预签名 URL → img 直接渲染（预签名 URL 无需鉴权）。
// 模块级缓存按 objectKey 复用：6 处侧栏 + 面板共享，同 key 只请求一次；上传成功后可用 setAvatarUrlCache 直接注入。
import { useEffect, useState } from "react";

import { api } from "@duanju/api-client";

const cache = new Map<string, string>();

// setAvatarUrlCache 上传成功后直接注入 presign 返回的 download_url，免去二次请求。
export function setAvatarUrlCache(objectKey: string, signedUrl: string) {
  cache.set(objectKey, signedUrl);
}

export function useAvatarUrl(objectKey?: string | null): string | null {
  const [url, setUrl] = useState<string | null>(objectKey ? cache.get(objectKey) ?? null : null);

  useEffect(() => {
    if (!objectKey) {
      setUrl(null);
      return;
    }
    const hit = cache.get(objectKey);
    if (hit) {
      setUrl(hit);
      return;
    }
    let alive = true;
    api
      .get<{ avatar_url: string }>("/users/me/avatar")
      .then((d) => {
        if (d.avatar_url) {
          cache.set(objectKey, d.avatar_url);
          if (alive) setUrl(d.avatar_url);
        }
      })
      .catch(() => {
        /* 头像解析失败回退字母头像，不阻塞页面 */
      });
    return () => {
      alive = false;
    };
  }, [objectKey]);

  return url;
}
