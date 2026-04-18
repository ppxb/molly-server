package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"molly-server/ent"
	"molly-server/ent/drive"
	"molly-server/ent/entry"
	"molly-server/ent/uploadpart"
	"molly-server/ent/uploadsession"
)

var (
	ErrNotFound = errors.New("upload repository: not found")
	ErrConflict = errors.New("upload repository: conflict")
)

type Repository struct {
	client *ent.Client
}

type SearchEntriesParams struct {
	DriveID      string
	ParentFileID string
	Name         string
	OrderBy      string
	Limit        int
}

type ListEntriesParams struct {
	DriveID        string
	ParentFileID   string
	Type           string
	Limit          int
	OrderBy        string
	OrderDirection string
}

type EntryRecord struct {
	DriveID             string
	FileID              string
	ParentFileID        string
	Name                string
	Type                string
	Size                int64
	ContentHash         string
	PreHash             string
	UploadID            string
	TrashedParentFileID string
	RevisionID          string
	EncryptMode         string
	TrashedAt           *time.Time
	ExpiredAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type CreateFolderParams struct {
	DriveID      string
	ParentFileID string
	Name         string
	FileID       string
	RevisionID   string
}

type CreateFileWithUploadParams struct {
	DriveID      string
	ParentFileID string
	Name         string
	FileID       string
	RevisionID   string
	UploadID     string
	Size         int64
	PreHash      string
	PartNumbers  []int
	ChunkSize    int64
	ExpiresAt    time.Time
}

type UploadSessionRecord struct {
	DriveID   string
	UploadID  string
	FileID    string
	PartCount int
	ChunkSize int64
	ExpiresAt *time.Time
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UploadPartRecord struct {
	UploadID   string
	PartNumber int
	Size       int64
	ETag       string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func New(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) EnsureDrive(ctx context.Context, driveID string) error {
	if driveID == "" {
		return fmt.Errorf("ensure drive: empty drive id")
	}

	_, err := r.client.Drive.Query().Where(drive.DriveIDEQ(driveID)).Only(ctx)
	if err == nil {
		return nil
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("ensure drive: query drive: %w", err)
	}

	if _, err := r.client.Drive.Create().SetDriveID(driveID).SetName("default").Save(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return nil
		}
		return fmt.Errorf("ensure drive: create drive: %w", err)
	}

	return nil
}

func (r *Repository) SearchEntries(ctx context.Context, params SearchEntriesParams) ([]EntryRecord, error) {
	query := r.client.Entry.Query().Where(entry.DriveIDEQ(params.DriveID))

	if params.ParentFileID != "" {
		query = query.Where(entry.ParentFileIDEQ(params.ParentFileID))
	}
	if params.Name != "" {
		query = query.Where(entry.NameEQ(params.Name))
	}
	applyEntryOrder(query, params.OrderBy, "DESC")

	if params.Limit > 0 {
		query = query.Limit(params.Limit)
	}

	records, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("search entries: %w", err)
	}

	items := make([]EntryRecord, 0, len(records))
	for _, item := range records {
		items = append(items, mapEntryRecord(item))
	}
	return items, nil
}

func (r *Repository) ListEntries(ctx context.Context, params ListEntriesParams) ([]EntryRecord, error) {
	query := r.client.Entry.Query().Where(entry.DriveIDEQ(params.DriveID))
	if params.ParentFileID != "" {
		query = query.Where(entry.ParentFileIDEQ(params.ParentFileID))
	}
	if params.Type != "" {
		switch strings.ToLower(strings.TrimSpace(params.Type)) {
		case "file":
			query = query.Where(entry.TypeEQ(entry.TypeFile))
		case "folder":
			query = query.Where(entry.TypeEQ(entry.TypeFolder))
		}
	}

	applyEntryOrder(query, params.OrderBy, params.OrderDirection)

	if params.Limit > 0 {
		query = query.Limit(params.Limit)
	}

	records, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}

	result := make([]EntryRecord, 0, len(records))
	for _, record := range records {
		result = append(result, mapEntryRecord(record))
	}
	return result, nil
}

