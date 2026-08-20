// 认证仓储：users / spaces / space_members / point_accounts / user_preferences 的写入与查询。
package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound 用户不存在。
var ErrNotFound = errors.New("user not found")

// ErrDuplicate 邮箱/手机号已注册。
var ErrDuplicate = errors.New("account already exists")

// UserRow users 表行。
type UserRow struct {
	ID             int64
	UserUUID       string
	DefaultSpaceID *int64
	Email          *string
	Phone          *string
	PasswordHash   string
	Nickname       string
	AvatarURL      *string
	Status         string
	VipLevel       int16
}

// PointRow point_accounts 行。
type PointRow struct {
	CurrentBalance int
	FrozenBalance  int
}

// Repo auth 模块数据访问。
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// CreateUserWithSpace 注册事务：用户 + 个人空间 + OWNER 成员 + 积分账户 + 偏好 + 初始积分。
// email/phone 至少提供一个（05 ReqAuthRegister oneOf）。
// initialPoints 从 platform_configs.initial_points 读取（后台可配置，非硬编码）。
func (r *Repo) CreateUserWithSpace(ctx context.Context, email, phone, passwordHash, nickname string) (*UserRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 读初始积分配置（config_value 为 JSONB 数字，::text 转 int）
	var initialPoints int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(config_value::text::int, 500) FROM platform_configs WHERE config_key='initial_points'`,
	).Scan(&initialPoints); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if initialPoints == 0 {
		initialPoints = 500
	}

	var u UserRow
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, phone, password_hash, nickname)
		VALUES (NULLIF($1, ''), NULLIF($2, ''), $3, $4)
		RETURNING id, user_uuid, default_space_id, email, phone, password_hash, nickname, avatar_url, status, vip_level`,
		email, phone, passwordHash, nickname,
	).Scan(&u.ID, &u.UserUUID, &u.DefaultSpaceID, &u.Email, &u.Phone, &u.PasswordHash, &u.Nickname, &u.AvatarURL, &u.Status, &u.VipLevel)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}

	// 个人空间
	var spaceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO spaces (name, type, owner_user_id, is_default)
		VALUES ($1, 'PERSONAL', $2, TRUE) RETURNING id`,
		nickname+" 的个人空间", u.ID,
	).Scan(&spaceID); err != nil {
		return nil, err
	}
	// 成员（OWNER）
	if _, err := tx.Exec(ctx, `
		INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, 'OWNER')`,
		spaceID, u.ID); err != nil {
		return nil, err
	}
	// 回填 default_space_id
	if _, err := tx.Exec(ctx, `UPDATE users SET default_space_id=$1 WHERE id=$2`, spaceID, u.ID); err != nil {
		return nil, err
	}
	u.DefaultSpaceID = &spaceID
	// 积分账户 + 初始积分
	if _, err := tx.Exec(ctx, `
		INSERT INTO point_accounts (owner_type, owner_id, current_balance, lifetime_earned_total)
		VALUES ('USER', $1::bigint, $2::int, $2::bigint)`,
		u.ID, initialPoints); err != nil {
		return nil, err
	}
	// 积分流水（首笔：注册赠送）
	if _, err := tx.Exec(ctx, `
		INSERT INTO point_transactions (account_id, owner_type, owner_id, txn_type, direction, amount, balance_after, description, idempotency_key)
		SELECT id, 'USER', $1::bigint, 'ADMIN_COMP', 'CREDIT', $2::int, current_balance, '注册赠送积分', 'register-grant-'||$1::text
		FROM point_accounts WHERE owner_type='USER' AND owner_id=$1::bigint`,
		u.ID, initialPoints); err != nil {
		return nil, err
	}
	// 用户偏好
	if _, err := tx.Exec(ctx, `INSERT INTO user_preferences (user_id) VALUES ($1)`, u.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByAccount 按邮箱或手机号查询（登录用）。
func (r *Repo) GetByAccount(ctx context.Context, account string) (*UserRow, error) {
	var u UserRow
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_uuid, default_space_id, email, phone, password_hash, nickname, avatar_url, status, vip_level
		FROM users
		WHERE deleted_at IS NULL AND (email = $1 OR phone = $1)`,
		account,
	).Scan(&u.ID, &u.UserUUID, &u.DefaultSpaceID, &u.Email, &u.Phone, &u.PasswordHash, &u.Nickname, &u.AvatarURL, &u.Status, &u.VipLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUUID 按 user_uuid 查询（/auth/me、中间件鉴权后取用户）。
func (r *Repo) GetByUUID(ctx context.Context, userUUID string) (*UserRow, error) {
	var u UserRow
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_uuid, default_space_id, email, phone, password_hash, nickname, avatar_url, status, vip_level
		FROM users
		WHERE deleted_at IS NULL AND user_uuid = $1`,
		userUUID,
	).Scan(&u.ID, &u.UserUUID, &u.DefaultSpaceID, &u.Email, &u.Phone, &u.PasswordHash, &u.Nickname, &u.AvatarURL, &u.Status, &u.VipLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateProfile 资料部分更新（nil 字段不更新；avatar 存 MinIO object key）。
func (r *Repo) UpdateProfile(ctx context.Context, userID int64, nickname, avatarKey *string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET
			nickname = COALESCE($2, nickname),
			avatar_url = COALESCE($3, avatar_url),
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`,
		userID, nickname, avatarKey)
	return err
}

// VerifyOwnedObject 校验对象归属：本人上传且已 CONFIRMED（头像引用安全检查）。
func (r *Repo) VerifyOwnedObject(ctx context.Context, userID int64, objectKey string) error {
	var one int
	err := r.pool.QueryRow(ctx, `
		SELECT 1 FROM s3_object_metadata
		WHERE object_key = $1 AND created_by_user_id = $2 AND upload_status = 'CONFIRMED'`,
		objectKey, userID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// GetAvatarKey 取头像 object key（读取时签新 URL 重定向用）。
func (r *Repo) GetAvatarKey(ctx context.Context, userUUID string) (string, error) {
	var key string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(avatar_url, '') FROM users WHERE deleted_at IS NULL AND user_uuid = $1`,
		userUUID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}

// GetPoints 查询积分账户。
func (r *Repo) GetPoints(ctx context.Context, userID int64) (*PointRow, error) {
	var p PointRow
	err := r.pool.QueryRow(ctx, `
		SELECT current_balance, frozen_balance FROM point_accounts WHERE owner_type='USER' AND owner_id=$1`,
		userID,
	).Scan(&p.CurrentBalance, &p.FrozenBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return &PointRow{}, nil
	}
	return &p, err
}

// GetDefaultSpaceUUID 查询用户默认空间对外 UUID（UserMeDTO.default_space_id）。
func (r *Repo) GetDefaultSpaceUUID(ctx context.Context, userID int64) (string, error) {
	var spaceUUID string
	err := r.pool.QueryRow(ctx, `
		SELECT s.space_uuid::text FROM spaces s
		JOIN space_members m ON m.space_id = s.id AND m.user_id = $1 AND m.deleted_at IS NULL
		WHERE s.is_default = TRUE AND s.deleted_at IS NULL ORDER BY s.id LIMIT 1`,
		userID,
	).Scan(&spaceUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return spaceUUID, err
}

// UpgradePasswordHash bcrypt → Argon2id 透明升级。
func (r *Repo) UpgradePasswordHash(ctx context.Context, userID int64, hash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, hash, userID)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
