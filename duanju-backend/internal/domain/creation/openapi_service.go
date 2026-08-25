// OpenAPI（S3-B09）：API Key 签发/吊销（HMAC 签名校验）+ webhook 回调注册。
package creation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
)

// OpenAPIStore 开发者数据边界。
type OpenAPIStore interface {
	// CreateKey 签发（返回明文 key，仅存哈希）。
	CreateKey(ctx context.Context, userID int64, name string) (string, error)
	// RevokeKey 吊销。
	RevokeKey(ctx context.Context, userID int64, keyPrefix string) error
	// ListKeys 我的密钥。
	ListKeys(ctx context.Context, userID int64) ([]map[string]any, error)
	// RegisterWebhook 注册回调。
	RegisterWebhook(ctx context.Context, userID int64, url, secret string, events []string) error
	// VerifyWebhook 签名校验（HMAC-SHA256）。
	VerifyWebhook(ctx context.Context, userID int64, url string, payload []byte, signature string) (bool, error)
}

// OpenAPIService 开发者服务。
type OpenAPIService struct {
	store OpenAPIStore
}

func NewOpenAPIService(store OpenAPIStore) *OpenAPIService {
	return &OpenAPIService{store: store}
}

// CreateKey 签发（10.x）。
func (s *OpenAPIService) CreateKey(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required,min=1,max=64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	key, err := s.store.CreateKey(c.Request.Context(), uid, req.Name)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "签发失败")
		return
	}
	httpx.OK(c, gin.H{"api_key": key, "tip": "仅展示一次，请妥善保存"})
}

// ListKeys 我的密钥。
func (s *OpenAPIService) ListKeys(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	items, err := s.store.ListKeys(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "获取密钥失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// RevokeKey 吊销。
func (s *OpenAPIService) RevokeKey(c *gin.Context, keyPrefix string) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	if err := s.store.RevokeKey(c.Request.Context(), uid, keyPrefix); err != nil {
		httpx.Fail(c, http.StatusBadRequest, 20009, err.Error())
		return
	}
	httpx.OK(c, nil)
}

// RegisterWebhook 注册回调。
func (s *OpenAPIService) RegisterWebhook(c *gin.Context) {
	uid := userID(c)
	if uid == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请先登录")
		return
	}
	var req struct {
		URL    string   `json:"url" binding:"required,url"`
		Secret string   `json:"secret" binding:"required,min=8,max=128"`
		Events []string `json:"events"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	// E01 SSRF 防护：拒绝内网/环回地址的 webhook URL
	if err := ValidatePublicURL(req.URL); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "URL 不合法："+err.Error())
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{"TASK_COMPLETED"}
	}
	if err := s.store.RegisterWebhook(c.Request.Context(), uid, req.URL, req.Secret, req.Events); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "注册失败")
		return
	}
	httpx.OK(c, nil)
}

// VerifyWebhookSignature 回调验签（HMAC-SHA256；08 审查 Major#18：按当前用户归属验签）。
func (s *OpenAPIService) VerifyWebhookSignature(c *gin.Context, url string) {
	uid := userID(c)
	payload, _ := c.GetRawData()
	sig := c.GetHeader("X-Signature")
	ok, err := s.store.VerifyWebhook(c.Request.Context(), uid, url, payload, sig)
	if err != nil || !ok {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "签名校验失败")
		return
	}
	httpx.OK(c, nil)
}

// SignPayload HMAC 签名（供测试/回调侧使用）。
func SignPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// ValidatePublicURL E01 SSRF 防护：URL 必须为 http/https 且主机非内网/环回/链路本地地址。
func ValidatePublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("缺少主机名")
	}
	// 解析 IP（域名先查 DNS，拒绝解析到内网的域名）
	ips, err := net.LookupIP(host)
	if err != nil {
		// 直接 IP 或解析失败：按字面处理
		ips = []net.IP{net.ParseIP(host)}
	}
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("禁止指向内网地址: %s", ip)
		}
	}
	return nil
}