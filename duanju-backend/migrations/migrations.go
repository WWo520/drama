// Package migrations 内嵌全部迁移 SQL 文件，供 cmd/migrate 与测试复用。
package migrations

import "embed"

// FS 嵌入的 migrations 目录内容（000001_*.sql 等）。
//
//go:embed *.sql
var FS embed.FS
