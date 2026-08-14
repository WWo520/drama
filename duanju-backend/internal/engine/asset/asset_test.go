// 资产引擎测试：预签名上传/回执闭环 + 过期/伪造 token 拒传（S1-B06 验收）。
package asset

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore 内存凭证。
type fakeStore struct {
	tokens map[uuid.UUID]string // token → objectKey
}

func newFakeStore() *fakeStore { return &fakeStore{tokens: map[uuid.UUID]string{}} }

func (f *fakeStore) CreatePresigned(_ context.Context, _, _ int64, _, objectKey, _ string, _ int64) (uuid.UUID, error) {
	t := uuid.New()
	f.tokens[t] = objectKey
	return t, nil
}

func (f *fakeStore) ConfirmObject(_ context.Context, token uuid.UUID, objectKey string, _ int64, _ string, _ int64) error {
	got, ok := f.tokens[token]
	if !ok || got != objectKey {
		return ErrTokenExpired
	}
	delete(f.tokens, token)
	return nil
}

// testClient 连接本地 MinIO（docker compose duanju-minio:9000）。
func testClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(Config{
		Endpoint:  "127.0.0.1:9000",
		AccessKey: "duanju",
		SecretKey: "duanju_dev_pwd",
		Bucket:    "duanju-test-" + uuid.NewString()[:8],
		UseSSL:    false,
	}, newFakeStore())
	require.NoError(t, err)
	return c
}

// putFile 用预签名 URL 真实上传。
func putFile(t *testing.T, url string, content []byte) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "image/png")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp.Header.Get("ETag")
}

func TestPresignUploadAndComplete(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()

	res, err := c.PresignUpload(ctx, 1, 1, "asset_upload", "角色图.png", "image/png", 128)
	require.NoError(t, err)
	assert.Contains(t, res.ObjectKey, "spaces/1/asset_upload/")
	assert.NotEmpty(t, res.UploadURL)
	assert.NotEmpty(t, res.DownloadURL)
	assert.NotEmpty(t, res.UploadToken)
	// 有效期 15min（05 G 章）
	assert.InDelta(t, 15*60, res.ExpiresAt.Sub(time.Now()).Seconds(), 60)

	// 真实 PUT 上传
	etag := putFile(t, res.UploadURL, []byte("fake-png-bytes"))
	require.NotEmpty(t, etag)

	// 回执确认（服务端 HeadObject 校验）
	token, _ := uuid.Parse(res.UploadToken)
	err = c.CompleteUpload(ctx, token, res.ObjectKey, etag, 1)
	require.NoError(t, err)
}

func TestCompleteRejectsExpiredOrForgedToken(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()

	// 场景1：对象不存在（伪造 object_key）→ HeadObject 校验失败
	err := c.CompleteUpload(ctx, uuid.New(), "spaces/1/asset_upload/20260812/nonexist.png", "etag", 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrObjectMismatch)

	// 场景2：真实上传对象 + 伪造/过期 token → store 层拒绝（预签名过期拒传验收）
	res, err := c.PresignUpload(ctx, 1, 1, "asset_upload", "真实对象.png", "image/png", 64)
	require.NoError(t, err)
	etag := putFile(t, res.UploadURL, []byte("real-bytes"))
	err = c.CompleteUpload(ctx, uuid.New(), res.ObjectKey, etag, 1) // 伪造 token
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTokenExpired)

	// 场景3：登记凭证但未上传对象 → HeadObject 失败
	res2, err := c.PresignUpload(ctx, 1, 1, "asset_upload", "未上传.png", "image/png", 64)
	require.NoError(t, err)
	token2, _ := uuid.Parse(res2.UploadToken)
	err = c.CompleteUpload(ctx, token2, res2.ObjectKey, "etag", 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrObjectMismatch)
}

func TestCompleteTwiceRejected(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	res, err := c.PresignUpload(ctx, 1, 1, "asset_upload", "重复回执.png", "image/png", 64)
	require.NoError(t, err)
	etag := putFile(t, res.UploadURL, []byte("bytes"))
	token, _ := uuid.Parse(res.UploadToken)

	require.NoError(t, c.CompleteUpload(ctx, token, res.ObjectKey, etag, 1))
	// 重复确认同一 token → 凭证已消费（幂等由上层 store 决定，此处拒绝）
	err = c.CompleteUpload(ctx, token, res.ObjectKey, etag, 1)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "mismatch"))
}
