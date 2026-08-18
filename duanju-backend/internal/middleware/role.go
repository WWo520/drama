// RBAC 角色校验中间件：按 roles.code 校验当前用户是否持有指定角色（对齐 04 C 章 L2）。
package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireRole 校验当前用户是否持有任一指定角色代码（如 SUPER_ADMIN/AUDITOR）。
// 依赖 LoadUserID 中间件注入的 ctx.user_id；未登录或角色不足返回 403。
func RequireRole(pool *pgxpool.Pool, roleCodes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetInt64(CtxUserID)
		if uid == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":       10001,
				"message":    "请先登录",
				"data":       nil,
				"request_id": c.GetString("request_id"),
			})
			return
		}
		has, err := HasAnyRole(c.Request.Context(), pool, uid, roleCodes...)
		if err != nil || !has {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":       10004,
				"message":    "无权限执行此操作",
				"data":       nil,
				"request_id": c.GetString("request_id"),
			})
			return
		}
		c.Next()
	}
}

// HasAnyRole 判断用户是否持有任一角色（服务层也可复用）。
func HasAnyRole(ctx context.Context, pool *pgxpool.Pool, userID int64, roleCodes ...string) (bool, error) {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id=$1 AND r.code = ANY($2) AND ur.deleted_at IS NULL`, userID, roleCodes).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
