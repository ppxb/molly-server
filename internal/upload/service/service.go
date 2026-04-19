package service

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"molly-server/internal/config"
	"molly-server/internal/upload/repository"
	"molly-server/pkg/objectstorage"
)

const (
	rootFolderID                = "root"
	recycleBinFolderID          = "recyclebin"
	maxSearchLimit              = 1_000
	defaultSearchLimit          = 100
	defaultListLimit            = 20
	maxListLimit                = 500
	defaultUploadURLTTLSecs     = 900
	defaultDownloadTTLSecs      = 900
	defaultRecycleRetentionDays = 10
	defaultSinglePutMaxSize     = 32 * 1024 * 1024
	pendingHashPrefix           = "pending:"
)

var (
	parentFileIDPattern = regexp.MustCompile(`(?i)parent_file_id\s*=\s*"([^"]+)"`)
	namePattern         = regexp.MustCompile(`(?i)name\s*=\s*"([^"]+)"`)
)

type service struct {
	repo       *repository.Repository
	uploadCfg  config.UploadConfig
	storageCfg config.ObjectStorageConfig
	storage    objectstorage.Client
}

type folderNode struct {
	ID        string
	Name      string
	ParentID  string
	Path      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func New(
	repo *repository.Repository,
	uploadCfg config.UploadConfig,
	storageCfg config.ObjectStorageConfig,
	storage objectstorage.Client,
) Service {
	return &service{
		repo:       repo,
		uploadCfg:  uploadCfg,
		storageCfg: storageCfg,
		storage:    storage,
	}
}

func (s *service) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return SearchResponse{}, fmt.Errorf("search: ensure drive: %w", err)
	}

	parentFileID, name, err := parseSearchQuery(req.Query)
	if err != nil {
		return SearchResponse{}, err
	}

	entries, err := s.repo.SearchEntries(ctx, repository.SearchEntriesParams{
		DriveID:      driveID,
		ParentFileID: parentFileID,
		Name:         name,
		OrderBy:      req.OrderBy,
		Limit:        sanitizeLimit(req.Limit, defaultSearchLimit, maxSearchLimit),
	})
	if err != nil {
		return SearchResponse{}, fmt.Errorf("search: query entries: %w", err)
	}

	sessionMap, err := s.buildUploadSessionMap(ctx, driveID, entries)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("search: query upload sessions: %w", err)
	}

	items := make([]SearchItem, 0, len(entries))
	for _, item := range entries {
		if !isVisibleEntry(item, sessionMap) {
			continue
		}
		items = append(items, SearchItem{
			DriveID:      item.DriveID,
			FileID:       item.FileID,
			ParentFileID: item.ParentFileID,
			Name:         item.Name,
			Type:         item.Type,
			Size:         item.Size,
			CreatedAt:    toRFC3339(item.CreatedAt),
			UpdatedAt:    toRFC3339(item.UpdatedAt),
		})
	}

	return SearchResponse{
		Items:      items,
		NextMarker: "",
	}, nil
}

func (s *service) CreateWithFolders(ctx context.Context, req CreateWithFoldersRequest) (CreateWithFoldersResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return CreateWithFoldersResponse{}, fmt.Errorf("create with folders: ensure drive: %w", err)
	}

	parentFileID := normalizeFolderID(req.ParentFileID)
	if err := s.ensureFolderExists(ctx, driveID, parentFileID); err != nil {
		return CreateWithFoldersResponse{}, err
	}

	entryType := strings.ToLower(strings.TrimSpace(req.Type))
	if entryType != "file" && entryType != "folder" {
		return CreateWithFoldersResponse{}, fmt.Errorf("%w: type must be file or folder", ErrInvalidArgument)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return CreateWithFoldersResponse{}, fmt.Errorf("%w: name is required", ErrInvalidArgument)
	}

	resolvedName, err := s.resolveEntryName(ctx, driveID, parentFileID, name, req.CheckNameMode)
	if err != nil {
		return CreateWithFoldersResponse{}, err
	}

	if entryType == "folder" {
		fileID := newHexID(40)
		revisionID := newHexID(40)
		created, err := s.repo.CreateFolder(ctx, repository.CreateFolderParams{
			DriveID:      driveID,
			ParentFileID: parentFileID,
			Name:         resolvedName,
			FileID:       fileID,
			RevisionID:   revisionID,
		})
		if err != nil {
			if err == repository.ErrConflict {
				return CreateWithFoldersResponse{}, fmt.Errorf("%w: folder already exists", ErrConflict)
			}
			return CreateWithFoldersResponse{}, fmt.Errorf("create with folders: create folder: %w", err)
		}

		return CreateWithFoldersResponse{
			ParentFileID: created.ParentFileID,
			Type:         "folder",
			FileID:       created.FileID,
			DomainID:     s.uploadCfg.DomainID,
			DriveID:      created.DriveID,
			FileName:     created.Name,
			EncryptMode:  created.EncryptMode,
			CreatedAt:    toRFC3339(created.CreatedAt),
			UpdatedAt:    toRFC3339(created.UpdatedAt),
		}, nil
	}

	if req.Size < 0 {
		return CreateWithFoldersResponse{}, fmt.Errorf("%w: size cannot be negative", ErrInvalidArgument)
	}
	if err := s.ensureStorageAvailable(); err != nil {
		return CreateWithFoldersResponse{}, err
	}

	partNumbers, err := normalizePartNumbers(req.PartInfoList)
	if err != nil {
		return CreateWithFoldersResponse{}, err
	}
	if len(partNumbers) == 0 {
		partNumbers = []int{1}
	}

	fileID := newHexID(40)
	revisionID := newHexID(40)
	objectKey := buildObjectKey(driveID, fileID, revisionID)
	contentType := resolveContentType(req.ContentType, resolvedName)
	useSinglePut := s.shouldUseSinglePutUpload(req.Size, partNumbers)

	uploadID := newHexID(32)
	if !useSinglePut {
		uploadID, err = s.storage.CreateMultipartUpload(ctx, objectKey, contentType)
		if err != nil {
			return CreateWithFoldersResponse{}, fmt.Errorf("create with folders: create multipart upload: %w", err)
		}
	}

	chunkSize := normalizeChunkSize(req.ChunkSize, req.Size, len(partNumbers))
	if useSinglePut {
		chunkSize = 0
		partNumbers = []int{1}
	}

	entry, err := s.repo.CreateFileWithUpload(ctx, repository.CreateFileWithUploadParams{
		DriveID:      driveID,
		ParentFileID: parentFileID,
		Name:         resolvedName,
		FileID:       fileID,
		RevisionID:   revisionID,
		UploadID:     uploadID,
		Size:         req.Size,
		PreHash:      strings.TrimSpace(req.PreHash),
		PartNumbers:  partNumbers,
		ChunkSize:    chunkSize,
		ExpiresAt:    time.Now().UTC().Add(s.uploadURLTTL()),
	})
	if err != nil {
		if !useSinglePut {
			_ = s.storage.AbortMultipartUpload(ctx, objectKey, uploadID)
		}
		if err == repository.ErrConflict {
			return CreateWithFoldersResponse{}, fmt.Errorf("%w: file already exists", ErrConflict)
		}
		return CreateWithFoldersResponse{}, fmt.Errorf("create with folders: persist upload entry: %w", err)
	}

	var partInfoList []UploadPartInfo
	if useSinglePut {
		partInfoList, err = s.buildSinglePutPartInfo(ctx, objectKey, contentType)
		if err != nil {
			return CreateWithFoldersResponse{}, err
		}
	} else {
		partInfoList, err = s.buildPartInfoList(ctx, objectKey, uploadID, partNumbers, contentType)
		if err != nil {
			return CreateWithFoldersResponse{}, err
		}
	}

	return CreateWithFoldersResponse{
		ParentFileID: entry.ParentFileID,
		PartInfoList: partInfoList,
		UploadID:     uploadID,
		RapidUpload:  false,
		Type:         "file",
		FileID:       entry.FileID,
		RevisionID:   entry.RevisionID,
		DomainID:     s.uploadCfg.DomainID,
		DriveID:      entry.DriveID,
		FileName:     entry.Name,
		EncryptMode:  entry.EncryptMode,
		Location:     s.uploadCfg.Location,
		CreatedAt:    toRFC3339(entry.CreatedAt),
		UpdatedAt:    toRFC3339(entry.UpdatedAt),
	}, nil
}

