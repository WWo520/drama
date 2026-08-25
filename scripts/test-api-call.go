// test-api-call.go - 测试直接调用 DeepSeek API
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	fmt.Println("🔍 测试 DeepSeek V4 Flash API")
	fmt.Println("═══════════════════════════════════════════")
	
	ctx := context.Background()
	
	// 从环境变量或数据库获取配置
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("⚠️  警告：未设置 DEEPSEEK_API_KEY 环境变量")
		fmt.Println("💡 建议：使用数据库查询工具查看 API Key")
		os.Exit(1)
	}
	
	fmt.Printf("BaseURL: %s\n", baseURL)
	fmt.Printf("API Key: %s...%s\n", apiKey[:8], apiKey[len(apiKey)-4:])
	fmt.Println()
	
	// 准备请求
	prompt := `请拆分以下剧本为剧集和叙事单元：

场景一：城市街道，夜晚
主角站在雨中，望着远处的霓虹灯。
他深吸一口气，转身离开。

场景二：咖啡馆内
主角坐在角落，面前放着一杯咖啡。
电话响起，他接起。
“喂，我是谢无敌。”
`
	
	messages := []map[string]string{
		{"role": "system", "content": "你是一个专业的剧本拆分助手。请将剧本拆分为剧集和叙事单元，每个叙事单元必须逐字摘录原文，不得改写或摘要。"},
		{"role": "user", "content": prompt},
	}
	
	body := map[string]interface{}{
		"model":       "deepseek-v4-flash",
		"messages":    messages,
		"temperature": 0.3,
		"max_tokens":  16384,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	
	payload, _ := json.Marshal(body)
	
	endpoint := baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 创建请求失败：%v\n", err)
		os.Exit(1)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ HTTP 请求失败：%v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	
	raw, _ := io.ReadAll(resp.Body)
	
	fmt.Printf("HTTP Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("Response Body:\n%s\n", string(raw))
	
	if resp.StatusCode >= 400 {
		var errorResp map[string]interface{}
		if err := json.Unmarshal(raw, &errorResp); err == nil {
			fmt.Printf("\nError Details:\n")
			if msg, ok := errorResp["message"].(string); ok {
				fmt.Printf("  Message: %s\n", msg)
			}
		}
	} else {
		// 解析成功响应
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &result); err == nil {
			if len(result.Choices) > 0 && result.Choices[0].Message.Content != "" {
				content := result.Choices[0].Message.Content
				if len(content) > 500 {
					content = content[:500] + "..."
				}
				fmt.Printf("\n✅ API 调用成功!\n")
				fmt.Printf("内容预览 (前 500 字):\n%s\n", content)
			}
		}
	}
}
