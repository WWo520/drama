// diag-key 查看 H3 等模型 Key 的并发/限流配置（排查视频并发瓶颈）。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	}
	pool, _ := pgxpool.New(context.Background(), dsn)
	defer pool.Close()
	rows, err := pool.Query(context.Background(), `
		SELECT p.name, mk.name, mk.concurrent_limit,
		       COALESCE(mk.rate_limit_rpm,0), mk.enabled
		FROM model_keys mk
		JOIN model_providers p ON p.id = mk.provider_id
		ORDER BY p.name, mk.name`)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var prov, key string
		var cl, rpm int
		var en bool
		_ = rows.Scan(&prov, &key, &cl, &rpm, &en)
		fmt.Printf("%-12s %-16s 并发=%d rpm=%d enabled=%v\n", prov, key, cl, rpm, en)
	}
}
