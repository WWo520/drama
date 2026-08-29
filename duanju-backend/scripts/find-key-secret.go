//go:build ignore

// find-key-secret.go - 绮剧‘鎵惧嚭 Key[7] 鐨勫姞瀵嗗瘑閽?
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

	fmt.Println("馃攽 瀵绘壘 Key[7] 鐨勫姞瀵嗗瘑閽?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 鏀堕泦鍊欓€夊瘑閽?
	candidates := map[string]string{}

	// 1. .env 鏂囦欢涓殑 MODEL_KEY_ENC_SECRET锛堝畬鏁村€硷級
	if data, err := os.ReadFile("../.env"); err == nil {
		for _, line := range splitLines(string(data)) {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "MODEL_KEY_ENC_SECRET=") {
				v := strings.TrimSpace(strings.TrimPrefix(line, "MODEL_KEY_ENC_SECRET="))
				candidates["env.MODEL_KEY_ENC_SECRET"] = v
				fmt.Printf("馃搫 .env MODEL_KEY_ENC_SECRET 闀垮害=%d\n", len(v))
			}
		}
	}

	// 2. dev-run.ps1 涓殑纭紪鐮佸€?
	if data, err := os.ReadFile("bin/dev-run.ps1"); err == nil {
		for _, line := range splitLines(string(data)) {
			if strings.Contains(line, "MODEL_KEY_ENC_SECRET=") {
				idx := strings.Index(line, "MODEL_KEY_ENC_SECRET=")
				v := strings.Trim(line[idx+len("MODEL_KEY_ENC_SECRET="):], " '")
				candidates["dev-run.ps1.MODEL_KEY_ENC_SECRET"] = v
				fmt.Printf("馃搫 dev-run.ps1 MODEL_KEY_ENC_SECRET 闀垮害=%d\n", len(v))
			}
		}
	}

	// 3. JWT_SECRET
	candidates["JWT_SECRET"] = "dev-only-secret-change-in-prod"

	// 4. 褰撳墠鐜鍙橀噺
	if v := os.Getenv("MODEL_KEY_ENC_SECRET"); v != "" {
		candidates["os.MODEL_KEY_ENC_SECRET"] = v
	}

	// 璇诲彇 Key[7] 瀵嗘枃
	var encrypted string
	err = db.QueryRowContext(ctx, `SELECT key_encrypted FROM model_keys WHERE id=7`).Scan(&encrypted)
	if err != nil {
		fmt.Printf("鉂?璇诲彇 Key[7] 澶辫触锛?v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n馃攽 Key[7] 瀵嗘枃闀垮害=%d锛屽皾璇?%d 涓€欓€夊瘑閽?\n", len(encrypted), len(candidates))
	found := false
	for name, secret := range candidates {
		if secret == "" {
			fmt.Printf("  %-32s: (绌?\n", name)
			continue
		}
		plain, derr := cryptox.Decrypt(encrypted, cryptox.DeriveKey(secret))
		if derr == nil {
			fmt.Printf("  %-32s: 鉁?瑙ｅ瘑鎴愬姛 鈫?%s...\n", name, truncate(plain, 10))
			found = true
		} else {
			fmt.Printf("  %-32s: 鉂?澶辫触\n", name)
		}
	}

	if !found {
		fmt.Println("\n鈿狅笍  鎵€鏈夊€欓€夊瘑閽ュ潎鏃犳硶瑙ｅ瘑 Key[7]锛?)
		fmt.Println("  鍙兘鍘熷洜锛歛pi.exe 鍚姩鏃舵敞鍏ヤ簡鍏朵粬 MODEL_KEY_ENC_SECRET 鍊?)
		fmt.Println("  鈫?闇€瑕佹煡鐪?api 杩涚▼鍚姩鏂瑰紡锛坆in\\api.out.log 鎴栦换鍔¤鍒掞級")
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
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
	return s[:n] + "..."
}

