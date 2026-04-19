package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"molly-server/internal/platform/httpserver/response"
	"molly-server/internal/upload/service"
)

type Handler struct {
	service service.Service
	logger  *slog.Logger
}

func NewHandler(uploadService service.Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: uploadService,
		logger:  logger,
	}
}

func (h *Handler) RegisterPublicRoutes(group *gin.RouterGroup) {
	fileGroup := group.Group("/file")
	fileGroup.POST("/search", h.search)
	fileGroup.POST("/list", h.list)
	fileGroup.POST("/get", h.getFile)
	fileGroup.POST("/get_path", h.getFilePath)
	fileGroup.POST("/get_folder_size_info", h.getFolderSizeInfo)
	fileGroup.POST("/create_with_folders", h.createWithFolders)
	fileGroup.POST("/get_upload_url", h.getUploadURL)
	fileGroup.POST("/complete", h.completeFile)
	fileGroup.POST("/update", h.updateFile)
	fileGroup.POST("/delete", h.deleteFile)
	fileGroup.POST("/get_latest_async_task", h.getLatestAsyncTask)
	fileGroup.POST("/list_move_targets", h.listMoveTargets)
	fileGroup.POST("/get_access_url", h.getFileAccessURL)

	recycleBinGroup := group.Group("/recyclebin")
	recycleBinGroup.POST("/trash", h.recycleBinTrash)
	recycleBinGroup.POST("/list", h.recycleBinList)
	recycleBinGroup.POST("/restore", h.recycleBinRestore)

	group.POST("/batch", h.batch)
}

func (h *Handler) RegisterAuthRoutes(_ *gin.RouterGroup) {}

