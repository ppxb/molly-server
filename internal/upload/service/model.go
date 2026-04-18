package service

import (
	"context"
	"errors"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
)

type Service interface {
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
	CreateWithFolders(ctx context.Context, req CreateWithFoldersRequest) (CreateWithFoldersResponse, error)
	GetUploadURL(ctx context.Context, req GetUploadURLRequest) (GetUploadURLResponse, error)
	GetFile(ctx context.Context, req GetFileRequest) (GetFileResponse, error)
	GetFilePath(ctx context.Context, req GetFilePathRequest) (GetFilePathResponse, error)
	CompleteFile(ctx context.Context, req CompleteFileRequest) (CompleteFileResponse, error)
	List(ctx context.Context, req ListRequest) (ListResponse, error)
	RecycleBinTrash(ctx context.Context, req RecycleBinTrashRequest) error
	RecycleBinList(ctx context.Context, req RecycleBinListRequest) (RecycleBinListResponse, error)
	RecycleBinRestore(ctx context.Context, req RecycleBinRestoreRequest) error
	DeleteFile(ctx context.Context, req DeleteFileRequest) error
	Update(ctx context.Context, req UpdateRequest) (UpdateResponse, error)
	GetLatestAsyncTask(ctx context.Context, req GetLatestAsyncTaskRequest) (GetLatestAsyncTaskResponse, error)
	ListMoveTargets(ctx context.Context, req ListMoveTargetsRequest) (ListMoveTargetsResponse, error)
	MoveFile(ctx context.Context, req MoveFileRequest) (MoveFileResponse, error)
	MoveFolder(ctx context.Context, req MoveFolderRequest) (MoveFolderResponse, error)
	Batch(ctx context.Context, req BatchRequest) (BatchResponse, error)
	GetFileAccessURL(ctx context.Context, req GetFileAccessURLRequest) (GetFileAccessURLResponse, error)
}

type UploadPartRequest struct {
	PartNumber int
}

type UploadPartInfo struct {
	PartNumber        int
	UploadURL         string
	InternalUploadURL string
	ContentType       string
}

type SearchRequest struct {
	Query   string
	OrderBy string
	Limit   int
	DriveID string
}

type SearchItem struct {
	DriveID      string
	FileID       string
	ParentFileID string
	Name         string
	Type         string
	Size         int64
	CreatedAt    string
	UpdatedAt    string
}

type SearchResponse struct {
	Items      []SearchItem
	NextMarker string
}

type CreateWithFoldersRequest struct {
	DriveID         string
	PartInfoList    []UploadPartRequest
	ParentFileID    string
	Name            string
	Type            string
	CheckNameMode   string
	Size            int64
	CreateScene     string
	DeviceName      string
	LocalModifiedAt string
	PreHash         string
	ContentType     string
	ChunkSize       int64
}

type CreateWithFoldersResponse struct {
	ParentFileID string
	PartInfoList []UploadPartInfo
	UploadID     string
	RapidUpload  bool
	Type         string
	FileID       string
	RevisionID   string
	DomainID     string
	DriveID      string
	FileName     string
	EncryptMode  string
	Location     string
	CreatedAt    string
	UpdatedAt    string
}

type GetUploadURLRequest struct {
	DriveID      string
	UploadID     string
	PartInfoList []UploadPartRequest
	FileID       string
}

type GetUploadURLResponse struct {
	DomainID     string
	DriveID      string
	FileID       string
	PartInfoList []UploadPartInfo
	UploadID     string
	CreateAt     string
}

type CompleteFileRequest struct {
	DriveID  string
	UploadID string
	FileID   string
}

type CompleteFileResponse struct {
	DriveID                     string
	DomainID                    string
	FileID                      string
	Name                        string
	Type                        string
	ContentType                 string
	CreatedAt                   string
	UpdatedAt                   string
	ModifiedAt                  string
	FileExtension               string
	Hidden                      bool
	Size                        int64
	Starred                     bool
	Status                      string
	UserMeta                    string
	UploadID                    string
	ParentFileID                string
	CRC64Hash                   string
	ContentHash                 string
	ContentHashName             string
	Category                    string
	EncryptMode                 string
	MetaNamePunishFlag          int
	MetaNameInvestigationStatus int
	CreatorType                 string
	CreatorID                   string
	LastModifierType            string
	LastModifierID              string
	UserTags                    map[string]string
	LocalModifiedAt             string
	RevisionID                  string
	RevisionVersion             int
	SyncFlag                    bool
	SyncDeviceFlag              bool
	SyncMeta                    string
	Location                    string
	ContentURI                  string
}

type GetFileRequest struct {
	DriveID string
	FileID  string
}

type GetFileResponse struct {
	DriveID                     string
	DomainID                    string
	FileID                      string
	Name                        string
	Type                        string
	ContentType                 string
	CreatedAt                   string
	UpdatedAt                   string
	Hidden                      bool
	Starred                     bool
	Status                      string
	ParentFileID                string
	EncryptMode                 string
	MetaNamePunishFlag          int
	MetaNameInvestigationStatus int
	CreatorType                 string
	CreatorID                   string
	LastModifierType            string
	LastModifierID              string
	SyncFlag                    bool
	SyncDeviceFlag              bool
	SyncMeta                    string
	Trashed                     bool
	DownloadURL                 string
	URL                         string
}

