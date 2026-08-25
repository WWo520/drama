// show-tables.go - 显示数据库表结构
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
	
	ctx, cancel := context.WithDeadline(context.Background(), nil)
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
	
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_name = 'models'
		ORDER BY ordinal_position
	`)
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}
	defer rows.Close()
	
	for rows.Next() {
		var tableName, columnName, dataType string
		rows.Scan(&tableName, &columnName, &dataType)
		fmt.Printf("%-20s %-30s %s\n", tableName, columnName, dataType)
	}
}
