/** @type {import('tailwindcss').Config} */
// 设计真源：docs/04 - 前端ui设计/luxreal-clone/pages/admin-*.html 的 tailwind.config（深色金主题）
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        "bg-deep": "#0B0B1A",
        "bg-card": "#151525",
        "bg-card-light": "#1A1A2E",
        "border-dark": "#2A2A3E",
        "amber-gold": "#fbbf24",
        "amber-dim": "#d97706",
        "text-primary": "#ffffff",
        "text-secondary": "#9ca3af",
        "text-muted": "#6b7280",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "-apple-system", "sans-serif"],
      },
    },
  },
  plugins: [],
};
