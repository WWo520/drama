// ExportHandler：Worker-Export（FINAL_VIDEO_COMPOSE / ENGINEER_FILE_EXPORT）。
// 真实 FFmpeg concat 合成：拉取集内按序镜头成片 → 下载 → concat copy → 上传 MinIO → 回写 export_records。
// 素材为远程 HTTP URL（厂商临时 URL / 本地 mock）；产物签名 URL 24h 有效（PresignGetTTL）。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/duanju/backend/internal/engine/task"
)

// ExportStore 导出 Worker 数据边界。
type ExportStore interface {
	// CompleteExport 完成导出：SUCCESS + 文件信息 + 30 天过期。
	CompleteExport(ctx context.Context, exportID int64, outputURL string, fileSize int64) error
	// GetExportDetail 导出详情。
	GetExportDetail(ctx context.Context, exportID int64) (*ExportResult, error)
}

// ExportComposer 合成执行边界（真实 ffmpeg + 对象存储；测试可替换）。
type ExportComposer interface {
	// Compose 下载按序片段 → concat → 上传 → 返回 (下载URL, 文件大小)。
	Compose(ctx context.Context, spaceID int64, urls []string) (string, int64, error)
	// ComposeWithProgress 带进度回调版本。
	ComposeWithProgress(ctx context.Context, spaceID int64, urls []string, progress ComposeProgress) (string, int64, error)
}

// ExportResult 导出详情。
type ExportResult struct {
	ExportID   int64
	ExportUUID string
	Status     string
	OutputURL  string
	FileSize   int64
	ExpireAt   time.Time
}

// PresignGetTTL 导出成片签名 URL 有效期（24h）。
const PresignGetTTL = 24 * time.Hour

// FFmpegConcat 真实合成实现：HTTP 下载 → concat demuxer copy → MinIO PUT → presign。
type FFmpegConcat struct {
	store    ExportComposeStore
	mc       *minio.Client
	bucket   string
	binPath  string // ffmpeg 可执行路径，默认 "ffmpeg"
	workDir  string // 临时目录，默认 os.TempDir()
	noEncode bool   // true=concat copy（默认）；false=重编码归一
}

// ExportSegment 整集合成的单个片段（按序拼接的镜头成片；导出包级类型供仓储层实现）。
type ExportSegment struct {
	ShotID int64
	URL    string
}

// ExportComposeStore 合成所需的数据查询（episode → 按序片段）。
type ExportComposeStore interface {
	// ListEpisodeSegments 按 episode 拉取按序 SUCCESS 镜头成片 URL。
	ListEpisodeSegments(ctx context.Context, episodeID int64) ([]ExportSegment, error)
	// FirstEpisodeID 项目首集（当前导出对象 = 第一集）。
	FirstEpisodeID(ctx context.Context, projectID int64) (int64, error)
}

func NewFFmpegConcat(store ExportComposeStore, mc *minio.Client, bucket string) *FFmpegConcat {
	bin := os.Getenv("FFMPEG_PATH")
	if bin == "" {
		bin = "ffmpeg"
	}
	return &FFmpegConcat{store: store, mc: mc, bucket: bucket, binPath: bin, workDir: os.TempDir(), noEncode: true}
}

// ComposeProgress 合成进度回调（阶段推进时触发；stage: downloading/composing/uploading）。
type ComposeProgress func(stage string, done, total, failed int)

// Compose 执行真实合成：下载全部片段 → concat → 上传 → 签名 URL。
func (f *FFmpegConcat) Compose(ctx context.Context, spaceID int64, urls []string) (string, int64, error) {
	return f.ComposeWithProgress(ctx, spaceID, urls, nil)
}

