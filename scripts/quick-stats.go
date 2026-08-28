// quick-stats.go - 快速查询分集统计
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
	
	rows, err := db.Query(`SELECT COUNT(*) as count FROM episodes`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	var totalCount int
	rows.Scan(&totalCount)
	
	fmt.Printf("📊 总剧集数：%d集\n", totalCount)
}
