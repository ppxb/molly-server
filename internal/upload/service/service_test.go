package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"molly-server/internal/config"
	"molly-server/internal/upload/repository"
	"molly-server/pkg/objectstorage"
)

type mockRepository struct {
	ensureDriveFn                  func(ctx context.Context, driveID string) error
	searchEntriesFn                func(ctx context.Context, params repository.SearchEntriesParams) ([]repository.EntryRecord, error)
	listEntriesFn                  func(ctx context.Context, params repository.ListEntriesParams) ([]repository.EntryRecord, error)
	existsEntryFn                  func(ctx context.Context, driveID, parentFileID, name string) (bool, error)
	createFolderFn                 func(ctx context.Context, params repository.CreateFolderParams) (repository.EntryRecord, error)
	createFileWithUploadFn         func(ctx context.Context, params repository.CreateFileWithUploadParams) (repository.EntryRecord, error)
	getEntryByFileIDFn             func(ctx context.Context, driveID, fileID string) (repository.EntryRecord, error)
	getEntryByParentAndNameFn      func(ctx context.Context, driveID, parentFileID, name string) (repository.EntryRecord, error)
	getSubtreeStatsFn              func(ctx context.Context, driveID, folderID string) (repository.SubtreeStats, error)
	renameEntryFn                  func(ctx context.Context, driveID, fileID, newName string) (repository.EntryRecord, error)
	moveEntryFn                    func(ctx context.Context, driveID, fileID, targetParentFileID string) (repository.EntryRecord, error)
	trashEntryFn                   func(ctx context.Context, driveID, fileID, name, recycleBinParentID, trashedParentFileID string, trashedAt, expiredAt time.Time) (repository.EntryRecord, error)
	restoreEntryFn                 func(ctx context.Context, driveID, fileID, name, parentFileID string) (repository.EntryRecord, error)
	deleteEntryTreeFn              func(ctx context.Context, driveID, fileID string) ([]repository.EntryRecord, error)
	updateEntryHashFn              func(ctx context.Context, driveID, fileID, contentHash string) (repository.EntryRecord, error)
	getUploadSessionFn             func(ctx context.Context, driveID, uploadID string) (repository.UploadSessionRecord, error)
	getUploadSessionsByUploadIDsFn func(ctx context.Context, driveID string, uploadIDs []string) (map[string]repository.UploadSessionRecord, error)
	ensureUploadPartsFn            func(ctx context.Context, uploadID string, partNumbers []int) error
	markUploadPartUploadedFn       func(ctx context.Context, uploadID string, partNumber int, size int64, etag string) error
	setUploadSessionStatusFn       func(ctx context.Context, uploadID, status string) error
}

func (m *mockRepository) EnsureDrive(ctx context.Context, driveID string) error {
	if m.ensureDriveFn != nil {
		return m.ensureDriveFn(ctx, driveID)
	}
	return errors.New("unexpected call: EnsureDrive")
}

func (m *mockRepository) SearchEntries(ctx context.Context, params repository.SearchEntriesParams) ([]repository.EntryRecord, error) {
	if m.searchEntriesFn != nil {
		return m.searchEntriesFn(ctx, params)
	}
	return nil, errors.New("unexpected call: SearchEntries")
}

func (m *mockRepository) ListEntries(ctx context.Context, params repository.ListEntriesParams) ([]repository.EntryRecord, error) {
	if m.listEntriesFn != nil {
		return m.listEntriesFn(ctx, params)
	}
	return nil, errors.New("unexpected call: ListEntries")
}

func (m *mockRepository) ExistsEntry(ctx context.Context, driveID, parentFileID, name string) (bool, error) {
	if m.existsEntryFn != nil {
		return m.existsEntryFn(ctx, driveID, parentFileID, name)
	}
	return false, errors.New("unexpected call: ExistsEntry")
}

