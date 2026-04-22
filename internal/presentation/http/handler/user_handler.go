package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	appuser "molly-server/internal/application/user"
	domainuser "molly-server/internal/domain/user"
)

// UserHandler 用户相关 HTTP 处理器。
// 职责：解析 HTTP 请求 → 调用 use-case → 将结果序列化为 HTTP 响应。
// 不包含任何业务逻辑。
type UserHandler struct {
	uc *appuser.UseCase
}

func NewUserHandler(uc *appuser.UseCase) *UserHandler {
	return &UserHandler{uc: uc}
}

// RegisterPublic 注册无需鉴权的路由。
func (h *UserHandler) RegisterPublic(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.login)
	rg.POST("/auth/register", h.register)
}

// Register 注册需要鉴权的路由（如修改资料）。
func (h *UserHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/users/me", h.me)
}

// ── Handlers ─────────────────────────────────────────────────

// login godoc
// @Summary     用户登录
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body appuser.LoginReq true "登录信息"
// @Success     200  {object} response{data=appuser.LoginResp}
// @Failure     400  {object} response
// @Failure     401  {object} response
// @Router      /auth/login [post]
func (h *UserHandler) login(c *gin.Context) {
	var req appuser.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	resp, err := h.uc.Login(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, domainuser.ErrInvalidCredential):
			fail(c, http.StatusUnauthorized, err.Error())
		case errors.Is(err, domainuser.ErrDisabled):
			fail(c, http.StatusForbidden, err.Error())
		default:
			fail(c, http.StatusInternalServerError, "internal error")
		}
		return
	}

	ok(c, resp)
}

// register godoc
// @Summary     用户注册
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body appuser.RegisterReq true "注册信息"
// @Success     200  {object} response{data=appuser.UserResp}
// @Failure     400  {object} response
// @Failure     409  {object} response
// @Router      /auth/register [post]
func (h *UserHandler) register(c *gin.Context) {
	var req appuser.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	resp, err := h.uc.Register(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, domainuser.ErrUserNameConflict):
			fail(c, http.StatusConflict, err.Error())
		case errors.Is(err, domainuser.ErrEmailConflict):
			fail(c, http.StatusConflict, err.Error())
		default:
			fail(c, http.StatusInternalServerError, "internal error")
		}
		return
	}

	ok(c, resp)
}

// me 获取当前登录用户信息。
func (h *UserHandler) me(c *gin.Context) {
	// Session 已由 middleware.Verify 注入
	session := mustSession(c)
	u, err := h.uc.GetByID(c.Request.Context(), session.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal error")
		return
	}
	ok(c, u)
}
