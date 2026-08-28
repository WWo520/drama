// diag-tos 最小连通性测试：向上传一个测试对象并列出，验证 IMGSTORE_* 凭据与端点。
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	ep := env("IMGSTORE_ENDPOINT", "tos-s3-cn-beijing.volces.com")
	bk := os.Getenv("IMGSTORE_BUCKET")
	ak := env("IMGSTORE_ACCESS_KEY", "")
	sk := env("IMGSTORE_SECRET_KEY", "")
	insecure := os.Getenv("IMGSTORE_INSECURE") == "true"

	mc, err := minio.New(ep, &minio.Options{Creds: credentials.NewStaticV4(ak, sk, ""), Secure: !insecure, Region: "cn-beijing", BucketLookup: minio.BucketLookupDNS})
	if err != nil {
		fmt.Println("client:", err)
		return
	}
	ctx := context.Background()
	if bs, berr := mc.ListBuckets(ctx); berr == nil {
		for _, b := range bs {
			fmt.Println("bucket:", b.Name)
		}
	} else {
		fmt.Println("ListBuckets err:", berr)
	}
	ok, err := mc.BucketExists(ctx, bk)
	fmt.Printf("BucketExists(%s)=%v err=%v\n", bk, ok, err)

	// ListObjects 探测凭据是否有效（无效→SignatureDoesNotMatch/InvalidAccessKey）
	names := []string{}
	for obj := range mc.ListObjects(ctx, bk, minio.ListObjectsOptions{MaxKeys: 3}) {
		if obj.Err != nil {
			fmt.Println("ListObjects err:", obj.Err)
			break
		}
		names = append(names, obj.Key)
	}
	fmt.Printf("ListObjects: %d keys %v\n", len(names), names)

	body := strings.NewReader("tos-connectivity-test")
	key := fmt.Sprintf("test/%d.txt", time.Now().UnixMilli())
	up, uerr := mc.PutObject(ctx, bk, key, body, int64(body.Len()), minio.PutObjectOptions{})
	fmt.Printf("PutObject ok=%v err=%v\n", up.Size > 0 || uerr == nil, uerr)
	if uerr == nil {
		url := fmt.Sprintf("https://%s.%s/%s", bk, ep, key)
		fmt.Println("public url:", url)
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
