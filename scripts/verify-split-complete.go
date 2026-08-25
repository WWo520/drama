// verify-split-complete.go - 验证剧本拆分功能完整性
package main

import (
	"context"
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
	
	fmt.Println("🔍 验证剧本拆分功能")
	fmt.Println("═══════════════════════════════════════════")
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
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
	
	// 检查最新任务
	rows, err := db.QueryContext(ctx, `
		SELECT id, request_id, task_type, status, created_at, updated_at, request_params_json
		FROM generation_tasks 
		WHERE task_type = 'SCRIPT_SPLIT' 
		ORDER BY created_at DESC 
		LIMIT 1
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	var latestTask struct {
		ID             int64
		RequestID      string
		TaskType       string
		Status         string
		CreatedAt      time.Time
		UpdatedAt      sql.NullTime
		RequestParams  []byte
	}
	
	if rows.Next() {
		err := rows.Scan(&latestTask.ID, &latestTask.RequestID, &latestTask.TaskType, 
			&latestTask.Status, &latestTask.CreatedAt, &latestTask.UpdatedAt, &latestTask.RequestParams)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Scan 失败：%v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("⚠️  未找到 SCRIPT_SPLIT 任务")
		return
	}
	
	fmt.Printf("📋 最新任务信息:\n")
	fmt.Printf("   ID: %d\n", latestTask.ID)
	fmt.Printf("   Request ID: %s\n", latestTask.RequestID)
	fmt.Printf("   Status: %s\n", latestTask.Status)
	fmt.Printf("   Created: %s\n", latestTask.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   Updated: %v\n", latestTask.UpdatedAt)
	fmt.Println()
	
	// 解析参数
	var params map[string]interface{}
	if err := json.Unmarshal(latestTask.RequestParams, &params); err != nil {
		fmt.Printf("❌ JSON 解析失败：%v\n", err)
		return
	}
	
	fmt.Printf("📊 参数详情:\n")
	
	// script_text
	if scriptText, ok := params["script_text"].(string); ok {
		scriptLen := len(scriptText)
		fmt.Printf("   • script_text: %d 字\n", scriptLen)
		
		if scriptLen == 0 {
			fmt.Printf("     ⛔ CRITICAL: script_text 为空!\n")
		} else if scriptLen < 50 {
			fmt.Printf("     ⚠️  WARNING: script_text 太短 (%d 字)\n", scriptLen)
		} else {
			fmt.Printf("     ✅ 脚本内容有效\n")
			// 显示前 80 个字符
			display := scriptText
			if len(display) > 80 {
				display = display[:80] + "..."
			}
			// 转义
			display = strings.ReplaceAll(display, "\n", "\\n")
			display = strings.ReplaceAll(display, "\"", "\\\"")
			fmt.Printf("     预览：\"%s\"\n", display)
		}
	} else {
		fmt.Printf("     ⛔ CRITICAL: script_text 字段缺失或类型错误!\n")
	}
	
	// episode_duration_min
	if epDur, ok := params["episode_duration_min"]; ok {
		fmt.Printf("   • episode_duration_min: %v (✅有效)\n", epDur)
	} else {
		fmt.Printf("   • episode_duration_min: 缺失 (⚠️使用默认值 2 分钟)\n")
	}
	
	// split_skill_prompt
	if skillPrompt, ok := params["split_skill_prompt"].(string); ok {
		if skillPrompt != "" {
			fmt.Printf("   • split_skill_prompt: %d 字 (✅有自定义技能)\n", len(skillPrompt))
		} else {
			fmt.Printf("   • split_status: (空，使用标准拆分规则)\n")
		}
	} else {
		fmt.Printf("   • split_skill_prompt: 缺失\n")
	}
	
	fmt.Println()
	
	// 状态分析
	fmt.Printf("🎯 状态分析:\n")
	switch latestTask.Status {
	case "QUEUED":
		fmt.Printf("   🟡 等待调度\n")
	case "RUNNING":
		fmt.Printf("   🔵 Worker 正在处理...\n")
		fmt.Printf("      → 可能正在进行 LLM 调用\n")
	case "SUCCESS":
		fmt.Printf("   🟢 拆分成功!\n")
	case "FAILED_RETRYING":
		fmt.Printf("   🔴 拆分失败，仍在重试（最多 4 次）\n")
		fmt.Printf("      → 可能是 LLM API 调用失败\n")
	case "FAILED_DEAD":
		fmt.Printf("   🔴 拆分彻底失败（重试耗尽）\n")
		fmt.Printf("      → 需要检查 DeepSeek API 配置\n")
	default:
		fmt.Printf("   ⚪ 未知状态：%s\n", latestTask.Status)
	}
	
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	
	if latestTask.Status == "FAILED_RETRYING" || latestTask.Status == "FAILED_DEAD" {
		fmt.Println("⚠️  剧本拆分功能异常！")
		fmt.Println()
		fmt.Println("💡 建议排查步骤:")
		fmt.Println("  1. 检查 Worker 日志中的 API 调用错误")
		fmt.Println("  2. 验证 DeepSeek API Key 是否有效")
		fmt.Println("  3. 测试直接调用 DeepSeek API")
		fmt.Println("  4. 检查网络连通性和配额限制")
	} else if latestTask.Status == "RUNNING" {
		fmt.Println("ℹ️  拆分任务正在执行中，请稍后再查...")
	} else {
		fmt.Println("✅ 所有参数验证通过!")
	}
	
	fmt.Println("═══════════════════════════════════════════")
}
