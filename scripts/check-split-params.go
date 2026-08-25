// check-split-params.go - 检查拆分任务的参数是否正确
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	fmt.Println("🔍 检查拆分任务参数")
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
	
	// 查询最近的 SCRIPT_SPLIT 任务
	rows, err := db.QueryContext(ctx, `
		SELECT id, request_id, status, created_at, request_params_json
		FROM generation_tasks 
		WHERE task_type = 'SCRIPT_SPLIT' 
		ORDER BY created_at DESC 
		LIMIT 3
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var id int64
		var requestID, status string
		var createdAt interface{}
		var paramsJSON []byte
		
		err := rows.Scan(&id, &requestID, &status, &createdAt, &paramsJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Scan 失败：%v\n", err)
			continue
		}
		
		count++
		fmt.Printf("\n📋 任务 #%d [%s]\n", id, requestID)
		fmt.Printf("   状态：%s\n", status)
		fmt.Printf("   创建时间：%v\n", createdAt)
		
		// 解析 JSON 参数
		var params map[string]interface{}
		if err := json.Unmarshal(paramsJSON, &params); err != nil {
			fmt.Printf("   ❌ JSON 解析失败：%v\n", err)
			continue
		}
		
		fmt.Printf("   参数:\n")
		
		// 检查 script_text
		if scriptText, ok := params["script_text"].(string); ok {
			scriptLen := len(scriptText)
			fmt.Printf("     • script_text: %d 字\n", scriptLen)
			
			if scriptLen == 0 {
				fmt.Printf("       ⚠️  WARNING: script_text 为空!\n")
			} else if scriptLen < 50 {
				fmt.Printf("       ⚠️  WARNING: script_text 太短 (%d 字)\n", scriptLen)
			} else {
				// 显示前 100 个字符
				display := scriptText
				if len(display) > 100 {
					display = display[:100] + "..."
				}
				// 转义换行符用于显示
				display = strings.ReplaceAll(display, "\n", "\\n")
				display = strings.ReplaceAll(display, "\"", "\\\"")
				fmt.Printf("       内容预览 (前 100 字): \"%s\"\n", display)
				
				// 验证是否包含剧本关键词
				if strings.Contains(strings.ToLower(scriptText), "场景") || 
				   strings.Contains(strings.ToLower(scriptText), "第") ||
				   strings.Contains(scriptText, "【") {
					fmt.Printf("       ✅ 看起来像有效的剧本内容\n")
				}
			}
		}
		
		// 检查 episode_duration_min
		if epDur, ok := params["episode_duration_min"]; ok {
			fmt.Printf("     • episode_duration_min: %v\n", epDur)
		}
		
		// 检查 split_skill_prompt
		if skillPrompt, ok := params["split_skill_prompt"].(string); ok {
			if skillPrompt != "" {
				fmt.Printf("     • split_skill_prompt: %d 字\n", len(skillPrompt))
			} else {
				fmt.Printf("     • split_skill_prompt: (空，使用默认)\n")
			}
		}
	}
	
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	
	if count == 0 {
		fmt.Println("⚠️  未找到任何 SCRIPT_SPLIT 任务")
		fmt.Println()
		fmt.Println("💡 建议:")
		fmt.Println("  1. 先刷新浏览器页面")
		fmt.Println("  2. 进入《逃离云端的人》项目")
		fmt.Println("  3. 输入剧本原文后点击“拆分剧本”")
	} else {
		fmt.Printf("✅ 共检查 %d个最近的任务\n", count)
	}
	
	fmt.Println("═══════════════════════════════════════════")
}
