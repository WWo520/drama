// Package auth 认证模块：注册/登录/登出/刷新/当前用户（对齐 05 TAG 1 · 1.1~1.5 / E 章 schema）。
package auth

// RegisterReq 注册请求（05 ReqAuthRegister）：email 或 phone 二选一。
type RegisterReq struct {
	Email    string `json:"email" binding:"omitempty,email,max=128"`
	Phone    string `json:"phone" binding:"omitempty,max=20"`
	Password string `json:"password" binding:"required,min=8,max=64"`
	Nickname string `json:"nickname" binding:"required,min=2,max=64"`
	InviteCode string `json:"invite_code" binding:"omitempty,max=64"`
}

// HasAccount 校验 email/phone 至少提供一个。
func (r *RegisterReq) HasAccount() bool { return r.Email != "" || r.Phone != "" }

// LoginReq 登录请求（05 ReqAuthLogin）：email 或 phone 二选一 + password。
type LoginReq struct {
	Email    string `json:"email" binding:"omitempty,email"`
	Phone    string `json:"phone" binding:"omitempty"`
	Password string `json:"password" binding:"required"`
}

// Account 取登录账号（email 优先）。
func (r *LoginReq) Account() string {
	if r.Email != "" {
		return r.Email
	}
	return r.Phone
}

// LoginResp 登录/注册/刷新响应（05 ResAuthLogin）。
type LoginResp struct {
	AccessToken string     `json:"access_token"`
	ExpiresIn   int        `json:"expires_in"`
	TokenType   string     `json:"token_type"`
	User        *UserBrief `json:"user"`
}

// UserBrief 用户摘要（05 UserMeDTO）。
type UserBrief struct {
	UserID         string `json:"user_id"` // user_uuid 对外暴露
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	VipLevel       int    `json:"vip_level"`
	VipExpireAt    string `json:"vip_expire_at"`
	DefaultSpaceID string `json:"default_space_id"` // space_uuid 对外暴露
}

// MeResp /auth/me 响应（05 1.5：用户 + 积分账户）。
type MeResp struct {
	User          UserBrief `json:"user"`
	PointBalance  int       `json:"point_balance"`
	FrozenBalance int       `json:"frozen_balance"`
}
