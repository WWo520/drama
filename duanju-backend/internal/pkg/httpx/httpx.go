// Package httpx 统一响应包装 ApiResponse<T> 与业务错误码（对齐 05 B2/C 章）。
package httpx

import (
	"net/http"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 业务错误码（05 C 章错误码字典，9 大类 40+ 细分码；code=0 成功）
const (
	CodeOK                = 0
	CodeUnauthorized      = 10001 // 未登录 / Authorization 缺失 / Token 格式错误 → 跳登录页
	CodeTokenExpired      = 10002 // access_token 已过期 → 静默 refresh 后重放
	CodeTokenInvalid      = 10003 // Token 签名无效 / 被篡改 → 强制踢登录页
	CodeForbidden         = 10004 // 无权限（RBAC/空间角色不足）
	CodeRefreshExpired    = 10005 // refresh_token 已过期 / 无效 / 被登出拉黑 → 强制登录
	CodeLoginFailed       = 10006 // 邮箱/手机号或密码错误
	CodeBanned            = 10007 // 账号被封禁 / 禁用
	CodeDuplicate         = 10008 // 邮箱/手机号已被注册
	CodeMaintenance       = 10009 // 平台维护模式（HTTP 503，feature_maintenance=true）
	CodeProjectNotFound   = 20001 // 项目不存在 / 已删除 / 无权限
	CodeSpaceNotFound     = 20005 // 空间不存在 / 已关闭 / 不是成员
	CodeInsufficientPoints = 40003 // 积分余额不足
	CodeRateLimited       = 90001 // 接口限流
	CodeConflict          = 90006 // DB 唯一约束冲突（并发创建同名资源）
	CodeInternal          = 99999 // 未知内部错误
)

// CodeBadRequest 参数错误专用码（90007，与未登录 10001 分离，避免语义冲突；05 字典 90001~90006 已占用）。
const CodeBadRequest = 90007

// Response ApiResponse<T> 统一响应体（05 B2 章字段完全对齐）。
type Response struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Data       any    `json:"data"`
	RequestID  string `json:"request_id"`
	Timestamp  string `json:"timestamp"`
}

// OK 成功响应（H02 生产加固：响应体统一脱敏 email/phone/mobile 字段）。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, newResp(c, CodeOK, "ok", maskSensitive(data)))
}

// maskSensitive 递归掩码敏感字段：email → s***@test.com；phone/mobile → 138****1234。
// 用反射处理任意 map/slice/struct 类型（gin.H、[]map[string]any、DTO struct 等）。
func maskSensitive(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		out := make(map[string]any, rv.Len())
		for _, k := range rv.MapKeys() {
			key := k.String()
			val := rv.MapIndex(k).Interface()
			switch key {
			case "email":
				if s, ok := val.(string); ok {
					out[key] = maskEmail(s)
					continue
				}
			case "phone", "mobile":
				if s, ok := val.(string); ok {
					out[key] = maskPhone(s)
					continue
				}
			}
			out[key] = maskSensitive(val)
		}
		return out
	case reflect.Slice, reflect.Array:
		// []byte（含 json.RawMessage）原样透传：逐字节拆包会变 []any，类型不匹配导致零值（2026-08-20 实测）
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return v
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = maskSensitive(rv.Index(i).Interface())
		}
		return out
	case reflect.Ptr:
		if rv.IsNil() {
			return v
		}
		// 保持指针类型：新指针指向脱敏后的元素（避免 struct 外层 Set 类型不匹配丢失字段）
		masked := maskSensitive(rv.Elem().Interface())
		mv := reflect.ValueOf(masked)
		if mv.IsValid() && mv.Type().AssignableTo(rv.Elem().Type()) {
			np := reflect.New(rv.Elem().Type())
			np.Elem().Set(mv)
			return np.Interface()
		}
		return v
	case reflect.Struct:
		// 结构体：按字段名脱敏（支持指针字段再递归）
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.NumField(); i++ {
			f := rv.Field(i)
			name := rv.Type().Field(i).Name
			if !f.CanInterface() {
				continue
			}
			fv := f.Interface()
			if s, ok := fv.(string); ok {
				switch name {
				case "Email":
					out.Field(i).SetString(maskEmail(s))
					continue
				case "Phone", "Mobile":
					out.Field(i).SetString(maskPhone(s))
					continue
				}
			}
			masked := maskSensitive(fv)
			if mv := reflect.ValueOf(masked); mv.IsValid() && mv.Type().AssignableTo(f.Type()) {
				out.Field(i).Set(mv)
			}
		}
		return out.Interface()
	default:
		return v
	}
}

func maskEmail(s string) string {
	at := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			at = i
			break
		}
	}
	if at <= 1 {
		return "***" + s
	}
	// 首字母 + *** + @域名（规范：s***@test.com）
	return s[:1] + "***" + s[at:]
}

func maskPhone(s string) string {
	if len(s) < 7 {
		return "****"
	}
	return s[:3] + "****" + s[len(s)-4:]
}

// Fail 业务失败响应（HTTP 状态码由 status 决定，body 仍为 ApiResponse 包装）。
func Fail(c *gin.Context, status, code int, msg string) {
	c.JSON(status, newResp(c, code, msg, nil))
}

// FailErr 便捷：根据错误类型映射（暂简单实现，后续可按 05 C 章扩展）。
func FailErr(c *gin.Context, status, code int, err error) {
	Fail(c, status, code, err.Error())
}

func newResp(c *gin.Context, code int, msg string, data any) Response {
	return Response{
		Code:      code,
		Message:   msg,
		Data:      data,
		RequestID: reqID(c),
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// requestID 从中间件注入；未注入则兜底生成。
func reqID(c *gin.Context) string {
	if id := c.GetString("request_id"); id != "" {
		return id
	}
	id := uuid.NewString()
	c.Set("request_id", id)
	return id
}
