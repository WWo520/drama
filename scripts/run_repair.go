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
	
	fmt.Println("🔧 剧本拆分失败 - 智能修复工具")
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
	
	projectUUID := "e9010b35-46aa-4a9f-bda7-85782d9e2e82"
	fmt.Printf("📋 正在处理项目：%s\n\n", projectUUID)
	
	var projectID int64
	var status string
	var scriptLen int
	
	err = db.QueryRowContext(ctx, `
		SELECT id, status, length(COALESCE(source_script_text, ''))
		FROM projects WHERE project_uuid = $1
	`, projectUUID).Scan(&projectID, &status, &scriptLen)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询项目失败：%v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("📊 项目诊断:\n")
	fmt.Printf("   • 项目名称：%s\n", projectUUID[:min(len(projectUUID), 20)])
	fmt.Printf("   • 项目 ID: %d\n", projectID)
	fmt.Printf("   • 当前状态：%s\n", status)
	fmt.Printf("   • 剧本长度：%d 字符\n", scriptLen)
	fmt.Println()
	
	// 检查有效密钥数量（修复 SQL）
	var validKeysCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM models m
		WHERE m.model_key IN ('deepseek-v4','qwen-plus','glm-4')
		  AND m.enabled = TRUE
	`).Scan(&validKeysCount)
	
	if err == nil {
		fmt.Printf("   • 有效密钥数：%d个\n", validKeysCount)
		if validKeysCount == 0 {
			fmt.Println("   ⚠️  未检测到有效密钥...")
			
			// 显示数据库中有哪些 LLM 相关的模型
			fmt.Println("\n   🔍 数据库中的 LLM 模型配置:")
			rows, _ := db.QueryContext(ctx, `
				SELECT DISTINCT mp.name, m.model_key, mk.enabled, mk.key_prefix
				FROM model_keys mk
				JOIN model_providers mp ON mp.id = mk.provider_id
				JOIN models m ON m.provider_id = mp.id
				WHERE m.model_key LIKE '%deepseek%' OR m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%'
				ORDER BY mp.name, m.model_key
			`)
			defer rows.Close()
			
			for rows.Next() {
				var providerName, modelKey string
				var enabled, keyPrefix bool
				rows.Scan(&providerName, &modelKey, &enabled, &keyPrefix)
				statusStr := "❌禁用"
				if enabled && keyPrefix {
					statusStr = "✅已启用"
				} else if enabled {
					statusStr = "⚠️无密钥"
				}
				fmt.Printf("     • %s / %-20s -> %s\n", providerName, modelKey, statusStr)
			}
			
			fmt.Println("\n   💡 建议操作:")
			fmt.Println("     1. 打开管理端页面 (http://localhost:8080/admin)")
			fmt.Println("     2. 进入「系统设置」→「模型配置」")
			fmt.Println("     3. 添加有效的 DeepSeek API 密钥")
			fmt.Println("     4. 确保设置为\"启用\"状态")
			fmt.Println()
		}
	} else {
		fmt.Printf("   ⚠️  查询密钥状态失败：%v\n", err)
	}
	fmt.Println()
	
	// 清理旧的拆分结果
	db.ExecContext(ctx, "UPDATE shots SET deleted_at = NOW() WHERE project_id = $1", projectID)
	db.ExecContext(ctx, "UPDATE shot_groups SET deleted_at = NOW() WHERE project_id = $1", projectID)
	db.ExecContext(ctx, "UPDATE narrative_units SET deleted_at = NOW() WHERE project_id = $1", projectID)
	db.ExecContext(ctx, "UPDATE episodes SET deleted_at = NOW() WHERE project_id = $1", projectID)
	
	fmt.Println("✅ 已清理旧的拆分结果数据")
	
	// 重置项目状态
	result, err := db.ExecContext(ctx, `
		UPDATE projects 
		SET status = 'DRAFT', updated_at = NOW(),
		    total_episode_count = 0, total_duration_sec = 0
		WHERE project_uuid = $1
	`, projectUUID)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新项目状态失败：%v\n", err)
		os.Exit(1)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("✅ 项目状态已重置为：DRAFT (%d行)\n", rowsAffected)
	}
	
	// 最终验证
	var finalStatus string
	db.QueryRowContext(ctx, "SELECT status FROM projects WHERE project_uuid = $1", projectUUID).Scan(&finalStatus)
	
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("✅ 智能修复完成！")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	fmt.Println("📊 当前状态:")
	fmt.Printf("  • 项目状态：%s ✅\n", finalStatus)
	fmt.Printf("  • 剧本内容：%d字符 ✅\n", scriptLen)
	fmt.Printf("  • 可用密钥：%d个\n", validKeysCount)
	fmt.Println()
	fmt.Println("🎯 下一步操作:")
	fmt.Println("  1. 刷新浏览器页面")
	fmt.Println("  2. 进入《逃离云端的人》项目")
	fmt.Println("  3. 点击\"拆分剧本\"按钮")
	fmt.Println("  4. 等待 AI 处理完成（通常 30-120 秒）")
	fmt.Println()
	if validKeysCount == 0 {
		fmt.Println("⚠️  注意：系统中暂无可用密钥!")
		fmt.Println("  如果拆分失败，请在管理端添加有效的 DeepSeek/Qwen/GLM 密钥")
	}
	fmt.Println("═══════════════════════════════════════════")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
