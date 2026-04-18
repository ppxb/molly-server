package api

import "molly-server/internal/upload/service"

type searchRequest struct {
	Query   string `json:"query" binding:"required"`
	OrderBy string `json:"order_by"`
	Limit   int    `json:"limit"`
	DriveID string `json:"drive_id"`
}

type searchItem struct {
	DriveID      string `json:"drive_id"`
	FileID       string `json:"file_id"`
	ParentFileID string `json:"parent_file_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Size         int64  `json:"size"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type searchResponse struct {
	Items      []searchItem `json:"items"`
	NextMarker string       `json:"next_marker"`
}

type fileListRequest struct {
	DriveID               string `json:"drive_id"`
	ParentFileID          string `json:"parent_file_id"`
	Limit                 int    `json:"limit"`
	All                   bool   `json:"all"`
	URLExpireSec          int    `json:"url_expire_sec"`
	OrderBy               string `json:"order_by"`
	OrderDirection        string `json:"order_direction"`
	Fields                string `json:"fields"`
	ImageThumbnailProcess string `json:"image_thumbnail_process"`
	ImageURLProcess       string `json:"image_url_process"`
	VideoThumbnailProcess string `json:"video_thumbnail_process"`
}

type fileListItem struct {
	Category       string            `json:"category,omitempty"`
	ContentHash    string            `json:"content_hash,omitempty"`
	CreatedAt      string            `json:"created_at"`
	DriveID        string            `json:"drive_id"`
	FileID         string            `json:"file_id"`
	MimeType       string            `json:"mime_type,omitempty"`
	Name           string            `json:"name"`
	ParentFileID   string            `json:"parent_file_id"`
	PunishFlag     int               `json:"punish_flag,omitempty"`
	Size           int64             `json:"size,omitempty"`
	Starred        bool              `json:"starred"`
	SyncDeviceFlag bool              `json:"sync_device_flag"`
	SyncFlag       bool              `json:"sync_flag"`
	SyncMeta       string            `json:"sync_meta"`
	Type           string            `json:"type"`
	UpdatedAt      string            `json:"updated_at"`
	URL            string            `json:"url"`
	UserMeta       string            `json:"user_meta,omitempty"`
	UserTags       map[string]string `json:"user_tags,omitempty"`
}

type fileListResponse struct {
	Items      []fileListItem `json:"items"`
	NextMarker string         `json:"next_marker"`
}

type fileGetRequest struct {
	DriveID string `json:"drive_id"`
	FileID  string `json:"file_id" binding:"required"`
}

type fileGetResponse struct {
	DriveID                     string `json:"drive_id"`
	DomainID                    string `json:"domain_id"`
	FileID                      string `json:"file_id"`
	Name                        string `json:"name"`
	Type                        string `json:"type"`
	ContentType                 string `json:"content_type,omitempty"`
	CreatedAt                   string `json:"created_at"`
	UpdatedAt                   string `json:"updated_at"`
	Hidden                      bool   `json:"hidden"`
	Starred                     bool   `json:"starred"`
	Status                      string `json:"status"`
	ParentFileID                string `json:"parent_file_id"`
	EncryptMode                 string `json:"encrypt_mode"`
	MetaNamePunishFlag          int    `json:"meta_name_punish_flag"`
	MetaNameInvestigationStatus int    `json:"meta_name_investigation_status"`
	CreatorType                 string `json:"creator_type,omitempty"`
	CreatorID                   string `json:"creator_id,omitempty"`
	LastModifierType            string `json:"last_modifier_type,omitempty"`
	LastModifierID              string `json:"last_modifier_id,omitempty"`
	SyncFlag                    bool   `json:"sync_flag"`
	SyncDeviceFlag              bool   `json:"sync_device_flag"`
	SyncMeta                    string `json:"sync_meta"`
	Trashed                     bool   `json:"trashed"`
	DownloadURL                 string `json:"download_url"`
	URL                         string `json:"url"`
}

type fileGetPathRequest struct {
	DriveID string `json:"drive_id"`
	FileID  string `json:"file_id" binding:"required"`
}

type fileGetPathItem struct {
	Trashed      bool   `json:"trashed"`
	DriveID      string `json:"drive_id"`
	FileID       string `json:"file_id"`
	CreatedAt    string `json:"created_at"`
	DomainID     string `json:"domain_id"`
	EncryptMode  string `json:"encrypt_mode"`
	Hidden       bool   `json:"hidden"`
	Name         string `json:"name"`
	ParentFileID string `json:"parent_file_id"`
	Starred      bool   `json:"starred"`
	Status       string `json:"status"`
	Type         string `json:"type"`
	UpdatedAt    string `json:"updated_at"`
	SyncFlag     bool   `json:"sync_flag"`
}

type fileGetPathResponse struct {
	Items []fileGetPathItem `json:"items"`
}

type createPartInfoRequest struct {
	PartNumber int `json:"part_number" binding:"required,min=1"`
}

type createWithFoldersRequest struct {
	DriveID         string                  `json:"drive_id"`
	PartInfoList    []createPartInfoRequest `json:"part_info_list"`
	ParentFileID    string                  `json:"parent_file_id"`
	Name            string                  `json:"name" binding:"required"`
	Type            string                  `json:"type" binding:"required,oneof=file folder"`
	CheckNameMode   string                  `json:"check_name_mode"`
	Size            int64                   `json:"size"`
	CreateScene     string                  `json:"create_scene"`
	DeviceName      string                  `json:"device_name"`
	LocalModifiedAt string                  `json:"local_modified_at"`
	PreHash         string                  `json:"pre_hash"`
	ContentType     string                  `json:"content_type"`
	ChunkSize       int64                   `json:"chunk_size"`
}

type uploadSignatureInfoResponse struct {
	AuthType string `json:"auth_type"`
	STSToken string `json:"sts_token"`
}

type uploadPartInfoResponse struct {
	PartNumber        int                         `json:"part_number"`
	UploadURL         string                      `json:"upload_url"`
	InternalUploadURL string                      `json:"internal_upload_url"`
	ContentType       string                      `json:"content_type"`
	SignatureInfo     uploadSignatureInfoResponse `json:"signature_info"`
}

type createWithFoldersResponse struct {
	ParentFileID string                   `json:"parent_file_id"`
	PartInfoList []uploadPartInfoResponse `json:"part_info_list,omitempty"`
	UploadID     string                   `json:"upload_id,omitempty"`
	RapidUpload  bool                     `json:"rapid_upload,omitempty"`
	Type         string                   `json:"type"`
	FileID       string                   `json:"file_id"`
	RevisionID   string                   `json:"revision_id,omitempty"`
	DomainID     string                   `json:"domain_id"`
	DriveID      string                   `json:"drive_id"`
	FileName     string                   `json:"file_name"`
	EncryptMode  string                   `json:"encrypt_mode"`
	Location     string                   `json:"location,omitempty"`
	CreatedAt    string                   `json:"created_at,omitempty"`
	UpdatedAt    string                   `json:"updated_at,omitempty"`
}

type getUploadURLRequest struct {
	DriveID      string                  `json:"drive_id"`
	UploadID     string                  `json:"upload_id" binding:"required"`
	PartInfoList []createPartInfoRequest `json:"part_info_list" binding:"required"`
	FileID       string                  `json:"file_id" binding:"required"`
}

type getUploadURLResponse struct {
	DomainID     string                   `json:"domain_id"`
	DriveID      string                   `json:"drive_id"`
	FileID       string                   `json:"file_id"`
	PartInfoList []uploadPartInfoResponse `json:"part_info_list"`
	UploadID     string                   `json:"upload_id"`
	CreateAt     string                   `json:"create_at"`
}

type completeFileRequest struct {
	DriveID  string `json:"drive_id"`
	UploadID string `json:"upload_id" binding:"required"`
	FileID   string `json:"file_id" binding:"required"`
}

type completeFileResponse struct {
	DriveID                     string            `json:"drive_id"`
	DomainID                    string            `json:"domain_id"`
	FileID                      string            `json:"file_id"`
	Name                        string            `json:"name"`
	Type                        string            `json:"type"`
	ContentType                 string            `json:"content_type"`
	CreatedAt                   string            `json:"created_at"`
	UpdatedAt                   string            `json:"updated_at"`
	ModifiedAt                  string            `json:"modified_at"`
	FileExtension               string            `json:"file_extension,omitempty"`
	Hidden                      bool              `json:"hidden"`
	Size                        int64             `json:"size"`
	Starred                     bool              `json:"starred"`
	Status                      string            `json:"status"`
	UserMeta                    string            `json:"user_meta,omitempty"`
	UploadID                    string            `json:"upload_id"`
	ParentFileID                string            `json:"parent_file_id"`
	CRC64Hash                   string            `json:"crc64_hash,omitempty"`
	ContentHash                 string            `json:"content_hash,omitempty"`
	ContentHashName             string            `json:"content_hash_name,omitempty"`
	Category                    string            `json:"category,omitempty"`
	EncryptMode                 string            `json:"encrypt_mode"`
	MetaNamePunishFlag          int               `json:"meta_name_punish_flag"`
	MetaNameInvestigationStatus int               `json:"meta_name_investigation_status"`
	CreatorType                 string            `json:"creator_type,omitempty"`
	CreatorID                   string            `json:"creator_id,omitempty"`
	LastModifierType            string            `json:"last_modifier_type,omitempty"`
	LastModifierID              string            `json:"last_modifier_id,omitempty"`
	UserTags                    map[string]string `json:"user_tags,omitempty"`
	LocalModifiedAt             string            `json:"local_modified_at,omitempty"`
	RevisionID                  string            `json:"revision_id"`
	RevisionVersion             int               `json:"revision_version"`
	SyncFlag                    bool              `json:"sync_flag"`
	SyncDeviceFlag              bool              `json:"sync_device_flag"`
	SyncMeta                    string            `json:"sync_meta"`
	Location                    string            `json:"location"`
	ContentURI                  string            `json:"content_uri,omitempty"`
}

type uploadFolderRecord struct {
	ID         string  `json:"id"`
	FolderName string  `json:"folderName"`
	ParentID   *string `json:"parentId"`
	FolderPath string  `json:"folderPath"`
	ParentPath string  `json:"parentPath"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

type uploadedFileRecord struct {
	ID             string `json:"id"`
	FileName       string `json:"fileName"`
	FileExtension  string `json:"fileExtension"`
	FolderID       string `json:"folderId"`
	FolderPath     string `json:"folderPath"`
	ContentType    string `json:"contentType"`
	FileSize       int64  `json:"fileSize"`
	FileHash       string `json:"fileHash"`
	FileSampleHash string `json:"fileSampleHash"`
	ObjectKey      string `json:"objectKey"`
	Bucket         string `json:"bucket"`
	Strategy       string `json:"strategy"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type listUploadMoveTargetsRequest struct {
	DriveID         string `json:"drive_id"`
	ExcludeFolderID string `json:"excludeFolderId"`
}

type listUploadMoveTargetsResponse struct {
	Folders []uploadFolderRecord `json:"folders"`
}

type fileUpdateRequest struct {
	DriveID       string `json:"drive_id"`
	FileID        string `json:"file_id" binding:"required"`
	Name          string `json:"name" binding:"required"`
	CheckNameMode string `json:"check_name_mode"`
}

type fileGetLatestAsyncTaskRequest struct {
	DriveID string `json:"drive_id"`
}

type fileGetLatestAsyncTaskResponse struct {
	TotalProcess         int64 `json:"total_process"`
	TotalFailedProcess   int64 `json:"total_failed_process"`
	TotalSkippedProcess  int64 `json:"total_skipped_process"`
	TotalConsumedProcess int64 `json:"total_consumed_process"`
}

type uploadBatchRequestItem struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    map[string]any    `json:"body"`
}

type uploadBatchRequest struct {
	Resource string                   `json:"resource"`
	Requests []uploadBatchRequestItem `json:"requests"`
}

type uploadBatchResponseItem struct {
	ID     string         `json:"id"`
	Status int            `json:"status"`
	Body   map[string]any `json:"body,omitempty"`
}

type uploadBatchResponse struct {
	Responses []uploadBatchResponseItem `json:"responses"`
}

type getFileAccessURLRequest struct {
	DriveID string `json:"drive_id"`
	FileID  string `json:"fileId" binding:"required"`
	Mode    string `json:"mode"`
}

type getFileAccessURLResponse struct {
	File             uploadedFileRecord `json:"file"`
	URL              string             `json:"url"`
	Disposition      string             `json:"disposition"`
	ExpiresInSeconds int                `json:"expiresInSeconds"`
}

func toUploadPartInfoResponse(items []service.UploadPartInfo) []uploadPartInfoResponse {
	result := make([]uploadPartInfoResponse, 0, len(items))
	for _, item := range items {
		result = append(result, uploadPartInfoResponse{
			PartNumber:        item.PartNumber,
			UploadURL:         item.UploadURL,
			InternalUploadURL: item.InternalUploadURL,
			ContentType:       item.ContentType,
			SignatureInfo: uploadSignatureInfoResponse{
				AuthType: "url",
				STSToken: "",
			},
		})
	}
	return result
}

func toUploadFolderRecord(record service.UploadFolderRecord) uploadFolderRecord {
	return uploadFolderRecord{
		ID:         record.ID,
		FolderName: record.FolderName,
		ParentID:   record.ParentID,
		FolderPath: record.FolderPath,
		ParentPath: record.ParentPath,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
}

func toUploadedFileRecord(record service.UploadedFileRecord) uploadedFileRecord {
	return uploadedFileRecord{
		ID:             record.ID,
		FileName:       record.FileName,
		FileExtension:  record.FileExtension,
		FolderID:       record.FolderID,
		FolderPath:     record.FolderPath,
		ContentType:    record.ContentType,
		FileSize:       record.FileSize,
		FileHash:       record.FileHash,
		FileSampleHash: record.FileSampleHash,
		ObjectKey:      record.ObjectKey,
		Bucket:         record.Bucket,
		Strategy:       string(record.Strategy),
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}
