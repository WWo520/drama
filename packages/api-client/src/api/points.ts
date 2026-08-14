// 积分/支付 API：9.3 余额 / 9.5 下单 / 9.8 状态轮询 / 9.4 流水。
import { api } from "../index";

export interface PointsAccount {
  current_balance: number;
  frozen_balance: number;
}

export interface OrderResp {
  order_no: string;
  qr_code_url: string;
  amount_yuan: string;
  expire_at: string;
}

export interface OrderStatus {
  order_no: string;
  pay_status: string; // PENDING / PAID
  points: number;
}

// 积分包 3 档（03 E.6.2 默认；S-01 定值后替换）
export const POINT_PACKAGES = [
  { code: "POINTS_PKG_SMALL", name: "小包", points: 100, amount_yuan: "9.90", desc: "尝鲜体验", badge: null },
  { code: "POINTS_PKG_MEDIUM", name: "中包", points: 1000, amount_yuan: "99.00", desc: "创作主力", badge: "首购 +50% 加赠" },
  { code: "POINTS_PKG_LARGE", name: "大包", points: 3000, amount_yuan: "299.00", desc: "批量生产", badge: "最划算" },
] as const;

export const pointsApi = {
  // 9.3 我的积分账户
  me: () => api.get<PointsAccount>("/points"),
  // 9.5 创建订单（幂等键防重复下单）
  createOrder: (pkg: { package_code: string; amount_yuan: string; points: number }, idempotencyKey: string) =>
    api.post<OrderResp>("/orders", { ...pkg, idempotency_key: idempotencyKey }),
  // 9.8 订单状态轮询
  orderStatus: (orderNo: string) => api.get<OrderStatus>(`/orders/${orderNo}/status`),
};