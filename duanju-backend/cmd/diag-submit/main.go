// diag-submit 复现提交：查 card 11-13 的单元 beat_uuid 并尝试走完整提交前置检查。
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

	var beatUUID, puuid, pstatus string
	var ownerID int64
	err := pool.QueryRow(ctx, `
		SELECT nu.nu_uuid::text, p.project_uuid::text, p.status::text
		FROM narrative_units nu
		JOIN projects p ON p.id = nu.project_id
		WHERE nu.id = 170`).Scan(&beatUUID, &puuid, &pstatus)
	if err != nil {
		fmt.Println("unit:", err)
		return
	}
	_ = pool.QueryRow(ctx, `SELECT user_id FROM projects WHERE project_uuid=$1`, puuid).Scan(&ownerID)
	fmt.Printf("beat=%s project_status=%s\n", beatUUID[:8], pstatus)

	// 模拟 6002 门控：该单元关联资产就绪度明细
	rows, err := pool.Query(ctx, `
		SELECT nal.asset_kind::text, nal.asset_id,
		       COALESCE(c.name, sc.base_name, pr.name, '') AS name,
		       COALESCE(cv.main_image_url, sc.main_image_url, pr.main_image_url, '') AS img
		FROM nu_asset_links nal
		LEFT JOIN scenes sc ON nal.asset_kind='SCENE' AND sc.id=nal.asset_id AND sc.deleted_at IS NULL
		LEFT JOIN props pr ON nal.asset_kind='PROP' AND pr.id=nal.asset_id AND pr.deleted_at IS NULL
		LEFT JOIN characters c ON nal.asset_kind='CHARACTER' AND c.id=nal.asset_id AND c.deleted_at IS NULL
		LEFT JOIN LATERAL (SELECT main_image_url FROM character_variants cv
			WHERE cv.character_id=c.id AND cv.deleted_at IS NULL ORDER BY id LIMIT 1) cv ON TRUE
		WHERE nal.narrative_unit_id = 170`)
	if err != nil {
		fmt.Println("assets:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var kind, name, img string
		var aid int64
		_ = rows.Scan(&kind, &aid, &name, &img)
		status := "READY"
		if img == "" {
			status = "NO_IMAGE"
		}
		fmt.Printf("asset %s/%d %s -> %s\n", kind, aid, name, status)
	}

	// 用户积分余额（ Freeze 失败=积分不足）
	var balance int64
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0) FROM point_transactions WHERE user_id=$1`, ownerID).Scan(&balance); err == nil {
		fmt.Printf("user=%d points_balance=%d\n", ownerID, balance)
	}
}
