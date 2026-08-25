# LuxReal 空间（团队）与积分机制调研 —— 与本项目对比

> 创建日期：2026-08-22
> 状态：调研完成 + 差距已识别（未实施）
> 依据：`docs/00 - 参考文档/02 - luxreal/08`（D1 用户与团队协作、D6 商业化）、`09`（B2.8/B3 商业化清单、M0/M1 里程碑）、本项目 `duanju-backend/migrations` 与 `internal/domain/space/`、`docs/01 - 设计方案/02 - 数据库设计.md` B1/B8 章

---

## 一、LuxReal 的空间是怎么设计的

### 1.1 组织模型（3 张表）

| 表 | 关键字段 | 说明 |
|----|---------|------|
| teams | owner_id（创建者）、name、logo_url、**member_limit**（最大成员数=套餐席位）、**storage_limit_gb** | 空间本体，配额由套餐决定 |
| team_members (M:N) | team_id+user_id 唯一、**role**（OWNER/ADMIN/EDITOR/VIEWER）、invited_by | 成员关系+角色 |
| users.team_id | NULL=个人 | 用户归属单一团队 |

- 角色枚举完整集：`OWNER / ADMIN / EDITOR / MEMBER / VIEWER / BILLING_ONLY（仅看账单）`；
- 项目多态归属：`projects.owner_type = USER/TEAM`；模板/风格也有 `TEAM_PUBLIC/TEAM_SHARED` scope——**技能与样式可团队级共享**；
- 演进节奏：M0 全砍（无 teams 表、owner_type 强制 USER）→ M1 加 teams/team_members +「**团队协作 5 席免费**」套餐权益。

### 1.2 积分设计：加入空间后积分怎么算？

**结论：团队共享积分池（1 团队 = 1 账户），创建者不向成员分配积分。**

| # | 证据 | 含义 |
|---|------|------|
| 1 | user_points 表注释「账户主表（**1用户/团队=1行**）」，owner_type+owner_id UNIQUE（USER/TEAM） | 成员加入后**没有个人配额**，全部消耗从团队共享池扣 |
| 2 | point_transactions.txn_type 24+ 种（充值10/消耗14/冻结解冻/冲销），**无任何"成员配额划拨/积分转移"类型** | 机制上不存在"创建者给成员分积分" |
| 3 | teams.member_limit / storage_limit_gb 来自套餐 | **创建者用套餐控席位与存储，不用积分控成员** |
| 4 | BILLING_ONLY 角色（仅看账单） | 计费是 owner/admin 的事，普通成员只管用 |
| 5 | 充值入口：积分包/月卡充入账户（owner_type=TEAM 时即团队池）；批量生成走"冻结→成功扣/失败退" | 团队消费与个人消费同一套结算管线，仅账户归属不同 |

即：**加入空间 = 获得共享池使用权 + 席位内资格**；钱由创建者/管理员充进团队池，成员按需消耗，平台用 BILLING_ONLY 角色+流水满足对账需求。

---

## 二、与本项目（duanju）对比

### 2.1 高度一致项（架构已对齐，无需改动）