func (s *service) GetUploadURL(ctx context.Context, req GetUploadURLRequest) (GetUploadURLResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	uploadID := strings.TrimSpace(req.UploadID)
	fileID := strings.TrimSpace(req.FileID)
	if uploadID == "" || fileID == "" {
		return GetUploadURLResponse{}, fmt.Errorf("%w: upload_id and file_id are required", ErrInvalidArgument)
	}
	if err := s.ensureStorageAvailable(); err != nil {
		return GetUploadURLResponse{}, err
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return GetUploadURLResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return GetUploadURLResponse{}, fmt.Errorf("get upload url: query file: %w", err)
	}
	if entry.UploadID != uploadID {
		return GetUploadURLResponse{}, fmt.Errorf("%w: upload session does not match file", ErrInvalidArgument)
	}

	session, err := s.repo.GetUploadSession(ctx, driveID, uploadID)
	if err != nil {
		if err == repository.ErrNotFound {
			return GetUploadURLResponse{}, fmt.Errorf("%w: upload session not found", ErrNotFound)
		}
		return GetUploadURLResponse{}, fmt.Errorf("get upload url: query session: %w", err)
	}

	partNumbers, err := normalizePartNumbers(req.PartInfoList)
	if err != nil {
		return GetUploadURLResponse{}, err
	}
	if len(partNumbers) == 0 {
		return GetUploadURLResponse{}, fmt.Errorf("%w: part_info_list is required", ErrInvalidArgument)
	}

	contentType := resolveContentType("", entry.Name)
	objectKey := buildObjectKey(entry.DriveID, entry.FileID, entry.RevisionID)

	var partInfo []UploadPartInfo
	if isSinglePutUploadSession(session) {
		if err := validateSinglePutPartNumbers(partNumbers); err != nil {
			return GetUploadURLResponse{}, err
		}
		partInfo, err = s.buildSinglePutPartInfo(ctx, objectKey, contentType)
		if err != nil {
			return GetUploadURLResponse{}, err
		}
	} else {
		if err := s.repo.EnsureUploadParts(ctx, uploadID, partNumbers); err != nil {
			return GetUploadURLResponse{}, fmt.Errorf("get upload url: ensure upload parts: %w", err)
		}
		partInfo, err = s.buildPartInfoList(ctx, objectKey, uploadID, partNumbers, contentType)
		if err != nil {
			return GetUploadURLResponse{}, err
		}
	}

	return GetUploadURLResponse{
		DomainID:     s.uploadCfg.DomainID,
		DriveID:      driveID,
		FileID:       fileID,
		PartInfoList: partInfo,
		UploadID:     uploadID,
		CreateAt:     toRFC3339(session.CreatedAt),
	}, nil
}

func (s *service) GetFile(ctx context.Context, req GetFileRequest) (GetFileResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return GetFileResponse{}, fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}

	if fileID == rootFolderID {
		return GetFileResponse{
			DriveID:                     driveID,
			DomainID:                    s.uploadCfg.DomainID,
			FileID:                      rootFolderID,
			Name:                        rootFolderID,
			Type:                        "folder",
			CreatedAt:                   "",
			UpdatedAt:                   "",
			Hidden:                      false,
			Starred:                     false,
			Status:                      "available",
			ParentFileID:                "",
			EncryptMode:                 "none",
			MetaNamePunishFlag:          0,
			MetaNameInvestigationStatus: 0,
			CreatorType:                 "User",
			CreatorID:                   "",
			LastModifierType:            "User",
			LastModifierID:              "",
			SyncFlag:                    false,
			SyncDeviceFlag:              false,
			SyncMeta:                    "",
			Trashed:                     false,
			DownloadURL:                 "",
			URL:                         "",
		}, nil
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return GetFileResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return GetFileResponse{}, fmt.Errorf("get file: query file: %w", err)
	}

	return s.toFileGetResponse(entry), nil
}

func (s *service) toFileGetResponse(entry repository.EntryRecord) GetFileResponse {
	contentType := ""
	if entry.Type == "file" {
		contentType = resolveContentType("", entry.Name)
	}
	trashed := isInRecycleBin(entry)

	return GetFileResponse{
		DriveID:                     entry.DriveID,
		DomainID:                    s.uploadCfg.DomainID,
		FileID:                      entry.FileID,
		Name:                        entry.Name,
		Type:                        entry.Type,
		ContentType:                 contentType,
		CreatedAt:                   toRFC3339(entry.CreatedAt),
		UpdatedAt:                   toRFC3339(entry.UpdatedAt),
		Hidden:                      false,
		Starred:                     false,
		Status:                      "available",
		ParentFileID:                entry.ParentFileID,
		EncryptMode:                 entry.EncryptMode,
		MetaNamePunishFlag:          0,
		MetaNameInvestigationStatus: 0,
		CreatorType:                 "User",
		CreatorID:                   "",
		LastModifierType:            "User",
		LastModifierID:              "",
		SyncFlag:                    false,
		SyncDeviceFlag:              false,
		SyncMeta:                    "",
		Trashed:                     trashed,
		DownloadURL:                 "",
		URL:                         "",
	}
}

func (s *service) GetFilePath(ctx context.Context, req GetFilePathRequest) (GetFilePathResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return GetFilePathResponse{}, fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}
	if fileID == rootFolderID {
		return GetFilePathResponse{Items: []GetFilePathItem{}}, nil
	}

	pathEntries := make([]repository.EntryRecord, 0, 8)
	visited := map[string]struct{}{}
	currentID := fileID

	for currentID != "" && currentID != rootFolderID {
		if _, ok := visited[currentID]; ok {
			break
		}
		visited[currentID] = struct{}{}

		entry, err := s.repo.GetEntryByFileID(ctx, driveID, currentID)
		if err != nil {
			if err == repository.ErrNotFound {
				return GetFilePathResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
			}
			return GetFilePathResponse{}, fmt.Errorf("get file path: query path entry: %w", err)
		}
		pathEntries = append(pathEntries, entry)
		if isInRecycleBin(entry) {
			break
		}
		currentID = normalizeFolderID(entry.ParentFileID)
	}

	for left, right := 0, len(pathEntries)-1; left < right; left, right = left+1, right-1 {
		pathEntries[left], pathEntries[right] = pathEntries[right], pathEntries[left]
	}

	items := make([]GetFilePathItem, 0, len(pathEntries))
	for _, entry := range pathEntries {
		items = append(items, GetFilePathItem{
			Trashed:      isInRecycleBin(entry),
			DriveID:      entry.DriveID,
			FileID:       entry.FileID,
			CreatedAt:    toRFC3339(entry.CreatedAt),
			DomainID:     s.uploadCfg.DomainID,
			EncryptMode:  entry.EncryptMode,
			Hidden:       false,
			Name:         entry.Name,
			ParentFileID: entry.ParentFileID,
			Starred:      false,
			Status:       "available",
			Type:         entry.Type,
			UpdatedAt:    toRFC3339(entry.UpdatedAt),
			SyncFlag:     false,
		})
	}

	return GetFilePathResponse{Items: items}, nil
}