func (m *mockRepository) CreateFolder(ctx context.Context, params repository.CreateFolderParams) (repository.EntryRecord, error) {
	if m.createFolderFn != nil {
		return m.createFolderFn(ctx, params)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: CreateFolder")
}

func (m *mockRepository) CreateFileWithUpload(ctx context.Context, params repository.CreateFileWithUploadParams) (repository.EntryRecord, error) {
	if m.createFileWithUploadFn != nil {
		return m.createFileWithUploadFn(ctx, params)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: CreateFileWithUpload")
}

func (m *mockRepository) GetEntryByFileID(ctx context.Context, driveID, fileID string) (repository.EntryRecord, error) {
	if m.getEntryByFileIDFn != nil {
		return m.getEntryByFileIDFn(ctx, driveID, fileID)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: GetEntryByFileID")
}

func (m *mockRepository) GetEntryByParentAndName(ctx context.Context, driveID, parentFileID, name string) (repository.EntryRecord, error) {
	if m.getEntryByParentAndNameFn != nil {
		return m.getEntryByParentAndNameFn(ctx, driveID, parentFileID, name)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: GetEntryByParentAndName")
}

func (m *mockRepository) GetSubtreeStats(ctx context.Context, driveID, folderID string) (repository.SubtreeStats, error) {
	if m.getSubtreeStatsFn != nil {
		return m.getSubtreeStatsFn(ctx, driveID, folderID)
	}
	return repository.SubtreeStats{}, errors.New("unexpected call: GetSubtreeStats")
}

func (m *mockRepository) RenameEntry(ctx context.Context, driveID, fileID, newName string) (repository.EntryRecord, error) {
	if m.renameEntryFn != nil {
		return m.renameEntryFn(ctx, driveID, fileID, newName)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: RenameEntry")
}

func (m *mockRepository) MoveEntry(ctx context.Context, driveID, fileID, targetParentFileID string) (repository.EntryRecord, error) {
	if m.moveEntryFn != nil {
		return m.moveEntryFn(ctx, driveID, fileID, targetParentFileID)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: MoveEntry")
}

func (m *mockRepository) TrashEntry(ctx context.Context, driveID, fileID, name, recycleBinParentID, trashedParentFileID string, trashedAt, expiredAt time.Time) (repository.EntryRecord, error) {
	if m.trashEntryFn != nil {
		return m.trashEntryFn(ctx, driveID, fileID, name, recycleBinParentID, trashedParentFileID, trashedAt, expiredAt)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: TrashEntry")
}

func (m *mockRepository) RestoreEntry(ctx context.Context, driveID, fileID, name, parentFileID string) (repository.EntryRecord, error) {
	if m.restoreEntryFn != nil {
		return m.restoreEntryFn(ctx, driveID, fileID, name, parentFileID)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: RestoreEntry")
}

func (m *mockRepository) DeleteEntryTree(ctx context.Context, driveID, fileID string) ([]repository.EntryRecord, error) {
	if m.deleteEntryTreeFn != nil {
		return m.deleteEntryTreeFn(ctx, driveID, fileID)
	}
	return nil, errors.New("unexpected call: DeleteEntryTree")
}

func (m *mockRepository) UpdateEntryHash(ctx context.Context, driveID, fileID, contentHash string) (repository.EntryRecord, error) {
	if m.updateEntryHashFn != nil {
		return m.updateEntryHashFn(ctx, driveID, fileID, contentHash)
	}
	return repository.EntryRecord{}, errors.New("unexpected call: UpdateEntryHash")
}

func (m *mockRepository) GetUploadSession(ctx context.Context, driveID, uploadID string) (repository.UploadSessionRecord, error) {
	if m.getUploadSessionFn != nil {
		return m.getUploadSessionFn(ctx, driveID, uploadID)
	}
	return repository.UploadSessionRecord{}, errors.New("unexpected call: GetUploadSession")
}

func (m *mockRepository) GetUploadSessionsByUploadIDs(ctx context.Context, driveID string, uploadIDs []string) (map[string]repository.UploadSessionRecord, error) {
	if m.getUploadSessionsByUploadIDsFn != nil {
		return m.getUploadSessionsByUploadIDsFn(ctx, driveID, uploadIDs)
	}
	return nil, errors.New("unexpected call: GetUploadSessionsByUploadIDs")
}

func (m *mockRepository) EnsureUploadParts(ctx context.Context, uploadID string, partNumbers []int) error {
	if m.ensureUploadPartsFn != nil {
		return m.ensureUploadPartsFn(ctx, uploadID, partNumbers)
	}
	return errors.New("unexpected call: EnsureUploadParts")
}

func (m *mockRepository) MarkUploadPartUploaded(ctx context.Context, uploadID string, partNumber int, size int64, etag string) error {
	if m.markUploadPartUploadedFn != nil {
		return m.markUploadPartUploadedFn(ctx, uploadID, partNumber, size, etag)
	}
	return errors.New("unexpected call: MarkUploadPartUploaded")
}

func (m *mockRepository) SetUploadSessionStatus(ctx context.Context, uploadID, status string) error {
	if m.setUploadSessionStatusFn != nil {
		return m.setUploadSessionStatusFn(ctx, uploadID, status)
	}
	return errors.New("unexpected call: SetUploadSessionStatus")
}

type mockStorage struct {
	createMultipartUploadFn func(ctx context.Context, key, contentType string) (string, error)
	presignUploadPartFn     func(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error)
	listUploadedPartsFn     func(ctx context.Context, key, uploadID string) ([]objectstorage.UploadedPart, error)
	completeMultipartFn     func(ctx context.Context, key, uploadID string, parts []objectstorage.CompletedPart) error
	abortMultipartFn        func(ctx context.Context, key, uploadID string) error
	deleteObjectFn          func(ctx context.Context, key string) error
	openObjectFn            func(ctx context.Context, key string) (io.ReadCloser, error)
	presignGetObjectFn      func(ctx context.Context, key, disposition string, expires time.Duration) (string, error)
	presignPutObjectFn      func(ctx context.Context, key, contentType string, expires time.Duration) (string, error)
}

func (m *mockStorage) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	if m.createMultipartUploadFn != nil {
		return m.createMultipartUploadFn(ctx, key, contentType)
	}
	return "", errors.New("unexpected call: CreateMultipartUpload")
}

func (m *mockStorage) PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	if m.presignUploadPartFn != nil {
		return m.presignUploadPartFn(ctx, key, uploadID, partNumber, expires)
	}
	return "", errors.New("unexpected call: PresignUploadPart")
}

func (m *mockStorage) ListUploadedParts(ctx context.Context, key, uploadID string) ([]objectstorage.UploadedPart, error) {
	if m.listUploadedPartsFn != nil {
		return m.listUploadedPartsFn(ctx, key, uploadID)
	}
	return nil, errors.New("unexpected call: ListUploadedParts")
}

func (m *mockStorage) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []objectstorage.CompletedPart) error {
	if m.completeMultipartFn != nil {
		return m.completeMultipartFn(ctx, key, uploadID, parts)
	}
	return errors.New("unexpected call: CompleteMultipartUpload")
}

func (m *mockStorage) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	if m.abortMultipartFn != nil {
		return m.abortMultipartFn(ctx, key, uploadID)
	}
	return errors.New("unexpected call: AbortMultipartUpload")
}

func (m *mockStorage) DeleteObject(ctx context.Context, key string) error {
	if m.deleteObjectFn != nil {
		return m.deleteObjectFn(ctx, key)
	}
	return errors.New("unexpected call: DeleteObject")
}

func (m *mockStorage) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.openObjectFn != nil {
		return m.openObjectFn(ctx, key)
	}
	return nil, errors.New("unexpected call: OpenObject")
}

func (m *mockStorage) PresignGetObject(ctx context.Context, key, disposition string, expires time.Duration) (string, error) {
	if m.presignGetObjectFn != nil {
		return m.presignGetObjectFn(ctx, key, disposition, expires)
	}
	return "", errors.New("unexpected call: PresignGetObject")
}

func (m *mockStorage) PresignPutObject(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	if m.presignPutObjectFn != nil {
		return m.presignPutObjectFn(ctx, key, contentType, expires)
	}
	return "", errors.New("unexpected call: PresignPutObject")
}

func newTestService(repo Repository, storage objectstorage.Client) *service {
	return &service{
		repo: repo,
		uploadCfg: config.UploadConfig{
			DomainID:           "bj29",
			Location:           "cn-beijing",
			DefaultDriveID:     "default",
			UploadURLTTLSecs:   900,
			DownloadURLTTLSecs: 900,
			SinglePutMaxSize:   32 * 1024 * 1024,
		},
		storageCfg: config.ObjectStorageConfig{
			Bucket: "test-bucket",
		},
		storage: storage,
	}
}

func TestCreateWithFolders_SmallFileUsesSinglePut(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{
		ensureDriveFn: func(ctx context.Context, driveID string) error {
			if driveID != "default" {
				t.Fatalf("unexpected drive id: %s", driveID)
			}
			return nil
		},
		getEntryByParentAndNameFn: func(ctx context.Context, driveID, parentFileID, name string) (repository.EntryRecord, error) {
			if parentFileID != rootFolderID {
				t.Fatalf("unexpected parent file id: %s", parentFileID)
			}
			return repository.EntryRecord{}, repository.ErrNotFound
		},
		createFileWithUploadFn: func(ctx context.Context, params repository.CreateFileWithUploadParams) (repository.EntryRecord, error) {
			if params.ChunkSize != 0 {
				t.Fatalf("expected single-put chunk size 0, got %d", params.ChunkSize)
			}
			if len(params.PartNumbers) != 1 || params.PartNumbers[0] != 1 {
				t.Fatalf("expected part numbers [1], got %#v", params.PartNumbers)
			}
			if params.UploadID == "" {
				t.Fatal("expected generated upload id")
			}
			return repository.EntryRecord{
				DriveID:      params.DriveID,
				FileID:       params.FileID,
				ParentFileID: params.ParentFileID,
				Name:         params.Name,
				Type:         "file",
				UploadID:     params.UploadID,
				RevisionID:   params.RevisionID,
				EncryptMode:  "none",
				CreatedAt:    time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC),
				UpdatedAt:    time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC),
			}, nil
		},
	}

	storage := &mockStorage{
		presignPutObjectFn: func(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
			if !strings.Contains(key, "default/") {
				t.Fatalf("unexpected object key: %s", key)
			}
			if contentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
				t.Fatalf("unexpected content type: %s", contentType)
			}
			return "https://example.com/upload", nil
		},
	}

	svc := newTestService(repo, storage)

	resp, err := svc.CreateWithFolders(context.Background(), CreateWithFoldersRequest{
		DriveID:       "default",
		ParentFileID:  rootFolderID,
		Name:          "report.xlsx",
		Type:          "file",
		CheckNameMode: "refuse",
		Size:          300 * 1024,
		PartInfoList:  []UploadPartRequest{{PartNumber: 1}},
	})
	if err != nil {
		t.Fatalf("CreateWithFolders returned error: %v", err)
	}

	if resp.Type != "file" {
		t.Fatalf("expected file type, got %s", resp.Type)
	}
	if len(resp.PartInfoList) != 1 {
		t.Fatalf("expected one upload url, got %d", len(resp.PartInfoList))
	}
	if resp.PartInfoList[0].UploadURL != "https://example.com/upload" {
		t.Fatalf("unexpected upload url: %s", resp.PartInfoList[0].UploadURL)
	}
}

