//go:build ignore

// cleanup-model-keys.go - 娓呯悊澶辨晥鐨?deepseek Key锛堜粎淇濈暀 Key[2]锛?
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

	fmt.Println("馃Ч 娓呯悊澶辨晥鐨?deepseek Key")
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 纭褰撳墠鐘舵€侊紙闃叉璇垹锛?
	rows, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mk.deleted_at IS NOT NULL as is_deleted
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek' AND mk.deleted_at IS NULL
		ORDER BY mk.id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	var existing []int64
	for rows.Next() {
		var id int64
		var prefix string
		var enabled, isDeleted bool
		rows.Scan(&id, &prefix, &enabled, &isDeleted)
		fmt.Printf("  Key[%d] 鍓嶇紑=%s 鍚敤=%v 宸插垹闄?%v\n", id, prefix, enabled, isDeleted)
		existing = append(existing, id)
	}
	rows.Close()

	// 2. 瀹夊叏妫€鏌ワ細Key[2] 蹇呴』瀛樺湪涓斿惎鐢?
	var k2Enabled bool
	err = db.QueryRowContext(ctx, `SELECT enabled FROM model_keys WHERE id=2 AND deleted_at IS NULL`).Scan(&k2Enabled)
	if err != nil || !k2Enabled {
		fmt.Println("鉂?涓锛欿ey[2] 涓嶅瓨鍦ㄦ垨鏈惎鐢紝鍏堣繍琛?fix-model-keys.go 淇")
		os.Exit(1)
	}
	fmt.Println("\n鉁?瀹夊叏妫€鏌ラ€氳繃锛欿ey[2] 瀛樺湪涓斿惎鐢紙淇濈暀锛?)

	// 3. 杞垹闄?Key[1]/[3]/[4]锛堝苟寮哄埗鍋滅敤锛岄槻姝换浣曟畫浣欒矾鐢憋級
	targets := []int64{1, 3, 4}
	for _, id := range targets {
		tag, err := db.ExecContext(ctx, `
			UPDATE model_keys SET enabled=FALSE, deleted_at=NOW(), updated_at=NOW()
			WHERE id=$1 AND deleted_at IS NULL`, id)
		if err != nil {
			fmt.Printf("鉂?Key[%d] 娓呯悊澶辫触锛?v\n", id, err)
			continue
		}
		if n, _ := tag.RowsAffected(); n > 0 {
			fmt.Printf("鉁?Key[%d] 宸茶蒋鍒犻櫎骞跺仠鐢╘n", id)
		} else {
			fmt.Printf("鈩癸笍  Key[%d] 宸蹭笉瀛樺湪鎴栧凡鍒犻櫎锛岃烦杩嘰n", id)
		}
	}

	// 4. 楠岃瘉鏈€缁堢姸鎬?
	fmt.Println("\n馃搵 娓呯悊鍚?deepseek Key 鏈€缁堢姸鎬侊細")
	rows2, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mk.deleted_at IS NOT NULL as is_deleted
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek'
		ORDER BY mk.id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows2.Close()

	for rows2.Next() {
		var id int64
		var prefix string
		var enabled, isDeleted bool
		rows2.Scan(&id, &prefix, &enabled, &isDeleted)
		status := "馃棏锔?宸插垹闄?
		if !isDeleted {
			status = "鉁?瀛樻椿"
		}
		fmt.Printf("  Key[%d] %s 鍓嶇紑=%s 鍚敤=%v\n", id, status, prefix, enabled)
	}

	// 5. 纭鍓╀綑瀛樻椿涓斿惎鐢ㄧ殑 key 鎭板ソ鏄?1 涓?
	var aliveEnabled int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek' AND mk.enabled = TRUE AND mk.deleted_at IS NULL
	`).Scan(&aliveEnabled); err == nil {
		fmt.Printf("\n馃搳 瀛樻椿涓斿惎鐢ㄧ殑 Key 鏁伴噺锛?d锛堝簲涓?1锛塡n", aliveEnabled)
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("鉁?娓呯悊瀹屾垚锛?)
	fmt.Println("   - 淇濈暀锛欿ey[2]锛圝WT_SECRET 鍙В瀵嗭紝鍞竴鍙敤锛?)
	fmt.Println("   - 宸插垹锛欿ey[1]锛堟祴璇曞亣 key锛夈€並ey[3]銆並ey[4]锛堝瘑鏂囧瘑閽ヤ笉鍖归厤/鍐椾綑锛?)
	fmt.Println("   - worker 鏃犻渶閲嶅惎锛岃矾鐢卞嵆鏃剁敓鏁?)
}