func (s *service) GetFolderSizeInfo(ctx context.Context, req GetFolderSizeInfoRequest) (GetFolderSizeInfoResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return GetFolderSizeInfoResponse{}, fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}

	if fileID != rootFolderID {
		folder, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
		if err != nil {
			if err == repository.ErrNotFound {
				return GetFolderSizeInfoResponse{}, fmt.Errorf("%w: folder not found", ErrNotFound)
			}
			return GetFolderSizeInfoResponse{}, fmt.Errorf("get folder size info: query folder: %w", err)
		}
		if folder.Type != "folder" {
			return GetFolderSizeInfoResponse{}, fmt.Errorf("%w: target is not a folder", ErrInvalidArgument)
		}
	}

	entries, err := s.repo.ListEntries(ctx, repository.ListEntriesParams{
		DriveID: driveID,
	})
	if err != nil {
		return GetFolderSizeInfoResponse{}, fmt.Errorf("get folder size info: list entries: %w", err)
	}

	sessionMap, err := s.buildUploadSessionMap(ctx, driveID, entries)
	if err != nil {
		return GetFolderSizeInfoResponse{}, fmt.Errorf("get folder size info: query upload sessions: %w", err)
	}

	childrenByParent := make(map[string][]repository.EntryRecord, len(entries))
	for _, item := range entries {
		if !isVisibleEntry(item, sessionMap) {
			continue
		}
		parentID := normalizeFolderID(item.ParentFileID)
		childrenByParent[parentID] = append(childrenByParent[parentID], item)
	}

	visited := map[string]struct{}{
		fileID: {},
	}
	queue := []string{fileID}

	var size int64
	var folderCount int64
	var fileCount int64

	for len(queue) > 0 {
		currentFolderID := queue[0]
		queue = queue[1:]

		children := childrenByParent[currentFolderID]
		for _, child := range children {
			if child.Type == "folder" {
				if _, exists := visited[child.FileID]; exists {
					continue
				}
				visited[child.FileID] = struct{}{}
				folderCount += 1
				queue = append(queue, child.FileID)
				continue
			}
			fileCount += 1
			size += child.Size
		}
	}

	return GetFolderSizeInfoResponse{
		Size:           size,
		FolderCount:    folderCount,
		FileCount:      fileCount,
		DisplaySummary: formatFolderSizeSummary(size, fileCount, folderCount),
	}, nil
}

func (s *service) CompleteFile(ctx context.Context, req CompleteFileRequest) (CompleteFileResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	uploadID := strings.TrimSpace(req.UploadID)
	fileID := strings.TrimSpace(req.FileID)
	if uploadID == "" || fileID == "" {
		return CompleteFileResponse{}, fmt.Errorf("%w: upload_id and file_id are required", ErrInvalidArgument)
	}
	if err := s.ensureStorageAvailable(); err != nil {
		return CompleteFileResponse{}, err
	}

	session, err := s.repo.GetUploadSession(ctx, driveID, uploadID)
	if err != nil {
		if err == repository.ErrNotFound {
			return CompleteFileResponse{}, fmt.Errorf("%w: upload session not found", ErrNotFound)
		}
		return CompleteFileResponse{}, fmt.Errorf("complete file: query upload session: %w", err)
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return CompleteFileResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return CompleteFileResponse{}, fmt.Errorf("complete file: query file: %w", err)
	}
	if entry.Type != "file" {
		return CompleteFileResponse{}, fmt.Errorf("%w: target is not a file", ErrInvalidArgument)
	}
	if strings.TrimSpace(entry.UploadID) != uploadID {
		return CompleteFileResponse{}, fmt.Errorf("%w: upload session does not match file", ErrInvalidArgument)
	}
	if strings.TrimSpace(session.FileID) != "" && strings.TrimSpace(session.FileID) != fileID {
		return CompleteFileResponse{}, fmt.Errorf("%w: upload session does not match file", ErrInvalidArgument)
	}

	if isSinglePutUploadSession(session) {
		if err := s.repo.MarkUploadPartUploaded(ctx, uploadID, 1, entry.Size, ""); err != nil {
			return CompleteFileResponse{}, fmt.Errorf("complete file: mark single upload part: %w", err)
		}
		finalHash, err := s.computeObjectContentHash(ctx, entry)
		if err != nil {
			return CompleteFileResponse{}, err
		}
		if finalHash != entry.ContentHash {
			entry, err = s.repo.UpdateEntryHash(ctx, driveID, entry.FileID, finalHash)
			if err != nil {
				return CompleteFileResponse{}, fmt.Errorf("complete file: update file hash: %w", err)
			}
		}

		if err := s.repo.SetUploadSessionStatus(ctx, uploadID, "completed"); err != nil && err != repository.ErrNotFound {
			return CompleteFileResponse{}, fmt.Errorf("complete file: set session status: %w", err)
		}

		return s.toCompleteFileResponse(entry, uploadID), nil
	}

	completedParts, uploadedParts, err := s.collectCompletedPartsFromStorage(ctx, entry, uploadID, session.PartCount)
	if err != nil {
		return CompleteFileResponse{}, err
	}

	objectKey := buildObjectKey(entry.DriveID, entry.FileID, entry.RevisionID)
	if err := s.storage.CompleteMultipartUpload(ctx, objectKey, uploadID, completedParts); err != nil {
		return CompleteFileResponse{}, fmt.Errorf("complete file: complete multipart upload: %w", err)
	}

	for _, part := range uploadedParts {
		if err := s.repo.MarkUploadPartUploaded(ctx, uploadID, int(part.PartNumber), part.Size, part.ETag); err != nil {
			return CompleteFileResponse{}, fmt.Errorf("complete file: mark uploaded part: %w", err)
		}
	}

	if err := s.repo.SetUploadSessionStatus(ctx, uploadID, "completed"); err != nil && err != repository.ErrNotFound {
		return CompleteFileResponse{}, fmt.Errorf("complete file: set session status: %w", err)
	}

	finalHash, err := s.computeObjectContentHash(ctx, entry)
	if err != nil {
		return CompleteFileResponse{}, err
	}
	if finalHash != entry.ContentHash {
		entry, err = s.repo.UpdateEntryHash(ctx, driveID, entry.FileID, finalHash)
		if err != nil {
			return CompleteFileResponse{}, fmt.Errorf("complete file: update file hash: %w", err)
		}
	}

	return s.toCompleteFileResponse(entry, uploadID), nil
}

func (s *service) List(ctx context.Context, req ListRequest) (ListResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return ListResponse{}, fmt.Errorf("list: ensure drive: %w", err)
	}

	entries, err := s.repo.ListEntries(ctx, repository.ListEntriesParams{
		DriveID:        driveID,
		ParentFileID:   normalizeFolderID(req.ParentFileID),
		Limit:          sanitizeLimit(req.Limit, defaultListLimit, maxListLimit),
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	})
	if err != nil {
		return ListResponse{}, fmt.Errorf("list: query entries: %w", err)
	}

	uploadIDs := collectUploadIDs(entries)
	sessionMap, err := s.repo.GetUploadSessionsByUploadIDs(ctx, driveID, uploadIDs)
	if err != nil {
		return ListResponse{}, fmt.Errorf("list: query upload sessions: %w", err)
	}

	items := make([]ListItem, 0, len(entries))
	for _, item := range entries {
		if !isVisibleEntry(item, sessionMap) {
			continue
		}
		items = append(items, s.toListItem(item, sessionMap[item.UploadID]))
	}

	return ListResponse{
		Items:      items,
		NextMarker: "",
	}, nil
}

func (s *service) RecycleBinTrash(ctx context.Context, req RecycleBinTrashRequest) error {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return fmt.Errorf("recyclebin trash: ensure drive: %w", err)
	}

	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}
	if fileID == rootFolderID {
		return fmt.Errorf("%w: root folder cannot be moved to recycle bin", ErrInvalidArgument)
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return fmt.Errorf("recyclebin trash: query file: %w", err)
	}
	if isInRecycleBin(entry) {
		return nil
	}

	resolvedName, err := s.resolveEntryName(ctx, driveID, recycleBinFolderID, entry.Name, "auto_rename")
	if err != nil {
		return err
	}

	trashedAt := time.Now().UTC()
	expiredAt := trashedAt.Add(s.recycleRetention())
	if _, err := s.repo.TrashEntry(
		ctx,
		driveID,
		fileID,
		resolvedName,
		recycleBinFolderID,
		normalizeFolderID(entry.ParentFileID),
		trashedAt,
		expiredAt,
	); err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		if err == repository.ErrConflict {
			return fmt.Errorf("%w: recycle bin already has an item with the same name", ErrConflict)
		}
		return fmt.Errorf("recyclebin trash: move to recycle bin: %w", err)
	}

	return nil
}

