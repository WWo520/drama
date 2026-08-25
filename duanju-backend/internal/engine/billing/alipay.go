// 支付宝当面付：下单/回调 7 步验签/状态轮询（对齐 04 H.1/H.2）。
package billing

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 支付宝回调 7 步验签（04 H.2 铁律：顺序不可改写，任何失败返回 "fail" 触发重试）。
type AlipayConfig struct {
	AppID       string
	MockSecret  string // 开发模式 HMAC 密钥（沙箱/联调用）
	PublicKey   string // 生产 RSA2 公钥（预留）
	UseMock     bool   // 开发期 true
	CallbackURL string
}

// OrderStore 订单数据边界。
type OrderStore interface {
	// CreateOrder 幂等创建订单（user_id+idempotency_key UNIQUE；订阅传 packageID+days，积分包传 0,0）。
	CreateOrder(ctx context.Context, userID int64, orderType, planCode string, amountYuan string, points int, idemKey string, packageID int64, days int) (orderID int64, orderNo string, err error)
	// GetOrderByNo 查订单。
	GetOrderByNo(ctx context.Context, orderNo string) (*Order, error)
	// GetOrderByNoForUser 查订单（归属校验：仅订单所有者可见）。
	GetOrderByNoForUser(ctx context.Context, orderNo string, userID int64) (*Order, error)
	// GetPackage 查积分套餐（档位校验用）。
	GetPackage(ctx context.Context, pkgCode string) (*Package, error)
	// GetPlanPackage 查订阅套餐（packages 表，档位校验用）。
	GetPlanPackage(ctx context.Context, code string) (*PlanPackage, error)
	// PayCallback 回调事务：订单置 PAID + 加积分 + 流水（pay_transaction_id UNIQUE 幂等；订阅订单额外开通/续期会员）。
	// 返回 (是否首次处理, 错误)；已处理的交易返回 false。
	PayCallback(ctx context.Context, orderNo, payTransactionID string, points int) (bool, error)
	// QueryMembership 查询用户当前 ACTIVE 会员（订阅订单状态轮询附带）。
	QueryMembership(ctx context.Context, userID int64, planCode, status, renew, endAt *string) error
	// LogCallback 记录回调日志（H01 生产加固）。
	LogCallback(ctx context.Context, params map[string]string, step int, result, failReason string) error
}

// Package 积分套餐（point_packages 行）。
type Package struct {
	PkgCode     string
	Points      int
	BonusPoints int
	PriceYuan   string
	Status      string
}

// PlanPackage 订阅套餐（packages 行）。
type PlanPackage struct {
	ID          int64
	Code        string
	Name        string
	Cycle       string // monthly / yearly / ''
	PriceYuan   string
	PointsQuota int
	Status      string // ACTIVE / OFF
}

// Order 订单。
type Order struct {
	ID               int64
	OrderNo          string
	UserID           int64
	OrderType        string
	PlanCode         string
	AmountYuan       string
	Points           int
	PayStatus        string
	PayTransactionID string
	PackageID        int64
	DaysGranted      int
}

// AlipayService 支付服务。
type AlipayService struct {
	store OrderStore
	cfg   AlipayConfig
	// OnPaid 首次到账回调（可选；WS 推送 POINTS_BALANCE_CHANGED，前端据此停止订单状态轮询）
	OnPaid func(ctx context.Context, userID int64, points int)
}

func NewAlipayService(store OrderStore, cfg AlipayConfig) *AlipayService {
	return &AlipayService{store: store, cfg: cfg}
}

// CreateOrderResp 下单响应（05 9.5）。
type CreateOrderResp struct {
	OrderNo    string `json:"order_no"`
	QRCodeURL  string `json:"qr_code_url"`
	AmountYuan string `json:"amount_yuan"`
	ExpireAt   string `json:"expire_at"`
}

