// 空间仓储：列表/详情/配额/成员校验（owner_type=USER 个人空间，ADR-04 复合索引）。
package space

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound 空间不存在/无权访问。
var ErrNotFound = errors.New("space not found")

// ErrNotMember 非空间成员（越权）。
var ErrNotMember = errors.New("not a space member")

// Row 空间行（含我的角色）。
type Row struct {
	ID         int64
	SpaceUUID  string
	Name       string
	AvatarURL  *string
	Description *string
	Type       string
	OwnerID    int64
	OwnerUUID  string
	MyRole     string
	IsDefault  bool
	Status     string
	MemberCount int
	QuotaStorageBytes int64
	QuotaMemberCount  int
	CreatedAt  string
	UpdatedAt  string
}

// Repo 空间数据访问。
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// ListMine 我的空间列表（我拥有的 + 我加入的，默认空间优先）。
func (r *Repo) ListMine(ctx context.Context, userID int64) ([]Row, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.space_uuid::text, s.name, s.avatar_url, s.description, s.type::text,
		       s.owner_user_id, u.user_uuid::text, m.role::text, s.is_default, s.status::text,
		       (SELECT count(*) FROM space_members sc WHERE sc.space_id = s.id AND sc.deleted_at IS NULL) AS member_count,
		       s.quota_storage_bytes, s.quota_member_count,
		       to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM spaces s
		JOIN space_members m ON m.space_id = s.id AND m.user_id = $1 AND m.deleted_at IS NULL
		JOIN users u ON u.id = s.owner_user_id
		WHERE s.deleted_at IS NULL AND s.status = 'ACTIVE'
		ORDER BY s.is_default DESC, s.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var rw Row
		if err := rows.Scan(&rw.ID, &rw.SpaceUUID, &rw.Name, &rw.AvatarURL, &rw.Description, &rw.Type,
			&rw.OwnerID, &rw.OwnerUUID, &rw.MyRole, &rw.IsDefault, &rw.Status, &rw.MemberCount,
			&rw.QuotaStorageBytes, &rw.QuotaMemberCount, &rw.CreatedAt, &rw.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rw)
	}
	return out, rows.Err()
}

