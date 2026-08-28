// diag-why-ok 对比成功卡与失败卡：成功卡的参考图 URL 是什么形态（解释"之前为什么可以"）。
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
		SELECT vc.id, vc.generation_status::text, vc.narrative_unit_id,
		       COALESCE(vr.refs, '[]')
		FROM video_cards vc
		LEFT JOIN LATERAL (
			SELECT json_agg(DISTINCT u) AS refs FROM (
				SELECT LEFT(COALESCE(cv.main_image_url, sc.main_image_url, pr.main_image_url), 60) AS u
				FROM shots sh
				JOIN shot_asset_refs sar ON sar.shot_id = sh.id
				LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
				LEFT JOIN props pr ON sar.asset_kind='PROP' AND pr.id=sar.asset_id AND pr.deleted_at IS NULL
				LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
				LEFT JOIN LATERAL (SELECT main_image_url FROM character_variants cv
					WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY id LIMIT 1) cv ON TRUE
				WHERE sh.video_card_id = vc.id
			) t WHERE u IS NOT NULL
		) vr ON TRUE
		WHERE vc.deleted_at IS NULL AND vc.generated_video_url IS NOT NULL
		ORDER BY vc.id DESC LIMIT 5`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var st string
		var nuID int64
		var refs []byte
		_ = rows.Scan(&id, &st, &nuID, &refs)
		fmt.Printf("card=%d %s unit=%d refs=%s\n", id, st, nuID, string(refs))
	}
}
