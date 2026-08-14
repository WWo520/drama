// Package config 统一配置加载（viper：YAML + ENV 覆盖，对齐 04 A.1）。
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置结构。
type Config struct {
	App      App
	Postgres Postgres
	Redis    Redis
	S3       S3
	JWT      JWT
}

type App struct {
	Env   string
	Port  int
	Debug bool
}

type Postgres struct {
	DSN string
}

type Redis struct {
	Addr     string
	Password string
	DB       int
}

type S3 struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type JWT struct {
	Secret    string
	AccessTTL time.Duration
}

// Load 加载配置：configs/config.yaml 为基底，环境变量（APP_/DATABASE_URL 等）覆盖。
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("configs")
	v.AddConfigPath(".")

	v.SetDefault("app.env", "dev")
	v.SetDefault("app.port", 8000)

	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		// 允许无配置文件，纯环境变量运行（docker compose 场景）
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置: %w", err)
		}
	}

	cfg := &Config{
		App: App{
			Env:   v.GetString("app.env"),
			Port:  v.GetInt("app.port"),
			Debug: v.GetBool("app.debug"),
		},
		Postgres: Postgres{
			DSN: v.GetString("DATABASE_URL"),
		},
		Redis: Redis{
			Addr:     v.GetString("REDIS_ADDR"),
			Password: v.GetString("REDIS_PASSWORD"),
			DB:       v.GetInt("REDIS_DB"),
		},
		S3: S3{
			Endpoint:  v.GetString("S3_ENDPOINT"),
			AccessKey: v.GetString("S3_ACCESS_KEY"),
			SecretKey: v.GetString("S3_SECRET_KEY"),
			Bucket:    v.GetString("S3_BUCKET"),
			UseSSL:    v.GetBool("S3_USE_SSL"),
		},
		JWT: JWT{
			Secret:    v.GetString("JWT_SECRET"),
			AccessTTL: 2 * time.Hour,
		},
	}

	// 环境变量直接注入（viper 对裸变量名需显式绑定）
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		cfg.Postgres.DSN = dsn
	}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		cfg.Redis.Addr = addr
	}
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		cfg.JWT.Secret = secret
	}
	if env := os.Getenv("APP_ENV"); env != "" {
		cfg.App.Env = env
	}
	if port := os.Getenv("APP_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.App.Port)
	}
	// S3（同 DATABASE_URL 裸变量名）
	if ep := os.Getenv("S3_ENDPOINT"); ep != "" {
		cfg.S3.Endpoint = ep
	}
	if ak := os.Getenv("S3_ACCESS_KEY"); ak != "" {
		cfg.S3.AccessKey = ak
	}
	if sk := os.Getenv("S3_SECRET_KEY"); sk != "" {
		cfg.S3.SecretKey = sk
	}
	if bk := os.Getenv("S3_BUCKET"); bk != "" {
		cfg.S3.Bucket = bk
	}
	if ssl := os.Getenv("S3_USE_SSL"); ssl != "" {
		cfg.S3.UseSSL = ssl == "true" || ssl == "1"
	}

	if cfg.Postgres.DSN == "" {
		return nil, fmt.Errorf("缺少 DATABASE_URL 配置")
	}
	if cfg.Redis.Addr == "" {
		return nil, fmt.Errorf("缺少 REDIS_ADDR 配置")
	}
	if cfg.JWT.Secret == "" {
		return nil, fmt.Errorf("缺少 JWT_SECRET 配置")
	}
	return cfg, nil
}
