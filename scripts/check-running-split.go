// check-running-split.go - 检查正在运行的拆分任务
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	db, _ := sql.Open("pgx", dsn)
	defer db.Close()
	
	rows, err := db.Query(`
		SELECT id, request_id, status, created_at, updated_at, 
		       jsonb_extract_path_text(request_params_json, 'script_text')::text as script_text
		FROM generation_tasks 
		WHERE task_type = 'SCRIPT_SPLIT' 
		AND status IN ('RUNNING', 'QUEUED')
		ORDER BY created_at DESC
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	count := 0
	
	for rows.Next() {
		count++
		var id int64
		var requestID string
		var status string
		var createdAt string
		var updatedAt string
		var scriptText string
		
		err := rows.Scan(&id, &requestID, &status, &createdAt, &updatedAt, &scriptText)
		if err != nil {
			continue
		}
		
		scriptLen := len(scriptText)
		
		fmt.Printf("\n🔥 正在运行的任务:\n")
		fmt.Printf("   ID: %d\n", id)
		fmt.Printf("   Request ID: %s\n", requestID)
		fmt.Printf("   状态：%s\n", status)
		fmt.Printf("   创建时间：%s\n", createdAt)
		fmt.Printf("   更新时间：%s\n", updatedAt)
		fmt.Printf("   剧本长度：%d字\n", scriptLen)
		
		// 显示前 80 字
		display := scriptText
		if len(display) > 80 {
			display = display[:80] + "..."
		}
		display = fmt.Sprintf("%q", display)
		fmt.Printf("   内容预览：%s\n", display)
	}
	
	if count == 0 {
		fmt.Println("\n⚠️  没有正在运行的拆分任务")
	} else {
		fmt.Printf("\n✅ 找到 %d个正在运行的任务\n", count)
	}
}
