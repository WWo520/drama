// 生成两份清单 md：03 后端接口实际清单 / 04 前端接口实际清单
const fs = require("fs");
const r = JSON.parse(fs.readFileSync("tmp_align_report.json", "utf8"));

const usedSet = new Set(r.frontCalls.map((m) => m.back));
const unusedSet = new Set(r.backendRoutesUnusedByFrontend);

// 后端按域分组
function groupOf(route) {
  const p = route.split(" ")[1];
  if (p.startsWith("/admin")) return "管理端 /admin";
  if (p.startsWith("/audit")) return "审核 /audit";
  if (p.startsWith("/auth")) return "认证 /auth";
  if (p.startsWith("/spaces")) return "空间 /spaces";
  if (p.startsWith("/projects")) return "项目 /projects";
  if (p.startsWith("/episodes")) return "剧集 /episodes";
  if (p.startsWith("/narrative-units")) return "叙事单元";
  if (p.startsWith("/beats")) return "节拍 /beats";
  if (p.startsWith("/shot")) return "镜头 /shots|shot-groups";
  if (p.startsWith("/storyboards")) return "分镜 /storyboards";
  if (p.startsWith("/assets") || p.startsWith("/characters") || p.startsWith("/scenes") || p.startsWith("/props") || p.startsWith("/character-variants")) return "资产";
  if (p.startsWith("/canvas")) return "画布 /canvas(s)";
  if (p.startsWith("/works") || p.startsWith("/recycle-bin") || p.startsWith("/comments")) return "内容 /works";
  if (p.startsWith("/task")) return "任务与接单 /tasks|task-orders";
  if (p.startsWith("/membership")) return "会员 /membership(s)";
  if (p.startsWith("/templates") || p.startsWith("/style-library") || p.startsWith("/skills")) return "生态 模板/风格/技能";
  if (p.startsWith("/uploads")) return "上传 /uploads";
  if (p.startsWith("/exports")) return "导出 /exports";
  if (p.startsWith("/api-keys") || p.startsWith("/webhooks")) return "开发者";
  if (p.startsWith("/users") || p.startsWith("/me")) return "用户 /users|me";
  if (p.startsWith("/api/v1")) return "公开路由（router.go 注册）";
  return "其他";
}

const groups = new Map();
for (const route of r.backendRoutes) {
  const g = groupOf(route);
  if (!groups.has(g)) groups.set(g, []);
  groups.get(g).push(route);
}

let md3 = "";
md3 += `> 生成方式：脚本静态扫描 router.go + 3 个 domain handler.go（组前缀展开）。共 **${r.stats.backendRoutes}** 条路由。\n`;
md3 += `> 使用状态：✅=前端有调用（${r.stats.matched} 条匹配）；⬜=前端未使用（${r.stats.unused} 条，分类见 05 对齐方案）。\n\n`;
for (const [g, list] of groups) {
  md3 += `\n## ${g}（${list.length} 条）\n\n| 方法 | 路径 | 前端使用 |\n|------|------|---------|\n`;
  for (const route of list) {
    const [m, ...rest] = route.split(" ");
    md3 += `| ${m} | \`${rest.join(" ")}\` | ${usedSet.has(route) ? "✅" : "⬜"} |\n`;
  }
}
const today = new Date().toISOString().slice(0, 10);
const H3 = `# 03 - 后端接口实际清单（静态扫描）

> 生成日期：${today}
> 来源：duanju-backend/internal/router/router.go + domain/{creation,space,auth}/handler.go
> 说明：运行时全部挂在 /api/v1 前缀下（v1Auth 需 JWT；v1Pub 公开；/admin 需 SUPER_ADMIN；/audit 需 SUPER_ADMIN/AUDITOR）。

---

`;
fs.writeFileSync("docs/12 - 阶段优化/03 - 后端接口实际清单.md", H3 + md3);

// 前端按文件分组
const byFile = new Map();
for (const m of r.frontCalls) {
  for (const f of m.files) {
    if (!byFile.has(f)) byFile.set(f, []);
    byFile.get(f).push(m.front);
  }
}
let md4 = "";
md4 += `> 生成方式：脚本静态扫描 apps/ 与 packages/ 全部 ts/tsx 中 \`api.(get|post|put|patch|delete)\` 调用点。共 **${r.stats.frontCalls}** 个调用模式，命中后端 **${r.stats.matched}** 个，缺失 **0**（查询串/前缀差异已人工排除）。\n\n`;
const files = [...byFile.keys()].sort();
for (const f of files) {
  const calls = [...new Set(byFile.get(f))].sort();
  md4 += `\n## ${f}（${calls.length} 个）\n\n`;
  for (const c of calls) md4 += `- \`${c}\`\n`;
}
const H4 = `# 04 - 前端接口实际清单（静态扫描）

> 生成日期：${today}
> 来源：apps/web-user、apps/web-admin、packages/api-client 全部 ts/tsx
> 说明：api-client 统一加 /api/v1 前缀；模板变量路径归一化为 :id。

---

`;
fs.writeFileSync("docs/12 - 阶段优化/04 - 前端接口实际清单.md", H4 + md4);
console.log("done", groups.size, "groups,", files.length, "files");
