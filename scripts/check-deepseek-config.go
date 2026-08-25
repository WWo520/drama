// check-deepseek-config.go - 检查 DeepSeek 配置
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
	
	fmt.Println("🔍 检查 DeepSeek 模型配置")
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
	
	// 查询启用的 DeepSeek V4 Flash 配置
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, mp.name as provider_name,
		       mk.api_key_decrypted, mk.base_url, mp.webhook_secret
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		LEFT JOIN model_keys mk ON mk.id = m.current_key_id
		WHERE m.model_key LIKE '%v4-flash%' AND m.enabled = true
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	for rows.Next() {
		var id int64
		var modelKey, providerName string
		var apiKey, baseURL, webhookSecret sql.NullString
		
		err := rows.Scan(&id, &modelKey, &providerName, &apiKey, &baseURL, &webhookSecret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Scan 失败：%v\n", err)
			continue
		}
		
		fmt.Printf("🔑 DeepSeek 配置:\n")
		fmt.Printf("   ID: %d\n", id)
		fmt.Printf("   Model: %s\n", modelKey)
		fmt.Printf("   Provider: %s\n", providerName)
		fmt.Printf("   BaseURL: ")
		if baseURL.Valid {
			fmt.Printf("%s\n", baseURL.String)
		} else {
			fmt.Printf("(未设置)\n")
		}
		
		fmt.Printf("   API Key: ")
		if apiKey.Valid {
			keyStr := apiKey.String
			if len(keyStr) > 20 {
				// 显示前 10 个字符和后缀
				fmt.Printf("%s...%s [%d 字符]\n", keyStr[:10], keyStr[len(keyStr)-4:], len(keyStr))
			} else {
				fmt.Printf("%s [完整]\n", keyStr)
			}
		} else {
			fmt.Printf("(未设置)\n")
		}
		
		fmt.Printf("   Webhook Secret: ")
		if webhookSecret.Valid && webhookSecret.String != "" {
			fmt.Printf("%s...[已加密]\n", webhookSecret.String[:8])
		} else {
			fmt.Printf("(未设置)\n")
		}
		
		fmt.Println()
		
		// 验证配置完整性
		var missing []string
		if !apiKey.Valid || apiKey.String == "" {
			missing = append(missing, "API Key")
		}
		if !baseURL.Valid || baseURL.String == "" {
			missing = append(missing, "BaseURL")
		}
		
		if len(missing) > 0 {
			fmt.Printf("⚠️  WARNING: 缺少必要的配置项：%v\n", missing)
			fmt.Printf("💡 建议：请检查 env 文件或数据库中的配置\n")
		} else {
			fmt.Printf("✅ 配置完整\n")
		}
		fmt.Println("═══════════════════════════════════════════")
	}
}