func (s *service) RecycleBinList(ctx context.Context, req RecycleBinListRequest) (RecycleBinListResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return RecycleBinListResponse{}, fmt.Errorf("recyclebin list: ensure drive: %w", err)
	}

	entries, err := s.repo.ListEntries(ctx, repository.ListEntriesParams{
		DriveID:        driveID,
		ParentFileID:   recycleBinFolderID,
		Limit:          sanitizeLimit(req.Limit, defaultListLimit, maxListLimit),
		OrderBy:        req.OrderBy,
		OrderDirection: req.OrderDirection,
	})
	if err != nil {
		return RecycleBinListResponse{}, fmt.Errorf("recyclebin list: query entries: %w", err)
	}

	items := make([]RecycleBinListItem, 0, len(entries))
	for _, entry := range entries {
		trashedAt := toRFC3339Ptr(entry.TrashedAt)
		if trashedAt == "" {
			trashedAt = toRFC3339(entry.UpdatedAt)
		}

		gmtExpired := toRFC3339Ptr(entry.ExpiredAt)
		if gmtExpired == "" {
			gmtExpired = toRFC3339(entry.UpdatedAt.Add(s.recycleRetention()))
		}

		item := RecycleBinListItem{
			Name:         entry.Name,
			Type:         entry.Type,
			Hidden:       false,
			Status:       "available",
			Starred:      false,
			ParentFileID: recycleBinFolderID,
			DriveID:      entry.DriveID,
			FileID:       entry.FileID,
			EncryptMode:  entry.EncryptMode,
			DomainID:     s.uploadCfg.DomainID,
			CreatedAt:    toRFC3339(entry.CreatedAt),
			UpdatedAt:    toRFC3339(entry.UpdatedAt),
			TrashedAt:    trashedAt,
			GMTExpired:   gmtExpired,
		}

		if entry.Type == "file" {
			contentType := resolveContentType("", entry.Name)
			_, ext := splitName(entry.Name)
			item.Category = categoryFromMime(contentType)
			item.URL = ""
			item.Size = entry.Size
			item.FileExtension = strings.TrimPrefix(strings.ToLower(ext), ".")
			item.ContentHash = entry.ContentHash
			item.ContentHashName = resolveContentHashName(item.ContentHash)
			item.PunishFlag = 0
		}

		items = append(items, item)
	}

	return RecycleBinListResponse{
		Items:      items,
		NextMarker: "",
	}, nil
}

func (s *service) RecycleBinRestore(ctx context.Context, req RecycleBinRestoreRequest) error {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return fmt.Errorf("recyclebin restore: ensure drive: %w", err)
	}

	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return fmt.Errorf("recyclebin restore: query file: %w", err)
	}
	if !isInRecycleBin(entry) {
		return fmt.Errorf("%w: file is not in recycle bin", ErrInvalidArgument)
	}

	targetParentID := normalizeFolderID(entry.TrashedParentFileID)
	if targetParentID == recycleBinFolderID {
		targetParentID = rootFolderID
	}
	if targetParentID != rootFolderID {
		targetParent, err := s.repo.GetEntryByFileID(ctx, driveID, targetParentID)
		if err != nil {
			if err != repository.ErrNotFound {
				return fmt.Errorf("recyclebin restore: query target parent: %w", err)
			}
			targetParentID = rootFolderID
		} else if targetParent.Type != "folder" || isInRecycleBin(targetParent) {
			targetParentID = rootFolderID
		}
	}

	resolvedName, err := s.resolveEntryName(ctx, driveID, targetParentID, entry.Name, "auto_rename")
	if err != nil {
		return err
	}

	if _, err := s.repo.RestoreEntry(ctx, driveID, fileID, resolvedName, targetParentID); err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		if err == repository.ErrConflict {
			return fmt.Errorf("%w: target folder already has a file or folder with the same name", ErrConflict)
		}
		return fmt.Errorf("recyclebin restore: restore entry: %w", err)
	}

	return nil
}

func (s *service) DeleteFile(ctx context.Context, req DeleteFileRequest) error {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return fmt.Errorf("file delete: ensure drive: %w", err)
	}

	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}
	if fileID == rootFolderID {
		return fmt.Errorf("%w: root folder cannot be deleted", ErrInvalidArgument)
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return fmt.Errorf("file delete: query file: %w", err)
	}
	if !isInRecycleBin(entry) {
		return fmt.Errorf("%w: file is not in recycle bin", ErrInvalidArgument)
	}

	if err := s.repo.DeleteEntryTree(ctx, driveID, fileID); err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return fmt.Errorf("file delete: delete entry tree: %w", err)
	}
	return nil
}

func (s *service) ListMoveTargets(ctx context.Context, req ListMoveTargetsRequest) (ListMoveTargetsResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return ListMoveTargetsResponse{}, fmt.Errorf("list move targets: ensure drive: %w", err)
	}

	folderNodes, err := s.loadFolderNodes(ctx, driveID)
	if err != nil {
		return ListMoveTargetsResponse{}, err
	}

	excludeFolderID := strings.TrimSpace(req.ExcludeFolderID)
	if excludeFolderID != "" && excludeFolderID != rootFolderID {
		if _, ok := folderNodes[excludeFolderID]; !ok {
			return ListMoveTargetsResponse{}, fmt.Errorf("%w: exclude folder not found", ErrNotFound)
		}
	}

	folders := make([]UploadFolderRecord, 0, len(folderNodes))
	for _, node := range folderNodes {
		if excludeFolderID != "" && isDescendantOrSelf(node.ID, excludeFolderID, folderNodes) {
			continue
		}
		folders = append(folders, toFolderRecord(node, folderNodes))
	}

	sort.SliceStable(folders, func(i, j int) bool {
		if folders[i].FolderPath == folders[j].FolderPath {
			return folders[i].FolderName < folders[j].FolderName
		}
		return folders[i].FolderPath < folders[j].FolderPath
	})

	return ListMoveTargetsResponse{Folders: folders}, nil
}

func (s *service) Update(ctx context.Context, req UpdateRequest) (UpdateResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return UpdateResponse{}, fmt.Errorf("update file: ensure drive: %w", err)
	}

	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return UpdateResponse{}, fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}
	if fileID == rootFolderID {
		return UpdateResponse{}, fmt.Errorf("%w: root folder cannot be updated", ErrInvalidArgument)
	}

	nextName := strings.TrimSpace(req.Name)
	if nextName == "" {
		return UpdateResponse{}, fmt.Errorf("%w: name is required", ErrInvalidArgument)
	}

	current, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return UpdateResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return UpdateResponse{}, fmt.Errorf("update file: query file: %w", err)
	}

	if current.Type == "file" {
		nextBaseName := nextName
		_, currentExt := splitName(current.Name)
		if nextBase, providedExt := splitName(nextName); providedExt != "" {
			if !strings.EqualFold(providedExt, currentExt) {
				return UpdateResponse{}, fmt.Errorf("%w: file extension cannot be changed", ErrInvalidArgument)
			}
			nextBaseName = nextBase
		}

		nextBaseName = strings.TrimSpace(nextBaseName)
		if nextBaseName == "" {
			return UpdateResponse{}, fmt.Errorf("%w: file name cannot be empty", ErrInvalidArgument)
		}
		nextName = nextBaseName + currentExt
	}

	updated := current
	if current.Name == nextName {
		return UpdateResponse{File: s.toFileGetResponse(current)}, nil
	}

	resolvedName, err := s.resolveEntryName(
		ctx,
		driveID,
		normalizeFolderID(current.ParentFileID),
		nextName,
		req.CheckNameMode,
	)
	if err != nil {
		return UpdateResponse{}, err
	}

	if resolvedName != current.Name {
		updated, err = s.repo.RenameEntry(ctx, driveID, fileID, resolvedName)
		if err != nil {
			if err == repository.ErrConflict {
				return UpdateResponse{}, fmt.Errorf("%w: file or folder already exists", ErrConflict)
			}
			if err == repository.ErrNotFound {
				return UpdateResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
			}
			return UpdateResponse{}, fmt.Errorf("update file: rename entry: %w", err)
		}
	}

	return UpdateResponse{File: s.toFileGetResponse(updated)}, nil
}

func (s *service) GetLatestAsyncTask(_ context.Context, _ GetLatestAsyncTaskRequest) (GetLatestAsyncTaskResponse, error) {
	return GetLatestAsyncTaskResponse{
		TotalProcess:         0,
		TotalFailedProcess:   0,
		TotalSkippedProcess:  0,
		TotalConsumedProcess: 0,
	}, nil
}

