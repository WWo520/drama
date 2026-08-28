// diag-status 查看项目状态机现状（排查合成按钮禁用等状态类问题）。
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
	fmt.Println("== 最近更新的项目 ==")
	rows, err := pool.Query(context.Background(), `
		SELECT p.id, LEFT(p.title,20), p.status::text,
		       (SELECT count(*) FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL) AS total_shots,
		       (SELECT count(*) FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
		         AND s.video_generation_status::text='SUCCESS') AS done_shots,
		       (SELECT count(*) FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
		         AND s.video_generation_status::text='FAILED') AS failed_shots
		FROM projects p WHERE p.deleted_at IS NULL ORDER BY p.updated_at DESC LIMIT 8`)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title, st string
		var total, done, failed int
		_ = rows.Scan(&id, &title, &st, &total, &done, &failed)
		fmt.Printf("project=%d「%s」 status=%s 镜头总数=%d 已出片=%d 失败=%d\n", id, title, st, total, done, failed)
	}
	rows.Close()

	// `fix` 参数：对卡在 VIDEO_GENERATING 且已有出片的项目做状态推进（语义同 VideoStore.CompleteVideoIfAllDone）。
	if len(os.Args) > 1 && os.Args[1] == "fix" {
		tag, ferr := pool.Exec(context.Background(), `
			UPDATE projects p SET
			  status = CASE WHEN EXISTS (
			      SELECT 1 FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
			        AND s.video_generation_status::text <> 'SUCCESS'
			  ) THEN 'PARTIAL_VIDEOS'::project_status_enum
			      ELSE 'ALL_VIDEOS_READY'::project_status_enum END,
			  current_step = CASE WHEN EXISTS (
		      SELECT 1 FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
		        AND s.video_generation_status::text <> 'SUCCESS'
		  ) THEN 'VIDEO_PRODUCTION'::project_step_enum ELSE 'FINAL_PREVIEW'::project_step_enum END,
			  updated_at = NOW()
			WHERE p.status='VIDEO_GENERATING'
			  AND EXISTS (SELECT 1 FROM shots s WHERE s.project_id=p.id AND s.deleted_at IS NULL
			        AND s.video_generation_status::text='SUCCESS')`)
		if ferr != nil {
			fmt.Println("fix err:", ferr)
			return
		}
		fmt.Printf("已推进 %d 个项目的状态机（VIDEO_GENERATING → PARTIAL_VIDEOS/ALL_VIDEOS_READY）\n", tag.RowsAffected())
	}
}
