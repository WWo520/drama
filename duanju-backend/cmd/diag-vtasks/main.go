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

statusRows, err := pool.Query(context.Background(), `
SELECT status::text, COUNT(*)
FROM generation_tasks
WHERE task_type='VIDEO_SEGMENT_GEN'
GROUP BY status`)
if err != nil {
fmt.Fprintln(os.Stderr, "status query:", err)
return
}
fmt.Println("=== status stats ===")
for statusRows.Next() {
var status string
var cnt int
_ = statusRows.Scan(&status, &cnt)
fmt.Printf("%s: %d\n", status, cnt)
}
statusRows.Close()

rows, err := pool.Query(context.Background(), `
SELECT id, status::text, COALESCE(retry_count,0),
       to_char(created_at AT TIME ZONE 'Asia/Shanghai','MM-DD HH24:MI') AS created,
       to_char(updated_at AT TIME ZONE 'Asia/Shanghai','MM-DD HH24:MI') AS updated,
       LEFT(COALESCE(error_message,''), 500) AS err,
       request_params_json::text
FROM generation_tasks
WHERE task_type='VIDEO_SEGMENT_GEN'
ORDER BY id DESC LIMIT 15`)
if err != nil {
fmt.Fprintln(os.Stderr, "detail query:", err)
return
}
defer rows.Close()
fmt.Println("\n=== recent 15 tasks ===")
for rows.Next() {
var id, status, created, updated, emsg, params string
var retries int
_ = rows.Scan(&id, &status, &retries, &created, &updated, &emsg, &params)
line := fmt.Sprintf("task=%s %s retry=%d created=%s updated=%s", id, status, retries, created, updated)
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
