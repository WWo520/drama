// scripts/check-asset-images.go - 检查资产图片存储状态
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatalf("无法连接数据库：%v", err)
	}
	defer pool.Close(ctx)

	fmt.Println("=" + repeat(80, "="))
	fmt.Println("资产图片存储状态诊断")
	fmt.Println("=" + repeat(80, "="))

	// 1. 检查角色及其变体图片
	fmt.Println("\n[1/6] 检查角色变体图片...")
	checkVariants(ctx, pool)

	// 2. 检查场景图片
	fmt.Println("\n[2/6] 检查场景图片...")
	checkScenes(ctx, pool)

	// 3. 检查道具图片
	fmt.Println("\n[3/6] 检查道具图片...")
	checkProps(ctx, pool)

	// 4. 检查 generated_images 表
	fmt.Println("\n[4/6] 检查 generated_images 生成记录...")
	checkGeneratedImages(ctx, pool)

	// 5. 检查失败资产统计
	fmt.Println("\n[5/6] 检查失败资产...")
	checkFailedAssets(ctx, pool)

	// 6. 检查 TOS URL 过期情况
	fmt.Println("\n[6/6] 检查图片 URL 类型...")
	checkURLTypes(ctx, pool)

	fmt.Println("\n" + repeat(80, "="))
	fmt.Println("诊断完成")
	fmt.Println(repeat(80, "="))
}

func checkVariants(ctx context.Context, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, `
		SELECT 
			c.id as char_id,
			c.name as char_name,
			cv.id as variant_id,
			cv.variant_name,
			COALESCE(cv.main_image_url, '') as image_url,
			cv.generation_status::text as status,
			CASE 
				WHEN COALESCE(cv.main_image_url, '') = '' THEN '无图'
				WHEN cv.main_image_url LIKE '%volces.com%' AND cv.main_image_url LIKE '%X-Tos-Expires%' THEN 'TOS 签名 URL(可能已过期)'
				WHEN cv.main_image_url LIKE '%127.0.0.1%' OR cv.main_image_url LIKE '%localhost%' THEN '私网地址 (不可达)'
				ELSE '可用'
			END as url_type
		FROM characters c
		LEFT JOIN LATERAL (
			SELECT * FROM character_variants 
			WHERE character_id = c.id AND deleted_at IS NULL 
			ORDER BY id LIMIT 1
		) cv ON true
		WHERE c.deleted_at IS NULL
		ORDER BY c.id
		LIMIT 20
	`)
	if err != nil {
		fmt.Printf("查询失败：%v\n", err)
		return
	}
	defer rows.Close()

	noImageCount := 0
	expiredCount := 0
	totalCount := 0

	for rows.Next() {
		var charID, variantID int64
		var charName, variantName, imgUrl, status, urlType string
		if err := rows.Scan(&charID, &charName, &variantID, &variantName, &imgUrl, &status, &urlType); err != nil {
			continue
		}
		totalCount++
		if imgUrl == "" {
			noImageCount++
			urlType = "❌ 缺失"
		} else if contains(urlType, "过期") {
			expiredCount++
			urlType = "⚠️ " + urlType
		}
		fmt.Printf("  角色 [%d]%s -> %s (变体：%s)\n", charID, charName, status, variantName)
		fmt.Printf("    主图 URL 长度：%d | 类型：%s\n", len(imgUrl), urlType)
		if imgUrl != "" && len(imgUrl) > 60 {
			fmt.Printf("    %s...\n", imgUrl[:60])
		}
	}

	fmt.Printf("\n统计：总计 %d 个角色，%d 个无图，%d 个可能 URL 过期\n", totalCount, noImageCount, expiredCount)
}

func checkScenes(ctx context.Context, pool *pgxpool.Pool) {
	stats(ctx, pool, `
		SELECT 
			id, base_name,
			COALESCE(main_image_url, '') as image_url,
			generation_status::text as status
		FROM scenes WHERE deleted_at IS NULL
		ORDER BY id
		LIMIT 10
	`, "场景")
}

func checkProps(ctx context.Context, pool *pgxpool.Pool) {
	stats(ctx, pool, `
		SELECT 
			id, name,
			COALESCE(main_image_url, '') as image_url,
			generation_status::text as status
		FROM props WHERE deleted_at IS NULL
		ORDER BY id
		Limit 10
	`, "道具")
}

