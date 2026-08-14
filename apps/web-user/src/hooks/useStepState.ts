// useStepState：Step 状态封装（03 A.5 规则）——后端 status × 路由 step 双向同步 + 可点击规则。
import { useEffect, useMemo } from "react";
import { useNavigate, useParams } from "react-router-dom";

import type { ProjectStatusValue } from "@duanju/api-client";

// 状态 → Step 映射（03 A.5）
const STATUS_TO_STEP: Record<string, number> = {
  DRAFT: 1,
  SCRIPT_SPLITTING: 1,
  EPISODES_READY: 2,
  ASSETS_GENERATING: 2,
  ASSETS_PARTIAL_DONE: 2,
  ASSETS_DONE: 2,
  ASSETS_FAILED: 2,
  STORYBOARD_GENERATING: 3,
  VIDEOS_PARTIAL_DONE: 3,
  ALL_VIDEOS_READY: 3,
  UNITS_DONE: 4,
  PROJECT_FINISHED: 4,
  PUBLISHED: 4,
  ARCHIVED: 4,
};

// 可点击规则（Step n 需要后端达到的状态）
const STEP_UNLOCK: Record<number, string[]> = {
  1: ["*"], // Step1 永远可回
  2: ["EPISODES_READY", "ASSETS_GENERATING", "ASSETS_PARTIAL_DONE", "ASSETS_DONE", "ASSETS_FAILED",
      "STORYBOARD_GENERATING", "VIDEOS_PARTIAL_DONE", "ALL_VIDEOS_READY", "UNITS_DONE", "PROJECT_FINISHED", "PUBLISHED", "ARCHIVED"],
  3: ["ASSETS_DONE", "ASSETS_PARTIAL_DONE", "STORYBOARD_GENERATING", "VIDEOS_PARTIAL_DONE",
      "ALL_VIDEOS_READY", "UNITS_DONE", "PROJECT_FINISHED", "PUBLISHED", "ARCHIVED"],
  4: ["UNITS_DONE", "PROJECT_FINISHED", "PUBLISHED", "ARCHIVED"],
};

export interface StepState {
  /** 后端 status 映射出的 Step */
  backendStep: number;
  /** 路由参数优先的当前 Step */
  currentStep: number;
  /** 各 Step 是否可点击 */
  canClick: (step: number) => boolean;
  /** 切换到某 Step（更新路由，不跳未解锁 Step） */
  goStep: (step: number) => void;
}

export function useStepState(projectId: string, status: ProjectStatusValue | null): StepState {
  const nav = useNavigate();
  const { step } = useParams();

  const backendStep = useMemo(() => (status ? (STATUS_TO_STEP[status] ?? 1) : 1), [status]);
  const routeStep = step ? Number(step) : 0;

  // 路由参数优先（A.5：用户手动点 StepBar 优先），兜底后端映射
  const currentStep = Number.isInteger(routeStep) && routeStep >= 1 && routeStep <= 4 ? routeStep : backendStep;

  const canClick = (s: number) => {
    if (s === 1) return true;
    if (!status) return false;
    return (STEP_UNLOCK[s] ?? []).includes(status);
  };

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

  return { backendStep, currentStep, canClick, goStep };
}