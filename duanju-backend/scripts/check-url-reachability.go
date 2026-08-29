//go:build ignore

// check-url-reachability.go - 楠岃瘉璧勪骇鍥剧墖 URL 鍙揪鎬?
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable"

	fmt.Println("馃寪 璧勪骇鍥剧墖 URL 鍙揪鎬ч獙璇?)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "鉂?鏁版嵁搴撹繛鎺ュけ璐ワ細%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. 鎶芥牱娴嬭瘯鍚勭被 URL锛堟瘡绫诲彇 3 鏉★級
	fmt.Println("\n銆?銆戝悇绫?URL 鍙揪鎬ф娊鏍锋祴璇?)
	testURLSamples(ctx, db, client, "character_variants", "瑙掕壊鍙樹綋")
	testURLSamples(ctx, db, client, "scenes", "鍦烘櫙")
	testURLSamples(ctx, db, client, "props", "閬撳叿")

	// 2. TOS 绛惧悕 URL 杩囨湡妫€娴?
	fmt.Println("\n銆?銆慣OS 绛惧悕 URL 杩囨湡妫€娴嬶紙瑙掕壊鍙樹綋锛?)
	checkTOSExpiry(ctx, db, client, "character_variants")

	// 3. 妫€鏌?MinIO 鏈嶅姟鏄惁杩愯
	fmt.Println("\n銆?銆戞湰鍦?MinIO 鏈嶅姟妫€鏌?)
	testURL(client, "http://127.0.0.1:9000/minio/health/live", "MinIO 鍋ュ悍妫€鏌?)

	fmt.Println("\n鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}

func testURLSamples(ctx context.Context, db *sql.DB, client *http.Client, table, label string) {
	// 鎸?URL 绫诲埆鍚勫彇 1-2 鏉?
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (url_type) url, url_type FROM (
			SELECT main_image_url AS url,
				CASE
					WHEN main_image_url LIKE '%%volces.com%%' AND main_image_url LIKE '%%X-Tos-Expires%%' THEN 'TOS绛惧悕'
					WHEN main_image_url LIKE '%%asset-placeholder%%' THEN 'mock'
					WHEN main_image_url LIKE '%%127.0.0.1%%' OR main_image_url LIKE '%%localhost%%' OR main_image_url LIKE '%%minio%%' THEN '鏈湴MinIO'
					WHEN COALESCE(main_image_url,'') = '' THEN '绌?
					ELSE '鍥哄畾URL'
				END AS url_type
			FROM %s WHERE deleted_at IS NULL
		) t WHERE url <> '' ORDER BY url_type LIMIT 8`, table))
	if err != nil {
		fmt.Printf("  鉂?%s 鏌ヨ澶辫触锛?v\n", label, err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var url, urlType string
		rows.Scan(&url, &urlType)
		count++
		if count > 8 {
			break
		}
		fmt.Printf("  [%s] %s\n", label, urlType)
		short := url
		if len(short) > 70 {
			short = short[:70] + "..."
		}
		fmt.Printf("    %s\n", short)
		testURL(client, url, "  鈫?)
	}
}

func checkTOSExpiry(ctx context.Context, db *sql.DB, client *http.Client, table string) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT main_image_url FROM %s
		WHERE deleted_at IS NULL AND main_image_url LIKE '%%X-Tos-Expires%%'
		LIMIT 5`, table))
	if err != nil {
		fmt.Printf("  鉂?鏌ヨ澶辫触锛?v\n", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var url string
		rows.Scan(&url)
		count++
		// 瑙ｆ瀽 X-Tos-Date 鍜?X-Tos-Expires
		dateStr, expiresStr := extractParam(url, "X-Tos-Date"), extractParam(url, "X-Tos-Expires")
		expiry := "?"
		if dateStr != "" && expiresStr != "" {
			t, err := time.Parse("20060102T150405Z", dateStr)
			if err == nil {
				var exp int
				fmt.Sscanf(expiresStr, "%d", &exp)
				if exp == 0 {
					exp = 86400
				}
				expiry = t.Add(time.Duration(exp) * time.Second).Format("2006-01-02 15:04:05")
			}
		}
		now := time.Now()
		expired := ""
		if expiry != "?" {
			if t2, err := time.Parse("2006-01-02 15:04:05", expiry); err == nil && now.After(t2) {
				expired = " 鈿狅笍 宸茶繃鏈?"
			}
		}
		fmt.Printf("  绛惧悕 URL #%d 杩囨湡鏃堕棿=%s%s\n", count, expiry, expired)
	}
}

func extractParam(url, key string) string {
	idx := indexOf(url, key+"=")
	if idx < 0 {
		return ""
	}
	start := idx + len(key) + 1
	end := start
	for end < len(url) && url[end] != '&' {
		end++
	}
	return url[start:end]
}

func testURL(client *http.Client, url, prefix string) {
	if url == "" {
		return
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Printf("%s 鉂?璇锋眰鏋勯€犲け璐ワ細%v\n", prefix, err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s 鉂?涓嶅彲杈撅細%v\n", prefix, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		ct := resp.Header.Get("Content-Type")
		fmt.Printf("%s 鉁?鍙揪 (HTTP 200, %s)\n", prefix, ct)
	} else {
		fmt.Printf("%s 鈿狅笍 HTTP %d\n", prefix, resp.StatusCode)
	}
}

func contains(s, sub string) bool {
	return indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ = os.Getenv

