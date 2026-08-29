//go:build ignore

// check-char-prompt.go - 妫€鏌ヨ鑹叉彁绀鸿瘝涓槸鍚﹀惈鑳屾櫙鎻忚堪
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

	fmt.Println("馃攳 瑙掕壊鎻愮ず璇嶈儗鏅弿杩版鏌?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 鎶芥牱 8 涓鑹叉煡鐪?description_notes
	fmt.Println("\n銆?銆戣鑹?description_notes 鎶芥牱锛堝墠 8 涓級")
	rows, err := db.QueryContext(ctx, `
		SELECT c.id, c.name, LEFT(COALESCE(c.description_notes,''), 200),
		       CASE WHEN c.description_notes LIKE '%鑳屾櫙%' OR c.description_notes LIKE '%鍦烘櫙%' OR c.description_notes LIKE '%鐜%' OR c.description_notes LIKE '%鎴块棿%' OR c.description_notes LIKE '%琛楅亾%' THEN '鈿狅笍 鍚儗鏅瘝' ELSE '鉁?鏃犺儗鏅瘝' END
		FROM characters c
		WHERE c.deleted_at IS NULL
		ORDER BY c.id DESC LIMIT 8`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows.Next() {
			var id int64
			var name, notes, flag string
			rows.Scan(&id, &name, &notes, &flag)
			fmt.Printf("  [%d] %s %s\n", id, name, flag)
			fmt.Printf("      %s\n", notes)
		}
		rows.Close()
	}

	// 2. 缁熻鏈夊灏戣鑹叉弿杩板惈鑳屾櫙绫昏瘝姹?
	fmt.Println("\n銆?銆戝惈鑳屾櫙绫昏瘝姹囩殑瑙掕壊缁熻")
	var total, hasBg int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM characters WHERE deleted_at IS NULL`).Scan(&total)
	db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM characters WHERE deleted_at IS NULL AND (
			description_notes LIKE '%鑳屾櫙%' OR description_notes LIKE '%鍦烘櫙%'
			OR description_notes LIKE '%鐜%' OR description_notes LIKE '%鎴块棿%'
			OR description_notes LIKE '%琛楅亾%' OR description_notes LIKE '%瀹ゅ唴%'
			OR description_notes LIKE '%鎴峰%' OR description_notes LIKE '%澶╃┖%')`).Scan(&hasBg)
	fmt.Printf("  瑙掕壊鎬绘暟锛?d | 鎻忚堪鍚儗鏅被璇嶏細%d (%.0f%%)\n", total, hasBg, float64(hasBg)/float64(total)*100)

	// 3. 鏌ョ湅鍦烘櫙涓庨亾鍏风殑 prompt 鏄惁娣峰叆瑙掕壊鎻忚堪
	fmt.Println("\n銆?銆戣鑹插彉浣?generation_params 涓殑 prompt 蹇収锛堝墠 5锛?)
	rows3, err := db.QueryContext(ctx, `
		SELECT cv.id, cv.variant_name, LEFT(COALESCE(cv.generation_params::text,''), 300)
		FROM character_variants cv WHERE cv.deleted_at IS NULL
		ORDER BY cv.id DESC LIMIT 5`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows3.Next() {
			var id int64
			var name, params string
			rows3.Scan(&id, &name, &params)
			fmt.Printf("  [鍙樹綋%d] %s\n", id, name)
			fmt.Printf("      %s\n", params)
		}
		rows3.Close()
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