func applyEntryOrder(query *ent.EntryQuery, orderBy, orderDirection string) {
	direction := strings.ToUpper(strings.TrimSpace(orderDirection))
	if direction != "ASC" {
		direction = "DESC"
	}

	column := strings.ToLower(strings.TrimSpace(orderBy))
	if column == "" {
		column = "updated_at"
	}

	asc := direction == "ASC"
	switch column {
	case "name":
		if asc {
			query.Order(ent.Asc(entry.FieldName))
		} else {
			query.Order(ent.Desc(entry.FieldName))
		}
	case "created_at":
		if asc {
			query.Order(ent.Asc(entry.FieldCreatedAt))
		} else {
			query.Order(ent.Desc(entry.FieldCreatedAt))
		}
	case "size":
		if asc {
			query.Order(ent.Asc(entry.FieldSize))
		} else {
			query.Order(ent.Desc(entry.FieldSize))
		}
	default:
		if asc {
			query.Order(ent.Asc(entry.FieldUpdatedAt))
		} else {
			query.Order(ent.Desc(entry.FieldUpdatedAt))
		}
	}
}

func (r *Repository) ExistsEntry(ctx context.Context, driveID, parentFileID, name string) (bool, error) {
	exists, err := r.client.Entry.Query().Where(
		entry.DriveIDEQ(driveID),
		entry.ParentFileIDEQ(parentFileID),
		entry.NameEQ(name),
	).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("exists entry: %w", err)
	}
	return exists, nil
}

func (r *Repository) CreateFolder(ctx context.Context, params CreateFolderParams) (EntryRecord, error) {
	record, err := r.client.Entry.Create().
		SetDriveID(params.DriveID).
		SetFileID(params.FileID).
		SetParentFileID(params.ParentFileID).
		SetName(params.Name).
		SetType(entry.TypeFolder).
		SetSize(0).
		SetRevisionID(params.RevisionID).
		SetEncryptMode("none").
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("create folder: %w", err)
	}

	return mapEntryRecord(record), nil
}

func (r *Repository) CreateFileWithUpload(ctx context.Context, params CreateFileWithUploadParams) (EntryRecord, error) {
	if len(params.PartNumbers) == 0 {
		return EntryRecord{}, fmt.Errorf("create file: empty part list")
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return EntryRecord{}, fmt.Errorf("create file: open transaction: %w", err)
	}

	record, err := tx.Entry.Create().
		SetDriveID(params.DriveID).
		SetFileID(params.FileID).
		SetParentFileID(params.ParentFileID).
		SetName(params.Name).
		SetType(entry.TypeFile).
		SetSize(params.Size).
		SetUploadID(params.UploadID).
		SetRevisionID(params.RevisionID).
		SetEncryptMode("none").
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("create file: create entry: %w", err)
	}

	if params.PreHash != "" {
		record, err = tx.Entry.UpdateOneID(record.ID).SetPreHash(params.PreHash).Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return EntryRecord{}, fmt.Errorf("create file: update pre_hash: %w", err)
		}
	}

	_, err = tx.UploadSession.Create().
		SetDriveID(params.DriveID).
		SetUploadID(params.UploadID).
		SetFileID(params.FileID).
		SetPartCount(len(params.PartNumbers)).
		SetChunkSize(params.ChunkSize).
		SetStatus(uploadsession.StatusInit).
		SetExpiresAt(params.ExpiresAt).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return EntryRecord{}, fmt.Errorf("create file: create upload session: %w", err)
	}

	for _, partNumber := range params.PartNumbers {
		_, err := tx.UploadPart.Create().
			SetUploadID(params.UploadID).
			SetPartNumber(partNumber).
			SetStatus(uploadpart.StatusPending).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				continue
			}
			_ = tx.Rollback()
			return EntryRecord{}, fmt.Errorf("create file: create upload part: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return EntryRecord{}, fmt.Errorf("create file: commit transaction: %w", err)
	}

	return mapEntryRecord(record), nil
}

