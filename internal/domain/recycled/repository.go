package recycled

import (
	"context"
	"time"
)

type Repository interface {
	Create(ctx context.Context, r *Recycled) error
	GetByID(ctx context.Context, id string) (*Recycled, error)
	GetByUserFileID(ctx context.Context, userFileID string) (*Recycled, error)
	ListByUser(ctx context.Context, userID string, offset, limit int) ([]*Recycled, error)
	CountByUser(ctx context.Context, userID string) (int64, error)
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID string) error                  // 清空回收站
	ListExpired(ctx context.Context, before time.Time) ([]*Recycled, error) // 定时清理
	// CountActiveUserFiles 统计 file_id 对应的活跃（未软删除）UserFile 数量。
	// 永久删除时用于判断是否还有其他用户持有该物理文件。
	CountActiveUserFiles(ctx context.Context, fileID string) (int64, error)
}
