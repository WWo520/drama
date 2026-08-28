// get-split-stats.go - 获取分集统计信息
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	
	// 查询最近的拆分结果
	rows, err := db.QueryContext(ctx, `
		SELECT project_id, created_at, episode_count, unit_count
		FROM split_results 
		ORDER BY created_at DESC 
		LIMIT 5
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Query error: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	fmt.Println("📊 最近拆分任务的分集情况:")
	fmt.Println("═══════════════════════════════════════════")
	
	for rows.Next() {
		var projectID int64
		var createdAt time.Time
		var episodeCount, unitCount int
		
		err := rows.Scan(&projectID, &createdAt, &episodeCount, &unitCount)
		if err != nil {
			continue
		}
		
		// 查找对应的剧本长度
		var scriptLen int
		db.QueryRowContext(ctx, `
			SELECT jsonb_extract_path_text(request_params_json, 'script_text')::text
			FROM generation_tasks 
			WHERE id = (
				SELECT id FROM generation_tasks 
				WHERE task_type = 'SCRIPT_SPLIT' 
				AND status IN ('SUCCESS', 'FAILED_DEAD', 'FAILED_RETRYING')
				ORDER BY created_at DESC LIMIT 1
			)
		`).Scan(&scriptLen)
		
		fmt.Printf("\n项目 #%d [%s]\n", projectID, createdAt.Format("2006-01-02 15:04"))
		fmt.Printf("  剧集数：%d集\n", episodeCount)
		fmt.Printf("  叙事单元总数：%d个\n", unitCount)
		fmt.Printf("  平均每集单元数：%.1f个\n", float64(unitCount)/float64(episodeCount))
		
		// 性能推断
		if episodeCount > 8 {
			fmt.Printf("  ⚠️  注意：%d集可能需要多次 API 调用!\n", episodeCount)
		} else if episodeCount <= 3 {
			fmt.Printf("  ✅ 快速：只有%d集，应该比较快\n", episodeCount)
		}
	}
}
