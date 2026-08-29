//go:build ignore

// check-asset-worker.go - 妫€鏌ヨ祫浜?Worker 涓?T2I Key 閰嶇疆銆佸緟澶勭悊浠诲姟
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

	fmt.Println("馃攳 璧勪骇 Worker 閾捐矾妫€鏌?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 鐏北 T2I 鍘傚晢/妯″瀷/Key 閰嶇疆
	fmt.Println("\n銆?銆戠伀灞?T2I 閰嶇疆")
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled, mp.name, mp.enabled, COALESCE(mp.base_url,'')
		FROM models m JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.capability = 'T2I' AND m.deleted_at IS NULL
	`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows.Next() {
			var mid int64
			var modelKey, pname, baseURL string
			var mEn, pEn bool
			rows.Scan(&mid, &modelKey, &mEn, &pname, &pEn, &baseURL)
			status := "鉁?
			if !mEn || !pEn {
				status = "鉂?
			}
			fmt.Printf("  %s 妯″瀷[%d] %s/%s 妯″瀷鍚敤=%v 鍘傚晢鍚敤=%v\n", status, mid, pname, modelKey, mEn, pEn)
			if baseURL != "" {
				fmt.Printf("     base_url=%s\n", baseURL)
			}
		}
		rows.Close()
	}

	// 2. 鐏北鍘傚晢鐨?Key锛堝惈瑙ｅ瘑娴嬭瘯锛?
	fmt.Println("\n銆?銆戠伀灞卞巶鍟?Key 瑙ｅ瘑娴嬭瘯")
	// 璇诲彇 .env 涓殑 MODEL_KEY_ENC_SECRET
	envSecret := ""
	if data, err := os.ReadFile("../.env"); err == nil {
		for _, line := range splitLines(string(data)) {
			if len(line) > 20 && line[:20] == "MODEL_KEY_ENC_SECRET" {
				envSecret = line[21:]
			}
		}
	}
	if envSecret != "" {
		fmt.Printf("  鈩癸笍  .env MODEL_KEY_ENC_SECRET 鍓嶇紑: %s...\n", truncate(envSecret, 12))
	}

	// 鍊欓€夊瘑閽?
	keyCandidates := []struct {
		label  string
		secret string
	}{
		{"JWT_SECRET", "dev-only-secret-change-in-prod"},
		{".env MODEL_KEY_ENC_SECRET", envSecret},
	}

	rows2, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_prefix, mk.enabled, mk.key_encrypted, mp.name, mk.concurrent_limit, mk.priority
		FROM model_keys mk JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name IN ('volcengine','autodl-comfy') AND mk.deleted_at IS NULL
	`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		for rows2.Next() {
			var id int64
			var prefix, encrypted, pname string
			var enabled bool
			var concurrency, priority int
			rows2.Scan(&id, &prefix, &enabled, &encrypted, &pname, &concurrency, &priority)
			status := "鉂屽仠鐢?
			if enabled {
				status = "鉁呭惎鐢?
			}
			fmt.Printf("  Key[%d] %s | %s | 鍓嶇紑=%s | 骞跺彂=%d 浼樺厛绾?%d\n",
				id, pname, status, prefix, concurrency, priority)
			for _, c := range keyCandidates {
				if c.secret == "" {
					continue
				}
				if plain, derr := cryptox.Decrypt(encrypted, cryptox.DeriveKey(c.secret)); derr == nil {
					fmt.Printf("    %s: 鉁?鍙В瀵?(%s...)\n", c.label, truncate(plain, 8))
				} else {
					fmt.Printf("    %s: 鉂?瑙ｅ瘑澶辫触\n", c.label)
				}
			}
		}
		rows2.Close()
	}

	// 3. 寰呭鐞嗙殑璧勪骇浠诲姟锛圦UEUED/RUNNING/PENDING锛?
	fmt.Println("\n銆?銆戝緟澶勭悊璧勪骇浠诲姟")
	rows3, err := db.QueryContext(ctx, `
		SELECT g.id, g.task_type, g.status, g.retry_count, g.created_at::text,
		       COALESCE(LEFT(g.request_params_json::text, 80), '')
		FROM generation_tasks g
		WHERE g.task_type IN ('CHAR_IMAGE_GEN','SCENE_IMAGE_GEN','PROP_IMAGE_GEN')
		  AND g.status IN ('QUEUED','RUNNING','PENDING')
		ORDER BY g.created_at DESC LIMIT 10
	`)
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
	} else {
		cnt := 0
		for rows3.Next() {
			cnt++
			var id, attempt int64
			var taskType, status, createdAt, params string
			rows3.Scan(&id, &taskType, &status, &attempt, &createdAt, &params)
			fmt.Printf("  浠诲姟[%d] %s 鐘舵€?%s 灏濊瘯=%d 鍒涘缓=%s\n", id, taskType, status, attempt, createdAt)
			fmt.Printf("    鍙傛暟: %s\n", params)
		}
		if cnt == 0 {
			fmt.Println("  鉁?娌℃湁鍗′綇鐨勮祫浜т换鍔?)
		} else {
			fmt.Printf("  鈿狅笍  %d 涓祫浜т换鍔℃棤浜哄鐞嗭紙娌℃湁 asset worker锛塡n", cnt)
		}
		rows3.Close()
	}

	// 4. outbox 涓緟鎶曢€掔殑璧勪骇浠诲姟
	fmt.Println("\n銆?銆憈ask_dispatch_outbox 寰呮姇閫?)
	var outboxCnt int
	db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_dispatch_outbox o
		JOIN generation_tasks g ON g.id = o.task_id
		WHERE g.task_type IN ('CHAR_IMAGE_GEN','SCENE_IMAGE_GEN','PROP_IMAGE_GEN')
		  AND o.processed_at IS NULL
	`).Scan(&outboxCnt)
	fmt.Printf("  寰呮姇閫掕祫浜т换鍔? %d 涓猏n", outboxCnt)

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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