// GetByUUID 按 space_uuid 查详情并校验成员身份（越权返回 ErrNotMember）。
func (r *Repo) GetByUUID(ctx context.Context, spaceUUID string, userID int64) (*Row, error) {
	var rw Row
	err := r.pool.QueryRow(ctx, `
		SELECT s.id, s.space_uuid::text, s.name, s.avatar_url, s.description, s.type::text,
		       s.owner_user_id, u.user_uuid::text, m.role::text, s.is_default, s.status::text,
		       (SELECT count(*) FROM space_members sc WHERE sc.space_id = s.id AND sc.deleted_at IS NULL),
		       s.quota_storage_bytes, s.quota_member_count,
		       to_char(s.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(s.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM spaces s
		JOIN space_members m ON m.space_id = s.id AND m.user_id = $2 AND m.deleted_at IS NULL
		JOIN users u ON u.id = s.owner_user_id
		WHERE s.space_uuid = $1 AND s.deleted_at IS NULL AND s.status = 'ACTIVE'`,
		spaceUUID, userID,
	).Scan(&rw.ID, &rw.SpaceUUID, &rw.Name, &rw.AvatarURL, &rw.Description, &rw.Type,
		&rw.OwnerID, &rw.OwnerUUID, &rw.MyRole, &rw.IsDefault, &rw.Status, &rw.MemberCount,
		&rw.QuotaStorageBytes, &rw.QuotaMemberCount, &rw.CreatedAt, &rw.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rw, nil
}

// UsedStorage 空间已用存储（s3_object_metadata CONFIRMED 汇总）。
func (r *Repo) UsedStorage(ctx context.Context, spaceID int64) (int64, error) {
	var used int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(size_bytes), 0) FROM s3_object_metadata
		WHERE space_id = $1 AND upload_status = 'CONFIRMED' AND deleted_at IS NULL`,
		spaceID,
	).Scan(&used)
	return used, err
}

// CheckMembership 校验成员身份，返回角色；非成员返回 ErrNotMember。
func (r *Repo) CheckMembership(ctx context.Context, spaceID, userID int64) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `
		SELECT m.role::text FROM space_members m
		WHERE m.space_id = $1 AND m.user_id = $2 AND m.deleted_at IS NULL`,
		spaceID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotMember
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

// GetUserIDByUUID 按 user_uuid 查数字主键（供 LoadUserID 中间件注入 ctx）。
func (r *Repo) GetUserIDByUUID(ctx context.Context, userUUID string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM users WHERE user_uuid = $1 AND deleted_at IS NULL`, userUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// ResolveUUID 空间 uuid → id（供中间件/其他模块复用）。
func (r *Repo) ResolveUUID(ctx context.Context, spaceUUID string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM spaces WHERE space_uuid = $1 AND deleted_at IS NULL`, spaceUUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// CreateTeam 创建团队空间 + 空间积分账户 + 添加 OWNER（S2-B01：TEAM 类型；空间账户自动创建）。
func (r *Repo) CreateTeam(ctx context.Context, name, description string, ownerID int64) (*Row, error) {
	var rw Row
	spaceUUID := uuid.NewString()
	// 创建空间
	err := r.pool.QueryRow(ctx, `
		INSERT INTO spaces (space_uuid, name, description, type, owner_user_id, is_default, status,
			quota_storage_bytes, quota_member_count)
		VALUES ($1::uuid, $2, NULLIF($3,''), 'TEAM', $4, FALSE, 'ACTIVE', 5368709120, 10)
		RETURNING id, space_uuid::text, name, COALESCE(avatar_url,''), COALESCE(description,''),
		          type::text, owner_user_id, is_default, status::text,
		          to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		          to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
		spaceUUID, name, description, ownerID,
	).Scan(&rw.ID, &rw.SpaceUUID, &rw.Name, &rw.AvatarURL, &rw.Description,
		&rw.Type, &rw.OwnerID, &rw.IsDefault, &rw.Status, &rw.CreatedAt, &rw.UpdatedAt)
	if err != nil {
		return nil, err
	}
	// 添加创建者为 OWNER
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO space_members (space_id, user_id, role, invited_by_user_id)
		VALUES ($1, $2, 'OWNER', $2) ON CONFLICT (space_id, user_id) WHERE deleted_at IS NULL DO NOTHING`,
		rw.ID, ownerID); err != nil {
		return nil, err
	}
	// 创建空间积分账户（02 B8.1：SPACE 类型账户与 TEAM 空间 1:1）
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO point_accounts (owner_type, owner_id, current_balance, frozen_balance)
		VALUES ('SPACE', $1, 0, 0) ON CONFLICT (owner_type, owner_id) DO NOTHING`,
		rw.ID); err != nil {
		return nil, err
	}
	// 回填用户 UUID
	_ = r.pool.QueryRow(ctx, `SELECT user_uuid::text FROM users WHERE id=$1`, ownerID).Scan(&rw.OwnerUUID)
	rw.MyRole = "OWNER"
	rw.MemberCount = 1
	rw.QuotaStorageBytes = 5368709120
	rw.QuotaMemberCount = 10
	return &rw, nil
}

