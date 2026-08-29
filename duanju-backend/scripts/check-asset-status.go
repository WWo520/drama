//go:build ignore

// check-asset-status.go - 璇婃柇璧勪骇鍥剧墖鏄剧ず闂
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
		log.Fatalf("鏃犳硶杩炴帴鏁版嵁搴擄細%v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("鏁版嵁搴?Ping 澶辫触锛?v", err)
	}

	fmt.Println("=" + repeat(80, "="))
	fmt.Println("璧勪骇鍥剧墖鏄剧ず闂璇婃柇鎶ュ憡")
	fmt.Println("鏃堕棿锛? + time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("=" + repeat(80, "="))

	// 1. 缁熻鍚勮〃鏁版嵁
	checkSummary(ctx, db)

	// 2. 璇︾粏妫€鏌ユ棤鍥捐祫浜?
	noImageCharacters(ctx, db)
	noImageScenes(ctx, db)
	noImageProps(ctx, db)

	// 3. TOS URL 杩囨湡鍒嗘瀽
	tosURLAnalysis(ctx, db)

	fmt.Println("\n" + repeat(80, "="))
	fmt.Println("馃挕 寤鸿鎺柦:")
	fmt.Println("   1. 瀵逛簬 PENDING/FAILED_DEAD 鐘舵€佺殑璧勪骇 鈫?閲嶆柊鐢熸垚")
	fmt.Println("   2. 瀵逛簬 TOS 绛惧悕 URL 鈫?宸茶嚜鍔ㄨ浆瀛樺埌鑷湁瀛樺偍锛屽簲璇ユ甯?)
	fmt.Println("   3. 瀵逛簬瀹屽叏鏃犲浘鐨?SUCCESS 璧勪骇 鈫?妫€鏌?saveGeneratedImage 娴佺▼")
	fmt.Println(repeat(80, "="))
}

func checkSummary(ctx context.Context, db *sql.DB) {
	fmt.Println("\n銆愪竴銆佹€讳綋缁熻銆慭n")

	queries := map[string]string{
		"瑙掕壊鎬绘暟":           `SELECT COUNT(*) FROM characters WHERE deleted_at IS NULL`,
		"鏈夊浘鐗囩殑瑙掕壊鍙樹綋":    `SELECT COUNT(*) FROM character_variants WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''`,
		"鍦烘櫙鎬绘暟":           `SELECT COUNT(*) FROM scenes WHERE deleted_at IS NULL`,
		"鏈夊浘鐗囩殑鍦烘櫙":       `SELECT COUNT(*) FROM scenes WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''`,
		"閬撳叿鎬绘暟":           `SELECT COUNT(*) FROM props WHERE deleted_at IS NULL`,
		"鏈夊浘鐗囩殑閬撳叿":       `SELECT COUNT(*) FROM props WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''`,
		"鐢熸垚鐨勫浘鐗囪褰?:      `SELECT COUNT(*) FROM generated_images WHERE deleted_at IS NULL`,
		"鎴愬姛鐨勫浘鐗囪褰?:      `SELECT COUNT(*) FROM generated_images WHERE state='SUCCESS'`,
	}

	results := make(map[int64]string)
	for name, query := range queries {
		var count int64
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			fmt.Printf("鉂?%s鏌ヨ澶辫触锛?v\n", name, err)
			continue
		}
		results[name] = fmt.Sprintf("%d", count)
		fmt.Printf("鉁?%-15s: %d\n", name+":", count)
	}

	// 璁＄畻鏈夊浘鐜?
	if v, ok := results["瑙掕壊鎬绘暟"]; ok && v != "0" {
		var variantsTotal int64
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_variants WHERE deleted_at IS NULL").Scan(&variantsTotal)
		rate := float64(0)
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_variants WHERE generation_status='SUCCESS' AND COALESCE(main_image_url,'') <> ''").Scan(&rate)
		fmt.Printf("馃搳 瑙掕壊鍥剧墖瑕嗙洊鐜囷細%.1f%% (%.0f/%.0f)\n", rate/float64(variantsTotal)*100, rate, variantsTotal)
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
		fmt.Printf("\n鉂?鏌ヨ鏃犲浘瑙掕壊澶辫触锛?v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n銆愪簩銆佹棤鍥捐鑹茬ず渚?(鍓?10 涓?銆慭n")
	count := 0
	for rows.Next() {
		count++
		var name, variantName, status, url string
		rows.Scan(&name, &variantName, &status, &url)
		fmt.Printf("%d. [%s] %s (鍙樹綋:%s) 鐘舵€?%s\n", count, truncate(name, 15), truncate(variantName, 15), status, truncate(url, 40))
	}
	if count == 0 {
		fmt.Println("鉁?鎵€鏈夎鑹查兘鏈夊浘鐗?)
	} else {
		fmt.Printf("\n鈫?鎬昏 %d 涓鑹茬己鍥綷n", count)
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
		fmt.Printf("\n鉂?鏌ヨ鏃犲浘鍦烘櫙澶辫触锛?v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n銆愪笁銆佹棤鍥惧満鏅ず渚?(鍓?10 涓?銆慭n")
	count := 0
	for rows.Next() {
		count++
		var name, status, url string
		rows.Scan(&name, &status, &url)
		fmt.Printf("%d. [%s] 鐘舵€?%s URL=%s\n", count, truncate(name, 20), status, truncate(url, 40))
	}
	if count == 0 {
		fmt.Println("鉁?鎵€鏈夊満鏅兘鏈夊浘鐗?)
	} else {
		fmt.Printf("\n鈫?鎬昏 %d 涓満鏅己鍥綷n", count)
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
		fmt.Printf("\n鉂?鏌ヨ鏃犲浘閬撳叿澶辫触锛?v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n銆愬洓銆佹棤鍥鹃亾鍏风ず渚?(鍓?10 涓?銆慭n")
	count := 0
	for rows.Next() {
		count++
		var name, status, url string
		rows.Scan(&name, &status, &url)
		fmt.Printf("%d. [%s] 鐘舵€?%s URL=%s\n", count, truncate(name, 20), status, truncate(url, 40))
	}
	if count == 0 {
		fmt.Println("鉁?鎵€鏈夐亾鍏烽兘鏈夊浘鐗?)
	} else {
		fmt.Printf("\n鈫?鎬昏 %d 涓亾鍏风己鍥綷n", count)
	}
}

func tosURLAnalysis(ctx context.Context, db *sql.DB) {
	fmt.Println("\n銆愪簲銆乀OS 绛惧悕 URL 鍒嗘瀽銆慭n")

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

	fmt.Printf("鈿狅笍  瑙掕壊浣跨敤 TOS 绛惧悕 URL: %d 涓猏n", charCount)
	fmt.Printf("鈿狅笍  鍦烘櫙浣跨敤 TOS 绛惧悕 URL: %d 涓猏n", sceneCount)
	fmt.Printf("鈿狅笍  閬撳叿浣跨敤 TOS 绛惧悕 URL: %d 涓猏n", propCount)

	if charCount+sceneCount+propCount > 0 {
		fmt.Println("\n馃挕 鎻愮ず锛氳繖浜?URL 宸茶繃鏈燂紒浣嗗簲璇ュ湪 asset_handler 涓凡璋冪敤 persistGeneratedImage() 杞瓨")
		fmt.Println("          濡傛灉鍓嶇浠嶇湅涓嶅埌锛岃鏄庢寔涔呭寲澶辫触鎴栨湭鎵ц")
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

