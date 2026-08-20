// 管理端路由：/admin/login 独立；/admin/* 走 AdminShell + 登录守卫。
import { createBrowserRouter, Navigate } from "react-router-dom";

import AdminShell from "./components/AdminShell";
import AdminLoginPage from "./pages/AdminLoginPage";
import AdminDashboardPage from "./pages/AdminDashboardPage";
import AdminUsersPage from "./pages/AdminUsersPage";
import AdminBillingPage from "./pages/AdminBillingPage";
import AdminAuditPage from "./pages/AdminAuditPage";
import AdminWorksPage from "./pages/AdminWorksPage";
import AdminTasksPage from "./pages/AdminTasksPage";
import AdminConfigPage from "./pages/AdminConfigPage";
import { getToken } from "./lib";

function RequireAuth({ children }: { children: React.ReactNode }) {
  if (!getToken()) {
    return <Navigate to="/admin/login" replace />;
  }
  return <>{children}</>;
}

export const router = createBrowserRouter([
  { path: "/admin/login", element: <AdminLoginPage /> },
  // api-client refresh 失败会跳 /login → 转投管理端登录页
  { path: "/login", element: <Navigate to="/admin/login" replace /> },
  {
    path: "/admin",
    element: (
      <RequireAuth>
        <AdminShell />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <Navigate to="/admin/dashboard" replace /> },
      { path: "dashboard", element: <AdminDashboardPage /> },
      { path: "users", element: <AdminUsersPage /> },
      { path: "audit", element: <AdminAuditPage /> },
      { path: "billing", element: <AdminBillingPage /> },
      { path: "works", element: <AdminWorksPage /> },
      { path: "tasks", element: <AdminTasksPage /> },
      { path: "settings", element: <AdminConfigPage /> },
    ],
  },
  { path: "/", element: <Navigate to="/admin/dashboard" replace /> },
  { path: "*", element: <Navigate to="/admin/dashboard" replace /> },
]);
