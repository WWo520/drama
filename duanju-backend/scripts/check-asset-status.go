// check-asset-status.go - 诊断资产图片显示问题
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", url)
	if err != nil {
		log.Fatalf("无法连接数据库：%v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("数据库 Ping 失败：%v", err)
	}

	fmt.Println("=" + repeat(80, "="))
	fmt.Println("资产图片显示问题诊断报告")
	fmt.Println("时间：" + time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("=" + repeat(80, "="))

	// 1. 统计各表数据
	checkSummary(ctx, db)

	// 2. 详细检查无图资产
	noImageCharacters(ctx, db)
	noImageScenes(ctx, db)
	noImageProps(ctx, db)

	// 3. TOS URL 过期分析
	tosURLAnalysis(ctx, db)

	fmt.Println("\n" + repeat(80, "="))
	fmt.Println("💡 建议措施:")
	fmt.Println("   1. 对于 PENDING/FAILED_DEAD 状态的资产 → 重新生成")
	fmt.Println("   2. 对于 TOS 签名 URL → 已自动转存到自有存储，应该正常")
	fmt.Println("   3. 对于完全无图的 SUCCESS 资产 → 检查 saveGeneratedImage 流程")
	fmt.Println(repeat(80, "="))
}

func checkSummary(ctx context.Context, db *sql.DB) {
	fmt.Println("\n【一、总体统计】\n")

	queries := map[string]string{
		"角色总数":           `SELECT COUNT(*) FROM characters WHERE deleted_at IS NULL`,
		"有图片的角色变体":    `SELECT COUNT(*) FROM character_variants WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''`,
		"场景总数":           `SELECT COUNT(*) FROM scenes WHERE deleted_at IS NULL`,
		"有图片的场景":       `SELECT COUNT(*) FROM scenes WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''`,
		"道具总数":           `SELECT COUNT(*) FROM props WHERE deleted_at IS NULL`,
		"有图片的道具":       `SELECT COUNT(*) FROM props WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''`,
		"生成的图片记录":      `SELECT COUNT(*) FROM generated_images WHERE deleted_at IS NULL`,
		"成功的图片记录":      `SELECT COUNT(*) FROM generated_images WHERE state='SUCCESS'`,
	}

	results := make(map[int64]string)
	for name, query := range queries {
		var count int64
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			fmt.Printf("❌ %s查询失败：%v\n", name, err)
			continue
		}
		results[name] = fmt.Sprintf("%d", count)
		fmt.Printf("✓ %-15s: %d\n", name+":", count)
	}

	// 计算有图率
	if v, ok := results["角色总数"]; ok && v != "0" {
		var variantsTotal int64
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_variants WHERE deleted_at IS NULL").Scan(&variantsTotal)
		rate := float64(0)
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_variants WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''").Scan(&rate)
		fmt.Printf("📊 角色图片覆盖率：%.1f%% (%.0f/%.0f)\n", rate/float64(variantsTotal)*100, rate, variantsTotal)
	}
}

func noImageCharacters(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.name, cv.variant_name, cv.generation_status::text, 
		       LEFT(COALESCE(cv.main_image_url,''), 80) as url_preview
		FROM characters c
		LEFT JOIN LATERAL (
			SELECT * FROM character_variants 
			WHERE character_id = c.id AND deleted_at IS NULL 
			ORDER BY id LIMIT 1
		) cv ON true
		WHERE c.deleted_at IS NULL
		  AND (cv.main_image_url IS NULL OR cv.main_image_url = '')
		LIMIT 10
	`)
	if err != nil {
		fmt.Printf("\n❌ 查询无图角色失败：%v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n【二、无图角色示例 (前 10 个)】\n")
	count := 0
	for rows.Next() {
		count++
		var name, variantName, status, url string
		rows.Scan(&name, &variantName, &status, &url)
		fmt.Printf("%d. [%s] %s (变体:%s) 状态=%s\n", count, truncate(name, 15), truncate(variantName, 15), status, truncate(url, 40))
	}
	if count == 0 {
		fmt.Println("✓ 所有角色都有图片")
	} else {
		fmt.Printf("\n→ 总计 %d 个角色缺图\n", count)
	}
}

func noImageScenes(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx, `
		SELECT base_name, generation_status::text, LEFT(COALESCE(main_image_url,''), 60)
		FROM scenes 
		WHERE deleted_at IS NULL
		  AND (main_image_url IS NULL OR main_image_url = '')
		LIMIT 10
	`)
	if err != nil {
		fmt.Printf("\n❌ 查询无图场景失败：%v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n【三、无图场景示例 (前 10 个)】\n")
	count := 0
	for rows.Next() {
		count++
		var name, status, url string
		rows.Scan(&name, &status, &url)
		fmt.Printf("%d. [%s] 状态=%s URL=%s\n", count, truncate(name, 20), status, truncate(url, 40))
	}
	if count == 0 {
		fmt.Println("✓ 所有场景都有图片")
	} else {
		fmt.Printf("\n→ 总计 %d 个场景缺图\n", count)
	}
}

func noImageProps(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, generation_status::text, LEFT(COALESCE(main_image_url,''), 60)
		FROM props 
		WHERE deleted_at IS NULL
		  AND (main_image_url IS NULL OR main_image_url = '')
		LIMIT 10
	`)
	if err != nil {
		fmt.Printf("\n❌ 查询无图道具失败：%v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n【四、无图道具示例 (前 10 个)】\n")
	count := 0
	for rows.Next() {
		count++
		var name, status, url string
		rows.Scan(&name, &status, &url)
		fmt.Printf("%d. [%s] 状态=%s URL=%s\n", count, truncate(name, 20), status, truncate(url, 40))
	}
	if count == 0 {
		fmt.Println("✓ 所有道具都有图片")
	} else {
		fmt.Printf("\n→ 总计 %d 个道具缺图\n", count)
	}
}

func tosURLAnalysis(ctx context.Context, db *sql.DB) {
	fmt.Println("\n【五、TOS 签名 URL 分析】\n")

	var charCount, sceneCount, propCount int
	db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM character_variants 
		WHERE main_image_url LIKE '%volces.com%' 
		  AND main_image_url LIKE '%X-Tos-Expires%'
		  AND deleted_at IS NULL
	`).Scan(&charCount)

	db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scenes 
		WHERE main_image_url LIKE '%volces.com%' 
		  AND main_image_url LIKE '%X-Tos-Expires%'
		  AND deleted_at IS NULL
	`).Scan(&sceneCount)

	db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM props 
		WHERE main_image_url LIKE '%volces.com%' 
		  AND main_image_url LIKE '%X-Tos-Expires%'
		  AND deleted_at IS NULL
	`).Scan(&propCount)

	fmt.Printf("⚠️  角色使用 TOS 签名 URL: %d 个\n", charCount)
	fmt.Printf("⚠️  场景使用 TOS 签名 URL: %d 个\n", sceneCount)
	fmt.Printf("⚠️  道具使用 TOS 签名 URL: %d 个\n", propCount)

	if charCount+sceneCount+propCount > 0 {
		fmt.Println("\n💡 提示：这些 URL 已过期！但应该在 asset_handler 中已调用 persistGeneratedImage() 转存")
		fmt.Println("          如果前端仍看不到，说明持久化失败或未执行")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-2] + ".."
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
