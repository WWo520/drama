// Package asset 资产引擎（E3）：S3 预签名直传/下载/生命周期（对齐 04 I.1~I.2 / 05 G 章）。
package asset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// 预签名有效期（05 G 章铁律：PUT 15min / GET 24h）。
const (
	PresignPutTTL = 15 * time.Minute
	PresignGetTTL = 24 * time.Hour
)

// ErrTokenExpired 上传凭证过期/不存在（预签名过期拒传验收）。
var ErrTokenExpired = errors.New("upload token expired or not found")

// ErrObjectMismatch 回执对象与凭证不一致（object_key/etag 校验失败）。
var ErrObjectMismatch = errors.New("object mismatch with upload token")

// Store 资产元数据边界（s3_object_metadata）。
type Store interface {
	// CreatePresigned 登记预签名凭证（PRESIGNED）并返回 token。
	CreatePresigned(ctx context.Context, spaceID, userID int64, bucket, objectKey, contentType string, size int64) (uuid.UUID, error)
	// ConfirmObject 回执确认：锁定单行 → CONFIRMED（upload_token + object_key + 用户）。
	// 返回对象元数据（含 etag/size 校验所需）。
	ConfirmObject(ctx context.Context, token uuid.UUID, objectKey string, userID int64, etag string, size int64) error
}

// Client S3 客户端封装。
type Client struct {
	mc       *minio.Client
	bucket   string
	store    Store
}

// Config S3 配置（对齐 .env：S3_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET/USE_SSL）。
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// New 建立 MinIO 客户端并确保 bucket 存在。
func New(cfg Config, store Store) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 S3 客户端: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exists, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := mc.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &Client{mc: mc, bucket: cfg.Bucket, store: store}, nil
}

// PresignResult 预签名响应（05 ResUploadPresign）。
type PresignResult struct {
	UploadURL   string    `json:"upload_url"`
	ObjectKey   string    `json:"object_key"`
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
	UploadToken string    `json:"upload_token"`
}

// BuildObjectKey 对象 Key 规范：spaces/{space_id}/{asset_type}/{yyyyMMdd}/{uuid8}.{ext}（04 I.1）。
func BuildObjectKey(spaceID int64, assetType string, fileName string) string {
	ext := "bin"
	if idx := strings.LastIndex(fileName, "."); idx >= 0 && idx < len(fileName)-1 {
		ext = strings.ToLower(fileName[idx+1:])
	}
	return fmt.Sprintf("spaces/%d/%s/%s/%s.%s",
		spaceID, assetType, time.Now().Format("20060102"), uuid.NewString()[:8], ext)
}

// PresignUpload POST 预签名（PUT 15min）+ 登记凭证（B3.0 PRESIGNED）。
func (c *Client) PresignUpload(ctx context.Context, spaceID, userID int64, assetType, fileName, contentType string, size int64) (*PresignResult, error) {
	objectKey := BuildObjectKey(spaceID, assetType, fileName)
	url, err := c.mc.PresignedPutObject(ctx, c.bucket, objectKey, PresignPutTTL)
	if err != nil {
		return nil, fmt.Errorf("生成预签名上传 URL: %w", err)
	}
	token, err := c.store.CreatePresigned(ctx, spaceID, userID, c.bucket, objectKey, contentType, size)
	if err != nil {
		return nil, err
	}
	// GET 下载预签名（24h）
	downloadURL, err := c.mc.PresignedGetObject(ctx, c.bucket, objectKey, PresignGetTTL, nil)
	if err != nil {
		return nil, err
	}
	return &PresignResult{
		UploadURL:   url.String(),
		ObjectKey:   objectKey,
		DownloadURL: downloadURL.String(),
		ExpiresAt:   time.Now().Add(PresignPutTTL),
		UploadToken: token.String(),
	}, nil
}

// CompleteUpload 上传完成回执：HeadObject 校验 + 凭证确认（3.4.5）。
func (c *Client) CompleteUpload(ctx context.Context, token uuid.UUID, objectKey, etag string, userID int64) error {
	// HeadObject 回读真实对象（服务端校验，不信任客户端声明——04 I.1）
	info, err := c.mc.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("%w: 对象不存在或不可读: %v", ErrObjectMismatch, err)
	}
	actualETag := strings.Trim(info.ETag, `"`)
	if strings.Trim(etag, `"`) != "" && !strings.EqualFold(actualETag, strings.Trim(etag, `"`)) {
		return fmt.Errorf("%w: etag 不匹配", ErrObjectMismatch)
	}
	return c.store.ConfirmObject(ctx, token, objectKey, userID, actualETag, info.Size)
}

// PresignDownload 下载预签名（GET 24h；导出/资产下载场景）。
func (c *Client) PresignDownload(ctx context.Context, objectKey string) (string, error) {
	url, err := c.mc.PresignedGetObject(ctx, c.bucket, objectKey, PresignGetTTL, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}
