package service

import (
	"context"
	"fmt"
	"strings"

	"molly-server/internal/upload/repository"
	"molly-server/pkg/objectstorage"
)

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
