// Package middleware 横切关注点：RequestID / 访问日志 / Recovery / CORS / 限流 / JWT 认证（对齐 04 B.2 中间件链）。
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RequestID 注入 X-Request-ID 与 ctx.request_id。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// AccessLog 结构化访问日志：强制 trace_id/user_id/space_id/latency_ms（01 F.4）。
func AccessLog(lg *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start).Milliseconds()
		lg.Info("http",
			zap.String("request_id", c.GetString("request_id")),
			zap.Int64("user_id", c.GetInt64("user_id")),
			zap.Int64("space_id", c.GetInt64("space_id")),
			zap.Int64("latency_ms", latency),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.String("ip", c.ClientIP()),
			zap.String("ua", c.Request.UserAgent()),
		)
	}
}

// Recovery panic 恢复 → 500 + 日志（Sentry 上报留 M0 后续接入）。
func Recovery(lg *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				lg.Error("panic recovered",
					zap.String("request_id", c.GetString("request_id")),
					zap.Any("panic", r),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(500, gin.H{
					"code":       99999,
					"message":    "服务器内部错误",
					"data":       nil,
					"request_id": c.GetString("request_id"),
					"timestamp":  time.Now().UTC().Format("2006-01-02T15:04:05Z07:00"),
				})
			}
		}()
		c.Next()
	}
}

// CORS 跨域：显式 origin 白名单，允许携带 Cookie（Refresh Token 依赖），禁止通配 + credentials 组合。
// 所有环境（含 dev）均仅放行白名单 origin（M 修复：收紧非生产环境任意 origin 放行）。
func CORS(env string) gin.HandlerFunc {
	allowOrigins := map[string]bool{
		"http://localhost:5173": true, // Vite dev
		"http://127.0.0.1:5173": true,
		"http://localhost:4173": true, // Vite preview
	}
	if env == "production" {
		// 生产域名在部署时补充（05 B1：生产限定域名）
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Request-ID,Idempotency-Key")
		c.Header("Access-Control-Expose-Headers", "X-RateLimit-Limit,X-RateLimit-Remaining,X-RateLimit-Reset")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
