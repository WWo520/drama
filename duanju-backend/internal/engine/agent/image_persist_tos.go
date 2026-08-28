// TOSImagePersist 火山官方 TOS SDK 实现（minio-go 对 TOS 的 S3 兼容签名存在不兼容，
// 实测 SignatureDoesNotMatch/Forbidden path；官方 SDK 直连验证通过 → 迁移与持久化统一走此实现）。
package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

type TOSImagePersist struct {
	cli        *tos.ClientV2
	bucket     string
	publicBase string // 含 {bucket} 占位符 → 虚拟主机式；否则路径式拼接
	client     *http.Client
}

func NewTOSImagePersist(region, endpoint, ak, sk, bucket, publicBase string) (*TOSImagePersist, error) {
	// endpoint 为空时由 WithRegion 自动补全官方区域端点
	opts := []tos.ClientOption{tos.WithRegion(region), tos.WithCredentials(tos.NewStaticCredentials(ak, sk))}
	// 官方 SDK 只接受 TOS 原生端点：tos-s3-{region}.volces.com(兼容层) 自动转 tos-{region}.volces.com
	tosEp := strings.Replace(strings.TrimPrefix(endpoint, "https://"), "tos-s3-", "tos-", 1)
	cli, err := tos.NewClientV2(tosEp, opts...)
	if err != nil {
		return nil, err
	}
	return &TOSImagePersist{
		cli:        cli,
		bucket:     bucket,
		publicBase: strings.TrimRight(publicBase, "/"),
		client:     &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// EnsurePublicRead 幂等设置桶匿名只读策略（整桶 download；已设置时幂等覆盖无副作用）。
func (t *TOSImagePersist) EnsurePublicRead(ctx context.Context) error {
	policy := fmt.Sprintf(`{"Statement":[{"Effect":"Allow","Principal":["*"],"Action":["tos:GetObject"],"Resource":["tot:%s/*"]}]}`, t.bucket)
	_, err := t.cli.PutBucketPolicyV2(ctx, &tos.PutBucketPolicyV2Input{Bucket: t.bucket, Policy: policy})
	return err
}

// PersistImage HTTP 拉取源图字节 → PutObjectV2 → 固定公共 URL。
func (t *TOSImagePersist) PersistImage(ctx context.Context, srcURL, objectKey string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("拉取源图 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if len(body) < 1024 {
		return "", fmt.Errorf("源图字节数异常过小(%d)", len(body))
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || strings.Contains(contentType, "octet-stream") {
		contentType = "image/png"
	}
	in := &tos.PutObjectV2Input{
		PutObjectBasicInput: tos.PutObjectBasicInput{Bucket: t.bucket, Key: objectKey, ContentType: contentType},
		Content:             bytes.NewReader(body),
	}
	if _, err = t.cli.PutObjectV2(ctx, in); err != nil {
		return "", err
	}
	log.Printf("image-persist-tos: 已转存 %s -> %s", truncateStr(srcURL, 60), objectKey)
	if strings.Contains(t.publicBase, "{bucket}") {
		return strings.ReplaceAll(t.publicBase, "{bucket}", t.bucket) + "/" + objectKey, nil
	}
	return t.publicBase + "/" + t.bucket + "/" + objectKey, nil
}
