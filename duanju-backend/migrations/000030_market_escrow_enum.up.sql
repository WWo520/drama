-- UP: point_txn_type_enum 扩展任务市场托管流水类型（08 审查 Critical#5 修复）。
-- 注意：PG 不允许在新增枚举值的同一事务内引用该值；本文件仅做枚举扩展，Go 代码侧消费。
ALTER TYPE point_txn_type_enum ADD VALUE IF NOT EXISTS 'MARKET_FREEZE';
ALTER TYPE point_txn_type_enum ADD VALUE IF NOT EXISTS 'MARKET_UNFREEZE';
ALTER TYPE point_txn_type_enum ADD VALUE IF NOT EXISTS 'MARKET_REWARD_DEBIT';
ALTER TYPE point_txn_type_enum ADD VALUE IF NOT EXISTS 'MARKET_REWARD_CREDIT';
