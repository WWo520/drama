// mock 产物基址：验收阶段 mock 视频/成片指向可播放资源（前端 public/sample.mp4）。
package agent

import "os"

// mockCDNBase 返回 mock 产物基址（环境变量 MOCK_CDN_BASE 覆盖，默认指向前端 dev server）。
func mockCDNBase() string {
	if base := os.Getenv("MOCK_CDN_BASE"); base != "" {
		return base
	}
	return "http://localhost:5174"
}
