# MiniMax H3 视频生成模型对接 AutoDL

> **Workspace:** d:\develop\a_work\duanju

---

**User**

调查一下 MiniMax H3 多图参考提示词的视频生成模板，完善一下 20 视频生成模型。接下来，我要进行与平台的对接，是通过AutoDL来调用 MiniMax H3，你去问一下需要什么条件，我去管理端进行配置，包括 API Key 等需要什么条件。 

**Assistant**

文档已读完。现在开始调研 MiniMax H3 的多图参考模板与 AutoDL 接入条件。

已拿到官方 V2 API 规格。现在深入调研 AutoDL 侧的接入方式和多图参考提示词模板。

已获取完整技术资料。最后核实 AutoDL 侧部署/调用 MiniMax H3 的具体条件。

补充确认 AutoDL 普通实例的外网服务暴露方式。

调研完成，信息充足。现在重写完善文档——修正旧文档中过时的 API 字段（H3 实际用 V2 接口 `duration`/`ratio`/`content` 数组，而非旧版 `seconds`/`aspect_ratio`），补充 AutoDL 双路径接入条件与多图参考提示词模板。

写入超时，文件仍是旧内容。重试写入。

文档已完成完善：[01 - MiniMax H3 按秒计费视频生成接入技术方案.md](file:///d:/develop/a_work/duanju/docs/20视频生成模型/01%20-%20MiniMax%20H3%20按秒计费视频生成接入技术方案.md)。以下是调研结论与你要去准备的条件。

## 关键调研结论

**1. AutoDL 没有“托管的 MiniMax H3 API”**——AutoDL 是 GPU 云，所谓“通过 AutoDL 调用 H3”= 租 GPU 实例 + 自部署开源权重（H3 已于 2026-08-03 开源）。旧文档写的“AutoDL 控制台开通模型服务”不存在，已修正为双路径方案。

**2. 修正了旧文档的 API 字段错误**：H3 走 **V2 接口**（`POST /v2/video_generation` + `GET /v2/query/video_generation`），字段是 `duration`（整数 4–15，非 seconds）、`ratio`（非 aspect_ratio）、`resolution` 枚举 `768P/2K`（没有 P720/P1080）、V2 无需 GroupId。多图参考通过 `content[]` 数组 + `role=reference_image`（≤9 张），与首尾帧 role 互斥。

**3. 新增第五章“多图参考提示词模板”**：核心规则是 prompt 中必须逐图声明用途（“图1：男主外貌参考，保持面部/发型/体型一致…”），六段式模板（任务画风→图序约定→镜头语言→表演节拍→台词音频→约束）已按短剧场景写好，可直接映射到 `LoadShotRefs` 的角色/场景/道具图。

## 管理端配置所需条件（你去准备）

**路径 A：AutoDL 自部署（主推，成本=GPU租金）**
| 条件 | 内容 |
|---|---|
| 账号 | autodl.com 注册 + **个人实名**（企业认证仅弹性部署 API 需要） |
| 充值 | GPU 按量计费，建议机型 **4×A100/H100 80G**（单 Ref2VA 检查点推理） |
| 数据盘 | ≥200GB，下载 `MiniMaxAI/MiniMax-H3` 权重（国内走 ModelScope） |
| BaseURL | SGLang 监听 **6006 端口** → 控制台“自定义服务”复制的公网地址（`https://xxx.seetacloud.com:8443`） |
| **API Key** | **AutoDL 不发 Key**——是 SGLang 启动参数 `--api-key` 我们自定的 32 位随机令牌，录入 `model_keys` |

**路径 B：MiniMax 官方 API（2K 直出，按秒计费 768P 0.5 元/s、2K 0.8 元/s）**
| 条件 | 内容 |
|---|---|
| 账号 | platform.minimaxi.com 注册 + 实名 |
| API Key | 「账户管理→接口密钥」创建，仅明文展示一次，录入 `model_keys` |
| 充值 | 视频资源包不支持 H3，只能按量余额 |

两条路径请求契约完全相同，Provider 只换 BaseURL+Key，建议管理端各录一条 provider（`minimax-h3-autodl` / `minimax-h3-official`）按 priority 互备。管理端具体录入字段表见文档第三章 3.1。

Sources:
- [MiniMax 视频生成 V2 API 官方文档](https://platform.minimaxi.com/docs/api-reference/video-generation-v2-create)
- [MiniMax H3 开源公告（输入规格/三模块架构）](https://www.minimaxi.com/news/minimax-h3-open-source)
- [MiniMax 按量计费价格页](https://platform.minimaxi.com/docs/guides/pricing-paygo)
- [MiniMax H3 部署全指南：API 调用、SGLang 部署与 Full 2K Workflow](https://www.cnblogs.com/vibecodinghuanzhe/p/22277237)
- [AutoDL 开放端口/自定义服务文档](https://www.autodl.com/docs/port/)
- [AutoDL 容器实例 Pro API 文档](https://www.autodl.com/docs/instance_pro_api/)
- [AutoDL 弹性部署 API 文档（需企业认证）](https://www.autodl.com/docs/esd_api_doc/)

---

**User**

去看一下 refer 里面的 toonflow 的项目，它就能在 auto dl 里面配置并使用 MiniMax h3。

**Assistant**

找到了 Toonflow 项目。看它的 vendor 配置和 MiniMax H3 接入方式。

看到了 vendor 结构，但 `toonflow.ts`（中转平台）和 `minimax.ts`（官方 Hailuo v1）里都没有 H3。搜一下项目里 AutoDL 和 H3 相关的内容。

代码里没有 AutoDL 直连痕迹（H3 命中多为十六进制误报）。看一下 serve/app.js、README 部署方式、以及多图参考提示词模板（正好对应我们文档第五章）。

vendor 机制和多图参考模板已明确。再看 vendor 的加载/配置机制和 README，确认用户如何在 UI 里配置自定义 endpoint。

README 揭示了核心机制：“可编程供应商系统——设置中心直接编写供应商 TypeScript 即时生效”。读部署与供应商配置细节。

最后确认 vendor 的运行时加载机制（设置中心热编辑 TS 如何生效）。

查一下 Toonflow 是否有 ComfyUI 对接（智星云教程里启动了 ComfyUI，这可能是本地跑 H3 的关键）。

调研完成，机制已完全弄清。更新文档，补充 Toonflow 参考实现章节和 `@图N` 引用模板。

现在我将 Toonflow `@图N` 提示词模板变体添加到第 5.2 节。

调研完成，文档已更新（新增 2.4 Toonflow 参考调研 + 5.2 `@图N` 模板变体）。

## Toonflow 调研结论

看完 [Toonflow-app](file:///d:/develop/a_work/duanju/refer/Toonflow-app/data/vendor/toonflow.ts) 后，**它“在 AutoDL 里配置并使用 MiniMax H3”的真实机制**是这样的：

**1. Toonflow 本身不在 AutoDL 上跑 H3 推理。** 它是一个纯 Node.js 编排服务（Express + SQLite），README 的智星云教程（与 AutoDL 同类的 GPU 云）模式是：租 4090 → Windows 镜像内置 Toonflow → RDP 登录 → **在设置中心填外部模型的 API Key + 请求地址**。GPU 云只提供桌面环境，生成调用仍走远程 HTTP API。

**2. 核心是“可编程供应商系统”。** 每个 vendor 是一个 TS 文件（[vm.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/src/utils/vm.ts) 用 vm2 沙盒热加载），在设置中心**直接编辑代码即时生效，无需改源码重启**。其内置的 [minimax.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/data/vendor/minimax.ts) 只有旧 Hailuo 2.3/02（v1 接口），**并没有 H3**——要用 H3 就是新写/改一个 vendor 指向 H3 的 V2 端点。vendor 只需配两样东西：`apiKey` + `baseUrl`。

**3. 两个可直接复用的生产验证资产：**
- **多参考请求格式互相印证**：[toonflow.ts](file:///d:/develop/a_work/duanju/refer/Toonflow-app/data/vendor/toonflow.ts) L524-551 的 Seedance 多参考代码，请求格式 `references: [{role:"reference_image", type:"image_url", image_url:{url}}]` 与我们文档 5.1 的 H3 V2 组装完全一致。
- **`@图N` 提示词模板**：[universalMulti-parameterMode.md](file:///d:/develop/a_work/duanju/refer/Toonflow-app/data/modelPrompt/video/universalMulti-parameterMode.md) 的两段式结构（`[References]` 逐图声明 + `[Instruction]` 正文内嵌 `@图N` 引用，按分镜 12 字段自动渲染）已吸收进文档 5.2，比纯六段中文模板更适合 Agent 批量生成。

**对我们管理端配置的最终答案**（不变，且被 Toonflow 模式验证）：只需配 `BaseURL + API Key + model=MiniMax-H3` 三要素。你可以先走 **MiniMax 官方 API**（platform.minimaxi.com 注册→接口密钥→充值，2K 0.8 元/秒）最快跑通；AutoDL 自部署（SGLang + Ref2VA 权重 + 6006 端口映射）作为后续降本路径，两者对 Provider 是同一个 V2 契约，随时切换。

---

**User**

我要AutoDL 自部署和toonflow一样，给我完善部署文档

**Assistant**

需要先确认 Toonflow 的构建/运行细节和许可证，再写部署文档。

再确认 vendor 配置的存储/编辑机制（设置中心如何保存自定义 vendor TS）。

机制已全部确认（vendor 热编辑存 `<data>/vendor/<id>.ts`，设置中心有 addVendor/updateCode 路由）。现在写 AutoDL 部署文档。

---

