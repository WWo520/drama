// ModelKeyStore 实现 model.KeyStore：从 DB（model_providers + model_keys + models）读取 Key 配置。
// 与 worker 开发期 mock KeyStore 对应：配置了 DB 数据即真实路由，无数据时降级返回 mock 凭据。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/pkg/cryptox"
)

// ModelKeyStore DB 驱动的模型 Key 存储（Key 密文用 encKey 解密）。
type ModelKeyStore struct {
	pool   *pgxpool.Pool
	encKey []byte
	// fallbackMock 无 DB 配置时降级返回 mock 凭据（开发期保留 mock 链路）
	fallbackMock bool
}

func NewModelKeyStore(pool *pgxpool.Pool, encKey []byte, fallbackMock bool) *ModelKeyStore {
	return &ModelKeyStore{pool: pool, encKey: encKey, fallbackMock: fallbackMock}
}

// AvailableKeys 返回指定能力（+可选指定模型）的可用 Key（按 priority 降序）。
func (s *ModelKeyStore) AvailableKeys(ctx context.Context, capability model.Capability, modelKey string) ([]*model.KeyCredential, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT mk.id, mp.name,
		       COALESCE(
		           (SELECT m.model_key FROM models m
		            WHERE m.provider_id = mp.id AND m.capability::text = $1::text
		              AND m.enabled AND m.deleted_at IS NULL
		            LIMIT 1),
		           mk.name
		       ) AS model_key,
		       mk.key_encrypted, mp.base_url,
		       mk.concurrent_limit, mk.priority, mk.enabled
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
			AND mp.enabled = TRUE AND mp.deleted_at IS NULL
		WHERE mk.enabled = TRUE AND mk.deleted_at IS NULL
		  AND mp.capabilities ? $1::text
		  AND ($2 = '' OR EXISTS (
		      SELECT 1 FROM models m
		      WHERE m.provider_id = mp.id AND m.model_key = $2
		        AND m.capability::text = $1::text AND m.enabled = TRUE AND m.deleted_at IS NULL))
		ORDER BY mk.priority DESC, mk.id ASC`, string(capability), modelKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.KeyCredential, 0)
	sawKey := false // 是否存在启用的 Key 行（区分"无配置"与"配置了但全部损坏"）
	for rows.Next() {
		var k model.KeyCredential
		var encrypted string
		if err := rows.Scan(&k.ID, &k.ProviderName, &k.ModelKey, &encrypted, &k.BaseURL, &k.Concurrency, &k.Priority, &k.Enabled); err != nil {
			return nil, err
		}
		sawKey = true
		// 解密 API Key：失败则跳过该 Key 并记录（曾因解密失败静默带空 APIKey 发请求，导致厂商 401）
		plain, derr := decryptKey(encrypted, s.encKey)
		if derr != nil {
			log.Printf("model: key %d (%s/%s) 解密失败已跳过（检查 MODEL_KEY_ENC_SECRET 是否与加密时一致）: %v",
				k.ID, k.ProviderName, k.ModelKey, derr)
			continue
		}
		k.APIKey = plain
		out = append(out, &k)
	}
	if len(out) == 0 {
		if sawKey {
			// 有配置但全部解密失败：明确报错，禁止降级 mock（否则真实任务静默产出假结果）
			return nil, errors.New("model: 已启用的 Key 均解密失败，请检查 MODEL_KEY_ENC_SECRET 配置")
		}
		if s.fallbackMock {
			// 降级：无 DB 配置时返回 mock 凭据（worker 注册 mock provider 时使用）
			return []*model.KeyCredential{{
				ID: 1, ProviderName: "mock-llm", ModelKey: "luxchat-mock",
				APIKey: "mock-key-1", BaseURL: "mock://llm", Concurrency: 8, Priority: 0, Enabled: true,
			}}, nil
		}
	}
	return out, rows.Err()
}

// MarkKeyUnavailable 429/5xx 连续失败标记（Redis 冷却由引擎内存管理，此处记录审计即可）。
func (s *ModelKeyStore) MarkKeyUnavailable(_ context.Context, _ int64, _ string) error {
	return nil
}

// ---- 管理端数据操作 ----

// ProviderRow 厂商行。
type ProviderRow struct {
	ID           int64
	Name         string
	DisplayName  string
	Capabilities []string
	BaseURL      string
	Enabled      bool
	SortOrder    int
}

// ListProviders 厂商列表。
func (s *ModelKeyStore) ListProviders(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, display_name, COALESCE(capabilities::text,'[]'), base_url, enabled, sort_order
		FROM model_providers WHERE deleted_at IS NULL ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, sortOrder int64
		var name, display, caps, baseURL string
		var enabled bool
		if err := rows.Scan(&id, &name, &display, &caps, &baseURL, &enabled, &sortOrder); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "display_name": display, "capabilities": caps, "base_url": baseURL, "enabled": enabled, "sort_order": sortOrder})
	}
	return out, rows.Err()
}

// CreateProvider 新建厂商。
func (s *ModelKeyStore) CreateProvider(ctx context.Context, name, displayName string, capabilities []string, baseURL string, enabled bool) (int64, error) {
	capsJSON, _ := json.Marshal(capabilities)
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO model_providers (name, display_name, capabilities, base_url, enabled, sort_order)
		VALUES ($1, $2, $3::jsonb, $4, $5, 0) RETURNING id`,
		name, displayName, string(capsJSON), baseURL, enabled).Scan(&id)
	return id, err
}

