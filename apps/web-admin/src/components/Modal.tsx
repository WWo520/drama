// 通用弹窗：遮罩 + 居中卡片，管理端视觉语言（bg-bg-card / #2A2A3E 描边 / amber-gold 主按钮）。
// 设计稿无 modal 结构，此处按管理端既有 token 自建，供各管理页复用。
import { X } from "lucide-react";
import type { ReactNode } from "react";

interface ModalProps {
  title: string;
  subtitle?: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  width?: number; // px，默认 480
}

export default function Modal({ title, subtitle, open, onClose, children, footer, width = 480 }: ModalProps) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative bg-bg-card border border-[#2A2A3E] rounded-xl w-full max-w-[92vw] shadow-2xl max-h-[86vh] flex flex-col"
        style={{ width }}
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#2A2A3E] flex-shrink-0">
          <div>
            <h3 className="text-base font-semibold text-white">{title}</h3>
            {subtitle && <p className="text-xs text-text-muted mt-0.5">{subtitle}</p>}
          </div>
          <button
            onClick={onClose}
            aria-label="关闭"
            className="w-8 h-8 flex items-center justify-center rounded-lg text-text-muted hover:text-white hover:bg-[#2A2A3E]/60 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="px-6 py-5 overflow-y-auto">{children}</div>
        {footer && (
          <div className="flex items-center justify-end gap-3 px-6 py-4 border-t border-[#2A2A3E] flex-shrink-0">{footer}</div>
        )}
      </div>
    </div>
  );
}
