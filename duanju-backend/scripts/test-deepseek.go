//go:build ignore

// test-deepseek.go - 娴嬭瘯 DeepSeek 閰嶇疆
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
	
	fmt.Println("馃攳 娴嬭瘯 DeepSeek 閰嶇疆")
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏃犳硶 Ping 閫氭暟鎹簱锛?v\n", err)
		os.Exit(1)
	}
	fmt.Println("鉁?鏁版嵁搴撹繛鎺ユ垚鍔?)
	fmt.Println()
	
	// 鐩存帴鏌ヨ models 琛ㄧ殑鎵€鏈?LLM 鐩稿叧璁板綍
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled, mp.name as provider_name
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%'
		ORDER BY mp.name, m.model_key
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var id int64
		var modelKey, enabledStr, providerName string
		rows.Scan(&id, &modelKey, &enabledStr, &providerName)
		
		fmt.Printf("[%d] %s / %-30s -> enabled=%s\n", id, providerName, modelKey, enabledStr)
		count++
	}
	
	fmt.Println()
	fmt.Printf("鎬昏鎵惧埌 %d 涓?LLM 鐩稿叧妯″瀷\n", count)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

