// show-db-tables.go - 显示数据库表
package main

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	
	db, _ := sql.Open("pgx", dsn)
	defer db.Close()
	
	rows, _ := db.Query(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public'
		ORDER BY table_name
	`)
	defer rows.Close()
	
	fmt.Println("📋 数据库表:")
	for rows.Next() {
		var tableName string
		rows.Scan(&tableName)
		if tableName != "" {
			fmt.Printf("   • %s\n", tableName)
		}
	}
	
	// 查找包含 split 的表
	rows2, _ := db.Query(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name LIKE '%split%'
	`)
	defer rows2.Close()
	
	fmt.Println("\n🔍 拆分相关表:")
	for rows2.Next() {
		var tableName string
		rows2.Scan(&tableName)
		fmt.Printf("   • %s\n", tableName)
	}
}
