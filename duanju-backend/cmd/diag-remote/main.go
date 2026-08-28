// diag-remote 查询在跑视频任务的 AutoDL 远端状态分布（验证平台实际并行度）：
// 统计 external_task_id 在远端处于 QUEUED / RUNNING / SUCCESS 的数量与已渲时长。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/pkg/cryptox"
)

type remoteResp struct {
	Code string `json:"code"`
	Data struct {
		TaskID    string `json:"task_id"`
		Status    string `json:"status"`
		StartedAt string `json:"started_at"`
		Duration  int    `json:"duration"`
	} `json:"data"`
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"
	}
	secret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if secret == "" {
		secret = "prod-model-key-enc-secret-change-in-production-at-least-32-chars-long" // 开发默认回退
	}
	pool, _ := pgxpool.New(context.Background(), dsn)
	defer pool.Close()

	// 取 autodl-comfy 的 Key：解密 Token + base_url（取 mp.base_url）
	var enc, pbase string
	err := pool.QueryRow(context.Background(), `
		SELECT mk.key_encrypted, COALESCE(mp.base_url,'')
		FROM model_keys mk JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name='autodl-comfy' AND mk.enabled AND mk.deleted_at IS NULL
		ORDER BY mk.id LIMIT 1`).Scan(&enc, &pbase)
	if err != nil {
		fmt.Println("取 h3 key 失败:", err)
		return
	}
	token, derr := cryptox.Decrypt(enc, cryptox.DeriveKey(secret))
	if derr != nil || strings.TrimSpace(token) == "" {
		fmt.Println("token 解密失败:", derr)
		return
	}

	// 取本地 RUNNING 视频任务的 external_task_id（result_output_json）
	rows, rerr := pool.Query(context.Background(), `
		SELECT result_output_json::text FROM generation_tasks
		WHERE task_type='VIDEO_SEGMENT_GEN' AND status='RUNNING'
		ORDER BY id DESC LIMIT 30`)
	if rerr != nil {
		fmt.Println(rerr)
		return
	}
	var extIDs []string
	for rows.Next() {
		var raw string
		_ = rows.Scan(&raw)
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			if v, ok := m["external_task_id"].(string); ok && v != "" {
				extIDs = append(extIDs, v)
			}
		}
	}
	rows.Close()
	fmt.Printf("本地 RUNNING 视频任务: %d 个，逐个查询远端真实状态…\n\n", len(extIDs))

	client := &http.Client{Timeout: 15 * time.Second}
	counts := map[string]int{}
	for _, id := range extIDs {
		url := fmt.Sprintf("https://autodl.art/api/v1/comfyui/comfyui_workflow/result/%s", id)
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
		resp, herr := client.Do(req)
		if herr != nil {
			fmt.Printf("%s 请求失败: %v\n", short(id), herr)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var rr remoteResp
		status := "HTTP_" + fmt.Sprint(resp.StatusCode)
		started, dur := "", 0
		if json.Unmarshal(body, &rr) == nil && rr.Data.Status != "" {
			status = rr.Data.Status
			started = rr.Data.StartedAt
			dur = rr.Data.Duration
		}
		counts[status]++
		fmt.Printf("ext=%s  remote_status=%-8s started_at=%s 已渲=%ds\n", short(id), status, started, dur)
	}
	fmt.Println("\n远端状态分布:")
	for st, n := range counts {
		fmt.Printf("  %s: %d\n", st, n)
	}
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12] + "…"
	}
	return s
}
