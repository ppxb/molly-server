package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"molly-server/internal/upload/repository"
)

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

func (s *service) RecycleBinClear(ctx context.Context, req RecycleBinClearRequest) (RecycleBinClearResponse, error) {
	driveID := s.normalizeDriveID(req.DriveID)
	if err := s.repo.EnsureDrive(ctx, driveID); err != nil {
		return RecycleBinClearResponse{}, fmt.Errorf("recyclebin clear: ensure drive: %w", err)
	}

	entries, err := s.repo.ListEntries(ctx, repository.ListEntriesParams{
		DriveID:      driveID,
		ParentFileID: recycleBinFolderID,
	})
	if err != nil {
		return RecycleBinClearResponse{}, fmt.Errorf("recyclebin clear: query recycle bin entries: %w", err)
	}

	for _, entry := range entries {
		deletedRecords, err := s.repo.DeleteEntryTree(ctx, driveID, entry.FileID)
		if err != nil && err != repository.ErrNotFound {
			return RecycleBinClearResponse{}, fmt.Errorf("recyclebin clear: delete entry tree: %w", err)
		}
		s.scheduleObjectDeletion(deletedRecords)
	}

	taskID := fmt.Sprintf("%sclear", driveID)
	return RecycleBinClearResponse{
		DomainID:    s.uploadCfg.DomainID,
		DriveID:     driveID,
		TaskID:      taskID,
		AsyncTaskID: taskID,
	}, nil
}
