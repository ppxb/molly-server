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

type folderNode struct {
	ID        string
	Name      string
	ParentID  string
	Path      string
	CreatedAt time.Time
	UpdatedAt time.Time
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
		deletedRecords, err := s.repo.DeleteEntryTree(ctx, driveID, existing.FileID)
		if err != nil && err != repository.ErrNotFound {
			return "", fmt.Errorf("resolve entry name: overwrite existing entry: %w", err)
		}
		s.scheduleObjectDeletion(deletedRecords)
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

func (s *service) scheduleObjectDeletion(entries []repository.EntryRecord) {
	if s.storage == nil || len(entries) == 0 {
		return
	}

	keys := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		revisionID := strings.TrimSpace(entry.RevisionID)
		if revisionID == "" {
			continue
		}
		key := buildObjectKey(entry.DriveID, entry.FileID, revisionID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return
	}

	go func(deleteKeys []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for _, key := range deleteKeys {
			_ = s.storage.DeleteObject(ctx, key)
		}
	}(keys)
}

func (s *service) schedulePostCompleteProcessing(
	entry repository.EntryRecord,
	uploadID string,
	uploadedParts []objectstorage.UploadedPart,
) {
	if s.storage == nil {
		return
	}

	go func(entry repository.EntryRecord, uploadID string, uploadedParts []objectstorage.UploadedPart) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		for _, part := range uploadedParts {
			_ = s.repo.MarkUploadPartUploaded(ctx, uploadID, int(part.PartNumber), part.Size, part.ETag)
		}

		finalHash, err := s.computeObjectContentHash(ctx, entry)
		if err != nil {
			return
		}
		if strings.TrimSpace(finalHash) == "" || finalHash == entry.ContentHash {
			return
		}

		_, _ = s.repo.UpdateEntryHash(ctx, entry.DriveID, entry.FileID, finalHash)
	}(entry, uploadID, append([]objectstorage.UploadedPart(nil), uploadedParts...))
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
		return false, fmt.Errorf("resolve entry name: cleanup stale entry: %w", err)
	}

	deletedRecords, err := s.repo.DeleteEntryTree(ctx, driveID, existing.FileID)
	if err != nil && err != repository.ErrNotFound {
		return false, fmt.Errorf("resolve entry name: cleanup stale entry: %w", err)
	}
	s.scheduleObjectDeletion(deletedRecords)
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
