package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	appfile "molly-server/internal/application/file"
	domainfile "molly-server/internal/domain/file"
)

// FileHandler 文件相关 HTTP 处理器。
// 职责：解析请求 → 调用 use-case → 返回响应。无业务逻辑。
type FileHandler struct {
	uc *appfile.UseCase
}

func NewFileHandler(uc *appfile.UseCase) *FileHandler {
	return &FileHandler{uc: uc}
}

// Register 注册需要登录的路由（由 router.go 在 authed group 调用）。
func (h *FileHandler) Register(rg *gin.RouterGroup) {
	f := rg.Group("/files")
	{
		// 上传
		f.POST("/precheck", h.precheck)
		f.POST("/upload", h.upload)
		f.GET("/upload/progress", h.uploadProgress)
		f.GET("/upload/tasks", h.listUploadTasks)
		f.DELETE("/upload/tasks/:id", h.deleteUploadTask)
		f.PUT("/upload/tasks/:id/renew", h.renewUploadTask)

		// 文件列表 & 搜索
		f.GET("", h.list)
		f.GET("/search", h.search)
		f.GET("/public", h.publicList)

		// 文件操作
		f.DELETE("", h.deleteFiles)
		f.PUT("/move", h.move)
		f.PUT("/rename", h.rename)
		f.PUT("/public", h.setPublic)

		// 目录操作
		f.POST("/dirs", h.makeDir)
		f.PUT("/dirs/:id/rename", h.renameDir)
		f.DELETE("/dirs/:id", h.deleteDir)
	}
}

// ── 上传 ──────────────────────────────────────────────────────

func (h *FileHandler) precheck(c *gin.Context) {
	var req appfile.PrecheckReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	resp, err := h.uc.Precheck(c.Request.Context(), session.UserID, req)
	if err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, resp)
}

func (h *FileHandler) upload(c *gin.Context) {
	var req appfile.UploadChunkReq
	if err := c.ShouldBind(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	session := mustSession(c)
	resp, err := h.uc.UploadChunk(c.Request.Context(), session.UserID, req, file, header)
	if err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, resp)
}

func (h *FileHandler) uploadProgress(c *gin.Context) {
	var req appfile.UploadProgressReq
	if err := c.ShouldBindQuery(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	resp, err := h.uc.GetUploadProgress(c.Request.Context(), session.UserID, req)
	if err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, resp)
}

func (h *FileHandler) listUploadTasks(c *gin.Context) {
	var req appfile.TaskListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	resp, err := h.uc.ListUploadTasks(c.Request.Context(), session.UserID, req)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal error")
		return
	}
	ok(c, resp)
}

func (h *FileHandler) deleteUploadTask(c *gin.Context) {
	taskID := c.Param("id")
	session := mustSession(c)
	if err := h.uc.DeleteUploadTask(c.Request.Context(), session.UserID, taskID); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

func (h *FileHandler) renewUploadTask(c *gin.Context) {
	var req appfile.RenewTaskReq
	req.TaskID = c.Param("id")
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	item, err := h.uc.RenewUploadTask(c.Request.Context(), session.UserID, req)
	if err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, item)
}

// ── 文件列表 ──────────────────────────────────────────────────

func (h *FileHandler) list(c *gin.Context) {
	var req appfile.FileListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	resp, err := h.uc.ListFiles(c.Request.Context(), session.UserID, req)
	if err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, resp)
}

func (h *FileHandler) search(c *gin.Context) {
	var req appfile.SearchReq
	if err := c.ShouldBindQuery(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	// 搜索复用 UserFile.Search，通过 use-case 联查 FileInfo
	// 此处暂时省略，后续扩展 SearchFiles use-case
	_ = session
	_ = req
	ok(c, gin.H{"message": "search not yet implemented"})
}

func (h *FileHandler) publicList(c *gin.Context) {
	ok(c, gin.H{"message": "public list not yet implemented"})
}

// ── 文件操作 ──────────────────────────────────────────────────

func (h *FileHandler) deleteFiles(c *gin.Context) {
	var req appfile.DeleteFilesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	resp, err := h.uc.DeleteFiles(c.Request.Context(), session.UserID, req)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal error")
		return
	}
	ok(c, resp)
}

func (h *FileHandler) move(c *gin.Context) {
	var req appfile.MoveFileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	if err := h.uc.MoveFile(c.Request.Context(), session.UserID, req); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

func (h *FileHandler) rename(c *gin.Context) {
	var req appfile.RenameFileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	if err := h.uc.RenameFile(c.Request.Context(), session.UserID, req); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

func (h *FileHandler) setPublic(c *gin.Context) {
	var req appfile.SetPublicReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	if err := h.uc.SetPublic(c.Request.Context(), session.UserID, req); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

// ── 目录操作 ──────────────────────────────────────────────────

func (h *FileHandler) makeDir(c *gin.Context) {
	var req appfile.MakeDirReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	session := mustSession(c)
	if err := h.uc.MakeDir(c.Request.Context(), session.UserID, req); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

func (h *FileHandler) renameDir(c *gin.Context) {
	var req appfile.RenameDirReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	req.DirID = c.GetInt("id")
	session := mustSession(c)
	if err := h.uc.RenameDir(c.Request.Context(), session.UserID, req); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

func (h *FileHandler) deleteDir(c *gin.Context) {
	var req appfile.DeleteDirReq
	req.DirID = c.GetInt("id")
	session := mustSession(c)
	if err := h.uc.DeleteDir(c.Request.Context(), session.UserID, req); err != nil {
		h.handleFileErr(c, err)
		return
	}
	ok(c, nil)
}

// ── 错误映射 ──────────────────────────────────────────────────

func (h *FileHandler) handleFileErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domainfile.ErrNotFound),
		errors.Is(err, domainfile.ErrDirNotFound),
		errors.Is(err, domainfile.ErrTaskNotFound):
		fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domainfile.ErrForbidden):
		fail(c, http.StatusForbidden, err.Error())
	case errors.Is(err, domainfile.ErrDirAlreadyExists),
		errors.Is(err, domainfile.ErrDirNameConflict),
		errors.Is(err, domainfile.ErrFileNameConflict),
		errors.Is(err, domainfile.ErrEncryptedPublic),
		errors.Is(err, domainfile.ErrDirIsRoot),
		errors.Is(err, domainfile.ErrTaskNotExpired):
		fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domainfile.ErrInsufficientSpace),
		errors.Is(err, domainfile.ErrNoDiskAvailable):
		fail(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domainfile.ErrPrecheckExpired):
		fail(c, http.StatusGone, err.Error())
	default:
		fail(c, http.StatusInternalServerError, "internal error")
	}
}
