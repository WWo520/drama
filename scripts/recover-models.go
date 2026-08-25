// recover-models.go - 恢复豆包和 Qwen 模型
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
	
	fmt.Println("🔧 恢复 LLM 模型")
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
	
	// 恢复 id 为 4 和 19 的模型（qwen-turbo 和 doubao-seedream-4-5）
	result, err := db.ExecContext(ctx, `
		UPDATE models 
		SET enabled = TRUE, deleted_at = NULL, updated_at = NOW()
		WHERE id IN (4, 19) AND deleted_at IS NOT NULL
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 恢复失败：%v\n", err)
		os.Exit(1)
	}
	
	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("✅ 已恢复 %d个模型\n", rowsAffected)
	fmt.Println()
	
	// 显示所有启用的模型
	fmt.Println("📋 所有启用的 LLM 模型:")
	rows, _ := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, mp.name as provider_name, m.enabled
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.deleted_at IS NULL AND m.enabled = TRUE
		ORDER BY mp.name, m.model_key
	`)
	defer rows.Close()
	
	for rows.Next() {
		var id int64
		var modelKey, providerName string
		var enabled bool
		rows.Scan(&id, &modelKey, &providerName, &enabled)
		
		status := "⚠️"
		if enabled {
			status = "✅"
		}
		fmt.Printf("   • %s [%d] -> %-40s (%s)\n", status, id, modelKey, providerName)
	}
	
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
}
