// LoadUserID 中间件：JWT 鉴权后按 user_uuid 反查数字主键注入 ctx（04 C.2 L1 之后的用户解析）。
package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UserResolver user_uuid → 数字主键。
type UserResolver func(ctx context.Context, userUUID string) (int64, error)

// LoadUserID 注入 ctx.user_id（数字主键），供仓储层使用。
func LoadUserID(resolve UserResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		uuid := c.GetString(CtxUserUUID)
		if uuid == "" {
			// 未经过 JWT 中间件（不应发生），放行由业务层处理
			c.Next()
			return
		}
		id, err := resolve(c.Request.Context(), uuid)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":       10001,
				"message":    "账号不存在",
				"data":       nil,
				"request_id": c.GetString("request_id"),
			})
			return
		}
		c.Set(CtxUserID, id)
		c.Next()
	}
}
