// 对齐数据提取：后端路由清单 vs 前端调用清单 vs 交叉比对
const fs = require("fs");
const path = require("path");

const ROOT = process.cwd();
const routeFiles = [
  "duanju-backend/internal/router/router.go",
  "duanju-backend/internal/domain/creation/handler.go",
  "duanju-backend/internal/domain/space/handler.go",
  "duanju-backend/internal/domain/auth/handler.go",
];

// ---- 后端：组前缀展开 ----
const routes = new Set();
for (const f of routeFiles) {
  const p = path.join(ROOT, f);
  if (!fs.existsSync(p)) continue;
  const content = fs.readFileSync(p, "utf8");
  const prefix = { rg: "" };
  for (const m of content.matchAll(/(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"/g)) {
    prefix[m[1]] = (prefix[m[2]] ?? "") + m[3];
  }
  const lines = content.split("\n");
  for (const line of lines) {
    const m = line.match(/(\w+)\.(GET|POST|PUT|PATCH|DELETE)\("([^"]*)"/);
    if (!m) continue;
    const base = prefix[m[1]] ?? "";
    routes.add(`${m[2]} ${base}${m[3]}`.replace(/\/+/g, "/"));
  }
}
const routeList = [...routes].sort();

// ---- 前端：所有 api.xxx 调用 ----
const apiRe = /api\.(get|post|put|patch|delete)[^(]*\(\s*([`"])([^`"]+)\2/g;
const frontCalls = new Map(); // "GET /path" -> Set<file>
function walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (!/node_modules|dist/.test(e.name)) walk(full);
      continue;
    }
    if (!/\.(ts|tsx)$/.test(e.name)) continue;
    const content = fs.readFileSync(full, "utf8");
    for (const m of content.matchAll(apiRe)) {
      const method = m[1].toUpperCase();
      let p = m[3];
      const hasTpl = p.includes("${");
      const norm = p.replace(/\$\{[^}]+\}/g, ":id").replace(/:[^/]+/g, ":id");
      const key = `${method} ${norm}`;
      if (!frontCalls.has(key)) frontCalls.set(key, { files: new Set(), tpl: hasTpl, raw: p });
      frontCalls.get(key).files.add(path.relative(ROOT, full));
    }
  }
}
walk(path.join(ROOT, "apps"));
walk(path.join(ROOT, "packages"));

// ---- 交叉比对 ----
const normRoute = (s) => s.replace(/:[^/]+/g, ":id");
const routeNormSet = new Map(routeList.map((r) => [normRoute(r), r]));

const missing = []; // 前端调用但后端无
const matched = [];
for (const [key, info] of frontCalls) {
  const r = routeNormSet.get(key);
  if (r) matched.push({ front: key, back: r, files: [...info.files].slice(0, 3) });
  else missing.push({ front: key, raw: info.raw, tpl: info.tpl, files: [...info.files].slice(0, 3) });
}
const usedNorm = new Set(matched.map((m) => m.back));
const unusedRoutes = routeList.filter((r) => !usedNorm.has(r));

fs.writeFileSync(
  "tmp_align_report.json",
  JSON.stringify(
    {
      backendRoutes: routeList,
      frontCalls: matched,
      frontCallsMissingBackend: missing,
      backendRoutesUnusedByFrontend: unusedRoutes,
      stats: {
        backendRoutes: routeList.length,
        frontCalls: frontCalls.size,
        matched: matched.length,
        missing: missing.length,
        unused: unusedRoutes.length,
      },
    },
    null,
    1,
  ),
);
console.log(JSON.stringify({
  backendRoutes: routeList.length,
  frontCalls: frontCalls.size,
  matched: matched.length,
  missing: missing.length,
  unused: unusedRoutes.length,
}));
