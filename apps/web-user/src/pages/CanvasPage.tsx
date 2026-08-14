// S2-F02/F03 画布页：7 类节点面板 + 画布保存（乐观锁）+ 工作流执行 + 会员（S2-F04）整合入口。
// 简化实现：节点点击添加 + 网格展示 + 执行结果。S2-F04 会员弹窗以抽屉形式。
import { useCallback, useEffect, useRef, useState } from "react";

import { canvasApi, membershipApi, type CanvasNode } from "@duanju/api-client";

const NODE_TYPES: { type: CanvasNode["node_type"]; label: string; color: string }[] = [
  { type: "INPUT", label: "剧本输入", color: "bg-sky-500/20 text-sky-300" },
  { type: "SCRIPT_SPLIT", label: "剧本拆分", color: "bg-violet-500/20 text-violet-300" },
  { type: "ASSET_GEN", label: "资产生成", color: "bg-emerald-500/20 text-emerald-300" },
  { type: "VIDEO_GEN", label: "视频生成", color: "bg-amber-500/20 text-amber-300" },
  { type: "EXPORT", label: "导出合成", color: "bg-orange-500/20 text-orange-300" },
  { type: "OUTPUT", label: "输出", color: "bg-rose-500/20 text-rose-300" },
  { type: "LLM", label: "LLM 增强", color: "bg-indigo-500/20 text-indigo-300" },
];

