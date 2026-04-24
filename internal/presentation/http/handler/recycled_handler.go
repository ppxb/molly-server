package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apprecycled "molly-server/internal/application/recycled"
	domainrecycled "molly-server/internal/domain/recycled"
)

type RecycledHandler struct {
	uc *apprecycled.UseCase
}

func NewRecycledHandler(uc *apprecycled.UseCase) *RecycledHandler {
	return &RecycledHandler{uc: uc}
}

// Register 注册路由（在 authed group 下）。
func (h *RecycledHandler) Register(rg *gin.RouterGroup) {
	r := rg.Group("/recycled")
	{
		r.GET("", h.list)
		r.POST("/restore", h.restore)
		r.DELETE("/:id", h.deletePermanently)
		r.DELETE("", h.empty)
	}
}

func (h *RecycledHandler) list(c *gin.Context) {
	var req apprecycled.ListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	resp, err := h.uc.List(c.Request.Context(), session.UserID, req)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal error")
		return
	}
	ok(c, resp)
}

func (h *RecycledHandler) restore(c *gin.Context) {
	var req apprecycled.RestoreReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	msg, err := h.uc.Restore(c.Request.Context(), session.UserID, req)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	ok(c, gin.H{"message": msg})
}

func (h *RecycledHandler) deletePermanently(c *gin.Context) {
	req := apprecycled.DeleteReq{RecycledID: c.Param("id")}
	session := mustSession(c)
	if err := h.uc.DeletePermanently(c.Request.Context(), session.UserID, req); err != nil {
		h.handleErr(c, err)
		return
	}
	ok(c, nil)
}

func (h *RecycledHandler) empty(c *gin.Context) {
	session := mustSession(c)
	resp, err := h.uc.Empty(c.Request.Context(), session.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal error")
		return
	}
	ok(c, resp)
}

func (h *RecycledHandler) handleErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainrecycled.ErrNotFound):
		fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domainrecycled.ErrForbidden):
		fail(c, http.StatusForbidden, err.Error())
	default:
		fail(c, http.StatusInternalServerError, "internal error")
	}
}