func (h *Handler) search(c *gin.Context) {
	var req searchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.Search(c.Request.Context(), service.SearchRequest{
		Query:   req.Query,
		OrderBy: req.OrderBy,
		Limit:   req.Limit,
		DriveID: req.DriveID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	items := make([]searchItem, 0, len(res.Items))
	for _, item := range res.Items {
		items = append(items, searchItem{
			DriveID:      item.DriveID,
			FileID:       item.FileID,
			ParentFileID: item.ParentFileID,
			Name:         item.Name,
			Type:         item.Type,
			Size:         item.Size,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}

	response.JSON(c, http.StatusOK, searchResponse{
		Items:      items,
		NextMarker: res.NextMarker,
	})
}

func (h *Handler) list(c *gin.Context) {
	var req fileListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.List(c.Request.Context(), service.ListRequest{
		DriveID:               req.DriveID,
		ParentFileID:          req.ParentFileID,
		Limit:                 req.Limit,
		All:                   req.All,
		URLExpireSec:          req.URLExpireSec,
		OrderBy:               req.OrderBy,
		OrderDirection:        req.OrderDirection,
		Fields:                req.Fields,
		ImageThumbnailProcess: req.ImageThumbnailProcess,
		ImageURLProcess:       req.ImageURLProcess,
		VideoThumbnailProcess: req.VideoThumbnailProcess,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	items := make([]fileListItem, 0, len(res.Items))
	for _, item := range res.Items {
		items = append(items, fileListItem{
			Category:       item.Category,
			ContentHash:    item.ContentHash,
			FileExtension:  item.FileExtension,
			CreatedAt:      item.CreatedAt,
			DriveID:        item.DriveID,
			FileID:         item.FileID,
			MimeType:       item.MimeType,
			Name:           item.Name,
			ParentFileID:   item.ParentFileID,
			PunishFlag:     item.PunishFlag,
			Size:           item.Size,
			Starred:        item.Starred,
			SyncDeviceFlag: item.SyncDeviceFlag,
			SyncFlag:       item.SyncFlag,
			SyncMeta:       item.SyncMeta,
			Type:           item.Type,
			UpdatedAt:      item.UpdatedAt,
			URL:            item.URL,
			UserMeta:       item.UserMeta,
			UserTags:       item.UserTags,
		})
	}

	response.JSON(c, http.StatusOK, fileListResponse{
		Items:      items,
		NextMarker: res.NextMarker,
	})
}

func (h *Handler) getFile(c *gin.Context) {
	var req fileGetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.GetFile(c.Request.Context(), service.GetFileRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, fileGetResponse{
		DriveID:                     res.DriveID,
		DomainID:                    res.DomainID,
		FileID:                      res.FileID,
		Name:                        res.Name,
		Type:                        res.Type,
		ContentType:                 res.ContentType,
		CreatedAt:                   res.CreatedAt,
		UpdatedAt:                   res.UpdatedAt,
		Hidden:                      res.Hidden,
		Starred:                     res.Starred,
		Status:                      res.Status,
		ParentFileID:                res.ParentFileID,
		EncryptMode:                 res.EncryptMode,
		MetaNamePunishFlag:          res.MetaNamePunishFlag,
		MetaNameInvestigationStatus: res.MetaNameInvestigationStatus,
		CreatorType:                 res.CreatorType,
		CreatorID:                   res.CreatorID,
		LastModifierType:            res.LastModifierType,
		LastModifierID:              res.LastModifierID,
		SyncFlag:                    res.SyncFlag,
		SyncDeviceFlag:              res.SyncDeviceFlag,
		SyncMeta:                    res.SyncMeta,
		Trashed:                     res.Trashed,
		DownloadURL:                 res.DownloadURL,
		URL:                         res.URL,
	})
}

func (h *Handler) getFilePath(c *gin.Context) {
	var req fileGetPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.GetFilePath(c.Request.Context(), service.GetFilePathRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	items := make([]fileGetPathItem, 0, len(res.Items))
	for _, item := range res.Items {
		items = append(items, fileGetPathItem{
			Trashed:      item.Trashed,
			DriveID:      item.DriveID,
			FileID:       item.FileID,
			CreatedAt:    item.CreatedAt,
			DomainID:     item.DomainID,
			EncryptMode:  item.EncryptMode,
			Hidden:       item.Hidden,
			Name:         item.Name,
			ParentFileID: item.ParentFileID,
			Starred:      item.Starred,
			Status:       item.Status,
			Type:         item.Type,
			UpdatedAt:    item.UpdatedAt,
			SyncFlag:     item.SyncFlag,
		})
	}

	response.JSON(c, http.StatusOK, fileGetPathResponse{Items: items})
}

func (h *Handler) getFolderSizeInfo(c *gin.Context) {
	var req fileGetFolderSizeInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.GetFolderSizeInfo(c.Request.Context(), service.GetFolderSizeInfoRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, fileGetFolderSizeInfoResponse{
		Size:           res.Size,
		FolderCount:    res.FolderCount,
		FileCount:      res.FileCount,
		DisplaySummary: res.DisplaySummary,
	})
}

func (h *Handler) createWithFolders(c *gin.Context) {
	var req createWithFoldersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	partInfo := make([]service.UploadPartRequest, 0, len(req.PartInfoList))
	for _, part := range req.PartInfoList {
		partInfo = append(partInfo, service.UploadPartRequest{PartNumber: part.PartNumber})
	}

	res, err := h.service.CreateWithFolders(c.Request.Context(), service.CreateWithFoldersRequest{
		DriveID:         req.DriveID,
		PartInfoList:    partInfo,
		ParentFileID:    req.ParentFileID,
		Name:            req.Name,
		Type:            req.Type,
		CheckNameMode:   req.CheckNameMode,
		Size:            req.Size,
		CreateScene:     req.CreateScene,
		DeviceName:      req.DeviceName,
		LocalModifiedAt: req.LocalModifiedAt,
		PreHash:         req.PreHash,
		ContentType:     req.ContentType,
		ChunkSize:       req.ChunkSize,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, createWithFoldersResponse{
		ParentFileID: res.ParentFileID,
		PartInfoList: toUploadPartInfoResponse(res.PartInfoList),
		UploadID:     res.UploadID,
		RapidUpload:  res.RapidUpload,
		Type:         res.Type,
		FileID:       res.FileID,
		RevisionID:   res.RevisionID,
		DomainID:     res.DomainID,
		DriveID:      res.DriveID,
		FileName:     res.FileName,
		EncryptMode:  res.EncryptMode,
		Location:     res.Location,
		CreatedAt:    res.CreatedAt,
		UpdatedAt:    res.UpdatedAt,
	})
}

func (h *Handler) getUploadURL(c *gin.Context) {
	var req getUploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	partInfo := make([]service.UploadPartRequest, 0, len(req.PartInfoList))
	for _, part := range req.PartInfoList {
		partInfo = append(partInfo, service.UploadPartRequest{PartNumber: part.PartNumber})
	}

	res, err := h.service.GetUploadURL(c.Request.Context(), service.GetUploadURLRequest{
		DriveID:      req.DriveID,
		UploadID:     req.UploadID,
		PartInfoList: partInfo,
		FileID:       req.FileID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, getUploadURLResponse{
		DomainID:     res.DomainID,
		DriveID:      res.DriveID,
		FileID:       res.FileID,
		PartInfoList: toUploadPartInfoResponse(res.PartInfoList),
		UploadID:     res.UploadID,
		CreateAt:     res.CreateAt,
	})
}

func (h *Handler) completeFile(c *gin.Context) {
	var req completeFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.CompleteFile(c.Request.Context(), service.CompleteFileRequest{
		DriveID:  req.DriveID,
		UploadID: req.UploadID,
		FileID:   req.FileID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, completeFileResponse{
		DriveID:                     res.DriveID,
		DomainID:                    res.DomainID,
		FileID:                      res.FileID,
		Name:                        res.Name,
		Type:                        res.Type,
		ContentType:                 res.ContentType,
		CreatedAt:                   res.CreatedAt,
		UpdatedAt:                   res.UpdatedAt,
		ModifiedAt:                  res.ModifiedAt,
		FileExtension:               res.FileExtension,
		Hidden:                      res.Hidden,
		Size:                        res.Size,
		Starred:                     res.Starred,
		Status:                      res.Status,
		UserMeta:                    res.UserMeta,
		UploadID:                    res.UploadID,
		ParentFileID:                res.ParentFileID,
		CRC64Hash:                   res.CRC64Hash,
		ContentHash:                 res.ContentHash,
		ContentHashName:             res.ContentHashName,
		Category:                    res.Category,
		EncryptMode:                 res.EncryptMode,
		MetaNamePunishFlag:          res.MetaNamePunishFlag,
		MetaNameInvestigationStatus: res.MetaNameInvestigationStatus,
		CreatorType:                 res.CreatorType,
		CreatorID:                   res.CreatorID,
		LastModifierType:            res.LastModifierType,
		LastModifierID:              res.LastModifierID,
		UserTags:                    res.UserTags,
		LocalModifiedAt:             res.LocalModifiedAt,
		RevisionID:                  res.RevisionID,
		RevisionVersion:             res.RevisionVersion,
		SyncFlag:                    res.SyncFlag,
		SyncDeviceFlag:              res.SyncDeviceFlag,
		SyncMeta:                    res.SyncMeta,
		Location:                    res.Location,
		ContentURI:                  res.ContentURI,
	})
}

func (h *Handler) listMoveTargets(c *gin.Context) {
	var req listUploadMoveTargetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.ListMoveTargets(c.Request.Context(), service.ListMoveTargetsRequest{
		DriveID:         req.DriveID,
		ExcludeFolderID: req.ExcludeFolderID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	folders := make([]uploadFolderRecord, 0, len(res.Folders))
	for _, item := range res.Folders {
		folders = append(folders, toUploadFolderRecord(item))
	}

	response.JSON(c, http.StatusOK, listUploadMoveTargetsResponse{Folders: folders})
}

func (h *Handler) updateFile(c *gin.Context) {
	var req fileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.Update(c.Request.Context(), service.UpdateRequest{
		DriveID:       req.DriveID,
		FileID:        req.FileID,
		Name:          req.Name,
		CheckNameMode: req.CheckNameMode,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, fileGetResponse{
		DriveID:                     res.File.DriveID,
		DomainID:                    res.File.DomainID,
		FileID:                      res.File.FileID,
		Name:                        res.File.Name,
		Type:                        res.File.Type,
		ContentType:                 res.File.ContentType,
		CreatedAt:                   res.File.CreatedAt,
		UpdatedAt:                   res.File.UpdatedAt,
		Hidden:                      res.File.Hidden,
		Starred:                     res.File.Starred,
		Status:                      res.File.Status,
		ParentFileID:                res.File.ParentFileID,
		EncryptMode:                 res.File.EncryptMode,
		MetaNamePunishFlag:          res.File.MetaNamePunishFlag,
		MetaNameInvestigationStatus: res.File.MetaNameInvestigationStatus,
		CreatorType:                 res.File.CreatorType,
		CreatorID:                   res.File.CreatorID,
		LastModifierType:            res.File.LastModifierType,
		LastModifierID:              res.File.LastModifierID,
		SyncFlag:                    res.File.SyncFlag,
		SyncDeviceFlag:              res.File.SyncDeviceFlag,
		SyncMeta:                    res.File.SyncMeta,
		Trashed:                     res.File.Trashed,
		DownloadURL:                 res.File.DownloadURL,
		URL:                         res.File.URL,
	})
}

func (h *Handler) getLatestAsyncTask(c *gin.Context) {
	var req fileGetLatestAsyncTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.GetLatestAsyncTask(c.Request.Context(), service.GetLatestAsyncTaskRequest{
		DriveID: req.DriveID,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, fileGetLatestAsyncTaskResponse{
		TotalProcess:         res.TotalProcess,
		TotalFailedProcess:   res.TotalFailedProcess,
		TotalSkippedProcess:  res.TotalSkippedProcess,
		TotalConsumedProcess: res.TotalConsumedProcess,
	})
}

func (h *Handler) recycleBinTrash(c *gin.Context) {
	var req recycleBinTrashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if err := h.service.RecycleBinTrash(c.Request.Context(), service.RecycleBinTrashRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
	}); err != nil {
		h.writeServiceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) recycleBinList(c *gin.Context) {
	var req recycleBinListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.RecycleBinList(c.Request.Context(), service.RecycleBinListRequest{
		DriveID:        req.DriveID,
		Limit:          req.Limit,
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
		Marker:         req.Marker,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	items := make([]recycleBinListItem, 0, len(res.Items))
	for _, item := range res.Items {
		items = append(items, recycleBinListItem{
			Name:            item.Name,
			Type:            item.Type,
			Hidden:          item.Hidden,
			Status:          item.Status,
			Starred:         item.Starred,
			ParentFileID:    item.ParentFileID,
			DriveID:         item.DriveID,
			FileID:          item.FileID,
			EncryptMode:     item.EncryptMode,
			DomainID:        item.DomainID,
			CreatedAt:       item.CreatedAt,
			UpdatedAt:       item.UpdatedAt,
			TrashedAt:       item.TrashedAt,
			GMTExpired:      item.GMTExpired,
			Category:        item.Category,
			URL:             item.URL,
			Size:            item.Size,
			FileExtension:   item.FileExtension,
			ContentHash:     item.ContentHash,
			ContentHashName: item.ContentHashName,
			PunishFlag:      item.PunishFlag,
		})
	}

	response.JSON(c, http.StatusOK, recycleBinListResponse{
		Items:      items,
		NextMarker: res.NextMarker,
	})
}

func (h *Handler) recycleBinRestore(c *gin.Context) {
	var req recycleBinRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if err := h.service.RecycleBinRestore(c.Request.Context(), service.RecycleBinRestoreRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
	}); err != nil {
		h.writeServiceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) deleteFile(c *gin.Context) {
	var req fileDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if err := h.service.DeleteFile(c.Request.Context(), service.DeleteFileRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
	}); err != nil {
		h.writeServiceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) batch(c *gin.Context) {
	var req uploadBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	requests := make([]service.BatchRequestItem, 0, len(req.Requests))
	for _, item := range req.Requests {
		requests = append(requests, service.BatchRequestItem{
			ID:      item.ID,
			Method:  item.Method,
			URL:     item.URL,
			Headers: item.Headers,
			Body:    item.Body,
		})
	}

	res, err := h.service.Batch(c.Request.Context(), service.BatchRequest{
		Resource: req.Resource,
		Requests: requests,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	items := make([]uploadBatchResponseItem, 0, len(res.Responses))
	for _, item := range res.Responses {
		items = append(items, uploadBatchResponseItem{
			ID:     item.ID,
			Status: item.Status,
			Body:   item.Body,
		})
	}
	response.JSON(c, http.StatusOK, uploadBatchResponse{Responses: items})
}

func (h *Handler) getFileAccessURL(c *gin.Context) {
	var req getFileAccessURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "InvalidParameter", fmt.Sprintf("invalid request body: %v", err))
		return
	}

	res, err := h.service.GetFileAccessURL(c.Request.Context(), service.GetFileAccessURLRequest{
		DriveID: req.DriveID,
		FileID:  req.FileID,
		Mode:    req.Mode,
	})
	if err != nil {
		h.writeServiceError(c, err)
		return
	}

	response.JSON(c, http.StatusOK, getFileAccessURLResponse{
		File:             toUploadedFileRecord(res.File),
		URL:              res.URL,
		Disposition:      res.Disposition,
		ExpiresInSeconds: res.ExpiresInSeconds,
	})
}

func (h *Handler) writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidArgument):
		response.Error(c, http.StatusBadRequest, "InvalidParameter", err.Error())
	case errors.Is(err, service.ErrNotFound):
		response.Error(c, http.StatusNotFound, "NotFound.File", err.Error())
	case errors.Is(err, service.ErrConflict):
		response.Error(c, http.StatusConflict, "AlreadyExists.File", err.Error())
	default:
		requestID, _ := c.Get("request_id")
		h.logger.Error("request failed", "request_id", requestID, "error", err, "path", c.FullPath())
		response.Error(c, http.StatusInternalServerError, "InternalError", "internal server error")
	}
}
