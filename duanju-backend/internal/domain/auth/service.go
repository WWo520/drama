// 认证服务：注册/登录/登出/刷新业务逻辑（对齐 04 C 章）。
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
	"github.com/duanju/backend/internal/pkg/jwt"
	"github.com/duanju/backend/internal/pkg/password"
	"github.com/duanju/backend/internal/pkg/platformcfg"
)

// Service 认证服务。
type Service struct {
	repo   *Repo
	jwtMgr *jwt.Manager
	rdb    *redis.Client
	env    string
	lg     *zap.Logger
	pf     *platformcfg.Store
}

func NewService(repo *Repo, jwtMgr *jwt.Manager, rdb *redis.Client, env string, lg *zap.Logger, pf *platformcfg.Store) *Service {
	return &Service{repo: repo, jwtMgr: jwtMgr, rdb: rdb, env: env, lg: lg, pf: pf}
}

// Register 注册：创建用户+个人空间+积分账户，签发双 Token（05 1.1）。
func (s *Service) Register(c *gin.Context, req *RegisterReq) {
	// 注册开关（feature_register=false 时拒绝；配置缺失/异常默认开放）
	if open, err := s.pf.Bool(c.Request.Context(), "feature_register"); err == nil && !open {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "平台暂未开放注册")
		return
	}
	hash, err := password.Hash(req.Password)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "密码哈希失败")
		return
	}
	u, err := s.repo.CreateUserWithSpace(c.Request.Context(), strings.ToLower(req.Email), req.Phone, hash, req.Nickname)
	if err != nil {
		if errors.Is(err, ErrDuplicate) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeDuplicate, "该邮箱/手机号已被注册")
			return
		}
		s.lg.Error("注册失败", zap.String("request_id", c.GetString("request_id")), zap.Error(err))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "注册失败，请稍后重试")
		return
	}
	s.issueTokens(c, u, "注册成功")
}

// Login 登录（05 1.2）：封禁拦截 + 失败限流 + bcrypt→Argon2id 透明升级。
func (s *Service) Login(c *gin.Context, req *LoginReq) {
	// 失败次数限流（防暴力破解）：同账号+IP 15 分钟内失败 5 次锁定
	failKey := fmt.Sprintf("duanju:%s:auth:loginfail:%s:%s", s.env, strings.ToLower(req.Account()), c.ClientIP())
	if n, _ := s.rdb.Get(c.Request.Context(), failKey).Int(); n >= 5 {
		httpx.Fail(c, http.StatusTooManyRequests, httpx.CodeRateLimited, "失败次数过多，请 15 分钟后再试")
		return
	}
	u, err := s.repo.GetByAccount(c.Request.Context(), req.Account())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// 账号不存在也计数（防枚举）
			s.countLoginFail(c, failKey)
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeLoginFailed, "账号或密码错误")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "登录失败，请稍后重试")
		return
	}
	// 封禁拦截（P1 修复：封禁账号禁止登录）
	if u.Status == "BANNED" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeBanned, "账号已被封禁，请联系客服")
		return
	}
	ok, upgrade := password.Verify(req.Password, u.PasswordHash)
	if !ok {
		s.countLoginFail(c, failKey)
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeLoginFailed, "账号或密码错误")
		return
	}
	_ = s.rdb.Del(c.Request.Context(), failKey).Err()
	if upgrade {
		// bcrypt → Argon2id 透明升级
		if newHash, err := password.Hash(req.Password); err == nil {
			_ = s.repo.UpgradePasswordHash(c.Request.Context(), u.ID, newHash)
		}
	}
	s.issueTokens(c, u, "登录成功")
}

// countLoginFail 登录失败计数（15 分钟窗口）。
func (s *Service) countLoginFail(c *gin.Context, key string) {
	_ = s.rdb.Incr(c.Request.Context(), key).Err()
	_ = s.rdb.Expire(c.Request.Context(), key, 15*time.Minute).Err()
}

// Refresh 用 Refresh Token 换新 Access（05 1.3，Cookie 携带）。
func (s *Service) Refresh(c *gin.Context) {
	refreshStr, err := c.Cookie("refresh_token")
	if err != nil || refreshStr == "" {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "请重新登录")
		return
	}
	claims, err := s.jwtMgr.Parse(refreshStr)
	if err != nil || claims[jwt.ClaimKind] != jwt.KindRefresh {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeRefreshExpired, "登录已过期，请重新登录")
		return
	}
	jti, err := jwt.JTI(claims)
	if err == nil {
		blacklisted, _ := s.rdb.Exists(c.Request.Context(), s.refreshBlacklistKey(jti)).Result()
		if blacklisted > 0 {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeRefreshExpired, "登录已过期，请重新登录")
			return
		}
	}
	uid, err := jwt.UserID(claims)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeRefreshExpired, "登录已过期，请重新登录")
		return
	}
	// 指纹校验（M 修复：Refresh 同样绑定 UA+IP，与设计文档一致）
	if fp, ok := claims[jwt.ClaimFinger].(string); ok && fp != "" {
		if fp != c.ClientIP()+"|"+c.Request.UserAgent() {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeRefreshExpired, "登录环境变化，请重新登录")
			return
		}
	}
	u, err := s.repo.GetByUUID(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeRefreshExpired, "账号不存在")
		return
	}
	if u.Status == "BANNED" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeBanned, "账号已被封禁，请联系客服")
		return
	}
	s.issueTokens(c, u, "刷新成功")
}

