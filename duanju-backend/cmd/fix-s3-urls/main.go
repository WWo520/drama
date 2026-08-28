// fix-s3-urls 把历史转存图 URL 的 127.0.0.1:9000 基址批量替换为可达的 S3_PUBLIC_BASE。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable")
	}
	newBase := "http://192.168.31.103:9000"
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()
	ctx := context.Background()

	for _, spec := range []struct{ table, col string }{
		{"character_variants", "main_image_url"},
		{"scenes", "main_image_url"},
		{"props", "main_image_url"},
		{"shots", "reference_image_url"},
		{"generated_images", "original_url"},
	} {
		tag, err := pool.Exec(ctx, fmt.Sprintf(
			`UPDATE %s SET %s = '%s' || substring(%s from position('/duanju/' in %s))
			 WHERE %s LIKE 'http://127.0.0.1:9000%%'`,
			spec.table, spec.col, newBase, spec.col, spec.col, spec.col))
		if err != nil {
			fmt.Printf("%s.%s ERROR: %v\n", spec.table, spec.col, err)
			continue
		}
		fmt.Printf("%s.%s updated=%d\n", spec.table, spec.col, tag.RowsAffected())
	}
}
