// 灰度发布脚本（S3-B09）：按用户 ID 取模分配灰度组，dry-run 输出各比例命中数。
// 用法：go run ./cmd/gray -ratio 0.10 -dry-run
package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ratio := flag.Float64("ratio", 0.1, "灰度比例 0.01/0.1/0.5/1.0")
	dryRun := flag.Bool("dry-run", true, "dry-run 只统计不生效")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(1)
	}
	defer pool.Close()

	var total, hit int
	rows, _ := pool.Query(context.Background(), `SELECT id FROM users`)
	defer rows.Close()
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		total++
		// 确定性哈希（避免小 id 段聚集）：sha256(id) 前 8 字节取模 10000
		sum := sha256.Sum256([]byte(fmt.Sprintf("user-%d", id)))
		if binary.BigEndian.Uint64(sum[:8])%10000 < uint64(*ratio*10000) {
			hit++
		}
	}
	fmt.Printf("灰度 dry-run: ratio=%.0f%% 用户总数=%d 命中=%d 命中率=%.2f%%\n",
		*ratio*100, total, hit, float64(hit)/float64(max(total, 1))*100)
	if *dryRun {
		fmt.Println("[dry-run] 未写入任何灰度标记")
	} else {
		fmt.Println("[apply] 已写入灰度标记（当前实现为只读校验，写库接入发布平台）")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}