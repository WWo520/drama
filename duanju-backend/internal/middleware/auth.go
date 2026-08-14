// JWT 认证中间件：L1 用户鉴权（对齐 04 C 章 3 层中间件之 L1）。
// Access Token 校验 + UA+IP 指纹绑定；Refresh 黑名单在 domain/auth 层处理。
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/duanju/backend/internal/pkg/jwt"
)

// AuthContext 中间件注入到 ctx 的键。
const (
	CtxUserUUID    = "user_uuid"
	CtxUserID      = "user_id" // 数字主键（由 LoadUserID 中间件填充）
	CtxAccessJTI   = "access_jti"
	CtxFingerprint = "fp"
	CtxSpaceID     = "space_id" // 当前工作空间（switch 后注入）
)

// JWT 认证：解析 Bearer Token，校验 kind=access，绑定指纹。
func JWT(mgr *jwt.Manager, rdb *redis.Client, env string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := bearerToken(c.GetHeader("Authorization"))
		if tokenStr == "" {
			abortUnauthorized(c, 10001, "未登录")
			return
		}
		claims, err := mgr.Parse(tokenStr)
		if err != nil {
			// 过期（10002，前端静默刷新）与签名无效（10003，强制踢登录）区分
			if errors.Is(err, jwt.ErrTokenExpired) {
				abortUnauthorized(c, 10002, "登录已过期")
			} else {
				abortUnauthorized(c, 10003, "登录状态无效")
			}
			return
		}
		if claims[jwt.ClaimKind] != jwt.KindAccess {
			abortUnauthorized(c, 10003, "Token 类型错误")
			return
		}
		uid, err := jwt.UserID(claims)
		if err != nil {
			abortUnauthorized(c, 10003, "Token 载荷缺失")
			return
		}
		// 指纹绑定（04 C.1：Token 绑定 UA+IP）
		if fp, ok := claims[jwt.ClaimFinger].(string); ok && fp != "" {
			if fp != fingerprint(c) {
				abortUnauthorized(c, 10003, "登录环境变化，请重新登录")
				return
			}
		}
		// 登出拉黑：access jti 命中黑名单 → 10005 强制登录（05 C 章 10005 语义）
		if jti, err := jwt.JTI(claims); err == nil {
			ctx := context.Background()
			key := fmt.Sprintf("duanju:%s:auth:refresh_blacklist:%s", env, jti)
			blacklisted, _ := rdb.Exists(ctx, key).Result()
			if blacklisted > 0 {
				abortUnauthorized(c, 10005, "登录已过期，请重新登录")
				return
			}
		}

		c.Set(CtxUserUUID, uid)
		c.Set(CtxAccessJTI, claims[jwt.ClaimJTI])
		c.Set(CtxFingerprint, fingerprint(c))
		c.Next()
	}
}

// bearerToken 提取 Authorization: Bearer xxx。
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && header[:len(prefix)] == prefix {
		return header[len(prefix):]
	}
	return ""
}

// fingerprint UA + IP 指纹（04 C.1）。
func fingerprint(c *gin.Context) string {
	return c.ClientIP() + "|" + c.Request.UserAgent()
}

func abortUnauthorized(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":       code,
		"message":    msg,
		"data":       nil,
		"request_id": c.GetString("request_id"),
	})
}
