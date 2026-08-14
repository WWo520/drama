// 契约校验（B08）：api-client 声明路径 ⊆ 后端注册路由（05 接口文档为契约源，回归时人工对照）。
// 非零退出 = 契约不一致，CI 门禁。
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	// 1. 收集后端注册路由
	backendRoutes := collectRoutes("internal/router/router.go", "internal/domain/creation/handler.go", "internal/domain/space/handler.go")
	// 2. 收集 api-client 声明的端点
	clientPaths := collectClientPaths("../packages/api-client/src/api")

	missing := []string{}
	for _, p := range clientPaths {
		if !routeExists(backendRoutes, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		fmt.Printf("契约不一致：api-client 声明但后端未注册 %d 个端点：\n", len(missing))
		for _, m := range missing {
			fmt.Println("  -", m)
		}
		os.Exit(1)
	}
	fmt.Printf("契约校验通过：api-client %d 个端点全部匹配后端 %d 条路由\n", len(clientPaths), len(backendRoutes))
}

func collectRoutes(paths ...string) []string {
	re := regexp.MustCompile(`\.(GET|POST|PUT|PATCH|DELETE)\("([^"]*)"`)
	groupRe := regexp.MustCompile(`(\w+) := (?:rg|\w+)\.Group\("([^"]+)"`)
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Println("读取路由文件失败:", path, err)
			os.Exit(1)
		}
		content := string(b)
		// 1. 组前缀映射：varName → prefix
		prefix := map[string]string{}
		for _, g := range groupRe.FindAllStringSubmatch(content, -1) {
			prefix[g[1]] = g[2]
		}
		// 2. 展开注册：支持 组.POST("/x") 与 子组 := 组.Group("")
		subGroupRe := regexp.MustCompile(`(\w+) := (\w+)\.Group\("([^"]*)"`)
		for _, g := range subGroupRe.FindAllStringSubmatch(content, -1) {
			base := prefix[g[2]]
			if base != "" {
				prefix[g[1]] = base + g[3]
			}
		}
		for _, mi := range re.FindAllStringSubmatchIndex(content, -1) {
			method := content[mi[2]:mi[3]]
			rel := content[mi[4]:mi[5]]
			// 找注册调用行变量（基于匹配绝对位置）
			if v := findGroupVarAt(content, mi[0]); v != "" && prefix[v] != "" {
				rel = prefix[v] + rel
			}
			key := method + " " + rel
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}

// findGroupVarAt 找到注册调用所在行的接收者变量（基于绝对位置）。
func findGroupVarAt(content string, idx int) string {
	lineStart := strings.LastIndex(content[:idx], "\n") + 1
	line := content[lineStart:idx]
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSuffix(parts[0], ".")
}

func collectClientPaths(dir string) []string {
	// 扫描 api 目录所有 .ts 文件中的 api.xxx("/path") 字面量（兼容嵌套泛型）
	re := regexp.MustCompile(`api\.(get|post|put|patch|delete)[^(]*\(\s*"([^"]+)"`)
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("读取 api-client 失败:", err)
		os.Exit(1)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".ts") {
			continue
		}
		b, _ := os.ReadFile(dir + "/" + e.Name())
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			p := m[2]
			if strings.Contains(p, "${") {
				continue // 模板变量路径跳过
			}
			// 归一化 :param
			norm := regexp.MustCompile(`:[^/]+`).ReplaceAllString(p, ":id")
			out = append(out, "/api/v1"+norm)
		}
	}
	return out
}

func routeExists(routes []string, clientPath string) bool {
	// 客户端路径带 /api/v1 前缀，后端注册的是 /api/v1 之后的部分
	rel := strings.TrimPrefix(clientPath, "/api/v1")
	normRel := regexp.MustCompile(`:[^/]+`).ReplaceAllString(rel, ":id")
	for _, r := range routes {
		parts := strings.SplitN(r, " ", 2)
		if len(parts) != 2 {
			continue
		}
		method, path := parts[0], parts[1]
		normPath := regexp.MustCompile(`:[^/]+`).ReplaceAllString(path, ":id")
		normPath = strings.TrimPrefix(normPath, "/api/v1")
		normPath = strings.TrimPrefix(normPath, "/api/v1")
		if normPath == normRel {
			_ = method
			return true
		}
	}
	return false
}