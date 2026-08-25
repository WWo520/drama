// disable-deepseek-chat.go - 禁用已弃用的 DeepSeek Chat 模型
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
	
	fmt.Println("🔧 禁用已弃用的 DeepSeek Chat 模型")
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
	
	// Step 1: 显示当前的 DeepSeek 配置
	fmt.Println("📊 当前的 DeepSeek 模型配置:")
	rows, _ := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.model_key LIKE '%deepseek%' AND m.deleted_at IS NULL
		ORDER BY m.model_key
	`)
	defer rows.Close()
	
	var foundDeepSeekChat bool
	for rows.Next() {
		var id int64
		var modelKey, enabledStr string
		rows.Scan(&id, &modelKey, &enabledStr)
		
		fmt.Printf("   • %s [%d]\n", modelKey, id)
		if modelKey == "deepseek-chat" {
			foundDeepSeekChat = true
		}
	}
	fmt.Println()
	
	if !foundDeepSeekChat {
		fmt.Println("✅ 未找到 deepseek-chat 模型，无需禁用")
		fmt.Println("═══════════════════════════════════════════")
		return
	}
	
	// Step 2: 禁用 deepseek-chat
	result, err := db.ExecContext(ctx, `
		UPDATE models 
		SET enabled = FALSE, deleted_at = NOW()
		WHERE model_key = 'deepseek-chat'
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新失败：%v\n", err)
		os.Exit(1)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("✅ 已禁用 deepseek-chat 模型 (%d行)\n", rowsAffected)
	} else {
		fmt.Println("⚠️  未找到需要禁用的 deepseek-chat 模型")
	}
	fmt.Println()
	
	// Step 3: 显示最终状态
	fmt.Println("📊 最终的 DeepSeek 模型配置:")
	rows, _ = db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.model_key LIKE '%deepseek%' AND m.deleted_at IS NULL
		ORDER BY m.model_key
	`)
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var id int64
		var modelKey string
		var enabledStr string
		rows.Scan(&id, &modelKey, &enabledStr)
		
		status := "❌禁用"
		if enabledStr == "true" {
			status = "✅启用"
		}
		
		fmt.Printf("   • %s [%d] -> %s\n", modelKey, id, status)
		count++
	}
	fmt.Println()
	
	// Step 4: 总结
	fmt.Println("═══════════════════════════════════════════")
	if count == 0 {
		fmt.Println("⚠️  警告：所有 DeepSeek 模型都已被禁用!")
	} else if count == 1 && !foundDeepSeekChat {
		fmt.Println("✅ 只保留了一个 DeepSeek 模型（推荐）")
	}
	fmt.Println()
	fmt.Println("💡 建议:")
	fmt.Println("  使用 'deepseek-v4-flash' 作为主要的拆分模型")
	fmt.Println("  它是 DeepSeek 的最新版本，性能更优")
	fmt.Println("═══════════════════════════════════════════")
}
