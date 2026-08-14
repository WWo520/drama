import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 对齐 06 前端方案：Vite 5 SPA，开发代理到后端 8080（05 B1 无 BFF，浏览器直连）
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/healthz": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      // WS 代理（S1-F08）：/ws/v1 升级转发到后端
      "/ws": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          react: ["react", "react-dom", "react-router-dom"],
          query: ["@tanstack/react-query"],
        },
      },
    },
  },
});