// Logout 登出：拉黑 access jti + refresh jti（05 1.4；access 立即失效）。
func (s *Service) Logout(c *gin.Context) {
	// 拉黑 access token（剩余有效期窗口，保证登出后 access 立即失效）
	if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		if claims, perr := s.jwtMgr.Parse(strings.TrimPrefix(authHeader, "Bearer ")); perr == nil {
			if jti, jerr := jwt.JTI(claims); jerr == nil {
				_ = s.rdb.Set(c.Request.Context(), s.refreshBlacklistKey(jti), "1", 2*time.Hour).Err()
			}
		}
	}
	if refreshStr, err := c.Cookie("refresh_token"); err == nil && refreshStr != "" {
		if claims, perr := s.jwtMgr.Parse(refreshStr); perr == nil {
			if jti, jerr := jwt.JTI(claims); jerr == nil {
				ttl := 7 * 24 * time.Hour
				_ = s.rdb.Set(c.Request.Context(), s.refreshBlacklistKey(jti), "1", ttl).Err()
			}
		}
	}
	c.SetCookie("refresh_token", "", -1, "/", "", true, true)
	httpx.OK(c, nil)
}

// Me 当前用户 + 积分余额（05 1.5）。
func (s *Service) Me(c *gin.Context) {
	uid := c.GetString(middleware.CtxUserUUID)
	u, err := s.repo.GetByUUID(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "账号不存在")
		return
	}
	points, _ := s.repo.GetPoints(c.Request.Context(), u.ID)
	spaceUUID, _ := s.repo.GetDefaultSpaceUUID(c.Request.Context(), u.ID)
	brief := toBrief(u)
	brief.DefaultSpaceID = spaceUUID
	resp := MeResp{
		User:          *brief,
		PointBalance:  points.CurrentBalance,
		FrozenBalance: points.FrozenBalance,
	}
	httpx.OK(c, resp)
}

// UpdateMe 资料更新（PUT /auth/me）：昵称/头像部分更新；头像存 object key，
// 引用前校验对象归属（本人上传且 CONFIRMED，防引用他人私有对象）。
func (s *Service) UpdateMe(c *gin.Context, req *UpdateMeReq) {
	uid := c.GetString(middleware.CtxUserUUID)
	u, err := s.repo.GetByUUID(c.Request.Context(), uid)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "账号不存在")
		return
	}
	var nickname, avatar *string
	if req.Nickname != "" {
		nickname = &req.Nickname
	}
	if req.AvatarURL != "" {
		if !strings.HasPrefix(req.AvatarURL, "spaces/") {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "头像对象路径非法")
			return
		}
		if err := s.repo.VerifyOwnedObject(c.Request.Context(), u.ID, req.AvatarURL); err != nil {
			if errors.Is(err, ErrNotFound) {
				httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "头像对象不存在或不属于当前用户")
				return
			}
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "头像校验失败")
			return
		}
		avatar = &req.AvatarURL
	}
	if nickname == nil && avatar == nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "无可更新字段")
		return
	}
	if err := s.repo.UpdateProfile(c.Request.Context(), u.ID, nickname, avatar); err != nil {
		s.lg.Error("资料更新失败", zap.String("request_id", c.GetString("request_id")), zap.Error(err))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "资料更新失败")
		return
	}
	if nickname != nil {
		u.Nickname = *nickname
	}
	if avatar != nil {
		u.AvatarURL = avatar
	}
	httpx.OK(c, toBrief(u))
}

// issueTokens 签发双 Token 并写 Refresh Cookie。
func (s *Service) issueTokens(c *gin.Context, u *UserRow, msg string) {
	fp := c.ClientIP() + "|" + c.Request.UserAgent()
	pair, refresh, err := s.jwtMgr.Sign(u.UserUUID, fp)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "签发 Token 失败")
		return
	}
	// 生产环境 Secure=true（HTTPS），开发环境 http 本地可携带
	secure := s.env == "production"
	c.SetCookie("refresh_token", refresh, int((7*24*time.Hour).Seconds()), "/", "", secure, true)
	c.Set(middleware.CtxUserID, u.ID)
	c.Set(middleware.CtxUserUUID, u.UserUUID)
	httpx.OK(c, LoginResp{
		AccessToken: pair.AccessToken,
		ExpiresIn:   pair.ExpiresInSec,
		TokenType:   "Bearer",
		User:        toBrief(u),
	})
}

func toBrief(u *UserRow) *UserBrief {
	email, phone, avatar := "", "", ""
	if u.Email != nil {
		email = *u.Email
	}
	if u.Phone != nil {
		phone = *u.Phone
	}
	if u.AvatarURL != nil {
		avatar = *u.AvatarURL
	}
	return &UserBrief{
		UserID:    u.UserUUID,
		Email:     email,
		Phone:     phone,
		Nickname:  u.Nickname,
		AvatarURL: avatar,
		VipLevel:  int(u.VipLevel),
	}
}

func (s *Service) refreshBlacklistKey(jti string) string {
	return fmt.Sprintf("duanju:%s:auth:refresh_blacklist:%s", s.env, jti)
}
