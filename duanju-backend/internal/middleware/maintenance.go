// Maintenance 维护模式中间件：platform_configs.feature_maintenance=true 时拦截用户端 API。
// 放行：/auth/*（登录/刷新入口）、/admin/*（管理端可进维护操作）、/healthz（探针）。
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/duanju/backend/internal/pkg/httpx"
	"github.com/duanju/backend/internal/pkg/platformcfg"
)

// Maintenance 构建维护模式中间件（pf 为平台配置读取器，30s 缓存）。
func Maintenance(pf *platformcfg.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if p == "/healthz" || strings.HasPrefix(p, "/api/v1/auth/") || strings.HasPrefix(p, "/api/v1/admin/") {
			c.Next()
			return
		}
		on, err := pf.Bool(c.Request.Context(), "feature_maintenance")
		if err == nil && on {
			httpx.Fail(c, http.StatusServiceUnavailable, httpx.CodeMaintenance, "平台维护中，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}