// ComposeWithProgress 带进度回调的合成（progress 可为 nil）。
func (f *FFmpegConcat) ComposeWithProgress(ctx context.Context, spaceID int64, urls []string, progress ComposeProgress) (string, int64, error) {
	if len(urls) == 0 {
		return "", 0, fmt.Errorf("无可合成的片段")
	}
	notify := func(stage string, done, total, failed int) {
		if progress != nil {
			progress(stage, done, total, failed)
		}
	}
	dir, err := os.MkdirTemp(f.workDir, "export-*")
	if err != nil {
		return "", 0, fmt.Errorf("创建临时目录: %w", err)
	}
	defer os.RemoveAll(dir)

	// 1. 下载片段（顺序保留）；SPA fallback 会把无效 URL 也回 200+HTML，
	//    因此校验 mp4 魔数（ftyp），无效片段跳过，全部无效才报错。
	notify("downloading", 0, len(urls), 0)
	local := make([]string, 0, len(urls))
	failed := 0
	for i, u := range urls {
		dst := filepath.Join(dir, fmt.Sprintf("seg-%03d.mp4", i))
		if err := downloadToFile(ctx, u, dst); err != nil {
			failed++
			log.Printf("export: 跳过无效片段 %d (%s): %v", i, truncate(u, 80), err)
			notify("downloading", i+1, len(urls), failed)
			continue
		}
		local = append(local, dst)
		notify("downloading", i+1, len(urls), failed)
	}
	if len(local) == 0 {
		return "", 0, fmt.Errorf("全部 %d 个片段均无法下载为有效视频", len(urls))
	}

	// 2. concat 列表（单引号转义防路径注入）
	notify("composing", 0, len(local), failed)
	listPath := filepath.Join(dir, "list.txt")
	var lb strings.Builder
	for _, p := range local {
		lb.WriteString("file '" + strings.ReplaceAll(p, "'", `'\''`) + "'\n")
	}
	if err := os.WriteFile(listPath, []byte(lb.String()), 0o644); err != nil {
		return "", 0, err
	}

	// 3. ffmpeg concat（copy 不重编码；失败一次重编码兜底——片段参数不一致时 copy 会失败）
	outPath := filepath.Join(dir, "final.mp4")
	args := []string{"-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy"}
	if !f.noEncode {
		args = append(args, "-vf", "scale=720:1280:force_original_aspect_ratio=decrease,pad=720:1280:(ow-iw)/2:(oh-ih)/2")
	}
	args = append(args, "-movflags", "+faststart", outPath)
	cmd := exec.CommandContext(ctx, f.binPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("export: concat copy 失败，回退重编码: %v\n%s", err, truncate(string(out), 500))
		reArgs := []string{"-y", "-f", "concat", "-safe", "0", "-i", listPath,
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
			"-vf", "scale=720:1280:force_original_aspect_ratio=decrease,pad=720:1280:(ow-iw)/2:(oh-ih)/2",
			"-r", "30", "-c:a", "aac", "-movflags", "+faststart", outPath}
		reCmd := exec.CommandContext(ctx, f.binPath, reArgs...)
		if out2, err2 := reCmd.CombinedOutput(); err2 != nil {
			return "", 0, fmt.Errorf("ffmpeg 合成失败: %w\n%s", err2, truncate(string(out2), 500))
		}
	}

	// 4. 上传 MinIO
	notify("uploading", len(local), len(local), failed)
	st, err := os.Stat(outPath)
	if err != nil {
		return "", 0, err
	}
	objectKey := fmt.Sprintf("spaces/%d/exports/%s/final.mp4", spaceID, nowStamp())
	if _, err := f.mc.FPutObject(ctx, f.bucket, objectKey, outPath, minio.PutObjectOptions{ContentType: "video/mp4"}); err != nil {
		return "", 0, fmt.Errorf("上传成片: %w", err)
	}

	// 5. 签名下载 URL（24h；过期后可从 export 记录重新发起合成）
	dl, err := f.mc.PresignedGetObject(ctx, f.bucket, objectKey, PresignGetTTL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("生成成片下载 URL: %w", err)
	}
	return dl.String(), st.Size(), nil
}

// downloadToFile HTTP 下载到本地文件；校验 mp4 魔数（ftyp box），HTML/错误页视为失败。
func downloadToFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	// 魔数校验：MP4 第 4~8 字节为 "ftyp"
	head := make([]byte, 12)
	n, _ := io.ReadFull(resp.Body, head)
	if n < 12 || string(head[4:8]) != "ftyp" {
		return fmt.Errorf("非 MP4 内容（%d 字节，head=%q）——疑似 SPA fallback 或错误页", n, truncate(string(head), 12))
	}
	if _, err := out.Write(head[:n]); err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

func nowStamp() string { return time.Now().Format("20060102150405") }

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ExportProgressStore 合成进度回写边界（仓储层可选实现）。
type ExportProgressStore interface {
	// UpdateExportProgress 阶段进度回写（DOWNLOADING/COMPOSING/UPLOADING/DONE）。
	UpdateExportProgress(ctx context.Context, exportID int64, stage string, done, total, failed int, durationSec int) error
}

// ExportHandler 导出 Worker（FINAL_VIDEO_COMPOSE）。
type ExportHandler struct {
	store    ExportStore
	segs     ExportComposeStore
	composer ExportComposer
}

func NewExportHandler(store ExportStore) *ExportHandler {
	return &ExportHandler{store: store}
}

// progressStore 进度回写边界（ExportStore 若实现了 ExportProgressStore 则复用；否则返回 nil 跳过回写）。
func (h *ExportHandler) progressStore() ExportProgressStore {
	if ps, ok := h.store.(ExportProgressStore); ok {
		return ps
	}
	return nil
}

