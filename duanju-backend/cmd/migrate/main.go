// migrate 是数据库迁移命令：up / down / version / status / create。
//
// 用法示例：
//
//	go run ./cmd/migrate up        # 应用全部迁移（DATABASE_URL 或 -db-url）
//	go run ./cmd/migrate create -name add_users
//	go run ./cmd/migrate version
//	go run ./cmd/migrate down 1    # 回退 1 步
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/duanju/backend/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbURL := fs.String("db-url", "", "PostgreSQL DSN（默认读环境变量 DATABASE_URL）")
	name := fs.String("name", "", "create 子命令的迁移名称，如 add_users")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法: migrate [-db-url DSN] <up|down [n]|version|status|create -name xxx>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("缺少子命令")
	}

	cmd := fs.Arg(0)
	switch cmd {
	case "create":
		if *name == "" {
			return errors.New("create 需要 -name 参数")
		}
		return createMigration(*name)
	}

	dsn := *dbURL
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return errors.New("缺少 DATABASE_URL 环境变量或 -db-url 参数")
	}

	m, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		v, dirty, verr := m.Version()
		if verr != nil {
			return verr
		}
		fmt.Printf("up 完成，当前版本 %d（dirty=%v）\n", v, dirty)
	case "force":
		if fs.NArg() < 2 {
			return errors.New("force 需要版本号参数，如: migrate force 10")
		}
		n, err := strconv.Atoi(fs.Arg(1))
		if err != nil {
			return errors.New("force 版本号必须是整数")
		}
		if err := m.Force(n); err != nil {
			return err
		}
		fmt.Printf("已强制标记版本 %d\n", n)
	case "down":
		n := 1
		if fs.NArg() > 1 {
			n, err = strconv.Atoi(fs.Arg(1))
			if err != nil || n <= 0 {
				return errors.New("down 步数必须是正整数")
			}
		}
		if err := m.Steps(-n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		v, _, _ := m.Version()
		fmt.Printf("down 完成，当前版本 %d\n", v)
	case "version":
		v, dirty, err := m.Version()
		if err != nil {
			return err
		}
		fmt.Printf("版本 %d（dirty=%v）\n", v, dirty)
	case "status":
		return status(m)
	default:
		return fmt.Errorf("未知子命令 %q", cmd)
	}
	return nil
}

func newMigrator(dsn string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("加载嵌入迁移: %w", err)
	}
	// pgx5 驱动注册名为 pgx5，统一将 postgres:// 前缀映射为 pgx5://（与后端 pgx 一致）。
	if strings.HasPrefix(dsn, "postgres://") {
		dsn = "pgx5://" + strings.TrimPrefix(dsn, "postgres://")
	}
	return migrate.NewWithSourceInstance("iofs", src, dsn)
}

func status(m *migrate.Migrate) error {
	v, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return err
	}
	fmt.Printf("当前版本 %d dirty=%v\n", v, dirty)
	return nil
}

// createMigration 生成 NNNN_name.up.sql / .down.sql 空模板（在 duanju-backend/migrations/ 下执行时生效）。
func createMigration(name string) error {
	// 名称只允许字母/数字/下划线，避免路径穿越。
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return fmt.Errorf("迁移名称只允许字母/数字/下划线，收到 %q", name)
		}
	}
	// 定位 migrations 目录：相对当前工作目录向上查找。
	dir := "migrations"
	for range 4 {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		dir = filepath.Join("..", dir)
	}
	ts := time.Now().UTC().Format("20060102150405")
	base := filepath.Join(dir, fmt.Sprintf("%s_%s", ts, name))
	for _, suffix := range []string{"up", "down"} {
		p := base + "." + suffix + ".sql"
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s 已存在", p)
		}
		if err := os.WriteFile(p, []byte(fmt.Sprintf("-- %s\n", strings.ToUpper(suffix))), 0o644); err != nil {
			return err
		}
		fmt.Println("created", p)
	}
	return nil
}