func (s *service) MoveFile(ctx context.Context, req MoveFileRequest) (MoveFileResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return MoveFileResponse{}, fmt.Errorf("move file: ensure drive: %w", err)
	}

	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return MoveFileResponse{}, fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}

	targetFolderID := normalizeFolderID(req.TargetFolderID)
	if err := s.ensureFolderExists(ctx, driveID, targetFolderID); err != nil {
		return MoveFileResponse{}, err
	}

	current, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return MoveFileResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return MoveFileResponse{}, fmt.Errorf("move file: query file: %w", err)
	}
	if current.Type != "file" {
		return MoveFileResponse{}, fmt.Errorf("%w: target is not a file", ErrInvalidArgument)
	}
	if current.ParentFileID == targetFolderID {
		return MoveFileResponse{File: s.mapSingleFile(ctx, current)}, nil
	}

	updated, err := s.repo.MoveEntry(ctx, driveID, fileID, targetFolderID)
	if err != nil {
		if err == repository.ErrConflict {
			return MoveFileResponse{}, fmt.Errorf("%w: target folder already has a file with the same name", ErrConflict)
		}
		if err == repository.ErrNotFound {
			return MoveFileResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return MoveFileResponse{}, fmt.Errorf("move file: update file: %w", err)
	}

	return MoveFileResponse{File: s.mapSingleFile(ctx, updated)}, nil
}

func (s *service) MoveFolder(ctx context.Context, req MoveFolderRequest) (MoveFolderResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return MoveFolderResponse{}, fmt.Errorf("move folder: ensure drive: %w", err)
	}

	folderID := normalizeFolderID(req.FolderID)
	if folderID == rootFolderID {
		return MoveFolderResponse{}, fmt.Errorf("%w: root folder cannot be moved", ErrInvalidArgument)
	}
	targetParentID := normalizeFolderID(req.TargetParentID)

	if err := s.ensureFolderExists(ctx, driveID, folderID); err != nil {
		return MoveFolderResponse{}, err
	}
	if err := s.ensureFolderExists(ctx, driveID, targetParentID); err != nil {
		return MoveFolderResponse{}, err
	}

	folderNodes, err := s.loadFolderNodes(ctx, driveID)
	if err != nil {
		return MoveFolderResponse{}, err
	}
	if isDescendantOrSelf(targetParentID, folderID, folderNodes) {
		return MoveFolderResponse{}, fmt.Errorf("%w: cannot move a folder into itself or its descendants", ErrInvalidArgument)
	}

	if _, err := s.repo.MoveEntry(ctx, driveID, folderID, targetParentID); err != nil {
		if err == repository.ErrConflict {
			return MoveFolderResponse{}, fmt.Errorf("%w: target parent already has a folder with the same name", ErrConflict)
		}
		if err == repository.ErrNotFound {
			return MoveFolderResponse{}, fmt.Errorf("%w: folder not found", ErrNotFound)
		}
		return MoveFolderResponse{}, fmt.Errorf("move folder: update folder: %w", err)
	}

	folderNodes, err = s.loadFolderNodes(ctx, driveID)
	if err != nil {
		return MoveFolderResponse{}, err
	}

	node, ok := folderNodes[folderID]
	if !ok {
		return MoveFolderResponse{}, fmt.Errorf("%w: folder not found", ErrNotFound)
	}

	return MoveFolderResponse{Folder: toFolderRecord(node, folderNodes)}, nil
}

func (s *service) Batch(ctx context.Context, req BatchRequest) (BatchResponse, error) {
	responses := make([]BatchResponseItem, 0, len(req.Requests))

	for _, item := range req.Requests {
		responseItem := BatchResponseItem{
			ID:     item.ID,
			Status: http.StatusInternalServerError,
		}
		if strings.TrimSpace(responseItem.ID) == "" {
			responseItem.ID = newHexID(16)
		}

		if strings.ToUpper(strings.TrimSpace(item.Method)) != "POST" {
			responseItem.Status = http.StatusBadRequest
			responseItem.Body = map[string]any{
				"code":    "InvalidParameter",
				"message": "unsupported method",
			}
			responses = append(responses, responseItem)
			continue
		}

		switch strings.TrimSpace(item.URL) {
		case "/file/move":
			driveID := s.normalizeDriveID(batchBodyString(item.Body, "drive_id"))
			fileID := strings.TrimSpace(batchBodyString(item.Body, "file_id"))
			toDriveID := s.normalizeDriveID(batchBodyString(item.Body, "to_drive_id"))
			if toDriveID == "" {
				toDriveID = driveID
			}
			if toDriveID != driveID {
				responseItem.Status = http.StatusBadRequest
				responseItem.Body = map[string]any{
					"code":    "InvalidParameter",
					"message": "cross-drive move is not supported",
				}
				responses = append(responses, responseItem)
				continue
			}
			toParentFileID := normalizeFolderID(batchBodyString(item.Body, "to_parent_file_id"))
			entryType := strings.ToLower(strings.TrimSpace(batchBodyString(item.Body, "type")))

			var moveErr error
			if entryType == "folder" {
				_, moveErr = s.MoveFolder(ctx, MoveFolderRequest{
					DriveID:        driveID,
					FolderID:       fileID,
					TargetParentID: toParentFileID,
				})
			} else {
				_, moveErr = s.MoveFile(ctx, MoveFileRequest{
					DriveID:        driveID,
					FileID:         fileID,
					TargetFolderID: toParentFileID,
				})
			}

			if moveErr != nil {
				responseItem.Status = batchErrorStatus(moveErr)
				responseItem.Body = map[string]any{
					"code":    "OperationFailed",
					"message": moveErr.Error(),
				}
				responses = append(responses, responseItem)
				continue
			}

			updated, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
			if err != nil {
				responseItem.Status = http.StatusInternalServerError
				responseItem.Body = map[string]any{
					"code":    "OperationFailed",
					"message": fmt.Sprintf("batch move: query updated file: %v", err),
				}
				responses = append(responses, responseItem)
				continue
			}

			responseItem.Status = http.StatusOK
			responseItem.Body = map[string]any{
				"domain_id":    s.uploadCfg.DomainID,
				"updated_at":   toRFC3339(updated.UpdatedAt),
				"drive_id":     updated.DriveID,
				"file_name":    updated.Name,
				"file_id":      updated.FileID,
				"download_url": "",
				"url":          "",
				"revision_id":  updated.RevisionID,
			}
		default:
			responseItem.Status = http.StatusBadRequest
			responseItem.Body = map[string]any{
				"code":    "InvalidParameter",
				"message": fmt.Sprintf("unsupported url: %s", item.URL),
			}
		}

		responses = append(responses, responseItem)
	}

	return BatchResponse{Responses: responses}, nil
}

func (s *service) GetFileAccessURL(ctx context.Context, req GetFileAccessURLRequest) (GetFileAccessURLResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	fileID := strings.TrimSpace(req.FileID)
	if fileID == "" {
		return GetFileAccessURLResponse{}, fmt.Errorf("%w: file_id is required", ErrInvalidArgument)
	}
	if err := s.ensureStorageAvailable(); err != nil {
		return GetFileAccessURLResponse{}, err
	}

	entry, err := s.repo.GetEntryByFileID(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return GetFileAccessURLResponse{}, fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return GetFileAccessURLResponse{}, fmt.Errorf("get file access url: query file: %w", err)
	}
	if entry.Type != "file" {
		return GetFileAccessURLResponse{}, fmt.Errorf("%w: target is not a file", ErrInvalidArgument)
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "preview" {
		mode = "download"
	}

	disposition := objectstorage.BuildContentDisposition(mode, entry.Name)
	url, err := s.storage.PresignGetObject(
		ctx,
		buildObjectKey(entry.DriveID, entry.FileID, entry.RevisionID),
		disposition,
		s.downloadURLTTL(),
	)
	if err != nil {
		return GetFileAccessURLResponse{}, fmt.Errorf("get file access url: presign get object: %w", err)
	}

	finalDisposition := "attachment"
	if mode == "preview" {
		finalDisposition = "inline"
	}

	return GetFileAccessURLResponse{
		File:             s.mapSingleFile(ctx, entry),
		URL:              url,
		Disposition:      finalDisposition,
		ExpiresInSeconds: int(s.downloadURLTTL().Seconds()),
	}, nil
}

func (s *service) ensureStorageAvailable() error {
	if s.storage == nil {
		return fmt.Errorf("%w: object storage is not configured", ErrInvalidArgument)
	}
	return nil
}

func (s *service) ensureFolderExists(ctx context.Context, driveID, folderID string) error {
	folderID = normalizeFolderID(folderID)
	if folderID == rootFolderID {
		return nil
	}

	record, err := s.repo.GetEntryByFileID(ctx, driveID, folderID)
	if err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: folder not found", ErrNotFound)
		}
		return fmt.Errorf("ensure folder exists: query folder: %w", err)
	}
	if record.Type != "folder" {
		return fmt.Errorf("%w: target is not a folder", ErrInvalidArgument)
	}
	return nil
}

