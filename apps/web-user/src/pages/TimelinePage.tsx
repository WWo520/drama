// S2-F05 多轨剪辑页：三轨时间轴可视化 + AI 对话 SSE 流式输出（4.5）。
import { useEffect, useRef, useState } from "react";

const TRACKS = [
  { name: "视频轨", color: "bg-amber-500/30", clips: ["镜头1", "镜头2", "镜头3", "镜头4"] },
  { name: "字幕轨", color: "bg-sky-500/30", clips: ["台词A", "台词B"] },
  { name: "音频轨", color: "bg-emerald-500/30", clips: ["BGM", "环境音", "对白"] },
];

export default function TimelinePage() {
  const [sseLines, setSseLines] = useState<string[]>([]);
  const [sseActive, setSseActive] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    return () => abortRef.current?.abort();
  }, []);

  const startSSE = () => {
    setSseLines([]);
    setSseActive(true);
    abortRef.current = new AbortController();
    const es = new EventSource("/api/v1/ai/dialog/stream");
    es.onmessage = (ev) => {
      try {
        const d = JSON.parse(ev.data);
        setSseLines((prev) => [...prev, String(d.text ?? ev.data)]);
      } catch {
        setSseLines((prev) => [...prev, ev.data]);
      }
    };
    es.onerror = () => {
      es.close();
      setSseActive(false);
    };
    es.addEventListener("done", () => {
      es.close();
      setSseActive(false);
    });
    abortRef.current = es as unknown as AbortController;
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] p-6 text-slate-100">
      <h1 className="text-lg font-semibold">多轨剪辑</h1>
      <p className="mt-1 text-xs text-slate-500">三轨时间轴（视频/字幕/音频）——M1 完整编辑器；当前为可视化预览</p>

      {/* 三轨时间轴 */}
      <div className="mt-5 space-y-3">
        {TRACKS.map((track) => (
          <div key={track.name} className="rounded-xl border border-white/5 bg-white/[0.02] p-3">
            <div className="mb-2 text-xs font-semibold text-slate-400">{track.name}</div>
            <div className="flex gap-2">
              {track.clips.map((clip, i) => (
                <div
                  key={i}
                  className={`flex h-12 flex-1 items-center justify-center rounded-lg text-xs font-medium ${track.color}`}
                >
                  {clip}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      {/* AI 对话 SSE */}
      <div className="mt-6 rounded-2xl border border-white/5 bg-white/[0.03] p-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">AI 剪辑助手（SSE 流式）</h2>
          <button
            onClick={startSSE}
            disabled={sseActive}
            className="rounded-lg bg-violet-500/20 px-4 py-1.5 text-xs font-semibold text-violet-300 disabled:opacity-50"
          >
            {sseActive ? "流式输出中…" : "开始对话"}
          </button>
        </div>
        <div className="mt-3 min-h-[100px] space-y-1 rounded-lg bg-black/20 p-3">
          {sseLines.length === 0 ? (
            <span className="text-xs text-slate-600">点击「开始对话」体验 SSE 流式输出（4.5）</span>
          ) : (
            sseLines.map((line, i) => (
              <div key={i} className="text-xs leading-relaxed text-slate-300">
                {line}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}