package file

import "time"

// ── FileInfo 物理文件（存储层） ────────────────────────────────
// 多个用户可以引用同一个 FileInfo（秒传本质就是共享同一条记录）

type FileInfo struct {
	ID string

	// 命名
	Name       string // 原始文件名（首次上传时的名称）
	RandomName string // 磁盘存储名（随机生成，防冲突）

	// 大小与类型
	Size int64
	Mime string

	// 存储路径
	Path    string // 明文路径
	EncPath string // 加密文件路径

	// 缩略图
	ThumbnailImg string

	// 哈希与秒传
	FileHash        string // 全量哈希，用于最终秒传确认
	FileEncHash     string // 加密文件哈希
	ChunkSignature  string // 分片签名（快速预检，比全量哈希快）
	FirstChunkHash  string
	SecondChunkHash string
	ThirdChunkHash  string
	HasFullHash     bool

	// 加密
	IsEnc bool

	// 分块存储
	IsChunk    bool
	ChunkCount int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ── UserFile 用户-文件逻辑关联 ────────────────────────────────
// 每个 UserFile 是用户看到的"一个文件"，对应一个 FileInfo（可共享）。
// 软删除：DeletedAt != nil 表示已移入回收站。

type UserFile struct {
	ID          string // uf_id：用户文件唯一 ID
	UserID      string
	FileID      string // → FileInfo.ID
	FileName    string // 用户自定义名称（与 FileInfo.Name 可以不同）
	VirtualPath string // 所在目录 ID（字符串化的 VirtualPath.ID）
	IsPublic    bool
	CreatedAt   time.Time
	DeletedAt   *time.Time // nil = 正常，非 nil = 已删除
}

// IsDeleted 判断是否已软删除。
func (f *UserFile) IsDeleted() bool { return f.DeletedAt != nil }

// ── VirtualPath 虚拟目录节点 ─────────────────────────────────

type VirtualPath struct {
	ID          int
	UserID      string
	Path        string // 目录名，如 "/documents"
	IsFile      bool
	IsDir       bool
	ParentLevel string // 父目录 ID（字符串），空字符串表示根目录
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsRoot 判断是否为根目录（无父级）。
func (v *VirtualPath) IsRoot() bool { return v.ParentLevel == "" }

// ── UploadTask 分片上传任务 ───────────────────────────────────

type UploadTask struct {
	ID             string // precheck_id，前端断点续传的锚点
	UserID         string
	FileName       string
	FileSize       int64
	ChunkSize      int64
	TotalChunks    int
	UploadedChunks int
	ChunkSignature string
	PathID         string // 目标目录 ID
	TempDir        string // 服务端临时分片目录
	Status         UploadStatus
	ErrorMessage   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time
}

// Progress 返回 0-100 的上传进度。
func (t *UploadTask) Progress() float64 {
	if t.TotalChunks == 0 {
		return 0
	}
	return float64(t.UploadedChunks) / float64(t.TotalChunks) * 100
}

// IsComplete 所有分片均已到位。
func (t *UploadTask) IsComplete() bool {
	return t.TotalChunks > 0 && t.UploadedChunks >= t.TotalChunks
}

// IsExpired 任务是否已超过有效期。
func (t *UploadTask) IsExpired() bool { return time.Now().After(t.ExpiresAt) }

// ── UploadStatus ─────────────────────────────────────────────

type UploadStatus string

const (
	UploadStatusPending   UploadStatus = "pending"
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusCompleted UploadStatus = "completed"
	UploadStatusFailed    UploadStatus = "failed"
	UploadStatusAborted   UploadStatus = "aborted"
)

// ── Breadcrumb 面包屑导航项 ───────────────────────────────────

type Breadcrumb struct {
	ID   int
	Name string
	Path string // 字符串化的 ID，供前端路由用
}
