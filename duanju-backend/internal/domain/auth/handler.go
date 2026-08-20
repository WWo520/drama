// 认证 HTTP 入口：注册/登录/刷新/登出/当前用户（05 TAG 1 · 1.1~1.5）。
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/duanju/backend/internal/middleware"
	"github.com/duanju/backend/internal/pkg/httpx"
	"github.com/duanju/backend/internal/pkg/jwt"
)

// Handler 认证路由处理器。
type Handler struct {
	svc    *Service
	jwtMgr *jwt.Manager
	rdb    *redis.Client
	env    string
}

func NewHandler(svc *Service, jwtMgr *jwt.Manager, rdb *redis.Client, env string) *Handler {
	return &Handler{svc: svc, jwtMgr: jwtMgr, rdb: rdb, env: env}
}

// RegisterRoutes 注册 /auth/* 路由（05 1.1~1.5）。
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	{
		auth.POST("/register", h.register)
		auth.POST("/login", h.login)
		auth.POST("/refresh", h.refresh)
		auth.POST("/logout", h.logout)
		auth.GET("/me", middleware.JWT(h.jwtMgr, h.rdb, h.env), h.me)
		auth.PUT("/me", middleware.JWT(h.jwtMgr, h.rdb, h.env), h.updateMe)
	}
}

func (h *Handler) register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if !req.HasAccount() {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "邮箱或手机号至少填写一项")
		return
	}
	h.svc.Register(c, &req)
}

func (h *Handler) login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	if req.Account() == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "邮箱或手机号至少填写一项")
		return
	}
	h.svc.Login(c, &req)
}

func (h *Handler) refresh(c *gin.Context) {
	h.svc.Refresh(c)
}

func (h *Handler) logout(c *gin.Context) {
	h.svc.Logout(c)
}

func (h *Handler) me(c *gin.Context) {
	h.svc.Me(c)
}

func (h *Handler) updateMe(c *gin.Context) {
	var req UpdateMeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeBadRequest, "参数错误："+err.Error())
		return
	}
	h.svc.UpdateMe(c, &req)
}
