# 04 - HTML 转 React 一比一还原方案（不写代码版·实施规格书）

> **文档日期**: 2026-08-17
> **输入**: `docs/04 - 前端ui设计/luxreal-clone/pages/*.html`（design 注册 17 页；HTML 已产出 13 页 + 待生成 4 页）
> **输出**: `apps/web-user`（用户端 9 页）+ `apps/web-admin`（管理端 8 页）React 页面，渲染与设计稿**逐像素一致**
> **本文定位**: 转换动作的技术规格书（语法映射/三件套处理/逐页清单/顺序/验收），代码实施时照此执行

---

## 一、设计稿技术构成（2026-08-17 实测扫描）

设计稿页面是**三件套混用**，转换前必须认清每个页面用了什么：

| 页面 | class 总数 | 其中 Tailwind 类 | Lucide 图标数 | Tailwind CDN 引用 |
|------|-----------|----------------|--------------|------------------|
| workbench-script.html（基准页） | 327 | 174 | 56 | 无（类存在于内联，依赖设计工具环境） |
| create-video.html | 583 | 111 | 79 | 有（tailwindcss/browser@4.3.1） |
| admin-dashboard.html | 412 | 387 | 44 | 无 |
| 其余 12 页 | - | 混用（各页比例不同） | 20~80 | 部分有 |

**三件套**：

| 技术 | 设计稿用法 | React 工程对应 |
|------|-----------|---------------|
| ① Tailwind CDN v4.3.1 工具类 | `flex px-6 mx-auto w-[200px] max-w-[65%]` 等 | web-user 已装 Tailwind **v3.4**（版本差异见 3.1） |
| ② 内联自定义 CSS | 每页 `<style>` 220~2,718 行（类名如 `.luxreal-nav-item` `.project-card`） | 整块搬入每页 `.css` 文件（铁律①） |
| ③ Lucide CDN | `<i data-lucide="settings">` + `lucide.createIcons()` | web-user 已装 **lucide-react**（`<Settings/>`） |

> ⚠️ 修正认知：之前"零 Tailwind"判断有误（正则只匹配响应式前缀类）。设计稿实际是 Tailwind 类 + 自定义类混合，转换时两类都要原样保留。

---

## 二、转换总流程（每页六步）

```
第 1 步 建骨架     pages/{page}/ 目录 + {Page}.tsx 空组件 + {page}.css 空文件
第 2 步 搬 CSS     设计稿 <style> 块 → {page}.css（逐字搬入，零修改）
第 3 步 转 DOM     <body> 内容 → JSX（按第三节语法映射表逐行转换，类名/层级不动）
第 4 步 拆组件     页面内重复/独立区块 → 子组件（DOM 模板逐字保留，props 注入数据）
第 5 步 转交互     内联 <script> 逻辑 → hooks/事件（按第四节对照表）
第 6 步 换数据     静态 mock → TanStack Query（见 05-00 文档第四节映射）
```

**每步验收**：第 2 步后 CSS 逐字比对；第 3 步后浏览器渲染与设计稿并排无差异（此时还是静态数据）；第 6 步后再验一次。

---

## 三、HTML → JSX 语法映射表（第 3 步执行规范）

### 3.1 属性名转换

| HTML | JSX | 说明 |
|------|-----|------|
| `class="x"` | `className="x"` | 类名值**原样**（含 Tailwind 类和自定义类） |
| `style="a: b; c: d"` | `style={{ a: 'b', c: 'd' }}` | CSS 属性名转 camelCase：`background-color` → `backgroundColor` |
| `for="id"` | `htmlFor="id"` | label 关联 |
| `tabindex` | `tabIndex` | - |
| `autoplay` `controls` `loop` | `autoPlay` `controls` `loop` | 布尔属性显式赋值 |
| `onclick="fn()"` | `onClick={fn}` | 见第四节 |
| `aria-*` / `data-*` | 原样 | 自定义属性保留 |

### 3.2 SVG 属性（页面内有 L 形 Logo、大量内联 SVG）

| HTML | JSX |
|------|-----|
| `stroke-width` | `strokeWidth` |
| `stroke-linecap` | `strokeLinecap` |
| `stroke-linejoin` | `strokeLinejoin` |
| `fill-rule` / `clip-rule` | `fillRule` / `clipRule` |
| `clip-path` | `clipPath` |
| `viewBox` | 原样 |
| `<path d="..."/>` | `<path d="..."/>` |

### 3.3 标签闭合

`<img>` `<br>` `<input>` `<hr>` → 自闭合 `<img />` `<br />` `<input />` `<hr />`；注释 `<!-- x -->` → `{/* x */}`。

### 3.4 保留不动的东西

