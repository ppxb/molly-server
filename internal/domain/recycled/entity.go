package recycled

import "time"

// Recycled 回收站记录。
//
// 设计说明：
// - 每条 Recycled 对应一条已软删除的 UserFile（通过 UserFileID 关联）
// - 彻底删除时：检查 FileInfo 的活跃引用数，决定是否同步删除物理文件
// - 还原时：清除 UserFile.DeletedAt，删除本条 Recycled 记录
type Recycled struct {
	ID         string
	UserID     string
	UserFileID string // → UserFile.ID (uf_id)，软删除前的用户文件标识
	FileID     string // → FileInfo.ID，冗余存储方便查物理文件
	CreatedAt  time.Time
}

// ExpiresAt 计算过期时间（超期自动清理）。
func (r *Recycled) ExpiresAt(retentionDays int) time.Time {
	return r.CreatedAt.AddDate(0, 0, retentionDays)
}

// IsExpired 是否已超过保留期。
func (r *Recycled) IsExpired(retentionDays int) bool {
	return time.Now().After(r.ExpiresAt(retentionDays))
}
