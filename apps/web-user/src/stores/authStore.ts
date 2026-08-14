// authStore：登录态本地状态（Zustand，只存交互态，远程数据在 Query Cache——06 1.1 真源规则）。
import { create } from "zustand";
import { persist } from "zustand/middleware";

interface UserBrief {
  user_uuid: string;
  email: string;
  nickname: string;
  avatar_url: string;
  vip_level: number;
  default_space_id: number;
}

interface AuthState {
  accessToken: string | null;
  user: UserBrief | null;
  setAuth: (token: string, user: UserBrief) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: null,
      user: null,
      setAuth: (token, user) => set({ accessToken: token, user }),
      clearAuth: () => set({ accessToken: null, user: null }),
    }),
    { name: "duanju-auth" },
  ),
);