// AddMember 添加成员（席位校验：成员数 < quota_member_count 才可加入；重复加入幂等返回）。
func (r *Repo) AddMember(ctx context.Context, spaceID, userID, invitedBy int64, role string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 锁空间行取配额
	var quota, memberCount int
	if err := tx.QueryRow(ctx, `
		SELECT quota_member_count, (SELECT count(*) FROM space_members m WHERE m.space_id=spaces.id AND m.deleted_at IS NULL)
		FROM spaces WHERE id=$1 FOR UPDATE`, spaceID).Scan(&quota, &memberCount); err != nil {
		return err
	}
	if memberCount >= quota {
		return errors.New("空间成员席位已满") // 席位配额生效（S2-B01 验收）
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO space_members (space_id, user_id, role, invited_by_user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (space_id, user_id) WHERE deleted_at IS NULL DO NOTHING`,
		spaceID, userID, role, invitedBy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MemberCount 空间成员数。
func (r *Repo) MemberCount(ctx context.Context, spaceID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM space_members WHERE space_id=$1 AND deleted_at IS NULL`, spaceID).Scan(&n)
	return n, err
}

// MemberRow 成员行。
type MemberRow struct {
	UserID   int64
	UserUUID string
	Nickname string
	Role     string
	JoinedAt string
}

// ListMembers 成员列表（2.6）。
func (r *Repo) ListMembers(ctx context.Context, spaceID int64) ([]MemberRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.user_id, u.user_uuid::text, u.nickname, m.role::text,
		       to_char(m.joined_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM space_members m JOIN users u ON u.id = m.user_id
		WHERE m.space_id=$1 AND m.deleted_at IS NULL ORDER BY m.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MemberRow, 0)
	for rows.Next() {
		var m MemberRow
		if err := rows.Scan(&m.UserID, &m.UserUUID, &m.Nickname, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpdateMemberRole 修改成员角色（2.9）：OWNER 不可被改。
func (r *Repo) UpdateMemberRole(ctx context.Context, spaceID, userID int64, role string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE space_members SET role=$3 WHERE space_id=$1 AND user_id=$2
		  AND role != 'OWNER' AND deleted_at IS NULL`, spaceID, userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("目标成员不存在或为 OWNER 不可修改")
	}
	return nil
}

// RemoveMember 移除成员（2.10）：OWNER 不可移除（转让后才行）。
func (r *Repo) RemoveMember(ctx context.Context, spaceID, userID int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE space_members SET deleted_at=NOW() WHERE space_id=$1 AND user_id=$2
		  AND role != 'OWNER' AND deleted_at IS NULL`, spaceID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("目标成员不存在或为 OWNER 不可移除")
	}
	return nil
}

// TransferOwner 转让所有权（2.11）：旧 OWNER → 新 OWNER，旧 OWNER 降为 ADMIN。
func (r *Repo) TransferOwner(ctx context.Context, spaceID, newOwnerID, oldOwnerID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// 新成员须已在空间内
	var one int
	if err := tx.QueryRow(ctx, `
		SELECT 1 FROM space_members WHERE space_id=$1 AND user_id=$2 AND deleted_at IS NULL`,
		spaceID, newOwnerID).Scan(&one); err != nil {
		return errors.New("新所有者须先成为空间成员")
	}
	// 旧 OWNER 降为 ADMIN
	if _, err := tx.Exec(ctx, `
		UPDATE space_members SET role='ADMIN' WHERE space_id=$1 AND user_id=$2 AND role='OWNER'`,
		spaceID, oldOwnerID); err != nil {
		return err
	}
	// 新 OWNER
	if _, err := tx.Exec(ctx, `
		UPDATE space_members SET role='OWNER' WHERE space_id=$1 AND user_id=$2`,
		spaceID, newOwnerID); err != nil {
		return err
	}
	// spaces.owner_user_id 同步
	if _, err := tx.Exec(ctx, `UPDATE spaces SET owner_user_id=$2 WHERE id=$1`, spaceID, newOwnerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateInvitation 创建邀请（2.8）：token 7 天有效。
func (r *Repo) CreateInvitation(ctx context.Context, spaceID, inviterID int64, account, role string) (string, error) {
	token := uuid.NewString()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO space_invitations (space_id, inviter_user_id, invitee_account, invite_token, role, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', NOW() + interval '7 days')`,
		spaceID, inviterID, account, token, role)
	if err != nil {
		return "", err
	}
	return token, nil
}

// AcceptInvitation 接受邀请（2.8a）：token 校验（未过期/PENDING）→ 加入空间（席位校验）。
func (r *Repo) AcceptInvitation(ctx context.Context, token string, userID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var spaceID int64
	var role string
	if err := tx.QueryRow(ctx, `
		SELECT space_id, role::text FROM space_invitations
		WHERE invite_token=$1 AND status='PENDING' AND expires_at > NOW() FOR UPDATE`,
		token).Scan(&spaceID, &role); err != nil {
		return errors.New("邀请无效或已过期")
	}
	// 席位校验
	var quota, memberCount int
	if err := tx.QueryRow(ctx, `
		SELECT quota_member_count, (SELECT count(*) FROM space_members m WHERE m.space_id=spaces.id AND m.deleted_at IS NULL)
		FROM spaces WHERE id=$1`, spaceID).Scan(&quota, &memberCount); err != nil {
		return err
	}
	if memberCount >= quota {
		return errors.New("空间成员席位已满")
	}
	// 加入 + 邀请置 ACCEPTED
	if _, err := tx.Exec(ctx, `
		INSERT INTO space_members (space_id, user_id, role)
		VALUES ($1, $2, $3) ON CONFLICT (space_id, user_id) WHERE deleted_at IS NULL DO NOTHING`,
		spaceID, userID, role); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE space_invitations SET status='ACCEPTED', accepted_at=NOW(), accepted_by_user_id=$2
		WHERE invite_token=$1`, token, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
