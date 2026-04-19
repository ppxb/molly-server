package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"molly-server/internal/upload/repository"
	"molly-server/pkg/objectstorage"
)

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
		if err := s.repo.SetUploadSessionStatus(ctx, uploadID, "completed"); err != nil && err != repository.ErrNotFound {
			return CompleteFileResponse{}, fmt.Errorf("complete file: set session status: %w", err)
		}

		s.schedulePostCompleteProcessing(entry, uploadID, []objectstorage.UploadedPart{
			{
				PartNumber: 1,
				ETag:       "",
				Size:       entry.Size,
			},
		})

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

	if err := s.repo.SetUploadSessionStatus(ctx, uploadID, "completed"); err != nil && err != repository.ErrNotFound {
		return CompleteFileResponse{}, fmt.Errorf("complete file: set session status: %w", err)
	}

	s.schedulePostCompleteProcessing(entry, uploadID, uploadedParts)

	return s.toCompleteFileResponse(entry, uploadID), nil
}
