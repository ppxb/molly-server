package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

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
