// analyze-split-performance.go - 分析拆分性能瓶颈
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	fmt.Println("⏱️  分析剧本拆分性能")
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
	
	// 查询成功的拆分任务
	rows, err := db.QueryContext(ctx, `
		SELECT id, request_id, status, created_at, updated_at, request_params_json
		FROM generation_tasks 
		WHERE task_type = 'SCRIPT_SPLIT' AND status = 'SUCCESS'
		ORDER BY created_at DESC 
		LIMIT 5
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	var totalTasks int
	for rows.Next() {
		totalTasks++
		var id int64
		var requestID string
		var status string
		var createdAt, updatedAt time.Time
		var paramsJSON []byte
		
		err := rows.Scan(&id, &requestID, &status, &createdAt, &updatedAt, &paramsJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Scan 失败：%v\n", err)
			continue
		}
		
		// 计算耗时
		duration := updatedAt.Sub(createdAt)
		durationMinutes := duration.Minutes()
		durationFormatted := formatDuration(duration)
		
		// 解析参数
		var params map[string]interface{}
		json.Unmarshal(paramsJSON, &params)
		
		scriptText := ""
		if text, ok := params["script_text"].(string); ok {
			scriptText = text
		}
		scriptLen := len(scriptText)
		
		episodeDur := 2
		if dur, ok := params["episode_duration_min"]; ok {
			if d, ok := dur.(float64); ok {
				episodeDur = int(d)
			}
		}
		
		// 性能评估
		perfRating := "🟡 正常"
		reason := ""
		
		if durationMinutes > 30 {
			perfRating = "🔴 极慢"
			reason = "超过 30 分钟，可能存在严重的 API 超时或重试"
		} else if durationMinutes > 10 {
			perfRating = "🟠 较慢"
			reason = "10-30 分钟，可能有少量重试或网络延迟"
		} else if durationMinutes > 5 {
			perfRating = "🟡 中等"
			reason = "5-10 分钟，属于预期范围"
		} else {
			perfRating = "🟢 快"
			reason = "5 分钟内，性能优异"
		}
		
		fmt.Printf("📋 任务 #%d [%s]\n", id, requestID)
		fmt.Printf("   状态：%s\n", status)
		fmt.Printf("   创建时间：%s\n", createdAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("   完成时间：%s\n", updatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("   ⏱️  总耗时：%s (%.1f 分钟)\n", durationFormatted, durationMinutes)
		fmt.Printf("   剧本长度：%d 字\n", scriptLen)
		fmt.Printf("   单集时长：%d 分钟\n", episodeDur)
		fmt.Printf("   性能评估：%s %s\n", perfRating, reason)
		
		if durationMinutes > 15 {
			fmt.Printf("\n   🚨 警告：此任务耗时过长!\n")
			fmt.Printf("   可能的原因:\n")
			fmt.Printf("     1. DeepSeek API 响应超时\n")
			fmt.Printf("     2. 多次重试导致累积延迟\n")
			fmt.Printf("     3. JSON 验证失败需要重新生成\n")
			fmt.Printf("     4. 资产提示词优化任务阻塞\n")
		}
		
		fmt.Println()
		fmt.Println("───")
		fmt.Println()
	}
	
	fmt.Println("═══════════════════════════════════════════")
	
	if totalTasks == 0 {
		fmt.Println("⚠️  未找到成功的拆分任务")
		fmt.Println()
		fmt.Println("💡 说明：系统可能还在初始化或首次使用")
	} else {
		fmt.Printf("✅ 共分析了 %d个最近的成功任务\n", totalTasks)
		fmt.Println()
	}
	
	fmt.Println("💡 性能优化建议:")
	fmt.Println("  1. 检查 DeepSeek API 响应速度和配额")
	fmt.Println("  2. 减少不必要的重试次数")
	fmt.Println("  3. 考虑禁用异步的资产提示词优化（如不需要）")
	fmt.Println("  4. 增加并发请求数量提升吞吐量")
	fmt.Println("═══════════════════════════════════════════")
}

// formatDuration 格式化持续时间
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f 秒", d.Seconds())
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%d分%d秒", minutes, seconds)
		}
		return fmt.Sprintf("%d分", minutes)
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%d小时%d分", hours, minutes)
		}
		return fmt.Sprintf("%d小时", hours)
	}
}
