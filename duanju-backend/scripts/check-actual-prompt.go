//go:build ignore

// check-actual-prompt.go - 妫€鏌ュ疄闄呭彂閫佺粰 T2I 鐨勮鑹叉彁绀鸿瘝
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

	fmt.Println("馃攳 瀹為檯 T2I 瑙掕壊鎻愮ず璇嶅揩鐓ф鏌?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. generated_images 涓鑹茬浉鍏崇殑 prompt 蹇収
	fmt.Println("\n銆?銆慻enerated_images 瑙掕壊 prompt 蹇収锛堟湁寮圭獥鍙傛暟鐨勫墠 6 鏉★級")
	rows, err := db.QueryContext(ctx, `
		SELECT gi.id, gi.target_kind, gi.target_id, COALESCE(gi.prompt,''),
		       gi.created_at::text
		FROM generated_images gi
		WHERE gi.deleted_at IS NULL AND gi.target_kind='CHARACTER_VARIANT'
		  AND COALESCE(gi.prompt,'') <> ''
		ORDER BY gi.id DESC LIMIT 6`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows.Next() {
			var id int64
			var kind string
			var tid int64
			var prompt, createdAt string
			rows.Scan(&id, &kind, &tid, &prompt, &createdAt)
			fmt.Printf("  [gi%d] kind=%s target=%d 鏃堕棿=%s\n", id, kind, tid, createdAt)
			// 妫€鏌ユ槸鍚﹀惈鑳屾櫙璇?
			bg := ""
			if containsAny(prompt, []string{"鑳屾櫙", "鎴块棿", "琛楅亾", "瀹ゅ唴", "鍦烘櫙", "鐜", "绐楀", "鍦版澘", "澧欓潰", "澶╃┖"}) {
				bg = " 鈿狅笍銆愬惈鑳屾櫙璇嶃€?
			}
			fmt.Printf("      %s%s\n", truncateStr(prompt, 180), bg)
		}
		rows.Close()
	}

	// 2. 妫€鏌?characters.edited_prompt锛堝脊绐椾紭鍖栧悗鍐欏叆鐨勬彁绀鸿瘝锛?
	fmt.Println("\n銆?銆慶haracters.edited_prompt 妫€鏌ワ紙寮圭獥 AI 浼樺寲浜х墿锛?)
	var editedCount int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM characters WHERE edited_prompt IS NOT NULL AND edited_prompt <> '' AND deleted_at IS NULL`).Scan(&editedCount)
	fmt.Printf("  鏈?edited_prompt 鐨勮鑹叉暟锛?d\n", editedCount)

	rows2, err := db.QueryContext(ctx, `
		SELECT id, name, LEFT(COALESCE(edited_prompt,''), 250)
		FROM characters WHERE edited_prompt IS NOT NULL AND edited_prompt <> '' AND deleted_at IS NULL
		ORDER BY id DESC LIMIT 5`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows2.Next() {
			var id int64
			var name, prompt string
			rows2.Scan(&id, &name, &prompt)
			bg := ""
			if containsAny(prompt, []string{"鑳屾櫙", "鎴块棿", "琛楅亾", "瀹ゅ唴", "鍦烘櫙", "鐜", "绐楀", "鍦版澘", "澧欓潰", "澶╃┖"}) {
				bg = " 鈿狅笍銆愬惈鑳屾櫙璇嶃€?
			}
			fmt.Printf("  [%d] %s%s\n", id, name, bg)
			fmt.Printf("      %s\n", prompt)
		}
		rows2.Close()
	}

	// 3. 妫€鏌?character_variants 鐨?extra_appearance_prompt / clothing_desc
	fmt.Println("\n銆?銆慶haracter_variants 鍙樹綋鎻忚堪妫€鏌?)
	rows3, err := db.QueryContext(ctx, `
		SELECT cv.id, cv.variant_name, LEFT(COALESCE(cv.extra_appearance_prompt,''),150),
		       LEFT(COALESCE(cv.clothing_desc,''),150)
		FROM character_variants cv WHERE cv.deleted_at IS NULL
		ORDER BY cv.id DESC LIMIT 5`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows3.Next() {
			var id int64
			var name, extra, clothing string
			rows3.Scan(&id, &name, &extra, &clothing)
			fmt.Printf("  [鍙樹綋%d] %s\n", id, name)
			if extra != "" {
				fmt.Printf("      extra_appearance: %s\n", extra)
			}
			if clothing != "" {
				fmt.Printf("      clothing_desc: %s\n", clothing)
			}
		}
		rows3.Close()
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

