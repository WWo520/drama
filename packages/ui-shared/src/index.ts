// @duanju/ui-shared：跨端共享基础组件占位（自建组件库后续在 06 3.2 基础上扩展）。
export {};

// 示例占位：统一 Button 样式令牌（后续封装 Radix 组件时引用）
export const buttonVariants = {
  primary: "bg-indigo-600 text-white hover:bg-indigo-500",
  secondary: "bg-zinc-800 text-zinc-200 hover:bg-zinc-700",
  ghost: "text-zinc-300 hover:bg-zinc-800",
} as const;

export type ButtonVariant = keyof typeof buttonVariants;
