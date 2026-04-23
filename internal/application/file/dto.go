package file

import "time"

// ── 上传 ──────────────────────────────────────────────────────

type PrecheckReq struct {
	FileName       string   `json:"file_name"       binding:"required"`
	FileSize       int64    `json:"file_size"       binding:"required,min=1"`
	ChunkSignature string   `json:"chunk_signature"`
	PathID         string   `json:"path_id"         binding:"required"`
	FilesMd5       []string `json:"files_md5"` // 前三分片 MD5，用于秒传校验
}

type PrecheckResp struct {
	PrecheckID  string   `json:"precheck_id"`
	AlreadyDone bool     `json:"already_done"` // true = 秒传成功，无需再上传
	UploadedMd5 []string `json:"uploaded_md5"` // 已上传的分片 MD5（断点续传用）
}

type UploadChunkReq struct {
	PrecheckID   string `form:"precheck_id"  binding:"required"`
	ChunkIndex   *int   `form:"chunk_index"` // nil = 小文件直传
	TotalChunks  *int   `form:"total_chunks"`
	ChunkMD5     string `form:"chunk_md5"`
	IsEnc        bool   `form:"is_enc"`
	FilePassword string `form:"file_password"`
}

type UploadResp struct {
	FileID     string `json:"file_id"`
	IsComplete bool   `json:"is_complete"`
	Uploaded   int    `json:"uploaded,omitempty"`
	Total      int    `json:"total,omitempty"`
}

// ── 文件列表 ──────────────────────────────────────────────────

type FileListReq struct {
	VirtualPath string `form:"virtualPath"`
	Type        string `form:"type"`
	SortBy      string `form:"sortBy"`
	Page        int    `form:"page"     binding:"required,min=1"`
	PageSize    int    `form:"pageSize" binding:"required,min=1,max=100"`
}

type FileListResp struct {
	Breadcrumbs []BreadcrumbItem `json:"breadcrumbs"`
	CurrentPath string           `json:"current_path"`
	Folders     []FolderItem     `json:"folders"`
	Files       []FileItem       `json:"files"`
	Total       int64            `json:"total"`
	Page        int              `json:"page"`
	PageSize    int              `json:"page_size"`
}

type BreadcrumbItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type FolderItem struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	CreatedTime time.Time `json:"created_time"`
}

type FileItem struct {
	FileID       string    `json:"file_id"` // uf_id
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	MimeType     string    `json:"mime_type"`
	IsEnc        bool      `json:"is_enc"`
	HasThumbnail bool      `json:"has_thumbnail"`
	IsPublic     bool      `json:"public"`
	CreatedAt    time.Time `json:"created_at"`
}

// ── 目录操作 ──────────────────────────────────────────────────

type MakeDirReq struct {
	ParentLevel string `json:"parent_level"` // 父目录 ID，0 或空 = 根目录
	DirPath     string `json:"dir_path"      binding:"required"`
}

type RenameDirReq struct {
	DirID      int    `json:"dir_id"      binding:"required"`
	NewDirName string `json:"new_dir_name" binding:"required"`
}

type DeleteDirReq struct {
	DirID int `json:"dir_id" binding:"required"`
}

// ── 文件操作 ──────────────────────────────────────────────────

type DeleteFilesReq struct {
	FileIDs []string `json:"file_ids" binding:"required,min=1"`
}

type DeleteFilesResp struct {
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

type MoveFileReq struct {
	FileID     string `json:"file_id"     binding:"required"`
	TargetPath string `json:"target_path" binding:"required"`
}

type RenameFileReq struct {
	FileID      string `json:"file_id"       binding:"required"`
	NewFileName string `json:"new_file_name" binding:"required"`
}

type SetPublicReq struct {
	FileID string `json:"file_id" binding:"required"`
	Public bool   `json:"public"`
}

// ── 搜索 ──────────────────────────────────────────────────────

type SearchReq struct {
	Keyword  string `form:"keyword"  binding:"required"`
	Type     string `form:"type"`
	SortBy   string `form:"sortBy"`
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
}

// ── 上传任务管理 ──────────────────────────────────────────────

type UploadProgressReq struct {
	PrecheckID string `form:"precheck_id" binding:"required"`
}

type UploadProgressResp struct {
	PrecheckID  string   `json:"precheck_id"`
	FileName    string   `json:"file_name"`
	FileSize    int64    `json:"file_size"`
	Uploaded    int      `json:"uploaded"`
	Total       int      `json:"total"`
	Progress    float64  `json:"progress"`
	UploadedMd5 []string `json:"uploaded_md5"`
	IsComplete  bool     `json:"is_complete"`
}

type TaskListReq struct {
	Page     int `form:"page"     binding:"required,min=1"`
	PageSize int `form:"pageSize" binding:"required,min=1,max=100"`
}

type TaskItem struct {
	ID             string    `json:"id"`
	FileName       string    `json:"file_name"`
	FileSize       int64     `json:"file_size"`
	ChunkSize      int64     `json:"chunk_size"`
	TotalChunks    int       `json:"total_chunks"`
	UploadedChunks int       `json:"uploaded_chunks"`
	Progress       float64   `json:"progress"`
	Status         string    `json:"status"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	PathID         string    `json:"path_id"`
	CreatedAt      time.Time `json:"create_time"`
	UpdatedAt      time.Time `json:"update_time"`
	ExpiresAt      time.Time `json:"expire_time"`
}

type TaskListResp struct {
	Tasks    []TaskItem `json:"tasks"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

type DeleteTaskReq struct {
	TaskID string `json:"task_id" binding:"required"`
}

type RenewTaskReq struct {
	TaskID string `json:"task_id" binding:"required"`
	Days   int    `json:"days"`
}
