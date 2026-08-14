// 接口覆盖率核对：05 接口文档（OpenAPI YAML）路径 vs 后端注册路由。
// 输出：文档路径数 / 后端路由数 / 未实现清单。
package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

func main() {
	// 1. 提取接口文档路径（格式：/xxx/yyy: 或 /xxx/{id}:）
	doc, err := os.ReadFile("../docs/01 - 设计方案/05 - 接口文档.md")
	if err != nil {
		fmt.Println("读取接口文档失败:", err)
		os.Exit(1)
	}
	docPaths := map[string]bool{}
	re := regexp.MustCompile(`(?m)^\s*(/\S+):\s*$`)
	for _, m := range re.FindAllStringSubmatch(string(doc), -1) {
		p := normalize(m[1])
		if strings.HasPrefix(p, "/api/") {
			p = strings.TrimPrefix(p, "/api/v1")
		}
		docPaths[p] = true
	}

	// 2. 提取后端路由（router.go + creation/space handler）
	backend := map[string]bool{}
	for _, f := range []string{"internal/router/router.go", "internal/domain/creation/handler.go", "internal/domain/space/handler.go", "internal/engine/billing/payment_handler.go", "internal/domain/auth/handler.go", "internal/engine/notify/notify_handler.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(b)
		groupRe := regexp.MustCompile(`(\w+) := (?:rg|\w+)\.Group\("([^"]+)"`)
		prefix := map[string]string{}
		for _, g := range groupRe.FindAllStringSubmatch(content, -1) {
			prefix[g[1]] = g[2]
		}
		subRe := regexp.MustCompile(`(\w+) := (\w+)\.Group\("([^"]*)"`)
		for _, g := range subRe.FindAllStringSubmatch(content, -1) {
			if base := prefix[g[2]]; base != "" {
				prefix[g[1]] = base + g[3]
			}
		}
		routeRe := regexp.MustCompile(`\.(GET|POST|PUT|PATCH|DELETE)\("([^"]*)"`)
		for _, mi := range routeRe.FindAllStringSubmatchIndex(content, -1) {
			rel := content[mi[4]:mi[5]]
			lineStart := strings.LastIndex(content[:mi[0]], "\n") + 1
			line := content[lineStart:mi[0]]
			parts := strings.Fields(line)
			var v string
			if len(parts) > 0 {
				v = strings.TrimSuffix(parts[0], ".")
			}
			if p := prefix[v]; p != "" {
				rel = p + rel
			}
			backend[normalize(rel)] = true
		}
	}

	// 3. 对比
	missing := []string{}
	for p := range docPaths {
		if !backend[p] {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	fmt.Printf("接口文档端点: %d | 后端路由: %d | 未实现: %d\n", len(docPaths), len(backend), len(missing))
	for _, m := range missing {
		fmt.Println("  MISS:", m)
	}
}

func normalize(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/api/v1")
	// {id} 归一化
	re := regexp.MustCompile(`\{[^}]+\}`)
	p = re.ReplaceAllString(p, ":id")
	re2 := regexp.MustCompile(`:[a-zA-Z_]+`)
	p = re2.ReplaceAllString(p, ":id")
	// 清理查询串
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return p
}