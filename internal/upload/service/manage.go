package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"molly-server/internal/upload/repository"
)

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

	deletedRecords, err := s.repo.DeleteEntryTree(ctx, driveID, fileID)
	if err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("%w: file not found", ErrNotFound)
		}
		return fmt.Errorf("file delete: delete entry tree: %w", err)
	}
	s.scheduleObjectDeletion(deletedRecords)
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
