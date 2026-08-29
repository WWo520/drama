// cleanup-edited-prompt.go - 清理含背景描述的 edited_prompt（回退到拆分阶段干净描述）
//go:build ignore

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var bgKeywords = []string{"背景", "房间", "街道", "室内", "场景", "环境", "窗外", "地板", "墙面", "天空", "办公室", "客厅", "公寓", "工位", "电脑", "沙发", "桌子", "窗户", "茶水间"}

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("🧹 清理含背景描述的 edited_prompt")
	fmt.Println("═══════════════════════════════════════════")

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 数据库连接失败：%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 查询所有 edited_prompt 非空且含背景词的角色
	fmt.Println("\n【1】含背景词的 edited_prompt 清单")
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, edited_prompt
		FROM characters
		WHERE deleted_at IS NULL AND edited_prompt IS NOT NULL AND edited_prompt <> ''
		ORDER BY id`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 查询失败：%v\n", err)
		os.Exit(1)
	}

	type charRow struct {
		id      int64
		name    string
		prompt  string
		hasBG   bool
		matched []string
	}
	var all []charRow
	var dirty []charRow

	for rows.Next() {
		var c charRow
		rows.Scan(&c.id, &c.name, &c.prompt)
		all = append(all, c)
		for _, kw := range bgKeywords {
			if indexOf(c.prompt, kw) >= 0 {
				c.hasBG = true
				c.matched = append(c.matched, kw)
			}
		}
		if c.hasBG {
			dirty = append(dirty, c)
		}
	}
	rows.Close()

	fmt.Printf("  有 edited_prompt 的角色：%d 个\n", all)
	fmt.Printf("  其中含背景词：%d 个\n", len(dirty))
	for _, c := range dirty {
		fmt.Printf("\n  ⚠️  [%d] %s  命中词: %v\n", c.id, c.name, c.matched)
		fmt.Printf("      %s\n", truncateStr(c.prompt, 160))
	}

	if len(dirty) == 0 {
		fmt.Println("\n✅ 无需清理")
		return
	}

	// 2. 执行清理：edited_prompt 置 NULL（回退 description_notes）
	fmt.Println("\n【2】执行清理（edited_prompt → NULL）")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 事务启动失败：%v\n", err)
		os.Exit(1)
	}
	for _, c := range dirty {
		if _, err := tx.ExecContext(ctx,
			`UPDATE characters SET edited_prompt=NULL, updated_at=NOW() WHERE id=$1`, c.id); err != nil {
			fmt.Printf("  ❌ [%d] %s 清理失败：%v\n", c.id, c.name, err)
			tx.Rollback()
			os.Exit(1)
		}
		fmt.Printf("  ✅ [%d] %s edited_prompt 已清空（回退拆分描述）\n", c.id, c.name)
	}
	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 提交失败：%v\n", err)
		os.Exit(1)
	}

	// 3. 验证
	fmt.Println("\n【3】清理后验证")
	var remain int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM characters WHERE deleted_at IS NULL AND edited_prompt IS NOT NULL AND edited_prompt <> ''`).Scan(&remain)
	fmt.Printf("  剩余有 edited_prompt 的角色：%d（全部为无背景的干净版本）\n", remain)

	fmt.Println("\n═══════════════════════════════════════════")
	fmt.Println("✅ 完成！下次生成将回退到拆分阶段的干净描述 + 新无背景兜底约束")
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
