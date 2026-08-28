// diag-cards：一次性诊断——成片卡状态 vs 任务终态、资产图片存量；-fix 清理僵尸卡。
// 用法: go run ./cmd/diag-cards [-fix]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/duanju/backend/internal/domain/creation"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable")
	}
	dsn := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()
	ctx := context.Background()

	fix := flag.Bool("fix", false, "清理僵尸 RUNNING 卡与镜头")
	flag.Parse()
	if *fix {
		c, s, ferr := creation.NewRepo(pool).RepairStaleVideoCards(ctx)
		fmt.Printf("REPAIR: cards=%d shots=%d err=%v\n", c, s, ferr)
	}

	fmt.Println("==== 卡片状态 vs 最近任务 ====")
	rows, err := pool.Query(ctx, `
		SELECT vc.id, vc.title, vc.generation_status::text,
		       COALESCE(vc.generated_video_url,'') != '' AS has_url,
		       g.id IS NOT NULL AS has_task, COALESCE(g.status::text,'-') AS task_status,
		       COALESCE(g.retry_count,0), COALESCE(g.error_message,'')
		FROM video_cards vc
		LEFT JOIN LATERAL (
			SELECT id, status, retry_count, error_message FROM generation_tasks
			WHERE id = vc.generation_task_id
		) g ON TRUE
		WHERE vc.deleted_at IS NULL
		ORDER BY vc.id DESC LIMIT 12`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cards:", err)
	} else {
		for rows.Next() {
			var id int64
			var title, cstatus, tstatus, emsg string
			var hasURL, hasTask bool
			var retries int
			_ = rows.Scan(&id, &title, &cstatus, &hasURL, &hasTask, &tstatus, &retries, &emsg)
			line := fmt.Sprintf("card=%d [%s] card_status=%s has_url=%v | task=%v(%s) retries=%d", id, title, cstatus, hasURL, hasTask, tstatus, retries)
			if emsg != "" && len(emsg) > 80 {
				emsg = emsg[:80]
			}
			if emsg != "" {
				line += " err=" + emsg
			}
			fmt.Println(line)
		}
		rows.Close()
	}

	fmt.Println("==== 各项目资产图片存量（角色主形象/场景/道具） ====")
	arows, err := pool.Query(ctx, `
		SELECT p.title,
		       (SELECT count(*) FROM characters c WHERE c.project_id=p.id AND c.deleted_at IS NULL) AS chars,
		       (SELECT count(*) FROM character_variants cv JOIN characters c2 ON c2.id=cv.character_id
		         WHERE c2.project_id=p.id AND c2.deleted_at IS NULL AND cv.deleted_at IS NULL) AS variants,
		       (SELECT count(*) FROM character_variants cv2 JOIN characters c3 ON c3.id=cv2.character_id
		         WHERE c3.project_id=p.id AND c3.deleted_at IS NULL AND cv2.deleted_at IS NULL
		           AND COALESCE(cv2.main_image_url,'')!='') AS var_with_img,
		       (SELECT count(*) FROM scenes sc WHERE sc.project_id=p.id AND sc.deleted_at IS NULL) AS scenes,
		       (SELECT count(*) FROM scenes sc2 WHERE sc2.project_id=p.id AND sc2.deleted_at IS NULL AND COALESCE(sc2.main_image_url,'')!='') AS scenes_img,
		       (SELECT count(*) FROM props pr WHERE pr.project_id=p.id AND pr.deleted_at IS NULL) AS props,
		       (SELECT count(*) FROM props pr2 WHERE pr2.project_id=p.id AND pr2.deleted_at IS NULL AND COALESCE(pr2.main_image_url,'')!='') AS props_img,
		       (SELECT count(*) FROM generated_images gi
		          WHERE gi.state='SUCCESS' AND (
		            gi.target_kind='CHARACTER_VARIANT' OR gi.target_kind='SCENE' OR gi.target_kind='PROP'
		          ) AND EXISTS (SELECT 1 FROM characters cx WHERE gi.target_kind='CHARACTER_VARIANT'
		                AND CAST(gi.target_id AS text) ~ '^\d+$' AND cx.project_id=p.id
		                AND EXISTS (SELECT 1 FROM character_variants cvv WHERE cvv.character_id=cx.id AND cvv.id=CAST(gi.target_id AS bigint)))
		        ) + (SELECT count(*) FROM generated_images gi2 JOIN scenes sx ON gi2.target_kind='SCENE' AND sx.id=gi2.target_id AND sx.project_id=p.id
		           WHERE gi2.state='SUCCESS')
		        + (SELECT count(*) FROM generated_images gi3 JOIN props px ON gi3.target_kind='PROP' AND px.id=gi3.target_id AND px.project_id=p.id
		           WHERE gi3.state='SUCCESS') AS gen_images
		FROM projects p ORDER BY p.updated_at DESC LIMIT 6`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "assets:", err)
	} else {
		for arows.Next() {
			var title string
			var chars, variants, varImg, scenes, scenesImg, props, propsImg, genImages int
			_ = arows.Scan(&title, &chars, &variants, &varImg, &scenes, &scenesImg, &props, &propsImg, &genImages)
			fmt.Printf("%s | 角色%d(变体%d 有图%d) 场景%d(有图%d) 道具%d(有图%d) 历史图%d\n",
				title, chars, variants, varImg, scenes, scenesImg, props, propsImg, genImages)
		}
		arows.Close()
	}

	fmt.Println("==== 近期视频任务终态分布 ====")
	trows, err := pool.Query(ctx, `
		SELECT status::text, count(*), max(to_char(updated_at AT TIME ZONE 'Asia/Shanghai','MM-DD HH24:MI'))
		FROM generation_tasks WHERE task_type='VIDEO_SEGMENT_GEN'
		GROUP BY status::text ORDER BY 1`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
	} else {
		for trows.Next() {
			var st, lastAt string
			var n int
			_ = trows.Scan(&st, &n, &lastAt)
			fmt.Printf("%s x%d (最后活动 %s)\n", st, n, lastAt)
		}
		trows.Close()
	}
}
