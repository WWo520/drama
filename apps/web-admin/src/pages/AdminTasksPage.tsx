// AdminTasksPage：迁移自 admin-tasks.html（DOM 1:1）。
// 数据策略：任务列表 GET /admin/tasks（status/task_type 服务端过滤）真数据，失败回退设计稿 6 行示例；
// 任务队列/平均时长统计卡接 GET /admin/tasks/stats 真数据（失败回退设计稿静态值）；
// GPU 利用率卡与 32 节点网格无后端数据源，保持设计稿静态（标注示例数据）。
import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@duanju/api-client";
import {
  ArrowUp,
  ChevronLeft,
  ChevronRight,
  Clock,
  Download,
  Eye,
  ListTodo,
  RefreshCw,
  RotateCw,
  Server,
  Settings2,
  Square,
  X,
} from "lucide-react";

import { toast, errMessage, fmtNum } from "../lib";
import Modal from "../components/Modal";
import "./AdminTasksPage.css";

type TaskTab = "all" | "queued" | "running" | "done" | "failed";

interface BackendTask {
  task_uuid: string;
  request_id: string;
  task_type: string;
  status: string;
  progress_pct: number;
  project_title: string;
  author: string;
  gpu_node_id: string;
  execution_ms: number;
  created_at: string;
}

interface TasksResp {
  items: BackendTask[];
  pagination: { page: number; page_size: number; total: number };
}

interface TaskStatsResp {
  queued: number;
  running: number;
  done_today: number;
  failed: number;
  avg_execution_ms: number;
}

// 任务详情（GET /admin/tasks/:task_id）
interface TaskDetailResp {
  task_uuid: string;
  request_id: string;
  task_type: string;
  status: string;
  progress_pct: number;
  priority: number;
  retry_count: number;
  gpu_node_id: string;
  external_task_id: string;
  ai_model_actual_used: string;
  request_params: unknown;
  result_output: unknown;
  error_message: string | null;
  user_display_error: string | null;
  enqueued_at: string;
  started_at: string;
  completed_at: string;
  created_at: string;
}

interface TaskRow {
  task_uuid: string;
  task_id: string;
  typeLabel: string;
  typeCls: string;
  project: string;
  author: string;
  authorInitial: string;
  authorGrad: string;
  progress: number;
  status: string;
  gpu: string;
  cost: string;
  createdAt: string;
  real: boolean;
}

const PAGE_SIZE = 10;

// Tab → 服务端 status 过滤（多值逗号分隔，后端 IN 查询）
const TAB_STATUS: Record<TaskTab, string> = {
  all: "",
  queued: "PENDING,QUEUED",
  running: "RUNNING",
  done: "SUCCESS",
  failed: "FAILED_RETRYING,FAILED_DEAD",
};

const TAB_LABEL: Record<TaskTab, string> = {
  all: "全部任务",
  queued: "排队中",
  running: "生成中",
  done: "已完成",
  failed: "失败",
};

// 状态 → 展示（对齐 generation_status_enum）
const STATUS_META: Record<string, { label: string; cls: string; dot: string }> = {
  PENDING: { label: "排队中", cls: "bg-yellow-500/15 text-yellow-400", dot: "bg-yellow-400" },
  QUEUED: { label: "排队中", cls: "bg-yellow-500/15 text-yellow-400", dot: "bg-yellow-400" },
  RUNNING: { label: "生成中", cls: "bg-blue-500/15 text-blue-400", dot: "bg-blue-400" },
  SUCCESS: { label: "已完成", cls: "bg-green-500/15 text-green-400", dot: "bg-green-400" },
  FAILED_RETRYING: { label: "重试中", cls: "bg-yellow-500/15 text-yellow-400", dot: "bg-yellow-400" },
  FAILED_DEAD: { label: "失败", cls: "bg-red-500/15 text-red-400", dot: "bg-red-400" },
  CANCELED_USER: { label: "已取消", cls: "bg-gray-500/15 text-gray-400", dot: "bg-gray-400" },
  CANCELED_TIMEOUT: { label: "超时取消", cls: "bg-gray-500/15 text-gray-400", dot: "bg-gray-400" },
  SKIPPED: { label: "已跳过", cls: "bg-gray-500/15 text-gray-400", dot: "bg-gray-400" },
  PAUSED_USER: { label: "已暂停", cls: "bg-gray-500/15 text-gray-400", dot: "bg-gray-400" },
};

