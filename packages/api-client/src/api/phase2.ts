// 阶段 2 API：空间协作（成员/角色/邀请/团队积分）+ 画布 + 会员（S2-F01~F05）。
import { api, type Paginated } from "../index";

// ---- 空间协作（S2-F01）----
export interface MemberItem {
  user_id: number;
  user_uuid: string;
  nickname: string;
  role: string;
  joined_at: string;
}

export interface TeamAccount {
  space_id: number;
  current_balance: number;
  frozen_balance: number;
}

export const spaceApi = {
  // 创建团队空间（2.2）
  createTeam: (name: string, description?: string) =>
    api.post<{ space_id: string; name: string; type: string; my_role: string }>("/spaces", { name, description }),
  // 2.6 成员列表
  members: (spaceId: string) => api.get<{ items: MemberItem[]; my_role: string }>(`/spaces/${spaceId}/members`),
  // 2.7 添加成员
  addMember: (spaceId: string, userId: string, role = "MEMBER") =>
    api.post<null>(`/spaces/${spaceId}/members`, { user_id: userId, role }),
  // 2.9 改角色
  updateRole: (spaceId: string, userId: string, role: string) =>
    api.patch<null>(`/spaces/${spaceId}/members/${userId}`, { role }),
  // 2.10 移除成员
  removeMember: (spaceId: string, userId: string) => api.delete<null>(`/spaces/${spaceId}/members/${userId}`),
  // 2.11 转让所有权
  transfer: (spaceId: string, userId: string) => api.post<null>(`/spaces/${spaceId}/transfer/${userId}`),
  // 2.8 邀请链接
  invite: (spaceId: string, account: string, role = "MEMBER") =>
    api.post<{ invite_token: string; invite_url: string; expires_in_days: number }>(
      `/spaces/${spaceId}/invitations`,
      { account, role },
    ),
  // 2.8a 接受邀请
  acceptInvite: (token: string) => api.post<null>(`/spaces/invitations/${token}/accept`),
  // 9.3 团队账户
  teamBilling: (spaceId: string) => api.get<TeamAccount>(`/spaces/${spaceId}/billing`),
  // 9.4 团队流水
  teamTxns: (spaceId: string) =>
    api.get<Paginated<Record<string, unknown>>>(`/spaces/${spaceId}/billing/transactions`),
  // 成员消费报表
  teamReport: (spaceId: string) => api.get<{ items: Record<string, unknown>[] }>(`/spaces/${spaceId}/billing/report`),
  // 2.14 团队充值
  teamRecharge: (spaceId: string, pkg: { package_code: string; amount_yuan: string; points: number }) =>
    api.post<{ order_no: string; amount_yuan: string; points: number }>(`/spaces/${spaceId}/billing/recharge`, pkg),
  // 开发期入账
  teamRechargePay: (orderNo: string) => api.post<null>(`/spaces/billing/orders/${orderNo}/pay`),
};

// ---- 画布（S2-F02/F03）----
export interface CanvasNode {
  id: string;
  node_type: "INPUT" | "SCRIPT_SPLIT" | "ASSET_GEN" | "VIDEO_GEN" | "EXPORT" | "OUTPUT" | "LLM";
  position_x: number;
  position_y: number;
  config?: Record<string, unknown>;
}

export interface CanvasEdge {
  id: string;
  source: string;
  target: string;
}

export const canvasApi = {
  // 4.1 创建画布
  create: (name: string) => api.post<{ canvas_id: string }>("/canvases", { name }),
  // 4.1 加载
  load: (canvasId: string) => api.get<Record<string, unknown>>(`/canvases/${canvasId}`),
  // 4.2 保存（乐观锁版本）
  save: (canvasId: string, data: unknown, version: number) =>
    api.put<null>(`/canvases/${canvasId}`, { data, version }),
  // 4.2 节点保存
  saveNodes: (canvasId: string, nodes: CanvasNode[]) => api.put<null>(`/canvases/${canvasId}/nodes`, { nodes }),
  // 4.4 执行工作流
  execute: (canvasId: string, nodes: CanvasNode[], edges: CanvasEdge[]) =>
    api.post<{ results: Record<string, unknown>[] }>(`/canvases/${canvasId}/execute`, { nodes, edges }),
};

// ---- 会员（S2-F04）----
export const membershipApi = {
  // 9.9 购买
  purchase: (planCode: "PRO" | "ENTERPRISE" | "ULTIMATE", billingCycle: "MONTHLY" | "ANNUAL") =>
    api.post<{ order_no: string; amount_yuan: string; plan_code: string; billing_cycle: string }>(
      "/memberships/purchase",
      { plan_code: planCode, billing_cycle: billingCycle },
    ),
  // 9.9 查询
  detail: () => api.get<{ plan_code: string; status: string; renew_strategy: string; expires_at?: string }>("/memberships"),
  // 9.10 取消
  cancel: () => api.post<null>("/memberships/cancel"),
  // 开发期激活
  activate: (orderNo: string) => api.post<null>(`/memberships/orders/${orderNo}/activate`),
};

// ---- 多变体（S2-B04）----
export const variantApi = {
  list: (characterId: number) => api.get<{ items: Record<string, unknown>[] }>(`/characters/${characterId}/variants`),
  create: (characterId: number, variantName: string, variantType?: string) =>
    api.post<{ variant_id: number; status: string }>(`/characters/${characterId}/variants`, {
      variant_name: variantName,
      variant_type: variantType,
    }),
};

// ---- 版本回滚（S2-B05）----
export interface NuVersionItem {
  version_id: number;
  version_num: number;
  label: string;
  created_from: string;
  created_at: string;
}

export const versionApi = {
  list: (nuId: string) => api.get<{ items: NuVersionItem[] }>(`/narrative-units/${nuId}/versions`),
  rollback: (nuId: string, versionId: string | number) => api.post<null>(`/narrative-units/${nuId}/rollback/${versionId}`),
};

// ---- 预览消耗（S2-B06）----
export const previewApi = {
  preview: (projectId: string, items: { kind: string; target_id: number }[]) =>
    api.post<{ total_items: number; total_cost: number; exempt_count: number; exempt_amount: number; actual_cost: number }>(
      "/points/preview",
      { project_id: projectId, items },
    ),
};