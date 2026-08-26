# AutoDL 自部署 Toonflow 接入 MiniMax H3 部署文档

> **定位：** 复刻 Toonflow 官方在智星云（GPU 云 Windows 镜像开箱即用）的形态，改在 **AutoDL Linux 实例**手动部署 Toonflow 编排服务，并通过**可编程供应商（vendor）**接入 **MiniMax H3（V2 API）** 多图参考视频生成。
> **架构要点：** Toonflow 本体是纯 Node.js 编排服务（Express + SQLite + Socket.IO），**不消耗 GPU**——生成调用全部走远程模型 API（MiniMax 官方 / 其他厂商）。因此 AutoDL 实例可用**无卡模式**运行，成本极低。
> **姊妹文档：** 平台侧接入方案见 [01 - MiniMax H3 按秒计费视频生成接入技术方案.md](file:///d:/develop/a_work/duanju/docs/20视频生成模型/01%20-%20MiniMax%20H3%20按秒计费视频生成接入技术方案.md)（API 规格、双轨计费、提示词模板）。

---

## 目录

1. 架构与部署形态
2. 前置条件清单
3. AutoDL 实例创建
4. 环境安装（Node.js 24 / Yarn / PM2）
5. 获取代码与构建
6. 启动服务与公网访问（6006 端口映射）
7. 首次登录与基础配置（文本/图像供应商）
8. MiniMax H3 自定义 vendor（完整代码 + 配置步骤）
9. 多图参考视频生成使用要点
10. 日常运维（自启/关机/备份/成本）
11. 常见问题（FAQ）

---

## 一、架构与部署形态

```
┌─ AutoDL 实例（无卡模式，Ubuntu）────────────────┐
│  Toonflow（Node.js + PM2，监听 0.0.0.0:6006）   │
│   ├─ Web UI + Socket.IO（编排/工作台）           │
│   ├─ SQLite（项目/资产/分镜数据，本地 data/）    │
│   └─ vendor 沙盒（data/vendor/*.ts 热加载）      │
└──────────────┬───────────────────────────────┘
               │ AutoDL"自定义服务"公网映射（6006 → https://xxx.seetacloud.com:8443）
               ▼ 用户浏览器访问
   Toonflow ──HTTP──▶ MiniMax 官方 API（api.minimaxi.com/v2/video_generation）
                 ──HTTP──▶ 文本模型（DeepSeek 等）/ 图像模型（Seedream 等）
```

与智星云官方镜像的对应关系：

| 智星云官方形态 | AutoDL 自部署（本文） |
|---|---|
| Windows 镜像内置 Toonflow 桌面版 | Linux 实例手动 `yarn build` + PM2 常驻 |
| RDP 登录图形桌面 | 浏览器访问"自定义服务"公网地址 |
| 智星云 Token 服务（文本模型） | 自选：DeepSeek 官方 / 任意 OpenAI 兼容端点 |
| 内置 vendor 走旧 Hailuo v1（无 H3） | **自定义 vendor 走 H3 V2**（第八章） |
| 4090 GPU（供 ComfyUI 可选） | 无卡模式即可（不用 ComfyUI 本地推理时） |

> **注意：** AutoDL 实例本身是容器，**实例内无法运行 Docker**（无 privileged），README 的 Docker 部署方式在 AutoDL 不可用，本文走 Node.js + PM2 路线。

---

## 二、前置条件清单

| # | 条件 | 说明 |
|---|---|---|
| 1 | **AutoDL 账号 + 实名认证** | [autodl.com](https://www.autodl.com) 注册；个人实名即可（6006/6008 端口自带公网映射，**无需企业认证**） |
| 2 | **AutoDL 账户余额** | 无卡模式约 ¥0.1~0.2/小时量级（以控制台实价为准）；数据盘存储费另计（GB/小时，很低） |
| 3 | **MiniMax 开放平台 API Key** | [platform.minimaxi.com](https://platform.minimaxi.com) 注册实名 → 「账户管理→接口密钥」创建（**仅明文一次，立即保存**）→ 充值（H3 按量：768P ¥0.50/秒、2K ¥0.80/秒） |
| 4 | **文本模型 API Key**（驱动小说转剧本 Agent） | 推荐 DeepSeek 官方（platform.deepseek.com）或任意 OpenAI 兼容端点 |
| 5 | **图像模型 API Key**（资产/分镜图，可选） | 如火山 Seedream、智星云 Token 等；不配则无法批量出图，视频多图参考也缺素材 |
| 6 | **Toonflow 源码** | 本地已有 `refer/Toonflow-app`（Apache-2.0，可自部署；商用分发需自行评估许可证与商标） |

---

## 三、AutoDL 实例创建

1. **地域**：任选（建议与常用网络近的，如西北/北京区）。
2. **计费模式**：先按量计费试用。
3. **镜像**：任选基础镜像（如 `PyTorch 2.x / Ubuntu 22.04 / CUDA 12.x`）——只用到它的 Ubuntu 环境，不跑训练。
4. **GPU**：**选无卡模式开机**（控制台"无卡模式开机"按钮）——Toonflow 编排 + 远程 API 调用不需要 GPU；后续若要跑 ComfyUI 本地推理再切换有卡开机。
5. **数据盘**：默认 50GB 免费额度内足够（代码+依赖 ~2GB、SQLite 项目数据很小）。
6. 创建后记录：**SSH 登录指令与密码**（控制台可见）。

> 关机规则：AutoDL 关机后**系统盘免费保留、数据盘按存储计费**；实例**释放**后系统盘数据清空（重要数据放数据盘 `autodl-tmp` 或定期备份）。

---

## 四、环境安装（Node.js 24 / Yarn / PM2）

SSH 连入实例后执行：

```bash
# 0) 开启学术加速（拉 GitHub/nvm 快）；用完可 unset http_proxy https_proxy 关闭
source /etc/network_turbo

# 1) 基础编译工具（better-sqlite3/sharp 原生模块兜底）
apt update && apt install -y build-essential python3

# 2) 安装 Node.js 24（README 要求 23.11.1+，推荐 24）
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.bashrc
nvm install 24
node -v   # 应输出 v24.x

# 3) Yarn 1.x 与 PM2
npm install -g yarn pm2
```

> 若 GitHub 直连慢/失败：保持 `source /etc/network_turbo` 后重试；npm 慢可用镜像 `npm config set registry https://registry.npmmirror.com`（yarn 同理加 `--registry`）。

---

## 五、获取代码与构建

### 方式 A：直接 Git 克隆（推荐）

```bash
mkdir -p ~/autodl-tmp && cd ~/autodl-tmp
source /etc/network_turbo
git clone https://gitee.com/HBAI-Ltd/Toonflow-app.git   # 国内；或 github.com/HBAI-Ltd/Toonflow-app
cd Toonflow-app
```

### 方式 B：上传本地 refer/Toonflow-app

本地 PowerShell 打包（排除 node_modules）→ JupyterLab 上传到 `autodl-tmp` → 解压：

```powershell
# 本地（Windows）
Compress-Archive -Path refer\Toonflow-app\* -DestinationPath toonflow.zip -Force
# 上传：AutoDL 控制台 → JupyterLab → 文件管理 → 上传到 /root/autodl-tmp/
```

```bash
cd ~/autodl-tmp && unzip toonflow.zip -d Toonflow-app && cd Toonflow-app
```

### 安装依赖与构建（两方式相同）

```bash
# 0)（推荐）剥离 Electron 桌面端依赖——服务端部署用不到，且其 postinstall 要从
#    GitHub 拉二进制，是 yarn install 最常见的失败点（官方 Dockerfile 同款做法）
node -e "const fs=require('fs');const pkg=JSON.parse(fs.readFileSync('package.json','utf8'));for(const s of ['dependencies','devDependencies']){if(!pkg[s])continue;for(const n of ['custom-electron-titlebar','electron','electron-builder','electron-rebuild','electronmon'])delete pkg[s][n];}fs.writeFileSync('package.json',JSON.stringify(pkg,null,2)+'\n');"

# 1) 安装依赖（原生模块 better-sqlite3/sharp 较多，首次 5~15 分钟）
yarn install --registry https://registry.npmmirror.com

# 2) 改监听端口（关键）：源码把端口硬编码为 10588（src/app.ts L188，不读 PORT 环境变量），
#    而 AutoDL 仅 6006/6008 有公网映射 → 构建前改成 6006
sed -i 's/randomPort ? 0 : 10588/randomPort ? 0 : 6006/' src/app.ts

# 3) 构建（esbuild 打包后端 src/app.ts → data/serve/app.js）
yarn build
ls data/serve/app.js   # 存在即构建成功
```

> **构建产物说明（已对源码核实）：**
> - `yarn build` 只做一件事：esbuild 把 `src/app.ts` 打包成 `data/serve/app.js`（另一产物 `build/main.js` 是 Electron 主进程，服务端部署用不到）。
> - 前端页面 `data/web/` 是**仓库里预构建好的成品**，无需任何构建步骤，启动时由 [app.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/app.ts) 直接挂载为静态站点。
> - 内置 vendor 清单打包在 `src/lib/vendor.json`（仓库已提交），**首次启动时**由 [fixDB.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/lib/fixDB.ts) 自动落盘为 `data/vendor/*.ts` 并登记数据库；此后运行期以 `data/vendor/<id>.ts` 磁盘文件为准热加载（见 [vendor.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/utils/vendor.ts)）。自定义 vendor（如 minimax-h3）不在内置清单内，不受升级同步影响。
> - `yarn vendor2json` 是**维护者脚本**（把 data/vendor 回写为 JSON 快照供发版提交），部署时**不要**执行，`yarn build` 也**不会**自动调用它。

---

## 六、启动服务与公网访问（6006 端口映射）

监听端口已在第五章用 `sed` 改为 6006。**注意：README 写的 `PORT` 环境变量实际不生效**——[app.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/app.ts) L188 把端口硬编码为 10588，全源码仅在 Electron 模式下读过一次 `PORT`（oss.ts），PM2 里配 `PORT` 只是摆设。

### 1）PM2 配置

```bash
cd ~/autodl-tmp/Toonflow-app
cat > pm2.json <<'EOF'
{
  "name": "toonflow",
  "script": "data/serve/app.js",
  "cwd": "/root/autodl-tmp/Toonflow-app",
  "exec_mode": "fork",
  "env": {
    "NODE_ENV": "prod"
  }
}
EOF
pm2 start pm2.json && pm2 save
pm2 logs toonflow --lines 20   # 看到 [服务启动成功]: http://localhost:6006 即成功
```

配置要点（均已对源码核实）：

- **`NODE_ENV` 必须为 `prod`**：dev 模式下 [oss.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/utils/oss.ts) 会把素材 URL 硬编码成 `http://localhost:10588/...`，公网访问时图片全部 404；prod 模式生成**相对路径** `/oss/...`，经 AutoDL 网关访问天然正常。
- **不要设 `OSSURL`**：README 表格里的 `OSSURL` 是**无效变量**（源码无任何消费点），真实变量名是**小写 `ossURL`**（用于拼接绝对素材地址）；默认相对路径已经够用，不设反而更稳。
- **`cwd` 必须固定为项目根**：数据目录按 `process.cwd()/data` 解析（[getPath.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/utils/getPath.ts)），cwd 错了 SQLite、vendor TS、素材会落到别的目录。
- README 的 `instances:"max" + cluster` 模式**不要用**：Socket.IO + 内存态（vendor 沙盒/任务轮询）在 cluster 多进程下会状态分裂，`fork` 单进程最稳。

### 2）获取公网访问地址

控制台 → 该实例 → **「自定义服务」** → 复制地址，形如：
`https://u39-xxxx-xxxx.seetacloud.com:8443`
（即 6006 端口的公网映射，协议保持默认 HTTP。）

浏览器打开即见 Toonflow 登录页。

### 3）验证 WebSocket

Toonflow 用 Socket.IO 实时推送生成进度。打开页面 → F12 控制台：
- 无红色 `WebSocket connection failed` 即正常；
- 若 WS 被网关拦截：改用 **AutoDL-SSH-Tools 端口转发**本地访问（控制台"自定义服务"页可下载工具，代理后访问 `http://127.0.0.1:6006`）。

### 4）安全（必做）

- 首登 **admin / admin123**，登录后立即改密；
- 公网地址含随机串但本质公开——**不要在公开场合分享该 URL**；
- 所有 API Key 只存在服务端 SQLite（`data/` 下），不会下发前端。

---

## 七、首次登录与基础配置（文本/图像供应商）

登录后进入 **设置中心 → 模型服务**。最小可用配置 = 1 个文本 + 1 个图像 + 1 个视频（H3，第八章）。

| 类别 | 推荐配置 | 操作 |
|---|---|---|
| **文本**（Agent 推理） | 选内置 `DeepSeek` 供应商 → 填 API Key；或选 `OpenAI 标准接口` → 填任意兼容端点（baseUrl + key） | 保存后在「模型测试」跑一次连通 |
| **图像**（资产/分镜图） | 火山 Seedream / 智星云 Token / Toonflow 官方中转（`toonflow` 供应商，key 在 [api.toonflow.net](https://api.toonflow.net/) 申请） | 同上 |
| **视频（H3）** | **内置 minimax 供应商没有 H3**（只有 Hailuo 2.3/02 旧模型）→ 按第八章新增自定义 vendor | 见下章 |

配置完成后检查两处（智星云教程同款要求）：
1. 模型服务中各供应商开关已开启（`enable=1`）；
2. Agent 配置中实际调用的模型与所配一致。

---

## 八、MiniMax H3 自定义 vendor（核心章节）

Toonflow 的供应商是**热加载 TS**：设置中心编辑保存 → 服务端 sucrase 转译 → vm2 沙盒执行 → 即时生效（无需重启）。存储位置：`<项目>/data/vendor/<id>.ts`（对应路由 `setting/vendorConfig/addVendor|updateCode|updateVendorInputs`，见 [vendorConfig 目录](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/routes/setting/vendorConfig)）。

### 8.1 新增步骤（已对照 addVendor / updateCode 路由源码核实）

1. 设置中心 → 模型服务 → **新建/添加供应商**：把 8.2 的完整 TS 粘贴进代码编辑器保存即可——**供应商 id 无需单独填写**，取自代码内 `vendor.id`（即 `minimax-h3`）。后端保存前会用 sucrase 转译 + vm2 沙盒试运行校验：**必须导出 `textRequest / imageRequest / videoRequest / vendor` 四件套**（8.2 代码已满足，缺一会报"脚本文件必须导出xx对象"）；
2. **打开启用开关**：新增供应商默认 `enable=0`（[addVendor.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/routes/setting/vendorConfig/addVendor.ts) 仅官方 `toonflow` 中转默认开启），不开开关模型列表里看不到它；
3. 在供应商表单填入（保存即写 `o_vendorConfig.inputValues`，热生效无需重启）：
   - **API 密钥**：MiniMax 开放平台的 Key（`eyJ...`）；
   - **请求地址**：`https://api.minimaxi.com`（海外账号填 `https://api.minimax.io`）；
4. 「模型测试」选 `MiniMax H3 (多图参考)` 发一条纯文生视频（如 `一只橘猫在窗台打哈欠，镜头缓推` + duration 5 + 768P）验证；
5. 通过后在工作台选型处即可看到该模型。

### 8.2 完整 vendor 代码（可直接粘贴）

以下按 Toonflow vendor 模板（`minimax.ts` v2.1 结构）实现 **H3 V2 多模态接口**，支持文生/多图参考（≤9 图 + 3 视频 + 3 音频）：

```typescript
/**
 * Toonflow 供应商 - MiniMax H3 (海螺3) V2 多模态接口
 * 端点: POST /v2/video_generation + GET /v2/query/video_generation
 * 支持: 文生视频 / 多图参考 R2V (imageReference:9 / videoReference:3 / audioReference:3)
 */

type VideoMode =
  | "singleImage"
  | "startEndRequired"
  | "endFrameOptional"
  | "startFrameOptional"
  | "text"
  | (`videoReference:${number}` | `imageReference:${number}` | `audioReference:${number}`)[];

interface TextModel { name: string; modelName: string; type: "text"; think: boolean; }
interface VideoModel {
  name: string; modelName: string; type: "video";
  mode: VideoMode[]; audio: "optional" | false | true;
  durationResolutionMap: { duration: number[]; resolution: string[] }[];
}
interface ImageModel { name: string; modelName: string; type: "image"; mode: ("text" | "singleImage" | "multiReference")[]; }
interface TTSModel { name: string; modelName: string; type: "tts"; voices: { title: string; voice: string }[]; }

interface VendorConfig {
  id: string; version: string; name: string; author: string; description?: string; icon?: string;
  inputs: { key: string; label: string; type: "text" | "password" | "url"; required: boolean; placeholder?: string }[];
  inputValues: Record<string, string>;
  models: (TextModel | ImageModel | VideoModel | TTSModel)[];
}

type ReferenceList =
  | { type: "image"; sourceType: "base64"; base64: string }
  | { type: "audio"; sourceType: "base64"; base64: string }
  | { type: "video"; sourceType: "base64"; base64: string };

interface VideoConfig {
  duration: number;
  resolution: string;
  aspectRatio: "16:9" | "9:16";
  prompt: string;
  referenceList?: ReferenceList[];
  audio?: boolean;
  mode: VideoMode[];
}
interface ImageConfig { prompt: string; referenceList?: any[]; size: "1K" | "2K" | "4K"; aspectRatio: `${number}:${number}`; }
interface TTSConfig { text: string; voice: string; speechRate: number; pitchRate: number; volume: number; }
interface PollResult { completed: boolean; data?: string; error?: string; }

declare const logger: (msg: string) => void;
declare const zipImage: (base64: string, size: number) => Promise<string>;
declare const urlToBase64: (url: string) => Promise<string>;
declare const pollTask: (fn: () => Promise<PollResult>, interval?: number, timeout?: number) => Promise<PollResult>;
declare const createOpenAI: any;
declare const exports: {
  vendor: VendorConfig;
  textRequest: (m: TextModel, t: boolean, tl: 0 | 1 | 2 | 3) => any;
  uploadReference?: (base64: string, fileType: "image" | "audio" | "video") => Promise<ReferenceList>;
  imageRequest: (c: ImageConfig, m: ImageModel) => Promise<string>;
  videoRequest: (c: VideoConfig, m: VideoModel) => Promise<string>;
  ttsRequest: (c: TTSConfig, m: TTSModel) => Promise<string>;
};

const vendor: VendorConfig = {
  id: "minimax-h3",
  version: "1.0",
  author: "duanju",
  name: "MiniMax H3 (海螺3)",
  description: "MiniMax H3 V2 多模态视频接口：文生视频 + 多图参考(≤9图/3视频/3音频)，原生立体声，4-15秒，768P/2K。按秒计费 768P ¥0.5/s、2K ¥0.8/s。",
  inputs: [
    { key: "apiKey", label: "API密钥", type: "password", required: true },
    { key: "baseUrl", label: "请求地址", type: "url", required: true, placeholder: "示例：https://api.minimaxi.com" },
  ],
  inputValues: { apiKey: "", baseUrl: "https://api.minimaxi.com" },
  models: [
    {
      name: "MiniMax H3 (多图参考)",
      modelName: "MiniMax-H3",
      type: "video",
      // text=纯文生；数组=多模态参考模式（R2V）
      mode: ["text", ["imageReference:9", "videoReference:3", "audioReference:3"]],
      audio: true, // H3 原生生成 32kHz 立体声
      durationResolutionMap: [
        { duration: [4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15], resolution: ["768P", "2K"] },
      ],
    },
  ],
};

const getHeaders = (): Record<string, string> => ({
  Authorization: `Bearer ${vendor.inputValues.apiKey.replace(/^Bearer\s+/i, "")}`,
  "Content-Type": "application/json",
});
const getBaseUrl = (): string => vendor.inputValues.baseUrl.replace(/\/$/, "");

// 文本模型走官方 OpenAI 兼容端点（可选；Agent 文本建议单独配 DeepSeek 供应商）
const textRequest = (model: TextModel, _think: boolean, _tl: 0 | 1 | 2 | 3) => {
  if (!vendor.inputValues.apiKey) throw new Error("缺少API Key");
  return createOpenAI({ baseURL: `${getBaseUrl()}/v1`, apiKey: vendor.inputValues.apiKey.replace(/^Bearer\s+/i, "") }).chat(model.modelName);
};

// 参考素材前置处理：图片压缩到 ≤3MB/张
// （V2 官方限制：请求体总 ≤64MB、单图 ≤30MB；9 张 × 3MB 经 base64 膨胀约 36MB，留足余量。
//   注意 zipImage 第二参单位是【字节】——内置 minimax.ts 写 20*1024 实为 20KB，画质严重劣化，勿照抄）
const uploadReference = async (base64: string, fileType: "image" | "audio" | "video"): Promise<ReferenceList> => {
  if (fileType === "image") {
    const compressed = await zipImage(base64, 3 * 1024 * 1024);
    return { type: "image", sourceType: "base64", base64: compressed };
  }
  return { type: fileType, sourceType: "base64", base64 } as ReferenceList;
};

const withHead = (b64: string, mime: string): string => (b64.startsWith("data:") ? b64 : `data:${mime};base64,${b64}`);

const videoRequest = async (config: VideoConfig, model: VideoModel): Promise<string> => {
  if (!vendor.inputValues.apiKey) throw new Error("缺少API Key");
  const baseUrl = getBaseUrl();
  const headers = getHeaders();

  // ---- 组装 V2 content[] 多模态数组 ----
  const content: any[] = [{ type: "text", text: config.prompt }];
  const imageRefs = (config.referenceList ?? []).filter((r) => r.type === "image");
  const videoRefs = (config.referenceList ?? []).filter((r) => r.type === "video");
  const audioRefs = (config.referenceList ?? []).filter((r) => r.type === "audio");

  if (imageRefs.length > 9) throw new Error(`参考图最多 9 张，当前 ${imageRefs.length} 张`);
  if (videoRefs.length > 3) throw new Error(`参考视频最多 3 段，当前 ${videoRefs.length} 段`);
  if (audioRefs.length > 3) throw new Error(`参考音频最多 3 段，当前 ${audioRefs.length} 段`);
  // R2V：全部参考素材走 reference_* role（与 first/last_frame 互斥，本 vendor 不混用首尾帧）
  for (const r of imageRefs) {
    content.push({ type: "image_url", image_url: { url: withHead(r.base64, "image/jpeg") }, role: "reference_image" });
  }
  for (const r of videoRefs) {
    content.push({ type: "video_url", video_url: { url: withHead(r.base64, "video/mp4") }, role: "reference_video" });
  }
  for (const r of audioRefs) {
    content.push({ type: "audio_url", audio_url: { url: withHead(r.base64, "audio/mpeg") }, role: "reference_audio" });
  }

  const hasReference = content.length > 1;
  const reqBody: any = {
    model: model.modelName,
    content,
    resolution: config.resolution,          // "768P" | "2K"
    duration: config.duration,              // 4~15 整数
    // 纯文生时 ratio 必填且不可 adaptive；带参考时恒 adaptive（由参考输入决定）
    ratio: hasReference ? "adaptive" : (config.aspectRatio ?? "16:9"),
    aigc_watermark: false,
  };

  logger(`[H3] 提交视频任务 refs=${imageRefs.length}图/${videoRefs.length}视频/${audioRefs.length}音频 duration=${config.duration} res=${config.resolution}`);
  const submitResp = await fetch(`${baseUrl}/v2/video_generation`, {
    method: "POST",
    headers,
    body: JSON.stringify(reqBody),
  });
  const submitData = await submitResp.json().catch(() => ({}));
  if (!submitResp.ok || submitData.error) {
    const msg = submitData?.error?.message ?? submitData?.base_resp?.status_msg ?? JSON.stringify(submitData);
    throw new Error(`任务提交失败(${submitResp.status})：${msg}`);
  }
  const taskId = submitData.task_id;
  logger(`[H3] 任务提交成功 task_id=${taskId}`);

  // ---- 轮询（10s 间隔起步，总超时 15min：2K/多参考生成较慢）----
  const pollResult = await pollTask(
    async () => {
      const queryResp = await fetch(`${baseUrl}/v2/query/video_generation?task_id=${taskId}`, { headers });
      const queryData = await queryResp.json().catch(() => ({}));
      if (!queryResp.ok || queryData.error) {
        const msg = queryData?.error?.message ?? queryData?.base_resp?.status_msg ?? `HTTP ${queryResp.status}`;
        return { completed: true, error: `查询失败：${msg}` };
      }
      const task = queryData.task ?? {};
      switch (task.status) {
        case "succeeded":
          return { completed: true, data: task.content?.url };
        case "failed":
        case "cancelled":
          return { completed: true, error: `视频生成失败：${task.status}` };
        default:
          logger(`[H3] 生成中 status=${task.status}`);
          return { completed: false };
      }
    },
    10000,
    15 * 60 * 1000,
  );

  if (pollResult.error) throw new Error(pollResult.error);
  logger("[H3] 视频生成成功，开始下载转存");
  return await urlToBase64(pollResult.data!);
};

const imageRequest = async (_c: ImageConfig, _m: ImageModel): Promise<string> => {
  throw new Error("本供应商仅提供视频模型，图像请配置其他供应商");
};
const ttsRequest = async (_c: TTSConfig, _m: TTSModel): Promise<string> => {
  return "";
};

exports.vendor = vendor;
exports.textRequest = textRequest;
exports.uploadReference = uploadReference;
exports.videoRequest = videoRequest;
exports.imageRequest = imageRequest;
exports.ttsRequest = ttsRequest;

export {};
```

### 8.3 代码要点（与内置 minimax.ts 的差异）

| 差异点 | 内置 minimax.ts（旧） | 本 vendor（H3 V2） |
|---|---|---|
| 端点 | `/v1/video_generation` + `/v1/query/video_generation` + `/v1/files/retrieve`（file_id 二跳） | `/v2/video_generation` + `/v2/query/video_generation`（直接返回 `task.content.url`，无需二跳） |
| 参考图 | `first_frame_image`/`last_frame_image` 平铺字段（单图/首尾帧） | `content[]` 数组 + `role=reference_image`（多图参考 ≤9） |
| 参考图载体 | 只收 base64 平铺 | `image_url.url` 兼容**公网 URL 与 base64 data URL**（官方文档已确认，总请求体 ≤64MB）。Toonflow 管道只给 vendor base64，故本 vendor 走 data URL 并压缩到 3MB/张 |
| 宽高比 | v1 由图片自然决定 | 纯文生 `ratio` 必填且不可 adaptive；带参考默认 `adaptive`（官方也允许显式指定，本 vendor 固定 adaptive 由参考输入决定） |
| 时长/分辨率 | 6/10 秒、512P/768P/1080P | **4–15 任意整数**、768P/2K |
| 音频 | 无 | 原生 32kHz 立体声（prompt 里的对白/环境音描述会直接生成音轨） |

---

## 九、多图参考视频生成使用要点

1. **选模型**：工作台/分镜视频处选 `MiniMax H3 (多图参考)`，模式切到**多参考模式**（模型 mode 数组声明的能力）。
2. **传参考**：上传/绑定资产图（角色、场景、道具），**上限 9 张**——超出按"角色 > 场景 > 道具"优先保留；vendor 会自动压缩到 ≤3MB/张（保证 9 张满载时请求体不超 V2 的 64MB 限制）。
3. **写提示词（`@图N` 引用式，Toonflow 生产模板）**：
   ```
   [References]
   @图1 : [男主参考图]
   @图2 : [女主参考图]
   @图3 : [场景参考图]

   [Instruction]
   @图1 sitting by the window with coffee, expression tired,
   @图2 walking toward @图1 , slight smile,
   set in the modern café environment of @图3 ,
   medium shot, slow dolly in, cinematic, warm afternoon light,
   no dialogue, café ambience, soft piano.
   ```
   规则：资产按绑定顺序从 `@图1` 连续编号；正文无需描述角色外观（参考图负责）；台词标注 dialogue/OS/VO 且**不翻译**。完整 12 字段生成规则见 `data/modelPrompt/video/universalMulti-parameterMode.md`。
4. **role 互斥**：多参考模式与首尾帧模式不可混用（V2 强校验，混用直接 400）。
5. **比例**：带参考图时画幅由参考输入自适应（`adaptive`），界面的比例选项仅对纯文生视频生效。
6. **计费提醒**：每次提交即按秒计费（768P ¥0.5/s）——10 秒 768P ≈ ¥5/条；失败/审核拦截不扣费。建议先用 768P + 5 秒小样验证 prompt，满意再上 2K/长时长。

---

## 十、日常运维

| 事项 | 操作 |
|---|---|
| **开机自启** | `pm2 startup && pm2 save`（AutoDL 实例重启后进入 SSH 执行 `pm2 resurrect`；或把这两条写进实例的启动脚本） |
| **日志** | `pm2 logs toonflow`；vendor 内部 `logger()` 输出带 `【VM】` 前缀 |
| **省电停用** | 短期不用：AutoDL 控制台**关机**（系统盘免费保留，数据盘少量存储费）；长期不用：备份 `data/` 目录后释放实例 |
| **数据备份** | 全部业务数据（SQLite + 上传素材 + vendor TS）在 `~/autodl-tmp/Toonflow-app/data/`（建议挪数据盘）——tar 打包下载即可迁移/容灾 |
| **实例迁移** | AutoDL「克隆实例」或新机重跑本文第四~六章 + 解压 data/ 备份 |
| **升级 Toonflow** | `git pull` 会**还原 src/app.ts 的 6006 端口改动** → 必须重跑第五章第 2 步的 `sed` 命令，再 `yarn install && yarn build && pm2 restart toonflow`（升级前备份 data/） |
| **换 Key/地址** | 设置中心改表单即时生效（写 `o_vendorConfig.inputValues`），无需重启；vendor 代码改动同样热生效 |
| **成本监控** | MiniMax 控制台「费用中心」按日核对；建议给账户设余额告警 |

---

## 十一、常见问题（FAQ）

| 问题 | 处置 |
|---|---|
| **实例里 docker 命令报错** | 正常，AutoDL 容器不支持嵌套 Docker，用本文 PM2 方式 |
| **页面能开但图片/素材全 404（URL 指向 localhost:10588）** | `NODE_ENV` 不是 `prod`（dev 模式素材 URL 硬编码 localhost）→ pm2.json 的 env 改 `"NODE_ENV": "prod"` 后 `pm2 restart toonflow` |
| **配了 `PORT=6006` 但服务仍监听 10588** | 正常现象——[app.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/app.ts) L188 硬编码端口、不读 `PORT`（README 环境变量表表述有误）；按第五章第 2 步 `sed` 改端口后重新 `yarn build` |
| `yarn install` 卡在 GitHub（sharp 等原生模块下载二进制） | 保持 `source /etc/network_turbo`；或 `yarn config set registry https://registry.npmmirror.com`；sharp 走镜像 `SHARP_DIST_BASE_URL`（npmmirror 有托管）；Electron 相关失败直接用第五章第 0 步剥离 |
| `better-sqlite3` 编译失败 | `apt install -y build-essential python3` 后 `yarn rebuild better-sqlite3` |
| 页面开了但一直转圈/进度不动 | F12 看 WebSocket 是否连接失败——AutoDL 网关偶发不转发 WS，改用 AutoDL-SSH-Tools 端口转发本地访问 `127.0.0.1:6006` |
| 公网地址打不开 | 确认服务监听 `0.0.0.0:6006`（`ss -lntp | grep 6006`）、「自定义服务」协议选 http、实例未关机 |
| H3 提交返回 400 "content must include a non-empty text item" | prompt 为空；纯文生也必须有 text 项 |
| 400 提示 role 混用/互斥 | 该请求同时出现 reference_* 与 first/last_frame；本 vendor 只走 reference，检查是否手改了 vendor 或前端传参 |
| 429 限流 | 按量档并发保守，串行提交；间隔 30s+ 或联系 MiniMax 商务提额 |
| 生成慢/超时（>15min） | 2K + 多参考最慢；先用 768P 试；vendor 轮询超时已设 15min，仍超时查 MiniMax 控制台任务状态 |
| 想要 2K 但想省 GPU | 本文方案 2K 走官方 API 直出（¥0.8/s），**无需 GPU**；仅当需要"自部署 H3-Base 768P + 官方 Regenerate-2K"混合链路时才租 GPU（另见文档 01 路径 A） |
| 忘记 admin 密码 | SQLite 在 `data/` 下，按 README/仓库 issue 处理；或备份后重置实例重装 |

---

## 附：与平台（duanju）方案的关系

本文部署的 Toonflow 用于**参考验证与内容生产试验**（验证 H3 多图参考在短剧流程的真实效果、成本、prompt 模板有效性）；验证结论（prompt 模板、参数档位、成本数据）将反哺 duanju 平台自身的 `MiniMaxH3Provider` 实现（文档 01 第五、六章）。两条线共用同一把 MiniMax API Key 与同一套 V2 契约。