// 任务类型 → 展示（对齐 task_type_enum 14 项）
const TYPE_META: Record<string, { label: string; cls: string }> = {
  SCRIPT_SPLIT: { label: "剧本拆分", cls: "bg-cyan-500/15 text-cyan-400" },
  STORYBOARD_PLAN_V1_LUXCHAT: { label: "分镜规划", cls: "bg-amber-500/15 text-amber-400" },
  CHAR_IMAGE_GEN: { label: "角色图片", cls: "bg-pink-500/15 text-pink-400" },
  SCENE_IMAGE_GEN: { label: "场景图片", cls: "bg-teal-500/15 text-teal-400" },
  PROP_IMAGE_GEN: { label: "道具图片", cls: "bg-orange-500/15 text-orange-400" },
  SMART_SHOT_GROUP_PRE_ANCHOR: { label: "智能锚点", cls: "bg-cyan-500/15 text-cyan-400" },
  PROMPT_REFINEMENT_LUXGEM: { label: "提示词优化", cls: "bg-rose-500/15 text-rose-400" },
  VIDEO_SEGMENT_GEN: { label: "视频生成", cls: "bg-purple-500/15 text-purple-400" },
  VIDEO_GROUP_CONCAT: { label: "视频合成", cls: "bg-blue-500/15 text-blue-400" },
  "3DGS_SCENE_BUILD": { label: "3D 重建", cls: "bg-emerald-500/15 text-emerald-400" },
  "3DGS_RENDER_IMAGE": { label: "3D 渲染", cls: "bg-emerald-500/15 text-emerald-400" },
  "3DGS_RENDER_PANORAMA": { label: "3D 全景", cls: "bg-emerald-500/15 text-emerald-400" },
  FINAL_VIDEO_COMPOSE: { label: "成片合成", cls: "bg-violet-500/15 text-violet-400" },
  ENGINEER_FILE_EXPORT: { label: "工程导出", cls: "bg-gray-500/15 text-gray-400" },
};

// 类型下拉（服务端单值过滤；覆盖主要类型）
const TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "全部类型" },
  { value: "SCRIPT_SPLIT", label: "剧本拆分" },
  { value: "CHAR_IMAGE_GEN", label: "角色图片" },
  { value: "SCENE_IMAGE_GEN", label: "场景图片" },
  { value: "PROP_IMAGE_GEN", label: "道具图片" },
  { value: "VIDEO_SEGMENT_GEN", label: "视频生成" },
  { value: "VIDEO_GROUP_CONCAT", label: "视频合成" },
  { value: "FINAL_VIDEO_COMPOSE", label: "成片合成" },
  { value: "ENGINEER_FILE_EXPORT", label: "工程导出" },
];

// 设计稿静态回退行（接口失败时展示，示例数据不可操作）
const FALLBACK_ROWS: TaskRow[] = [
  { task_uuid: "t1", task_id: "task_8a3f2d1c", typeLabel: "视频生成", typeCls: "bg-purple-500/15 text-purple-400", project: "霸总追妻：重生后我成了千亿继承人", author: "林小雨", authorInitial: "林", authorGrad: "from-pink-500 to-purple-500", progress: 67, status: "RUNNING", gpu: "GPU-03", cost: "4分32秒", createdAt: "2026-08-19 10:20", real: false },
  { task_uuid: "t2", task_id: "task_7c1e4a9b", typeLabel: "剧本拆分", typeCls: "bg-cyan-500/15 text-cyan-400", project: "穿越古代：我带系统当皇帝", author: "墨尘居士", authorInitial: "墨", authorGrad: "from-blue-500 to-cyan-500", progress: 42, status: "RUNNING", gpu: "GPU-09", cost: "1分28秒", createdAt: "2026-08-19 09:55", real: false },
  { task_uuid: "t3", task_id: "task_9d4b7c2e", typeLabel: "视频生成", typeCls: "bg-purple-500/15 text-purple-400", project: "替嫁新娘：冷面总裁的契约妻", author: "苏暖暖", authorInitial: "苏", authorGrad: "from-violet-500 to-purple-500", progress: 100, status: "SUCCESS", gpu: "GPU-02", cost: "6分15秒", createdAt: "2026-08-19 09:30", real: false },
  { task_uuid: "t4", task_id: "task_2f5a8d3c", typeLabel: "场景图片", typeCls: "bg-teal-500/15 text-teal-400", project: "迷雾追踪：连环杀人案调查实录", author: "陈推理", authorInitial: "陈", authorGrad: "from-slate-500 to-slate-700", progress: 100, status: "FAILED_DEAD", gpu: "GPU-07", cost: "0分45秒", createdAt: "2026-08-19 09:10", real: false },
  { task_uuid: "t5", task_id: "task_6e7c1b4a", typeLabel: "视频生成", typeCls: "bg-purple-500/15 text-purple-400", project: "废柴逆袭：我靠系统打脸全场", author: "爽文达人", authorInitial: "爽", authorGrad: "from-orange-500 to-red-500", progress: 0, status: "QUEUED", gpu: "", cost: "预计 3分20秒", createdAt: "2026-08-19 08:45", real: false },
  { task_uuid: "t6", task_id: "task_3b9d6f8e", typeLabel: "剧本拆分", typeCls: "bg-cyan-500/15 text-cyan-400", project: "职场囧途：我的奇葩同事们", author: "笑料百出", authorInitial: "笑", authorGrad: "from-green-500 to-emerald-500", progress: 88, status: "RUNNING", gpu: "GPU-16", cost: "2分10秒", createdAt: "2026-08-19 08:20", real: false },
];

