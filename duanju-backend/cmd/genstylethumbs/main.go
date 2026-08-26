// genstylethumbs 风格库预览图生成工具（一次性/可重跑）：
// 读取 style_library 27 项风格 → 用统一人像模板 + 风格后缀（prompt_suffix_static）
// 调火山 Seedream 生成预览图 → 回填 style_library.thumbnail_url。
//
// 用法（仓库根目录）：
//
//	go run ./cmd/genstylethumbs            # 仅生成 thumbnail_url 为空的风格
//	go run ./cmd/genstylethumbs -force     # 全部重新生成
//
// 依赖环境变量：DATABASE_URL、MODEL_KEY_ENC_SECRET（从仓库根 .env 读取，也可直接注入）。
package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/pkg/cryptox"
	"github.com/duanju/backend/internal/repository"
)

// previewSubject 统一预览主体：固定人像保证 27 项风格可直接横向对比差异。
const previewSubject = "中国古风年轻女性半身肖像，精致面容，发间银簪点缀，肩颈线条优雅，电影感构图"

const fallbackSuffix = "写实风格，真实质感，自然光影"

// loadDotEnv 解析仓库根 .env（KEY=VALUE，忽略注释与空行）；已存在的环境变量不覆盖。
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
	force := flag.Bool("force", false, "全部重新生成（含已有预览图的风格）")
	flag.Parse()

	loadDotEnv(".env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("缺少 DATABASE_URL")
	}
	encSecret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if encSecret == "" {
		encSecret = os.Getenv("JWT_SECRET") // 开发期回退（与 worker 一致）
	}
	if encSecret == "" {
		log.Fatal("缺少 MODEL_KEY_ENC_SECRET")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer conn.Close(ctx)

	// 读取待生成风格
	where := "thumbnail_url IS NULL OR thumbnail_url=''"
	if *force {
		where = "TRUE"
	}
	rows, err := conn.Query(ctx, `
		SELECT style_code, name, COALESCE(prompt_suffix_static,'')
		FROM style_library
		WHERE enabled=TRUE AND deleted_at IS NULL AND (`+where+`)
		ORDER BY sort_order`)
	if err != nil {
		log.Fatalf("读取风格库失败: %v", err)
	}
	type styleItem struct{ code, name, suffix string }
	items := make([]styleItem, 0)
	for rows.Next() {
		var it styleItem
		if err := rows.Scan(&it.code, &it.name, &it.suffix); err != nil {
			log.Fatalf("扫描风格行失败: %v", err)
		}
		items = append(items, it)
	}
	rows.Close()
	if len(items) == 0 {
		log.Println("无需生成：所有风格均已有预览图")
		return
	}
	log.Printf("待生成风格 %d 项", len(items))

	// 取火山 T2I Key（pool 适配 keystore 接口）
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()
	ks := repository.NewModelKeyStore(pool, cryptox.DeriveKey(encSecret), false)
	keys, err := ks.AvailableKeys(ctx, model.CapT2I, "")
	if err != nil {
		log.Fatalf("读取 T2I 模型 Key 失败: %v", err)
	}
	if len(keys) == 0 {
		log.Fatal("无可用 T2I Key（请在管理端配置厂商/模型/Key）")
	}
	k := keys[0]
	log.Printf("使用厂商=%s 模型=%s base=%s", k.ProviderName, k.ModelKey, k.BaseURL)

	provider := model.NewVolcImageProvider("genstylethumbs", []model.Capability{model.CapT2I}, "")

	okCount, failCount := 0, 0
	for _, it := range items {
		suffix := it.suffix
		if suffix == "" {
			suffix = fallbackSuffix
		}
		prompt := previewSubject + "。" + suffix + "。画面中不能出现任何文字、字母、数字和水印"
		url := ""
		var lastErr error
		for attempt := 0; attempt < 2 && url == ""; attempt++ {
			if attempt > 0 {
				time.Sleep(3 * time.Second)
			}
			resp, gerr := provider.GenerateImage(ctx, k, &model.ImageRequest{
				Prompt: prompt, AspectRatio: "1:1", Resolution: "1K", ModelKey: "",
			})
			if gerr == nil && resp.ImageURL != "" {
				url = resp.ImageURL
			} else {
				lastErr = gerr
			}
		}
		if url == "" {
			failCount++
			log.Printf("  [失败] %s（%s）: %v", it.name, it.code, lastErr)
			continue
		}
		if _, uerr := conn.Exec(ctx, `
			UPDATE style_library SET thumbnail_url=$2, updated_at=NOW()
			WHERE style_code=$1 AND deleted_at IS NULL`, it.code, url); uerr != nil {
			failCount++
			log.Printf("  [回填失败] %s: %v", it.name, uerr)
			continue
		}
		okCount++
		log.Printf("  [完成 %d/%d] %s → %s", okCount, len(items), it.name, truncate(url, 80))
	}
	log.Printf("生成结束：成功 %d，失败 %d", okCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
