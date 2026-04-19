package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"molly-server/ent"
	"molly-server/ent/drive"
	"molly-server/ent/entry"
	"molly-server/ent/uploadpart"
	"molly-server/ent/uploadsession"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("upload repository: not found")
	ErrConflict = errors.New("upload repository: conflict")
)

type Repository struct {
	client *ent.Client
	db     *sql.DB
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
	InternalID          int
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

type SubtreeStats struct {
	Size        int64
	FileCount   int64
	FolderCount int64
}

func New(client *ent.Client, db *sql.DB) *Repository {
	return &Repository{
		client: client,
		db:     db,
	}
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

func (r *Repository) GetSubtreeStats(ctx context.Context, driveID, folderID string) (SubtreeStats, error) {
	if r.db == nil {
		return SubtreeStats{}, fmt.Errorf("get subtree stats: database handle is not configured")
	}

	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return SubtreeStats{}, fmt.Errorf("get subtree stats: empty folder id")
	}

	const query = `
WITH RECURSIVE subtree AS (
    SELECT e.file_id, e.parent_file_id, e.type, e.size, e.upload_id
    FROM entries e
    WHERE e.drive_id = $1
      AND (
          ($2 = 'root' AND e.parent_file_id = 'root')
          OR e.file_id = $2
      )
    UNION ALL
    SELECT e.file_id, e.parent_file_id, e.type, e.size, e.upload_id
    FROM entries e
    INNER JOIN subtree s ON e.parent_file_id = s.file_id
    WHERE e.drive_id = $1
),
visible AS (
    SELECT s.*
    FROM subtree s
    LEFT JOIN upload_sessions us
      ON us.drive_id = $1
     AND us.upload_id = s.upload_id
    WHERE s.type <> 'file'
       OR s.upload_id IS NULL
       OR LOWER(COALESCE(us.status, '')) = 'completed'
)
SELECT
    COALESCE(SUM(CASE WHEN type = 'file' THEN size ELSE 0 END), 0) AS total_size,
    COALESCE(SUM(CASE WHEN type = 'file' THEN 1 ELSE 0 END), 0) AS file_count,
    GREATEST(COALESCE(SUM(CASE WHEN type = 'folder' THEN 1 ELSE 0 END), 0) - CASE WHEN $2 = 'root' THEN 0 ELSE 1 END, 0) AS folder_count
FROM visible;
`

	var stats SubtreeStats
	if err := r.db.QueryRowContext(ctx, query, driveID, folderID).Scan(&stats.Size, &stats.FileCount, &stats.FolderCount); err != nil {
		return SubtreeStats{}, fmt.Errorf("get subtree stats: %w", err)
	}

	return stats, nil
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
	const query = `
UPDATE entries
SET name = $3, updated_at = NOW()
WHERE drive_id = $1 AND file_id = $2
RETURNING id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at;
`

	record, err := scanEntryRecord(r.db.QueryRowContext(ctx, query, driveID, fileID, newName))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryRecord{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("rename entry: %w", err)
	}
	return record, nil
}

func (r *Repository) MoveEntry(ctx context.Context, driveID, fileID, targetParentFileID string) (EntryRecord, error) {
	const query = `
UPDATE entries
SET parent_file_id = $3, updated_at = NOW()
WHERE drive_id = $1 AND file_id = $2
RETURNING id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at;
`

	record, err := scanEntryRecord(r.db.QueryRowContext(ctx, query, driveID, fileID, targetParentFileID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryRecord{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("move entry: %w", err)
	}
	return record, nil
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
	const query = `
UPDATE entries
SET name = $3,
    parent_file_id = $4,
    trashed_parent_file_id = $5,
    trashed_at = $6,
    expired_at = $7,
    updated_at = NOW()
WHERE drive_id = $1 AND file_id = $2
RETURNING id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at;
`

	record, err := scanEntryRecord(
		r.db.QueryRowContext(ctx, query, driveID, fileID, name, recycleBinParentID, trashedParentFileID, trashedAt, expiredAt),
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryRecord{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("trash entry: %w", err)
	}
	return record, nil
}

func (r *Repository) RestoreEntry(
	ctx context.Context,
	driveID string,
	fileID string,
	name string,
	parentFileID string,
) (EntryRecord, error) {
	const query = `
UPDATE entries
SET name = $3,
    parent_file_id = $4,
    trashed_parent_file_id = NULL,
    trashed_at = NULL,
    expired_at = NULL,
    updated_at = NOW()
WHERE drive_id = $1 AND file_id = $2
RETURNING id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at;
`

	record, err := scanEntryRecord(r.db.QueryRowContext(ctx, query, driveID, fileID, name, parentFileID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryRecord{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return EntryRecord{}, ErrConflict
		}
		return EntryRecord{}, fmt.Errorf("restore entry: %w", err)
	}
	return record, nil
}

func (r *Repository) DeleteEntryTree(ctx context.Context, driveID, fileID string) ([]EntryRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("delete entry tree: database handle is not configured")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("delete entry tree: open transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const selectQuery = `
WITH RECURSIVE subtree AS (
    SELECT id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at
    FROM entries
    WHERE drive_id = $1 AND file_id = $2
    UNION ALL
    SELECT e.id, e.drive_id, e.file_id, e.parent_file_id, e.name, e.type, e.size, e.content_hash, e.pre_hash, e.upload_id, e.trashed_parent_file_id, e.trashed_at, e.expired_at, e.revision_id, e.encrypt_mode, e.created_at, e.updated_at
    FROM entries e
    INNER JOIN subtree s ON e.parent_file_id = s.file_id
    WHERE e.drive_id = $1
)
SELECT id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at
FROM subtree;
`

	rows, err := tx.QueryContext(ctx, selectQuery, driveID, fileID)
	if err != nil {
		return nil, fmt.Errorf("delete entry tree: query tree entries: %w", err)
	}
	defer rows.Close()

	deletedRecords := make([]EntryRecord, 0, 8)
	for rows.Next() {
		record, scanErr := scanEntryRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("delete entry tree: scan tree entry: %w", scanErr)
		}
		deletedRecords = append(deletedRecords, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delete entry tree: iterate tree entries: %w", err)
	}
	if len(deletedRecords) == 0 {
		return nil, ErrNotFound
	}

	uploadIDSet := make(map[string]struct{}, len(deletedRecords))
	for _, entryRecord := range deletedRecords {
		uploadID := strings.TrimSpace(entryRecord.UploadID)
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

		if err := deleteByStringValues(ctx, tx, "upload_parts", "upload_id", uploadIDs); err != nil {
			return nil, fmt.Errorf("delete entry tree: delete upload parts: %w", err)
		}
		if err := deleteByStringValues(ctx, tx, "upload_sessions", "upload_id", uploadIDs); err != nil {
			return nil, fmt.Errorf("delete entry tree: delete upload sessions: %w", err)
		}
	}

	const deleteEntriesQuery = `
WITH RECURSIVE subtree AS (
    SELECT file_id
    FROM entries
    WHERE drive_id = $1 AND file_id = $2
    UNION ALL
    SELECT e.file_id
    FROM entries e
    INNER JOIN subtree s ON e.parent_file_id = s.file_id
    WHERE e.drive_id = $1
)
DELETE FROM entries e
USING subtree s
WHERE e.drive_id = $1 AND e.file_id = s.file_id;
`
	if _, err := tx.ExecContext(ctx, deleteEntriesQuery, driveID, fileID); err != nil {
		return nil, fmt.Errorf("delete entry tree: delete entries: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("delete entry tree: commit transaction: %w", err)
	}
	return deletedRecords, nil
}

func (r *Repository) UpdateEntryHash(ctx context.Context, driveID, fileID, contentHash string) (EntryRecord, error) {
	const query = `
UPDATE entries
SET content_hash = $3, updated_at = NOW()
WHERE drive_id = $1 AND file_id = $2
RETURNING id, drive_id, file_id, parent_file_id, name, type, size, content_hash, pre_hash, upload_id, trashed_parent_file_id, trashed_at, expired_at, revision_id, encrypt_mode, created_at, updated_at;
`

	record, err := scanEntryRecord(r.db.QueryRowContext(ctx, query, driveID, fileID, contentHash))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EntryRecord{}, ErrNotFound
		}
		return EntryRecord{}, fmt.Errorf("update entry hash: %w", err)
	}
	return record, nil
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func scanEntryRecord(scanner interface{ Scan(dest ...any) error }) (EntryRecord, error) {
	var (
		record              EntryRecord
		contentHash         sql.NullString
		preHash             sql.NullString
		uploadID            sql.NullString
		trashedParentFileID sql.NullString
		trashedAt           sql.NullTime
		expiredAt           sql.NullTime
	)

	if err := scanner.Scan(
		&record.InternalID,
		&record.DriveID,
		&record.FileID,
		&record.ParentFileID,
		&record.Name,
		&record.Type,
		&record.Size,
		&contentHash,
		&preHash,
		&uploadID,
		&trashedParentFileID,
		&trashedAt,
		&expiredAt,
		&record.RevisionID,
		&record.EncryptMode,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return EntryRecord{}, err
	}

	record.ContentHash = nullStringValue(contentHash)
	record.PreHash = nullStringValue(preHash)
	record.UploadID = nullStringValue(uploadID)
	record.TrashedParentFileID = nullStringValue(trashedParentFileID)
	record.TrashedAt = nullTimePtr(trashedAt)
	record.ExpiredAt = nullTimePtr(expiredAt)

	return record, nil
}

func nullStringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	normalized := value.Time
	return &normalized
}

func deleteByStringValues(ctx context.Context, tx *sql.Tx, tableName, columnName string, values []string) error {
	if len(values) == 0 {
		return nil
	}

	args := make([]any, 0, len(values))
	placeholders := make([]string, 0, len(values))
	for index, value := range values {
		args = append(args, value)
		placeholders = append(placeholders, fmt.Sprintf("$%d", index+1))
	}

	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN (%s)",
		tableName,
		columnName,
		strings.Join(placeholders, ", "),
	)

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return nil
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
		InternalID:          record.ID,
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
