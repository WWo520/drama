// 占位页：菜单中暂无设计稿的功能（作品管理/算力任务/平台配置）。
import { Construction } from "lucide-react";

export default function PlaceholderPage({ title }: { title: string }) {
  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-white mb-1">{title}</h1>
        <p className="text-sm text-text-muted">该模块规划中，敬请期待</p>
      </div>
      <div className="bg-bg-card border border-[#2A2A3E] rounded-xl p-12 flex flex-col items-center justify-center text-center">
        <div className="w-14 h-14 rounded-xl bg-amber-gold/10 flex items-center justify-center mb-4">
          <Construction className="w-7 h-7 text-amber-gold" />
        </div>
        <div className="text-white font-semibold mb-1">{title}模块建设中</div>
        <div className="text-sm text-text-muted">当前批次仅交付：工作台 / 用户管理 / 内容审核 / 计费财务</div>
      </div>
    </div>
  );
}
