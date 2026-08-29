// analyze-all-split-tasks.go - 分析所有拆分任务的详细情况
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	db, _ := sql.Open("pgx", dsn)
	defer db.Close()
	
	rows, err := db.Query(`
		SELECT id, request_id, status, created_at, updated_at, request_params_json
		FROM generation_tasks 
		WHERE task_type = 'SCRIPT_SPLIT'
		ORDER BY created_at DESC 
		LIMIT 5
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	fmt.Println("📊 最近拆分任务列表:")
	fmt.Println("═══════════════════════════════════════════")
	
	for rows.Next() {
		var id int64
		var requestID string
		var status string
		var createdAt, updatedAt time.Time
		var paramsJSON []byte
		
		err := rows.Scan(&id, &requestID, &status, &createdAt, &updatedAt, &paramsJSON)
		if err != nil {
			continue
		}
		
		duration := "N/A"
		if !updatedAt.IsZero() && !createdAt.IsZero() {
			d := updatedAt.Sub(createdAt)
			if d < time.Minute {
				duration = fmt.Sprintf("%.0f 秒", d.Seconds())
			} else if d < time.Hour {
				duration = fmt.Sprintf("%d分%d秒", int(d.Minutes()), int(d.Seconds())%60)
			} else {
				duration = fmt.Sprintf("%d小时%d分", int(d.Hours()), int(d.Minutes())%60)
			}
		}
		
		// 解析参数
		paramsMap := make(map[string]interface{})
		json.Unmarshal(paramsJSON, &paramsMap)
		
		scriptText := ""
		if text, ok := paramsMap["script_text"].(string); ok {
			scriptText = text
		}
		scriptLen := len(scriptText)
		
		epDur := "默认 2 分钟"
		if dur, ok := paramsMap["episode_duration_min"]; ok {
			epDur = fmt.Sprintf("%v 分钟", dur)
		}
		
		skillPrompt := "(无)"
		if prompt, ok := paramsMap["split_skill_prompt"].(string); ok {
			if prompt != "" {
				skillPrompt = fmt.Sprintf("%d字自定义技能", len(prompt))
			} else {
				skillPrompt = "(使用默认规则)"
			}
		}
		
		// 状态说明
		statusDesc := ""
		switch status {
		case "QUEUED":
			statusDesc = "等待中"
		case "RUNNING":
			statusDesc = "进行中"
		case "SUCCESS":
			statusDesc = "✅成功"
		case "FAILED_RETRYING":
			statusDesc = "⚠️ 重试中"
		case "FAILED_DEAD":
			statusDesc = "❌彻底失败"
		default:
			statusDesc = "未知状态"
		}
		
		fmt.Printf("\n#%d [%s] %s\n", id, requestID, status)
		fmt.Printf("   ⏱️  耗时：%s\n", duration)
		fmt.Printf("   📝 剧本：%d字 | 时长：%s\n", scriptLen, epDur)
		fmt.Printf("   🎯 技能：%s\n", skillPrompt)
		fmt.Printf("   🔍 内容预览 (前 80 字): %s\n", truncate(scriptText, 80))
		fmt.Printf("   📋 状态说明：%s\n", statusDesc)
		
		if scriptLen > 10000 {
			fmt.Printf("   ⚠️  警告：剧本很长 (%d字)，可能耗时较久!\n", scriptLen)
		}
		
		if strings.Contains(requestID, "split-") && id >= 520 {
			fmt.Printf("   ℹ️  这是优化后的版本（段 B 并行处理）\n")
		}
	}
	
	fmt.Println("\n═══════════════════════════════════════════")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	display := s[:n]
	// 转义
	display = strings.ReplaceAll(display, "\n", "\\n")
	display = strings.ReplaceAll(display, "\"", "\\\"")
	return display + "..."
}
