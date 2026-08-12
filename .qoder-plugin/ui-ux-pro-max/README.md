# UI/UX Pro Max (Qoder Plugin)

[UI UX Pro Max](https://uupm.cc) 的 Qoder 原生插件包：一套为跨多平台和框架构建专业 UI/UX 提供设计智能的 AI 技能集。

## 包含的 Skills（7 个）

| Skill | 用途 | 关键能力 |
|-------|------|---------|
| `ui-ux-pro-max` | **核心设计智能**（推荐） | 可搜索设计数据库：84 风格 / 192 配色 / 74 字体配对 / 98 UX 准则 / 104 图标 / 16 GSAP 动效 / 25 图表类型 × 22 技术栈（React、Next.js、Vue、Tailwind、shadcn/ui 等）；含 Python 搜索脚本 `scripts/search.py` |
| `design` | 综合设计 | 品牌识别、设计令牌、UI 样式、logo 生成（55 风格）、CIP 企业识别（50 交付物）、HTML 演示（Chart.js）、横幅（22 风格）、图标（15 风格 SVG）、社媒图片 |
| `design-system` | 设计系统 | 三层令牌架构（primitive→semantic→component）、CSS 变量、间距/字体刻度、组件规格、幻灯片生成 |
| `brand` | 品牌 | 品牌声音、视觉识别、消息框架、资产组织、一致性清单 |
| `ui-styling` | UI 样式实现 | shadcn/ui（Radix+Tailwind）组件、Tailwind 工具类、暗色模式、无障碍组件；附本地 Canvas 字体库 |
| `banner-design` | 横幅设计 | 社媒/广告/网页 hero/印刷多格式；极简/渐变/粗体排版/玻璃拟态等 13+ 风格 |
| `slides` | HTML 演示文稿 | Chart.js 数据可视化、版式模式、文案公式、分页策略 |

## 与 design-taste-frontend 配合使用

- **design-taste-frontend**：负责整体视觉方向、设计系统搭建、落地页/作品集/改版的高质量 UI 输出
- **ui-ux-pro-max（本插件）**：在设计过程中查询具体设计决策——配色方案、字体配对、UX 准则（对比度/可点击区域/动效时长）、图表规范、无障碍检查，以及特定技术栈（React/Tailwind/shadcn）的实现建议

建议工作流：`design-taste-frontend` 定方向 → `ui-ux-pro-max` 查设计规则与数据 → `ui-styling` 落地组件代码。

## 来源与版权

- **Source**: https://github.com/nextlevelbuilder/ui-ux-pro-max-skill （v2.13.0，MIT License）
- **License**: MIT（详见各 skill 目录 LICENSE.txt / 源仓库 LICENSE）
- **Logo**: 本插件自绘占位 SVG（源仓库无可用 logo 资产）

## 已包含的支持文件

- `skills/ui-ux-pro-max/`：SKILL.md + `references/`（pro-rules、quick-reference）+ `scripts/`（search.py 等 4 个 Python 脚本）+ `data/`（16 个设计 CSV + `stacks/` 18 个技术栈 CSV）
- `skills/design/`：SKILL.md + `references/`（17 个设计指南）+ `scripts/` + `data/`（logo/CIP/icon 配置 CSV）
- `skills/design-system/`：SKILL.md + `references/` + `scripts/` + `templates/`
- `skills/brand/`：SKILL.md + `references/`（10 个）+ `scripts/`（4 个 Node 脚本）+ `templates/`
- `skills/ui-styling/`：SKILL.md + `references/`（7 个）+ `scripts/`（2 个 Python）+ `canvas-fonts/`（30+ 本地字体，OFL 许可）
- `skills/banner-design/`：SKILL.md + `references/`
- `skills/slides/`：SKILL.md + `references/`（5 个）

## 已省略

- 各 skill 的 `scripts/tests/` 单元测试与 `.coverage` 覆盖率数据（运行时不需要）
- 源仓库的 `cli/`（npm CLI 安装器）、`gallery/`（Next.js 画廊）、`projects/`（示例项目）、`stack/`、`docs/`——均为开发/展示用，非 skill 运行时依赖

## 环境要求

- `ui-ux-pro-max` 与 `design-system` 的搜索脚本需要 **Python 3.x**（`python3` 命令可用）
- `brand` 的脚本需要 **Node.js**（`node` 命令可用）
- 若脚本不可用，SKILL.md 仍可直接作为设计规则参考使用（数据在 `data/*.csv` 与 `references/*.md` 中）

## 安装

1. 将本插件目录（或其打包 zip）放入 Qoder 插件目录：`C:\Users\yz02\.qoder\plugins\custom\`
2. 或将 `skills/` 下需要的 skill 目录复制到 `C:\Users\yz02\.qoder\skills\`
3. 重启 Qoder 后技能生效，可通过斜杠命令调用（如 `/ui-ux-pro-max`）
