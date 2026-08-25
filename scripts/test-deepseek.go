// test-deepseek.go - 测试 DeepSeek 配置
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
	
	fmt.Println("🔍 测试 DeepSeek 配置")
	fmt.Println("═══════════════════════════════════════════")
	
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 数据库连接失败：%v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法 Ping 通数据库：%v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 数据库连接成功")
	fmt.Println()
	
	// 直接查询 models 表的所有 LLM 相关记录
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled, mp.name as provider_name
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%'
		ORDER BY mp.name, m.model_key
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
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
	fmt.Printf("总计找到 %d 个 LLM 相关模型\n", count)
	fmt.Println("═══════════════════════════════════════════")
}
