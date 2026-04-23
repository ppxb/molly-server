package file

import (
	"context"
	"time"
)

// FileInfoRepository 物理文件数据访问接口。
type FileInfoRepository interface {
	Create(ctx context.Context, f *FileInfo) error
	GetByID(ctx context.Context, id string) (*FileInfo, error)
	GetByChunkSignature(ctx context.Context, signature string, size int64) (*FileInfo, error)
	GetByHash(ctx context.Context, hash string) (*FileInfo, error)
	Update(ctx context.Context, f *FileInfo) error
	Delete(ctx context.Context, id string) error
}

// UserFileRepository 用户-文件逻辑关联接口。
type UserFileRepository interface {
	Create(ctx context.Context, uf *UserFile) error
	GetByID(ctx context.Context, id string) (*UserFile, error) // by uf_id
	GetByUserAndUfID(ctx context.Context, userID, ufID string) (*UserFile, error)
	Update(ctx context.Context, uf *UserFile) error
	SoftDelete(ctx context.Context, id string) error // 设置 deleted_at
	ListByVirtualPath(ctx context.Context, userID, pathID string, offset, limit int) ([]*UserFile, error)
	CountByVirtualPath(ctx context.Context, userID, pathID string) (int64, error)
	Search(ctx context.Context, userID, keyword string, offset, limit int) ([]*UserFile, error)
	CountSearch(ctx context.Context, userID, keyword string) (int64, error)
	ListPublic(ctx context.Context, offset, limit int) ([]*UserFile, error)
	CountPublic(ctx context.Context) (int64, error)
}

// VirtualPathRepository 虚拟目录数据访问接口。
type VirtualPathRepository interface {
	Create(ctx context.Context, vp *VirtualPath) error
	GetByID(ctx context.Context, id int) (*VirtualPath, error)
	GetByPath(ctx context.Context, userID, path string) (*VirtualPath, error)
	GetRoot(ctx context.Context, userID string) (*VirtualPath, error)
	Update(ctx context.Context, vp *VirtualPath) error
	Delete(ctx context.Context, id int) error
	ListSubFolders(ctx context.Context, userID string, parentID int, offset, limit int) ([]*VirtualPath, error)
	CountSubFolders(ctx context.Context, userID string, parentID int) (int64, error)
	ListAll(ctx context.Context, userID string) ([]*VirtualPath, error)
}

// UploadTaskRepository 上传任务数据访问接口。
type UploadTaskRepository interface {
	Create(ctx context.Context, t *UploadTask) error
	GetByID(ctx context.Context, id string) (*UploadTask, error)
	Update(ctx context.Context, t *UploadTask) error
	Delete(ctx context.Context, id string) error
	ListByUser(ctx context.Context, userID string, offset, limit int) ([]*UploadTask, error)
	CountByUser(ctx context.Context, userID string) (int64, error)
	ListPendingByUser(ctx context.Context, userID string) ([]*UploadTask, error) // pending + uploading
	ListExpiredByUser(ctx context.Context, userID string) ([]*UploadTask, error)
	DeleteExpired(ctx context.Context, before time.Time) (int64, error) // 系统定时清理
}
