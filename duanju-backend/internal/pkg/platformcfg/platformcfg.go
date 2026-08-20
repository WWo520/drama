// Package platformcfg 平台配置读取（platform_configs 表，30s TTL 缓存）。
// 供维护模式中间件、注册流程与公开配置接口共用；管理员保存后最多 30s 生效。
package platformcfg

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cacheTTL 配置读取缓存有效期（短 TTL 平衡生效延迟与 DB 压力）。
const cacheTTL = 30 * time.Second

type cacheEntry struct {
	val string
	ok  bool
	ts  time.Time
}

// Store 平台配置读取器（带 TTL 缓存，线程安全）。
type Store struct {
	pool  *pgxpool.Pool
	mu    sync.Mutex
	cache map[string]cacheEntry
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, cache: make(map[string]cacheEntry)}
}

// Bool 读取布尔开关；配置缺失或查询失败返回默认值 true（安全默认：异常时功能保持开放）。
func (s *Store) Bool(ctx context.Context, key string) (bool, error) {
	raw, ok, err := s.get(ctx, key)
	if err != nil || !ok {
		return true, err
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		// 存量值可能是 "1"/"0"（旧格式兜底）
		return raw == "1", nil
	}
	return v, nil
}

// get 读单 key（30s 缓存；ErrNoRows 视为未配置 ok=false）。
func (s *Store) get(ctx context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	if e, ok := s.cache[key]; ok && time.Since(e.ts) < cacheTTL {
		s.mu.Unlock()
		return e.val, e.ok, nil
	}
	s.mu.Unlock()

	var raw string
	err := s.pool.QueryRow(ctx,
		`SELECT config_value::text FROM platform_configs WHERE config_key=$1`, key,
	).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	s.mu.Lock()
	s.cache[key] = cacheEntry{val: raw, ok: true, ts: time.Now()}
	s.mu.Unlock()
	return raw, true, nil
}

// PublicItem 公开配置项（config_value 保留 JSON 原文，前端可直接消费）。
type PublicItem struct {
	ConfigKey   string          `json:"config_key"`
	ConfigValue json.RawMessage `json:"config_value"`
	ConfigGroup string          `json:"config_group"`
}

// PublicList 公开配置列表（is_public=TRUE，按组排序）。
func (s *Store) PublicList(ctx context.Context) ([]PublicItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT config_key, config_value::text, COALESCE(config_group,'') FROM platform_configs WHERE is_public=TRUE ORDER BY config_group, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicItem{}
	for rows.Next() {
		var k, raw, g string
		if err := rows.Scan(&k, &raw, &g); err != nil {
			return nil, err
		}
		// string 中转（pgx 对 json.RawMessage 目标扫描会留空），raw 本身是合法 JSON 文本
		out = append(out, PublicItem{ConfigKey: k, ConfigValue: json.RawMessage(raw), ConfigGroup: g})
	}
	return out, rows.Err()
}
