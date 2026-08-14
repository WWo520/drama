// Package logger 封装 zap 结构化日志（对齐 01 F.4：JSON 输出 + 强制 trace_id/user_id/space_id/latency_ms）。
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 构建 zap.Logger；dev 环境 Console 彩色输出，生产 JSON 输出。
func New(env string) *zap.Logger {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
	lg, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return lg
}

// F 便捷：构造带公共字段的日志字段。
func F(kvs ...any) []zap.Field {
	fields := make([]zap.Field, 0, len(kvs)/2)
	for i := 0; i+1 < len(kvs); i += 2 {
		fields = append(fields, zap.Any(kvs[i].(string), kvs[i+1]))
	}
	return fields
}
