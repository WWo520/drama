# 08 - 技术架构与数据库 Schema 设计方案（P0 级）

> **文档定位**：从竞品分析阶段正式进入「工程化落地」的第一份技术设计文档。解决"程序员团队看了前 7 份文档还是不知道怎么建表、怎么拆服务、怎么调度长任务"的核心空白。
>
> **前 7 份文档关联索引**：
> - 业务规则依据：[03 - 短剧Agent工作台深度分析](file:///c:/Users/qsevn/Desktop/Code/duanju/docs/00%20-%20参考文档/02%20-%20luxreal/03%20-%20短剧Agent工作台深度分析.md)
> - 自由画布依据：[07 - 自由画布架构与教程体系深度挖掘报告](file:///c:/Users/qsevn/Desktop/Code/duanju/docs/00%20-%20参考文档/02%20-%20luxreal/07%20-%20自由画布架构与教程体系深度挖掘报告.md)
> - 商业化规则：[04 - 商业化与会员体系分析](file:///c:/Users/qsevn/Desktop/Code/duanju/docs/00%20-%20参考文档/02%20-%20luxreal/04%20-%20商业化与会员体系分析.md)
> - 功能补全依据：[06 - 遗漏信息全面审查与补充 → F节 129~136号](file:///c:/Users/qsevn/Desktop/Code/duanju/docs/00%20-%20参考文档/02%20-%20luxreal/06%20-%20遗漏信息全面审查与补充.md#L357-L437)

---

## A. 技术选型总览（2026 年主流稳定栈 + AI 场景特化）

| 层级 | 技术选型 | 选型理由 |
|------|---------|---------|
| **前端 Web** | Next.js 14 (App Router) + TypeScript 5.5 + React 19 + Tailwind CSS 3 + shadcn/ui + Zustand + ReactFlow（自由画布） | - ReactFlow 天然适合做节点式工作流编辑器（完美适配自由画布架构）<br>- shadcn/ui 组件齐全（Modal/Drawer/Toast/Slider/Progress 覆盖 90% 组件需求） |
| **前端 App（可选）** | Tauri 2.0 或 Electron（后期桌面端） | 自由画布重度用户需要桌面端体验 |
| **后端 API 网关** | Go (Gin) + Kong / APISIX | Go 高并发低延迟，统一鉴权/限流/熔断/灰度 |
| **核心业务服务** | Python 3.12 + FastAPI + SQLAlchemy 2 + Pydantic v2 | AI 生态全部是 Python，统一语言减少跨语言损耗 |
| **异步任务调度** | Celery 5.4 + Redis 7 (Broker) + Flower | 最成熟的长任务队列，支持重试/死信/ETA，完美匹配视频生成 5-10 分钟长任务 |
| **AI 模型集成层** | vLLM（自研双 LLM 部署）+ 各家官方 SDK + 统一 ModelAdapter 抽象 | vLLM 推理吞吐比原生 HF 高 5-10x；Adapter Pattern 封装方便替换 |
| **3D 片场服务** | Node.js + Three.js / Babylon.js + 3DGS Python Server | WebSocket 推送实时渲染结果 |
| **主数据库（OLTP）** | PostgreSQL 16 + pgvector（可选） | JSONB 支持最好（画布存节点图）；全文检索/复杂事务 |
| **缓存层** | Redis 7 Cluster（16 槽） | Session/进度缓存/热点资产/幂等 key/分布式锁 |
| **对象存储** | MinIO / 阿里云 OSS / S3 + Cloudflare R2（冷备） | S3 协议兼容海量小文件存储 |
| **CDN** | Cloudflare / 阿里云 CDN | 生成的 MP4/PNG CDN 分发降源站带宽 |
| **搜索（后期）** | MeiliSearch | 作品/风格/资产全局搜，部署轻量中文好 |
| **监控 & 可观测性** | Prometheus + Grafana + Sentry + Jaeger | 指标面板/错误上报/分布式链路追踪 |
| **部署 & DevOps** | K8s 1.30 + Helm + GitHub Actions CI/CD + ArgoCD（GitOps） | GPU 节点 HPA 自动扩缩容 |

---

## B. 整体系统架构图（6 层架构 ASCII）

```
┌────────────────────────────────────────────────────────────────────────────┐
│ L1 客户端接入层                                                             │
│ [Web Next.js] [桌面 Tauri] [Mobile H5] [第三方 OpenAPI(Webhook回调)]        │
└──────────┬───────────────┬─────────────┬──────────────────────────┬────────┘
           ▼ HTTPS/WSS     ▼             ▼ HTTPS                  ▼ HTTPS
┌────────────────────────────────────────────────────────────────────────────┐
│ L2 统一 API 网关 (APISIX/Kong)                                              │
│ 统一鉴权 │ 限流熔断 │ IP黑白名单 │ CORS │ 请求ID注入 │ 日志审计              │
└──────────────────────────────┬─────────────────────────────────────────────┘
                               ▼ 路由分发
          ┌────────────────────┴─────────────────────┐
          ▼                                          ▼
┌─────────────────────────────┐         ┌─────────────────────────────────────┐
│ L3A 同步业务层 FastAPI(Monolith) │     │ L3B 异步 AI 生成服务层 Celery Worker │
│ 用户服务│项目服务*│画布服务│资产服务  │ │ worker-llm(剧本拆分/分镜/润色)        │
│ 商业化服务*│样式库服务│团队协作│任务查 │ │ worker-image(角色/场景/道具图生成)   │
│ (同步不做生成，只写DB+发Celery) │  │ Broker Redis 队列 │ worker-video(视频生成+拼接)│
└─────────────┬───────────────┘         │ worker-3d(3DGS实时渲染)             │
              ▼ 读写                     │ GPU节点池(HPA自动伸缩 2→30)          │
┌─────────────────────────────────────────────────────────────────────────────┐
│ L4 数据存储层                                                                │
│ [PostgreSQL 16 + 30表+JSONB+pgvector] [Redis 7 Cluster] [MinIO/S3 对象]      │
│ [MeiliSearch 全局搜]                                                         │
└────────────┬───────────────────────────────┬─────────────────────────────────┘
             ▼                               ▼ 预签名上传/下载
┌─────────────────────────────────────────────────────────────────────────────┐
│ L5 AI 模型服务层 Model Serving                                               │
│ [LuxChat5.2 vLLM] [LuxGem3.0 vLLM] [Seedance2.5VIP/2.0/Fast + LuxImage2]    │
│ [Kling O3 可插拔] [Lux Banana2 一致性图生] [3DGS 虚拟影棚服务]               │
└─────────────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────────────┐
│ L6 基础设施 & 可观测性                                                        │
│ K8s+Helm+ArgoCD │ Prometheus+Grafana │ Sentry │ Jaeger │ Cloudflare CDN      │
└─────────────────────────────────────────────────────────────────────────────┘
```
> 标 `*` 初期合并到 Monolith，后期流量上来再拆。**前期不要过度微服务化。**

---

## C. 微服务拆分 & 异步任务调度架构

### C1. 服务清单（初期：1 Monolith + 4 类 Worker）

| 服务名 | 类型 | 职责范围 | 节点类型 |
|--------|------|---------|---------|
| **api-main** | 同步HTTP | 鉴权/项目CRUD/剧本拆分触发/资产查询/商业化充值/自由画布存取/样式库/团队权限/任务状态查询 | CPU节点 HPA 2-50 |
| **worker-llm** | 异步Celery | Step1剧本拆分(LuxChat5.2) / Step3分镜脚本生成 / 单视频提示词润色(LuxGem3.0) | GPU(A10/A100) HPA 2-20 |
| **worker-image** | 异步Celery | Step2角色/场景/道具图生成(Lux Banana2) / 3D片场参考图/全景图生成 | GPU 可复用LLM池 |
| **worker-video** | 异步Celery | 智能镜头预演抽锚 / 单镜头视频生成(Seedance/Kling) / Step4多段拼接 | GPU大显存(A100 80G优先) HPA 1-30 |
| **worker-3d** | WSS异步 | 3D片场实时渲染WebSocket通信 / 3DGS虚拟影棚机位取图 | GPU+部分CPU |
| **scheduler** | 定时 | 订单超时取消/积分过期清理/冷数据归档/成本日报 | CPU单节点主备 |

### C2. 剧本→成片 核心异步调用链路
```
[用户]POST /projects→[api-main]写projects/episodes→POST /split-script(202)
  → send_task('llm.split_script') → [worker-llm]调LuxChat5.2
  → 写入beats/shot_groups/shots → 预写characters/scenes/props空壳
  → generation_tasks=SUCCESS → WebSocket推送Step1→Step2跳转

[用户Step2点批量生成资产]→POST /assets/batch-generate(202)
  → [api-main]校验积分→冻结frozen_points→send_task('image.generate_batch')
  → [worker-image]逐条Banana2生成→存MinIO→更新asset.image_url
  → 全部完成后解冻积分，成功扣/失败退→写point_transactions
  → WebSocket实时推%进度

[Step3/Step4同理：worker-llm分镜规划→worker-video生成视频]
```

### C3. 生成任务通用状态机（generation_tasks）
```
PENDING(初始) → QUEUED(入队) → RUNNING(执行)
                        ┌─────────┼─────────┐
                        ▼         ▼         ▼
                    SUCCESS    FAILED    CANCELED
                               (重试<3)   (用户/超时)
                                 └─► QUEUED(指数退避15s/60s/180s)
                                         └─失败≥3→DEAD_LETTER(死信人工)
```
> 积分规则：仅 SUCCESS 扣积分；FAILED/CANCELED 永不扣，冻结余额退回。

## D. 核心数据库 Schema（30 张表完整设计）

PostgreSQL 16。主键 BIGSERIAL；软删除统一 `deleted_at TIMESTAMPTZ NULL`，全部索引加 `WHERE deleted_at IS NULL` 部分索引。

---

### D1. 用户与团队协作（5 张表：01~05）
#### 01 users
| 字段 | 类型 | 默认 | 索引 | 说明 |
|------|------|------|------|------|
| id | BIGSERIAL | PK | PK |  |
| team_id | BIGINT | NULL | BTREE | 所属团队（空=个人） |
| email | VARCHAR(128) | NULL | UNIQUE PARTIAL | 邮箱登录 |
| phone | VARCHAR(20) | NULL | UNIQUE | 手机登录 |
| password_hash | VARCHAR(255) | NOT NULL | - | bcrypt/argon2 |
| nickname | VARCHAR(64) | NOT NULL | - | 显示名 |
| avatar_url | VARCHAR(512) | NULL | - | 头像 |
| role_in_team | ENUM_USER_ROLE | 'MEMBER' | - | OWNER/ADMIN/EDITOR/VIEWER |
| vip_level | INT | 0 | BTREE | 0免费~4至尊（对应[04号四档月卡](file:///c:/Users/qsevn/Desktop/Code/duanju/docs/00%20-%20参考文档/02%20-%20luxreal/04%20-%20商业化与会员体系分析.md)） |
| vip_expire_at | TIMESTAMPTZ | NULL | BTREE | 过了自动降为0 |
| is_active | BOOLEAN | true | BTREE | 冻结用户 |
| created_at / updated_at | TIMESTAMPTZ | NOW() | BTREE/- |  |
| deleted_at | TIMESTAMPTZ | NULL | PARTIAL | 软删 |

#### 02 teams
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| owner_id | BIGINT | BTREE | FK users.id（团队创建者） |
| name | VARCHAR(128) | - | 团队名 |
| logo_url | VARCHAR(512) | - |  |
| member_limit | INT | - | 最大成员数（对应套餐团队席位） |
| storage_limit_gb | INT | - | 存储空间上限 GB |

#### 03 team_members (M:N)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| team_id + user_id | BIGINT | UNIQUE PARTIAL(team_id,user_id) |  |
| role | ENUM_USER_ROLE | - | OWNER/ADMIN/EDITOR/VIEWER |
| invited_by | BIGINT | - | 邀请人 user_id |

#### 04 api_keys (OpenAPI)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| user_id | BIGINT | BTREE |  |
| key_hash | VARCHAR(128) | UNIQUE | sha256 |
| key_prefix | VARCHAR(16) | - | 展示前缀 sk-xxxx |
| name | VARCHAR(64) | - | 用户备注名 |
| rate_limit_per_min | INT | - | 默认60 |
| expires_at | TIMESTAMPTZ | - | NULL=永久 |
| is_active | BOOLEAN | - |  |
| last_used_at | TIMESTAMPTZ | - |  |

#### 05 webhook_endpoints (OpenAPI回调)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| user_id | BIGINT | BTREE |  |
| url | VARCHAR(1024) | - | https回调 |
| secret | VARCHAR(64) | - | HMAC签名秘钥 |
| events | JSONB | - | 订阅事件列表(task.completed等) |

---

### D2. 项目流水线（5层一对多：projects→episodes→beats→shot_groups→shots）
#### 06 projects (整部短剧=1行)
| 字段 | 类型 | 默认 | 索引 | 说明 |
|------|------|------|------|------|
| id | BIGSERIAL | PK | PK |  |
| owner_type+owner_id | ENUM+BIGINT | USER | BTREE复合 | USER/TEAM + user_id/team_id |
| user_id | BIGINT | NOT NULL | BTREE | 创建者统计用 |
| title | VARCHAR(256) | NOT NULL | BTREE | 例"【LuxReal官方案例】一语成谶" |
| cover_image_url | VARCHAR(512) | NULL | - | 作品页缩略图 |
| project_type | ENUM_PROJECT_TYPE | SHORT_DRAMA_AGENT | BTREE | SHORT_DRAMA_AGENT / FREE_CANVAS / ECOMMERCE_TEMPLATE / IP_DANCE_TEMPLATE / FILM_CINEMATOGRAPHY_TEMPLATE |
| total_duration_sec | INT | 0 | - | 全项目总时长秒 |
| source_template_id | BIGINT | NULL | BTREE | FK canvas_templates |
| current_step | ENUM_PROJECT_STEP | SCRIPT_SPLIT | - | Step1~4 / CANVAS_EDITING |
| status | ENUM_PROJECT_STATUS | DRAFT | BTREE | DRAFT / PARTIAL_ASSETS / ALL_ASSETS_READY / PARTIAL_VIDEOS / ALL_VIDEOS_READY / COMPLETED / ARCHIVED |
| tags | JSONB | [] | GIN | 搜索标签 |
| extra_config | JSONB | {} | - | 水印/字幕/SLA等 |
| last_opened_at | TIMESTAMPTZ | NOW() | BTREE | 作品页排序用 |
| **复合索引** | - | - | (owner_type,owner_id,deleted_at) PARTIAL | "我的项目"列表主查询 |

#### 07 episodes (剧集)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id + sort_order | BIGINT+INT | BTREE(project_id,sort_order) | 第几集1/2/3 |
| title | VARCHAR(256) | - | 例"第1集 - 一语成谶" |
| preset_total_duration | INT | - | NULL=自动集级预设 |
| actual_duration_sec | INT | - | 所有镜头加总 |
| original_script_text | TEXT | - | 该集剧本最多5万字 |
| script_file_url | VARCHAR(512) | - | 上传原件留档 |
| current_step | ENUM_PROJECT_STEP | - | 单集进度可跨集不同步 |
| status | ENUM_PROJECT_STATUS | - | 单集状态 |

#### 08 beats (叙事单元Step1拆分产物)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| episode_id + sort_order | BIGINT+INT | BTREE | 叙事单元序号1/2/3 |
| title | VARCHAR(256) | - | 例"陆言开场说书" |
| preset_duration_sec | INT | - | NULL=自动单元级预设 |
| actual_duration_sec | INT | - | 镜头合计 |
| script_original_text | TEXT | - | 剧本原文全文(Step1展开内容) |
| linked_asset_count | INT | - | **关联资产数**(重新规划弹窗显示) |
| planning_mode | ENUM_PLANNING_MODE | - | MULTI_GRID_PRO / MULTI_GRID_FAST / VIDEO_FRAME_PRE_ANCHOR(智能镜头预演抽锚) |

#### 09 beat_versions (叙事单元历史版本 → 「历史版本」按钮)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| beat_id + version_num DESC | BIGINT+INT | BTREE | 版本号v1/v2递增 |
| label | VARCHAR(128) | - | 用户自定义标签例"导演版初稿" |
| script_original_snapshot | TEXT | - | 当时剧本快照 |
| storyboard_json_snapshot | JSONB | - | 当时分镜脚本完整JSON(镜头块+镜头) |
| created_by | BIGINT | - | FK users.id |
| created_from | ENUM_VERSION_FROM | - | AGENT_AUTO / AGENT_REPLAN(重新规划) / USER_EDIT / VERSION_RESTORE |

#### 10 shot_groups (镜头块表=HeroShotGroup TypeA/B)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| beat_id + sort_order | BIGINT+INT | BTREE | 镜头块序号 |
| group_type | ENUM_SHOT_GROUP_TYPE | - | HERO_SHOT_GROUP(TypeA候选包) / NAMED_SEGMENT(TypeB例"陆言数钱藏钱遇异响") |
| internal_code | VARCHAR(64) | BTREE | 例"第1集-叙事单元2-HeroShotGroup" |
| title | VARCHAR(256) | - | 成片时长开头显示标题 |
| duration_sec | INT | - | 成片时长(例10s/13s) |
| pre_anchor_video_url | VARCHAR(512) | - | 智能镜头预演抽锚视频 |
| generated_video_url | VARCHAR(512) | - | 最终成片视频(Step3卡片头图) |
| reference_images_json | JSONB | - | 参考图列表[{shot_pos,name,url,asset_ref}] |
| video_generation_status | ENUM_GENERATION_STATUS | BTREE |  |

#### 11 shots (单镜头表，镜头块内2-4个)
| 字段 | 类型 | 说明 |
|------|------|------|
| id / shot_group_id+sort_order | BIGSERIAL / BTREE | 镜头序号1/2/3/4 |
| duration_sec | INT | 秒(例3/2/9) |
| shot_size | ENUM_SHOT_SIZE | 景别9种：ECU/CU/MCU/MS/FS/LS/ELS/TWO_SHOT/OTS |
| camera_movement | ENUM_CAMERA_MOVE | 运镜18种：STATIC/SLOW_PUSH_IN/DOLLY_IN/PAN_LEFT/TRACK_LEFT/TILT_UP/CRANE_UP/HANDHELD_SHAKY/WIPE_CLIP等 |
| camera_angle | ENUM_CAMERA_ANGLE | 视角10种：EYE_LEVEL_FRONT/SIDE/BACK/SLIGHT_LOW/LOW/SLIGHT_HIGH/HIGH_BIRDVIEW/OTS_LEFT/OTS_RIGHT/DUTCH |
| subject_description | TEXT | 主体描述(人物动作表情走位) |
| background_description | TEXT | 背景(市井古街/夜间烛光) |
| local_light | TEXT | **局部光影**(门缝火光晃动/火把红光扫脸) |
| dialogue_text | TEXT | 纯台词(无修饰) |
| dialogue_meta_json | JSONB | 修饰{speaker,emotion,role_tag画外音,tone_loudness} |
| ambient_sound | TEXT | 环境音(嘈杂市井/门闩咔声) |
| prompt_original | TEXT | 润色前原始提示词(字段组合) |
| prompt_refined | TEXT | **LuxGem3.0润色后→送视频模型** |
| reference_images | JSONB | 本镜头参考图[{pos,img_url,asset_id,type:char/scene/prop/3dgs}] |
| linked_assets_json | JSONB | @绑定详情{char_ids:[1,2],scene_id:5,prop_ids:[7,8]} |
| ai_model | ENUM_VIDEO_MODEL | BTREE SEEDANCE_2_5_VIP等 |
| video_generation_status | ENUM_GENERATION_STATUS | BTREE |
| generated_video_url | VARCHAR(512) | 最终MP4 |
| deleted_at | TIMESTAMPTZ | PARTIAL索引软删 |

---

### D3. 资产层（7 张表：角色/场景/道具/智能镜头候选/通用素材/复用判定流水）
#### 12 characters (角色主表)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id | BIGINT | BTREE |  |
| owner_of_asset_origin_episode_id | BIGINT | BTREE | **资产首次出现集数(跨集复用判定核心)** |
| code | VARCHAR(64) | UNIQUE(project_id,code) | SHEN_QING_HE / LU_YAN |
| name | VARCHAR(128) | BTREE | 沈青禾 / 陆言 |
| appearance_base_prompt | TEXT | - | 不变部分：性别/年龄/五官/体型/肤色/发型发色 |
| ai_model_for_image | ENUM_IMAGE_MODEL | - | 默认LUX_BANANA_2 |
| tags | JSONB | GIN | 男/女/老/少/古风/现代/主配反 |
| description_notes | TEXT | - | 备注 |

#### 13 character_variants (角色变体表-服装/造型/年龄)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| character_id | BIGINT | BTREE | FK characters |
| variant_name | VARCHAR(128) | - | 干练老板娘装 / 市井说书装 / 捕头公服 |
| variant_type | ENUM_CHAR_VARIANT | - | COSTUME/HAIRSTYLE/AGE/INJURY/OTHER |
| clothing_desc | TEXT | - | 浅蓝色衬衫+深灰西装+干练知性 |
| extra_appearance_prompt | TEXT | - | 在base_prompt上追加(OR运算符拼接) |
| generation_params | JSONB | - | seed/步数/CFG |
| main_image_url | VARCHAR(512) | - | Step2卡片缩略图9:16竖 |
| multi_view_images_json | JSONB | - | {front,side_left,side_right,back,upper_body,close_up_face} |
| image_resolution | ENUM_IMAGE_RESOLUTION | - | 1K/2K/4K 默认2K |
| aspect_ratio | ENUM_IMAGE_ASPECT_RATIO | - | R9_16角色竖 |
| generation_status | ENUM_GENERATION_STATUS | BTREE |  |
| point_cost | INT | - | 失败=0 |
| generation_task_id | BIGINT | BTREE | FK |

#### 14 scenes (场景表含日夜变体)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id | BIGINT | BTREE |  |
| origin_episode_id | BIGINT | BTREE | 首次出现集(复用判定) |
| code | VARCHAR(64) | UNIQUE(project_id,code) | SHIJING_GUJIE / CHAGUAN / LUYAN_YARD |
| base_name | VARCHAR(128) | - | 场景基础名市井古街 |
| time_of_day_variant | ENUM_TIME_OF_DAY | BTREE | DAY白日闹市/SUNSET_DUSK暮色夜景/NIGHT_CANDLELIT夜间烛光/DAWN/NOON/GOLDEN_HOUR |
| weather | ENUM_WEATHER | - | 晴雨雪雾 |
| base_prompt | TEXT | - | 中式古代青石板古街两侧木建筑招牌 |
| extra_prompt_for_variant | TEXT | - | 变体补充：夜晚红灯笼点亮石板湿润反光 |
| ai_model_for_image | ENUM_IMAGE_MODEL | - | LUX_BANANA_2 |
| main_image_url | VARCHAR(512) | - | 主图16:9横 |
| aspect_ratio | ENUM_IMAGE_ASPECT_RATIO | - | R16_9场景横 |
| image_resolution | ENUM_IMAGE_RESOLUTION | - | 2K |
| generation_status | ENUM_GENERATION_STATUS | BTREE |  |
| point_cost | INT | - |  |
| generation_task_id | BIGINT | BTREE |  |
| three_d_scene_id | BIGINT | BTREE | **若已建3DGS虚拟影棚FK**three_d_scenes.id |

#### 15 props (道具表)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id | BIGINT | BTREE |  |
| origin_episode_id | BIGINT | BTREE | 首次出现集 |
| code | VARCHAR(64) | UNIQUE(project_id,code) | TONG_GONG铜锣/ZHE_SHAN折扇 |
| name | VARCHAR(128) | - | 铜锣/折扇 |
| category | ENUM_PROP_CATEGORY | - | WEAPON/CLOTHING/FOOD/BOOK/JEWELRY/FURNITURE/ELECTRONIC/VEHICLE/TOOL/TOY/PLANT/LIGHTING/OTHER |
| description_prompt | TEXT | - | 黄铜制中式小铜锣直径30cm轻微磨损痕 |
| main_image_url | VARCHAR(512) | - | 主图1:1正方 |
| aspect_ratio | ENUM_IMAGE_ASPECT_RATIO | - | R1_1道具强制1:1 |
| image_resolution | ENUM_IMAGE_RESOLUTION | - | 2K |
| generation_status | ENUM_GENERATION_STATUS | BTREE |  |
| point_cost | INT | - |  |
| generation_task_id | BIGINT | BTREE |  |

#### 16 smart_shot_groups (智能镜头库候选包TypeA HeroShotGroup)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| beat_id | BIGINT | BTREE | 属于哪个叙事单元 |
| package_code | VARCHAR(64) | - | 例"叙事单元2-HeroShotGroup" TypeA |
| internal_named_label | VARCHAR(256) | - | TypeB具体命名"陆言数钱藏钱遇异响" |
| anchor_video_url | VARCHAR(512) | - | 预演抽锚视频(一致性来源) |
| anchor_frames_json | JSONB | - | 抽出关键帧锚点图列表 |
| selected_as_final | BOOLEAN | BTREE | 用户是否选中这套为最终分镜 |
| generation_status | ENUM_GENERATION_STATUS | - |  |

#### 17 common_assets (通用素材库UUID素材-NH6DJYAKTL...png等)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id | BIGINT | BTREE |  |
| origin_uuid | VARCHAR(64) | UNIQUE(project_id,origin_uuid) | 原始UUID文件名 |
| name | VARCHAR(256) | - | 用户可手动改别名 |
| asset_type | ENUM_COMMON_ASSET_TYPE | - | IMAGE/VIDEO/AUDIO/FONT/LUT/MASK |
| file_url | VARCHAR(512) | - |  |
| file_size_bytes | BIGINT | - |  |
| mime_type | VARCHAR(64) | - |  |
| width_px / height_px | INT | - | 图/视频尺寸 |
| duration_sec | NUMERIC(12,3) | - | 音视频时长秒 |
| user_uploaded | BOOLEAN | - | true用户上传 false AI沉淀 |
| created_by | BIGINT | - | FK users.id |

#### 18 asset_usage_logs (跨集复用判定流水表-计费最核心!)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| asset_kind | ENUM_ASSET_KIND | BTREE | CHARACTER_VARIANT / SCENE / PROP / SMART_SHOT_GROUP |
| asset_id | BIGINT | BTREE(asset_kind,asset_id,episode_id) | 对应各资产表PK |
| project_id | BIGINT | BTREE |  |
| episode_id | BIGINT | BTREE | **这次使用在第几集** |
| is_first_time_use_project_wide | BOOLEAN | - | **TRUE=全项目首次→扣；FALSE=跨集复用→0元不扣** |
| is_first_time_use_episode_wide | BOOLEAN | - | 集内首次(集内多次复用也不扣) |
| point_cost_applied | INT | - | 实际扣费0或N |
| generation_task_id | BIGINT | BTREE |  |
| user_notified | BOOLEAN | - | 是否已在批量弹窗大字告知用户 |
| **UNIQUE约束** | - | UNIQUE(asset_kind,asset_id,episode_id) | 同资产同集只记1次防重 |

---

### D4. 生成任务层（3 张表：任务/重试/导出）
#### 19 generation_tasks (任务主表)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| task_uuid | UUID | UNIQUE | 对外暴露防遍历 |
| user_id | BIGINT | BTREE | 发起者 |
| project_id | BIGINT | BTREE |  |
| episode_id | BIGINT | BTREE |  |
| task_type | ENUM_TASK_TYPE | BTREE | 14种：SCRIPT_SPLIT/STORYBOARD_PLAN/CHAR_IMAGE_GEN/SCENE_IMAGE_GEN/PROP_IMAGE_GEN/SMART_SHOT_PRE_ANCHOR/PROMPT_REFINEMENT/VIDEO_SEGMENT_GEN/VIDEO_GROUP_CONCAT/3DGS_SCENE_BUILD/3DGS_RENDER_IMAGE/3DGS_RENDER_PANORAMA/FINAL_VIDEO_COMPOSE/ENGINEER_FILE_EXPORT |
| asset_kind | ENUM_ASSET_KIND | BTREE |  |
| target_id | BIGINT | BTREE | 目标character_variant.id/shot.id等 |
| parent_task_id | BIGINT | BTREE | 父任务(批量拆分出子任务) |
| status | ENUM_GENERATION_STATUS | BTREE(updated_at DESC) PARTIAL | 主索引(Worker轮询) NOT IN SUCCESS/FAILED_DEAD |
| priority | SMALLINT | - | 0~9 VIP自动提权到7/8 |
| ai_model_requested / actual_used | VARCHAR(64) | - | 用户选的 vs 实际跑(降级时变) |
| request_params_json | JSONB | - | prompt/seed/步数/分辨率/参考图数组快照 |
| progress_pct | SMALLINT | - | 0-100 WS实时推 |
| progress_message | VARCHAR(512) | - | 阶段文字：预Anchoring/第1次去噪/解码中 |
| retry_count | SMALLINT | - | 最大3 |
| result_output_json | JSONB | - | {image_url,video_url,thumbnails,metadata} |
| error_code | VARCHAR(32) | BTREE | MODEL_TIMEOUT/CONTENT_VIOLATION/NSFW_BLOCKED/OUT_OF_VRAM/INVALID_PROMPT |
| error_message / user_display_error | TEXT/VARCHAR(512) | - | 前者堆栈(内部)后者用户友好提示 |
| queue_wait_ms / execution_ms / total_execution_ms | INT | - | 耗时统计ms |
| gpu_node_id | VARCHAR(128) | - | 哪个Worker节点名(排查) |
| celery_task_id | VARCHAR(64) | - | Flower追踪 |
| point_cost_final | INT | - | **最终扣积分(成功才>0,失败=0)** |
| idempotency_key | VARCHAR(128) | UNIQUE | 幂等key防重复点排队 |
| created_at | TIMESTAMPTZ | BTREE |  |
| started_at / completed_at | TIMESTAMPTZ | BTREE/BTREE | Worker取/结束时间 |

#### 20 generation_task_retry_logs (重试日志)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| task_id | BIGINT | BTREE |  |
| retry_num | SMALLINT | - | 第几次1/2/3 |
| from_status / to_status | ENUM_GENERATION_STATUS | - |  |
| reason | VARCHAR(256) | - | MODEL_TIMEOUT/网络抖动 |
| backoff_seconds | INT | - | 指数退避秒数 |

#### 21 final_export_logs (成片/工程文件导出记录)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| user_id | BIGINT | BTREE |  |
| project_id | BIGINT | BTREE |  |
| export_type | ENUM_EXPORT_TYPE | BTREE | FINAL_VIDEO_MP4 / ENGINEER_FILE_JSON / ENG_ZIP(含素材打包) / SRT_SUBTITLE / PNG_FRAMES / FCPXML_PREMIERE |
| export_range | JSONB | - | {episode_ids:[1,2], beat_ids:null, shot_group_ids:null} |
| config_resolution | ENUM_VIDEO_RESOLUTION | - | P480/P720/P1080 |
| config_watermark | BOOLEAN | - | 免费用户强制true |
| config_subtitle_language | VARCHAR(8) | - | zh-CN/en-US NULL无字幕 |
| output_file_url | VARCHAR(512) | - | 下载地址 |
| output_file_size_bytes | BIGINT | - |  |
| point_cost | INT | - | 合成额外扣费默认0 |
| status | ENUM_GENERATION_STATUS | BTREE |  |
| task_id | BIGINT | BTREE | FK generation_tasks |
| downloaded_at | TIMESTAMPTZ | - | 用户首次下载时间 |
| expire_at | TIMESTAMPTZ | - | OSS保留30天默认 |

---

### D5. 自由画布层（5 张表 = 项目/节点/边/模板/快照）
#### 22 canvas_projects (每个自由画布项目1条)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id | BIGINT | UNIQUE | 一对一FK projects (project_type=FREE_CANVAS) |
| source_agent_project_id | BIGINT | BTREE | **从哪个Agent流水线转来→双向溯源Agent↔Canvas** |
| name | VARCHAR(256) | - | 画布名：电商创意短片 / 从「一语成谶」转画布 |
| description | TEXT | - |  |
| canvas_version | INT | - | 结构版本号(前端升级兼容) |
| thumbnail_url | VARCHAR(512) | - | 画布缩略图30s自动截一张首页卡片用 |
| viewport_json | JSONB | - | {zoom:0.8,panX:120,panY:-50,miniMapVisible:true} 下次打开恢复视角 |
| grid_config | JSONB | - | {size:20,visible:true,snap:true} 网格20px/显示/吸附 |
| total_nodes / total_edges | INT | - | 节点/连线数量卡片统计 |
| last_saved_at | TIMESTAMPTZ | - | 30s自动存+Ctrl+S主动 |
| saved_by | BIGINT | - | FK users.id |

#### 23 canvas_nodes (节点表图数据库节点-ReactFlow key)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| canvas_project_id | BIGINT | BTREE | FK |
| node_uuid | UUID | UNIQUE | ReactFlow节点key |
| node_type | ENUM_CANVAS_NODE_TYPE | BTREE | 28种: START/END/INPUT_TEXT/INPUT_IMAGE_UPLOAD/LLM_STORYBOARD_GEN_LUXCHAT/ASSET_EXTRACTOR/CHARACTER_IMAGE_GEN/**VIDEO_SEEDANCE_GEN**/CONDITION_IF_BRANCH/LOOP_N_TIMES/MANUAL_HUMAN_REVIEW/COMMENT_NOTE_STICKY/SHAPE_RECT/SHAPE_ARROW/CANVAS_TEMPLATE_REF等 |
| title | VARCHAR(256) | - | 1、输入产品图和产品类型 |
| subtitle | VARCHAR(256) | - | 1.2 输入产品名字(灰小字) |
| node_category_display | ENUM_CANVAS_NODE_CATEGORY | - | 分类颜色:IO_INPUT(蓝)/AI_PROCESSING(紫)/MEDIA_GEN(橙)/FLOW_CONTROL(青)/HUMAN_REVIEW(黄)/IO_OUTPUT(绿)/ANNOTATION(灰) |
| color_skin | VARCHAR(32) | - | 节点配色(对应category) |
| position_x / position_y | NUMERIC(14,3) | - | 画布坐标(px/grid) |
| width_px / height_px | INT | - | 四向拉伸后实际宽高 |
| z_index | INT | - | 叠放顺序 |
| collapsed / locked | BOOLEAN | - | 折叠(查看全文前)/锁定(防误改) |
| config_payload_json | JSONB | - | **节点内部大JSON**(不同type结构不同-见F1节示例) |
| generation_history_json | JSONB | - | 节点右上角「生成历史Released▼」版本快照列表 |
| linked_generation_task_ids | BIGINT[] | - | 关联任务ID列表 |
| linked_asset_ids | JSONB | - | {char_variant_ids:[],scene_ids:[],prop_ids:[]} |
| comment_note | TEXT | - | 用户便签(Shapes注释框) |

#### 24 canvas_edges (节点连线表-有向边)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| canvas_project_id | BIGINT | BTREE |  |
| edge_uuid | UUID | UNIQUE |  |
| source_node_uuid | UUID | BTREE | 起点FK canvas_nodes.node_uuid |
| source_port | VARCHAR(64) | - | out/success_out/fail_out/output_video_0 |
| target_node_uuid | UUID | BTREE | 终点uuid |
| target_port | VARCHAR(64) | - | in/prompt_in/image_in |
| edge_type | ENUM_EDGE_TYPE | - | DATA_FLOW/CONTROL_FLOW_IF/CONTROL_FLOW_LOOP/DEPENDENCY |
| edge_style | VARCHAR(32) | - | BEZIER贝塞尔/STRAIGHT直线/STEP直角 |
| color / animated | VARCHAR(16)/BOOLEAN | - | 自定义色/跑任务时光效动画 |
| label | VARCHAR(128) | - | 连线上文字(if分支的"是"/"否") |

#### 25 canvas_templates (画布模板库)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| template_code | VARCHAR(64) | UNIQUE | ECOMMERCE_SHORT_AD/IP_DANCE_MIGRATION/FILM_CINEMATOGRAPHY/MULTI_GRID_PRO/DRAMA_80_EPISODE_STANDARD(官方5个) |
| scope | ENUM_TEMPLATE_SCOPE | BTREE | PUBLIC_OFFICIAL(官方推荐27个同级) / TEAM_PUBLIC / USER_PRIVATE / MARKETPLACE_PAID(市集付费) |
| category | VARCHAR(64) | BTREE | 电商/短剧/营销/教程/通用 |
| name | VARCHAR(256) | BTREE | "电商创意短片-头戴式降噪耳机45秒广告" |
| description | TEXT | - |  |
| cover_image_url / preview_video_url | VARCHAR(512) | - | 模板封面/演示成品视频 |
| owner_user_id / owner_team_id | BIGINT | BTREE/BTREE | 官方=NULL |
| price_points | INT | - | 市集积分价格0=免费 |
| canvas_structure_json | JSONB | - | **完整画布深拷贝结构 {nodes:[{type,x,y,config}],edges:[]}** |
| default_models_json | JSONB | - | 默认模型推荐{storyboard:LUX_CHAT_5_2,video:SEEDANCE_2_5_VIP} |
| tags | JSONB | GIN | 标签 |
| used_count / like_count | INT | - | 使用次数/点赞(排序) |
| rating_avg | NUMERIC(3,2) | - | 评分 |
| is_official_recommended / sort_order | BOOLEAN/INT | BTREE/BTREE | 是否顶栏/排序权重 |
| min_vip_level_required | INT | - | 最低VIP等级0=免费 |
| version_num | INT | - | 模板版本(官方更新不破坏老项目) |

#### 26 canvas_auto_save_snapshots (画布防崩溃自动快照)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| canvas_project_id | BIGINT | BTREE(canvas_project_id,created_at DESC) |  |
| saved_by | BIGINT | - | 谁存的 |
| save_type | ENUM_SAVE_TYPE | - | AUTO_30S/AUTO_CHANGE_DEBOUNCE_5S/USER_CTRL_S/USER_SAVE_BTN/VERSION_TAG_USER/PRE_BATCH_RUN |
| version_tag | VARCHAR(64) | - | "V1.0导演审通过版" |
| nodes_snapshot_json / edges_snapshot_json | JSONB | - | 节点/边完整快照 |
| viewport_json | JSONB | - | 视口状态 |
| size_bytes | INT | - | 监控用 |
| **保留策略** | - | - | AUTO类保留最近50条，VERSION_TAG永久保留 |

---

### D6. 商业化+样式库+3D片场（4 张表：27~30）
#### 27 user_points + point_transactions (积分账户+流水二合一说明)
**①账户主表(1用户/团队=1行) user_points**
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| owner_type + owner_id | ENUM + BIGINT | UNIQUE PARTIAL(owner_type,owner_id) | USER/TEAM |
| current_balance | INT | - | **当前可用积分=扣减来源** |
| frozen_balance | INT | - | **冻结积分(批量前冻结→成功扣/失败解冻)** |
| lifetime_earned_total / lifetime_spent_total | INT | - | 历史累计获得/消耗(LTV用) |
| total_generation_count | INT | - | 累计生成次数 |

**②流水子表 point_transactions(每次余额变动1条)**
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| account_id | BIGINT | BTREE(account_id,created_at DESC) | FK user_points.id |
| txn_uuid | UUID | UNIQUE | 对外订单号 |
| txn_type | ENUM_POINT_TXN_TYPE | BTREE | ①充值获得(10种：PURCHASE_POINTS_PACKAGE/SUB_MONTHLY_GRANT/**FIRST_PURCHASE_LIMITED_BONUS首购加赠**/REDEEM_CODE/REFER_FRIEND/DAILY_CHECKIN/BIRTHDAY/OFFICIAL_EVENT/ADMIN_COMP) ②消耗扣减(14种：GEN_CHAR_V/GEN_SCENE/GEN_PROP/GEN_VIDEO_15S_*/GEN_3DGS_RENDER/EXPORT_COMPOSE/TEMPLATE_BUY等) ③冻结解冻(BATCH_GEN_FREEZE/UNFREEZE_SUCCESS/UNFREEZE_PARTIAL_FAIL) ④冲销特殊(AUTO_RETRY_REFUND/CS_WIN/EXPIRATION_DEDUCT/ADMIN_NEG) |
| direction | ENUM_TXN_DIRECTION | - | CREDIT加/DEBIT减/FROZEN冻/UNFREEZE解 |
| amount | INT | - | 变动量永远正 direction表方向 |
| balance_after | INT | - | **变动后余额快照-对账追溯核心** |
| related_order_id | BIGINT | BTREE | FK orders |
| related_generation_task_id | BIGINT | BTREE | FK tasks |
| related_project_id | BIGINT | BTREE | 关联项目 |
| cross_episode_reuse_exempt_amount | INT | - | **因跨集复用豁免积分数-透明化权益** |
| description | VARCHAR(512) | - | 人类可读："第1集资产20个批量生成(16成功/4失败,豁免3个跨集复用)" |

#### 28 orders + subscriptions (订单&订阅二合一)
**orders表关键字段**：id, user_id, order_no(UNIQUE), order_type(SUBSCRIPTION_MONTH/SUBSCRIPTION_YEAR/POINTS_PACKAGE/ONE_TIME)
- plan_code(MONTH_BASIC/MONTH_ADVANCED/MONTH_PREMIUM/MONTH_ULTIMATE/YEAR_DISCOUNT/POINTS_PKG_10K等)
- original_amount_YUAN, discount_amount_YUAN, **first_purchase_bonus_points(首购加赠积分)**
- paid_amount_YUAN, pay_channel(ALIPAY/WECHAT/STRIPE/APPLE), pay_status, paid_at
- points_granted_total, subscription_days_granted
- receipt_url, invoice_requested
- created_at, expired_at(未支付30分钟超时取消)

**subscriptions表关键字段**：id, user_id, plan_code, status(ACTIVE/EXPIRED/CANCELLED/PENDING_RENEWAL)
- renew_strategy(AUTO_RENEW_CONTINUOUS连续包月/MANUAL_ONE_TIME手动续费)
- start_at, current_period_end, cancel_at_period_end(BOOLEAN 连续包月点取消后本周期结束才停)
- monthly_points_grant_balance, next_grant_at, total_cycles_paid

#### 29 user_styles (样式库=27公共风格+用户自定义-三层结构)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| style_code | VARCHAR(64) | UNIQUE | WONG_KAR_WAI/XU_HARK/SHAW_BROTHERS_WUXIA/GENERAL_REALISTIC等共27公共 |
| scope | ENUM_TEMPLATE_SCOPE | BTREE | PUBLIC_OFFICIAL/TEAM_SHARED/USER_PRIVATE/CUSTOM_TRAINED_OWN |
| library_layer | ENUM_STYLE_LAYER | BTREE | **三层结构**OFFICIAL_PUBLIC_LIBRARY(官方公共27个)/USER_MY_LIBRARY(我的收藏)/USER_CUSTOM_CREATED_LIBRARY(自定义训练上传) |
| name | VARCHAR(128) | - | 显示名王家卫美学/徐克美学/邵氏武侠 |
| category | ENUM_STYLE_CATEGORY | BTREE | 11大类：REALISTIC_GENERAL通用写实1号/DIRECTOR_TRIBUTE_WONG导演致敬王家卫2号/XU_HARK徐克3号/SHAW_BROTHERS邵氏武侠4号/GUOFENG_CLASSIC/MODERN_URBAN/CYBERPUNK_NEO_NOIR/SUSPENSE_THRILLER/ROMANCE_IDYLLIC_MIYAZAKI/WAR_PERIOD/COMIC_CARTOON/HORROR_SUSPENSE/YOUTH_CAMPUS/HISTORICAL_COSTUME 完整对应06号102清单顺序 |
| description | TEXT | - | 风格说明剧情色调运镜情绪 |
| thumbnail_url / preview_image_urls / preview_video_url | VARCHAR(512) / JSONB / VARCHAR | - | 缩略图/4-6张预览图hover大图/示例成品 |
| prompt_suffix_static | TEXT | - | 公共风格→所有镜头prompt固定拼接后缀(色调颗粒氛围词) |
| camera_preset_json | JSONB | - | 风格预设运镜偏好(王家卫=手持+慢门抽帧/邵氏=推轨+ZOOM IN)景别偏好/LUT预设ID |
| lighting_preset_id | BIGINT | BTREE | 关联3D片场光影模板库FK |
| creator_user_id / creator_team_id | BIGINT | BTREE/BTREE | 自定义风格创建者(公共=NULL) |
| reference_works | JSONB | - | [{type:MOVIE,name:重庆森林,director:王家卫,year:1994}] |
| suitable_genres / tags | JSONB | - / GIN | 适用短剧类型["悬疑","言情","权谋"]/搜索标签 |
| is_default | BOOLEAN | BTREE | 顶栏默认选中=通用写实TRUE |
| sort_order | INT | BTREE | 排序(完全按06号102 27个顺序王家卫导演类第一) |
| min_vip_level_required | INT | - | 0免费2高级4至尊 |
| used_count | INT | - | 全平台被使用次数(社区热度排序) |
| official_star_rating | NUMERIC(2,1) | - | 官方推荐星0.0~5.0 |

#### 30 three_d_scenes (3D片场-3DGS虚拟影棚)
| 字段 | 类型 | 索引 | 说明 |
|------|------|------|------|
| id | BIGSERIAL | PK |  |
| project_id | BIGINT | BTREE |  |
| corresponding_scene_id | BIGINT | BTREE | 对应scenes.id(市井古街3D版) |
| name | VARCHAR(256) | - | 市井古街-3DGS v2 |
| gs_file_url | VARCHAR(512) | - | 3DGS点云高斯.ply/.splat文件 |
| source_type | ENUM_3D_SOURCE | - | AI_GENERATED_AUTO场景图转3DGS/MANUAL_RECONSTRUCT用户上传照片重建/PURCHASED_MARKETPLACE/OFFICIAL_BUILTIN/EXPORTED_FROM_BLENDER_PLUGIN |
| space_size_meters_json | JSONB | - | {length_m:80,width_m:20,height_m:6}实际物理空间 |
| available_camera_presets_json | JSONB | - | 预置机位[{preset_name:茶馆门口机位1平视,position:[x,y,z],rotation:[rx,ry,rz],fov:50,output_cover_url}] |
| lighting_preset_active_id | BIGINT | - | 当前激活光影方案FK(场景+光影+滤镜组合双引擎) |
| build_status | ENUM_GENERATION_STATUS | BTREE | 3DGS模型构建状态(AI重建跑一会) |
| build_task_id | BIGINT | BTREE | FK tasks |
| total_render_count | INT | - | 累计渲染参考图数(热度) |
| point_cost_per_render_normal | INT | - | 普通图每次积分消耗 |
| point_cost_per_render_panorama | INT | - | 全景图每次(比普通贵5x) |

## E. ER 关系说明 & 全量枚举数据字典

### E1. 核心五层一对多 + 各模块关系
```
users ─ teams ─ team_members
  │
  ├─► projects(owner_type USER/TEAM) 1──────────────────┐
  │     │                                                │
  │     ├─► episodes N ─► beats N ─► shot_groups N ─► shots N
  │     │                   │         │              │
  │     │                   │   beat_versions N      ├─► generation_tasks N
  │     │                   │   每块2-4镜头                  │
  │     │                   │                               │
  │     ├─► characters ──► character_variants N ──┐        │
  │     ├─► scenes N(含time_of_day日夜变体)       ├─► asset_usage_logs N◄─┤跨集复用不扣
  │     ├─► props N                               │
  │     ├─► smart_shot_groups N                   │
  │     └─► common_assets N                       ┘
  │
  ├─► canvas_projects 1 ──► canvas_nodes N
  │                   └─► canvas_edges N
  ├─► canvas_templates ← 加载模板时深拷贝nodes/edges
  ├─► canvas_auto_save_snapshots N
  ├─► user_points(1账户) ──► point_transactions N流水
  ├─► orders N ──► subscriptions N
  ├─► user_styles(27公共+自定义)
  └─► three_d_scenes ──► scenes FK

generation_tasks ──► generation_task_retry_logs
                └─► final_export_logs
```

### E2. 全量枚举类型完整取值（ENUM_ 开头）
#### 用户/团队类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_USER_ROLE | OWNER / ADMIN / EDITOR / MEMBER / VIEWER / BILLING_ONLY（仅看账单） |
| ENUM_OWNER_TYPE | USER / TEAM |
| ENUM_TEMPLATE_SCOPE | PUBLIC_OFFICIAL / TEAM_PUBLIC / TEAM_PRIVATE / USER_PRIVATE / MARKETPLACE_PAID |

#### 项目流水线类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_PROJECT_TYPE | SHORT_DRAMA_AGENT / **FREE_CANVAS** / ECOMMERCE_TEMPLATE / IP_DANCE_MIGRATION / FILM_CINEMATOGRAPHY_TEMPLATE / REAL_TIME_3D_STAGE |
| ENUM_PROJECT_STEP | SCRIPT_SPLIT(Step1) / ASSET_GENERATION(Step2) / VIDEO_PRODUCTION(Step3) / FINAL_PREVIEW(Step4) / CANVAS_EDITING |
| ENUM_PROJECT_STATUS | DRAFT / SCRIPT_SPLITTING / SCRIPT_SPLIT_FAILED / PARTIAL_ASSETS / ALL_ASSETS_READY / VIDEO_GENERATING / PARTIAL_VIDEOS / ALL_VIDEOS_READY / EXPORTING / COMPLETED / ARCHIVED / CANCELLED |
| ENUM_PLANNING_MODE | MULTI_GRID_PRO(九宫格Pro镜头连贯) / MULTI_GRID_FAST(九宫格Fast画风统一) / VIDEO_FRAME_PRE_ANCHOR(智能镜头预演抽锚一致性最强) |
| ENUM_VERSION_FROM | AGENT_AUTO_SPLIT / **AGENT_REPLAN重新规划** / LLM_REFINEMENT / USER_MANUAL_EDIT / VERSION_RESTORE回滚 / BATCH_REGEN |
| ENUM_SHOT_GROUP_TYPE | HERO_SHOT_GROUP(TypeA九宫格候选包) / NAMED_SEGMENT(TypeB具体片段) |

#### 镜头类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_SHOT_SIZE景别9 | ECU极特写/CU特写/MCU近景/MS中景/FS全景/LS远景/ELS大远景/TWO_SHOT双人景/OTS过肩 |
| ENUM_CAMERA_MOVE运镜18 | STATIC/SLOW_PUSH_IN慢慢推进/SLOW_PULL_OUT/FAST_PUSH_IN/DOLLY_IN/DOLLY_OUT/PAN_LEFT横移左/PAN_RIGHT/TRACK_LEFT/TRACK_RIGHT/TILT_UP升/TILT_DOWN/CRANE_UP/CRANE_DOWN/HANDHELD_SHAKY手持摇晃/WIPE_CLIP甩切/ZOOM_IN_OPTICAL/SLOW_360_ORBIT |
| ENUM_CAMERA_ANGLE视角10 | EYE_LEVEL_FRONT平视正面/SIDE侧面/BACK背面/SLIGHT_LOW_ANGLE微仰/LOW_ANGLE仰/SLIGHT_HIGH微俯/HIGH_BIRDVIEW俯拍鸟瞰/OTS_LEFT左肩/OTS_RIGHT右肩/DUTCH_ANGLE倾斜 |

#### 生成任务类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_GENERATION_STATUS(状态机) | PENDING/QUEUED/RUNNING/SUCCESS/FAILED_RETRYING/FAILED_DEAD超限死信/CANCELED_USER用户取消/CANCELED_TIMEOUT超时/SKIPPED跨集复用跳过/PAUSED_USER |
| ENUM_TASK_TYPE14种 | SCRIPT_SPLIT/STORYBOARD_PLAN_V1_LUXCHAT/CHAR_IMAGE_GEN/SCENE_IMAGE_GEN/PROP_IMAGE_GEN/SMART_SHOT_GROUP_PRE_ANCHOR/PROMPT_REFINEMENT_LUXGEM/VIDEO_SEGMENT_GEN/VIDEO_GROUP_CONCAT/3DGS_SCENE_BUILD/3DGS_RENDER_IMAGE/3DGS_RENDER_PANORAMA/FINAL_VIDEO_COMPOSE/ENGINEER_FILE_EXPORT |
| ENUM_ASSET_KIND | CHARACTER_VARIANT/SCENE/PROP/SMART_SHOT_GROUP_ANCHOR/COMMON_ASSET_UPLOAD |

#### 模型类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_VIDEO_MODEL | SEEDANCE_2_5_VIP/SEEDANCE_2_0/SEEDANCE_2_0_FAST/KLING_O3/LUX_VIDEO_ALPHA预留 |
| ENUM_IMAGE_MODEL | LUX_BANANA_2/LUX_IMAGE_2/SDXL_1_0_BASE/FLUX_1_DEV预留 |
| ENUM_IMAGE_RESOLUTION | 1K/2K/4K/8K预留 |
| ENUM_IMAGE_ASPECT_RATIO | R16_9场景横/R9_16角色竖/**R1_1道具正方**/R4_3/R3_4/R21_9_IMAX/R1_2_VERTICAL_PANO/R2_1_HORIZONTAL_PANO |
| ENUM_VIDEO_RESOLUTION | P480/P720/P1080/P2K_2560x1440/P4K_UHD |
| ENUM_CHAR_VARIANT | COSTUME服装/HAIRSTYLE发型/MAKEUP/AGE_PROGRESSION年龄/INJURY_DIRTY伤脏/SPECIAL_OUTFIT_UNIFORM制服/CASUAL便装/FORMAL正装 |
| ENUM_TIME_OF_DAY场景时段 | DAWN_BLUE_HOUR/MORNING_SUNRISE/NOON_HIGH_SUN/DAY_BRIGHT白日闹市/AFTERNOON_GOLDEN_HOUR/SUNSET_DUSK暮色夜景/NIGHT_CANDLELIT夜间烛光/NIGHT_NEON/OVERCAST_DAY/STORM_NIGHT |
| ENUM_WEATHER | CLEAR_SUNNY/CLOUDY_OVERCAST/LIGHT_RAIN/HEAVY_RAIN/SNOW/FOG_MIST/WINDY_DUST/THUNDERSTORM/INDOOR_CONTROLLED |
| ENUM_PROP_CATEGORY16 | WEAPON/CLOTHING_ACCESSORY/FOOD_DRINK/STATIONERY_BOOK/JEWELRY_ORNAMENT/FURNITURE_DECOR/ELECTRONIC_APPLIANCE/VEHICLE/TOOL_MUSICAL_INSTRUMENT/TOY/PLANT_FLOWER/ANIMAL_PET/LIGHTING_SOURCE/DOCUMENT_SCROLL_MAP/FLAG_BANNER/OTHER |

#### 自由画布类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_CANVAS_NODE_TYPE28种 | START/END/INPUT_TEXT/INPUT_IMAGE_UPLOAD/INPUT_VIDEO_UPLOAD/INPUT_AUDIO_UPLOAD/CONDITION_IF_BRANCH/LOOP_N_TIMES/LOOP_UNTIL/PARALLEL_SPLIT/PARALLEL_JOIN/SUBGRAPH_INCLUDE/**LLM_STORYBOARD_GEN_LUXCHAT节点2**/ASSET_EXTRACTOR节点3/**CHARACTER_IMAGE_GEN节点4角色**/SCENE_IMAGE_GEN/PROP_IMAGE_GEN/PROMPT_REFINER_LUXGEM/**VIDEO_SEEDANCE_GEN节点5**/VIDEO_GROUP_CONCAT/FINAL_EXPORT_MP4/EXPORT_ENGINEER_ZIP/**MANUAL_HUMAN_REVIEW_GATE人工审核**/COMMENT_NOTE_STICKY便签/SHAPE_RECT矩形框/SHAPE_ARROW箭头/SHAPE_GROUP_BOX分组框/**CANVAS_TEMPLATE_REF子图引用**/WEBHOOK_CALL/API_CALL |
| ENUM_CANVAS_NODE_CATEGORY7色 | IO_INPUT蓝/AI_PROCESSING紫/MEDIA_GENERATION橙/FLOW_CONTROL青/HUMAN_REVIEW黄/IO_OUTPUT绿/ANNOTATION灰 |
| ENUM_EDGE_TYPE | DATA_FLOW数据流/CONTROL_FLOW_IF控制if分支/CONTROL_FLOW_LOOP循环/DEPENDENCY_ONLY纯依赖/COMMENT_LINK注释连线 |
| ENUM_SAVE_TYPE快照类型 | AUTO_30S/AUTO_ON_CHANGE_DEBOUNCE_5S/USER_CTRL_S/USER_SAVE_BUTTON_CLICK/VERSION_TAG_USER用户打版本/VERSION_TAG_AUTO_BIG_CHANGE/PRE_BATCH_RUN_SNAPSHOT |

#### 商业化&样式库类
| 枚举名 | 取值列表 |
|--------|---------|
| ENUM_EXPORT_TYPE10 | FINAL_VIDEO_MP4/FINAL_VIDEO_GIF/FINAL_VIDEO_WEBM/**ENGINEER_FILE_JSON导出工程文件**/ENGINEER_FILE_FULL_ZIP含素材全打包/SRT_ASS_SUBTITLE_ONLY/PNG_FRAMES_1FPS/PNG_FRAMES_KEYFRAMES_ONLY/XML_FCPXML_PREMIERE_PROJECT/EDL剪辑表 |
| ENUM_POINT_TXN_TYPE4大类40+ | ①充值获得10种：PURCHASE_POINTS_PACKAGE/SUBSCRIPTION_MONTHLY/SUBSCRIPTION_ANNUAL/**FIRST_PURCHASE_LIMITED_BONUS首购加赠**/REDEEM_CODE/REFER_FRIEND/DAILY_CHECKIN/BIRTHDAY/OFFICIAL_EVENT/ADMIN_COMP ②消耗扣减14种：GEN_CHAR_V/GEN_SCENE/GEN_PROP/GEN_SMART_ANCHOR/GEN_VIDEO_15S_SEEDANCE2_5_VIP/GEN_VIDEO_15S_SEEDANCE2_0/GEN_VIDEO_15S_SEEDANCE2_0_FAST/GEN_VIDEO_15S_KLING_O3/GEN_3DGS_NORMAL/PANORAMA/EXPORT_COMPOSE/ENG_ZIP/TEMPLATE_BUY/API_CHARGE ③冻结解冻3种：BATCH_GEN_FREEZE/UNFREEZE_SUCCESS_DEDUCT/UNFREEZE_PARTIAL_FAIL ④冲销4种：RETRY_REFUND/CS_TICKET_WIN/POINT_EXPIRATION_DEDUCT/ADMIN_NEGATIVE |
| ENUM_TXN_DIRECTION4向 | CREDIT加余额/DEBIT扣/FROZEN冻(balance→frozen)/UNFREEZE解(失败退回frozen→balance) |
| ENUM_STYLE_LAYER三层 | OFFICIAL_PUBLIC_LIBRARY官方27个/USER_MY_LIBRARY我收藏/USER_CUSTOM_CREATED_LIBRARY自定义训练上传 |
| ENUM_STYLE_CATEGORY14类 | GENERAL_REALISTIC/DIRECTOR_WONG_KAR_WAI王家卫2号/DIRECTOR_XU_HARK徐克3号/DIRECTOR_SHAW_BROTHERS邵氏武侠4号/GUOFENG_CLASSIC/MODERN_URBAN/CYBERPUNK_NEO_NOIR/SUSPENSE_THRILLER_HITCHCOCK/ROMANCE_IDYLLIC_MIYAZAKI吉卜力田园/WAR_PERIOD_SOVIET苏式战争/COMIC_CARTOON/HORROR_SUSPENSE_JAPANESE日式悬疑/YOUTHS_CAMPUS校园青春/HISTORICAL_COSTUME_SHENGUO_FENG |
| ENUM_3D_SOURCE5种 | AI_AUTO_3DGS_RECONSTRUCT场景图AI自转/PHOTO_RECONSTRUCT_USER用户多照重建/PURCHASED_MARKETPLACE市集购买/OFFICIAL_BUILTIN_LIBRARY官方内置/EXPORTED_FROM_BLENDER_PLUGIN Blender插件导出 |

## F. 自由画布图数据 JSON Schema + 10 个核心 API 骨架

### F1. config_payload_json 按 node_type 结构约定(6核心节点)
```yaml
# node_type: INPUT_IMAGE_UPLOAD (节点1.1「上传产品图像」)
config_payload:
  label: 上传产品图像(耳机3/4侧视图白底最佳)
  field_name: product_main_image
  accept_mime: [image/png,image/jpeg,image/webp]
  max_file_mb: 10; max_count: 4
  require_white_background_tip: true
  ai_enhance_enable: true
  uploaded_files: []  # [{url,name,size,thumb}]

# node_type: INPUT_TEXT (节点1.2「输入产品的名字」)
config_payload:
  label: 输入产品名字 + 一句话卖点描述
  field_name: product_pitch_input
  placeholder: 头戴式降噪耳机剧情广告 核心卖点降噪45dB+HiFi音质
  max_length: 200; char_count_limit: 200
  ai_enhance_button: true  # 右侧「润色」按钮调LuxGem
  user_value: <用户实际填>

# node_type: LLM_STORYBOARD_GEN_LUXCHAT (节点2「生成分镜脚本」)
config_payload:
  label: 分镜脚本生成(点击生成即可)
  llm_model: LUX_CHAT_5_2
  skill_template_code: SCRIPT_TO_STORYBOARD_ECOMMERCE_45SEC
  skill_template_override_prompt_text: null
  target_total_seconds: 45
  segments_count: 3; per_segment_seconds: 15
  aspect_ratio: R9_16
  style_code: GENERAL_REALISTIC_URBAN
  generated_result: null  # 生成后存完整Markdown
  last_run_task_id: null

# node_type: ASSET_EXTRACTOR (节点3「提取资产」)
config_payload:
  label: 3.1 提取人物和场景资产
  auto_extract_from: "{node_uuid_of_LLM_STORYBOARD_2}.generated_result"
  manual_override_allowed: true
  character_slots_count: 4; scene_slots_count: 2; prop_slots_count: 5
  extracted_characters: [{slot_name:职场女生,desc_prompt:...}, ...]
  extracted_scenes: [{slot_name:嘈杂咖啡馆,desc_prompt:...}, ...]
  linked_variant_ids: {char:[], scene:[], prop:[]}

# node_type: VIDEO_SEEDANCE_GEN (节点5「0-15s 生成视频」)
config_payload:
  label: 任务1｜0–15s｜钩子→设定→危机峰值
  segment_time_range: [0, 15]
  ai_model: SEEDANCE_2_5_VIP
  video_resolution: P480; aspect_ratio: R9_16; max_fps: 24
  consistency_mode: PRE_ANCHOR_REFER_FRAMES_STRONG
  reference_images:
    - {asset_id:123, kind:CHARACTER_VARIANT, weight_pct:0.6}
    - {asset_id:45,  kind:SCENE, weight_pct:0.4}
  prompt_original: "年轻职场女生..."
  prompt_refined: null  # LuxGem润色后填
  pre_anchor_task_id: null
  output_video_url: null
  gen_history: [] # 节点右上角「生成历史 Released▼」列表

# node_type: CONDITION_IF_BRANCH (控制流if/else)
config_payload:
  label: CS审核结果分支
  expression_lang: JS_LIKE_SIMPLE
  expression: "{prev_node}.content_safety_passed === true"
  true_label: 通过; false_label: 不通过
  pass_branch_target_edge_uuid: xxx-edge-uuid-1
  fail_branch_target_edge_uuid: xxx-edge-uuid-2
```

### F2. 10 个核心 API 骨架(REST v1 /api/v1)
**API 1. 创建短剧Agent项目(上传剧本)**
```ts
// POST /projects Auth:JWT
type ReqCreateProject = {
  title: string
  project_type: 'SHORT_DRAMA_AGENT' | 'FREE_CANVAS'
  source_template_id?: number
  script_text?: string             // 粘贴 ≤5万字
  script_file_upload_token?: string  // OSS预签名上传token
  config: {  // 模型配置面板
    video_model: 'SEEDANCE_2_5_VIP'|'SEEDANCE_2_0'|'SEEDANCE_2_0_FAST'|'KLING_O3'
    resolution: 'P480'|'P720'
    aspect_ratio: 'R16_9'|'R1_1'|'R9_16'
    per_episode_default_duration_min: 'AUTO'|1|'1-2'|'2-3'|'3-5'|'5-10'
    char_image_model: 'LUX_BANANA_2'
    char_image_resolution: '1K'|'2K'|'4K'
    char_image_ratio: 'R16_9'|'R1_1'|'R9_16'
    scene_image_model: 'LUX_BANANA_2'; scene_resolution: '1K'|'2K'|'4K'; scene_ratio: 'R16_9'
    prop_image_model: 'LUX_BANANA_2'; prop_resolution: '1K'|'2K'|'4K'; prop_ratio: 'R1_1'
    style_code: string  // 默认 GENERAL_REALISTIC
  }
}
type ResCreateProject = { project_id:number; first_episode_id:number
  split_script_task_uuid:string; jump_next_step_url:string }
// 错误码 4001脚本空/4002脚本过长/4003VIP资格不足模型/4004风格不存在
```
**API 2. 批量生成资产图(Step2顶栏按钮)**
```ts
// POST /assets/batch-generate Auth:JWT
type ReqBatchGenAssets = {
  project_id:number; episode_id:number
  character_variant_ids:number[]; scene_ids:number[]; prop_ids:number[]
  smart_shot_group_ids:number[]; common_assets_ids:number[]
  enable_3d_stage_first:boolean  // 默认开
  image_model:'LUX_BANANA_2'
  char_resolution:'1K'|'2K'|'4K'; scene_resolution:同上; prop_resolution:同上
  regenerate_existing_success:boolean  // 已成功过是否强制重抽
  enable_cross_episode_dedup:boolean   // 强制true只读 跨集不扣费
  seed_randomize:boolean; locked_seed_value:number|null
}
type ResBatchGenAssets = { accepted:boolean
  points_preview:{gross_total:number; cross_episode_exempt_count:number;
    deduplication_episode_internal:number; net_to_freeze:number;
    user_current_balance:number; sufficient:boolean}
  batch_id:number; sub_task_uuids:string[]; websocket_room_id:string }
// 错误码 4000积分不够/4005零勾选/40103D片场构建中
```
**API 3. 叙事单元重新规划(Step3重新规划弹窗确定)**
```ts
// POST /beats/:beat_id/replan Auth:JWT
type ReqBeatReplan = {
  active_tab: 'EDIT_STORYBOARD' | 'REGEN_FROM_SCRIPT'
  edited_storyboard_markdown?: string  // Tab1手改
  regen_options?: {  // Tab2重生成分镜
    llm_model:'LUX_CHAT_5_2'; skill_code_override?:string
    planning_mode: 'MULTI_GRID_PRO'|'MULTI_GRID_FAST'|'VIDEO_FRAME_PRE_ANCHOR'
    extract_assets_again:boolean; override_style_code?:string
    rewrite_prompt_suffix?:string; seed_randomize:boolean }
  save_as_new_version:boolean; new_version_label?:string
}
type ResBeatReplan = { task_uuid:string; status:'QUEUED'
  new_version_num_will_be:number|null; estimated_duration_seconds:number; ws_channel:string }
```
**API 4. 保存自由画布(Ctrl+S/30s自动存/离开页面sendBeacon)**
```ts
// POST /canvas-projects/:id/save
type ReqCanvasSave = {
  save_type:'AUTO_30S'|'AUTO_CHANGE'|'USER_CTRL_S'|'USER_SAVE_BTN'|'BEFORE_LEAVE_PAGE'
  version_tag?:string
  nodes: Array<{node_uuid,type,title,position_x,position_y,width_px,height_px,
    collapsed,locked,z_index,config_payload_json,generation_history_json}>
  edges: Array<{edge_uuid,source_uuid,source_port,target_uuid,target_port,
    edge_type,label,style,animated}>
  viewport:{zoom:number;pan_x:number;pan_y:number;mini_map_visible:boolean}
  last_saved_at_iso8601_token:string  // 乐观锁防A/B互覆盖
}
type ResCanvasSave = { saved:boolean; snapshot_id:number; new_last_saved_at_token:string
  conflict_detected:boolean; node_count_saved:number; edge_count_saved:number; file_size_bytes:number }
```
**API 5. 生成任务进度 WebSocket 协议**
- 连接：`WS /ws/v1/tasks/updates?jwt=xxx&room_id=batch-xxx-yyy`
- 客户端发：`SUBSCRIBE/UNSUBSCRIBE task_uuids[]` / `PING`
- 服务端推3种：① TASK_PROGRESS_UPDATE{task_uuid,status,progress_pct,stage_message,elapsed_ms,eta_ms,gpu_node,cost_range}
② TASK_COMPLETED{task_uuid,point_cost_final,user_new_balance,cross_episode_exempt,result:{output_type,url,thumbnails,duration,w,h}}
③ TASK_FAILED{task_uuid,final:boolean,retry_num,backoff_seconds,error_code,user_display_error,point_refunded}

**API 6-10 简要骨架**
| # | Method | URL | 用途 | 核心参数 |
|---|---|---|---|---|
|6|POST|/canvas-projects/run-node|自由画布右键跑单个节点|{node_uuid,input_override}→task_uuid|
|7|POST|/canvas-projects/run-full|自由画布▶Run全流程Start→End|{dry_run,stop_on_human_review:true}→task_uuids|
|8|GET |/payments/recharge-popup-data|充值弹窗-首页/顶栏「充值」|返回VIP剩余+积分余额/4档月卡权益价格加赠/积分包/年卡省51%Banner/积分明细Tab入口|
|9|GET |/styles/library|风格库三层(官方/我的/自定义)|Query layer+category+keyword→27个风格按官方顺序缩略图/VIP等级|
|10|POST|/exports/compose-video|Step4合成视频or导出工程文件|{project_id,episode_ids:[],export_type:'MP4'|'ENG_JSON'|'ENG_ZIP',resolution,watermark,subtitle_lang}→task_uuid+预估积分|

---

## G. 技术落地 3 个里程碑(M0/M1/M2)
| 里程碑 | 周期 | 交付物 | 核心实现表/服务 |
|---|---|---|---|
| **M0 MVP最小可用 (第1~6周)** | 6周 | 剧本上传→Step1拆分→Step2资产生成→Step3分镜+单镜头视频生成→Step4合成下载MP4；无自由画布/无3D片场/无团队版 | users/projects/episodes/beats/shot_groups/shots/characters/variants/scenes/props/generation_tasks/user_points+流水/orders/共15张表；worker-llm/image/video 3类Worker；API 1/2/3/5/10；基础样式库(10种核心风格) |
| **M1 完整核心 (第7~16周)** | 10周 | +历史版本/叙事单元重新规划/跨集复用扣费透明化/完整27风格库/样式库三层结构/团队协作5席免费/充值弹窗/充值支付(支付宝+微信)/自由画布核心(28节点存/读/运行/模板)/节点式编辑器(ReactFlow) | beat_versions/asset_usage_logs/final_export_logs/subscriptions/user_styles/teams/team_members/canvas_projects/canvas_nodes/canvas_edges/canvas_templates/canvas_auto_save_snapshots 共12张；API 4/6/7/8/9；画布乐观锁+版本回滚；智能镜头预演抽锚一致性模式 |
| **M2 高级功能 (第17~28周)** | 12周 | +3D片场实时渲染(3DGS)双引擎(Scene+Lighting)/机位取图/3DGS构建/自由画布多人协同编辑(OT算法)/画布冲突合并面板/OpenAPI开放平台(api_keys/webhooks)/市集付费模板分销/CDN分发全球加速/会员到期自动续费连续包月/作品公开分享页+一键转模板 | three_d_scenes/api_keys/webhook_endpoints/common_assets/smart_shot_groups 共5张(凑齐完整30张表)；worker-3d WS服务；MeiliSearch全站搜；K8s GPU HPA自动扩缩容压测验证；Prometheus+Grafana全链路监控上线 |

> **总计30张表 = D1(5)+D2(6)+D3(7)+D4(3)+D5(5)+D6(4)**，M0完成15张核心50%，M1累计27张90%，M2 30张100%全闭环。



