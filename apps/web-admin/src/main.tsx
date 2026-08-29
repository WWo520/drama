// 管理端入口：Router + 全局样式（薄入口，页面见 src/pages）。
import React from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider } from "react-router-dom";

import { setAppRealm } from "@duanju/api-client";
import { router } from "./router";
import "./index.css";

// 声明管理端身份：后端据此隔离 refresh Cookie，防止与用户端(5173) 同 host 串号
setAppRealm("admin");

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>,
);
