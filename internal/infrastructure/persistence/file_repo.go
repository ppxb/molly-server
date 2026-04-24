package persistence

import (
	"context"
	"strconv"
	"time"

	domain "molly-server/internal/domain/file"
	ent "molly-server/internal/ent/gen"
	entfile "molly-server/internal/ent/gen/fileinfo"
	entuptask "molly-server/internal/ent/gen/uploadtask"
	entuserfile "molly-server/internal/ent/gen/userfile"
	entvpath "molly-server/internal/ent/gen/virtualpath"
)

// ── FileInfoRepo ─────────────────────────────────────────────

type fileInfoRepo struct{ client *ent.Client }

func NewFileInfoRepo(client *ent.Client) domain.FileInfoRepository {
	return &fileInfoRepo{client: client}
}

func (r *fileInfoRepo) Create(ctx context.Context, f *domain.FileInfo) error {
	_, err := r.client.FileInfo.Create().
		SetID(f.ID).
		SetName(f.Name).
		SetRandomName(f.RandomName).
		SetSize(f.Size).
		SetMime(f.Mime).
		SetPath(f.Path).
		SetEncPath(f.EncPath).
		SetNillableThumbnailImg(nilStr(f.ThumbnailImg)).
		SetFileHash(f.FileHash).
		SetNillableFileEncHash(nilStr(f.FileEncHash)).
		SetNillableChunkSignature(nilStr(f.ChunkSignature)).
		SetNillableFirstChunkHash(nilStr(f.FirstChunkHash)).
		SetNillableSecondChunkHash(nilStr(f.SecondChunkHash)).
		SetNillableThirdChunkHash(nilStr(f.ThirdChunkHash)).
		SetHasFullHash(f.HasFullHash).
		SetIsEnc(f.IsEnc).
		SetIsChunk(f.IsChunk).
		SetChunkCount(f.ChunkCount).
		Save(ctx)
	return err
}

func (r *fileInfoRepo) GetByID(ctx context.Context, id string) (*domain.FileInfo, error) {
	e, err := r.client.FileInfo.Get(ctx, id)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toFileInfo(e), nil
}

