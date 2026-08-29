// VideoPersist 视频持久化：厂商返回的结果 URL（CodeWithGPU/AutoDL COS 输出桶、火山 TOS 签名链等）
// 均为临时存储，保留期 1~2 天后即清理 → 历史视频 404「凭空消失」的根因。
// 与 image_persist 同构：Worker 出片成功后立即转存自有对象存储，落库固定 URL 永不过期；
// 转存失败保留原 URL 不阻断主流程。
package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

// mPutVideoOpts MinIO 视频上传选项。
var mPutVideoOpts = minio.PutObjectOptions{ContentType: "video/mp4"}

// VideoPersister 视频持久化能力（可选注入；nil 时保持原 URL 直存旧链路）。
type VideoPersister interface {
	// PersistVideo 拉取 srcURL 视频字节并转存对象存储 objectKey，返回固定访问 URL。
	PersistVideo(ctx context.Context, srcURL, objectKey string) (string, error)
}

// videoHTTPClient 视频文件大（几 MB~几十 MB），下载超时放宽到 10 分钟。
var videoHTTPClient = &http.Client{Timeout: 10 * time.Minute}

// downloadVideoToTemp 流式下载视频到临时文件；校验 mp4 魔数（ftyp box），
// HTML/错误页视为失败。调用方负责删除临时文件。
func downloadVideoToTemp(ctx context.Context, srcURL string) (path string, size int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := videoHTTPClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("拉取源视频 HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "dj-video-*.mp4")
	if err != nil {
		return "", 0, err
	}
	tmpPath := tmp.Name()
	// 魔数校验：MP4 第 4~8 字节为 "ftyp"（与 export_handler.downloadToFile 同规则）
	head := make([]byte, 12)
	n, rerr := io.ReadFull(resp.Body, head)
	if rerr != nil || n < 12 || string(head[4:8]) != "ftyp" {
		tmp.Close()
		os.Remove(tmpPath)
		return "", 0, fmt.Errorf("非 MP4 内容（%d 字节，head=%q）——源 URL 已失效或返回错误页", n, truncate(string(head), 12))
	}
	if _, err = tmp.Write(head[:n]); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", 0, err
	}
	written, err := io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", 0, err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", 0, err
	}
	size = int64(12) + written
	if size < 4096 {
		os.Remove(tmpPath)
		return "", 0, fmt.Errorf("源视频字节数异常过小(%d)", size)
	}
	return tmpPath, size, nil
}

// objectPublicURL 公共 URL 拼装：虚拟主机式（publicBase 含 {bucket} 占位符）与路径式两种。
func objectPublicURL(publicBase, bucket, objectKey string) string {
	if strings.Contains(publicBase, "{bucket}") {
		return strings.ReplaceAll(publicBase, "{bucket}", bucket) + "/" + objectKey
	}
	return publicBase + "/" + bucket + "/" + objectKey
}

// PersistVideo TOS 实现：临时文件 → PutObjectV2（ContentLength 显式传文件大小）→ 固定公共 URL。
func (t *TOSImagePersist) PersistVideo(ctx context.Context, srcURL, objectKey string) (string, error) {
	tmpPath, size, err := downloadVideoToTemp(ctx, srcURL)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)
	f, err := os.Open(tmpPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	in := &tos.PutObjectV2Input{
		PutObjectBasicInput: tos.PutObjectBasicInput{
			Bucket: t.bucket, Key: objectKey, ContentType: "video/mp4", ContentLength: size,
		},
		Content: f,
	}
	if _, err = t.cli.PutObjectV2(ctx, in); err != nil {
		return "", err
	}
	log.Printf("video-persist-tos: 已转存 %s -> %s (%.1fMB)", truncateStr(srcURL, 60), objectKey, float64(size)/1024/1024)
	return objectPublicURL(t.publicBase, t.bucket, objectKey), nil
}

// PersistVideo MinIO 实现（任何 S3 兼容端点均可）：临时文件 → PutObject → 固定公共 URL。
func (m *MinIOImagePersist) PersistVideo(ctx context.Context, srcURL, objectKey string) (string, error) {
	tmpPath, size, err := downloadVideoToTemp(ctx, srcURL)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)
	f, err := os.Open(tmpPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err = m.mc.PutObject(ctx, m.bucket, objectKey, f, size, mPutVideoOpts); err != nil {
		return "", err
	}
	log.Printf("video-persist-minio: 已转存 %s -> %s (%.1fMB)", truncateStr(srcURL, 60), objectKey, float64(size)/1024/1024)
	return objectPublicURL(m.publicBase, m.bucket, objectKey), nil
}

// persistGeneratedVideo 统一入口：真实生成视频转存自有存储；失败保留原 URL 不阻断主流程。
func persistGeneratedVideo(ctx context.Context, p VideoPersister, projectID int64, kind string, id int64, srcURL string) string {
	if p == nil || srcURL == "" || isMockLikeURL(srcURL) {
		return srcURL
	}
	key := fmt.Sprintf("spaces/%d/videos/%s-%d/%d.mp4", projectID, strings.ToLower(kind), id, time.Now().UnixMilli())
	durable, err := p.PersistVideo(ctx, srcURL, key)
	if err != nil {
		log.Printf("video-persist: 转存失败(保留原URL %s): %v", truncateStr(srcURL, 60), err)
		return srcURL
	}
	log.Printf("video-persist: 已转存 %s -> %s", truncateStr(srcURL, 60), durable)
	return durable
}
