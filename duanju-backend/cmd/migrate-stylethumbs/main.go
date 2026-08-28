// migrate-stylethumbs 风格库预览图转存工具（一次性/可重跑）：
// 火山 TOS 签名 URL 仅 24h 有效——genstylethumbs 直写厂商 URL 导致主页风格库图片集体过期失效。
// 本工具把 style_library.thumbnail_url 中的 TOS 图拉取字节转存自有 MinIO，回填固定未签名 URL。
//
// 用法（仓库根目录）：
//
//	go run ./cmd/migrate-stylethumbs
//
// 依赖环境变量：DATABASE_URL、S3_ENDPOINT/S3_ACCESS_KEY/S3_SECRET_KEY/S3_BUCKET、
// S3_PUBLIC_BASE（公共访问基址，须与 worker 一致）、MODEL_KEY_ENC_SECRET（本工具不解密 Key，可不带）。
package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/duanju/backend/internal/engine/agent"
)

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	loadDotEnv(".env")
	dsn := os.Getenv("DATABASE_URL")
	endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	if dsn == "" || endpoint == "" || bucket == "" {
		log.Fatal("缺少 DATABASE_URL / S3_ENDPOINT / S3_BUCKET")
	}
	publicBase := os.Getenv("S3_PUBLIC_BASE")
	if publicBase == "" {
		publicBase = "http://" + endpoint
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, `
		SELECT style_code, thumbnail_url FROM style_library
		WHERE deleted_at IS NULL AND thumbnail_url IS NOT NULL AND thumbnail_url <> ''`)
	if err != nil {
		log.Fatalf("读取风格库失败: %v", err)
	}
	type item struct{ code, url string }
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.code, &it.url); err != nil {
			log.Fatalf("扫描行失败: %v", err)
		}
		items = append(items, it)
	}
	rows.Close()

	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), ""),
		Secure: strings.EqualFold(os.Getenv("S3_USE_SSL"), "true"),
	})
	if err != nil {
		log.Fatalf("MinIO 客户端初始化失败: %v", err)
	}
	persist := agent.NewMinIOImagePersist(mc, bucket, publicBase)
	if err := persist.EnsurePublicRead(ctx); err != nil {
		log.Printf("警告：桶匿名读策略设置失败: %v", err)
	}

	okCount, skipCount, failCount := 0, 0, 0
	for _, it := range items {
		// 已是固定 URL（非 TOS 签名）则跳过，幂等重跑安全
		if !strings.Contains(it.url, "volces.com") {
			skipCount++
			continue
		}
		key := "styles/" + it.code + "/" + it.code + ".png"
		durable, err := persist.PersistImage(ctx, it.url, key)
		if err != nil {
			failCount++
			// TOS 已过期（403/404）：提示需 -force 重新生成
			log.Printf("[失败] %s: %v（若为签名过期请运行 go run ./cmd/genstylethumbs -force 重生成）", it.code, err)
			continue
		}
		if _, uerr := pool.Exec(ctx, `
			UPDATE style_library SET thumbnail_url=$2, updated_at=NOW() WHERE style_code=$1`, it.code, durable); uerr != nil {
			failCount++
			log.Printf("[回填失败] %s: %v", it.code, uerr)
			continue
		}
		okCount++
		log.Printf("[完成] %s -> %s", it.code, durable)
	}
	log.Printf("转存结束：成功 %d，跳过(已固定) %d，失败 %d", okCount, skipCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}
