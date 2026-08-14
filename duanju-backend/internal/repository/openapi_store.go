// openapi_store 实现 creation.OpenAPIStore：Key 哈希存储 + webhook 验签。
package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/domain/creation"
)

// OpenAPIStore 开发者数据访问。
type OpenAPIStore struct {
	pool *pgxpool.Pool
}

func NewOpenAPIStore(pool *pgxpool.Pool) *OpenAPIStore {
	return &OpenAPIStore{pool: pool}
}

var _ creation.OpenAPIStore = (*OpenAPIStore)(nil)

func (s *OpenAPIStore) CreateKey(ctx context.Context, userID int64, name string) (string, error) {
	plain := "dj_" + uuid.NewString() + uuid.NewString()[:12]
	hashed := sha256.Sum256([]byte(plain))
	prefix := plain[:12]
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (api_key_hashed, api_key_prefix, user_id, name)
		VALUES ($1, $2, $3, $4)`,
		hex.EncodeToString(hashed[:]), prefix, userID, name)
	if err != nil {
		return "", err
	}
	return plain, nil
}

func (s *OpenAPIStore) RevokeKey(ctx context.Context, userID int64, keyPrefix string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_keys SET status='REVOKED', revoked_at=NOW()
		WHERE api_key_prefix=$1 AND user_id=$2 AND status='ACTIVE'`, keyPrefix, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("密钥不存在或已吊销")
	}
	return nil
}

func (s *OpenAPIStore) ListKeys(ctx context.Context, userID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT api_key_prefix, name, status::text, quota_per_min,
		       to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM api_keys WHERE user_id=$1 ORDER BY id DESC LIMIT 20`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var prefix, name, status, created string
		var quota int
		rows.Scan(&prefix, &name, &status, &quota, &created)
		out = append(out, map[string]any{
			"api_key_prefix": prefix, "name": name, "status": status, "quota_per_min": quota, "created_at": created,
		})
	}
	return out, rows.Err()
}

func (s *OpenAPIStore) RegisterWebhook(ctx context.Context, userID int64, url, secret string, events []string) error {
	ev, _ := json.Marshal(events)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhooks (user_id, url, secret, events) VALUES ($1, $2, $3, $4::jsonb)`,
		userID, url, secret, string(ev))
	return err
}

func (s *OpenAPIStore) VerifyWebhook(ctx context.Context, url string, payload []byte, signature string) (bool, error) {
	var secret string
	if err := s.pool.QueryRow(ctx, `
		SELECT secret FROM webhooks WHERE url=$1 AND status='ACTIVE' ORDER BY id DESC LIMIT 1`, url).Scan(&secret); err != nil {
		return false, errors.New("webhook 未注册")
	}
	expect := creation.SignPayload(secret, payload)
	return hmac.Equal([]byte(expect), []byte(signature)), nil
}