- 所有 class 值（Tailwind 类 + 自定义类，禁止改任何一个字）
- DOM 层级与顺序（设计稿的 CSS 选择器可能依赖后代关系）
- 内联 `style=""` 的值
- 文本内容与空白结构（`&nbsp;` → `{'\u00A0'}`）

---

## 四、三件套的 React 映射细节

### 4.1 ① Tailwind 类：CDN v4 → 工程 v3.4 的兼容处理

| 差异点 | 处理 |
|--------|------|
| v4 CDN 任意值语法 `w-[200px]` `max-w-[65%]` | v3.4 完全支持，原样保留 |
| v4 新色板/新类（如 `bg-linear-*` 等 v4 专属） | 抽查扫描：若出现 v3 不渲染的类 → 在 `tailwind.config.js` 补对应定义，**不改设计稿类名** |
| v3 需要 content 扫描配置 | `apps/web-user/tailwind.config.js` content 数组需包含 `src/pages/**/*.tsx`（新增路径时同步） |
| v4 默认 preflight 差异 | web-user 现有 Tailwind 3.4 preflight 与设计稿 CDN 渲染有细微差异时，以设计稿视觉为准，差异项写入 index.css 补偿 |

> 验证方法：同一页面分别用 CDN(v4) 与工程(v3.4) 渲染，对比 class 渲染结果，差异清单记入页面转换记录。

### 4.2 ② Lucide CDN → lucide-react

| 设计稿 | React |
|--------|-------|
| `<i data-lucide="settings"></i>` | `import { Settings } from 'lucide-react'` → `<Settings />` |
| `lucide.createIcons()`（初始化扫描） | 删除（lucide-react 直接渲染） |
| 图标尺寸/颜色靠 class 控制 | 原样（class 搬进 SVG 的 `className` 属性） |
| 设计稿用到的图标名（如 `folder`/`sparkles`/`compass`） | 与 lucide-react 组件名对照（PascalCase）；设计稿名不存在时记录并找替代 |

### 4.3 ③ 内联 CSS → 每页 .css

- 每页 `<style>` 多个块合并为一个 `.css` 文件（块顺序保持）
- 全局 `colors_and_type.css`（585 行）→ `src/index.css`（替换现有），其中 24 个 `--luxreal-*` 变量同步映射进 tailwind.config.js theme.extend（见 05-03 文档 1.3）
- **禁止**：重命名类、合并重复选择器、"优化"嵌套、改任何值

---

## 五、内联 script → React 对照表（第 5 步）

| 设计稿模式 | React 实现 |
|-----------|-----------|
| `document.querySelectorAll('.project-card')` 批量绑点击 | 组件 map 内直接 `onClick` |
| `classList.toggle('visible')` 弹窗/下拉 | `useState<boolean>` + `className={open ? 'x visible' : 'x'}`（CSS 不变） |
| `getBoundingClientRect()` 面板定位 | `useLayoutEffect` + `useRef`（逻辑照搬） |
| `window.location.href = 'xxx.html'` | `navigate('/xxx')` |
| `setInterval` 轮询 | TanStack Query `refetchInterval`（收敛单查询，防限流） |
| 静态写死的卡片列表 | `data.items.map()`（卡片 DOM 模板逐字保留） |
| `lucide.createIcons()` | 删除 |

---

## 六、逐页转换清单（17 页 × 产物与难点；⏳ = HTML 待用户生成）

### 6.1 用户端（web-user，9 页）

| # | 设计稿 | 产物 | 已知难点 |
|---|--------|------|---------|
| 1 | workbench-script.html（**基准页，先行**） | `pages/workbench/script/Step1Page.tsx` + `script.css` + `layouts/{AppShell,WorkbenchShell,shell.css}` + `components/{WorkflowPill,EpisodeSidebar}` | 共用布局外壳从此页提取；SVG Logo；叙事单元列表（纯文本无树） |
| 2 | create-video.html | `pages/home/HomePage.tsx` + `home.css` + `components/{ProjectCard,CreateProjectDialog,StylePicker,ProfilePopover}` | agent/canvas 双 Tab；轮播 Banner；583 个 class 工作量最大 |
| 3 | workbench-assets.html | `pages/workbench/assets/Step2Page.tsx` + `assets.css` | 6 Tab；FloatingActionBar；近全宽 |
| 4 | workbench-video.html | `pages/workbench/video/Step3Page.tsx` + `video.css` | 85% 宽度；镜头卡网格；FloatingActionBar |
| 5 | workbench-preview.html | `pages/workbench/preview/Step4Page.tsx` + `preview.css` | 播放器 400px/68vh；9:16 水印/画质选择器 |
| 6 | 登录注册（⏳ HTML 待生成） | `pages/login/LoginPage.tsx` + `login.css` | 双 Tab（验证码 Tab 首期隐藏）；品牌名 Empty Broken |
| 7 | discover.html | `pages/feed/FeedPage.tsx` + `feed.css` | 瀑布流 |
| 8 | works.html | `pages/works/ProjectListPage.tsx` + `works.css` | 三 Tab + 分页 |
| 9 | recharge.html | `pages/recharge/PointsPage.tsx` + `recharge.css` | 订阅套餐卡 + 确认支付弹窗 |
| - | notifications.html（design 未注册，⚠️ 待确认） | `pages/notifications/NotificationsPage.tsx` + `notifications.css` | 三 Tab |

