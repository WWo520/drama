// ImagePersist 资产图持久化：厂商返回的火山 TOS 私有签名 URL 仅 24h 有效，
// 过期即前端图裂 + AutoDL 拉不到参考图（视频必失败的根因）。
// 此处在 Worker 出图成功后立即转存自有 MinIO，落库固定未签名 URL（配合桶匿名只读策略永不过期）。
package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

// ImagePersister 图片持久化能力（可选注入；nil 时保持原 URL 直存旧链路）。
type ImagePersister interface {
	// PersistImage 拉取 srcURL 字节并转存对象存储 objectKey，返回固定访问 URL。
	PersistImage(ctx context.Context, srcURL, objectKey string) (string, error)
	// EnsurePublicRead 保证桶具备匿名下载策略（转存后的固定 URL 才可被浏览器/远端 GPU 直接拉取）。
	EnsurePublicRead(ctx context.Context) error
}

// MinIOImagePersist MinIO 实现（任何 S3 兼容端点均可：本地 MinIO / 火山 TOS S3 兼容端点等）。
type MinIOImagePersist struct {
	mc         *minio.Client
	bucket     string
	publicBase string // 公共访问基址：
	//   含 "{bucket}" 占位符 → 虚拟主机式（如 https://{bucket}.tos-cn-beijing.volces.com）
	//   不含占位符           → 路径式（如 http://127.0.0.1:9000，最终 URL 为 base/bucket/key）
	client *http.Client
}

func NewMinIOImagePersist(mc *minio.Client, bucket, publicBase string) *MinIOImagePersist {
	return &MinIOImagePersist{
		mc:         mc,
		bucket:     bucket,
		publicBase: strings.TrimRight(publicBase, "/"),
		client:     &http.Client{Timeout: 120 * time.Second},
	}
}

// EnsurePublicRead 幂等设置桶匿名只读策略（整桶 download）。
func (m *MinIOImagePersist) EnsurePublicRead(ctx context.Context) error {
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, m.bucket)
	return m.mc.SetBucketPolicy(ctx, m.bucket, policy)
}

// PersistImage HTTP 拉取源图字节 → FPutObject → 返回 publicBase/bucket/key 固定 URL。
func (m *MinIOImagePersist) PersistImage(ctx context.Context, srcURL, objectKey string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client.Do(req)
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
	_, err = m.mc.PutObject(ctx, m.bucket, objectKey, strings.NewReader(string(body)), int64(len(body)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", err
	}
	// 公共 URL 拼装：支持虚拟主机式（publicBase 含 {bucket} 占位符）与路径式两种
	if strings.Contains(m.publicBase, "{bucket}") {
		return strings.ReplaceAll(m.publicBase, "{bucket}", m.bucket) + "/" + objectKey, nil
	}
	return m.publicBase + "/" + m.bucket + "/" + objectKey, nil
}

// persistGeneratedImage 统一入口：真实生成图转存自有存储；失败保留原 URL 不阻断主流程。
func persistGeneratedImage(ctx context.Context, p ImagePersister, projectID int64, kind string, id int64, srcURL string) string {
	if p == nil || srcURL == "" || isMockLikeURL(srcURL) {
		return srcURL
	}
	key := fmt.Sprintf("spaces/%d/assets/%s-%d/%d.png", projectID, strings.ToLower(kind), id, time.Now().UnixMilli())
	durable, err := p.PersistImage(ctx, srcURL, key)
	if err != nil {
		log.Printf("image-persist: 转存失败(保留原URL %s): %v", truncateStr(srcURL, 60), err)
		return srcURL
	}
	log.Printf("image-persist: 已转存 %s -> %s", truncateStr(srcURL, 60), durable)
	return durable
}

// PersistImagePublic 外部工具（genstylethumbs 等）复用的转存入口：
// 真实图转存对象存储并返回固定 URL；失败保留原 URL 不阻断调用方主流程。
func PersistImagePublic(ctx context.Context, p ImagePersister, keyPrefix, srcURL string) string {
	if p == nil || srcURL == "" || isMockLikeURL(srcURL) {
		return srcURL
	}
	key := fmt.Sprintf("%s/%d.png", strings.Trim(keyPrefix, "/"), time.Now().UnixMilli())
	durable, err := p.PersistImage(ctx, srcURL, key)
	if err != nil {
		log.Printf("image-persist: 转存失败(保留原URL %s): %v", truncateStr(srcURL, 60), err)
		return srcURL
	}
	return durable
}

// isMockLikeURL 开发期占位地址不走转存。
func isMockLikeURL(u string) bool {
	return strings.Contains(u, "/sample.mp4") || strings.Contains(u, "asset-placeholder")
}

// tosSignedExpiry 解析火山 TOS 私有签名 URL 的过期时刻（X-Tos-Date + X-Tos-Expires）。
// 非 TOS 签名 URL（MinIO 固定链 / mock 占位等）返回 ok=false，调用方原样放行。
func tosSignedExpiry(raw string) (time.Time, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.Contains(u.Host, "volces.com") {
		return time.Time{}, false
	}
	q := u.Query()
	dateStr, expStr := q.Get("X-Tos-Date"), q.Get("X-Tos-Expires")
	if dateStr == "" || expStr == "" {
		return time.Time{}, false
	}
	signedAt, err := time.Parse("20060102T150405Z", dateStr)
	if err != nil {
		return time.Time{}, false
	}
	expires, err := strconv.Atoi(expStr)
	if err != nil {
		expires = 86400
	}
	return signedAt.Add(time.Duration(expires) * time.Second), true
}

// filterExpiredRefs 过滤不可达的参考图（AutoDL 拉不到会导致整卡失败——
// 已知症状：带 1 张坏图的卡必败、纯 T2V 反而成功）。过滤两类：
//  1. 已过期的 TOS 签名 URL（X-Tos-Date/X-Tos-Expires 超时）
//  2. 私网地址（127.0.0.1/localhost/192.168.x/10.x 等——云 GPU 永远拉不到）
//
// 返回过滤后列表与被剔除的名字。
func filterExpiredRefs(refs []RefAsset) ([]RefAsset, []string) {
	now := time.Now()
	out := make([]RefAsset, 0, len(refs))
	var dropped []string
	for _, ra := range refs {
		if expAt, ok := tosSignedExpiry(ra.URL); ok && now.After(expAt) {
			dropped = append(dropped, ra.Name)
			continue
		}
		if isPrivateNetworkURL(ra.URL) {
			dropped = append(dropped, ra.Name)
			continue
		}
		out = append(out, ra)
	}
	return out, dropped
}

// isPrivateNetworkURL 判定是否为云 GPU 不可达的私网地址。
func isPrivateNetworkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	switch host {
	case "127.0.0.1", "localhost", "0.0.0.0", "::1":
		return true
	}
	if strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.16.") || strings.HasPrefix(host, "172.17.") ||
		strings.HasPrefix(host, "172.18.") || strings.HasPrefix(host, "172.19.") ||
		strings.HasPrefix(host, "172.2") || strings.HasPrefix(host, "172.30.") || strings.HasPrefix(host, "172.31.") {
		return true
	}
	return false
}
