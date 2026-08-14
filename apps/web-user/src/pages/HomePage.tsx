// 首页：登录后自动进入项目列表（S1-F02）
import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

import { useAuthStore } from "../stores/authStore";

export default function HomePage() {
  const navigate = useNavigate();
  const accessToken = useAuthStore((s) => s.accessToken);

  useEffect(() => {
    if (accessToken) {
      navigate("/projects", { replace: true });
    } else {
      navigate("/login", { replace: true });
    }
  }, [accessToken, navigate]);

  return null;
}
