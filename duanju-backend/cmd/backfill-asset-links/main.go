// 存量项目资产链回填工具（18 - 视频制作 阶段一）：为已有拆分产物但无 nu_asset_links /
// shot_asset_refs 的项目按同一匹配逻辑补链，使视频生成可注入资产参考图。
// 用法：
//
//	backfill-asset-links              # 全部有拆分产物的项目
//	backfill-asset-links <project_id> # 指定项目（数字 ID）
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/pkg/db"
	"github.com/duanju/backend/internal/repository"
)

func main() {
	flag.Parse()
	projectID := int64(0)
	if flag.NArg() > 0 {
		id, err := strconv.ParseInt(flag.Arg(0), 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, "project_id 须为数字")
			os.Exit(1)
		}
		projectID = id
	}
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	pool, err := db.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, "数据库连接失败:", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 幂等：ON CONFLICT DO NOTHING，可重复执行
	n, err := repository.BackfillProjectAssetLinks(ctx, pool, projectID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "回填失败:", err)
		os.Exit(1)
	}
	fmt.Printf("资产链回填完成： %d 个镜头建立参考图引用\n", n)
}