func (s *service) resolveEntryName(
	ctx context.Context,
	driveID string,
	parentFileID string,
	name string,
	checkNameMode string,
) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidArgument)
	}

	mode := strings.ToLower(strings.TrimSpace(checkNameMode))
	if mode == "" {
		mode = "refuse"
	}

	existing, err := s.repo.GetEntryByParentAndName(ctx, driveID, parentFileID, normalized)
	if err == repository.ErrNotFound {
		return normalized, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve entry name: check existing entry: %w", err)
	}

	cleaned, err := s.cleanupStaleUploadEntry(ctx, driveID, existing)
	if err != nil {
		return "", err
	}
	if cleaned {
		return normalized, nil
	}

	if mode == "refuse" {
		return "", fmt.Errorf("%w: file or folder already exists", ErrConflict)
	}
	if mode == "overwrite" {
		if err := s.repo.DeleteEntryTree(ctx, driveID, existing.FileID); err != nil && err != repository.ErrNotFound {
			return "", fmt.Errorf("resolve entry name: overwrite existing entry: %w", err)
		}
		return normalized, nil
	}
	if mode != "auto_rename" {
		return "", fmt.Errorf("%w: unsupported check_name_mode: %s", ErrInvalidArgument, checkNameMode)
	}

	base, ext := splitName(normalized)
	for i := 1; i <= 9_999; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		existingCandidate, checkErr := s.repo.GetEntryByParentAndName(ctx, driveID, parentFileID, candidate)
		if checkErr == repository.ErrNotFound {
			return candidate, nil
		}
		if checkErr != nil {
			return "", fmt.Errorf("resolve entry name: check renamed candidate: %w", checkErr)
		}
		cleanedCandidate, cleanupErr := s.cleanupStaleUploadEntry(ctx, driveID, existingCandidate)
		if cleanupErr != nil {
			return "", cleanupErr
		}
		if cleanedCandidate {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("%w: failed to generate available name", ErrConflict)
}

func (s *service) buildPartInfoList(
	ctx context.Context,
	objectKey string,
	uploadID string,
	partNumbers []int,
	contentType string,
) ([]UploadPartInfo, error) {
	result := make([]UploadPartInfo, 0, len(partNumbers))
	for _, partNumber := range partNumbers {
		uploadURL, err := s.storage.PresignUploadPart(
			ctx,
			objectKey,
			uploadID,
			int32(partNumber),
			s.uploadURLTTL(),
		)
		if err != nil {
			return nil, fmt.Errorf("build part info list: presign part upload url: %w", err)
		}

		result = append(result, UploadPartInfo{
			PartNumber:        partNumber,
			UploadURL:         uploadURL,
			InternalUploadURL: "",
			ContentType:       contentType,
		})
	}
	return result, nil
}

func (s *service) buildSinglePutPartInfo(
	ctx context.Context,
	objectKey string,
	contentType string,
) ([]UploadPartInfo, error) {
	uploadURL, err := s.storage.PresignPutObject(ctx, objectKey, contentType, s.uploadURLTTL())
	if err != nil {
		return nil, fmt.Errorf("build single put part info: presign upload url: %w", err)
	}

	return []UploadPartInfo{
		{
			PartNumber:        1,
			UploadURL:         uploadURL,
			InternalUploadURL: "",
			ContentType:       contentType,
		},
	}, nil
}

func isSinglePutUploadSession(session repository.UploadSessionRecord) bool {
	return session.PartCount <= 1 && session.ChunkSize == 0
}

func validateSinglePutPartNumbers(partNumbers []int) error {
	if len(partNumbers) != 1 || partNumbers[0] != 1 {
		return fmt.Errorf("%w: single upload accepts only part_number = 1", ErrInvalidArgument)
	}
	return nil
}

func (s *service) collectCompletedPartsFromStorage(
	ctx context.Context,
	entry repository.EntryRecord,
	uploadID string,
	expectedPartCount int,
) ([]objectstorage.CompletedPart, []objectstorage.UploadedPart, error) {
	objectKey := buildObjectKey(entry.DriveID, entry.FileID, entry.RevisionID)
	uploadedParts, err := s.storage.ListUploadedParts(ctx, objectKey, uploadID)
	if err != nil {
		return nil, nil, fmt.Errorf("collect completed parts: list uploaded parts: %w", err)
	}
	if len(uploadedParts) == 0 {
		return nil, nil, fmt.Errorf("%w: not all parts are uploaded", ErrInvalidArgument)
	}

	partByNumber := make(map[int32]objectstorage.UploadedPart, len(uploadedParts))
	for _, part := range uploadedParts {
		if part.PartNumber <= 0 || strings.TrimSpace(part.ETag) == "" {
			continue
		}
		partByNumber[part.PartNumber] = part
	}

	if expectedPartCount <= 0 {
		expectedPartCount = len(partByNumber)
	}
	if expectedPartCount <= 0 {
		return nil, nil, fmt.Errorf("%w: not all parts are uploaded", ErrInvalidArgument)
	}

	completedParts := make([]objectstorage.CompletedPart, 0, expectedPartCount)
	orderedParts := make([]objectstorage.UploadedPart, 0, expectedPartCount)
	for partNumber := 1; partNumber <= expectedPartCount; partNumber++ {
		part, ok := partByNumber[int32(partNumber)]
		if !ok {
			return nil, nil, fmt.Errorf("%w: not all parts are uploaded", ErrInvalidArgument)
		}
		completedParts = append(completedParts, objectstorage.CompletedPart{
			PartNumber: int32(partNumber),
			ETag:       strings.TrimSpace(part.ETag),
		})
		orderedParts = append(orderedParts, part)
	}

	return completedParts, orderedParts, nil
}

func (s *service) toCompleteFileResponse(entry repository.EntryRecord, uploadID string) CompleteFileResponse {
	userMeta, userTags := defaultUserMetaAndTags()
	fileExtension := strings.TrimPrefix(strings.ToLower(path.Ext(entry.Name)), ".")
	contentHashName := resolveContentHashName(entry.ContentHash)
	contentType := resolveContentType("", entry.Name)

	return CompleteFileResponse{
		DriveID:                     entry.DriveID,
		DomainID:                    s.uploadCfg.DomainID,
		FileID:                      entry.FileID,
		Name:                        entry.Name,
		Type:                        entry.Type,
		ContentType:                 contentType,
		CreatedAt:                   toRFC3339(entry.CreatedAt),
		UpdatedAt:                   toRFC3339(entry.UpdatedAt),
		ModifiedAt:                  toRFC3339(entry.UpdatedAt),
		FileExtension:               fileExtension,
		Hidden:                      false,
		Size:                        entry.Size,
		Starred:                     false,
		Status:                      "available",
		UserMeta:                    userMeta,
		UploadID:                    uploadID,
		ParentFileID:                entry.ParentFileID,
		CRC64Hash:                   "",
		ContentHash:                 entry.ContentHash,
		ContentHashName:             contentHashName,
		Category:                    categoryFromMime(contentType),
		EncryptMode:                 entry.EncryptMode,
		MetaNamePunishFlag:          0,
		MetaNameInvestigationStatus: 0,
		CreatorType:                 "User",
		CreatorID:                   "",
		LastModifierType:            "User",
		LastModifierID:              "",
		UserTags:                    userTags,
		LocalModifiedAt:             "",
		RevisionID:                  entry.RevisionID,
		RevisionVersion:             1,
		SyncFlag:                    false,
		SyncDeviceFlag:              false,
		SyncMeta:                    "",
		Location:                    s.uploadCfg.Location,
		ContentURI:                  "",
	}
}