// 封面/头像渐变池
const AVATAR_GRADS = [
  "from-pink-500 to-purple-500",
  "from-blue-500 to-cyan-500",
  "from-violet-500 to-purple-500",
  "from-slate-500 to-slate-700",
  "from-orange-500 to-red-500",
  "from-green-500 to-emerald-500",
];

// GPU 节点网格（设计稿静态 32 卡：正常/高负载/异常/离线/空闲）
const GPU_NODES: { name: string; model: string; pct: string; cls: string; nameCls: string; pctCls: string }[] = [
  { name: "GPU-01", model: "A100", pct: "68%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-02", model: "A100", pct: "82%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-03", model: "A100", pct: "95%", cls: "bg-amber-gold/10 border-amber-gold/30 hover:bg-amber-gold/20", nameCls: "text-amber-gold", pctCls: "text-white" },
  { name: "GPU-04", model: "A100", pct: "55%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-05", model: "A100", pct: "77%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-06", model: "A100", pct: "61%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-07", model: "A100", pct: "ERR", cls: "bg-red-500/10 border-red-500/30 hover:bg-red-500/20", nameCls: "text-red-400", pctCls: "text-white" },
  { name: "GPU-08", model: "A100", pct: "89%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-09", model: "H100", pct: "92%", cls: "bg-amber-gold/10 border-amber-gold/30 hover:bg-amber-gold/20", nameCls: "text-amber-gold", pctCls: "text-white" },
  { name: "GPU-10", model: "H100", pct: "71%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-11", model: "H100", pct: "63%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-12", model: "H100", pct: "78%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-13", model: "H100", pct: "OFF", cls: "bg-gray-500/10 border-gray-500/30 hover:bg-gray-500/20", nameCls: "text-gray-400", pctCls: "text-white/50" },
  { name: "GPU-14", model: "H100", pct: "59%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-15", model: "H100", pct: "85%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-16", model: "H100", pct: "97%", cls: "bg-amber-gold/10 border-amber-gold/30 hover:bg-amber-gold/20", nameCls: "text-amber-gold", pctCls: "text-white" },
  { name: "GPU-17", model: "A100", pct: "45%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-18", model: "A100", pct: "72%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-19", model: "A100", pct: "66%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-20", model: "A100", pct: "81%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-21", model: "A100", pct: "53%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-22", model: "A100", pct: "74%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-23", model: "A100", pct: "69%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-24", model: "A100", pct: "88%", cls: "bg-green-500/10 border-green-500/30 hover:bg-green-500/20", nameCls: "text-green-400", pctCls: "text-white" },
  { name: "GPU-25", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-26", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-27", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-28", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-29", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-30", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-31", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
  { name: "GPU-32", model: "空闲", pct: "0%", cls: "bg-[#0B0B1A] border-[#2A2A3E] hover:border-[#3A3A4E]", nameCls: "text-text-muted", pctCls: "text-text-muted" },
];

function fmtExec(ms: number): string {
  if (ms <= 0) return "—";
  const sec = Math.round(ms / 1000);
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return m > 0 ? `${m}分${String(s).padStart(2, "0")}秒` : `${s}秒`;
}

function fmtAvgMin(ms: number): string {
  if (ms <= 0) return "0.0";
  return (ms / 60000).toFixed(1);
}

export default function AdminTasksPage() {
  const [tab, setTab] = useState<TaskTab>("all");
  const [taskType, setTaskType] = useState("");
  const [rows, setRows] = useState<TaskRow[]>(FALLBACK_ROWS);
  const [isReal, setIsReal] = useState(false);
  const [total, setTotal] = useState(335);
  const [page, setPage] = useState(1);
  const [stats, setStats] = useState<TaskStatsResp | null>(null);

  const load = useCallback(async (p: number, t: TaskTab, tt: string) => {
    try {
      const status = TAB_STATUS[t];
      const resp = await api.get<TasksResp>(
        `/admin/tasks?status=${status}&task_type=${tt}&page=${p}&page_size=${PAGE_SIZE}`,
      );
      const mapped: TaskRow[] = resp.items.map((w, i) => {
        const tm = TYPE_META[w.task_type] ?? { label: w.task_type, cls: "bg-gray-500/15 text-gray-400" };
        return {
          task_uuid: w.task_uuid,
          task_id: w.request_id || w.task_uuid.slice(0, 8),
          typeLabel: tm.label,
          typeCls: tm.cls,
          project: w.project_title || "—",
          author: w.author || "—",
          authorInitial: w.author ? w.author.charAt(0) : "?",
          authorGrad: AVATAR_GRADS[i % AVATAR_GRADS.length],
          progress: w.progress_pct || 0,
          status: w.status,
          gpu: w.gpu_node_id || "—",
          cost: w.status === "SUCCESS" ? fmtExec(w.execution_ms) : w.status === "QUEUED" || w.status === "PENDING" ? "等待分配" : "—",
          createdAt: w.created_at,
          real: true,
        };
      });
      setRows(mapped);
      setTotal(resp.pagination.total);
      setIsReal(true);
    } catch {
      setRows(FALLBACK_ROWS);
      setTotal(335);
      setIsReal(false);
    }
  }, []);

  const loadStats = useCallback(async () => {
    try {
      const s = await api.get<TaskStatsResp>("/admin/tasks/stats");
      setStats(s);
    } catch {
      /* 回退设计稿静态值 */
    }
  }, []);

  useEffect(() => {
    loadStats();
    void load(1, tab, taskType);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab, taskType]);

  const switchTab = (t: TaskTab) => {
    if (t === tab) return;
    setTab(t);
    setPage(1);
  };

  const changeType = (v: string) => {
    setTaskType(v);
    setPage(1);
  };

  const refresh = () => {
    loadStats();
    void load(page, tab, taskType);
    toast("已刷新任务数据", "success");
  };

  // 管理端操作：重试/取消（失败 toast 占位：查看/置顶/下载）
  const retryTask = async (row: TaskRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.post(`/admin/tasks/${row.task_uuid}/retry`);
      toast(`已重新入队 ${row.task_id}`, "success");
      void load(page, tab, taskType);
      loadStats();
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  const cancelTask = async (row: TaskRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    try {
      await api.post(`/admin/tasks/${row.task_uuid}/cancel`);
      toast(`已取消 ${row.task_id}`, "success");
      void load(page, tab, taskType);
      loadStats();
    } catch (e) {
      toast(errMessage(e), "error");
    }
  };

  // 任务详情弹窗（GET /admin/tasks/:task_id）
  const [detail, setDetail] = useState<TaskDetailResp | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const openDetail = async (row: TaskRow) => {
    if (!row.real) {
      toast("示例数据不可操作", "error");
      return;
    }
    setDetailLoading(true);
    try {
      const d = await api.get<TaskDetailResp>(`/admin/tasks/${row.task_uuid}`);
      setDetail(d);
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setDetailLoading(false);
    }
  };

  // 优先级调整（PUT /admin/tasks/:task_id/priority，仅排队中）
  const [prioTarget, setPrioTarget] = useState<TaskRow | null>(null);
  const [prioValue, setPrioValue] = useState(5);
  const [savingPrio, setSavingPrio] = useState(false);

  const savePriority = async () => {
    if (!prioTarget) return;
    setSavingPrio(true);
    try {
      await api.put(`/admin/tasks/${prioTarget.task_uuid}/priority`, { priority: prioValue });
      setPrioTarget(null);
      toast(`已调整 ${prioTarget.task_id} 优先级为 ${prioValue}`, "success");
    } catch (e) {
      toast(errMessage(e), "error");
    } finally {
      setSavingPrio(false);
    }
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const pageBtns = useMemo(() => {
    const arr: (number | "...")[] = [];
    if (totalPages <= 7) {
      for (let i = 1; i <= totalPages; i++) arr.push(i);
    } else {
      for (let i = 1; i <= 5; i++) arr.push(i);
      arr.push("...");
      arr.push(totalPages);
    }
    return arr;
  }, [totalPages]);

  const gotoPage = (p: number) => {
    if (p < 1 || p > totalPages || !isReal) return;
    setPage(p);
  };

  // 进度列渲染
  const progressCell = (r: TaskRow) => {
    if (r.status === "SUCCESS") {
      return (
        <div className="flex items-center gap-2">
          <div className="flex-1 progress-bar">
            <div className="progress-fill bg-green-500" style={{ width: "100%" }} />
          </div>
          <span className="text-xs text-green-400 font-medium w-10 text-right">完成</span>
        </div>
      );
    }
    if (r.status === "FAILED_DEAD" || r.status === "FAILED_RETRYING") {
      return (
        <div className="flex items-center gap-2">
          <div className="flex-1 progress-bar">
            <div className="progress-fill bg-red-500" style={{ width: "100%" }} />
          </div>
          <span className="text-xs text-red-400 font-medium w-10 text-right">失败</span>
        </div>
      );
    }
    if (r.status === "RUNNING") {
      const pct = Math.max(5, Math.min(95, r.progress || 50));
      return (
        <div className="flex items-center gap-2">
          <div className="flex-1 progress-bar">
            <div className="progress-fill progress-running bg-gradient-to-r from-amber-gold to-yellow-400" style={{ width: `${pct}%` }} />
          </div>
          <span className="text-xs text-white font-medium w-10 text-right">{r.progress || "进行中"}</span>
        </div>
      );
    }
    return (
      <div className="flex items-center gap-2">
        <div className="flex-1 progress-bar">
          <div className="progress-fill bg-gray-500" style={{ width: "0%" }} />
        </div>
        <span className="text-xs text-text-muted font-medium w-10 text-right">排队</span>
      </div>
    );
  };

  // 操作列渲染（按状态）
  const actionCell = (r: TaskRow) => {
    const running = r.status === "RUNNING";
    const queued = r.status === "QUEUED" || r.status === "PENDING";
    const done = r.status === "SUCCESS";
    const failed = r.status === "FAILED_DEAD" || r.status === "FAILED_RETRYING";
    return (
      <div className="flex justify-end gap-1">
        <button className="action-btn text-text-secondary" title="查看详情" onClick={() => openDetail(r)}>
          <Eye className="w-3.5 h-3.5" />
        </button>
        {running && (
          <button className="action-btn text-red-400" title="停止任务" onClick={() => cancelTask(r)}>
            <Square className="w-3.5 h-3.5" />
          </button>
        )}
        {queued && (
          <>
            <button
              className="action-btn text-amber-gold"
              title="调整优先级"
              onClick={() => {
                setPrioTarget(r);
                setPrioValue(5);
              }}
            >
              <ArrowUp className="w-3.5 h-3.5" />
            </button>
            <button className="action-btn text-red-400" title="取消任务" onClick={() => cancelTask(r)}>
              <X className="w-3.5 h-3.5" />
            </button>
          </>
        )}
        {done && (
          <button className="action-btn text-amber-gold" title="下载结果" onClick={() => openDetail(r)}>
            <Download className="w-3.5 h-3.5" />
          </button>
        )}
        {failed && (
          <button className="action-btn text-amber-gold" title="重试" onClick={() => retryTask(r)}>
            <RotateCw className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    );
  };

  const statCard1 = (
    <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
      <div className="flex items-center justify-between mb-4">
        <span className="text-text-muted text-sm">GPU 集群利用率</span>
        <div className="w-9 h-9 rounded-lg bg-green-500/10 flex items-center justify-center">
          <Server className="w-4 h-4 text-green-400" />
        </div>
      </div>
      <div className="flex items-end gap-2 mb-2">
        <span className="text-3xl font-bold text-white">73%</span>
        <span className="text-xs text-green-400 mb-1">健康</span>
      </div>
      <div className="gpu-bar">
        <div className="gpu-fill bg-gradient-to-r from-green-500 to-emerald-400" style={{ width: "73%" }} />
      </div>
      <div className="flex justify-between mt-2 text-xs text-text-muted">
        <span>24 / 32 卡运行中</span>
        <span>8 卡空闲</span>
      </div>
    </div>
  );

  const queuedCount = stats?.queued ?? 34;
  const runningCount = stats?.running ?? 12;
  const doneCount = stats?.done_today ?? 286;
  const failedCount = stats?.failed ?? 3;

  const statCard2 = (
    <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
      <div className="flex items-center justify-between mb-4">
        <span className="text-text-muted text-sm">任务队列</span>
        <div className="w-9 h-9 rounded-lg bg-amber-gold/10 flex items-center justify-center">
          <ListTodo className="w-4 h-4 text-amber-gold" />
        </div>
      </div>
      <div className="flex items-end gap-2 mb-2">
        <span className="text-3xl font-bold text-white">{fmtNum(queuedCount)}</span>
        <span className="text-xs text-amber-gold mb-1">排队中</span>
      </div>
      <div className="grid grid-cols-4 gap-2 text-center">
        <div className="bg-[#0B0B1A] rounded-lg py-2">
          <div className="text-lg font-semibold text-amber-gold">{fmtNum(queuedCount)}</div>
          <div className="text-[10px] text-text-muted">排队</div>
        </div>
        <div className="bg-[#0B0B1A] rounded-lg py-2">
          <div className="text-lg font-semibold text-blue-400">{fmtNum(runningCount)}</div>
          <div className="text-[10px] text-text-muted">生成中</div>
        </div>
        <div className="bg-[#0B0B1A] rounded-lg py-2">
          <div className="text-lg font-semibold text-green-400">{fmtNum(doneCount)}</div>
          <div className="text-[10px] text-text-muted">今日完成</div>
        </div>
        <div className="bg-[#0B0B1A] rounded-lg py-2">
          <div className="text-lg font-semibold text-red-400">{fmtNum(failedCount)}</div>
          <div className="text-[10px] text-text-muted">失败</div>
        </div>
      </div>
    </div>
  );

  const statCard3 = (
    <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5">
      <div className="flex items-center justify-between mb-4">
        <span className="text-text-muted text-sm">平均处理时长</span>
        <div className="w-9 h-9 rounded-lg bg-blue-500/10 flex items-center justify-center">
          <Clock className="w-4 h-4 text-blue-400" />
        </div>
      </div>
      <div className="flex items-end gap-2 mb-2">
        <span className="text-3xl font-bold text-white">
          {fmtAvgMin(stats?.avg_execution_ms ?? 8500)}
          <span className="text-lg">分</span>
        </span>
        <span className="text-xs text-green-400 mb-1">-12% ↓</span>
      </div>
      <div className="space-y-2 mt-3">
        <div className="flex items-center justify-between text-xs">
          <span className="text-text-muted">视频生成</span>
          <span className="text-white">12.3 分钟</span>
        </div>
        <div className="flex items-center justify-between text-xs">
          <span className="text-text-muted">AI 配音</span>
          <span className="text-white">3.2 分钟</span>
        </div>
        <div className="flex items-center justify-between text-xs">
          <span className="text-text-muted">封面生成</span>
          <span className="text-white">1.5 分钟</span>
        </div>
      </div>
    </div>
  );

  return (
    <>
      {/* Page title */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white mb-1">算力任务</h1>
          <p className="text-sm text-text-muted">监控 AI 生成任务队列与 GPU 集群资源使用情况</p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={refresh}
            className="px-4 py-2 bg-bg-card border border-[#2A2A3E] rounded-lg text-sm text-text-secondary hover:border-[#3A3A4E] transition-colors flex items-center gap-2"
          >
            <RefreshCw className="w-4 h-4" />
            刷新
          </button>
          <button
            onClick={() => toast("算力调度功能规划中", "info")}
            className="px-4 py-2 bg-amber-gold text-[#0B0B1A] rounded-lg text-sm font-semibold hover:bg-amber-400 transition-colors flex items-center gap-2"
          >
            <Settings2 className="w-4 h-4" />
            算力调度
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        {statCard1}
        {statCard2}
        {statCard3}
      </div>

      {/* GPU Node Grid（设计稿静态：无 GPU 资源后端数据源） */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-5 mb-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-white font-semibold">GPU 节点状态</h3>
          <div className="flex items-center gap-3 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-green-400" />
              <span className="text-text-muted">正常</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-amber-gold" />
              <span className="text-text-muted">高负载</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-red-400" />
              <span className="text-text-muted">异常</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-gray-500" />
              <span className="text-text-muted">离线</span>
            </span>
          </div>
        </div>
        <div className="grid grid-cols-8 gap-2">
          {GPU_NODES.map((n) => (
            <div key={n.name} className={`${n.cls} rounded-lg p-2 text-center cursor-pointer transition-colors border`}>
              <div className={`text-[10px] ${n.nameCls} font-medium`}>{n.name}</div>
              <div className={`text-xs font-bold mt-0.5 ${n.pctCls}`}>{n.pct}</div>
              <div className="text-[9px] text-text-muted">{n.model}</div>
            </div>
          ))}
        </div>
      </div>

      {/* Task List */}
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl">
        <div className="flex items-center justify-between px-5 py-4 border-b border-[#2A2A3E]">
          <div className="flex items-center gap-4">
            {(Object.keys(TAB_LABEL) as TaskTab[]).map((t) => (
              <button
                key={t}
                className={`text-sm pb-1 transition-colors ${
                  tab === t
                    ? "text-amber-gold font-medium border-b-2 border-amber-gold"
                    : "text-text-secondary hover:text-white"
                }`}
                onClick={() => switchTab(t)}
              >
                {TAB_LABEL[t]}
                {t === "queued" && ` (${fmtNum(queuedCount)})`}
                {t === "running" && ` (${fmtNum(runningCount)})`}
                {t === "failed" && ` (${fmtNum(failedCount)})`}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <select
              value={taskType}
              onChange={(e) => changeType(e.target.value)}
              className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none text-xs"
            >
              {TYPE_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-[#2A2A3E]">
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">任务ID</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">任务类型</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">所属作品</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">用户</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">进度</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">状态</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">GPU节点</th>
                <th className="text-left px-4 py-3 text-text-muted text-xs font-medium uppercase">耗时</th>
                <th className="text-right px-4 py-3 text-text-muted text-xs font-medium uppercase">操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-sm text-text-muted">
                    暂无符合条件的任务
                  </td>
                </tr>
              ) : (
                rows.map((r) => {
                  const meta = STATUS_META[r.status] ?? { label: r.status, cls: "bg-gray-500/15 text-gray-400", dot: "bg-gray-400" };
                  return (
                    <tr key={r.task_uuid} className="border-b border-[#2A2A3E]/50 table-row">
                      <td className="px-4 py-3 text-sm font-mono text-white">{r.task_id}</td>
                      <td className="px-4 py-3">
                        <span className={`tag ${r.typeCls}`}>{r.typeLabel}</span>
                      </td>
                      <td className="px-4 py-3 text-sm text-white truncate max-w-[180px]">{r.project}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div
                            className={`w-6 h-6 rounded-full bg-gradient-to-br ${r.authorGrad} flex items-center justify-center text-white text-[10px] font-bold`}
                          >
                            {r.authorInitial}
                          </div>
                          <span className="text-sm text-text-secondary">{r.author}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3 w-36">{progressCell(r)}</td>
                      <td className="px-4 py-3">
                        <span className={`tag ${meta.cls}`}>
                          <span className={`status-dot ${meta.dot}`} />
                          {meta.label}
                        </span>
                      </td>
                      <td className={`px-4 py-3 text-sm font-mono ${r.status === "FAILED_DEAD" ? "text-red-400" : "text-text-secondary"}`}>
                        {r.gpu}
                      </td>
                      <td className="px-4 py-3 text-sm text-text-secondary">{r.cost}</td>
                      <td className="px-4 py-3 text-right">{actionCell(r)}</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-between px-4 py-3 border-t border-[#2A2A3E]">
          <div className="text-sm text-text-muted">
            共 <span className="text-white font-medium">{fmtNum(total)}</span> 条任务记录
          </div>
          <div className="flex items-center gap-1">
            <button
              className="w-8 h-8 rounded-lg border border-[#2A2A3E] flex items-center justify-center text-text-muted hover:border-[#3A3A4E] disabled:opacity-40"
              disabled={page <= 1 || !isReal}
              onClick={() => gotoPage(page - 1)}
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            {pageBtns.map((n, idx) =>
              n === "..." ? (
                <span key={`dot-${idx}`} className="text-text-muted px-1 text-sm">
                  ...
                </span>
              ) : (
                <button
                  key={n}
                  className={`w-8 h-8 rounded-lg border text-sm ${
                    n === page
                      ? "bg-amber-gold text-[#0B0B1A] font-medium border-amber-gold"
                      : "border-[#2A2A3E] text-text-secondary hover:border-[#3A3A4E]"
                  }`}
                  onClick={() => gotoPage(n)}
                >
                  {n}
                </button>
              ),
            )}
            <button
              className="w-8 h-8 rounded-lg border border-[#2A2A3E] flex items-center justify-center text-text-secondary hover:border-[#3A3A4E] disabled:opacity-40"
              disabled={page >= totalPages || !isReal}
              onClick={() => gotoPage(page + 1)}
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      {/* 任务详情弹窗（GET /admin/tasks/:task_id） */}
      <Modal
        title="任务详情"
        subtitle={detail ? `ID: ${detail.request_id || detail.task_uuid.slice(0, 8)}` : ""}
        open={Boolean(detail)}
        onClose={() => setDetail(null)}
        width={560}
        footer={
          <button
            className="px-4 py-2 rounded-lg text-sm text-text-secondary hover:text-white transition-colors"
            onClick={() => setDetail(null)}
          >
            关闭
          </button>
        }
      >
        {detailLoading || !detail ? (
          <div className="py-8 text-center text-sm text-text-muted">加载中…</div>
        ) : (
          <div className="space-y-3 text-sm">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-xs text-text-muted mb-0.5">状态</div>
                <span className={`tag ${(STATUS_META[detail.status] ?? STATUS_META.QUEUED).cls}`}>
                  {(STATUS_META[detail.status] ?? STATUS_META.QUEUED).label}
                </span>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">任务类型</div>
                <div className="text-white">{(TYPE_META[detail.task_type] ?? { label: detail.task_type }).label}</div>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">进度</div>
                <div className="text-white">{detail.progress_pct}%</div>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">优先级</div>
                <div className="text-white">{detail.priority}</div>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">重试次数</div>
                <div className="text-white">{detail.retry_count}</div>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">GPU 节点</div>
                <div className="text-white font-mono">{detail.gpu_node_id || "—"}</div>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">外部任务 ID</div>
                <div className="text-white font-mono text-xs">{detail.external_task_id || "—"}</div>
              </div>
              <div>
                <div className="text-xs text-text-muted mb-0.5">实际模型</div>
                <div className="text-white">{detail.ai_model_actual_used || "—"}</div>
              </div>
            </div>
            <div>
              <div className="text-xs text-text-muted mb-0.5">时间线</div>
              <div className="text-text-secondary text-xs space-y-0.5">
                <div>创建：{detail.created_at || "—"}</div>
                <div>入队：{detail.enqueued_at || "—"}</div>
                <div>开始：{detail.started_at || "—"}</div>
                <div>完成：{detail.completed_at || "—"}</div>
              </div>
            </div>
            {detail.user_display_error && (
              <div className="bg-red-500/10 border border-red-500/30 rounded-lg px-3 py-2 text-xs text-red-400">
                {detail.user_display_error}
              </div>
            )}
            {Boolean(detail.result_output && typeof detail.result_output === "object" && Object.keys(detail.result_output as object).length > 0) && (
              <div>
                <div className="text-xs text-text-muted mb-0.5">生成结果</div>
                <pre className="bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-xs text-text-secondary overflow-x-auto max-h-40">
                  {JSON.stringify(detail.result_output, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </Modal>

      {/* 优先级调整弹窗（PUT /admin/tasks/:task_id/priority） */}
      <Modal
        title="调整任务优先级"
        subtitle={prioTarget ? `${prioTarget.task_id}（当前排队中）` : ""}
        open={Boolean(prioTarget)}
        onClose={() => setPrioTarget(null)}
        footer={
          <>
            <button
              className="px-4 py-2 rounded-lg text-sm text-text-secondary hover:text-white transition-colors"
              onClick={() => setPrioTarget(null)}
            >
              取消
            </button>
            <button
              className="px-4 py-2 rounded-lg text-sm font-semibold bg-gradient-to-r from-amber-gold to-amber-dim text-[#0B0B1A] hover:shadow-lg hover:shadow-amber-gold/20 transition-all disabled:opacity-50"
              disabled={savingPrio}
              onClick={() => void savePriority()}
            >
              {savingPrio ? "保存中…" : "确认调整"}
            </button>
          </>
        }
      >
        <div>
          <label className="block text-sm text-text-secondary mb-1.5">优先级（1-100，数值越大越先执行）</label>
          <input
            type="number"
            min={1}
            max={100}
            value={prioValue}
            onChange={(e) => setPrioValue(Number(e.target.value) || 1)}
            className="w-full bg-[#0B0B1A] border border-[#2A2A3E] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-amber-gold/50"
          />
        </div>
      </Modal>
    </>
  );
}