// UpdateProvider 更新厂商（按 id）。
func (s *ModelKeyStore) UpdateProvider(ctx context.Context, id int64, displayName string, capabilities []string, baseURL string, enabled bool) error {
	capsJSON, _ := json.Marshal(capabilities)
	_, err := s.pool.Exec(ctx, `
		UPDATE model_providers SET display_name=$2, capabilities=$3::jsonb, base_url=$4, enabled=$5, updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`,
		id, displayName, string(capsJSON), baseURL, enabled)
	return err
}

// CreateModel 新建模型（二级）。
func (s *ModelKeyStore) CreateModel(ctx context.Context, providerID int64, modelKey, name, capability string, costPoints int, enabled bool) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO models (provider_id, model_key, name, capability, cost_points_per_call, enabled, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, 0) RETURNING id`,
		providerID, modelKey, name, capability, costPoints, enabled).Scan(&id)
	return id, err
}

// UpdateModelByKey 更新模型（按 model_key）。
func (s *ModelKeyStore) UpdateModelByKey(ctx context.Context, modelKey, name, capability string, costPoints int, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE models SET name=$2, capability=$3, cost_points_per_call=$4, enabled=$5, updated_at=NOW()
		WHERE model_key=$1 AND deleted_at IS NULL`,
		modelKey, name, capability, costPoints, enabled)
	return err
}

// CreateModelKey 新建 API Key（密文入库）。
func (s *ModelKeyStore) CreateModelKey(ctx context.Context, providerID int64, name, apiKey string, rpm, qps, concurrent int, enabled bool, priority int) (int64, string, error) {
	encrypted, err := encryptKey(apiKey, s.encKey)
	if err != nil {
		return 0, "", err
	}
	prefix := apiKey
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO model_keys (provider_id, name, key_encrypted, key_prefix, rate_limit_rpm, rate_limit_qps, concurrent_limit, enabled, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		providerID, name, encrypted, prefix, rpm, qps, concurrent, enabled, priority).Scan(&id)
	return id, prefix, err
}

// UpdateModelKey 更新 API Key 配置（状态/并发/配额/优先级；不更新密文）。
func (s *ModelKeyStore) UpdateModelKey(ctx context.Context, id int64, rpm, qps, concurrent int, enabled bool, priority int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE model_keys SET rate_limit_rpm=$2, rate_limit_qps=$3, concurrent_limit=$4, enabled=$5, priority=$6, updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`,
		id, rpm, qps, concurrent, enabled, priority)
	return err
}

// RevokeModelKey 吊销（enabled=false）。
func (s *ModelKeyStore) RevokeModelKey(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE model_keys SET enabled=FALSE, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("密钥不存在")
	}
	return nil
}

// ListModelKeys 管理端 API Key 列表（隐藏密文，只展示前缀与配额）。
func (s *ModelKeyStore) ListModelKeys(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT mk.id, mk.provider_id, mp.name, mk.name, mk.key_prefix,
		       mk.rate_limit_rpm, mk.rate_limit_qps, mk.concurrent_limit,
		       COALESCE(mk.daily_quota_limit,0), COALESCE(mk.daily_quota_used,0), mk.enabled, mk.priority,
		       COALESCE(to_char(mk.last_used_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),'')
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mk.deleted_at IS NULL ORDER BY mk.priority DESC, mk.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, providerID, rpm, qps, concurrent, priority int64
		var quotaLimit, quotaUsed int64
		var providerName, name, prefix, lastUsed string
		var enabled bool
		if err := rows.Scan(&id, &providerID, &providerName, &name, &prefix, &rpm, &qps, &concurrent, &quotaLimit, &quotaUsed, &enabled, &priority, &lastUsed); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "provider_id": providerID, "provider_name": providerName,
			"name": name, "key_prefix": prefix, "rate_limit_rpm": rpm, "rate_limit_qps": qps,
			"concurrent_limit": concurrent, "daily_quota_limit": quotaLimit,
			"daily_quota_used": quotaUsed, "enabled": enabled, "priority": priority, "last_used_at": lastUsed,
		})
	}
	return out, rows.Err()
}

// ListModelsAdmin 管理端模型列表（含厂商名）。
func (s *ModelKeyStore) ListModelsAdmin(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.provider_id, COALESCE(p.name,''), m.model_key, COALESCE(m.name,''),
		       COALESCE(m.capability::text,''), m.cost_points_per_call, m.enabled, m.sort_order
		FROM models m LEFT JOIN model_providers p ON p.id=m.provider_id
		WHERE m.deleted_at IS NULL ORDER BY m.sort_order ASC, m.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, providerID, cost, sortOrder int64
		var providerName, modelKey, name, capability string
		var enabled bool
		if err := rows.Scan(&id, &providerID, &providerName, &modelKey, &name, &capability, &cost, &enabled, &sortOrder); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "provider_id": providerID, "provider_name": providerName,
			"model_key": modelKey, "name": name, "capability": capability,
			"cost_points_per_call": cost, "enabled": enabled, "sort_order": sortOrder,
		})
	}
	return out, rows.Err()
}

// ---- 加解密 ----

func encryptKey(plain string, key []byte) (string, error) {
	return cryptox.Encrypt(plain, key)
}

func decryptKey(encoded string, key []byte) (string, error) {
	return cryptox.Decrypt(encoded, key)
}
