package user

import (
	"context"
)

// Repository 用户数据访问接口（Port）。
//
// 定义在 domain 层，infrastructure 层实现它。
// 依赖方向：infrastructure → domain（永远从外向内）。
type Repository interface {
	// GetByUserName 按用户名精确查找，不存在返回 ErrNotFound。
	GetByUserName(ctx context.Context, userName string) (*User, error)

	// GetByID 按主键查找。
	GetByID(ctx context.Context, id string) (*User, error)

	// Create 写入新用户，返回完整实体（含 DB 生成的字段）。
	Create(ctx context.Context, u *User) (*User, error)

	// Update 更新用户可变字段。
	Update(ctx context.Context, u *User) error

	// GetGroupWithPowers 加载用户组及其全部权限，用于构建 Session。
	GetGroupWithPowers(ctx context.Context, groupID int) (*Group, []Power, error)

	// GetAPIKey 按 key 字符串查找 API Key 记录。
	GetAPIKey(ctx context.Context, key string) (*APIKey, error)
}
