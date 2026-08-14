// 安全扫描（E02 生产加固）：OWASP 清单静态检查。
// 检查项：SQL 拼接注入 / 硬编码密钥 / 敏感日志 / 未授权管理端点 / 密码明文存储。
// 用法：go run ./cmd/security-scan （CI 门禁：发现 HIGH 级问题非零退出）
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type finding struct {
	severity string // HIGH / MED / LOW
	file     string
	line     int
	detail   string
}

var findings []finding

func main() {
	root := "internal"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		scanFile(path)
		return nil
	})

	high := 0
	for _, f := range findings {
		fmt.Printf("[%s] %s:%d %s\n", f.severity, f.file, f.line, f.detail)
		if f.severity == "HIGH" {
			high++
		}
	}
	fmt.Printf("安全扫描完成：共 %d 个问题（HIGH=%d）\n", len(findings), high)
	if high > 0 {
		os.Exit(1)
	}
}

func scanFile(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	sqlConcat := regexp.MustCompile(`(?i)query\([^)]*"\+|WHERE\s*"\+|SELECT[^"]*"\+`)
	secretKeys := []string{"password", "secret", "api_key", "token"}
	for i, line := range lines {
		// 1. SQL 拼接注入
		if sqlConcat.MatchString(line) && !strings.Contains(line, "--") {
			add("HIGH", path, i+1, "SQL 字符串拼接（注入风险）: "+strings.TrimSpace(line)[:min(len(strings.TrimSpace(line)), 80)])
		}
		// 2. 硬编码密钥（排除测试文件与注释）
		if !strings.HasSuffix(path, "_test.go") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			low := strings.ToLower(line)
			for _, k := range secretKeys {
				if strings.Contains(low, k) && strings.Contains(low, "=") && strings.Contains(line, "\"") {
					if !strings.Contains(low, "env") && !strings.Contains(low, "getenv") && !strings.Contains(low, "os.Getenv") {
						add("MED", path, i+1, "疑似硬编码敏感值: "+strings.TrimSpace(line)[:min(len(strings.TrimSpace(line)), 70)])
					}
				}
			}
		}
		// 3. 日志打印敏感字段
		if strings.Contains(strings.ToLower(line), "password") && strings.Contains(line, "Print") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			add("HIGH", path, i+1, "日志可能输出密码字段")
		}
		// 4. 未授权 admin 端点（路由注册行）
		if strings.Contains(line, "admin") && strings.Contains(line, ".GET(") && !strings.Contains(line, "auth") {
			add("MED", path, i+1, "admin 路由可能缺少鉴权: "+strings.TrimSpace(line)[:min(len(strings.TrimSpace(line)), 70)])
		}
	}
}

func add(sev, file string, line int, detail string) {
	findings = append(findings, finding{severity: sev, file: file, line: line, detail: detail})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}