func (s *service) computeObjectContentHash(ctx context.Context, entry repository.EntryRecord) (string, error) {
	objectKey := buildObjectKey(entry.DriveID, entry.FileID, entry.RevisionID)
	reader, err := s.storage.OpenObject(ctx, objectKey)
	if err != nil {
		return "", fmt.Errorf("complete file: open object for hashing: %w", err)
	}
	defer reader.Close()

	hasher := sha1.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", fmt.Errorf("complete file: stream object for hashing: %w", err)
	}

	return strings.ToUpper(hex.EncodeToString(hasher.Sum(nil))), nil
}

func resolveContentHashName(contentHash string) string {
	hashValue := strings.TrimSpace(contentHash)
	if hashValue == "" {
		return ""
	}
	if strings.HasPrefix(hashValue, pendingHashPrefix) {
		return ""
	}
	return "sha1"
}

func defaultUserMetaAndTags() (string, map[string]string) {
	return `{"channel":"file_upload","client":"web"}`, map[string]string{
		"channel": "file_upload",
		"client":  "web",
	}
}

func (s *service) toListItem(item repository.EntryRecord, _ repository.UploadSessionRecord) ListItem {
	userMeta, userTags := defaultUserMetaAndTags()

	listItem := ListItem{
		CreatedAt:      toRFC3339(item.CreatedAt),
		DriveID:        item.DriveID,
		FileID:         item.FileID,
		Name:           item.Name,
		ParentFileID:   item.ParentFileID,
		Starred:        false,
		SyncDeviceFlag: false,
		SyncFlag:       false,
		SyncMeta:       "",
		Type:           item.Type,
		UpdatedAt:      toRFC3339(item.UpdatedAt),
		URL:            "",
		UserMeta:       userMeta,
		UserTags:       userTags,
	}

	if item.Type == "file" {
		contentType := resolveContentType("", item.Name)
		listItem.MimeType = contentType
		listItem.Category = categoryFromMime(contentType)
		listItem.ContentHash = item.ContentHash
		_, ext := splitName(item.Name)
		listItem.FileExtension = strings.TrimPrefix(strings.ToLower(ext), ".")
		listItem.Size = item.Size
		listItem.PunishFlag = 0
	}

	return listItem
}

func (s *service) toUploadedFileRecord(
	record repository.EntryRecord,
	folderPath string,
	session repository.UploadSessionRecord,
) UploadedFileRecord {
	fileExtension := ""
	if dot := strings.LastIndex(record.Name, "."); dot > 0 && dot+1 < len(record.Name) {
		fileExtension = record.Name[dot+1:]
	}

	strategy := UploadStrategySingle
	if record.UploadID == "" {
		strategy = UploadStrategyInstant
	} else if session.PartCount > 1 {
		strategy = UploadStrategyMultipart
	}

	return UploadedFileRecord{
		ID:             record.FileID,
		FileName:       record.Name,
		FileExtension:  fileExtension,
		FolderID:       record.ParentFileID,
		FolderPath:     folderPath,
		ContentType:    resolveContentType("", record.Name),
		FileSize:       record.Size,
		FileHash:       record.ContentHash,
		FileSampleHash: record.PreHash,
		ObjectKey:      buildObjectKey(record.DriveID, record.FileID, record.RevisionID),
		Bucket:         strings.TrimSpace(s.storageCfg.Bucket),
		Strategy:       strategy,
		CreatedAt:      toRFC3339(record.CreatedAt),
		UpdatedAt:      toRFC3339(record.UpdatedAt),
	}
}

func (s *service) mapSingleFile(ctx context.Context, record repository.EntryRecord) UploadedFileRecord {
	folderPath := s.folderPathByID(ctx, record.DriveID, record.ParentFileID)
	session := repository.UploadSessionRecord{}
	if record.UploadID != "" {
		if queried, err := s.repo.GetUploadSession(ctx, record.DriveID, record.UploadID); err == nil {
			session = queried
		}
	}
	return s.toUploadedFileRecord(record, folderPath, session)
}

func (s *service) folderPathByID(ctx context.Context, driveID, folderID string) string {
	nodes, err := s.loadFolderNodes(ctx, driveID)
	if err != nil {
		return ""
	}
	node, ok := nodes[normalizeFolderID(folderID)]
	if !ok {
		return ""
	}
	return node.Path
}

func (s *service) loadFolderNodes(ctx context.Context, driveID string) (map[string]*folderNode, error) {
	folderEntries, err := s.repo.ListEntries(ctx, repository.ListEntriesParams{
		DriveID:        driveID,
		Type:           "folder",
		OrderBy:        "name",
		OrderDirection: "ASC",
	})
	if err != nil {
		return nil, fmt.Errorf("load folder nodes: query folders: %w", err)
	}

	folderByID := make(map[string]repository.EntryRecord, len(folderEntries))
	for _, folder := range folderEntries {
		folderByID[folder.FileID] = folder
	}

	isUnderRecycle := func(fileID string) bool {
		currentID := strings.TrimSpace(fileID)
		visited := make(map[string]struct{}, 8)
		for currentID != "" && currentID != rootFolderID {
			if _, ok := visited[currentID]; ok {
				return false
			}
			visited[currentID] = struct{}{}

			current, ok := folderByID[currentID]
			if !ok {
				return false
			}

			parentID := normalizeFolderID(current.ParentFileID)
			if parentID == recycleBinFolderID {
				return true
			}
			currentID = parentID
		}
		return false
	}

	nodes := map[string]*folderNode{
		rootFolderID: {
			ID:        rootFolderID,
			Name:      rootFolderID,
			ParentID:  "",
			Path:      "",
			CreatedAt: time.Time{},
			UpdatedAt: time.Time{},
		},
	}

	for _, folder := range folderEntries {
		if isUnderRecycle(folder.FileID) {
			continue
		}

		nodes[folder.FileID] = &folderNode{
			ID:        folder.FileID,
			Name:      folder.Name,
			ParentID:  normalizeFolderID(folder.ParentFileID),
			CreatedAt: folder.CreatedAt,
			UpdatedAt: folder.UpdatedAt,
		}
	}

	var resolvePath func(id string, visiting map[string]bool) string
	resolvePath = func(id string, visiting map[string]bool) string {
		node, ok := nodes[id]
		if !ok {
			return ""
		}
		if node.Path != "" || id == rootFolderID {
			return node.Path
		}
		if visiting[id] {
			node.ParentID = rootFolderID
			node.Path = node.Name
			return node.Path
		}

		visiting[id] = true
		parentID := normalizeFolderID(node.ParentID)
		if parentID == id {
			parentID = rootFolderID
			node.ParentID = rootFolderID
		}
		parentNode, ok := nodes[parentID]
		if !ok {
			parentNode = nodes[rootFolderID]
			node.ParentID = rootFolderID
		}

		parentPath := resolvePath(parentNode.ID, visiting)
		node.Path = joinFolderPath(parentPath, node.Name)
		delete(visiting, id)
		return node.Path
	}

	for id := range nodes {
		resolvePath(id, map[string]bool{})
	}

	return nodes, nil
}

func (s *service) uploadURLTTL() time.Duration {
	ttl := s.uploadCfg.UploadURLTTLSecs
	if ttl <= 0 {
		ttl = defaultUploadURLTTLSecs
	}
	return time.Duration(ttl) * time.Second
}

func (s *service) downloadURLTTL() time.Duration {
	ttl := s.uploadCfg.DownloadURLTTLSecs
	if ttl <= 0 {
		ttl = defaultDownloadTTLSecs
	}
	return time.Duration(ttl) * time.Second
}

