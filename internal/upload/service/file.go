package service

import (
	"context"
	"fmt"
	"strings"

	"molly-server/internal/upload/repository"
)

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

	stats, err := s.repo.GetSubtreeStats(ctx, driveID, fileID)
	if err != nil {
		return GetFolderSizeInfoResponse{}, fmt.Errorf("get folder size info: query subtree stats: %w", err)
	}

	return GetFolderSizeInfoResponse{
		Size:           stats.Size,
		FolderCount:    stats.FolderCount,
		FileCount:      stats.FileCount,
		DisplaySummary: formatFolderSizeSummary(stats.Size, stats.FileCount, stats.FolderCount),
	}, nil
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