func TestGetFolderSizeInfo_UsesRepositoryStats(t *testing.T) {
	t.Parallel()

	repo := &mockRepository{
		getEntryByFileIDFn: func(ctx context.Context, driveID, fileID string) (repository.EntryRecord, error) {
			return repository.EntryRecord{
				DriveID: driveID,
				FileID:  fileID,
				Name:    "docs",
				Type:    "folder",
			}, nil
		},
		getSubtreeStatsFn: func(ctx context.Context, driveID, folderID string) (repository.SubtreeStats, error) {
			if folderID != "folder-1" {
				t.Fatalf("unexpected folder id: %s", folderID)
			}
			return repository.SubtreeStats{
				Size:        420980503754,
				FileCount:   157,
				FolderCount: 10,
			}, nil
		},
	}

	svc := newTestService(repo, nil)

	resp, err := svc.GetFolderSizeInfo(context.Background(), GetFolderSizeInfoRequest{
		DriveID: "default",
		FileID:  "folder-1",
	})
	if err != nil {
		t.Fatalf("GetFolderSizeInfo returned error: %v", err)
	}

	if resp.Size != 420980503754 || resp.FileCount != 157 || resp.FolderCount != 10 {
		t.Fatalf("unexpected stats: %#v", resp)
	}
	if resp.DisplaySummary != "392.07 GB（包含 157 个文件，10 个文件夹）" {
		t.Fatalf("unexpected display summary: %s", resp.DisplaySummary)
	}
}

