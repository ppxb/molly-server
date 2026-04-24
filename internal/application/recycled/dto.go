package recycled

import "time"

type ListReq struct {
	Page     int `form:"page"     binding:"required,min=1"`
	PageSize int `form:"pageSize" binding:"required,min=1,max=100"`
}

type ListResp struct {
	Items    []Item `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type Item struct {
	RecycledID   string    `json:"recycled_id"`
	UserFileID   string    `json:"file_id"` // 前端惯用 file_id 表示 uf_id
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	MimeType     string    `json:"mime_type"`
	IsEnc        bool      `json:"is_enc"`
	HasThumbnail bool      `json:"has_thumbnail"`
	DeletedAt    time.Time `json:"deleted_at"`
}

type RestoreReq struct {
	RecycledID string `json:"recycled_id" binding:"required"`
}

type DeleteReq struct {
	RecycledID string `json:"recycled_id" binding:"required"`
}

type EmptyResp struct {
	Deleted int `json:"deleted"`
	Failed  int `json:"failed"`
}
