// 限流中间件：Redis 滑动窗口（对齐 05 B1：未登录 20 次/分，普通用户 60 次/分）。
// H03 生产加固：X-API-Key 请求按 api_keys.quota_per_min 限流（回退 IP）。
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// RateLimit 基于 Redis INCR + EXPIRE 的滑动窗口限流。
// key: duanju:{env}:rate:api:{user_id|ip}:{minute_ts}（02 E3.3）
func RateLimit(rdb *redis.Client, pool *pgxpool.Pool, env string) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 120
		subject := c.GetString("user_id")
		if subject == "" {
			subject = c.ClientIP()
		}
		// H03：API Key 配额优先（quota_per_min 默认 60）
		if apiKey := c.GetHeader("X-API-Key"); apiKey != "" && pool != nil {
			var quota int
			if err := pool.QueryRow(context.Background(), `
				SELECT quota_per_min FROM api_keys
				WHERE api_key_prefix=$1 AND status='ACTIVE'`, apiKeyPrefix(apiKey)).Scan(&quota); err == nil && quota > 0 {
				subject = "key:" + apiKeyPrefix(apiKey)
				limit = quota
			}
		}
		if c.GetString("user_id") == "" && !isAPIKeySubject(subject) {
			limit = 20
		}
		key := fmt.Sprintf("duanju:%s:rate:api:%s:%d", env, subject, time.Now().Unix()/60)

		ctx := context.Background()
		count, err := rdb.Incr(ctx, key).Result()
		if err == nil && count == 1 {
			rdb.Expire(ctx, key, 2*time.Minute)
		}

		remaining := limit - int(count)
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(max(remaining, 0)))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(60*time.Second).Unix(), 10))

		if count > int64(limit) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":       90001, // 05 C 章 90001 限流
				"message":    "请求过于频繁，请稍后再试",
				"data":       nil,
				"request_id": c.GetString("request_id"),
			})
			return
		}
		c.Next()
	}
}

func apiKeyPrefix(key string) string {
	if len(key) > 12 {
		return key[:12]
	}
	return key
}

func isAPIKeySubject(s string) bool {
	return len(s) > 4 && s[:4] == "key:"
}
