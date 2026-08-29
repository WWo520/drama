// 合成视频/图片下载到本地：fetch → blob → a[download] 触发保存（不跳转新页面）。
// 跨域 fetch 受限时回退 a[download] 直接触发（同源或服务端带 Content-Disposition 时仍为下载）。

export async function downloadToLocal(url: string, filename: string): Promise<void> {
  try {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const blob = await resp.blob();
    const objectUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = objectUrl;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(objectUrl);
  } catch {
    // 回退：a[download] 触发（浏览器可能忽略跨域 download 属性，但多数场景仍可保存）
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.rel = "noopener";
    document.body.appendChild(a);
    a.click();
    a.remove();
  }
}
