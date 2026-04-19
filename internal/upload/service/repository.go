package service

import (
	"context"
	"time"

	"molly-server/internal/upload/repository"
)

type Repository interface {
	EnsureDrive(ctx context.Context, driveID string) error
	SearchEntries(ctx context.Context, params repository.SearchEntriesParams) ([]repository.EntryRecord, error)
	ListEntries(ctx context.Context, params repository.ListEntriesParams) ([]repository.EntryRecord, error)
	ExistsEntry(ctx context.Context, driveID, parentFileID, name string) (bool, error)
	CreateFolder(ctx context.Context, params repository.CreateFolderParams) (repository.EntryRecord, error)
	CreateFileWithUpload(ctx context.Context, params repository.CreateFileWithUploadParams) (repository.EntryRecord, error)
	GetEntryByFileID(ctx context.Context, driveID, fileID string) (repository.EntryRecord, error)
	GetEntryByParentAndName(ctx context.Context, driveID, parentFileID, name string) (repository.EntryRecord, error)
	GetSubtreeStats(ctx context.Context, driveID, folderID string) (repository.SubtreeStats, error)
	RenameEntry(ctx context.Context, driveID, fileID, newName string) (repository.EntryRecord, error)
	MoveEntry(ctx context.Context, driveID, fileID, targetParentFileID string) (repository.EntryRecord, error)
	TrashEntry(ctx context.Context, driveID, fileID, name, recycleBinParentID, trashedParentFileID string, trashedAt, expiredAt time.Time) (repository.EntryRecord, error)
	RestoreEntry(ctx context.Context, driveID, fileID, name, parentFileID string) (repository.EntryRecord, error)
	DeleteEntryTree(ctx context.Context, driveID, fileID string) ([]repository.EntryRecord, error)
	UpdateEntryHash(ctx context.Context, driveID, fileID, contentHash string) (repository.EntryRecord, error)
	GetUploadSession(ctx context.Context, driveID, uploadID string) (repository.UploadSessionRecord, error)
	GetUploadSessionsByUploadIDs(ctx context.Context, driveID string, uploadIDs []string) (map[string]repository.UploadSessionRecord, error)
	EnsureUploadParts(ctx context.Context, uploadID string, partNumbers []int) error
	MarkUploadPartUploaded(ctx context.Context, uploadID string, partNumber int, size int64, etag string) error
	SetUploadSessionStatus(ctx context.Context, uploadID, status string) error
}