func TestDeleteFile_SchedulesObjectDeletion(t *testing.T) {
	t.Parallel()

	deletedKeyCh := make(chan string, 1)

	repo := &mockRepository{
		ensureDriveFn: func(ctx context.Context, driveID string) error {
			return nil
		},
		getEntryByFileIDFn: func(ctx context.Context, driveID, fileID string) (repository.EntryRecord, error) {
			return repository.EntryRecord{
				DriveID:      driveID,
				FileID:       fileID,
				Name:         "archive.rar",
				Type:         "file",
				ParentFileID: recycleBinFolderID,
				RevisionID:   "rev-root",
			}, nil
		},
		deleteEntryTreeFn: func(ctx context.Context, driveID, fileID string) ([]repository.EntryRecord, error) {
			return []repository.EntryRecord{
				{
					DriveID:    driveID,
					FileID:     fileID,
					Type:       "file",
					RevisionID: "rev-root",
				},
				{
					DriveID:    driveID,
					FileID:     "child-folder",
					Type:       "folder",
					Name:       "nested",
					RevisionID: "",
				},
			}, nil
		},
	}

	storage := &mockStorage{
		deleteObjectFn: func(ctx context.Context, key string) error {
			deletedKeyCh <- key
			return nil
		},
	}

	svc := newTestService(repo, storage)

	if err := svc.DeleteFile(context.Background(), DeleteFileRequest{
		DriveID: "default",
		FileID:  "file-1",
	}); err != nil {
		t.Fatalf("DeleteFile returned error: %v", err)
	}

	select {
	case key := <-deletedKeyCh:
		if key != "default/file-1/rev-root" {
			t.Fatalf("unexpected deleted object key: %s", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected async object deletion to be scheduled")
	}
}
