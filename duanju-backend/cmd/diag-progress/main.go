// diag-cards2 查卡片归属与生成提交前的门控状态。
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
	ctx := context.Background()

	// 最近 3 张卡 → 所属单元/项目/用户
	rows, err := pool.Query(ctx, `
		SELECT vc.id, vc.title, vc.generation_status::text, vc.narrative_unit_id,
		       p.project_uuid::text, p.status::text
		FROM video_cards vc
		JOIN narrative_units nu ON nu.id = vc.narrative_unit_id
		JOIN projects p ON p.id = nu.project_id
		WHERE vc.deleted_at IS NULL ORDER BY vc.id DESC LIMIT 5`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title, cstatus, puuid, pstatus string
		var nuID int64
		_ = rows.Scan(&id, &title, &cstatus, &nuID, &puuid, &pstatus)
		fmt.Printf("card=%d [%s] %s | unit=%d project=%s status=%s\n",
			id, title, cstatus, nuID, puuid[:8], pstatus)
	}

	// 这些单元的资产就绪度（linked 但无图=会触发 6002）
	arows, _ := pool.Query(ctx, `
		SELECT nal.narrative_unit_id, count(*),
		       count(*) FILTER (WHERE COALESCE(cv.main_image_url, sc.main_image_url, pr.main_image_url, '') <> '')
		FROM nu_asset_links nal
		LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
		LEFT JOIN props pr ON nal.asset_kind='PROP' AND pr.id=nal.asset_id AND pr.deleted_at IS NULL
		LEFT JOIN characters ch ON nal.asset_kind='CHARACTER' AND ch.id=nal.asset_id AND ch.deleted_at IS NULL
		LEFT JOIN LATERAL (SELECT main_image_url FROM character_variants cv
			WHERE cv.character_id=ch.id AND cv.deleted_at IS NULL ORDER BY id LIMIT 1) cv ON TRUE
		GROUP BY nal.narrative_unit_id ORDER BY 1 DESC LIMIT 5`)
	if err == nil {
		fmt.Println("-- unit asset readiness --")
		defer arows.Close()
		for arows.Next() {
			var uid int64
			var total, ready int
			_ = arows.Scan(&uid, &total, &ready)
			fmt.Printf("unit=%d linked=%d ready=%d\n", uid, total, ready)
		}
	}
}
