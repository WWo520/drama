//go:build ignore

// explain-key-diff.go - 鍒嗘瀽 4 涓?Key 鐨勫垱寤烘椂闂寸嚎涓庡姞瀵嗗瘑閽ュ樊寮?
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/duanju/backend/internal/pkg/cryptox"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("馃攳 鍒嗘瀽 Key 宸紓锛堝垱寤烘椂闂寸嚎 + 鍔犲瘑瀵嗛挜锛?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 璇诲彇 .env 涓殑 MODEL_KEY_ENC_SECRET
	envSecret := ""
	if data, err := os.ReadFile("../.env"); err == nil {
		for _, line := range splitLines(string(data)) {
			if len(line) > 20 && line[:20] == "MODEL_KEY_ENC_SECRET" {
				envSecret = line[21:]
			}
		}
	}

	// 鍊欓€夊瘑閽ユ睜
	candidates := []struct {
		label  string
		secret string
	}{
		{"JWT_SECRET (dev)", "dev-only-secret-change-in-prod"},
		{".env MODEL_KEY_ENC_SECRET", envSecret},
	}

	// 鏌ヨ鎵€鏈?deepseek key 鐨勫畬鏁翠俊鎭?
	rows, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mk.created_at::text,
		       mk.updated_at::text, mk.key_encrypted,
		       mp.name
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

	for rows.Next() {
		var id int64
		var prefix, enabledStr, createdStr, updatedStr, encrypted, providerName string
		var enabled bool
		if err := rows.Scan(&id, &prefix, &enabled, &createdStr, &updatedStr, &encrypted, &providerName); err != nil {
			continue
		}
		_ = enabledStr

		// 灏濊瘯鎵€鏈夊€欓€夊瘑閽ヨВ瀵?
		fmt.Printf("\n馃攽 Key[%d] 鍓嶇紑=%s 鍚敤=%v\n", id, prefix, enabled)
		fmt.Printf("   鍒涘缓鏃堕棿: %s\n", createdStr)
		fmt.Printf("   鏇存柊鏃堕棿: %s\n", updatedStr)

		decryptable := false
		for _, c := range candidates {
			if c.secret == "" {
				continue
			}
			key := cryptox.DeriveKey(c.secret)
			plain, err := cryptox.Decrypt(encrypted, key)
			if err != nil {
				fmt.Printf("   瑙ｅ瘑 %-28s: 鉂?涓嶅尮閰峔n", c.label)
			} else {
				fmt.Printf("   瑙ｅ瘑 %-28s: 鉁?鎴愬姛 (鍚屼竴鎶?key: %s...%s)\n", c.label, truncate(plain, 6), plain[len(plain)-4:])
				decryptable = true
			}
		}
		if !decryptable {
			fmt.Printf("   瑙ｅ瘑 %-28s: 鉂?涓ょ鍊欓€夊瘑閽ュ潎涓嶅尮閰?鈫?鐢ㄧ涓変釜瀵嗛挜鍔犲瘑鐨刓n", "(鍏ㄩ儴鍊欓€?")
		}
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("馃挕 瑙ｉ噴锛?)
	fmt.Println("   - Key 鐨勫墠缂€鐩稿悓 鈮?瀵嗘枃鍙В瀵?)
	fmt.Println("   - 鏁版嵁搴撳瓨鐨勬槸銆庡姞瀵嗗悗鐨勫瘑鏂囥€忥紝涓嶆槸鏄庢枃 API Key")
	fmt.Println("   - 鑳藉惁瑙ｅ瘑鍙栧喅浜庛€庡垱寤烘椂鐢ㄧ殑鍔犲瘑瀵嗛挜銆忎笌銆庡綋鍓嶈繘绋嬭В瀵嗗瘑閽ャ€忔槸鍚︿竴鑷?)
	fmt.Println("   - Key[2]/[3] 鏄?JWT_SECRET 鐜鍒涘缓鐨勶紱Key[4] 鏄彟涓€涓瘑閽ョ幆澧冨垱寤虹殑")
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var _ = time.Now

