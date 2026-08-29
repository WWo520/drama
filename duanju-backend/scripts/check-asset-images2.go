//go:build ignore

// check-asset-images2.go - 璧勪骇鐢熸垚鍥剧墖鏄剧ず闂鍏ㄩ潰璇婃柇
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("馃柤锔? 璧勪骇鍥剧墖鏄剧ず闂鍏ㄩ潰璇婃柇")
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 鍚勮祫浜ц〃鐨勫浘鐗囪鐩栨儏鍐?
	fmt.Println("\n銆?銆戣祫浜у浘鐗囪鐩栫粺璁?)
	checkTable(ctx, db, "character_variants", "瑙掕壊鍙樹綋")
	checkTable(ctx, db, "scenes", "鍦烘櫙")
	checkTable(ctx, db, "props", "閬撳叿")

	// 2. URL 绫诲瀷鍒嗗竷锛堝畾浣嶆槸杩囨湡/涓嶅彲杈?鍗犱綅锛?
	fmt.Println("\n銆?銆慤RL 绫诲瀷鍒嗗竷")
	urlTypes(ctx, db, "character_variants", "瑙掕壊鍙樹綋")
	urlTypes(ctx, db, "scenes", "鍦烘櫙")
	urlTypes(ctx, db, "props", "閬撳叿")

	// 2.5 鏃犲浘璧勪骇鐨勭姸鎬佸垎甯冿紙瀹氫綅鏄?PENDING 鏈敓鎴?/ SUCCESS 涓㈠浘锛?
	fmt.Println("\n銆?.5銆戞棤鍥捐祫浜х姸鎬佸垎甯?)
	emptyStatus(ctx, db, "character_variants", "瑙掕壊鍙樹綋")
	emptyStatus(ctx, db, "scenes", "鍦烘櫙")
	emptyStatus(ctx, db, "props", "閬撳叿")

	// 3. 鎶芥牱鏌ョ湅鐪熷疄 URL
	fmt.Println("\n銆?銆戞娊鏍?URL锛堝墠 8 鏉￠潪绌猴級")
	sampleURLs(ctx, db, "character_variants", "瑙掕壊鍙樹綋", "main_image_url", "variant_name")
	sampleURLs(ctx, db, "scenes", "鍦烘櫙", "main_image_url", "base_name")
	sampleURLs(ctx, db, "props", "閬撳叿", "main_image_url", "name")

	// 4. generated_images 鐘舵€?
	fmt.Println("\n銆?銆慻enerated_images 鐢熸垚璁板綍")
	rows, err := db.QueryContext(ctx, `
		SELECT state, COUNT(*) FROM generated_images WHERE deleted_at IS NULL GROUP BY state ORDER BY state`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows.Next() {
			var state string
			var cnt int
			rows.Scan(&state, &cnt)
			fmt.Printf("  %s: %d 鏉n", state, cnt)
		}
		rows.Close()
	}

	// 5. 鏈€杩戠敓鎴愪换鍔＄姸鎬?
	fmt.Println("\n銆?銆戞渶杩戣祫浜х敓鎴愪换鍔★紙5 鏉★級")
	rows5, err := db.QueryContext(ctx, `
		SELECT g.id, g.task_type, g.status, COALESCE(LEFT(g.error_message, 100), ''),
		       g.created_at::text
		FROM generation_tasks g
		WHERE g.task_type IN ('CHAR_IMAGE_GEN','SCENE_IMAGE_GEN','PROP_IMAGE_GEN')
		ORDER BY g.created_at DESC LIMIT 5`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows5.Next() {
			var id int64
			var taskType, status, errMsg, createdAt string
			rows5.Scan(&id, &taskType, &status, &errMsg, &createdAt)
			fmt.Printf("  浠诲姟[%d] %s 鐘舵€?%s 鍒涘缓=%s\n", id, taskType, status, createdAt)
			if errMsg != "" {
				fmt.Printf("    閿欒锛?s\n", errMsg)
			}
		}
		rows5.Close()
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

func checkTable(ctx context.Context, db *sql.DB, table, label string) {
	var total, withImg int
	var failed int
	err := db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE COALESCE(main_image_url,'') <> ''),
		       COUNT(*) FILTER (WHERE generation_status = 'FAILED_DEAD')
		FROM %s WHERE deleted_at IS NULL`, table)).Scan(&total, &withImg, &failed)
	if err != nil {
		fmt.Printf("  鉂?%s 鏌ヨ澶辫触锛?v\n", label, err)
		return
	}
	rate := 0.0
	if total > 0 {
		rate = float64(withImg) / float64(total) * 100
	}
	fmt.Printf("  %s: 鍏?%d | 鏈夊浘 %d (%.0f%%) | 澶辫触 %d\n", label, total, withImg, rate, failed)
}

func urlTypes(ctx context.Context, db *sql.DB, table, label string) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			COUNT(*) FILTER (WHERE main_image_url LIKE '%%volces.com%%' AND main_image_url LIKE '%%X-Tos-Expires%%') AS tos_signed,
			COUNT(*) FILTER (WHERE main_image_url LIKE '%%asset-placeholder%%' OR main_image_url LIKE '%%sample.mp4%%') AS mock,
			COUNT(*) FILTER (WHERE main_image_url LIKE '%%127.0.0.1%%' OR main_image_url LIKE '%%localhost%%' OR main_image_url LIKE '%%minio%%') AS local_minio,
			COUNT(*) FILTER (WHERE COALESCE(main_image_url,'') = '') AS empty,
			COUNT(*) FILTER (WHERE COALESCE(main_image_url,'') <> ''
				AND main_image_url NOT LIKE '%%volces.com%%'
				AND main_image_url NOT LIKE '%%asset-placeholder%%'
				AND main_image_url NOT LIKE '%%sample.mp4%%'
				AND main_image_url NOT LIKE '%%127.0.0.1%%'
				AND main_image_url NOT LIKE '%%localhost%%'
				AND main_image_url NOT LIKE '%%minio%%') AS other
		FROM %s WHERE deleted_at IS NULL`, table))
	if err != nil {
		fmt.Printf("  鉂?%s 鏌ヨ澶辫触锛?v\n", label, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var tos, mock, local, empty, other int
		rows.Scan(&tos, &mock, &local, &empty, &other)
		fmt.Printf("  %s: TOS绛惧悕=%d | mock鍗犱綅=%d | 鏈湴MinIO=%d | 绌?%d | 鍏朵粬=%d\n",
			label, tos, mock, local, empty, other)
	}
}

// emptyStatus 鏃犲浘璧勪骇鐨勭姸鎬佸垎甯?
func emptyStatus(ctx context.Context, db *sql.DB, table, label string) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT COALESCE(generation_status::text, 'NULL'), COUNT(*)
		FROM %s WHERE deleted_at IS NULL AND COALESCE(main_image_url,'') = ''
		GROUP BY generation_status ORDER BY COUNT(*) DESC`, table))
	if err != nil {
		fmt.Printf("  鉂?%s 鏌ヨ澶辫触锛?v\n", label, err)
		return
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		var status string
		var cnt int
		rows.Scan(&status, &cnt)
		if first {
			fmt.Printf("  %s 鏃犲浘鍒嗗竷锛?, label)
			first = false
		} else {
			fmt.Printf(" | ")
		}
		fmt.Printf("%s=%d", status, cnt)
	}
	if first {
		fmt.Printf("  %s 鏃犲浘鍒嗗竷锛氭棤", label)
	}
	fmt.Println()
}

// sampleURLs 鎶芥牱鏌ョ湅闈炵┖ URL
func sampleURLs(ctx context.Context, db *sql.DB, table, label, urlCol, nameCol string) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT %s, LEFT(COALESCE(%s,''), 90), generation_status::text
		FROM %s WHERE deleted_at IS NULL AND COALESCE(%s,'') <> ''
		ORDER BY id DESC LIMIT 8`, nameCol, urlCol, table, urlCol))
	if err != nil {
		fmt.Printf("  鉂?%s 鏌ヨ澶辫触锛?v\n", label, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name, url, status string
		rows.Scan(&name, &url, &status)
		kind := classifyURL(url)
		fmt.Printf("  [%s] %s 鐘舵€?%s\n", kind, name, status)
		fmt.Printf("    %s\n", url)
	}
}

func classifyURL(u string) string {
	if u == "" {
		return "鉂岀┖"
	}
	if contains(u, "volces.com") && contains(u, "X-Tos-Expires") {
		return "鈿狅笍TOS绛惧悕"
	}
	if contains(u, "asset-placeholder") {
		return "馃幁mock"
	}
	if contains(u, "127.0.0.1") || contains(u, "localhost") || contains(u, "minio") {
		return "馃彔鏈湴MinIO"
	}
	return "鉁呭浐瀹歎RL"
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

