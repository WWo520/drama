// 全局风格选择 store：首页选定的风格同步到资产生成等所有后续页面（localStorage 持久化 + 事件广播）。
// 首页 HomePage 选风格 → setGlobalStyle；资产弹窗 AssetDetailDialog / Step2 批量生成 → getGlobalStyle 初始值。

const KEY = "duanju.global_style";
export const STYLE_CHANGED_EVENT = "duanju:style-changed";

export interface GlobalStyle {
  code: string;
  name: string;
}

export function getGlobalStyle(): GlobalStyle {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as GlobalStyle;
      if (parsed?.name) return parsed;
    }
  } catch {
    /* 忽略损坏数据 */
  }
  return { code: "", name: "通用写实" };
}

export function setGlobalStyle(style: GlobalStyle) {
  try {
    localStorage.setItem(KEY, JSON.stringify(style));
  } catch {
    /* 存储不可用时静默 */
  }
  window.dispatchEvent(new CustomEvent(STYLE_CHANGED_EVENT, { detail: style }));
}
