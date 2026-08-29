//go:build ignore

// fix-model-keys.go - 淇 deepseek Key 閰嶇疆锛堝惎鐢?JWT_SECRET 鍙В瀵嗙殑 Key锛?
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

	fmt.Println("馃敡 淇 deepseek Key 閰嶇疆")
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 鍋滅敤 Key[4]锛堣В瀵嗗け璐ョ殑 key锛?
	if _, err := db.ExecContext(ctx, `UPDATE model_keys SET enabled=FALSE, updated_at=NOW() WHERE id=4`); err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鍋滅敤 Key[4] 澶辫触锛?v\n", err)
		os.Exit(1)
	}
	fmt.Println("鉁?宸插仠鐢?Key[4]锛堝姞瀵嗗瘑閽ヤ笉鍖归厤锛?)

	// 2. 鍚敤 Key[2]锛圝WT_SECRET 鍙В瀵嗭級
	if _, err := db.ExecContext(ctx, `UPDATE model_keys SET enabled=TRUE, updated_at=NOW() WHERE id=2`); err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鍚敤 Key[2] 澶辫触锛?v\n", err)
		os.Exit(1)
	}
	fmt.Println("鉁?宸插惎鐢?Key[2]锛圝WT_SECRET 鍙В瀵嗭級")

	// 3. 楠岃瘉鏈€缁堢姸鎬?
	rows, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mp.name
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek' AND mk.deleted_at IS NULL
		ORDER BY mk.id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	fmt.Println("\n馃搵 淇鍚?deepseek Key 鐘舵€侊細")
	for rows.Next() {
		var id int64
		var prefix string
		var enabled bool
		var providerName string
		rows.Scan(&id, &prefix, &enabled, &providerName)
		status := "鉂?鍋滅敤"
		if enabled {
			status = "鉁?鍚敤"
		}
		fmt.Printf("  Key[%d] %s 鍓嶇紑=%s\n", id, status, prefix)
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("鉁?淇瀹屾垚锛佽閲嶆柊瑙﹀彂鍓ф湰鎷嗗垎浠诲姟銆?)
}

func main2() { _ = context.Background() } // keep import