func (r *fileInfoRepo) GetByChunkSignature(ctx context.Context, sig string, size int64) (*domain.FileInfo, error) {
	e, err := r.client.FileInfo.Query().
		Where(entfile.ChunkSignature(sig), entfile.Size(size)).
		Only(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toFileInfo(e), nil
}

func (r *fileInfoRepo) GetByHash(ctx context.Context, hash string) (*domain.FileInfo, error) {
	e, err := r.client.FileInfo.Query().
		Where(entfile.FileHash(hash)).
		Only(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toFileInfo(e), nil
}

func (r *fileInfoRepo) Update(ctx context.Context, f *domain.FileInfo) error {
	return r.client.FileInfo.UpdateOneID(f.ID).
		SetName(f.Name).
		SetSize(f.Size).
		SetMime(f.Mime).
		SetPath(f.Path).
		SetFileHash(f.FileHash).
		SetHasFullHash(f.HasFullHash).
		SetIsEnc(f.IsEnc).
		Exec(ctx)
}

func (r *fileInfoRepo) Delete(ctx context.Context, id string) error {
	return r.client.FileInfo.DeleteOneID(id).Exec(ctx)
}

// ── UserFileRepo ─────────────────────────────────────────────

type userFileRepo struct{ client *ent.Client }

func NewUserFileRepo(client *ent.Client) domain.UserFileRepository {
	return &userFileRepo{client: client}
}

func (r *userFileRepo) Create(ctx context.Context, uf *domain.UserFile) error {
	_, err := r.client.UserFile.Create().
		SetID(uf.ID).
		SetUserID(uf.UserID).
		SetFileID(uf.FileID).
		SetFileName(uf.FileName).
		SetVirtualPath(uf.VirtualPath).
		SetIsPublic(uf.IsPublic).
		Save(ctx)
	return err
}

func (r *userFileRepo) GetByID(ctx context.Context, id string) (*domain.UserFile, error) {
	e, err := r.client.UserFile.Get(ctx, id)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toUserFile(e), nil
}

func (r *userFileRepo) GetByUserAndUfID(ctx context.Context, userID, ufID string) (*domain.UserFile, error) {
	e, err := r.client.UserFile.Query().
		Where(entuserfile.ID(ufID), entuserfile.UserID(userID)).
		Only(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toUserFile(e), nil
}

func (r *userFileRepo) Update(ctx context.Context, uf *domain.UserFile) error {
	u := r.client.UserFile.UpdateOneID(uf.ID).
		SetFileName(uf.FileName).
		SetVirtualPath(uf.VirtualPath).
		SetIsPublic(uf.IsPublic)
	return u.Exec(ctx)
}

func (r *userFileRepo) SoftDelete(ctx context.Context, id string) error {
	now := time.Now()
	return r.client.UserFile.UpdateOneID(id).
		SetDeletedAt(now).
		Exec(ctx)
}

func (r *userFileRepo) Restore(ctx context.Context, uf *domain.UserFile) error {
	return r.client.UserFile.UpdateOneID(uf.ID).
		ClearDeletedAt().
		SetVirtualPath(uf.VirtualPath).
		Exec(ctx)
}

func (r *userFileRepo) HardDelete(ctx context.Context, id string) error {
	return r.client.UserFile.DeleteOneID(id).Exec(ctx)
}

func (r *userFileRepo) ListByVirtualPath(ctx context.Context, userID, pathID string, offset, limit int) ([]*domain.UserFile, error) {
	es, err := r.client.UserFile.Query().
		Where(
			entuserfile.UserID(userID),
			entuserfile.VirtualPath(pathID),
			entuserfile.DeletedAtIsNil(),
		).
		Order(ent.Desc(entuserfile.FieldCreatedAt)).
		Offset(offset).Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapUserFiles(es), nil
}

func (r *userFileRepo) CountByVirtualPath(ctx context.Context, userID, pathID string) (int64, error) {
	n, err := r.client.UserFile.Query().
		Where(
			entuserfile.UserID(userID),
			entuserfile.VirtualPath(pathID),
			entuserfile.DeletedAtIsNil(),
		).Count(ctx)
	return int64(n), err
}

func (r *userFileRepo) Search(ctx context.Context, userID, keyword string, offset, limit int) ([]*domain.UserFile, error) {
	es, err := r.client.UserFile.Query().
		Where(
			entuserfile.UserID(userID),
			entuserfile.FileNameContains(keyword),
			entuserfile.DeletedAtIsNil(),
		).
		Offset(offset).Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapUserFiles(es), nil
}

func (r *userFileRepo) CountSearch(ctx context.Context, userID, keyword string) (int64, error) {
	n, err := r.client.UserFile.Query().
		Where(
			entuserfile.UserID(userID),
			entuserfile.FileNameContains(keyword),
			entuserfile.DeletedAtIsNil(),
		).Count(ctx)
	return int64(n), err
}

func (r *userFileRepo) ListPublic(ctx context.Context, offset, limit int) ([]*domain.UserFile, error) {
	es, err := r.client.UserFile.Query().
		Where(entuserfile.IsPublic(true), entuserfile.DeletedAtIsNil()).
		Offset(offset).Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapUserFiles(es), nil
}

func (r *userFileRepo) CountPublic(ctx context.Context) (int64, error) {
	n, err := r.client.UserFile.Query().
		Where(entuserfile.IsPublic(true), entuserfile.DeletedAtIsNil()).
		Count(ctx)
	return int64(n), err
}

// ── VirtualPathRepo ──────────────────────────────────────────

type virtualPathRepo struct{ client *ent.Client }

func NewVirtualPathRepo(client *ent.Client) domain.VirtualPathRepository {
	return &virtualPathRepo{client: client}
}

func (r *virtualPathRepo) Create(ctx context.Context, vp *domain.VirtualPath) error {
	_, err := r.client.VirtualPath.Create().
		SetUserID(vp.UserID).
		SetPath(vp.Path).
		SetIsFile(vp.IsFile).
		SetIsDir(vp.IsDir).
		SetParentLevel(vp.ParentLevel).
		Save(ctx)
	return err
}

func (r *virtualPathRepo) GetByID(ctx context.Context, id int) (*domain.VirtualPath, error) {
	e, err := r.client.VirtualPath.Get(ctx, id)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrDirNotFound)
	}
	return toVirtualPath(e), nil
}

func (r *virtualPathRepo) GetByPath(ctx context.Context, userID, path string) (*domain.VirtualPath, error) {
	e, err := r.client.VirtualPath.Query().
		Where(entvpath.UserID(userID), entvpath.Path(path)).
		Only(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrDirNotFound)
	}
	return toVirtualPath(e), nil
}

