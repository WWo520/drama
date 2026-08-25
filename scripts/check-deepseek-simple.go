// check-deepseek.go - 检查 DeepSeek 配置（简化版）
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	fmt.Println("🔍 检查 DeepSeek 模型")
	fmt.Println("═══════════════════════════════════════════")
	
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 数据库连接失败：%v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法 Ping 通数据库：%v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 数据库连接成功")
	fmt.Println()
	
	rows, err := db.Query(`
		SELECT m.id, m.model_key, mp.name as provider_name, m.enabled
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%doubao%'
		ORDER BY mp.name, m.model_key
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	for rows.Next() {
		var id int64
		var modelKey, providerName, enabledStr string
		
		err := rows.Scan(&id, &modelKey, &providerName, &enabledStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Scan 失败：%v\n", err)
			continue
		}
		
		status := "⚠️禁用"
		if enabledStr == "true" {
			status = "✅启用"
		}
		
		fmt.Printf("[%d] %s / %-30s -> %s\n", id, providerName, modelKey, status)
	}
	
	fmt.Println("═══════════════════════════════════════════")
}
