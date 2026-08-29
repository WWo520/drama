//go:build ignore

// verify-video-persist.go - 一次性验证视频转存链路：
// 指定厂商视频 URL → TOSImagePersist.PersistVideo 转存自有桶 → HEAD 回读确认 200。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/duanju/backend/internal/engine/agent"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run scripts/verify-video-persist.go <src-video-url>")
		os.Exit(1)
	}
	src := os.Args[1]
	p, err := agent.NewTOSImagePersist(
		envOr("IMGSTORE_REGION", "cn-beijing"),
		envOr("IMGSTORE_ENDPOINT", ""),
		envOr("IMGSTORE_ACCESS_KEY", ""),
		envOr("IMGSTORE_SECRET_KEY", ""),
		envOr("IMGSTORE_BUCKET", ""),
		envOr("IMGSTORE_PUBLIC_BASE", ""),
	)
	if err != nil {
		fmt.Println("❌ TOS 客户端初始化失败:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	key := fmt.Sprintf("spaces/0/videos/verify/%d.mp4", time.Now().UnixMilli())
	durable, err := p.PersistVideo(ctx, src, key)
	if err != nil {
		fmt.Println("❌ 转存失败:", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已转存:", durable)
	resp, err := http.Head(durable)
	if err != nil {
		fmt.Println("❌ 回读失败:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	fmt.Printf("✅ 回读 HTTP %d, Content-Type=%s, Content-Length=%.1fMB\n",
		resp.StatusCode, resp.Header.Get("Content-Type"), float64(resp.ContentLength)/1024/1024)
	if resp.StatusCode != 200 {
		os.Exit(1)
	}
}

func envOr(k, fb string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fb
}
