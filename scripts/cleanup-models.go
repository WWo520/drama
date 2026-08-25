// cleanup-models.go - 清理 LLM 模型列表，只保留豆包 4.5 和 DeepSeek V4 Flash
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
	
	fmt.Println("🔧 清理 LLM 模型列表")
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
	
	// Step 1: 显示当前的所有 LLM 相关模型
	fmt.Println("📊 当前的 LLM 模型配置:")
	rows, _ := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled, mp.name as provider_name
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.deleted_at IS NULL
		AND (m.model_key LIKE '%doubao%' OR m.model_key LIKE '%deepseek%' OR 
		     m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%')
		ORDER BY mp.name, m.model_key
	`)
	defer rows.Close()
	
	type ModelInfo struct {
		ID           int64
		ModelKey     string
		Enabled      bool
		ProviderName string
	}
	
	var allModels []ModelInfo
	for rows.Next() {
		var id int64
		var modelKey, enabledStr, providerName string
		rows.Scan(&id, &modelKey, &enabledStr, &providerName)
		
		enabled := enabledStr == "true"
		allModels = append(allModels, ModelInfo{
			ID:           id,
			ModelKey:     modelKey,
			Enabled:      enabled,
			ProviderName: providerName,
		})
	}
	
	for _, m := range allModels {
		status := "❌禁用"
		if m.Enabled {
			status = "✅启用"
		}
		fmt.Printf("   • %s / %-30s [%d] -> %s\n", 
			m.ProviderName, m.ModelKey, m.ID, status)
	}
	fmt.Println()
	
	// Step 2: 确定要保留的模型
	const (
		keepDoubao45    = true  // 保留豆包 4.5
		keepDeepSeekV4  = true  // 保留 DeepSeek V4 Flash
	)
	
	// Step 3: 处理每个模型
	fmt.Println("🔨 开始清理模型...")
	fmt.Println()
	
	var keptModels []ModelInfo
	var removedCount int
	
	for _, m := range allModels {
		shouldKeep := false
		
		// 检查是否是目标保留的模型
		if keepDoubao45 && containsString(m.ModelKey, []string{"doubao", "seedream", "v4-5"}) {
			shouldKeep = true
		}
		
		if keepDeepSeekV4 && containsString(m.ModelKey, []string{"deepseek-v4-flash", "deepseek-flash", "v4-flash"}) {
			shouldKeep = true
		}
		
		if shouldKeep {
			keptModels = append(keptModels, m)
			fmt.Printf("   ✅ 保留：%-30s [%d] (%s)\n", m.ModelKey, m.ID, m.ProviderName)
		} else {
			// 禁用不需要的模型
			result, err := db.ExecContext(ctx, `
				UPDATE models 
				SET enabled = FALSE, deleted_at = NOW(), updated_at = NOW()
				WHERE id = $1 AND deleted_at IS NULL
			`, m.ID)
			
			if err != nil {
				fmt.Printf("   ⚠️  禁用失败：%-30s [%d] - %v\n", m.ModelKey, m.ID, err)
			} else {
				rowsAffected, _ := result.RowsAffected()
				if rowsAffected > 0 {
					fmt.Printf("   ❌ 禁用：%-30s [%d] (%s)\n", m.ModelKey, m.ID, m.ProviderName)
					removedCount++
				} else {
					fmt.Printf("   ⏭️  跳过：%-30s [%d] (已删除或未找到)\n", m.ModelKey, m.ID)
				}
			}
		}
	}
	
	fmt.Println()
	
	// Step 4: 显示最终状态
	fmt.Println("📋 最终的 LLM 模型配置:")
	finalRows, _ := db.QueryContext(ctx, `
		SELECT m.id, m.model_key, m.enabled, mp.name as provider_name
		FROM models m
		JOIN model_providers mp ON mp.id = m.provider_id
		WHERE m.deleted_at IS NULL
		AND (m.model_key LIKE '%doubao%' OR m.model_key LIKE '%deepseek%' OR 
		     m.model_key LIKE '%qwen%' OR m.model_key LIKE '%glm%')
		ORDER BY mp.name, m.model_key
	`)
	defer finalRows.Close()
	
	for finalRows.Next() {
		var id int64
		var modelKey, enabledStr, providerName string
		finalRows.Scan(&id, &modelKey, &enabledStr, &providerName)
		
		status := "❌禁用"
		if enabledStr == "true" {
			status = "✅启用"
		}
		fmt.Printf("   • %s / %-30s [%d] -> %s\n", 
			providerName, modelKey, id, status)
	}
	fmt.Println()
	
	// Step 5: 总结
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("📊 统计:\n")
	fmt.Printf("   • 保留：%d个\n", len(keptModels))
	fmt.Printf("   • 禁用：%d个\n", removedCount)
	fmt.Printf("   • 总计：%d个\n", len(keptModels)+removedCount)
	fmt.Println()
	
	if len(keptModels) == 2 {
		fmt.Println("✅ 清理完成！系统现在只有 2 个模型:")
		fmt.Println("   1. 豆包 4.5 (推荐用于文生图)")
		fmt.Println("   2. DeepSeek V4 Flash (推荐用于文本生成)")
	} else if len(keptModels) == 1 {
		fmt.Printf("⚠️  警告：只剩 %d个模型，可能影响功能!\n", len(keptModels))
	}
	fmt.Println()
	fmt.Println("💡 提示:")
	fmt.Println("  • Doubao-V4-Flash: 性能优异，性价比高")
	fmt.Println("  • DeepSeek-V4-Flash: 长上下文，逻辑推理强")
	fmt.Println("═══════════════════════════════════════════")
}

// containsString 辅助函数：判断字符串切片是否包含指定子串
func containsString(s string, substrings []string) bool {
	sLower := fmt.Sprintf("%s", s)
	for _, sub := range substrings {
		if contains(sLower, sub) {
			return true
		}
	}
	return false
}

// contains 简单的字符串包含检查
func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