func (r *Repository) GetEntryByFileID(ctx context.Context, driveID, fileID string) (EntryRecord, error) {
	record, err := r.client.Entry.Query().Where(
		entry.DriveIDEQ(driveID),
		entry.FileIDEQ(fileID),
	).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return EntryRecord{}, ErrNotFound
		}
		return EntryRecord{}, fmt.Errorf("get entry by file id: %w", err)
	}
	return mapEntryRecord(record), nil
}

func (r *Repository) GetEntryByParentAndName(ctx context.Context, driveID, parentFileID, name string) (EntryRecord, error) {
	record, err := r.client.Entry.Query().Where(
		entry.DriveIDEQ(driveID),
		entry.ParentFileIDEQ(parentFileID),
		entry.NameEQ(name),
	).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return EntryRecord{}, ErrNotFound
		}
		return EntryRecord{}, fmt.Errorf("get entry by parent and name: %w", err)
	}
	return mapEntryRecord(record), nil
}

func (r *Repository) RenameEntry(ctx context.Context, driveID, fileID, newName string) (EntryRecord, error) {
	record, err := r.client.Entry.Update().
		Where(entry.DriveIDEQ(driveID), entry.FileIDEQ(fileID)).
		SetName(newName).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("rename entry: %w", err)
	}
	if record != 1 {
		return EntryRecord{}, ErrNotFound
	}
	return r.GetEntryByFileID(ctx, driveID, fileID)
}

func (r *Repository) MoveEntry(ctx context.Context, driveID, fileID, targetParentFileID string) (EntryRecord, error) {
	record, err := r.client.Entry.Update().
		Where(entry.DriveIDEQ(driveID), entry.FileIDEQ(fileID)).
		SetParentFileID(targetParentFileID).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("move entry: %w", err)
	}
	if record != 1 {
		return EntryRecord{}, ErrNotFound
	}
	return r.GetEntryByFileID(ctx, driveID, fileID)
}

func (r *Repository) TrashEntry(
	ctx context.Context,
	driveID string,
	fileID string,
	name string,
	recycleBinParentID string,
	trashedParentFileID string,
	trashedAt time.Time,
	expiredAt time.Time,
) (EntryRecord, error) {
	record, err := r.client.Entry.Update().
		Where(entry.DriveIDEQ(driveID), entry.FileIDEQ(fileID)).
		SetName(name).
		SetParentFileID(recycleBinParentID).
		SetTrashedParentFileID(trashedParentFileID).
		SetTrashedAt(trashedAt).
		SetExpiredAt(expiredAt).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("trash entry: %w", err)
	}
	if record != 1 {
		return EntryRecord{}, ErrNotFound
	}
	return r.GetEntryByFileID(ctx, driveID, fileID)
}

func (r *Repository) RestoreEntry(
	ctx context.Context,
	driveID string,
	fileID string,
	name string,
	parentFileID string,
) (EntryRecord, error) {
	record, err := r.client.Entry.Update().
		Where(entry.DriveIDEQ(driveID), entry.FileIDEQ(fileID)).
		SetName(name).
		SetParentFileID(parentFileID).
		ClearTrashedParentFileID().
		ClearTrashedAt().
		ClearExpiredAt().
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("restore entry: %w", err)
	}
	if record != 1 {
		return EntryRecord{}, ErrNotFound
	}
	return r.GetEntryByFileID(ctx, driveID, fileID)
}

