// diag-refs 查看当前卡 15/16 的参考图 URL 是否可达（诊断远程生成前置条件）。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable")
	}
	pool, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	defer pool.Close()
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT DISTINCT COALESCE(cv.main_image_url, sc.main_image_url, pr.main_image_url)
		FROM video_cards vc
		JOIN shots sh ON sh.video_card_id = vc.id AND sh.deleted_at IS NULL
		JOIN shot_asset_refs sar ON sar.shot_id = sh.id
		LEFT JOIN scenes sc ON sar.asset_kind='SCENE' AND sc.id=sar.asset_id AND sc.deleted_at IS NULL
		LEFT JOIN props pr ON sar.asset_kind='PROP' AND pr.id=sar.asset_id AND pr.deleted_at IS NULL
		LEFT JOIN characters c ON sar.asset_kind='CHARACTER' AND c.id=sar.asset_id AND c.deleted_at IS NULL
		LEFT JOIN LATERAL (SELECT main_image_url FROM character_variants cv
			WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY id LIMIT 1) cv ON TRUE
		WHERE vc.id IN (15, 16)`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer rows.Close()
	client := &http.Client{Timeout: 8 * time.Second}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil || u == "" {
			continue
		}
		resp, err := client.Get(u)
		status := "UNREACHABLE: " + shortErr(err)
		if err == nil {
			defer resp.Body.Close()
			status = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		show := u
		if len(show) > 80 {
			show = show[:80] + "..."
		}
		fmt.Printf("%s -> %s\n", show, status)
	}
}

func shortErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 60 {
		return s[:60]
	}
	return s
}
