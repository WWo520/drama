// diag-video-tasks 列出最近 VIDEO_SEGMENT_GEN 任务终态与错误（用户"又失败了"排查）。
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
	pool, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	rows, err := pool.Query(context.Background(), `
		SELECT g.id, g.status::text, COALESCE(g.retry_count,0),
		       to_char(g.created_at AT TIME ZONE 'Asia/Shanghai','HH24:MI'),
		       LEFT(COALESCE(g.error_message,''), 90),
		       g.request_params_json::text
		FROM generation_tasks g
		WHERE g.task_type='VIDEO_SEGMENT_GEN'
		ORDER BY g.id DESC LIMIT 30`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, emsg, params string
		var retries int
		var created string
		_ = rows.Scan(&id, &status, &retries, &created, &emsg, &params)
		line := fmt.Sprintf("task=%s %s retry=%d %s", id, status, retries, created)
		if len(params) > 40 {
			params = params[:40]
		}
		line += " " + params
		if emsg != "" {
			line += "\n   ERR: " + emsg
		}
		fmt.Println(line)
	}
}
