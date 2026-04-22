package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"molly-server/internal/domain/user"
	"molly-server/internal/presentation/http/middleware"
)

// response 是所有 API 的统一响应结构。
// HTTP 状态码和 body 中的 code 始终保持一致。
type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{
		Code:    http.StatusOK,
		Message: "ok",
		Data:    data,
	})
}

func fail(c *gin.Context, httpStatus int, msg string) {
	c.AbortWithStatusJSON(httpStatus, response{
		Code:    httpStatus,
		Message: msg,
	})
}

// mustSession 从 gin.Context 取 Session，供需要登录的 handler 使用。
// 前置条件：路由已挂载 middleware.Verify()，否则 panic（属于编程错误）。
func mustSession(c *gin.Context) *user.Session {
	val, _ := c.Get(string(middleware.KeyUserSession))
	return val.(*user.Session)
}
