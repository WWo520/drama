//go:build ignore

// unify-key-encryption.go - 缁熶竴鎵€鏈?Key 鐨勫姞瀵嗗瘑閽ヤ负 MODEL_KEY_ENC_SECRET
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/duanju/backend/internal/pkg/cryptox"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("馃敡 缁熶竴 Key 鍔犲瘑瀵嗛挜 鈫?MODEL_KEY_ENC_SECRET")
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	// 璇诲彇 .env 涓殑 MODEL_KEY_ENC_SECRET锛堢洰鏍囧姞瀵嗗瘑閽ワ級
	newSecret := ""
	if data, err := os.ReadFile("../.env"); err == nil {
		for _, line := range splitLines(string(data)) {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "MODEL_KEY_ENC_SECRET=") {
				newSecret = strings.TrimSpace(strings.TrimPrefix(line, "MODEL_KEY_ENC_SECRET="))
			}
		}
	}
	if newSecret == "" {
		fmt.Println("鉂?.env 涓湭鎵惧埌 MODEL_KEY_ENC_SECRET")
		os.Exit(1)
	}
	newKey := cryptox.DeriveKey(newSecret)
	fmt.Printf("鉁?鐩爣瀵嗛挜 MODEL_KEY_ENC_SECRET 闀垮害=%d\n", len(newSecret))

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 鏌ユ壘鎵€鏈?褰撳墠鐢?JWT_SECRET 鍔犲瘑浣嗗惎鐢ㄤ腑"鐨?Key锛岀粺涓€閲嶅姞瀵?
	rows, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mk.key_encrypted, mp.name
		FROM model_keys mk JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mk.deleted_at IS NULL AND mk.enabled = TRUE
		ORDER BY mk.id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}

	type keyInfo struct {
		id        int64
		prefix    string
		encrypted string
		provider  string
	}
	var keys []keyInfo
	for rows.Next() {
		var k keyInfo
		var enabled bool
		rows.Scan(&k.id, &k.prefix, &enabled, &k.encrypted, &k.provider)
		keys = append(keys, k)
	}
	rows.Close()

	for _, k := range keys {
		// 灏濊瘯鐢?JWT_SECRET 瑙ｅ瘑锛堟棫瀵嗛挜锛?
		plain, err := cryptox.Decrypt(k.encrypted, cryptox.DeriveKey("dev-only-secret-change-in-prod"))
		if err != nil {
			// 涓嶆槸 JWT_SECRET 鍔犲瘑鐨勶細鍐嶆祴璇曟槸鍚﹀凡鏄柊瀵嗛挜
			if _, err2 := cryptox.Decrypt(k.encrypted, newKey); err2 == nil {
				fmt.Printf("  Key[%d] %s/%s 宸叉槸鏂板瘑閽ュ姞瀵?鉁?璺宠繃\n", k.id, k.provider, k.prefix)
			} else {
				fmt.Printf("  Key[%d] %s/%s 鏈煡瀵嗛挜鍔犲瘑 鈿狅笍 璺宠繃锛堥渶鎵嬪姩澶勭悊锛塡n", k.id, k.provider, k.prefix)
			}
			continue
		}
		// JWT_SECRET 鍙В瀵?鈫?鐢ㄦ柊瀵嗛挜閲嶆柊鍔犲瘑骞舵洿鏂?
		newEnc, err := cryptox.Encrypt(plain, newKey)
		if err != nil {
			fmt.Printf("  Key[%d] %s/%s 閲嶅姞瀵嗗け璐ワ細%v\n", k.id, k.provider, k.prefix, err)
			continue
		}
		if _, err := db.ExecContext(ctx, `UPDATE model_keys SET key_encrypted=$2, updated_at=NOW() WHERE id=$1`, k.id, newEnc); err != nil {
			fmt.Printf("  Key[%d] %s/%s 鏇存柊澶辫触锛?v\n", k.id, k.provider, k.prefix, err)
			continue
		}
		fmt.Printf("  Key[%d] %s/%s 宸查噸鍔犲瘑涓?MODEL_KEY_ENC_SECRET 鉁匼n", k.id, k.provider, k.prefix)
	}

	// 楠岃瘉锛氭墍鏈夊惎鐢?Key 鍧囧彲鐢ㄦ柊瀵嗛挜瑙ｅ瘑
	fmt.Println("\n馃搵 鏈€缁堥獙璇侊紙MODEL_KEY_ENC_SECRET 瑙ｅ瘑锛夛細")
	rows2, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mk.key_encrypted, mp.name
		FROM model_keys mk JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mk.deleted_at IS NULL AND mk.enabled = TRUE
		ORDER BY mk.id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows2.Close()
	for rows2.Next() {
		var id int64
		var prefix, encrypted, provider string
		var enabled bool
		rows2.Scan(&id, &prefix, &enabled, &encrypted, &provider)
		if _, derr := cryptox.Decrypt(encrypted, newKey); derr == nil {
			fmt.Printf("  鉁?Key[%d] %s/%s 鍙В瀵哱n", id, provider, prefix)
		} else {
			fmt.Printf("  鉂?Key[%d] %s/%s 浠嶆棤娉曡В瀵哱n", id, provider, prefix)
		}
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("鉁?瀹屾垚锛佺幇鍦ㄥ彲鐢?start-worker.ps1 鍚姩 worker锛堟敞鍏?MODEL_KEY_ENC_SECRET锛?)
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