### 6.2 管理端（web-admin，8 页）

| # | 设计稿 | 产物 | 已知难点 |
|---|--------|------|---------|
| 10 | admin-login.html（品牌 DUANJU Admin） | `pages/login/AdminLoginPage.tsx` + `admin-login.css` | - |
| 11 | admin-dashboard.html | `pages/dashboard/AdminDashboardPage.tsx`（387 个 Tailwind 类，几乎无自定义 CSS） | 趋势图（图表库选型：recharts 或纯 CSS/SVG，视觉对齐设计稿） |
| 12 | admin-users.html | `pages/users/AdminUsersPage.tsx` + `users.css` | 5 个管理弹窗 |
| 13 | admin-audit.html | `pages/audit/AdminAuditPage.tsx` + `audit.css` | 四 Tab |
| 14 | admin-billing.html | `pages/billing/AdminBillingPage.tsx` + `billing.css` | 提现 Tab 置灰 |
| 15 | 作品管理（⏳） | `pages/works/AdminWorksPage.tsx` | 后端 `/admin/works` 已就绪 |
| 16 | 算力任务（⏳） | `pages/tasks/AdminTasksPage.tsx` | 后端缺 `/admin/tasks`（见 01 文档 7.11） |
| 17 | 平台配置（⏳） | `pages/config/AdminConfigPage.tsx` | 对接 platform-config + 大模型配置 CRUD |

---

## 七、执行顺序（还原保真的关键）

> 排期口径：02 文档第四节为最终紧凑排期（8 步 10 个工作日）；本节为按页推进的任务视图，两者冲突时以 02 为准。

```
阶段 A（0.5 天）  基建：tailwind content 扫描配置 + index.css（全局 token 搬入）+ 品牌名替换规则
阶段 B（1 天）    基准页 workbench-script → Step1Page + 提取 AppShell/WorkbenchShell/共用组件
                  → 验收：设计稿 8901 端口 vs 5173 并排零差异（共用布局 60% 工作量集中在此）
阶段 C（1.5 天）  其余 3 个工作台页（复用 WorkbenchShell，只做内容区）
阶段 D（2 天）    前台页：login → create-video → discover → works → recharge → notifications
阶段 E（2 天）    管理端 5 页（web-admin）
阶段 F（0.5 天）  15 页全量并排验收 + 差异清单归零
```

**为什么基准页先行**：共用布局（72px 导航/220px 剧集栏/工作流胶囊/浮动按钮）集中在此页，React 化后其余 3 个工作台页只需转换内容区——与设计稿"基准页规则"同构。

---

## 八、一比一还原验收（每页与全量）

```
单页验收（每页完成后）：
1. python -m http.server 8901（luxreal-clone 目录）→ 设计稿 http://localhost:8901/pages/xxx.html
2. React 页 http://localhost:5173/xxx（数据先用设计稿同款 mock，保证对比公平）
3. 同视口 1440×900 截图并排，Chrome DevTools 逐区块检查（检查清单见 05-03 文档第五节）

全量验收（阶段 F）：
□ 15 页全部零差异通过
□ 无遗留 Tailwind 不渲染类（v4→v3 差异清单已清零）
□ 无遗留 data-lucide 未转换
□ 品牌名全部 Empty Broken、类前缀 luxreal- 保留
□ 交互一致性：弹窗开关/悬停/滚动与设计稿同行为
```

---

## 九、工具辅助（可选，非必需）

| 辅助 | 说明 |
|------|------|
| AI 辅助转换 | 每页 HTML 交给 AI 做第 3 步（DOM→JSX 语法转换），人工按第三节映射表复核——类名/CSS 零改动是复核重点 |
| Tailwind v4→v3 差异检查 | 抽取所有 class 与 v3.4 渲染对比，生成差异清单 |
| Lucide 图标名清单 | 扫描全部 `data-lucide` 值生成去重清单，对照 lucide-react 组件名 |
| 截图对比脚本 | 设计稿与 React 页同视口截图逐像素 diff（超阈值高亮） |

> 本方案为规格书：先审后动工。代码实施严格按第三节映射表与第六节清单执行，每页完成后先做单页验收再进下一页。
