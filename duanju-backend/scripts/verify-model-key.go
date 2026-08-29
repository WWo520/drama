//go:build ignore

// verify-model-key.go - 楠岃瘉 MODEL_KEY_ENC_SECRET 鏄惁鑳借В瀵嗘暟鎹簱涓殑 Key
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/duanju/backend/internal/pkg/cryptox"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("馃攽 MODEL_KEY_ENC_SECRET 鍖归厤鎬ч獙璇?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	// 鍊欓€夊瘑閽ワ細MODEL_KEY_ENC_SECRET 鍜?JWT_SECRET
	candidates := map[string]string{
		"MODEL_KEY_ENC_SECRET": os.Getenv("MODEL_KEY_ENC_SECRET"),
		"JWT_SECRET (dev)":     "dev-only-secret-change-in-prod",
	}
	if os.Getenv("MODEL_KEY_ENC_SECRET") == "" {
		fmt.Println("鈿狅笍  褰撳墠鐜娌℃湁 MODEL_KEY_ENC_SECRET锛堣鏄?worker 浼氬洖閫€ JWT_SECRET锛?)
	}

	// 璇诲彇 .env 涓殑 MODEL_KEY_ENC_SECRET
	envFile := "../.env"
	if data, err := os.ReadFile(envFile); err == nil {
		fmt.Println("馃搫 璇诲彇 ../.env 鎴愬姛")
		for _, line := range splitLines(string(data)) {
			if len(line) > 20 && line[:20] == "MODEL_KEY_ENC_SECRET" {
				val := line[21:]
				candidates["MODEL_KEY_ENC_SECRET (.env)"] = val
				fmt.Printf("馃搫 .env 涓殑 MODEL_KEY_ENC_SECRET = %s...\n", truncate(val, 20))
			}
		}
	} else {
		fmt.Printf("鈿狅笍  璇诲彇 ../.env 澶辫触锛?v\n", err)
	}

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 璇诲彇鍚敤鐨?deepseek Key 瀵嗘枃
	rows, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.key_encrypted, mk.enabled
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek'
		ORDER BY mk.id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var prefix, encrypted string
		var enabled bool
		if err := rows.Scan(&id, &prefix, &encrypted, &enabled); err != nil {
			continue
		}
		fmt.Printf("\n馃攽 Key[%d] 鍓嶇紑=%s 鍚敤=%v\n", id, prefix, enabled)
		for name, secret := range candidates {
			if secret == "" {
				fmt.Printf("   %-25s: (鏈缃?\n", name)
				continue
			}
			key := cryptox.DeriveKey(secret)
			plain, err := cryptox.Decrypt(encrypted, key)
			if err != nil {
				fmt.Printf("   %-25s: 鉂?瑙ｅ瘑澶辫触 (%v)\n", name, err)
			} else {
				fmt.Printf("   %-25s: 鉁?瑙ｅ瘑鎴愬姛 鈫?%s...%s\n", name, truncate(plain, 8), plain[len(plain)-4:])
			}
		}
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("馃挕 缁撹锛?)
	fmt.Println("   - 鑻?MODEL_KEY_ENC_SECRET 瑙ｅ瘑鎴愬姛 鈫?鐢?start-worker.ps1 閲嶅惎鍗冲彲")
	fmt.Println("   - 鑻ュ叏閮ㄥけ璐?鈫?闇€瑕佺敤姝ｇ‘鐨勫瘑閽ラ噸鏂板姞瀵?Key锛堟垨鍦ㄧ鐞嗙閲嶆柊娣诲姞锛?)
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

