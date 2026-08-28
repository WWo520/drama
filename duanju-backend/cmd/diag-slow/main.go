// diag-slow 查看任务耗时与错误明细（排查视频生成慢）。
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
		SELECT g.id, g.status::text,
		       to_char(g.created_at AT TIME ZONE 'Asia/Shanghai','HH24:MI'),
		       to_char(g.started_at AT TIME ZONE 'Asia/Shanghai','HH24:MI'),
		       to_char(g.updated_at AT TIME ZONE 'Asia/Shanghai','HH24:MI'),
		       COALESCE(g.retry_count,0),
		       LEFT(COALESCE(g.error_message,''),110)
		FROM generation_tasks g
		WHERE g.id >= 564
		ORDER BY g.id DESC`)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var st, created, started, updated, emsg string
		var rc int
		_ = rows.Scan(&id, &st, &created, &started, &updated, &rc, &emsg)
		line := fmt.Sprintf("task=%d %-11s 创建:%s 开始:%s 更新:%s retry=%d", id, st, created, started, updated, rc)
		if emsg != "" {
			line += "\n     ERR: " + emsg
		}
		fmt.Println(line)
	}

	r2, err := pool.Query(context.Background(), `
		SELECT task_type, count(*) FROM generation_tasks
		WHERE created_at > now() - interval '3 hours' AND task_type IN ('SHOT_REF_IMAGE_GEN')
		GROUP BY task_type`)
	if err == nil {
		defer r2.Close()
		for r2.Next() {
			var tt string
			var n int
			_ = r2.Scan(&tt, &n)
			fmt.Printf("近3小时 %s 任务数: %d\n", tt, n)
		}
	}
}