func stats(ctx context.Context, pool *pgxpool.Pool, sql, label string) {
	rows, err := pool.Query(ctx, sql)
	if err != nil {
		fmt.Printf("%s查询失败：%v\n", label, err)
		return
	}
	defer rows.Close()

	var emptyCount, total int
	for rows.Next() {
		total++
		var id int64
		var name, imgUrl, status string
		if err := rows.Scan(&id, &name, &imgUrl, &status); err != nil {
			continue
		}
		if imgUrl == "" {
			emptyCount++
			fmt.Printf("  ❌ [%d]%s 状态=%s 无图\n", id, name, status)
		} else {
			fmt.Printf("  ✓ [%d]%s 状态=%s 图长=%d\n", id, name, status, len(imgUrl))
		}
	}

	fmt.Printf("\n【统计】%s：总计%d, 无图%d (%.1f%%)\n", label, total, emptyCount, float64(emptyCount)/float64(total)*100)
}

func checkGeneratedImages(ctx context.Context, pool *pgxpool.Pool) {
	result, err := pool.Exec(ctx, `
		SELECT 
			state,
			count(*),
			bool_or(original_url LIKE '%volces.com%') as has_tos_url,
			bool_or(original_url = '') as has_empty_url
		FROM generated_images 
		WHERE deleted_at IS NULL 
		GROUP BY state
	`)
	if err != nil {
		fmt.Printf("查询失败：%v\n", err)
		return
	}
	rows, _ := pool.Query(ctx, `SELECT state, count(*), original_url LIKE '%volces.com%', original_url = '' FROM generated_images WHERE deleted_at IS NULL GROUP BY state`)
	defer rows.Close()

	fmt.Println("generated_images 表状态分布:")
	for rows.Next() {
		var state string
		var cnt int
		var hasTOS, isEmpty bool
		if err := rows.Scan(&state, &cnt, &hasTOS, &isEmpty); err != nil {
			continue
		}
		tosMark := ""
		if hasTOS {
			tosMark = " [含 TOS 签名 URL]"
		}
		if isEmpty {
			tosMark += " [空 URL]"
		}
		fmt.Printf("  %s: %d 条%s\n", state, cnt, tosMark)
	}
	fmt.Printf("影响行数：%v\n", result.RowsAffected())
}

func checkFailedAssets(ctx context.Context, pool *pgxpool.Pool) {
	fmt.Println("失败的资产统计:")
	
	queries := map[string]string{
		"角色变体": `SELECT COUNT(*) FROM character_variants WHERE generation_status = 'FAILED_DEAD' AND deleted_at IS NULL`,
		"场景":    `SELECT COUNT(*) FROM scenes WHERE generation_status = 'FAILED_DEAD' AND deleted_at IS NULL`,
		"道具":    `SELECT COUNT(*) FROM props WHERE generation_status = 'FAILED_DEAD' AND deleted_at IS NULL`,
	}

	for name, sql := range queries {
		var cnt int
		if err := pool.QueryRow(ctx, sql).Scan(&cnt); err != nil {
			fmt.Printf("  %s查询失败：%v\n", name, err)
			continue
		}
		fmt.Printf("  %s FAILED_DEAD: %d\n", name, cnt)
	}
}

func checkURLTypes(ctx context.Context, pool *pgxpool.Pool) {
	fmt.Println("资产 URL 类型分布:")
	
	queries := map[string]string{
		"角色变体": `SELECT COALESCE(main_image_url, '') != '', main_image_url LIKE '%volces.com%', main_image_url = '' FROM character_variants WHERE deleted_at IS NULL`,
		"场景":    `SELECT COALESCE(main_image_url, '') != '', main_image_url LIKE '%volces.com%', main_image_url = '' FROM scenes WHERE deleted_at IS NULL`,
		"道具":    `SELECT COALESCE(main_image_url, '') != '', main_image_url LIKE '%volces.com%', main_image_url = '' FROM props WHERE deleted_at IS NULL`,
	}

	for name, sql := range queries {
		rows, err := pool.Query(ctx, sql)
		if err != nil {
			fmt.Printf("  %s 查询失败：%v\n", name, err)
			continue
		}
		
		hasUrl, hasTOS, hasEmpty := 0, 0, 0
		for rows.Next() {
			var has, isTOS, isEmpty bool
			if err := rows.Scan(&has, &isTOS, &isEmpty); err != nil {
				continue
			}
			if has {
				hasUrl++
				if isTOS {
					hasTOS++
				}
			} else {
				hasEmpty++
			}
		}
		fmt.Printf("  %s: 有图%d, TOS 签名%d, 无图%d\n", name, hasUrl, hasTOS, hasEmpty)
	}
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