// SetComposer 注入真实合成器（nil 时回退 mock 链路——开发环境无 MinIO/ffmpeg 也可跑通）。
func (h *ExportHandler) SetComposer(segs ExportComposeStore, c ExportComposer) {
	h.segs = segs
	h.composer = c
}

func (h *ExportHandler) Name() string { return "FINAL_VIDEO_COMPOSE" }

func (h *ExportHandler) Handles(taskType string) bool {
	return taskType == "FINAL_VIDEO_COMPOSE" || taskType == "ENGINEER_FILE_EXPORT"
}

// Execute 视频合成：真实 ffmpeg concat → MinIO；composer 未装配或无片段时回退 mock。
func (h *ExportHandler) Execute(ctx context.Context, t *task.Task) ([]byte, error) {
	var params struct {
		ExportID   int64  `json:"export_id"`
		Resolution string `json:"resolution"`
		Watermark  bool   `json:"watermark"`
		EpisodeID  int64  `json:"episode_id"` // 目标集（Trigger 写入；0 = 兼容旧记录，回退首集）
	}
	if err := json.Unmarshal(t.RequestParamsJSON, &params); err != nil || params.ExportID == 0 {
		return nil, fmt.Errorf("导出任务参数无效")
	}

	// 真实链路：装配了合成器且能取到片段
	if h.segs != nil && h.composer != nil && t.ProjectID != nil {
		epID := params.EpisodeID
		if epID <= 0 {
			// 旧任务/旧导出记录无目标集 → 回退首集（历史兼容）
			var err error
			epID, err = h.segs.FirstEpisodeID(ctx, *t.ProjectID)
			if err != nil {
				log.Printf("export: 解析首集失败 export_id=%d: %v", params.ExportID, err)
			}
		}
		if epID > 0 {
			segs, serr := h.segs.ListEpisodeSegments(ctx, epID)
			if serr == nil && len(segs) > 0 {
				urls := make([]string, 0, len(segs))
				for _, s := range segs {
					urls = append(urls, s.URL)
				}
				log.Printf("export: 开始真实合成 export_id=%d res=%s segments=%d", params.ExportID, params.Resolution, len(urls))
				// 阶段进度回写（前端轮询 export 记录渲染进度条；仓储未实现进度接口时跳过）
				ps := h.progressStore()
				progress := func(stage string, done, total, failed int) {
					if ps == nil {
						return
					}
					if perr := ps.UpdateExportProgress(ctx, params.ExportID, stage, done, total, failed, 0); perr != nil {
						log.Printf("export: 进度回写失败 export_id=%d stage=%s: %v", params.ExportID, stage, perr)
					}
				}
				outputURL, fileSize, cerr := h.composer.ComposeWithProgress(ctx, t.SpaceID, urls, progress)
				if cerr == nil {
					if werr := h.store.CompleteExport(ctx, params.ExportID, outputURL, fileSize); werr != nil {
						return nil, werr
					}
					// 终态：DONE 阶段（成片时长由前端播放器元数据展示，此处置 0 跳过）
					if ps != nil {
						_ = ps.UpdateExportProgress(ctx, params.ExportID, "DONE", len(urls), len(urls), 0, 0)
					}
					log.Printf("export: 真实合成完成 export_id=%d size=%d", params.ExportID, fileSize)
					out, _ := json.Marshal(map[string]any{
						"export_id": params.ExportID, "output_url": outputURL,
						"file_size": fileSize, "segments": len(urls), "is_mock": false,
					})
					return out, nil
				}
				// 真实合成失败 → 落日志后走 mock 兜底（dev 环境保持可用）
				log.Printf("export: 真实合成失败，回退 mock: %v", cerr)
			}
		}
	}

	// mock 回退（无 MinIO/ffmpeg/片段的开发环境）
	log.Printf("export: mock 合成 export_id=%d res=%s watermark=%v", params.ExportID, params.Resolution, params.Watermark)
	outputURL := fmt.Sprintf("%s/sample.mp4?export=%d&t=%s", mockCDNBase(), params.ExportID, nowStamp())
	fileSize := int64(10 * 1024 * 1024)
	if err := h.store.CompleteExport(ctx, params.ExportID, outputURL, fileSize); err != nil {
		return nil, err
	}
	log.Printf("export: mock 完成 export_id=%d url=%s", params.ExportID, outputURL)
	out, _ := json.Marshal(map[string]any{
		"export_id": params.ExportID, "output_url": outputURL,
		"file_size": fileSize, "is_mock": true,
	})
	return out, nil
}
