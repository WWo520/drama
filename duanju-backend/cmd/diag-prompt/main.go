// diag-prompt 诊断最新成片卡/镜头的实际提示词内容（排查拼接不流畅）。
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

	fmt.Println("===== 最新 3 张成片卡 prompt_text =====")
	rows, err := pool.Query(context.Background(), `
		SELECT id, title, duration_sec, shot_count,
		       LEFT(COALESCE(prompt_text,''), 400)
		FROM video_cards WHERE deleted_at IS NULL
		ORDER BY id DESC LIMIT 3`)
	if err != nil {
		fmt.Println("query err:", err)
		return
	}
	for rows.Next() {
		var id int64
		var title string
		var dur, cnt int
		var pt string
		_ = rows.Scan(&id, &title, &dur, &cnt, &pt)
		fmt.Printf("--- card %d 「%s」 %ds/%d镜\n%s\n\n", id, title, dur, cnt, pt)
	}
	rows.Close()

	fmt.Println("===== 最近生成的 6 个镜头 prompt_refined 头部 =====")
	r2, err := pool.Query(context.Background(), `
		SELECT id, LEFT(COALESCE(prompt_refined,''), 300)
		FROM shots WHERE deleted_at IS NULL AND COALESCE(prompt_refined,'') <> ''
		ORDER BY updated_at DESC NULLS LAST, id DESC LIMIT 6`)
	if err != nil {
		fmt.Println("query err:", err)
		return
	}
	for r2.Next() {
		var id int64
		var pr string
		_ = r2.Scan(&id, &pr)
		fmt.Printf("--- shot %d\n%s\n\n", id, pr)
	}
}