export default function CanvasPage() {
  const [canvasId, setCanvasId] = useState("");
  const [name, setName] = useState("");
  const [nodes, setNodes] = useState<CanvasNode[]>([]);
  const [version, setVersion] = useState(1);
  const [results, setResults] = useState<Record<string, unknown>[]>([]);
  const [toast, setToast] = useState<string | null>(null);
  const [member, setMember] = useState<{ plan_code: string; renew_strategy: string } | null>(null);
  const [showMember, setShowMember] = useState(false);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const showToast = useCallback((m: string) => {
    setToast(m);
    setTimeout(() => setToast(null), 3000);
  }, []);

  // 30s 防抖保存（03 E.4.4）
  const scheduleSave = useCallback(() => {
    if (!canvasId || nodes.length === 0) return;
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(async () => {
      try {
        await canvasApi.save(canvasId, { nodes }, version);
        await canvasApi.saveNodes(canvasId, nodes);
        setVersion((v) => v + 1);
      } catch {
        showToast("保存冲突，请刷新");
      }
    }, 30000);
  }, [canvasId, nodes, version, showToast]);

  useEffect(() => {
    void membershipApi.detail().then(setMember).catch(() => undefined);
  }, []);

  const createCanvas = async () => {
    if (!name.trim()) return;
    try {
      const r = await canvasApi.create(name.trim());
      setCanvasId(r.canvas_id);
      showToast("画布已创建");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "创建失败");
    }
  };

  const addNode = (type: CanvasNode["node_type"]) => {
    const n: CanvasNode = {
      id: `node-${Date.now()}`,
      node_type: type,
      position_x: 50 + nodes.length * 20,
      position_y: 50 + nodes.length * 20,
      config: {},
    };
    setNodes((prev) => [...prev, n]);
    scheduleSave();
  };

  const removeNode = (id: string) => {
    setNodes((prev) => prev.filter((n) => n.id !== id));
    scheduleSave();
  };

  const execute = async () => {
    if (!canvasId) return;
    try {
      const edges = nodes.slice(1).map((n) => ({
        id: `e-${n.id}`,
        source: nodes[0].id,
        target: n.id,
      }));
      const r = await canvasApi.execute(canvasId, nodes, edges);
      setResults(r.results);
      showToast("工作流执行完成");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "执行失败");
    }
  };

  const buyMember = async (planCode: "PRO" | "ENTERPRISE" | "ULTIMATE", cycle: "MONTHLY" | "ANNUAL") => {
    try {
      const r = await membershipApi.purchase(planCode, cycle);
      await membershipApi.activate(r.order_no);
      showToast(`${r.plan_code} 会员已开通（¥${r.amount_yuan}）`);
      void membershipApi.detail().then(setMember);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "购买失败");
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B1A] p-6 text-slate-100">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">自由画布</h1>
        <button
          onClick={() => setShowMember(true)}
          className="rounded-lg border border-amber-400/30 px-3 py-1.5 text-xs text-amber-300"
        >
          当前：{member?.plan_code ?? "BASIC"} · 会员中心
        </button>
      </div>

      {/* 创建画布 */}
      {!canvasId ? (
        <div className="mt-6 flex items-center gap-3 rounded-2xl border border-white/5 bg-white/[0.03] p-5">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="画布名称"
            className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm"
          />
          <button
            onClick={() => void createCanvas()}
            className="rounded-lg bg-gradient-to-r from-amber-400 to-amber-500 px-5 py-2 text-sm font-semibold text-black"
          >
            创建画布
          </button>
        </div>
      ) : (
        <>
          {/* 节点面板 */}
          <div className="mt-4 flex flex-wrap gap-2">
            {NODE_TYPES.map((t) => (
              <button
                key={t.type}
                onClick={() => addNode(t.type)}
                className={`rounded-lg px-3 py-1.5 text-xs font-medium ${t.color}`}
              >
                + {t.label}
              </button>
            ))}
            <button
              onClick={() => void execute()}
              className="ml-auto rounded-lg bg-emerald-500/20 px-4 py-1.5 text-xs font-semibold text-emerald-300"
            >
              ▶ 执行工作流
            </button>
          </div>

          {/* 节点网格 */}
          <div className="mt-4 min-h-[300px] rounded-2xl border border-white/5 bg-black/20 p-4">
            {nodes.length === 0 ? (
              <div className="flex h-64 items-center justify-center text-sm text-slate-600">
                点击上方节点添加（自动 30s 防抖保存）
              </div>
            ) : (
              <div className="space-y-2">
                {nodes.map((n) => {
                  const meta = NODE_TYPES.find((t) => t.type === n.node_type);
                  return (
                    <div
                      key={n.id}
                      className={`flex items-center justify-between rounded-xl border border-white/5 px-4 py-2.5 ${meta?.color ?? ""}`}
                    >
                      <span className="text-sm">{meta?.label}</span>
                      <span className="text-xs text-slate-500">x:{Math.round(n.position_x)} y:{Math.round(n.position_y)}</span>
                      <button onClick={() => removeNode(n.id)} className="text-xs text-red-400">
                        删除
                      </button>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* 执行结果 */}
          {results.length > 0 && (
            <div className="mt-4 rounded-2xl border border-emerald-400/20 bg-emerald-400/5 p-4">
              <h3 className="text-sm font-semibold text-emerald-300">执行结果（拓扑序）</h3>
              {results.map((r, i) => (
                <div key={i} className="mt-1 text-xs text-slate-400">
                  {String(r.node_type)} → {String(r.status)}：{String(r.output)}
                </div>
              ))}
            </div>
          )}
        </>
      )}

      {/* 会员抽屉（S2-F04） */}
      {showMember && (
        <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/60" onClick={() => setShowMember(false)}>
          <div className="w-full max-w-md rounded-2xl border border-white/10 bg-[#12122A] p-6" onClick={(e) => e.stopPropagation()}>
            <h2 className="text-base font-semibold">会员套餐</h2>
            <p className="mt-1 text-xs text-slate-500">当前：{member?.plan_code ?? "BASIC"}</p>
            <div className="mt-4 space-y-3">
              {[
                { code: "PRO", name: "Pro", price: "¥29/月", desc: "高清导出" },
                { code: "ENTERPRISE", name: "Enterprise", price: "¥99/月", desc: "团队协作" },
                { code: "ULTIMATE", name: "Ultimate", price: "¥299/月", desc: "全功能" },
              ].map((p) => (
                <div key={p.code} className="flex items-center justify-between rounded-xl border border-white/10 bg-white/[0.03] p-4">
                  <div>
                    <div className="text-sm font-semibold">{p.name}</div>
                    <div className="text-xs text-slate-500">{p.desc}</div>
                  </div>
                  <div className="text-sm text-amber-300">{p.price}</div>
                  <button
                    onClick={() => void buyMember(p.code as "PRO", "MONTHLY")}
                    className="rounded-lg bg-amber-400/15 px-3 py-1.5 text-xs font-semibold text-amber-300"
                  >
                    购买
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 rounded-lg border border-amber-400/30 bg-[#1a1a35] px-5 py-2.5 text-sm text-amber-200">
          {toast}
        </div>
      )}
    </div>
  );
}