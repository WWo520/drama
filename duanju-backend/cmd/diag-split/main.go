// diag-split 诊断拆分任务状态（QUEUED/RUNNING 卡住、重试计数、耗时）。
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
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT g.id, g.status::text, COALESCE(g.retry_count,0),
		       to_char(g.created_at AT TIME ZONE 'Asia/Shanghai','HH24:MI'),
		       to_char(g.updated_at AT TIME ZONE 'Asia/Shanghai','HH24:MI'),
		       LEFT(COALESCE(g.error_message,''), 60)
		FROM generation_tasks g
		WHERE g.task_type='SCRIPT_SPLIT'
		ORDER BY g.id DESC LIMIT 8`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, created, updated string
		var retries int
		var emsg string
		_ = rows.Scan(&id, &status, &retries, &created, &updated, &emsg)
		if emsg == "-" {
			emsg = ""
		}
		fmt.Printf("task=%s %s retry=%d %s->%s %s\n", id, status, retries, created, updated, emsg)
	}

	prows, err := pool.Query(ctx, `
		SELECT p.id, p.status::text FROM projects p ORDER BY p.updated_at DESC LIMIT 4`)
	if err != nil {
		return
	}
	defer prows.Close()
	fmt.Println("-- projects --")
	for prows.Next() {
		var id int64
		var st string
		_ = prows.Scan(&id, &st)
		fmt.Printf("project=%d %s\n", id, st)
	}
}
