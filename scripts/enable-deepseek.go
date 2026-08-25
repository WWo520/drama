// enable-deepseek.go - 快速启用 DeepSeek 模型的自动化脚本
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
	
	fmt.Println("🔧 启用 DeepSeek 模型")
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
	
	// Step 1: 显示启用前的状态
	fmt.Println("📊 启用前的配置:")
	rows, _ := db.QueryContext(ctx, `
		SELECT DISTINCT mp.name, m.model_key, m.enabled, mk.key_prefix IS NOT NULL as has_key
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		JOIN models m ON m.provider_id = mp.id
		WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%'
		ORDER BY mp.name, m.model_key
	`)
	defer rows.Close()
	
	for rows.Next() {
		var providerName, modelKey string
		var enabled, hasKey bool
		rows.Scan(&providerName, &modelKey, &enabled, &hasKey)
		statusStr := "❌禁用"
		if enabled {
			if hasKey {
				statusStr = "✅已启用且有密钥"
			} else {
				statusStr = "⚠️已启用但无密钥"
			}
		} else if hasKey {
			statusStr = "⚠️禁用但有密钥"
		}
		fmt.Printf("   • %s / %-20s -> %s\n", providerName, modelKey, statusStr)
	}
	fmt.Println()
	
	// Step 2: 启用所有 DeepSeek/Qwen/GLM 模型
	result, err := db.ExecContext(ctx, `
		UPDATE models 
		SET enabled = TRUE 
		WHERE model_key LIKE '%deepseek%' 
		   OR model_key LIKE '%qwen%' 
		   OR model_key LIKE '%glm%'
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新模型状态失败：%v\n", err)
		os.Exit(1)
	}
	
	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("✅ 已启用 %d 个模型\n", rowsAffected)
	fmt.Println()
	
	// Step 3: 显示启用后的状态
	fmt.Println("📊 启用后的配置:")
	rows, _ = db.QueryContext(ctx, `
		SELECT DISTINCT mp.name, m.model_key, m.enabled, mk.key_prefix IS NOT NULL as has_key
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		JOIN models m ON m.provider_id = mp.id
		WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%'
		ORDER BY mp.name, m.model_key
	`)
	defer rows.Close()
	
	countEnabled := 0
	countWithKey := 0
	
	for rows.Next() {
		var providerName, modelKey string
		var enabled, hasKey bool
		rows.Scan(&providerName, &modelKey, &enabled, &hasKey)
		
		statusStr := ""
		if enabled {
			countEnabled++
			if hasKey {
				statusStr = "✅已启用且有密钥"
				countWithKey++
			} else {
				statusStr = "⚠️已启用但需添加密钥"
			}
		} else {
			statusStr = "❌禁用"
		}
		fmt.Printf("   • %s / %-20s -> %s\n", providerName, modelKey, statusStr)
	}
	fmt.Println()
	
	// Step 4: 最终结果
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("✅ 总计：%d个已启用，%d个有有效密钥\n", countEnabled, countWithKey)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	
	if countEnabled == 0 {
		fmt.Println("⚠️  警告：没有成功启用任何模型！")
		fmt.Println("建议：检查数据库中的模型配置表")
	} else if countWithKey == 0 {
		fmt.Println("⚠️  提示：模型已启用，但暂无有效密钥!")
		fmt.Println("💡 请在管理端为这些模型添加 API 密钥：")
		fmt.Println("    http://localhost:8080/admin/settings/models")
	} else {
		fmt.Println("✅ 一切就绪！系统现在有可用的模型和密钥！")
	}
	fmt.Println()
}
