//go:build ignore

// check-llm-config.go - 妫€鏌?LLM 瀹屾暣閰嶇疆
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

	fmt.Println("馃攳 LLM 瀹屾暣閰嶇疆妫€鏌?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 鏌ョ湅鎵€鏈夋ā鍨嬪強鍘傚晢
	fmt.Println("\n銆?銆戞ā鍨嬩笌鍘傚晢閰嶇疆")
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.capability, m.enabled, m.deleted_at IS NOT NULL as is_deleted,
		       mp.name as provider_name, mp.enabled as provider_enabled, mp.deleted_at IS NOT NULL as provider_deleted,
		       COALESCE(mp.base_url, '') as base_url
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		ORDER BY mp.name, m.model_key
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var modelKey, capability, providerName, baseURL string
		var enabled, isDeleted, providerEnabled, providerDeleted bool
		rows.Scan(&id, &modelKey, &capability, &enabled, &isDeleted, &providerName, &providerEnabled, &providerDeleted, &baseURL)
		status := "鉁?
		if !enabled || isDeleted || !providerEnabled || providerDeleted {
			status = "鉂?
		}
		urlDisplay := baseURL
		if len(urlDisplay) > 60 {
			urlDisplay = urlDisplay[:60] + "..."
		}
		fmt.Printf("%s [%d] %s / %s\n", status, id, providerName, modelKey)
		fmt.Printf("   鑳藉姏=%s | 妯″瀷鍚敤=%v | 鍘傚晢鍚敤=%v | base_url=%s\n", capability, enabled, providerEnabled, urlDisplay)
	}

	// 2. 鏌ョ湅鍚敤鐨?deepseek 鍘傚晢鐨?Key 閰嶇疆
	fmt.Println("\n銆?銆慸eepseek 鍘傚晢鐨?Key 閰嶇疆")
	rows2, err := db.QueryContext(ctx, `
		SELECT mk.id, mk.key_encrypted IS NOT NULL as has_key, mk.enabled,
		       mk.concurrent_limit, mk.priority, mk.deleted_at IS NOT NULL as is_deleted,
		       mp.name as provider_name, COALESCE(mk.key_prefix, '') as key_prefix
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek'
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows2.Close()

	keyCount := 0
	for rows2.Next() {
		var id int64
		var hasKey, enabled, isDeleted bool
		var concurrency, priority int
		var providerName, keyPrefix string
		rows2.Scan(&id, &hasKey, &enabled, &concurrency, &priority, &isDeleted, &providerName, &keyPrefix)
		status := "鉁?
		if !hasKey || !enabled || isDeleted {
			status = "鉂?
		}
		fmt.Printf("%s Key[%d] %s | 鏈夊瘑閽?%v | 鍚敤=%v | 骞跺彂=%d | 浼樺厛绾?%d | 鍓嶇紑=%s\n", status, id, providerName, hasKey, enabled, concurrency, priority, keyPrefix)
		keyCount++
	}
	if keyCount == 0 {
		fmt.Println("鈿狅笍  娌℃湁鎵惧埌 deepseek 鐨?Key锛?)
	}

	// 3. 妫€鏌?SCRIPT_SPLIT 鐩稿叧浠诲姟鐘舵€?
	fmt.Println("\n銆?銆戞渶杩戠殑 SCRIPT_SPLIT 浠诲姟鐘舵€?)
	rows3, err := db.QueryContext(ctx, `
		SELECT g.id, g.status, g.task_type,
		       COALESCE(LEFT(g.error_message, 120), '') as err_msg,
		       g.created_at::text,
		       p.title as project_name, p.status as project_status
		FROM generation_tasks g
		LEFT JOIN projects p ON p.id = g.project_id
		WHERE g.task_type = 'SCRIPT_SPLIT'
		ORDER BY g.created_at DESC
		LIMIT 5
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows3.Close()

	found := false
	for rows3.Next() {
		found = true
		var id int64
		var status, taskType, errMsg, createdAt, projectName, projectStatus string
		rows3.Scan(&id, &status, &taskType, &errMsg, &createdAt, &projectName, &projectStatus)
		fmt.Printf("[浠诲姟 %d] 鐘舵€?%s | 椤圭洰=%s (%s)\n", id, status, projectName, projectStatus)
		if errMsg != "" {
			fmt.Printf("   閿欒锛?s\n", errMsg)
		}
		fmt.Printf("   鍒涘缓鏃堕棿锛?s\n", createdAt)
	}
	if !found {
		fmt.Println("鈿狅笍  娌℃湁浠讳綍 SCRIPT_SPLIT 浠诲姟璁板綍")
	}

	// 4. 妫€鏌ラ」鐩姸鎬?
	fmt.Println("\n銆?銆戦」鐩綋鍓嶇姸鎬?)
	rows4, err := db.QueryContext(ctx, `
		SELECT id, title, status, current_step, updated_at::text
		FROM projects
		ORDER BY updated_at DESC
		LIMIT 5
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows4.Close()

	for rows4.Next() {
		var id int64
		var name, status, step, updated string
		rows4.Scan(&id, &name, &status, &step, &updated)
		statusIcon := "鉁?
		if status == "SCRIPT_SPLIT_FAILED" || status == "FAILED" {
			statusIcon = "鉂?
		}
		fmt.Printf("%s [%d] %s | 鐘舵€?%s | 姝ラ=%s | 鏇存柊=%s\n", statusIcon, id, name, status, step, updated)
	}

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

