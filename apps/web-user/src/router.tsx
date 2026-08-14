// 路由骨架：loader 守卫做鉴权/进度拦截（06 前端方案 2.1：createBrowserRouter）。
import { createBrowserRouter, Navigate, useParams } from "react-router-dom";

import { useAuthStore } from "./stores/authStore";
import LoginPage from "./pages/LoginPage";
import HomePage from "./pages/HomePage";
import ProjectListPage from "./pages/ProjectListPage";
import Step1Page from "./pages/Step1Page";
import Step2Page from "./pages/Step2Page";
import Step3Page from "./pages/Step3Page";
import Step4Page from "./pages/Step4Page";
import PointsPage from "./pages/PointsPage";
import SpaceManagePage from "./pages/SpaceManagePage";
import CanvasPage from "./pages/CanvasPage";
import TimelinePage from "./pages/TimelinePage";
import FeedPage from "./pages/FeedPage";
import TasksPage from "./pages/TasksPage";
import AdminPage from "./pages/AdminPage";
import GlobalWS from "./components/GlobalWS";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.accessToken);
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return (
    <>
      <GlobalWS />
      {children}
    </>
  );
}

// Step 路由分发：/step/:step → 对应 Step 页面
function StepRouter() {
  const { step } = useParams();
  switch (step) {
    case "2":
      return <Step2Page />;
    case "3":
      return <Step3Page />;
    case "4":
      return <Step4Page />;
    default:
      return <Step1Page />;
  }
}

export const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  {
    path: "/",
    element: (
      <RequireAuth>
        <HomePage />
      </RequireAuth>
    ),
  },
  {
    path: "/projects",
    element: (
      <RequireAuth>
        <ProjectListPage />
      </RequireAuth>
    ),
  },
  {
    path: "/projects/:projectId",
    element: (
      <RequireAuth>
        <Step1Page />
      </RequireAuth>
    ),
  },
  {
    path: "/points",
    element: (
      <RequireAuth>
        <PointsPage />
      </RequireAuth>
    ),
  },
  {
    path: "/spaces",
    element: (
      <RequireAuth>
        <SpaceManagePage />
      </RequireAuth>
    ),
  },
  {
    path: "/canvas",
    element: (
      <RequireAuth>
        <CanvasPage />
      </RequireAuth>
    ),
  },
  {
    path: "/timeline",
    element: (
      <RequireAuth>
        <TimelinePage />
      </RequireAuth>
    ),
  },
  {
    path: "/feed",
    element: (
      <RequireAuth>
        <FeedPage />
      </RequireAuth>
    ),
  },
  {
    path: "/tasks",
    element: (
      <RequireAuth>
        <TasksPage />
      </RequireAuth>
    ),
  },
  {
    path: "/admin",
    element: (
      <RequireAuth>
        <AdminPage />
      </RequireAuth>
    ),
  },
  {
    path: "/projects/:projectId/step/:step",
    element: (
      <RequireAuth>
        <StepRouter />
      </RequireAuth>
    ),
  },
]);
