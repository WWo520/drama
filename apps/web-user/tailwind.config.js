/** @type {import('tailwindcss').Config} */
// 设计系统真源：design-system/duanju/MASTER.md（ui-ux-pro-max 生成，Theater/Cinema 影视配色）
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: [
          "Inter",
          "PingFang SC",
          "Microsoft YaHei",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "sans-serif",
        ],
      },
      colors: {
        // 影视感暗夜基底（Theater/Cinema：dramatic dark + spotlight gold）
        cinema: {
          bg: "#0F0F23", // 夜场暗蓝背景
          card: "#1B1B30", // 卡片
          elevated: "#22223B", // 悬浮层
          muted: "#27273B", // 弱化块
          "muted-fg": "#94A3B8", // 弱化文字
          border: "rgba(255,255,255,0.08)", // 分隔线
          gold: "#CA8A04", // 聚光灯金（CTA/重点）
          "gold-soft": "#EAB308",
        },
        primary: {
          DEFAULT: "#6366F1", // 品牌靛蓝（AI 能力）
          hover: "#818CF8",
          deep: "#4F46E5",
        },
        foreground: "#F8FAFC",
        destructive: "#EF4444",
      },
    },
  },
  plugins: [],
};
