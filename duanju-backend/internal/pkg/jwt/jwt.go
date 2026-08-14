// Package jwt 封装 JWT 双 Token 签发与解析（对齐 04 C 章 / 05 B3 章）。
// Access 2h（前端内存）、Refresh 7d（HttpOnly Cookie），Token 绑定 UA+IP 指纹（jti 指纹校验）。
package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrTokenExpired token 已过期（供中间件区分 10002/10003）。
var ErrTokenExpired = errors.New("token expired")

const (
	ClaimUserID  = "uid" // user_uuid
	ClaimJTI     = "jti"
	ClaimFinger  = "fp" // UA+IP 指纹
	ClaimKind    = "k"  // access / refresh
	KindAccess   = "access"
	KindRefresh  = "refresh"
)

// Config JWT 配置（与 .env 对应）。
type Config struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// TokenPair 双 Token 签发结果。
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	ExpiresInSec int    `json:"expires_in"`
}

// Manager JWT 管理器。
type Manager struct {
	cfg Config
}

func NewManager(cfg Config) *Manager {
	if cfg.Secret == "" {
		panic("jwt: secret 不能为空")
	}
	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = 2 * time.Hour
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 7 * 24 * time.Hour
	}
	return &Manager{cfg: cfg}
}

// Sign 签发 access + refresh，返回 pair 与 refresh jti（用于黑名单登记）。
func (m *Manager) Sign(userUUID, fingerprint string) (*TokenPair, string, error) {
	refreshJTI := uuid.NewString()
	accessJTI := uuid.NewString()

	now := time.Now()
	accessClaims := jwt.MapClaims{
		ClaimUserID: userUUID,
		ClaimJTI:    accessJTI,
		ClaimFinger: fingerprint,
		ClaimKind:   KindAccess,
		"iat":       now.Unix(),
		"exp":       now.Add(m.cfg.AccessTTL).Unix(),
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(m.cfg.Secret))
	if err != nil {
		return nil, "", err
	}

	refreshClaims := jwt.MapClaims{
		ClaimUserID: userUUID,
		ClaimJTI:    refreshJTI,
		ClaimFinger: fingerprint,
		ClaimKind:   KindRefresh,
		"iat":       now.Unix(),
		"exp":       now.Add(m.cfg.RefreshTTL).Unix(),
	}
	refresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(m.cfg.Secret))
	if err != nil {
		return nil, "", err
	}

	return &TokenPair{
		AccessToken:  access,
		ExpiresInSec: int(m.cfg.AccessTTL.Seconds()),
	}, refresh, nil
}

// Parse 解析并校验 token；返回 claims（uid/jti/kind/fingerprint）。
func (m *Manager) Parse(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("签名算法不符")
		}
		return []byte(m.cfg.Secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("token 无效")
	}
	return claims, nil
}

// UserID 从 claims 取 user_uuid。
func UserID(claims jwt.MapClaims) (string, error) {
	uid, ok := claims[ClaimUserID].(string)
	if !ok || uid == "" {
		return "", fmt.Errorf("claims 缺少 %s", ClaimUserID)
	}
	return uid, nil
}

// JTI 从 claims 取 jti。
func JTI(claims jwt.MapClaims) (string, error) {
	j, ok := claims[ClaimJTI].(string)
	if !ok || j == "" {
		return "", fmt.Errorf("claims 缺少 %s", ClaimJTI)
	}
	return j, nil
}