func (s *service) recycleRetention() time.Duration {
	days := s.uploadCfg.RecycleRetentionDays
	if days <= 0 {
		days = defaultRecycleRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

func (s *service) singlePutMaxSize() int64 {
	size := s.uploadCfg.SinglePutMaxSize
	if size <= 0 {
		size = defaultSinglePutMaxSize
	}
	return size
}

func (s *service) shouldUseSinglePutUpload(fileSize int64, partNumbers []int) bool {
	if fileSize < 0 {
		return false
	}
	if len(partNumbers) > 1 {
		return false
	}
	return fileSize <= s.singlePutMaxSize()
}

func (s *service) normalizeDriveID(driveID string) string {
	trimmed := strings.TrimSpace(driveID)
	if trimmed != "" {
		return trimmed
	}
	defaultDriveID := strings.TrimSpace(s.uploadCfg.DefaultDriveID)
	if defaultDriveID != "" {
		return defaultDriveID
	}
	return "default"
}

func normalizeFolderID(folderID string) string {
	trimmed := strings.TrimSpace(folderID)
	if trimmed == "" {
		return rootFolderID
	}
	return trimmed
}

func isInRecycleBin(entry repository.EntryRecord) bool {
	return normalizeFolderID(entry.ParentFileID) == recycleBinFolderID
}

func parseSearchQuery(query string) (parentFileID string, name string, err error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", "", fmt.Errorf("%w: query is required", ErrInvalidArgument)
	}

	parentFileID = rootFolderID
	if matched := parentFileIDPattern.FindStringSubmatch(trimmed); len(matched) == 2 {
		parentFileID = normalizeFolderID(matched[1])
	}

	if matched := namePattern.FindStringSubmatch(trimmed); len(matched) == 2 {
		name = strings.TrimSpace(matched[1])
	}
	if name == "" {
		return "", "", fmt.Errorf("%w: query must include name", ErrInvalidArgument)
	}

	return parentFileID, name, nil
}

func sanitizeLimit(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func normalizePartNumbers(parts []UploadPartRequest) ([]int, error) {
	if len(parts) == 0 {
		return nil, nil
	}

	seen := make(map[int]struct{}, len(parts))
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		if part.PartNumber <= 0 {
			return nil, fmt.Errorf("%w: part_number must be positive", ErrInvalidArgument)
		}
		if _, ok := seen[part.PartNumber]; ok {
			continue
		}
		seen[part.PartNumber] = struct{}{}
		result = append(result, part.PartNumber)
	}
	sort.Ints(result)
	return result, nil
}

func normalizeChunkSize(chunkSize, totalSize int64, partCount int) int64 {
	if chunkSize > 0 {
		return chunkSize
	}
	if totalSize > 0 && partCount > 0 {
		return int64(math.Ceil(float64(totalSize) / float64(partCount)))
	}
	return 0
}

func resolveContentType(contentType, fileName string) string {
	trimmed := strings.TrimSpace(contentType)
	if trimmed != "" {
		return trimmed
	}

	ext := strings.ToLower(path.Ext(strings.TrimSpace(fileName)))
	if ext != "" {
		if guessed := mime.TypeByExtension(ext); guessed != "" {
			return guessed
		}
	}
	return "application/octet-stream"
}

func categoryFromMime(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case strings.Contains(mimeType, "pdf"):
		return "doc"
	default:
		return "others"
	}
}

func buildObjectKey(driveID, fileID, revisionID string) string {
	return strings.Trim(strings.Join([]string{driveID, fileID, revisionID}, "/"), "/")
}

func toRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatFolderSizeSummary(size, fileCount, folderCount int64) string {
	return fmt.Sprintf("%s（包含 %d 个文件，%d 个文件夹）", formatBytes(size), fileCount, folderCount)
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(size)
	unitIndex := 0
	for value >= 1024 && unitIndex < len(units)-1 {
		value /= 1024
		unitIndex += 1
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", size, units[unitIndex])
	}
	return fmt.Sprintf("%.2f %s", value, units[unitIndex])
}

func toRFC3339Ptr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return toRFC3339(*value)
}

func newHexID(length int) string {
	if length <= 0 {
		return ""
	}

	raw := make([]byte, (length+1)/2)
	if _, err := rand.Read(raw); err != nil {
		fallback := fmt.Sprintf("%x", time.Now().UTC().UnixNano())
		if len(fallback) > length {
			return fallback[:length]
		}
		return fallback
	}
	encoded := hex.EncodeToString(raw)
	if len(encoded) > length {
		return encoded[:length]
	}
	return encoded
}

func batchBodyString(body map[string]any, key string) string {
	if body == nil {
		return ""
	}
	raw, ok := body[key]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprintf("%v", value)
	}
}

func batchErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrInvalidArgument):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func splitName(name string) (base, ext string) {
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 {
		return name, ""
	}
	return name[:dot], name[dot:]
}

func collectUploadIDs(entries []repository.EntryRecord) []string {
	if len(entries) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.UploadID) == "" {
			continue
		}
		set[entry.UploadID] = struct{}{}
	}

	ids := make([]string, 0, len(set))
	for uploadID := range set {
		ids = append(ids, uploadID)
	}
	return ids
}

func (s *service) buildUploadSessionMap(
	ctx context.Context,
	driveID string,
	entries []repository.EntryRecord,
) (map[string]repository.UploadSessionRecord, error) {
	uploadIDs := collectUploadIDs(entries)
	if len(uploadIDs) == 0 {
		return map[string]repository.UploadSessionRecord{}, nil
	}

	sessionMap, err := s.repo.GetUploadSessionsByUploadIDs(ctx, driveID, uploadIDs)
	if err != nil {
		return nil, err
	}

	return sessionMap, nil
}

func isVisibleEntry(entry repository.EntryRecord, sessionMap map[string]repository.UploadSessionRecord) bool {
	if entry.Type != "file" {
		return true
	}

	uploadID := strings.TrimSpace(entry.UploadID)
	if uploadID == "" {
		return true
	}

	session, ok := sessionMap[uploadID]
	if !ok {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(session.Status), "completed")
}

func (s *service) cleanupStaleUploadEntry(
	ctx context.Context,
	driveID string,
	existing repository.EntryRecord,
) (bool, error) {
	if existing.Type != "file" {
		return false, nil
	}

	uploadID := strings.TrimSpace(existing.UploadID)
	if uploadID == "" {
		return false, nil
	}

	session, err := s.repo.GetUploadSession(ctx, driveID, uploadID)
	if err == nil {
		if strings.EqualFold(strings.TrimSpace(session.Status), "completed") {
			return false, nil
		}
	} else if err != repository.ErrNotFound {
		return false, fmt.Errorf("resolve entry name: query upload session: %w", err)
	}

	if err := s.repo.DeleteEntryTree(ctx, driveID, existing.FileID); err != nil && err != repository.ErrNotFound {
		return false, fmt.Errorf("resolve entry name: cleanup stale entry: %w", err)
	}
	return true, nil
}

func toFolderRecord(node *folderNode, nodes map[string]*folderNode) UploadFolderRecord {
	var parentID *string
	if node.ID != rootFolderID {
		parent := normalizeFolderID(node.ParentID)
		parentID = &parent
	}

	parentPath := ""
	if parentID != nil {
		if parent, ok := nodes[*parentID]; ok {
			parentPath = parent.Path
		}
	}

	return UploadFolderRecord{
		ID:         node.ID,
		FolderName: node.Name,
		ParentID:   parentID,
		FolderPath: node.Path,
		ParentPath: parentPath,
		CreatedAt:  toRFC3339(node.CreatedAt),
		UpdatedAt:  toRFC3339(node.UpdatedAt),
	}
}

func isDescendantOrSelf(candidateID, ancestorID string, nodes map[string]*folderNode) bool {
	candidateID = normalizeFolderID(candidateID)
	ancestorID = normalizeFolderID(ancestorID)
	if candidateID == ancestorID {
		return true
	}

	currentID := candidateID
	seen := make(map[string]struct{}, 8)
	for currentID != "" && currentID != rootFolderID {
		if _, ok := seen[currentID]; ok {
			return false
		}
		seen[currentID] = struct{}{}

		node, ok := nodes[currentID]
		if !ok {
			return false
		}
		parentID := normalizeFolderID(node.ParentID)
		if parentID == ancestorID {
			return true
		}
		currentID = parentID
	}
	return false
}

func joinFolderPath(parentPath, name string) string {
	parentPath = strings.Trim(strings.TrimSpace(parentPath), "/")
	name = strings.Trim(strings.TrimSpace(name), "/")
	if parentPath == "" {
		return name
	}
	if name == "" {
		return parentPath
	}
	return parentPath + "/" + name
}