func (r *Repository) DeleteEntryTree(ctx context.Context, driveID, fileID string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("delete entry tree: open transaction: %w", err)
	}

	entryIDs, err := r.queryDescendantFileIDs(ctx, tx, driveID, fileID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if len(entryIDs) == 0 {
		_ = tx.Rollback()
		return ErrNotFound
	}

	entryRecords, err := tx.Entry.Query().
		Where(entry.DriveIDEQ(driveID), entry.FileIDIn(entryIDs...)).
		All(ctx)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete entry tree: query tree entries: %w", err)
	}
	if len(entryRecords) == 0 {
		_ = tx.Rollback()
		return ErrNotFound
	}

	uploadIDSet := make(map[string]struct{}, len(entryRecords))
	for _, entryRecord := range entryRecords {
		if entryRecord.UploadID == nil {
			continue
		}
		uploadID := strings.TrimSpace(*entryRecord.UploadID)
		if uploadID == "" {
			continue
		}
		uploadIDSet[uploadID] = struct{}{}
	}

	if len(uploadIDSet) > 0 {
		uploadIDs := make([]string, 0, len(uploadIDSet))
		for uploadID := range uploadIDSet {
			uploadIDs = append(uploadIDs, uploadID)
		}

		if _, err := tx.UploadPart.Delete().Where(uploadpart.UploadIDIn(uploadIDs...)).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete entry tree: delete upload parts: %w", err)
		}
		if _, err := tx.UploadSession.Delete().Where(uploadsession.UploadIDIn(uploadIDs...)).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete entry tree: delete upload sessions: %w", err)
		}
	}

	if _, err := tx.Entry.Delete().Where(entry.DriveIDEQ(driveID), entry.FileIDIn(entryIDs...)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete entry tree: delete entries: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete entry tree: commit transaction: %w", err)
	}
	return nil
}

func (r *Repository) UpdateEntryHash(ctx context.Context, driveID, fileID, contentHash string) (EntryRecord, error) {
	record, err := r.client.Entry.Update().
		Where(entry.DriveIDEQ(driveID), entry.FileIDEQ(fileID)).
		SetContentHash(contentHash).
		Save(ctx)
	if err != nil {
		return EntryRecord{}, fmt.Errorf("update entry hash: %w", err)
	}
	if record != 1 {
		return EntryRecord{}, ErrNotFound
	}
	return r.GetEntryByFileID(ctx, driveID, fileID)
}

func (r *Repository) GetUploadSession(ctx context.Context, driveID, uploadID string) (UploadSessionRecord, error) {
	record, err := r.client.UploadSession.Query().Where(
		uploadsession.DriveIDEQ(driveID),
		uploadsession.UploadIDEQ(uploadID),
	).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return UploadSessionRecord{}, ErrNotFound
		}
		return UploadSessionRecord{}, fmt.Errorf("get upload session: %w", err)
	}
	return mapUploadSessionRecord(record), nil
}

func (r *Repository) GetUploadSessionsByUploadIDs(ctx context.Context, driveID string, uploadIDs []string) (map[string]UploadSessionRecord, error) {
	result := make(map[string]UploadSessionRecord)
	if len(uploadIDs) == 0 {
		return result, nil
	}

	records, err := r.client.UploadSession.Query().Where(
		uploadsession.DriveIDEQ(driveID),
		uploadsession.UploadIDIn(uploadIDs...),
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("get upload sessions by ids: %w", err)
	}
	for _, record := range records {
		mapped := mapUploadSessionRecord(record)
		result[mapped.UploadID] = mapped
	}
	return result, nil
}

func (r *Repository) EnsureUploadParts(ctx context.Context, uploadID string, partNumbers []int) error {
	maxPartNumber := 0
	for _, partNumber := range partNumbers {
		if partNumber > maxPartNumber {
			maxPartNumber = partNumber
		}
		_, err := r.client.UploadPart.Create().
			SetUploadID(uploadID).
			SetPartNumber(partNumber).
			SetStatus(uploadpart.StatusPending).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				continue
			}
			return fmt.Errorf("ensure upload parts: create part: %w", err)
		}
	}

	if maxPartNumber > 0 {
		if _, err := r.client.UploadSession.Update().
			Where(uploadsession.UploadIDEQ(uploadID), uploadsession.PartCountLT(maxPartNumber)).
			SetPartCount(maxPartNumber).
			Save(ctx); err != nil {
			return fmt.Errorf("ensure upload parts: update part count: %w", err)
		}
	}
	return nil
}

