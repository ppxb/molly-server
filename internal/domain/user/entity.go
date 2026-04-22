package user

import (
	"time"
)

type State int

type User struct {
	ID           string
	NickName     string
	UserName     string
	Password     string // bcrypt hash，永远不序列化为 JSON
	Email        string
	Phone        string
	GroupID      int
	Space        int64 // 总存储配额（字节）
	FreeSpace    int64 // 剩余可用空间（字节）
	FilePassword string
	State        State
	CreatedAt    time.Time
}

const (
	StateActive   State = 0
	StateDisabled State = 1
)

func (u *User) IsActive() bool {
	return u.State == StateActive
}
func (u *User) IsDisabled() bool {
	return u.State == StateDisabled
}

// Session 登录会话的权限快照，由 middleware 注入 gin.Context。
// 每次请求从 DB 重新加载，确保权限变更即时生效。
type Session struct {
	UserID   string
	UserName string
	GroupID  int
	Powers   []string // Power.Characteristic 集合，如 ["file:upload","admin:manage"]
}

// IsAdmin 管理员判断逻辑收归 domain，middleware 不硬编码魔法数字。
func (s *Session) IsAdmin() bool {
	return s.GroupID == 1
}

// HasPower 检查是否拥有指定权限标识符。
func (s *Session) HasPower(characteristic string) bool {
	for _, p := range s.Powers {
		if p == characteristic {
			return true
		}
	}
	return false
}

type Group struct {
	ID        int
	Name      string
	IsDefault bool
	Space     int64
}

type Power struct {
	ID             int
	Name           string
	Description    string
	Characteristic string // 唯一标识符，如 "file:upload"
}

// APIKey API 密钥实体。
type APIKey struct {
	ID          int
	UserID      string
	Key         string
	PrivateKey  string     // RSA 私钥，用于解密签名
	S3SecretKey string     // HMAC-SHA256 密钥
	ExpiresAt   *time.Time // nil 表示永不过期
	CreatedAt   time.Time
}
