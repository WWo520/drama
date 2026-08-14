// S3-F01 广场页 + S3-F02 任务/集市页 + S3-F03/F04 管理端 + S3-F05 开发者页（合并为 3 个页面）。
// FeedPage：广场信息流 + 互动。
import { useCallback, useEffect, useState } from "react";

import { workApi, type WorkItem } from "@duanju/api-client";

export default function FeedPage() {
  const [items, setItems] = useState<WorkItem[]>([]);
  const [toast, setToast] = useState<string | null>(null);
  const [comment, setComment] = useState<Record<string, string>>({});

  const showToast = useCallback((m: string) => {
    setToast(m);
    setTimeout(() => setToast(null), 3000);
  }, []);

  const load = useCallback(async () => {
    try {
      const d = await workApi.feed();
      setItems(d.items);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "加载失败");
    }
  }, [showToast]);

  useEffect(() => {
    void load();
  }, [load]);

  const act = async (fn: () => Promise<unknown>, okMsg: string) => {
    try {
      await fn();
      showToast(okMsg);
      void load();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "操作失败");
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] p-6 text-slate-100">
      <h1 className="text-lg font-semibold">发现 · 广场</h1>
      <p className="mt-1 text-xs text-slate-500">仅展示已审核发布的作品（S3-B01）</p>
      <div className="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {items.length === 0 && (
          <div className="col-span-full rounded-2xl border border-white/5 bg-white/[0.03] p-10 text-center text-sm text-slate-500">
            暂无已发布作品。在项目 Step4 导出后提交发布，审核通过即可在此展示。
          </div>
        )}
        {items.map((w) => (
          <div key={w.work_id} className="rounded-2xl border border-white/5 bg-white/[0.03] p-4">
            <div className="flex h-36 items-center justify-center rounded-xl bg-black/30 text-3xl">🎬</div>
            <h3 className="mt-3 text-sm font-semibold">{w.title}</h3>
            <div className="mt-1 text-xs text-slate-500">
              {w.author} · {w.duration_sec}s · {w.view_count} 播放
            </div>
            <div className="mt-3 flex items-center gap-3 text-xs text-slate-400">
              <button onClick={() => void act(() => workApi.like(w.work_id), "已点赞")}>👍 {w.like_count}</button>
              <button onClick={() => void act(() => workApi.favorite(w.work_id), "已收藏")}>⭐ {w.favorite_count}</button>
              <span>💬 {w.comment_count}</span>
            </div>
            <div className="mt-2 flex gap-2">
              <input
                value={comment[w.work_id] ?? ""}
                onChange={(e) => setComment((c) => ({ ...c, [w.work_id]: e.target.value }))}
                placeholder="评论…"
                className="flex-1 rounded-lg border border-white/10 bg-white/5 px-2 py-1.5 text-xs"
              />
              <button
                onClick={() => {
                  const c = comment[w.work_id];
                  if (c?.trim()) void act(() => workApi.comment(w.work_id, c.trim()), "评论成功");
                }}
                className="rounded-lg bg-white/10 px-3 py-1.5 text-xs"
              >
                发送
              </button>
            </div>
            <button
              onClick={() => void act(() => workApi.report(w.work_id, "内容违规"), "已举报，等待审核")}
              className="mt-2 text-xs text-red-400/70"
            >
              举报
            </button>
          </div>
        ))}
      </div>
      {toast && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200">
          {toast}
        </div>
      )}
    </div>
  );
}