func (r *virtualPathRepo) GetRoot(ctx context.Context, userID string) (*domain.VirtualPath, error) {
	e, err := r.client.VirtualPath.Query().
		Where(entvpath.UserID(userID), entvpath.ParentLevel("")).
		First(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrDirNotFound)
	}
	return toVirtualPath(e), nil
}

func (r *virtualPathRepo) Update(ctx context.Context, vp *domain.VirtualPath) error {
	return r.client.VirtualPath.UpdateOneID(vp.ID).
		SetPath(vp.Path).
		SetParentLevel(vp.ParentLevel).
		Exec(ctx)
}

func (r *virtualPathRepo) Delete(ctx context.Context, id int) error {
	return r.client.VirtualPath.DeleteOneID(id).Exec(ctx)
}

func (r *virtualPathRepo) ListSubFolders(ctx context.Context, userID string, parentID int, offset, limit int) ([]*domain.VirtualPath, error) {
	parentIDStr := strconv.Itoa(parentID)
	es, err := r.client.VirtualPath.Query().
		Where(
			entvpath.UserID(userID),
			entvpath.ParentLevel(parentIDStr),
			entvpath.IsDir(true),
		).
		Order(ent.Asc(entvpath.FieldPath)).
		Offset(offset).Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapVirtualPaths(es), nil
}

func (r *virtualPathRepo) CountSubFolders(ctx context.Context, userID string, parentID int) (int64, error) {
	parentIDStr := strconv.Itoa(parentID)
	n, err := r.client.VirtualPath.Query().
		Where(
			entvpath.UserID(userID),
			entvpath.ParentLevel(parentIDStr),
			entvpath.IsDir(true),
		).Count(ctx)
	return int64(n), err
}

func (r *virtualPathRepo) ListAll(ctx context.Context, userID string) ([]*domain.VirtualPath, error) {
	es, err := r.client.VirtualPath.Query().
		Where(entvpath.UserID(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapVirtualPaths(es), nil
}

// ── UploadTaskRepo ───────────────────────────────────────────

type uploadTaskRepo struct{ client *ent.Client }

func NewUploadTaskRepo(client *ent.Client) domain.UploadTaskRepository {
	return &uploadTaskRepo{client: client}
}

func (r *uploadTaskRepo) Create(ctx context.Context, t *domain.UploadTask) error {
	_, err := r.client.UploadTask.Create().
		SetID(t.ID).
		SetUserID(t.UserID).
		SetFileName(t.FileName).
		SetFileSize(t.FileSize).
		SetChunkSize(t.ChunkSize).
		SetTotalChunks(t.TotalChunks).
		SetUploadedChunks(t.UploadedChunks).
		SetChunkSignature(t.ChunkSignature).
		SetPathID(t.PathID).
		SetTempDir(t.TempDir).
		SetStatus(string(t.Status)).
		SetExpiresAt(t.ExpiresAt).
		Save(ctx)
	return err
}

func (r *uploadTaskRepo) GetByID(ctx context.Context, id string) (*domain.UploadTask, error) {
	e, err := r.client.UploadTask.Get(ctx, id)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrTaskNotFound)
	}
	return toUploadTask(e), nil
}

func (r *uploadTaskRepo) Update(ctx context.Context, t *domain.UploadTask) error {
	return r.client.UploadTask.UpdateOneID(t.ID).
		SetUploadedChunks(t.UploadedChunks).
		SetStatus(string(t.Status)).
		SetErrorMessage(t.ErrorMessage).
		SetTempDir(t.TempDir).
		SetExpiresAt(t.ExpiresAt).
		Exec(ctx)
}

func (r *uploadTaskRepo) Delete(ctx context.Context, id string) error {
	return r.client.UploadTask.DeleteOneID(id).Exec(ctx)
}

func (r *uploadTaskRepo) ListByUser(ctx context.Context, userID string, offset, limit int) ([]*domain.UploadTask, error) {
	es, err := r.client.UploadTask.Query().
		Where(entuptask.UserID(userID)).
		Order(ent.Desc(entuptask.FieldCreatedAt)).
		Offset(offset).Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapUploadTasks(es), nil
}