| 维度 | LuxReal | 本项目 | 评价 |
|------|---------|--------|------|
| 空间本体 | teams | `spaces`（PERSONAL/TEAM 双类型、owner_user_id、quota_storage_bytes/quota_member_count/quota_compute_config、is_default 个人默认空间） | ✅ 我们多一层 PERSONAL 空间统一模型，更完整 |
| 成员+角色 | team_members + 6档角色 | `space_members` + `user_role_enum`（OWNER/ADMIN/EDITOR/MEMBER/VIEWER/**BILLING_ONLY**，逐字一致） | ✅ 完全对齐 |
| 邀请 | invited_by | `space_invitations`（token、7天过期、accepted_by） | ✅ 我们更完整（独立邀请表） |
| 积分账户 | user_points：1用户/团队=1行 | `point_accounts` owner_type USER/**SPACE**、UNIQUE(owner_type,owner_id)；仅 TEAM 空间可建 SPACE 账户，PERSONAL 空间强制用提交者 USER 账户 | ✅ 完全对齐且约束写明 |
| 创建者不分配积分 | 无划拨类型流水 | 同样无成员配额机制；团队消费直接扣 SPACE 账户 | ✅ 理念一致 |
| 席位/存储由套餐控 | member_limit/storage_limit_gb | `packages.member_limit/storage_quota_gb` + `spaces.quota_*` + 9.11 空间扩容（积分购买） | ✅ 对齐且多了积分扩容通道 |

### 2.2 我们比 LuxReal 更细的地方（优势，保留）

1. **扣费账户固化快照**：`generation_tasks.charged_account_id/charged_owner_type/charged_owner_id` 在任务提交时固化（个人空间=USER、团队空间=SPACE），退费时「不得按当前成员关系或用户账户重新解析」——LuxReal 文档未覆盖"成员退出后的结算竞态"，我们已封死；
2. **操作者归因双索引**：`billing_records/point_transactions` 记 space_id + operator_user_id + 复合索引，支持"谁在空间里花了多少"汇总/明细；已有接口 2.13~2.17（团队共享账户/成员消耗汇总/成员明细/本人明细）+ 9.3/9.4 团队账户与流水；
3. **不可变流水约束**：成员退出/角色变更/空间改名不得更新既有流水；团队报表仅聚合 SPACE+DEBIT+已结算流水，可经 task_id→point_txn_id 回溯对账。

### 2.3 后端接口现状（已实现，`internal/domain/space/handler.go`）

```
2.1 空间列表 / 2.2 创建团队空间 / 2.3 详情 / 2.5 切换 / 配额
2.6 成员列表 / 2.7 添加成员 / 2.8 改角色 / 2.10 移除 / 2.11 转让
2.8 邀请链接 / 2.8a 接受邀请
9.3 团队账户 / 9.4 团队流水 / 成员消费报表
2.13 团队共享积分账户 / 2.14 团队充值订单 / 2.15 成员消耗汇总
2.16 成员消耗明细 / 2.17 本人消费明细 / 9.11 空间扩容
```

---

## 三、问题清单（真正的差距项）

| # | 问题 | 详情 | 优先级 |
|---|------|------|--------|
| S1 | **前端空间管理 UI 未验证/可能缺失** | 后端接口齐全，但 web-user 是否有空间切换器、成员管理页、邀请页、团队账单页（对齐 LuxReal M1 `/teams/*`、`/invite/*` 路由）需专项核查；缺失则补齐 | 🔴 待核查 |
| S2 | **团队席位套餐权益闭环** | LuxReal「团队协作 5 席免费」是套餐权益；我们 packages.member_limit 字段在，但需验证：购买套餐→spaces.quota_member_count 升级的联动是否打通（含 quota_purchase 积分扩容路径） | 🟡 |
| S3 | **团队级技能/风格/模板共享** | LuxReal 模板/风格有 TEAM_PUBLIC/TEAM_SHARED scope +「团队内 Skill 可共享，经验沉淀为一键复用资产」；需核查我们 skills/templates/styles 是否有 space scope，缺失则 P2 补 | 🟢 P2 |
| S4 | **BILLING_ONLY 角色的界面适配** | 枚举已有该角色，但前端账单页/余额展示对该角色的可见性（只看账单、不能创作消费）需验证权限矩阵覆盖 | 🟢 P2 |
| S5 | **跨集资产复用豁免的团队口径** | LuxReal 强调"资产参演所有剧集不重复扣费"；我们已有 cross_episode_reuse_exempt_amount 字段，但团队空间下多成员并发批量生成的豁免判定与展示需回归验证 | 🟡 |

### 实施建议

```
第一批：S1 前端空间管理核查（无 UI 则后端能力全部空转，最高优先）
第二批：S2 套餐-席位联动验证 + S5 团队豁免回归
第三批：S3 团队级技能共享 + S4 BILLING_ONLY 权限矩阵（P2，观察需求）
```

> 总评：空间与积分的**数据模型与后端结算本项目已与 LuxReal 对齐甚至更严谨**（扣费快照、操作者归因、不可变流水）；差距主要在**前端界面落地**与**团队级内容共享**两块。LuxReal 把团队版放 M1 的节奏同样适用于我们：单人闭环跑稳后再开团队功能。
