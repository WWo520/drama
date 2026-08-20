// ProjectLoadError：Step 工作台项目加载失败降级页（项目 404/无权限/后端异常时整页提示）
import { useNavigate } from "react-router-dom";
import { FileQuestion } from "lucide-react";

export default function ProjectLoadError({ message = "项目不存在或已被删除" }: { message?: string }) {
  const navigate = useNavigate();
  return (
    <div
      className="flex-1 flex flex-col items-center justify-center min-h-screen"
      style={{ background: "#030911" }}
    >
      <FileQuestion size={48} style={{ color: "#4A4A5A" }} />
      <div className="mt-4 text-white font-medium">{message}</div>
      <p className="mt-2 text-xs" style={{ color: "#6B7088" }}>
        请检查链接是否正确，或返回作品列表重新进入
      </p>
      <button
        className="mt-6 px-4 py-2 rounded-lg text-sm transition-colors hover:bg-[#4C7FF8]/20"
        style={{ color: "#4C7FF8", background: "rgba(76, 127, 248, 0.1)", border: "1px solid rgba(76, 127, 248, 0.3)" }}
        onClick={() => navigate("/projects")}
      >
        返回作品列表
      </button>
    </div>
  );
}