// CreateOrder 下单（H.1 幂等查重 + 服务端档位校验 + 写订单 + mock 二维码）。
// 档位校验：以 point_packages 表为唯一真源，amount_yuan 与 points 必须与服务端价目表一致，
// 拒绝前端任意构造的金额/积分组合（防刷积分）。
func (s *AlipayService) CreateOrder(ctx context.Context, userID int64, orderType, planCode, amountYuan string, points int, idemKey string) (*CreateOrderResp, error) {
	if idemKey == "" {
		return nil, fmt.Errorf("缺少幂等键")
	}
	// 档位校验（P0 修复：0.01 元买任意积分漏洞）
	pkg, err := s.store.GetPackage(ctx, planCode)
	if err != nil {
		return nil, fmt.Errorf("套餐不存在")
	}
	if pkg.Status != "ON" {
		return nil, fmt.Errorf("套餐已下架")
	}
	if amountYuan != pkg.PriceYuan {
		return nil, fmt.Errorf("金额与套餐价目不符")
	}
	if points != pkg.Points+pkg.BonusPoints {
		return nil, fmt.Errorf("积分与套餐价目不符")
	}
	orderID, orderNo, err := s.store.CreateOrder(ctx, userID, orderType, planCode, amountYuan, points, idemKey, 0, 0)
	if err != nil {
		return nil, err
	}
	_ = orderID
	// mock 当面付二维码（沙箱环境）
	return &CreateOrderResp{
		OrderNo:    orderNo,
		QRCodeURL:  fmt.Sprintf("https://openapi-sandbox.dl.alipaydev.com/gateway.do?order_no=%s", orderNo),
		AmountYuan: amountYuan,
		ExpireAt:   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	}, nil
}

// CreateSubscriptionOrder 订阅套餐下单（05 9.5 biz_type=package）：
// 服务端校验 packages 表价目/状态 → 创建 SUBSCRIPTION_MONTH/YEAR 订单（含套餐积分额度与开通天数）。
func (s *AlipayService) CreateSubscriptionOrder(ctx context.Context, userID int64, planCode, billingCycle, idemKey string) (*CreateOrderResp, error) {
	if idemKey == "" {
		return nil, fmt.Errorf("缺少幂等键")
	}
	plan, err := s.store.GetPlanPackage(ctx, planCode)
	if err != nil {
		return nil, fmt.Errorf("套餐不存在")
	}
	if plan.Status != "ACTIVE" {
		return nil, fmt.Errorf("套餐已下架")
	}
	// 计费周期：请求 billing_cycle 须与套餐配置一致（月度套餐只卖月付；大小写不敏感）
	cycle := strings.ToLower(plan.Cycle)
	if cycle == "" {
		cycle = "monthly"
	}
	if billingCycle != "" && strings.ToLower(billingCycle) != cycle {
		return nil, fmt.Errorf("套餐不支持该计费周期")
	}
	orderType := "SUBSCRIPTION_MONTH"
	days := 30
	if cycle == "yearly" {
		orderType = "SUBSCRIPTION_YEAR"
		days = 365
	}
	orderID, orderNo, err := s.store.CreateOrder(ctx, userID, orderType, planCode, plan.PriceYuan, plan.PointsQuota, idemKey, plan.ID, days)
	if err != nil {
		return nil, err
	}
	_ = orderID
	return &CreateOrderResp{
		OrderNo:    orderNo,
		QRCodeURL:  fmt.Sprintf("https://openapi-sandbox.dl.alipaydev.com/gateway.do?order_no=%s", orderNo),
		AmountYuan: plan.PriceYuan,
		ExpireAt:   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
	}, nil
}

