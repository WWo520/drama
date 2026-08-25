// useStepState：Step 状态封装（03 A.5 规则）——后端 status × 路由 step 双向同步 + 可点击规则。
import { useEffect, useMemo } from "react";
import { useNavigate, useParams } from "react-router-dom";

import type { ProjectStatusValue } from "@duanju/api-client";

// 状态 → Step 映射（03 A.5，对齐后端 project_status_enum）
const STATUS_TO_STEP: Record<string, number> = {
  DRAFT: 1,
  SCRIPT_SPLITTING: 1,
  SCRIPT_SPLIT_FAILED: 1,
  PARTIAL_ASSETS: 2, // 拆分成功（旧名 EPISODES_READY）
  ALL_ASSETS_READY: 3,
  VIDEO_GENERATING: 3,
  PARTIAL_VIDEOS: 3,
  ALL_VIDEOS_READY: 4,
  EXPORTING: 4,
  COMPLETED: 4,
  ARCHIVED: 4,
  CANCELLED: 1,
};

// 可点击规则（17 - 工作台对齐，对齐 LuxReal）：四步全阶段平行可达，
// 未就绪由各页面内优雅降级处理（空态引导 / 占位 + CTA），不再做步骤级硬门控。

// 各步骤的完成状态集合（17 - 工作台对齐 P2：步骤条"已完成 ✓ / 进行中 / 未开始"三态）
const STEP_DONE: Record<number, string[]> = {
  1: ["PARTIAL_ASSETS", "ALL_ASSETS_READY", "VIDEO_GENERATING", "PARTIAL_VIDEOS",
      "ALL_VIDEOS_READY", "EXPORTING", "COMPLETED", "ARCHIVED"], // 拆分成功即完成
  2: ["ALL_ASSETS_READY", "VIDEO_GENERATING", "PARTIAL_VIDEOS",
      "ALL_VIDEOS_READY", "EXPORTING", "COMPLETED", "ARCHIVED"], // 资产全部就绪
  3: ["ALL_VIDEOS_READY", "EXPORTING", "COMPLETED", "ARCHIVED"], // 视频全部完成
  4: ["COMPLETED", "ARCHIVED"], // 成片完成
};

export interface StepState {
  /** 后端 status 映射出的 Step */
  backendStep: number;
  /** 路由参数优先的当前 Step */
  currentStep: number;
  /** 各 Step 是否可点击 */
  canClick: (step: number) => boolean;
  /** 各 Step 是否已完成（步骤条 ✓ 态） */
  stepDone: (step: number) => boolean;
  /** 切换到某 Step（更新路由；项目未加载完成时不跳转） */
  goStep: (step: number) => void;
}

export function useStepState(projectId: string, status: ProjectStatusValue | null): StepState {
  const nav = useNavigate();
  const { step } = useParams();

  const backendStep = useMemo(() => (status ? (STATUS_TO_STEP[status] ?? 1) : 1), [status]);
  const routeStep = step ? Number(step) : 0;

  // 路由参数优先（A.5：用户手动点 StepBar 优先），兜底后端映射
  const currentStep = Number.isInteger(routeStep) && routeStep >= 1 && routeStep <= 4 ? routeStep : backendStep;

  // 四步平行可达：Step1 永远可点；其余 Step 只需项目状态加载完成（防加载期闪烁）
  const canClick = (s: number) => {
    if (s === 1) return true;
    return status !== null;
  };

  // 步骤完成判定（步骤条 ✓ 态；未加载完成视为未完成）
  const stepDone = (s: number) => !!status && (STEP_DONE[s] ?? []).includes(status);

  const goStep = (s: number) => {
    if (!canClick(s)) return;
    nav(`/projects/${projectId}/step/${s}`, { replace: true });
  };

  // 后端状态变化时若路由未指定 step 则跟随
  useEffect(() => {
    if (!step && status) {
      nav(`/projects/${projectId}/step/${backendStep}`, { replace: true });
    }
  }, [backendStep, status, projectId, step, nav]);

  return { backendStep, currentStep, canClick, stepDone, goStep };
}