// migrate-images 存量图片回填：把库里已有的资产图/参考图 URL 统一转存到 IMGSTORE_*
// 指定的火山 TOS 桶，并把引用位置刷新为固定公共 URL。
// 幂等：已是 TOS 域名的跳过；单张失败记录不阻断整体。
// 用法（需先设置 IMGSTORE_* 与 DATABASE_URL）：
//
//	go run ./cmd/migrate-images
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duanju/backend/internal/engine/agent"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func short(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 90 {
		s = s[:90]
	}
	return s
}

func main() {
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "postgres://duanju:duanju_dev_pwd@127.0.0.1:5432/duanju?sslmode=disable")
	}
	ep := envOr("IMGSTORE_ENDPOINT", "tos-s3-cn-beijing.volces.com")
	region := envOr("IMGSTORE_REGION", "cn-beijing")
	bk := envOr("IMGSTORE_BUCKET", "")
	ak := envOr("IMGSTORE_ACCESS_KEY", "")
	sk := envOr("IMGSTORE_SECRET_KEY", "")
	pb := envOr("IMGSTORE_PUBLIC_BASE", fmt.Sprintf("https://{bucket}.tos-%s.volces.com", region))

	if bk == "" || ak == "" || sk == "" {
		fmt.Fprintln(os.Stderr, "缺少 IMGSTORE_BUCKET / IMGSTORE_ACCESS_KEY / IMGSTORE_SECRET_KEY")
		os.Exit(1)
	}

	mp, merr := agent.NewTOSImagePersist(region, ep, ak, sk, bk, pb)
	if merr != nil {
		fmt.Fprintln(os.Stderr, "TOS 客户端初始化失败:", merr)
		os.Exit(1)
	}
	ctx := context.Background()
	if perr := mp.EnsurePublicRead(ctx); perr != nil {
		fmt.Printf("(提示) 桶公共读策略设置未成功(%s)，请到火山控制台确认桶为公共读，否则远端拉不到\n", short(perr))
	}

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	kindByTable := map[string]string{
		"character_variants": "character_variant",
		"scenes":             "scene",
		"props":              "prop",
		"shots":              "shot_ref",
	}

	migrate := func(table, urlCol string) {
		kind := kindByTable[table]
		rows, qerr := pool.Query(ctx, fmt.Sprintf(
			`SELECT id, %s FROM %s WHERE COALESCE(%s,'') <> ''
			    AND %s NOT LIKE 'https://%%volces.com%%' AND deleted_at IS NULL`,
			urlCol, table, urlCol, urlCol))
		if qerr != nil {
			fmt.Printf("%s: query err %v\n", table, qerr)
			return
		}
		type item struct {
			id int64
			u  string
		}
		items := []item{}
		for rows.Next() {
			var it item
			_ = rows.Scan(&it.id, &it.u)
			items = append(items, it)
		}
		rows.Close()

		var ok, fail int
		for _, it := range items {
			key := fmt.Sprintf("spaces/migrated/assets/%s-%d/%d.png", kind, it.id, time.Now().UnixMilli())
			nurl, perr := mp.PersistImage(ctx, it.u, key)
			if perr != nil {
				fail++
				fmt.Printf("  FAIL %s id=%d src=%s: %s\n", table, it.id, short(fmt.Errorf("%s", it.u)), short(perr))
				continue
			}
			if _, uerr := pool.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$2 WHERE id=$1`, table, urlCol), it.id, nurl); uerr != nil {
				fail++
				continue
			}
			ok++
		}
		fmt.Printf("%s: migrated=%d failed=%d (total scanned=%d)\n", table, ok, fail, len(items))
	}

	migrate("character_variants", "main_image_url")
	migrate("scenes", "main_image_url")
	migrate("props", "main_image_url")
	migrate("shots", "reference_image_url")

	// generated_images 历史快照（限最近 500 条，非关键路径）
	grows, gerr := pool.Query(ctx, `
		SELECT id, original_url FROM generated_images
		WHERE state='SUCCESS' AND COALESCE(original_url,'') <> ''
		  AND original_url NOT LIKE 'https://%volces.com%'
		ORDER BY id DESC LIMIT 500`)
	if gerr == nil {
		type gi struct {
			id int64
			u  string
		}
		list := []gi{}
		for grows.Next() {
			var g gi
			_ = grows.Scan(&g.id, &g.u)
			list = append(list, g)
		}
		grows.Close()
		var ok, fail int
		for _, g := range list {
			key := fmt.Sprintf("archive/generated_images/%d/%d.png", g.id, time.Now().UnixMilli())
			nurl, perr := mp.PersistImage(ctx, g.u, key)
			if perr != nil {
				fail++
				continue
			}
			if _, uerr := pool.Exec(ctx, `UPDATE generated_images SET original_url=$2 WHERE id=$1`, g.id, nurl); uerr == nil {
				ok++
			} else {
				fail++
			}
		}
		fmt.Printf("generated_images: migrated=%d failed=%d (scanned=%d)\n", ok, fail, len(list))
	}

	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("DONE — 库内图片引用已统一切换到你的 TOS 桶固定 URL（源图拉取失败的条目保持原样，可重跑本工具补齐）")
}