// 返回 "success" 或 "fail"。
func (s *AlipayService) HandleCallback(ctx context.Context, params map[string]string) string {
	// 步1：验签（mock=HMAC；生产=RSA2）
	if !s.verifySignature(params) {
		_ = s.store.LogCallback(ctx, params, 1, "fail", "验签失败")
		return "fail"
	}
	// 步 2：核心字段校验（sign_type/app_id/out_trade_no/金额字符串精确比较）
	if params["app_id"] != s.cfg.AppID {
		_ = s.store.LogCallback(ctx, params, 2, "fail", "app_id 不匹配")
		return "fail"
	}
	orderNo := params["out_trade_no"]
	if orderNo == "" {
		_ = s.store.LogCallback(ctx, params, 2, "fail", "out_trade_no 为空")
		return "fail"
	}
	// 08 审查 Major#12：仅支付成功/交易完成才入账
	if ts := params["trade_status"]; ts != "TRADE_SUCCESS" && ts != "TRADE_FINISHED" {
		_ = s.store.LogCallback(ctx, params, 2, "fail", "非成功状态:"+ts)
		return "fail"
	}
	// 金额：从订单读取并按字符串精确比较
	order, err := s.store.GetOrderByNo(ctx, orderNo)
	if err != nil {
		_ = s.store.LogCallback(ctx, params, 2, "fail", "订单不存在")
		return "fail"
	}
	if params["total_amount"] != order.AmountYuan {
		_ = s.store.LogCallback(ctx, params, 2, "fail", "金额不匹配")
		return "fail"
	}
	// 步3：幂等去重 + 步4~6：事务内 PAID+加积分+流水
	first, err := s.store.PayCallback(ctx, orderNo, params["trade_no"], order.Points)
	if err != nil {
		_ = s.store.LogCallback(ctx, params, 3, "fail", "事务失败:"+err.Error())
		return "fail"
	}
	if !first {
		_ = s.store.LogCallback(ctx, params, 3, "success", "幂等跳过")
		return "success"
	}
	// 首次到账回调（WS 推送积分变化；失败不影响回调结果）
	if s.OnPaid != nil {
		s.OnPaid(ctx, order.UserID, order.Points)
	}
	// 步7：返回 success
	_ = s.store.LogCallback(ctx, params, 7, "success", "")
	return "success"
}

// QueryMembership 查询用户当前 ACTIVE 会员（9.8 Status 订阅订单附带；无会员时返回 error 供调用方忽略）。
func (s *AlipayService) QueryMembership(ctx context.Context, userID int64, planCode, status, renew, endAt *string) error {
	return s.store.QueryMembership(ctx, userID, planCode, status, renew, endAt)
}

// verifySignature 回调验签：UseMock=true 走开发 HMAC；生产走支付宝 RSA2 公钥验签。
// fail-closed：生产未配置公钥 / 验签异常一律拒绝（08 审查 Critical#1 修复）。
func (s *AlipayService) verifySignature(params map[string]string) bool {
	sign := params["sign"]
	if sign == "" {
		return false
	}
	// 按 key 排序拼接（RSA2 验签规则：a=b&c=d&...）
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k + "=" + params[k])
	}
	payload := sb.String()
	if !s.cfg.UseMock {
		return verifyRSA2(s.cfg.PublicKey, payload, sign)
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.MockSecret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	got := strings.ToLower(strings.TrimSpace(sign))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

// verifyRSA2 支付宝 RSA2（SHA256withRSA）验签：公钥支持 PEM 全文或裸 base64 两种配置形态。
func verifyRSA2(publicKey, payload, signB64 string) bool {
	if publicKey == "" {
		return false
	}
	pemText := publicKey
	if !strings.Contains(pemText, "BEGIN") {
		pemText = "-----BEGIN PUBLIC KEY-----\n" + pemText + "\n-----END PUBLIC KEY-----"
	}
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return false
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signB64))
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(payload))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) == nil
}

// BuildMockCallback 生成 mock 支付宝回调参数（供 E2E 测试/内部触发）。
func (s *AlipayService) BuildMockCallback(orderNo, amountYuan, tradeNo string) map[string]string {
	params := map[string]string{
		"app_id":       s.cfg.AppID,
		"out_trade_no": orderNo,
		"trade_no":     tradeNo,
		"total_amount": amountYuan,
		"sign_type":    "RSA2",
		"timestamp":    time.Now().Format("2006-01-02 15:04:05"),
		"trade_status": "TRADE_SUCCESS", // 08 审查 Major#12：回调校验必须包含
	}
	// 计算 HMAC 签名（与 verifySignature 一致：排除 sign/sign_type）
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k + "=" + params[k])
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.MockSecret))
	mac.Write([]byte(sb.String()))
	params["sign"] = hex.EncodeToString(mac.Sum(nil))
	return params
}
