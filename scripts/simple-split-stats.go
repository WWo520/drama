// simple-split-stats.go - 简单查询拆分结果
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
		SELECT id, episode_count, unit_count, created_at 
		FROM split_results 
		ORDER BY created_at DESC 
		LIMIT 5
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	fmt.Println("📊 最近拆分任务的分集情况:")
	fmt.Println("═══════════════════════════════════════════")
	
	for rows.Next() {
		var id int64
		var episodeCount, unitCount int
		var createdAt string
		
		err := rows.Scan(&id, &episodeCount, &unitCount, &createdAt)
		if err != nil {
			continue
		}
		
		fmt.Printf("\nID: %d | 创建时间：%s\n", id, createdAt)
		fmt.Printf("  剧集数：%d集\n", episodeCount)
		fmt.Printf("  叙事单元总数：%d个\n", unitCount)
		fmt.Printf("  平均每集单元数：%.1f个\n", float64(unitCount)/float64(episodeCount))
		
		if episodeCount > 8 {
			fmt.Printf("  ⚠️  警告：%d集，需要多次 API 调用!\n", episodeCount)
		} else if episodeCount <= 3 {
			fmt.Printf("  ✅ 快速：%d集，应该比较快\n", episodeCount)
		}
	}
}