type GetFilePathRequest struct {
	DriveID string
	FileID  string
}

type GetFilePathItem struct {
	Trashed      bool
	DriveID      string
	FileID       string
	CreatedAt    string
	DomainID     string
	EncryptMode  string
	Hidden       bool
	Name         string
	ParentFileID string
	Starred      bool
	Status       string
	Type         string
	UpdatedAt    string
	SyncFlag     bool
}

type GetFilePathResponse struct {
	Items []GetFilePathItem
}

type ListRequest struct {
	DriveID               string
	ParentFileID          string
	Limit                 int
	All                   bool
	URLExpireSec          int
	OrderBy               string
	OrderDirection        string
	Fields                string
	ImageThumbnailProcess string
	ImageURLProcess       string
	VideoThumbnailProcess string
}

type ListItem struct {
	Category       string
	ContentHash    string
	FileExtension  string
	CreatedAt      string
	DriveID        string
	FileID         string
	MimeType       string
	Name           string
	ParentFileID   string
	PunishFlag     int
	Size           int64
	Starred        bool
	SyncDeviceFlag bool
	SyncFlag       bool
	SyncMeta       string
	Type           string
	UpdatedAt      string
	URL            string
	UserMeta       string
	UserTags       map[string]string
}

type ListResponse struct {
	Items      []ListItem
	NextMarker string
}

type RecycleBinTrashRequest struct {
	DriveID string
	FileID  string
}

type RecycleBinListRequest struct {
	DriveID        string
	Limit          int
	OrderBy        string
	OrderDirection string
	Marker         string
}

type RecycleBinListItem struct {
	Name            string
	Type            string
	Hidden          bool
	Status          string
	Starred         bool
	ParentFileID    string
	DriveID         string
	FileID          string
	EncryptMode     string
	DomainID        string
	CreatedAt       string
	UpdatedAt       string
	TrashedAt       string
	GMTExpired      string
	Category        string
	URL             string
	Size            int64
	FileExtension   string
	ContentHash     string
	ContentHashName string
	PunishFlag      int
}

type RecycleBinListResponse struct {
	Items      []RecycleBinListItem
	NextMarker string
}

type RecycleBinRestoreRequest struct {
	DriveID string
	FileID  string
}

type DeleteFileRequest struct {
	DriveID string
	FileID  string
}

type UploadFolderRecord struct {
	ID         string
	FolderName string
	ParentID   *string
	FolderPath string
	ParentPath string
	CreatedAt  string
	UpdatedAt  string
}

type UploadStrategy string

const (
	UploadStrategySingle    UploadStrategy = "single"
	UploadStrategyMultipart UploadStrategy = "multipart"
	UploadStrategyInstant   UploadStrategy = "instant"
)

type UploadedFileRecord struct {
	ID             string
	FileName       string
	FileExtension  string
	FolderID       string
	FolderPath     string
	ContentType    string
	FileSize       int64
	FileHash       string
	FileSampleHash string
	ObjectKey      string
	Bucket         string
	Strategy       UploadStrategy
	CreatedAt      string
	UpdatedAt      string
}

type ListMoveTargetsRequest struct {
	DriveID         string
	ExcludeFolderID string
}

type ListMoveTargetsResponse struct {
	Folders []UploadFolderRecord
}

type UpdateRequest struct {
	DriveID       string
	FileID        string
	Name          string
	CheckNameMode string
}

type UpdateResponse struct {
	File GetFileResponse
}

type GetLatestAsyncTaskRequest struct {
	DriveID string
}

type GetLatestAsyncTaskResponse struct {
	TotalProcess         int64
	TotalFailedProcess   int64
	TotalSkippedProcess  int64
	TotalConsumedProcess int64
}

type MoveFileRequest struct {
	DriveID        string
	FileID         string
	TargetFolderID string
}

type MoveFileResponse struct {
	File UploadedFileRecord
}

type MoveFolderRequest struct {
	DriveID        string
	FolderID       string
	TargetParentID string
}

type MoveFolderResponse struct {
	Folder UploadFolderRecord
}

type BatchRequestItem struct {
	ID      string
	Method  string
	URL     string
	Headers map[string]string
	Body    map[string]any
}

type BatchRequest struct {
	Resource string
	Requests []BatchRequestItem
}

type BatchResponseItem struct {
	ID     string
	Status int
	Body   map[string]any
}

type BatchResponse struct {
	Responses []BatchResponseItem
}

type GetFileAccessURLRequest struct {
	DriveID string
	FileID  string
	Mode    string
}

type GetFileAccessURLResponse struct {
	File             UploadedFileRecord
	URL              string
	Disposition      string
	ExpiresInSeconds int
}