func (r *Repository) MarkUploadPartUploaded(ctx context.Context, uploadID string, partNumber int, size int64, etag string) error {
	count, err := r.client.UploadPart.Update().
		Where(uploadpart.UploadIDEQ(uploadID), uploadpart.PartNumberEQ(partNumber)).
		SetSize(size).
		SetEtag(etag).
		SetStatus(uploadpart.StatusUploaded).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("mark upload part uploaded: %w", err)
	}
	if count > 0 {
		return nil
	}

	_, err = r.client.UploadPart.Create().
		SetUploadID(uploadID).
		SetPartNumber(partNumber).
		SetSize(size).
		SetEtag(etag).
		SetStatus(uploadpart.StatusUploaded).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("mark upload part uploaded: create fallback: %w", err)
	}
	return nil
}

func (r *Repository) SetUploadSessionStatus(ctx context.Context, uploadID, status string) error {
	var enum uploadsession.Status
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "init":
		enum = uploadsession.StatusInit
	case "uploading":
		enum = uploadsession.StatusUploading
	case "completed":
		enum = uploadsession.StatusCompleted
	case "aborted":
		enum = uploadsession.StatusAborted
	default:
		return fmt.Errorf("set upload session status: unsupported status %s", status)
	}

	count, err := r.client.UploadSession.Update().
		Where(uploadsession.UploadIDEQ(uploadID)).
		SetStatus(enum).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("set upload session status: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) queryDescendantFileIDs(
	ctx context.Context,
	tx *ent.Tx,
	driveID string,
	rootFileID string,
) ([]string, error) {
	rootFileID = strings.TrimSpace(rootFileID)
	if rootFileID == "" {
		return nil, fmt.Errorf("delete entry tree: empty file_id")
	}

	ids := []string{rootFileID}
	seen := map[string]struct{}{
		rootFileID: {},
	}
	queue := []string{rootFileID}

	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]

		children, err := tx.Entry.Query().
			Where(entry.DriveIDEQ(driveID), entry.ParentFileIDEQ(parentID)).
			Select(entry.FieldFileID).
			Strings(ctx)
		if err != nil {
			return nil, fmt.Errorf("delete entry tree: query child entries: %w", err)
		}

		for _, childID := range children {
			if _, ok := seen[childID]; ok {
				continue
			}
			seen[childID] = struct{}{}
			ids = append(ids, childID)
			queue = append(queue, childID)
		}
	}

	return ids, nil
}

func mapEntryRecord(record *ent.Entry) EntryRecord {
	contentHash := ""
	if record.ContentHash != nil {
		contentHash = *record.ContentHash
	}
	preHash := ""
	if record.PreHash != nil {
		preHash = *record.PreHash
	}
	uploadID := ""
	if record.UploadID != nil {
		uploadID = *record.UploadID
	}
	trashedParentFileID := ""
	if record.TrashedParentFileID != nil {
		trashedParentFileID = *record.TrashedParentFileID
	}

	return EntryRecord{
		DriveID:             record.DriveID,
		FileID:              record.FileID,
		ParentFileID:        record.ParentFileID,
		Name:                record.Name,
		Type:                string(record.Type),
		Size:                record.Size,
		ContentHash:         contentHash,
		PreHash:             preHash,
		UploadID:            uploadID,
		TrashedParentFileID: trashedParentFileID,
		RevisionID:          record.RevisionID,
		EncryptMode:         record.EncryptMode,
		TrashedAt:           record.TrashedAt,
		ExpiredAt:           record.ExpiredAt,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}

func mapUploadSessionRecord(record *ent.UploadSession) UploadSessionRecord {
	return UploadSessionRecord{
		DriveID:   record.DriveID,
		UploadID:  record.UploadID,
		FileID:    record.FileID,
		PartCount: record.PartCount,
		ChunkSize: record.ChunkSize,
		ExpiresAt: record.ExpiresAt,
		Status:    string(record.Status),
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

func mapUploadPartRecord(record *ent.UploadPart) UploadPartRecord {
	eTag := ""
	if record.Etag != nil {
		eTag = *record.Etag
	}
	return UploadPartRecord{
		UploadID:   record.UploadID,
		PartNumber: record.PartNumber,
		Size:       record.Size,
		ETag:       eTag,
		Status:     string(record.Status),
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
}
