import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 管理端（B05 骨架）：5174 端口，/api 代理到后端 8080
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5174,
    proxy: {
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: true },
      "/ws": { target: "ws://127.0.0.1:8080", ws: true },
    },
  },
});