func (r *uploadTaskRepo) CountByUser(ctx context.Context, userID string) (int64, error) {
	n, err := r.client.UploadTask.Query().
		Where(entuptask.UserID(userID)).Count(ctx)
	return int64(n), err
}

func (r *uploadTaskRepo) ListPendingByUser(ctx context.Context, userID string) ([]*domain.UploadTask, error) {
	es, err := r.client.UploadTask.Query().
		Where(
			entuptask.UserID(userID),
			entuptask.StatusIn("pending", "uploading"),
			entuptask.ExpiresAtGT(time.Now()),
		).
		Order(ent.Desc(entuptask.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapUploadTasks(es), nil
}

func (r *uploadTaskRepo) ListExpiredByUser(ctx context.Context, userID string) ([]*domain.UploadTask, error) {
	es, err := r.client.UploadTask.Query().
		Where(
			entuptask.UserID(userID),
			entuptask.StatusIn("pending", "uploading", "aborted"),
			entuptask.ExpiresAtLT(time.Now()),
		).All(ctx)
	if err != nil {
		return nil, err
	}
	return mapUploadTasks(es), nil
}

func (r *uploadTaskRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.client.UploadTask.Delete().
		Where(
			entuptask.ExpiresAtLT(before),
			entuptask.StatusIn("pending", "uploading", "aborted"),
		).Exec(ctx)
	return int64(n), err
}

// ── 映射函数 ─────────────────────────────────────────────────

func toFileInfo(e *ent.FileInfo) *domain.FileInfo {
	return &domain.FileInfo{
		ID: e.ID, Name: e.Name, RandomName: e.RandomName,
		Size: e.Size, Mime: e.Mime, Path: e.Path, EncPath: e.EncPath,
		ThumbnailImg: e.ThumbnailImg,
		FileHash:     e.FileHash, FileEncHash: e.FileEncHash,
		ChunkSignature: e.ChunkSignature,
		FirstChunkHash: e.FirstChunkHash, SecondChunkHash: e.SecondChunkHash, ThirdChunkHash: e.ThirdChunkHash,
		HasFullHash: e.HasFullHash, IsEnc: e.IsEnc,
		IsChunk: e.IsChunk, ChunkCount: e.ChunkCount,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func toUserFile(e *ent.UserFile) *domain.UserFile {
	return &domain.UserFile{
		ID: e.ID, UserID: e.UserID, FileID: e.FileID,
		FileName: e.FileName, VirtualPath: e.VirtualPath,
		IsPublic: e.IsPublic, CreatedAt: e.CreatedAt, DeletedAt: e.DeletedAt,
	}
}

func toVirtualPath(e *ent.VirtualPath) *domain.VirtualPath {
	return &domain.VirtualPath{
		ID: e.ID, UserID: e.UserID, Path: e.Path,
		IsFile: e.IsFile, IsDir: e.IsDir, ParentLevel: e.ParentLevel,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func toUploadTask(e *ent.UploadTask) *domain.UploadTask {
	return &domain.UploadTask{
		ID: e.ID, UserID: e.UserID, FileName: e.FileName,
		FileSize: e.FileSize, ChunkSize: e.ChunkSize,
		TotalChunks: e.TotalChunks, UploadedChunks: e.UploadedChunks,
		ChunkSignature: e.ChunkSignature, PathID: e.PathID, TempDir: e.TempDir,
		Status: domain.UploadStatus(e.Status), ErrorMessage: e.ErrorMessage,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, ExpiresAt: e.ExpiresAt,
	}
}

func mapUserFiles(es []*ent.UserFile) []*domain.UserFile {
	out := make([]*domain.UserFile, len(es))
	for i, e := range es {
		out[i] = toUserFile(e)
	}
	return out
}

func mapVirtualPaths(es []*ent.VirtualPath) []*domain.VirtualPath {
	out := make([]*domain.VirtualPath, len(es))
	for i, e := range es {
		out[i] = toVirtualPath(e)
	}
	return out
}

func mapUploadTasks(es []*ent.UploadTask) []*domain.UploadTask {
	out := make([]*domain.UploadTask, len(es))
	for i, e := range es {
		out[i] = toUploadTask(e)
	}
	return out
}

// nilStr 将空字符串转换为 nil 指针（用于 Optional 字段）。
func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
