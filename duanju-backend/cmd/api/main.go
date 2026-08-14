// cmd/api 主入口：装配配置 → 连接池 → 路由 → 启动（对齐 04 B.1 cmd/server/main.go）。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/pkg/db"
	"github.com/duanju/backend/internal/pkg/logger"
	"github.com/duanju/backend/internal/pkg/redisx"
	"github.com/duanju/backend/internal/router"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	lg := logger.New(cfg.App.Env)
	defer func() { _ = lg.Sync() }()

	pool, err := db.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		lg.Fatal("数据库连接失败", logger.F("err", err)...)
	}
	defer pool.Close()

	rdb, err := redisx.New(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		lg.Fatal("Redis 连接失败", logger.F("err", err)...)
	}
	defer rdb.Close()

	engine := router.Setup(cfg, lg, pool, rdb)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		lg.Info("API 服务启动", logger.F("addr", srv.Addr, "env", cfg.App.Env)...)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lg.Fatal("HTTP 服务异常退出", logger.F("err", err)...)
		}
	}()

	// 优雅停机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	lg.Info("正在优雅停机...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		lg.Error("停机超时", logger.F("err", err)...)
	}
	lg.Info("服务已退出")
}
