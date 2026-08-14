// S1-Q04 并发压测：10 并发 × 核心端点，统计 P50/P95 延迟与错误率。
// 用法：go run ./cmd/loadtest -concurrency 10 -requests 200
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

const base = "http://127.0.0.1:8080"

type result struct {
	latency time.Duration
	ok      bool
}

func main() {
	concurrency := flag.Int("concurrency", 10, "并发数")
	requests := flag.Int("requests", 200, "总请求数")
	flag.Parse()

	// 登录获取 token
	loginBody, _ := json.Marshal(map[string]string{"email": "s3e2e@test.com", "password": "Passw0rd123"})
	resp, err := http.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		fmt.Println("登录失败:", err)
		os.Exit(1)
	}
	var login map[string]any
	json.NewDecoder(resp.Body).Decode(&login)
	resp.Body.Close()
	token := login["data"].(map[string]any)["access_token"].(string)

	// 压测端点（GET 只读为主，避免数据污染）
	endpoints := []string{"/api/v1/points", "/api/v1/tasks", "/api/v1/works", "/api/v1/templates", "/api/v1/notifications"}

	var mu sync.Mutex
	var results []result
	var wg sync.WaitGroup
	sem := make(chan struct{}, *concurrency)
	perReq := *requests / len(endpoints)

	for _, ep := range endpoints {
		for i := 0; i < perReq; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(path string) {
				defer wg.Done()
				defer func() { <-sem }()
				start := time.Now()
				req, _ := http.NewRequest("GET", base+path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				r, err := http.DefaultClient.Do(req)
				ok := err == nil && (r.StatusCode == 200 || r.StatusCode == 429)
				if r != nil {
					r.Body.Close()
				}
				mu.Lock()
				results = append(results, result{latency: time.Since(start), ok: ok})
				mu.Unlock()
			}(ep)
		}
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].latency < results[j].latency })
	total := len(results)
	errCount := 0
	var sum time.Duration
	for _, r := range results {
		sum += r.latency
		if !r.ok {
			errCount++
		}
	}
	p50 := results[total/2].latency
	p95 := results[total*95/100].latency
	fmt.Printf("压测完成：并发=%d 请求=%d 错误=%d 错误率=%.2f%%\n", *concurrency, total, errCount, float64(errCount)/float64(total)*100)
	fmt.Printf("P50=%v P95=%v AVG=%v（429 限流响应计入非错误）\n", p50, p95, sum/time.Duration(total))
	if errCount > 0 || p95 > 500*time.Millisecond {
		fmt.Println("❌ S1-Q04 压测未达标（错误率>0 或 P95>500ms）")
		os.Exit(1)
	}
	fmt.Println("✅ S1-Q04 压测通过：无错误且 P95 < 500ms")
}