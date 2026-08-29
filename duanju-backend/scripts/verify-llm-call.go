//go:build ignore

// verify-llm-call.go - 瑙ｅ瘑 Key[2] 骞剁湡瀹炶皟鐢?DeepSeek API 楠岃瘉杩為€氭€?
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/duanju/backend/internal/pkg/cryptox"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("馃攳 楠岃瘉 DeepSeek LLM 璋冪敤杩為€氭€?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 妯℃嫙 worker 鐨勫瘑閽ヨВ鏋愶細鏃?MODEL_KEY_ENC_SECRET 鈫?鍥為€€ JWT_SECRET
	encSecret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if encSecret == "" {
		encSecret = "dev-only-secret-change-in-prod"
		fmt.Println("鈩癸笍  浣跨敤 JWT_SECRET (涓庡綋鍓?worker 涓€鑷?")
	}
	encKey := cryptox.DeriveKey(encSecret)

	// 璇诲彇鍚敤涓殑 deepseek Key锛圞ey[2]锛?
	var encrypted string
	var prefix string
	err = db.QueryRowContext(ctx, `
		SELECT mk.key_encrypted, mk.key_prefix
		FROM model_keys mk
		JOIN model_providers mp ON mp.id = mk.provider_id
		WHERE mp.name = 'deepseek' AND mk.enabled = TRUE AND mk.deleted_at IS NULL
		ORDER BY mk.priority DESC, mk.id ASC
		LIMIT 1
	`).Scan(&encrypted, &prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏌ヨ鍚敤 Key 澶辫触锛?v\n", err)
		os.Exit(1)
	}

	apiKey, err := cryptox.Decrypt(encrypted, encKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?瑙ｅ瘑澶辫触锛?v\n", err)
		os.Exit(1)
	}
	fmt.Printf("鉁?瑙ｅ瘑鎴愬姛 鈫?%s...%s\n", truncate(apiKey, 10), apiKey[len(apiKey)-4:])

	// 鐪熷疄璋冪敤 DeepSeek API锛堟渶灏忚姹傞獙璇侊級
	body := map[string]any{
		"model":    "deepseek-v4-flash",
		"messages": []map[string]string{{"role": "user", "content": "鍥炲'OK'涓や釜瀛?}},
		"max_tokens": 16,
		"thinking": map[string]string{"type": "disabled"}, // 鍏抽棴鎬濊€?
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.deepseek.com/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?璇锋眰鏋勯€犲け璐ワ細%v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?API 璋冪敤澶辫触锛?v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		fmt.Printf("鉂?API 杩斿洖 %d: %s\n", resp.StatusCode, truncate(string(raw), 200))
		os.Exit(1)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		fmt.Printf("鉂?瑙ｆ瀽鍝嶅簲澶辫触锛?v\n%s\n", err, truncate(string(raw), 300))
		os.Exit(1)
	}
	if len(out.Choices) == 0 {
		fmt.Println("鉂?鏃?choices 杩斿洖")
		os.Exit(1)
	}

	fmt.Printf("鉁?DeepSeek API 璋冪敤鎴愬姛锛乗n")
	fmt.Printf("   鍝嶅簲鍐呭锛?s\n", truncate(out.Choices[0].Message.Content, 50))
	fmt.Printf("   鐢ㄩ噺锛歱rompt=%d tokens, completion=%d tokens\n", out.Usage.PromptTokens, out.Usage.CompletionTokens)
	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("馃帀 LLM 閾捐矾宸叉仮澶嶏紝鍓ф湰鎷嗗垎鍙互姝ｅ父鎵ц锛?)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

