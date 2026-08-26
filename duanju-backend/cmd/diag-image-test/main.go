// 一次性诊断：用与 AssetHandler 完全相同的提示词/尺寸直接调火山 Seedream，对比线上出图。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/duanju/backend/internal/config"
	"github.com/duanju/backend/internal/engine/model"
	"github.com/duanju/backend/internal/pkg/cryptox"
	"github.com/duanju/backend/internal/pkg/db"
	"github.com/duanju/backend/internal/repository"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	pool, err := db.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	encSecret := os.Getenv("MODEL_KEY_ENC_SECRET")
	if encSecret == "" {
		encSecret = cfg.JWT.Secret
	}
	keyStore := repository.NewModelKeyStore(pool, cryptox.DeriveKey(encSecret), true)
	volcImage := model.NewVolcImageProvider("volcengine", []model.Capability{model.CapT2I, model.CapI2I}, "")
	eng := model.NewEngine([]model.Provider{volcImage}, keyStore)

	// 与 AssetHandler 对 target=172 组装的最终提示词一致（含双句号原样保留）
	prompt := "中国年轻男性，职场新人，瘦削身材；圆脸，戴黑框眼镜，发型是棕色卷发；穿着休闲衬衫和牛仔裤，运动鞋；性格活泼，喜欢八卦，在茶水间与同事低声议论。。写实风格，真实质感，自然光影。生成该主体的全身三视图：正面视图、侧面视图和背面视图。画面布局：图像左侧为主体上半身特写，图像右侧并排展示主体的正面、侧面和背面三个全身视图。纯白色背景，画面仅展示主体上半身特写和三视图。这不是角色设定图纸，禁止绘制身高标注线、尺寸标注线、网格线和参考线，画面中不能出现任何文字、字母、数字和水印"

	resp, err := eng.GenerateImage(ctx, model.CapT2I, &model.ImageRequest{
		Prompt:      prompt,
		AspectRatio: "16:9",
		Resolution:  "1K",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
	fmt.Println("URL:", resp.ImageURL)
	fmt.Println("Model:", resp.ModelUsed)
}
